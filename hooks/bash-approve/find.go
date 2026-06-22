package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// findExecFlags are find flags that accept a command terminated by ; or +.
var findExecFlags = map[string]bool{
	"-exec":    true,
	"-execdir": true,
	"-ok":      true,
	"-okdir":   true,
}

// findCwdChangingExecFlags are exec flags that run the command from the
// matched file's directory. The recursive validator can't model that
// changed cwd, so any -execdir/-okdir invocation drops to ask regardless
// of the embedded command.
var findCwdChangingExecFlags = map[string]bool{
	"-execdir": true,
	"-okdir":   true,
}

// findDangerousFlags are find actions that modify the filesystem.
// `-fprint`/`-fprint0`/`-fprintf`/`-fls` open the next argument for
// writing — find treats them as actions that emit results to a file
// rather than stdout, so an attacker-supplied path argument (e.g.
// `~/.bashrc`) can be created or truncated without going through the
// redirect/tee validators.
var findDangerousFlags = map[string]bool{
	"-delete":  true,
	"-fprint":  true,
	"-fprint0": true,
	"-fprintf": true,
	"-fls":     true,
}

var findTrustedPathParams = map[string]bool{
	"HOME":           true,
	"TMPDIR":         true,
	"TMP":            true,
	"TEMP":           true,
	"XDG_CACHE_HOME": true,
}

var findTrustedGoEnvPathVars = map[string]bool{
	"GOCACHE":    true,
	"GOMODCACHE": true,
	"GOPATH":     true,
	"GOROOT":     true,
}

// isFindSafe validates a find invocation by checking -exec/-execdir
// commands through the normal evaluation pipeline. Returns false (→ ask
// via validateFallback) if any embedded command is unrecognized, if a
// destructive action is present, or if -execdir / -okdir is used (cwd
// changes at runtime can't be modeled).
//
// `{}` placeholders are passed through to the evaluator as literal
// arguments. Read-only inner commands (cat, rg, wc, grep, …) treat the
// substituted path the same as any other path argument and stay
// auto-approve. Mutating commands (rm, mv, plain cp, tee, …) don't
// match the read-only patterns, so they still drop to ask.
func isFindSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}

	expressionStarted := false
	for i := 1; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			if !expressionStarted && isFindStartPathArgSafe(args[i]) {
				continue
			}
			// Non-literal find arg (e.g. `find . $(printf %s -delete)`)
			// can expand into a dangerous predicate at runtime.
			return false
		}

		if !expressionStarted && isFindExpressionStart(lit) {
			expressionStarted = true
		}

		if findDangerousFlags[lit] {
			return false
		}

		if !findExecFlags[lit] {
			continue
		}

		if findCwdChangingExecFlags[lit] {
			return false
		}

		// Collect the command between -exec and the terminator (; or +).
		i++
		var cmdWords []*syntax.Word
		for i < len(args) {
			part := wordLiteral(args[i])
			if part == "" {
				return false
			}
			if part == ";" || part == "+" {
				break
			}
			cmdWords = append(cmdWords, args[i])
			i++
		}

		if len(cmdWords) == 0 {
			return false
		}

		cmd := argsText(cmdWords)
		r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
		if r == nil {
			return false
		}
		if r.decision != decisionAllow {
			return false
		}
	}

	return true
}

func isFindExpressionStart(lit string) bool {
	return strings.HasPrefix(lit, "-") || lit == "(" || lit == ")" || lit == "!" || lit == ","
}

func isFindStartPathArgSafe(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	ok, pathShaped, trustedPathProducer := inspectFindStartPathParts(w.Parts, false)
	return ok && (pathShaped || trustedPathProducer)
}

func inspectFindStartPathParts(parts []syntax.WordPart, quoted bool) (ok, pathShaped, trustedPathProducer bool) {
	ok = true
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.Contains(stripUnquotedBackslashes(p.Value), "/") {
				pathShaped = true
			}
		case *syntax.SglQuoted:
			if strings.Contains(p.Value, "/") {
				pathShaped = true
			}
		case *syntax.DblQuoted:
			childOK, childPathShaped, childTrusted := inspectFindStartPathParts(p.Parts, true)
			if !childOK {
				return false, false, false
			}
			pathShaped = pathShaped || childPathShaped
			trustedPathProducer = trustedPathProducer || childTrusted
		case *syntax.CmdSubst:
			if !quoted {
				return false, false, false
			}
			if isTrustedFindPathCmdSubst(p) {
				trustedPathProducer = true
			}
		case *syntax.ParamExp:
			if !quoted {
				return false, false, false
			}
			if isTrustedFindPathParamExp(p) {
				trustedPathProducer = true
			}
		default:
			return false, false, false
		}
	}
	return ok, pathShaped, trustedPathProducer
}

func isTrustedFindPathCmdSubst(cs *syntax.CmdSubst) bool {
	if cs == nil || len(cs.Stmts) != 1 {
		return false
	}
	stmt := cs.Stmts[0]
	if len(stmt.Redirs) > 0 || stmt.Cmd == nil {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) != 3 {
		return false
	}
	if wordLiteral(call.Args[0]) != "go" || wordLiteral(call.Args[1]) != "env" {
		return false
	}
	return findTrustedGoEnvPathVars[wordLiteral(call.Args[2])]
}

func isTrustedFindPathParamExp(p *syntax.ParamExp) bool {
	if p == nil || p.Param == nil {
		return false
	}
	if p.Excl || p.Length || p.Width || p.Index != nil || p.Slice != nil ||
		p.Repl != nil || p.Names != 0 {
		return false
	}
	if p.Exp != nil {
		if p.Exp.Op != syntax.DefaultUnset && p.Exp.Op != syntax.DefaultUnsetOrNull {
			return false
		}
		if p.Exp.Word == nil || !wordPartsHaveLiteralSlash(p.Exp.Word.Parts) {
			return false
		}
	}
	return findTrustedPathParams[p.Param.Value]
}

func wordPartsHaveLiteralSlash(parts []syntax.WordPart) bool {
	for _, part := range parts {
		switch p := part.(type) {
		case *syntax.Lit:
			if strings.Contains(stripUnquotedBackslashes(p.Value), "/") {
				return true
			}
		case *syntax.SglQuoted:
			if strings.Contains(p.Value, "/") {
				return true
			}
		case *syntax.DblQuoted:
			if wordPartsHaveLiteralSlash(p.Parts) {
				return true
			}
		}
	}
	return false
}

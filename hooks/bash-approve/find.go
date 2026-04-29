package main

import (
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

	for i := 1; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			// Non-literal find arg (e.g. `find . $(printf %s -delete)`)
			// can expand into a dangerous predicate at runtime.
			return false
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

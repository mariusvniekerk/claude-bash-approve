package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// sedSpec marks -e/-f as value-taking so parseArgs consumes their argument
// (otherwise a script like `s/foo/-i/` would be scanned as a flag cluster and
// false-positive on the in-place check).
var sedSpec = flagSpec{
	short: map[byte]string{
		'e': "expression", 'f': "file", 'n': "quiet",
		'r': "regexp-extended", 'E': "regexp-extended",
		's': "separate", 'u': "unbuffered",
	},
	takesValue: map[string]bool{
		"expression": true, "file": true,
	},
}

// isSedSafe accepts read-only sed invocations unconditionally and accepts
// in-place forms (`-i`, `-i.bak`, `--in-place`) only when every positional
// file argument resolves inside the current repo. Returns false (→ ask
// via validateFallback) for in-place edits that target paths outside the
// repo, in-place invocations with no file argument, and any non-literal
// flag form. args includes the command name at args[0]; flags start at
// args[1].
func isSedSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}
	parsed := parseArgsWithContext(args[1:], sedSpec, ctx)

	// `-i`, `-i.bak`, `-ni`, etc. all leave a literal `i` key in flags
	// after short-cluster expansion. `--in-place` / `--in-place=.bak`
	// land under "in-place".
	_, hasShortI := parsed.flags["i"]
	_, hasLongI := parsed.flags["in-place"]
	if !hasShortI && !hasLongI {
		return parsed.allLiteral || isReadOnlySedSafe(args[1:])
	}
	if !parsed.allLiteral {
		// Dynamic in-place targets can't be verified — be conservative.
		return false
	}

	// In-place edit. Determine which positionals are file paths:
	// without -e/-f the first positional is the script.
	files := parsed.positional
	if hasShortI && len(files) > 0 {
		if suffix, ok := wordDecodedLiteralWithContext(files[0], ctx); ok && suffix == "" {
			// BSD/macOS sed spells in-place without backup as `sed -i '' ...`.
			// The empty suffix is an option value, not a target file.
			files = files[1:]
		}
	}
	_, hasExpr := parsed.flags["expression"]
	_, hasFile := parsed.flags["file"]
	if !hasExpr && !hasFile {
		if len(files) == 0 {
			return false
		}
		files = files[1:]
	}
	if len(files) == 0 {
		return false
	}
	for _, w := range files {
		path := wordLiteralPathWithContext(w, ctx)
		if path == "" {
			return false
		}
		if writeTargetInRepoFamily(ctx.cwd, path) {
			continue
		}
		if isSafeWriteTarget(path) {
			continue
		}
		return false
	}
	return true
}

func isReadOnlySedSafe(args []*syntax.Word) bool {
	hasProgram := false
	afterDoubleDash := false
	for i := 0; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			// Once the sed program is known, remaining dynamic args
			// are input file operands. Before that point a dynamic
			// arg could be a flag or the sed program itself.
			if hasProgram {
				continue
			}
			if !isCmdSubstSedProgram(args[i]) {
				return false
			}
			hasProgram = true
			continue
		}
		if !afterDoubleDash && lit == "--" {
			afterDoubleDash = true
			continue
		}
		if !afterDoubleDash && strings.HasPrefix(lit, "--") {
			if !scanLongSedFlag(lit, args, &i, &hasProgram) {
				return false
			}
			continue
		}
		if !afterDoubleDash && strings.HasPrefix(lit, "-") && len(lit) > 1 {
			if !scanShortSedFlags(lit, args, &i, &hasProgram) {
				return false
			}
			continue
		}
		if !hasProgram {
			hasProgram = true
		}
	}
	return true
}

func scanLongSedFlag(lit string, args []*syntax.Word, idx *int, hasProgram *bool) bool {
	name := strings.TrimPrefix(lit, "--")
	if flag, _, ok := strings.Cut(name, "="); ok {
		name = flag
	}
	if name == "in-place" {
		return false
	}
	if !sedSpec.takesValue[name] {
		return true
	}
	if !strings.Contains(lit, "=") {
		(*idx)++
		if *idx >= len(args) || !isSedProgramArgSafe(args[*idx]) {
			return false
		}
	}
	*hasProgram = true
	return true
}

func scanShortSedFlags(lit string, args []*syntax.Word, idx *int, hasProgram *bool) bool {
	for j := 1; j < len(lit); j++ {
		c := lit[j]
		if c == 'i' {
			return false
		}
		canonical := string(c)
		if name, ok := sedSpec.short[c]; ok {
			canonical = name
		}
		if !sedSpec.takesValue[canonical] {
			continue
		}
		if j+1 >= len(lit) {
			(*idx)++
			if *idx >= len(args) || !isSedProgramArgSafe(args[*idx]) {
				return false
			}
		}
		*hasProgram = true
		return true
	}
	return true
}

func isSedProgramArgSafe(w *syntax.Word) bool {
	if wordLiteral(w) != "" {
		return true
	}
	return isCmdSubstSedProgram(w)
}

func isCmdSubstSedProgram(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	sawCmdSubst := false
	for _, part := range w.Parts {
		if !sedProgramPartAllowsCmdSubst(part, &sawCmdSubst) {
			return false
		}
	}
	return sawCmdSubst
}

func sedProgramPartAllowsCmdSubst(part syntax.WordPart, sawCmdSubst *bool) bool {
	switch p := part.(type) {
	case *syntax.Lit, *syntax.SglQuoted:
		return true
	case *syntax.CmdSubst:
		*sawCmdSubst = true
		return true
	case *syntax.DblQuoted:
		for _, inner := range p.Parts {
			if !sedProgramPartAllowsCmdSubst(inner, sawCmdSubst) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

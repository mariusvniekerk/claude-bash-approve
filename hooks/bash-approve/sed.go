package main

import (
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
	parsed := parseArgs(args[1:], sedSpec)
	if !parsed.allLiteral {
		// Dynamic flags/scripts can't be verified — be conservative.
		return false
	}

	// `-i`, `-i.bak`, `-ni`, etc. all leave a literal `i` key in flags
	// after short-cluster expansion. `--in-place` / `--in-place=.bak`
	// land under "in-place".
	_, hasShortI := parsed.flags["i"]
	_, hasLongI := parsed.flags["in-place"]
	if !hasShortI && !hasLongI {
		return true
	}

	// In-place edit. Determine which positionals are file paths:
	// without -e/-f the first positional is the script.
	files := parsed.positional
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
		path := wordLiteralPath(w)
		if path == "" {
			return false
		}
		if teeTargetInRepo(ctx.cwd, path) {
			continue
		}
		if isSafeWriteTarget(path) {
			continue
		}
		return false
	}
	return true
}

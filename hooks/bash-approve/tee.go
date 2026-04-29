package main

import (
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// teeBoolFlags are tee flags that don't take a value.
var teeBoolFlags = map[string]bool{
	"-a": true, "--append": true,
	"-i": true, "--ignore-interrupts": true,
	"-p": true,
	"--help": true, "--version": true,
}

// isTeeInRepo allows tee only when every positional file argument resolves
// inside the current repo. Bare tee (no targets) drops to ask conservatively.
// Targets may be non-existent files (tee creates them) — in that case we
// validate via the parent directory. Returns false (→ ask via
// validateFallback) for any path the validator cannot prove safe.
func isTeeInRepo(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return false
	}
	var positional []string
	for _, w := range args[1:] {
		// wordLiteral already returns the bash-decoded form so
		// `\-a` matches the boolean-flag set.
		lit := wordLiteral(w)
		if lit == "" {
			return false
		}
		if strings.HasPrefix(lit, "-") {
			if teeBoolFlags[lit] {
				continue
			}
			// Unknown flag — be conservative.
			return false
		}
		if hasUnquotedGlob(w) {
			return false
		}
		path := wordLiteralPath(w)
		if path == "" {
			return false
		}
		positional = append(positional, path)
	}
	if len(positional) == 0 {
		return false
	}
	for _, p := range positional {
		if teeTargetInRepo(ctx.cwd, p) {
			continue
		}
		if isSafeWriteTarget(p) {
			continue
		}
		return false
	}
	return true
}

// teeTargetInRepo reports whether target (which may not exist yet) would be
// written inside the current repo. Validates the parent directory through
// pathInCurrentRepo so symlink resolution still applies. If the leaf itself
// is a symlink escaping the repo, lstat catches it and we don't fall back
// to the (always-in-repo) parent.
func teeTargetInRepo(cwd, target string) bool {
	if pathInCurrentRepo(cwd, target) {
		return true
	}
	// pathInCurrentRepo failed. Two reasons:
	//   (a) the target doesn't exist yet (legitimate — tee creates files)
	//   (b) the target IS a symlink escaping the repo (malicious)
	// lstat distinguishes: it succeeds for (b) and fails for (a).
	abs := target
	if !filepath.IsAbs(target) {
		abs = filepath.Join(cwd, target)
	}
	if _, err := os.Lstat(abs); err == nil {
		// Leaf exists at the unresolved path → pathInCurrentRepo's failure
		// was a real symlink escape. Do not fall back.
		return false
	}
	// Leaf doesn't exist; validate via parent directory.
	parent := filepath.Dir(target)
	if parent == target {
		return false
	}
	return pathInCurrentRepo(cwd, parent)
}

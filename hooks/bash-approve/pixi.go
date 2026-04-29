package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// pixiRunValueFlags are pixi-run flags that consume the next argument as a
// value (path, environment name, color choice, …).
var pixiRunValueFlags = map[string]bool{
	"--manifest-path": true,
	"-e":              true, "--environment": true,
	"--with-feature": true,
	"--color":        true,
}

// pixiRunBoolFlags are pixi-run flags that don't consume a value.
var pixiRunBoolFlags = map[string]bool{
	"--locked":             true,
	"--frozen":             true,
	"--no-install":         true,
	"--revalidate":         true,
	"--clean-env":          true,
	"--no-lockfile-update": true,
	"-v":                   true, "-q": true,
	"--verbose": true, "--quiet": true,
	"--help": true, "--version": true,
}

// isPixiRunSafe parses pixi-run flags, then recursively evaluates whatever
// follows as a shell command — same model as `nix develop --command`. A
// pixi task name (like `pixi run test`) that happens to share its name
// with a recognized command will auto-approve via that command's pattern;
// names that don't match any pattern fall through to ask.
//
// args includes "pixi" at args[0] and "run" at args[1].
func isPixiRunSafe(args []*syntax.Word, ctx evalContext) bool {
	i := 2
	for i < len(args) {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false
		}
		if lit == "--" {
			i++
			break
		}
		if pixiRunBoolFlags[lit] {
			i++
			continue
		}
		if pixiRunValueFlags[lit] {
			i += 2
			continue
		}
		if strings.HasPrefix(lit, "--") {
			if k, _, ok := strings.Cut(lit, "="); ok {
				if !pixiRunValueFlags[k] && !pixiRunBoolFlags[k] {
					break
				}
				i++
				continue
			}
			break
		}
		if strings.HasPrefix(lit, "-") && lit != "-" {
			return false
		}
		break
	}
	if i >= len(args) {
		return false
	}
	cmd := argsText(args[i:])
	r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
	return r != nil && r.decision == decisionAllow
}

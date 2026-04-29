package main

import (
	"mvdan.cc/sh/v3/syntax"
)

// isNixRunShellSafe validates `nix run`, `nix shell`, and `nix develop`.
//
// `nix run PKG` and `nix run PKG -- ARGS` invoke the package's main
// program. ARGS go to that program, which is opaque to the validator
// (PKG could resolve to bash, python, etc.). Pass-through args drop
// to ask.
//
// `nix shell PKGS [--command CMD ARGS...]` and `nix develop [REF]
// [--command CMD ARGS...]` open an interactive shell unless --command
// is given, in which case ARGS form the exec-style command. Recurse
// through the normal pipeline; allow only when the inner command would
// auto-approve standalone.
//
// `--expr EXPR` asks unconditionally. EXPR is uninspected nix code and
// can run arbitrary code via `builtins.fetchurl`/import-from-derivation
// during evaluation, before any --command branch even runs.
//
// args includes the command name at args[0] and the subcommand at args[1].
func isNixRunShellSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}
	for i := 2; i < len(args); i++ {
		if wordLiteral(args[i]) == "--expr" {
			return false
		}
	}
	sub := wordLiteral(args[1])
	if sub == "run" {
		for i := 2; i < len(args); i++ {
			lit := wordLiteral(args[i])
			if lit == "" {
				return false
			}
			if lit == "--" {
				return false
			}
		}
		return true
	}

	// shell / develop
	for i := 2; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false
		}
		if lit != "--command" && lit != "-c" {
			continue
		}
		rest := args[i+1:]
		if len(rest) == 0 {
			return false
		}
		cmd := argsText(rest)
		r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
		return r != nil && r.decision == decisionAllow
	}
	return true
}

// isNixOldShellSafe validates `nix-shell` invocations. `--run STR` and
// `--command STR` take a single string parsed as a shell command —
// recurse via evaluate(). `--expr EXPR` asks unconditionally for the
// same reason as `nix shell --expr`. Other flags pass.
//
// args includes the command name at args[0].
func isNixOldShellSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}
	for i := 1; i < len(args); i++ {
		if wordLiteral(args[i]) == "--expr" {
			return false
		}
	}
	for i := 1; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false
		}
		if lit != "--run" && lit != "--command" {
			continue
		}
		if i+1 >= len(args) {
			return false
		}
		cmdStr := wordLiteral(args[i+1])
		if cmdStr == "" {
			return false
		}
		r := evaluate(cmdStr, ctx, ctx.wrapperPats, ctx.commandPats)
		if r == nil || r.decision != decisionAllow {
			return false
		}
		i++
	}
	return true
}

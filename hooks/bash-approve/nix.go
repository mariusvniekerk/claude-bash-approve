package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// handleNixRun evaluates `nix run <flake>#<installable> [-- args...]` by
// extracting the installable name as the program and dispatching the
// matching pipeline as if the user had typed that command directly.
//
// Returns (result, handled). handled=true means the result (an allow or
// deny from the inner command) is final; handled=false means the call
// doesn't fit the supported form OR the inner command didn't match any
// pattern — the caller should fall through so the generic
// `^nix\s+(run|shell|develop)\b` pattern + isNixRunShellSafe validator
// can apply the conservative "ask" decision.
//
// `--expr EXPR` falls through. EXPR is uninspected Nix code and can run
// arbitrary code via `builtins.fetchurl` / import-from-derivation during
// evaluation, before any --command branch executes — defer to the outer
// validator's ask.
func handleNixRun(call *syntax.CallExpr, ctx evalContext, wrapperPats, commandPats []pattern) (*result, bool) {
	if len(call.Args) < 3 {
		return nil, false
	}
	if hasNixExprFlag(call.Args[2:]) {
		return nil, false
	}
	pkg := wordLiteral(call.Args[2])
	if pkg == "" || strings.HasPrefix(pkg, "-") {
		return nil, false
	}
	cmdName := extractNixRunCommand(pkg)
	if cmdName == "" {
		return nil, false
	}

	var userArgs []*syntax.Word
	if len(call.Args) >= 4 {
		if wordLiteral(call.Args[3]) != "--" {
			return nil, false
		}
		userArgs = call.Args[4:]
	}

	if r, prop := substitutionPropagate(call, ctx, wrapperPats, commandPats); prop {
		return r, true
	}

	r := matchAndBuild(cmdName, userArgs, call.Assigns, synthCommandArgs(cmdName, userArgs), ctx, wrapperPats, commandPats)
	if r == nil {
		return nil, false
	}
	if r.reason != "" {
		r.reason = "nix run+" + r.reason
	}
	return r, true
}

// handleNixShell evaluates `nix shell <pkgs...> (--command|-c) <cmd> [args...]`
// and `nix develop [REF] (--command|-c) <cmd> [args...]` by extracting the
// embedded command and dispatching the matching pipeline as if the user
// had typed that command directly. Same return semantics as handleNixRun.
//
// nix shell / nix develop `--command CMD ARG1 ARG2 ...` invokes CMD
// directly with the remaining words as argv (not as a shell command
// string), so we treat the first post-flag word as the binary name and
// the rest as its arguments.
//
// `--expr EXPR` falls through. See handleNixRun for the rationale.
func handleNixShell(call *syntax.CallExpr, ctx evalContext, wrapperPats, commandPats []pattern) (*result, bool) {
	if hasNixExprFlag(call.Args[2:]) {
		return nil, false
	}
	cmdIdx := -1
	for i := 2; i < len(call.Args); i++ {
		switch wordLiteral(call.Args[i]) {
		case "--command", "-c":
			cmdIdx = i + 1
		}
		if cmdIdx > 0 {
			break
		}
	}
	if cmdIdx <= 0 || cmdIdx >= len(call.Args) {
		return nil, false
	}
	cmdName := wordLiteral(call.Args[cmdIdx])
	if cmdName == "" {
		return nil, false
	}

	if r, prop := substitutionPropagate(call, ctx, wrapperPats, commandPats); prop {
		return r, true
	}

	userArgs := call.Args[cmdIdx+1:]
	r := matchAndBuild(cmdName, userArgs, call.Assigns, call.Args[cmdIdx:], ctx, wrapperPats, commandPats)
	if r == nil {
		return nil, false
	}
	sub := wordLiteral(call.Args[1])
	if r.reason != "" {
		r.reason = "nix " + sub + "+" + r.reason
	}
	return r, true
}

// hasNixExprFlag reports whether `--expr` appears anywhere in args. The
// flag tells nix to evaluate inline Nix code, which can run arbitrary
// code via `builtins.fetchurl` / import-from-derivation before any
// --command branch executes — so dispatch handlers always defer to the
// outer ask path when it's present.
func hasNixExprFlag(args []*syntax.Word) bool {
	for _, a := range args {
		if wordLiteral(a) == "--expr" {
			return true
		}
	}
	return false
}

// extractNixRunCommand returns the implied program name for a nix flake
// installable spec like `nixpkgs#hello`, `./.#hello`, or
// `nixpkgs#legacyPackages.x86_64-linux.hello`. It strips a trailing
// `^output` selector and walks dot-separated attribute paths down to the
// leaf. Returns "" when the spec lacks `#` or the leaf isn't a plausible
// command name.
//
// Follows the convention that `nix run`'s installable name matches the
// produced binary — packages that diverge will fall through to ask.
func extractNixRunCommand(pkg string) string {
	hash := strings.IndexByte(pkg, '#')
	if hash < 0 || hash == len(pkg)-1 {
		return ""
	}
	name := pkg[hash+1:]
	if caret := strings.IndexByte(name, '^'); caret >= 0 {
		name = name[:caret]
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	if !looksLikeCommandName(name) {
		return ""
	}
	return name
}

// looksLikeCommandName reports whether s is plausibly a POSIX program
// name: non-empty, starts with a letter or underscore, then letters,
// digits, `-`, `_`, or `+` (covers names like `g++`).
func looksLikeCommandName(s string) bool {
	if s == "" || !isCmdNameStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isCmdNameByte(s[i]) {
			return false
		}
	}
	return true
}

func isCmdNameStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isCmdNameByte(b byte) bool {
	return isCmdNameStart(b) || (b >= '0' && b <= '9') || b == '-' || b == '+'
}

// synthCommandArgs builds a synthetic argv where args[0] is a literal Word
// for cmdName, followed by the user's args. argsValidators that look at
// args[1:] (sed flags, find -exec, curl URL, etc.) operate on the real AST
// words; args[0] is a placeholder so indexing args[1:] is safe.
func synthCommandArgs(cmdName string, userArgs []*syntax.Word) []*syntax.Word {
	name := &syntax.Word{Parts: []syntax.WordPart{&syntax.Lit{Value: cmdName}}}
	out := make([]*syntax.Word, 0, 1+len(userArgs))
	out = append(out, name)
	out = append(out, userArgs...)
	return out
}

// isNixRunShellSafe is the fallback validator for `nix run`, `nix shell`,
// and `nix develop` when handleNixRun / handleNixShell didn't reach a
// strong decision (the inner command didn't match any pattern, the form
// had nix-only flags, --expr, etc.). It mirrors the conservative pre-
// dispatch behavior: ask whenever the inner command can't be statically
// proven safe.
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
// recurse via evaluate(). `--expr EXPR` asks unconditionally since EXPR
// is uninspected Nix code. Other flags pass.
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

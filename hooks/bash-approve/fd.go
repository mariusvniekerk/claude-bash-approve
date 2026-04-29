package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// fdValueFlags consume the next argument as a value.
var fdValueFlags = map[string]bool{
	"-d": true, "--max-depth": true, "--min-depth": true, "--exact-depth": true,
	"-t": true, "--type": true,
	"-e": true, "--extension": true,
	"-S": true, "--size": true,
	"--changed-within": true, "--changed-before": true,
	"--owner":   true,
	"-c":        true, "--color": true,
	"-j":        true, "--threads": true,
	"--max-results": true,
	"--max-buffer-time": true,
	"--base-directory": true,
	"--path-separator": true,
	"--search-path":    true,
	"--ignore-file":    true,
	"--strip-cwd-prefix": true,
	"--format":         true,
	"-E":               true, "--exclude": true,
}

// fdExecFlags introduce a command terminated by `;` (single) or by end of args.
// Once the validator hits any of these, every remaining arg becomes part of an
// inner command and must be evaluated through the standard pipeline.
var fdExecFlags = map[string]bool{
	"-x": true, "--exec": true,
	"-X": true, "--exec-batch": true,
}

// isFdSafe accepts plain `fd` invocations (read-only file finder) and
// validates `-x`/`-X`/`--exec`/`--exec-batch` forms by recursing into
// the embedded command — same model as `find -exec`. The `{}`
// placeholder passes through to the inner evaluator: read-only inner
// commands like `rg`/`cat`/`wc` stay allow regardless of the substituted
// path; mutating commands (`rm`/`mv`/…) drop to ask through the inner
// pipeline. Unknown flags fall through to ask defensively.
//
// args includes the command name at args[0].
func isFdSafe(args []*syntax.Word, ctx evalContext) bool {
	for i := 1; i < len(args); i++ {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false
		}
		if fdExecFlags[lit] {
			return validateFdExec(args[i+1:], ctx)
		}
		if k, _, ok := strings.Cut(lit, "="); ok && fdExecFlags[k] {
			// `--exec=cmd` form is unusual but cheap to be conservative on.
			return false
		}
		if fdValueFlags[lit] {
			i++
			continue
		}
	}
	return true
}

// validateFdExec evaluates the command following `-x`/`-X`. fd's exec
// terminator is `;` (literal) for `-x`; everything before is the
// command. For `-X` (batch) there is no terminator — the rest of args
// is the command. Treat both with the same scan: collect args until a
// literal `;`, then evaluate.
func validateFdExec(args []*syntax.Word, ctx evalContext) bool {
	var cmdWords []*syntax.Word
	for _, w := range args {
		lit := wordLiteral(w)
		if lit == "" {
			return false
		}
		if lit == ";" {
			break
		}
		cmdWords = append(cmdWords, w)
	}
	if len(cmdWords) == 0 {
		return false
	}
	cmd := argsText(cmdWords)
	r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
	return r != nil && r.decision == decisionAllow
}

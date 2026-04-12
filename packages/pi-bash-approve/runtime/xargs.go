package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// xargsFlags are xargs flags that consume the next argument as a value.
var xargsValueFlags = map[string]bool{
	"-I": true, "-L": true, "-n": true, "-P": true, "-d": true,
	"-s": true, "-a": true, "-R": true, "-E": true,
	"--replace": true, "--max-lines": true, "--max-args": true,
	"--max-procs": true, "--delimiter": true, "--max-chars": true,
	"--arg-file": true, "--max-replace": true, "--eof": true,
}

// xargsBoolFlags are xargs flags that don't consume a value.
var xargsBoolFlags = map[string]bool{
	"-0": true, "-r": true, "-t": true, "-p": true, "-x": true,
	"--null": true, "--no-run-if-empty": true, "--verbose": true,
	"--interactive": true, "--exit": true, "--open-tty": true,
	"--process-slot-var": true,
}

// isXargsSafe validates an xargs invocation by extracting the command
// that xargs will run and evaluating it through the normal pipeline.
// args includes the command name at args[0].
func isXargsSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true // bare xargs with no args — reads stdin, echo by default
	}

	// Skip xargs flags to find the command it will execute.
	var cmdParts []string
	i := 1 // skip "xargs" at args[0]
	for i < len(args) {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false // non-literal arg — can't validate
		}

		// --flag=value
		if strings.HasPrefix(lit, "--") {
			if k, _, ok := strings.Cut(lit, "="); ok {
				if !xargsValueFlags[k] && !xargsBoolFlags[k] {
					break // not a known xargs flag — start of command
				}
				i++
				continue
			}
		}

		if xargsBoolFlags[lit] {
			i++
			continue
		}
		if xargsValueFlags[lit] {
			i += 2 // skip flag + its value
			continue
		}

		// Not a known xargs flag — this is the start of the command
		break
	}

	// Everything from i onward is the command xargs will run
	for j := i; j < len(args); j++ {
		lit := wordLiteral(args[j])
		if lit == "" {
			return false // non-literal in command — can't validate
		}
		cmdParts = append(cmdParts, lit)
	}

	if len(cmdParts) == 0 {
		return true // xargs with no command defaults to echo
	}

	cmd := strings.Join(cmdParts, " ")
	r := evaluate(cmd, ctx, wrapperPatterns(), commandPatterns())
	if r == nil {
		return false
	}
	return r.decision == decisionAllow
}

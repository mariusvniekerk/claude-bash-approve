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

// xargsSafeAppendCommands are commands where xargs's default behaviour
// (appending each input record as additional positional arguments) does
// not change the approval decision. Anything outside this set with no
// -I asks; that catches `xargs touch`, `xargs mkdir`, `xargs gh issue
// close`, and similar invocations where an attacker-controlled
// positional argument changes what the command does.
//
// `less`, `more`, `yq` are intentionally NOT here. less and more accept
// output-file options; yq has --inplace; any of those turns a runtime-
// controlled appended record into a filesystem write.
var xargsSafeAppendCommands = map[string]bool{
	"cat": true, "head": true, "tail": true,
	"wc": true, "file": true, "stat": true,
	"grep": true, "rg": true, "egrep": true, "fgrep": true,
	"jq":     true,
	"md5sum": true, "sha256sum": true, "sha1sum": true, "shasum": true,
	"xxd": true, "od": true, "hexdump": true,
	"echo": true, "printf": true, "true": true, "false": true,
	"ls": true, "diff": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
}

// isXargsSafe validates an xargs invocation by extracting the command
// that xargs will run and evaluating it through the normal pipeline.
// args includes the command name at args[0].
func isXargsSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true // bare xargs with no args — reads stdin, echo by default
	}

	replacementToken := ""
	i := 1
	for i < len(args) {
		lit := wordLiteral(args[i])
		if lit == "" {
			return false
		}

		// --flag=value
		if strings.HasPrefix(lit, "--") {
			if k, v, ok := strings.Cut(lit, "="); ok {
				if !xargsValueFlags[k] && !xargsBoolFlags[k] {
					break
				}
				if k == "--replace" {
					replacementToken = v
					if replacementToken == "" {
						replacementToken = "{}"
					}
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
			if lit == "-I" || lit == "--replace" {
				if i+1 < len(args) {
					replacementToken = wordLiteral(args[i+1])
				}
				if replacementToken == "" {
					replacementToken = "{}"
				}
			}
			i += 2
			continue
		}

		// Attached short option: `-I{}` / `-Ifoo` — xargs accepts the
		// value glued to the flag.
		if strings.HasPrefix(lit, "-I") && len(lit) > 2 {
			replacementToken = lit[2:]
			i++
			continue
		}

		break
	}

	// Everything from i onward is the command xargs will run.
	var cmdWords []*syntax.Word
	var cmdLits []string
	for j := i; j < len(args); j++ {
		lit := wordLiteral(args[j])
		if lit == "" {
			return false
		}
		cmdWords = append(cmdWords, args[j])
		cmdLits = append(cmdLits, lit)
	}

	if len(cmdWords) == 0 {
		return true
	}

	// xargs replaces every occurrence of the -I/--replace token with
	// each input record at runtime, so a literal `tee {}` template
	// would clobber arbitrary input-controlled paths the validator
	// cannot model. Treat any template containing the token as ask.
	if replacementToken != "" {
		for _, p := range cmdLits {
			if strings.Contains(p, replacementToken) {
				return false
			}
		}
	} else if !ctx.xargsInputFromReadOnlyPipe && !xargsSafeAppendCommands[cmdLits[0]] {
		// No -I/--replace: xargs appends each input record as
		// additional positional arguments. For commands not in the
		// safe-append list, that runtime-controlled tail can change the
		// approval decision, so ask unless the input stream is known to
		// come entirely from read-only pipeline stages.
		return false
	}

	cmd := argsText(cmdWords)
	r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
	if r == nil {
		return false
	}
	return r.decision == decisionAllow
}

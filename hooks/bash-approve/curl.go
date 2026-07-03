package main

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// flagSpec describes a command's flag syntax for normalization.
type flagSpec struct {
	// short maps single-character flags to their canonical long name.
	// Flags listed here that also appear in takesValue consume the next arg.
	short map[byte]string
	// takesValue lists canonical long names that consume a value argument.
	takesValue map[string]bool
}

// parsedArgs is the normalized result of flag parsing.
type parsedArgs struct {
	flags      map[string]*syntax.Word // canonical flag name → value word (nil for boolean)
	positional []*syntax.Word
	allLiteral bool // false if any flag-position word had expansions/substitutions
}

// parseArgs normalizes command arguments into canonical long flag names.
// Short flags are expanded via spec.short; combined flags like -sLd are split.
// Unknown flags are kept verbatim (minus leading dashes).
func parseArgs(args []*syntax.Word, spec flagSpec) parsedArgs {
	return parseArgsWithContext(args, spec, evalContext{})
}

func parseArgsWithContext(args []*syntax.Word, spec flagSpec, ctx evalContext) parsedArgs {
	result := parsedArgs{
		flags:      make(map[string]*syntax.Word),
		allLiteral: true,
	}
	for i := 0; i < len(args); i++ {
		lit, ok := wordDecodedLiteralWithContext(args[i], ctx)
		if !ok {
			result.allLiteral = false
			result.positional = append(result.positional, args[i])
			continue
		}
		if lit == "" {
			result.positional = append(result.positional, args[i])
			continue
		}
		if lit == "--" {
			result.positional = append(result.positional, args[i+1:]...)
			break
		}
		// --flag=value or --flag
		if strings.HasPrefix(lit, "--") {
			name := lit[2:]
			if k, _, ok := strings.Cut(name, "="); ok {
				result.flags[k] = args[i]
			} else if spec.takesValue[name] && i+1 < len(args) {
				i++
				result.flags[name] = args[i]
			} else {
				result.flags[name] = nil
			}
			continue
		}
		// short flags: -sLd value
		if lit[0] == '-' && len(lit) > 1 {
			for j := 1; j < len(lit); j++ {
				c := lit[j]
				canonical := string(c)
				if name, ok := spec.short[c]; ok {
					canonical = name
				}
				if spec.takesValue[canonical] {
					if j+1 < len(lit) {
						// value is rest of token: -dfoo or -XPOST
						result.flags[canonical] = args[i]
					} else if i+1 < len(args) {
						// value is next token: -d foo
						i++
						result.flags[canonical] = args[i]
					} else {
						result.flags[canonical] = nil
					}
					break // rest consumed as value
				}
				result.flags[canonical] = nil
			}
			continue
		}
		result.positional = append(result.positional, args[i])
	}
	return result
}

var curlSpec = flagSpec{
	short: map[byte]string{
		'd': "data", 'F': "form", 'T': "upload-file", 'X': "request",
		'o': "output", 'H': "header", 'u': "user", 'b': "cookie",
		'E': "cert", 'C': "continue-at", 'A': "user-agent",
		's': "silent", 'S': "show-error", 'L': "location",
		'v': "verbose", 'k': "insecure", 'I': "head", 'G': "get",
		'e': "referer", 'w': "write-out", 'f': "fail",
	},
	takesValue: map[string]bool{
		"data": true, "data-raw": true, "data-binary": true, "data-urlencode": true,
		"form": true, "form-string": true, "upload-file": true, "request": true,
		"output": true, "header": true, "user": true, "cookie": true,
		"cert": true, "cacert": true, "capath": true, "continue-at": true,
		"user-agent": true, "json": true, "referer": true, "write-out": true,
		"max-time": true, "connect-timeout": true, "retry": true,
		"retry-delay": true, "retry-max-time": true, "url": true,
		"proxy": true, "resolve": true, "interface": true,
	},
}

// curlReadOnlyFlags are flags that are provably read-only.
// Any flag NOT in this set causes a fallback to "ask".
var curlReadOnlyFlags = map[string]bool{
	"silent": true, "show-error": true, "location": true,
	"insecure": true, "verbose": true, "head": true,
	"get": true, "compressed": true,
	"header": true, "user-agent": true, "cookie": true,
	"max-time": true, "connect-timeout": true, "retry": true,
	"retry-delay": true, "retry-max-time": true,
	"fail": true, "fail-with-body": true, "write-out": true,
	"cert": true, "cacert": true, "capath": true,
	"url": true, "referer": true, "user": true,
	"proxy": true, "resolve": true, "interface": true,
	"continue-at": true,
}

// isCurlReadOnly returns true only if every flag in the curl invocation is
// provably read-only. Unknown or non-literal flags fall back to "ask".
// `-o`/`--output` is read-only when the target is a safe device or a safe
// write prefix (same policy as write redirects).
// args includes the command name at args[0]; flags start at args[1].
func isCurlReadOnly(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}
	p := parseArgsWithContext(args[1:], curlSpec, ctx)
	if !p.allLiteral {
		return false
	}
	for flag, val := range p.flags {
		if flag == "output" {
			if !isSafeCurlOutputTarget(val, ctx) {
				return false
			}
			continue
		}
		if !curlReadOnlyFlags[flag] {
			return false
		}
	}
	return true
}

// isSafeCurlOutputTarget reports whether a curl -o/--output value word
// points somewhere a write is already considered harmless: a safe device
// (/dev/null) or a safe write prefix (/tmp, /var/tmp and their /private
// forms). parseArgs stores the whole flag token for attached-value forms
// (`--output=x`, `-sLox`), so strip the flag prefix before checking.
func isSafeCurlOutputTarget(val *syntax.Word, ctx evalContext) bool {
	lit, ok := wordDecodedLiteralWithContext(val, ctx)
	if !ok || lit == "" {
		return false
	}
	if strings.HasPrefix(lit, "--") {
		_, lit, ok = strings.Cut(lit[2:], "=")
		if !ok {
			return false
		}
	} else if strings.HasPrefix(lit, "-") {
		// Combined short token: the value is everything after the first
		// value-taking short flag (which parseArgs resolved to "output").
		lit = combinedShortFlagValue(lit, "output")
		if lit == "" {
			return false
		}
	}
	return safeRedirectDevices[lit] || isSafeWriteTarget(lit)
}

// combinedShortFlagValue extracts the attached value from a combined short
// flag token like `-sLo/dev/null`, mirroring parseArgs: boolean flags are
// skipped until the first value-taking flag, whose value is the remainder.
// Returns "" if that flag doesn't canonicalize to want.
func combinedShortFlagValue(lit, want string) string {
	for j := 1; j < len(lit); j++ {
		canonical := string(lit[j])
		if name, ok := curlSpec.short[lit[j]]; ok {
			canonical = name
		}
		if curlSpec.takesValue[canonical] {
			if canonical == want {
				return lit[j+1:]
			}
			return ""
		}
	}
	return ""
}

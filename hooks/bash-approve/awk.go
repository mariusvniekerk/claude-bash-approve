package main

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// awkSpec marks -f/-v as value-taking so the parser consumes their argument.
// gawk's -i/--include, -l/--load, and -E/--exec also consume a file argument
// and load arbitrary code (script, dynamic extension, or alternate script
// path), so they must be recognized too. gawk's -e/--source attach an
// inline awk program (alternative to the positional one); we capture it
// so the same system()/pipe/redirect scans run over every program text.
var awkSpec = flagSpec{
	short: map[byte]string{
		'f': "file", 'v': "var", 'F': "field-separator",
		'i': "include", 'l': "load", 'E': "exec", 'e': "source",
	},
	takesValue: map[string]bool{
		"file": true, "var": true, "field-separator": true,
		"include": true, "load": true, "exec": true,
		"source": true,
	},
}

// awkSystemCallStart marks the beginning of a `system(` call. The argument
// is parsed and recursively evaluated by isAwkSystemCallSafe — same model
// as `find -exec` and `xargs`. Whitespace between `system` and `(` is
// allowed; the word boundary prevents `mysystem(...)` from matching.
var awkSystemCallStart = regexp.MustCompile(`\bsystem\s*\(`)

// awkPipeGetlineRegex catches `| getline` (input pipe) and gawk's
// `|& getline` two-way coprocess form. Whitespace tolerant.
//
// For redirect-style danger (print > "...", printf | "...") where the operator
// is separated from print/printf by intervening expressions, see hasPrintRedirect.
var awkPipeGetlineRegex = regexp.MustCompile(`\|&?\s*getline\b`)

// awkSourceLoadRegex catches gawk's source-level `@include` and `@load`
// directives. Both load uninspected awk code (a script file or a
// dynamic extension) at parse time, equivalent in effect to the
// command-line `-i` / `-l` flags but inside the program text itself.
var awkSourceLoadRegex = regexp.MustCompile(`@(include|load)\b`)

// isAwkSafe rejects awk invocations with -f (script we can't read) or with
// program text that contains system() / pipe-redirect / file-redirect calls
// where the embedded shell command is not statically known to be allow.
// Both the positional program and any -e/--source program are scanned —
// gawk treats `-e 'BEGIN{system("...")}'` as the program when no
// positional program is given, so leaving --source unscanned would let
// `awk --source='BEGIN{system("...")}'` slip past the dangerous-pattern
// check. Returns false (→ ask via validateFallback) on any unsafe form.
func isAwkSafe(args []*syntax.Word, ctx evalContext) bool {
	if len(args) < 2 {
		return true
	}
	parsed := parseArgsWithContext(args[1:], awkSpec, ctx)
	// -f/--file (script), -i/--include (library), -l/--load (dynamic extension),
	// and -E/--exec (alternate script) all load code we cannot inspect.
	for _, flag := range []string{"file", "include", "load", "exec"} {
		if _, ok := parsed.flags[flag]; ok {
			return false
		}
	}
	for _, flag := range []string{"var", "field-separator", "source"} {
		value, ok := parsed.flags[flag]
		if ok && value != nil {
			if !isAwkFlagValueSafe(flag, value, ctx) {
				return false
			}
		}
	}
	if !parsed.allLiteral {
		// Dynamic flag-position words can hide -f/-i/-l/-E or source
		// values. Positional input files after a decoded program are
		// handled below.
		if len(parsed.positional) == 0 {
			return false
		}
	}
	// gawk allows -e / --source repeated; parseArgs's flags map only
	// retains the last value, so re-walk args here to collect every
	// program text before scanning.
	sources, ok := collectAwkSources(args[1:], ctx)
	if !ok {
		return false
	}
	programs := append([]string{}, sources...)
	if len(parsed.positional) > 0 {
		// wordLiteral already returns the bash-decoded form so a program
		// written without quoting (e.g. `awk BEGIN\{system\(\"sh\"\)\}`)
		// still resolves to the awk syntax bash hands the binary.
		lit, ok := wordDecodedLiteralWithContext(parsed.positional[0], ctx)
		if !ok || lit == "" {
			return false
		}
		programs = append(programs, lit)
	}
	if len(programs) == 0 {
		return true
	}
	for _, program := range programs {
		if !scanAwkProgram(program, ctx) {
			return false
		}
	}
	return true
}

func isAwkFlagValueSafe(flag string, value *syntax.Word, ctx evalContext) bool {
	if _, decoded := wordDecodedLiteralWithContext(value, ctx); decoded {
		return true
	}
	return flag == "var" && isAwkSedAddressVarAssignment(value, ctx)
}

// collectAwkSources returns every literal value supplied with -e or
// --source. Recognized forms (each across split literal/quoted parts):
//   - `-e PROG` and `--source PROG` (next argv)
//   - `-e=PROG` and `--source=PROG` (=-joined)
//   - `-ePROG` (attached short option)
//
// The second return value is false when a source value is non-literal —
// the caller should ask in that case.
func collectAwkSources(args []*syntax.Word, ctx evalContext) ([]string, bool) {
	var sources []string
	for i := 0; i < len(args); i++ {
		w := args[i]
		if lit, ok := wordDecodedLiteralWithContext(w, ctx); ok {
			switch {
			case strings.HasPrefix(lit, "--source="):
				sources = append(sources, lit[len("--source="):])
				continue
			case strings.HasPrefix(lit, "-e=") && len(lit) >= len("-e="):
				sources = append(sources, lit[len("-e="):])
				continue
			case strings.HasPrefix(lit, "-e") && len(lit) > 2:
				sources = append(sources, lit[2:])
				continue
			case lit == "-e" || lit == "--source":
				i++
				if i >= len(args) {
					return nil, false
				}
				val, ok := wordDecodedLiteralWithContext(args[i], ctx)
				if !ok || val == "" {
					return nil, false
				}
				sources = append(sources, val)
				continue
			}
		}
		// =-joined and attached forms first: detect the flag via the
		// leading literal prefix (which spans concatenated Lit /
		// quoted-Lit parts) so split-literal forms like
		// --source"=$prog" and -e"PROG" both register as a source
		// flag instead of being skipped because wordLiteral returns
		// "".
		prefix, fullyLiteral := wordLeadingLiteral(w)
		switch {
		case strings.HasPrefix(prefix, "--source="):
			if !fullyLiteral {
				return nil, false
			}
			sources = append(sources, prefix[len("--source="):])
			continue
		case strings.HasPrefix(prefix, "-e=") && len(prefix) >= len("-e="):
			if !fullyLiteral {
				return nil, false
			}
			sources = append(sources, prefix[len("-e="):])
			continue
		case strings.HasPrefix(prefix, "-e") && len(prefix) > 2:
			// Attached form: -e<PROG> with no separator. The
			// short-flag layout for awk does not combine `-e`
			// with another single-letter flag, so any extra
			// characters are the program value.
			if !fullyLiteral {
				return nil, false
			}
			sources = append(sources, prefix[2:])
			continue
		}
		// Spaced forms: `-e PROG` or `--source PROG`. The flag word
		// itself is fully literal in any reasonable invocation; the
		// program value uses wordLiteral so backslash-escaped
		// awk syntax (e.g. `\{system\(...\)\}`) is unescaped before
		// the dangerous-pattern scans.
		lit := wordLiteral(w)
		if lit == "-e" || lit == "--source" {
			i++
			if i >= len(args) {
				return nil, false
			}
			val := wordLiteral(args[i])
			if val == "" {
				return nil, false
			}
			sources = append(sources, val)
		}
	}
	return sources, true
}

// wordLeadingLiteral returns the bash-resolved literal prefix of w,
// walking through adjacent Lit / SglQuoted / DblQuoted-wrapped Lit parts
// and unescaping unquoted backslash escapes along the way until the
// first dynamic expansion (parameter expansion, command substitution,
// etc.). The second return value is true when the entire word is fully
// literal, false when a dynamic part terminated the prefix.
//
// `--source"=$prog"` parses as Word{Lit("--source"), DblQuoted{Lit("="),
// ParamExp}}; this helper concatenates the literal segments and reports
// `--source=` even though the value half is dynamic. `\-\-source=PROG`
// (Lit value `\-\-source=PROG`) resolves to `--source=PROG` so the flag
// detection still matches the bash-equivalent argv.
func wordLeadingLiteral(w *syntax.Word) (string, bool) {
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			for i := 0; i < len(p.Value); i++ {
				if p.Value[i] == '\\' && i+1 < len(p.Value) {
					sb.WriteByte(p.Value[i+1])
					i++
					continue
				}
				sb.WriteByte(p.Value[i])
			}
		case *syntax.SglQuoted:
			sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, dp := range p.Parts {
				if lit, ok := dp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				} else {
					return sb.String(), false
				}
			}
		default:
			return sb.String(), false
		}
	}
	return sb.String(), true
}

// scanAwkProgram applies the dangerous-pattern checks that isAwkSafe runs
// on each awk program text: comment stripping, recursive system() arg
// evaluation, pipe-getline, print/printf redirect, and getline-pipe.
// Returns true when none of those checks fire.
func scanAwkProgram(program string, ctx evalContext) bool {
	stripped := stripAwkComments(program)
	if !isAwkSystemCallSafe(stripped, ctx) {
		return false
	}
	if awkPipeGetlineRegex.MatchString(stripped) {
		return false
	}
	if awkSourceLoadRegex.MatchString(stripped) {
		return false
	}
	if hasUnsafePrintRedirect(stripped, ctx) {
		return false
	}
	if hasGetlinePipe(stripped) {
		return false
	}
	return true
}

// isAwkSystemCallSafe finds every `system(...)` call in program and recursively
// evaluates the argument string as a shell command — same pattern as find -exec
// and xargs. Returns true only if every call's argument is a literal string
// that evaluates to decisionAllow. Non-literal arguments (variables, expression
// concatenation, computed strings) and non-allow inner commands return false.
func isAwkSystemCallSafe(program string, ctx evalContext) bool {
	matches := awkSystemCallStart.FindAllStringIndex(program, -1)
	for _, m := range matches {
		cmd, ok := extractAwkLiteralArg(program[m[1]:])
		if !ok {
			return false
		}
		r := evaluate(cmd, ctx, ctx.wrapperPats, ctx.commandPats)
		if r == nil || r.decision != decisionAllow {
			return false
		}
	}
	return true
}

// extractAwkLiteralArg parses a quoted awk string at the start of s (after
// optional whitespace), then a `)`. Returns the unquoted string and true on
// success; ("", false) on any failure (variable, concatenation, unknown
// escape, missing close paren). Awk supports the same C-style escapes as
// printf; we handle the obvious ones and bail out conservatively on the rest.
func extractAwkLiteralArg(s string) (string, bool) {
	i := 0
	for i < len(s) && isAwkSpace(s[i]) {
		i++
	}
	if i >= len(s) || s[i] != '"' {
		return "", false
	}
	i++
	var b strings.Builder
	for i < len(s) {
		c := s[i]
		if c == '"' {
			i++
			for i < len(s) && isAwkSpace(s[i]) {
				i++
			}
			if i >= len(s) || s[i] != ')' {
				return "", false
			}
			return b.String(), true
		}
		if c == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '/':
				b.WriteByte('/')
			default:
				return "", false
			}
			i += 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return "", false
}

func isAwkSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// hasUnsafePrintRedirect returns true when the program contains `print` or
// `printf` followed by a redirect that the hook should ask about. Literal file
// targets inside the current repo family are allowed; pipe targets, dynamic
// targets, and file targets outside the repo family ask.
//
// Filters in priority order:
//   - characters inside `"..."` (awk's only string delimiter);
//   - `||` (logical or) and `>=` (comparison);
//   - `>` / `|` inside parentheses (comparison expressions like `(a > b)`);
//   - `>` / `|` appearing earlier in the same statement than `print`
//     (e.g. `if (a > b) print`).
//
// Single quotes (`'`) are NOT string delimiters in awk — they appear
// freely inside strings and regex literals (e.g. `"don't"`, `/a'/`) and
// must not change the scanner's state. Treating them as delimiters caused
// a regex apostrophe to flip the scanner into a fake string state that
// swallowed a subsequent `>` redirect.
//
// Awk regex literals (`/foo|bar/`) are also intentionally NOT skipped,
// because `/` is also division and statically distinguishing the two
// requires real parsing. A regex containing `|` after a print keyword
// over-asks rather than letting a `print x > "..."` after a `1/2`
// division slip through — the safer trade-off.
//
// Quoted, variable, and expression targets are all treated as redirects:
// awk evaluates any expression after a top-level `>` / `|` as a filename or
// shell command, so `print x > f` (variable target) and `print x > ("f" n)`
// (expression target) must ask just like `print x > "/etc/passwd"`.
func hasUnsafePrintRedirect(program string, ctx evalContext) bool {
	if !strings.Contains(program, "print") {
		return false
	}
	inDouble := false
	parenDepth := 0
	for i := 0; i < len(program); i++ {
		c := program[i]
		if c == '\\' && i+1 < len(program) {
			i++
			continue
		}
		if inDouble {
			if c == '"' {
				inDouble = false
			}
			continue
		}
		switch c {
		case '"':
			inDouble = true
			continue
		case '(':
			parenDepth++
			continue
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
			continue
		}
		if c != '>' && c != '|' {
			continue
		}
		// Skip `||` (logical or). The append redirect `>>` is handled by the second-`>`
		// iteration of the outer loop; we do not need a special case here.
		if c == '|' && i+1 < len(program) && program[i+1] == '|' {
			i++
			continue
		}
		// Skip `>=` comparison.
		if c == '>' && i+1 < len(program) && program[i+1] == '=' {
			i++
			continue
		}
		// Comparison expressions like `print (a > b)` keep `>` inside
		// parens; only top-level operators are redirects.
		if parenDepth > 0 {
			continue
		}
		if !hasPrintBefore(program, i) {
			continue
		}
		if c == '|' {
			return true
		}
		target, end, ok := parseAwkFileRedirectTarget(program, i)
		if !ok || !writeTargetInRepoFamily(ctx.cwd, target) {
			return true
		}
		i = end - 1
	}
	return false
}

func parseAwkFileRedirectTarget(program string, opPos int) (target string, end int, ok bool) {
	i := opPos + 1
	if i < len(program) && program[i] == '>' {
		i++
	}
	for i < len(program) && isAwkSpace(program[i]) {
		i++
	}
	if i >= len(program) || program[i] != '"' {
		return "", i, false
	}
	target, i, ok = parseAwkStringLiteral(program, i)
	if !ok {
		return "", i, false
	}
	for i < len(program) && isAwkSpace(program[i]) {
		i++
	}
	if i < len(program) && program[i] != ';' && program[i] != '\n' && program[i] != '}' {
		return "", i, false
	}
	return target, i, true
}

func parseAwkStringLiteral(program string, start int) (string, int, bool) {
	if start >= len(program) || program[start] != '"' {
		return "", start, false
	}
	i := start + 1
	var b strings.Builder
	for i < len(program) {
		c := program[i]
		if c == '"' {
			return b.String(), i + 1, true
		}
		if c == '\\' && i+1 < len(program) {
			switch program[i+1] {
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '/':
				b.WriteByte('/')
			default:
				return "", i, false
			}
			i += 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return "", i, false
}

// hasPrintBefore reports whether `print` or `printf` appears in the program
// text before position `pos`, within the same statement (no `;`, newline, or `}`
// between them). Match requires identifier boundaries so `sprint` /
// `printer` / `myprintf` do not falsely trigger.
func hasPrintBefore(program string, pos int) bool {
	start := 0
	for i := pos - 1; i >= 0; i-- {
		if program[i] == ';' || program[i] == '\n' || program[i] == '{' || program[i] == '}' {
			start = i + 1
			break
		}
	}
	segment := program[start:pos]
	for off := 0; off+len("print") <= len(segment); off++ {
		if segment[off] != 'p' {
			continue
		}
		if !strings.HasPrefix(segment[off:], "print") {
			continue
		}
		if off > 0 && isAwkIdentChar(segment[off-1]) {
			continue
		}
		end := off + len("print")
		if end < len(segment) && segment[end] == 'f' {
			end++
		}
		if end < len(segment) && isAwkIdentChar(segment[end]) {
			continue
		}
		return true
	}
	return false
}

// isAwkIdentChar reports whether b is a continuation byte of an awk
// identifier. Awk identifiers match [A-Za-z_][A-Za-z0-9_]* — 7-bit ASCII
// only — so a byte-wise check is sufficient for boundary detection.
func isAwkIdentChar(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// stripAwkComments removes everything after a # outside of strings/regex.
// Conservative: only handles trivial cases. If the program quoting is complex,
// the substring scan still catches dangerous tokens that appear in code.
func stripAwkComments(program string) string {
	var sb strings.Builder
	inSingle, inDouble, inSlash := false, false, false
	for i := 0; i < len(program); i++ {
		c := program[i]
		if c == '\\' && i+1 < len(program) {
			sb.WriteByte(c)
			sb.WriteByte(program[i+1])
			i++
			continue
		}
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			}
		case inSlash:
			if c == '/' {
				inSlash = false
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '/':
			inSlash = true
		case c == '#':
			// line comment until newline
			for i < len(program) && program[i] != '\n' {
				i++
			}
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// hasGetlinePipe detects `getline VAR; ... ; VAR |` style patterns where the
// variable receives a command's output. This is a coarse heuristic — substring
// "getline" followed later by `|` (excluding `||`) anywhere in the program.
func hasGetlinePipe(program string) bool {
	_, rest, ok := strings.Cut(program, "getline")
	if !ok {
		return false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] != '|' {
			continue
		}
		if i+1 < len(rest) && rest[i+1] == '|' {
			i++
			continue
		}
		return true
	}
	return false
}

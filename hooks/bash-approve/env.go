package main

import (
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

type envAssignment struct {
	name        string
	value       string
	staticValue bool
}

func envAssignmentsFromSyntax(assigns []*syntax.Assign) []envAssignment {
	out := make([]envAssignment, 0, len(assigns))
	for _, assign := range assigns {
		if assign.Name == nil {
			continue
		}
		envAssign := envAssignment{name: assign.Name.Value}
		if assign.Value != nil {
			if value, ok := wordDecodedLiteral(assign.Value); ok {
				envAssign.value = value
				envAssign.staticValue = true
			}
		}
		out = append(out, envAssign)
	}
	return out
}

// envHardDeny lists env-var names that should never auto-execute. These are
// known library-injection / shell-init / process-impersonation vectors.
var envHardDeny = map[string]bool{
	"LD_PRELOAD":                true,
	"LD_AUDIT":                  true,
	"DYLD_INSERT_LIBRARIES":     true,
	"DYLD_FORCE_FLAT_NAMESPACE": true,
	"BASH_ENV":                  true,
	"ENV":                       true,
	"PROMPT_COMMAND":            true,
	"PS0":                       true,
	"PS1":                       true,
	"PS2":                       true,
	"PS3":                       true,
	"PS4":                       true,
	"SHELLOPTS":                 true,
	"BASHOPTS":                  true,
}

// envAskExact lists env-var names with legitimate edge-case use. Auto-prompt.
var envAskExact = map[string]bool{
	"PATH":                             true,
	"IFS":                              true,
	"PYTHONSTARTUP":                    true,
	"RUBYOPT":                          true,
	"PERL5OPT":                         true,
	"GIT_DIR":                          true,
	"GIT_INDEX_FILE":                   true,
	"GIT_WORK_TREE":                    true,
	"GIT_OBJECT_DIRECTORY":             true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
	"SKIP":                             true,
	"HUSKY":                            true,
	"PRE_COMMIT_ALLOW_NO_CONFIG":       true,
	"GOFLAGS":                          true,
	"GOROOT":                           true,
	"CARGO_BUILD_RUSTFLAGS":            true,
	"RUSTFLAGS":                        true,
	// JVM (-javaagent: / -XX:OnError= injection)
	"JAVA_TOOL_OPTIONS": true,
	"JAVA_OPTS":         true,
	"JAVA_OPTIONS":      true,
	"_JAVA_OPTIONS":     true,
	"MAVEN_OPTS":        true,
	"GRADLE_OPTS":       true,
	// Node.js (--require / --experimental-loader / --inspect-brk)
	"NODE_OPTIONS": true,
	"NODE_PATH":    true,
	// Python (module hijacking / interpreter redirect)
	"PYTHONPATH":    true,
	"PYTHONHOME":    true,
	"PYTHONINSPECT": true,
	// Ruby (Bundler Gemfile redirection)
	"BUNDLE_GEMFILE": true,
	// pip (package source redirection)
	"PIP_INDEX_URL":       true,
	"PIP_EXTRA_INDEX_URL": true,
	"PIP_TARGET":          true,
	// Pager / editor / make values that already-allowed commands
	// interpret as commands or argv. e.g. `PAGER='sh -c "..."' git log`
	// makes Git execute the attacker-controlled pager; MAKEFLAGS can
	// inject options into make.
	"PAGER":     true,
	"EDITOR":    true,
	"VISUAL":    true,
	"MAKEFLAGS": true,
}

// envAllowExact is the exact-match allowlist.
var envAllowExact = map[string]bool{
	"CC": true, "CXX": true, "LD": true, "AR": true, "AS": true,
	"CFLAGS": true, "CXXFLAGS": true, "LDFLAGS": true, "CPPFLAGS": true,
	"KUBECONFIG": true,
	"TMPDIR":     true, "TMP": true, "TEMP": true,
	"NO_COLOR": true, "FORCE_COLOR": true, "COLORTERM": true, "TERM_PROGRAM": true,
	"DATABASE_URL": true, "DB_URL": true, "REDIS_URL": true,
	"LANG": true, "TZ": true,
	"DEBUG": true, "LOG_LEVEL": true, "VERBOSE": true,
	"TERM":    true,
	"COLUMNS": true, "LINES": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true,
	"FTP_PROXY": true, "ALL_PROXY": true,
	"HOME": true, "USER": true, "LOGNAME": true,
}

// envAllowPrefixes is the prefix-match allowlist. Order does not matter
// because hard-deny / ask sets are checked first.
var envAllowPrefixes = []string{
	"RUST_", "CARGO_", "PYTHON", "GO", "CGO_", "NODE_", "NPM_", "npm_",
	"RUBY_", "RAILS_", "BUNDLE_", "JAVA_", "JVM_", "MAVEN_",
	"GRADLE_", "GEM_", "RAKE_", "PIP_", "PYTEST_",
	"MAKE_",
	"PREK_",
	"DOCKER_", "KUBE_", "K8S_", "HELM_", "COMPOSE_",
	"TF_", "TERRAFORM_", "PULUMI_", "ANSIBLE_",
	"NIX_", "MISE_", "ASDF_", "DIRENV_",
	"LC_",
	"LOG_", "TEST_",
}

var envAllowExactValues = map[string]map[string]bool{
	"GIT_EDITOR": {"true": true},
}

var envAllowStaticValues = map[string]func(string) bool{
	"CARGO_BUILD_RUSTFLAGS": isSafeRustFlags,
	"GIT_SEQUENCE_EDITOR":   isSafeGitSequenceEditor,
	"GOFLAGS":               isSafeGoFlags,
	"GRADLE_OPTS":           isSafeJvmOptions,
	"JAVA_OPTS":             isSafeJvmOptions,
	"JAVA_OPTIONS":          isSafeJvmOptions,
	"JAVA_TOOL_OPTIONS":     isSafeJvmOptions,
	"MAVEN_OPTS":            isSafeJvmOptions,
	"MAKEFLAGS":             isSafeMakeFlags,
	"NODE_OPTIONS":          isSafeNodeOptions,
	"RUSTFLAGS":             isSafeRustFlags,
	"_JAVA_OPTIONS":         isSafeJvmOptions,
}

// validateEnvVarNames applies hard-deny → ask → allowlist → default-ask.
// Hard-deny is checked across ALL names first so it can't be bypassed by
// putting an unknown var earlier in the list.
// Returns nil only if every name matches the allowlist.
func validateEnvVarNames(names []string, _ evalContext) *result {
	assignments := make([]envAssignment, 0, len(names))
	for _, name := range names {
		assignments = append(assignments, envAssignment{name: name})
	}
	return validateEnvAssignments(assignments)
}

func validateEnvAssignments(assignments []envAssignment) *result {
	// Pass 1: hard-deny is unconditional regardless of where in the list it appears.
	for _, assignment := range assignments {
		if envHardDeny[assignment.name] {
			return &result{
				decision:   decisionDeny,
				denyReason: "BLOCKED: env var " + assignment.name + " can subvert command execution. Use a safer alternative.",
			}
		}
	}
	// Pass 2: per-name ask/allow check.
	for _, assignment := range assignments {
		name := assignment.name
		if allowedValues, ok := envAllowExactValues[name]; ok && assignment.staticValue && allowedValues[assignment.value] {
			continue
		}
		if allowStaticValue, ok := envAllowStaticValues[name]; ok {
			if assignment.staticValue && allowStaticValue(assignment.value) {
				continue
			}
			return &result{decision: decisionAsk}
		}
		if envAskExact[name] {
			return &result{decision: decisionAsk}
		}
		// LD_*/DYLD_* prefix match for ask (any not in hard-deny).
		if strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "DYLD_") {
			return &result{decision: decisionAsk}
		}
		if envAllowExact[name] {
			continue
		}
		matched := false
		for _, p := range envAllowPrefixes {
			if strings.HasPrefix(name, p) {
				matched = true
				break
			}
		}
		if !matched {
			return &result{decision: decisionAsk}
		}
	}
	return nil
}

func isSafeGitSequenceEditor(value string) bool {
	value = strings.TrimSpace(value)
	if value == "true" || value == ":" {
		return true
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(value), "")
	if err != nil || len(file.Stmts) != 1 {
		return false
	}
	stmt := file.Stmts[0]
	if stmt == nil || len(stmt.Redirs) > 0 {
		return false
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Assigns) > 0 || len(call.Args) == 0 {
		return false
	}
	name, ok := wordDecodedLiteral(call.Args[0])
	if !ok || name != "sed" {
		return false
	}
	return isSafeGitSequenceEditorSed(call.Args[1:])
}

func isSafeGitSequenceEditorSed(args []*syntax.Word) bool {
	inPlace := false
	var scripts []string
	for i := 0; i < len(args); i++ {
		lit, ok := wordDecodedLiteral(args[i])
		if !ok {
			return false
		}
		switch {
		case lit == "-i" || strings.HasPrefix(lit, "-i."):
			inPlace = true
		case lit == "--in-place" || strings.HasPrefix(lit, "--in-place="):
			inPlace = true
		case lit == "-e":
			i++
			if i >= len(args) {
				return false
			}
			script, ok := wordDecodedLiteral(args[i])
			if !ok {
				return false
			}
			scripts = append(scripts, script)
		case strings.HasPrefix(lit, "-e") && len(lit) > 2:
			scripts = append(scripts, lit[2:])
		case strings.HasPrefix(lit, "-"):
			return false
		default:
			scripts = append(scripts, lit)
			if i != len(args)-1 {
				return false
			}
		}
	}
	return inPlace && len(scripts) == 1 && isSafeGitRebaseTodoSedSubstitution(scripts[0])
}

func isSafeGitRebaseTodoSedSubstitution(program string) bool {
	if len(program) < 4 || program[0] != 's' {
		return false
	}
	delim := program[1]
	if delim == '\\' || strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \t\r\n", rune(delim)) {
		return false
	}
	parts := strings.Split(program[2:], string(delim))
	if len(parts) != 3 || parts[2] != "" {
		return false
	}
	const pickPrefix = "^pick "
	if !strings.HasPrefix(parts[0], pickPrefix) {
		return false
	}
	sha := strings.TrimSpace(strings.TrimPrefix(parts[0], pickPrefix))
	replacementFields := strings.Fields(parts[1])
	return isGitObjectIDPrefix(sha) &&
		len(replacementFields) == 2 &&
		replacementFields[0] == "edit" &&
		replacementFields[1] == sha
}

func isGitObjectIDPrefix(value string) bool {
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if ('0' <= r && r <= '9') || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F') {
			continue
		}
		return false
	}
	return true
}

func isSafeNodeOptions(value string) bool {
	return !slices.ContainsFunc(strings.Fields(value), isDangerousNodeOption)
}

func isDangerousNodeOption(option string) bool {
	dangerousLong := []string{
		"--env-file",
		"--env-file-if-exists",
		"--eval",
		"--experimental-loader",
		"--import",
		"--inspect",
		"--inspect-brk",
		"--loader",
		"--require",
	}
	for _, flag := range dangerousLong {
		if option == flag || strings.HasPrefix(option, flag+"=") {
			return true
		}
	}
	return option == "-e" || option == "-r" || strings.HasPrefix(option, "-e=") || strings.HasPrefix(option, "-r=")
}

func isSafeGoFlags(value string) bool {
	return !slices.ContainsFunc(strings.Fields(value), isDangerousGoFlag)
}

func isDangerousGoFlag(option string) bool {
	dangerous := []string{
		"-exec",
		"-toolexec",
	}
	for _, flag := range dangerous {
		if option == flag || strings.HasPrefix(option, flag+"=") {
			return true
		}
	}
	return false
}

func isSafeJvmOptions(value string) bool {
	return !slices.ContainsFunc(strings.Fields(value), isDangerousJvmOption)
}

func isDangerousJvmOption(option string) bool {
	if strings.HasPrefix(option, "@") {
		return true
	}
	dangerousPrefixes := []string{
		"-agentlib:",
		"-agentpath:",
		"-javaagent:",
		"-Xbootclasspath",
		"-Xdebug",
		"-Xrunjdwp",
		"-classpath",
		"--class-path",
		"--module-path",
		"-cp",
		"-XX:OnError=",
		"-XX:OnOutOfMemoryError=",
	}
	for _, prefix := range dangerousPrefixes {
		if option == prefix || strings.HasPrefix(option, prefix) {
			return true
		}
	}
	return false
}

func isSafeRustFlags(value string) bool {
	tokens := strings.Fields(value)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch {
		case isRustLintFlagWithValue(token):
			continue
		case isRustSplitLintFlag(token):
			if i+1 >= len(tokens) || strings.HasPrefix(tokens[i+1], "-") {
				return false
			}
			i++
		case token == "--cfg":
			if i+1 >= len(tokens) || strings.HasPrefix(tokens[i+1], "-") {
				return false
			}
			i++
		case strings.HasPrefix(token, "--cfg="):
			continue
		case token == "-C":
			if i+1 >= len(tokens) || !isSafeRustCodegenOption(tokens[i+1]) {
				return false
			}
			i++
		case strings.HasPrefix(token, "-C"):
			if !isSafeRustCodegenOption(strings.TrimPrefix(token, "-C")) {
				return false
			}
		case strings.HasPrefix(token, "--remap-path-prefix="):
			continue
		default:
			return false
		}
	}
	return true
}

func isRustSplitLintFlag(token string) bool {
	return token == "-A" || token == "-W" || token == "-D" || token == "-F"
}

func isRustLintFlagWithValue(token string) bool {
	if len(token) <= 2 {
		return false
	}
	prefix := token[:2]
	return prefix == "-A" || prefix == "-W" || prefix == "-D" || prefix == "-F"
}

func isSafeRustCodegenOption(option string) bool {
	key := option
	if before, _, ok := strings.Cut(option, "="); ok {
		key = before
	}
	switch key {
	case "codegen-units",
		"debuginfo",
		"debug-assertions",
		"embed-bitcode",
		"extra-filename",
		"incremental",
		"lto",
		"metadata",
		"opt-level",
		"overflow-checks",
		"panic",
		"prefer-dynamic",
		"relocation-model",
		"strip",
		"target-cpu",
		"target-feature":
		return true
	default:
		return false
	}
}

func isSafeMakeFlags(value string) bool {
	tokens := strings.Fields(value)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch {
		case token == "-j" || token == "--jobs" || token == "-l" || token == "--load-average" || token == "--max-load" || token == "-O" || token == "--output-sync":
			if i+1 >= len(tokens) || strings.HasPrefix(tokens[i+1], "-") {
				return false
			}
			i++
		case token == "-k" || token == "--keep-going" ||
			token == "-s" || token == "--silent" ||
			token == "--no-print-directory" ||
			token == "--warn-undefined-variables" ||
			token == "--trace":
			continue
		case strings.HasPrefix(token, "-j") && len(token) > 2:
			continue
		case strings.HasPrefix(token, "-l") && len(token) > 2:
			continue
		case strings.HasPrefix(token, "-O") && len(token) > 2:
			continue
		case strings.HasPrefix(token, "--jobs=") ||
			strings.HasPrefix(token, "--load-average=") ||
			strings.HasPrefix(token, "--max-load=") ||
			strings.HasPrefix(token, "--output-sync=") ||
			strings.HasPrefix(token, "--shuffle=") ||
			strings.HasPrefix(token, "--debug="):
			continue
		case token == "--shuffle" || token == "--debug":
			continue
		default:
			return false
		}
	}
	return true
}

// validateStandaloneAssignments is the lenient counterpart used by
// standalone assignments (`FOO=bar` with no command on the same line).
// Hard-deny / ask-exact / LD_*/DYLD_* still flag dangerous names, but
// unknown names auto-approve — bash convention treats lowercase locals
// like `hm_src=...` and most uppercase project-specific knobs as benign,
// and the strict allowlist would otherwise prompt on every one of them.
// Names that don't match any of the dangerous lists return nil.
func validateStandaloneAssignments(assignments []envAssignment) *result {
	for _, assignment := range assignments {
		if envHardDeny[assignment.name] {
			return &result{
				decision:   decisionDeny,
				denyReason: "BLOCKED: env var " + assignment.name + " can subvert command execution. Use a safer alternative.",
			}
		}
	}
	for _, assignment := range assignments {
		name := assignment.name
		if allowStaticValue, ok := envAllowStaticValues[name]; ok {
			if assignment.staticValue && allowStaticValue(assignment.value) {
				continue
			}
			return &result{decision: decisionAsk}
		}
		if envAskExact[name] {
			return &result{decision: decisionAsk}
		}
		if strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "DYLD_") {
			return &result{decision: decisionAsk}
		}
	}
	return nil
}

// isSafeEnvVarsWrapper parses the matched env-vars wrapper prefix and runs
// validateEnvVarNames on the extracted KEYs. The matched prefix has the form
// "K1=v1 K2=v2 " — values cannot contain whitespace (regex requires \s+ between
// tokens), so a simple Fields split is safe.
func isSafeEnvVarsWrapper(matchedPrefix string, ctx evalContext) *result {
	tokens := strings.Fields(matchedPrefix)
	assignments := make([]envAssignment, 0, len(tokens))
	for _, tok := range tokens {
		eq := strings.IndexByte(tok, '=')
		if eq <= 0 {
			continue
		}
		assignments = append(assignments, envAssignment{
			name:        tok[:eq],
			value:       tok[eq+1:],
			staticValue: true,
		})
	}
	return validateEnvAssignments(assignments)
}

package main

import (
	"strings"
)

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
	"RUST_", "CARGO_", "PYTHON", "GO", "NODE_", "NPM_", "npm_",
	"RUBY_", "RAILS_", "BUNDLE_", "JAVA_", "JVM_", "MAVEN_",
	"GRADLE_", "GEM_", "RAKE_", "PIP_", "PYTEST_",
	"MAKE_",
	"DOCKER_", "KUBE_", "K8S_", "HELM_", "COMPOSE_",
	"TF_", "TERRAFORM_", "PULUMI_", "ANSIBLE_",
	"NIX_", "MISE_", "ASDF_", "DIRENV_",
	"LC_",
	"LOG_", "TEST_",
}

// validateEnvVarNames applies hard-deny → ask → allowlist → default-ask.
// Hard-deny is checked across ALL names first so it can't be bypassed by
// putting an unknown var earlier in the list.
// Returns nil only if every name matches the allowlist.
func validateEnvVarNames(names []string, _ evalContext) *result {
	// Pass 1: hard-deny is unconditional regardless of where in the list it appears.
	for _, name := range names {
		if envHardDeny[name] {
			return &result{
				decision:   decisionDeny,
				denyReason: "BLOCKED: env var " + name + " can subvert command execution. Use a safer alternative.",
			}
		}
	}
	// Pass 2: per-name ask/allow check.
	for _, name := range names {
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

// validateStandaloneAssignNames is the lenient counterpart used by
// standalone assignments (`FOO=bar` with no command on the same line).
// Hard-deny / ask-exact / LD_*/DYLD_* still flag dangerous names, but
// unknown names auto-approve — bash convention treats lowercase locals
// like `hm_src=...` and most uppercase project-specific knobs as benign,
// and the strict allowlist would otherwise prompt on every one of them.
// Names that don't match any of the dangerous lists return nil.
func validateStandaloneAssignNames(names []string) *result {
	for _, name := range names {
		if envHardDeny[name] {
			return &result{
				decision:   decisionDeny,
				denyReason: "BLOCKED: env var " + name + " can subvert command execution. Use a safer alternative.",
			}
		}
	}
	for _, name := range names {
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
	names := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		eq := strings.IndexByte(tok, '=')
		if eq <= 0 {
			continue
		}
		names = append(names, tok[:eq])
	}
	return validateEnvVarNames(names, ctx)
}

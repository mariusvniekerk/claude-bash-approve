package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEnvVarNames(t *testing.T) {
	tests := []struct {
		name         string
		varNames     []string
		wantNil      bool
		wantDecision string
	}{
		{"allowlist single", []string{"RUST_BACKTRACE"}, true, ""},
		{"allowlist multi", []string{"RUST_BACKTRACE", "CARGO_HOME"}, true, ""},
		{"multi with PYTHONPATH asks", []string{"RUST_BACKTRACE", "PYTHONPATH"}, false, decisionAsk},
		{"hard-deny LD_PRELOAD", []string{"LD_PRELOAD"}, false, decisionDeny},
		{"hard-deny BASH_ENV", []string{"BASH_ENV"}, false, decisionDeny},
		{"hard-deny PROMPT_COMMAND", []string{"PROMPT_COMMAND"}, false, decisionDeny},
		{"hard-deny SHELLOPTS", []string{"SHELLOPTS"}, false, decisionDeny},
		{"ask PATH", []string{"PATH"}, false, decisionAsk},
		{"ask LD_LIBRARY_PATH", []string{"LD_LIBRARY_PATH"}, false, decisionAsk},
		{"ask DYLD_LIBRARY_PATH", []string{"DYLD_LIBRARY_PATH"}, false, decisionAsk},
		{"ask IFS", []string{"IFS"}, false, decisionAsk},
		{"ask SKIP", []string{"SKIP"}, false, decisionAsk},
		{"ask GIT_DIR", []string{"GIT_DIR"}, false, decisionAsk},
		{"unknown var asks", []string{"FOO"}, false, decisionAsk},
		{"PYTHONSTARTUP asks before PYTHON allowlist", []string{"PYTHONSTARTUP"}, false, decisionAsk},
		{"RUBYOPT asks before RUBY allowlist", []string{"RUBYOPT"}, false, decisionAsk},
		{"mixed allowlist and hard-deny", []string{"RUST_BACKTRACE", "LD_PRELOAD"}, false, decisionDeny},
		{"unknown then hard-deny still denies", []string{"FOO", "LD_PRELOAD"}, false, decisionDeny},
		{"allowlist then hard-deny still denies", []string{"RUST_BACKTRACE", "FOO", "LD_PRELOAD"}, false, decisionDeny},
		{"GOFLAGS asks (toolexec injection)", []string{"GOFLAGS"}, false, decisionAsk},
		{"GOROOT asks", []string{"GOROOT"}, false, decisionAsk},
		{"CARGO_BUILD_RUSTFLAGS asks", []string{"CARGO_BUILD_RUSTFLAGS"}, false, decisionAsk},
		{"RUSTFLAGS asks", []string{"RUSTFLAGS"}, false, decisionAsk},
		{"JAVA_TOOL_OPTIONS asks", []string{"JAVA_TOOL_OPTIONS"}, false, decisionAsk},
		{"MAVEN_OPTS asks", []string{"MAVEN_OPTS"}, false, decisionAsk},
		{"NODE_OPTIONS asks", []string{"NODE_OPTIONS"}, false, decisionAsk},
		{"PYTHONPATH asks (module hijack)", []string{"PYTHONPATH"}, false, decisionAsk},
		{"BUNDLE_GEMFILE asks", []string{"BUNDLE_GEMFILE"}, false, decisionAsk},
		{"PIP_INDEX_URL asks (typosquat)", []string{"PIP_INDEX_URL"}, false, decisionAsk},
		{"PAGER asks (command injection via git)", []string{"PAGER"}, false, decisionAsk},
		{"EDITOR asks (command-as-value)", []string{"EDITOR"}, false, decisionAsk},
		{"VISUAL asks (command-as-value)", []string{"VISUAL"}, false, decisionAsk},
		{"MAKEFLAGS asks (option injection)", []string{"MAKEFLAGS"}, false, decisionAsk},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validateEnvVarNames(tt.varNames, evalContext{})
			if tt.wantNil {
				assert.Nil(t, r)
			} else {
				require.NotNil(t, r)
				assert.Equal(t, tt.wantDecision, r.decision)
				if tt.wantDecision == decisionDeny {
					assert.NotEmpty(t, r.denyReason)
				}
			}
		})
	}
}

func TestIsSafeEnvVarsWrapper(t *testing.T) {
	tests := []struct {
		name         string
		prefix       string
		wantNil      bool
		wantDecision string
	}{
		{"allowlist", "RUST_BACKTRACE=1 ", true, ""},
		{"hard-deny", "LD_PRELOAD=/tmp/evil.so ", false, decisionDeny},
		{"unknown", "FOO=bar ", false, decisionAsk},
		{"multi allowlist", "RUST_BACKTRACE=1 CARGO_HOME=/tmp ", true, ""},
		{"multi mixed", "RUST_BACKTRACE=1 LD_PRELOAD=/x ", false, decisionDeny},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := isSafeEnvVarsWrapper(tt.prefix, evalContext{})
			if tt.wantNil {
				assert.Nil(t, r)
			} else {
				require.NotNil(t, r)
				assert.Equal(t, tt.wantDecision, r.decision)
			}
		})
	}
}

func TestEvaluate_EnvVarFlows(t *testing.T) {
	t.Run("LD_PRELOAD denied", func(t *testing.T) {
		r := evaluateAll("LD_PRELOAD=/tmp/evil.so cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
		assert.NotEmpty(t, r.denyReason)
	})

	t.Run("BASH_ENV denied", func(t *testing.T) {
		r := evaluateAll("BASH_ENV=/tmp/evil cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("PROMPT_COMMAND denied", func(t *testing.T) {
		r := evaluateAll("PROMPT_COMMAND='rm -rf /' cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("PATH asks", func(t *testing.T) {
		r := evaluateAll("PATH=/tmp:$PATH cargo build")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("LD_LIBRARY_PATH asks", func(t *testing.T) {
		r := evaluateAll("LD_LIBRARY_PATH=/opt/lib cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("RUST_BACKTRACE allows", func(t *testing.T) {
		r := evaluateAll("RUST_BACKTRACE=1 cargo test")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("PYTHONPATH asks (module hijack vector)", func(t *testing.T) {
		r := evaluateAll("PYTHONPATH=. python script.py")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("FOO unknown asks", func(t *testing.T) {
		r := evaluateAll("FOO=bar pytest")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("mixed allowlist and hard-deny denies", func(t *testing.T) {
		r := evaluateAll("RUST_BACKTRACE=1 LD_PRELOAD=/x cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("stacked timeout + LD_PRELOAD denies", func(t *testing.T) {
		r := evaluateAll("timeout 30 LD_PRELOAD=/x cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("stacked timeout + RUST_BACKTRACE allows", func(t *testing.T) {
		r := evaluateAll("timeout 30 RUST_BACKTRACE=1 cargo test")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("stacked nice + BASH_ENV denies", func(t *testing.T) {
		r := evaluateAll("nice -n 5 BASH_ENV=/x cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("unknown then hard-deny still denies end-to-end", func(t *testing.T) {
		r := evaluateAll("FOO=1 LD_PRELOAD=/x cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	// Standalone assignments (no command on the same line) only flag
	// dangerous names. Unknown names auto-approve so multi-line scripts
	// like `hm_src=/path; rg ...` don't prompt on every benign local.
	t.Run("standalone unknown lowercase allows", func(t *testing.T) {
		r := evaluateAll("hm_src=/nix/store/x")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone unknown uppercase allows", func(t *testing.T) {
		r := evaluateAll("MY_PROJECT_VAR=foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone LD_PRELOAD still denies", func(t *testing.T) {
		r := evaluateAll("LD_PRELOAD=/tmp/evil")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("standalone PATH still asks", func(t *testing.T) {
		r := evaluateAll("PATH=/tmp:$PATH")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone LD_LIBRARY_PATH still asks", func(t *testing.T) {
		r := evaluateAll("LD_LIBRARY_PATH=/opt/lib")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("multi-line standalone + read-only chain allows", func(t *testing.T) {
		r := evaluateAll("hm_src=/nix/store/x\nrg pat \"$hm_src/y\" 2>/dev/null | head -30")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})
}

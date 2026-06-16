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
		{"allowlist CGO_ENABLED", []string{"CGO_ENABLED"}, true, ""},
		{"allowlist CGO_CFLAGS", []string{"CGO_CFLAGS"}, true, ""},
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

func TestEnvAllowStaticValues(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		value   string
		want    bool
	}{
		{"GOFLAGS buildvcs", "GOFLAGS", "-buildvcs=false", true},
		{"GOFLAGS tags", "GOFLAGS", "-tags=fts5", true},
		{"GOFLAGS trimpath and mod", "GOFLAGS", "-trimpath -mod=readonly", true},
		{"GOFLAGS toolexec equals", "GOFLAGS", "-toolexec=/tmp/wrap", false},
		{"GOFLAGS toolexec split", "GOFLAGS", "-toolexec /tmp/wrap", false},
		{"GOFLAGS exec equals", "GOFLAGS", "-exec=/tmp/runner", false},
		{"GOFLAGS exec split", "GOFLAGS", "-exec /tmp/runner", false},
		{"GRADLE_OPTS memory", "GRADLE_OPTS", "-Xmx2g -Dorg.gradle.daemon=false", true},
		{"GRADLE_OPTS module path", "GRADLE_OPTS", "--module-path /tmp/modules", false},
		{"_JAVA_OPTIONS memory", "_JAVA_OPTIONS", "-Xmx2g", true},
		{"_JAVA_OPTIONS OnError", "_JAVA_OPTIONS", "-XX:OnError=/tmp/hook", false},
		{"JAVA_OPTIONS memory", "JAVA_OPTIONS", "-Xmx2g -Duser.timezone=UTC", true},
		{"JAVA_OPTIONS argfile", "JAVA_OPTIONS", "@/tmp/java.args", false},
		{"JAVA_OPTS memory", "JAVA_OPTS", "-Xms512m -Xmx2g", true},
		{"JAVA_OPTS classpath", "JAVA_OPTS", "-classpath /tmp/classes", false},
		{"JAVA_TOOL_OPTIONS memory", "JAVA_TOOL_OPTIONS", "-Xmx2g -Dfile.encoding=UTF-8", true},
		{"JAVA_TOOL_OPTIONS javaagent", "JAVA_TOOL_OPTIONS", "-javaagent:/tmp/agent.jar", false},
		{"MAVEN_OPTS memory", "MAVEN_OPTS", "-Xmx2g -Dmaven.repo.local=/tmp/m2", true},
		{"MAVEN_OPTS agentlib", "MAVEN_OPTS", "-agentlib:jdwp=transport=dt_socket", false},
		{"NODE_OPTIONS memory", "NODE_OPTIONS", "--max-old-space-size=3072", true},
		{"NODE_OPTIONS require", "NODE_OPTIONS", "--require ./hook.js", false},
		{"NODE_OPTIONS loader", "NODE_OPTIONS", "--experimental-loader=./loader.mjs", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowStaticValue, ok := envAllowStaticValues[tt.envName]
			require.True(t, ok)
			assert.Equal(t, tt.want, allowStaticValue(tt.value))
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

	t.Run("PREK_ALLOW_NO_CONFIG allows git commit", func(t *testing.T) {
		r := evaluateAll("PREK_ALLOW_NO_CONFIG=1 git commit --amend --no-edit")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git write op", r.reason)
	})

	t.Run("PREK_GIT_REF allows git commit", func(t *testing.T) {
		r := evaluateAll("PREK_GIT_REF=HEAD git commit --amend --no-edit")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git write op", r.reason)
	})

	t.Run("GIT_EDITOR true allows git-spice rebase continue", func(t *testing.T) {
		r := evaluateAll("GIT_EDITOR=true git-spice rebase continue")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git-spice", r.reason)
	})

	t.Run("GIT_EDITOR true allows git-spice no-prompt rebase continue", func(t *testing.T) {
		r := evaluateAll("GIT_EDITOR=true git-spice --no-prompt rebase continue")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git-spice", r.reason)
	})

	t.Run("GIT_EDITOR true allows git rebase continue", func(t *testing.T) {
		r := evaluateAll("GIT_EDITOR=true git rebase --continue")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git write op", r.reason)
	})

	t.Run("GIT_EDITOR true allows git rebase branch", func(t *testing.T) {
		r := evaluateAll("GIT_EDITOR=true git rebase main")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+git write op", r.reason)
	})

	t.Run("GIT_EDITOR arbitrary command still asks", func(t *testing.T) {
		r := evaluateAll("GIT_EDITOR='sh -c evil' git rebase --continue")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("PYTHONPATH asks (module hijack vector)", func(t *testing.T) {
		r := evaluateAll("PYTHONPATH=. python script.py")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("NODE_OPTIONS max old space size allows bun test", func(t *testing.T) {
		r := evaluateAll(`NODE_OPTIONS="--max-old-space-size=3072" bun run test > /tmp/fulltest.log 2>&1; echo "exit=$?"; tail -8 /tmp/fulltest.log`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+bun | echo | read-only", r.reason)
	})

	t.Run("NODE_OPTIONS max old space size allows bun test in for loop", func(t *testing.T) {
		r := evaluateAll(`for f in ../packages/ui/src/stores/activity.svelte.test.ts ../packages/ui/src/components/ActivityFeed.test.ts; do NODE_OPTIONS="--max-old-space-size=2048" bun run test "$f" > /tmp/t1.log 2>&1; echo "$f exit=$?"; grep -E 'Tests |✕|× ' /tmp/t1.log | head -6; done`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "for{env vars+bun | echo | read-only | read-only}", r.reason)
	})

	t.Run("NODE_OPTIONS require still asks", func(t *testing.T) {
		r := evaluateAll(`NODE_OPTIONS="--require ./hook.js" bun run test`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("NODE_OPTIONS experimental loader still asks", func(t *testing.T) {
		r := evaluateAll(`NODE_OPTIONS="--experimental-loader ./loader.mjs" bun run test`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("NODE_OPTIONS inspect brk still asks", func(t *testing.T) {
		r := evaluateAll(`NODE_OPTIONS="--inspect-brk=0.0.0.0:9229" bun run test`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("GOFLAGS buildvcs allows go build after frontend build", func(t *testing.T) {
		repo := initGitRepo(t)
		r := evaluateAllInDir(`cd `+repo+`; make frontend > /tmp/fe2.log 2>&1 && echo "frontend built" && GOFLAGS="-buildvcs=false" go build -o ./cmd/e2e-server/e2e-server ./cmd/e2e-server && echo "e2e-server built"`, repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "cd | make | echo | env vars+go | echo", r.reason)
	})

	t.Run("GOFLAGS tags allows npm generate api", func(t *testing.T) {
		r := evaluateAll(`CGO_ENABLED=1 GOFLAGS="-tags=fts5" npm run generate:api 2>&1 | head -40`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+npm | read-only", r.reason)
	})

	t.Run("GOFLAGS toolexec still asks", func(t *testing.T) {
		r := evaluateAll(`GOFLAGS="-toolexec=/tmp/wrap" go build ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("GOFLAGS exec still asks", func(t *testing.T) {
		r := evaluateAll(`GOFLAGS="-exec=/tmp/runner" go run ./cmd/tool`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("JAVA_TOOL_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll(`JAVA_TOOL_OPTIONS="-Xmx2g -Dfile.encoding=UTF-8" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("JAVA_TOOL_OPTIONS javaagent still asks", func(t *testing.T) {
		r := evaluateAll(`JAVA_TOOL_OPTIONS="-javaagent:/tmp/agent.jar" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("JAVA_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll(`JAVA_OPTS="-Xms512m -Xmx2g" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("JAVA_OPTS classpath still asks", func(t *testing.T) {
		r := evaluateAll(`JAVA_OPTS="-classpath /tmp/classes" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("JAVA_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll(`JAVA_OPTIONS="-Xmx2g -Duser.timezone=UTC" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("JAVA_OPTIONS argfile still asks", func(t *testing.T) {
		r := evaluateAll(`JAVA_OPTIONS="@/tmp/java.args" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("_JAVA_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll(`_JAVA_OPTIONS="-Xmx2g" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("_JAVA_OPTIONS OnError still asks", func(t *testing.T) {
		r := evaluateAll(`_JAVA_OPTIONS="-XX:OnError=/tmp/hook" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("MAVEN_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll(`MAVEN_OPTS="-Xmx2g -Dmaven.repo.local=/tmp/m2" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("MAVEN_OPTS agentlib still asks", func(t *testing.T) {
		r := evaluateAll(`MAVEN_OPTS="-agentlib:jdwp=transport=dt_socket" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("GRADLE_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll(`GRADLE_OPTS="-Xmx2g -Dorg.gradle.daemon=false" go test ./...`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
		assert.Equal(t, "env vars+go", r.reason)
	})

	t.Run("GRADLE_OPTS module path still asks", func(t *testing.T) {
		r := evaluateAll(`GRADLE_OPTS="--module-path /tmp/modules" go test ./...`)
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

	t.Run("standalone NODE_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll("NODE_OPTIONS=--max-old-space-size=3072")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone NODE_OPTIONS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("NODE_OPTIONS=--require=./hook.js")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone GOFLAGS safe value allows", func(t *testing.T) {
		r := evaluateAll("GOFLAGS=-buildvcs=false")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone GOFLAGS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("GOFLAGS=-toolexec=/tmp/wrap")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone JAVA_TOOL_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll("JAVA_TOOL_OPTIONS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone JAVA_TOOL_OPTIONS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("JAVA_TOOL_OPTIONS=-javaagent:/tmp/agent.jar")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone JAVA_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll("JAVA_OPTS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone JAVA_OPTS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("JAVA_OPTS=-classpath")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone JAVA_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll("JAVA_OPTIONS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone JAVA_OPTIONS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("JAVA_OPTIONS=@/tmp/java.args")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone _JAVA_OPTIONS safe value allows", func(t *testing.T) {
		r := evaluateAll("_JAVA_OPTIONS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone _JAVA_OPTIONS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("_JAVA_OPTIONS=-XX:OnError=/tmp/hook")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone MAVEN_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll("MAVEN_OPTS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone MAVEN_OPTS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("MAVEN_OPTS=-agentlib:jdwp")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("standalone GRADLE_OPTS safe value allows", func(t *testing.T) {
		r := evaluateAll("GRADLE_OPTS=-Xmx2g")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("standalone GRADLE_OPTS dangerous value asks", func(t *testing.T) {
		r := evaluateAll("GRADLE_OPTS=--module-path")
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

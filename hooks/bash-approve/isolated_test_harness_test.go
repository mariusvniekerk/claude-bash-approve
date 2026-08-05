package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const telemetry84211 = `task_test_home=$(mktemp -d -t forge-pr818-ui-direct.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL -u KENN_FORGE_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/node/24/bin/node ../node_modules/vite-plus/bin/vp test run --project unit ../packages/ui/src/components/detail/EventTimeline.test.ts`

const guardedBunHarness = `task_frontend_home=$(mktemp -d -t forge-pr817-round3-vitest.XXXXXX); case "$task_frontend_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_frontend_home"; trap 'task_status=$?; rm -rf -- "$task_frontend_home"; exit $task_status' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL HOME="$task_frontend_home" XDG_CONFIG_HOME="$task_frontend_home/config" XDG_DATA_HOME="$task_frontend_home/data" XDG_CACHE_HOME="$task_frontend_home/cache" XDG_STATE_HOME="$task_frontend_home/state" BUN_INSTALL_CACHE_DIR="$task_frontend_home/bun-cache" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin/bun run test -- ../packages/ui/src/components/detail/EventTimeline.test.ts`

func TestGuardedIsolatedTestHarnessTelemetry(t *testing.T) {
	t.Setenv("HOME", "/Users/mariusvniekerk")

	tests := []struct {
		name    string
		command string
	}{
		{name: "telemetry 84211 vite plus", command: telemetry84211},
		{name: "guarded bun harness", command: guardedBunHarness},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.command)
			require.NotNil(t, r)
			assert.Equal(t, decisionAllow, r.decision)
			assert.Equal(t, "isolated test harness", r.reason)
		})
	}
}

func TestGuardedIsolatedTestHarnessRejectsBoundaryMutations(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "mktemp must create directory",
			command: strings.Replace(telemetry84211, "mktemp -d -t", "mktemp -t", 1),
		},
		{
			name:    "case guard cannot widen roots",
			command: strings.Replace(telemetry84211, "/var/folders/*)", "/var/folders/*|$HOME/*)", 1),
		},
		{
			name:    "chmod must target temp variable",
			command: strings.Replace(telemetry84211, `chmod 700 "$task_test_home"`, `chmod 700 "$other_home"`, 1),
		},
		{
			name:    "trap must remove temp variable",
			command: strings.Replace(telemetry84211, `rm -rf -- "$task_test_home"`, `rm -rf -- "$other_home"`, 1),
		},
		{
			name:    "trap cannot run extra command",
			command: strings.Replace(telemetry84211, `rm -rf -- "$task_test_home"'`, `rm -rf -- "$task_test_home"; echo leaked'`, 1),
		},
		{
			name:    "home must be temp variable",
			command: strings.Replace(telemetry84211, `HOME="$task_test_home"`, `HOME=/tmp/shared-home`, 1),
		},
		{
			name:    "xdg path must descend from temp variable",
			command: strings.Replace(telemetry84211, `XDG_CONFIG_HOME="$task_test_home/config"`, `XDG_CONFIG_HOME="$task_test_home/../config"`, 1),
		},
		{
			name:    "path cannot contain empty component",
			command: strings.Replace(telemetry84211, `node/24/bin:/usr/bin`, `node/24/bin::/usr/bin`, 1),
		},
		{
			name:    "node script must be vite plus",
			command: strings.Replace(telemetry84211, `../node_modules/vite-plus/bin/vp`, `./arbitrary-script.js`, 1),
		},
		{
			name:    "extra shell statement is outside harness",
			command: telemetry84211 + `; echo extra`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.command)
			if r != nil {
				assert.NotEqual(t, decisionAllow, r.decision)
			}
		})
	}
}

func TestGuardedIsolatedTestHarnessPropagatesRunnerDeny(t *testing.T) {
	t.Setenv("HOME", "/Users/mariusvniekerk")
	command := strings.Replace(
		telemetry84211,
		`/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin/node ../node_modules/vite-plus/bin/vp test run --project unit ../packages/ui/src/components/detail/EventTimeline.test.ts`,
		"git stash",
		1,
	)

	r := evaluateAll(command)
	require.NotNil(t, r)
	assert.Equal(t, decisionDeny, r.decision)
}

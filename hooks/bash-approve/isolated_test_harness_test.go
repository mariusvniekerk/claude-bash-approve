package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

const telemetry84211 = `task_test_home=$(mktemp -d -t forge-pr818-ui-direct.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL -u KENN_FORGE_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/node/24/bin/node ../node_modules/vite-plus/bin/vp test run --project unit ../packages/ui/src/components/detail/EventTimeline.test.ts`

const guardedBunHarness = `task_frontend_home=$(mktemp -d -t forge-pr817-round3-vitest.XXXXXX); case "$task_frontend_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_frontend_home"; trap 'task_status=$?; rm -rf -- "$task_frontend_home"; exit $task_status' EXIT; env -u GIT_CONFIG -u KENN_CONFIG -u KENN_API_URL HOME="$task_frontend_home" XDG_CONFIG_HOME="$task_frontend_home/config" XDG_DATA_HOME="$task_frontend_home/data" XDG_CACHE_HOME="$task_frontend_home/cache" XDG_STATE_HOME="$task_frontend_home/state" BUN_INSTALL_CACHE_DIR="$task_frontend_home/bun-cache" PATH="/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin:/Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin:/usr/bin:/bin:/usr/sbin:/sbin" /Users/mariusvniekerk/.local/share/mise/installs/bun/1.3.14/bin/bun run test -- ../packages/ui/src/components/detail/EventTimeline.test.ts`

const telemetry93386 = `task_verify_root="$(/usr/bin/mktemp -d -t thimble-transient-verify.XXXXXX)"
case "$task_verify_root" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac
/bin/chmod 700 "$task_verify_root"
/bin/mkdir -p "$task_verify_root/home" "$task_verify_root/config" "$task_verify_root/data" "$task_verify_root/cache" "$task_verify_root/state"
trap '/bin/rm -rf -- "$task_verify_root"' EXIT
/usr/bin/env -u GIT_CONFIG -u THIMBLE_CODESIGN_IDENTITY -u CLI_PROXY_API_URL -u CLIPROXY_API_URL HOME="$task_verify_root/home" XDG_CONFIG_HOME="$task_verify_root/config" XDG_DATA_HOME="$task_verify_root/data" XDG_CACHE_HOME="$task_verify_root/cache" XDG_STATE_HOME="$task_verify_root/state" /usr/bin/xcrun swift test --scratch-path "$task_verify_root/swift-build"
/usr/bin/env -u GIT_CONFIG -u THIMBLE_CODESIGN_IDENTITY -u CLI_PROXY_API_URL -u CLIPROXY_API_URL HOME="$task_verify_root/home" XDG_CONFIG_HOME="$task_verify_root/config" XDG_DATA_HOME="$task_verify_root/data" XDG_CACHE_HOME="$task_verify_root/cache" XDG_STATE_HOME="$task_verify_root/state" /usr/bin/xcrun swift build -c release --product Thimble --scratch-path "$task_verify_root/swift-build"`

const telemetry93387 = `/Users/mariusvniekerk/.local/share/mise/installs/go/1.26.5/bin/gofmt -w nexus/admin/package_control_test.go nexus/kata/binding_test.go spoke/localstate/migrations_test.go; task_test_home=$(mktemp -d -t kenn-pr199-regressions.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; env -u GIT_CONFIG -u KENN_FORGE_RUNTIME_SESSION_KEY -u KENN_HOME -u KENN_CONFIG -u KENN_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" GOPATH='/Users/mariusvniekerk/go' GOCACHE="$task_test_home/go-build" KENN_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:32771/postgres?sslmode=disable' /Users/mariusvniekerk/.local/share/mise/installs/go/1.26.5/bin/go test ./nexus/admin ./nexus/kata ./spoke/localstate -run '(TestPackageSnapshotImportUsesAuthenticatedSourceAndCommitsAudit|TestReconcileProjectBindingConvergesConcurrentExactCalls|TestStaleEnsureFailureDoesNotOverwriteReadyBinding|TestReconcileProjectBindingArchivesOnlyAfterHumanRetirement|TestReconcileProjectBindingCreatesOneStableKataProject|TestRunMigrations_CreatesFreshRecordPlacementSchema|TestLatestMigrationVersion_ReadsEmbeddedFiles)' -shuffle=on`

const telemetry93388 = `task_test_home=$(mktemp -d -t kenn-pr199-openapi.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; task_tool_path='/Users/mariusvniekerk/.local/share/mise/installs/go/1.26.5/bin'; env -u GIT_CONFIG -u KENN_FORGE_RUNTIME_SESSION_KEY -u KENN_HOME -u KENN_CONFIG -u KENN_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" GOPATH='/Users/mariusvniekerk/go' GOCACHE="$task_test_home/go-build" PATH="$task_tool_path:$PATH" make openapi-check`

const telemetry93389 = `task_test_home=$(mktemp -d -t kenn-pr199-full-final.XXXXXX); case "$task_test_home" in /tmp/*|/private/tmp/*|/var/folders/*) ;; *) exit 91 ;; esac; chmod 700 "$task_test_home"; trap 'rm -rf -- "$task_test_home"' EXIT; env -u GIT_CONFIG -u KENN_FORGE_RUNTIME_SESSION_KEY -u KENN_HOME -u KENN_CONFIG -u KENN_API_URL HOME="$task_test_home" XDG_CONFIG_HOME="$task_test_home/config" XDG_DATA_HOME="$task_test_home/data" XDG_CACHE_HOME="$task_test_home/cache" XDG_STATE_HOME="$task_test_home/state" GOPATH='/Users/mariusvniekerk/go' GOCACHE="$task_test_home/go-build" KENN_TEST_POSTGRES_DSN='postgres://postgres:postgres@127.0.0.1:32771/postgres?sslmode=disable' /Users/mariusvniekerk/.local/share/mise/installs/go/1.26.5/bin/go test ./... -shuffle=on`

const telemetry93376 = `task_red_tmp='/var/folders/p_/xgk1mhb53y17p5gbndxpdhv40000gn/T/kenn-pr199-red.XXXXXX.4tB1vl9pIH'; case "$task_red_tmp" in /var/folders/*/T/kenn-pr199-red.*) ;; *) exit 91 ;; esac; test -d "$task_red_tmp"; chmod -R u+w "$task_red_tmp"; rm -rf -- "$task_red_tmp"; test ! -e "$task_red_tmp"`

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

func TestGeneralGuardedHarnessTelemetry(t *testing.T) {
	t.Setenv("HOME", "/Users/mariusvniekerk")
	t.Setenv("TMPDIR", "/var/folders/p_/xgk1mhb53y17p5gbndxpdhv40000gn/T")

	tests := []struct {
		name    string
		command string
	}{
		{name: "swift test and build", command: telemetry93386},
		{name: "safe formatter before go test", command: telemetry93387},
		{name: "tool path before make", command: telemetry93388},
		{name: "go full suite", command: telemetry93389},
		{
			name: "safe command between trap and runner",
			command: strings.Replace(
				telemetry93389,
				`trap 'rm -rf -- "$task_test_home"' EXIT; env`,
				`trap 'rm -rf -- "$task_test_home"' EXIT; /Users/mariusvniekerk/.local/share/mise/installs/go/1.26.5/bin/go version; env`,
				1,
			),
		},
		{name: "guarded existing temp cleanup", command: telemetry93376},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.command)
			require.NotNil(t, r)
			assert.Equal(t, decisionAllow, r.decision)
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
			name: "trap status cannot overwrite temp variable",
			command: strings.Replace(
				telemetry84211,
				`trap 'rm -rf -- "$task_test_home"' EXIT`,
				`trap 'task_test_home=$?; rm -rf -- "$task_test_home"; exit $task_test_home' EXIT`,
				1,
			),
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
			name:    "unsafe surrounding statement propagates",
			command: telemetry84211 + `; git stash`,
		},
		{
			name: "temp variable cannot be reassigned after trap",
			command: strings.Replace(
				telemetry84211,
				`trap 'rm -rf -- "$task_test_home"' EXIT; env`,
				`trap 'rm -rf -- "$task_test_home"' EXIT; task_test_home=/; env`,
				1,
			),
		},
		{
			name: "surrounding statement cannot redirect temp descendants",
			command: strings.Replace(
				telemetry84211,
				`trap 'rm -rf -- "$task_test_home"' EXIT; env`,
				`trap 'rm -rf -- "$task_test_home"' EXIT; ln -s /tmp/escape "$task_test_home/config"; env`,
				1,
			),
		},
		{
			name:    "pre harness eval cannot redefine lifecycle commands",
			command: `eval 'mktemp() { printf /; }'; ` + telemetry84211,
		},
		{
			name:    "command wrapped eval cannot redefine lifecycle commands",
			command: `command eval 'mktemp() { printf /; }'; ` + telemetry84211,
		},
		{
			name: "nameref cannot mutate temp variable",
			command: strings.Replace(
				telemetry84211,
				`trap 'rm -rf -- "$task_test_home"' EXIT; env`,
				`trap 'rm -rf -- "$task_test_home"' EXIT; declare -n task_alias=task_test_home; task_alias=/; env`,
				1,
			),
		},
		{
			name: "command wrapped unset cannot mutate temp variable",
			command: strings.Replace(
				telemetry84211,
				`trap 'rm -rf -- "$task_test_home"' EXIT; env`,
				`trap 'rm -rf -- "$task_test_home"' EXIT; command unset task_test_home; env`,
				1,
			),
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

func TestGuardedHarnessPropagatesEnvironmentAndRunnerDecisions(t *testing.T) {
	t.Setenv("HOME", "/Users/mariusvniekerk")

	tests := []struct {
		name     string
		old      string
		new      string
		expected string
	}{
		{
			name:     "dangerous derived environment denies",
			old:      `HOME="$task_test_home"`,
			new:      `BASH_ENV="$task_test_home/bash-env" HOME="$task_test_home"`,
			expected: decisionDeny,
		},
		{
			name:     "unknown static environment asks",
			old:      `HOME="$task_test_home"`,
			new:      `UNKNOWN_RUNTIME=value HOME="$task_test_home"`,
			expected: decisionAsk,
		},
		{
			name:     "unknown derived environment asks",
			old:      `HOME="$task_test_home"`,
			new:      `UNKNOWN_CACHE="$task_test_home/unknown-cache" HOME="$task_test_home"`,
			expected: decisionAsk,
		},
		{
			name:     "no opinion runner remains no opinion",
			old:      `/Users/mariusvniekerk/.local/share/mise/installs/node/24/bin/node ../node_modules/vite-plus/bin/vp test run --project unit ../packages/ui/src/components/detail/EventTimeline.test.ts`,
			new:      `git push origin main`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := strings.Replace(telemetry84211, tt.old, tt.new, 1)
			r := evaluateAll(command)
			require.NotNil(t, r)
			assert.Equal(t, tt.expected, r.decision)
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

func TestGuardedKnownTempCleanupRejectsMalformedProbe(t *testing.T) {
	command := strings.Replace(telemetry93376, `test -d "$task_red_tmp"`, `task_probe=1`, 1)
	assert.NotPanics(t, func() {
		r := evaluateAll(command)
		if r != nil {
			assert.NotEqual(t, decisionAllow, r.decision)
		}
	})
}

func TestGuardedKnownTempCleanupRejectsUnsafeASTShapes(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "indexed temp assignment",
			command: strings.Replace(telemetry93376, `task_red_tmp=`, `task_red_tmp[0]=`, 1),
		},
		{
			name:    "case fallthrough",
			command: strings.Replace(telemetry93376, `) ;; *)`, `) ;& *)`, 1),
		},
		{
			name:    "fallback command assignment",
			command: strings.Replace(telemetry93376, `*) exit 91`, `*) MARKER=value exit 91`, 1),
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

	assert.NotPanics(t, func() {
		assert.False(t, guardedVarTest(
			&syntax.Stmt{Cmd: &syntax.CallExpr{}},
			"task_red_tmp",
			false,
			evalContext{},
		))
	})
}

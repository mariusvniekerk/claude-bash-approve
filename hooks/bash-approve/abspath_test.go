package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSafeAbsolutePath(t *testing.T) {
	t.Setenv("HOME", "/tmp/test-home")

	tests := []struct {
		name         string
		prefix       string
		wantNil      bool
		wantDecision string
	}{
		{"/usr/bin/", "/usr/bin/", true, ""},
		{"/usr/local/bin/", "/usr/local/bin/", true, ""},
		{"/bin/", "/bin/", true, ""},
		{"/sbin/", "/sbin/", true, ""},
		{"/opt/homebrew/bin/", "/opt/homebrew/bin/", true, ""},
		{"/opt/homebrew/sbin/", "/opt/homebrew/sbin/", true, ""},
		{"/opt/local/bin/", "/opt/local/bin/", true, ""},
		{"/opt/local/sbin/", "/opt/local/sbin/", true, ""},
		{"/nix/store/", "/nix/store/abc-coreutils/bin/", true, ""},
		{"/run/current-system/", "/run/current-system/sw/bin/", true, ""},
		{"/etc/profiles/per-user/", "/etc/profiles/per-user/me/bin/", true, ""},
		{"$HOME/.cargo/bin/", "/tmp/test-home/.cargo/bin/", true, ""},
		{"$HOME/.rustup/", "/tmp/test-home/.rustup/toolchains/stable/bin/", true, ""},
		{"$HOME/.local/bin/", "/tmp/test-home/.local/bin/", true, ""},
		{"$HOME/go/bin/", "/tmp/test-home/go/bin/", true, ""},
		{"$HOME/.bun/bin/", "/tmp/test-home/.bun/bin/", true, ""},
		{"$HOME/.nvm/", "/tmp/test-home/.nvm/versions/node/v18.0.0/bin/", true, ""},
		{"$HOME/.fnm/", "/tmp/test-home/.fnm/aliases/", true, ""},
		{"$HOME/.npm/", "/tmp/test-home/.npm/_cacache/", true, ""},
		{"$HOME/.pyenv/", "/tmp/test-home/.pyenv/shims/", true, ""},
		{"$HOME/.nix-profile/", "/tmp/test-home/.nix-profile/bin/", true, ""},
		{"$HOME/.local/share/mise/installs/", "/tmp/test-home/.local/share/mise/installs/github-golangci-golangci-lint/2.12.2/", true, ""},
		{"$HOME/.local/share/mise/shims/", "/tmp/test-home/.local/share/mise/shims/", true, ""},
		{"/opt/sketchy/ unknown", "/opt/sketchy/", false, decisionAsk},
		{"/tmp/ unknown", "/tmp/", false, decisionAsk},
		{"/tmp/anything/ unknown", "/tmp/anything/", false, decisionAsk},
		{"/var/ unknown", "/var/", false, decisionAsk},

		// Path-traversal escapes — the regex captures /usr/.. but Clean
		// resolves it to a non-allowlist directory so the validator asks.
		{"/usr/../../tmp/ traversal", "/usr/../../tmp/", false, decisionAsk},
		{"$HOME/.cargo/bin/../../etc/ traversal", "/tmp/test-home/.cargo/bin/../../etc/", false, decisionAsk},
		{"/opt/homebrew/bin/../../foo/ traversal", "/opt/homebrew/bin/../../foo/", false, decisionAsk},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := isSafeAbsolutePath(tt.prefix, evalContext{})
			if tt.wantNil {
				assert.Nil(t, r)
			} else {
				require.NotNil(t, r)
				assert.Equal(t, tt.wantDecision, r.decision)
			}
		})
	}
}

func TestIsSafeAbsolutePath_RepoPathAsks(t *testing.T) {
	// Absolute paths that resolve into the current repo are NOT
	// auto-approved: a repo can contain attacker- or model-supplied
	// binaries whose basenames match allow-listed commands (e.g.
	// /repo/bin/git running arbitrary repo-controlled code instead of
	// system git). Repo paths must ask.
	repo := initGitRepo(t)
	scriptsPrefix := filepath.Join(repo, "scripts") + "/"
	r := isSafeAbsolutePath(scriptsPrefix, evalContext{cwd: repo})
	require.NotNil(t, r)
	assert.Equal(t, decisionAsk, r.decision)
}

func TestIsSafeAbsolutePath_NoHome(t *testing.T) {
	t.Setenv("HOME", "")

	// $HOME-relative entries are skipped when HOME is empty.
	r := isSafeAbsolutePath("/tmp/.cargo/bin/", evalContext{})
	require.NotNil(t, r)
	assert.Equal(t, decisionAsk, r.decision)
}

func TestEvaluate_AbsolutePathFlows(t *testing.T) {
	t.Run("/usr/bin/cat allowed", func(t *testing.T) {
		r := evaluateAll("/usr/bin/cat foo")
		require.NotNil(t, r)
		assert.Equal(t, "absolute path+read-only", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("/tmp/cat asks", func(t *testing.T) {
		r := evaluateAll("/tmp/cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("/tmp/git stash stays deny", func(t *testing.T) {
		// Override merge: git stash is deny, abs-path validator returns ask, deny wins.
		r := evaluateAll("/tmp/git stash")
		require.NotNil(t, r)
		assert.Equal(t, decisionDeny, r.decision)
	})

	t.Run("path-traversal escape from /usr/ asks", func(t *testing.T) {
		// /usr/../../tmp/cat resolves to /tmp/cat — the cleaned prefix
		// must not match /usr/.
		r := evaluateAll("/usr/../../tmp/cat foo")
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("mise-installed golangci-lint with safe goflags allowed", func(t *testing.T) {
		t.Setenv("HOME", "/tmp/test-home")

		r := evaluateAll(`GOFLAGS="-tags=fts5" /tmp/test-home/.local/share/mise/installs/github-golangci-golangci-lint/2.12.2/golangci-lint run --enable-only unused ./internal/parser/ 2>&1 | head -20`)
		require.NotNil(t, r)
		assert.Equal(t, "env vars+absolute path+golangci-lint | read-only", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})
}

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedirectSafeWritePrefixes(t *testing.T) {
	repo := initGitRepo(t)
	tmpDir := t.TempDir()

	t.Run("redirect to /tmp allows", func(t *testing.T) {
		r := evaluateAllInDir("git status > /tmp/before.txt", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("redirect to /var/tmp allows", func(t *testing.T) {
		r := evaluateAllInDir("git status > /var/tmp/before.txt", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("append to /tmp allows", func(t *testing.T) {
		r := evaluateAllInDir("git log >> /tmp/log.txt", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("redirect to /etc still asks", func(t *testing.T) {
		r := evaluateAllInDir("git status > /etc/hosts", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("redirect to home still asks", func(t *testing.T) {
		r := evaluateAllInDir("git status > /home/cloud/something.txt", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("redirect to in-repo path still allows", func(t *testing.T) {
		r := evaluateAllInDir("git status > scratch.log", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("redirect to /tmp via outside-prefix path asks", func(t *testing.T) {
		// /tmpfoo doesn't start with /tmp/ — must not match the prefix.
		r := evaluateAllInDir("git status > /tmpfoo", repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("redirect to symlink in /tmp escaping out asks", func(t *testing.T) {
		// Plant /tmp-test target outside /tmp, then a symlink under tmpDir
		// pointing at it. We can't actually plant a symlink in /tmp from
		// a test, so use a tmpDir-based prefix substitute via t.TempDir.
		// Instead, prove the opposite direction: a symlink whose resolved
		// target stays in /tmp/ allows.
		t.Skip("symlink-escape test requires writing to /tmp; covered indirectly by isSafeWriteTarget unit tests")
	})

	// Sanity: tmpDir from t.TempDir is rooted under the OS tmp dir on
	// Linux (/tmp/...), so a redirect to a file we just created there
	// behaves the same as the prefix check predicts.
	t.Run("redirect to t.TempDir file allows", func(t *testing.T) {
		// Some platforms (e.g. macOS) put t.TempDir under /var/folders;
		// only run the assertion when we're actually under a safe prefix.
		clean := filepath.Clean(tmpDir) + "/"
		safe := false
		for _, p := range safeWritePrefixes {
			if len(clean) >= len(p) && clean[:len(p)] == p {
				safe = true
				break
			}
		}
		if !safe {
			t.Skip("t.TempDir not under a safe prefix on this platform")
		}
		target := filepath.Join(tmpDir, "out.txt")
		require.NoError(t, os.WriteFile(target, []byte(""), 0644))
		r := evaluateAllInDir("git status > "+target, repo)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})
}

func TestIsSafeWriteTargetSymlinkEscape(t *testing.T) {
	// Build a symlink under a safe prefix that escapes to t.TempDir.
	// We can't pollute the real /tmp, so use t.TempDir which (on Linux)
	// is itself under /tmp.
	tmpDir := t.TempDir()
	clean := filepath.Clean(tmpDir) + "/"
	safe := false
	for _, p := range safeWritePrefixes {
		if len(clean) >= len(p) && clean[:len(p)] == p {
			safe = true
			break
		}
	}
	if !safe {
		t.Skip("t.TempDir not under a safe prefix on this platform")
	}

	// Outside target: write somewhere clearly not under any safe prefix.
	outside := "/etc/passwd"

	link := filepath.Join(tmpDir, "escape")
	require.NoError(t, os.Symlink(outside, link))

	t.Run("symlink escaping prefix refuses", func(t *testing.T) {
		assert.False(t, isSafeWriteTarget(link))
	})

	// Symlink staying in-prefix is fine.
	staysInside := filepath.Join(tmpDir, "stays")
	require.NoError(t, os.WriteFile(staysInside, []byte(""), 0644))
	insideLink := filepath.Join(tmpDir, "inside-link")
	require.NoError(t, os.Symlink(staysInside, insideLink))

	t.Run("symlink staying in prefix allows", func(t *testing.T) {
		assert.True(t, isSafeWriteTarget(insideLink))
	})

	t.Run("non-existent path under prefix allows", func(t *testing.T) {
		assert.True(t, isSafeWriteTarget(filepath.Join(tmpDir, "not-yet")))
	})

	t.Run("relative path refused", func(t *testing.T) {
		assert.False(t, isSafeWriteTarget("relative/path"))
	})
}

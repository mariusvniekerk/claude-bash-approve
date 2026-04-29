package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTeeInRepo(t *testing.T) {
	repo := initGitRepo(t)
	insideFile := filepath.Join(repo, "build.log")
	require.NoError(t, os.WriteFile(insideFile, []byte(""), 0644))

	t.Run("in-repo target allowed", func(t *testing.T) {
		args := parseCallArgs(t, "tee build.log")
		assert.True(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("in-repo target with -a allowed", func(t *testing.T) {
		args := parseCallArgs(t, "tee -a build.log")
		assert.True(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("absolute outside repo drops to ask", func(t *testing.T) {
		args := parseCallArgs(t, "tee /etc/passwd")
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("safe write prefix /tmp allows", func(t *testing.T) {
		args := parseCallArgs(t, "tee /tmp/scratch.log")
		assert.True(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("safe write prefix /var/tmp allows", func(t *testing.T) {
		args := parseCallArgs(t, "tee /var/tmp/scratch.log")
		assert.True(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("relative outside repo drops to ask", func(t *testing.T) {
		args := parseCallArgs(t, "tee ../../../etc/passwd")
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("bare tee drops to ask", func(t *testing.T) {
		args := parseCallArgs(t, "tee")
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("in-repo symlink to outside drops to ask", func(t *testing.T) {
		link := filepath.Join(repo, "escape")
		require.NoError(t, os.Symlink("/etc/passwd", link))
		args := parseCallArgs(t, "tee escape")
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("unquoted tilde target asks despite literal in-repo ~ dir", func(t *testing.T) {
		require.NoError(t, os.Mkdir(filepath.Join(repo, "~"), 0755))
		args := parseCallArgs(t, `tee ~/target`)
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("escaped name resolving to in-repo symlink escaping repo asks", func(t *testing.T) {
		// `tee out\*` writes to literal filename `out*`. If `out*` is
		// an in-repo symlink to a file outside the repo, the validator
		// must follow wordLiteralPath + lstat to ask, not auto-approve
		// via the parent-dir fallback.
		outside := filepath.Join(t.TempDir(), "evil")
		require.NoError(t, os.WriteFile(outside, []byte("x"), 0644))
		require.NoError(t, os.Symlink(outside, filepath.Join(repo, "out*")))
		args := parseCallArgs(t, `tee out\*`)
		assert.False(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})

	t.Run("non-existent in-repo target allowed via parent fallback", func(t *testing.T) {
		args := parseCallArgs(t, "tee not-yet-created.log")
		assert.True(t, isTeeInRepo(args, evalContext{cwd: repo}))
	})
}

func TestEvaluate_TeeFlows(t *testing.T) {
	repo := initGitRepo(t)

	t.Run("in-repo target allowed end-to-end", func(t *testing.T) {
		r := evaluateAllInDir("tee build.log", repo)
		require.NotNil(t, r)
		assert.Equal(t, "tee", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("safe write prefix allowed end-to-end", func(t *testing.T) {
		r := evaluateAllInDir("tee /tmp/output.log", repo)
		require.NotNil(t, r)
		assert.Equal(t, "tee", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("outside repo and safe prefixes drops to ask", func(t *testing.T) {
		r := evaluateAllInDir("tee /etc/passwd", repo)
		require.NotNil(t, r)
		assert.Equal(t, "tee", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChmodScopedTargetsAllowed(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	require.NoError(t, os.Mkdir(filepath.Join(repo, "generated"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(repo, "cache"), 0o755))
	root := "/var/folders/bash-approve-tests/T"
	t.Setenv("TMPDIR", root)
	tempTarget := filepath.Join(root, "forge-ci-e2e.123")

	for _, command := range []string{
		"chmod -R u+w " + tempTarget,
		"chmod 700 generated",
		"chmod -R u+rw generated cache",
		"ls -ld " + tempTarget + "; chmod -R u+w " + tempTarget + "; rm -rf " + tempTarget + "; test ! -e " + tempTarget,
	} {
		r := evaluateAllInDir(command, repo)
		require.NotNil(t, r, command)
		assert.Equal(t, decisionAllow, r.decision, command)
	}
}

func TestChmodUnprovenTargetsRemainNoOpinion(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	root := "/var/folders/bash-approve-tests/T"
	t.Setenv("TMPDIR", root)

	for _, command := range []string{
		"chmod 700 .",
		"chmod 700 " + root,
		"chmod 700 /etc/bash-approve-unsafe",
		"chmod 700 " + filepath.Join(root, "safe") + " /etc/bash-approve-unsafe",
		"chmod 700 " + filepath.Join(root, "*"),
		"target=$(date); chmod 700 \"$target\"",
		"chmod --reference=/tmp/reference " + filepath.Join(root, "safe"),
		"chmod -RL u+w " + filepath.Join(root, "safe"),
	} {
		r := evaluateAllInDir(command, repo)
		require.NotNil(t, r, command)
		assert.Empty(t, r.decision, command)
	}
}

func TestChmodSymlinkEscapeRemainsNoOpinion(t *testing.T) {
	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "-C", repo, "init").Run())
	tempRoot, err := os.MkdirTemp("/tmp", "bash-approve-chmod-symlink.")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempRoot)) })
	escape := filepath.Join(tempRoot, "escape")
	require.NoError(t, os.Symlink("/etc", escape))

	command := "chmod 700 " + filepath.Join(escape, "hosts")
	r := evaluateAllInDir(command, repo)
	require.NotNil(t, r)
	assert.Empty(t, r.decision)
}

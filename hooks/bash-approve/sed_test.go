package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

// parseCallArgs parses a single bash command string and returns the AST args
// of the resulting CallExpr. Reused by sed/awk/tee/env tests in this package.
func parseCallArgs(t *testing.T, cmd string) []*syntax.Word {
	t.Helper()
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(cmd), "")
	require.NoError(t, err)
	call := file.Stmts[0].Cmd.(*syntax.CallExpr)
	return call.Args
}

func TestIsSedSafe(t *testing.T) {
	repo := initGitRepo(t)
	insideFile := filepath.Join(repo, "in.txt")
	require.NoError(t, os.WriteFile(insideFile, []byte("hi\n"), 0644))

	tests := []struct {
		name string
		cmd  string
		ctx  evalContext
		// want is the expected validator return value. true = pass
		// (auto-approve). false = drop to ask via fallback.
		want bool
	}{
		// No -i: read-only sed always passes regardless of cwd.
		{"plain substitution", "sed 's/foo/bar/' file", evalContext{}, true},
		{"print range", "sed -n '1,5p' file", evalContext{}, true},
		{"-e expression", "sed -e 's/a/b/' -e 's/c/d/' file", evalContext{}, true},
		{"-f script", "sed -f script.sed file", evalContext{}, true},

		// -i without an in-repo cwd: nothing to validate, drop to ask.
		{"in-place short outside repo", "sed -i 's/foo/bar/' file", evalContext{}, false},
		{"in-place backup outside repo", "sed -i.bak 's/foo/bar/' file", evalContext{}, false},
		{"in-place long outside repo", "sed --in-place 's/foo/bar/' file", evalContext{}, false},
		{"escaped in-place short outside repo", `sed \-i 's/foo/bar/' file`, evalContext{}, false},
		{"escaped in-place long outside repo", `sed \--in-place 's/foo/bar/' file`, evalContext{}, false},

		// -i with an in-repo target: allowed.
		{"in-place in-repo file", "sed -i 's/foo/bar/' in.txt", evalContext{cwd: repo}, true},
		{"in-place in-repo file with backup", "sed -i.bak 's/foo/bar/' in.txt", evalContext{cwd: repo}, true},
		{"in-place long in-repo file", "sed --in-place 's/foo/bar/' in.txt", evalContext{cwd: repo}, true},
		{"in-place range edit in-repo", "sed -i '1004,1543d' in.txt", evalContext{cwd: repo}, true},
		{"in-place -e in-repo", "sed -i -e 's/x/y/' in.txt", evalContext{cwd: repo}, true},

		// -i targeting a file outside the repo: drops to ask.
		{"in-place outside repo absolute", "sed -i 's/foo/bar/' /etc/hosts", evalContext{cwd: repo}, false},
		{"in-place outside repo relative", "sed -i 's/foo/bar/' ../../etc/hosts", evalContext{cwd: repo}, false},

		// -i targeting a safe scratch prefix: allowed.
		{"in-place /tmp", "sed -i 's/foo/bar/' /tmp/scratch.txt", evalContext{cwd: repo}, true},
		{"in-place /var/tmp", "sed -i 's/foo/bar/' /var/tmp/scratch.txt", evalContext{cwd: repo}, true},

		// -i with multiple files: all must be in repo.
		{"in-place in-repo multiple files", "sed -i 's/x/y/' in.txt in.txt", evalContext{cwd: repo}, true},

		// -i with no file args: ask (no target).
		{"in-place no file", "sed -i 's/foo/bar/'", evalContext{cwd: repo}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := parseCallArgs(t, tt.cmd)
			ok := isSedSafe(args, tt.ctx)
			if tt.want {
				assert.True(t, ok, "expected validator to accept %q", tt.cmd)
			} else {
				assert.False(t, ok, "expected validator to reject %q", tt.cmd)
			}
		})
	}
}

func TestEvaluate_SedFlows(t *testing.T) {
	repo := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "test.py"), []byte("x\n"), 0644))

	t.Run("plain sed allowed", func(t *testing.T) {
		r := evaluateAll("sed 's/foo/bar/' file")
		require.NotNil(t, r)
		assert.Equal(t, "sed", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("sed -i in-repo allowed", func(t *testing.T) {
		r := evaluateAllInDir("sed -i '1004,1543d' test.py", repo)
		require.NotNil(t, r)
		assert.Equal(t, "sed", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("sed -i outside repo drops to ask", func(t *testing.T) {
		r := evaluateAllInDir("sed -i 's/foo/bar/' /etc/hosts", repo)
		require.NotNil(t, r)
		assert.Equal(t, "sed", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("sed -i without cwd drops to ask", func(t *testing.T) {
		r := evaluateAll("sed -i 's/foo/bar/' file")
		require.NotNil(t, r)
		assert.Equal(t, "sed", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindSafe(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		// basic find — no exec
		{"find simple", "find . -name '*.go'", "find"},
		{"find with type", "find . -type f -name '*.go'", "find"},
		{"find with maxdepth", "find . -maxdepth 2 -name '*.txt'", "find"},
		{"find with print", "find . -name '*.go' -print", "find"},
		{"find with print0", "find . -name '*.go' -print0", "find"},
		{"find multiple paths", "find src tests -name '*.py'", "find"},
		{"find with regex", "find . -regex '.*\\.go$'", "find"},
		{"find with size", "find . -size +1M -type f", "find"},
		{"find with mtime", "find . -mtime -7 -name '*.log'", "find"},

		// -exec with safe commands
		{"find exec safe cmd", "find . -name '*.go' -exec wc -l {} \\;", "find"},
		{"find exec git diff", "find . -name '*.go' -exec git diff {} \\;", "find"},
		{"find exec with echo", "find . -type f -exec echo {} \\;", "find"},
		{"find exec cat", "find . -name 'README*' -exec cat {} \\;", "find"},
		{"find exec head", "find . -name '*.log' -exec head -20 {} \\;", "find"},
		{"find exec grep", "find . -name '*.py' -exec grep -l TODO {} \\;", "find"},

		// -execdir with safe commands
		{"find execdir safe", "find . -name '*.py' -execdir grep TODO {} +", "find"},
		{"find execdir cat", "find . -name '*.md' -execdir cat {} \\;", "find"},

		// + terminator (batch mode)
		{"find exec batch", "find . -name '*.go' -exec wc -l {} +", "find"},

		// multiple -exec
		{"find multiple execs", "find . -name '*.go' -exec wc -l {} \\; -exec head -1 {} \\;", "find"},

		// -exec after many find flags
		{"find complex then exec", "find . -maxdepth 3 -type f -name '*.go' -not -path '*/vendor/*' -exec cat {} \\;", "find"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, "allow", r.decision)
			assert.Equal(t, tt.reason, r.reason)
		})
	}
}

func TestFindUnsafe(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		// dangerous flags
		{"find delete", "find . -name '*.tmp' -delete"},
		{"find delete after filters", "find . -type f -name '*.bak' -mtime +30 -delete"},

		// -exec with unknown commands
		{"find exec unknown", "find . -exec some-unknown-cmd {} \\;"},
		{"find execdir unknown", "find . -execdir unfamiliar-tool {} +"},

		// -exec with denied commands
		{"find exec rm -rf", "find . -name '*.bak' -exec rm -rf {} \\;"},
		{"find exec rm -r", "find . -exec rm -r {} \\;"},
		{"find execdir rm -rf", "find . -execdir rm -rf {} \\;"},

		// -ok (interactive exec — still needs validation)
		{"find ok unknown", "find . -ok some-cmd {} \\;"},
		{"find okdir unknown", "find . -okdir unknown-cmd {} \\;"},

		// multiple execs where one is unsafe
		{"find mixed execs", "find . -exec cat {} \\; -exec rm {} \\;"},

		// empty exec (no command between -exec and terminator)
		{"find exec empty", "find . -exec \\;"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "unsafe find should get ask decision for %q", tt.cmd)
		})
	}
}

func TestFindXargsNested(t *testing.T) {
	t.Run("find piped to xargs with safe command", func(t *testing.T) {
		r := evaluateAll("find . -name '*.go' -print0 | xargs -0 grep TODO")
		require.NotNil(t, r)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("find exec xargs exec safe", func(t *testing.T) {
		r := evaluateAll(`find . -name '*.txt' -exec xargs -0 cat {} \;`)
		require.NotNil(t, r)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("find exec xargs exec unsafe", func(t *testing.T) {
		r := evaluateAll(`find . -name '*.txt' -exec xargs -0 rm {} \;`)
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision)
	})

	t.Run("nested find xargs find xargs safe", func(t *testing.T) {
		r := evaluateAll(`find . -name Makefile -exec xargs -I {} find {} -name '*.o' -exec xargs cat {} \; \;`)
		require.NotNil(t, r)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("nested find xargs find xargs unsafe at leaf", func(t *testing.T) {
		r := evaluateAll(`find . -name Makefile -exec xargs -I {} find {} -name '*.o' -exec xargs rm {} \; \;`)
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision)
	})
}

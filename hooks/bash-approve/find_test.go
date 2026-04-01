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
		{"find simple", "find . -name '*.go'", "find"},
		{"find with type", "find . -type f -name '*.go'", "find"},
		{"find with maxdepth", "find . -maxdepth 2 -name '*.txt'", "find"},
		{"find exec safe cmd", "find . -name '*.go' -exec wc -l {} \\;", "find"},
		{"find exec git diff", "find . -name '*.go' -exec git diff {} \\;", "find"},
		{"find execdir safe", "find . -name '*.py' -execdir grep TODO {} +", "find"},
		{"find exec with echo", "find . -type f -exec echo {} \\;", "find"},
		{"find exec cat", "find . -name 'README*' -exec cat {} \\;", "find"},
		{"find multiple execs", "find . -name '*.go' -exec wc -l {} \\; -exec head -1 {} \\;", "find"},
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
		{"find delete", "find . -name '*.tmp' -delete"},
		{"find exec unknown", "find . -exec some-unknown-cmd {} \\;"},
		{"find execdir unknown", "find . -execdir unfamiliar-tool {} +"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "unsafe find should get ask decision for %q", tt.cmd)
		})
	}
}

func TestFindExecDenied(t *testing.T) {
	t.Run("find exec with denied command", func(t *testing.T) {
		r := evaluateAll("find . -name '*.bak' -exec rm -rf {} \\;")
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision, "find -exec with denied command should get ask")
	})
}

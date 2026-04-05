package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXargsSafe(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		// bare xargs (defaults to echo)
		{"xargs bare", "xargs", "xargs"},

		// safe commands
		{"xargs echo", "xargs echo", "xargs"},
		{"xargs grep", "xargs grep TODO", "xargs"},
		{"xargs cat", "xargs cat", "xargs"},
		{"xargs wc", "xargs wc -l", "xargs"},
		{"xargs head", "xargs head -5", "xargs"},

		// xargs flags before safe commands
		{"xargs null", "xargs -0 wc -l", "xargs"},
		{"xargs max-args", "xargs -n 1 cat", "xargs"},
		{"xargs replace", "xargs -I {} git diff {}", "xargs"},
		{"xargs parallel", "xargs -P 4 grep pattern", "xargs"},
		{"xargs delimiter", "xargs -d '\\n' echo", "xargs"},
		{"xargs multiple flags", "xargs -0 -n 1 -P 4 grep pattern", "xargs"},
		{"xargs no-run-if-empty", "xargs -r cat", "xargs"},

		// long flags
		{"xargs long null", "xargs --null wc -l", "xargs"},
		{"xargs long max-args", "xargs --max-args 1 cat", "xargs"},
		{"xargs long max-args=", "xargs --max-args=1 head -5", "xargs"},
		{"xargs long combined", "xargs --null --no-run-if-empty --max-args=1 echo", "xargs"},
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

func TestXargsUnsafe(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		// unknown commands
		{"xargs unknown cmd", "xargs some-unknown-cmd"},
		{"xargs unknown after flags", "xargs -0 -n 1 unknown-tool"},

		// rm variants
		{"xargs rm", "xargs rm"},
		{"xargs rm with flags", "xargs -0 -n 1 rm"},
		{"xargs rm -rf", "xargs -0 rm -rf"},
		{"xargs rm -r", "xargs rm -r"},

		// other dangerous commands
		{"xargs git stash", "xargs git stash"},

		// all xargs flags consumed, then dangerous command
		{"xargs many flags then rm", "xargs -0 -r -n 1 -P 4 rm"},
		{"xargs long flags then rm", "xargs --null --no-run-if-empty --max-args=1 rm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "unsafe xargs should get ask decision for %q", tt.cmd)
		})
	}
}

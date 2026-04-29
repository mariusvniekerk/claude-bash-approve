package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPixiRunSafe(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"pixi run pytest", "pixi run pytest tests/"},
		{"pixi run -e env pytest", "pixi run -e cu13 pytest tests/test_program_cache.py"},
		{"pixi run --manifest-path -e env pytest", "pixi run --manifest-path . -e cu13 pytest tests/test_program_cache.py"},
		{"pixi run --frozen pytest", "pixi run --frozen pytest"},
		{"pixi run --manifest-path=. pytest", "pixi run --manifest-path=. pytest tests/"},
		{"pixi run -- pytest", "pixi run -- pytest tests/"},
		{"pixi run rg pattern", "pixi run rg pattern path"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected approval for %q, got rejected", tt.cmd)
			assert.Equal(t, "allow", r.decision, "expected allow for %q, got %q (reason=%q)", tt.cmd, r.decision, r.reason)
		})
	}
}

func TestPixiRunUnsafe(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		// Inner command isn't auto-approve.
		{"pixi run rm", "pixi run rm important.txt"},
		{"pixi run unknown", "pixi run some-unknown-tool"},
		// No command after flags.
		{"pixi run no command", "pixi run"},
		{"pixi run only flags", "pixi run -e cu13 --frozen"},
		// Unknown short flag — be conservative.
		{"pixi run unknown short flag", "pixi run -X pytest"},
		// Non-literal flag word — `$X` parameter expansion can't be
		// statically resolved.
		{"pixi run dynamic flag", `pixi run "$X" pytest`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "expected ask for %q, got %q", tt.cmd, r.decision)
		})
	}
}

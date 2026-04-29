package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFdUnsafe(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
	}{
		{"fd -x rm", "fd -t f '\\.tmp$' -x rm {} \\;"},
		{"fd -X rm", "fd -t f '\\.tmp$' -X rm"},
		{"fd --exec rm", "fd -t f '\\.tmp$' --exec rm {} \\;"},
		{"fd --exec-batch unknown", "fd -t f '\\.tmp$' --exec-batch some-unknown-tool"},
		{"fd -x empty", "fd -t f '\\.tmp$' -x"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "expected ask for %q, got %q", tt.cmd, r.decision)
		})
	}
}

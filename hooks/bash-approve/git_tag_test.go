package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitTagDecision(t *testing.T) {
	readOnly := []string{
		"git tag",
		"git -C /repo tag",
		"git tag -l 'v*'",
		"git tag --list",
		"git tag --sort=-creatordate",
		"git tag --contains 7088ce0",
		"git tag --no-contains 7088ce0",
		"git tag --points-at HEAD",
		"git tag --merged main",
		"git tag --no-merged main",
	}
	for _, cmd := range readOnly {
		t.Run("allow "+cmd, func(t *testing.T) {
			r := evaluateAll(cmd)
			require.NotNil(t, r)
			assert.Equal(t, "git read op", r.reason)
			assert.Equal(t, decisionAllow, r.decision)
		})
	}

	mutating := []string{
		"git tag v1.0.0",
		"git tag -a v1.0.0 -m release",
		"git tag -d v1.0.0",
		"git tag -f v1.0.0 HEAD",
		"git tag -v v1.0.0",
	}
	for _, cmd := range mutating {
		t.Run("ask "+cmd, func(t *testing.T) {
			r := evaluateAll(cmd)
			require.NotNil(t, r)
			assert.Equal(t, "git tag", r.reason)
			assert.Equal(t, decisionAsk, r.decision)
		})
	}

	ambiguous := []string{
		"git tag --sort=-creatordate -d v1.0.0",
		`git tag --list "$pattern"`,
		"git tag --unknown-option",
	}
	for _, cmd := range ambiguous {
		t.Run("ask ambiguous "+cmd, func(t *testing.T) {
			r := evaluateAll(cmd)
			require.NotNil(t, r)
			assert.Equal(t, decisionAsk, r.decision)
		})
	}

	t.Run("telemetry chain allows", func(t *testing.T) {
		r := evaluateAll("git tag | tail -5; echo ---; gh release list --repo kenn-io/agentsview --limit 3 2>&1")
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})
}

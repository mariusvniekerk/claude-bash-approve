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
		{"xargs attached -I no template", "xargs -I{} true", "xargs"},
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

		// quoted/escaped command names — wordLiteral decoding makes
		// these resolve to the same dangerous tool the shell would invoke.
		{"xargs single-quoted rm", "xargs 'rm'"},
		{"xargs ANSI-C-quoted rm", `xargs $'rm'`},
		{"xargs backslash rm", `xargs \rm`},

		// argv boundary — single quoted argv element with embedded
		// space must not be reassembled as two argv elements.
		{"xargs quoted command with space", `xargs 'git status'`},

		// -I/--replace substitutes input into the command template at
		// runtime, so the validator cannot know what the substituted
		// value is.
		{"xargs -I tee placeholder", "xargs -I {} tee {}"},
		{"xargs -I git diff placeholder", "xargs -I {} git diff {}"},
		{"xargs --replace tee", "xargs --replace=PH tee PH"},
		{"xargs -I custom token", "xargs -I PH echo PH"},

		// Default xargs (no -I) appends each input record as
		// additional positional arguments. For commands not in the
		// safe-append list, that runtime tail can change the approval
		// decision.
		{"xargs touch (write command)", "xargs touch"},
		{"xargs mkdir (write command)", "xargs mkdir"},
		{"xargs git add direct", "xargs git add --"},
		{"xargs less (log-file option)", "xargs less"},
		{"xargs more (option-driven)", "xargs more"},
		{"xargs yq (in-place option)", "xargs yq"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected (nil)", tt.cmd)
			assert.Equal(t, "ask", r.decision, "unsafe xargs should get ask decision for %q", tt.cmd)
		})
	}
}

func TestXargsReadOnlyPipelineCanAppendToAllowedCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{
			name: "telemetry conflict resolution chain",
			cmd:  `git diff --name-only --diff-filter=U | grep -v -e '^frontend/' -e '^README\.md' | tr '\n' '\0' | xargs -0 git checkout --theirs -- && git diff --name-only --diff-filter=U | grep -v -e '^frontend/' -e '^README\.md' | tr '\n' '\0' | xargs -0 git add -- && git diff --name-only --diff-filter=U`,
		},
		{
			name: "git read pipeline to git add",
			cmd:  `git diff --name-only --diff-filter=U | grep -v -e '^frontend/' | xargs git add --`,
		},
		{
			name: "git read pipeline to touch",
			cmd:  `git ls-files | xargs touch`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNil(t, r)
			assert.Equal(t, decisionAllow, r.decision)
		})
	}
}

func TestXargsReadOnlyPipelineBoundaries(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"synthetic input is not trusted", `printf 'README.md\0' | xargs -0 git add --`},
		{"placeholder replacement still asks", `git diff --name-only | xargs -I {} git add {}`},
		{"dangerous command still asks", `git diff --name-only | xargs git stash`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNil(t, r)
			assert.Equal(t, decisionAsk, r.decision)
		})
	}
}

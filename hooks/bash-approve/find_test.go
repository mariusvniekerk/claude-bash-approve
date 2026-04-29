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

		// -exec with safe commands runs them per match. The inner
		// command is evaluated through the normal pipeline; if it
		// would auto-approve standalone, it auto-approves under -exec
		// regardless of whether it uses the `{}` placeholder.
		{"find exec true", "find . -name '*.go' -exec true \\;", "find"},
		{"find exec git status no placeholder", "find . -name '*.go' -exec git status \\;", "find"},
		{"find exec wc placeholder", "find . -name '*.go' -exec wc -l {} \\;", "find"},
		{"find exec git diff placeholder", "find . -name '*.go' -exec git diff {} \\;", "find"},
		{"find exec echo placeholder", "find . -type f -exec echo {} \\;", "find"},
		{"find exec cat placeholder", "find . -name 'README*' -exec cat {} \\;", "find"},
		{"find exec head placeholder", "find . -name '*.log' -exec head -20 {} \\;", "find"},
		{"find exec grep placeholder", "find . -name '*.py' -exec grep -l TODO {} \\;", "find"},
		{"find exec rg placeholder batch", "find . -name '*.go' -exec rg -h pattern {} +", "find"},
		{"find exec wc batch placeholder", "find . -name '*.go' -exec wc -l {} +", "find"},
		{"find multiple safe execs placeholder", "find . -name '*.go' -exec wc -l {} \\; -exec head -1 {} \\;", "find"},
		{"find complex then exec placeholder", "find . -maxdepth 3 -type f -name '*.go' -not -path '*/vendor/*' -exec cat {} \\;", "find"},
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

		// quoted/escaped command names — wordLiteral decoding ensures
		// the inner exec target is recognized regardless of quoting.
		{"find exec single-quoted rm", `find . -exec 'rm' {} \;`},
		{"find exec ANSI-C-quoted rm", `find . -exec $'rm' {} \;`},
		{"find exec backslash rm", `find . -exec \rm {} \;`},

		// argv boundary — single quoted argv element with embedded
		// space must not be reassembled as two argv elements.
		{"find exec quoted command with space", `find . -exec 'git status' {} \;`},

		// -execdir / -okdir change cwd to the matched file's directory;
		// the validator can't model the changed cwd so they always ask.
		{"find execdir grep", "find . -name '*.py' -execdir grep TODO {} +"},
		{"find execdir cat", "find . -name '*.md' -execdir cat {} \\;"},
		{"find execdir safe cmd no placeholder", "find . -execdir true \\;"},

		// File-output actions that open their next arg for writing.
		{"find fprint outside repo", "find . -maxdepth 0 -fprint /tmp/log"},
		{"find fprint home", "find . -fprint ~/.bashrc"},
		{"find fprint0 outside repo", "find . -fprint0 /tmp/null-list"},
		{"find fprintf outside repo", "find . -fprintf /tmp/out '%p\\n'"},
		{"find fls outside repo", "find . -fls /tmp/listing"},

		// Non-literal find arg can expand into a dangerous predicate
		// at runtime (e.g. -delete via $(printf ...)).
		{"find with non-literal arg expanding to -delete", `find . $(printf %s -delete)`},
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

	t.Run("find exec with safe xargs cat allows", func(t *testing.T) {
		// xargs -0 cat {} is a chain of read-only operations: the
		// outer find's {} is a filename; xargs appends stdin records
		// to the cat invocation. cat is in xargsSafeAppendCommands,
		// so the inner xargs validator accepts.
		r := evaluateAll(`find . -name '*.txt' -exec xargs -0 cat {} \;`)
		require.NotNil(t, r)
		assert.Equal(t, "allow", r.decision)
	})

	t.Run("find exec xargs exec unsafe", func(t *testing.T) {
		r := evaluateAll(`find . -name '*.txt' -exec xargs -0 rm {} \;`)
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision)
	})

	t.Run("nested find xargs with placeholder asks", func(t *testing.T) {
		r := evaluateAll(`find . -name Makefile -exec xargs -I {} find {} -name '*.o' -exec xargs cat {} \; \;`)
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision)
	})

	t.Run("nested find xargs find xargs unsafe at leaf", func(t *testing.T) {
		r := evaluateAll(`find . -name Makefile -exec xargs -I {} find {} -name '*.o' -exec xargs rm {} \; \;`)
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision)
	})
}

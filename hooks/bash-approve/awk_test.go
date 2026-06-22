package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAwkSafe(t *testing.T) {
	tests := []struct {
		name         string
		cmd          string
		wantOverride bool
	}{
		{"simple field print", "awk '{print $1}' file", false},
		{"reading via < is fine", `awk 'BEGIN{getline foo < "/etc/passwd"}'`, false},
		{"system call", `awk 'BEGIN{system("rm -rf /")}'`, true},
		{"system call with space before paren", `awk 'BEGIN{system ("rm -rf /tmp/x")}'`, true},
		{"system call with tab before paren", "awk 'BEGIN{system\t(\"rm\")}'", true},
		{"system call with newline whitespace", "awk 'BEGIN{system\n(\"rm\")}'", true},
		{"mysystem is not system", `awk 'BEGIN{x = mysystem("foo")}'`, false},
		{"pipe getline", `awk '{cmd | getline x}'`, true},
		{"pipe getline with tab", "awk '{cmd |\tgetline x}'", true},
		{"print to file", `awk '{print > "/etc/passwd"}'`, true},
		{"printf to file", `awk '{printf "%s", $0 > "out"}'`, true},
		{"print to pipe", `awk '{print | "rm"}'`, true},
		{"printf to pipe", `awk '{printf "%s" | "tee /etc/passwd"}'`, true},
		{"print to variable file", `awk 'BEGIN{f="/etc/passwd"; print "x" > f}'`, true},
		{"printf to variable file", `awk 'BEGIN{f="out"; printf "%s", $0 > f}'`, true},
		{"print to variable pipe", `awk 'BEGIN{c="rm -rf /tmp/x"; print "x" | c}'`, true},
		{"print to expression file", `awk 'BEGIN{print "x" > ("out_" NR)}'`, true},

		// Redirects that look like operators but aren't
		{"print with > inside string", `awk 'BEGIN{print "a > b"}'`, false},
		{"print with | inside string", `awk 'BEGIN{print "a | b"}'`, false},
		{"print of comparison in parens", `awk 'BEGIN{print (1 > 0)}'`, false},
		{"if guard with > before print", `awk 'BEGIN{if (a > b) print "x"}'`, false},
		{"division before print redirect", `awk 'BEGIN{x=1/2; print "x" > "/tmp/out"}'`, true},
		{"print action before regex with gt chars", `awk 'p{print NR": "$0} /^>>>>>>>/{p=0}' file`, false},
		{"sprint identifier before > is comparison", `awk 'BEGIN{sprint = 1 > 0}'`, false},
		{"printer identifier before > is comparison", `awk 'BEGIN{printer = 5 > 3}'`, false},
		{"myprintf identifier before > is comparison", `awk 'BEGIN{myprintf = a > b}'`, false},

		// gawk -e/--source program text gets the same scans as the
		// positional program. A system() call inside --source must not
		// slip past the validator just because there is no positional
		// program to inspect.
		{"--source with system rm", `awk --source='BEGIN{system("rm -rf /")}'`, true},
		{"--source with system git diff allows", `awk --source='BEGIN{system("git diff")}'`, false},
		{"-e short with system rm", `awk -e 'BEGIN{system("rm -rf /")}'`, true},
		{"-e short with print redirect", `awk -e 'BEGIN{print "x" > "/etc/passwd"}'`, true},
		{"--source plus positional both safe", `awk --source='BEGIN{print "hi"}' '{print $1}' file`, false},
		{"--source safe but positional has system rm", `awk --source='BEGIN{print "hi"}' 'BEGIN{system("rm -rf /")}'`, true},

		// gawk allows multiple -e / --source. Earlier dangerous source
		// values must not be hidden by a later safe one.
		{"two -e values dangerous first", `awk -e 'BEGIN{system("rm -rf /")}' -e 'BEGIN{print "ok"}'`, true},
		{"two --source values dangerous first", `awk --source 'BEGIN{system("rm -rf /")}' --source 'BEGIN{print "ok"}'`, true},
		{"two -e values both safe", `awk -e 'BEGIN{print "a"}' -e '{print $1}'`, false},
		{"--source= attached form dangerous", `awk --source='BEGIN{system("rm -rf /")}'`, true},

		// Dynamic source values must ask: parameter expansion or
		// command substitution can hide arbitrary awk programs.
		{"--source= with parameter expansion asks", `awk --source="$prog"`, true},
		{"-e= with parameter expansion asks", `awk -e="$prog"`, true},
		{"-e space with parameter expansion asks", `awk -e "$prog"`, true},
		{"--source space with cmdsubst asks", "awk --source \"$(printf 'BEGIN{system(\\\"rm -rf /\\\")}')\"", true},

		// Split-literal forms: bash concatenates Word parts so
		// --source"=$prog" and -e"=$prog" become --source=<value> /
		// -e=<value>. Detect via wordLeadingLiteral and ask.
		{"--source split-literal with parameter expansion asks", `awk --source"=$prog"`, true},
		{"-e split-literal with parameter expansion asks", `awk -e"=$prog"`, true},

		// Attached short-option form: -ePROG with no space and no =.
		{"-e attached dangerous", `awk -e'BEGIN{system("rm -rf /")}'`, true},
		{"-e attached safe", `awk -e'BEGIN{print "hi"}'`, false},
		{"-e attached with parameter expansion asks", `awk -e"$prog"`, true},
		{"-e attached print redirect", `awk -e'BEGIN{print "x" > "/etc/passwd"}'`, true},

		// Single quotes appear inside awk strings and regex literals;
		// they are not string delimiters in awk. A scanner that
		// flipped state on `'` would skip the later `>` redirect.
		{"apostrophe in regex before redirect", `awk "BEGIN{if (\"a'\" ~ /a'/) print \"x\" > \"/tmp/out\"}"`, true},
		{"apostrophe inside string before redirect", `awk "BEGIN{print \"don't\" > \"/tmp/out\"}"`, true},

		// gawk two-way coprocess: `|& getline` reads from a command
		// the same as `| getline`. The regex must catch the optional
		// `&` after `|`.
		{"two-way coprocess getline literal", `awk 'BEGIN{"rm -rf /" |& getline}'`, true},
		{"two-way coprocess getline with whitespace", "awk 'BEGIN{cmd |&\tgetline x}'", true},
		{"two-way coprocess in -e source", `awk -e 'BEGIN{"id" |& getline}'`, true},
		{"two-way coprocess in --source", `awk --source='BEGIN{"id" |& getline}'`, true},

		// gawk @include / @load directives load uninspected awk code
		// from inside the program text, equivalent to -i / -l on the
		// command line. Both must ask.
		{"@include directive", `awk '@include "/tmp/evil.awk"'`, true},
		{"@load directive", `awk '@load "/tmp/evil.so"'`, true},
		{"@include in -e source", `awk -e '@include "lib.awk"'`, true},

		// Backslash-escaped program syntax: bash hands awk argv
		// `BEGIN{system("sh")}` while the parsed Lit keeps every
		// backslash. wordArgvLiteral resolves the argv form so the
		// system() scan still triggers.
		{"escaped program with system", `awk BEGIN\{system\(\"sh\"\)\}`, true},
		{"escaped --source value with system", `awk \--source=BEGIN\{system\(\"sh\"\)\}`, true},
		{"-f script not inspectable", "awk -f /tmp/script.awk", true},
		{"-i include not inspectable", "awk -i lib.awk '{print}'", true},
		{"--include long form", "awk --include lib.awk '{print}'", true},
		{"-l load extension", "awk -l ext.so '{print}'", true},
		{"--load long form", "awk --load ext.so '{print}'", true},
		{"-E exec", "awk -E /tmp/script.awk", true},
		{"--exec long form", "awk --exec /tmp/script.awk", true},

		// Recursive eval of system() argument: literal allow-listed inner
		// command passes through; deny/ask/unrecognized inner commands fall
		// back to ask.
		{"system git diff allowed", `awk 'BEGIN{system("git diff")}'`, false},
		{"system ls allowed", `awk 'BEGIN{system("ls -la")}'`, false},
		{"system rm denied", `awk 'BEGIN{system("rm -rf /")}'`, true},
		{"system unrecognized", `awk 'BEGIN{system("totallyfakecmd")}'`, true},
		{"system non-literal arg", `awk 'BEGIN{system(cmd)}'`, true},
		{"system concatenation", `awk 'BEGIN{system("git " op)}'`, true},
		{"mixed allow then deny", `awk 'BEGIN{system("git diff"); system("rm -rf /")}'`, true},
		{"both allow", `awk 'BEGIN{system("git diff"); system("ls")}'`, false},
		{"escape sequences", `awk 'BEGIN{system("echo a\nb")}'`, true},
	}

	ctx := evalContext{
		wrapperPats: wrapperPatterns(),
		commandPats: commandPatterns(),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := parseCallArgs(t, tt.cmd)
			ok := isAwkSafe(args, ctx)
			if tt.wantOverride {
				assert.False(t, ok, "expected validator to reject %q", tt.cmd)
			} else {
				assert.True(t, ok, "expected validator to accept %q", tt.cmd)
			}
		})
	}
}

func TestEvaluate_AwkFlows(t *testing.T) {
	repo := initGitRepo(t)

	t.Run("simple awk allowed", func(t *testing.T) {
		r := evaluateAll(`awk '{print $1}' file`)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("awk system drops to ask", func(t *testing.T) {
		r := evaluateAll(`awk 'BEGIN{system("rm -rf /")}'`)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("awk system with allowed inner command stays allowed", func(t *testing.T) {
		r := evaluateAll(`awk 'BEGIN{system("git diff")}'`)
		require.NotNil(t, r)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("conflict marker region printer stays allowed", func(t *testing.T) {
		r := evaluateAll(`awk '/^<<<<<<< /{p=1} p{print NR": "$0} /^>>>>>>>/{p=0; print "----"}' internal/sync/engine.go | head -60`)
		require.NotNil(t, r)
		assert.Equal(t, "awk | read-only", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("dynamic input file after static program stays allowed", func(t *testing.T) {
		r := evaluateAll(`f=/tmp/input.txt; awk '/func \(e \*ErrorResponse\) Error\(\) string/,/^}/' "$f" 2>/dev/null`)
		require.NotNil(t, r)
		assert.Equal(t, "var assignment | awk", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("literal loop variable in awk program stays allowed", func(t *testing.T) {
		r := evaluateAll(`for a in Claude Codex Cowork Hermes; do echo "=== $a ==="; awk "/Type:        Agent$a,/||/Type:           Agent$a,/{f=1} f{print} /^\t\},/{if(f)exit}" internal/parser/types.go | grep -E "DiscoverFunc|Type:"; done`)
		require.NotNil(t, r)
		assert.Equal(t, "for{echo | awk | read-only}", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("dynamic option value still asks", func(t *testing.T) {
		r := evaluateAll(`awk -v x="$dynamic" '{print x}' file`)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("static dangerous program still asks after decoding", func(t *testing.T) {
		r := evaluateAll(`prog='BEGIN{system("rm -rf /")}'; awk "$prog" file`)
		require.NotNil(t, r)
		assert.Equal(t, "var assignment | awk", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})

	t.Run("file redirect to repo path stays allowed", func(t *testing.T) {
		r := evaluateAllInDir(`awk 'BEGIN{print "x" > "awk-output.txt"}'`, repo)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("append redirect to repo path stays allowed", func(t *testing.T) {
		r := evaluateAllInDir(`awk 'BEGIN{print "x" >> "awk-output.txt"}'`, repo)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("file redirect to linked worktree path stays allowed", func(t *testing.T) {
		linkedWorktree := filepath.Join(t.TempDir(), "feature-worktree")
		runGit(t, repo, "worktree", "add", "-b", "awk-redirect-test", linkedWorktree, "HEAD")
		target := filepath.Join(linkedWorktree, "awk-output.txt")

		r := evaluateAllInDir(`awk 'BEGIN{print "x" > "`+target+`"}'`, repo)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAllow, r.decision)
	})

	t.Run("file redirect outside repo still asks", func(t *testing.T) {
		r := evaluateAllInDir(`awk 'BEGIN{print "x" > "/etc/passwd"}'`, repo)
		require.NotNil(t, r)
		assert.Equal(t, "awk", r.reason)
		assert.Equal(t, decisionAsk, r.decision)
	})
}

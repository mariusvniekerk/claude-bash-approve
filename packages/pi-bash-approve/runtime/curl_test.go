package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

// parseTestArgs creates []*syntax.Word from args (without command name).
// Used to test parseArgs directly, which receives args after the command name.
func parseTestArgs(parts ...string) []*syntax.Word {
	cmd := strings.Join(parts, " ")
	f, _ := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(cmd), "")
	if len(f.Stmts) == 0 {
		return nil
	}
	call, ok := f.Stmts[0].Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) < 2 {
		return nil
	}
	return call.Args[1:] // skip command name for parseArgs tests
}

func TestCurlReadOnly(t *testing.T) {
	tests := []struct {
		name   string
		cmd    string
		reason string
	}{
		{"curl simple GET", "curl https://example.com", "curl"},
		{"curl silent", "curl -s https://example.com", "curl"},
		{"curl with flags", "curl -sL https://example.com", "curl"},
		{"curl with output", "curl -o file.txt https://example.com", "curl"},
		{"curl with timeout", "curl --max-time 30 https://example.com", "curl"},
		{"curl with header", "curl -H 'Accept: application/json' https://example.com", "curl"},
		{"curl with multiple flags", "curl -sSL --fail --max-time 10 https://example.com", "curl"},
		{"curl with user-agent", "curl -A 'my-agent' https://example.com", "curl"},
		{"curl verbose", "curl -v https://example.com", "curl"},
		{"curl head request", "curl -I https://example.com", "curl"},
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

func TestCurlWriteAsks(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
	}{
		{"curl POST", "curl -X POST https://example.com"},
		{"curl PUT", "curl -X PUT https://example.com"},
		{"curl DELETE", "curl -X DELETE https://example.com"},
		{"curl PATCH", "curl --request PATCH https://example.com"},
		{"curl with data", "curl -d 'payload' https://example.com"},
		{"curl with data long", "curl --data 'payload' https://example.com"},
		{"curl with data-raw", "curl --data-raw 'payload' https://example.com"},
		{"curl with data-binary", "curl --data-binary @file https://example.com"},
		{"curl with data-urlencode", "curl --data-urlencode 'key=val' https://example.com"},
		{"curl with form", "curl -F 'file=@upload.txt' https://example.com"},
		{"curl with form long", "curl --form 'file=@upload.txt' https://example.com"},
		{"curl upload", "curl -T file.txt https://example.com"},
		{"curl upload long", "curl --upload-file file.txt https://example.com"},
		{"curl json", "curl --json '{\"key\":\"val\"}' https://example.com"},
		{"curl combined flags with data", "curl -sLd 'payload' https://example.com"},
		{"curl -XPOST no space", "curl -XPOST https://example.com"},
		{"curl data equals", "curl --data=payload https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := evaluateAll(tt.cmd)
			require.NotNilf(t, r, "expected ask for %q, got rejected", tt.cmd)
			assert.Equal(t, "ask", r.decision, "curl write command should get ask decision")
		})
	}
}

func TestCurlUnknownFlagAsks(t *testing.T) {
	t.Run("unknown flag falls to ask", func(t *testing.T) {
		r := evaluateAll("curl --some-future-flag https://example.com")
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision, "unknown curl flag should get ask decision")
	})
}

func TestCurlWithExpansionAsks(t *testing.T) {
	t.Run("variable in flags falls to ask", func(t *testing.T) {
		r := evaluateAll("curl $CURL_OPTS https://example.com")
		require.NotNil(t, r)
		assert.Equal(t, "ask", r.decision, "non-literal curl args should get ask decision")
	})
}

func TestParseArgs(t *testing.T) {
	// Test that short flags normalize to canonical names
	t.Run("short flags normalize", func(t *testing.T) {
		args := parseTestArgs("curl", "-sL", "https://example.com")
		p := parseArgs(args, curlSpec)
		_, hasSilent := p.flags["silent"]
		_, hasLocation := p.flags["location"]
		assert.True(t, hasSilent, "expected -s to normalize to 'silent'")
		assert.True(t, hasLocation, "expected -L to normalize to 'location'")
	})

	t.Run("combined flag with value", func(t *testing.T) {
		args := parseTestArgs("curl", "-sLd", "payload", "https://example.com")
		p := parseArgs(args, curlSpec)
		_, hasData := p.flags["data"]
		assert.True(t, hasData, "expected -d to normalize to 'data'")
		assert.Len(t, p.positional, 1, "URL should be positional")
	})

	t.Run("long flag with equals", func(t *testing.T) {
		args := parseTestArgs("curl", "--data=payload", "https://example.com")
		p := parseArgs(args, curlSpec)
		_, hasData := p.flags["data"]
		assert.True(t, hasData, "expected --data=payload to parse as 'data'")
	})

	t.Run("double dash stops flags", func(t *testing.T) {
		args := parseTestArgs("curl", "--", "--not-a-flag")
		p := parseArgs(args, curlSpec)
		assert.Empty(t, p.flags, "nothing after -- should be a flag")
		assert.Len(t, p.positional, 1)
	})
}

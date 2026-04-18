package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildCodexOutputAllow(t *testing.T) {
	out := buildCodexOutput(&result{decision: decisionAllow, reason: "git read op"})
	assert.JSONEq(t, `{"continue":true,"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`, marshalCodexOutputForTest(t, out))
}

func TestBuildCodexOutputDenyUsesDenyReason(t *testing.T) {
	out := buildCodexOutput(&result{decision: decisionDeny, reason: "git destructive", denyReason: "Denied: dangerous command"})
	assert.JSONEq(t, `{"continue":true,"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","reason":"Denied: dangerous command"}}}`, marshalCodexOutputForTest(t, out))
}

func TestBuildCodexOutputNoDecisionDefers(t *testing.T) {
	assert.JSONEq(t, `{"continue":true}`, marshalCodexOutputForTest(t, buildCodexOutput(nil)))
	assert.JSONEq(t, `{"continue":true}`, marshalCodexOutputForTest(t, buildCodexOutput(&result{decision: "", reason: "git push"})))
	assert.JSONEq(t, `{"continue":true}`, marshalCodexOutputForTest(t, buildCodexOutput(&result{decision: decisionAsk, reason: "git tag"})))
}

func TestCodexModeCLIPermissionRequestAllow(t *testing.T) {
	out := runApproveBashCLI(t, []string{"--codex"}, []byte(`{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"git status"},"cwd":"/repo"}`))
	assert.JSONEq(t, `{"continue":true,"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`, out)
}

func TestCodexModeCLIPermissionRequestDeny(t *testing.T) {
	out := runApproveBashCLI(t, []string{"--codex"}, []byte(`{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -r build"},"cwd":"/repo"}`))
	assert.JSONEq(t, `{"continue":true,"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","reason":"BLOCKED: rm -r is banned. Remove specific files only, not entire directory trees."}}}`, out)
}

func TestCodexModeCLIDefersUnsupportedEvent(t *testing.T) {
	out := runApproveBashCLI(t, []string{"--codex"}, []byte(`{"hook_event_name":"SessionStart","tool_name":"Bash","tool_input":{"command":"git status"},"cwd":"/repo"}`))
	assert.JSONEq(t, `{"continue":true}`, out)
}

func marshalCodexOutputForTest(t *testing.T, out CodexOutput) string {
	t.Helper()
	data, err := json.Marshal(out)
	assert.NoError(t, err)
	return string(data)
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type CodexInput struct {
	HookEventName string         `json:"hook_event_name"`
	ToolName      string         `json:"tool_name"`
	ToolInput     CodexToolInput `json:"tool_input"`
	Cwd           string         `json:"cwd"`
}

type CodexToolInput struct {
	Command string `json:"command"`
}

type CodexOutput struct {
	Continue           bool                     `json:"continue"`
	HookSpecificOutput *CodexHookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type CodexHookSpecificOutput struct {
	HookEventName string                 `json:"hookEventName"`
	Decision      *CodexPermissionOutput `json:"decision,omitempty"`
}

type CodexPermissionOutput struct {
	Behavior string `json:"behavior"`
	Reason   string `json:"reason,omitempty"`
}

func emitCodexOutput(out CodexOutput) {
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	os.Exit(0)
}

func evaluateCodexToolUse(input CodexInput, cfg Config) (string, *result) {
	if input.HookEventName != "PermissionRequest" || input.ToolName != "Bash" {
		return "", nil
	}
	return input.ToolInput.Command, Evaluate(input.ToolInput.Command, cfg, evalContext{cwd: input.Cwd})
}

func buildCodexOutput(r *result) CodexOutput {
	out := CodexOutput{Continue: true}
	if r == nil || r.decision == "" || r.decision == decisionAsk {
		return out
	}

	reason := r.reason
	if r.decision == decisionDeny && r.denyReason != "" {
		reason = r.denyReason
	}

	out.HookSpecificOutput = &CodexHookSpecificOutput{
		HookEventName: "PermissionRequest",
		Decision: &CodexPermissionOutput{
			Behavior: r.decision,
		},
	}
	if r.decision == decisionDeny && reason != "" {
		out.HookSpecificOutput.Decision.Reason = reason
	}
	return out
}

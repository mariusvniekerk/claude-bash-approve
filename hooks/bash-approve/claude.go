package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type HookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Cwd       string          `json:"cwd"`
}

type BashInput struct {
	Command         string `json:"command"`
	Description     string `json:"description"`
	Timeout         int    `json:"timeout"`
	RunInBackground bool   `json:"run_in_background"`
}

type HookOutput struct {
	HookSpecificOutput HookDecision `json:"hookSpecificOutput"`
}

type HookDecision struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func emitDecision(decision, reason string) {
	out := HookOutput{
		HookSpecificOutput: HookDecision{
			HookEventName:            "PreToolUse",
			PermissionDecision:       decision,
			PermissionDecisionReason: reason,
		},
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	os.Exit(0)
}

func evaluateToolUse(input HookInput, cfg Config) (string, *result) {
	ctx := evalContext{cwd: input.Cwd}

	switch input.ToolName {
	case "Bash":
		var bashInput BashInput
		if err := json.Unmarshal(input.ToolInput, &bashInput); err != nil {
			return "", nil
		}
		return bashInput.Command, Evaluate(bashInput.Command, cfg, ctx)
	case "Read":
		var readInput ReadInput
		if err := json.Unmarshal(input.ToolInput, &readInput); err != nil {
			return "", nil
		}
		return readInput.FilePath, evaluateReadTool(readInput, ctx)
	case "Grep":
		var grepInput GrepInput
		if err := json.Unmarshal(input.ToolInput, &grepInput); err != nil {
			return "", nil
		}
		return grepCommandLabel(grepInput), evaluateGrepTool(grepInput, ctx)
	default:
		return "", nil
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type OpenCodeInput struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}

type OpenCodeOutput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

func emitOpenCodeOutput(out OpenCodeOutput) {
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
	os.Exit(0)
}

func evaluateOpenCodeToolUse(input OpenCodeInput, cfg Config) (string, *result) {
	if input.Tool != "bash" {
		return "", nil
	}
	return input.Command, Evaluate(input.Command, cfg, evalContext{cwd: input.Cwd})
}

func buildOpenCodeOutput(r *result) OpenCodeOutput {
	if r == nil {
		return OpenCodeOutput{Decision: "noop"}
	}
	if r.decision == "" {
		return OpenCodeOutput{Decision: "noop", Reason: r.reason}
	}
	reason := r.reason
	if r.decision == decisionDeny && r.denyReason != "" {
		reason = r.denyReason
	}
	return OpenCodeOutput{Decision: r.decision, Reason: reason}
}

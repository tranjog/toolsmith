package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

var Bash = ToolDefinition{
	Name:        "bash",
	Description: "Run bash script. Returns combined stdout+stderr.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"script": map[string]any{
				"type":        "string",
				"description": "Bash script to execute via `bash -c`.",
			},
		},
		"required": []string{"script"},
	},
	Function: bashFn,
}

type bashInput struct {
	Script string `json:"script"`
}

func bashFn(input json.RawMessage) (string, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Script == "" {
		return "", fmt.Errorf("script required")
	}
	out, err := exec.Command("bash", "-c", in.Script).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("bash failed: %w", err)
	}
	return string(out), nil
}

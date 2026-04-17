package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

var ReadFile = ToolDefinition{
	Name:        "read_file",
	Description: "Read contents of a file using cat. Path relative to working directory.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to file to read.",
			},
		},
		"required": []string{"path"},
	},
	Function: readFileFn,
}

type readFileInput struct {
	Path string `json:"path"`
}

func readFileFn(input json.RawMessage) (string, error) {
	var in readFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Path == "" {
		return "", fmt.Errorf("path required")
	}
	out, err := exec.Command("cat", in.Path).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("cat failed: %w", err)
	}
	return string(out), nil
}

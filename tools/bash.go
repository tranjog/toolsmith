package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

const (
	bashDefaultTimeout = 120
	bashMaxTimeout     = 600
	bashMaxOutputBytes = 64 * 1024
)

var Bash = ToolDefinition{
	Name:             "bash",
	Description:      "Run a bash script. Returns combined stdout+stderr. Default timeout 120s (max 600s). Output truncated at 64 KiB.",
	ShortDescription: "Run a bash script",
	Category:         "shell",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"script":     map[string]any{"type": "string", "description": "Bash script executed via `bash -c`."},
			"cwd":        map[string]any{"type": "string", "description": "Optional working directory."},
			"timeout_s":  map[string]any{"type": "integer", "description": "Timeout in seconds (default 120, max 600)."},
		},
		"required": []string{"script"},
	},
	Function: bashFn,
}

type bashInput struct {
	Script   string `json:"script"`
	Cwd      string `json:"cwd"`
	Timeout  int    `json:"timeout_s"`
}

func bashFn(input json.RawMessage) (string, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Script == "" {
		return "", fmt.Errorf("script required")
	}

	timeout := in.Timeout
	if timeout <= 0 {
		timeout = bashDefaultTimeout
	}
	if timeout > bashMaxTimeout {
		timeout = bashMaxTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", in.Script)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	out, err := cmd.CombinedOutput()
	result := capOutput(string(out), bashMaxOutputBytes)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return result, fmt.Errorf("bash timed out after %ds", timeout)
	}
	if err != nil {
		return result, fmt.Errorf("bash failed: %w", err)
	}
	return result, nil
}

func capOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n...[truncated, %d bytes dropped]", len(s)-n)
}

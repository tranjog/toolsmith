package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const maxReadBytes = 1 << 20 // 1 MiB

var ReadFile = ToolDefinition{
	Name:        "read_file",
	Description: "Read contents of a file. Path relative to working directory. Optional line offset/limit for large files.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to file."},
			"offset": map[string]any{"type": "integer", "description": "1-based starting line (optional)."},
			"limit":  map[string]any{"type": "integer", "description": "Max lines to return (optional)."},
		},
		"required": []string{"path"},
	},
	Function: readFileFn,
}

type readFileInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func readFileFn(input json.RawMessage) (string, error) {
	var in readFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Path == "" {
		return "", fmt.Errorf("path required")
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", in.Path)
	}

	if in.Offset == 0 && in.Limit == 0 {
		if info.Size() > maxReadBytes {
			return "", fmt.Errorf("file too large (%d bytes > %d); use offset/limit", info.Size(), maxReadBytes)
		}
		b, err := os.ReadFile(in.Path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	f, err := os.Open(in.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var out strings.Builder
	line := 0
	start := in.Offset
	if start < 1 {
		start = 1
	}
	for scanner.Scan() {
		line++
		if line < start {
			continue
		}
		if in.Limit > 0 && line >= start+in.Limit {
			break
		}
		out.WriteString(scanner.Text())
		out.WriteByte('\n')
	}
	return out.String(), scanner.Err()
}

package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ListFiles = ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories at given path. If no path, lists current directory. Directories end with '/'.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Optional relative path. Defaults to current directory.",
			},
		},
	},
	Function: listFilesFn,
}

type listFilesInput struct {
	Path string `json:"path"`
}

func listFilesFn(input json.RawMessage) (string, error) {
	var in listFilesInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
	}
	root := in.Path
	if root == "" {
		root = "."
	}

	var entries []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			entries = append(entries, rel+"/")
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		entries = append(entries, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk failed: %w", err)
	}

	out, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

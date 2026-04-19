package tools

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

const listFilesMaxEntries = 2000

var ListFiles = ToolDefinition{
	Name:             "list_files",
	Description:      "List files and directories at given path (recursive). Directories end with '/'. Skips common vendored dirs (.git, node_modules, vendor, dist, build, target, .next, __pycache__) and dotfiles. Capped at 2000 entries.",
	ShortDescription: "List files and directories",
	Category:         "fs",
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
	truncated := false

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] || (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return fs.SkipDir
			}
			entries = append(entries, rel+"/")
		} else {
			entries = append(entries, rel)
		}
		if len(entries) >= listFilesMaxEntries {
			truncated = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk failed: %w", err)
	}

	payload := map[string]any{"entries": entries}
	if truncated {
		payload["truncated"] = true
		payload["limit"] = listFilesMaxEntries
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

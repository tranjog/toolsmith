package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var EditFile = ToolDefinition{
	Name: "edit_file",
	Description: `Edit a text file by replacing old_str with new_str. old_str and new_str must differ.
If file does not exist and old_str is empty, the file is created with new_str as contents.
Otherwise old_str must appear exactly once in the file.`,
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file.",
			},
			"old_str": map[string]any{
				"type":        "string",
				"description": "Text to replace. Empty when creating a new file.",
			},
			"new_str": map[string]any{
				"type":        "string",
				"description": "Replacement text, or full contents for a new file.",
			},
		},
		"required": []string{"path", "new_str"},
	},
	Function: editFileFn,
}

type editFileInput struct {
	Path   string `json:"path"`
	OldStr string `json:"old_str"`
	NewStr string `json:"new_str"`
}

func editFileFn(input json.RawMessage) (string, error) {
	var in editFileInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Path == "" {
		return "", fmt.Errorf("path required")
	}
	if in.OldStr == in.NewStr {
		return "", fmt.Errorf("old_str and new_str must differ")
	}

	data, err := os.ReadFile(in.Path)
	if err != nil {
		if os.IsNotExist(err) && in.OldStr == "" {
			return createFile(in.Path, in.NewStr)
		}
		return "", err
	}

	if in.OldStr == "" {
		return "", fmt.Errorf("old_str required when editing existing file")
	}

	content := string(data)
	count := strings.Count(content, in.OldStr)
	if count == 0 {
		return "", fmt.Errorf("old_str not found")
	}
	if count > 1 {
		return "", fmt.Errorf("old_str matched %d times, must be unique", count)
	}

	updated := strings.Replace(content, in.OldStr, in.NewStr, 1)

	info, err := os.Stat(in.Path)
	if err != nil {
		return "", err
	}
	if err := atomicWrite(in.Path, []byte(updated), info.Mode().Perm()); err != nil {
		return "", err
	}
	return "OK", nil
}

func createFile(path, content string) (string, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
	}
	if err := atomicWrite(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return "created " + path, nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".edit-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

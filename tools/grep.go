package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var Grep = ToolDefinition{
	Name:             "grep",
	Description:      "Search file contents with a regex. Returns matching lines as path:line:text. Skips binary files and common vendored dirs (.git, node_modules, vendor, dist, build, target, .next, __pycache__).",
	ShortDescription: "Search file contents by regex",
	Category:         "fs",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern":     map[string]any{"type": "string", "description": "Go regex pattern (RE2 syntax)."},
			"path":        map[string]any{"type": "string", "description": "Root directory or file to search. Default '.'."},
			"glob":        map[string]any{"type": "string", "description": "Filename glob filter, e.g. '*.go' or '*.{ts,tsx}'. Matches basename only."},
			"ignore_case": map[string]any{"type": "boolean", "description": "Case-insensitive match."},
			"max_results": map[string]any{"type": "integer", "description": "Cap on matches returned. Default 200."},
		},
		"required": []string{"pattern"},
	},
	Function: grepFn,
}

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	IgnoreCase bool   `json:"ignore_case"`
	MaxResults int    `json:"max_results"`
}

var skipDirs = map[string]bool{
	".git":        true,
	"node_modules": true,
	"vendor":      true,
	"dist":        true,
	"build":       true,
	"target":      true,
	".next":       true,
	"__pycache__": true,
}

const (
	grepMaxFileBytes = 1 << 20
	grepMaxLineLen   = 2000
)

func grepFn(input json.RawMessage) (string, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.Pattern == "" {
		return "", fmt.Errorf("pattern required")
	}
	if in.Path == "" {
		in.Path = "."
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 200
	}

	pat := in.Pattern
	if in.IgnoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %w", err)
	}

	if in.Glob != "" {
		if _, err := filepath.Match(in.Glob, "test"); err != nil {
			return "", fmt.Errorf("invalid glob: %w", err)
		}
	}

	var out strings.Builder
	matches := 0
	truncated := false

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] && path != in.Path {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if in.Glob != "" {
			ok, _ := filepath.Match(in.Glob, d.Name())
			if !ok {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > grepMaxFileBytes {
			return nil
		}
		if matches >= in.MaxResults {
			truncated = true
			return fs.SkipAll
		}
		scanFile(path, re, &out, &matches, in.MaxResults, &truncated)
		return nil
	}

	info, err := os.Stat(in.Path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if err := filepath.WalkDir(in.Path, walk); err != nil {
			return "", err
		}
	} else {
		scanFile(in.Path, re, &out, &matches, in.MaxResults, &truncated)
	}

	if matches == 0 {
		return "no matches\n", nil
	}
	if truncated {
		fmt.Fprintf(&out, "... (truncated at %d matches)\n", in.MaxResults)
	}
	return out.String(), nil
}

func scanFile(path string, re *regexp.Regexp, out *strings.Builder, matches *int, max int, truncated *bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		return
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if !re.MatchString(text) {
			continue
		}
		if len(text) > grepMaxLineLen {
			text = text[:grepMaxLineLen] + "..."
		}
		fmt.Fprintf(out, "%s:%d:%s\n", path, line, text)
		*matches++
		if *matches >= max {
			*truncated = true
			return
		}
	}
}

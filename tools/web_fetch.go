package tools

import (
	"encoding/json"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"strings"
)

const (
	maxWebFetchChars = 10000
	ctSentinel       = "__WF_CT__"
)

var WebFetch = ToolDefinition{
	Name:        "web_fetch",
	Description: "Fetch a URL via curl. Returns JSON bodies verbatim; strips tags/scripts/styles from HTML. Useful for search engine result pages or JSON APIs. Output truncated at 10000 chars.",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Absolute URL (http/https).",
			},
		},
		"required": []string{"url"},
	},
	Function: webFetchFn,
}

type webFetchInput struct {
	URL string `json:"url"`
}

func webFetchFn(input json.RawMessage) (string, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", err
	}
	if in.URL == "" {
		return "", fmt.Errorf("url required")
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		return "", fmt.Errorf("url must start with http:// or https://")
	}

	cmd := exec.Command("curl",
		"-sL",
		"--max-time", "15",
		"-A", "Mozilla/5.0 (compatible; gemma-agent/1.0)",
		"-w", "\n"+ctSentinel+"%{content_type}",
		in.URL,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("curl failed: %w", err)
	}

	body, ct := splitContentType(string(out))
	ct = strings.ToLower(ct)

	switch {
	case strings.Contains(ct, "json"):
		return truncate(body, maxWebFetchChars), nil
	case strings.Contains(ct, "html"), strings.Contains(ct, "xml"):
		return truncate(stripHTML(body), maxWebFetchChars), nil
	default:
		return truncate(body, maxWebFetchChars), nil
	}
}

func splitContentType(s string) (body, ct string) {
	idx := strings.LastIndex(s, ctSentinel)
	if idx < 0 {
		return s, ""
	}
	return strings.TrimRight(s[:idx], "\n"), strings.TrimSpace(s[idx+len(ctSentinel):])
}

var (
	reScript = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	reCmt    = regexp.MustCompile(`(?s)<!--.*?-->`)
	reTag    = regexp.MustCompile(`<[^>]+>`)
	reWS     = regexp.MustCompile(`[ \t]+`)
	reNL     = regexp.MustCompile(`\n{3,}`)
)

func stripHTML(s string) string {
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reCmt.ReplaceAllString(s, "")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reWS.ReplaceAllString(s, " ")
	s = reNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n...[truncated]"
}

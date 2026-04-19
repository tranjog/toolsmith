package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	maxWebFetchChars = 10000
	webFetchTimeout  = 15 * time.Second
	webFetchMaxBytes = 2 * 1024 * 1024
	webFetchUA       = "toolsmith/1.0"
)

var WebFetch = ToolDefinition{
	Name:             "web_fetch",
	Description:      "Fetch a URL. Returns JSON bodies verbatim; strips tags/scripts/styles from HTML. Useful for search engine result pages or JSON APIs. Output truncated at 10000 chars.",
	ShortDescription: "Fetch a URL and return text",
	Category:         "web",
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

var webFetchClient = &http.Client{Timeout: webFetchTimeout}

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

	ctx, cancel := context.WithTimeout(context.Background(), webFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", webFetchUA)

	resp, err := webFetchClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes))
	if err != nil {
		return "", err
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	text := string(body)

	if resp.StatusCode >= 400 {
		return truncate(text, maxWebFetchChars), fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	switch {
	case strings.Contains(ct, "json"):
		return truncate(text, maxWebFetchChars), nil
	case strings.Contains(ct, "html"), strings.Contains(ct, "xml"):
		return truncate(stripHTML(text), maxWebFetchChars), nil
	default:
		return truncate(text, maxWebFetchChars), nil
	}
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

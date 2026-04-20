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
	Description:      "Fetch a URL. Returns JSON bodies verbatim; strips tags/scripts/styles from HTML. Useful for search engine result pages or JSON APIs. Output truncated at 10000 chars. For JSON responses, pass an optional dot/bracket path (e.g. `current.temperature_2m`, `list[0].name`) to extract a single value before truncation — much smaller payload than the full document.",
	ShortDescription: "Fetch a URL and return text",
	Category:         "web",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Absolute URL (http/https).",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional. For JSON responses, extract a single value by dot/bracket path (e.g. `current.temperature_2m`, `results[0].name`). Omit to return the whole body.",
			},
		},
		"required": []string{"url"},
	},
	Function: webFetchFn,
}

type webFetchInput struct {
	URL  string `json:"url"`
	Path string `json:"path,omitempty"`
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
		if in.Path != "" {
			extracted, err := extractJSONPath(body, in.Path)
			if err != nil {
				return "", err
			}
			return truncate(extracted, maxWebFetchChars), nil
		}
		return truncate(text, maxWebFetchChars), nil
	case strings.Contains(ct, "html"), strings.Contains(ct, "xml"):
		return truncate(stripHTML(text), maxWebFetchChars), nil
	default:
		return truncate(text, maxWebFetchChars), nil
	}
}

func extractJSONPath(body []byte, path string) (string, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	tokens, err := tokenizeJSONPath(path)
	if err != nil {
		return "", err
	}
	cur := root
	for i, tok := range tokens {
		switch t := tok.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return "", fmt.Errorf("path %q: segment %d (%q) expects object, got %T", path, i, t, cur)
			}
			v, ok := m[t]
			if !ok {
				return "", fmt.Errorf("path %q: key %q not found at segment %d", path, t, i)
			}
			cur = v
		case int:
			a, ok := cur.([]any)
			if !ok {
				return "", fmt.Errorf("path %q: segment %d ([%d]) expects array, got %T", path, i, t, cur)
			}
			if t < 0 || t >= len(a) {
				return "", fmt.Errorf("path %q: index %d out of range (len %d) at segment %d", path, t, len(a), i)
			}
			cur = a[t]
		}
	}
	if s, ok := cur.(string); ok {
		return s, nil
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func tokenizeJSONPath(p string) ([]any, error) {
	var tokens []any
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '.':
			flush()
		case '[':
			flush()
			end := strings.IndexByte(p[i+1:], ']')
			if end < 0 {
				return nil, fmt.Errorf("path %q: unclosed [", p)
			}
			idxStr := p[i+1 : i+1+end]
			var idx int
			if _, err := fmt.Sscanf(idxStr, "%d", &idx); err != nil {
				return nil, fmt.Errorf("path %q: bad index %q", p, idxStr)
			}
			tokens = append(tokens, idx)
			i += end + 1
		default:
			buf.WriteByte(c)
		}
	}
	flush()
	if len(tokens) == 0 {
		return nil, fmt.Errorf("path %q: empty", p)
	}
	return tokens, nil
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

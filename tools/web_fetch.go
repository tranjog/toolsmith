package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/itchyny/gojq"
)

const (
	maxWebFetchChars = 10000
	webFetchTimeout  = 15 * time.Second
	webFetchMaxBytes = 2 * 1024 * 1024
	webFetchUA       = "toolsmith/1.0"
)

var WebFetch = ToolDefinition{
	Name:             "web_fetch",
	Description:      "Fetch a URL. Returns JSON verbatim; strips HTML tags/scripts/styles. 10000-char output cap. For JSON, an optional jq `filter` extracts and reshapes data before truncation (e.g. `.current.temperature_2m`, `.daily | {time, tmax: .temperature_2m_max}`).",
	ShortDescription: "Fetch a URL; optional jq `filter` extractor for JSON",
	Category:         "web",
	InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Absolute URL (http/https).",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "Optional jq program applied to a JSON response body, e.g. `.current.temperature_2m` or `.daily | {time, tmax: .temperature_2m_max}`.",
			},
		},
		"required": []string{"url"},
	},
	Function: webFetchFn,
}

type webFetchInput struct {
	URL    string `json:"url"`
	Filter string `json:"filter,omitempty"`
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

	text := string(body)
	if resp.StatusCode >= 400 {
		return truncate(text, maxWebFetchChars), fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(ct, "json"):
		if in.Filter != "" {
			extracted, err := extractJSONFilter(body, in.Filter)
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

func extractJSONFilter(body []byte, filter string) (string, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	q, err := gojq.Parse(filter)
	if err != nil {
		return "", fmt.Errorf("jq parse: %w", err)
	}
	iter := q.Run(root)
	var results []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, ok := v.(error); ok {
			return "", fmt.Errorf("jq run: %w", e)
		}
		results = append(results, v)
	}
	var payload any
	switch len(results) {
	case 0:
		payload = nil
	case 1:
		payload = results[0]
	default:
		payload = results
	}
	if isEmpty(payload) {
		if rm, ok := root.(map[string]any); ok {
			return "", fmt.Errorf("filter %q produced no data; response top-level keys: %s", filter, strings.Join(sortedKeys(rm), ", "))
		}
		return "", fmt.Errorf("filter %q produced no data", filter)
	}
	if s, ok := payload.(string); ok {
		return s, nil
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case map[string]any:
		for _, x := range t {
			if x != nil {
				return false
			}
		}
		return true
	case []any:
		for _, x := range t {
			if x != nil {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

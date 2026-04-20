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
	Description:      "Fetch a URL. Returns JSON bodies verbatim; strips tags/scripts/styles from HTML. Output truncated at 10000 chars. For JSON, prefer an optional extractor to shrink the payload before truncation: `path` (single dot/bracket path, e.g. `current.temperature_2m`), `paths` (array of such paths, returns `{path: value}` map), or `filter` (a jq program, e.g. `.daily | {time, tmax: .temperature_2m_max}`). Use `filter` when you need to reshape parallel arrays into paired records.",
	ShortDescription: "Fetch a URL (JSON) with optional jq `filter`, `paths` array, or single `path` extractor",
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
				"description": "Optional. Single dot/bracket path to extract one value from a JSON response (e.g. `current.temperature_2m`, `results[0].name`).",
			},
			"paths": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional. List of dot/bracket paths. Returns a JSON object keyed by path string, each mapped to the extracted value.",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "Optional. A jq program run over the JSON body. Most powerful option — can reshape parallel arrays, e.g. `.daily | {time, tmax: .temperature_2m_max, tmin: .temperature_2m_min}`.",
			},
		},
		"required": []string{"url"},
	},
	Function: webFetchFn,
}

type webFetchInput struct {
	URL    string   `json:"url"`
	Path   string   `json:"path,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Filter string   `json:"filter,omitempty"`
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
		if in.Filter != "" {
			extracted, err := extractJSONFilter(body, in.Filter)
			if err != nil {
				return "", err
			}
			return truncate(extracted, maxWebFetchChars), nil
		}
		if len(in.Paths) > 0 {
			extracted, err := extractJSONPaths(body, in.Paths)
			if err != nil {
				return "", err
			}
			return truncate(extracted, maxWebFetchChars), nil
		}
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
	v, err := walkJSONPath(root, path)
	if err != nil {
		return "", err
	}
	if s, ok := v.(string); ok {
		return s, nil
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func extractJSONPaths(body []byte, paths []string) (string, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return "", fmt.Errorf("parse json: %w", err)
	}
	result := make(map[string]any, len(paths))
	for _, p := range paths {
		v, err := walkJSONPath(root, p)
		if err != nil {
			return "", err
		}
		result[p] = v
	}
	out, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
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
	if isEmptyExtract(payload) {
		if rm, ok := root.(map[string]any); ok {
			return "", fmt.Errorf("jq filter %q produced no data; response top-level keys: %s", filter, strings.Join(sortedKeys(rm), ", "))
		}
		return "", fmt.Errorf("jq filter %q produced no data", filter)
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

func isEmptyExtract(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case map[string]any:
		if len(t) == 0 {
			return true
		}
		for _, x := range t {
			if x != nil {
				return false
			}
		}
		return true
	case []any:
		if len(t) == 0 {
			return true
		}
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

func walkJSONPath(root any, path string) (any, error) {
	tokens, err := tokenizeJSONPath(path)
	if err != nil {
		return nil, err
	}
	cur := root
	for i, tok := range tokens {
		switch t := tok.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("path %q: segment %d (%q) expects object, got %T", path, i, t, cur)
			}
			v, ok := m[t]
			if !ok {
				return nil, fmt.Errorf("path %q: key %q not found at segment %d; available keys: %s", path, t, i, strings.Join(sortedKeys(m), ", "))
			}
			cur = v
		case int:
			a, ok := cur.([]any)
			if !ok {
				return nil, fmt.Errorf("path %q: segment %d ([%d]) expects array, got %T", path, i, t, cur)
			}
			if t < 0 || t >= len(a) {
				return nil, fmt.Errorf("path %q: index %d out of range (len %d) at segment %d", path, t, len(a), i)
			}
			cur = a[t]
		}
	}
	return cur, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

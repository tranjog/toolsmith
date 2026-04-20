package tools

import "encoding/json"

type ToolDefinition struct {
	Name             string                                      `json:"name"`
	Description      string                                      `json:"description"`
	ShortDescription string                                      `json:"short_description,omitempty"`
	Category         string                                      `json:"category,omitempty"`
	InputSchema      map[string]any                              `json:"input_schema"`
	Function         func(input json.RawMessage) (string, error) `json:"-"`
}

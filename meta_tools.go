package main

import (
	"encoding/json"
	"fmt"

	"agent/tools"
)

const progressiveSystemPrompt = `You have access to tools, but most are hidden to keep the context small. ` +
	`Use list_tools to see the catalog, then describe_tool(name) to load a tool's full schema. ` +
	`After a tool is loaded you can call it directly. Calling an unloaded tool returns an error — load it first.`

func (a *Agent) buildMetaTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Name:        "list_tools",
			Description: "List every tool available in this session. Returns name, short description, and category for each. Call this first to discover what is available.",
			Category:    "meta",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Function: func(_ json.RawMessage) (string, error) {
				catalog := a.registry.Catalog()
				b, err := json.Marshal(catalog)
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
		},
		{
			Name:        "describe_tool",
			Description: "Load a tool by name. Returns its full JSON schema and makes it callable for the rest of this session.",
			Category:    "meta",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{"type": "string", "description": "Name of the tool to load."},
				},
				"required": []string{"name"},
			},
			Function: func(input json.RawMessage) (string, error) {
				var in struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(input, &in); err != nil {
					return "", err
				}
				if in.Name == "" {
					return "", fmt.Errorf("name required")
				}
				def, ok := a.registry.Get(in.Name)
				if !ok {
					return "", fmt.Errorf("unknown tool %q", in.Name)
				}
				a.active[in.Name] = true
				payload := map[string]any{
					"name":        def.Name,
					"description": def.Description,
					"parameters":  def.InputSchema,
					"loaded":      true,
				}
				b, err := json.Marshal(payload)
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
		},
	}
}

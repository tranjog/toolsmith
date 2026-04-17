package llm

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolName   string
	ToolCallID string
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []Tool) (Message, error)
	Model() string
}

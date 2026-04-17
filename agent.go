package main

import (
	"context"
	"fmt"

	"agent/llm"
	"agent/tools"
)

type Agent struct {
	provider       llm.Provider
	getUserMessage func() (string, bool)
	tools          []tools.ToolDefinition
	name           string
}

func NewAgent(provider llm.Provider, getUserMessage func() (string, bool), toolSet []tools.ToolDefinition, name string) *Agent {
	return &Agent{
		provider:       provider,
		getUserMessage: getUserMessage,
		tools:          toolSet,
		name:           name,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []llm.Message{}
	wireTools := a.llmTools()

	fmt.Printf("Chat with %s [%s] (use 'ctrl-c' to quit)\n", a.name, a.provider.Model())

	readUser := true
	for {
		if readUser {
			fmt.Print("\u001b[94mYou\u001b[0m: ")
			userInput, ok := a.getUserMessage()
			if !ok {
				return nil
			}
			conversation = append(conversation, llm.Message{Role: "user", Content: userInput})
		}

		reply, err := a.provider.Chat(ctx, conversation, wireTools)
		if err != nil {
			return err
		}
		conversation = append(conversation, reply)

		if reply.Content != "" {
			fmt.Printf("\u001b[93m%s\u001b[0m: %s\n", a.name, reply.Content)
		}

		if len(reply.ToolCalls) == 0 {
			readUser = true
			continue
		}

		for _, tc := range reply.ToolCalls {
			result := a.dispatchTool(tc)
			conversation = append(conversation, llm.Message{
				Role:       "tool",
				Content:    result,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
			})
		}
		readUser = false
	}
}

func (a *Agent) dispatchTool(tc llm.ToolCall) string {
	fmt.Printf("\u001b[92mtool\u001b[0m: %s(%s)\n", tc.Name, string(tc.Arguments))
	for _, t := range a.tools {
		if t.Name != tc.Name {
			continue
		}
		out, err := t.Function(tc.Arguments)
		if err != nil {
			return fmt.Sprintf("error: %s\n%s", err.Error(), out)
		}
		return out
	}
	return fmt.Sprintf("error: tool %q not found", tc.Name)
}

func (a *Agent) llmTools() []llm.Tool {
	out := make([]llm.Tool, 0, len(a.tools))
	for _, t := range a.tools {
		out = append(out, llm.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.InputSchema,
		})
	}
	return out
}

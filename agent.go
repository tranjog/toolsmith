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
	registry       *tools.Registry
	active         map[string]bool
	name           string
	logTokens      bool
}

func NewAgent(provider llm.Provider, getUserMessage func() (string, bool), registry *tools.Registry, activeNames []string, name string, logTokens bool) *Agent {
	active := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		active[n] = true
	}
	return &Agent{
		provider:       provider,
		getUserMessage: getUserMessage,
		registry:       registry,
		active:         active,
		name:           name,
		logTokens:      logTokens,
	}
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []llm.Message{}

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

		reply, err := a.provider.Chat(ctx, conversation, a.llmTools())
		if err != nil {
			return err
		}
		conversation = append(conversation, reply)

		if a.logTokens {
			fmt.Printf("\u001b[90m[tokens] prompt=%d completion=%d\u001b[0m\n", reply.PromptTokens, reply.CompletionTokens)
		}

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
	def, ok := a.registry.Get(tc.Name)
	if !ok {
		return fmt.Sprintf("error: tool %q not found", tc.Name)
	}
	if !a.active[tc.Name] {
		return fmt.Sprintf("error: tool %q not loaded", tc.Name)
	}
	out, err := def.Function(tc.Arguments)
	if err != nil {
		return fmt.Sprintf("error: %s\n%s", err.Error(), out)
	}
	return out
}

func (a *Agent) llmTools() []llm.Tool {
	out := make([]llm.Tool, 0, len(a.active))
	for _, name := range a.registry.Names() {
		if !a.active[name] {
			continue
		}
		def, _ := a.registry.Get(name)
		out = append(out, llm.Tool{
			Name:        def.Name,
			Description: def.Description,
			Parameters:  def.InputSchema,
		})
	}
	return out
}

package main

import (
	"context"
	"encoding/json"
	"fmt"

	"agent/llm"
	"agent/tools"
)

const progressiveSystemPrompt = "Tools without parameter schemas load their schema on first call."

type Agent struct {
	provider             llm.Provider
	getUserMessage       func() (string, bool)
	registry             *tools.Registry
	active               map[string]bool
	systemPrompt         string
	name                 string
	logTokens            bool
	turnCount            int
	promptTokenTotal     int
	completionTokenTotal int
}

func NewAgent(provider llm.Provider, getUserMessage func() (string, bool), registry *tools.Registry, activeNames []string, name string, logTokens bool, progressive bool) *Agent {
	active := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		active[n] = true
	}
	a := &Agent{
		provider:       provider,
		getUserMessage: getUserMessage,
		registry:       registry,
		active:         active,
		name:           name,
		logTokens:      logTokens,
	}
	if progressive {
		a.systemPrompt = progressiveSystemPrompt
	}
	return a
}

func (a *Agent) Run(ctx context.Context) error {
	conversation := []llm.Message{}
	if a.systemPrompt != "" {
		conversation = append(conversation, llm.Message{Role: "system", Content: a.systemPrompt})
	}

	fmt.Printf("Chat with %s [%s] (use 'ctrl-c' to quit)\n", a.name, a.provider.Model())

	if a.logTokens {
		defer func() {
			fmt.Printf("\u001b[90m[totals] turns=%d prompt=%d completion=%d\u001b[0m\n",
				a.turnCount, a.promptTokenTotal, a.completionTokenTotal)
		}()
	}

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

		a.turnCount++
		a.promptTokenTotal += reply.PromptTokens
		a.completionTokenTotal += reply.CompletionTokens

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
	firstCall := !a.active[tc.Name]
	if firstCall {
		a.active[tc.Name] = true
	}
	out, err := def.Function(tc.Arguments)
	if err != nil {
		if firstCall {
			schema, _ := json.Marshal(def.InputSchema)
			return fmt.Sprintf(
				"tool %q is now loaded. First call failed: %s\nSchema:\n%s\nRetry with correct arguments.",
				tc.Name, err.Error(), string(schema))
		}
		return fmt.Sprintf("error: %s\n%s", err.Error(), out)
	}
	return out
}

var stubSchema = map[string]any{"type": "object", "properties": map[string]any{}}

func (a *Agent) llmTools() []llm.Tool {
	names := a.registry.Names()
	out := make([]llm.Tool, 0, len(names))
	for _, name := range names {
		def, _ := a.registry.Get(name)
		if a.active[name] {
			out = append(out, llm.Tool{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  def.InputSchema,
			})
			continue
		}
		desc := def.ShortDescription
		if desc == "" {
			desc = def.Description
		}
		out = append(out, llm.Tool{
			Name:        def.Name,
			Description: desc,
			Parameters:  stubSchema,
		})
	}
	return out
}

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"agent/llm"
	"agent/tools"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func main() {
	if err := loadEnvFile(".env"); err != nil {
		fmt.Fprintf(os.Stderr, "warn: .env load failed: %s\n", err)
	}

	providerFlag := flag.String("provider", envOr("TOOLSMITH_PROVIDER", "ollama"), "LLM provider: ollama | openai")
	urlFlag := flag.String("url", envOr("TOOLSMITH_URL", ""), "Chat completions endpoint URL")
	modelFlag := flag.String("model", envOr("TOOLSMITH_MODEL", ""), "Model identifier")
	apiKeyFlag := flag.String("api-key", envOr("TOOLSMITH_API_KEY", ""), "API key (openai-compatible providers)")
	nameFlag := flag.String("agent-name", envOr("TOOLSMITH_AGENT_NAME", "agent"), "Display name for the agent")
	logTokensFlag := flag.Bool("log-tokens", envBool("TOOLSMITH_LOG_TOKENS"), "Log prompt/completion token counts per turn")
	discoveryFlag := flag.String("tool-discovery", envOr("TOOLSMITH_TOOL_DISCOVERY", "static"), "Tool exposure: static | progressive")
	scriptFlag := flag.String("script", envOr("TOOLSMITH_SCRIPT", ""), "Read user turns from a file (one turn per non-blank line). Exits when the file is exhausted.")
	flag.Parse()

	if *modelFlag == "" {
		fmt.Fprintln(os.Stderr, "error: model required (-model or TOOLSMITH_MODEL)")
		os.Exit(2)
	}

	provider, err := buildProvider(*providerFlag, *urlFlag, *modelFlag, *apiKeyFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		os.Exit(2)
	}

	getUserMessage, err := userMessageSource(*scriptFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}

	registry := tools.NewRegistry(tools.ReadFile, tools.ListFiles, tools.EditFile, tools.Grep, tools.Bash, tools.WebFetch)

	var progressive bool
	var initialActive []string
	switch *discoveryFlag {
	case "static":
		initialActive = registry.Names()
	case "progressive":
		progressive = true
		initialActive = nil
	default:
		fmt.Fprintf(os.Stderr, "error: unknown -tool-discovery %q (want: static | progressive)\n", *discoveryFlag)
		os.Exit(2)
	}

	agent := NewAgent(provider, getUserMessage, registry, initialActive, *nameFlag, *logTokensFlag, progressive)

	if err := agent.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
}

func userMessageSource(scriptPath string) (func() (string, bool), error) {
	if scriptPath == "" {
		scanner := bufio.NewScanner(os.Stdin)
		return func() (string, bool) {
			if !scanner.Scan() {
				return "", false
			}
			return scanner.Text(), true
		}, nil
	}

	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("script %q: %w", scriptPath, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return func() (string, bool) {
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fmt.Printf("\u001b[94mYou\u001b[0m: %s\n", line)
			return line, true
		}
		return "", false
	}, nil
}

func buildProvider(kind, url, model, apiKey string) (llm.Provider, error) {
	switch kind {
	case "ollama":
		if url == "" {
			url = "http://localhost:11434/api/chat"
		}
		return llm.NewOllama(url, model), nil
	case "openai":
		if url == "" {
			return nil, fmt.Errorf("openai provider requires -url (chat completions endpoint)")
		}
		return llm.NewOpenAI(url, model, apiKey), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (want: ollama | openai)", kind)
	}
}

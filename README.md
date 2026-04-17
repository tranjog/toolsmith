# toolsmith

Minimal Go CLI agent that pairs a local or hosted LLM with a small toolbox for reading, editing, and running code in the current working directory. Think of it as a bare-bones Claude Code / aider clone you can point at any OpenAI-compatible or Ollama-native endpoint.

## Requirements

- Go 1.26+
- One of:
  - [Ollama](https://ollama.com) running locally (default), or
  - Any OpenAI-compatible chat-completions endpoint (OpenRouter, vLLM, llama.cpp server, Ollama's `/v1` shim, etc.)
- The model you pick **must support tool/function calling**

## Getting started

```bash
git clone <this-repo> toolsmith
cd toolsmith
cp .env.example .env
# edit .env — set TOOLSMITH_MODEL at minimum
go run .
```

Exit with `Ctrl-C`.

### Configuration

All flags have matching env vars. Flags win over env. `.env` is auto-loaded from the working directory.

| Flag           | Env var               | Default                          | Notes                                              |
|----------------|-----------------------|----------------------------------|----------------------------------------------------|
| `-provider`    | `TOOLSMITH_PROVIDER`  | `ollama`                         | `ollama` or `openai`                               |
| `-url`         | `TOOLSMITH_URL`       | `http://localhost:11434/api/chat` (ollama) | Required for `openai` provider           |
| `-model`       | `TOOLSMITH_MODEL`     | —                                | Required                                           |
| `-api-key`     | `TOOLSMITH_API_KEY`   | —                                | Required for hosted OpenAI-compatible providers    |
| `-agent-name`  | `TOOLSMITH_AGENT_NAME`| `agent`                          | Display name in the chat prompt                    |

### Examples

Local Ollama:
```bash
go run . -model qwen3:8b
```

OpenRouter (OpenAI-compatible):
```bash
go run . \
  -provider openai \
  -url https://openrouter.ai/api/v1/chat/completions \
  -model anthropic/claude-sonnet-4.5 \
  -api-key $OPENROUTER_API_KEY
```

Ollama via its OpenAI-compatible shim:
```bash
go run . -provider openai -url http://localhost:11434/v1/chat/completions -model qwen3:8b
```

## Tools

Registered in `main.go` and handed to the model on every turn. All paths are relative to the CWD where `toolsmith` was launched.

| Tool         | Purpose                                                                                       |
|--------------|-----------------------------------------------------------------------------------------------|
| `read_file`  | Read a file. Optional `offset`/`limit` for large files. 1 MiB cap on full reads.              |
| `list_files` | Recursive listing from a path. Skips `.git`, `node_modules`, `vendor`, `dist`, `build`, etc. Capped at 2000 entries. |
| `edit_file`  | Replace a unique `old_str` with `new_str`. Creates a new file when `old_str` is empty and the file does not exist. Atomic write. |
| `grep`       | RE2 regex search across files. Supports `glob`, `ignore_case`, `max_results`. Skips binaries and vendored dirs. |
| `bash`       | Run a `bash -c` script. Optional `cwd`, `timeout_s` (default 120s, max 600s). Output capped at 64 KiB. |
| `web_fetch`  | GET a URL. Returns JSON verbatim; strips tags/scripts from HTML. 10k-char cap.                |

## Layout

```
main.go        entry point, flag/env parsing, provider wiring
agent.go       chat loop, tool dispatch
envfile.go     .env loader
llm/           provider interface + ollama & openai implementations
tools/         tool definitions (one file per tool)
```

## Safety note

`bash` and `edit_file` give the model direct write/exec access to your filesystem. Run `toolsmith` from a scoped working directory and prefer a sandbox (container, VM, dedicated user) for untrusted models or prompts.

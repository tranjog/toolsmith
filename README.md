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

| Flag               | Env var                     | Default                          | Notes                                              |
|--------------------|-----------------------------|----------------------------------|----------------------------------------------------|
| `-provider`        | `TOOLSMITH_PROVIDER`        | `ollama`                         | `ollama` or `openai`                               |
| `-url`             | `TOOLSMITH_URL`             | `http://localhost:11434/api/chat` (ollama) | Required for `openai` provider           |
| `-model`           | `TOOLSMITH_MODEL`           | —                                | Required                                           |
| `-api-key`         | `TOOLSMITH_API_KEY`         | —                                | Required for hosted OpenAI-compatible providers    |
| `-agent-name`      | `TOOLSMITH_AGENT_NAME`      | `agent`                          | Display name in the chat prompt                    |
| `-log-tokens`      | `TOOLSMITH_LOG_TOKENS`      | `false`                          | Print per-turn and end-of-session token counts     |
| `-tool-discovery`  | `TOOLSMITH_TOOL_DISCOVERY`  | `static`                         | `static` or `progressive` — see [Tool discovery](#tool-discovery) |

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

Registered in `main.go` through a `tools.Registry` and handed to the model each turn. All paths are relative to the CWD where `toolsmith` was launched.

| Tool         | Purpose                                                                                       |
|--------------|-----------------------------------------------------------------------------------------------|
| `read_file`  | Read a file. Optional `offset`/`limit` for large files. 1 MiB cap on full reads.              |
| `list_files` | Recursive listing from a path. Skips `.git`, `node_modules`, `vendor`, `dist`, `build`, etc. Capped at 2000 entries. |
| `edit_file`  | Replace a unique `old_str` with `new_str`. Creates a new file when `old_str` is empty and the file does not exist. Atomic write. |
| `grep`       | RE2 regex search across files. Supports `glob`, `ignore_case`, `max_results`. Skips binaries and vendored dirs. |
| `bash`       | Run a `bash -c` script. Optional `cwd`, `timeout_s` (default 120s, max 600s). Output capped at 64 KiB. |
| `web_fetch`  | GET a URL. Returns JSON verbatim; strips tags/scripts from HTML. 10k-char cap. Optional `path` (dot/bracket, e.g. `current.temperature_2m`, `list[0].name`) extracts one value from JSON before truncation. |

## Tool discovery

Two modes for exposing tool schemas to the model:

- **`static`** (default) — every tool's full JSON schema is wired on every turn. Simple and predictable.
- **`progressive`** — inactive tools are wired as **stubs** (name + short description, empty parameters). The first time the model calls a stubbed tool it is marked active; the real function is invoked optimistically, and if the arguments happen to be right (common for single-parameter tools) the call just works. If the first call errors, the full schema is returned inline so the model can retry on the next turn.

### When is progressive worth it?

Progressive trades fewer schemas up front for occasional schema-recovery round-trips. It pays off when per-turn schema bytes are a meaningful fraction of the prompt — typically **local models without prompt caching** and/or **larger tool sets**.

Measured against Gemma 4B on Ollama with the default six tools, progressive cut prompt tokens by **16–40%** across three representative workloads. For hosted models with prompt caching (Claude, GPT-4) the schema prefix is cached after the first turn, so static's per-turn overhead is mostly free and progressive usually isn't worth the round-trip risk. Measure with `-log-tokens` before deciding:

```bash
./toolsmith -tool-discovery=static     -log-tokens < your_session.txt
./toolsmith -tool-discovery=progressive -log-tokens < your_session.txt
```

The `[totals]` line printed on exit gives you a single number per run to compare.

## Adding a tool

1. Create `tools/my_tool.go` and export a `ToolDefinition`:

   ```go
   package tools

   import "encoding/json"

   var MyTool = ToolDefinition{
       Name:             "my_tool",
       Description:      "Full description the model sees when this tool is active. Spell out argument semantics here.",
       ShortDescription: "One-line catalog summary",
       Category:         "fs", // fs | shell | web | meta | ...
       InputSchema: map[string]any{
           "type": "object",
           "properties": map[string]any{
               "arg": map[string]any{"type": "string", "description": "What this is for."},
           },
           "required": []string{"arg"},
       },
       Function: func(raw json.RawMessage) (string, error) {
           var in struct{ Arg string `json:"arg"` }
           if err := json.Unmarshal(raw, &in); err != nil {
               return "", err
           }
           // do the thing
           return "result", nil
       },
   }
   ```

2. Register it in `main.go` inside the `tools.NewRegistry(...)` call. That is the only wiring step — the registry exposes the tool automatically to both discovery modes.

`ShortDescription` and `Category` are only consumed by progressive mode; omit them if you do not care about that path.

## Layout

```
main.go            entry point, flag/env parsing, provider wiring
agent.go           chat loop, tool dispatch, active-tool tracking
envfile.go         .env loader
llm/               provider interface + ollama & openai implementations
tools/             tool definitions (one file per tool)
tools/registry.go  Registry container and Catalog helpers
```

## Safety note

`bash` and `edit_file` give the model direct write/exec access to your filesystem. Run `toolsmith` from a scoped working directory and prefer a sandbox (container, VM, dedicated user) for untrusted models or prompts.

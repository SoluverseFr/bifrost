# Tool Injector Plugin

The Tool Injector plugin injects static tools into outgoing Bifrost **chat completion** requests before they are forwarded upstream.

It is intended for cases where tools should be enforced centrally at the gateway layer instead of being supplied by every client.

## What It Does

- Injects one or more tools in `PreLLMHook`
- Supports standard function tools
- Supports raw provider-native tool types such as `openrouter:web_search`
- Optionally filters by provider
- Optionally short-circuits requests with `echo_tools` for debugging

## Build

```bash
cd plugins/toolinjector
make build
```

Output:

```text
build/tool_injector.so
```

## Configuration

### Minimal raw tool injection

```json
{
  "name": "tool-injector",
  "path": "/opt/tool_injector.so",
  "enabled": true,
  "config": {
    "type": "openrouter:web_search",
    "providers": ["openrouter"]
  }
}
```

### Function tool injection

```json
{
  "name": "tool-injector",
  "path": "./plugins/toolinjector/build/tool_injector.so",
  "enabled": true,
  "config": {
    "tools": [
      {
        "name": "get_weather",
        "description": "Return the current weather for a city.",
        "parameters": {
          "type": "object",
          "properties": {
            "city": {
              "type": "string"
            }
          },
          "required": ["city"]
        }
      }
    ]
  }
}
```

## Accepted Config Shapes

- Single tool shorthand:
  - `type` or `name` + `description` + `parameters`
- Multi-tool form:
  - `tools: [...]`
- Alias:
  - `custom: [...]` is accepted as an alias for `tools`

Optional fields:

- `providers`: only inject on matching Bifrost providers
- `echo_tools`: return a synthetic response listing tools after injection

## Notes

- The plugin currently only modifies `req.ChatRequest`, so it affects chat completions and not the Responses API path.
- Tool deduplication happens by function name or raw tool type before appending injected tools.
- For local development, see the end-to-end runner at `tests/e2e/api/runners/individual/run-tool-injector-test.sh`.
- For user-facing docs, see `docs/features/plugins/tool-injector.mdx`.

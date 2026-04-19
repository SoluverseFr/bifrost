# OpenRouter Provider Routing Plugin

This plugin injects OpenRouter `provider` routing preferences into outbound Bifrost requests.

It is designed for cases where you want to centrally force or constrain which OpenRouter upstream providers are used for a given model or family of models.

## What It Supports

- Plugin-level `providers` filter, matching the Bifrost provider on the request
- Rule-based model matching using exact strings or wildcard patterns
- Injection of OpenRouter `provider` preferences into:
  - chat completions
  - responses
  - text completions
  - embeddings
- Merge semantics with existing `ExtraParams.provider`, with plugin values taking precedence

## Build

```bash
cd plugins/openrouterrouting
make build
```

Output:

```text
build/openrouter_provider_routing.so
```

## Example

```json
{
  "name": "openrouter-provider-routing",
  "path": "./plugins/openrouterrouting/build/openrouter_provider_routing.so",
  "enabled": true,
  "config": {
    "providers": ["openrouter"],
    "rules": [
      {
        "models": ["openai/gpt-5*", "openai/gpt-4o*"],
        "provider": {
          "only": ["azure"],
          "allow_fallbacks": false,
          "require_parameters": true
        }
      },
      {
        "models": ["meta-llama/*"],
        "provider": {
          "order": ["together", "deepinfra/turbo"],
          "allow_fallbacks": false
        }
      }
    ]
  }
}
```

## Config Shape

Top-level fields:

- `providers`: Bifrost provider filter, for example `["openrouter"]`
- `rules`: ordered list of matching rules

Each rule supports:

- `models`: optional list of exact or wildcard patterns
- `provider`: OpenRouter provider preferences to inject

Supported OpenRouter `provider` fields in this v1:

- `order`
- `only`
- `ignore`
- `allow_fallbacks`
- `require_parameters`
- `data_collection`
- `zdr`
- `enforce_distillable_text`
- `quantizations`
- `sort`
- `preferred_min_throughput`
- `preferred_max_latency`
- `max_price`

## Matching Rules

- Rules are evaluated in declaration order
- The first matching rule wins
- If a rule omits `models`, it acts as a catch-all rule for the selected Bifrost providers

Pattern examples:

- `openai/gpt-5*`
- `meta-llama/*`
- `openai/text-embedding-3-small`

## Notes

- The plugin skips requests that use `RawRequestBody`, because Bifrost forwards those bodies as-is.
- The plugin enables `BifrostContextKeyPassthroughExtraParams` so the injected `provider` object is merged into the final provider request body.
- This plugin controls OpenRouter routing preferences, not Bifrost provider selection itself.
- For custom OpenRouter providers exposed through an OpenAI-compatible transport, requests can still enter `PreLLMHook` as `openai` before the final provider key is resolved. Provider names that contain `openrouter` (for example `openrouter_trustloop`) are treated as compatible with these OpenAI-compatible requests.

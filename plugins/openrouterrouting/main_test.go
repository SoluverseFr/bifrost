package main

import (
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeModelPatternsRejectsInvalidGlob(t *testing.T) {
	_, err := normalizeModelPatterns([]string{"openai/[gpt"})
	require.Error(t, err)
}

func TestRuleMatchesSupportsWildcards(t *testing.T) {
	assert.True(t, ruleMatches([]string{"openai/gpt-5*"}, "openai/gpt-5-mini"))
	assert.True(t, ruleMatches([]string{"meta-llama/*"}, "meta-llama/llama-3.3-70b-instruct"))
	assert.False(t, ruleMatches([]string{"openai/gpt-5*"}, "anthropic/claude-sonnet-4"))
}

func TestPreLLMHookFiltersByProviderAndModel(t *testing.T) {
	plugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
		Providers: []string{"openrouter"},
		Rules: []RoutingRule{
			{
				Models: []string{"openai/gpt-5*"},
				Provider: &ProviderPreferences{
					Only: []string{"azure"},
				},
			},
		},
	})
	require.NoError(t, err)

	t.Run("matching request", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Provider: schemas.OpenRouter,
				Model:    "openai/gpt-5-mini",
			},
		}

		out, short, err := plugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.Nil(t, short)
		require.NotNil(t, out.ChatRequest.Params)
		require.NotNil(t, out.ChatRequest.Params.ExtraParams)

		providerMap, ok := out.ChatRequest.Params.ExtraParams["provider"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, []string{"azure"}, providerMap["only"])
		assert.Equal(t, true, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
	})

	t.Run("non matching bifrost provider", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Provider: schemas.OpenAI,
				Model:    "openai/gpt-5-mini",
			},
		}

		out, short, err := plugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.Nil(t, short)
		assert.Nil(t, out.ChatRequest.Params)
		assert.Nil(t, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
	})

	t.Run("matches custom openrouter provider on openai-compatible request", func(t *testing.T) {
		customPlugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
			Providers: []string{"openrouter_trustloop"},
			Rules: []RoutingRule{
				{
					Models: []string{"google/gemma-4-31b-it"},
					Provider: &ProviderPreferences{
						Only: []string{"parasail"},
					},
				},
			},
		})
		require.NoError(t, err)

		ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
		req := &schemas.BifrostRequest{
			ResponsesRequest: &schemas.BifrostResponsesRequest{
				Provider: schemas.OpenAI,
				Model:    "google/gemma-4-31b-it",
			},
		}

		out, short, err := customPlugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.Nil(t, short)
		require.NotNil(t, out.ResponsesRequest.Params)
		require.NotNil(t, out.ResponsesRequest.Params.ExtraParams)

		providerMap, ok := out.ResponsesRequest.Params.ExtraParams["provider"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, []string{"parasail"}, providerMap["only"])
		assert.Equal(t, true, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
	})

	t.Run("does not match unrelated custom provider on openai-compatible request", func(t *testing.T) {
		customPlugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
			Providers: []string{"custom-trustloop"},
			Provider: &ProviderPreferences{
				Only: []string{"parasail"},
			},
		})
		require.NoError(t, err)

		ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
		req := &schemas.BifrostRequest{
			ResponsesRequest: &schemas.BifrostResponsesRequest{
				Provider: schemas.OpenAI,
				Model:    "google/gemma-4-31b-it",
			},
		}

		out, short, err := customPlugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.Nil(t, short)
		assert.Nil(t, out.ResponsesRequest.Params)
		assert.Nil(t, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
	})

	t.Run("non matching model", func(t *testing.T) {
		ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
		req := &schemas.BifrostRequest{
			ChatRequest: &schemas.BifrostChatRequest{
				Provider: schemas.OpenRouter,
				Model:    "anthropic/claude-sonnet-4",
			},
		}

		out, short, err := plugin.PreLLMHook(ctx, req)
		require.NoError(t, err)
		require.Nil(t, short)
		assert.Nil(t, out.ChatRequest.Params)
		assert.Nil(t, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
	})
}

func TestPreLLMHookMergesExistingProviderObject(t *testing.T) {
	allowFallbacks := false
	plugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
		Providers: []string{"openrouter"},
		Rules: []RoutingRule{
			{
				Models: []string{"meta-llama/*"},
				Provider: &ProviderPreferences{
					Order:          []string{"together", "deepinfra/turbo"},
					AllowFallbacks: &allowFallbacks,
				},
			},
		},
	})
	require.NoError(t, err)

	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenRouter,
			Model:    "meta-llama/llama-3.3-70b-instruct",
			Params: &schemas.ChatParameters{
				ExtraParams: map[string]any{
					"provider": map[string]any{
						"zdr": true,
					},
				},
			},
		},
	}

	out, short, err := plugin.PreLLMHook(ctx, req)
	require.NoError(t, err)
	require.Nil(t, short)

	providerMap, ok := out.ChatRequest.Params.ExtraParams["provider"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, providerMap["zdr"])
	assert.Equal(t, false, providerMap["allow_fallbacks"])
	assert.Equal(t, []string{"together", "deepinfra/turbo"}, providerMap["order"])
}

func TestPreLLMHookSkipsRawRequestBody(t *testing.T) {
	plugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
		Providers: []string{"openrouter"},
		Provider: &ProviderPreferences{
			Only: []string{"azure"},
		},
	})
	require.NoError(t, err)

	ctx := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider:       schemas.OpenRouter,
			Model:          "openai/gpt-5-mini",
			RawRequestBody: []byte(`{"model":"openai/gpt-5-mini"}`),
		},
	}

	out, short, err := plugin.PreLLMHook(ctx, req)
	require.NoError(t, err)
	require.Nil(t, short)
	assert.Nil(t, out.ChatRequest.Params)
	assert.Nil(t, ctx.Value(schemas.BifrostContextKeyPassthroughExtraParams))
}

func TestPreLLMHookSupportsResponsesAndEmbeddings(t *testing.T) {
	plugin, err := NewOpenRouterRoutingPlugin(OpenRouterRoutingConfig{
		Providers: []string{"openrouter"},
		Provider: &ProviderPreferences{
			Only: []string{"openai"},
		},
	})
	require.NoError(t, err)

	ctxResponses := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	respReq := &schemas.BifrostRequest{
		ResponsesRequest: &schemas.BifrostResponsesRequest{
			Provider: schemas.OpenRouter,
			Model:    "openai/gpt-5-mini",
		},
	}
	_, _, err = plugin.PreLLMHook(ctxResponses, respReq)
	require.NoError(t, err)
	require.NotNil(t, respReq.ResponsesRequest.Params)
	assert.Equal(t, true, ctxResponses.Value(schemas.BifrostContextKeyPassthroughExtraParams))

	ctxEmbedding := schemas.NewBifrostContext(nil, schemas.NoDeadline)
	embeddingReq := &schemas.BifrostRequest{
		EmbeddingRequest: &schemas.BifrostEmbeddingRequest{
			Provider: schemas.OpenRouter,
			Model:    "openai/text-embedding-3-small",
		},
	}
	_, _, err = plugin.PreLLMHook(ctxEmbedding, embeddingReq)
	require.NoError(t, err)
	require.NotNil(t, embeddingReq.EmbeddingRequest.Params)
	assert.Equal(t, true, ctxEmbedding.Value(schemas.BifrostContextKeyPassthroughExtraParams))
}

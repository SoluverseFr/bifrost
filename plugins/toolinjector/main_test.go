package main

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestInitAndPreLLMHookInjectsConfiguredTool(t *testing.T) {
	t.Cleanup(func() {
		_ = Cleanup()
	})

	config := map[string]any{
		"name":        "get_weather",
		"description": "Get current weather",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"location": map[string]any{
					"type":        "string",
					"description": "City name",
				},
			},
		},
	}

	if err := Init(config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o-mini",
			Params: &schemas.ChatParameters{
				Tools: []schemas.ChatTool{
					{
						Type: schemas.ChatToolTypeFunction,
						Function: &schemas.ChatToolFunction{
							Name: "existing_tool",
						},
					},
				},
			},
		},
	}

	modifiedReq, shortCircuit, err := PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook failed: %v", err)
	}
	if shortCircuit != nil {
		t.Fatalf("expected no short-circuit, got %#v", shortCircuit)
	}
	if modifiedReq != req {
		t.Fatal("expected request to be modified in place")
	}

	tools := req.ChatRequest.Params.Tools
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Function == nil || tools[0].Function.Name != "existing_tool" {
		t.Fatalf("expected existing tool to remain first, got %#v", tools[0].Function)
	}

	injected := tools[1]
	if injected.Function == nil {
		t.Fatal("expected injected tool function to be present")
	}
	if injected.Function.Name != "get_weather" {
		t.Fatalf("unexpected injected tool name: %q", injected.Function.Name)
	}
	if injected.Function.Description == nil || *injected.Function.Description != "Get current weather" {
		t.Fatalf("unexpected injected tool description: %#v", injected.Function.Description)
	}
	if injected.Function.Parameters == nil || injected.Function.Parameters.Properties == nil {
		t.Fatal("expected injected tool parameters to contain properties")
	}
	if _, ok := injected.Function.Parameters.Properties.Get("location"); !ok {
		t.Fatal("expected injected tool parameters to contain location property")
	}
}

func TestInitAndPreLLMHookInjectsRawToolType(t *testing.T) {
	t.Cleanup(func() {
		_ = Cleanup()
	})

	config := map[string]any{
		"type": "openrouter:web_search",
	}

	if err := Init(config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o-mini",
			Params: &schemas.ChatParameters{
				Tools: []schemas.ChatTool{
					{
						Type: schemas.ChatToolTypeFunction,
						Function: &schemas.ChatToolFunction{
							Name: "existing_tool",
						},
					},
				},
			},
		},
	}

	modifiedReq, shortCircuit, err := PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook failed: %v", err)
	}
	if shortCircuit != nil {
		t.Fatalf("expected no short-circuit, got %#v", shortCircuit)
	}
	if modifiedReq != req {
		t.Fatal("expected request to be modified in place")
	}

	tools := req.ChatRequest.Params.Tools
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	if tools[0].Function == nil || tools[0].Function.Name != "existing_tool" {
		t.Fatalf("expected existing tool to remain first, got %#v", tools[0].Function)
	}

	injected := tools[1]
	if injected.Type != schemas.ChatToolType("openrouter:web_search") {
		t.Fatalf("unexpected injected tool type: %q", injected.Type)
	}
	if injected.Function != nil {
		t.Fatalf("expected raw tool to have no function wrapper, got %#v", injected.Function)
	}
	if injected.Custom != nil {
		t.Fatalf("expected raw tool to have no custom wrapper, got %#v", injected.Custom)
	}
}

func TestInitAndPreLLMHookSkipsNonMatchingProvider(t *testing.T) {
	t.Cleanup(func() {
		_ = Cleanup()
	})

	config := map[string]any{
		"providers": []any{"openrouter"},
		"type":      "openrouter:web_search",
	}

	if err := Init(config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Provider: schemas.OpenAI,
			Model:    "gpt-4o-mini",
			Params: &schemas.ChatParameters{
				Tools: []schemas.ChatTool{
					{
						Type: schemas.ChatToolTypeFunction,
						Function: &schemas.ChatToolFunction{
							Name: "existing_tool",
						},
					},
				},
			},
		},
	}

	modifiedReq, shortCircuit, err := PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook failed: %v", err)
	}
	if shortCircuit != nil {
		t.Fatalf("expected no short-circuit, got %#v", shortCircuit)
	}
	if modifiedReq != req {
		t.Fatal("expected request to be modified in place")
	}

	tools := req.ChatRequest.Params.Tools
	if len(tools) != 1 {
		t.Fatalf("expected tools to remain unchanged, got %d", len(tools))
	}
	if tools[0].Function == nil || tools[0].Function.Name != "existing_tool" {
		t.Fatalf("expected existing tool to remain unchanged, got %#v", tools[0].Function)
	}
}

func TestInitEchoToolsReturnsInjectedToolNames(t *testing.T) {
	t.Cleanup(func() {
		_ = Cleanup()
	})

	config := map[string]any{
		"echo_tools": true,
		"name":       "get_weather",
		"parameters": map[string]any{
			"type": "object",
		},
	}

	if err := Init(config); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	ctx := schemas.NewBifrostContext(context.Background(), schemas.NoDeadline)
	req := &schemas.BifrostRequest{
		ChatRequest: &schemas.BifrostChatRequest{
			Model: "gpt-4o-mini",
		},
	}

	_, shortCircuit, err := PreLLMHook(ctx, req)
	if err != nil {
		t.Fatalf("PreLLMHook failed: %v", err)
	}
	if shortCircuit == nil || shortCircuit.Response == nil || shortCircuit.Response.ChatResponse == nil {
		t.Fatal("expected echo-tools mode to short-circuit with a chat response")
	}
	if len(shortCircuit.Response.ChatResponse.Choices) == 0 {
		t.Fatal("expected echo-tools mode to return at least one choice")
	}

	content := shortCircuit.Response.ChatResponse.Choices[0].ChatNonStreamResponseChoice.Message.Content.ContentStr
	if content == nil || !strings.Contains(*content, "get_weather") {
		t.Fatalf("expected echoed tool name in response, got %v", content)
	}
}

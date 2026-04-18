package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// ToolConfig defines the structure of a single tool to be injected.
type ToolConfig struct {
	Type        string                          `json:"type,omitempty"`
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Parameters  *schemas.ToolFunctionParameters `json:"parameters"`
}

// ToolInjectorConfig supports either a single top-level tool definition or a list of tools.
type ToolInjectorConfig struct {
	Tools       []ToolConfig                    `json:"tools,omitempty"`
	CustomTools []ToolConfig                    `json:"custom,omitempty"`
	Providers   []string                        `json:"providers,omitempty"`
	Type        string                          `json:"type,omitempty"`
	Name        string                          `json:"name,omitempty"`
	Description string                          `json:"description,omitempty"`
	Parameters  *schemas.ToolFunctionParameters `json:"parameters,omitempty"`
	EchoTools   bool                            `json:"echo_tools,omitempty"`
}

var pluginConfig = ToolInjectorConfig{}
var toolInjectorRuntime *ToolInjectorPlugin

// ToolInjectorPlugin implements schemas.LLMPlugin to inject configured tools into LLM requests.
type ToolInjectorPlugin struct {
	Config    []ToolConfig
	EchoTools bool
	Providers []string
}

func normalizeToolConfigs(config any) ([]ToolConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("plugin config cannot be nil")
	}

	switch cfg := config.(type) {
	case ToolConfig:
		return validateToolConfigs([]ToolConfig{cfg})
	case *ToolConfig:
		if cfg == nil {
			return nil, fmt.Errorf("plugin config cannot be nil")
		}
		return validateToolConfigs([]ToolConfig{*cfg})
	case []ToolConfig:
		return validateToolConfigs(cfg)
	case *[]ToolConfig:
		if cfg == nil {
			return nil, fmt.Errorf("plugin config cannot be nil")
		}
		return validateToolConfigs(*cfg)
	case ToolInjectorConfig:
		return toolConfigsFromInjectorConfig(cfg)
	case *ToolInjectorConfig:
		if cfg == nil {
			return nil, fmt.Errorf("plugin config cannot be nil")
		}
		return toolConfigsFromInjectorConfig(*cfg)
	case map[string]any:
		data, err := json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal config: %w", err)
		}
		var parsed ToolInjectorConfig
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		return toolConfigsFromInjectorConfig(parsed)
	default:
		data, err := json.Marshal(config)
		if err != nil {
			return nil, fmt.Errorf("unsupported config type %T: %w", config, err)
		}
		var parsed ToolInjectorConfig
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		return toolConfigsFromInjectorConfig(parsed)
	}
}

func toolConfigsFromInjectorConfig(cfg ToolInjectorConfig) ([]ToolConfig, error) {
	if len(cfg.Tools) > 0 {
		return validateToolConfigs(cfg.Tools)
	}
	if len(cfg.CustomTools) > 0 {
		return validateToolConfigs(cfg.CustomTools)
	}
	if cfg.Type != "" {
		return validateToolConfigs([]ToolConfig{{
			Type:        cfg.Type,
			Name:        cfg.Name,
			Description: cfg.Description,
			Parameters:  cfg.Parameters,
		}})
	}
	if cfg.Name != "" {
		return validateToolConfigs([]ToolConfig{{
			Name:        cfg.Name,
			Description: cfg.Description,
			Parameters:  cfg.Parameters,
		}})
	}
	return nil, fmt.Errorf("at least one tool must be provided in config")
}

func normalizeProviders(providers []string) []string {
	if len(providers) == 0 {
		return nil
	}
	out := make([]string, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	return out
}

func validateToolConfigs(tools []ToolConfig) ([]ToolConfig, error) {
	if len(tools) == 0 {
		return nil, fmt.Errorf("at least one tool must be provided in config")
	}
	out := make([]ToolConfig, 0, len(tools))
	for i, tool := range tools {
		normalized, err := normalizeToolConfig(tool, i)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeToolConfig(tool ToolConfig, idx int) (ToolConfig, error) {
	if tool.Type == "" {
		if tool.Name == "" {
			return ToolConfig{}, fmt.Errorf("tool name is required for tool at index %d", idx)
		}
		tool.Type = string(schemas.ChatToolTypeFunction)
	}

	switch tool.Type {
	case string(schemas.ChatToolTypeFunction):
		if tool.Name == "" {
			return ToolConfig{}, fmt.Errorf("tool name is required for tool at index %d", idx)
		}
	case string(schemas.ChatToolTypeCustom):
		// Custom tools are represented directly by the type field.
	default:
		if tool.Name == "" {
			// Provider-specific tools can be forwarded as-is without a function wrapper.
			return tool, nil
		}
	}

	return tool, nil
}

func parseToolInjectorConfig(config any) ToolInjectorConfig {
	var parsed ToolInjectorConfig
	if config == nil {
		return parsed
	}
	switch cfg := config.(type) {
	case ToolInjectorConfig:
		return cfg
	case *ToolInjectorConfig:
		if cfg != nil {
			return *cfg
		}
	case map[string]any:
		data, err := json.Marshal(cfg)
		if err == nil {
			_ = json.Unmarshal(data, &parsed)
		}
	default:
		data, err := json.Marshal(config)
		if err == nil {
			_ = json.Unmarshal(data, &parsed)
		}
	}
	return parsed
}

func providerMatches(allowed []string, provider schemas.ModelProvider) bool {
	if len(allowed) == 0 {
		return true
	}
	current := strings.ToLower(string(provider))
	for _, candidate := range allowed {
		if candidate == current {
			return true
		}
	}
	return false
}

// Init is called by the plugin loader when the shared object is loaded.
func Init(config any) error {
	parsed := parseToolInjectorConfig(config)
	tools, err := normalizeToolConfigs(config)
	if err != nil {
		return err
	}
	providers := normalizeProviders(parsed.Providers)
	pluginConfig = ToolInjectorConfig{Tools: tools, EchoTools: parsed.EchoTools, Providers: providers}
	toolInjectorRuntime = &ToolInjectorPlugin{Config: tools, EchoTools: parsed.EchoTools, Providers: providers}
	return nil
}

// GetName returns the plugin name expected by the Bifrost plugin loader.
func GetName() string {
	return "tool-injector"
}

// Cleanup releases the runtime instance.
func Cleanup() error {
	toolInjectorRuntime = nil
	return nil
}

// PreLLMHook is the package-level hook symbol required by the plugin loader.
func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if toolInjectorRuntime == nil {
		return req, nil, nil
	}
	return toolInjectorRuntime.PreLLMHook(ctx, req)
}

// PostLLMHook is the package-level hook symbol required by the plugin loader.
func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if toolInjectorRuntime == nil {
		return resp, bifrostErr, nil
	}
	return toolInjectorRuntime.PostLLMHook(ctx, resp, bifrostErr)
}

// NewToolInjectorPlugin creates a new instance of the ToolInjectorPlugin.
func NewToolInjectorPlugin(config any) (*ToolInjectorPlugin, error) {
	tools, err := normalizeToolConfigs(config)
	if err != nil {
		return nil, err
	}
	parsed := parseToolInjectorConfig(config)
	return &ToolInjectorPlugin{Config: tools, EchoTools: parsed.EchoTools, Providers: normalizeProviders(parsed.Providers)}, nil
}

// GetName returns the name of the plugin.
func (p *ToolInjectorPlugin) GetName() string {
	return "tool-injector"
}

// Cleanup is called on bifrost shutdown.
func (p *ToolInjectorPlugin) Cleanup() error {
	return nil
}

func buildInjectedTool(tool ToolConfig) schemas.ChatTool {
	if tool.Type != "" && tool.Type != string(schemas.ChatToolTypeFunction) {
		return schemas.ChatTool{
			Type: schemas.ChatToolType(tool.Type),
		}
	}

	params := tool.Parameters
	if params == nil {
		params = &schemas.ToolFunctionParameters{
			Type:       "object",
			Properties: schemas.NewOrderedMap(),
		}
	} else if params.Type == "" {
		params.Type = "object"
		if params.Properties == nil {
			params.Properties = schemas.NewOrderedMap()
		}
	} else if params.Type == "object" && params.Properties == nil {
		params.Properties = schemas.NewOrderedMap()
	}

	return schemas.ChatTool{
		Type: schemas.ChatToolTypeFunction,
		Function: &schemas.ChatToolFunction{
			Name:        tool.Name,
			Description: &tool.Description,
			Parameters:  params,
		},
	}
}

func echoToolsShortCircuit(req *schemas.BifrostRequest) *schemas.LLMPluginShortCircuit {
	toolNames := make([]string, 0, 4)
	if req.ChatRequest != nil && req.ChatRequest.Params != nil {
		for _, tool := range req.ChatRequest.Params.Tools {
			if tool.Function != nil && tool.Function.Name != "" {
				toolNames = append(toolNames, tool.Function.Name)
				continue
			}
			if tool.Type != "" {
				toolNames = append(toolNames, string(tool.Type))
			}
		}
	}

	message := "tools:"
	if len(toolNames) > 0 {
		message += " " + strings.Join(toolNames, ", ")
	} else {
		message += " none"
	}

	finishReason := "stop"
	return &schemas.LLMPluginShortCircuit{
		Response: &schemas.BifrostResponse{
			ChatResponse: &schemas.BifrostChatResponse{
				Model: req.ChatRequest.Model,
				Usage: &schemas.BifrostLLMUsage{
					PromptTokens:     1,
					CompletionTokens: 1,
					TotalTokens:      2,
				},
				Choices: []schemas.BifrostResponseChoice{
					{
						Index: 0,
						ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{
							Message: &schemas.ChatMessage{
								Role: schemas.ChatMessageRoleAssistant,
								Content: &schemas.ChatMessageContent{
									ContentStr: &message,
								},
							},
						},
						FinishReason: &finishReason,
					},
				},
				ExtraFields: schemas.BifrostResponseExtraFields{
					RequestType:            schemas.ChatCompletionRequest,
					OriginalModelRequested: req.ChatRequest.Model,
				},
			},
		},
	}
}

func mergeTools(existing []schemas.ChatTool, injected []schemas.ChatTool) []schemas.ChatTool {
	if len(injected) == 0 {
		return existing
	}

	result := make([]schemas.ChatTool, 0, len(existing)+len(injected))
	skip := make(map[string]struct{}, len(injected))
	for _, tool := range injected {
		skip[toolIdentity(tool)] = struct{}{}
	}

	for _, tool := range existing {
		if _, ok := skip[toolIdentity(tool)]; ok {
			continue
		}
		result = append(result, tool)
	}

	result = append(result, injected...)
	return result
}

func toolIdentity(tool schemas.ChatTool) string {
	switch {
	case tool.Function != nil && tool.Function.Name != "":
		return "function:" + tool.Function.Name
	case tool.Custom != nil:
		return "custom"
	default:
		return "type:" + string(tool.Type)
	}
}

// PreLLMHook intercepts the request and injects configured tools into the tools list.
func (p *ToolInjectorPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	// We only care about Chat Completion requests
	if req.ChatRequest == nil {
		return req, nil, nil
	}
	if !providerMatches(p.Providers, req.ChatRequest.Provider) {
		return req, nil, nil
	}

	// Ensure Params are initialized
	if req.ChatRequest.Params == nil {
		req.ChatRequest.Params = &schemas.ChatParameters{}
	}

	injectedTools := make([]schemas.ChatTool, 0, len(p.Config))
	for _, tool := range p.Config {
		injectedTools = append(injectedTools, buildInjectedTool(tool))
	}

	req.ChatRequest.Params.Tools = mergeTools(req.ChatRequest.Params.Tools, injectedTools)

	if p.EchoTools {
		return req, echoToolsShortCircuit(req), nil
	}

	return req, nil, nil
}

// PostLLMHook is not used by this plugin.
func (p *ToolInjectorPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

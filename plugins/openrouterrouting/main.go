package main

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

type ProviderPreferences struct {
	Order                  []string       `json:"order,omitempty"`
	Only                   []string       `json:"only,omitempty"`
	Ignore                 []string       `json:"ignore,omitempty"`
	AllowFallbacks         *bool          `json:"allow_fallbacks,omitempty"`
	RequireParameters      *bool          `json:"require_parameters,omitempty"`
	DataCollection         *string        `json:"data_collection,omitempty"`
	ZDR                    *bool          `json:"zdr,omitempty"`
	EnforceDistillableText *bool          `json:"enforce_distillable_text,omitempty"`
	Quantizations          []string       `json:"quantizations,omitempty"`
	Sort                   any            `json:"sort,omitempty"`
	PreferredMinThroughput any            `json:"preferred_min_throughput,omitempty"`
	PreferredMaxLatency    any            `json:"preferred_max_latency,omitempty"`
	MaxPrice               map[string]any `json:"max_price,omitempty"`
}

type RoutingRule struct {
	Models   []string             `json:"models,omitempty"`
	Provider *ProviderPreferences `json:"provider,omitempty"`
}

type OpenRouterRoutingConfig struct {
	Providers []string             `json:"providers,omitempty"`
	Rules     []RoutingRule        `json:"rules,omitempty"`
	Models    []string             `json:"models,omitempty"`
	Provider  *ProviderPreferences `json:"provider,omitempty"`
}

type compiledRule struct {
	Models       []string
	ProviderJSON map[string]any
}

type OpenRouterRoutingPlugin struct {
	Providers []string
	Rules     []compiledRule
}

var openRouterRoutingRuntime *OpenRouterRoutingPlugin

func parseConfig(config any) OpenRouterRoutingConfig {
	var parsed OpenRouterRoutingConfig
	if config == nil {
		return parsed
	}
	switch cfg := config.(type) {
	case OpenRouterRoutingConfig:
		return cfg
	case *OpenRouterRoutingConfig:
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

func normalizeProviders(providers []string) []string {
	if len(providers) == 0 {
		return nil
	}
	out := make([]string, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		normalized := strings.ToLower(strings.TrimSpace(provider))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeModelPatterns(models []string) ([]string, error) {
	if len(models) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		pattern := strings.ToLower(strings.TrimSpace(model))
		if pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, "probe"); err != nil {
			return nil, fmt.Errorf("invalid model pattern %q: %w", model, err)
		}
		if _, ok := seen[pattern]; ok {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	return out, nil
}

func normalizePreferences(pref *ProviderPreferences) (map[string]any, error) {
	if pref == nil {
		return nil, fmt.Errorf("provider preferences are required")
	}

	order := normalizeProviders(pref.Order)
	only := normalizeProviders(pref.Only)
	ignore := normalizeProviders(pref.Ignore)
	quantizations := normalizeProviders(pref.Quantizations)

	policy := make(map[string]any)
	if len(order) > 0 {
		policy["order"] = order
	}
	if len(only) > 0 {
		policy["only"] = only
	}
	if len(ignore) > 0 {
		policy["ignore"] = ignore
	}
	if pref.AllowFallbacks != nil {
		policy["allow_fallbacks"] = *pref.AllowFallbacks
	}
	if pref.RequireParameters != nil {
		policy["require_parameters"] = *pref.RequireParameters
	}
	if pref.DataCollection != nil {
		policy["data_collection"] = strings.ToLower(strings.TrimSpace(*pref.DataCollection))
	}
	if pref.ZDR != nil {
		policy["zdr"] = *pref.ZDR
	}
	if pref.EnforceDistillableText != nil {
		policy["enforce_distillable_text"] = *pref.EnforceDistillableText
	}
	if len(quantizations) > 0 {
		policy["quantizations"] = quantizations
	}
	if pref.Sort != nil {
		policy["sort"] = pref.Sort
	}
	if pref.PreferredMinThroughput != nil {
		policy["preferred_min_throughput"] = pref.PreferredMinThroughput
	}
	if pref.PreferredMaxLatency != nil {
		policy["preferred_max_latency"] = pref.PreferredMaxLatency
	}
	if len(pref.MaxPrice) > 0 {
		policy["max_price"] = pref.MaxPrice
	}
	if len(policy) == 0 {
		return nil, fmt.Errorf("provider preferences cannot be empty")
	}
	return policy, nil
}

func compileRules(cfg OpenRouterRoutingConfig) ([]compiledRule, error) {
	rules := cfg.Rules
	if len(rules) == 0 && cfg.Provider != nil {
		rules = []RoutingRule{{
			Models:   cfg.Models,
			Provider: cfg.Provider,
		}}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("at least one rule or top-level provider config is required")
	}

	out := make([]compiledRule, 0, len(rules))
	for idx, rule := range rules {
		models, err := normalizeModelPatterns(rule.Models)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", idx, err)
		}
		policy, err := normalizePreferences(rule.Provider)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", idx, err)
		}
		out = append(out, compiledRule{
			Models:       models,
			ProviderJSON: policy,
		})
	}
	return out, nil
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
	if !isOpenAICompatibleRequestProvider(provider) {
		return false
	}
	for _, candidate := range allowed {
		// Custom OpenRouter providers commonly sit behind an OpenAI-compatible
		// endpoint, so the request still enters PreLLMHook as "openai" before the
		// transport resolves the final custom provider key.
		if isCustomOpenRouterProvider(candidate) {
			return true
		}
	}
	return false
}

func isOpenAICompatibleRequestProvider(provider schemas.ModelProvider) bool {
	return provider == schemas.OpenAI || provider == schemas.Azure
}

func isCustomOpenRouterProvider(provider string) bool {
	return !schemas.IsKnownProvider(provider) && strings.Contains(provider, "openrouter")
}

func ruleMatches(patterns []string, model string) bool {
	if len(patterns) == 0 {
		return true
	}
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	for _, pattern := range patterns {
		matched, err := path.Match(pattern, normalizedModel)
		if err == nil && matched {
			return true
		}
		if pattern == normalizedModel {
			return true
		}
	}
	return false
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return make(map[string]any)
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func toObjectMap(value any) (map[string]any, bool) {
	if value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed), true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = v
		}
		return out, true
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, false
		}
		return out, true
	}
}

func mergeProviderPolicy(extraParams map[string]any, providerPolicy map[string]any) map[string]any {
	if extraParams == nil {
		extraParams = make(map[string]any)
	}

	merged := make(map[string]any)
	if existing, ok := toObjectMap(extraParams["provider"]); ok {
		for k, v := range existing {
			merged[k] = v
		}
	}
	for k, v := range providerPolicy {
		merged[k] = v
	}
	extraParams["provider"] = merged
	return extraParams
}

func Init(config any) error {
	parsed := parseConfig(config)
	rules, err := compileRules(parsed)
	if err != nil {
		return err
	}
	openRouterRoutingRuntime = &OpenRouterRoutingPlugin{
		Providers: normalizeProviders(parsed.Providers),
		Rules:     rules,
	}
	return nil
}

func GetName() string {
	return "openrouter-provider-routing"
}

func Cleanup() error {
	openRouterRoutingRuntime = nil
	return nil
}

func PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if openRouterRoutingRuntime == nil {
		return req, nil, nil
	}
	return openRouterRoutingRuntime.PreLLMHook(ctx, req)
}

func PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	if openRouterRoutingRuntime == nil {
		return resp, bifrostErr, nil
	}
	return openRouterRoutingRuntime.PostLLMHook(ctx, resp, bifrostErr)
}

func NewOpenRouterRoutingPlugin(config any) (*OpenRouterRoutingPlugin, error) {
	parsed := parseConfig(config)
	rules, err := compileRules(parsed)
	if err != nil {
		return nil, err
	}
	return &OpenRouterRoutingPlugin{
		Providers: normalizeProviders(parsed.Providers),
		Rules:     rules,
	}, nil
}

func (p *OpenRouterRoutingPlugin) GetName() string {
	return "openrouter-provider-routing"
}

func (p *OpenRouterRoutingPlugin) Cleanup() error {
	return nil
}

func (p *OpenRouterRoutingPlugin) findMatchingRule(model string) *compiledRule {
	for i := range p.Rules {
		if ruleMatches(p.Rules[i].Models, model) {
			return &p.Rules[i]
		}
	}
	return nil
}

func (p *OpenRouterRoutingPlugin) applyChat(req *schemas.BifrostChatRequest, rule *compiledRule) {
	if req.Params == nil {
		req.Params = &schemas.ChatParameters{}
	}
	req.Params.ExtraParams = mergeProviderPolicy(req.Params.ExtraParams, rule.ProviderJSON)
}

func (p *OpenRouterRoutingPlugin) applyText(req *schemas.BifrostTextCompletionRequest, rule *compiledRule) {
	if req.Params == nil {
		req.Params = &schemas.TextCompletionParameters{}
	}
	req.Params.ExtraParams = mergeProviderPolicy(req.Params.ExtraParams, rule.ProviderJSON)
}

func (p *OpenRouterRoutingPlugin) applyEmbedding(req *schemas.BifrostEmbeddingRequest, rule *compiledRule) {
	if req.Params == nil {
		req.Params = &schemas.EmbeddingParameters{}
	}
	req.Params.ExtraParams = mergeProviderPolicy(req.Params.ExtraParams, rule.ProviderJSON)
}

func (p *OpenRouterRoutingPlugin) applyResponses(req *schemas.BifrostResponsesRequest, rule *compiledRule) {
	if req.Params == nil {
		req.Params = &schemas.ResponsesParameters{}
	}
	req.Params.ExtraParams = mergeProviderPolicy(req.Params.ExtraParams, rule.ProviderJSON)
}

func usesRawRequestBody(ctx *schemas.BifrostContext, rawRequestBody []byte) bool {
	if len(rawRequestBody) > 0 {
		return true
	}
	if ctx == nil {
		return false
	}
	if useRaw, ok := ctx.Value(schemas.BifrostContextKeyUseRawRequestBody).(bool); ok && useRaw {
		return true
	}
	return false
}

func (p *OpenRouterRoutingPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	if req == nil {
		return nil, nil, nil
	}

	provider, model, _ := req.GetRequestFields()
	if !providerMatches(p.Providers, provider) {
		return req, nil, nil
	}

	rule := p.findMatchingRule(model)
	if rule == nil {
		return req, nil, nil
	}

	applied := false
	switch {
	case req.ChatRequest != nil:
		if usesRawRequestBody(ctx, req.ChatRequest.RawRequestBody) {
			return req, nil, nil
		}
		p.applyChat(req.ChatRequest, rule)
		applied = true
	case req.TextCompletionRequest != nil:
		if usesRawRequestBody(ctx, req.TextCompletionRequest.RawRequestBody) {
			return req, nil, nil
		}
		p.applyText(req.TextCompletionRequest, rule)
		applied = true
	case req.EmbeddingRequest != nil:
		if usesRawRequestBody(ctx, req.EmbeddingRequest.RawRequestBody) {
			return req, nil, nil
		}
		p.applyEmbedding(req.EmbeddingRequest, rule)
		applied = true
	case req.ResponsesRequest != nil:
		if usesRawRequestBody(ctx, req.ResponsesRequest.RawRequestBody) {
			return req, nil, nil
		}
		p.applyResponses(req.ResponsesRequest, rule)
		applied = true
	case req.CountTokensRequest != nil:
		if usesRawRequestBody(ctx, req.CountTokensRequest.RawRequestBody) {
			return req, nil, nil
		}
		p.applyResponses(req.CountTokensRequest, rule)
		applied = true
	}

	if applied && ctx != nil {
		ctx.SetValue(schemas.BifrostContextKeyPassthroughExtraParams, true)
	}
	return req, nil, nil
}

func (p *OpenRouterRoutingPlugin) PostLLMHook(ctx *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

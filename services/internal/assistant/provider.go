package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/automation"
)

// ChatProvider is the transport boundary. CAD scope and execution stay in Host.
type ChatProvider interface {
	Validate(context.Context, string) error
	Models(context.Context, string) ([]Model, error)
	Complete(context.Context, string, []Message, []Tool, ...Generation) (Completion, error)
}

type Model struct {
	ID     string       `json:"id"`
	Name   string       `json:"name,omitempty"`
	Effort *ModelEffort `json:"effort,omitempty"`
}
type ModelEffort struct {
	SupportedLevels []string `json:"supported_levels"`
}

// Generation freezes the actual configuration of one outbound attempt.
type Generation struct {
	Provider       string        `json:"provider"`
	ProviderOrigin string        `json:"providerOrigin,omitempty"`
	Model          string        `json:"model"`
	Thinking       string        `json:"thinking"`
	Version        uint64        `json:"version"`
	Limits         RequestLimits `json:"limits"`
	MaxTokens      int           `json:"maxTokens"`
}

func validThinking(value string) bool {
	return value == "off" || value == "low" || value == "high" || value == "max"
}

type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
}

func (message Message) MarshalJSON() ([]byte, error) {
	type wire Message
	if message.Role != "assistant" {
		return json.Marshal(wire(message))
	}
	// Empty reasoning is still a required transcript field for thinking tool
	// rounds. Never drop a field that the upstream provider returned empty.
	return json.Marshal(struct {
		wire
		Reasoning string `json:"reasoning_content"`
	}{wire(message), message.ReasoningContent})
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type Usage struct {
	Raw              json.RawMessage `json:"raw,omitempty"`
	CacheHitTokens   *int64          `json:"cacheHitTokens,omitempty"`
	ReasoningTokens  *int64          `json:"reasoningTokens,omitempty"`
	PromptTokens     int             `json:"prompt_tokens"`
	CompletionTokens int             `json:"completion_tokens"`
	TotalTokens      int             `json:"total_tokens"`
}

func (u *Usage) UnmarshalJSON(raw []byte) error {
	type wire struct {
		PromptTokens        int             `json:"prompt_tokens"`
		CompletionTokens    int             `json:"completion_tokens"`
		TotalTokens         int             `json:"total_tokens"`
		CacheHit            *int64          `json:"prompt_cache_hit_tokens"`
		CacheHitNormalized  *int64          `json:"cacheHitTokens"`
		ReasoningNormalized *int64          `json:"reasoningTokens"`
		Raw                 json.RawMessage `json:"raw"`
		Details             *struct {
			Reasoning *int64 `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
		PromptDetails *struct {
			Cached *int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	var v wire
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	u.PromptTokens = v.PromptTokens
	u.CompletionTokens = v.CompletionTokens
	u.TotalTokens = v.TotalTokens
	u.CacheHitTokens = v.CacheHit
	if u.CacheHitTokens == nil && v.PromptDetails != nil {
		u.CacheHitTokens = v.PromptDetails.Cached
	}
	if u.CacheHitTokens == nil {
		u.CacheHitTokens = v.CacheHitNormalized
	}
	if v.Details != nil {
		u.ReasoningTokens = v.Details.Reasoning
	} else {
		u.ReasoningTokens = v.ReasoningNormalized
	}
	// Keep numeric provider usage evidence, never arbitrary response text/keys.
	evidence := raw
	if len(v.Raw) > 0 {
		evidence = v.Raw
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(evidence, &object)
	clean := map[string]any{}
	for _, key := range []string{"prompt_tokens", "completion_tokens", "total_tokens", "prompt_cache_hit_tokens", "prompt_cache_miss_tokens"} {
		var n int64
		if value, ok := object[key]; ok && json.Unmarshal(value, &n) == nil && n >= 0 {
			clean[key] = n
		}
	}
	for _, key := range []string{"completion_tokens_details", "prompt_tokens_details"} {
		var details map[string]json.RawMessage
		if json.Unmarshal(object[key], &details) == nil {
			safe := map[string]int64{}
			for _, name := range []string{"reasoning_tokens", "cached_tokens", "accepted_prediction_tokens", "rejected_prediction_tokens"} {
				var n int64
				if data, ok := details[name]; ok && json.Unmarshal(data, &n) == nil && n >= 0 {
					safe[name] = n
				}
			}
			if len(safe) > 0 {
				clean[key] = safe
			}
		}
	}
	u.Raw, _ = json.Marshal(clean)
	return nil
}

type Completion struct {
	Message           Message
	Usage             *Usage
	ProviderRequestID string
	ResponseID        string
}
type OpenAIProvider struct {
	BaseURL string
	Model   string
	Client  *http.Client
}

func newProvider(baseURL, model string) *OpenAIProvider {
	return &OpenAIProvider{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, Client: &http.Client{
		// Never forward the user's bearer credential to a redirect target.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func aiFault(code, message string, retryable bool) error {
	return &automation.Fault{Code: code, Message: message, Retryable: retryable}
}

type providerRequestIDKey struct{}

func (p *OpenAIProvider) request(ctx context.Context, key, method, path string, body any, limits ...RequestLimits) ([]byte, error) {
	maxInput, maxResponse, timeout := 96<<10, 256<<10, 75*time.Second
	if len(limits) > 0 {
		budget := limits[0]
		maxInput = budget.MaxContextBytes + 64<<10          // function schemas and envelope
		maxResponse = 256<<10 + budget.MaxThinkingTokens*12 // bounded UTF-8/JSON overhead
		timeout = time.Duration(budget.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	base, err := url.Parse(p.BaseURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" ||
		(base.Scheme != "https" && !(base.Scheme == "http" && (base.Hostname() == "127.0.0.1" || base.Hostname() == "localhost" || base.Hostname() == "::1"))) {
		return nil, aiFault("AI_CONFIGURATION", "AI 服务地址配置无效", false)
	}
	var input []byte
	if body != nil {
		input, err = json.Marshal(body)
		if err != nil || len(input) > maxInput {
			return nil, budgetFault("AI_CONTEXT_BUDGET")
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, p.BaseURL+path, bytes.NewReader(input))
	if err != nil {
		return nil, aiFault("AI_CONFIGURATION", "无法构造 AI 请求", false)
	}
	r.Header.Set("Authorization", "Bearer "+key)
	r.Header.Set("Content-Type", "application/json")
	response, err := p.Client.Do(r)
	if err != nil {
		// Do not surface URLs, upstream bodies or headers containing credentials.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, aiFault("AI_UNAVAILABLE", "暂时无法连接 DeepSeek，请稍后重试", true)
	}
	defer response.Body.Close()
	if value, ok := ctx.Value(providerRequestIDKey{}).(*string); ok {
		*value = response.Header.Get("X-Request-ID")
		if *value == "" {
			*value = response.Header.Get("Request-ID")
		}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		switch response.StatusCode {
		case 401, 403:
			return nil, aiFault("AI_INVALID_KEY", "API Key 无效或没有访问权限，请在连接设置中更新", false)
		case 402:
			return nil, aiFault("AI_BALANCE", "DeepSeek 账户余额不足，请充值后重试", false)
		case 429:
			return nil, aiFault("AI_RATE_LIMIT", "DeepSeek 请求过于频繁，请稍后重试", true)
		default:
			return nil, aiFault("AI_PROVIDER_ERROR", "DeepSeek 未能完成请求，请稍后重试；若持续出现，请检查服务配置", response.StatusCode >= 500)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(maxResponse)+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || len(raw) > maxResponse {
		return nil, aiFault("AI_RESPONSE_INVALID", "AI 返回的数据不完整或超出大小上限", false)
	}
	return raw, nil
}
func (p *OpenAIProvider) Validate(ctx context.Context, key string) error {
	_, err := p.Models(ctx, key)
	return err
}
func (p *OpenAIProvider) Models(ctx context.Context, key string) ([]Model, error) {
	raw, err := p.request(ctx, key, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	var models struct {
		Data []Model `json:"data"`
	}
	if json.Unmarshal(raw, &models) != nil || len(models.Data) == 0 || len(models.Data) > 100 {
		return nil, aiFault("AI_MODEL_UNAVAILABLE", "当前 API Key 未返回可用的模型", false)
	}
	seen := map[string]bool{}
	for _, model := range models.Data {
		if model.ID == "" || len(model.ID) > 160 || len(model.Name) > 256 || strings.ContainsAny(model.ID, "\r\n\t ") || seen[model.ID] {
			return nil, aiFault("AI_RESPONSE_INVALID", "模型目录格式无效", false)
		}
		seen[model.ID] = true
	}
	sort.Slice(models.Data, func(i, j int) bool { return models.Data[i].ID < models.Data[j].ID })
	return models.Data, nil
}
func (p *OpenAIProvider) Complete(ctx context.Context, key string, messages []Message, tools []Tool, choices ...Generation) (Completion, error) {
	generation := Generation{Provider: "DeepSeek", Model: p.Model, Thinking: "off", Limits: defaultRequestLimits()}
	if len(choices) > 0 {
		generation = choices[0]
	}
	if generation.Model == "" || len(generation.Model) > 160 || !validThinking(generation.Thinking) {
		return Completion{}, aiFault("AI_CONFIGURATION", "模型或思考度设置无效", false)
	}
	budget := generation.Limits
	if budget.MaxContextBytes == 0 {
		budget = defaultRequestLimits()
	}
	if budget.MaxContextBytes < 2048 || budget.MaxContextBytes > 4<<20 || budget.MaxOutputTokens < 1 || budget.MaxThinkingTokens < 1 || budget.TimeoutSeconds < 1 {
		return Completion{}, aiFault("AI_CONFIGURATION", "单次请求资源设置无效", false)
	}
	if raw, err := json.Marshal(messages); err != nil || len(raw) > budget.MaxContextBytes {
		return Completion{}, budgetFault("AI_CONTEXT_BUDGET")
	}
	maxTokens := budget.MaxOutputTokens
	if generation.Thinking != "off" {
		maxTokens = budget.MaxThinkingTokens
	}
	if generation.MaxTokens > 0 && generation.MaxTokens < maxTokens {
		maxTokens = generation.MaxTokens
	}
	functions := make([]any, 0, len(tools))
	for _, tool := range tools {
		functions = append(functions, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
	}
	body := map[string]any{
		"model": generation.Model, "messages": messages, "tools": functions, "tool_choice": "auto",
		"stream": false, "thinking": map[string]string{"type": "disabled"}, "max_tokens": maxTokens,
	}
	if generation.Thinking != "off" {
		body["thinking"] = map[string]string{"type": "enabled"}
		body["reasoning_effort"] = generation.Thinking
	}
	var providerRequestID string
	ctx = context.WithValue(ctx, providerRequestIDKey{}, &providerRequestID)
	raw, err := p.request(ctx, key, http.MethodPost, "/chat/completions", body, budget)
	if err != nil {
		return Completion{ProviderRequestID: providerRequestID}, err
	}
	var output struct {
		ID      string `json:"id"`
		Choices []struct {
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if json.Unmarshal(raw, &output) != nil {
		return Completion{}, aiFault("AI_RESPONSE_INVALID", "AI 未返回有效回复", false)
	}
	// Preserve reported cost even when the response cannot produce a plan.
	result := Completion{Usage: output.Usage, ProviderRequestID: providerRequestID, ResponseID: output.ID}
	if u := output.Usage; u != nil && (u.PromptTokens < 0 || u.CompletionTokens < 0 || u.TotalTokens < 0 || u.TotalTokens > 1<<30 || u.PromptTokens > u.TotalTokens || u.CompletionTokens > u.TotalTokens-u.PromptTokens) {
		return Completion{}, aiFault("AI_RESPONSE_INVALID", "AI 返回的用量格式无效", false)
	}
	if len(output.Choices) != 1 {
		return result, aiFault("AI_RESPONSE_INVALID", "AI 未返回有效回复", false)
	}
	choice := output.Choices[0]
	if choice.FinishReason == "length" {
		return result, budgetFault("AI_OUTPUT_BUDGET")
	}
	if choice.Message.Role != "assistant" || len(choice.Message.Content) > budget.MaxOutputTokens*4 || len(choice.Message.ReasoningContent) > budget.MaxThinkingTokens*4 || len(choice.Message.ToolCalls) > 8 ||
		(choice.FinishReason != "stop" && choice.FinishReason != "tool_calls") || (choice.Message.Content == "" && len(choice.Message.ToolCalls) == 0) {
		return result, aiFault("AI_RESPONSE_INVALID", "AI 回复格式不完整，本轮未执行新计划", false)
	}
	ids := map[string]bool{}
	for _, call := range choice.Message.ToolCalls {
		if call.ID == "" || len(call.ID) > 160 || ids[call.ID] || call.Type != "function" || len(call.Function.Arguments) > 32<<10 {
			return result, aiFault("AI_RESPONSE_INVALID", "AI 工具调用格式无效", false)
		}
		ids[call.ID] = true
	}
	result.Message = choice.Message
	return result, nil
}

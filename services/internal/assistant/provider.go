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

// Generation is frozen once per turn. It contains no credential or reasoning.
type Generation struct {
	Model    string `json:"model"`
	Thinking string `json:"thinking"`
	Budget   string `json:"budget"`
	// Per-call remaining output allowance; never serialized or persisted.
	maxTokens int
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
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
type Completion struct {
	Message Message
	Usage   *Usage
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
func (p *OpenAIProvider) request(ctx context.Context, key, method, path string, body any, budgets ...BudgetPreset) ([]byte, error) {
	maxInput, maxResponse, timeout := 96<<10, 256<<10, 75*time.Second
	if len(budgets) > 0 {
		budget := budgets[0]
		maxInput = budget.MaxContextBytes + 64<<10          // function schemas and envelope
		maxResponse = 256<<10 + budget.MaxThinkingTokens*12 // bounded UTF-8/JSON overhead
		timeout = time.Duration(budget.RequestTimeoutSeconds) * time.Second
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
	generation := Generation{Model: p.Model, Thinking: "off", Budget: defaultBudget}
	if len(choices) > 0 {
		generation = choices[0]
	}
	if generation.Model == "" || len(generation.Model) > 160 || !validThinking(generation.Thinking) {
		return Completion{}, aiFault("AI_CONFIGURATION", "模型或思考度设置无效", false)
	}
	budget, err := budgetFor(generation.Budget)
	if err != nil {
		return Completion{}, err
	}
	if raw, err := json.Marshal(messages); err != nil || len(raw) > budget.MaxContextBytes {
		return Completion{}, budgetFault("AI_CONTEXT_BUDGET")
	}
	maxTokens := budget.MaxOutputTokens
	if generation.Thinking != "off" {
		maxTokens = budget.MaxThinkingTokens
	}
	if generation.maxTokens > 0 && generation.maxTokens < maxTokens {
		maxTokens = generation.maxTokens
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
	raw, err := p.request(ctx, key, http.MethodPost, "/chat/completions", body, budget)
	if err != nil {
		return Completion{}, err
	}
	var output struct {
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
	result := Completion{Usage: output.Usage}
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
	if choice.Message.Role != "assistant" || len(choice.Message.Content) > 16<<10 || len(choice.Message.ReasoningContent) > budget.MaxThinkingTokens*4 || len(choice.Message.ToolCalls) > 8 ||
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
	return Completion{Message: choice.Message, Usage: output.Usage}, nil
}

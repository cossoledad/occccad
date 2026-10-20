package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/automation"
)

// ChatProvider is the transport boundary. CAD scope and execution stay in Host.
type ChatProvider interface {
	Validate(context.Context, string) error
	Complete(context.Context, string, []Message, []Tool) (Completion, error)
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
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
		Timeout: 75 * time.Second,
		// Never forward the user's bearer credential to a redirect target.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func aiFault(code, message string, retryable bool) error {
	return &automation.Fault{Code: code, Message: message, Retryable: retryable}
}
func (p *OpenAIProvider) request(ctx context.Context, key, method, path string, body any) ([]byte, error) {
	base, err := url.Parse(p.BaseURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" ||
		(base.Scheme != "https" && !(base.Scheme == "http" && (base.Hostname() == "127.0.0.1" || base.Hostname() == "localhost" || base.Hostname() == "::1"))) {
		return nil, aiFault("AI_CONFIGURATION", "AI 服务地址配置无效", false)
	}
	var input []byte
	if body != nil {
		input, err = json.Marshal(body)
		if err != nil || len(input) > 96<<10 {
			return nil, aiFault("AI_CONTEXT_BUDGET", "本轮上下文已达到上限，请缩小查询范围或开始新的请求", false)
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil || len(raw) > 256<<10 {
		return nil, aiFault("AI_RESPONSE_INVALID", "AI 返回的数据不完整或超出大小上限", false)
	}
	return raw, nil
}
func (p *OpenAIProvider) Validate(ctx context.Context, key string) error {
	raw, err := p.request(ctx, key, http.MethodGet, "/models", nil)
	if err != nil {
		return err
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &models) == nil {
		for _, model := range models.Data {
			if model.ID == p.Model {
				return nil
			}
		}
	}
	return aiFault("AI_MODEL_UNAVAILABLE", "当前 API Key 无法访问已配置的模型", false)
}
func (p *OpenAIProvider) Complete(ctx context.Context, key string, messages []Message, tools []Tool) (Completion, error) {
	functions := make([]any, 0, len(tools))
	for _, tool := range tools {
		functions = append(functions, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": tool.InputSchema}})
	}
	raw, err := p.request(ctx, key, http.MethodPost, "/chat/completions", map[string]any{
		"model": p.Model, "messages": messages, "tools": functions, "tool_choice": "auto",
		"stream": false, "thinking": map[string]string{"type": "disabled"}, "max_tokens": 4096,
	})
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
	if json.Unmarshal(raw, &output) != nil || len(output.Choices) != 1 {
		return Completion{}, aiFault("AI_RESPONSE_INVALID", "AI 未返回有效回复", false)
	}
	choice := output.Choices[0]
	if choice.FinishReason == "length" {
		return Completion{}, aiFault("AI_OUTPUT_BUDGET", "AI 回复达到长度上限，本轮未执行新计划；请缩小任务范围", false)
	}
	if choice.Message.Role != "assistant" || len(choice.Message.Content) > 16<<10 || len(choice.Message.ToolCalls) > 8 ||
		(choice.FinishReason != "stop" && choice.FinishReason != "tool_calls") || (choice.Message.Content == "" && len(choice.Message.ToolCalls) == 0) {
		return Completion{}, aiFault("AI_RESPONSE_INVALID", "AI 回复格式不完整，本轮未执行新计划", false)
	}
	ids := map[string]bool{}
	for _, call := range choice.Message.ToolCalls {
		if call.ID == "" || len(call.ID) > 160 || ids[call.ID] || call.Type != "function" || len(call.Function.Arguments) > 32<<10 {
			return Completion{}, aiFault("AI_RESPONSE_INVALID", "AI 工具调用格式无效", false)
		}
		ids[call.ID] = true
	}
	return Completion{Message: choice.Message, Usage: output.Usage}, nil
}

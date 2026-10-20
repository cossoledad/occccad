package assistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantOpenAIProtocol(t *testing.T) {
	key := "synthetic-provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("missing bearer authorization")
		}
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-flash"}]}`))
			return
		}
		if r.URL.Path != "/v1/chat/completions" || r.Method != "POST" {
			t.Error("wrong completion route")
		}
		var input map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("invalid JSON")
		}
		if string(input["model"]) != `"deepseek-flash"` || string(input["stream"]) != "false" || string(input["max_tokens"]) != "4096" {
			t.Error("wrong default model/budget")
		}
		var thinking map[string]string
		_ = json.Unmarshal(input["thinking"], &thinking)
		if thinking["type"] != "disabled" {
			t.Error("non-thinking mode must be explicit")
		}
		if strings.Contains(string(input["messages"]), key) {
			t.Error("credential leaked into model input")
		}
		var functions []struct {
			Function struct {
				Parameters map[string]json.RawMessage `json:"parameters"`
			} `json:"function"`
		}
		if json.Unmarshal(input["tools"], &functions) != nil || len(functions) == 0 {
			t.Error("missing function schemas")
		}
		for _, function := range functions {
			required := function.Function.Parameters["required"]
			if len(required) == 0 || required[0] != '[' {
				t.Error("JSON Schema required must be an array, including for tools with optional/no arguments")
			}
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"cad_context","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":18,"completion_tokens":12,"total_tokens":30}}`))
	}))
	defer server.Close()
	provider := newProvider(server.URL+"/v1/", "deepseek-flash")
	if err := provider.Validate(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	result, err := provider.Complete(t.Context(), key, []Message{{Role: "user", Content: "创建一个零件"}}, planningTools())
	if err != nil || len(result.Message.ToolCalls) != 1 || result.Usage.TotalTokens != 30 {
		t.Fatalf("completion: %+v %v", result, err)
	}
}
func TestAssistantProviderErrorsAreBoundedAndRedacted(t *testing.T) {
	for _, status := range []int{401, 402, 429, 500, 307} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			targetCalls := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++ }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("raw-secret-upstream-error"))
			}))
			defer server.Close()
			_, err := newProvider(server.URL, "deepseek-flash").Complete(t.Context(), "raw-secret", nil, nil)
			if err == nil || strings.Contains(err.Error(), "raw-secret") || targetCalls != 0 {
				t.Fatalf("unsafe error/redirect %v", err)
			}
		})
	}
	for _, response := range []string{
		`{"choices":[{"message":{"role":"assistant","content":"partial"},"finish_reason":"length"}]}`,
		`{"choices":[]}`, `not-json`,
		`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"cad_context","arguments":"{}"}},{"id":"c","type":"function","function":{"name":"cad_context","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
		_, err := newProvider(server.URL, "deepseek-flash").Complete(t.Context(), "synthetic", nil, nil)
		server.Close()
		if err == nil {
			t.Fatal("invalid completion accepted", response)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := newProvider("https://api.deepseek.com", "deepseek-flash").Complete(ctx, "synthetic", nil, nil)
	if err != context.Canceled {
		t.Fatal("cancellation not preserved", err)
	}
}

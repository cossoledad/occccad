package assistant

// RequestLimits bounds one provider request, never the total work in a Run.
type RequestLimits struct {
	MaxOutputTokens   int `json:"maxOutputTokens"`
	MaxThinkingTokens int `json:"maxThinkingTokens"`
	MaxContextBytes   int `json:"maxContextBytes"`
	TimeoutSeconds    int `json:"timeoutSeconds"`
}

func defaultRequestLimits() RequestLimits { return RequestLimits{8192, 32768, 256 << 10, 180} }
func validTokenBudget(value *int64) bool  { return value == nil || *value > 0 }
func budgetFault(code string) error {
	return aiFault(code, "本次请求超过单次资源边界；运行进度已保留，将重建工作集或等待必要调整", false)
}

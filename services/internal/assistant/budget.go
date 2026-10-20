package assistant

import "time"

// BudgetPreset is the server-owned resource envelope for one user request,
// including all model/tool round trips. It is independent of thinking effort.
type BudgetPreset struct {
	ID                    string `json:"id"`
	Label                 string `json:"label"`
	TotalTokens           int    `json:"totalTokens"`
	MaxModelCalls         int    `json:"maxModelCalls"`
	MaxToolCalls          int    `json:"maxToolCalls"`
	MaxOutputTokens       int    `json:"maxOutputTokens"`
	MaxThinkingTokens     int    `json:"maxThinkingTokens"`
	MaxContextBytes       int    `json:"maxContextBytes"`
	TimeoutSeconds        int    `json:"timeoutSeconds"`
	RequestTimeoutSeconds int    `json:"requestTimeoutSeconds"`
}

const defaultBudget = "medium"

func budgetPresets() []BudgetPreset {
	return []BudgetPreset{
		{"low", "低", 80000, 10, 24, 4096, 16384, 128 << 10, 180, 90},
		{"medium", "中", 200000, 20, 48, 8192, 32768, 256 << 10, 360, 180},
		{"high", "高", 500000, 32, 96, 16384, 65536, 512 << 10, 720, 240},
	}
}

func budgetFor(id string) (BudgetPreset, error) {
	if id == "" {
		id = defaultBudget
	}
	for _, preset := range budgetPresets() {
		if preset.ID == id {
			return preset, nil
		}
	}
	return BudgetPreset{}, aiFault("AI_CONFIGURATION", "请选择低、中或高调用预算", false)
}

func (b BudgetPreset) timeout() time.Duration { return time.Duration(b.TimeoutSeconds) * time.Second }

func budgetFault(code string) error {
	return aiFault(code, "本次需求已达到调用预算；可在输入框上方提高预算后重新发送，或缩小任务范围", false)
}

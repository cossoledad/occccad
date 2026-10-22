package assistant

import (
	"testing"
)

// These utterances distinguish inquiries about operations from instructions to
// change geometry; a question must not retire an active modeling plan.
func TestAssistantInputIntentClassification(t *testing.T) {
	for _, text := range []string{"当前圆角半径是多少？", "为什么这个拉伸失败？", "这个装配还有多少自由度？", "如何创建一个矩形？", "How does fillet work?", "查看当前进展"} {
		if !isQuestion(text) {
			t.Errorf("inquiry classified as steer: %s", text)
		}
	}
	for _, text := range []string{"可以把厚度改成20mm吗", "给选中边做R2圆角", "在XY平面建立80×50mm矩形，拉伸12mm", "插入已有零件并同轴装配", "不要做圆角", "Please create a box"} {
		if isQuestion(text) {
			t.Errorf("change request classified as inquiry: %s", text)
		}
	}
}

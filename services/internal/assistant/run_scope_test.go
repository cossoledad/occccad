package assistant

import (
	"github.com/google/uuid"
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

func TestAssistantAssemblyRequirementsKeepExplicitValuesAndPerFieldSources(t *testing.T) {
	first := Turn{ID: uuid.NewString(), RunID: uuid.NewString(), Text: "同轴装配：轴线方向 SAME，底端面有向法线 SAME，沿第一端面法线偏置 0 mm，targetDOF=1"}
	requirements := reviseRequirements(nil, first)
	var original GoalRequirement
	for _, r := range requirements {
		if r.Kind == "ASSEMBLY_FIT" {
			original = r
		}
	}
	if original.AxisDirectionRelation != "SAME" || original.PlaneDirectionRelation != "SAME" || original.OffsetMM == nil || *original.OffsetMM != 0 || original.TargetDOF == nil || *original.TargetDOF != 1 {
		t.Fatalf("source contract not preserved: %+v", original)
	}
	later := Turn{ID: uuid.NewString(), RunID: first.RunID, Text: "把偏置改成 -0.2 cm，目标自由度改为2"}
	requirements = reviseRequirements(requirements, later)
	for _, r := range requirements {
		if r.Kind == "ASSEMBLY_FIT" {
			if r.AxisDirectionRelation != "SAME" || r.PlaneDirectionRelation != "SAME" || r.OffsetMM == nil || *r.OffsetMM != -2 || r.TargetDOF == nil || *r.TargetDOF != 2 || r.Sources["axisDirectionRelation"] != first.ID || r.Sources["planeDirectionRelation"] != first.ID || r.Sources["offsetMM"] != later.ID || r.Sources["targetDOF"] != later.ID {
				t.Fatalf("revised values/source lost: %+v", r)
			}
		}
	}
	missing := reviseRequirements(nil, Turn{ID: uuid.NewString(), RunID: first.RunID, Text: "请装配轴孔"})
	for _, r := range missing {
		if r.Kind == "ASSEMBLY_FIT" && (r.AxisDirectionRelation != "" || r.PlaneDirectionRelation != "" || r.OffsetMM != nil || r.TargetDOF != nil) {
			t.Fatal("inferred unconfirmed goal")
		}
	}
}

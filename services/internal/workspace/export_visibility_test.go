package workspace

import "testing"

func TestExportVisibilityUsesOccurrenceAndSemanticAncestors(t *testing.T) {
	visible, hidden := true, false
	part := func(path string, mode string) DocumentStructureNode {
		return DocumentStructureNode{Kind: "PART", DocumentID: "part", InstancePath: &InstancePath{Canonical: path}, Children: []DocumentStructureNode{
			{Kind: "BODY", EntityID: "body", DocumentID: "part", InstancePath: &InstancePath{Canonical: path}, LocalVisible: &hidden, VisibilityMode: mode},
			{Kind: "GEOMETRICAL_SET", EntityID: "outer", DocumentID: "part", InstancePath: &InstancePath{Canonical: path}, LocalVisible: &hidden},
			{Kind: "GEOMETRICAL_SET", EntityID: "inner", OwnerEntityID: "outer", DocumentID: "part", InstancePath: &InstancePath{Canonical: path}, LocalVisible: &visible},
			{Kind: "FEATURE", EntityID: "sheet", OwnerEntityID: "inner", DocumentID: "part", InstancePath: &InstancePath{Canonical: path}, LocalVisible: &visible},
		}}
	}
	root := DocumentStructureNode{Kind: "PRODUCT", DocumentID: "root", Children: []DocumentStructureNode{
		{Kind: "INSTANCE", EntityID: "a", DocumentID: "part", Subject: &StructureEntityRef{DocumentID: "root", EntityID: "a"}, InstancePath: &InstancePath{Canonical: "a"}, Children: []DocumentStructureNode{part("a", "SHOW")}},
		{Kind: "INSTANCE", EntityID: "b", DocumentID: "part", Subject: &StructureEntityRef{DocumentID: "root", EntityID: "b"}, InstancePath: &InstancePath{Canonical: "b"}, VisibilityMode: "HIDE", Children: []DocumentStructureNode{part("b", "SHOW")}},
	}}
	v := newExportVisibility(&root)
	if !v.visible(exportAddress{"part", "a", "BODY", "body"}) {
		t.Fatal("occurrence SHOW did not override definition")
	}
	if v.visible(exportAddress{"part", "b", "BODY", "body"}) {
		t.Fatal("SHOW escaped hidden parent")
	}
	if v.visible(exportAddress{"part", "a", "FEATURE", "sheet"}) {
		t.Fatal("hidden geometrical-set ancestor ignored")
	}
	if !v.resultLocalVisible(exportAddress{"part", "b", "BODY", "body"}) {
		t.Fatal("hidden parent contaminated local state in ALL export")
	}
	if v.localVisible(v.instances["b"]) {
		t.Fatal("instance hiding lost")
	}
}

func TestHiddenFailedObjectsDoNotBlockSelectedExport(t *testing.T) {
	hidden := false
	tree := &DocumentStructureNode{Kind: "PART", DocumentID: "part", Children: []DocumentStructureNode{{Kind: "BODY", DocumentID: "part", EntityID: "valid"}, {Kind: "BODY", DocumentID: "part", EntityID: "failed", LocalVisible: &hidden}, {Kind: "FEATURE", DocumentID: "part", EntityID: "sheet", LocalVisible: &hidden}}}
	model := PartModel{Bodies: []PartBody{{ID: "valid", GeometryKey: "accepted"}, {ID: "failed"}}, Features: []Feature{{ID: "pad", BodyID: "failed", Type: "PAD", EvaluationStatus: "FAILED"}, {ID: "sheet", Type: "GEOMETRY", EvaluationStatus: "BLOCKED"}}}
	vis := newExportVisibility(tree)
	bodies, results, err := selectExchangePartContents(model, model.Bodies, "part", "", vis, true)
	if err != nil || len(bodies) != 1 || bodies[0].GeometryKey != "accepted" || len(results) != 0 {
		t.Fatal("excluded failure blocked valid selection", err)
	}
	if _, _, err = selectExchangePartContents(model, model.Bodies, "part", "", vis, false); err == nil {
		t.Fatal("ALL silently dropped selected failure")
	}
	tree.Children[2].LocalVisible = nil
	if _, _, err = selectExchangePartContents(model, model.Bodies, "part", "", newExportVisibility(tree), true); err == nil {
		t.Fatal("visible failed result was dropped")
	}
	model.Bodies[1].Consumed = true
	model.Features[1].Suppressed = true
	if bodies, _, err = selectExchangePartContents(model, model.Bodies, "part", "", newExportVisibility(tree), false); err != nil || len(bodies) != 1 {
		t.Fatal("consumed/suppressed content was included", err)
	}
}

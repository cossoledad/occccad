package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestReplacementInvalidatesOnlyDependentRelations(t *testing.T) {
	stale := 15.0
	old := ProductInstance{ID: "selected", ReferencedDocumentID: "part", ReferencedVersionID: "r1"}
	path := appendInstancePath(InstancePath{RootDocumentID: "root"}, InstancePathSegment{InstanceID: old.ID, ReferencedDocumentID: old.ReferencedDocumentID, ResolvedVersionID: old.ReferencedVersionID})
	model := ProductModel{Constraints: []AssemblyConstraint{
		{ID: "geometry", Kind: "DISTANCE", Mode: "MEASURED", MeasuredValue: &stale, First: AssemblyGeometryRef{InstanceID: old.ID, Kind: "PLANE", GeometryID: "XY"}, EvaluationStatus: modelcore.AssemblyConstraintVerified},
		{ID: "body", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: old.ID, Kind: "BODY", InstancePath: &path}, EvaluationStatus: modelcore.AssemblyConstraintVerified},
		{ID: "unrelated", First: AssemblyGeometryRef{InstanceID: "other", Kind: "BODY"}, EvaluationStatus: modelcore.AssemblyConstraintVerified}},
		ContextBindings: []ContextBinding{{ID: "source", SourceInstancePath: path}, {ID: "receiver", OwningInstancePath: path}, {ID: "unrelated", Name: "unchanged"}},
		Publications:    []ProductPublication{{ID: "forward", Target: ProductPublicationTarget{InstancePath: path}}}}
	untouched, _ := json.Marshal(model.Constraints[2])
	unrelatedBinding, _ := json.Marshal(model.ContextBindings[2])
	invalidateReplacedRelations(&model, "root", "r0", old, ProductInstance{ID: old.ID, ReferencedDocumentID: "new-part", ReferencedVersionID: "new-r1"})
	if model.Constraints[0].MeasuredValue != nil {
		t.Fatal("stale measured evidence survived replacement")
	}
	if model.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintBroken || model.Constraints[0].First.InstancePath.Segments[0].ReferencedDocumentID != "part" {
		t.Fatal("old source identity was lost", model.Constraints[0])
	}
	if model.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated {
		t.Fatal("whole occurrence must await update")
	}
	after, _ := json.Marshal(model.Constraints[2])
	afterBinding, _ := json.Marshal(model.ContextBindings[2])
	if !reflect.DeepEqual(untouched, after) || !reflect.DeepEqual(unrelatedBinding, afterBinding) {
		t.Fatal("unrelated state changed")
	}

	model.Instances = []ProductInstance{{ID: old.ID, ReferencedDocumentID: "new-part", ReferencedVersionID: "new-r1"}}
	if _, _, err := resolveAssemblyOccurrence(model, model.Constraints[1].First, func(ProductInstance) (ProductModel, error) {
		t.Fatal("direct occurrence traversed a child")
		return ProductModel{}, nil
	}); err != nil {
		t.Fatal("whole-instance reference kept the obsolete Part identity", err)
	}
	for _, b := range model.ContextBindings[:2] {
		if b.Accepted.Status != "BROKEN" {
			t.Fatal("binding falsely connected", b)
		}
	}
	if model.Publications[0].Resolution.Status != "BROKEN_PUBLICATION" {
		t.Fatal("forwarded interface falsely connected")
	}
	if model.ContextBindings[0].SourceInstancePath.Segments[0].ReferencedDocumentID != "part" {
		t.Fatal("binding guessed a new source")
	}
}

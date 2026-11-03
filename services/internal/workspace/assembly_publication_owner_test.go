package workspace

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestLocalPublicationDependencyAndProtectedDeletion(t *testing.T) {
	path := appendInstancePath(InstancePath{RootDocumentID: "root"}, InstancePathSegment{OwnerDocumentID: "root", OwnerVersionID: "r1", InstanceID: "a", ReferencedDocumentID: "part", ResolvedVersionID: "p1"})
	model := ProductModel{Instances: []ProductInstance{{ID: "a", Name: "A.1"}, {ID: "b", Name: "B.1"}}, Publications: []ProductPublication{{ID: "public-face", Name: "Mounting", Type: "SURFACE", Target: ProductPublicationTarget{InstancePath: path, PublicationID: "child-face"}}}, Constraints: []AssemblyConstraint{{ID: "gap", Kind: "DISTANCE", First: AssemblyGeometryRef{InstanceID: "a", Kind: "FACE", PublicationRef: &PublicationRef{Scope: "LOCAL", PublicationID: "public-face"}}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "PLANE"}, EvaluationStatus: modelcore.AssemblyConstraintVerified}}}
	graph, _, err := buildProductEvaluation(model, "r1", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, edge := range graph.Edges {
		if edge.Source == "product-publication:public-face" && edge.Target == "assembly-constraint:gap" {
			found = true
		}
	}
	if !found {
		t.Fatal("local public dependency absent")
	}
	raw, _ := json.Marshal(model)
	request, _ := json.Marshal(publicationDeletePayload{PublicationID: "public-face"})
	if _, _, err := applyDeleteProductPublication(raw, request); !errors.Is(err, ErrValidation) {
		t.Fatal("used public interface deleted", err)
	}
	// A child port with the same opaque ID is a different scoped address.
	model.Constraints[0].First.PublicationRef.Scope = "OCCURRENCE"
	raw, _ = json.Marshal(model)
	if _, _, err := applyDeleteProductPublication(raw, request); err != nil {
		t.Fatal("child interface incorrectly protected a root interface", err)
	}
}

func TestLocalPublicationRetargetInvalidatesOnlyItsConsumers(t *testing.T) {
	port := ProductPublication{ID: "mount", Name: "Mounting", Type: "PLANE", CompatibilityVersion: "1", Contract: PublicationContract{GeometryKind: "PLANE"}, Target: ProductPublicationTarget{PublicationID: "old-child"}}
	value := 7.0
	model := ProductModel{Publications: []ProductPublication{port}, Constraints: []AssemblyConstraint{
		{ID: "local", First: AssemblyGeometryRef{InstanceID: "a", PublicationRef: &PublicationRef{Scope: "LOCAL", PublicationID: port.ID}}, EvaluationStatus: modelcore.AssemblyConstraintVerified, MeasuredValue: &value},
		{ID: "child", First: AssemblyGeometryRef{InstanceID: "a", PublicationRef: &PublicationRef{PublicationID: port.ID}}, EvaluationStatus: modelcore.AssemblyConstraintVerified},
	}}
	raw, _ := json.Marshal(model)
	renamed, _ := json.Marshal(productPublicationEditPayload{PublicationID: port.ID, Name: "Renamed"})
	after, _, err := applyEditProductPublication(raw, renamed)
	if err != nil {
		t.Fatal(err)
	}
	var next ProductModel
	next = ProductModel{}
	_ = json.Unmarshal(after, &next)
	if next.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("rename invalidated numerical evidence")
	}
	port.Target.PublicationID = "new-child"
	target, _ := json.Marshal(productPublicationEditPayload{PublicationID: port.ID, Name: "Mounting", Target: &port})
	after, changes, err := applyEditProductPublication(raw, target)
	if err != nil {
		t.Fatal(err)
	}
	next = ProductModel{}
	_ = json.Unmarshal(after, &next)
	if next.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintNotUpdated || next.Constraints[0].MeasuredValue != nil || next.Constraints[1].EvaluationStatus != modelcore.AssemblyConstraintVerified || len(changes.Changes) != 2 {
		t.Fatal("retarget did not record exact dependent state", next, changes)
	}
	// The entity slots restore the public target and dependent status together.
	changes, err = reconcilePersistedChanges("PRODUCT", raw, after, changes)
	if err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PRODUCT", raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := applyModelValues("PRODUCT", after, values)
	if err != nil {
		t.Fatal(err)
	}
	next = ProductModel{}
	_ = json.Unmarshal(undo, &next)
	if next.Publications[0].Target.PublicationID != "old-child" || next.Constraints[0].EvaluationStatus != modelcore.AssemblyConstraintVerified {
		t.Fatal("retarget undo lost the public contract", next)
	}
}

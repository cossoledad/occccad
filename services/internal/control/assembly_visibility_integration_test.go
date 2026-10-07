package control

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestAssemblyConstraintSetVisibilityHistoryThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "sphere-r6")
	view, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Constraint display history"})
	if err != nil {
		t.Fatal(err)
	}
	id, sequence := view.Document.ID, 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID, request.RequestID = f.actor, fmt.Sprintf("%s-display-%d", id, sequence)
		view, err = f.service.ApplyCommand(t.Context(), id, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
	}
	for _, name := range []string{"First", "Second"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	for _, instance := range view.Product.Instances {
		apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: instance.ID, Kind: "BODY"}})
	}
	apply(workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT", TargetID: view.Product.Constraints[1].ID, Visible: false})
	baseline := *view.Product
	counts := func() (revisions, solves int) {
		t.Helper()
		if err := f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.document_versions WHERE document_id=$1`, id).Scan(&revisions); err != nil {
			t.Fatal(err)
		}
		if err := f.db.QueryRow(t.Context(), `SELECT count(*) FROM occccad.product_solve_manifests WHERE root_product_document_id=$1`, id).Scan(&solves); err != nil {
			t.Fatal(err)
		}
		return
	}
	revisions, solves := counts()
	for index, step := range []struct {
		request workspace.CommandRequest
		visible []bool
	}{
		{workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT_SET", TargetID: "assembly-constraints", Visible: false}, []bool{false, false}},
		{workspace.CommandRequest{Type: "UNDO"}, []bool{true, false}},
		{workspace.CommandRequest{Type: "REDO"}, []bool{false, false}},
		{workspace.CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "ASSEMBLY_CONSTRAINT_SET", TargetID: "assembly-constraints", Visible: true}, []bool{true, true}},
	} {
		apply(step.request)
		currentRevisions, currentSolves := counts()
		if currentRevisions != revisions+index+1 || currentSolves != solves {
			t.Fatalf("%s must create one Revision and reuse accepted geometry/solve: revisions=%d solves=%d, baseline=%d/%d", step.request.Type, currentRevisions, currentSolves, revisions, solves)
		}
		read, err := workspace.NewWithArtifacts(f.db, f.client, f.store).GetDocument(t.Context(), id, f.actor)
		if err != nil || read.Document.VersionID != view.Document.VersionID || !reflect.DeepEqual(read.Product.Instances, baseline.Instances) {
			t.Fatal("display changed occurrence poses or cold Head", err)
		}
		for i, constraint := range read.Product.Constraints {
			if (constraint.Visible == nil || *constraint.Visible) != step.visible[i] {
				t.Fatal("display history lost per-constraint visibility", constraint)
			}
			constraint.Visible = baseline.Constraints[i].Visible
			if !reflect.DeepEqual(constraint, baseline.Constraints[i]) {
				t.Fatal("display changed constraint definition/acceptance/activation")
			}
		}
		var group *workspace.DocumentStructureNode
		for i := range read.StructureTree.Children {
			if read.StructureTree.Children[i].Kind == "ASSEMBLY_CONSTRAINT_SET" {
				group = &read.StructureTree.Children[i]
			}
		}
		if group == nil || group.LocalVisible == nil || *group.LocalVisible != (step.visible[0] || step.visible[1]) {
			t.Fatal("constraint set display projection disagrees with children")
		}
	}
}

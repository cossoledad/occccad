package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"path/filepath"
	"testing"
)

func TestReferenceElementsCommandPreviewParametersReopenAndHistory(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "references.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, e := artifact.NewLocalStore(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	service := NewWithArtifacts(db, nil, artifact.NewService(db, store))
	view, err := service.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PART", Name: "Reference elements", ActorID: access.DefaultUserID})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	run := func(r CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = access.DefaultUserID
		r.VersionID = view.Document.VersionID
		r.RequestID = fmt.Sprintf("reference-workflow-%d", seq)
		if r.ReferenceConstruction != nil {
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, r)
			if e != nil || preview.PreviewID == "" || preview.ReferenceGeometry == nil {
				t.Fatal("reference preview", preview, e)
			}
			r.PreviewID = preview.PreviewID
		}
		next, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = next
	}
	run(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{1, 2, 3}}})
	p1 := view.Part.DatumPoints[0].ID
	run(CommandRequest{Type: "CREATE_REFERENCE_POINT", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{1, 2, 13}}})
	p2 := view.Part.DatumPoints[1].ID
	run(CommandRequest{Type: "CREATE_DATUM_AXIS", ReferenceConstruction: &ReferenceConstruction{Mode: "LINE_TWO_POINTS", Inputs: []DatumReference{{Kind: "POINT", EntityID: p1}, {Kind: "POINT", EntityID: p2}}}})
	_ = view.Part.DatumAxes[0].ID
	if view.Part.DatumAxes[0].Direction != [3]float64{0, 0, 1} {
		t.Fatal(view.Part.DatumAxes)
	}
	run(CommandRequest{Type: "CREATE_DATUM_PLANE", ReferenceConstruction: &ReferenceConstruction{Mode: "PLANE_PARALLEL_POINT", Inputs: []DatumReference{{Kind: "PLANE", EntityID: "datum-xy"}, {Kind: "POINT", EntityID: p2}}}})
	_ = view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
	if view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].Origin[2] != 13 {
		t.Fatal(view.Part.DatumPlanes)
	}
	parameterID := ""
	for _, p := range view.Part.Parameters {
		if p.OwnerFeatureID == p2 && p.PropertySlot == "reference:z" {
			parameterID = p.ParameterID
		}
	}
	run(CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: parameterID, Value: 23, Unit: "mm"})
	if view.Part.DatumPoints[1].Position[2] != 23 || view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].Origin[2] != 23 {
		t.Fatal("derived point/plane did not follow parameter", view.Part.DatumPoints, view.Part.DatumPlanes)
	}
	run(CommandRequest{Type: "UNDO"})
	if view.Part.DatumPoints[1].Position[2] != 13 {
		t.Fatal("reference parameter undo", view.Part.DatumPoints)
	}
	run(CommandRequest{Type: "REDO"})
	reopened, e := service.GetDocument(t.Context(), view.Document.ID, access.DefaultUserID)
	if e != nil || reopened.Part.DatumPoints[1].Position[2] != 23 {
		t.Fatal("reopen", e)
	}
	for _, child := range reopened.StructureTree.Children {
		if child.Kind == "ORIGIN" {
			t.Fatal("Origin wrapper remains")
		}
	}
	for _, id := range []string{p1, p2} {
		node := mustStructureNode(t, reopened.StructureTree.Children, "REFERENCE_POINT", id)
		if node.Kind == "REFERENCE_POINT" && len(node.Children) != 3 {
			t.Fatal("point parameter children", node)
		}
	}
	run(CommandRequest{Type: "DELETE_NODE", TargetKind: "REFERENCE_POINT", TargetID: p2})
	if len(view.Part.DatumPoints) != 1 || len(view.Part.RetiredReferences) == 0 {
		t.Fatal("referenced point was not retired")
	}
	run(CommandRequest{Type: "UNDO"})

	run(CommandRequest{Type: "EDIT_REFERENCE_POINT", TargetID: p2, Name: "Edited point", ReferenceConstruction: &ReferenceConstruction{Mode: "POINT_COORDINATES", Coordinates: [3]float64{1, 2, 33}}})
	if view.Part.DatumPoints[1].Position[2] != 33 {
		t.Fatal("literal point edit lost to old source", view.Part.DatumPoints)
	}
	run(CommandRequest{Type: "UNDO"})
	if view.Part.DatumPoints[1].Position[2] != 23 {
		t.Fatal("literal point edit undo", view.Part.DatumPoints)
	}
	product, err := service.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: "Reference assembly", ActorID: access.DefaultUserID})
	if err != nil {
		t.Fatal(err)
	}
	product, err = service.ApplyCommand(t.Context(), product.Document.ID, CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: view.Document.ID, ActorID: access.DefaultUserID, VersionID: product.Document.VersionID, RequestID: "reference-scope-insert"})
	if err != nil || len(product.ReferenceDisplayScopes) != 1 || len(product.ReferenceDisplayScopes[0].ReferenceGeometry.DatumPoints) != 2 || product.ReferenceDisplayScopes[0].VersionID != view.Document.VersionID {
		t.Fatal("empty Part reference display scope", product.ReferenceDisplayScopes, err)
	}
}

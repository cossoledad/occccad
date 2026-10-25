package control

import (
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestSketchProjectionDeletionThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Projection deletion"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		r.ActorID = p6Actor
		v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := v.Part.Features[0].ID
	edit := func(ops ...workspace.SketchOperation) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
	}
	edit(workspace.SketchOperation{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "projection", SourceVersionID: v.Document.VersionID, DatumReference: &workspace.DatumReference{Kind: "AXIS_SYSTEM", EntityID: v.Part.AxisSystems[0].ID, Axis: "X"}})
	value := 3.0
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "point", Kind: "POINT", Role: "PROFILE", Point: &workspace.SketchPoint2{X: 5, Y: 3}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "distance", Kind: "DISTANCE", Unit: "mm", Value: &value, References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: "projection", SubElement: "WHOLE"}}}})
	parameter := v.Part.Features[0].Sketch.Constraints[0].ParameterID
	apply(workspace.CommandRequest{Type: "DELETE_NODES", Targets: []workspace.DeleteNodeTarget{{TargetKind: "SKETCH_EXTERNAL_GEOMETRY", TargetID: "projection", OwnerEntityID: sketch}}})
	checkDeleted := func() {
		t.Helper()
		s := v.Part.Features[0].Sketch
		if len(s.ExternalGeometry) != 0 || len(s.Constraints) != 0 || len(s.Entities) != 1 {
			t.Fatal("delete scope", s)
		}
		for _, p := range v.Part.Parameters {
			if string(p.ParameterID) == parameter {
				t.Fatal("dimension parameter retained")
			}
		}
	}
	checkDeleted()
	apply(workspace.CommandRequest{Type: "UNDO"})
	s := v.Part.Features[0].Sketch
	if len(s.ExternalGeometry) != 1 || s.ExternalGeometry[0].ID != "projection" || s.Constraints[0].ParameterID != parameter {
		t.Fatal("undo identities", s)
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	checkDeleted()
	v, err = service.GetDocument(t.Context(), v.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	checkDeleted()
}
func TestSketchDimensionPlacementThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Dimension placement"})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		r.ActorID = p6Actor
		v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := v.Part.Features[0].ID
	length := 10.0
	edit := func(ops ...workspace.SketchOperation) {
		t.Helper()
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
	}
	edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 10}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "length", Kind: "LENGTH", Unit: "mm", Value: &length, LabelPosition: &workspace.SketchPoint2{X: 5, Y: -8}, References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "line", SubElement: "WHOLE"}}}})
	parameter := v.Part.Features[0].Sketch.Constraints[0].ParameterID
	before := *v.Part.Features[0].Sketch.Constraints[0].LabelPosition
	edit(workspace.SketchOperation{Type: "UPDATE_ENTITY_POINT", EntityID: "line", SubElement: "START", Point: &workspace.SketchPoint2{X: 20, Y: 30}}, workspace.SketchOperation{Type: "UPDATE_ENTITY_POINT", EntityID: "line", SubElement: "END", Point: &workspace.SketchPoint2{X: 30, Y: 30}})
	moved := *v.Part.Features[0].Sketch.Constraints[0].LabelPosition
	if math.Abs(moved.X-before.X-20) > 1e-7 || math.Abs(moved.Y-before.Y-30) > 1e-7 {
		t.Fatal(before, moved)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if *v.Part.Features[0].Sketch.Constraints[0].LabelPosition != before {
		t.Fatal("undo layout")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	s := v.Part.Features[0].Sketch
	if *s.Constraints[0].LabelPosition != moved || s.Constraints[0].ParameterID != parameter {
		t.Fatal("redo layout/parameter")
	}
}

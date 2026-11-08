//go:build integration

package control

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

// Formal dimensions remain supported after geometry creation. Drawing itself
// no longer exposes numeric inputs or silently creates driving dimensions.
func TestSketchDimensionsDriveSolidThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	value := func(v float64) *float64 { return &v }
	parameterText := func(v string) *string { return &v }
	ref := func(id, sub string) workspace.SketchGeometryRef {
		return workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	rectangle := []workspace.SketchOperation{}
	relations := []workspace.SketchOperation{}
	points := []workspace.SketchPoint2{{X: 0, Y: 0}, {X: 40, Y: 0}, {X: 40, Y: 20}, {X: 0, Y: 20}}
	for i := range points {
		id, next := fmt.Sprintf("rectangle-%d", i), fmt.Sprintf("rectangle-%d", (i+1)%4)
		a, b := points[i], points[(i+1)%4]
		kind := "HORIZONTAL"
		if i%2 == 1 {
			kind = "VERTICAL"
		}
		rectangle = append(rectangle, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "LINE", Role: "PROFILE", Start: &a, End: &b}})
		relations = append(relations, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "join-" + id, Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{ref(id, "END"), ref(next, "START")}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "direction-" + id, Kind: kind, References: []workspace.SketchGeometryRef{ref(id, "WHOLE")}}})
	}
	rectangle = append(rectangle, relations...)
	rectangle = append(rectangle, workspace.SketchOperation{Type: "ADD_CONSTRAINT", ParameterSource: parameterText("width"), Constraint: &workspace.SketchConstraint{ID: "width-dimension", Kind: "HORIZONTAL_DISTANCE", Value: value(40), Unit: "mm", References: []workspace.SketchGeometryRef{ref("rectangle-0", "START"), ref("rectangle-0", "END")}}}, workspace.SketchOperation{Type: "ADD_CONSTRAINT", ParameterSource: parameterText("20 mm"), Constraint: &workspace.SketchConstraint{ID: "height-dimension", Kind: "VERTICAL_DISTANCE", Value: value(20), Unit: "mm", References: []workspace.SketchGeometryRef{ref("rectangle-1", "START"), ref("rectangle-1", "END")}}})
	cases := map[string][]workspace.SketchOperation{
		"rectangle": rectangle,
		"circle":    {{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 10, Y: 10}, Radius: 2.5}}, {Type: "ADD_CONSTRAINT", ParameterSource: parameterText("width / 16"), Constraint: &workspace.SketchConstraint{ID: "radius-dimension", Kind: "RADIUS", Value: value(2.5), Unit: "mm", References: []workspace.SketchGeometryRef{ref("circle", "WHOLE")}}}},
		"line":      {{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 20}}}, {Type: "ADD_CONSTRAINT", ParameterSource: parameterText("width / 2"), Constraint: &workspace.SketchConstraint{ID: "length-dimension", Kind: "LENGTH", Value: value(20), Unit: "mm", References: []workspace.SketchGeometryRef{ref("line", "WHOLE")}}}},
	}
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Parametric drawing"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-drawing-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
		for _, f := range view.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatal(f.Type, f.Diagnostic)
			}
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "width", Value: 40, Unit: "mm"})
	widthID := view.Part.Parameters[0].ParameterID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[0].ID
	edit := func(ops []workspace.SketchOperation) {
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: ops})
	}
	for _, name := range []string{"rectangle", "circle", "line"} {
		request := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-preview-%s", view.Document.ID, name), Type: "EDIT_SKETCH", SketchID: sketchID, Operations: cases[name]}
		head := view.Document.VersionID
		preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
		if e != nil || preview.PreviewID == "" {
			t.Fatal(name, e)
		}
		read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
		if e != nil || read.Document.VersionID != head {
			t.Fatal("drawing preview changed Head", e)
		}
		edit(cases[name])
	}
	apply(workspace.CommandRequest{Type: "PAD_SKETCH", SketchID: sketchID, Length: 4})
	volume := func(w float64) {
		t.Helper()
		want := 4 * (w*20 - math.Pi*math.Pow(w/16, 2))
		if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
			t.Fatalf("volume=%g want=%g", activeBodyArtifact(t, view).Volume, want)
		}
	}
	volume(40)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: widthID, Value: 80, Unit: "mm"})
	volume(80)
	sketch := func() workspace.SketchFeature {
		for _, f := range view.Part.Features {
			if f.ID == sketchID {
				return *f.Sketch
			}
		}
		t.Fatal("missing sketch")
		return workspace.SketchFeature{}
	}
	var length workspace.SketchConstraint
	for _, c := range sketch().Constraints {
		if c.Kind == "LENGTH" {
			length = c
		}
	}
	if length.ParameterID == "" || math.Abs(*length.Value-40) > 1e-8 {
		t.Fatal("drawing formula lost", length)
	}
	var source any
	for _, p := range view.Part.Parameters {
		if p.ParameterID == length.ParameterID {
			source = p.Source
		}
	}
	oldReferences := append([]workspace.SketchGeometryRef(nil), length.References...)
	edit([]workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "other-line", Kind: "LINE", Role: "CONSTRUCTION", Start: &workspace.SketchPoint2{X: 2, Y: 2}, End: &workspace.SketchPoint2{X: 7, Y: 2}}}})
	length.References = []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "other-line", SubElement: "WHOLE"}}
	edit([]workspace.SketchOperation{{Type: "UPDATE_CONSTRAINT", ConstraintID: length.ID, Constraint: &length}})
	verify := func(refs []workspace.SketchGeometryRef) {
		t.Helper()
		for _, c := range sketch().Constraints {
			if c.ID == length.ID {
				if c.ParameterID != length.ParameterID || !reflect.DeepEqual(c.References, refs) {
					t.Fatal("replacement lost stable identity", c)
				}
			}
		}
		for _, p := range view.Part.Parameters {
			if p.ParameterID == length.ParameterID && !reflect.DeepEqual(p.Source, source) {
				t.Fatal("replacement rewrote source")
			}
		}
		volume(80)
	}
	verify(length.References)
	apply(workspace.CommandRequest{Type: "UNDO"})
	verify(oldReferences)
	apply(workspace.CommandRequest{Type: "REDO"})
	verify(length.References)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, err = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	verify(length.References)
	pad := view.Part.Features[len(view.Part.Features)-1]
	preview, err := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{ActorID: p6Actor, RequestID: view.Document.ID + "-cold-drawing", Type: "EDIT_FEATURE", TargetID: pad.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, pad.ID), Feature: &pad})
	if err != nil || preview.Artifact == nil {
		t.Fatal("cold rebuild", err)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if preview.Artifact.Representations[role].Digest != activeBodyArtifact(t, view).Representations[role].Digest {
			t.Fatal("cold differs", role)
		}
	}
}

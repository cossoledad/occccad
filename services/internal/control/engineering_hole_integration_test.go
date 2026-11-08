//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestEngineeringHoleGroupThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Engineering hole group"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-hole-group-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	saved := func(id string) workspace.Feature {
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing feature")
		return workspace.Feature{}
	}
	volume := func(n int, depth float64) {
		t.Helper()
		tip := 1 / math.Tan(math.Pi/3)
		want := 4000 - float64(n)*(depth*math.Pi+math.Pi*tip/3)
		if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
			t.Fatal("engineering hole volume", got, want)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "cutDepth", Value: 5, Unit: "mm"})
	depthID := view.Part.Parameters[0].ParameterID
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 15, Y: 5}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	positions := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: positions, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "a", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "b", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 10}}}}})
	f := workspace.Feature{Type: "HOLE", BodyID: view.Part.ActiveBodyID, Operation: "REMOVE", Extent: "BLIND", Diameter: 5, Depth: 5, DrillTipAngle: 120, DepthConvention: "CYLINDER", Thread: &workspace.HoleThreadDefinition{Mode: "ANNOTATION", Designation: "M3x0.5", Hand: "RIGHT", MajorDiameter: 3, Pitch: .5, Depth: 4}}
	for _, id := range []string{"a", "b"} {
		f.HolePoints = append(f.HolePoints, workspace.PatternPointReference{SketchID: positions, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}})
	}
	req := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-hole-group-preview", Feature: &f, ParameterExpressions: map[string]string{"diameter": "4 mm / 2", "depth": "cutDepth", "drillTipAngle": "120 deg", "thread:pitch": "0.5 mm"}}
	head := view.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, req)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("preview advanced head", e)
	}
	// Use the same preview request ID on commit.
	req.PreviewID = preview.PreviewID
	view, err = service.ApplyCommand(t.Context(), view.Document.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	f = view.Part.Features[len(view.Part.Features)-1]
	volume(2, 5)
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: depthID, Value: 6, Unit: "mm"})
	volume(2, 6)
	apply(workspace.CommandRequest{Type: "UNDO"})
	volume(2, 5)
	apply(workspace.CommandRequest{Type: "REDO"})
	volume(2, 6)
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: positions, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"b"}}}})
	f = saved(f.ID)
	if f.EvaluationStatus != "FAILED" || len(f.HolePoints) != 2 {
		t.Fatal("lost member not retained", f)
	}
	_, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-hole-group-invalid", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e == nil {
		t.Fatal("missing member repair accepted")
	}
	f.HolePoints = f.HolePoints[:1]
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	volume(1, 6)
	final := activeBodyArtifact(t, view)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	volume(1, 6)
	f = saved(f.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-hole-group-cold", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold identity", role)
		}
	}
}

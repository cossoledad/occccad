//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestLoftBoundarySupportsThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Loft continuity acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-loft-boundary-%d", view.Document.ID, seq)
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
	sketch := func(z, r float64) string {
		t.Helper()
		apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: r}}}})
		return id
	}
	side := func() workspace.FeatureSelection {
		p := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
			return p.GeometryType == "CYLINDER"
		})
		return workspace.FeatureSelection{Selection: p.Selection, SourceVersionID: p.SourceVersionID}
	}
	apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "influence", Value: 1, Unit: "1"})
	influence := view.Part.Parameters[0].ParameterID
	base := sketch(0, 10)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: base, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
	start := side()
	lower := sketch(10, 10)
	upper := sketch(30, 5)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: upper, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 10})
	endFeature := view.Part.Features[len(view.Part.Features)-1].ID
	end := side()
	spareSketch := sketch(30, 5)
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: spareSketch, Generator: "LINEAR_EXTRUDE", Operation: "NEW_BODY", Length: 10})
	spare := side()
	f := workspace.Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: []workspace.LoftSection{{SketchID: lower}, {SketchID: upper}}, LoftStart: &workspace.LoftBoundaryDefinition{Condition: "G2", Magnitude: 1, Supports: []workspace.FeatureSelection{start}}, LoftEnd: &workspace.LoftBoundaryDefinition{Condition: "G2", Magnitude: 1, Supports: []workspace.FeatureSelection{end}}}
	req := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-loft-preview", Feature: &f, ParameterExpressions: map[string]string{"loft:startMagnitude": "influence"}}
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, req)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	req.PreviewID = preview.PreviewID
	view, err = service.ApplyCommand(t.Context(), view.Document.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	f = view.Part.Features[len(view.Part.Features)-1]
	want := 20 * math.Pi * (50 + 25*181./462)
	if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
		t.Fatal("exact quintic loft volume", got, want)
	}
	inputs, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{FeatureID: f.ID, VersionID: view.Document.VersionID})
	if e != nil || len(inputs.LoftStartPicks) != 1 || inputs.LoftStartPicks[0].Status != modelcore.SelectionResolved || len(inputs.LoftEndPicks) != 1 || inputs.LoftEndPicks[0].Status != modelcore.SelectionResolved {
		t.Fatal("formal boundary inputs", inputs.LoftStartPicks, inputs.LoftEndPicks, e)
	}
	apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: influence, Value: .8, Unit: "1"})
	if math.Abs(activeBodyArtifact(t, view).Volume-want) < 1e-3 {
		t.Fatal("magnitude did not change the loft")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
		t.Fatal("magnitude undo")
	}
	apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: endFeature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, endFeature), Suppressed: boolPointer(true)})
	f = saved(f.ID)
	if f.EvaluationStatus != "FAILED" {
		t.Fatal("lost support silently rebound to identical spare", f)
	}
	boundary := *f.LoftEnd
	f.LoftEnd = &boundary
	f.LoftEnd.Supports = []workspace.FeatureSelection{spare}
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if math.Abs(activeBodyArtifact(t, view).Volume-want) > 1e-5 {
		t.Fatal("explicit repair")
	}
	final := activeBodyArtifact(t, view)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	f = saved(f.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-loft-cold", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold identity", role)
		}
	}
}

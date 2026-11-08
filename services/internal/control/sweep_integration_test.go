//go:build integration

package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
	"testing"
)

func TestSweepFormalPathThroughRouter(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Sweep path acceptance"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-sweep-%d", view.Document.ID, seq)
		}
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	volume := func(want float64) {
		t.Helper()
		if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
			t.Fatal("sweep volume", got, want)
		}
	}
	saved := func(id string) workspace.Feature {
		for _, f := range view.Part.Features {
			if f.ID == id {
				return f
			}
		}
		t.Fatal("missing feature", id)
		return workspace.Feature{}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"})
	profile := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "section", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 1}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	path := view.Part.Features[len(view.Part.Features)-1].ID
	a := workspace.SketchEntity{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 10}}
	b := workspace.SketchEntity{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 10}, End: &workspace.SketchPoint2{X: 10, Y: 10}}
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: path, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &a}, {Type: "ADD_ENTITY", Entity: &b}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "join", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}}}}})
	f := workspace.Feature{Type: "SWEEP", BodyID: view.Part.ActiveBodyID, Profile: profile, Operation: "ADD", Sweep: &workspace.SweepDefinition{Path: workspace.CurveChainReference{SketchID: path, EntityIDs: []string{"b", "a"}}, FrameMode: "CORRECTED_FRENET", CornerMode: "MITER"}}
	req := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-sweep-preview", Feature: &f}
	head := view.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, req)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("preview advanced head", e)
	}
	req.PreviewID = preview.PreviewID
	apply(req)
	volume(20 * math.Pi)
	f = view.Part.Features[len(view.Part.Features)-1]
	if f.Sweep.Path.Direction == nil || f.Sweep.Path.Direction.EntityID != "a" {
		t.Fatal("direction not frozen", f.Sweep)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	volume(20 * math.Pi)
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: path, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"b"}}}})
	f = saved(f.ID)
	if f.EvaluationStatus != "FAILED" || !strings.Contains(f.Diagnostic, "CURVE_CHAIN_SOURCE_MISSING") {
		t.Fatal("missing path not retained", f)
	}
	_, e = service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-sweep-invalid", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e == nil {
		t.Fatal("unresolved repair accepted")
	}
	def := *f.Sweep
	f.Sweep = &def
	f.Sweep.Path.EntityIDs = []string{"a"}
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	volume(10 * math.Pi)
	final := activeBodyArtifact(t, view)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	volume(10 * math.Pi)
	f = saved(f.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-sweep-cold", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold identity", role)
		}
	}
}

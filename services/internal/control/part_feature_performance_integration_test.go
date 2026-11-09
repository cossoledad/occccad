//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/workspace"
)

// Uses real Domain Commands, configured storage and the production Router.
// Execution counts are the regression contract; wall-clock capacity is not.
func TestPartFeatureChainIncrementalAndColdThroughRouter(t *testing.T) {
	service, _, db, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Part incremental feature chain"})
	if err != nil {
		t.Fatal(err)
	}
	sequence := 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID = p6Actor
		request.RequestID = fmt.Sprintf("%s-part-chain-%d", view.Document.ID, sequence)
		request.VersionID = view.Document.VersionID
		var e error
		view, e = service.ApplyCommand(ctx, view.Document.ID, request)
		if e != nil {
			t.Fatalf("%s: %v", request.Type, e)
		}
		for _, feature := range view.Part.Features {
			if feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED" {
				t.Fatalf("%s: %s", feature.ID, feature.Diagnostic)
			}
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 100, Y: 100}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	var cuts []string
	for i := 0; i < 8; i++ {
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 5 + 10*float64(i), Y: 5}, Radius: 1}}}})
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: id, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 20})
		cuts = append(cuts, view.Part.Features[len(view.Part.Features)-1].ID)
	}
	checkVolume := func(want float64) {
		t.Helper()
		a := activeBodyArtifact(t, view)
		if math.Abs(a.Volume-want) > 1e-5 || !a.Naming.CanBind {
			t.Fatalf("volume/naming: %g != %g, %+v", a.Volume, want, a.Naming)
		}
	}
	checkRuns := func(minReused uint32, maxExecuted uint32) {
		t.Helper()
		var raw []byte
		if e := db.QueryRow(ctx, `SELECT evaluation_manifest FROM occccad.document_versions WHERE id=$1`, view.Document.VersionID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var m struct {
			Runtime struct {
				WorkerRuns []*workerv1.PartRuntimeStats `json:"workerRuns"`
			} `json:"runtime"`
		}
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		found := false
		for _, r := range m.Runtime.WorkerRuns {
			if r.GeneratorCalls > 0 {
				found = true
				if r.StagesReused < minReused || r.StagesExecuted > maxExecuted {
					t.Fatalf("unrelated prefix recomputed: %+v", r)
				}
			}
		}
		if !found {
			t.Fatal("missing real Worker execution evidence")
		}
	}
	checkVolume(200000 - 8*20*math.Pi)
	checkRuns(8, 1)
	var edited workspace.Feature
	for _, f := range view.Part.Features {
		if f.ID == cuts[2] {
			edited = f
		}
	}
	edited.Length = 15
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: edited.ID, Feature: &edited, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID)})
	checkVolume(200000 - (7*20+15)*math.Pi)
	checkRuns(3, 6)
	accepted := activeBodyArtifact(t, view)
	head := view.Document.VersionID
	cold, e := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-part-chain-cold", TargetID: edited.ID, Feature: &edited, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, edited.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatalf("cold replay: %v", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if accepted.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatalf("cold %s differs from incrementally evaluated result", role)
		}
	}
	if cold.Runtime == nil {
		t.Fatal("missing cold runtime")
	}
	for _, run := range cold.Runtime.WorkerRuns {
		if run.GeneratorCalls > 0 && run.StagesReused != 0 {
			t.Fatal("cold path borrowed a stage")
		}
	}
	reopened, e := service.GetDocument(ctx, view.Document.ID, p6Actor)
	if e != nil || reopened.Document.VersionID != head {
		t.Fatalf("preview changed Head: %v", e)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	checkVolume(200000 - 8*20*math.Pi)
	apply(workspace.CommandRequest{Type: "REDO"})
	checkVolume(200000 - (7*20+15)*math.Pi)
}

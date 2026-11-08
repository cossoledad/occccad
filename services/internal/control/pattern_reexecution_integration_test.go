//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPatternReexecutionPerMemberTerminationRepairAndIdentity(t *testing.T) {
	service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Pattern generator replay"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-reexecution-%d", view.Document.ID, seq)
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
	volume := func(removed float64) {
		t.Helper()
		if got := activeBodyArtifact(t, view).Volume; math.Abs(got-(6000-removed*math.Pi)) > 1e-5 {
			t.Fatal("per-position termination volume", got, 6000-removed*math.Pi)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	base := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: base, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 30, Y: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: base, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 2}, Normal: [3]float64{-1. / 3, 0, 1}, UDirection: [3]float64{0, 1, 0}})
	end := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 5, Y: 5}, Radius: 1}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: "TO_PLANE", TerminationPlaneID: end.ID, Reversed: true})
	seed := view.Part.Features[len(view.Part.Features)-1]
	if cap := view.ReplicationSources[seed.ID]; !cap.Reexecution || cap.Tool || cap.SourceKind != "FEATURE_REEXECUTION" {
		t.Fatal("reexecution not advertised", cap)
	}
	f := workspace.Feature{Type: "SOLID_PATTERN", BodyID: seed.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: seed.BodyID, FeatureID: seed.ID}, SourceKind: "AUTO", PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 10, Direction: [3]float64{1, 0, 0}}}}
	req := workspace.CommandRequest{Type: "CREATE_PATTERN", ActorID: p6Actor, RequestID: view.Document.ID + "-reexecute-preview", Feature: &f}
	head := view.Document.VersionID
	preview, e := service.PreviewCommand(t.Context(), view.Document.ID, req)
	if e != nil || preview.Artifact == nil {
		t.Fatal("preview", e)
	}
	calls := uint32(0)
	if preview.Runtime != nil {
		for _, r := range preview.Runtime.WorkerRuns {
			calls += r.GeneratorCalls
		}
	}
	if calls < 2 {
		t.Fatal("reexecution has no generator evidence", preview.Runtime)
	}
	read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || read.Document.VersionID != head {
		t.Fatal("preview advanced head", e)
	}
	req.PreviewID = preview.PreviewID
	view, err = service.ApplyCommand(t.Context(), view.Document.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	f = view.Part.Features[len(view.Part.Features)-1]
	volume(39)
	if f.Pattern.SourceKind != "FEATURE_REEXECUTION" || f.Operation != "REMOVE" {
		t.Fatal("AUTO did not resolve to the real recipe", f.Pattern)
	}
	pick := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return p.GeometryType == "CYLINDER" && s.Anchor.FeatureID == f.ID && strings.HasPrefix(s.Anchor.OutputSlot, "MEMBER/1/")
	})
	apply(workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: end.ID, Origin: [3]float64{0, 0, 3}, Normal: end.Normal, UDirection: end.UDirection})
	volume(36)
	resolution, e := service.ResolvePersistentSelection(t.Context(), view.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
	if e != nil || resolution.Status != modelcore.SelectionResolved {
		t.Fatal("member identity lost after target move", resolution, e)
	}
	f = saved(f.ID)
	pattern := *f.Pattern
	f.Pattern = &pattern
	f.Pattern.Spacing = 20
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	f = saved(f.ID)
	if f.EvaluationStatus != "FAILED" || !strings.Contains(f.Diagnostic, "PATTERN_MEMBER_FAILED:2:") {
		t.Fatal("unproductive member not diagnosed", f)
	}
	pattern = *f.Pattern
	f.Pattern = &pattern
	f.Pattern.SkippedSlots = []int{2}
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	volume(24)
	apply(workspace.CommandRequest{Type: "UNDO"})
	if saved(f.ID).EvaluationStatus != "FAILED" {
		t.Fatal("undo lost failed definition")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	volume(24)
	final := activeBodyArtifact(t, view)
	fresh := workspace.NewWithArtifacts(db, client, artifacts)
	view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	f = saved(f.ID)
	cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-reexecute-cold", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)})
	if e != nil || cold.Artifact == nil {
		t.Fatal("cold", e)
	}
	for _, role := range []string{"BREP", "NAMING"} {
		if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
			t.Fatal("cold identity", role)
		}
	}
	// Small, explicit samples: these are phase measurements, never capacity/P95
	// claims. Every sample changes an actual pattern parameter and checks volume.
	for _, mode := range []string{"hot", "cold"} {
		for i := 1; i <= 5; i++ {
			candidate := f
			p := *f.Pattern
			candidate.Pattern = &p
			delta := float64(i) * .05
			p.Spacing = 20 + delta
			r := workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: fmt.Sprintf("%s-measure-%s-%d", view.Document.ID, mode, i), TargetID: f.ID, Feature: &candidate, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID)}
			ctx, recorder := perf.WithRecorder(t.Context())
			if mode == "cold" {
				ctx = workspace.WithColdEvaluation(ctx)
			}
			started := time.Now()
			result, e := fresh.PreviewCommand(ctx, view.Document.ID, r)
			elapsed := float64(time.Since(started)) / float64(time.Millisecond)
			if e != nil || result.Artifact == nil {
				t.Fatal("measurement preview", mode, i, e)
			}
			want := 6000 - (24-delta/3)*math.Pi
			if math.Abs(result.Artifact.Volume-want) > 1e-5 {
				t.Fatal("measurement changed precision", mode, i, result.Artifact.Volume, want)
			}
			phases := recorder.SnapshotMilliseconds()
			if mode == "hot" && i > 1 && phases["sketch-rpc"] != 0 {
				t.Fatal("unchanged sketches repeated native diagnostics", phases)
			}
			native := map[string]float64{}
			var generators, reused, executed uint32
			if result.Runtime != nil {
				for _, run := range result.Runtime.WorkerRuns {
					native["queueMs"] += run.QueueMs
					native["exactMs"] += run.ExactMs
					native["namingMs"] += run.NamingMs
					native["displayMs"] += run.DisplayMs
					native["inputPrepareMs"] += run.InputPrepareMs
					native["artifactIOMs"] += run.ArtifactIoMs
					generators += run.GeneratorCalls
					reused += run.StagesReused
					executed += run.StagesExecuted
				}
			}
			raw, _ := json.Marshal(map[string]any{"mode": mode, "sample": i, "serviceTotalMs": elapsed, "phasesMs": phases, "workerMs": native, "generatorCalls": generators, "stagesReused": reused, "stagesExecuted": executed, "profileEntities": 5, "activeMembers": 2})
			t.Log("CAD_WORKFLOW_SAMPLE", string(raw))
		}
	}
	current, e := fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil || current.Document.VersionID != view.Document.VersionID {
		t.Fatal("measurement advanced head", e)
	}

}

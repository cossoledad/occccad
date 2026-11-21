//go:build integration

package control

import (
	"fmt"
	"math"
	"testing"
	"time"

	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
)

// Ten positioning points form one engineering Hole feature. A single linear
// pattern repeats that row on the same plate, including the original row.
func TestPlateHoleArrayThroughRouter(t *testing.T) {
	for _, holes := range []int{100, 500, 1000} {
		t.Run(fmt.Sprint(holes), func(t *testing.T) {
			service, _, _, _, _ := featureAssociationRuntimeService(t)
			ctx := t.Context()
			view, err := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: fmt.Sprintf("Plate with %d holes", holes)})
			if err != nil {
				t.Fatal(err)
			}
			request := func(r workspace.CommandRequest) workspace.CommandRequest {
				r.ActorID, r.VersionID = p6Actor, view.Document.VersionID
				return r
			}
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				view, err = service.ApplyCommand(ctx, view.Document.ID, request(r))
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			const width, height, thickness = 610., 60., 6.
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := view.Part.Features[0].ID
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: width, Y: height}}}})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: thickness})
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			positions := view.Part.Features[len(view.Part.Features)-1].ID
			body := view.Part.ActiveBodyID
			hole := workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "BLIND", HoleKind: "SIMPLE", Diameter: 2, Depth: 8}
			var ops []workspace.SketchOperation
			for i := 0; i < 10; i++ {
				id := fmt.Sprintf("position-%d", i)
				ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 3, Y: 3 + 6*float64(i)}}})
				hole.HolePoints = append(hole.HolePoints, workspace.PatternPointReference{SketchID: positions, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}})
			}
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: positions, Operations: ops})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &hole})
			seed := view.Part.Features[len(view.Part.Features)-1]
			pattern := workspace.Feature{
				Type: "SOLID_PATTERN", BodyID: body, Operation: "REMOVE",
				Pattern: &workspace.FeaturePattern{
					Source: workspace.FeatureStageRef{BodyID: body, FeatureID: seed.ID}, SourceKind: "GENERATOR_TOOL",
					PatternDefinition: workspace.PatternDefinition{
						Kind: "LINEAR", Distribution: "FIXED_STEP", Count: float64(holes / 10), Spacing: 6, Direction: [3]float64{1, 0, 0},
					},
				},
			}
			verify := func(a *workspace.Artifact) {
				t.Helper()
				if got := p6TopologyCount(t, a.Topology, "faces"); got != uint64(6+holes) {
					t.Fatalf("faces=%d want %d", got, 6+holes)
				}
				want := width*height*thickness - float64(holes)*math.Pi*thickness
				if got := measuredArtifactVolume(t, a); math.Abs(got-want) > 1e-4 {
					t.Fatalf("material volume=%g want %g", got, want)
				}
			}
			var previous string
			for _, phase := range []string{"create", "edit-spacing"} {
				r := request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &pattern, RequestID: view.Document.ID + "-" + phase})
				if phase == "edit-spacing" {
					pattern = view.Part.Features[len(view.Part.Features)-1]
					p := *pattern.Pattern
					p.Spacing = 6.1
					pattern.Pattern = &p
					r.Type, r.TargetID, r.ExpectedFeatureDigest = "EDIT_FEATURE", pattern.ID, findP6FeatureDigest(t, view.StructureTree, pattern.ID)
				}
				preview := engineeringFeaturePreview(t, service, view.Document.ID, r, phase)
				if preview.Runtime == nil || len(preview.Runtime.WorkerRuns) != 1 {
					t.Fatalf("%s requires a fresh Worker evaluation", phase)
				}
				rep := preview.Artifact.Representations
				if rep["BREP"].Digest == previous {
					t.Fatal("spacing edit did not move the holes")
				}
				previous = rep["BREP"].Digest
				current, err := service.GetDocument(ctx, view.Document.ID, p6Actor)
				if err != nil || current.Document.VersionID != r.VersionID {
					t.Fatal("preview changed the base revision", err)
				}
				r.PreviewID = preview.PreviewID
				timed, timings := perf.WithRecorder(ctx)
				start := time.Now()
				view, err = service.ApplyCommand(timed, view.Document.ID, r)
				commitMS := float64(time.Since(start)) / float64(time.Millisecond)
				if err != nil {
					t.Fatal("promote pattern", err)
				}
				if _, repeated := timings.SnapshotMilliseconds()["part-rpc"]; repeated {
					t.Fatal("promotion repeated geometry evaluation")
				}
				accepted := activeBodyArtifact(t, view)
				for _, role := range []string{"BREP", "NAMING", "VISUAL"} {
					if accepted.Representations[role].Digest != rep[role].Digest {
						t.Fatal("promotion changed", role)
					}
				}
				verify(&accepted)
				run := preview.Runtime.WorkerRuns[0]
				t.Logf("plate holes=%d phase=%s commit_ms=%.3f exact_ms=%.3f boolean_ms=%.3f witness_ms=%.3f naming_ms=%.3f display_ms=%.3f brep=%s", holes, phase, commitMS, run.ExactMs, run.BooleanBuildMs, run.MaterialWitnessMs, run.NamingKernelMs+run.NamingArtifactMs, run.DisplayMs, previous)
			}
			// Replay the accepted definition without any prefix or witness reuse.
			tip := view.Part.Features[len(view.Part.Features)-1]
			cold, err := service.PreviewCommand(workspace.WithColdEvaluation(ctx), view.Document.ID,
				request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: tip.ID, Feature: &tip,
					ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, tip.ID)}))
			if err != nil || cold.Artifact == nil || cold.Runtime == nil || len(cold.Runtime.WorkerRuns) == 0 {
				t.Fatal("cold pattern evaluation", err)
			}
			accepted := activeBodyArtifact(t, view)
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != accepted.Representations[role].Digest {
					t.Fatal("cold pattern mismatch", role)
				}
			}
		})
	}
}

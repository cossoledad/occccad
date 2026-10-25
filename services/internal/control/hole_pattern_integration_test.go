package control

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func engineeringHoleVolume(f workspace.Feature) float64 {
	r, R := f.Diameter/2, f.RecessDiameter/2
	volume := math.Pi * r * r * f.Depth
	if f.HoleKind == "COUNTERBORE" {
		volume += math.Pi * (R*R - r*r) * f.RecessDepth
	}
	if f.HoleKind == "COUNTERSINK" {
		h := (R - r) / math.Tan(f.CountersinkAngle*math.Pi/360)
		volume += math.Pi * h * ((R*R+R*r+r*r)/3 - r*r)
	}
	return volume
}

func TestBlindEngineeringHoleReplicationThroughRouter(t *testing.T) {
	for _, kind := range []string{"SIMPLE", "COUNTERBORE", "COUNTERSINK"} {
		t.Run(kind, func(t *testing.T) {
			service, store, db, client, _ := featureAssociationRuntimeService(t)
			v, e := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Finite Hole replication " + kind})
			if e != nil {
				t.Fatal(e)
			}
			seq := 0
			request := func(r workspace.CommandRequest) workspace.CommandRequest {
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-hole-array-%d", v.Document.ID, seq)
				}
				return r
			}
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				var err error
				v, err = service.ApplyCommand(t.Context(), v.Document.ID, request(r))
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range v.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("missing", id)
				return workspace.Feature{}
			}
			editRequest := func(f workspace.Feature) workspace.CommandRequest {
				return request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, v.StructureTree, f.ID)})
			}
			edit := func(f workspace.Feature) { t.Helper(); apply(editRequest(f)) }
			preview := func(r workspace.CommandRequest) workspace.CommandPreview {
				t.Helper()
				head := v.Document.VersionID
				p, err := service.PreviewCommand(t.Context(), v.Document.ID, r)
				if err != nil || p.Artifact == nil {
					t.Fatal("preview", r.Type, err)
				}
				unchanged, err := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
				if err != nil || unchanged.Document.VersionID != head {
					t.Fatal("preview/cancel wrote Head", err)
				}
				return p
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := v.Part.Features[0].ID
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -40, Y: -40}, Second: &workspace.SketchPoint2{X: 40, Y: 40}}}})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 20})
			body := v.Part.ActiveBodyID
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 20}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: v.Part.DatumPlanes[len(v.Part.DatumPlanes)-1].ID})
			location := v.Part.Features[len(v.Part.Features)-1].ID
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: -12, Y: 8}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "q", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: -24, Y: -15}}}}})
			holeDefinition := func(point string) workspace.Feature {
				return workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "BLIND", HoleKind: kind, Diameter: 2, Depth: 5, RecessDiameter: 4, RecessDepth: 1.5, CountersinkAngle: 90, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: location, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: point, SubElement: "POINT"}}}
			}
			hole := holeDefinition("p")
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &hole})
			hole = v.Part.Features[len(v.Part.Features)-1]
			unit := engineeringHoleVolume(hole)
			definition := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: hole.ID}, SourceKind: "AUTO", PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 12, Direction: [3]float64{1, 0, 0}}}}
			linearRequest := request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &definition})
			hot := preview(linearRequest)
			var reused, executed, generators uint32
			if hot.Runtime == nil {
				t.Fatal("runtime evidence missing")
			}
			for _, run := range hot.Runtime.WorkerRuns {
				reused += run.StagesReused
				executed += run.StagesExecuted
				generators += run.GeneratorCalls
			}
			if reused < 2 || executed != 1 || generators != 0 {
				t.Fatal("append failed to reuse unconsumed Hole tool checkpoint", reused, executed, generators)
			}
			if math.Abs(hot.Artifact.Volume-(128000-3*unit)) > 1e-5 {
				t.Fatal("linear volume", hot.Artifact.Volume)
			}
			for _, mode := range []string{"CIRCULAR", "MIRROR"} {
				f := definition
				p := *definition.Pattern
				f.Pattern = &p
				p.Kind = mode
				count := 3.
				if mode == "CIRCULAR" {
					p.Distribution = "FULL_CIRCLE"
					p.AxisEntityID = "AXIS_SYSTEM:" + v.Part.AxisSystems[0].ID + ":Z"
					p.Direction = [3]float64{0, 0, 1}
				} else {
					p.Count = 2
					p.Spacing = 0
					p.MirrorPlaneID = "datum-yz"
					count = 2
				}
				candidate := preview(request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &f}))
				if math.Abs(candidate.Artifact.Volume-(128000-count*unit)) > 1e-5 {
					t.Fatal(mode, candidate.Artifact.Volume)
				}
			}
			linearRequest.PreviewID = hot.PreviewID
			apply(linearRequest)
			pattern := v.Part.Features[len(v.Part.Features)-1]
			if pattern.Operation != "REMOVE" || pattern.Pattern.SourceKind != "GENERATOR_TOOL" {
				t.Fatal("AUTO did not preserve signed tool", pattern)
			}
			pickMember := func(stage string, slot int) workspace.FeatureSelection {
				t.Helper()
				input, err := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: stage})
				if err != nil {
					t.Fatal(err)
				}
				for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "edges"); local++ {
					r, err := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: stage, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
					if err != nil {
						t.Fatal(err)
					}
					if r.Selection != nil {
						s := r.Selection.Selection
						if s.Anchor.FeatureID == pattern.ID && strings.HasPrefix(s.Anchor.OutputSlot, fmt.Sprintf("MEMBER/%d/", slot)) && math.Abs(s.CreationEvidence.Centroid[2]-20) < 1e-6 && s.CreationEvidence.GeometryType == "CIRCLE" {
							return *r.Selection
						}
					}
				}
				t.Fatal("member mouth unavailable", slot)
				return workspace.FeatureSelection{}
			}
			mouth := findP7TopologyPick(t, service, v, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return s.Anchor.FeatureID == pattern.ID && strings.HasPrefix(s.Anchor.OutputSlot, "MEMBER/2/") && math.Abs(s.CreationEvidence.Centroid[2]-20) < 1e-6 && s.CreationEvidence.GeometryType == "CIRCLE"
			})
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: .1, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID, SourceFeatureID: pattern.ID}}}})
			child := v.Part.Features[len(v.Part.Features)-1]
			second := holeDefinition("q")
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &second})
			second = v.Part.Features[len(v.Part.Features)-1]
			repair := func(slot int) {
				t.Helper()
				f := saved(child.ID)
				f.Selections = []workspace.FeatureSelection{pickMember(child.ID, slot)}
				r := editRequest(f)
				p := preview(r)
				r.PreviewID = p.PreviewID
				apply(r)
				if saved(child.ID).EvaluationStatus != "" {
					t.Fatal("chamfer repair failed")
				}
			}
			invalidate := func(f workspace.Feature, expected float64) {
				t.Helper()
				r := editRequest(f)
				p := preview(r)
				if len(p.RepairRequired) != 1 || p.RepairRequired[0].FeatureID != child.ID || math.Abs(p.Artifact.Volume-expected) > 1e-5 {
					t.Fatal("pattern-stage repair preview", p.RepairRequired, p.Artifact.Volume)
				}
				invalid := f
				copyPattern := *f.Pattern
				copyPattern.Count = 0
				invalid.Pattern = &copyPattern
				bad := editRequest(invalid)
				bad.PreviewID = p.PreviewID
				if _, err := service.ApplyCommand(t.Context(), v.Document.ID, bad); err == nil {
					t.Fatal("invalid/current mismatch committed")
				}
				r.PreviewID = p.PreviewID
				apply(r)
				if saved(child.ID).EvaluationStatus != "FAILED" || saved(pattern.ID).EvaluationStatus != "" || saved(second.ID).EvaluationStatus != "BLOCKED" {
					t.Fatal("wrong failure propagation")
				}
			}
			changed := saved(pattern.ID)
			p := *changed.Pattern
			p.Count = 2
			changed.Pattern = &p
			invalidate(changed, 128000-2*unit)
			// The target itself must be valid even when an existing downstream input is broken.
			invalidTarget := saved(pattern.ID)
			invalidPattern := *invalidTarget.Pattern
			invalidPattern.Spacing = 1000
			invalidTarget.Pattern = &invalidPattern
			if _, err := service.PreviewCommand(t.Context(), v.Document.ID, editRequest(invalidTarget)); err == nil {
				t.Fatal("invalid target was accepted as a downstream repair preview")
			}
			repair(1)
			changed = saved(pattern.ID)
			p = *changed.Pattern
			p.Count = 3
			changed.Pattern = &p
			edit(changed)
			changed = saved(pattern.ID)
			p = *changed.Pattern
			p.SkippedSlots = []int{1}
			changed.Pattern = &p
			invalidate(changed, 128000-2*unit)
			repair(2)
			final := activeBodyArtifact(t, v)
			apply(workspace.CommandRequest{Type: "UNDO"})
			if saved(child.ID).EvaluationStatus != "FAILED" {
				t.Fatal("undo did not restore failed input")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			if saved(child.ID).EvaluationStatus != "" {
				t.Fatal("undo skip did not recover member")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			apply(workspace.CommandRequest{Type: "REDO"})
			if activeBodyArtifact(t, v).GeometryID != final.GeometryID {
				t.Fatal("redo differs")
			}
			changed = saved(hole.ID)
			changed.Diameter = 2.4
			changed.Depth = 6
			edit(changed)
			unit = engineeringHoleVolume(saved(hole.ID))
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: location, Operations: []workspace.SketchOperation{{Type: "UPDATE_ENTITY_POINT", EntityID: "p", SubElement: "POINT", Point: &workspace.SketchPoint2{X: -10, Y: 8}}}})
			stage, err := service.GetFeatureInput(t.Context(), v.Document.ID, workspace.FeatureInputRequest{VersionID: v.Document.VersionID, FeatureID: pattern.ID, ResultStage: true})
			if err != nil || math.Abs(stage.Artifact.Volume-(128000-2*unit)) > 1e-5 || saved(child.ID).EvaluationStatus != "" {
				t.Fatal("Hole dimensions/point did not update members", err)
			}
			second = saved(second.ID)
			multi := definition
			p = *definition.Pattern
			p.Count = 2
			p.Direction = [3]float64{0, 1, 0}
			p.Spacing = 25
			p.AdditionalSources = []workspace.PatternSeed{{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: second.ID}, SourceKind: "AUTO"}}
			multi.Pattern = &p
			before := activeBodyArtifact(t, v).Volume
			r := request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &multi})
			candidate := preview(r)
			r.PreviewID = candidate.PreviewID
			apply(r)
			multi = v.Part.Features[len(v.Part.Features)-1]
			if math.Abs(activeBodyArtifact(t, v).Volume-(before-unit-engineeringHoleVolume(second))) > 1e-5 {
				t.Fatal("multi-seed tool copied wrong material")
			}
			chain := definition
			p = *definition.Pattern
			p.Kind = "MIRROR"
			p.Spacing = 0
			p.Count = 2
			p.MirrorPlaneID = "datum-yz"
			p.Source = workspace.FeatureStageRef{BodyID: body, FeatureID: multi.ID}
			chain.Pattern = &p
			before = activeBodyArtifact(t, v).Volume
			r = request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &chain})
			candidate = preview(r)
			r.PreviewID = candidate.PreviewID
			apply(r)
			if math.Abs(activeBodyArtifact(t, v).Volume-(before-unit-engineeringHoleVolume(second))) > 1e-5 {
				t.Fatal("chain copied seed zero or filled holes")
			}
			// Unsupported contracts reject atomically and never become BODY_STAGE.
			for _, mode := range []string{"duplicate", "delta", "independent", "through"} {
				bad := definition
				q := *definition.Pattern
				bad.Pattern = &q
				q.Source = workspace.FeatureStageRef{BodyID: body, FeatureID: hole.ID}
				switch mode {
				case "duplicate":
					q.AdditionalSources = []workspace.PatternSeed{{Source: q.Source, SourceKind: "AUTO"}}
				case "delta":
					q.SourceKind = "FEATURE_DELTA"
					q.StartFeatureID = hole.ID
				case "independent":
					q.ResultMode = "INDEPENDENT"
				case "through":
					f := saved(hole.ID)
					f.Extent = "THROUGH_ALL"
					if _, err := service.PreviewCommand(t.Context(), v.Document.ID, editRequest(f)); err == nil {
						t.Fatal("through seed with consumer accepted")
					}
					continue
				}
				if _, err := service.PreviewCommand(t.Context(), v.Document.ID, request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &bad})); err == nil {
					t.Fatal("unsupported seed accepted", mode)
				}
			}
			final = activeBodyArtifact(t, v)
			fresh := workspace.NewWithArtifacts(db, client, store)
			opened, err := fresh.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if err != nil || !reflect.DeepEqual(opened.Part.Features, v.Part.Features) || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
				t.Fatal("reopen", err)
			}
			last := v.Part.Features[len(v.Part.Features)-1]
			cold, err := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), v.Document.ID, editRequest(last))
			if err != nil || cold.Artifact == nil {
				t.Fatal("cold", err)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("cold differs", role)
				}
			}
			t.Logf("warm append reused=%d executed=%d generator_calls=%d; original Hole and Pattern identities retained", reused, executed, generators)
		})
	}
}

//go:build integration

package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

func TestDraftAdditivePatternLifecycleThroughRouter(t *testing.T) {
	for _, kind := range []string{"LINEAR", "CIRCULAR", "MIRROR"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", kind, reverse), func(t *testing.T) {
				s, a, db, c, _ := featureAssociationRuntimeService(t)
				v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Draft net material " + kind})
				if e != nil {
					t.Fatal(e)
				}
				x := &insertionScene{t: t, s: s, artifacts: a, db: db, client: c, v: v}
				rectangle := func(x1, y1, x2, y2, length float64) {
					t.Helper()
					x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
					sk := x.v.Part.Features[len(x.v.Part.Features)-1].ID
					x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x1, Y: y1}, Second: &workspace.SketchPoint2{X: x2, Y: y2}}}})
					x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sk, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: length})
				}
				rectangle(-23, -16, 20, 18, 4)
				stock := activeBodyArtifact(t, x.v).Volume
				// Start the boss at the same support plane: the range gate must distinguish
				// its overlap with stock from the net material above the stock.
				rectangle(7, -2, 11, 2, 14)
				boss := x.v.Part.Features[len(x.v.Part.Features)-1]
				body := boss.BodyID
				pick := findP7TopologyPick(t, s, x.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == boss.ID && strings.HasPrefix(p.Anchor.OutputSlot, "SIDE_FROM_PROFILE_EDGE/") && p.CreationEvidence.Centroid[0] > 10.9
				})
				x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "DRAFT", BodyID: body, Angle: 3, Reversed: reverse, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID, SourceFeatureID: boss.ID}}}})
				draft := x.v.Part.Features[len(x.v.Part.Features)-1]
				if cap := x.v.ReplicationSources[draft.ID]; cap.SourceKind != "FEATURE_DELTA" || cap.StartFeatureID != boss.ID || cap.Tool {
					t.Fatal(cap)
				}
				angleFace := findP7TopologyPick(t, s, x.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == draft.ID && strings.HasPrefix(p.Anchor.OutputSlot, "DRAFTED_FROM/")
				})
				if angleFace.Selection.Anchor.FeatureID != draft.ID {
					t.Fatal("draft face ownership")
				}
				_, stream, err := a.Open(t.Context(), activeBodyArtifact(t, x.v).Representations["VISUAL"].ObjectID)
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(stream)
				stream.Close()
				if err != nil {
					t.Fatal(err)
				}
				_, raw, err := visual.Decode(data)
				if err != nil {
					t.Fatal(err)
				}
				var display workspace.VisualizationManifest
				if err = json.Unmarshal(raw, &display); err != nil || display.FeatureAssociations == nil {
					t.Fatal("display associations", err)
				}
				owned := false
				for _, element := range display.FeatureAssociations.Elements {
					if element.Kind == "FACE" && element.LocalID == angleFace.LocalID {
						owned = len(element.Primary) == 1 && element.Primary[0] == draft.ID
					}
				}
				if !owned {
					t.Fatal("draft angle face display ownership")
				}

				before := activeBodyArtifact(t, x.v).Volume
				delta := before - stock
				definition := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: kind, Distribution: "FIXED_STEP", Count: 2, Spacing: 18, Angle: 180, Direction: [3]float64{-1, 0, 0}}, Source: workspace.FeatureStageRef{BodyID: body, FeatureID: draft.ID}, SourceKind: "AUTO"}}
				if kind == "CIRCULAR" {
					definition.Pattern.Direction = [3]float64{0, 0, 1}
				}
				if kind == "MIRROR" {
					definition.Pattern.MirrorPlaneID = "datum-yz"
					definition.Pattern.Spacing = 0
					definition.Pattern.Angle = 0
				}
				r := x.request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &definition})
				head := x.v.Document.VersionID
				preview, err := s.PreviewCommand(t.Context(), x.v.Document.ID, r)
				if err != nil || preview.Artifact == nil {
					t.Fatal("preview", err)
				}
				read, err := s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
				if err != nil || read.Document.VersionID != head {
					t.Fatal("preview wrote history", err)
				}
				r.PreviewID = preview.PreviewID
				x.apply(r)
				pattern := x.v.Part.Features[len(x.v.Part.Features)-1]
				if math.Abs(activeBodyArtifact(t, x.v).Volume-(stock+2*delta)) > 1e-5 || pattern.Pattern.SourceKind != "FEATURE_DELTA" || pattern.Pattern.StartFeatureID != boss.ID {
					t.Fatal("wrong net material", activeBodyArtifact(t, x.v).Volume, stock+2*delta, pattern.Pattern)
				}
				final := activeBodyArtifact(t, x.v)
				x.apply(workspace.CommandRequest{Type: "UNDO"})
				if math.Abs(activeBodyArtifact(t, x.v).Volume-before) > 1e-5 {
					t.Fatal("undo")
				}
				x.apply(workspace.CommandRequest{Type: "REDO"})
				if activeBodyArtifact(t, x.v).GeometryID != final.GeometryID {
					t.Fatal("redo")
				}
				// Overlapping selected ranges collapse to the maximal range, preserving one
				// tool rather than copying the boss a second time.
				p := *pattern.Pattern
				p.AdditionalSources = []workspace.PatternSeed{{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: boss.ID}, SourceKind: "AUTO"}}
				pattern.Pattern = &p
				edit := func(f workspace.Feature) workspace.CommandRequest {
					return x.request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, f.ID)})
				}
				x.apply(edit(pattern))
				pattern = x.v.Part.Features[len(x.v.Part.Features)-1]
				if len(pattern.Pattern.AdditionalSources) != 1 || activeBodyArtifact(t, x.v).GeometryID != final.GeometryID {
					t.Fatal("range deduplication")
				}
				if kind == "LINEAR" {
					// All additional slots skipped: stage stays valid but stops advertising an
					// independent tool. Restoring a slot re-enables the exact same source.
					p = *pattern.Pattern
					p.SkippedSlots = []int{1}
					pattern.Pattern = &p
					x.apply(edit(pattern))
					if x.v.ReplicationSources[pattern.ID].Tool {
						t.Fatal("empty tool advertised")
					}
					bad := definition
					q := *definition.Pattern
					q.Source = workspace.FeatureStageRef{BodyID: body, FeatureID: pattern.ID}
					q.SourceKind = "GENERATOR_TOOL"
					bad.Pattern = &q
					h := x.v.Document.VersionID
					if _, err = s.PreviewCommand(t.Context(), x.v.Document.ID, x.request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &bad})); err == nil {
						t.Fatal("empty array accepted as tool")
					}
					read, _ = s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
					if read.Document.VersionID != h {
						t.Fatal("invalid preview wrote history")
					}
					pattern = x.v.Part.Features[len(x.v.Part.Features)-1]
					p = *pattern.Pattern
					p.SkippedSlots = nil
					pattern.Pattern = &p
					x.apply(edit(pattern))
					pattern = x.v.Part.Features[len(x.v.Part.Features)-1]
					if !x.v.ReplicationSources[pattern.ID].Tool {
						t.Fatal("restored tool unavailable")
					}
				}
				// An upstream angle edit recomputes both the source and all copies.
				draft.Angle = 4
				x.apply(edit(draft))
				if math.Abs(activeBodyArtifact(t, x.v).Volume-final.Volume) < 1e-5 {
					t.Fatal("angle did not propagate")
				}
				pattern = x.v.Part.Features[len(x.v.Part.Features)-1]
				coldTarget := pattern
				if kind == "LINEAR" {
					edge := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
						return p.Anchor.FeatureID == pattern.ID && strings.HasPrefix(p.Anchor.OutputSlot, "MEMBER/1/") && math.Abs(p.CreationEvidence.Centroid[2]-14) < 1e-6
					})
					x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: .1, Selections: []workspace.FeatureSelection{{Selection: edge.Selection, SourceVersionID: edge.SourceVersionID, SourceFeatureID: pattern.ID}}}})
					child := x.v.Part.Features[len(x.v.Part.Features)-1]
					reduced := pattern
					q := *pattern.Pattern
					q.Count = 1
					reduced.Pattern = &q
					request := edit(reduced)
					candidate, err := s.PreviewCommand(t.Context(), x.v.Document.ID, request)
					if err != nil || len(candidate.RepairRequired) != 1 || candidate.RepairRequired[0].FeatureID != child.ID {
						t.Fatal("member loss repair preview", err, candidate.RepairRequired)
					}
					request.PreviewID = candidate.PreviewID
					x.apply(request)
					if x.v.ReplicationSources[pattern.ID].Tool || x.v.Part.Features[len(x.v.Part.Features)-1].EvaluationStatus != "FAILED" {
						t.Fatal("member loss not retained")
					}
					x.apply(workspace.CommandRequest{Type: "UNDO"})
					if x.v.Part.Features[len(x.v.Part.Features)-1].EvaluationStatus != "" {
						t.Fatal("member restoration failed")
					}
					coldTarget = x.v.Part.Features[len(x.v.Part.Features)-1]
				}

				final = activeBodyArtifact(t, x.v)
				fresh := workspace.NewWithArtifacts(db, c, a)
				opened, err := fresh.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
				if err != nil || !reflect.DeepEqual(opened.Part.Features, x.v.Part.Features) {
					t.Fatal("reopen", err)
				}
				cold, err := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, edit(coldTarget))
				if err != nil || cold.Artifact == nil {
					t.Fatal("cold preview", err)
				}
				for _, role := range []string{"BREP", "NAMING"} {
					if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
						t.Fatal("cold mismatch", role)
					}
				}
			})
		}
	}
}

func TestFiniteRemovalRangeLifecycleThroughRouter(t *testing.T) {
	s, store, db, client, _ := featureAssociationRuntimeService(t)
	for _, family := range []string{"FINITE", "SYMMETRIC", "TWO_SIDED", "REVOLVE", "LOFT", "SIMPLE", "COUNTERBORE", "COUNTERSINK"} {
		t.Run(family, func(t *testing.T) {
			v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Finite removal range " + family})
			if e != nil {
				t.Fatal(e)
			}
			x := &insertionScene{t: t, s: s, artifacts: store, db: db, client: client, v: v}
			saved := func(id string) workspace.Feature {
				t.Helper()
				for _, f := range x.v.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("missing", id)
				return workspace.Feature{}
			}
			edit := func(f workspace.Feature) workspace.CommandRequest {
				return x.request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, Feature: &f, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, f.ID)})
			}
			closeVolume := func(got, want float64) {
				t.Helper()
				if math.Abs(got-want) > 1e-5 {
					t.Fatal("exact volume", got, want)
				}
			}
			preview := func(r workspace.CommandRequest) workspace.CommandPreview {
				t.Helper()
				head := x.v.Document.VersionID
				p, err := s.PreviewCommand(t.Context(), x.v.Document.ID, r)
				if err != nil || p.Artifact == nil {
					t.Fatal("preview", err)
				}
				now, err := s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
				if err != nil || now.Document.VersionID != head {
					t.Fatal("preview advanced Head", err)
				}
				return p
			}
			plane := func(z float64) string {
				t.Helper()
				x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
				return x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID
			}
			sketch := func(datum string) string {
				t.Helper()
				x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: datum})
				return x.v.Part.Features[len(x.v.Part.Features)-1].ID
			}
			circle := func(datum string) string {
				t.Helper()
				sk := sketch(datum)
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 12, Y: 6}, Radius: 2}}}})
				return sk
			}
			stockSketch := sketch("datum-xy")
			x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stockSketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -40, Y: -25}, Second: &workspace.SketchPoint2{X: 40, Y: 25}}}})
			x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stockSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 12})
			x.base = x.v.Part.Features[len(x.v.Part.Features)-1]
			body := x.base.BodyID
			top := plane(12)
			switch family {
			case "SIMPLE", "COUNTERBORE", "COUNTERSINK":
				sk := sketch(top)
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 12, Y: 6}}}}})
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "BLIND", HoleKind: family, Diameter: 4, Depth: 6, RecessDiameter: 6, RecessDepth: 2, CountersinkAngle: 90, Reversed: true, HolePoint: &workspace.PatternPointReference{SketchID: sk, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}}}})
			case "REVOLVE":
				x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 6, 0}, Normal: [3]float64{0, -1, 0}, UDirection: [3]float64{1, 0, 0}})
				sk := sketch(x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID)
				x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 12, Y: 6}, Second: &workspace.SketchPoint2{X: 14, Y: 12}}}})
				axis := ""
				for _, ent := range saved(sk).Sketch.Entities {
					if ent.Kind == "LINE" && ent.Start != nil && ent.End != nil && ent.Start.X == 12 && ent.End.X == 12 {
						axis = ent.ID
					}
				}
				if axis == "" {
					t.Fatal("revolve axis absent")
				}
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sk, Generator: "REVOLVE", Operation: "REMOVE", BodyID: body, Angle: 360, AxisEntityID: axis})
			case "LOFT":
				a, b := circle(top), circle(plane(6))
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", BodyID: body, Operation: "REMOVE", Sections: []workspace.LoftSection{{SketchID: a}, {SketchID: b}}}})
			default:
				sk := circle(top)
				length := 6.
				if family == "SYMMETRIC" {
					length = 12
				}
				x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sk, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", BodyID: body, Length: length, Length2: func() float64 {
					if family == "TWO_SIDED" {
						return 3
					}
					return 0
				}(), Extent: family, Reversed: true})
			}
			cut := x.v.Part.Features[len(x.v.Part.Features)-1]
			rawVolume := activeBodyArtifact(t, x.v).Volume
			floor := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
				return p.CreationEvidence.GeometryType == "CIRCLE" && math.Abs(p.CreationEvidence.Centroid[2]-6) < 1e-6
			})
			x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: body, Length: .5, Selections: []workspace.FeatureSelection{{Selection: floor.Selection, SourceVersionID: floor.SourceVersionID, SourceFeatureID: cut.ID}}}})
			round := x.v.Part.Features[len(x.v.Part.Features)-1]
			if activeBodyArtifact(t, x.v).Volume <= rawVolume {
				t.Fatal("inner fillet must add material locally")
			}
			rim := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
				return p.CreationEvidence.GeometryType == "CIRCLE" && math.Abs(p.CreationEvidence.Centroid[0]-12) < 1e-6 && math.Abs(p.CreationEvidence.Centroid[2]-12) < 1e-6
			})
			x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: .2, Selections: []workspace.FeatureSelection{{Selection: rim.Selection, SourceVersionID: rim.SourceVersionID, SourceFeatureID: round.ID}}}})
			end := x.v.Part.Features[len(x.v.Part.Features)-1]
			cap := x.v.ReplicationSources[end.ID]
			if cap.SourceKind != "FEATURE_DELTA" || cap.StartFeatureID != cut.ID || cap.Operation != "REMOVE" {
				t.Fatal("range capability", cap)
			}
			net := 48000 - activeBodyArtifact(t, x.v).Volume
			definition := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: end.ID}, SourceKind: "AUTO", ResultMode: "COMBINE", AdditionalSources: []workspace.PatternSeed{{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: cut.ID}, SourceKind: "AUTO"}, {Source: workspace.FeatureStageRef{BodyID: body, FeatureID: round.ID}, SourceKind: "AUTO"}}, PatternDefinition: workspace.PatternDefinition{Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 24, Direction: [3]float64{-1, 0, 0}}}}
			req := x.request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &definition})
			hot := preview(req)
			closeVolume(hot.Artifact.Volume, 48000-2*net)
			var reused, executed uint32
			if hot.Runtime == nil {
				t.Fatal("runtime evidence missing")
			}
			for _, run := range hot.Runtime.WorkerRuns {
				reused += run.StagesReused
				executed += run.StagesExecuted
			}
			if reused < 4 || executed != 1 {
				t.Fatal("unconsumed range baseline missing from warm checkpoint", reused, executed)
			}
			for _, mode := range []string{"CIRCULAR", "MIRROR"} {
				f := definition
				f.ID = ""
				p := *definition.Pattern
				f.Pattern = &p
				p.Kind = mode
				if mode == "CIRCULAR" {
					p.Angle = 180
					p.Direction = [3]float64{0, 0, 1}
				} else {
					p.Spacing = 0
					p.MirrorPlaneID = "datum-yz"
				}
				candidate := preview(x.request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &f}))
				closeVolume(candidate.Artifact.Volume, 48000-2*net)
				s.DiscardPreview(x.v.Document.ID, p6Actor, candidate.PreviewID)
			}
			req.PreviewID = hot.PreviewID
			x.apply(req)
			pattern := x.v.Part.Features[len(x.v.Part.Features)-1]
			if pattern.Operation != "REMOVE" || pattern.Pattern.SourceKind != "FEATURE_DELTA" || pattern.Pattern.StartFeatureID != cut.ID || len(pattern.Pattern.AdditionalSources) != 2 {
				t.Fatal("accepted references were rewritten", pattern)
			}
			for _, id := range []string{cut.ID, round.ID, end.ID} {
				if _, err := s.PreviewCommand(t.Context(), x.v.Document.ID, x.request(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: "FEATURE", TargetID: id, ExpectedFeatureDigest: findP6FeatureDigest(t, x.v.StructureTree, id)})); err == nil || !strings.Contains(err.Error(), "depends on it") {
					t.Fatal("range reference deletion allowed", id, err)
				}
			}
			first := activeBodyArtifact(t, x.v)
			x.apply(workspace.CommandRequest{Type: "UNDO"})
			closeVolume(activeBodyArtifact(t, x.v).Volume, 48000-net)
			x.apply(workspace.CommandRequest{Type: "REDO"})
			if activeBodyArtifact(t, x.v).GeometryID != first.GeometryID {
				t.Fatal("range redo differs")
			}
			stage := func(id string) workspace.Artifact {
				t.Helper()
				r, err := s.GetFeatureInput(t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: id, ResultStage: true})
				if err != nil {
					t.Fatal(err)
				}
				return r.Artifact
			}
			changed := saved(round.ID)
			changed.Length = .6
			r := edit(changed)
			r.ParameterExpressions = map[string]string{"length": "0.6 mm"}
			x.apply(r)
			net = 48000 - stage(end.ID).Volume
			closeVolume(activeBodyArtifact(t, x.v).Volume, 48000-2*net)
			if family == "FINITE" {
				// Inserting any step into the accepted machining interval is atomic rejection.
				x.reject(x.insert(cut.ID, [3]float64{40, 25, 6}, .1), "FEATURE_DELTA")
				f := saved(pattern.ID)
				p := *f.Pattern
				f.Pattern = &p
				p.Count = 3
				x.apply(edit(f))
				closeVolume(activeBodyArtifact(t, x.v).Volume, 48000-3*net)
				mouth := findP7TopologyPick(t, s, x.v, "EDGE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == pattern.ID && p.CreationEvidence.GeometryType == "CIRCLE" && math.Abs(p.CreationEvidence.Centroid[0]+36) < 1e-6 && math.Abs(p.CreationEvidence.Centroid[2]-12) < 1e-6
				})
				x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: .05, Selections: []workspace.FeatureSelection{{Selection: mouth.Selection, SourceVersionID: mouth.SourceVersionID, SourceFeatureID: pattern.ID}}}})
				child := x.v.Part.Features[len(x.v.Part.Features)-1]
				f = saved(pattern.ID)
				p = *f.Pattern
				f.Pattern = &p
				p.Count = 2
				r = edit(f)
				repair := preview(r)
				closeVolume(repair.Artifact.Volume, 48000-2*net)
				if len(repair.RepairRequired) != 1 || repair.RepairRequired[0].FeatureID != child.ID {
					t.Fatal("downstream repair preview absent", repair.RepairRequired)
				}
				r.PreviewID = repair.PreviewID
				x.apply(r)
				if saved(child.ID).EvaluationStatus != "FAILED" || saved(pattern.ID).EvaluationStatus != "" {
					t.Fatal("range failure boundary")
				}
				input, err := s.GetFeatureInput(t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: child.ID})
				if err != nil {
					t.Fatal(err)
				}
				var pick *workspace.FeatureSelection
				for local := uint64(1); local <= p6TopologyCount(t, input.Artifact.Topology, "edges"); local++ {
					bound, err := s.GetFeatureInput(t.Context(), x.v.Document.ID, workspace.FeatureInputRequest{VersionID: x.v.Document.VersionID, FeatureID: child.ID, GeometryKey: input.Artifact.GeometryKey, Kind: "EDGE", LocalID: local})
					if err != nil {
						t.Fatal(err)
					}
					if bound.Selection != nil {
						e := bound.Selection.Selection.CreationEvidence
						if e.GeometryType == "CIRCLE" && math.Abs(e.Centroid[0]+12) < 1e-6 && math.Abs(e.Centroid[2]-12) < 1e-6 {
							pick = bound.Selection
							break
						}
					}
				}
				if pick == nil {
					t.Fatal("replacement member missing")
				}
				fixed := saved(child.ID)
				fixed.Selections = []workspace.FeatureSelection{*pick}
				x.apply(edit(fixed))
				if saved(child.ID).EvaluationStatus != "" {
					t.Fatal("repair failed")
				}
				f = saved(pattern.ID)
				p = *f.Pattern
				f.Pattern = &p
				p.Count = 3
				x.apply(edit(f))
				before := activeBodyArtifact(t, x.v).Volume
				chain := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: pattern.ID}, SourceKind: "AUTO", ResultMode: "COMBINE", PatternDefinition: workspace.PatternDefinition{Kind: "MIRROR", Distribution: "FIXED_STEP", Count: 2, Direction: [3]float64{0, 1, 0}, MirrorPlaneID: "datum-xz"}}}
				x.apply(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &chain})
				closeVolume(activeBodyArtifact(t, x.v).Volume, before-2*net)
			}
			final := activeBodyArtifact(t, x.v)
			last := x.v.Part.Features[len(x.v.Part.Features)-1]
			fresh := workspace.NewWithArtifacts(db, client, store)
			opened, err := fresh.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
			if err != nil || !reflect.DeepEqual(opened.Part, x.v.Part) || activeBodyArtifact(t, opened).GeometryID != final.GeometryID {
				t.Fatal("reopen", err)
			}
			cold, err := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), x.v.Document.ID, edit(last))
			if err != nil || cold.Artifact == nil {
				t.Fatal("cold", err)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if cold.Artifact.Representations[role].Digest != final.Representations[role].Digest {
					t.Fatal("hot/cold mismatch", role)
				}
			}
		})
	}
}

func TestRemovalRangeRejectsOutsideMaterialThroughRouter(t *testing.T) {
	rejected := 0
	for _, reverse := range []bool{false, true} {
		s, a, db, c, _ := featureAssociationRuntimeService(t)
		v, e := s.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Net removal containment"})
		if e != nil {
			t.Fatal(e)
		}
		x := &insertionScene{t: t, s: s, artifacts: a, db: db, client: c, v: v}
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		sk := x.v.Part.Features[0].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -20}, Second: &workspace.SketchPoint2{X: 20, Y: 20}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sk, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
		body := x.v.Part.ActiveBodyID
		x.apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 10}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: x.v.Part.DatumPlanes[len(x.v.Part.DatumPlanes)-1].ID})
		sk = x.v.Part.Features[len(x.v.Part.Features)-1].ID
		x.apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sk, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: -8}, Radius: 6}}}})
		x.apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sk, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", BodyID: body, Length: 8, Reversed: true})
		cut := x.v.Part.Features[len(x.v.Part.Features)-1]
		face := findP7TopologyPick(t, s, x.v, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
			return p.CreationEvidence.GeometryType == "PLANE" && p.CreationEvidence.Centroid[0] > 19.9
		})
		x.apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "DRAFT", BodyID: body, Angle: 3, Reversed: reverse, NeutralPlaneID: "datum-xy", Selections: []workspace.FeatureSelection{{Selection: face.Selection, SourceVersionID: face.SourceVersionID, SourceFeatureID: cut.ID}}}})
		draft := x.v.Part.Features[len(x.v.Part.Features)-1]
		if activeBodyArtifact(t, x.v).Volume >= 16000 {
			t.Fatal("counterexample must have net volume removal")
		}
		f := workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "ADD", Pattern: &workspace.FeaturePattern{Source: workspace.FeatureStageRef{BodyID: body, FeatureID: draft.ID}, SourceKind: "AUTO", PatternDefinition: workspace.PatternDefinition{Kind: "MIRROR", Distribution: "FIXED_STEP", Count: 2, Direction: [3]float64{1, 0, 0}, MirrorPlaneID: "datum-yz"}}}
		head := x.v.Document.VersionID
		_, err := s.PreviewCommand(t.Context(), x.v.Document.ID, x.request(workspace.CommandRequest{Type: "CREATE_PATTERN", Feature: &f}))
		if err != nil {
			if !strings.Contains(err.Error(), "PATTERN_RANGE_ADDS_MATERIAL") {
				t.Fatal(err)
			}
			rejected++
		}
		now, err := s.GetDocument(t.Context(), x.v.Document.ID, p6Actor)
		if err != nil || now.Document.VersionID != head {
			t.Fatal("failed/cancelled range wrote Head", err)
		}
	}
	if rejected != 1 {
		t.Fatal("spatial containment gate not enforced", rejected)
	}
}

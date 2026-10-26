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

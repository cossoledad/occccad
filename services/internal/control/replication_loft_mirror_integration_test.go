package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"strings"
	"testing"
)

func TestLoftReplicationAndMirrorLifecycleThroughRouter(t *testing.T) {
	for _, kind := range []string{"CIRCULAR", "MIRROR"} {
		t.Run(kind, func(t *testing.T) {
			service, artifacts, db, client := featureAssociationTestService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Loft " + kind})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-replication-%d", view.Document.ID, seq)
				}
				v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
				if e != nil {
					t.Fatal(r.Type, e)
				}
				view = v
				for _, f := range v.Part.Features {
					if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
						t.Fatal(f.ID, f.Diagnostic)
					}
				}
			}
			rectangle := func(plane string, x1, y1, x2, y2 float64) string {
				t.Helper()
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
				id := view.Part.Features[len(view.Part.Features)-1].ID
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: x1, Y: y1}, Second: &workspace.SketchPoint2{X: x2, Y: y2}}}})
				return id
			}
			stockSketch := rectangle("datum-xy", -30, -30, 30, 30)
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stockSketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
			body := view.Part.Features[len(view.Part.Features)-1].BodyID
			eccentric := rectangle("datum-xy", -2, 17, 2, 19)
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: eccentric, Generator: "LINEAR_EXTRUDE", Operation: "REMOVE", Length: 10})
			baseVolume := activeBodyArtifact(t, view).Volume
			a := rectangle("datum-xy", 8, -2, 12, 2)
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: "loft top", Origin: [3]float64{0, 0, 10}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			top := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
			b := rectangle(top, 9, -1, 11, 1)
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", BodyID: body, Operation: "REMOVE", Sections: []workspace.LoftSection{{SketchID: a}, {SketchID: b}}}})
			seed := view.Part.Features[len(view.Part.Features)-1]
			seedVolume := activeBodyArtifact(t, view).Volume
			toolVolume := baseVolume - seedVolume
			if !view.ReplicationSources[seed.ID].Tool {
				t.Fatal("loft tool unavailable in authoritative view")
			}
			definition := workspace.PatternDefinition{Kind: kind, Count: 2, Distribution: "FIXED_STEP", Direction: [3]float64{0, 0, 1}, Angle: 180}
			if kind == "MIRROR" {
				definition.Angle = 0
				definition.Direction = [3]float64{1, 0, 0}
				definition.MirrorPlaneID = "datum-yz"
				plane := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
					return p.GeometryType == "PLANE" && math.Abs(s.CreationEvidence.Centroid[0]+2) < 1e-6 && math.Abs(s.CreationEvidence.Direction[0]) > 0.99
				})
				definition.MirrorPlaneID = ""
				definition.MirrorPlane = &workspace.FeatureSelection{Selection: plane.Selection, SourceVersionID: plane.SourceVersionID}

			}
			r := workspace.CommandRequest{ActorID: p6Actor, RequestID: fmt.Sprintf("%s-preview", view.Document.ID), Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: body, Operation: "REMOVE", Pattern: &workspace.FeaturePattern{PatternDefinition: definition, SourceKind: "GENERATOR_TOOL", Source: workspace.FeatureStageRef{BodyID: body, FeatureID: seed.ID}}}}
			before := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, r)
			if e != nil {
				t.Fatal(e)
			}
			unchanged, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || unchanged.Document.VersionID != before {
				t.Fatal("preview changed Head", e)
			}
			r.PreviewID = preview.PreviewID
			apply(r)
			pattern := view.Part.Features[len(view.Part.Features)-1]
			if kind == "MIRROR" {
				input, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: pattern.ID})
				if e != nil || input.NeutralPick == nil {
					t.Fatal("mirror historical plane input", e)
				}
			}
			checkVolume := func(expected float64) {
				t.Helper()
				if math.Abs(activeBodyArtifact(t, view).Volume-expected) > 1e-5 {
					t.Fatal("copied stock/eccentric feature instead of loft tool", activeBodyArtifact(t, view).Volume, expected)
				}
			}
			checkVolume(seedVolume - toolVolume)
			member := findP7TopologyPick(t, service, view, "EDGE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return s.Anchor.FeatureID == pattern.ID && strings.HasPrefix(s.Anchor.OutputSlot, "MEMBER/1/") && p.GeometryType == "LINE" && math.Abs(s.CreationEvidence.Centroid[2]-10) < 1e-5
			})
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "CHAMFER", BodyID: body, Length: 0.2, Selections: []workspace.FeatureSelection{{Selection: member.Selection, SourceVersionID: member.SourceVersionID}}}})
			downstreamVolume := activeBodyArtifact(t, view).Volume
			if downstreamVolume >= seedVolume-toolVolume {
				t.Fatal("member chamfer did not remove material")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			checkVolume(seedVolume - toolVolume)
			apply(workspace.CommandRequest{Type: "REDO"})
			checkVolume(downstreamVolume)
			// Seed edits recalculate the tool and retained member reference.
			seed.Ruled = true
			digest := ""
			var visit func(workspace.DocumentStructureNode)
			visit = func(n workspace.DocumentStructureNode) {
				if n.EntityID == seed.ID && n.PresentationRole != "INPUT_REFERENCE" {
					digest = n.DefinitionDigest
				}
				for _, c := range n.Children {
					visit(c)
				}
			}
			visit(*view.StructureTree)
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: seed.ID, ExpectedFeatureDigest: digest, Feature: &seed})
			checkVolume(downstreamVolume)
			if kind == "CIRCULAR" {
				pattern.Pattern.Count = 3
				pattern.Pattern.Distribution = "FULL_CIRCLE"
				digest = ""
				for _, f := range view.Part.Features {
					if f.ID == pattern.ID {
						pattern.EvaluationStatus = f.EvaluationStatus
					}
				}
				var scan func(workspace.DocumentStructureNode)
				scan = func(n workspace.DocumentStructureNode) {
					if n.EntityID == pattern.ID && n.PresentationRole != "INPUT_REFERENCE" {
						digest = n.DefinitionDigest
					}
					for _, c := range n.Children {
						scan(c)
					}
				}
				scan(*view.StructureTree)
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: pattern.ID, ExpectedFeatureDigest: digest, Feature: &pattern})
				downstreamVolume -= toolVolume
				if math.Abs(activeBodyArtifact(t, view).Volume-downstreamVolume) > 1e-3 {
					t.Fatal("count edit did not preserve member chamfer", activeBodyArtifact(t, view).Volume, downstreamVolume)
				}
				downstreamVolume = activeBodyArtifact(t, view).Volume
			}
			cold := workspace.NewWithArtifacts(db, client, artifacts)
			reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			if math.Abs(activeBodyArtifact(t, reopened).Volume-downstreamVolume) > 1e-5 || !reopened.ReplicationSources[seed.ID].Tool {
				t.Fatal("cold reopen lost replica or source capability")
			}
		})
	}
}

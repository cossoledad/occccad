package control

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"testing"

	"github.com/occccad/occccad/internal/visual"
	"github.com/occccad/occccad/internal/workspace"
)

// Exact dimensions of document 01a1088f-bd2a-740c-b23a-4dd792502bca.
// Rebuild through Domain Commands and the real Router, with fresh identities.
func TestPropellerLargeFilletMaterialRangeThroughRouter(t *testing.T) {
	for _, testCase := range []struct {
		asymmetric bool
		sourceKind string
	}{{false, "FEATURE_DELTA"}, {true, "FEATURE_DELTA"}, {false, "GENERATOR_TOOL"}, {true, "GENERATOR_TOOL"}} {
		asymmetric := testCase.asymmetric
		t.Run(fmt.Sprintf("asymmetricBase=%v/%s", asymmetric, testCase.sourceKind), func(t *testing.T) {
			service, artifacts, db, client := featureAssociationTestService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Large fillet propeller"})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-prop-%d", view.Document.ID, seq)
				}
				v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
				if e != nil {
					t.Fatalf("%s: %v", r.Type, e)
				}
				view = v
				for _, f := range view.Part.Features {
					if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
						t.Fatalf("%s: %s", f.Type, f.Diagnostic)
					}
				}
			}
			polygon := func(points [][2]float64, prefix string) {
				t.Helper()
				sketch := view.Part.Features[len(view.Part.Features)-1]
				ops := []workspace.SketchOperation{}
				for i, p := range points {
					q := points[(i+1)%len(points)]
					ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: fmt.Sprintf("%s-%d", prefix, i), Kind: "LINE", Role: "PROFILE", StartPointID: fmt.Sprintf("%s-p-%d", prefix, i), EndPointID: fmt.Sprintf("%s-p-%d", prefix, (i+1)%len(points)), Start: &workspace.SketchPoint2{X: p[0], Y: p[1]}, End: &workspace.SketchPoint2{X: q[0], Y: q[1]}}})
				}
				for i := range points {
					ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: fmt.Sprintf("%s-join-%d", prefix, i), Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: fmt.Sprintf("%s-%d", prefix, i), SubElement: "END"}, {Target: "ENTITY", EntityID: fmt.Sprintf("%s-%d", prefix, (i+1)%len(points)), SubElement: "START"}}}})
				}
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch.ID, Operations: ops})
			}
			load := func() (visual.Mesh, workspace.VisualizationManifest) {
				t.Helper()
				a := activeBodyArtifact(t, view)
				_, r, e := artifacts.Open(t.Context(), a.Representations["VISUAL"].ObjectID)
				if e != nil {
					t.Fatal(e)
				}
				b, e := io.ReadAll(r)
				r.Close()
				if e != nil {
					t.Fatal(e)
				}
				mesh, raw, e := visual.Decode(b)
				if e != nil {
					t.Fatal(e)
				}
				var v workspace.VisualizationManifest
				if e = json.Unmarshal(raw, &v); e != nil {
					t.Fatal(e)
				}
				return mesh, v
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			s := view.Part.Features[len(view.Part.Features)-1]
			top := 50.0
			if asymmetric {
				top = 55
			} // A rotated whole Body would duplicate a 5 mm baseline strip.
			polygon([][2]float64{{60, -50}, {-60, -50}, {-60, top}, {60, top}}, "base")
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: s.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 90})
			base := view.Part.Features[len(view.Part.Features)-1]
			baseVolume := activeBodyArtifact(t, view).Volume
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{60, 50, 0}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}})
			plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1]
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane.ID})
			s = view.Part.Features[len(view.Part.Features)-1]
			polygon([][2]float64{{-20, 0}, {-100, 70}, {-86.3716814159292, 85.57522123893804}, {-6.371681415929206, 15.575221238938047}}, "blade")
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: s.ID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 120})
			blade := view.Part.Features[len(view.Part.Features)-1]
			mesh, _ := load()
			var selected []workspace.FeatureSelection
			for _, edge := range mesh.Edges {
				p, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "EDGE", edge.LocalID)
				if e != nil {
					t.Fatal(e)
				}
				if p.PersistentSelection != nil && p.PersistentSelection.Anchor.FeatureID == blade.ID && p.PersistentSelection.Anchor.OutputSlot == "START_BOUNDARY_FROM_PROFILE_EDGE/blade-0" {
					selected = []workspace.FeatureSelection{{Selection: *p.PersistentSelection, SourceVersionID: view.Document.VersionID, SourceFeatureID: blade.ID}}
					break
				}
			}
			if selected == nil {
				t.Fatal("large fillet root missing")
			}
			unroundedVolume := activeBodyArtifact(t, view).Volume
			apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 80, Selections: selected}})
			fillet := view.Part.Features[len(view.Part.Features)-1]
			_, before := load()
			faces := 0
			for _, e := range before.FeatureAssociations.Elements {
				if e.Kind != "FACE" {
					continue
				}
				if !asymmetric && (e.LocalID == 3 || e.LocalID == 5 || e.LocalID == 12) {
					if slices.Contains(e.Origins, fillet.ID) {
						t.Fatalf("original face %d attributed to fillet: %+v", e.LocalID, e)
					}
				}
				if slices.Contains(e.Origins, fillet.ID) {
					faces++
				}
			}
			if faces < 1 {
				t.Fatal("missing actual fillet contribution")
			}
			if !asymmetric {
				var faceOrigins, edgeOrigins []string
				for _, e := range before.FeatureAssociations.Elements {
					if e.Kind != "FACE" && e.Kind != "EDGE" {
						continue
					}
					props, err := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, e.Kind, e.LocalID)
					if err != nil {
						t.Fatal(err)
					}
					if props.PersistentSelection == nil {
						continue
					}
					evidence := props.PersistentSelection.CreationEvidence
					if evidence.MeasureSI == nil {
						continue
					}
					// Locate the enlarged y=-50 cap and its extended z=0 edge
					// in this fixture; local numbering differs from the user's document.
					if e.Kind == "FACE" && evidence.GeometryType == "PLANE" && math.Abs(evidence.Centroid[1]+50) < 1e-6 && *evidence.MeasureSI > .0108+1e-8 {
						faceOrigins = e.Origins
					}
					if e.Kind == "EDGE" && evidence.GeometryType == "LINE" && math.Abs(evidence.Centroid[1]+50) < 1e-6 && math.Abs(evidence.Centroid[2]) < 1e-6 && math.Abs(*evidence.MeasureSI-.124809324) < 1e-7 {
						edgeOrigins = e.Origins
					}
				}
				if !slices.Contains(faceOrigins, base.ID) || !slices.Contains(faceOrigins, fillet.ID) || !slices.Contains(edgeOrigins, base.ID) || !slices.Contains(edgeOrigins, fillet.ID) {
					t.Fatalf("merged cap/boundary sources disagree: face=%v edge=%v", faceOrigins, edgeOrigins)
				}
			}
			seedVolume := activeBodyArtifact(t, view).Volume
			sourceID, startID := fillet.ID, blade.ID
			if testCase.sourceKind == "GENERATOR_TOOL" {
				sourceID, startID = blade.ID, ""
			}
			request := workspace.CommandRequest{ActorID: p6Actor, Type: "CREATE_PATTERN", Feature: &workspace.Feature{Type: "SOLID_PATTERN", BodyID: base.BodyID, Operation: "ADD", Pattern: &workspace.FeaturePattern{PatternDefinition: workspace.PatternDefinition{Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 2, AxisEntityID: "AXIS_SYSTEM:axis-system-default:Z", Direction: [3]float64{0, 0, 1}}, Source: workspace.FeatureStageRef{BodyID: base.BodyID, FeatureID: sourceID}, SourceKind: testCase.sourceKind, StartFeatureID: startID, ResultMode: "COMBINE"}}}
			request.RequestID = fmt.Sprintf("%s-prop-%d", view.Document.ID, seq+1)
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil {
				t.Fatal("rounded pattern preview", e)
			}
			if preview.PreviewID == "" {
				t.Fatal("pattern preview missing")
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			pattern := view.Part.Features[len(view.Part.Features)-1]
			expectedVolume := 2*seedVolume - baseVolume
			if testCase.sourceKind == "GENERATOR_TOOL" {
				expectedVolume = seedVolume + unroundedVolume - baseVolume
			}
			if math.Abs(activeBodyArtifact(t, view).Volume-expectedVolume) > 1e-4 {
				t.Fatalf("baseline copied or rounded leaf missing: %v want %v", activeBodyArtifact(t, view).Volume, 2*seedVolume-baseVolume)
			}
			if testCase.sourceKind == "GENERATOR_TOOL" {
				memberMesh, memberVisual := load()
				var rootID uint64
				for _, edge := range memberMesh.Edges {
					for _, assoc := range memberVisual.FeatureAssociations.Elements {
						if assoc.Kind != "EDGE" || assoc.LocalID != edge.LocalID {
							continue
						}
						member := false
						for _, m := range assoc.Members {
							if m.PatternID == pattern.ID && m.Slot == 1 {
								member = true
							}
						}
						if !member {
							continue
						}
						// The accepted member transform maps this fixture's root;
						// coordinates select a test fixture edge only, never a saved identity.
						props, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "EDGE", edge.LocalID)
						if e != nil {
							t.Fatal(e)
						}
						if props.PersistentSelection == nil {
							continue
						}
						evidence := props.PersistentSelection.CreationEvidence
						if evidence.GeometryType == "LINE" && math.Abs(evidence.Centroid[0]+60) < 1e-6 && math.Abs(evidence.Centroid[1]-10) < 1e-6 && math.Abs(evidence.Centroid[2]-35) < 1e-6 {
							rootID = edge.LocalID
							break
						}
					}
					if rootID != 0 {
						break
					}
				}
				if rootID == 0 {
					t.Fatal("unrounded opposite root absent")
				}
				props, e := service.GetTopologyElementPropertiesAtVersion(t.Context(), view.Document.ID, view.Document.VersionID, activeBodyArtifact(t, view).GeometryKey, "EDGE", rootID)
				if e != nil {
					t.Fatal(e)
				}
				apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: "FILLET", BodyID: base.BodyID, Length: 80, Selections: []workspace.FeatureSelection{{Selection: *props.PersistentSelection, SourceVersionID: view.Document.VersionID, SourceFeatureID: pattern.ID}}}})
				second := view.Part.Features[len(view.Part.Features)-1]
				_, paired := load()
				firstMember, secondMember := false, false
				for _, assoc := range paired.FeatureAssociations.Elements {
					if assoc.Kind != "FACE" {
						continue
					}
					if slices.Contains(assoc.Origins, fillet.ID) {
						firstMember = true
					}
					if slices.Contains(assoc.Origins, second.ID) {
						for _, m := range assoc.Members {
							if m.PatternID == pattern.ID && m.Slot == 1 {
								secondMember = true
							}
						}
					}
				}
				if !firstMember || !secondMember {
					t.Fatal("second fillet lost first blend or member identity")
				}
				if !asymmetric && math.Abs(activeBodyArtifact(t, view).Volume-(2*seedVolume-baseVolume)) > 1e-4 {
					t.Fatal("second root fillet differs from transformed first root")
				}
				apply(workspace.CommandRequest{Type: "UNDO"})
				apply(workspace.CommandRequest{Type: "REDO"})
				return
			}
			_, after := load()
			seedBlend, copyBlend := false, false
			for _, e := range after.FeatureAssociations.Elements {
				if e.Kind != "FACE" || !slices.Contains(e.Origins, fillet.ID) {
					continue
				}
				if len(e.Members) == 0 {
					seedBlend = true
				}
				for _, m := range e.Members {
					if m.PatternID == pattern.ID && m.Slot == 1 {
						copyBlend = true
					}
				}
			}
			if !seedBlend || !copyBlend {
				t.Fatalf("fillet source/member lost: seed=%v copy=%v", seedBlend, copyBlend)
			}
			edit := func(f workspace.Feature) {
				t.Helper()
				var digest string
				var visit func(workspace.DocumentStructureNode)
				visit = func(n workspace.DocumentStructureNode) {
					if n.EntityID == f.ID && n.PresentationRole != "INPUT_REFERENCE" {
						digest = n.DefinitionDigest
					}
					for _, c := range n.Children {
						visit(c)
					}
				}
				visit(*view.StructureTree)
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, ExpectedFeatureDigest: digest, Feature: &f})
			}
			fillet.Length = 70
			edit(fillet)
			blade.Length = 125
			edit(blade)
			// The end-stage source stays explicit while the accepted range recomputes.
			stage, e := service.GetFeatureInput(t.Context(), view.Document.ID, workspace.FeatureInputRequest{VersionID: view.Document.VersionID, FeatureID: fillet.ID, ResultStage: true})
			if e != nil {
				t.Fatal(e)
			}
			if math.Abs(activeBodyArtifact(t, view).Volume-(2*stage.Artifact.Volume-baseVolume)) > 1e-4 {
				t.Fatal("radius/seed edit did not recompute material range")
			}
			pattern.Pattern.Count = 3
			pattern.Pattern.Angle = 120
			edit(pattern)
			_, changed := load()
			member2 := false
			for _, e := range changed.FeatureAssociations.Elements {
				for _, m := range e.Members {
					if m.PatternID == pattern.ID && m.Slot == 2 {
						member2 = true
					}
				}
			}
			if !member2 {
				t.Fatal("new member slot missing after count edit")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			apply(workspace.CommandRequest{Type: "REDO"})
			cold := workspace.NewWithArtifacts(db, client, artifacts)
			reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			if reopened.Document.VersionID != view.Document.VersionID || activeBodyArtifact(t, reopened).GeometryKey != activeBodyArtifact(t, view).GeometryKey {
				t.Fatal("cold read changed range snapshot")
			}
		})
	}
}

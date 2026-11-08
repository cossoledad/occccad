//go:build integration

package control

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestThinFeaturesThroughRouter(t *testing.T) {
	for _, shape := range []string{"CIRCLE", "NESTED_CIRCLES", "OPEN_MITER", "OPEN_ROUND"} {
		t.Run(shape, func(t *testing.T) {
			service, artifacts, db, client, _ := featureAssociationRuntimeService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: shape + " thin acceptance"})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				if r.RequestID == "" {
					r.RequestID = fmt.Sprintf("%s-thin-%d", view.Document.ID, seq)
				}
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			volume := func(want float64) {
				t.Helper()
				if got := activeBodyArtifact(t, view).Volume; math.Abs(got-want) > 1e-5 {
					t.Fatal("thin volume", got, want)
				}
			}
			saved := func(id string) workspace.Feature {
				for _, f := range view.Part.Features {
					if f.ID == id {
						return f
					}
				}
				t.Fatal("feature missing")
				return workspace.Feature{}
			}
			apply(workspace.CommandRequest{Type: "CREATE_PARAMETER", Name: "wall", Value: 2, Unit: "mm"})
			wallID := view.Part.Parameters[0].ParameterID
			if strings.HasPrefix(shape, "OPEN_") {
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
				stock := view.Part.Features[len(view.Part.Features)-1].ID
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: stock, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -5, Y: -5}, Second: &workspace.SketchPoint2{X: 15, Y: 15}}}})
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: stock, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := view.Part.Features[len(view.Part.Features)-1].ID
			thin := &workspace.ThinWallDefinition{Input: "REGIONS", Mode: "ONE_SIDED", Side: "LEFT", Join: "MITER", Thickness: 1}
			operation := "ADD"
			want := 180 * math.Pi
			if shape == "CIRCLE" || shape == "NESTED_CIRCLES" {
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}}}})
				if shape == "NESTED_CIRCLES" {
					apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "inner", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 9}}}})
					thin.Mode = "SYMMETRIC"
					want = 285 * math.Pi
				}
			} else {
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{}, End: &workspace.SketchPoint2{X: 10}}}, {Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 10}, End: &workspace.SketchPoint2{X: 10, Y: 10}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "join", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}}}}})
				thin.Input = "CHAINS"
				thin.Mode = "SYMMETRIC"
				thin.EntityIDs = []string{"a", "b"}
				operation = "REMOVE"
				want = 3800
				if shape == "OPEN_ROUND" {
					thin.Join = "ROUND"
					want = 4000 - 5*(39+math.Pi/4)
				}
			}
			request := workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-thin-preview", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: operation, Length: 5, ThinWall: thin, ParameterExpressions: map[string]string{"thin:thickness": "wall"}}
			head := view.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), view.Document.ID, request)
			if e != nil || preview.Artifact == nil {
				t.Fatal("thin preview", e)
			}
			read, e := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil || read.Document.VersionID != head {
				t.Fatal("preview advanced Head", e)
			}
			request.PreviewID = preview.PreviewID
			apply(request)
			feature := view.Part.Features[len(view.Part.Features)-1]
			volume(want)
			if shape == "CIRCLE" {
				apply(workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: wallID, Value: 3, Unit: "mm"})
				want = 255 * math.Pi
				volume(want)
				apply(workspace.CommandRequest{Type: "UNDO"})
				volume(180 * math.Pi)
				apply(workspace.CommandRequest{Type: "REDO"})
				volume(want)
				feature = saved(feature.ID)
				definition := *feature.ThinWall
				feature.ThinWall = &definition
				feature.ThinWall.Mode = "TWO_SIDED"
				feature.ThinWall.Thickness2 = 1
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
				want = 360 * math.Pi
				volume(want)
			} else if shape == "OPEN_MITER" {
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "0", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: 10, Y: 10}, End: &workspace.SketchPoint2{Y: 10}}}, {Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "extension", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "b", SubElement: "END"}, {Target: "ENTITY", EntityID: "0", SubElement: "START"}}}}}})
				extended := saved(feature.ID)
				direction := *extended.ThinWall
				extended.ThinWall = &direction
				extended.ThinWall.Mode = "ONE_SIDED"
				extended.ThinWall.EntityIDs = []string{"a", "b", "0"}
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: extended.ID, Feature: &extended, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, extended.ID)})
				volume(3740)
				apply(workspace.CommandRequest{Type: "UNDO"})
				volume(3800)
				apply(workspace.CommandRequest{Type: "UNDO"})
				volume(3800)
				feature = saved(feature.ID)
				feature.Extent = "TO_NEXT"
				feature.StartOffset = 2
				feature.EndOffset = -1
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
				want = 3720
				volume(want)
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"b"}}}})
				feature = saved(feature.ID)
				if feature.EvaluationStatus != "FAILED" || !strings.Contains(feature.Diagnostic, "CURVE_CHAIN_SOURCE_MISSING") {
					t.Fatal("lost chain was not retained", feature)
				}
				rejected, e := service.ApplyCommand(t.Context(), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-thin-unresolved", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
				if e == nil {
					t.Fatal("missing thin chain accepted", rejected.Document.VersionID)
				}
				definition := *feature.ThinWall
				feature.ThinWall = &definition
				feature.ThinWall.EntityIDs = []string{"a"}
				apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
				want = 3860
				volume(want)
			}
			final := activeBodyArtifact(t, view)
			fresh := workspace.NewWithArtifacts(db, client, artifacts)
			view, e = fresh.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			volume(want)
			feature = saved(feature.ID)
			cold, e := fresh.PreviewCommand(workspace.WithColdEvaluation(t.Context()), view.Document.ID, workspace.CommandRequest{Type: "EDIT_FEATURE", ActorID: p6Actor, RequestID: view.Document.ID + "-thin-cold", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
			if e != nil || cold.Artifact == nil {
				t.Fatal("thin cold rebuild", e)
			}
			for _, role := range []string{"BREP", "NAMING"} {
				if final.Representations[role].Digest != cold.Artifact.Representations[role].Digest {
					t.Fatal("cold thin mismatch", role)
				}
			}
		})
	}
}

func TestThinModeRetainsUnchangedSourceBoundaryThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Thin unchanged boundary"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-thin-boundary-%d", view.Document.ID, seq)
		view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: 10}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
	feature := view.Part.Features[len(view.Part.Features)-1]
	side := findP7TopologyPick(t, service, view, "FACE", func(p workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
		return p.GeometryType == "CYLINDER"
	})
	cap := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
		return strings.HasPrefix(p.Anchor.OutputSlot, "END_CAP/")
	})
	feature.ThinWall = &workspace.ThinWallDefinition{Input: "REGIONS", Mode: "ONE_SIDED", Side: "LEFT", Join: "MITER", Thickness: 2}
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: feature.ID, Feature: &feature, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, feature.ID)})
	for _, pick := range []p7TopologyPick{side, cap} {
		resolution, e := service.ResolvePersistentSelection(t.Context(), view.Document.ID, workspace.ResolvePersistentSelectionRequest{Selection: pick.Selection, SourceVersionID: pick.SourceVersionID, TargetVersionID: view.Document.VersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if e != nil || resolution.Status != modelcore.SelectionResolved {
			t.Fatal("unchanged outer boundary lost identity", pick.Selection.Anchor.OutputSlot, resolution, e)
		}
	}
}

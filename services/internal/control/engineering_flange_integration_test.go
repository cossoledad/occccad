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

// Exercise the same versioned commands and B-Rep artifacts as a workbench edit.
func TestEngineeringFlangeThroughRouter(t *testing.T) {
	for _, operation := range []string{"StandardHole", "Rib", "DraftedRib", "SplitFace", "TwoDistances", "DistanceAngle"} {
		t.Run(operation, func(t *testing.T) {
			service, _, _, _, _ := featureAssociationRuntimeService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Flange " + operation})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-flange-%d", view.Document.ID, seq)
				view, err = service.ApplyCommand(t.Context(), view.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			last := func() workspace.Feature { return view.Part.Features[len(view.Part.Features)-1] }
			measure := func(want float64) {
				t.Helper()
				if got := measuredArtifactVolume(t, activeBodyArtifact(t, view)); math.Abs(got-want) > 1e-4 {
					t.Fatalf("material volume %g, want %g", got, want)
				}
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			profile := last().ID
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -20, Y: -20}, Second: &workspace.SketchPoint2{X: 20, Y: 20}}}})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 10})
			base := last()
			pick := func(kind, prefix string) workspace.FeatureSelection {
				t.Helper()
				match := findP7TopologyPick(t, service, view, kind, func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
					return p.Anchor.FeatureID == base.ID && strings.HasPrefix(p.Anchor.OutputSlot, prefix)
				})
				return workspace.FeatureSelection{SourceVersionID: match.SourceVersionID, Selection: match.Selection}
			}
			if operation == "SplitFace" || operation == "TwoDistances" || operation == "DistanceAngle" {
				top := pick("FACE", "END_CAP/")
				f := workspace.Feature{BodyID: base.BodyID, Selections: []workspace.FeatureSelection{top}}
				if operation == "SplitFace" {
					f.Type = "SPLIT_FACE"
					apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}})
					f.NeutralPlaneID = view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
					apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &f})
					measure(16000)
					id := last().ID
					for _, side := range []string{"/POSITIVE", "/NEGATIVE"} {
						sideFace := findP7TopologyPick(t, service, view, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
							return p.Anchor.FeatureID == id && strings.HasPrefix(p.Anchor.OutputSlot, "PLANE_SPLIT/") && strings.HasSuffix(p.Anchor.OutputSlot, side)
						})
						if side == "/NEGATIVE" {
							apply(workspace.CommandRequest{Type: "CREATE_SKETCH", TargetKind: "FACE", TopologyID: sideFace.LocalID, GeometryKey: sideFace.GeometryKey, VersionID: sideFace.SourceVersionID})
							supportID := last().ID
							apply(workspace.CommandRequest{Type: "EDIT_DATUM_PLANE", TargetID: f.NeutralPlaneID, Origin: [3]float64{5, 0, 0}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}})
							if last().ID != supportID || last().Sketch.Support.Status == "FAILED_SUPPORT" {
								t.Fatal("lost split face sketch support")
							}
							measure(16000)
						}
					}
					return
				}
				f.Type = "CHAMFER"
				f.Length = 1
				f.Selections = append(f.Selections, pick("EDGE", "END_BOUNDARY_FROM_PROFILE_EDGE/"))
				second := 2.
				if operation == "TwoDistances" {
					f.ChamferMode = "TWO_DISTANCES"
					f.Length2 = second
				} else {
					f.ChamferMode = "DISTANCE_ANGLE"
					f.Angle = 30
					second = math.Tan(math.Pi / 6)
				}
				apply(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &f})
				measure(16000 - 40*second/2)
				findP7TopologyPick(t, service, view, "VERTEX", func(props workspace.TopologyElementProperties, _ modelcore.PersistentSelection) bool {
					return math.Abs(queriedTopologyCenter(t, props)[2]-(10-second)) < 1e-6
				})
				return
			}
			if operation == "DraftedRib" {
				// Machine a 6 mm deep tapered recess. Its width grows from 20 to 26 mm.
				var sections []workspace.LoftSection
				for _, station := range []struct{ z, r float64 }{{4, 10}, {10, 13}} {
					apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, station.z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
					apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
					id := last().ID
					apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -station.r, Y: -station.r}, Second: &workspace.SketchPoint2{X: station.r, Y: station.r}}}})
					sections = append(sections, workspace.LoftSection{SketchID: id})
				}
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", BodyID: base.BodyID, Operation: "REMOVE", Sections: sections, Ruled: true}})
				measure(16000 - 3192)
			}
			z := 10.
			if operation == "DraftedRib" {
				z = 8
			}
			if operation == "Rib" {
				z = 16
			}
			apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, z}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
			sketch := last().ID
			line := workspace.SketchEntity{ID: "centerline", Kind: "LINE", Role: "PROFILE", StartPointID: "a", EndPointID: "b", Start: &workspace.SketchPoint2{X: -10}, End: &workspace.SketchPoint2{X: 10}}
			if operation == "Rib" || operation == "DraftedRib" {
				if operation == "DraftedRib" {
					line.Start.X = -21
					line.End.X = 21
				}
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &line}}})
				apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, BodyID: base.BodyID, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Extent: "TO_NEXT", Reversed: true, ThinWall: &workspace.ThinWallDefinition{Rib: true, Input: "CHAINS", Mode: "SYMMETRIC", Join: "MITER", Side: "LEFT", Thickness: 2, EntityIDs: []string{line.ID}}})
				if operation == "DraftedRib" {
					measure(16000 - 3192 + 176)
				} else {
					measure(16000 + 20*6*2)
				}
				return
			}
			line.Role = "CONSTRUCTION"
			circle := workspace.SketchEntity{ID: "boss-circle", Kind: "CIRCLE", Role: "CONSTRUCTION", Center: &workspace.SketchPoint2{X: 5, Y: 5}, Radius: 3}
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &line}, {Type: "ADD_ENTITY", Entity: &circle}}})
			refs := []workspace.PatternPointReference{{SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: line.ID, SubElement: "START", PointID: line.StartPointID}}, {SketchID: sketch, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: circle.ID, SubElement: "CENTER"}}}
			f := workspace.Feature{Type: "HOLE", BodyID: base.BodyID, Operation: "REMOVE", Extent: "THROUGH_ALL", Reversed: true, HoleStandard: "ISO273_MEDIUM_M4", HolePoints: refs}
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &f})
			measure(16000 - 20*math.Pi*2.25*2.25)
			f = last()
			f.HoleStandard = "ISO273_MEDIUM_M6"
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: f.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, f.ID), Feature: &f})
			measure(16000 - 20*math.Pi*3.3*3.3)
			for _, p := range view.Part.Parameters {
				if p.OwnerFeatureID == f.ID && p.PropertySlot == "diameter" {
					if p.Role != "OUTPUT" || p.EvaluatedValue == nil || math.Abs(p.EvaluatedValue.SIValue-.0066) > 1e-10 {
						t.Fatal("catalog diameter not projected", p)
					}
					apply(workspace.CommandRequest{Type: "UNDO"})
					measure(16000 - 20*math.Pi*2.25*2.25)
					apply(workspace.CommandRequest{Type: "REDO"})
					measure(16000 - 20*math.Pi*3.3*3.3)
					return
				}
			}
			t.Fatal("catalog diameter parameter missing")
		})
	}
}

func TestGearboxSplitThroughRouter(t *testing.T) {
	s, _, _, _, _ := featureAssociationRuntimeService(t)
	ctx := t.Context()
	view, err := s.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Gearbox split"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	request := func(r workspace.CommandRequest) workspace.CommandRequest {
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-gearbox-%d", view.Document.ID, seq)
		return r
	}
	commit := func(r workspace.CommandRequest) {
		t.Helper()
		v, e := s.ApplyCommand(ctx, view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = v
	}
	apply := func(r workspace.CommandRequest) { t.Helper(); commit(request(r)) }
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketch := view.Part.Features[0].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: 80, Y: 60}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: sketch, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 30})
	original := view.Part.ActiveBodyID
	plane := workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{25, 0, 0}, Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}}
	apply(plane)
	plane.TargetID = view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
	plane.Type = "EDIT_DATUM_PLANE"
	def := workspace.Feature{Type: "SPLIT_BODY", BodyID: original, SplitKeepSide: "POSITIVE", NeutralPlaneID: plane.TargetID}
	r := request(workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &def})
	preview, e := s.PreviewCommand(ctx, view.Document.ID, r)
	if e != nil {
		t.Fatal("preview", e)
	}
	if preview.SplitResultArtifact == nil || preview.Artifact == nil {
		t.Fatal("missing split preview")
	}
	secondary := preview.SplitResultArtifact.BodyID
	closeVolume := func(a workspace.Artifact, want float64) {
		t.Helper()
		if got := measuredArtifactVolume(t, a); math.Abs(got-want) > 1e-4 {
			t.Fatalf("volume %.8g, expected %g", got, want)
		}
	}
	closeVolume(*preview.Artifact, 55*60*30)
	closeVolume(*preview.SplitResultArtifact, 25*60*30)
	r.PreviewID = preview.PreviewID
	commit(r)
	check := func(positiveWidth, negativeWidth float64) {
		t.Helper()
		if view.Part.ActiveBodyID != original || len(view.Part.Bodies) != 2 {
			t.Fatal("wrong output Bodies", view.Part.Bodies)
		}
		for _, b := range view.Part.Bodies {
			a, ok := view.Artifacts[b.GeometryKey]
			if !ok {
				t.Fatalf("missing artifact for %s (%s)", b.ID, b.GeometryKey)
			}
			switch b.ID {
			case original:
				closeVolume(a, positiveWidth*60*30)
			case secondary:
				closeVolume(a, negativeWidth*60*30)
			default:
				t.Fatal("split changed Body ID", b.ID)
			}
		}
	}
	check(55, 25)
	parent := view.Part.Features[len(view.Part.Features)-2]
	if parent.SplitBodyID != secondary || parent.SplitKeepSide != "POSITIVE" {
		t.Fatal(parent)
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(view.Part.Bodies) != 1 {
		t.Fatal("undo retained sibling")
	}
	closeVolume(activeBodyArtifact(t, view), 80*60*30)
	apply(workspace.CommandRequest{Type: "REDO"})
	check(55, 25)
	plane.Origin[0] = 35
	apply(plane)
	check(45, 35)
	parent.SplitKeepSide = "NEGATIVE"
	apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: parent.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, parent.ID), Feature: &parent})
	check(35, 45)
	view, err = s.GetDocument(ctx, view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	check(35, 45)
	// Suppression removes both products from evaluation and restores the stock.
	setSuppressed := func(flag bool) {
		apply(workspace.CommandRequest{Type: "SET_FEATURE_SUPPRESSION", TargetID: parent.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, parent.ID), Suppressed: &flag})
	}
	setSuppressed(true)
	closeVolume(activeBodyArtifact(t, view), 80*60*30)
	for _, f := range view.Part.Features {
		if f.SplitParentID == parent.ID && !f.Suppressed {
			t.Fatal("split sibling remained active")
		}
	}
	setSuppressed(false)
	check(35, 45)
	apply(workspace.CommandRequest{Type: "DELETE_NODES", Targets: []workspace.DeleteNodeTarget{{TargetKind: "FEATURE", TargetID: parent.ID}}})
	if len(view.Part.Bodies) != 1 {
		t.Fatal("deletion left an empty split output Body")
	}
	closeVolume(activeBodyArtifact(t, view), 80*60*30)
	apply(workspace.CommandRequest{Type: "UNDO"})
	check(35, 45)
	// Verify that each machining face can be picked by semantic identity.
	for _, id := range []string{original, secondary} {
		apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: id})
		cut := findP7TopologyPick(t, s, view, "FACE", func(_ workspace.TopologyElementProperties, p modelcore.PersistentSelection) bool {
			return p.SourceBodyID == id && p.Anchor.OutputSlot == "SPLIT_SECTION"
		})
		if math.Abs(cut.Center[0]-35) > 1e-5 {
			t.Fatal("cut face not on parting plane", cut)
		}
	}
	apply(workspace.CommandRequest{Type: "SET_ACTIVE_BODY", BodyID: original})
	// Drill one maintenance hole in each housing half. Each hole owns a
	// separate stable reference even when both consume the same drilling setup.
	apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Origin: [3]float64{0, 0, 30}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
	layout := view.Part.Features[len(view.Part.Features)-1].ID
	for j, body := range []string{original, secondary} {
		point := fmt.Sprintf("bore-%d", j)
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: layout, Operations: []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: point, Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 10 + 50*float64(j), Y: 25}}}}})
		ref := workspace.PatternPointReference{SketchID: layout, Reference: workspace.SketchGeometryRef{Target: "ENTITY", EntityID: point, SubElement: "POINT"}}
		apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "HOLE", BodyID: body, Operation: "REMOVE", Extent: "THROUGH_ALL", Reversed: true, Diameter: 4, HolePoint: &ref}})
	}
	plane.Origin[0] = 28
	apply(plane)
	for _, b := range view.Part.Bodies {
		width := 52.
		if b.ID == original {
			width = 28
		}
		closeVolume(view.Artifacts[b.GeometryKey], width*60*30-30*4*math.Pi)
	}
	head := view.Document.VersionID
	bad := parent
	bad.NeutralPlaneID = "datum-yz"
	invalid := request(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: parent.ID, ExpectedFeatureDigest: findP6FeatureDigest(t, view.StructureTree, parent.ID), Feature: &bad})
	if _, e := s.PreviewCommand(ctx, view.Document.ID, invalid); e == nil {
		t.Fatal("accepted tangent split preview")
	}
	view, err = s.GetDocument(ctx, view.Document.ID, p6Actor)
	if err != nil || view.Document.VersionID != head {
		t.Fatal("failed preview changed head", err)
	}

}

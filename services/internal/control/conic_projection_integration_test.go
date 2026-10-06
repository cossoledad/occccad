package control

import (
	"fmt"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestConicProjectionLifecycleThroughRouter(t *testing.T) {
	for _, tc := range []struct {
		name, source, result string
		oblique              bool
	}{
		{"circle", "CIRCLE", "CIRCLE", false},
		{"arc", "ARC", "ARC", false},
		{"oblique-arc", "ARC", "ELLIPTICAL_ARC", true},
		{"ellipse", "ELLIPSE", "ELLIPSE", false},
		{"elliptical-arc", "ELLIPTICAL_ARC", "ELLIPTICAL_ARC", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, artifacts, db, client := featureAssociationTestService(t)
			view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: tc.name})
			if err != nil {
				t.Fatal(err)
			}
			seq := 0
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				seq++
				r.ActorID = p6Actor
				r.RequestID = fmt.Sprintf("%s-conic-%d", view.Document.ID, seq)
				v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
				if e != nil {
					t.Fatal(r.Type, e)
				}
				view = v
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			source := view.Part.Features[len(view.Part.Features)-1].ID
			entity := workspace.SketchEntity{ID: "source-conic", Kind: tc.source, Role: "PROFILE", Center: &workspace.SketchPoint2{X: 3, Y: 4}, Radius: 10, MajorRadius: 10, MinorRadius: 4, StartAngle: 0, EndAngle: math.Pi}
			ops := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &entity}}
			if tc.source == "ARC" || tc.source == "ELLIPTICAL_ARC" {
				ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "closure", Kind: "LINE", Role: "PROFILE", Start: &workspace.SketchPoint2{X: -7, Y: 4}, End: &workspace.SketchPoint2{X: 13, Y: 4}}})
			}
			if tc.source == "ARC" || tc.source == "ELLIPTICAL_ARC" {
				for _, pair := range []struct{ arc, line string }{{"START", "END"}, {"END", "START"}} {
					ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "closure-" + pair.arc, Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: entity.ID, SubElement: pair.arc}, {Target: "ENTITY", EntityID: "closure", SubElement: pair.line}}}})
				}
			}
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: source, Operations: ops})
			apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: source, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 5})
			solid := view.Part.Features[len(view.Part.Features)-1]
			family := "CIRCLE"
			if tc.source == "ELLIPSE" || tc.source == "ELLIPTICAL_ARC" {
				family = "ELLIPSE"
			}
			pick := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
				return s.CreationEvidence.GeometryType == family && math.Abs(s.CreationEvidence.Origin[2]) < 1e-6
			})
			if pick.Selection.CreationEvidence.RadiusMM == nil {
				t.Fatal("radius evidence lost")
			}
			if family == "ELLIPSE" && pick.Selection.CreationEvidence.MinorRadiusMM == nil {
				t.Fatal("ellipse radius evidence lost in Naming")
			}
			if tc.oblique {
				apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Normal: [3]float64{0, math.Sqrt(.75), .5}, UDirection: [3]float64{1, 0, 0}})
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID})
			} else {
				apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			}
			sketch := view.Part.Features[len(view.Part.Features)-1].ID
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "projection", TopologyKind: "EDGE", TopologyID: pick.LocalID, GeometryKey: pick.GeometryKey, SourceVersionID: pick.SourceVersionID}}})
			check := func(v workspace.DocumentView) {
				t.Helper()
				for _, f := range v.Part.Features {
					if f.ID != sketch {
						continue
					}
					e := f.Sketch.ExternalGeometry[0]
					if e.Status != "CONNECTED" || e.Snapshot == nil || e.Snapshot.Kind != tc.result {
						t.Fatalf("projection: %+v", e)
					}
					if e.Snapshot.Center == nil || math.Abs(e.Snapshot.Center.X-3) > 1e-8 {
						t.Fatal("center lost", e.Snapshot)
					}
					if tc.oblique && math.Abs(e.Snapshot.MinorRadius-5) > 1e-8 {
						t.Fatal("incorrect oblique radius", e.Snapshot)
					}
					if len(f.Sketch.Entities) > 0 {
						p := f.Sketch.Entities[0].Point
						if p == nil || math.Hypot(p.X-e.Snapshot.Center.X, p.Y-e.Snapshot.Center.Y) > 1e-6 {
							t.Fatal("external center is not solver input", p)
						}
					}
					return
				}
				t.Fatal("sketch absent")
			}
			check(view)
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{
				{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{}}},
				{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "center-reference", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "point", SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: "projection", SubElement: "CENTER"}}}},
			}})
			check(view)
			// Rebind against regenerated upstream topology, retaining ExternalId
			// and the center constraint instead of consuming the old snapshot.
			var digest string
			var visit func(workspace.DocumentStructureNode)
			visit = func(n workspace.DocumentStructureNode) {
				if n.EntityID == solid.ID && n.DefinitionDigest != "" {
					digest = n.DefinitionDigest
				}
				for _, c := range n.Children {
					visit(c)
				}
			}
			visit(*view.StructureTree)
			solid.Length = 7
			apply(workspace.CommandRequest{Type: "EDIT_FEATURE", TargetID: solid.ID, ExpectedFeatureDigest: digest, Feature: &solid})
			check(view)
			apply(workspace.CommandRequest{Type: "UNDO"})
			apply(workspace.CommandRequest{Type: "REDO"})
			check(view)
			cold := workspace.NewWithArtifacts(db, client, artifacts)
			reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
			if e != nil {
				t.Fatal(e)
			}
			check(reopened)
			apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DETACH_EXTERNAL_GEOMETRY", ExternalID: "projection"}}})
			detached := view.Part.Features[len(view.Part.Features)-1].Sketch
			if len(detached.ExternalGeometry) != 0 || detached.Entities[len(detached.Entities)-1].Kind != tc.result {
				t.Fatal("detach lost conic")
			}
			apply(workspace.CommandRequest{Type: "UNDO"})
			check(view)
		})
	}
}

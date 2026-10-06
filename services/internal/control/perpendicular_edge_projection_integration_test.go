package control

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestPerpendicularEdgeProjectionThroughRouter(t *testing.T) {
	service, artifacts, db, client := featureAssociationTestService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Perpendicular edge projection"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		r.RequestID = fmt.Sprintf("%s-project-%d", view.Document.ID, seq)
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = v
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	profile := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: profile, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: 2, Y: 3}, Second: &workspace.SketchPoint2{X: 8, Y: 9}}}})
	apply(workspace.CommandRequest{Type: "CREATE_SOLID_FEATURE", SketchID: profile, Generator: "LINEAR_EXTRUDE", Operation: "ADD", Length: 40})
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[len(view.Part.Features)-1].ID
	pick := findP7TopologyPick(t, service, view, "EDGE", func(_ workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return s.CreationEvidence.GeometryType == "LINE" && math.Abs(s.CreationEvidence.Direction[2]) > .99
	})
	evidence := pick.Selection.CreationEvidence
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "projected-point", TopologyKind: "EDGE", TopologyID: pick.LocalID, GeometryKey: pick.GeometryKey, SourceVersionID: pick.SourceVersionID}}})
	check := func(v workspace.DocumentView) {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.ID != sketchID {
				continue
			}
			if len(f.Sketch.ExternalGeometry) != 1 {
				t.Fatal("missing projection")
			}
			e := f.Sketch.ExternalGeometry[0]
			if e.Status != "CONNECTED" || e.Snapshot == nil || e.Snapshot.Kind != "POINT" || e.PersistentSelection.ExpectedType != "EDGE" {
				t.Fatalf("projection lost original edge identity: %+v", e)
			}
			if math.Abs(e.Snapshot.Point.X-evidence.Centroid[0]) > 1e-8 || math.Abs(e.Snapshot.Point.Y-evidence.Centroid[1]) > 1e-8 {
				t.Fatal("incorrect orthogonal point", e.Snapshot)
			}
			return
		}
		t.Fatal("missing sketch")
	}
	check(view)
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(view.Part.Features[len(view.Part.Features)-1].Sketch.ExternalGeometry) != 0 {
		t.Fatal("undo retained projection")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	check(view)
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{
		{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: "constrained-point", Kind: "POINT", Role: "CONSTRUCTION", Point: &workspace.SketchPoint2{X: 0, Y: 0}}},
		{Type: "ADD_CONSTRAINT", Constraint: &workspace.SketchConstraint{ID: "coincident-projection", Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: "constrained-point", SubElement: "POINT"}, {Target: "EXTERNAL", EntityID: "projected-point", SubElement: "POINT"}}}},
	}})
	solved := view.Part.Features[len(view.Part.Features)-1].Sketch.Entities[0].Point
	if solved == nil || math.Abs(solved.X-evidence.Centroid[0]) > 1e-6 || math.Abs(solved.Y-evidence.Centroid[1]) > 1e-6 {
		t.Fatal("projected point is not usable as solver input", solved)
	}
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	reopened, e := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if e != nil {
		t.Fatal(e)
	}
	check(reopened)
}

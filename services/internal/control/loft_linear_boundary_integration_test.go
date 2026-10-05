package control

import (
	"fmt"
	"math"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

func TestLoftLinearBoundaryProjectionThroughRouter(t *testing.T) {
	service, artifacts, db, client := featureAssociationTestService(t)
	view, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Linear loft boundaries"})
	if err != nil {
		t.Fatal(err)
	}
	seq := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = p6Actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-linear-loft-%d", view.Document.ID, seq)
		}
		v, e := service.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(r.Type, e)
		}
		view = v
		for _, f := range view.Part.Features {
			if f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
				t.Fatal(f.Diagnostic)
			}
		}
	}
	var sections []workspace.LoftSection
	for i := 0; i < 3; i++ {
		apply(workspace.CommandRequest{Type: "CREATE_DATUM_PLANE", Name: fmt.Sprintf("section %d", i), Origin: [3]float64{0, 0, float64(i * 10)}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
		plane := view.Part.DatumPlanes[len(view.Part.DatumPlanes)-1].ID
		apply(workspace.CommandRequest{Type: "CREATE_SKETCH", DatumPlaneID: plane})
		id := view.Part.Features[len(view.Part.Features)-1].ID
		width, height := float64(5-i), float64(3-i)
		apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: id, Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{X: -width, Y: -height}, Second: &workspace.SketchPoint2{X: width, Y: height}}}})
		sections = append(sections, workspace.LoftSection{SketchID: id})
	}
	before := view.Document.VersionID
	r := workspace.CommandRequest{ActorID: p6Actor, RequestID: "linear-loft-preview", Type: "CREATE_SOLID_FEATURE", Feature: &workspace.Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: sections}}
	preview, err := service.PreviewCommand(t.Context(), view.Document.ID, r)
	if err != nil {
		t.Fatal(err)
	}
	reread, err := service.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil || reread.Document.VersionID != before {
		t.Fatal("preview changed Head", err)
	}
	r.PreviewID = preview.PreviewID
	apply(r)
	pick := findP7TopologyPick(t, service, view, "EDGE", func(p workspace.TopologyElementProperties, s modelcore.PersistentSelection) bool {
		return p.GeometryType == "LINE" && p.Properties["underlyingCurveType"] == "BSPLINE_CURVE" && math.Abs(s.CreationEvidence.Centroid[2]) < 1e-6
	})
	evidence := pick.Selection.CreationEvidence
	if evidence.ParameterStart == nil || evidence.ParameterEnd == nil {
		t.Fatal("missing linear interval")
	}
	if *evidence.ParameterEnd-*evidence.ParameterStart < 1 || math.Abs(evidence.Direction[2]) > 1e-6 {
		t.Fatal("invalid linear interval/direction", evidence)
	}
	apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
	sketchID := view.Part.Features[len(view.Part.Features)-1].ID
	apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketchID, Operations: []workspace.SketchOperation{{Type: "ADD_EXTERNAL_GEOMETRY", ExternalID: "loft-line", TopologyKind: "EDGE", TopologyID: pick.LocalID, GeometryKey: pick.GeometryKey, SourceVersionID: pick.SourceVersionID}}})
	check := func(v workspace.DocumentView) {
		t.Helper()
		for _, f := range v.Part.Features {
			if f.ID == sketchID {
				if len(f.Sketch.ExternalGeometry) != 1 {
					t.Fatal("missing projection")
				}
				e := f.Sketch.ExternalGeometry[0]
				if e.Status != "CONNECTED" || e.Snapshot == nil || e.Snapshot.Kind != "LINE" {
					t.Fatalf("projection: %+v", e)
				}
				got := math.Hypot(e.Snapshot.End.X-e.Snapshot.Start.X, e.Snapshot.End.Y-e.Snapshot.Start.Y)
				if math.Abs(got-(*evidence.ParameterEnd-*evidence.ParameterStart)) > 1e-6 {
					t.Fatal("native spline parameter used as distance", got)
				}
				return
			}
		}
		t.Fatal("missing sketch")
	}
	check(view)
	apply(workspace.CommandRequest{Type: "UNDO"})
	apply(workspace.CommandRequest{Type: "REDO"})
	check(view)
	cold := workspace.NewWithArtifacts(db, client, artifacts)
	reopened, err := cold.GetDocument(t.Context(), view.Document.ID, p6Actor)
	if err != nil {
		t.Fatal(err)
	}
	check(reopened)
}

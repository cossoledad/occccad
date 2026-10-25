package control

import (
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"reflect"
	"testing"
)

func TestSplitCircleDeleteSegmentsKeepsExactClosedProfileThroughRouter(t *testing.T) {
	service, _, _, _, _ := featureAssociationRuntimeService(t)
	for _, mode := range []string{"viewport-batch", "tree-batch", "tree-sequential"} {
		t.Run(mode, func(t *testing.T) {
			v, err := service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: p6Actor, Type: "PART", Name: "Delete split junctions " + mode})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(r workspace.CommandRequest) {
				t.Helper()
				r.ActorID = p6Actor
				v, err = service.ApplyCommand(t.Context(), v.Document.ID, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
			}
			apply(workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			sketch := v.Part.Features[0].ID
			edit := func(ops ...workspace.SketchOperation) {
				t.Helper()
				apply(workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
			}
			small := workspace.SketchEntity{ID: "small", Name: "Circle.2", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: -1.974265603059874, Y: 58.381854261911535}, Radius: 42.94484989839571}
			large := workspace.SketchEntity{ID: "large", Name: "Circle.1", Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{X: 114.22536703417461, Y: -25.665452839777465}, Radius: 175.63152894299873}
			edit(workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &small}, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &large})
			hits, e := service.SketchIntersections(t.Context(), v.Document.ID, workspace.SketchIntersectionsRequest{VersionID: v.Document.VersionID, SketchID: sketch, EntityIDs: []string{"small", "large"}})
			if e != nil || len(hits.Intersections) != 2 {
				t.Fatal(e, hits)
			}
			params := append(append([]float64{}, hits.Intersections[0].Parameters[:]...), hits.Intersections[1].Parameters[:]...)
			edit(workspace.SketchOperation{Type: "SPLIT_INTERSECTION", OperationID: "split-delete", EntityIDs: []string{"small", "large"}, Parameters: params, Mode: "BOTH"})
			original := v.Part.Features[0].Sketch
			var child string
			for _, e := range original.Entities {
				if e.SourceEntityID == "small" && e.ID != "small" {
					child = e.ID
				}
			}
			if child == "" {
				t.Fatal("missing Arc.1")
			}
			request := workspace.CommandRequest{Type: "DELETE_NODES", Targets: []workspace.DeleteNodeTarget{{TargetKind: "SKETCH_ENTITY", TargetID: child, OwnerEntityID: sketch}, {TargetKind: "SKETCH_ENTITY", TargetID: "large", OwnerEntityID: sketch}}}
			if mode == "viewport-batch" {
				request = workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: []workspace.SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{child, "large"}}}}
			}
			head := v.Document.VersionID
			preview, e := service.PreviewCommand(t.Context(), v.Document.ID, workspace.CommandRequest{ActorID: p6Actor, Type: request.Type, Targets: request.Targets, SketchID: request.SketchID, Operations: request.Operations})
			if e != nil || preview.PreviewID == "" {
				t.Fatal("delete preview", e)
			}
			read, e := service.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if e != nil || read.Document.VersionID != head {
				t.Fatal("preview wrote Head", e)
			}
			if mode == "tree-sequential" {
				for _, target := range request.Targets {
					apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetKind: target.TargetKind, TargetID: target.TargetID, OwnerEntityID: sketch})
				}
			} else {
				request.PreviewID = preview.PreviewID
				apply(request)
			}
			closed := v.Part.Features[0].Sketch
			if len(closed.Entities) != 2 || len(closed.Constraints) != 2 {
				t.Fatal("surviving geometry/relations", closed)
			}
			analysis := v.SketchAnalyses[sketch]
			if analysis.Status != "CLOSED" || !analysis.GeometryVerified || analysis.RegionCount != 1 {
				t.Fatal("not an exact closed profile", analysis)
			}
			for _, hit := range hits.Intersections {
				assertSplitJunction(t, closed, hit.Point, 2, "")
			}
			undoCount := 1
			if mode == "tree-sequential" {
				undoCount = 2
			}
			for i := 0; i < undoCount; i++ {
				apply(workspace.CommandRequest{Type: "UNDO"})
			}
			if !reflect.DeepEqual(original, v.Part.Features[0].Sketch) {
				t.Fatal("undo deletion")
			}
			for i := 0; i < undoCount; i++ {
				apply(workspace.CommandRequest{Type: "REDO"})
			}
			if !reflect.DeepEqual(closed, v.Part.Features[0].Sketch) {
				t.Fatal("redo deletion")
			}
			read, e = service.GetDocument(t.Context(), v.Document.ID, p6Actor)
			if e != nil || read.SketchAnalyses[sketch].Status != "CLOSED" || !reflect.DeepEqual(closed, read.Part.Features[0].Sketch) {
				t.Fatal("reopen", e)
			}
			apply(workspace.CommandRequest{Type: "PAD_SKETCH", SketchID: sketch, Length: 2})
			d := math.Hypot(small.Center.X-large.Center.X, small.Center.Y-large.Center.Y)
			r, R := small.Radius, large.Radius
			overlap := r*r*math.Acos((d*d+r*r-R*R)/(2*d*r)) + R*R*math.Acos((d*d+R*R-r*r)/(2*d*R)) - .5*math.Sqrt((-d+r+R)*(d+r-R)*(d-r+R)*(d+r+R))
			want := 2 * (math.Pi*(r*r+R*R) - overlap)
			volume := 0.0
			for _, a := range v.Artifacts {
				volume += a.Volume
			}
			if math.Abs(volume-want) > 1e-5 {
				t.Fatal("exact closed extrusion volume", volume, want)
			}
		})
	}
}

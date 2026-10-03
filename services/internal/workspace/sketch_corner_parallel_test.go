package workspace

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/geometry"
)

func TestSketchCornerParallelRetainsFiniteCut(t *testing.T) {
	address := os.Getenv("OCCCCAD_TEST_WORKER_ADDRESS")
	if address == "" {
		t.Skip("matching-source Worker required")
	}
	worker, err := geometry.Open(address)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	data, err := os.ReadFile("testdata/corner_parallel.json")
	if err != nil {
		t.Fatal(err)
	}
	var sketch SketchFeature
	if err = json.Unmarshal(data, &sketch); err != nil {
		t.Fatal(err)
	}
	sketch.Constraints = append(sketch.Constraints, SketchConstraint{ID: "parallel-visible-lines", Kind: "PARALLEL", References: []SketchGeometryRef{
		{Target: "ENTITY", EntityID: "32ffe143-3739-5f61-9b31-69e964929441", SubElement: "WHOLE"},
		{Target: "ENTITY", EntityID: "fac2b76a-4839-4a63-a905-5aee594f4561", SubElement: "WHOLE"},
	}})
	model := PartModel{Features: []Feature{{ID: "corner-parallel", Type: "SKETCH", Sketch: &sketch}}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = (&Service{worker: worker}).solveSketchFeature(ctx, "corner-parallel", &model, 0)
	// Keep failed candidate evidence local, never modify the original document.
	if err != nil {
		for _, e := range sketch.Entities {
			if e.Kind == "LINE" {
				t.Logf("%s start=%+v end=%+v", e.ID, *e.Start, *e.End)
			}
		}
		t.Fatal(err)
	}
	if sketch.Solve.Status == "CONFLICTING" || sketch.Solve.Status == "FAILED" {
		t.Fatal(sketch.Solve)
	}
	assertRoundedParallel(t, &sketch, "32ffe143-3739-5f61-9b31-69e964929441", "fac2b76a-4839-4a63-a905-5aee594f4561")
}

func assertRoundedParallel(t *testing.T, sketch *SketchFeature, childID, otherID string) {
	t.Helper()
	var child, other SketchEntity
	for _, e := range sketch.Entities {
		if e.ID == childID {
			child = e
		}
		if e.ID == otherID {
			other = e
		}
	}
	if child.Start == nil || other.Start == nil {
		t.Fatal("missing stable line identity")
	}
	d, o := sub2(*child.End, *child.Start), sub2(*other.End, *other.Start)
	if math.Hypot(d.X, d.Y) < 1 {
		t.Fatal("collapsed trimmed edge")
	}
	if math.Abs(cornerCross(d, o))/(math.Hypot(d.X, d.Y)*math.Hypot(o.X, o.Y)) > 1e-8 {
		t.Fatal("parallel direction residual")
	}
	if err := validateSketchCornerGeometry(sketch); err != nil {
		t.Fatal(err)
	}
	for _, e := range sketch.Entities {
		if e.Kind == "ARC" {
			if math.Abs(e.Radius-90) > 1e-7 {
				t.Fatal("radius changed")
			}
			if cornerDistance(cornerEndpoint(e, "END"), *child.Start) > 1e-7 {
				t.Fatal("fillet connection lost")
			}
		}
	}
}

func TestSketchWorkflowRoundedQuadrilateralParallelHistory(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	data, err := os.ReadFile("testdata/corner_parallel.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot SketchFeature
	if err = json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	ops := []SketchOperation{}
	for _, e := range snapshot.Entities[:4] {
		e.Role = "PROFILE"
		ops = append(ops, workflowEntity(e))
	}
	for _, c := range snapshot.Constraints[:4] {
		ops = append(ops, workflowConstraint(c))
	}
	f.edit(ops...)
	a, b, other := snapshot.Entities[0].ID, snapshot.Entities[1].ID, snapshot.Entities[3].ID
	op := cornerOperation("FILLET_ENTITIES", "rounded-parallel-corner", a, b, "END", "START")
	op.Value = workflowValue(90)
	// The original vertex determines the selected local branch.
	op.Point = &SketchPoint2{X: 47.16450811081129, Y: 96.1430357643463}
	before := f.edit(op)
	childID := ""
	for _, e := range f.sketch(before).Entities {
		if e.SourceEntityID == b && e.Kind == "LINE" && e.Role == "PROFILE" {
			childID = e.ID
		}
	}
	if childID == "" {
		t.Fatal("no trimmed line")
	}
	after := f.edit(workflowConstraint(SketchConstraint{ID: "rounded-visible-parallel", Kind: "PARALLEL", References: []SketchGeometryRef{workflowRef(childID, "WHOLE"), workflowRef(other, "WHOLE")}}))
	assertRoundedParallel(t, f.sketch(after), childID, other)
	f.command(CommandRequest{Type: "UNDO"})
	redo := f.command(CommandRequest{Type: "REDO"})
	assertRoundedParallel(t, f.sketch(redo), childID, other)
	cold, err := f.service.GetDocument(f.ctx, f.documentID)
	if err != nil {
		t.Fatal(err)
	}
	assertRoundedParallel(t, f.sketch(cold), childID, other)
}

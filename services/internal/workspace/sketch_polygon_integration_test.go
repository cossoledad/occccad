package workspace

import (
	"fmt"
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestSketchWorkflowPolygonExtrudeHistory(t *testing.T) {
	for _, mode := range []string{"INSCRIBED", "CIRCUMSCRIBED"} {
		for _, n := range []int{4, 6, 7, 8} {
			t.Run(fmt.Sprintf("%s/%d", mode, n), func(t *testing.T) {
				f := newSketchWorkflowFixture(t)
				r := 5.0
				v := f.edit(SketchOperation{Type: "CREATE_POLYGON", OperationID: uuid.NewString(), Point: &SketchPoint2{X: 13, Y: 29}, Value: &r, Angle: .37, Sides: n, Mode: mode, Role: "PROFILE"})
				if f.sketch(v).Solve.Status == "CONFLICTING" {
					t.Fatal("polygon constraints conflict")
				}
				created := f.sketch(v)
				if created.Solve.DegreesOfFreedom != 4 {
					t.Fatalf("regular polygon should retain translation, rotation and size: %+v", created.Solve)
				}
				assertRegularPolygon(t, created, n, mode)
				undone := f.command(CommandRequest{Type: "UNDO"})
				if len(f.sketch(undone).Entities) != 0 || len(f.sketch(undone).Constraints) != 0 {
					t.Fatal("one undo must remove the complete polygon operation")
				}
				v = f.command(CommandRequest{Type: "REDO"})
				restored := f.sketch(v)
				if len(restored.Entities) != len(created.Entities) || len(restored.Constraints) != len(created.Constraints) {
					t.Fatal("redo must restore the complete polygon")
				}
				for i := range created.Entities {
					if restored.Entities[i].ID != created.Entities[i].ID {
						t.Fatal("redo changed a polygon entity identity")
					}
				}
				for i := range created.Constraints {
					if restored.Constraints[i].ID != created.Constraints[i].ID {
						t.Fatal("redo changed a polygon constraint identity")
					}
				}
				driver := uuid.NewString()
				v = f.edit(workflowConstraint(SketchConstraint{ID: driver, Kind: "RADIUS", References: []SketchGeometryRef{workflowRef(f.sketch(v).Entities[0].ID, "WHOLE")}, Value: workflowValue(5), Unit: "mm"}))
				v = f.command(CommandRequest{Type: "PAD_SKETCH", Length: 2})
				area := float64(n) * r * r * math.Sin(2*math.Pi/float64(n)) / 2
				if mode == "INSCRIBED" {
					area = float64(n) * r * r * math.Tan(math.Pi/float64(n))
				}
				f.volume(v, 2*area)
				f.volume(f.command(CommandRequest{Type: "UNDO"}), 0)
				f.volume(f.command(CommandRequest{Type: "REDO"}), 2*area)
				v = f.edit(SketchOperation{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: driver, Value: workflowValue(6)})
				f.volume(v, 2*area*36/25)
				assertRegularPolygon(t, f.sketch(v), n, mode)
				f.volume(f.command(CommandRequest{Type: "UNDO"}), 2*area)
				f.volume(f.command(CommandRequest{Type: "REDO"}), 2*area*36/25)
				cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
				if err != nil {
					t.Fatal(err)
				}
				f.volume(cold, 2*area*36/25)
			})
		}
	}
}

func assertRegularPolygon(t *testing.T, sketch *SketchFeature, n int, mode string) {
	t.Helper()
	circle := sketch.Entities[0]
	for _, edge := range sketch.Entities[1 : n+1] {
		ax, ay := edge.Start.X-circle.Center.X, edge.Start.Y-circle.Center.Y
		bx, by := edge.End.X-circle.Center.X, edge.End.Y-circle.Center.Y
		expected := circle.Radius
		if mode == "INSCRIBED" {
			expected /= math.Cos(math.Pi / float64(n))
		}
		if math.Abs(math.Hypot(ax, ay)-expected) > 1e-5 || math.Abs(math.Hypot(bx, by)-expected) > 1e-5 {
			t.Fatal("solver lost regular polygon circumradius")
		}
		if mode == "INSCRIBED" && math.Abs(math.Hypot((ax+bx)/2, (ay+by)/2)-circle.Radius) > 1e-5 {
			t.Fatal("tangency moved away from side midpoint")
		}
	}
}

func TestSketchWorkflowPolygonDiagnosesRhombus(t *testing.T) {
	f := newSketchWorkflowFixture(t)
	radius := 5.0
	v := f.edit(SketchOperation{Type: "CREATE_POLYGON", OperationID: uuid.NewString(), Point: &SketchPoint2{X: 13, Y: 29}, Value: &radius, Angle: .37, Sides: 4, Mode: "INSCRIBED", Role: "PROFILE"})
	polygon := f.sketch(v)
	result, err := f.service.ApplyCommand(f.ctx, f.documentID, CommandRequest{Type: "EDIT_SKETCH", RequestID: uuid.NewString(), ActorID: f.actor, SketchID: f.sketchID,
		Operations: []SketchOperation{workflowConstraint(SketchConstraint{ID: uuid.NewString(), Kind: "ANGLE", References: []SketchGeometryRef{workflowRef(polygon.Entities[1].ID, "WHOLE"), workflowRef(polygon.Entities[2].ID, "WHOLE")}, Value: workflowValue(70), Unit: "deg"})}})
	if err != nil {
		t.Fatal(err)
	}
	if f.sketch(result).Solve.Status != "CONFLICTING" {
		t.Fatalf("non-square rhombus must have a conflict diagnostic: %+v", f.sketch(result).Solve)
	}
	cold, err := f.service.GetDocument(f.ctx, f.documentID, f.actor)
	if err != nil {
		t.Fatal(err)
	}
	if f.sketch(cold).Solve.Status != "CONFLICTING" {
		t.Fatal("cold read lost conflict diagnostic")
	}
	assertRegularPolygon(t, f.sketch(f.command(CommandRequest{Type: "UNDO"})), 4, "INSCRIBED")
}

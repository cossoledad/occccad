package workspace

import (
	"math"
	"testing"

	"github.com/google/uuid"
)

func TestSketchWorkflowPolygonExtrudeHistory(t *testing.T) {
	for _, mode := range []string{"INSCRIBED", "CIRCUMSCRIBED"} {
		t.Run(mode, func(t *testing.T) {
			f := newSketchWorkflowFixture(t)
			r := 5.0
			n := 7
			v := f.edit(SketchOperation{Type: "CREATE_POLYGON", OperationID: uuid.NewString(), Point: &SketchPoint2{X: 13, Y: 29}, Value: &r, Angle: .37, Sides: n, Mode: mode, Role: "PROFILE"})
			if f.sketch(v).Solve.Status == "CONFLICTING" {
				t.Fatal("polygon constraints conflict")
			}
			created := f.sketch(v)
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

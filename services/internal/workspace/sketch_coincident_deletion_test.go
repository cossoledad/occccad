package workspace

import (
	"reflect"
	"testing"
)

func TestDeleteGeometryPreservesOnlyProvenSurvivingCoincidences(t *testing.T) {
	ref := func(id string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}
	}
	point := func(id string) SketchEntity {
		return SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{}}
	}
	base := SketchFeature{Entities: []SketchEntity{point("a"), point("hub"), point("b"), point("unrelated")}, Constraints: []SketchConstraint{
		{ID: "first", Kind: "COINCIDENT", References: []SketchGeometryRef{ref("a"), ref("hub")}},
		{ID: "second", Kind: "COINCIDENT", References: []SketchGeometryRef{ref("hub"), ref("b")}},
	}}
	for _, mode := range []string{"active", "suppressed", "released", "existing"} {
		t.Run(mode, func(t *testing.T) {
			s := base
			s.Entities = append([]SketchEntity(nil), base.Entities...)
			s.Constraints = append([]SketchConstraint(nil), base.Constraints...)
			op := SketchOperation{Type: "DELETE_ENTITIES", EntityIDs: []string{"hub"}}
			if mode == "suppressed" {
				s.Constraints[0].Suppressed = true
			}
			if mode == "released" {
				op.DetachConstraintIDs = []string{"first"}
			}
			if mode == "existing" {
				s.Constraints = append(s.Constraints, SketchConstraint{ID: "retained", Kind: "COINCIDENT", References: []SketchGeometryRef{ref("a"), ref("b")}})
			}
			if err := applySketchGeometryEdit(&s, op); err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "suppressed" || mode == "released" {
				want = 0
			}
			if len(s.Constraints) != want {
				t.Fatal(s.Constraints)
			}
			if want > 0 {
				if !sketchPointsJoined(&s, ref("a"), ref("b")) {
					t.Fatal("equality lost")
				}
				if sketchPointsJoined(&s, ref("a"), ref("unrelated")) {
					t.Fatal("coordinate-based coincidence invented")
				}
			}
			if mode == "existing" && s.Constraints[0].ID != "retained" {
				t.Fatal("existing relation identity replaced")
			}
			if len(s.Entities) != 3 {
				t.Fatal(s.Entities)
			}
			again := base
			again.Entities = append([]SketchEntity(nil), base.Entities...)
			again.Constraints = append([]SketchConstraint(nil), base.Constraints...)
			if mode == "active" {
				if err := applySketchGeometryEdit(&again, op); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(s, again) {
					t.Fatal("non-deterministic deletion projection")
				}
			}
		})
	}
}

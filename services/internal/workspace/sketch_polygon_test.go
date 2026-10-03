package workspace

import (
	"fmt"
	"math"
	"testing"
)

func TestSketchPolygonGeometryAndAtomicity(t *testing.T) {
	for _, mode := range []string{"INSCRIBED", "CIRCUMSCRIBED"} {
		for _, n := range []int{3, 4, 7, 50} {
			t.Run(fmt.Sprintf("%s/%d", mode, n), func(t *testing.T) {
				s := SketchFeature{SchemaVersion: SketchSchemaVersion}
				radius := 5.0
				op := SketchOperation{Type: "CREATE_POLYGON", OperationID: "polygon", Point: &SketchPoint2{X: 13, Y: -9}, Value: &radius, Angle: .37, Sides: n, Mode: mode, Role: "PROFILE"}
				if err := applySketchOperations(&s, []SketchOperation{op}); err != nil {
					t.Fatal(err)
				}
				if err := validateSketch(s); err != nil {
					t.Fatal(err)
				}
				if len(s.Entities) != n+1 || s.Entities[0].Role != "CONSTRUCTION" {
					t.Fatal("missing ordinary geometry or construction circle")
				}
				for _, e := range s.Entities[1:] {
					distance := math.Hypot(e.Start.X-13, e.Start.Y+9)
					want := radius
					if mode == "INSCRIBED" {
						want /= math.Cos(math.Pi / float64(n))
					}
					if math.Abs(distance-want) > 1e-10 {
						t.Fatal("wrong polygon radius")
					}
					if mode == "INSCRIBED" {
						dx, dy := e.End.X-e.Start.X, e.End.Y-e.Start.Y
						normalDistance := math.Abs(dx*(-9-e.Start.Y)-dy*(13-e.Start.X)) / math.Hypot(dx, dy)
						if math.Abs(normalDistance-radius) > 1e-10 {
							t.Fatal("side not tangent to circle")
						}
					}
				}
				loops, err := buildProfileLoops(Feature{ID: "sketch", Type: "SKETCH", Sketch: &s}, true)
				if err != nil || len(loops) != 1 {
					t.Fatalf("polygon closure: %v", err)
				}
				before := sketchDragDigest(&s)
				op.Sides = 2
				if err := applySketchOperations(&s, []SketchOperation{op}); err == nil || sketchDragDigest(&s) != before {
					t.Fatal("invalid polygon was not atomic")
				}
			})
		}
	}
}

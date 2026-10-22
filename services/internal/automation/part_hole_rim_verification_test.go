package automation

import (
	"math"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func TestHoleRimAcceptanceRequiresTwoCompleteDistinctCircles(t *testing.T) {
	circle := func(z, end float64) workspace.BodyTopologyItem {
		return workspace.BodyTopologyItem{Geometry: workspace.TopologyGeometry{OriginMM: &[3]float64{50, 50, z}, Direction: &[3]float64{0, 0, -1}, Range: &workspace.TopologyCurveRange{Start: 0, End: end, Convention: "ANGLE_RADIANS"}}}
	}
	center, normal := [3]float64{50, 50, 0}, [3]float64{0, 0, 1}
	valid := []workspace.BodyTopologyItem{circle(0, 2*math.Pi), circle(100, 2*math.Pi)}
	if !completeRimCircles(valid, center, normal, 1e-6) {
		t.Fatal("complete boundaries rejected")
	}
	for _, items := range [][]workspace.BodyTopologyItem{{circle(0, math.Pi), circle(100, math.Pi)}, {circle(0, 2*math.Pi), circle(0, 2*math.Pi)}, {circle(0, 2*math.Pi)}} {
		if completeRimCircles(items, center, normal, 1e-6) {
			t.Fatal("partial/coincident boundaries accepted")
		}
	}
	valid[1].Geometry.OriginMM = nil
	if completeRimCircles(valid, center, normal, 1e-6) {
		t.Fatal("missing position accepted")
	}
}

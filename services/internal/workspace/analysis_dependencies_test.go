package workspace

import (
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestMeasurementInputFacetsRigidMotionAndGeometry(t *testing.T) {
	reads := []modelcore.DependencyRead{{DocumentID: "a", EntityID: "face", Facet: modelcore.IntrinsicGeometry, Digest: "a1"}, {DocumentID: "b", EntityID: "face", Facet: modelcore.IntrinsicGeometry, Digest: "b1"}}
	placements := []analysisPlacement{{DocumentID: "a", EntityID: "face", Pose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}, {DocumentID: "b", EntityID: "face", Pose: InstancePose{Translation: [3]float64{20, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}}
	before, _, err := measurementInputStamps("BETWEEN", reads, placements)
	if err != nil {
		t.Fatal(err)
	}
	placements[1].Pose.Translation[0] = 30
	after, _, err := measurementInputStamps("BETWEEN", reads, placements)
	if err != nil {
		t.Fatal(err)
	}
	if before["INTRINSIC"] != after["INTRINSIC"] || before["RELATIVE"] == after["RELATIVE"] || before["FRAME"] == after["FRAME"] {
		t.Fatal("wrong movement dependency")
	}
	placements[1].Pose.Translation[0] = 120
	placements[0].Pose.Translation[0] = 100
	common, _, err := measurementInputStamps("BETWEEN", reads, placements)
	if err != nil {
		t.Fatal(err)
	}
	if before["RELATIVE"] != common["RELATIVE"] || before["FRAME"] == common["FRAME"] {
		t.Fatal("common motion scalar/coordinate currency")
	}
	reads[0].Digest = "a2"
	changed, _, _ := measurementInputStamps("BETWEEN", reads, placements)
	if before["INTRINSIC"] == changed["INTRINSIC"] {
		t.Fatal("geometry ignored")
	}
	if measurementOutputFacet("ITEM", "areaMm2") != "INTRINSIC" || measurementOutputFacet("ITEM", "xMm") != "FRAME" || measurementOutputFacet("BETWEEN", "distanceMm") != "RELATIVE" {
		t.Fatal("output grouping")
	}
}

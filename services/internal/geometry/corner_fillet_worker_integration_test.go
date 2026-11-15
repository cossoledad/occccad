//go:build integration

package geometry

import (
	"fmt"
	"math"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/testsupport"
)

func TestCppWorkerChamferedCornerFillets(t *testing.T) {
	root := testsupport.Environment(t).DataDirectory
	namespace := fmt.Sprintf("tests/fillet/%d/", time.Now().UnixNano())
	client := openCppWorkerForNamingContract(t)
	base := ProfilePad{
		FeatureID: "base", BodyID: "body", ProfileFeatureID: "sketch",
		Length: 40, Plane: "XY", BodyOperation: "ADD", Generator: "LINEAR_EXTRUDE",
		Regions: []ProfileRegion{{ID: "region", Outer: ProfileLoop{ID: "outer", Curves: []ProfileCurve{
			{EntityID: "a", Kind: "LINE", Start: [2]float64{0, 0}, End: [2]float64{20, 0}},
			{EntityID: "b", Kind: "LINE", Start: [2]float64{20, 0}, End: [2]float64{20, 30}},
			{EntityID: "c", Kind: "LINE", Start: [2]float64{20, 30}, End: [2]float64{0, 30}},
			{EntityID: "d", Kind: "LINE", Start: [2]float64{0, 30}, End: [2]float64{0, 0}},
		}}}},
	}
	sequence := 0
	evaluate := func(pads []ProfilePad) (*workerv1.EvaluatePartResponse, *workerv1.PartTopologyManifest) {
		t.Helper()
		sequence++
		key := fmt.Sprintf("%scorner-%d", namespace, sequence)
		response, err := client.EvaluateProfilePartFromArtifact(t.Context(), key, key, pads, ArtifactReference{}, key+".brep", key+".glb")
		if err != nil {
			t.Fatal(err)
		}
		manifest, _ := readWorkerNamingArtifact(t, root, response)
		return response, manifest
	}
	// Coordinates select fixture edges only. The next request carries actual
	// stable semantic references resolved from that stage's frozen manifest.
	pick := func(response *workerv1.EvaluatePartResponse, manifest *workerv1.PartTopologyManifest, predicate func(*workerv1.EdgeInfo) bool) []modelcore.SemanticTopologyRef {
		t.Helper()
		topology, _, err := client.GetTopology(t.Context(), response.GetGeometryId(), nil, "", 0)
		if err != nil {
			t.Fatal(err)
		}
		var refs []modelcore.SemanticTopologyRef
		for _, edge := range topology.GetEdges() {
			if !predicate(edge) {
				continue
			}
			for _, locator := range manifest.GetBodies()[0].GetTip() {
				if locator.GetTopologyType() == workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE && locator.GetLocalId() == edge.GetLocalId() {
					ref := manifest.GetSemanticRefs()[locator.GetSemanticRef()-1]
					refs = append(refs, modelcore.SemanticTopologyRef{FeatureID: ref.GetFeatureId(), OutputSlot: ref.GetOutputSlot(), SourceIDs: ref.GetSourceIds()})
				}
			}
		}
		return refs
	}
	zeroCoordinates := func(edge *workerv1.EdgeInfo) (bool, bool, bool) {
		box := edge.GetBbox()
		if box == nil {
			return false, false, false
		}
		return math.Abs(box.MinX)+math.Abs(box.MaxX) < 1e-5, math.Abs(box.MinY)+math.Abs(box.MaxY) < 1e-5, math.Abs(box.MinZ)+math.Abs(box.MaxZ) < 1e-5
	}
	initial, initialNaming := evaluate([]ProfilePad{base})
	chamfer := ProfilePad{FeatureID: "chamfer", BodyID: "body", InputFeatureID: "base", Generator: "CHAMFER", Length: 1}
	chamfer.Selections = pick(initial, initialNaming, func(e *workerv1.EdgeInfo) bool { x, y, _ := zeroCoordinates(e); return x && y })
	if len(chamfer.Selections) != 1 {
		t.Fatal("expected one corner chamfer edge")
	}
	beveled, naming := evaluate([]ProfilePad{base, chamfer})
	fillet := ProfilePad{FeatureID: "fillet", BodyID: "body", InputFeatureID: "chamfer", Generator: "FILLET"}
	fillet.Selections = pick(beveled, naming, func(e *workerv1.EdgeInfo) bool { x, y, z := zeroCoordinates(e); return z && (x || y) })
	if len(fillet.Selections) != 2 {
		t.Fatal("expected two bottom edges separated by the bevel")
	}
	previousVolume := beveled.GetVolume()
	for _, radius := range []float64{1, 1.5, 2} {
		fillet.Length = radius
		result, _ := evaluate([]ProfilePad{base, chamfer, fillet})
		if result.GetVolume() <= 0 || result.GetVolume() >= previousVolume {
			t.Fatalf("radius %g: incorrect material removal %g", radius, result.GetVolume())
		}
		previousVolume = result.GetVolume()
	}
}

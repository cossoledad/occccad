//go:build integration

package geometry

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/testsupport"
)

func TestCppWorkerBoundaryFilletParameterRoundTrip(t *testing.T) {
	root := testsupport.Environment(t).DataDirectory
	namespace := fmt.Sprintf("tests/fillet/%d/", time.Now().UnixNano())
	client := openCppWorkerForNamingContract(t)
	base := ProfilePad{
		FeatureID: "base", BodyID: "body", ProfileFeatureID: "sketch",
		Length: 2.2, Plane: "XY", BodyOperation: "ADD", Generator: "LINEAR_EXTRUDE",
		Regions: []ProfileRegion{{ID: "region", Outer: ProfileLoop{ID: "outer", Curves: []ProfileCurve{
			{EntityID: "a", Kind: "LINE", Start: [2]float64{0, 0}, End: [2]float64{60, 0}},
			{EntityID: "b", Kind: "LINE", Start: [2]float64{60, 0}, End: [2]float64{60, 30}},
			{EntityID: "c", Kind: "LINE", Start: [2]float64{60, 30}, End: [2]float64{0, 30}},
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
	fillet := ProfilePad{FeatureID: "fillet", BodyID: "body", InputFeatureID: "base", Generator: "FILLET"}
	fillet.Selections = pick(initial, initialNaming, func(e *workerv1.EdgeInfo) bool { _, _, z := zeroCoordinates(e); return z })
	if len(fillet.Selections) != 4 {
		t.Fatal("expected four bottom edges")
	}
	volumes := map[float64]float64{}
	for _, radius := range []float64{2, 2.2, 2.3, 3, 4, 3} {
		fillet.Length = radius
		result, _ := evaluate([]ProfilePad{base, fillet})
		if measuredEvaluationVolume(t, client, result) <= 0 || measuredEvaluationVolume(t, client, result) >= measuredEvaluationVolume(t, client, initial) {
			t.Fatalf("radius %g: material range %g", radius, measuredEvaluationVolume(t, client, result))
		}
		if prior, ok := volumes[radius]; ok && math.Abs(prior-measuredEvaluationVolume(t, client, result)) > 1e-7 {
			t.Fatal("parameter round trip changed result")
		}
		volumes[radius] = measuredEvaluationVolume(t, client, result)
		t.Logf("radius=%g volume=%g triangles=%d", radius, measuredEvaluationVolume(t, client, result), result.GetTriangleCount())
	}
	corner := ProfilePad{FeatureID: "corners", BodyID: "body", InputFeatureID: "base", Generator: "FILLET", Length: 0.5}
	corner.Selections = pick(initial, initialNaming, func(e *workerv1.EdgeInfo) bool {
		b := e.GetBbox()
		return b != nil && b.MaxZ-b.MinZ > 2 && math.Abs(b.MaxX-b.MinX) < 1e-5 && math.Abs(b.MaxY-b.MinY) < 1e-5
	})
	if len(corner.Selections) != 4 {
		t.Fatal("expected four vertical edges")
	}
	curved, curvedNaming := evaluate([]ProfilePad{base, corner})
	fillet.InputFeatureID = "corners"
	fillet.Length = 3
	fillet.Selections = pick(curved, curvedNaming, func(e *workerv1.EdgeInfo) bool { _, _, z := zeroCoordinates(e); return z })
	if len(fillet.Selections) != 8 {
		t.Fatal("expected straight and circular bottom boundaries")
	}
	_, err := client.EvaluateProfilePartFromArtifact(t.Context(), "invalid-corner", "invalid-corner", []ProfilePad{base, corner, fillet}, ArtifactReference{}, "invalid-corner.brep", "invalid-corner.glb")
	if err == nil || !strings.Contains(err.Error(), "FILLET_BOUNDARY_CORNER_TANGENCY_FAILED") {
		t.Fatalf("non-tangent corner accepted or misdiagnosed: %v", err)
	}
}

package geometry

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

func TestCppWorkerChamferedCornerFillets(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OCCCCAD_DATA_DIR", root)
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
		key := fmt.Sprintf("corner-%d", sequence)
		response, err := client.EvaluateProfilePartFromArtifact(t.Context(), key, key, pads, ArtifactReference{}, key+".brep", key+".glb")
		if err != nil {
			t.Fatal(err)
		}
		manifest := new(workerv1.PartTopologyManifest)
		data, err := os.ReadFile(filepath.Join(root, response.GetEvaluationManifest().GetTopologyManifestArtifact().GetObjectKey()))
		if err != nil {
			t.Fatal(err)
		}
		if err := proto.Unmarshal(data, manifest); err != nil {
			t.Fatal(err)
		}
		if response.GetTriangleCount() == 0 || response.GetGlbArtifact().GetSizeBytes() == 0 || response.GetBrepArtifact().GetSizeBytes() == 0 {
			t.Fatal("missing exact or display artifact")
		}
		for _, transition := range manifest.GetTransitions() {
			if !transition.GetComplete() {
				t.Fatalf("incomplete topology history: %s", transition.GetFeatureId())
			}
		}
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
		valid := len(edge.GetRenderPoints()) > 1
		x, y, z := valid, valid, valid
		for _, p := range edge.GetRenderPoints() {
			x, y, z = x && math.Abs(p.X) < 1e-7, y && math.Abs(p.Y) < 1e-7, z && math.Abs(p.Z) < 1e-7
		}
		return x, y, z
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

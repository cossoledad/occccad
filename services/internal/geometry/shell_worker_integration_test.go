package geometry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

func TestCppWorkerShellAfterFilletAndMixedLoft(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OCCCCAD_DATA_DIR", root)
	client := openCppWorkerForNamingContract(t)
	sequence := 0
	evaluate := func(t *testing.T, pads []ProfilePad) (*workerv1.EvaluatePartResponse, *workerv1.PartTopologyManifest) {
		t.Helper()
		sequence++
		key := fmt.Sprintf("shell-%d", sequence)
		started := time.Now()
		response, err := client.EvaluateProfilePartFromArtifact(t.Context(), key, key, pads, ArtifactReference{}, key+".brep", key+".glb")
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, response.GetEvaluationManifest().GetTopologyManifestArtifact().GetObjectKey()))
		if err != nil {
			t.Fatal(err)
		}
		manifest := new(workerv1.PartTopologyManifest)
		if err := proto.Unmarshal(data, manifest); err != nil {
			t.Fatal(err)
		}
		for _, transition := range manifest.GetTransitions() {
			if !transition.GetComplete() {
				t.Fatalf("incomplete history: %s", transition.GetFeatureId())
			}
		}
		if response.GetTriangleCount() == 0 || response.GetBrepArtifact().GetSizeBytes() == 0 || response.GetGlbArtifact().GetSizeBytes() == 0 {
			t.Fatal("missing exact or display artifact")
		}
		t.Logf("evaluate %s: %s cache_hit=%v triangles=%d GLB=%d bytes", pads[len(pads)-1].Generator, time.Since(started), response.GetCacheHit(), response.GetTriangleCount(), response.GetGlbArtifact().GetSizeBytes())
		return response, manifest
	}
	refs := func(manifest *workerv1.PartTopologyManifest, kind workerv1.PersistentTopologyType, prefix string) []modelcore.SemanticTopologyRef {
		var result []modelcore.SemanticTopologyRef
		for _, locator := range manifest.GetBodies()[0].GetTip() {
			ref := manifest.GetSemanticRefs()[locator.GetSemanticRef()-1]
			if locator.GetTopologyType() == kind && strings.HasPrefix(ref.GetOutputSlot(), prefix) {
				result = append(result, modelcore.SemanticTopologyRef{FeatureID: ref.GetFeatureId(), OutputSlot: ref.GetOutputSlot(), SourceIDs: ref.GetSourceIds()})
			}
		}
		return result
	}
	region := ProfileRegion{ID: "rectangle", Outer: ProfileLoop{ID: "outer", Curves: []ProfileCurve{
		{EntityID: "a", Kind: "LINE", Start: [2]float64{-15, -15}, End: [2]float64{15, -15}},
		{EntityID: "b", Kind: "LINE", Start: [2]float64{15, -15}, End: [2]float64{15, 15}},
		{EntityID: "c", Kind: "LINE", Start: [2]float64{15, 15}, End: [2]float64{-15, 15}},
		{EntityID: "d", Kind: "LINE", Start: [2]float64{-15, 15}, End: [2]float64{-15, -15}},
	}}}
	base := ProfilePad{FeatureID: "base", BodyID: "body", ProfileFeatureID: "sketch", Length: 40, Plane: "XY", BodyOperation: "ADD", Generator: "LINEAR_EXTRUDE", Regions: []ProfileRegion{region}}
	_, naming := evaluate(t, []ProfilePad{base})
	fillet := ProfilePad{FeatureID: "fillet", BodyID: "body", InputFeatureID: "base", Generator: "FILLET", Length: 8,
		Selections: refs(naming, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE, "")}
	if len(fillet.Selections) != 12 {
		t.Fatal("expected all twelve box edges")
	}
	loft := ProfilePad{FeatureID: "loft", BodyID: "body", Generator: "LOFT", Sections: []LoftSection{
		{SketchID: "rectangle", Region: region, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, SeamEntityID: "a"},
		{SketchID: "circle", Origin: [3]float64{0, 0, 40}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, SeamEntityID: "circle-edge",
			Region: ProfileRegion{ID: "circle", Outer: ProfileLoop{ID: "circle-loop", Curves: []ProfileCurve{{EntityID: "circle-edge", Kind: "CIRCLE", Radius: 10}}}}},
	}}
	offsetLoft := loft
	offsetLoft.Sections = append([]LoftSection(nil), loft.Sections...)
	points := [][2]float64{{-82.8172482034993, -87.47795914135841}, {111.25953546530698, -87.47795914135841}, {111.25953546530698, 69.55214784189991}, {-82.8172482034993, 69.55214784189991}}
	offsetLoft.Sections[0].Region.Outer.Curves = make([]ProfileCurve, 4)
	for i := range points {
		offsetLoft.Sections[0].Region.Outer.Curves[i] = ProfileCurve{EntityID: string(rune('a' + i)), Kind: "LINE", Start: points[i], End: points[(i+1)%4]}
	}
	offsetLoft.Sections[1].Origin = [3]float64{0, 0, 100}
	offsetLoft.Sections[1].Region.Outer.Curves = []ProfileCurve{{EntityID: "circle-edge", Kind: "CIRCLE", Radius: 90}}
	for _, fixture := range []struct {
		name, cap string
		chain     []ProfilePad
	}{{"large-fillet", "END_CAP/", []ProfilePad{base, fillet}}, {"rectangle-circle-loft", "LOFT_END_CAP", []ProfilePad{loft}}, {"offset-rectangle-larger-circle", "LOFT_END_CAP", []ProfilePad{offsetLoft}}} {
		t.Run(fixture.name, func(t *testing.T) {
			original, naming := evaluate(t, fixture.chain)
			shell := ProfilePad{FeatureID: "shell", BodyID: "body", InputFeatureID: fixture.chain[len(fixture.chain)-1].FeatureID, Generator: "SHELL", Length: 1,
				Selections: refs(naming, workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE, fixture.cap)}
			if len(shell.Selections) != 1 {
				t.Fatal("expected one opening face")
			}
			result, _ := evaluate(t, append(fixture.chain, shell))
			if result.GetVolume() <= 0 || result.GetVolume() >= original.GetVolume() {
				t.Fatal("incorrect inward shell volume")
			}
			repeated, repeatedNaming := evaluate(t, append(fixture.chain, shell))
			if !repeated.GetCacheHit() || repeated.GetGlbArtifact().GetSha256() != result.GetGlbArtifact().GetSha256() || repeated.GetBrepArtifact().GetSha256() != result.GetBrepArtifact().GetSha256() || repeatedNaming.GetGeometryId() != result.GetGeometryId() {
				t.Fatal("repeated shell did not reuse the complete immutable result")
			}
		})
	}
}

// Same caller key cannot alias different geometry, precision or naming policy.
// Concurrent identical requests produce one result even with different output paths.
func TestCppWorkerVisualSnapshotCacheIdentity(t *testing.T) {
	t.Setenv("OCCCCAD_DATA_DIR", t.TempDir())
	client := openCppWorkerForNamingContract(t)
	request := &workerv1.EvaluatePartRequest{RequestId: "first", GeometryKey: "same-key",
		RectangularPad:   &workerv1.RectangularPadSpec{Width: 20, Height: 30, PadLength: 40, Plane: "XY", Units: "mm"},
		LinearDeflection: 0.1, AngularDeflection: 0.5, BrepOutputKey: "first.brep", GlbOutputKey: "first.glb"}
	first, err := client.worker.EvaluatePart(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	type answer struct {
		response *workerv1.EvaluatePartResponse
		err      error
	}
	request.AngularDeflection = 0.3 // two concurrent cold requests for the same new policy
	results := make(chan answer, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			r := proto.Clone(request).(*workerv1.EvaluatePartRequest)
			r.RequestId = fmt.Sprint(i)
			r.BrepOutputKey = fmt.Sprintf("repeat%d.brep", i)
			r.GlbOutputKey = fmt.Sprintf("repeat%d.glb", i)
			response, err := client.worker.EvaluatePart(t.Context(), r)
			results <- answer{response, err}
		}(i)
	}
	hits := 0
	for i := 0; i < 2; i++ {
		a := <-results
		if a.err != nil {
			t.Fatal(a.err)
		}
		if a.response.GetCacheHit() {
			hits++
		}
		if a.response.GetGlbArtifact().GetSha256() != first.GetGlbArtifact().GetSha256() {
			t.Fatal("snapshot reuse mismatch")
		}
	}
	if hits != 1 {
		t.Fatalf("expected one producer and one cache hit, got %d hits", hits)
	}
	request.RectangularPad.PadLength = 50
	changed, err := client.worker.EvaluatePart(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.GetCacheHit() || changed.GetGeometryId() == first.GetGeometryId() {
		t.Fatal("caller key hid changed inputs")
	}
	request.LinearDeflection = 0.02
	refined, err := client.worker.EvaluatePart(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if refined.GetCacheHit() {
		t.Fatal("precision missing from cache identity")
	}
	request.TopologyPolicy = &workerv1.TopologyNamingPolicy{PolicyId: "different"}
	policyResult, err := client.worker.EvaluatePart(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if policyResult.GetCacheHit() {
		t.Fatal("naming policy missing from cache identity")
	}
	// Rectangular pads don't consume naming; still cannot reuse the prior policy's response.
	if err == nil {
		response, e := client.worker.EvaluatePart(t.Context(), request)
		if e != nil || !response.GetCacheHit() {
			t.Fatal("policy-specific entry missing")
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = client.worker.EvaluatePart(cancelled, request); err == nil {
		t.Fatal("cancelled request accepted")
	}
}

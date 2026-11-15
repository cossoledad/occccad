//go:build integration

package geometry

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/testsupport"
)

func TestCppWorkerPatternArtifactsAndColdRead(t *testing.T) {
	root := testsupport.Environment(t).DataDirectory
	namespace := fmt.Sprintf("tests/pattern/%d/", time.Now().UnixNano())
	client := openCppWorkerForNamingContract(t)
	base := ProfilePad{FeatureID: "plate", BodyID: "body", ProfileFeatureID: "plate-sketch", Generator: "LINEAR_EXTRUDE", BodyOperation: "ADD", Plane: "XY", Length: 5,
		Regions: []ProfileRegion{{ID: "plate-region", Outer: ProfileLoop{ID: "plate-loop", Curves: []ProfileCurve{
			{EntityID: "a", Kind: "LINE", Start: [2]float64{-30, -30}, End: [2]float64{30, -30}},
			{EntityID: "b", Kind: "LINE", Start: [2]float64{30, -30}, End: [2]float64{30, 30}},
			{EntityID: "c", Kind: "LINE", Start: [2]float64{30, 30}, End: [2]float64{-30, 30}},
			{EntityID: "d", Kind: "LINE", Start: [2]float64{-30, 30}, End: [2]float64{-30, -30}},
		}}}}}
	cut := ProfilePad{FeatureID: "cut", BodyID: "body", InputFeatureID: "plate", ProfileFeatureID: "hole", Generator: "LINEAR_EXTRUDE", BodyOperation: "REMOVE", Plane: "XY", Length: 5,
		Regions: []ProfileRegion{{ID: "hole-region", Outer: ProfileLoop{ID: "hole-loop", Curves: []ProfileCurve{{EntityID: "circle", Kind: "CIRCLE", Center: [2]float64{20, 0}, Radius: 2}}}}}}
	pattern := ProfilePad{FeatureID: "holes", BodyID: "body", InputFeatureID: "cut", Generator: "SOLID_PATTERN", BodyOperation: "REMOVE", PatternSourceFeatureID: "cut", PatternSourceKind: "GENERATOR_TOOL"}
	for _, count := range []int{6, 8, 4} {
		pattern.PatternPlacements = nil
		for slot := 0; slot < count; slot++ {
			if slot == 2 {
				continue
			}
			a := float64(slot) * 2 * math.Pi / float64(count)
			c, s := math.Cos(a), math.Sin(a)
			pattern.PatternPlacements = append(pattern.PatternPlacements, PatternPlacement{Slot: uint32(slot), Matrix: [12]float64{c, -s, 0, 0, s, c, 0, 0, 0, 0, 1, 0}})
		}
		key := fmt.Sprintf("%spattern-%d", namespace, count)
		started := time.Now()
		result, err := client.EvaluateProfilePartFromArtifact(t.Context(), key, key, []ProfilePad{base, cut, pattern}, ArtifactReference{}, key+".brep", key+".glb")
		if err != nil {
			t.Fatal(err)
		}
		if result.GetTriangleCount() == 0 || result.GetBrepArtifact().GetSizeBytes() == 0 || result.GetGlbArtifact().GetSizeBytes() == 0 {
			t.Fatal("missing artifacts")
		}
		manifest, blocks := readWorkerNamingArtifact(t, root, result)
		if len(blocks) != 3 || !manifest.Bodies[0].Complete {
			t.Fatal("missing complete history blocks")
		}
		// A fresh process must reproduce semantic snapshots without warm Shape state.
		cold := openCppWorkerForNamingContract(t)
		other, err := cold.EvaluateProfilePartFromArtifact(t.Context(), key+"-cold", key+"-cold", []ProfilePad{base, cut, pattern}, ArtifactReference{}, key+"-cold.brep", key+"-cold.glb")
		if err != nil {
			t.Fatal(err)
		}
		if result.GetEvaluationManifest().GetTopologyManifestDigest() != other.GetEvaluationManifest().GetTopologyManifestDigest() {
			t.Fatal("cold Naming root changed")
		}
		if result.GetGeometryId() != other.GetGeometryId() {
			t.Fatal("cold evaluation changed exact geometry identity")
		}
		t.Logf("members=%d active=%d elapsed=%s triangles=%d glb_bytes=%d", count, count-1, time.Since(started), result.GetTriangleCount(), result.GetGlbArtifact().GetSizeBytes())
	}
}

func TestCppWorkerDefaultCircularSolidPattern(t *testing.T) {
	root := testsupport.Environment(t).DataDirectory
	namespace := fmt.Sprintf("tests/pattern/%d/", time.Now().UnixNano())
	client := openCppWorkerForNamingContract(t)
	seed := ProfilePad{FeatureID: "seed", BodyID: "body", ProfileFeatureID: "sketch", Generator: "LINEAR_EXTRUDE", BodyOperation: "ADD", Plane: "XY", Length: 5, Regions: []ProfileRegion{{ID: "region", Outer: ProfileLoop{ID: "loop", Curves: []ProfileCurve{{EntityID: "circle", Kind: "CIRCLE", Center: [2]float64{20, 0}, Radius: 2}}}}}}
	pattern := ProfilePad{FeatureID: "pattern", BodyID: "body", InputFeatureID: "seed", Generator: "SOLID_PATTERN", BodyOperation: "ADD", PatternSourceFeatureID: "seed", PatternSourceKind: "GENERATOR_TOOL", PatternResultMode: "COMBINE"}
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		c, s := math.Cos(a), math.Sin(a)
		pattern.PatternPlacements = append(pattern.PatternPlacements, PatternPlacement{Slot: uint32(i), Matrix: [12]float64{c, -s, 0, 0, s, c, 0, 0, 0, 0, 1, 0}})
	}
	result, err := client.EvaluateProfilePartFromArtifact(t.Context(), namespace+"circular-default", namespace+"circular-default", []ProfilePad{seed, pattern}, ArtifactReference{}, namespace+"default.brep", namespace+"default.glb")
	if err != nil {
		t.Fatal(err)
	}
	if result.GetTriangleCount() == 0 {
		t.Fatal("empty pattern")
	}
	manifest, blocks := readWorkerNamingArtifact(t, root, result)
	if len(blocks) != 2 || !manifest.Bodies[0].Complete {
		t.Fatal("pattern history missing")
	}
	slots := map[int]bool{}
	for _, body := range manifest.GetBodies() {
		for _, locator := range body.GetTip() {
			evidence := manifest.GetEvidence()[locator.GetEvidence()-1]
			if cylinder := evidence.GetCylinder(); cylinder != nil {
				origin := cylinder.GetOrigin()
				a := math.Atan2(origin.GetY(), origin.GetX())
				if a < 0 {
					a += 2 * math.Pi
				}
				slot := int(math.Round(a/(math.Pi/3))) % 6
				expected := float64(slot) * math.Pi / 3
				if math.Hypot(origin.GetX()-20*math.Cos(expected), origin.GetY()-20*math.Sin(expected)) > 1e-7 || math.Abs(evidence.GetRadiusMm()-2) > 1e-9 || math.Abs(evidence.GetMeasureSi()-20*math.Pi*1e-6) > 1e-10 {
					t.Fatal("incorrect pattern dimensions/distribution", evidence)
				}
				if slots[slot] {
					t.Fatal("duplicate circular member", slot)
				}
				slots[slot] = true
			}
		}
	}
	if len(slots) != 6 {
		t.Fatal("missing cylindrical member", slots)
	}

}

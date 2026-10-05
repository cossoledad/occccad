package workspace

import (
	"math"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestReplicationToolCapabilities(t *testing.T) {
	for _, kind := range []string{"PAD", "LINEAR_EXTRUDE", "REVOLVE", "LOFT"} {
		for _, op := range []string{"ADD", "REMOVE", "NEW_BODY"} {
			c := replicationSource(Feature{Type: kind, Operation: op})
			if !c.Tool || c.RangeStart != (op != "REMOVE") {
				t.Fatal(kind, op, c)
			}
		}
		if replicationSource(Feature{Type: kind, Extent: "THROUGH_ALL", Operation: "REMOVE"}).Tool {
			t.Fatal("through-all cannot be transformed as a finite tool")
		}
	}
	for _, kind := range []string{"FILLET", "CHAMFER", "SHELL", "BOOLEAN", "SOLID_PATTERN"} {
		if replicationSource(Feature{Type: kind}).Tool {
			t.Fatal("invented independent tool", kind)
		}
	}
	c := replicationSources(PartModel{Features: []Feature{{ID: "loft", Type: "LOFT", Operation: "ADD", EvaluationStatus: "FAILED"}}})["loft"]
	if c.Tool || c.BodyStage {
		t.Fatal("failed stage exposed")
	}
}

func TestMirrorPlaneReflectionAndValidation(t *testing.T) {
	p := PatternDefinition{ID: "mirror", Kind: "MIRROR", Count: 2, Distribution: "FIXED_STEP", Origin: [3]float64{2, 1, 0}, Direction: [3]float64{1, 1, 0}}
	m, err := patternPlacements(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 2 || m[0].point([3]float64{6, 3, 4}) != [3]float64{6, 3, 4} {
		t.Fatal("seed moved")
	}
	point := [3]float64{6, 3, 4}
	reflected := m[1].point(point)
	again := m[1].point(reflected)
	for i := range point {
		if math.Abs(again[i]-point[i]) > 1e-10 {
			t.Fatal("reflection not involutive")
		}
	}
	if math.Abs(reflected[0])+math.Abs(reflected[1]+3) > 1e-10 || reflected[2] != 4 {
		t.Fatal(reflected)
	}
	for _, bad := range []PatternDefinition{func() PatternDefinition { x := p; x.Count = 3; return x }(), func() PatternDefinition { x := p; x.SkippedSlots = []int{1}; return x }(), func() PatternDefinition { x := p; x.Angle = 180; return x }()} {
		if _, e := patternPlacements(bad); e == nil {
			t.Fatal("accepted invalid mirror")
		}
	}
	if len(patternParameters(&p)) != 0 {
		t.Fatal("mirror has editable array count")
	}
	seed := Feature{ID: "loft", Type: "LOFT", BodyID: "body", Operation: "REMOVE"}
	f := Feature{ID: "mirror", Type: "SOLID_PATTERN", BodyID: "body", Operation: "REMOVE", Pattern: &FeaturePattern{PatternDefinition: p, SourceKind: "GENERATOR_TOOL", Source: FeatureStageRef{BodyID: "body", FeatureID: "loft"}}}
	if validateFeaturePattern(f, map[string]Feature{"loft": seed}) == nil {
		t.Fatal("missing plane accepted")
	}
	f.Pattern.MirrorPlaneID = "datum-yz"
	if e := validateFeaturePattern(f, map[string]Feature{"loft": seed}); e != nil {
		t.Fatal(e)
	}
	f.Type = "SKETCH_PATTERN"
	if validateFeaturePattern(f, map[string]Feature{"loft": seed}) == nil {
		t.Fatal("sketch mirror exposed")
	}
}

func TestMirrorDatumDependencyAndDeleteProtection(t *testing.T) {
	model := newPartModel()
	model.DatumPlanes = append(model.DatumPlanes, DatumPlane{ID: "mirror-plane", Plane: "CUSTOM", Normal: [3]float64{1, 0, 0}, UDirection: [3]float64{0, 1, 0}, Size: 180})
	model.Features = []Feature{{ID: "source", Type: "IMPORT_BODY", BodyID: model.ActiveBodyID}, {ID: "mirror", Type: "SOLID_PATTERN", BodyID: model.ActiveBodyID, Operation: "ADD", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{ID: "mirror", Kind: "MIRROR", Distribution: "FIXED_STEP", Count: 2, Direction: [3]float64{1, 0, 0}, MirrorPlaneID: "mirror-plane"}, Source: FeatureStageRef{BodyID: model.ActiveBodyID, FeatureID: "source"}, SourceKind: "BODY_STAGE"}}}
	normalizePartModel(&model)
	if _, _, e := deleteDatum(model, "DATUM_PLANE", "mirror-plane"); e == nil {
		t.Fatal("deleted mirror support")
	}
	_, manifest, e := buildPartEvaluation(model, "revision", "hash", []modelcore.DependencyKey{"datum:mirror-plane"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, key := range manifest.DirtyNodes {
		if key == "feature:mirror" {
			found = true
		}
	}
	if !found {
		t.Fatal("plane change did not dirty mirror")
	}
	input := featureInputs(model.Features[1])
	found = false
	for _, r := range input {
		if r.Role == "MIRROR_PLANE" && r.FeatureID == "mirror-plane" {
			found = true
		}
	}
	if !found {
		t.Fatal("plane reference absent from ordered definition inputs")
	}
}

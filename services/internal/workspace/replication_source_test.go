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
			if !c.Tool || !c.RangeStart {
				t.Fatal(kind, op, c)
			}
		}
		for _, extent := range []string{"THROUGH_ALL", "TO_PLANE", "UNKNOWN"} {
			if replicationSource(Feature{Type: kind, Extent: extent, Operation: "REMOVE"}).Tool {
				t.Fatal("non-finite extent exposed as generator tool", extent)
			}
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

func TestAutomaticPatternSourceKeepsSignedToolsAndExplicitStages(t *testing.T) {
	m := PartModel{Features: []Feature{{ID: "base", BodyID: "b", Type: "LINEAR_EXTRUDE", Operation: "ADD"}, {ID: "round", BodyID: "b", Type: "FILLET"}, {ID: "cut", BodyID: "b", Type: "LINEAR_EXTRUDE", Operation: "REMOVE"}, {ID: "mirror", BodyID: "b", Type: "SOLID_PATTERN", Operation: "REMOVE", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{Count: 2}, SourceKind: "GENERATOR_TOOL", Source: FeatureStageRef{BodyID: "b", FeatureID: "cut"}}}}}
	for _, tc := range []struct{ source, kind, op, start string }{{"base", "GENERATOR_TOOL", "ADD", ""}, {"round", "FEATURE_DELTA", "ADD", "base"}, {"cut", "GENERATOR_TOOL", "REMOVE", ""}, {"mirror", "GENERATOR_TOOL", "REMOVE", ""}} {
		f := Feature{ID: "new", BodyID: "b", Type: "SOLID_PATTERN", Operation: "ADD", Pattern: &FeaturePattern{SourceKind: "AUTO", Source: FeatureStageRef{BodyID: "b", FeatureID: tc.source}}}
		if err := resolveAutomaticPatternSource(m, &f); err != nil {
			t.Fatal(err)
		}
		if f.Pattern.SourceKind != tc.kind || f.Operation != tc.op || f.Pattern.StartFeatureID != tc.start {
			t.Fatal(tc, f)
		}
	}
	f := Feature{Type: "SOLID_PATTERN", Operation: "ADD", Pattern: &FeaturePattern{SourceKind: "BODY_STAGE", Source: FeatureStageRef{BodyID: "b", FeatureID: "mirror"}}}
	if err := resolveAutomaticPatternSource(m, &f); err != nil || f.Pattern.SourceKind != "BODY_STAGE" || f.Operation != "ADD" {
		t.Fatal("accepted Body stage reinterpreted", err, f)
	}
	f.ID = "mirror"
	f.Pattern.SourceKind = "AUTO"
	if err := resolveAutomaticPatternSource(m, &f); err == nil {
		t.Fatal("self reference accepted")
	}
	m.Features[2].EvaluationStatus = "FAILED"
	if replicationSources(m)["cut"].SourceKind != "" {
		t.Fatal("failed stage advertised")
	}
}

func TestPatternSeedSetResolvesEveryInputAndRejectsMixedOperations(t *testing.T) {
	m := PartModel{Features: []Feature{{ID: "a", BodyID: "b", Type: "LINEAR_EXTRUDE", Operation: "REMOVE"}, {ID: "c", BodyID: "b", Type: "LINEAR_EXTRUDE", Operation: "REMOVE"}}}
	f := Feature{ID: "array", BodyID: "b", Type: "SOLID_PATTERN", Operation: "ADD", Pattern: &FeaturePattern{Source: FeatureStageRef{BodyID: "b", FeatureID: "a"}, SourceKind: "AUTO", AdditionalSources: []PatternSeed{{Source: FeatureStageRef{BodyID: "b", FeatureID: "c"}, SourceKind: "AUTO"}}, PatternDefinition: PatternDefinition{ID: "array", Kind: "LINEAR", Count: 2, Spacing: 10, Distribution: "FIXED_STEP", Direction: [3]float64{1, 0, 0}}}}
	if err := resolveAutomaticPatternSource(m, &f); err != nil {
		t.Fatal(err)
	}
	if f.Operation != "REMOVE" || f.Pattern.AdditionalSources[0].SourceKind != "GENERATOR_TOOL" {
		t.Fatal(f)
	}
	earlier := map[string]Feature{"a": m.Features[0], "c": m.Features[1]}
	if err := validateFeaturePattern(f, earlier); err != nil {
		t.Fatal(err)
	}
	inputs := featureInputs(f)
	found := map[string]bool{}
	for _, input := range inputs {
		if input.Role == "SEED" {
			found[input.FeatureID] = true
		}
	}
	if !found["a"] || !found["c"] {
		t.Fatal(inputs)
	}
	delete(earlier, "c")
	if validateFeaturePattern(f, earlier) == nil {
		t.Fatal("missing additional seed accepted")
	}
	m.Features[1].Operation = "ADD"
	f.Pattern.AdditionalSources[0].SourceKind = "AUTO"
	if resolveAutomaticPatternSource(m, &f) == nil {
		t.Fatal("mixed signs silently accepted")
	}
}

func TestPatternSeedsCopyMaximalSelectedRangesWithoutDroppingReferences(t *testing.T) {
	m := PartModel{Features: []Feature{{ID: "base", BodyID: "b", Type: "LINEAR_EXTRUDE"}, {ID: "boss", BodyID: "b", Type: "LINEAR_EXTRUDE"}, {ID: "round", BodyID: "b", Type: "FILLET"}, {ID: "bevel", BodyID: "b", Type: "CHAMFER"}, {ID: "other", BodyID: "b", Type: "LINEAR_EXTRUDE"}}}
	p := &FeaturePattern{Source: FeatureStageRef{BodyID: "b", FeatureID: "boss"}, SourceKind: "GENERATOR_TOOL", AdditionalSources: []PatternSeed{{Source: FeatureStageRef{BodyID: "b", FeatureID: "round"}, SourceKind: "FEATURE_DELTA", StartFeatureID: "boss"}, {Source: FeatureStageRef{BodyID: "b", FeatureID: "bevel"}, SourceKind: "FEATURE_DELTA", StartFeatureID: "boss"}, {Source: FeatureStageRef{BodyID: "b", FeatureID: "other"}, SourceKind: "GENERATOR_TOOL"}}}
	got := effectivePatternSeeds(m, p)
	if len(got) != 2 || got[0].Source.FeatureID != "bevel" || got[1].Source.FeatureID != "other" {
		t.Fatal(got)
	}
	if len(patternSeeds(p)) != 4 {
		t.Fatal("formal input set was rewritten")
	}
	inputs := featureInputs(Feature{Pattern: p})
	found := map[string]bool{}
	for _, r := range inputs {
		if r.Role == "SEED" {
			found[r.FeatureID] = true
		}
	}
	if len(found) != 4 {
		t.Fatal("covered inputs lost dependency identity", inputs)
	}
	p.AdditionalSources = nil
	if len(effectivePatternSeeds(m, p)) != 1 {
		t.Fatal("standalone tool changed")
	}
	p.SourceKind = "BODY_STAGE"
	p.AdditionalSources = []PatternSeed{{Source: FeatureStageRef{BodyID: "b", FeatureID: "round"}, SourceKind: "FEATURE_DELTA", StartFeatureID: "boss"}}
	if len(effectivePatternSeeds(m, p)) != 2 {
		t.Fatal("explicit Body-stage intent was reinterpreted")
	}
}

func TestBlindHoleReplicationCapabilitiesAndAutomaticSource(t *testing.T) {
	for _, kind := range []string{"", "SIMPLE", "COUNTERBORE", "COUNTERSINK"} {
		hole := Feature{ID: "hole", BodyID: "body", Type: "HOLE", Operation: "REMOVE", Extent: "BLIND", HoleKind: kind, Diameter: 2, Depth: 5, RecessDiameter: 4, RecessDepth: 1, CountersinkAngle: 90}
		model := PartModel{Features: []Feature{hole}}
		capability := replicationSources(model)[hole.ID]
		if !capability.Tool || capability.SourceKind != "GENERATOR_TOOL" || capability.Operation != "REMOVE" || !capability.RangeStart || capability.RangeModifier || !capability.BodyStage {
			t.Fatal(kind, capability)
		}
		pattern := Feature{Type: "SOLID_PATTERN", BodyID: "body", Operation: "ADD", Pattern: &FeaturePattern{Source: FeatureStageRef{BodyID: "body", FeatureID: "hole"}, SourceKind: "AUTO"}}
		if e := resolveAutomaticPatternSource(model, &pattern); e != nil || pattern.Operation != "REMOVE" || pattern.Pattern.SourceKind != "GENERATOR_TOOL" {
			t.Fatal(pattern, e)
		}
		pattern.Operation = "ADD"
		pattern.Pattern.SourceKind = "BODY_STAGE"
		if e := resolveAutomaticPatternSource(model, &pattern); e != nil || pattern.Operation != "ADD" || pattern.Pattern.SourceKind != "BODY_STAGE" {
			t.Fatal("Body-stage intent changed", e)
		}
		through := hole
		through.Extent = "THROUGH_ALL"
		through.Depth = 0 // Directional tools have no finite depth parameter.
		if c := replicationSource(through); !c.Tool || c.RangeStart || c.RangeModifier || !c.BodyStage {
			t.Fatal("through Hole capability", kind, c)
		}
		pattern.Pattern.SourceKind = "AUTO"
		if e := resolveAutomaticPatternSource(PartModel{Features: []Feature{through}}, &pattern); e != nil || pattern.Operation != "REMOVE" || pattern.Pattern.SourceKind != "GENERATOR_TOOL" {
			t.Fatal("through AUTO", kind, pattern, e)
		}
		for _, extent := range []string{"TO_PLANE", "FINITE", ""} {
			bad := hole
			bad.Extent = extent
			capability := replicationSource(bad)
			if capability.Tool || !capability.BodyStage {
				t.Fatal("unsupported Hole became tool", bad, capability)
			}
			pattern.Pattern.SourceKind = "AUTO"
			if e := resolveAutomaticPatternSource(PartModel{Features: []Feature{bad}}, &pattern); e == nil {
				t.Fatal("unsupported AUTO silently downgraded", extent)
			}
		}
		for _, bad := range []Feature{func() Feature { x := hole; x.Depth = 0; return x }(), func() Feature { x := hole; x.HoleKind = "UNKNOWN"; return x }(), func() Feature { x := hole; x.Operation = "ADD"; return x }()} {
			if replicationSource(bad).Tool {
				t.Fatal("invalid Hole became a tool", bad)
			}
		}
	}
}

func TestPatternToolRequiresEffectiveAdditionalMembers(t *testing.T) {
	for _, tc := range []struct {
		count   float64
		skipped []int
		tool    bool
	}{
		{1, nil, false}, {3, []int{1, 2}, false}, {3, []int{1, 2, 9}, false}, {3, []int{1}, true}, {2, nil, true},
	} {
		f := Feature{ID: "array", Type: "SOLID_PATTERN", Operation: "ADD", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{Count: tc.count, SkippedSlots: tc.skipped}}}
		if got := replicationSource(f).Tool; got != tc.tool {
			t.Fatalf("count=%v skipped=%v tool=%v", tc.count, tc.skipped, got)
		}
	}
}
func TestDraftReplicationRangeRetainsExplicitContract(t *testing.T) {
	m := PartModel{Features: []Feature{{ID: "base", BodyID: "b", Type: "LINEAR_EXTRUDE", Operation: "ADD", Order: 0}, {ID: "draft", BodyID: "b", Type: "DRAFT", Order: 1}, {ID: "round", BodyID: "b", Type: "FILLET", Order: 2}}}
	caps := replicationSources(m)
	if caps["draft"].Tool || caps["draft"].SourceKind != "FEATURE_DELTA" || caps["draft"].StartFeatureID != "base" || caps["round"].StartFeatureID != "base" {
		t.Fatal(caps)
	}
	p := Feature{ID: "copies", Type: "SOLID_PATTERN", BodyID: "b", Order: 3, Operation: "ADD", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{ID: "copies", Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 20, Direction: [3]float64{1, 0, 0}}, Source: FeatureStageRef{BodyID: "b", FeatureID: "draft"}, SourceKind: "FEATURE_DELTA", StartFeatureID: "base"}}
	if e := validateFeaturePattern(p, map[string]Feature{"base": m.Features[0], "draft": m.Features[1]}); e != nil {
		t.Fatal(e)
	}
}

func TestFiniteRemovalRangeCapabilitiesAndAuto(t *testing.T) {
	starts := []Feature{{Type: "LINEAR_EXTRUDE", Extent: "FINITE"}, {Type: "LINEAR_EXTRUDE", Extent: "SYMMETRIC"}, {Type: "LINEAR_EXTRUDE", Extent: "TWO_SIDED"}, {Type: "REVOLVE"}, {Type: "LOFT"}}
	for _, kind := range []string{"SIMPLE", "COUNTERBORE", "COUNTERSINK"} {
		starts = append(starts, Feature{Type: "HOLE", Extent: "BLIND", HoleKind: kind, Diameter: 2, Depth: 6, RecessDiameter: 4, RecessDepth: 2, CountersinkAngle: 90})
	}
	for _, start := range starts {
		start.ID = "cut"
		start.BodyID = "body"
		start.Operation = "REMOVE"
		start.Order = 0
		m := PartModel{Features: []Feature{start, {ID: "round", BodyID: "body", Type: "FILLET", Order: 1}, {ID: "bevel", BodyID: "body", Type: "CHAMFER", Order: 2}, {ID: "draft", BodyID: "body", Type: "DRAFT", Order: 3}}}
		caps := replicationSources(m)
		if !caps[start.ID].RangeStart || caps["draft"].SourceKind != "FEATURE_DELTA" || caps["draft"].Operation != "REMOVE" || caps["draft"].StartFeatureID != start.ID {
			t.Fatal(start, caps)
		}
		f := Feature{ID: "copies", Type: "SOLID_PATTERN", BodyID: "body", Operation: "ADD", Pattern: &FeaturePattern{Source: FeatureStageRef{BodyID: "body", FeatureID: "draft"}, SourceKind: "AUTO", PatternDefinition: PatternDefinition{ID: "copies", Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 2, Spacing: 20, Direction: [3]float64{1, 0, 0}}}}
		if e := resolveAutomaticPatternSource(m, &f); e != nil || f.Operation != "REMOVE" || f.Pattern.StartFeatureID != start.ID {
			t.Fatal(start, f, e)
		}
		earlier := map[string]Feature{}
		for _, v := range m.Features {
			earlier[v.ID] = v
		}
		if e := validateFeaturePattern(f, earlier); e != nil {
			t.Fatal(start, e)
		}
		f.Operation = "ADD"
		if e := validateFeaturePattern(f, earlier); e == nil {
			t.Fatal("subtractive range accepted additive operation")
		}
		f.Operation = "REMOVE"
		p := *f.Pattern
		p.AdditionalSources = []PatternSeed{{Source: FeatureStageRef{BodyID: "body", FeatureID: start.ID}, SourceKind: "GENERATOR_TOOL"}, {Source: FeatureStageRef{BodyID: "body", FeatureID: "round"}, SourceKind: "FEATURE_DELTA", StartFeatureID: start.ID}}
		f.Pattern = &p
		if got := effectivePatternSeeds(m, f.Pattern); len(got) != 1 || got[0].Source.FeatureID != "draft" || len(patternSeeds(f.Pattern)) != 3 {
			t.Fatal("raw cutter or earlier range recopied", got)
		}
		for _, bad := range []Feature{{Type: "HOLE", Extent: "THROUGH_ALL", Diameter: 2}, {Type: "HOLE", Extent: "TO_PLANE", Diameter: 2}, {Type: "LINEAR_EXTRUDE", Extent: "THROUGH_ALL"}, {Type: "LINEAR_EXTRUDE", Extent: "TO_PLANE"}, {Type: "BOOLEAN"}, {Type: "SHELL"}, {Type: "SOLID_PATTERN"}} {
			bad.ID = start.ID
			bad.BodyID = "body"
			bad.Operation = "REMOVE"
			earlier[bad.ID] = bad
			if e := validateFeaturePattern(f, earlier); e == nil {
				t.Fatal("unsupported range start", bad)
			}
		}
	}
}

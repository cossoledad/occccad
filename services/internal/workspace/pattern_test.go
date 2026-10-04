package workspace

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestPatternDistributionAndStableSlots(t *testing.T) {
	p := PatternDefinition{ID: "holes", Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 6, Direction: [3]float64{0, 0, 1}, Phase: 30}
	for _, count := range []float64{6, 8, 4, 1} {
		p.Count = count
		placements, err := patternPlacements(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(placements) != int(count) {
			t.Fatal("wrong count")
		}
		for _, m := range placements {
			v := m.point([3]float64{20, 0, 3})
			a := (30 + 360*float64(m.Slot)/count) * math.Pi / 180
			if math.Abs(v[0]-20*math.Cos(a))+math.Abs(v[1]-20*math.Sin(a))+math.Abs(v[2]-3) > 1e-10 {
				t.Fatal("incorrect placement", v)
			}
		}
	}
	p.Count = 8
	p.SkippedSlots = []int{2, 6}
	placements, err := patternPlacements(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 6 || placements[2].Slot != 3 || placements[5].Slot != 7 {
		t.Fatal("skip renumbered slots")
	}
	member := patternMemberID(p.ID, 3, "circle")
	p.Count = 4
	placements, err = patternPlacements(p)
	if err != nil {
		t.Fatal(err)
	}
	if patternMemberID(p.ID, placements[2].Slot, "circle") != member {
		t.Fatal("member identity changed")
	}
	for _, bad := range []float64{0, -1, 1.5, 257, math.NaN(), math.Inf(1)} {
		p.Count = bad
		if _, err := patternPlacements(p); err == nil {
			t.Fatal("accepted invalid count", bad)
		}
	}
	p = PatternDefinition{ID: "line", Kind: "LINEAR", Distribution: "TOTAL_SPAN", Count: 4, Spacing: 30, Direction: [3]float64{2, 0, 0}}
	placements, err = patternPlacements(p)
	if err != nil {
		t.Fatal(err)
	}
	if placements[3].point([3]float64{}) != [3]float64{30, 0, 0} {
		t.Fatal("incorrect span")
	}
	p.Count = 1
	placements, err = patternPlacements(p)
	if err != nil || placements[0].point([3]float64{}) != [3]float64{} {
		t.Fatal("count one", err)
	}
}

func TestPatternParametersHistoryAndSpatialMember(t *testing.T) {
	model := newPartModel()
	seed := testRectangleSketch("seed", "XY")
	model.Features = append(model.Features, seed)
	normalizePartModel(&model)
	pattern := Feature{ID: "frames", Type: "SKETCH_PATTERN", BodyID: model.Features[0].BodyID, Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{ID: "frames", Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 6, Spacing: 10, Direction: [3]float64{0, 0, 1}}, Source: FeatureStageRef{BodyID: model.Features[0].BodyID, FeatureID: "seed"}, SourceKind: "SKETCH_FRAME"}}
	initial, _ := json.Marshal(model)
	payload, _ := json.Marshal(createFeaturePayload{Feature: pattern})
	after, changes, err := applyCreateFeature(initial, payload)
	if err != nil {
		t.Fatal(err)
	}
	var current PartModel
	if err = json.Unmarshal(after, &current); err != nil {
		t.Fatal(err)
	}
	if len(current.Features) != 2 || len(current.Parameters) != 2 {
		t.Fatal("pattern was not persisted with parameters")
	}
	countID := "parameter:frames:pattern:count"
	spacingID := "parameter:frames:pattern:spacing"
	expression, err := modelcore.CompileExpression("10 mm * n", map[string]modelcore.ParameterBinding{"n": {ParameterID: countID, Dimension: modelcore.Dimensionless}}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	for i := range current.Parameters {
		p := &current.Parameters[i]
		if p.ParameterID == countID {
			p.Key = "renamed_count"
		}
		if p.ParameterID == spacingID {
			p.Source = modelcore.ValueSource{Expression: &expression}
		}
	}
	for _, count := range []float64{8, 4, 6} {
		for i := range current.Parameters {
			p := &current.Parameters[i]
			if p.ParameterID == countID {
				q, _ := modelcore.NewQuantity(count, "1")
				p.Source = modelcore.ValueSource{Literal: &q}
			}
		}
		if err = validateAndResolvePartParameters(&current); err != nil {
			t.Fatal(err)
		}
		if current.Features[1].Pattern.Spacing != count*10 {
			t.Fatal("stable parameter dependency lost on rename")
		}
		features := map[string]Feature{}
		for _, f := range current.Features {
			features[f.ID] = f
		}
		slot := 5
		member, e := resolvePatternSketch(current, features, "frames", &slot)
		if count == 4 {
			if e == nil || !strings.Contains(e.Error(), "PATTERN_MEMBER_MISSING") {
				t.Fatal("missing member rebound", e)
			}
			continue
		}
		if e != nil {
			t.Fatal(e)
		}
		origin, _, _, valid := supportFrame(current, member.Sketch.Support)
		if !valid || origin[2] != count*50 {
			t.Fatal("incorrect spatial frame", origin)
		}
		regions, e := buildProfileRegions(member)
		if e != nil || len(regions) != 1 {
			t.Fatal("spatial member is not a formal profile", e)
		}
	}
	applyHistory := func(raw json.RawMessage, undo bool) json.RawMessage {
		t.Helper()
		values, e := modelValues("PART", raw, changes)
		if e != nil {
			t.Fatal(e)
		}
		var desired map[modelcore.PropertyAddress]json.RawMessage
		if undo {
			desired, e = changes.Compensate(values)
		} else {
			desired, e = changes.Reapply(values)
		}
		if e != nil {
			t.Fatal(e)
		}
		out, e := applyModelValues("PART", raw, desired)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	undone := applyHistory(after, true)
	redone := applyHistory(undone, false)
	var restored PartModel
	_ = json.Unmarshal(redone, &restored)
	if err = validateAndResolvePartParameters(&restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Features) != 2 || restored.Features[1].Pattern.ID != "frames" {
		t.Fatal("redo lost definition")
	}
}

func TestPatternSketchProjectionTracksSeedWithoutSolverCopies(t *testing.T) {
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Entities: []SketchEntity{{ID: "seed", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 20}, Radius: 2}}, Patterns: []SketchPattern{{PatternDefinition: PatternDefinition{ID: "holes", Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 6, Direction: [3]float64{0, 0, 1}}, EntityIDs: []string{"seed"}}}}
	first, err := evaluatedSketchPatternEntities(sketch)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 6 || len(sketch.Entities) != 1 || len(sketch.Constraints) != 0 {
		t.Fatal("expanded solver state")
	}
	sketch.Entities[0].Radius = 3
	sketch.Entities[0].Center.X = 25
	sketch.Patterns[0].Count = 8
	second, err := evaluatedSketchPatternEntities(sketch)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range second {
		if e.Radius != 3 || math.Abs(math.Hypot(e.Center.X, e.Center.Y)-25) > 1e-10 {
			t.Fatal("seed not propagated")
		}
		if i < 6 && first[i].ID != e.ID {
			t.Fatal("identity changed")
		}
	}
	sketch.Patterns[0].Count = 4
	sketch.Patterns[0].SkippedSlots = []int{2}
	third, err := evaluatedSketchPatternEntities(sketch)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range third {
		if e.ID == first[2].ID || e.ID == first[4].ID {
			t.Fatal("removed member silently rebound")
		}
	}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"seed"}}}); err == nil {
		// Full validation must reject dangling pattern sources before solving.
		if validateSketch(sketch) == nil {
			t.Fatal("dangling seed accepted")
		}
	}
}

func TestPatternDetachPreservesClosedProfiles(t *testing.T) {
	feature := testRectangleSketch("seed", "XY")
	var seeds []string
	for _, entity := range feature.Sketch.Entities {
		seeds = append(seeds, entity.ID)
	}
	feature.Sketch.Patterns = []SketchPattern{{PatternDefinition: PatternDefinition{ID: "rectangles", Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 100, Direction: [3]float64{1, 0, 0}}, EntityIDs: seeds}}
	before, err := buildProfileRegions(feature)
	if err != nil || len(before) != 3 {
		t.Fatalf("linked profiles: %v %v", len(before), err)
	}
	if err = applySketchPatternOperation(feature.Sketch, SketchOperation{Type: "DETACH_PATTERN", PatternID: "rectangles"}); err != nil {
		t.Fatal(err)
	}
	after, err := buildProfileRegions(feature)
	if err != nil || len(after) != 3 {
		t.Fatalf("detached profiles: %v %v", len(after), err)
	}
	if len(feature.Sketch.Patterns) != 0 {
		t.Fatal("detachment retained link")
	}
}

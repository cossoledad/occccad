package workspace

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"
)

func patternReferenceFixture() PartModel {
	model := newPartModel()
	f := testRectangleSketch("sketch", "XY")
	f.Sketch.Entities = []SketchEntity{
		{ID: "seed", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 20, Y: 20}, Radius: 2},
		{ID: "center", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 10, Y: 20}},
		{ID: "direction", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}},
	}
	f.Sketch.Constraints = nil
	f.Sketch.Patterns = []SketchPattern{{PatternDefinition: PatternDefinition{ID: "pattern", Kind: "CIRCULAR", Distribution: "FULL_CIRCLE", Count: 4, Direction: [3]float64{0, 0, 1}, CenterReference: &PatternPointReference{SketchID: "sketch", Reference: SketchGeometryRef{Target: "ENTITY", EntityID: "center", SubElement: "POINT"}}}, EntityIDs: []string{"seed"}}}
	f.Sketch.Solve = SketchSolveState{Status: "UNDER_CONSTRAINED", DefinitionStatus: "UNDER_CONSTRAINED", Components: []SketchSolveComponent{{EntityIDs: []string{"seed"}, Status: "FULLY_CONSTRAINED", DefinitionStatus: "FULLY_CONSTRAINED"}}}
	model.Features = []Feature{f}
	normalizePartModel(&model)
	return model
}

func TestPatternPickedReferencesFollowGeometryAndReverse(t *testing.T) {
	model := patternReferenceFixture()
	check := func(x, y float64) {
		t.Helper()
		if err := resolveSketchPatternReferences(&model); err != nil {
			t.Fatal(err)
		}
		entities, err := evaluatedSketchPatternEntities(*model.Features[0].Sketch)
		if err != nil {
			t.Fatal(err)
		}
		id := patternMemberID("pattern", 1, "seed")
		for _, e := range entities {
			if e.ID == id {
				if math.Hypot(e.Center.X-x, e.Center.Y-y) > 1e-9 {
					t.Fatalf("member center=%+v want=(%g,%g)", e.Center, x, y)
				}
				return
			}
		}
		t.Fatal("member missing")
	}
	check(10, 30)
	model.Features[0].Sketch.Entities[1].Point.X = 5
	check(5, 35)
	p := &model.Features[0].Sketch.Patterns[0]
	p.Reversed = true
	check(5, 5)
	p.Kind = "LINEAR"
	p.Distribution = "FIXED_STEP"
	p.Spacing = 10
	p.CenterReference = nil
	p.AxisEntityID = "SKETCH_LINE:sketch:direction"
	check(10, 20)
	model.Features[0].Sketch.Entities[2].End = &SketchPoint2{Y: 10}
	check(20, 10)
	p.AxisEntityID = "SKETCH_AXIS:U"
	check(10, 20)
	p.AxisEntityID = "AXIS_SYSTEM:axis-system-default:Z"
	if resolveSketchPatternReferences(&model) == nil {
		t.Fatal("accepted a direction outside the sketch plane")
	}
	p.AxisEntityID = "SKETCH_LINE:sketch:missing"
	if resolveSketchPatternReferences(&model) == nil {
		t.Fatal("missing line silently became a literal direction")
	}
}

func TestPatternTreeAndMemberConstraintProjection(t *testing.T) {
	model := patternReferenceFixture()
	if err := resolveSketchPatternReferences(&model); err != nil {
		t.Fatal(err)
	}
	f := model.Features[0]
	node := featureStructureNode(f, "part/body", "doc", "rev", "digest", true, true)
	var pattern *DocumentStructureNode
	for i := range node.Children {
		if node.Children[i].Kind == "SKETCH_PATTERN_DEFINITION" {
			pattern = &node.Children[i]
		}
	}
	if pattern == nil || !slices.Contains(pattern.Capabilities, "EDIT") || !slices.Contains(pattern.Capabilities, "DELETE") || len(pattern.Children) != 4 {
		t.Fatalf("pattern missing from editable tree: %+v", pattern)
	}
	for _, child := range pattern.Children {
		if child.Diagnostic != "FULLY_CONSTRAINED" || len(child.Capabilities) != 0 {
			t.Fatal("member lost seed state or became independently editable", child)
		}
	}
	manifest := visualizationManifest(model)
	count := 0
	for _, p := range manifest.Primitives {
		if p.EntityType == "CIRCLE" {
			count++
			if p.Status != "FULLY_CONSTRAINED" || !p.Selectable {
				t.Fatal("unrelated free geometry changed pattern state", p)
			}
		}
	}
	if count != 4 {
		t.Fatal("missing members", count)
	}
	ensureFeatureParameters(&model)
	raw, _ := json.Marshal(model)
	payload, _ := json.Marshal(deleteNodePayload{TargetKind: "SKETCH_PATTERN_DEFINITION", TargetID: "pattern", OwnerEntityID: f.ID})
	deleted, changes, err := applyDeletePartNode(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var after PartModel
	_ = json.Unmarshal(deleted, &after)
	if len(after.Features[0].Sketch.Patterns) != 0 || len(after.Features[0].Sketch.Entities) != 3 || len(changes.Changes) == 0 {
		t.Fatal("tree delete did not remove definition atomically")
	}
}

func TestSpatialPatternHasSeparateTreeAndVisualMembers(t *testing.T) {
	model := patternReferenceFixture()
	model.Features[0].Sketch.Patterns = nil
	p := Feature{ID: "frames", Name: "Frames", Type: "SKETCH_PATTERN", BodyID: model.Features[0].BodyID, Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{ID: "frames", Kind: "LINEAR", Distribution: "FIXED_STEP", Count: 3, Spacing: 10, Direction: [3]float64{0, 0, 1}}, SourceKind: "SKETCH_FRAME", Source: FeatureStageRef{FeatureID: "sketch", BodyID: model.Features[0].BodyID}}}
	model.Features = append(model.Features, p)
	tree := featureStructureNode(p, "part/body", "doc", "rev", "digest", true, true)
	if len(tree.Children) != 3 {
		t.Fatal("space members missing")
	}
	visible := map[string]bool{}
	for _, primitive := range visualizationManifest(model).Primitives {
		if primitive.PatternID == p.ID {
			if primitive.PatternMemberSlot == nil || primitive.SketchMemberID != primitive.FeatureID {
				t.Fatal("lost member binding", primitive)
			}
			if primitive.EntityType == "CIRCLE" && primitive.DisplayEntityID != "seed" {
				t.Fatal("display lost the source entity identity used by downstream seams", primitive)
			}
			visible[primitive.FeatureID] = true
		}
	}
	for slot, node := range tree.Children {
		if node.PatternMemberSlot == nil || *node.PatternMemberSlot != slot || !visible[node.EntityID] {
			t.Fatal("tree/visual member identities disagree", node)
		}
	}
	earlier := map[string]Feature{"sketch": model.Features[0], "frames": p}
	first, last := 0, 2
	a, err := resolvePatternSketch(model, earlier, p.ID, &first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolvePatternSketch(model, earlier, p.ID, &last)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.Sketch.Support.Origin[2] != 0 || b.Sketch.Support.Origin[2] != 20 {
		t.Fatal("members not independent profiles")
	}
}

func TestPatternReferencesStayInSketchAndUseProjection(t *testing.T) {
	model := patternReferenceFixture()
	sketch := model.Features[0].Sketch
	p := &sketch.Patterns[0]
	// A non-world sketch frame must not change the local definition.
	sketch.Support = SketchSupport{Type: "PATTERN_FRAME", Origin: [3]float64{70, 80, 90}, Normal: [3]float64{1, 0, 0}, XDirection: [3]float64{0, 1, 0}}
	p.Kind = "LINEAR"
	p.Distribution = "FIXED_STEP"
	p.Spacing = 5
	p.DirectionReference = &SketchGeometryRef{Target: "SKETCH_Y_AXIS", SubElement: "DIRECTION"}
	if err := resolveSketchPatternReferences(&model); err != nil {
		t.Fatal(err)
	}
	if (p.Direction[0] != 0 || p.Direction[1] <= 0 || p.Direction[2] != 0) || p.Origin != [3]float64{10, 20, 0} {
		t.Fatal("world frame leaked into sketch", p)
	}
	sketch.ExternalGeometry = []SketchExternalGeometry{{ID: "projection", Status: "CONNECTED", Snapshot: &SketchExternalGeometrySnapshot{Kind: "LINE", Start: &SketchPoint2{X: 3, Y: 4}, End: &SketchPoint2{X: 7, Y: 4}}}}
	p.DirectionReference = &SketchGeometryRef{Target: "EXTERNAL", EntityID: "projection", SubElement: "DIRECTION"}
	p.CenterReference = &PatternPointReference{SketchID: "sketch", Reference: SketchGeometryRef{Target: "EXTERNAL", EntityID: "projection", SubElement: "START"}}
	if err := resolveSketchPatternReferences(&model); err != nil {
		t.Fatal(err)
	}
	if p.Direction != [3]float64{4, 0, 0} || p.Origin != [3]float64{3, 4, 0} {
		t.Fatal("projection not resolved")
	}
	sketch.ExternalGeometry[0].Status = "BROKEN"
	if resolveSketchPatternReferences(&model) == nil {
		t.Fatal("used broken projection")
	}
	sketch.ExternalGeometry[0].Status = "CONNECTED"
	p.CenterReference.SketchID = "foreign"
	if resolveSketchPatternReferences(&model) == nil {
		t.Fatal("accepted foreign sketch")
	}
}

func TestPatternTreeDeletionPassesPublicCommandAdapter(t *testing.T) {
	for _, command := range []string{"DELETE_NODE", "DELETE_NODES"} {
		t.Run(command, func(t *testing.T) {
			model := patternReferenceFixture()
			ensureFeatureParameters(&model)
			raw, _ := json.Marshal(model)
			target := DeleteNodeTarget{TargetKind: "SKETCH_PATTERN_DEFINITION", TargetID: "pattern", OwnerEntityID: "sketch"}
			request := CommandRequest{Type: command, TargetKind: target.TargetKind, TargetID: target.TargetID, OwnerEntityID: target.OwnerEntityID, Targets: []DeleteNodeTarget{target}}
			kind, payload, err := (&Service{}).adaptLegacyCommand(t.Context(), "doc", "PART", raw, request)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(payload)
			var result json.RawMessage
			if kind == typeDeletePartNodes {
				result, _, err = applyDeletePartNodes(raw, data)
			} else if kind == typeDeletePartNode {
				result, _, err = applyDeletePartNode(raw, data)
			} else {
				t.Fatal("wrong route", kind)
			}
			if err != nil {
				t.Fatal(err)
			}
			var next PartModel
			_ = json.Unmarshal(result, &next)
			if len(next.Features[0].Sketch.Patterns) != 0 || len(next.Features[0].Sketch.Entities) != 3 {
				t.Fatal("deletion must preserve seed geometry")
			}
			request.OwnerEntityID = ""
			request.Targets[0].OwnerEntityID = ""
			if _, _, err = (&Service{}).adaptLegacyCommand(t.Context(), "doc", "PART", raw, request); err == nil {
				t.Fatal("ownerless delete accepted")
			}
		})
	}
}

func TestDeletePatternThroughMemberSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ids       []string
		remaining int
	}{
		{"one", []string{patternMemberID("pattern", 1, "seed")}, 3},
		{"marquee", []string{"seed", patternMemberID("pattern", 0, "seed"), patternMemberID("pattern", 2, "seed"), "direction"}, 2},
		{"seed", []string{"seed"}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := patternReferenceFixture()
			ensureFeatureParameters(&model)
			raw, _ := json.Marshal(model)
			payload, _ := json.Marshal(editSketchPayload{SketchID: "sketch", Operations: []SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: tc.ids}}})
			result, changes, err := applyEditSketch(raw, payload)
			if err != nil {
				t.Fatal(err)
			}
			var next PartModel
			_ = json.Unmarshal(result, &next)
			if len(next.Features[0].Sketch.Patterns) != 0 || len(next.Features[0].Sketch.Entities) != tc.remaining || len(changes.Changes) == 0 {
				t.Fatal("incorrect pattern deletion")
			}
			if next.Features[0].Sketch.Entities[0].ID != "seed" {
				t.Fatal("seed was deleted")
			}
			for _, p := range next.Parameters {
				if p.OwnerFeatureID == "sketch" && strings.HasPrefix(p.PropertySlot, "pattern:pattern:") {
					t.Fatal("orphaned managed pattern parameter")
				}
			}
		})
	}
}

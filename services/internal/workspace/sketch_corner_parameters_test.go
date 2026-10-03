package workspace

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestCornerClickChoosesLocalRoundingNotComplementaryMajorArc(t *testing.T) {
	first := SketchEntity{Kind: "LINE", Start: &SketchPoint2{X: 10, Y: 10}, End: &SketchPoint2{X: 130, Y: 10}}
	second := SketchEntity{Kind: "LINE", Start: &SketchPoint2{X: 130, Y: 10}, End: &SketchPoint2{X: 130, Y: 80}}
	for _, click := range []SketchPoint2{{X: 100, Y: 27.5}, {X: 129, Y: 11}} {
		candidates := cornerFilletCandidates(first, second, first, second, "END", "START", 5, &click)
		if len(candidates) != 1 {
			t.Fatalf("expected one admissible local corner, got %d", len(candidates))
		}
		arc := candidates[0]
		if math.Abs(math.Abs(arc.end-arc.start)-math.Pi/2) > 1e-12 || cornerDistance(arc.center, SketchPoint2{X: 125, Y: 15}) > 1e-12 {
			t.Fatalf("wrong local fillet candidate: %+v", arc)
		}
	}
}

func TestCornerPrimarySourceControlsCandidateAndPersistsExpression(t *testing.T) {
	quantity, _ := modelcore.NewQuantity(5, "mm")
	model := newPartModel()
	model.Features = []Feature{{ID: "sketch", Type: "SKETCH", BodyID: model.ActiveBodyID, Sketch: func() *SketchFeature { s := cornerRectangleFixture(); return &s }()}}
	model.Parameters = append(model.Parameters, modelcore.ParameterDefinition{ParameterID: "user-radius", Key: "cornerRadius", Dimension: quantity.Dimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Literal: &quantity}, EvaluatedValue: &quantity})
	for _, kind := range []string{"FILLET_ENTITIES", "CHAMFER_ENTITIES"} {
		t.Run(kind, func(t *testing.T) {
			source := "cornerRadius / 5"
			op := cornerOperation(kind, "source-op", "bottom", "left", "START", "END")
			op.ParameterSource = &source
			op.Value = workflowValue(999)
			op.ChamferFirst = 999
			resolved, e := resolveSketchCornerOperationValues(model, []SketchOperation{op})
			if e != nil {
				t.Fatal(e)
			}
			if kind == "FILLET_ENTITIES" && *resolved[0].Value != 1 || kind == "CHAMFER_ENTITIES" && resolved[0].ChamferFirst != 1 {
				t.Fatal("client preview replaced parameter source")
			}
			raw, _ := json.Marshal(model)
			var clone PartModel
			_ = json.Unmarshal(raw, &clone)
			before := *clone.Features[0].Sketch
			if e = applySketchOperations(clone.Features[0].Sketch, resolved); e != nil {
				t.Fatal(e)
			}
			ensureFeatureParameters(&clone)
			if e = applySketchDimensionLifecycle(&clone, "sketch", before, resolved); e != nil {
				t.Fatal(e)
			}
			label := "radius"
			if kind == "CHAMFER_ENTITIES" {
				label = "first-length"
			}
			found := false
			for _, p := range clone.Parameters {
				if p.ParameterID == sketchConstraintParameterID("sketch", macroID(op.OperationID, "corner/"+label)) {
					found = true
					if p.Source.Expression == nil || !strings.Contains(p.Source.Expression.SourceText, "cornerRadius") {
						t.Fatal("typed source AST discarded")
					}
				}
			}
			if !found {
				t.Fatal("stable primary parameter missing")
			}
		})
	}
	sourceText := "0.5"
	docUnits := model
	docUnits.Units = "cm"
	op := cornerOperation("FILLET_ENTITIES", "document-unit", "bottom", "left", "START", "END")
	op.ParameterSource = &sourceText
	normalized, e := resolveSketchCornerOperationValues(docUnits, []SketchOperation{op})
	if e != nil || *normalized[0].Value != 5 || *normalized[0].ParameterSource != "0.5 cm" {
		t.Fatalf("document unit literal lost: %+v %v", normalized, e)
	}
	for _, source := range []string{"-1 mm", "1 deg", "missingName"} {
		op := cornerOperation("FILLET_ENTITIES", "bad", "bottom", "left", "START", "END")
		op.ParameterSource = &source
		if _, e := resolveSketchCornerOperationValues(model, []SketchOperation{op}); e == nil {
			t.Fatalf("invalid primary source %q accepted", source)
		}
	}
}

func TestWholeCurveTrimMigratesOnlyExactCircularSupportReferences(t *testing.T) {
	center := SketchPoint2{X: 3, Y: 4}
	a := SketchEntity{ID: "a", Kind: "ARC", Center: &center, Radius: 5, StartAngle: 0, EndAngle: 3.14, Role: "PROFILE"}
	b := a
	b.ID = "b"
	b.StartAngle = 3.14
	b.EndAngle = 6.28
	s := SketchFeature{Entities: []SketchEntity{a, b}, Constraints: []SketchConstraint{{ID: "support", Kind: "SAME_SUPPORT", Internal: true, References: []SketchGeometryRef{workflowRef("a", "WHOLE"), workflowRef("b", "WHOLE")}}, {ID: "radius", Kind: "RADIUS", References: []SketchGeometryRef{workflowRef("a", "WHOLE")}, Value: workflowValue(5), ParameterID: "stable-radius"}, {ID: "center", Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef("a", "CENTER")}, FixedPoint: &center}}}
	entities := map[string]SketchEntity{"a": a, "b": b}
	op := SketchOperation{Type: "REPLACE_CURVE_INTERVALS", OperationID: "trim", EntityIDs: []string{"a"}, CurveSourceDigest: sketchCurveDigest(a)}
	if e := applyComputedSketchIntervals(&s, op, entities, map[string]bool{}); e != nil {
		t.Fatal(e)
	}
	if len(s.Entities) != 1 || s.Entities[0].ID != "b" || len(s.Constraints) != 2 {
		t.Fatalf("wrong atomic trim: %+v", s)
	}
	for _, c := range s.Constraints {
		if c.References[0].EntityID != "b" {
			t.Fatal("shared support reference not migrated")
		}
		if c.ID == "radius" && c.ParameterID != "stable-radius" {
			t.Fatal("dimension identity lost")
		}
	}
	inactive := SketchFeature{Entities: []SketchEntity{a, b}, Constraints: []SketchConstraint{{ID: "inactive-support", Kind: "SAME_SUPPORT", Internal: true, Suppressed: true, References: []SketchGeometryRef{workflowRef("a", "WHOLE"), workflowRef("b", "WHOLE")}}, {ID: "radius", Kind: "RADIUS", References: []SketchGeometryRef{workflowRef("a", "WHOLE")}, Value: workflowValue(5)}}}
	if e := applyComputedSketchIntervals(&inactive, op, entities, map[string]bool{}); e == nil {
		t.Fatal("inactive support silently justified a reference replacement")
	}
	s = SketchFeature{Entities: []SketchEntity{a, b}, Constraints: []SketchConstraint{{ID: "endpoint-design", Kind: "FIXED_POINT", References: []SketchGeometryRef{workflowRef("a", "START")}, FixedPoint: &center}}}
	if e := applyComputedSketchIntervals(&s, op, entities, map[string]bool{}); e == nil {
		t.Fatal("whole trim silently discarded user endpoint design relation")
	}
}

package workspace

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestLinkedOffsetRecomputesStableGeometryAndDetachConnectivity(t *testing.T) {
	s := cornerRectangleFixture()
	o := SketchOffset{ID: "linked", EntityIDs: []string{"bottom", "right", "top", "left"}, Distance: -1, Join: "ROUND"}
	if err := applySketchOperations(&s, []SketchOperation{{Type: "CREATE_OFFSET", Offset: &o}}); err != nil {
		t.Fatal(err)
	}
	first, joins, err := evaluateSketchOffset(s, o)
	if err != nil || len(first) != 8 || len(joins) != 16 {
		t.Fatal(len(first), len(joins), err)
	}
	s.Entities[0].Start.Y -= 2
	s.Entities[0].End.Y -= 2
	// Restore formal adjoining endpoints to the same accepted geometry.
	for i := range s.Entities {
		if s.Entities[i].ID == "left" {
			s.Entities[i].End.Y -= 2
		}
		if s.Entities[i].ID == "right" {
			s.Entities[i].Start.Y -= 2
		}
	}
	second, _, err := evaluateSketchOffset(s, o)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatal("source shape change replaced stable offset identity")
		}
	}
	if err := applySketchOperations(&s, []SketchOperation{{Type: "DETACH_OFFSET", OffsetID: o.ID}}); err != nil {
		t.Fatal(err)
	}
	if len(s.Offsets) != 0 || len(s.Entities) != 12 {
		t.Fatal("detachment did not materialize generated curves")
	}
	for _, e := range s.Entities[4:] {
		if e.SourceEntityID != "" || e.CreatedByOperationID != "" {
			t.Fatal("detached curve remained derived")
		}
	}
	if err := validateSketch(s); err != nil {
		t.Fatal(err)
	}
}

func TestLinkedOffsetFormulaLifecycleFailureAndIndependentRegions(t *testing.T) {
	radius := 10.0
	s := SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}, Entities: []SketchEntity{{ID: "seed", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: radius}}, Constraints: []SketchConstraint{}}
	q, _ := modelcore.NewQuantity(2, "mm")
	model := PartModel{Units: "mm", Features: []Feature{{ID: "sketch", Type: "SKETCH", Sketch: &s}}, Parameters: []modelcore.ParameterDefinition{{ParameterID: "wall", Key: "wall", Role: "INPUT", ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Literal: &q}}}}
	normalizePartModel(&model)
	if err := validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	definition := SketchOffset{ID: "inner", EntityIDs: []string{"seed"}, Distance: 1, Join: "MITER"}
	source := "wall"
	data, _ := json.Marshal(model)
	payload, _ := json.Marshal(editSketchPayload{SketchID: "sketch", Operations: []SketchOperation{{Type: "CREATE_OFFSET", Offset: &definition, ParameterSource: &source}}})
	next, _, err := applyEditSketch(data, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(next, &model); err != nil {
		t.Fatal(err)
	}
	if model.Features[0].Sketch.Offsets[0].Distance != 2 {
		t.Fatal("offset source was not authoritative")
	}
	entities, err := evaluatedSketchEntities(*model.Features[0].Sketch)
	if err != nil || len(entities) != 2 || entities[1].Radius != 8 {
		t.Fatal(entities, err)
	}
	identity := entities[1].ID
	q, _ = modelcore.NewQuantity(3, "mm")
	model.Parameters[0].Source = modelcore.ValueSource{Literal: &q}
	if err = validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	entities, _ = evaluatedSketchEntities(*model.Features[0].Sketch)
	if entities[1].ID != identity || entities[1].Radius != 7 {
		t.Fatal("formula update lost identity or radius")
	}
	model.Features[0].Sketch.Entities[0].Radius = 1
	if len(sketchOffsetIssues(*model.Features[0].Sketch)) != 1 {
		t.Fatal("degenerate offset missing diagnostic")
	}
	if _, err = buildProfileLoops(model.Features[0], true); err == nil {
		t.Fatal("whole sketch ignored failed offset")
	}
	loops, err := buildAvailableProfileLoops(model.Features[0], true)
	if err != nil || len(loops) != 1 {
		t.Fatal("failed offset removed independent valid source region", err)
	}
	analysis := analyzeSketchProfile(model.Features[0])
	if analysis.Status != "INVALID" || analysis.Issues[0].Code != "OFFSET_FAILED" {
		t.Fatal(analysis)
	}
	data, _ = json.Marshal(model)
	payload, _ = json.Marshal(editSketchPayload{SketchID: "sketch", Operations: []SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"seed"}}}})
	if _, _, err = applyEditSketch(data, payload); err == nil || !strings.Contains(err.Error(), "linked offset") {
		t.Fatal("source deletion did not protect association", err)
	}
}

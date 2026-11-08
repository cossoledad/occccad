package workspace

import (
	"encoding/json"
	"math"
	"testing"
)

func TestExtrudeOffsetFormulaLifecycleAndCompensatedHistory(t *testing.T) {
	m := newPartModel()
	body := m.ActiveBodyID
	m.Features = []Feature{{ID: "s", Type: "SKETCH", BodyID: body, Plane: "XY", Sketch: &SketchFeature{SchemaVersion: 2, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}}}, {ID: "pad", Type: "LINEAR_EXTRUDE", BodyID: body, Profile: "s", Operation: "ADD", Length: 10, StartOffset: -2}}
	normalizePartModel(&m)
	initial, _ := json.Marshal(m)
	before := m.Features[1]
	after := before
	after.Extent = "TO_PLANE"
	after.TerminationPlaneID = "datum-xy"
	after.EndOffset = 3
	sources, err := featureParameterExpressions(m, after, map[string]string{"startOffset": "-1 cm", "endOffset": "2 mm"})
	if err != nil {
		t.Fatal(err)
	}
	raw, changes, err := editSolidDefinition(m, &m.Features[1], after, sources)
	if err != nil {
		t.Fatal(err)
	}
	var edited PartModel
	_ = json.Unmarshal(raw, &edited)
	f := edited.Features[1]
	if f.StartOffset != -10 || f.EndOffset != 2 {
		t.Fatal("signed unit-aware offsets not resolved", f)
	}
	for _, p := range edited.Parameters {
		if p.OwnerFeatureID == f.ID && p.PropertySlot == "length" {
			t.Fatal("dormant driving length exposed")
		}
	}
	values, err := modelValues("PART", raw, changes)
	if err != nil {
		t.Fatal(err)
	}
	compensation, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", raw, compensation)
	if err != nil {
		t.Fatal(err)
	}
	var undo PartModel
	_ = json.Unmarshal(restored, &undo)
	if undo.Features[1].StartOffset != -2 || undo.Features[1].EndOffset != 0 || undo.Features[1].Extent != "" {
		t.Fatal("offset undo failed", string(initial), undo.Features[1])
	}
	if len(undo.Parameters) != 2 {
		t.Fatal("finite parameter lifecycle not restored", undo.Parameters)
	}
}

func TestExtrudeTerminationRejectsInvalidOffsetsAndTargets(t *testing.T) {
	base := Feature{ID: "base", Type: "IMPORT_BODY", BodyID: "body"}
	earlier := map[string]Feature{"base": base}
	valid := Feature{ID: "pad", Type: "LINEAR_EXTRUDE", BodyID: "body", Operation: "REMOVE", Extent: "TO_NEXT", StartOffset: -2, EndOffset: 3}
	if err := validateExtrudeTermination(valid, earlier); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Feature){func(f *Feature) { f.StartOffset = math.Inf(1) }, func(f *Feature) { f.Operation = "NEW_BODY" }, func(f *Feature) { f.Extent = "FINITE" }, func(f *Feature) { f.Extent = "THROUGH_ALL" }, func(f *Feature) { f.Extent = "TO_SURFACE" }} {
		bad := valid
		mutate(&bad)
		if validateExtrudeTermination(bad, earlier) == nil {
			t.Fatal("invalid termination accepted", bad)
		}
	}
	if validateExtrudeTermination(valid, map[string]Feature{}) == nil {
		t.Fatal("next accepted without target")
	}
}

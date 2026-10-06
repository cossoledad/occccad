package workspace

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssociativeDatumTransformAndParameterIdentity(t *testing.T) {
	m := newPartModel()
	// Rotating an offset plane about an offset axis proves this is a point rotation,
	// not merely a normal rotation, and translation is applied afterwards.
	m.DatumPlanes = append(m.DatumPlanes, DatumPlane{ID: "source", Origin: [3]float64{0, 1, 0}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}})
	m.DatumAxes = append(m.DatumAxes, DatumAxis{ID: "pivot", Origin: [3]float64{0, 0, 1}, Direction: [3]float64{1, 0, 0}})
	def := &DatumTransform{Source: DatumReference{Kind: "PLANE", EntityID: "source"}, RotationAxis: &DatumReference{Kind: "AXIS", EntityID: "pivot"}, TranslationDirection: &DatumReference{Kind: "AXIS_SYSTEM", EntityID: m.AxisSystems[0].ID, Axis: "Z"}, Angle: 90, Distance: 5}
	m.DatumPlanes = append(m.DatumPlanes, DatumPlane{ID: "result", Definition: def})
	ensureFeatureParameters(&m)
	if err := validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	s := &Service{}
	if err := s.resolveDatumDefinitions(context.Background(), "doc", "request", &m, 0); err != nil {
		t.Fatal(err)
	}
	p := m.DatumPlanes[len(m.DatumPlanes)-1]
	for i, want := range [3]float64{0, 1, 7} {
		if math.Abs(p.Origin[i]-want) > 1e-10 {
			t.Fatal("rotation/translation", p)
		}
	}
	if math.Abs(p.Normal[1]+1) > 1e-10 || math.Abs(p.Normal[2]) > 1e-10 {
		t.Fatal(p.Normal)
	}
	for _, parameter := range m.Parameters {
		if parameter.OwnerFeatureID == "result" && parameter.Lifecycle != "DATUM_REQUIRED" {
			t.Fatal(parameter)
		}
	}
	// Rename is presentation: an unchanged expression keeps its bound stable ID.
	q, _ := modelcore.NewQuantity(3, "mm")
	m.Parameters = append(m.Parameters, modelcore.ParameterDefinition{ParameterID: "user-distance", Key: "height", Dimension: modelcore.LengthDimension, Source: modelcore.ValueSource{Literal: &q}})
	sources, err := datumParameterSources(m, def, "", "height + 5 mm", "result")
	if err != nil {
		t.Fatal(err)
	}
	applyDatumParameterSources(&m, "result", sources)
	for i := range m.Parameters {
		if m.Parameters[i].ParameterID == "user-distance" {
			m.Parameters[i].Key = "support_height"
		}
	}
	retained, err := datumParameterSources(m, def, "", "height + 5 mm", "result")
	if err != nil {
		t.Fatal(err)
	}
	if retained["datum:distance"].Expression == nil {
		t.Fatal("lost bound expression")
	}
	if _, err = datumParameterSources(m, def, "support_height", "", "result"); err == nil {
		t.Fatal("accepted length as angle")
	}
}

func TestDatumReferencesRejectDownstreamAndCycles(t *testing.T) {
	for _, cyclic := range []bool{false, true} {
		m := newPartModel()
		def := &DatumTransform{Source: DatumReference{Kind: "PLANE", EntityID: "datum-xy"}, RotationAxis: &DatumReference{Kind: "SKETCH_LINE", FeatureID: "later", EntityID: "line"}}
		m.DatumPlanes = append(m.DatumPlanes, DatumPlane{ID: "derived", Definition: def})
		support := "datum-xy"
		if cyclic {
			support = "derived"
		}
		m.Features = []Feature{{ID: "earlier", Type: "SKETCH", BodyID: m.ActiveBodyID, Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "derived"}}}, {ID: "later", Type: "SKETCH", BodyID: m.ActiveBodyID, Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: support}}}}
		ensureFeatureParameters(&m)
		graph, _, err := buildPartEvaluation(m, "", "", nil, nil)
		if cyclic {
			if err == nil {
				t.Fatal("accepted dependency cycle")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = validateDatumFeatureOrder(m, graph); err == nil || !strings.Contains(err.Error(), "DATUM_DOWNSTREAM_REFERENCE") {
			t.Fatal("accepted future input", err)
		}
	}
	for _, ref := range []DatumReference{{Kind: "AXIS_SYSTEM", EntityID: "a"}, {Kind: "POINT", EntityID: "a", Axis: "X"}, {Kind: "AXIS", EntityID: "a", FeatureID: "b"}, {Kind: "TOPOLOGY"}} {
		if validateDatumReference(ref) == nil {
			t.Fatal("accepted ambiguous reference", ref)
		}
	}
}

func TestDatumEditAndDerivedChangesCompensate(t *testing.T) {
	m := newPartModel()
	m.DatumAxes = append(m.DatumAxes, DatumAxis{ID: "axis", Name: "Axis", Direction: [3]float64{0, 0, 1}})
	old, _ := json.Marshal(m)
	p := editDatumPayload{Axis: &DatumAxis{ID: "axis", Name: "Tilt axis", Direction: [3]float64{0, 1, 0}, Definition: &DatumTransform{Source: DatumReference{Kind: "AXIS_SYSTEM", EntityID: m.AxisSystems[0].ID, Axis: "Y"}, Distance: 10}}}
	payload, _ := json.Marshal(p)
	next, changes, err := applyEditDatum(old, payload)
	if err != nil {
		t.Fatal(err)
	}
	values, err := modelValues("PART", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", next, undo)
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(restored, &result)
	if result.DatumAxes[0].Name != "Axis" || result.DatumAxes[0].Definition != nil || len(result.Parameters) != 0 {
		t.Fatal("datum compensation lost original definition", result)
	}
	after := m
	after.DatumAxes = append([]DatumAxis(nil), m.DatumAxes...)
	after.DatumAxes[0].Origin = [3]float64{0, 0, 20}
	derived := appendEvaluatedDatumChanges(modelcore.ChangeSet{}, m, after)
	if len(derived.Changes) != 1 || derived.Changes[0].Target.SlotID != "datum.axis" {
		t.Fatal(derived)
	}
}

func TestDatumParameterTreeGroupsKeepStableSubjects(t *testing.T) {
	m := newPartModel()
	m.DatumPlanes = append(m.DatumPlanes, DatumPlane{ID: "slope", Name: "Slope", Definition: &DatumTransform{Source: DatumReference{Kind: "PLANE", EntityID: "datum-xy"}}})
	ensureFeatureParameters(&m)
	nodes := partStructureChildren(m, "root", "doc", "rev", true)
	found := false
	for _, node := range nodes {
		if node.Kind != "PARAMETER_SET" {
			continue
		}
		if len(node.Children) != 1 || node.Children[0].Kind != "PARAMETER_GROUP" || node.Children[0].Name != "Slope" {
			t.Fatal(node)
		}
		for _, p := range node.Children[0].Children {
			if p.Kind != "PARAMETER" || !strings.HasPrefix(p.EntityID, "parameter:slope:") || p.ID != "root/parameters/parameter:"+p.EntityID {
				t.Fatal("grouping changed parameter identity", p)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("datum parameters missing from grouped tree")
	}
}

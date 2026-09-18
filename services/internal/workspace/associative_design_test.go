package workspace

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func dimensionalSketchFeature(id string) Feature {
	length, radius, diameter, angle, distance := 20.0, 5.0, 10.0, 90.0, 15.0
	return Feature{ID: id, Type: "SKETCH", Name: "Dimensions", Plane: "XY", Sketch: &SketchFeature{
		SchemaVersion: SketchSchemaVersion,
		Support:       SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"},
		Entities: []SketchEntity{
			{ID: "point-a", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 0, Y: 0}},
			{ID: "point-b", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 15, Y: 0}},
			{ID: "line-a", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 20, Y: 0}},
			{ID: "line-b", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 0, Y: 20}},
			{ID: "circle-a", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 30, Y: 0}, Radius: 5},
			{ID: "circle-b", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 50, Y: 0}, Radius: 5},
		},
		Constraints: []SketchConstraint{
			{ID: "distance", Kind: "DISTANCE", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "point-a", SubElement: "POINT"}, {Target: "ENTITY", EntityID: "point-b", SubElement: "POINT"}}, Value: &distance, Unit: "mm"},
			{ID: "length", Kind: "LENGTH", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "WHOLE"}}, Value: &length, Unit: "mm"},
			{ID: "radius", Kind: "RADIUS", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "circle-a", SubElement: "WHOLE"}}, Value: &radius, Unit: "mm"},
			{ID: "diameter", Kind: "DIAMETER", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "circle-b", SubElement: "WHOLE"}}, Value: &diameter, Unit: "mm"},
			{ID: "angle", Kind: "ANGLE", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "line-a", SubElement: "DIRECTION"}, {Target: "ENTITY", EntityID: "line-b", SubElement: "DIRECTION"}}, Value: &angle, Unit: "deg"},
		},
	}}
}

func TestSketchDimensionsReceiveStableTypedParameterBindings(t *testing.T) {
	model := newPartModel()
	feature := dimensionalSketchFeature("sketch-parameters")
	model.Features = append(model.Features, feature)
	normalizePartModel(&model)
	if err := validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	if len(model.Parameters) != 5 {
		t.Fatalf("dimension parameter count = %d, want 5", len(model.Parameters))
	}
	byID := map[string]modelcore.ParameterDefinition{}
	for _, parameter := range model.Parameters {
		byID[parameter.ParameterID] = parameter
	}
	for _, constraint := range model.Features[0].Sketch.Constraints {
		expected := sketchConstraintParameterID("sketch-parameters", constraint.ID)
		if constraint.ParameterID != expected || byID[expected].EvaluatedValue == nil {
			t.Fatalf("constraint %s binding = %q, parameters=%#v", constraint.ID, constraint.ParameterID, byID)
		}
		if constraint.Kind == "ANGLE" {
			if byID[expected].Dimension != modelcore.AngleDimension || math.Abs(byID[expected].EvaluatedValue.SIValue-math.Pi/2) > 1e-12 {
				t.Fatalf("angle parameter was not canonicalized to radians: %#v", byID[expected])
			}
		} else if byID[expected].Dimension != modelcore.LengthDimension {
			t.Fatalf("%s dimension = %#v", constraint.Kind, byID[expected].Dimension)
		}
	}

	before, _ := json.Marshal(model)
	updated := 40.0
	payload, _ := json.Marshal(editSketchPayload{SketchID: "sketch-parameters", Operations: []SketchOperation{{Type: "UPDATE_CONSTRAINT_VALUE", ConstraintID: "length", Value: &updated}}})
	after, changes, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{CommandID: "dimension-40", TypeURI: typeEditSketch, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 2 {
		t.Fatalf("dimension edit must write sketch.model and parameter.source: %#v", changes)
	}
	if changes.Changes[1].Target != (modelcore.PropertyAddress{EntityID: sketchConstraintParameterID("sketch-parameters", "length"), SlotID: sketchLengthDimensionSlot.SlotID}) {
		t.Fatalf("dimension PropertySlot = %#v", changes.Changes[1].Target)
	}
	var edited PartModel
	_ = json.Unmarshal(after, &edited)
	parameterID := sketchConstraintParameterID("sketch-parameters", "length")
	for _, parameter := range edited.Parameters {
		if parameter.ParameterID == parameterID && (parameter.Source.Literal == nil || math.Abs(parameter.Source.Literal.SIValue-0.04) > 1e-12) {
			t.Fatalf("dimension literal = %#v, want 0.04 m", parameter.Source)
		}
	}
}

func TestPartExpressionsBindIDsSurviveRenameAndDriveDirtyClosure(t *testing.T) {
	model := newPartModel()
	first := testRectangleSketch("sketch-base", "XY")
	second := testRectangleSketch("sketch-driven", "XY")
	baseValue, drivenValue := 20.0, 10.0
	first.Sketch.Constraints = append(first.Sketch.Constraints, SketchConstraint{ID: "base-width", Kind: "LENGTH", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: first.Sketch.Entities[0].ID, SubElement: "WHOLE"}}, Value: &baseValue, Unit: "mm"})
	second.Sketch.Constraints = append(second.Sketch.Constraints, SketchConstraint{ID: "driven-width", Kind: "LENGTH", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: second.Sketch.Entities[0].ID, SubElement: "WHOLE"}}, Value: &drivenValue, Unit: "mm"})
	model.Features = append(model.Features, first, second, Feature{ID: "pad-driven", Type: "PAD", Profile: second.ID, Length: 30, Operation: "NEW_BODY"})
	normalizePartModel(&model)
	baseID := sketchConstraintParameterID(first.ID, "base-width")
	drivenID := sketchConstraintParameterID(second.ID, "driven-width")
	baseKey := ""
	for _, parameter := range model.Parameters {
		if parameter.ParameterID == baseID {
			baseKey = parameter.Key
		}
	}
	expression, err := modelcore.CompileExpression(baseKey+" / 2", map[string]modelcore.ParameterBinding{baseKey: {ParameterID: baseID, Dimension: modelcore.LengthDimension}}, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	for index := range model.Parameters {
		if model.Parameters[index].ParameterID == drivenID {
			model.Parameters[index].Source = modelcore.ValueSource{Expression: &expression}
		}
	}
	if err = validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	if value := *model.Features[1].Sketch.Constraints[len(model.Features[1].Sketch.Constraints)-1].Value; math.Abs(value-10) > 1e-9 {
		t.Fatalf("driven dimension = %g mm, want 10", value)
	}
	modelJSON, _ := json.Marshal(model)
	renameJSON, _ := json.Marshal(renameParameterPayload{ParameterID: baseID, Key: "base_width"})
	renamedJSON, _, err := applyRenameParameter(modelJSON, renameJSON)
	if err != nil {
		t.Fatal(err)
	}
	var renamed PartModel
	_ = json.Unmarshal(renamedJSON, &renamed)
	if err = validateAndResolvePartParameters(&renamed); err != nil {
		t.Fatalf("stable-ID AST broke after rename: %v", err)
	}
	for _, parameter := range renamed.Parameters {
		if parameter.ParameterID == drivenID && (parameter.Source.Expression == nil || !strings.Contains(parameter.Source.Expression.SourceText, "base_width")) {
			t.Fatalf("expression was not rendered with the renamed alias: %#v", parameter.Source)
		}
	}
	graph, _, err := buildPartEvaluation(renamed, "revision-p8b", canonicalModelHash(renamedJSON), []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(baseID)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := graph.DirtyClosure([]modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(baseID)})
	for _, expected := range []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(baseID), "parameter:" + modelcore.DependencyKey(drivenID), "feature:sketch-driven", "feature:pad-driven"} {
		if !containsDependency(dirty, expected) {
			t.Fatalf("dirty closure %v misses %s", dirty, expected)
		}
	}

	cycle, _ := modelcore.CompileExpression("driven", map[string]modelcore.ParameterBinding{"driven": {ParameterID: drivenID, Dimension: modelcore.LengthDimension}}, modelcore.LengthDimension)
	for index := range renamed.Parameters {
		if renamed.Parameters[index].ParameterID == baseID {
			renamed.Parameters[index].Source = modelcore.ValueSource{Expression: &cycle}
		}
	}
	if err = validateAndResolvePartParameters(&renamed); !errors.Is(err, modelcore.ErrDependencyCycle) {
		t.Fatalf("cycle error = %v", err)
	}
	missing := expression
	missing.Reads = []modelcore.DependencyKey{"parameter:deleted-parameter"}
	missing.CheckedAST.ParameterID = "deleted-parameter"
	for index := range renamed.Parameters {
		if renamed.Parameters[index].ParameterID == baseID {
			renamed.Parameters[index].Source = modelcore.ValueSource{Expression: &missing}
		}
	}
	if err = validateAndResolvePartParameters(&renamed); !errors.Is(err, modelcore.ErrParameterMissing) {
		t.Fatalf("deleted parameter error = %v", err)
	}
}

func TestDeletingDimensionPreservesParameterSourceForUndoRedo(t *testing.T) {
	model := newPartModel()
	sketch := dimensionalSketchFeature("sketch-delete-dimension")
	sketch.Sketch.Constraints = sketch.Sketch.Constraints[:1]
	model.Features = append(model.Features, sketch)
	normalizePartModel(&model)
	parameterID := sketchConstraintParameterID(sketch.ID, "distance")
	expression, err := modelcore.CompileExpression("15 mm", nil, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	for index := range model.Parameters {
		if model.Parameters[index].ParameterID == parameterID {
			model.Parameters[index].Source = modelcore.ValueSource{Expression: &expression}
		}
	}
	if err = validateAndResolvePartParameters(&model); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(model)
	payload, _ := json.Marshal(deleteNodePayload{TargetKind: "SKETCH_CONSTRAINT", TargetID: "distance", OwnerEntityID: sketch.ID})
	after, changes, err := applyDeletePartNode(before, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Changes) != 2 || changes.Changes[1].Target != (modelcore.PropertyAddress{EntityID: parameterID, SlotID: "parameter.entity"}) {
		t.Fatalf("dimension deletion did not preserve parameter tombstone: %#v", changes)
	}
	current, _ := modelValues("PART", after, changes)
	desired, err := changes.Compensate(current)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, err := applyModelValues("PART", after, desired)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	_ = json.Unmarshal(restoredJSON, &restored)
	for _, parameter := range restored.Parameters {
		if parameter.ParameterID == parameterID && (parameter.Source.Expression == nil || parameter.Source.Expression.SourceText != "15 mm") {
			t.Fatalf("undo lost the expression source: %#v", parameter.Source)
		}
	}
}

func TestPlanarFaceSupportUsesPersistentSelectionAndTopologyDependency(t *testing.T) {
	failure := &sketchSupportFailure{diagnosticCode: "SUPPORT_TYPE_MISMATCH", diagnostic: "resolved support is not planar"}
	if failure.Code() != "FAILED_SUPPORT" || failure.Phase() != "SKETCH_SUPPORT" || !strings.Contains(failure.Error(), "SUPPORT_TYPE_MISMATCH") {
		t.Fatalf("support failure contract = code %s phase %s error %q", failure.Code(), failure.Phase(), failure.Error())
	}
	model := newPartModel()
	base := testRectangleSketch("sketch-base", "XY")
	pad := Feature{ID: "pad-base", Type: "PAD", Profile: base.ID, Length: 20, Operation: "NEW_BODY"}
	cutSketch := testRectangleSketch("sketch-cut", "XY")
	cut := Feature{ID: "cut-later", Type: "PAD", Profile: cutSketch.ID, Length: 5, Operation: "REMOVE"}
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: "part-1", SourceBodyID: "body-main",
		Anchor: modelcore.SemanticTopologyRef{FeatureID: pad.ID, OutputSlot: "END_CAP/region"}, ExpectedType: modelcore.PersistentTopologyFace,
		Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: modelcore.TopologySelectionEvidence{
			GeometryType: "PLANE", Origin: [3]float64{0, 0, 0.02}, Direction: [3]float64{0, 0, 1}}}
	xDirection, ok := stableSupportX(selection.CreationEvidence.Direction, [3]float64{})
	if !ok {
		t.Fatal("failed to build deterministic support X direction")
	}
	faceSketch := Feature{ID: "sketch-face", Type: "SKETCH", Plane: "CUSTOM", Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion,
		Support: SketchSupport{Type: "PLANAR_FACE", Plane: "CUSTOM", PersistentSelection: &selection, SourceVersionID: "revision-source",
			Origin: selection.CreationEvidence.Origin, XDirection: xDirection, Normal: selection.CreationEvidence.Direction,
			OrientationRule: sketchSupportOrientationRule, Status: "CONNECTED"}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}}}
	model.Features = append(model.Features, base, pad, cutSketch, cut, faceSketch)
	normalizePartModel(&model)
	if err := validatePartStructure(model); err != nil {
		t.Fatal(err)
	}
	modelJSON, _ := json.Marshal(model)
	graph, _, err := buildPartEvaluation(model, "revision-face", canonicalModelHash(modelJSON), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found, bodyChainFound := false, false
	for _, edge := range graph.Edges {
		if edge.Source == "feature:cut-later" && edge.Target == "feature:sketch-face" && edge.Kind == modelcore.ReadTopology {
			found = true
		}
		if edge.Source == "feature:pad-base" && edge.Target == "feature:cut-later" && edge.Kind == modelcore.ReadGeometry {
			bodyChainFound = true
		}
	}
	if !found {
		t.Fatalf("PLANAR_FACE must depend on the complete upstream body tip: %#v", graph.Edges)
	}
	if !bodyChainFound {
		t.Fatalf("sequential body dependency edge missing: %#v", graph.Edges)
	}
	_, repairedX, repairedNormal, err := validatedSupportFrame([3]float64{}, [3]float64{0, 0, 1}, [3]float64{0, 0, 1})
	if err != nil || math.Abs(dot3(repairedX, repairedNormal)) > 1e-12 {
		t.Fatalf("support orientation rule did not deterministically repair X: x=%v normal=%v err=%v", repairedX, repairedNormal, err)
	}
}

func TestExternalGeometryRemainsSeparateSupportsConstraintsAndDetachesWithStableID(t *testing.T) {
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion,
		SourceDocumentID: "part-1", SourceBodyID: "body-main",
		Anchor:       modelcore.SemanticTopologyRef{FeatureID: "pad-base", OutputSlot: "SIDE/profile-edge"},
		ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant},
		CreationEvidence: modelcore.TopologySelectionEvidence{GeometryType: "LINE", Origin: [3]float64{0, 0, 0}, Direction: [3]float64{1, 0, 0}}}
	external := SketchExternalGeometry{ID: "external-edge", ProjectionKind: "ORTHOGONAL", PersistentSelection: selection,
		SourceVersionID: "revision-source", Status: "CONNECTED", ResolvedSourceDigest: "source-digest",
		Snapshot: &SketchExternalGeometrySnapshot{Kind: "LINE", Start: &SketchPoint2{X: 0, Y: 0}, End: &SketchPoint2{X: 20, Y: 0}}}
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion,
		Support:          SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"},
		Entities:         []SketchEntity{{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 2, Y: 0}}},
		ExternalGeometry: []SketchExternalGeometry{external}, Constraints: []SketchConstraint{{ID: "point-on-external", Kind: "POINT_ON_OBJECT",
			References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "point", SubElement: "POINT"},
				{Target: "EXTERNAL", EntityID: external.ID, SubElement: "WHOLE"}}}}}
	if err := validateSketch(sketch); err != nil {
		t.Fatalf("external reference contract rejected: %v", err)
	}
	reconnectSketch := sketch
	reconnectSketch.ExternalGeometry = append([]SketchExternalGeometry(nil), sketch.ExternalGeometry...)
	replacement := external
	replacement.ID, replacement.Status, replacement.Snapshot = "replacement-must-not-win", "PENDING", nil
	if err := applySketchOperations(&reconnectSketch, []SketchOperation{{Type: "RECONNECT_EXTERNAL_GEOMETRY", ExternalID: external.ID,
		ExternalGeometry: &replacement}}); err != nil {
		t.Fatal(err)
	}
	if reconnectSketch.ExternalGeometry[0].ID != external.ID || reconnectSketch.ExternalGeometry[0].Snapshot == nil {
		t.Fatalf("reconnect must preserve ExternalId and validation kind until authoritative projection: %#v", reconnectSketch.ExternalGeometry[0])
	}
	if err := applySketchOperations(&sketch, []SketchOperation{{Type: "DETACH_EXTERNAL_GEOMETRY", ExternalID: external.ID}}); err != nil {
		t.Fatal(err)
	}
	if len(sketch.ExternalGeometry) != 0 || len(sketch.Entities) != 2 || sketch.Entities[1].ID != external.ID || sketch.Entities[1].Role != "CONSTRUCTION" {
		t.Fatalf("detach did not freeze a construction entity with stable identity: %#v", sketch)
	}
	if reference := sketch.Constraints[0].References[1]; reference.Target != "ENTITY" || reference.EntityID != external.ID {
		t.Fatalf("detach rewrote downstream reference incorrectly: %#v", reference)
	}
}

func TestExternalGeometryAddsTopologyDependencyAndNeverUsesBrokenSnapshot(t *testing.T) {
	model := newPartModel()
	base := testRectangleSketch("sketch-base", "XY")
	pad := Feature{ID: "pad-base", Type: "PAD", Profile: base.ID, Length: 20, Operation: "NEW_BODY"}
	selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion,
		SourceDocumentID: "part-1", SourceBodyID: "body-main",
		Anchor:       modelcore.SemanticTopologyRef{FeatureID: pad.ID, OutputSlot: "SIDE/profile-edge"},
		ExpectedType: modelcore.PersistentTopologyEdge, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}}
	external := SketchExternalGeometry{ID: "external-broken", ProjectionKind: "ORTHOGONAL", PersistentSelection: selection,
		SourceVersionID: "revision-source", Status: "UNRESOLVED_EXTERNAL", DiagnosticCode: "EXTERNAL_SOURCE_AMBIGUOUS"}
	second := Feature{ID: "sketch-second", Type: "SKETCH", Plane: "XY", Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion,
		Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}, ExternalGeometry: []SketchExternalGeometry{external}}}
	model.Features = append(model.Features, base, pad, second)
	normalizePartModel(&model)
	modelJSON, _ := json.Marshal(model)
	modelHash := canonicalModelHash(modelJSON)
	graph, baseline, err := buildPartEvaluation(model, "revision-external", modelHash, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, edge := range graph.Edges {
		if edge.Source == "feature:pad-base" && edge.Target == "feature:sketch-second" && edge.Kind == modelcore.ReadTopology {
			found = true
		}
	}
	if !found {
		t.Fatalf("external projection topology dependency missing: %#v", graph.Edges)
	}
	_, incremental, err := buildPartEvaluation(model, "revision-incremental", modelHash,
		[]modelcore.DependencyKey{"feature:pad-base"}, &baseline)
	if err != nil {
		t.Fatal(err)
	}
	_, cold, err := buildPartEvaluation(model, "revision-cold", modelHash, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.DependencySnapshotDigest != cold.DependencySnapshotDigest || !reflect.DeepEqual(incremental.NodeResults, cold.NodeResults) {
		t.Fatalf("incremental and cold evaluation diverged: incremental=%#v cold=%#v", incremental, cold)
	}
	broken, ok := firstUnresolvedExternal(model)
	if !ok || broken.ID != external.ID || broken.Snapshot != nil {
		t.Fatalf("broken external geometry must be explicit and carry no stale snapshot: %#v", broken)
	}
	geometryKey, revisionState, evaluationStatus, unresolved := unresolvedExternalRevisionOutcome(model)
	if !unresolved || geometryKey != "" || revisionState != "FAILED" || evaluationStatus != "FAILED" {
		t.Fatalf("failed Revision must not borrow its dependency artifact: key=%q revision=%s evaluation=%s",
			geometryKey, revisionState, evaluationStatus)
	}
	manifest := visualizationManifest(model)
	for _, primitive := range manifest.Primitives {
		if primitive.ID == external.ID {
			t.Fatal("broken external geometry leaked a stale visualization primitive")
		}
	}
}

func TestExplicitExternalProjectionFailureRejectsTheCommand(t *testing.T) {
	unresolved := SketchExternalGeometry{ID: "external-arc", Status: "UNRESOLVED_EXTERNAL",
		DiagnosticCode: "EXTERNAL_PROJECTION_TYPE_UNSUPPORTED",
		Diagnostic:     "partial circular edges require arc orientation evidence"}
	model := PartModel{Features: []Feature{{ID: "sketch-face", Type: "SKETCH", Sketch: &SketchFeature{
		ExternalGeometry: []SketchExternalGeometry{unresolved},
	}}}}
	payload, _ := json.Marshal(editSketchPayload{SketchID: "sketch-face", Operations: []SketchOperation{{
		Type: "ADD_EXTERNAL_GEOMETRY", ExternalGeometry: &unresolved,
	}}})
	err := rejectExplicitUnresolvedExternal(modelcore.DomainCommand{TypeURI: typeEditSketch, Payload: payload}, model)
	var failure *externalGeometryProjectionFailure
	if !errors.As(err, &failure) {
		t.Fatalf("explicit unsupported projection must reject the command, got %v", err)
	}
	if failure.Code() != "EXTERNAL_PROJECTION_TYPE_UNSUPPORTED" || failure.Phase() != "EXTERNAL_GEOMETRY_PROJECTION" || failure.Retryable() {
		t.Fatalf("unexpected projection failure contract: code=%s phase=%s retryable=%v",
			failure.Code(), failure.Phase(), failure.Retryable())
	}

	reconnectPayload, _ := json.Marshal(editSketchPayload{SketchID: "sketch-face", Operations: []SketchOperation{{
		Type: "RECONNECT_EXTERNAL_GEOMETRY", ExternalID: unresolved.ID, ExternalGeometry: &unresolved,
	}}})
	if err := rejectExplicitUnresolvedExternal(modelcore.DomainCommand{TypeURI: typeEditSketch, Payload: reconnectPayload}, model); err == nil {
		t.Fatal("an unresolved reconnect must retain the old Head instead of committing a failed replacement")
	}

	ordinaryPayload, _ := json.Marshal(editSketchPayload{SketchID: "sketch-face", Operations: []SketchOperation{{
		Type: "ADD_ENTITY", Entity: &SketchEntity{ID: "point", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{}},
	}}})
	if err := rejectExplicitUnresolvedExternal(modelcore.DomainCommand{TypeURI: typeEditSketch, Payload: ordinaryPayload}, model); err != nil {
		t.Fatalf("an already-broken dependency must remain inspectable/editable: %v", err)
	}
}

package workspace

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestDefaultPartReferenceGeometryAndGLBExtension(t *testing.T) {
	t.Parallel()
	model := newPartModel()
	if len(model.DatumPlanes) != 3 || len(model.AxisSystems) != 1 {
		t.Fatalf("a new Part must own three planes and one axis system: %#v", model)
	}
	glb, err := glbWithReferenceGeometry(nil, referenceGeometry(model))
	if err != nil {
		t.Fatalf("encode reference GLB: %v", err)
	}
	if binary.LittleEndian.Uint32(glb[:4]) != 0x46546c67 {
		t.Fatal("missing GLB magic")
	}
	jsonLength := int(binary.LittleEndian.Uint32(glb[12:16]))
	var document map[string]any
	if err := json.Unmarshal(glb[20:20+jsonLength], &document); err != nil {
		t.Fatalf("decode GLB JSON: %v", err)
	}
	extensions, ok := document["extensions"].(map[string]any)
	if !ok || extensions[referenceGeometryExtension] == nil {
		t.Fatalf("GLB does not contain %s", referenceGeometryExtension)
	}
}

func TestLegacyVerticalSliceUsesTypedHandlersAndStableParameterFacades(t *testing.T) {
	modelJSON, _ := json.Marshal(newPartModel())
	sketch := Feature{ID: "sketch-stable", Type: "RECTANGLE_SKETCH", Name: "Sketch 1", Plane: "XY", Rectangle: &Rectangle{Width: 20, Height: 10}}
	payload, _ := json.Marshal(createFeaturePayload{Feature: sketch})
	next, sketchChanges, err := workspaceCommandRegistry.Apply("PART", modelJSON, modelcore.DomainCommand{CommandID: "command-sketch", TypeURI: typeCreateSketch, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	pad := Feature{ID: "pad-stable", Type: "PAD", Name: "Pad 1", Profile: sketch.ID, Length: 5, Operation: "ADD"}
	payload, _ = json.Marshal(createFeaturePayload{Feature: pad})
	next, _, err = workspaceCommandRegistry.Apply("PART", next, modelcore.DomainCommand{CommandID: "command-pad", TypeURI: typeCreatePad, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var model PartModel
	if err := json.Unmarshal(next, &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Parameters) != 3 {
		t.Fatalf("expected width, height and length facades, got %#v", model.Parameters)
	}
	if model.Parameters[2].ParameterID == model.Parameters[2].Key {
		t.Fatal("persistent parameter identity must not depend on its display key")
	}
	graph, _, err := buildPartEvaluation(model, "revision-1", canonicalModelHash(next), sketchChanges.ImpactSeeds, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := graph.DirtyClosure([]modelcore.DependencyKey{"parameter:parameter:sketch-stable:width"})
	if !containsDependency(dirty, "feature:sketch-stable") || !containsDependency(dirty, "feature:pad-stable") {
		t.Fatalf("width change did not dirty sketch and pad: %v", dirty)
	}
}

func TestParameterExpressionUpdatesFacadeWithoutNameBoundPersistence(t *testing.T) {
	model := newPartModel()
	model.Features = []Feature{{ID: "sketch-1", Type: "RECTANGLE_SKETCH", Rectangle: &Rectangle{Width: 20, Height: 10}}}
	normalizePartModel(&model)
	widthID := "parameter:sketch-1:width"
	heightID := "parameter:sketch-1:height"
	names := map[string]modelcore.ParameterBinding{}
	for _, parameter := range model.Parameters {
		names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
	}
	expression, err := modelcore.CompileExpression("sketch_1_width + 5 mm", names, modelcore.LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	modelJSON, _ := json.Marshal(model)
	payload, _ := json.Marshal(parameterSourcePayload{ParameterID: heightID, Source: modelcore.ValueSource{Expression: &expression}})
	next, _, err := workspaceCommandRegistry.Apply("PART", modelJSON, modelcore.DomainCommand{CommandID: "command-expression", TypeURI: typeSetParameterExpression, SchemaVersion: 1, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var updated PartModel
	_ = json.Unmarshal(next, &updated)
	if updated.Features[0].Rectangle.Height != 25 {
		t.Fatalf("expected 25 mm, got %g", updated.Features[0].Rectangle.Height)
	}
	for _, read := range expression.Reads {
		if read == modelcore.DependencyKey("parameter:"+widthID) {
			return
		}
	}
	t.Fatal("checked AST did not retain stable ParameterId")
}

func containsDependency(values []modelcore.DependencyKey, target modelcore.DependencyKey) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestHistoryChangeSetForRecreatedEntityRemainsUndoable(t *testing.T) {
	address := modelcore.PropertyAddress{EntityID: "feature-1", SlotID: "entity"}
	after, _ := json.Marshal(Feature{ID: "feature-1", Type: "PAD", Length: 10})
	set, err := changesBetweenValues(map[modelcore.PropertyAddress]json.RawMessage{}, map[modelcore.PropertyAddress]json.RawMessage{address: after}, []modelcore.DependencyKey{"feature:feature-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 1 || set.Changes[0].Kind != modelcore.ChangeCreate {
		t.Fatalf("redo did not retain a compensatable create: %#v", set)
	}
}

func TestPartStructureRejectsPadWhoseSketchWasRemoved(t *testing.T) {
	model := PartModel{Units: "mm", Features: []Feature{{ID: "pad-1", Type: "PAD", Profile: "sketch-1", Length: 10}}}
	if err := validatePartStructure(model); err == nil || !strings.Contains(err.Error(), "requires an earlier sketch") {
		t.Fatalf("expected an explicit structural dependency error, got %v", err)
	}
}

func TestSketchPadSupportsTwoUndoAndTwoRedoModelSteps(t *testing.T) {
	initial, _ := json.Marshal(newPartModel())
	sketch := Feature{ID: "sketch-history", Type: "RECTANGLE_SKETCH", Name: "Sketch 1", Plane: "XY", Rectangle: &Rectangle{Width: 20, Height: 10}}
	sketchPayload, _ := json.Marshal(createFeaturePayload{Feature: sketch})
	afterSketch, sketchChanges, err := workspaceCommandRegistry.Apply("PART", initial, modelcore.DomainCommand{CommandID: "create-sketch", TypeURI: typeCreateSketch, SchemaVersion: 1, Payload: sketchPayload})
	if err != nil {
		t.Fatal(err)
	}
	pad := Feature{ID: "pad-history", Type: "PAD", Name: "Pad 1", Profile: sketch.ID, Length: 5, Operation: "ADD"}
	padPayload, _ := json.Marshal(createFeaturePayload{Feature: pad})
	afterPad, padChanges, err := workspaceCommandRegistry.Apply("PART", afterSketch, modelcore.DomainCommand{CommandID: "create-pad", TypeURI: typeCreatePad, SchemaVersion: 1, Payload: padPayload})
	if err != nil {
		t.Fatal(err)
	}
	apply := func(model json.RawMessage, changes modelcore.ChangeSet, undo bool) json.RawMessage {
		t.Helper()
		current, applyErr := modelValues("PART", model, changes)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		var desired map[modelcore.PropertyAddress]json.RawMessage
		if undo {
			desired, applyErr = changes.Compensate(current)
		} else {
			desired, applyErr = changes.Reapply(current)
		}
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		next, applyErr := applyModelValues("PART", model, desired)
		if applyErr != nil {
			t.Fatal(applyErr)
		}
		var part PartModel
		_ = json.Unmarshal(next, &part)
		if _, _, applyErr = buildPartEvaluation(part, "test-revision", canonicalModelHash(next), changes.ImpactSeeds, nil); applyErr != nil {
			t.Fatal(applyErr)
		}
		return next
	}
	undoPad := apply(afterPad, padChanges, true)
	undoSketch := apply(undoPad, sketchChanges, true)
	redoSketch := apply(undoSketch, sketchChanges, false)
	redoPad := apply(redoSketch, padChanges, false)
	var final PartModel
	if err = json.Unmarshal(redoPad, &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Features) != 2 || final.Features[0].ID != sketch.ID || final.Features[1].ID != pad.ID {
		t.Fatalf("two-step redo did not restore Sketch then Pad: %#v", final.Features)
	}
}

func TestPartSketchAndPadCommands(t *testing.T) {
	t.Parallel()
	model := PartModel{Units: "mm", Features: []Feature{}}
	if err := mutatePart(&model, CommandRequest{
		Type: "CREATE_RECTANGLE_SKETCH", Plane: "YZ", Origin: [2]float64{-10, 5},
		Width: 80, Height: 50,
	}); err != nil {
		t.Fatalf("create sketch: %v", err)
	}
	if len(model.Features) != 1 || model.Features[0].Plane != "YZ" || model.Features[0].Rectangle == nil {
		t.Fatalf("unexpected sketch model: %#v", model.Features)
	}
	sketchID := model.Features[0].ID
	if err := mutatePart(&model, CommandRequest{
		Type: "PAD_SKETCH", SketchID: sketchID, Length: 35,
	}); err != nil {
		t.Fatalf("pad sketch: %v", err)
	}
	if len(model.Features) != 2 || model.Features[1].Profile != sketchID || model.Features[1].Length != 35 {
		t.Fatalf("unexpected pad model: %#v", model.Features)
	}
}

func TestPartStructureNestsConsumedSketchUnderPad(t *testing.T) {
	t.Parallel()
	model := PartModel{Units: "mm", Features: []Feature{
		{ID: "sketch-1", Type: "RECTANGLE_SKETCH", Name: "Sketch 1"},
		{ID: "pad-1", Type: "PAD", Name: "Pad 1", Profile: "sketch-1"},
		{ID: "sketch-2", Type: "RECTANGLE_SKETCH", Name: "Sketch 2"},
	}}
	children := partStructureChildren(model, "document:part-1", "part-1", "version-1")
	if len(children) != 2 || children[1].Kind != "BODY" || len(children[1].Children) != 2 {
		t.Fatalf("unexpected part structure: %#v", children)
	}
	pad := children[1].Children[0]
	if pad.Kind != "PAD" || len(pad.Children) != 1 || pad.Children[0].EntityID != "sketch-1" {
		t.Fatalf("pad must own its consumed sketch: %#v", pad)
	}
	if children[1].Children[1].EntityID != "sketch-2" {
		t.Fatalf("unconsumed sketch must remain in PartBody: %#v", children[1].Children)
	}
	if len(children[0].Children) != 4 || children[0].Children[3].Kind != "AXIS_SYSTEM" {
		t.Fatalf("Origin must contain three planes and one axis system: %#v", children[0])
	}
	axisSystem := children[0].Children[3]
	if len(axisSystem.Children) != 3 || axisSystem.Children[0].Axis != "X" || axisSystem.Children[1].Axis != "Y" || axisSystem.Children[2].Axis != "Z" {
		t.Fatalf("axis system must expose independently selectable X/Y/Z axes: %#v", axisSystem)
	}
}

func TestPartCommandValidation(t *testing.T) {
	t.Parallel()
	tests := []CommandRequest{
		{Type: "CREATE_RECTANGLE_SKETCH", Plane: "AB", Width: 10, Height: 10},
		{Type: "CREATE_RECTANGLE_SKETCH", Plane: "XY", Width: -1, Height: 10},
		{Type: "PAD_SKETCH", SketchID: "missing", Length: 10},
	}
	for _, command := range tests {
		model := PartModel{Units: "mm", Features: []Feature{}}
		if err := mutatePart(&model, command); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
			t.Fatalf("command %#v should be rejected, got %v", command, err)
		}
	}
}

func TestProductFollowHeadIsDefaultAndUndoableModelState(t *testing.T) {
	t.Parallel()
	model := ProductModel{Instances: []ProductInstance{{
		ID: "instance-1", ReferencedDocumentID: "document-1", ReferencedVersionID: "version-1",
	}}}
	service := &Service{}
	if err := service.mutateProduct(t.Context(), nil, "product-1", &model, CommandRequest{
		Type: "SET_REFERENCE_MODE", InstanceID: "instance-1", ReferenceMode: "FOLLOW_HEAD",
	}); err != nil {
		t.Fatalf("set follow-head reference: %v", err)
	}
	if model.Instances[0].ReferenceMode != "FOLLOW_HEAD" {
		t.Fatalf("unexpected reference mode: %q", model.Instances[0].ReferenceMode)
	}
}

func TestProductReferenceModeValidation(t *testing.T) {
	t.Parallel()
	model := ProductModel{Instances: []ProductInstance{{ID: "instance-1"}}}
	service := &Service{}
	err := service.mutateProduct(t.Context(), nil, "product-1", &model, CommandRequest{
		Type: "SET_REFERENCE_MODE", InstanceID: "instance-1", ReferenceMode: "floating",
	})
	if err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("invalid reference mode should be rejected, got %v", err)
	}
}

func TestDocumentManagementValidation(t *testing.T) {
	t.Parallel()
	service := &Service{}
	if _, err := service.ListDocuments(t.Context(), DocumentListOptions{Scope: "unknown"}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("invalid document scope should be rejected, got %v", err)
	}
	if _, err := service.ListDocuments(t.Context(), DocumentListOptions{Scope: "active", Type: "DRAWING"}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("invalid document type should be rejected, got %v", err)
	}
	if _, err := service.ListDocuments(t.Context(), DocumentListOptions{FolderID: "not-a-uuid"}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("invalid folder id should be rejected, got %v", err)
	}
	if _, err := service.ListDocuments(t.Context(), DocumentListOptions{Limit: 201}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("invalid page size should be rejected, got %v", err)
	}
	if _, err := service.UpdateDocument(t.Context(), "document-1", UpdateDocumentRequest{Name: ""}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("empty document name should be rejected, got %v", err)
	}
	if _, err := service.CreateDocument(t.Context(), CreateDocumentRequest{
		Name: "Part", Type: "PART", Description: strings.Repeat("x", 501),
	}); err == nil || !strings.Contains(err.Error(), ErrValidation.Error()) {
		t.Fatalf("long description should be rejected, got %v", err)
	}
}

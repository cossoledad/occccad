package workspace

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestPartVisualizationManifestAndGLBExtensionContainSelectableSketchGeometry(t *testing.T) {
	t.Parallel()
	model := newPartModel()
	model.Features = append(model.Features, Feature{ID: "sketch-visible", Type: "SKETCH", Plane: "XZ",
		Sketch: &SketchFeature{SchemaVersion: 1,
			Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xz", Plane: "XZ"},
			Entities: []SketchEntity{
				{ID: "point-visible", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: 2, Y: 3}},
				{ID: "line-visible", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 1, Y: 4}, End: &SketchPoint2{X: 5, Y: 6}},
			}, Solve: SketchSolveState{Status: "UNDER_CONSTRAINED"}}})
	if len(model.DatumPlanes) != 3 || len(model.AxisSystems) != 1 {
		t.Fatalf("a new Part must own three planes and one axis system: %#v", model)
	}
	visualization := visualizationManifest(model)
	if len(visualization.Primitives) != 2 || visualization.Primitives[0].ID != "point-visible" ||
		visualization.Primitives[0].Positions[0] != [3]float64{2, 0, 3} ||
		visualization.Primitives[1].Kind != "POLYLINE" || !visualization.Primitives[1].Selectable {
		t.Fatalf("unexpected visualization manifest: %#v", visualization)
	}
	glb, err := glbWithVisualization(nil, visualization)
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
	if !ok || extensions[visualizationExtension] == nil {
		t.Fatalf("GLB does not contain %s", visualizationExtension)
	}
}

func TestImportedBodyUsesOrdinaryPartTemplateAndCompensatableFeature(t *testing.T) {
	model := newPartModel()
	if len(model.DatumPlanes) != 3 || len(model.AxisSystems) != 1 {
		t.Fatal("imported Parts must start from the ordinary private reference template")
	}
	before, _ := json.Marshal(model)
	typeURI, payload, err := (&Service{}).adaptLegacyCommand(context.Background(), "", "PART", before, CommandRequest{
		Type: "IMPORT_EXCHANGE", GeometryKey: "sha256:imported", FileName: "gear.step", SourceFormat: "STEP",
	})
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON, _ := json.Marshal(payload)
	after, changes, err := workspaceCommandRegistry.Apply("PART", before, modelcore.DomainCommand{
		CommandID: "import-command", TypeURI: typeURI, SchemaVersion: 1, Payload: payloadJSON,
	})
	if err != nil {
		t.Fatal(err)
	}
	var imported PartModel
	if err := json.Unmarshal(after, &imported); err != nil {
		t.Fatal(err)
	}
	if len(imported.Features) != 1 || imported.Features[0].Type != "IMPORT_BODY" ||
		imported.Features[0].SourceFormat != "STEP" || imported.Features[0].GeometryKey != "sha256:imported" {
		t.Fatalf("unexpected imported feature: %#v", imported.Features)
	}
	current, err := modelValues("PART", after, changes)
	if err != nil {
		t.Fatal(err)
	}
	compensated, err := changes.Compensate(current)
	if err != nil {
		t.Fatalf("import feature is not undoable: %v", err)
	}
	undone, err := applyModelValues("PART", after, compensated)
	if err != nil {
		t.Fatal(err)
	}
	var restored PartModel
	if err := json.Unmarshal(undone, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Features) != 0 || len(restored.DatumPlanes) != 3 || len(restored.AxisSystems) != 1 {
		t.Fatalf("undo must remove only the imported body: %#v", restored)
	}
}

func TestLegacyVerticalSliceUsesTypedHandlersAndStableParameterFacades(t *testing.T) {
	modelJSON, _ := json.Marshal(newPartModel())
	sketch := testRectangleSketch("sketch-stable", "XY")
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
	if len(model.Parameters) != 1 {
		t.Fatalf("expected pad length facade, got %#v", model.Parameters)
	}
	if model.Parameters[0].ParameterID == model.Parameters[0].Key {
		t.Fatal("persistent parameter identity must not depend on its display key")
	}
	graph, _, err := buildPartEvaluation(model, "revision-1", canonicalModelHash(next), sketchChanges.ImpactSeeds, nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := graph.DirtyClosure([]modelcore.DependencyKey{"feature:sketch-stable"})
	if !containsDependency(dirty, "feature:sketch-stable") || !containsDependency(dirty, "feature:pad-stable") {
		t.Fatalf("sketch change did not dirty sketch and pad: %v", dirty)
	}
}

func testRectangleSketch(id, plane string) Feature {
	operations, _ := rectangleMacro(id, SketchPoint2{0, 0}, SketchPoint2{20, 10})
	sketch := &SketchFeature{SchemaVersion: 1, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-" + strings.ToLower(plane), Plane: plane}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}}
	_ = applySketchOperations(sketch, operations)
	return Feature{ID: id, Type: "SKETCH", Name: "Sketch 1", Plane: plane, Sketch: sketch}
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

func TestSolvedSketchChangeSetUsesPersistedAfterValue(t *testing.T) {
	original := testRectangleSketch("sketch-solved-history", "XY")
	model := newPartModel()
	model.Features = append(model.Features, original)
	beforeJSON, _ := json.Marshal(model)
	operation := SketchOperation{Type: "ADD_ENTITY", Entity: &SketchEntity{
		ID: "point-solved", Kind: "POINT", Role: "PROFILE", Point: &SketchPoint2{X: 2, Y: 3},
	}}
	payload, _ := json.Marshal(editSketchPayload{SketchID: original.ID, Operations: []SketchOperation{operation}})
	candidateJSON, handlerChanges, err := workspaceCommandRegistry.Apply("PART", beforeJSON, modelcore.DomainCommand{
		CommandID: "edit-solved-sketch", TypeURI: typeEditSketch, SchemaVersion: 1, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Model the authoritative solver changing both geometry and diagnostics
	// after the pure command handler has produced its candidate ChangeSet.
	var solved PartModel
	if err = json.Unmarshal(candidateJSON, &solved); err != nil {
		t.Fatal(err)
	}
	point := solved.Features[0].Sketch.Entities[len(solved.Features[0].Sketch.Entities)-1].Point
	point.X, point.Y = 2.25, 3.5
	solved.Features[0].Sketch.Solve = SketchSolveState{Status: "UNDER_CONSTRAINED", DegreesOfFreedom: 2}
	persistedJSON, _ := json.Marshal(solved)
	changes, err := reconcilePersistedChanges("PART", beforeJSON, persistedJSON, handlerChanges)
	if err != nil {
		t.Fatal(err)
	}

	current, err := modelValues("PART", persistedJSON, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(current)
	if err != nil {
		t.Fatalf("undo rejected the persisted solver result: %v", err)
	}
	undoneJSON, err := applyModelValues("PART", persistedJSON, desired)
	if err != nil {
		t.Fatal(err)
	}
	reappliedCurrent, err := modelValues("PART", undoneJSON, changes)
	if err != nil {
		t.Fatal(err)
	}
	reapplied, err := changes.Reapply(reappliedCurrent)
	if err != nil {
		t.Fatalf("redo rejected the compensated sketch: %v", err)
	}
	redoneJSON, err := applyModelValues("PART", undoneJSON, reapplied)
	if err != nil {
		t.Fatal(err)
	}
	if canonicalModelHash(redoneJSON) != canonicalModelHash(persistedJSON) {
		t.Fatal("redo did not restore the exact solver-normalized sketch model")
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
	sketch := testRectangleSketch("sketch-history", "XY")
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
	if err := mutatePart(&model, CommandRequest{Type: "CREATE_SKETCH", Plane: "YZ"}); err != nil {
		t.Fatalf("create sketch: %v", err)
	}
	if len(model.Features) != 1 || model.Features[0].Plane != "YZ" || model.Features[0].Sketch == nil {
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

func TestRectangleMacroExpandsToStableLinesAndExplicitConstraints(t *testing.T) {
	first := SketchPoint2{X: 20, Y: 10}
	second := SketchPoint2{X: -5, Y: -2}
	operations, err := rectangleMacro("request-1/0", first, second)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := rectangleMacro("request-1/0", first, second)
	if len(operations) != 12 || len(again) != 12 {
		t.Fatalf("rectangle must be one macro containing 4 entities and 8 constraints: %#v", operations)
	}
	for index := range operations {
		if operations[index].Type != again[index].Type {
			t.Fatal("macro operation order is not deterministic")
		}
		if operations[index].Entity != nil && operations[index].Entity.ID != again[index].Entity.ID {
			t.Fatal("macro entity identity is not deterministic")
		}
		if operations[index].Constraint != nil && operations[index].Constraint.ID != again[index].Constraint.ID {
			t.Fatal("macro constraint identity is not deterministic")
		}
	}
	sketch := SketchFeature{SchemaVersion: 1, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}}
	if err := applySketchOperations(&sketch, operations); err != nil {
		t.Fatal(err)
	}
	if len(sketch.Entities) != 4 || len(sketch.Constraints) != 8 {
		t.Fatalf("unexpected rectangle expansion: %#v", sketch)
	}
	for index, constraint := range sketch.Constraints {
		want := "COINCIDENT"
		if index >= 4 {
			want = "PARALLEL"
		}
		if constraint.Kind != want {
			t.Fatalf("constraint %d is %s, want %s", index, constraint.Kind, want)
		}
	}
}

func TestPartStructureNestsConsumedSketchUnderPad(t *testing.T) {
	t.Parallel()
	model := PartModel{Units: "mm", Features: []Feature{
		{ID: "sketch-1", Type: "SKETCH", Name: "Sketch 1"},
		{ID: "pad-1", Type: "PAD", Name: "Pad 1", Profile: "sketch-1"},
		{ID: "sketch-2", Type: "SKETCH", Name: "Sketch 2"},
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
		{Type: "CREATE_SKETCH", Plane: "AB"},
		{Type: "EDIT_SKETCH", SketchID: "missing", Operations: []SketchOperation{{Type: "ADD_ENTITY"}}},
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

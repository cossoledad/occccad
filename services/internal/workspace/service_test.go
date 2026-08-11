package workspace

import (
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
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

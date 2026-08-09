package workspace

import (
	"strings"
	"testing"
)

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

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

package workspace

import (
	"encoding/json"
	"testing"
)

func TestCreationAppearanceFreezesDefaultsWithoutChangingGeometry(t *testing.T) {
	model := newPartModel()
	model.Bodies[0].GeometryKey = "frozen-geometry"
	raw, _ := json.Marshal(model)
	roughness := .7
	style := &SurfaceStyle{Color: "#AABBCC", GraphicProperties: GraphicProperties{Roughness: &roughness}}
	next, err := seedCreationAppearance(raw, "PART", style)
	if err != nil {
		t.Fatal(err)
	}
	style.Color = "#000000"
	roughness = .1
	var saved PartModel
	if err = json.Unmarshal(next, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DefaultAppearance.Color != "#aabbcc" || *saved.DefaultAppearance.Roughness != .7 || saved.Bodies[0].GeometryKey != "frozen-geometry" {
		t.Fatalf("creation defaults were not frozen: %s", next)
	}
	if model.DefaultAppearance != nil {
		t.Fatal("creation mutated another model")
	}
	if _, err = seedCreationAppearance(raw, "PRODUCT", style); err == nil {
		t.Fatal("Part defaults became a Product override")
	}
	invalid := &SurfaceStyle{Color: "#bad"}
	if _, err = creationAppearance(invalid); err == nil {
		t.Fatal("accepted invalid default color")
	}
}

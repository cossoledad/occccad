package workspace

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestIndependentObjectGraphicsInheritanceHistoryAndGeometryIdentity(t *testing.T) {
	m := deletionFixture()
	m.Features[0].Result = &FeatureGeometryResult{ID: "result", Domain: "WIRE_SET", GeometryKey: "frozen-wire"}
	raw, _ := json.Marshal(m)
	color := map[string]json.RawMessage{"color": json.RawMessage(`"#12ab34"`), "edgeWidth": json.RawMessage(`3`), "pointSymbol": json.RawMessage(`"PLUS"`)}
	payload, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "GEOMETRICAL_SET", ID: "curves"}}, Appearance: color})
	next, changes, err := applyPartProperties(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !displayPropertiesOnly(changes) {
		t.Fatal("graphics mutation entered geometry evaluation")
	}
	var result PartModel
	json.Unmarshal(next, &result)
	if result.Features[0].Result.GeometryKey != "frozen-wire" {
		t.Fatal("graphics changed exact identity")
	}
	style, err := (&Service{}).partResultAppearance(context.Background(), result, result.Features[0])
	if err != nil {
		t.Fatal(err)
	}
	if style.Color != "#12ab34" || style.EdgeWidth == nil || *style.EdgeWidth != 3 || style.Sources["color"].Kind != "GEOMETRICAL_SET" {
		t.Fatalf("inheritance missing: %+v", style)
	}
	patch, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "FEATURE", ID: "line"}}, Appearance: map[string]json.RawMessage{"color": json.RawMessage(`"#abcdef"`)}})
	changed, second, err := applyPartProperties(next, patch)
	if err != nil {
		t.Fatal(err)
	}
	result = PartModel{}
	json.Unmarshal(changed, &result)
	style, err = (&Service{}).partResultAppearance(context.Background(), result, result.Features[0])
	if err != nil || style.Color != "#abcdef" {
		t.Fatal("explicit result override", style, err)
	}
	values, err := modelValues("PART", changed, second)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := second.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", changed, undo)
	if err != nil {
		t.Fatal(err)
	}
	values, err = modelValues("PART", restored, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err = changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err = applyModelValues("PART", restored, undo)
	if err != nil {
		t.Fatal(err)
	}
	result = PartModel{}
	json.Unmarshal(restored, &result)
	if result.GeometricalSets[0].Appearance != nil || result.Features[0].Appearance != nil || result.Features[0].Result.GeometryKey != "frozen-wire" {
		t.Fatal("two-step undo lost inheritance or frozen geometry")
	}
}

func TestObjectStyleSourcesAndImportedFaceReset(t *testing.T) {
	m := deletionFixture()
	opacity := 0.6
	m.GeometryDefaultAppearance = &SurfaceAppearance{GraphicProperties: GraphicProperties{Opacity: &opacity}}
	m.GeometricalSets[0].Appearance = &SurfaceAppearance{Color: "#12ab34"}
	m.DatumPoints = []DatumPoint{{ID: "p", Name: "P", GeometricalSetID: "curves", Construction: &ReferenceConstruction{Mode: "POINT_COORDINATES"}}}
	style := partObjectAppearances(m)["REFERENCE_POINT:p"]
	if style.Sources["color"].Kind != "GEOMETRICAL_SET" || style.Sources["opacity"].Kind != "DOCUMENT_DEFAULT" {
		t.Fatal("per-field source attribution", style)
	}
	m.Features[0].ImportedAppearance = &SurfaceAppearance{Faces: []FaceAppearance{{Anchor: modelcore.SemanticTopologyRef{}, Color: "#ff0000"}}}
	raw, _ := json.Marshal(m)
	payload, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "FEATURE", ID: "line"}}, ClearFaceOverrides: true})
	next, _, err := applyPartProperties(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var after PartModel
	json.Unmarshal(next, &after)
	if after.Features[0].Appearance == nil || len(after.Features[0].Appearance.Faces) != 1 || len(after.Features[0].Appearance.Faces[0].InheritedFields) != 5 {
		t.Fatal("imported colors would reappear after clear")
	}
	// Definition editors preserve graphic changes owned by SET_PROPERTIES.
	after.Features[0].ImportedAppearance = nil
	after.Features[0].Appearance = &SurfaceAppearance{Color: "#abcdef"}
	f := after.Features[0]
	f.Appearance = nil
	_, _, err = editSolidDefinition(after, &after.Features[0], f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.Features[0].Appearance == nil || after.Features[0].Appearance.Color != "#abcdef" {
		t.Fatal("editing definition lost appearance")
	}
}

func TestEngineeringDefaultsKeepRoleColorsAndInheritExplicitDocumentEdits(t *testing.T) {
	m := deletionFixture()
	m.Features[0].Result = &FeatureGeometryResult{ID: "result", Domain: "WIRE_SET", GeometryKey: "wire"}
	roughness := .5
	m.DefaultAppearance = &SurfaceAppearance{Color: "#98a8b8", GraphicProperties: GraphicProperties{Roughness: &roughness}}
	style, err := (&Service{}).partResultAppearance(context.Background(), m, m.Features[0])
	if err != nil {
		t.Fatal(err)
	}
	if style.Color != "" || style.Roughness != nil {
		t.Fatal("material creation defaults erased engineering role styling", style)
	}
	raw, _ := json.Marshal(m)
	payload, _ := json.Marshal(PropertyUpdateRequest{Targets: []PropertyTarget{{Kind: "DOCUMENT", ID: "part"}}, Appearance: map[string]json.RawMessage{"opacity": json.RawMessage(`0.4`)}})
	next, changes, err := applyPartProperties(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var changed PartModel
	json.Unmarshal(next, &changed)
	style, err = (&Service{}).partResultAppearance(context.Background(), changed, changed.Features[0])
	if err != nil {
		t.Fatal(err)
	}
	if style.Color != "" || style.Opacity == nil || *style.Opacity != .4 || style.Sources["opacity"].Kind != "DOCUMENT_DEFAULT" || !displayPropertiesOnly(changes) {
		t.Fatal("explicit field inheritance polluted another domain", style)
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
	json.Unmarshal(restored, &changed)
	var check PartModel
	json.Unmarshal(restored, &check)
	if check.GeometryDefaultAppearance != nil || check.DefaultAppearance.Color != "#98a8b8" {
		t.Fatal("document appearance compensation lost role defaults")
	}
}

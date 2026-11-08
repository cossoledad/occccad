package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func TestCurveChainsRequireFormalConnectionsAndKeepDirectionAcrossSelectionOrder(t *testing.T) {
	a := SketchEntity{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}}
	b := SketchEntity{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}}
	source := Feature{ID: "sketch", Sketch: &SketchFeature{Entities: []SketchEntity{b, a}}}
	chains, e := sketchCurveChains(source, []string{"b", "a"})
	if e != nil || len(chains) != 2 {
		t.Fatal("coordinate-only junction welded", chains, e)
	}
	source.Sketch.Constraints = []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef("a", "END"), cornerRef("b", "START")}}}
	chains, e = sketchCurveChains(source, []string{"b", "a"})
	if e != nil || len(chains) != 1 || chains[0].Closed || chains[0].Curves[0].EntityID != "a" {
		t.Fatal(chains, e)
	}
	reordered, e := sketchCurveChains(source, []string{"a", "b"})
	if e != nil || !reflect.DeepEqual(chains, reordered) {
		t.Fatal("selection order changes path identity", e)
	}
	source.Sketch.Entities = append(source.Sketch.Entities, SketchEntity{ID: "c", Kind: "LINE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 12, Y: 4}})
	source.Sketch.Constraints = append(source.Sketch.Constraints, SketchConstraint{ID: "branch", Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef("b", "START"), cornerRef("c", "START")}})
	if _, e = sketchCurveChains(source, []string{"a", "b", "c"}); e == nil {
		t.Fatal("branch accepted")
	}
	if _, e = sketchCurveChains(source, []string{"deleted"}); e == nil {
		t.Fatal("missing entity accepted")
	}
}

func TestThinFormalDirectionSurvivesAnExtensionWithEarlierEntityIdentity(t *testing.T) {
	m := newPartModel()
	body := m.ActiveBodyID
	source := Feature{ID: "s", Type: "SKETCH", BodyID: body, Sketch: &SketchFeature{Entities: []SketchEntity{{ID: "a", Kind: "LINE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}}, {ID: "b", Kind: "LINE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}}}, Constraints: []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef("a", "END"), cornerRef("b", "START")}}}}}
	m.Features = []Feature{source}
	feature := Feature{ID: "thin", Type: "LINEAR_EXTRUDE", Profile: "s", ThinWall: &ThinWallDefinition{Input: "CHAINS", Mode: "ONE_SIDED", Side: "LEFT", Join: "MITER", Thickness: 2, EntityIDs: []string{"a", "b"}}}
	if err := resolveThinDirections(m, &feature); err != nil {
		t.Fatal(err)
	}
	if len(feature.ThinWall.Directions) != 1 || feature.ThinWall.Directions[0].EntityID != "a" {
		t.Fatal("formal direction not frozen", feature.ThinWall.Directions)
	}
	source.Sketch.Entities = append(source.Sketch.Entities, SketchEntity{ID: "0", Kind: "LINE", Start: &SketchPoint2{X: 10, Y: 10}, End: &SketchPoint2{Y: 10}})
	source.Sketch.Constraints = append(source.Sketch.Constraints, SketchConstraint{ID: "extension", Kind: "COINCIDENT", References: []SketchGeometryRef{cornerRef("b", "END"), cornerRef("0", "START")}})
	chains, err := sketchCurveChains(source, []string{"a", "b", "0"})
	if err != nil {
		t.Fatal(err)
	}
	oriented, err := orientCurveChains(chains, feature.ThinWall.Directions)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range oriented[0].Curves {
		if c.EntityID == "a" && c.Reversed {
			t.Fatal("UUID order reversed the saved direction")
		}
	}
	if _, err := orientCurveChains(chains, []CurveChainDirection{{EntityID: "a"}, {EntityID: "b", Reversed: true}}); err == nil {
		t.Fatal("conflicting direction anchors accepted")
	}
}

func TestThinAnalyticSectionsPreserveSourcesAndDerivedIdentity(t *testing.T) {
	chain := sketchCurveChain{ID: "chain", Curves: []geometry.ProfileCurve{{EntityID: "a", Kind: "LINE", Start: [2]float64{}, End: [2]float64{10, 0}}, {EntityID: "b", Kind: "LINE", Start: [2]float64{10, 0}, End: [2]float64{10, 10}}}}
	definition := ThinWallDefinition{Input: "CHAINS", Mode: "SYMMETRIC", Side: "LEFT", Join: "MITER", Thickness: 2, EntityIDs: []string{"a", "b"}}
	feature := Feature{ID: "thin", Type: "LINEAR_EXTRUDE", Profile: "source", ThinWall: &definition}
	before, _ := json.Marshal(chain)
	section, origins, e := thinProfileOutline(feature, []sketchCurveChain{chain})
	if e != nil {
		t.Fatal(e)
	}
	regions, e := buildProfileRegions(section)
	if e != nil || len(regions) != 1 {
		t.Fatal("open chain not capped", e)
	}
	if area := polygonArea(sampleProfileLoop(regions[0].Outer)); area != 40 {
		t.Fatal("symmetric L section area", area)
	}
	ids := map[string]bool{}
	for _, entity := range section.Sketch.Entities {
		ids[entity.ID] = true
		if len(origins[entity.ID]) == 0 {
			t.Fatal("derived section lost source", entity.ID)
		}
	}
	definition.Thickness = 4
	changed, _, e := thinProfileOutline(feature, []sketchCurveChain{chain})
	if e != nil {
		t.Fatal(e)
	}
	for _, entity := range changed.Sketch.Entities {
		if !ids[entity.ID] {
			t.Fatal("thickness replaced derived identity")
		}
	}
	after, _ := json.Marshal(chain)
	if string(before) != string(after) {
		t.Fatal("source geometry mutated")
	}
	chain.Closed = true
	chain.Curves = []geometry.ProfileCurve{{EntityID: "circle", Kind: "CIRCLE", Radius: 10}}
	definition.Input = "REGIONS"
	definition.EntityIDs = nil
	definition.Mode = "ONE_SIDED"
	definition.Thickness = 2
	circle, _, e := thinProfileOutline(feature, []sketchCurveChain{chain})
	if e != nil {
		t.Fatal(e)
	}
	radii := map[float64]bool{}
	for _, entity := range circle.Sketch.Entities {
		radii[entity.Radius] = true
	}
	if !radii[8] || !radii[10] {
		t.Fatal("closed inward thin section", radii)
	}
	definition.Side = "RIGHT"
	circle, _, e = thinProfileOutline(feature, []sketchCurveChain{chain})
	if e != nil {
		t.Fatal(e)
	}
	radii = map[float64]bool{}
	for _, entity := range circle.Sketch.Entities {
		radii[entity.Radius] = true
	}
	if !radii[10] || !radii[12] {
		t.Fatal("closed outward thin section", radii)
	}
	definition.Side = "LEFT"
	definition.Thickness = 11
	if _, _, e = thinProfileOutline(feature, []sketchCurveChain{chain}); e == nil {
		t.Fatal("collapsed inner radius accepted")
	}
}

func TestThinThicknessExpressionAndModeParameterLifecycle(t *testing.T) {
	m := newPartModel()
	body := m.ActiveBodyID
	m.Features = []Feature{{ID: "s", Type: "SKETCH", BodyID: body, Sketch: &SketchFeature{SchemaVersion: 2, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}, Entities: []SketchEntity{{ID: "circle", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 10}}}}, {ID: "pad", Type: "LINEAR_EXTRUDE", BodyID: body, Profile: "s", Operation: "ADD", Length: 5, ThinWall: &ThinWallDefinition{Input: "REGIONS", Mode: "ONE_SIDED", Side: "LEFT", Join: "MITER", Thickness: 1}}}
	normalizePartModel(&m)
	f := m.Features[1]
	sources, e := featureParameterExpressions(m, f, map[string]string{"thin:thickness": "1 cm / 5"})
	if e != nil {
		t.Fatal(e)
	}
	raw, _, e := editSolidDefinition(m, &m.Features[1], f, sources)
	if e != nil {
		t.Fatal(e)
	}
	_ = json.Unmarshal(raw, &m)
	if m.Features[1].ThinWall.Thickness != 2 {
		t.Fatal("thickness formula not evaluated")
	}
	id := "parameter:pad:thin:thickness"
	found := false
	for _, p := range m.Parameters {
		if p.ParameterID == id {
			found = p.Source.Expression != nil
		}
	}
	if !found {
		t.Fatal("stable thickness source missing")
	}
	if _, e := featureParameterExpressions(m, m.Features[1], map[string]string{"thin:thickness2": "1 mm"}); e == nil {
		t.Fatal("inactive thickness expression accepted")
	}
	f = m.Features[1]
	copy := *f.ThinWall
	f.ThinWall = &copy
	f.ThinWall.Mode = "TWO_SIDED"
	f.ThinWall.Thickness2 = 1
	raw, _, e = editSolidDefinition(m, &m.Features[1], f, nil)
	if e != nil {
		t.Fatal(e)
	}
	_ = json.Unmarshal(raw, &m)
	found = false
	for _, p := range m.Parameters {
		if p.ParameterID == id {
			found = p.Source.Expression != nil
		}
	}
	if !found {
		t.Fatal("mode edit discarded driving formula")
	}
}

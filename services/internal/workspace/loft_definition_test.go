package workspace

import (
	"context"
	"testing"
)

func TestLoftPointDefinitionValidationAndReferenceEvaluation(t *testing.T) {
	base := Feature{ID: "base", Type: "SKETCH", Sketch: &SketchFeature{}}
	point := Feature{ID: "tip", Type: "SKETCH", Sketch: &SketchFeature{Support: SketchSupport{Type: "DATUM_PLANE", Plane: "XY", DatumPlaneID: "datum-xy"}, Entities: []SketchEntity{{ID: "p", Kind: "POINT", Point: &SketchPoint2{X: 3, Y: 4}}}}}
	ref := &PatternPointReference{SketchID: "tip", Reference: SketchGeometryRef{Target: "ENTITY", EntityID: "p", SubElement: "POINT"}}
	section := LoftSection{SketchID: "tip", Point: ref}
	f := Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: []LoftSection{{SketchID: "base"}, section}}
	earlier := map[string]Feature{"base": base, "tip": point}
	if err := validateSolidStage(f, earlier); err != nil {
		t.Fatal(err)
	}
	if loftSectionIdentity(section) == loftSectionIdentity(LoftSection{SketchID: "tip"}) {
		t.Fatal("point/profile identity collision")
	}
	f.Sections = []LoftSection{{SketchID: "base"}, section, {SketchID: "base"}}
	if err := validateSolidStage(f, earlier); err == nil {
		t.Fatal("intermediate point accepted")
	}
	f.Sections = []LoftSection{section, section}
	if err := validateSolidStage(f, earlier); err == nil {
		t.Fatal("no profile accepted")
	}
	f.Sections = []LoftSection{{SketchID: "base"}, {SketchID: "wrong", Point: ref}}
	if err := validateSolidStage(f, earlier); err == nil {
		t.Fatal("mismatched point source accepted")
	}
	service := &Service{}
	model := newPartModel()
	model.Features = []Feature{base, point}
	inputs, err := service.loftGeometrySections(context.Background(), "test", model, Feature{Sections: []LoftSection{section}}, earlier)
	if err != nil {
		t.Fatal(err)
	}
	if inputs[0].Point != [3]float64{3, 4, 0} || inputs[0].PointID == "" {
		t.Fatal(inputs)
	}
	model.Features[1].Sketch.Entities = nil
	if _, err := service.loftGeometrySections(context.Background(), "test", model, Feature{Sections: []LoftSection{section}}, earlier); err == nil {
		t.Fatal("missing point rebound")
	}
}

func TestLoftDatumPointDeletionProtection(t *testing.T) {
	model := newPartModel()
	model.DatumAxes = []DatumAxis{{ID: "axis", Direction: [3]float64{0, 0, 1}}}
	model.Features = []Feature{{ID: "loft", Type: "LOFT", Sections: []LoftSection{{Point: &PatternPointReference{AxisEntityID: "DATUM_AXIS:axis"}}}}}
	if _, _, err := deleteDatum(model, "DATUM_AXIS", "axis"); err == nil {
		t.Fatal("deleted referenced loft point datum")
	}
}

func TestLoftWholePointSketchBinding(t *testing.T) {
	point := SketchEntity{ID: "p", Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{}}
	sketch := Feature{ID: "tip", Type: "SKETCH", Sketch: &SketchFeature{Entities: []SketchEntity{point}}}
	earlier := map[string]Feature{"tip": sketch, "profile": {ID: "profile", Type: "SKETCH", Sketch: &SketchFeature{}}}
	source := []LoftSection{{SketchID: "profile"}, {SketchID: "tip"}}
	bound, err := bindLoftPointSketches(source, earlier)
	if err != nil || bound[1].Point == nil || bound[1].Point.Reference.EntityID != "p" {
		t.Fatalf("binding: %+v %v", bound, err)
	}
	if source[1].Point != nil {
		t.Fatal("mutated input")
	}
	for _, sections := range [][]LoftSection{bound, {bound[1], bound[0]}} {
		if err := validateSolidStage(Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: sections}, earlier); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateSolidStage(Feature{Type: "LOFT", Operation: "NEW_BODY", Sections: []LoftSection{bound[0], bound[1], {SketchID: "other"}}}, earlier); err == nil {
		t.Fatal("intermediate point accepted")
	}
	point.ID = "replacement"
	sketch.Sketch.Entities = []SketchEntity{point}
	retained, err := bindLoftPointSketches(bound, earlier)
	if err != nil || retained[1].Point.Reference.EntityID != "p" {
		t.Fatal("rebound deleted point", err)
	}
	profile := []LoftSection{{SketchID: "tip", CorrespondenceResolved: true}}
	retained, err = bindLoftPointSketches(profile, earlier)
	if err != nil || retained[0].Point != nil {
		t.Fatal("reinterpreted accepted profile", err)
	}
	sketch.Sketch.Entities = append(sketch.Sketch.Entities, SketchEntity{ID: "q", Kind: "POINT", Point: &SketchPoint2{}})
	if _, err := bindLoftPointSketches(source, earlier); err == nil {
		t.Fatal("ambiguous point sketch accepted")
	}
	sketch.Sketch.Entities = append(sketch.Sketch.Entities, SketchEntity{ID: "line", Kind: "LINE"})
	retained, err = bindLoftPointSketches(source, earlier)
	if err != nil || retained[1].Point != nil {
		t.Fatal("mixed sketch converted to point", err)
	}
}

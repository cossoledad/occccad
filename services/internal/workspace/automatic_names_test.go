package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAutomaticNamesBindToDefinitionsAndHistory(t *testing.T) {
	if got := numberedFeatureName([]Feature{{Type: "PAD", Name: "Extrude.1"}, {Type: "LINEAR_EXTRUDE", Name: "Extrude.3"}, {Name: "custom"}}, "PAD", "Extrude"); got != "Extrude.4" {
		t.Fatal(got)
	}
	counters := map[string]int{"Fix": 4}
	constraint := AssemblyConstraint{Kind: "FIX"}
	assignConstraintName(&constraint, &counters, []AssemblyConstraint{{Name: "fix.5"}})
	if constraint.Name != "Fix.6" || counters["Fix"] != 6 {
		t.Fatal(constraint.Name, counters)
	}
	sketch := SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: "datum-xy", Plane: "XY"}}
	circle := func(id string) SketchOperation {
		return SketchOperation{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: id, Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 0, Y: 0}, Radius: 3}}
	}
	if e := applySketchOperations(&sketch, []SketchOperation{circle("a"), circle("b"), circle("c")}); e != nil {
		t.Fatal(e)
	}
	if sketch.Entities[0].Name != "Circle.1" || sketch.Entities[2].Name != "Circle.3" {
		t.Fatal(sketch.Entities)
	}
	if e := applySketchOperations(&sketch, []SketchOperation{{Type: "DELETE_ENTITIES", EntityIDs: []string{"b"}}, circle("d")}); e != nil {
		t.Fatal(e)
	}
	if sketch.Entities[1].ID != "c" || sketch.Entities[1].Name != "Circle.3" || sketch.Entities[2].Name != "Circle.4" {
		t.Fatal(sketch.Entities)
	}
	nodes := sketchStructureChildren(sketch, "tree", "s", "doc", "rev", true)
	if nodes[0].Children[1].Name != "Circle.3" {
		t.Fatal(nodes)
	}
	raw, _ := json.Marshal(sketch)
	var reopened SketchFeature
	_ = json.Unmarshal(raw, &reopened)
	if !reflect.DeepEqual(sketch, reopened) {
		t.Fatal("name lost on reopen")
	}
	m := PartModel{Features: []Feature{{ID: "s", Name: "Sketch.1", Type: "SKETCH", Sketch: &sketch}}}
	payload, _ := json.Marshal(editSketchPayload{SketchID: "s", Operations: []SketchOperation{circle("e")}})
	before, _ := json.Marshal(m)
	_, changes, e := applyEditSketch(before, payload)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, change := range changes.Changes {
		if string(change.Target.SlotID) == "sketch.model" {
			var prior, after SketchFeature
			_ = json.Unmarshal(change.Before, &prior)
			_ = json.Unmarshal(change.After, &after)
			if prior.Entities[1].Name != "Circle.3" || after.Entities[len(after.Entities)-1].Name != "Circle.5" {
				t.Fatal(prior, after)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("names absent from formal ChangeSet")
	}
}

package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestReferenceVisibilityHistoryAndGeometryIndependence(t *testing.T) {
	model := newPartModel()
	initial, _ := json.Marshal(model)
	targets := []definitionVisibilityPayload{{EntityKind: "ORIGIN", EntityID: "origin"}, {EntityKind: "PLANE", EntityID: model.DatumPlanes[0].ID}, {EntityKind: "AXIS_SYSTEM", EntityID: model.AxisSystems[0].ID}, {EntityKind: "DATUM_POINT", EntityID: model.AxisSystems[0].ID}, {EntityKind: "AXIS", EntityID: model.AxisSystems[0].ID, Axis: "X"}}
	model.DatumAxes = append(model.DatumAxes, DatumAxis{ID: "datum-test", Name: "axis", Direction: [3]float64{1, 0, 0}})
	initial, _ = json.Marshal(model)
	targets = append(targets, definitionVisibilityPayload{EntityKind: "DATUM_AXIS", EntityID: "datum-test"})
	for _, input := range targets {
		t.Run(input.EntityKind, func(t *testing.T) {
			payload, _ := json.Marshal(input)
			next, set, err := applyDefinitionVisibility(initial, payload)
			if err != nil {
				t.Fatal(err)
			}
			if len(set.ImpactSeeds) != 0 {
				t.Fatal("display must not invalidate geometry")
			}
			before, err := modelValues("PART", initial, set)
			if err != nil {
				t.Fatal(err)
			}
			after, err := modelValues("PART", next, set)
			if err != nil {
				t.Fatal(err)
			}
			if reflect.DeepEqual(before, after) {
				t.Fatal("display slot was not recorded")
			}
			restored, err := applyModelValues("PART", next, before)
			if err != nil {
				t.Fatal(err)
			}
			restoredValues, _ := modelValues("PART", restored, set)
			if !reflect.DeepEqual(before, restoredValues) {
				t.Fatal("Undo lost display default")
			}
			redone, err := applyModelValues("PART", restored, after)
			if err != nil {
				t.Fatal(err)
			}
			redoneValues, _ := modelValues("PART", redone, set)
			if !reflect.DeepEqual(after, redoneValues) {
				t.Fatal("Redo lost display state")
			}
			var reopened PartModel
			_ = json.Unmarshal(redone, &reopened)
			visible, found := partReferenceVisibility(&reopened, set.Changes[0].Target, nil, false)
			if !found || visible == nil || *visible {
				t.Fatal("cold reopen visibility")
			}
			if input.EntityKind == "AXIS" && !axisDisplayVisible(reopened.AxisSystems[0], "Y") {
				t.Fatal("hiding X hid Y")
			}
			if input.EntityKind == "DATUM_AXIS" {
				axis := reopened.DatumAxes[0]
				axis.Visible = nil
				axis.Name = "edited hidden axis"
				payload, _ := json.Marshal(editDatumPayload{Axis: &axis})
				edited, _, err := applyEditDatum(redone, payload)
				if err != nil {
					t.Fatal(err)
				}
				var editedModel PartModel
				_ = json.Unmarshal(edited, &editedModel)
				if editedModel.DatumAxes[0].Visible == nil || *editedModel.DatumAxes[0].Visible {
					t.Fatal("datum definition edit reset visibility")
				}
			}
			priorGraph, _, err := buildPartEvaluation(model, "before", "hash", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			nextGraph, _, err := buildPartEvaluation(reopened, "after", "hash", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(priorGraph.Nodes, nextGraph.Nodes) {
				t.Fatal("display polluted canonical geometry inputs")
			}
		})
	}
}

func TestManagedParameterAliasesAreReadableUniqueAndStable(t *testing.T) {
	model := newPartModel()
	model.DatumAxes = append(model.DatumAxes, DatumAxis{ID: "datum-test", Definition: &DatumTransform{Source: DatumReference{Kind: "AXIS_SYSTEM", EntityID: model.AxisSystems[0].ID, Axis: "X"}}})
	model.DatumPlanes = append(model.DatumPlanes, DatumPlane{ID: "datum-plane", Definition: &DatumTransform{Source: DatumReference{Kind: "PLANE", EntityID: model.DatumPlanes[0].ID}}})
	ensureFeatureParameters(&model)
	keys := map[string]bool{}
	ids := map[string]string{}
	for _, p := range model.Parameters {
		if !validParameterKey(p.Key) || keys[p.Key] || p.Key == "" {
			t.Fatalf("invalid alias %+v", p)
		}
		keys[p.Key] = true
		ids[p.ParameterID] = p.Key
		if p.OwnerFeatureID == "datum-test" && p.PropertySlot == "datum:distance" && p.Key != "distance_2" {
			t.Fatalf("deterministic datum order %s", p.Key)
		}
	}
	ensureFeatureParameters(&model)
	for _, p := range model.Parameters {
		if ids[p.ParameterID] != p.Key {
			t.Fatal("normalization changed alias")
		}
	}
	nodes := partStructureChildren(model, "part", "doc", "revision", true)
	var inspect func([]DocumentStructureNode)
	inspect = func(nodes []DocumentStructureNode) {
		for _, node := range nodes {
			if node.Kind == "PARAMETER" && node.ParameterAlias != ids[node.EntityID] {
				t.Fatal("tree alias did not preserve parameter identity")
			}
			inspect(node.Children)
		}
	}
	inspect(nodes)
}

func TestAssemblyConstraintVisibilityDoesNotSuppressAndRestores(t *testing.T) {
	model := ProductModel{Constraints: []AssemblyConstraint{{ID: "constraint", Kind: "FIX"}}}
	initial, _ := json.Marshal(model)
	payload, _ := json.Marshal(definitionVisibilityPayload{EntityKind: "ASSEMBLY_CONSTRAINT", EntityID: "constraint", Visible: false})
	next, set, err := applyConstraintVisibility(initial, payload)
	if err != nil {
		t.Fatal(err)
	}
	var hidden ProductModel
	_ = json.Unmarshal(next, &hidden)
	if hidden.Constraints[0].Suppressed || visibleOrDefault(hidden.Constraints[0].Visible) {
		t.Fatal("visibility changed suppression")
	}
	before, _ := modelValues("PRODUCT", initial, set)
	restored, err := applyModelValues("PRODUCT", next, before)
	if err != nil {
		t.Fatal(err)
	}
	var shown ProductModel
	_ = json.Unmarshal(restored, &shown)
	if !visibleOrDefault(shown.Constraints[0].Visible) {
		t.Fatal("constraint Undo")
	}
}

func TestAssemblyConstraintSetVisibilityAtomicHistory(t *testing.T) {
	model := ProductModel{Constraints: []AssemblyConstraint{
		{ID: "contact", Kind: "CONTACT", EvaluationStatus: "VERIFIED"},
		{ID: "fixed", Kind: "FIX", Visible: boolPointer(false), Suppressed: true, EvaluationStatus: "NOT_UPDATED"},
	}}
	initial, _ := json.Marshal(model)
	payload, _ := json.Marshal(definitionVisibilityPayload{EntityKind: "ASSEMBLY_CONSTRAINT_SET", EntityID: "assembly-constraints", Visible: false})
	next, set, err := applyConstraintVisibility(initial, payload)
	if err != nil || len(set.Changes) != 2 || len(set.ImpactSeeds) != 0 {
		t.Fatal("bulk display must record one atomic ChangeSet without geometry seeds", set, err)
	}
	var hidden ProductModel
	_ = json.Unmarshal(next, &hidden)
	for i := range hidden.Constraints {
		if visibleOrDefault(hidden.Constraints[i].Visible) {
			t.Fatal("constraint remained visible")
		}
		hidden.Constraints[i].Visible = model.Constraints[i].Visible
	}
	if !reflect.DeepEqual(model, hidden) {
		t.Fatal("bulk display changed constraint definitions/status/activation")
	}
	before, err := modelValues("PRODUCT", initial, set)
	if err != nil {
		t.Fatal(err)
	}
	after, err := modelValues("PRODUCT", next, set)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, before)
	if err != nil {
		t.Fatal(err)
	}
	var undone ProductModel
	_ = json.Unmarshal(restored, &undone)
	if !reflect.DeepEqual(model, undone) {
		t.Fatal("Undo lost each constraint's prior local visibility")
	}
	redone, err := applyModelValues("PRODUCT", restored, after)
	if err != nil {
		t.Fatal(err)
	}
	var result ProductModel
	_ = json.Unmarshal(redone, &result)
	for _, c := range result.Constraints {
		if visibleOrDefault(c.Visible) {
			t.Fatal("Redo failed to hide all constraints")
		}
	}
	invalid, _ := json.Marshal(definitionVisibilityPayload{EntityKind: "ASSEMBLY_CONSTRAINT_SET", EntityID: "unrelated-set"})
	if _, _, err := applyConstraintVisibility(initial, invalid); err == nil {
		t.Fatal("invalid group identity accepted")
	}
}

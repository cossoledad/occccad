package workspace

import (
	"encoding/json"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func deletionFixture() PartModel {
	m := newPartModel()
	m.GeometricalSets = []GeometricalSet{{ID: "curves", Name: "Curves"}, {ID: "surfaces", Name: "Surfaces"}}
	line := testGeometryLine()
	line.GeometricalSetID = "curves"
	m.Features = []Feature{line, {ID: "surface", Type: "GEOMETRY", GeometricalSetID: "surfaces", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: line.ID, ResultID: "result"}}, Parameters: GeometryOperationParameters{Distance: 20, Direction: [3]float64{0, 0, 1}}}}}
	normalizePartModel(&m)
	return m
}
func TestPartDeletePlanRetainsTypedMissingInputsAndCompensates(t *testing.T) {
	m := deletionFixture()
	raw, _ := json.Marshal(m)
	payload := deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "line"}}}
	plan, _, err := deletionPlan(m, payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Affected) != 1 || plan.Affected[0].ID != "surface" {
		t.Fatalf("missing impact: %+v", plan)
	}
	payload.PlanDigest = plan.Digest
	next, changes, err := applyPartObjectDeletion(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(next, &result)
	if len(result.Features) != 1 || result.Features[0].ID != "surface" || len(result.RetiredReferences) == 0 {
		t.Fatalf("definition lost: %+v", result)
	}
	graph, _, err := buildPartEvaluation(result, "r", "h", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	markRetiredReferenceFailures(&result, graph)
	if result.Features[0].EvaluationStatus != "FAILED" {
		t.Fatal("missing input was not diagnosed")
	}
	if graph.Nodes[modelcore.DependencyKey("feature:line")].Type != "MISSING_REFERENCE" {
		t.Fatal("missing graph endpoint")
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
	result = PartModel{}
	json.Unmarshal(restored, &result)
	if len(result.Features) != 2 || len(result.RetiredReferences) != 0 {
		t.Fatal("undo did not restore identities")
	}
	if err = validateAndResolvePartParameters(&result); err != nil {
		t.Fatal(err)
	}
	m.Features[1].Geometry.Inputs[0].FeatureID = "unknown"
	if err = validatePartStructure(m); err == nil {
		t.Fatal("arbitrary unknown references must remain invalid")
	}
}
func TestPartDeleteSetCascadeSharedInputsAndStalePlan(t *testing.T) {
	m := deletionFixture()
	raw, _ := json.Marshal(m)
	target := []deleteNodePayload{{TargetKind: "GEOMETRICAL_SET", TargetID: "curves"}}
	plan, _, err := deletionPlan(m, deleteNodesPayload{Targets: target})
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := applyPartObjectDeletion(raw, deleteNodesPayload{Targets: target, Options: DeleteOptions{DeleteDependents: true}})
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(next, &result)
	if len(result.Features) != 0 || len(result.GeometricalSets) != 1 || len(result.Parameters) != 0 || len(result.RetiredReferences) != 0 {
		t.Fatal("cascade left owned objects or removed unrelated set")
	}
	if _, _, err = applyPartObjectDeletion(raw, deleteNodesPayload{Targets: target, Options: DeleteOptions{DeleteDependents: true}, PlanDigest: plan.Digest}); err == nil {
		t.Fatal("changed options accepted stale impact plan")
	}
	m.Features = append(m.Features, Feature{ID: "second", Type: "GEOMETRY", GeometricalSetID: "surfaces", Geometry: &GeometricFeatureDefinition{Kind: "SURFACE_EXTRUDE", Inputs: []GeometricInput{{Kind: "RESULT", FeatureID: "line", ResultID: "result"}}, Parameters: GeometryOperationParameters{Distance: 10, Direction: [3]float64{0, 1, 0}}}})
	normalizePartModel(&m)
	plan, removed, err := deletionPlan(m, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "surface"}}, Options: DeleteOptions{DeleteExclusiveParents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, gone := removed[objectKey("FEATURE", "line")]; gone {
		t.Fatal("shared input deleted", plan)
	}
}
func TestPartDeleteBodyRemovesOwnedFeaturesButRetainsOtherBodies(t *testing.T) {
	m := newPartModel()
	sketch := testRectangleSketch("sketch", "XY")
	sketch.BodyID = m.ActiveBodyID
	m.Features = []Feature{sketch}
	normalizePartModel(&m)
	raw, _ := json.Marshal(m)
	next, _, err := applyPartObjectDeletion(raw, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "BODY", TargetID: m.ActiveBodyID}}})
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(next, &result)
	normalizePartModel(&result)
	if len(result.Bodies) != 0 || len(result.Features) != 0 || result.ActiveBodyID != "" {
		t.Fatal("last body recreated or leaked owned definitions")
	}
}

func TestPartDeletionKeepsUnavailableParameterTypesAndBlocksExpressions(t *testing.T) {
	m := deletionFixture()
	var source modelcore.ParameterDefinition
	for _, p := range m.Parameters {
		if p.OwnerFeatureID == "line" && p.PropertySlot == "geometry:point:end:x" {
			source = p
		}
	}
	expression, err := modelcore.CompileExpression("source * 2", map[string]modelcore.ParameterBinding{"source": {ParameterID: source.ParameterID, Dimension: source.Dimension}}, source.Dimension, "mm")
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Parameters {
		p := &m.Parameters[i]
		if p.OwnerFeatureID == "surface" && p.PropertySlot == "geometry:distance" {
			p.Source = modelcore.ValueSource{Expression: &expression}
		}
	}
	if err = validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(m)
	next, _, err := applyPartObjectDeletion(raw, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "line"}}})
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(next, &result)
	found := false
	for _, p := range result.Parameters {
		if p.OwnerFeatureID == "line" {
			t.Fatal("deleted parameter is still live")
		}
		if p.OwnerFeatureID == "surface" && p.PropertySlot == "geometry:distance" {
			found = p.CurrencyStatus == "BLOCKED" && p.EvaluatedValue == nil
		}
	}
	if !found {
		t.Fatal("expression consumed the deleted parameter's old value")
	}
}

func TestPartContainerMoveAndDissolvePreserveInputsAndCompensation(t *testing.T) {
	m := deletionFixture()
	m.GeometricalSets = append(m.GeometricalSets, GeometricalSet{ID: "child", Name: "Child", ParentID: "curves"})
	m.Features[0].GeometricalSetID = "child"
	next, changes, err := applyPartContainerChange(m, geometricalSetPayload{Action: "DISSOLVE", Set: GeometricalSet{ID: "child"}})
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	json.Unmarshal(next, &result)
	if len(result.GeometricalSets) != 2 || result.Features[0].GeometricalSetID != "curves" || result.Features[1].Geometry.Inputs[0].FeatureID != "line" {
		t.Fatal("dissolve changed reference identity")
	}
	if !geometricalSetChangesOnly(changes) {
		t.Fatal("organization change requested geometry evaluation")
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
	result = PartModel{}
	json.Unmarshal(restored, &result)
	if len(result.GeometricalSets) != 3 || result.Features[0].GeometricalSetID != "child" {
		t.Fatal("container undo lost original placement")
	}
	if _, _, err = applyPartContainerChange(m, geometricalSetPayload{Action: "MOVE", Set: GeometricalSet{ID: "child"}, Targets: []deleteNodePayload{{TargetKind: "GEOMETRICAL_SET", TargetID: "curves"}}}); err == nil {
		t.Fatal("container cycle accepted")
	}
}

func TestPartDeletionExclusiveInputsProtectFormulaAndInputPortReaders(t *testing.T) {
	m := deletionFixture()
	targets := []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "surface"}}
	_, removed, err := deletionPlan(m, deleteNodesPayload{Targets: targets, Options: DeleteOptions{DeleteExclusiveParents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed[objectKey("FEATURE", "line")]; !ok {
		t.Fatal("unshared construction input was not cleaned")
	}
	var source modelcore.ParameterDefinition
	for _, p := range m.Parameters {
		if p.OwnerFeatureID == "line" && p.PropertySlot == "geometry:point:end:x" {
			source = p
		}
	}
	expression, err := modelcore.CompileExpression("source * 2", map[string]modelcore.ParameterBinding{"source": {ParameterID: source.ParameterID, Dimension: source.Dimension}}, source.Dimension, "mm")
	if err != nil {
		t.Fatal(err)
	}
	m.Parameters = append(m.Parameters, modelcore.ParameterDefinition{ParameterID: "formula", Key: "read_line", Label: "Shared dimension", Dimension: source.Dimension, DisplayUnit: "mm", Source: modelcore.ValueSource{Expression: &expression}})
	_, removed, err = deletionPlan(m, deleteNodesPayload{Targets: targets, Options: DeleteOptions{DeleteExclusiveParents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed[objectKey("FEATURE", "line")]; ok {
		t.Fatal("input read by a surviving formula was removed")
	}
	m = deletionFixture()
	m.ContextInputs = []ContextInput{{ID: "port", Name: "Input", Type: "CURVE", Target: ContextInputTarget{Kind: "FEATURE_INPUT", TargetID: "line"}}}
	_, removed, err = deletionPlan(m, deleteNodesPayload{Targets: targets, Options: DeleteOptions{DeleteExclusiveParents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed[objectKey("FEATURE", "line")]; ok {
		t.Fatal("reusable interface input was removed")
	}
	plan, _, err := deletionPlan(m, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "line"}}})
	if err != nil || len(plan.Publications) != 1 || plan.Publications[0].Kind != "CONTEXT_INPUT" {
		t.Fatal("interface impact missing", plan, err)
	}
}

func TestPartBodyDeletionUndoRetainsImportedDefinitionAndGraphics(t *testing.T) {
	m := newPartModel()
	body := m.Bodies[0].ID
	m.Bodies[0].Appearance = &SurfaceAppearance{Color: "#12ab34"}
	m.Features = []Feature{{ID: "import", Type: "IMPORT_BODY", BodyID: body, GeometryKey: "frozen-imported-brep", ImportDefinitionID: "definition-root"}}
	normalizePartModel(&m)
	raw, _ := json.Marshal(m)
	next, changes, err := applyPartObjectDeletion(raw, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "BODY", TargetID: body}}})
	if err != nil {
		t.Fatal(err)
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
	var after PartModel
	json.Unmarshal(restored, &after)
	if len(after.Features) != 1 || after.Features[0].GeometryKey != "frozen-imported-brep" || after.Features[0].ImportDefinitionID != "definition-root" || after.Bodies[0].Appearance.Color != "#12ab34" {
		t.Fatal("imported definition or graphic baseline lost on deletion compensation")
	}
	publication := Publication{Target: PublicationTarget{Kind: "TOPOLOGY", PersistentSelection: &modelcore.PersistentSelection{SourceResult: &modelcore.GeometryResultReference{FeatureID: "result-owner", ResultID: "result"}, Anchor: modelcore.SemanticTopologyRef{FeatureID: "upstream"}}}}
	if !publicationTargetsObject(publication, RetiredPartObject{Kind: "FEATURE", ID: "result-owner"}) {
		t.Fatal("publication lost explicit result owner behind an upstream semantic anchor")
	}
}

func TestPartDeletionProtectsInputsReadBySuppressedDefinitions(t *testing.T) {
	m := deletionFixture()
	suppressed := m.Features[1]
	suppressed.ID = "suppressed-consumer"
	suppressed.Suppressed = true
	m.Features = append(m.Features, suppressed)
	normalizePartModel(&m)
	_, removed, err := deletionPlan(m, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "surface"}}, Options: DeleteOptions{DeleteExclusiveParents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed[objectKey("FEATURE", "line")]; ok {
		t.Fatal("input used by a suppressed definition was treated as exclusive")
	}
	plan, _, err := deletionPlan(m, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "line"}}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range plan.Affected {
		if item.ID == suppressed.ID {
			found = item.State == "INPUT_MISSING"
		}
	}
	if !found {
		t.Fatal("impact plan omitted a suppressed consumer", plan)
	}
	_, removed, err = deletionPlan(m, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: "FEATURE", TargetID: "line"}}, Options: DeleteOptions{DeleteDependents: true}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := removed[objectKey("FEATURE", suppressed.ID)]; !ok {
		t.Fatal("cascade skipped a stored suppressed dependency")
	}
}

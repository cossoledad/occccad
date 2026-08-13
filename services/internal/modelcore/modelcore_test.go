package modelcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

type testHandler struct{}

func (testHandler) TypeURI() string                   { return "occccad://test/value/set" }
func (testHandler) SupportedSchemaVersions() []uint32 { return []uint32{1} }
func (testHandler) TargetDocumentTypes() []string     { return []string{"PART"} }
func (testHandler) Apply(model, payload json.RawMessage) (json.RawMessage, ChangeSet, error) {
	var value struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, ChangeSet{}, err
	}
	change, _ := NewChange(ChangeUpdate, PropertyAddress{EntityID: "entity-1", SlotID: "test.value"}, 0, value.Value)
	set := ChangeSet{Changes: []ModelChange{change}, ImpactSeeds: []DependencyKey{"property:entity-1:test.value"}}
	next, _ := json.Marshal(value)
	return next, set, nil
}

func TestRegistryRejectsUnsupportedSchemaAndAppliesRegisteredHandler(t *testing.T) {
	registry, err := NewRegistry(testHandler{})
	if err != nil {
		t.Fatal(err)
	}
	command := DomainCommand{CommandID: "command-1", TypeURI: "occccad://test/value/set", SchemaVersion: 2, Payload: json.RawMessage(`{"value":2}`)}
	if _, _, err = registry.Apply("PART", json.RawMessage(`{"value":0}`), command); !errors.Is(err, ErrUnsupportedCommand) {
		t.Fatalf("expected unsupported schema, got %v", err)
	}
	command.SchemaVersion = 1
	next, changes, err := registry.Apply("PART", json.RawMessage(`{"value":0}`), command)
	if err != nil {
		t.Fatal(err)
	}
	if string(next) != `{"value":2}` || changes.CanonicalDigest == "" {
		t.Fatalf("unexpected result: %s %#v", next, changes)
	}
}

func TestChangeSetCompensationDetectsLaterEdit(t *testing.T) {
	address := PropertyAddress{EntityID: "feature-1", SlotID: "pad.length"}
	change, _ := NewChange(ChangeUpdate, address, 10, 20)
	set := ChangeSet{Changes: []ModelChange{change}}
	if err := set.Finalize(); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(20)
	values, err := set.Compensate(map[PropertyAddress]json.RawMessage{address: after})
	if err != nil {
		t.Fatal(err)
	}
	if string(values[address]) != "10" {
		t.Fatalf("unexpected compensation %s", values[address])
	}
	later, _ := json.Marshal(30)
	if _, err = set.Compensate(map[PropertyAddress]json.RawMessage{address: later}); !errors.Is(err, ErrChangeConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestValueDigestSurvivesJSONBKeyReordering(t *testing.T) {
	original := json.RawMessage(`{"id":"extrude-1","type":"PAD","length":10,"operation":"ADD"}`)
	jsonbRoundTrip := json.RawMessage(`{ "type": "PAD", "operation": "ADD", "length": 10, "id": "extrude-1" }`)
	if ValueDigest(original) != ValueDigest(jsonbRoundTrip) {
		t.Fatal("semantically equal JSON must retain its digest across a jsonb round trip")
	}
	change, err := NewChange(ChangeCreate, PropertyAddress{EntityID: "extrude-1", SlotID: "entity"}, nil, original)
	if err != nil {
		t.Fatal(err)
	}
	set := ChangeSet{Changes: []ModelChange{change}}
	if err := set.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := set.Compensate(map[PropertyAddress]json.RawMessage{change.Target: jsonbRoundTrip}); err != nil {
		t.Fatalf("jsonb reordering caused a false undo conflict: %v", err)
	}
}

func TestExpressionBindsStableIDsAndChecksUnits(t *testing.T) {
	expression, err := CompileExpression("Width + 5 mm", map[string]ParameterBinding{"Width": {ParameterID: "parameter-width", Dimension: LengthDimension}}, LengthDimension)
	if err != nil {
		t.Fatal(err)
	}
	if len(expression.Reads) != 1 || expression.Reads[0] != "parameter:parameter-width" {
		t.Fatalf("expression was not ID-bound: %#v", expression.Reads)
	}
	width, _ := NewQuantity(20, "mm")
	value, err := EvaluateExpression(expression, map[string]Quantity{"parameter-width": width})
	if err != nil {
		t.Fatal(err)
	}
	if difference := value.SIValue - 0.025; difference > 1e-12 || difference < -1e-12 {
		t.Fatalf("got SI value %g", value.SIValue)
	}
	if _, err = CompileExpression("Width + 2", map[string]ParameterBinding{"Width": {ParameterID: "parameter-width", Dimension: LengthDimension}}, LengthDimension); !errors.Is(err, ErrUnitMismatch) {
		t.Fatalf("expected unit mismatch, got %v", err)
	}
}

func TestDependencyCycleAndDirtyClosure(t *testing.T) {
	nodes := []DependencyNode{{Key: "a", Phase: 1}, {Key: "b", Phase: 1}, {Key: "c", Phase: 2}}
	graph, err := NewDependencyGraph(nodes, []DependencyEdge{{Source: "a", Target: "b", Kind: ReadValue}, {Source: "b", Target: "c", Kind: ReadGeometry}})
	if err != nil {
		t.Fatal(err)
	}
	dirty := graph.DirtyClosure([]DependencyKey{"a"})
	if fmt.Sprint(dirty) != "[a b c]" {
		t.Fatalf("unexpected closure %v", dirty)
	}
	_, err = NewDependencyGraph(nodes, []DependencyEdge{{Source: "a", Target: "b", Kind: ReadValue}, {Source: "b", Target: "a", Kind: ReadValue}})
	if !errors.Is(err, ErrDependencyCycle) {
		t.Fatalf("expected cycle, got %v", err)
	}
}

func TestIncrementalEvaluationEqualsColdEvaluation(t *testing.T) {
	nodes := []DependencyNode{{Key: "width", Phase: 1, CanonicalInput: json.RawMessage(`20`)}, {Key: "sketch", Phase: 2, CanonicalInput: json.RawMessage(`"rectangle"`)}, {Key: "pad", Phase: 3, CanonicalInput: json.RawMessage(`10`)}, {Key: "report", Phase: 4, CanonicalInput: json.RawMessage(`"volume"`)}}
	edges := []DependencyEdge{{Source: "width", Target: "sketch", Kind: ReadValue}, {Source: "sketch", Target: "pad", Kind: ReadGeometry}, {Source: "pad", Target: "report", Kind: ReadGeometry}}
	graph, err := NewDependencyGraph(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	evaluator := func(node DependencyNode, deps map[DependencyKey]NodeResult) (string, error) {
		data, _ := json.Marshal(struct {
			Input json.RawMessage
			Deps  map[DependencyKey]NodeResult
		}{node.CanonicalInput, deps})
		return ValueDigest(data), nil
	}
	base, err := graph.Evaluate("r1", "model-1", "eval-1", "units-1", nil, nil, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	nodes[0].CanonicalInput = json.RawMessage(`25`)
	changed, err := NewDependencyGraph(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	incremental, err := changed.Evaluate("r2", "model-2", "eval-1", "units-1", []DependencyKey{"width"}, &base, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := changed.Evaluate("r2", "model-2", "eval-1", "units-1", nil, nil, evaluator)
	if err != nil {
		t.Fatal(err)
	}
	incJSON, _ := json.Marshal(incremental.NodeResults)
	coldJSON, _ := json.Marshal(cold.NodeResults)
	if string(incJSON) != string(coldJSON) {
		t.Fatalf("incremental differs from cold\nincremental=%s\ncold=%s", incJSON, coldJSON)
	}
}

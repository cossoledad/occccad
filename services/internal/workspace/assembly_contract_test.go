package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// Test-only adapter: the single catalog supplies inputs and independent expected
// parameter availability; the implementation remains assemblyCapabilities.
func TestAssemblyContractCatalog(t *testing.T) {
	data, err := os.ReadFile("../../../tests/assembly-contract/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Cases []struct {
			CaseID   string `json:"caseId"`
			Adapter  string `json:"adapter"`
			Input    struct{ Kind, First, Second string }
			Expected struct {
				Direction, DistanceSide, DirectedAngle bool
			}
		}
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	var selected []string
	if raw := os.Getenv("OCCCCAD_ASSEMBLY_CONTRACT_CASES"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &selected); err != nil {
			t.Fatal(err)
		}
	}
	wanted := map[string]bool{}
	for _, id := range selected {
		wanted[id] = true
	}
	count := 0
	for _, c := range catalog.Cases {
		if c.Adapter != "go-flags" || (selected != nil && !wanted[c.CaseID]) {
			continue
		}
		count++
		t.Run(c.CaseID, func(t *testing.T) {
			got := assemblyCapabilities(c.Input.Kind, c.Input.First, c.Input.Second)
			t.Logf("observed %s/%s/%s: direction=%t distanceSide=%t directedAngle=%t", c.Input.Kind, c.Input.First, c.Input.Second, got.direction, got.distanceSide, got.directedAngle)
			if got.direction != c.Expected.Direction || got.distanceSide != c.Expected.DistanceSide || got.directedAngle != c.Expected.DirectedAngle {
				t.Fatalf("%s: got %+v, expected %+v", c.CaseID, got, c.Expected)
			}
			// Availability is symmetric; this does not assert equal preferred pose or
			// erase the FIRST-plane signed-quantity transformation requirement.
			swapped := assemblyCapabilities(c.Input.Kind, c.Input.Second, c.Input.First)
			if got != swapped {
				t.Fatalf("parameter availability differs after exchange: %+v %+v", got, swapped)
			}
		})
	}
	if count == 0 {
		t.Fatal("no catalog parameter-availability cases executed")
	}
}

func TestAssemblyContractInvalidDefinitions(t *testing.T) {
	for _, kind := range []string{"CONTACT", "FIX_TOGETHER"} {
		t.Run(kind, func(t *testing.T) {
			payload, err := json.Marshal(addAssemblyConstraintPayload{Constraint: AssemblyConstraint{
				ID: "invalid-contract", Kind: kind, First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "a"},
			}})
			if err != nil {
				t.Fatal(err)
			}
			next, changes, err := applyAddAssemblyConstraint(json.RawMessage(`{"instances":[],"constraints":[]}`), payload)
			if !errors.Is(err, ErrValidation) || next != nil || len(changes.Changes) != 0 {
				t.Fatalf("invalid missing Contact branch/group members must reject without candidate/ChangeSet: %s %+v %v", next, changes, err)
			}
		})
	}
}

func TestAssemblyContractValidationNoCandidate(t *testing.T) {
	for _, constraint := range []AssemblyConstraint{
		{ID: "negative-angle", Kind: "ANGLE", Value: -1},
		{ID: "missing-axis", Kind: "ANGLE", AngleRelation: "DIRECTED"},
		{ID: "relation-measure", Kind: "ANGLE", AngleRelation: "PARALLEL", Mode: "MEASURED"},
		{ID: "fix-measure", Kind: "FIX", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "a"}, Mode: "MEASURED"},
		{ID: "bad-pose", Kind: "FIX", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: "a"}, FixedPose: &InstancePose{}},
		{ID: "mode-is-not-suppression", Kind: "ANGLE", Mode: "SUPPRESSED"},
	} {
		t.Run(constraint.ID, func(t *testing.T) {
			payload, err := json.Marshal(addAssemblyConstraintPayload{Constraint: constraint})
			if err != nil {
				t.Fatal(err)
			}
			next, changes, err := applyAddAssemblyConstraint(json.RawMessage(`{"instances":[],"constraints":[]}`), payload)
			if !errors.Is(err, ErrValidation) || next != nil || len(changes.Changes) != 0 {
				t.Fatalf("illegal command produced candidate: %s %+v %v", next, changes, err)
			}
		})
	}
}

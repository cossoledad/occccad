package assistant

import (
	"math"
	"testing"

	"github.com/occccad/occccad/internal/workspace"
)

func assemblyContractFixture() (Run, workspace.ProductModel, GoalRequirement) {
	zero := 0.0
	one := 1
	r := Run{Targets: map[string]string{"AXIS_CONSTRAINT": "axis", "PLANE_CONSTRAINT": "plane"}, Scope: FrozenScope{Selections: []SelectedReference{{InstancePath: []string{"moving"}}, {InstancePath: []string{"reference"}}}}}
	secondAxis := workspace.AssemblyGeometryRef{InstanceID: "reference", Kind: "AXIS", GeometryID: "reference-axis"}
	secondPlane := workspace.AssemblyGeometryRef{InstanceID: "reference", Kind: "PLANE", GeometryID: "reference-plane"}
	p := workspace.ProductModel{Constraints: []workspace.AssemblyConstraint{{ID: "fixed", Kind: "FIX", FixMode: "SPACE", First: workspace.AssemblyGeometryRef{InstanceID: "reference", Kind: "BODY"}}, {ID: "axis", Kind: "CONCENTRIC", DirectionRelation: "SAME", First: workspace.AssemblyGeometryRef{InstanceID: "moving", Kind: "AXIS", GeometryID: "moving-axis"}, Second: &secondAxis}, {ID: "plane", Kind: "COINCIDENT", DirectionRelation: "SAME", First: workspace.AssemblyGeometryRef{InstanceID: "moving", Kind: "PLANE", GeometryID: "moving-plane"}, Second: &secondPlane}}}
	requirement := GoalRequirement{Kind: "ASSEMBLY_FIT", SourceTurnID: "user-source", Quote: "轴线方向 SAME，底端面有向法线 SAME，有向偏置0mm，targetDOF=1", Unit: "mm", AxisDirectionRelation: "SAME", PlaneDirectionRelation: "SAME", OffsetMM: &zero, TargetDOF: &one}
	return r, p, requirement
}

func TestAssistantAssemblyGoalUsesUserExpectedDirectionsOffsetAndDOF(t *testing.T) {
	r, product, requirement := assemblyContractFixture()
	input, reason := assemblyGoalInput(r, product, "actual-revision", requirement)
	if reason != "" || input.AxisDirectionRelation != "SAME" || input.DirectionRelation != "SAME" || input.OffsetMM == nil || *input.OffsetMM != 0 || input.TargetDOF == nil || *input.TargetDOF != 1 {
		t.Fatal("explicit user contract was not used", input, reason)
	}
	for _, kind := range []string{"axis", "plane", "offset"} {
		t.Run(kind, func(t *testing.T) {
			_, actual, source := assemblyContractFixture()
			switch kind {
			case "axis":
				actual.Constraints[1].DirectionRelation = "OPPOSITE"
			case "plane":
				actual.Constraints[2].DirectionRelation = "OPPOSITE"
			case "offset":
				actual.Constraints[2].Kind = "DISTANCE"
				actual.Constraints[2].DistanceRelation = "SELECTED_PLANE_NORMAL_V1"
				actual.Constraints[2].Value = 10
			}
			if _, why := assemblyGoalInput(r, actual, "actual-revision", source); why == "" {
				t.Fatal("model-selected actual values were allowed to redefine the user's target", kind)
			}
		})
	}
	// DOF remains the user's value even when a model-created arrangement would
	// normally leave just one rotational freedom. The native verifier must fail
	// a geometry/solver mismatch, rather than defaulting the target back to one.
	two := 2
	requirement.TargetDOF = &two
	input, reason = assemblyGoalInput(r, product, "actual-revision", requirement)
	if reason != "" || input.TargetDOF == nil || *input.TargetDOF != 2 {
		t.Fatal("explicit target DOF was replaced by an inferred value", input, reason)
	}
}

func TestAssistantAssemblyGoalRequiresExplicitUserAssemblyContract(t *testing.T) {
	r, _, _ := assemblyContractFixture()
	for _, missing := range []string{"axis", "plane", "offset", "dof", "invalid-offset"} {
		t.Run(missing, func(t *testing.T) {
			_, actual, source := assemblyContractFixture()
			switch missing {
			case "axis":
				source.AxisDirectionRelation = ""
			case "plane":
				source.PlaneDirectionRelation = ""
			case "offset":
				source.OffsetMM = nil
			case "dof":
				source.TargetDOF = nil
			case "invalid-offset":
				invalid := math.NaN()
				source.OffsetMM = &invalid
			}
			if _, why := assemblyGoalInput(r, actual, "actual-revision", source); why == "" {
				t.Fatal("missing source requirement was inferred from the resulting model", missing)
			}
		})
	}
}

func TestAssistantAssemblyGoalNormalizesDefinitionOrderBeforeCheckingUserOffset(t *testing.T) {
	r, _, _ := assemblyContractFixture()
	for _, direction := range []string{"SAME", "OPPOSITE"} {
		t.Run(direction, func(t *testing.T) {
			_, actual, source := assemblyContractFixture()
			desired := 5.0
			source.OffsetMM = &desired
			source.PlaneDirectionRelation = direction
			plane := &actual.Constraints[2]
			plane.Kind = "DISTANCE"
			plane.DistanceRelation = "SELECTED_PLANE_NORMAL_V1"
			plane.DirectionRelation = direction
			first, second := *plane.Second, plane.First
			plane.First = first
			plane.Second = &second
			plane.Value = 5
			if direction == "SAME" {
				plane.Value = -5
			}
			input, why := assemblyGoalInput(r, actual, "actual-revision", source)
			if why != "" || input.FirstPlane.InstanceID != "moving" || input.OffsetMM == nil || *input.OffsetMM != 5 {
				t.Fatal("definition order changed the source requirement's signed offset", input, why)
			}
			plane.DistanceRelation = "UNSIGNED"
			if _, why = assemblyGoalInput(r, actual, "actual-revision", source); why == "" {
				t.Fatal("unsigned nonzero distance was guessed to match a signed user goal")
			}
		})
	}
}

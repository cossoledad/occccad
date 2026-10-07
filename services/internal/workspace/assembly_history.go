package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"

	"github.com/occccad/occccad/internal/modelcore"
)

// History restores an accepted outcome, including unresolved definitions. A
// refresh is a new domain command, not a side effect of compensation.
func assemblyHistoryNeedsVerification(model ProductModel) bool {
	for _, constraint := range model.Constraints {
		if !constraint.Suppressed && constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			return false
		}
	}
	return len(model.Constraints) > 0
}

func (service *Service) verifyAssemblyHistory(ctx context.Context, documentID, revisionID, requestID string, snapshot ProductModel) error {
	if !assemblyHistoryNeedsVerification(snapshot) {
		return nil
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	var probe ProductModel
	if err := json.Unmarshal(raw, &probe); err != nil {
		return err
	}
	if err := service.solveAssembly(ctx, documentID, revisionID, requestID, "", nil, &probe, ""); err != nil {
		return err
	}
	for _, constraint := range probe.Constraints {
		if !constraint.Suppressed && constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			return fmt.Errorf("%w: ASSEMBLY_HISTORY_SUPPORT_CHANGED: %s", ErrValidation, constraint.ID)
		}
	}
	return validateAssemblyHistoryPoses(snapshot, probe)
}

// Display compensation may reuse the current accepted result only when the
// complete models match after removing each constraint's Visible property.
// Any support, pose, status, definition or reference change retains verification.
func constraintVisibilityOnlyHistory(before, after ProductModel, changes modelcore.ChangeSet) bool {
	if len(changes.Changes) == 0 || len(changes.ImpactSeeds) != 0 {
		return false
	}
	for _, change := range changes.Changes {
		if change.Target.SlotID != "assembly-constraint.entity" {
			return false
		}
	}
	before.Constraints = append([]AssemblyConstraint(nil), before.Constraints...)
	after.Constraints = append([]AssemblyConstraint(nil), after.Constraints...)
	for i := range before.Constraints {
		before.Constraints[i].Visible = nil
	}
	for i := range after.Constraints {
		after.Constraints[i].Visible = nil
	}
	return reflect.DeepEqual(before, after)
}

func validateAssemblyHistoryPoses(snapshot, evaluated ProductModel) error {
	if len(snapshot.Instances) != len(evaluated.Instances) {
		return fmt.Errorf("%w: assembly history instance set changed", ErrValidation)
	}
	poses := map[string]ProductInstance{}
	for _, instance := range evaluated.Instances {
		poses[instance.ID] = instance
	}
	for _, original := range snapshot.Instances {
		current, ok := poses[original.ID]
		if !ok {
			return fmt.Errorf("%w: assembly history instance disappeared", ErrValidation)
		}
		for i := range original.Translation {
			if math.Abs(original.Translation[i]-current.Translation[i]) > defaultAssemblySolverProfile().LengthTolerance {
				return fmt.Errorf("%w: ASSEMBLY_HISTORY_POSE_CHANGED", ErrValidation)
			}
		}
		a, b := normalizedInstanceRotation(original.Rotation), normalizedInstanceRotation(current.Rotation)
		same, opposite := 0.0, 0.0
		for i := range a {
			same += (a[i] - b[i]) * (a[i] - b[i])
			opposite += (a[i] + b[i]) * (a[i] + b[i])
		}
		if 2*math.Sqrt(math.Min(same, opposite)) > defaultAssemblySolverProfile().AngleTolerance {
			return fmt.Errorf("%w: ASSEMBLY_HISTORY_POSE_CHANGED", ErrValidation)
		}
	}
	return nil
}

// The desired value comes from the original command. The conflict precondition
// comes from the actual latest compensation outcome. This preserves strict
// field checks even if an evaluator normalized a past REVERT/REAPPLY Revision.
func historyChangeSetAgainstOutcome(documentType string, original modelcore.ChangeSet, outcome json.RawMessage, undo bool) (modelcore.ChangeSet, error) {
	expected, err := modelValues(documentType, outcome, original)
	if err != nil {
		return modelcore.ChangeSet{}, err
	}
	desired := map[modelcore.PropertyAddress]json.RawMessage{}
	for _, change := range original.Changes {
		if undo {
			desired[change.Target] = change.Before
		} else {
			desired[change.Target] = change.After
		}
	}
	if undo {
		return changesBetweenValues(desired, expected, original.ImpactSeeds)
	}
	return changesBetweenValues(expected, desired, original.ImpactSeeds)
}

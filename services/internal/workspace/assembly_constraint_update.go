package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

const (
	typeRefreshAssemblyConstraints = "occccad://product/assembly-constraint/refresh"
	typeUpdateAssemblyConstraints  = "occccad://product/assembly-constraint/update"
)

type assemblyConstraintUpdatePayload struct {
	ConstraintIDs []string `json:"constraintIds"`
}

// Only identities enter the domain command. Reference snapshots are resolved by
// the evaluator against this Product's accepted revisions, never source Heads.
func applyAssemblyConstraintUpdate(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload assemblyConstraintUpdatePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(payload.ConstraintIDs) == 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint identities required", ErrValidation)
	}
	wanted := map[string]bool{}
	for _, id := range payload.ConstraintIDs {
		if id == "" || wanted[id] {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: empty or duplicate constraint identity", ErrValidation)
		}
		wanted[id] = true
	}
	changes := modelcore.ChangeSet{}
	for _, c := range model.Constraints {
		if !wanted[c.ID] {
			continue
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, c, c)
		changes.Changes = append(changes.Changes, change)
		changes.ImpactSeeds = append(changes.ImpactSeeds, modelcore.DependencyKey("assembly-constraint:"+c.ID))
		delete(wanted, c.ID)
	}
	if len(wanted) != 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: assembly constraint does not exist", ErrValidation)
	}
	return modelJSON, changes, nil
}

func assemblyConstraintUpdateCommand(kind string) bool {
	return kind == typeRefreshAssemblyConstraints || kind == typeUpdateAssemblyConstraints
}

func (s *Service) evaluateAssemblyConstraintUpdate(ctx context.Context, documentID, revisionID, requestID string, command modelcore.DomainCommand, model *ProductModel) error {
	var payload assemblyConstraintUpdatePayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return err
	}
	wanted, excluded := map[string]bool{}, map[string]bool{}
	for _, id := range payload.ConstraintIDs {
		wanted[id] = true
	}
	for _, c := range model.Constraints {
		excluded[c.ID] = !wanted[c.ID]
	}
	if command.TypeURI == typeRefreshAssemblyConstraints {
		// CATIA Refresh Constraint repairs the support mask, not placement.
		// A repaired Broken constraint becomes NotUpdated until explicit Update.
		before, err := cloneAssemblyProduct(*model)
		if err != nil {
			return err
		}
		if err := s.resolveAssemblySupports(withAssemblyReadCache(ctx), model, excluded); err != nil {
			return err
		}
		for i := range model.Constraints {
			c := &model.Constraints[i]
			if !wanted[c.ID] {
				continue
			}
			previous := before.Constraints[i]
			if c.EvaluationStatus == modelcore.AssemblyConstraintNotUpdated && previous.EvaluationStatus != modelcore.AssemblyConstraintBroken && previous.EvaluationStatus != "" {
				// A link check cannot erase a conflict or certify that a previously
				// impossible definition has become numerically feasible.
				c.EvaluationStatus, c.EvaluationSummary = previous.EvaluationStatus, previous.EvaluationSummary
			} else if c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
				c.MeasuredValue, c.EvaluationFailure = nil, nil
			}
		}
		return nil
	}
	// Keep all accepted equations; retry only the selected pending definitions.
	// Admission still isolates conflicts and never commits failed trial poses.
	// Updating one definition has the same first/reference motion policy as
	// editing it. Dropping that intent changes both placement and convergence.
	intent := assemblyConstraintSolveIntent(command, *model)
	return s.solveAssemblyWithTargets(ctx, documentID, revisionID, requestID, "", intent, model, "", wanted)
}

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

type assemblySetEvaluator func(stage string, candidate *ProductModel, excluded map[string]bool) error

func cloneAssemblyProduct(model ProductModel) (ProductModel, error) {
	raw, err := json.Marshal(model)
	if err != nil {
		return ProductModel{}, err
	}
	var copy ProductModel
	err = json.Unmarshal(raw, &copy)
	return copy, err
}

func isolatedAssemblyFailure(err error) (*assemblySolveFailure, bool) {
	var failure *assemblySolveFailure
	ok := errors.As(err, &failure) && !failure.retryable
	return failure, ok
}

// Verified constraints are the accepted set. NotUpdated is a pending definition,
// not an equation during movement. Definition updates explicitly retry pending
// items in persisted insertion order; no user suppression flag is rewritten.
func evaluateAssemblyAdmission(model *ProductModel, moving bool, evaluate assemblySetEvaluator) error {
	working, err := cloneAssemblyProduct(*model)
	if err != nil {
		return err
	}
	original := working.Instances
	excluded := map[string]bool{}
	var pending []string
	for _, c := range working.Constraints {
		if !c.Suppressed && c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			excluded[c.ID] = true
			pending = append(pending, c.ID)
		}
	}
	excludeUnverified := func() {
		for _, c := range working.Constraints {
			if !c.Suppressed && c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
				excluded[c.ID] = true
			}
		}
	}
	run := func(stage string) (ProductModel, error) {
		candidate, err := cloneAssemblyProduct(working)
		if err != nil {
			return candidate, err
		}
		// Keep every trial's nominal pose equal to the command input. Admission must
		// not accumulate movement or redefine the motion preference between trials.
		for i := range candidate.Instances {
			candidate.Instances[i].Translation = original[i].Translation
			candidate.Instances[i].Rotation = original[i].Rotation
		}
		err = evaluate(stage, &candidate, excluded)
		return candidate, err
	}
	if !moving {
		hasAccepted := false
		for _, c := range working.Constraints {
			hasAccepted = hasAccepted || (!c.Suppressed && !excluded[c.ID])
		}
		baseline, baselineErr := working, error(nil)
		if hasAccepted {
			baseline, baselineErr = run("accepted")
		}
		if baselineErr != nil {
			if _, ok := isolatedAssemblyFailure(baselineErr); !ok {
				return baselineErr
			}
			// An upstream edit may invalidate the old accepted set. Rebuild in stable
			// creation order so a later conflicting definition cannot displace an earlier one.
			pending = nil
			for i := range working.Constraints {
				c := &working.Constraints[i]
				if c.Suppressed {
					continue
				}
				excluded[c.ID] = true
				pending = append(pending, c.ID)
				c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			}
		} else {
			working = baseline
			excludeUnverified()
		}
		for index, id := range pending {
			delete(excluded, id)
			candidate, trialErr := run(fmt.Sprintf("candidate-%d", index))
			if trialErr != nil {
				failure, ok := isolatedAssemblyFailure(trialErr)
				if !ok {
					return trialErr
				}
				excluded[id] = true
				for i := range working.Constraints {
					if working.Constraints[i].ID != id {
						continue
					}
					// Keep resolved supports/diagnostics from this attempt, never its poses.
					working.Constraints[i] = candidate.Constraints[i]
					working.Constraints[i].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
					working.Constraints[i].EvaluationSummary = "excluded from accepted solve set: " + failure.code + ": " + failure.diagnostic
					working.Constraints[i].MeasuredValue = nil
				}
				continue
			}
			working = candidate
			excludeUnverified()
		}
	}
	result, err := run("")
	if err != nil {
		return err
	}
	*model = result
	return nil
}

func (service *Service) solveAssembly(ctx context.Context, documentID, rootRevisionID, requestID, drivenInstanceID string, intent *geometry.AssemblySolveIntent, model *ProductModel, warmStartKey string, evidence ...*geometry.AssemblySolve) error {
	if len(model.Constraints) == 0 {
		return nil
	}
	ctx = withAssemblyReadCache(ctx)
	defer perf.Start(ctx, "assembly-total")()
	return evaluateAssemblyAdmission(model, drivenInstanceID != "", func(stage string, candidate *ProductModel, excluded map[string]bool) error {
		id, warm := requestID, warmStartKey
		targets := evidence
		if stage != "" {
			id += "/admission/" + stage
			warm = ""
			targets = nil
		}
		return service.solveAssemblySet(ctx, documentID, rootRevisionID, id, drivenInstanceID, intent, candidate, warm, excluded, stage != "", targets...)
	})
}

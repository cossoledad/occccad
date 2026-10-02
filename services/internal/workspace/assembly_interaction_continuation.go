package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
)

// Bounded internal continuation shares one request deadline and a total
// preference-iteration budget. Subtargets never receive candidate tokens and
// never change Session nominal, captured groups or the submitted final intent.
func (service *Service) solveInteractionTrajectory(ctx context.Context, requestID string, frozen AssemblySolveManifest) (result geometry.AssemblySolve, err error) {
	result, err = service.solveFrozenManifest(ctx, requestID, frozen, nil)
	if err != nil || result.Interaction == nil || result.Interaction.Status != "BUDGET" {
		return result, err
	}
	limit := uint64(100)
	if frozen.SolverProfile.MaxPreferenceIterations != nil {
		limit = *frozen.SolverProfile.MaxPreferenceIterations
	}
	remaining := 3 * limit
	if result.Interaction.Iterations >= remaining {
		return result, nil
	}
	remaining -= result.Interaction.Iterations
	totalIterations, totalRestorations := result.Interaction.Iterations, result.Interaction.Restorations
	var continuationSteps uint64
	defer func() {
		if result.Interaction != nil {
			result.Interaction.Iterations = totalIterations
			result.Interaction.Restorations = totalRestorations
			result.Interaction.ContinuationSteps = continuationSteps
		}
	}()
	desired := *frozen.DragTarget
	start := desired.TargetPose
	for _, body := range frozen.Bodies {
		if body.ID == desired.BodyID {
			start = body.Pose
			if body.InitialGuess != nil {
				start = *body.InitialGuess
			}
		}
	}
	// A fixed deterministic bridge is bounded to four probes. Only qualified
	// states transport warm/branch; UNKNOWN is never accepted as a checkpoint.
	for attempt, fraction := range []float64{0.25, 0.5, 0.75, 1} {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if remaining < 3 {
			break
		}
		target := desired
		for i := 0; i < 3; i++ {
			target.TargetPose.Translation[i] = start.Translation[i] + fraction*(desired.TargetPose.Translation[i]-start.Translation[i])
		}
		dot := 0.0
		for i := 0; i < 4; i++ {
			dot += start.Rotation[i] * desired.TargetPose.Rotation[i]
		}
		sign := 1.0
		if dot < 0 {
			sign = -1
		}
		norm := 0.0
		for i := 0; i < 4; i++ {
			target.TargetPose.Rotation[i] = (1-fraction)*start.Rotation[i] + fraction*sign*desired.TargetPose.Rotation[i]
			norm += target.TargetPose.Rotation[i] * target.TargetPose.Rotation[i]
		}
		for i := 0; i < 4; i++ {
			target.TargetPose.Rotation[i] /= math.Sqrt(norm)
		}
		cap := remaining / 3
		if cap > limit {
			cap = limit
		}
		frozen.SolverProfile.MaxPreferenceIterations = &cap
		frozen.DragTarget = &target
		frozen.Digest = assemblyManifestDigest(frozen)
		next, e := service.solveFrozenManifest(ctx, fmt.Sprintf("%s-continuation-%d", requestID, attempt), frozen, nil)
		if e != nil {
			return result, e
		}
		if next.Interaction == nil {
			break
		}
		continuationSteps++
		totalIterations += next.Interaction.Iterations
		totalRestorations += next.Interaction.Restorations
		used := next.Interaction.Iterations
		if used == 0 {
			used = 1
		}
		if used >= remaining {
			remaining = 0
		} else {
			remaining -= used
		}
		if fraction == 1 {
			result = next
		}
		if !next.Interaction.HardFeasible || !next.Interaction.TargetConverged || !next.Interaction.EligibleForCommit {
			continue
		}
		byID := map[string]geometry.AssemblyPose{}
		for _, b := range next.Bodies {
			byID[b.ID] = b.Pose
		}
		for i := range frozen.Bodies {
			if p, ok := byID[frozen.Bodies[i].ID]; ok {
				copy := p
				frozen.Bodies[i].InitialGuess = &copy
			}
		}
		for _, branch := range next.AngleBranches {
			for i := range frozen.Constraints {
				if frozen.Constraints[i].ID == branch.ConstraintID {
					state := branch.State
					frozen.Constraints[i].AngleBranchState = &state
				}
			}
		}
	}
	return result, nil
}

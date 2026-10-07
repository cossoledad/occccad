package geometry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const AssemblyReplaySchema = "occccad.3dreplay.v1"

// AssemblyReplay contains only the exact numerical RPC input and solve evidence.
// Geometry is already resolved in body-local coordinates; no database or B-Rep is needed to replay.
type AssemblyReplay struct {
	Schema         string                `json:"schema"`
	Units          string                `json:"units"`
	Request        json.RawMessage       `json:"request,omitempty"`
	Result         json.RawMessage       `json:"result,omitempty"`
	TransportError string                `json:"transportError,omitempty"`
	Snapshot       json.RawMessage       `json:"snapshot,omitempty"`
	AssemblyResult json.RawMessage       `json:"assemblyResult,omitempty"`
	Outcome        string                `json:"outcome,omitempty"`
	Budget         *AssemblyReplayBudget `json:"budget,omitempty"`
	Derivation     json.RawMessage       `json:"derivation,omitempty"`
	Stages         []json.RawMessage     `json:"stages,omitempty"`
	Missing        []string              `json:"missing,omitempty"`
}

type AssemblyReplayBudget struct {
	WallClockMS float64 `json:"wallClockMs"`
	ElapsedMS   float64 `json:"elapsedMs"`
}

func ReplayBudget(ctx context.Context, start time.Time) *AssemblyReplayBudget {
	budget := &AssemblyReplayBudget{ElapsedMS: float64(time.Since(start)) / float64(time.Millisecond)}
	if deadline, ok := ctx.Deadline(); ok {
		budget.WallClockMS = float64(deadline.Sub(start)) / float64(time.Millisecond)
	}
	return budget
}
func ClassifyAssemblyReplay(result *workerv1.SolveAssemblyResponse, solveErr error) string {
	if solveErr != nil {
		return "EXECUTION_FAILURE"
	}
	if result == nil {
		return "NOT_ATTEMPTED"
	}
	switch result.Status {
	case "CONVERGED":
		for _, d := range result.Diagnostics {
			if d.Code == "PREFERENCE_NOT_CONVERGED" {
				return "FEASIBLE_PREFERENCE_NOT_CONVERGED"
			}
		}
		return "FEASIBLE"
	case "INVALID_MODEL":
		return "INPUT_ERROR"
	case "INCONSISTENT":
		for _, d := range result.Diagnostics {
			if d.Code == "GROUNDED_CONTRADICTION" {
				return "PROVEN_CONTRADICTION"
			}
		}
		return "NUMERICAL_NONCONVERGENCE"
	case "UNSATISFIED", "MAX_ITERATIONS", "NUMERICAL_FAILURE":
		return "NUMERICAL_NONCONVERGENCE"
	default:
		return "EXECUTION_FAILURE"
	}
}

func makeAssemblyReplay(request *workerv1.SolveAssemblyRequest, result *workerv1.SolveAssemblyResponse, solveErr error) ([]byte, error) {
	return makeAssemblyReplayBudget(request, result, solveErr, nil)
}
func makeAssemblyReplayBudget(request *workerv1.SolveAssemblyRequest, result *workerv1.SolveAssemblyResponse, solveErr error, budget *AssemblyReplayBudget) ([]byte, error) {
	// Borrow immutable messages during synchronous serialization; never mutate input.
	effective := &workerv1.SolveAssemblyRequest{RequestId: request.RequestId, Bodies: request.Bodies, Geometry: request.Geometry, Constraints: request.Constraints, LengthScale: request.LengthScale, AngleScale: request.AngleScale, AffectedBodyIds: request.AffectedBodyIds, SolveIntent: request.SolveIntent, SolverProfile: request.SolverProfile, DragTarget: request.DragTarget, DisableConflictProbes: request.DisableConflictProbes, DebugOptions: request.DebugOptions}
	if result != nil && result.EffectiveSolverProfile != nil {
		effective.SolverProfile = result.EffectiveSolverProfile
	}

	if result != nil && result.EffectiveDebugOptions != nil {
		effective.DebugOptions = result.EffectiveDebugOptions
	}
	input, err := protojson.Marshal(effective)
	if err != nil {
		return nil, err
	}
	replay := AssemblyReplay{Schema: AssemblyReplaySchema, Units: "mm,rad; body-local geometry; quaternion xyzw", Request: input, Outcome: ClassifyAssemblyReplay(result, solveErr), Budget: budget}
	if result != nil {
		compact := &workerv1.SolveAssemblyResponse{Status: result.Status, Bodies: result.Bodies, Residuals: result.Residuals, Iterations: result.Iterations, NormalizedResidual: result.NormalizedResidual, Diagnostic: result.Diagnostic, Classification: result.Classification, RedundantConstraintIds: result.RedundantConstraintIds, ConflictingConstraintIds: result.ConflictingConstraintIds, Diagnostics: result.Diagnostics, UnsatisfiedConstraintIds: result.UnsatisfiedConstraintIds, ConstraintRanks: result.ConstraintRanks, AngleBranches: result.AngleBranches, SuspectedConflictingConstraintIds: result.SuspectedConflictingConstraintIds, SolverBuild: result.SolverBuild, Interaction: result.Interaction, AlignmentBranches: result.AlignmentBranches, DistanceBranches: result.DistanceBranches, Metrics: result.Metrics, EvaluationTrace: result.EvaluationTrace, TraceTruncated: result.TraceTruncated, ImplementationId: result.ImplementationId}
		if request.GetDebugOptions().GetCaptureMatrices() {
			compact.EquationResiduals = result.EquationResiduals
		}
		for _, c := range result.Components {
			component := &workerv1.AssemblyComponentDof{ComponentId: c.ComponentId, BodyIds: c.BodyIds, TangentVariableCount: c.TangentVariableCount, JacobianRank: c.JacobianRank, RelativeDof: c.RelativeDof, GaugeDof: c.GaugeDof, Solved: c.Solved, TangentClusterIds: c.TangentClusterIds, SingularValues: c.SingularValues, RankThreshold: c.RankThreshold, Preference: c.Preference}
			if request.GetDebugOptions().GetCaptureMatrices() {
				component.NullSpaceBasis = c.NullSpaceBasis
				component.Freedoms = c.Freedoms
			}
			compact.Components = append(compact.Components, component)
		}
		replay.Result, err = protojson.Marshal(compact)
		if err != nil {
			return nil, err
		}
	}
	if solveErr != nil {
		replay.TransportError = solveErr.Error()
		if result == nil {
			replay.Missing = append(replay.Missing, "NUMERICAL_RESULT_UNAVAILABLE")
		}
	}
	if result == nil || result.ImplementationId == "" {
		replay.Missing = append(replay.Missing, "IMPLEMENTATION_ID_UNAVAILABLE")
	}
	if result != nil && result.TraceTruncated {
		replay.Missing = append(replay.Missing, "EVALUATION_TRACE_BYTE_OR_FRAME_BUDGET_EXCEEDED")
	}
	if !request.GetDebugOptions().GetCaptureMatrices() {
		replay.Missing = append(replay.Missing, "FULL_MATRICES_NOT_CAPTURED")
	}
	if !request.GetDebugOptions().GetCaptureEvaluations() {
		replay.Missing = append(replay.Missing, "EVALUATION_TRACE_NOT_CAPTURED")
	}
	data, err := json.Marshal(replay)
	if err == nil && len(data) > 8<<20 {
		return nil, fmt.Errorf("numerical replay exceeds 8 MiB capture budget")
	}
	return data, err
}

// MakeAssemblyReplay freezes the exact production RPC encoding without a solve.
func MakeAssemblyReplay(requestID string, bodies []AssemblyBody, values []AssemblyGeometry, constraints []AssemblyConstraint, options AssemblySolveOptions) ([]byte, error) {
	request, err := CompileAssemblyRequest(requestID, bodies, values, constraints, options)
	if err != nil {
		return nil, err
	}
	return makeAssemblyReplay(request, nil, nil)
}

// ReplayAssembly uses the same public RPC and validation as an ordinary solve.
func (client *Client) ReplayAssembly(ctx context.Context, data []byte) ([]byte, error) {
	var replay AssemblyReplay
	if err := json.Unmarshal(data, &replay); err != nil {
		return nil, err
	}
	if replay.Schema != AssemblyReplaySchema {
		return nil, fmt.Errorf("unsupported 3dreplay schema %q", replay.Schema)
	}
	var input workerv1.SolveAssemblyRequest
	if err := protojson.Unmarshal(replay.Request, &input); err != nil {
		return nil, err
	}
	if replay.Budget != nil && replay.Budget.WallClockMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(replay.Budget.WallClockMS*float64(time.Millisecond)))
		defer cancel()
	}
	start := time.Now()
	result, solveErr := client.worker.SolveAssembly(ctx, &input)
	output, err := makeAssemblyReplay(&input, result, solveErr)
	if err != nil {
		return nil, err
	}
	var updated AssemblyReplay
	if err = json.Unmarshal(output, &updated); err != nil {
		return nil, err
	}
	updated.Snapshot = replay.Snapshot
	updated.Budget = ReplayBudget(ctx, start)
	return json.Marshal(updated)
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"google.golang.org/protobuf/encoding/protojson"
)

type AssemblyReplayOptions struct {
	Trace                   bool     `json:"trace,omitempty"`
	FullMatrices            bool     `json:"fullMatrices,omitempty"`
	TraceByteBudget         uint64   `json:"traceByteBudget,omitempty"`
	Mode                    string   `json:"mode"` // accepted, accepted-pending, all-driving, subsystem, original
	ConstraintIDs           []string `json:"constraintIds,omitempty"`
	TargetConstraintID      string   `json:"targetConstraintId,omitempty"`
	DiagnosticID            string   `json:"diagnosticId,omitempty"`
	MaxIterations           *uint64  `json:"maxIterations,omitempty"`
	MaxPreferenceIterations *uint64  `json:"maxPreferenceIterations,omitempty"`
	WallClockMS             *float64 `json:"wallClockMs,omitempty"`
	// A recorded numerical-input patch, restricted to warm starts and branch intent.
	InitialGuesses map[string]geometry.AssemblyPose             `json:"initialGuesses,omitempty"`
	AngleBranches  map[string]geometry.AssemblyAngleBranchState `json:"angleBranches,omitempty"`
}

// ReplayAssemblyDiagnostic preserves the original API's explicit pending opt-in.
func ReplayAssemblyDiagnostic(ctx context.Context, client *geometry.Client, data []byte, includeUnverified bool) ([]byte, error) {
	options := AssemblyReplayOptions{Mode: "accepted"}
	if includeUnverified {
		options.Mode = "accepted-pending"
	}
	return ReplayAssemblyDiagnosticWithOptions(ctx, client, data, options)
}
func replayInputError(file geometry.AssemblyReplay, message string) ([]byte, error) {
	file.Outcome = "INPUT_ERROR"
	file.TransportError = ""
	var err error
	file.AssemblyResult, err = json.Marshal(geometry.AssemblySolve{Status: "INVALID_MODEL", Diagnostic: message})
	if err != nil {
		return nil, err
	}
	return json.Marshal(file)
}
func applyReplayOverrides(manifest *AssemblySolveManifest, options AssemblyReplayOptions) error {
	if options.MaxIterations != nil {
		manifest.SolverProfile.MaxIterations = *options.MaxIterations
	}
	if options.MaxPreferenceIterations != nil {
		x := *options.MaxPreferenceIterations
		manifest.SolverProfile.MaxPreferenceIterations = &x
	}
	bodies := map[string]bool{}
	for i := range manifest.Bodies {
		b := &manifest.Bodies[i]
		bodies[b.ID] = true
		if p, ok := options.InitialGuesses[b.ID]; ok {
			p := p
			b.InitialGuess = &p
		}
	}
	for id := range options.InitialGuesses {
		if !bodies[id] {
			return fmt.Errorf("unknown initial guess body %s", id)
		}
	}
	cs := map[string]bool{}
	for i := range manifest.Constraints {
		c := &manifest.Constraints[i]
		cs[c.ID] = true
		if p, ok := options.AngleBranches[c.ID]; ok {
			if c.Kind != "ANGLE" {
				return fmt.Errorf("branch override requires angle constraint %s", c.ID)
			}
			p := p
			c.AngleBranchState = &p
		}
	}
	for id := range options.AngleBranches {
		if !cs[id] {
			return fmt.Errorf("unknown angle constraint %s", id)
		}
	}
	return nil
}
func selectDiagnosticConstraints(manifest *AssemblySolveManifest, options AssemblyReplayOptions) (map[string]bool, error) {
	definitions := map[string]AssemblyConstraint{}
	for _, d := range manifest.Definitions {
		definitions[d.ID] = d
	}
	pending := stringSet(options.ConstraintIDs)
	for id := range pending {
		d, ok := definitions[id]
		if !ok || d.EvaluationStatus != "NOT_UPDATED" || d.Suppressed {
			return nil, fmt.Errorf("pending selection must name an unsuppressed NotUpdated constraint: %s", id)
		}
	}
	selected := map[string]bool{}
	for _, d := range manifest.Definitions {
		if d.Suppressed {
			continue
		}
		switch options.Mode {
		case "accepted":
			selected[d.ID] = d.EvaluationStatus == "VERIFIED"
		case "accepted-pending":
			selected[d.ID] = d.EvaluationStatus == "VERIFIED" || (d.EvaluationStatus == "NOT_UPDATED" && (len(pending) == 0 || pending[d.ID]))
		case "all-driving", "subsystem":
			selected[d.ID] = d.Mode == "" || d.Mode == "DRIVING"
		default:
			return nil, fmt.Errorf("unsupported assembly replay mode %q", options.Mode)
		}
	}
	if options.Mode == "subsystem" {
		target, ok := definitions[options.TargetConstraintID]
		if !ok || !selected[target.ID] {
			return nil, fmt.Errorf("subsystem requires an unsuppressed driving target constraint")
		}
		// Close over the whole connected subsystem, including third axes and group members.
		// Explicit ground conditions travel with their bodies; no synthetic Fix is introduced.
		bodies := map[string]bool{}
		targets := map[string]bool{target.ID: true}
		groupModel := ProductModel{Constraints: manifest.Definitions}
		for _, b := range manifest.Bodies {
			groupModel.Instances = append(groupModel.Instances, ProductInstance{ID: b.ID})
		}
		groupUnits, err := assemblyGroupMembers(groupModel)
		if err != nil {
			return nil, err
		}
		constraintBodies := func(d AssemblyConstraint) []string {
			ids := []string{d.First.InstanceID}
			if d.Second != nil {
				ids = append(ids, d.Second.InstanceID)
			}
			if d.AngleAxis != nil {
				ids = append(ids, d.AngleAxis.InstanceID)
			}
			if isAssemblyGroup(d) {
				ids = append(ids, groupUnits[d.ID]...)
			}
			return ids
		}
		for _, id := range constraintBodies(target) {
			bodies[id] = true
		}
		changed := true
		for changed {
			changed = false
			for _, d := range manifest.Definitions {
				if !selected[d.ID] {
					continue
				}
				ids := constraintBodies(d)
				connected := targets[d.ID]
				for _, id := range ids {
					connected = connected || bodies[id]
				}
				if !connected {
					continue
				}
				if !targets[d.ID] {
					targets[d.ID] = true
					changed = true
				}
				for _, id := range ids {
					if !bodies[id] {
						bodies[id] = true
						changed = true
					}
				}
			}
		}
		selected = targets
		filtered := manifest.Bodies[:0]
		for _, b := range manifest.Bodies {
			if bodies[b.ID] {
				filtered = append(filtered, b)
			}
		}
		manifest.Bodies = filtered
		geometryValues := manifest.Geometry[:0]
		for _, g := range manifest.Geometry {
			if bodies[g.BodyID] {
				geometryValues = append(geometryValues, g)
			}
		}
		manifest.Geometry = geometryValues
		filteredCS := manifest.Constraints[:0]
		for _, c := range manifest.Constraints {
			if selected[c.ID] {
				filteredCS = append(filteredCS, c)
			}
		}
		manifest.Constraints = filteredCS
		if manifest.Intent != nil {
			intent := *manifest.Intent
			intent.MovingBodyIDs = nil
			intent.ReferenceBodyIDs = nil
			for _, id := range manifest.Intent.MovingBodyIDs {
				if bodies[id] {
					intent.MovingBodyIDs = append(intent.MovingBodyIDs, id)
				}
			}
			for _, id := range manifest.Intent.ReferenceBodyIDs {
				if bodies[id] {
					intent.ReferenceBodyIDs = append(intent.ReferenceBodyIDs, id)
				}
			}
			manifest.Intent = &intent
		}
		needed := map[string]bool{}
		var includeGroup func(string)
		includeGroup = func(id string) {
			if needed[id] {
				return
			}
			needed[id] = true
			for _, m := range definitions[id].GroupMembers {
				if m.GroupID != "" {
					includeGroup(m.GroupID)
				}
			}
		}
		for id, on := range selected {
			if on {
				includeGroup(id)
			}
		}
		filteredDefs := []AssemblyConstraint{}
		for _, d := range manifest.Definitions {
			if needed[d.ID] {
				filteredDefs = append(filteredDefs, d)
			}
		}
		manifest.Definitions = filteredDefs
		affected := []string{}
		for _, id := range manifest.AffectedBodyIDs {
			if bodies[id] {
				affected = append(affected, id)
			}
		}
		manifest.AffectedBodyIDs = affected
	}
	excluded := map[string]bool{}
	for _, d := range manifest.Definitions {
		excluded[d.ID] = !selected[d.ID]
	}
	for i := range manifest.Constraints {
		c := &manifest.Constraints[i]
		d := definitions[c.ID]
		c.Mode = d.Mode
		if c.Mode == "" {
			c.Mode = "DRIVING"
		}
		if excluded[c.ID] {
			c.Mode = "SUPPRESSED"
		}
	}
	participating := make([]geometry.AssemblyConstraint, 0, len(manifest.Constraints))
	for _, c := range manifest.Constraints {
		if selected[c.ID] {
			participating = append(participating, c)
		}
	}
	manifest.Constraints = participating

	var err error
	manifest.GroupStages, err = prepareAssemblyGroupStages(*manifest, excluded)
	return selected, err
}

// Derived files retain an immutable <=8 MiB snapshot, <=4 MiB sequence and
// the last <=8 MiB numerical result. Their reader/writer has a separate bound.
const AssemblyReplayFileBudget = 24 << 20

func marshalAssemblyReplay(file geometry.AssemblyReplay) ([]byte, error) {
	data, err := json.Marshal(file)
	if err == nil && len(data) > AssemblyReplayFileBudget {
		return nil, fmt.Errorf("derived replay exceeds byte budget")
	}
	return data, err
}

func ReplayAssemblyDiagnosticWithOptions(ctx context.Context, client *geometry.Client, data []byte, options AssemblyReplayOptions) ([]byte, error) {
	if len(data) > AssemblyReplayFileBudget {
		return nil, fmt.Errorf("3dreplay input exceeds byte budget")
	}
	if options.Mode == "" {
		options.Mode = "accepted"
	}
	if options.WallClockMS != nil {
		if !finite(*options.WallClockMS) || *options.WallClockMS <= 0 || *options.WallClockMS > 600000 {
			return nil, fmt.Errorf("wall clock budget must be in (0,600000] ms")
		}
		ctx = geometry.WithAssemblyReplayBudget(ctx, time.Duration(*options.WallClockMS*float64(time.Millisecond)))
	}
	var file geometry.AssemblyReplay
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Schema != geometry.AssemblyReplaySchema {
		return nil, fmt.Errorf("unsupported assembly replay schema")
	}
	file.Outcome = ""
	file.Result = nil
	file.TransportError = ""
	file.AssemblyResult = nil
	file.Stages = nil
	file.Missing = nil
	var err error
	file.Derivation, err = json.Marshal(options)
	if err != nil {
		return nil, err
	}
	if len(file.Snapshot) == 0 {
		if options.Mode != "original" && options.Mode != "accepted" {
			return nil, fmt.Errorf("selection modes require a complete diagnostic snapshot")
		}
		return replayOriginalFrame(ctx, client, file, options)
	}
	if len(file.Request) != 0 {
		return nil, fmt.Errorf("diagnostic snapshot has a second numerical authority")
	}
	var snapshot AssemblyDiagnosticSnapshot
	if err := json.Unmarshal(file.Snapshot, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.Schema != assemblyDiagnosticSchema || snapshot.Digest == "" || snapshot.Digest != snapshot.contentDigest() {
		return nil, fmt.Errorf("invalid assembly diagnostic schema or content digest")
	}
	if err := snapshot.validateReferences(); err != nil {
		return nil, err
	}
	// Validate every reference, including original stages that are not selected.
	if _, err := snapshot.ExpandFrame(snapshot.Current); err != nil {
		return nil, err
	}
	for _, attempt := range snapshot.Attempts {
		for _, frame := range attempt.Sequence {
			if _, err := snapshot.ExpandFrame(frame); err != nil {
				return nil, err
			}
		}
	}
	if options.Mode == "original" {
		matches := []AssemblyDiagnosticAttempt{}
		for _, attempt := range snapshot.Attempts {
			if (options.DiagnosticID != "" && attempt.DiagnosticID == options.DiagnosticID) || (options.DiagnosticID == "" && options.TargetConstraintID != "" && stringSet(attempt.ConstraintIDs)[options.TargetConstraintID]) {
				matches = append(matches, attempt)
			}
		}
		if len(matches) != 1 {
			return replayInputError(file, "ORIGINAL_ATTEMPT_UNAVAILABLE: select one diagnostic ID or NotUpdated constraint")
		}
		attempt := matches[0]
		if len(attempt.Missing) > 0 || len(attempt.Sequence) == 0 {
			file.Missing = attempt.Missing
			return replayInputError(file, "ORIGINAL_ATTEMPT_INCOMPLETE; current state is not an original failure replay")
		}
		stageBytes := 0
		stageTruncated := false
		for _, frame := range attempt.Sequence {
			numeric, err := snapshot.ExpandFrame(frame)
			if err != nil {
				return nil, err
			}
			output, err := replayOriginalFrame(ctx, client, numeric, options)
			if err != nil {
				return nil, err
			}
			if stageBytes+len(output) <= assemblyAttemptBudget && len(file.Stages) < assemblyAttemptLimit {
				file.Stages = append(file.Stages, json.RawMessage(output))
				stageBytes += len(output)
			} else if !stageTruncated {
				file.Missing = append(file.Missing, "REPLAY_SEQUENCE_BYTE_OR_FRAME_BUDGET_EXCEEDED")
				stageTruncated = true
			}
			var result geometry.AssemblyReplay
			if err = json.Unmarshal(output, &result); err != nil {
				return nil, err
			}
			file.Result = result.Result
			file.TransportError = result.TransportError
			file.Outcome = result.Outcome
			if result.TransportError != "" {
				break
			}
		}
		return marshalAssemblyReplay(file)
	}
	manifest, err := diagnosticManifest(snapshot)
	if err != nil {
		return replayInputError(file, err.Error())
	}
	selected, err := selectDiagnosticConstraints(&manifest, options)
	if err != nil {
		return replayInputError(file, err.Error())
	}
	compiled := map[string]bool{}
	for _, c := range manifest.Constraints {
		compiled[c.ID] = true
	}
	for _, d := range manifest.Definitions {
		if selected[d.ID] && !isAssemblyGroup(d) && !compiled[d.ID] {
			return replayInputError(file, "CONSTRAINT_NOT_COMPILED: "+d.ID)
		}
	}
	for _, failure := range snapshot.CompileFailures {
		if failure.ConstraintID == "" || selected[failure.ConstraintID] {
			return replayInputError(file, "COMPILE_FAILURE: "+failure.ConstraintID+"/"+failure.Endpoint+"/"+failure.Phase+": "+failure.Error)
		}
	}
	if err = applyReplayOverrides(&manifest, options); err != nil {
		return replayInputError(file, err.Error())
	}
	manifest.Digest = assemblyManifestDigest(manifest)
	if options.Trace || options.FullMatrices {
		ctx = geometry.WithAssemblyDebug(ctx, geometry.AssemblyDebugOptions{CaptureEvaluations: options.Trace, CaptureMatrices: options.FullMatrices, ByteBudget: options.TraceByteBudget})
	}
	stageBytes := 0
	stageTruncated := false
	result, solveErr := New(nil, client).solveFrozenManifest(ctx, "diagnostic/replay", manifest, func(data []byte, err error) {
		if err != nil {
			file.Missing = append(file.Missing, "NUMERICAL_CAPTURE_FAILED: "+err.Error())
			return
		}
		var last geometry.AssemblyReplay
		if err := json.Unmarshal(data, &last); err != nil {
			file.Missing = append(file.Missing, "NUMERICAL_CAPTURE_UNREADABLE")
			return
		}
		file.Result = last.Result
		file.TransportError = last.TransportError
		file.Outcome = last.Outcome
		if stageBytes+len(data) > assemblyAttemptBudget || len(file.Stages) >= assemblyAttemptLimit {
			if !stageTruncated {
				file.Missing = append(file.Missing, "REPLAY_SEQUENCE_BYTE_OR_FRAME_BUDGET_EXCEEDED")
				stageTruncated = true
			}
			return
		}
		stageBytes += len(data)
		file.Stages = append(file.Stages, json.RawMessage(data))
	})

	if solveErr != nil {
		file.TransportError = solveErr.Error()
		file.Outcome = "EXECUTION_FAILURE"
	}
	if result.Diagnostic == "GROUP_INTERNAL_UNRESOLVED" {
		file.Outcome = "INPUT_ERROR"
	}
	if file.Outcome == "" {
		if result.Status == "CONVERGED" {
			file.Outcome = "FEASIBLE"
		} else if result.Status == "INVALID_MODEL" {
			file.Outcome = "INPUT_ERROR"
		} else {
			file.Outcome = "NUMERICAL_NONCONVERGENCE"
		}
	}
	if result.Status == "CONVERGED" {
		if err := validateAssemblyPreference(result); err != nil {
			file.Outcome = "FEASIBLE_PREFERENCE_NOT_CONVERGED"
		}
	}
	// The domain result adds group orchestration evidence; numerical results
	// (candidate poses, residuals and ranks) have one source in the RPC result.
	result.Bodies = nil
	result.Components = nil
	result.EquationResiduals = nil
	result.ConstraintRanks = nil
	result.AngleBranches = nil
	result.Diagnostics = nil
	file.AssemblyResult, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return marshalAssemblyReplay(file)
}
func replayOriginalFrame(ctx context.Context, client *geometry.Client, file geometry.AssemblyReplay, options AssemblyReplayOptions) ([]byte, error) {
	var request workerv1.SolveAssemblyRequest
	if err := protojson.Unmarshal(file.Request, &request); err != nil {
		return nil, err
	}
	if options.MaxIterations != nil || options.MaxPreferenceIterations != nil || len(options.InitialGuesses) > 0 || len(options.AngleBranches) > 0 {
		bodies, values, cs, opts, err := geometry.AssemblyInputFromRequest(&request)
		if err != nil {
			return replayInputError(file, err.Error())
		}
		if opts.SolverProfile == nil {
			return replayInputError(file, "recorded effective profile is required for overrides")
		}
		manifest := AssemblySolveManifest{Bodies: bodies, Geometry: values, Constraints: cs, SolverProfile: *opts.SolverProfile}
		if err := applyReplayOverrides(&manifest, options); err != nil {
			return replayInputError(file, err.Error())
		}
		opts.SolverProfile = &manifest.SolverProfile
		requestPtr, err := geometry.CompileAssemblyRequest(request.RequestId, manifest.Bodies, manifest.Geometry, manifest.Constraints, opts)
		if err != nil {
			return replayInputError(file, err.Error())
		}
		file.Request, err = protojson.Marshal(requestPtr)
		if err != nil {
			return nil, err
		}
	}
	if options.WallClockMS != nil {
		file.Budget = &geometry.AssemblyReplayBudget{WallClockMS: *options.WallClockMS}
	}
	if options.Trace || options.FullMatrices {
		var input workerv1.SolveAssemblyRequest
		if err := protojson.Unmarshal(file.Request, &input); err != nil {
			return nil, err
		}
		input.DebugOptions = &workerv1.AssemblyDebugOptions{CaptureEvaluations: options.Trace, CaptureMatrices: options.FullMatrices, ByteBudget: options.TraceByteBudget}
		raw, err := protojson.Marshal(&input)
		if err != nil {
			return nil, err
		}
		file.Request = raw
	}
	raw, err := json.Marshal(file)
	if err != nil {
		return nil, err
	}
	output, err := client.ReplayAssembly(ctx, raw)
	if err != nil {
		return nil, err
	}
	var result geometry.AssemblyReplay
	if err = json.Unmarshal(output, &result); err != nil {
		return nil, err
	}
	result.Derivation = file.Derivation
	return marshalAssemblyReplay(result)
}

package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The capture is deliberately before persistence and numerical work. It is not
// admission: every resolvable active definition, including isolated definitions,
// is compiled using the exact production compiler.
type assemblyFrozenInputKey struct{}

func (service *Service) FreezeAssemblyInput(ctx context.Context, documentID, revisionID string, model ProductModel) (AssemblySolveManifest, error) {
	copy, err := cloneAssemblyProduct(model)
	if err != nil {
		return AssemblySolveManifest{}, err
	}
	raw, err := json.Marshal(model)
	if err != nil {
		return AssemblySolveManifest{}, err
	}
	var frozen AssemblySolveManifest
	ctx = context.WithValue(withAssemblyReadCache(ctx), assemblyFrozenInputKey{}, &frozen)
	err = service.solveAssemblySet(ctx, documentID, revisionID, "diagnostic/compile", "", nil, &copy, "", nil, true)
	if err != nil {
		return frozen, err
	}
	if frozen.SchemaVersion == 0 {
		bodies := []geometry.AssemblyBody{}
		for _, i := range copy.Instances {
			bodies = append(bodies, geometry.AssemblyBody{ID: i.ID, Pose: geometry.AssemblyPose{Translation: i.Translation, Rotation: normalizedInstanceRotation(i.Rotation)}})
		}
		if len(bodies) == 0 {
			return frozen, fmt.Errorf("%w: Product has no assembly motion units", ErrValidation)
		}
		frozen, err = newAssemblySolveManifest(documentID, revisionID, canonicalModelHash(raw), bodies, nil, nil, nil, nil, nil, copy.Constraints)
	}
	// No production warm start is used by read-only diagnostics or Session open.
	for i := range frozen.Bodies {
		frozen.Bodies[i].InitialGuess = nil
	}
	frozen.Purpose = "DIAGNOSTIC"
	frozen.Digest = assemblyManifestDigest(frozen)
	return frozen, err
}

type AssemblyConflictRequest struct {
	BaseRevisionID      string   `json:"baseRevisionId"`
	TargetConstraintIDs []string `json:"targetConstraintIds,omitempty"`
	MaxProbes           int      `json:"maxProbes,omitempty"`
	TimeBudgetMS        int      `json:"timeBudgetMs,omitempty"`
}

type AssemblyConflictMember struct {
	ConstraintID  string                `json:"constraintId"`
	GroupID       string                `json:"groupId,omitempty"`
	EquationIDs   []string              `json:"equationIds,omitempty"`
	First         AssemblyGeometryRef   `json:"first"`
	Second        *AssemblyGeometryRef  `json:"second,omitempty"`
	AngleAxis     *AssemblyGeometryRef  `json:"angleAxis,omitempty"`
	GroupMembers  []AssemblyGroupMember `json:"groupMembers,omitempty"`
	RepairActions []string              `json:"repairActions"`
}

type AssemblyConflictItem struct {
	Kind          string                   `json:"kind"`
	Evidence      string                   `json:"evidence"`
	Oracle        string                   `json:"oracle"`
	ConstraintIDs []string                 `json:"constraintIds"`
	Reason        string                   `json:"reason"`
	Members       []AssemblyConflictMember `json:"members"`
	Irreducible   bool                     `json:"irreducible"`
}

type AssemblyConflictProbe struct {
	ConstraintIDs []string `json:"constraintIds"`
	Oracle        string   `json:"oracle"`
	Reason        string   `json:"reason"`
	SolverStatus  string   `json:"solverStatus,omitempty"`
	ElapsedMS     float64  `json:"elapsedMs"`
}

type AssemblyConflictBranch struct {
	ConstraintID      string                             `json:"constraintId"`
	DirectionRelation string                             `json:"directionRelation,omitempty"`
	DistanceRelation  string                             `json:"distanceRelation,omitempty"`
	ContactBranch     int32                              `json:"contactBranch,omitempty"`
	AngleBranchState  *geometry.AssemblyAngleBranchState `json:"angleBranchState,omitempty"`
}

type AssemblyConflictReport struct {
	SchemaVersion           int                      `json:"schemaVersion"`
	ActorID                 string                   `json:"actorId"`
	DocumentID              string                   `json:"documentId"`
	BaseRevisionID          string                   `json:"baseRevisionId"`
	InputDigest             string                   `json:"inputDigest"`
	SolverBuildPolicy       string                   `json:"solverBuildPolicy"`
	ScopeConstraintIDs      []string                 `json:"scopeConstraintIds"`
	ScopeBodyIDs            []string                 `json:"scopeBodyIds"`
	BackgroundConstraintIDs []string                 `json:"backgroundConstraintIds"`
	Branches                []AssemblyConflictBranch `json:"branches"`
	Status                  string                   `json:"status"`
	Items                   []AssemblyConflictItem   `json:"items"`
	Probes                  []AssemblyConflictProbe  `json:"probes"`
	ProbeCount              int                      `json:"probeCount"`
	ElapsedMS               float64                  `json:"elapsedMs"`
	Complete                bool                     `json:"complete"`
	BudgetReason            string                   `json:"budgetReason,omitempty"`
}

// Authorization is enforced by the API boundary. Analysis is read-only and
// revision-bound; no Workspace transaction is retained during probes.
func (service *Service) AnalyzeAssemblyConflicts(ctx context.Context, documentID, actorID string, request AssemblyConflictRequest) (AssemblyConflictReport, error) {
	if request.MaxProbes < 0 || request.MaxProbes > 128 || request.TimeBudgetMS < 0 || request.TimeBudgetMS > 5000 {
		return AssemblyConflictReport{}, fmt.Errorf("%w: invalid diagnostic probe/time budget", ErrValidation)
	}
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return AssemblyConflictReport{}, err
	}
	if view.Product == nil {
		return AssemblyConflictReport{}, fmt.Errorf("%w: diagnostics require Product", ErrValidation)
	}
	if request.BaseRevisionID == "" || request.BaseRevisionID != view.Document.VersionID {
		return AssemblyConflictReport{}, fmt.Errorf("%w: diagnostic base revision changed", ErrValidation)
	}
	frozen, err := service.FreezeAssemblyInput(ctx, documentID, request.BaseRevisionID, *view.Product)
	if err != nil {
		if !errors.Is(err, ErrValidation) {
			return AssemblyConflictReport{}, err
		}
		// Expression/structure/source compilation failures are not nonlinear
		// UNSAT. Preserve their domain reason and repairable public identities.
		kind := "STRUCTURE_INVALID"
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "quantity") || strings.Contains(lower, "parameter") || strings.Contains(lower, "expression") || strings.Contains(lower, "dimension") {
			kind = "PARAMETER_INVALID"
		}
		defs := map[string]AssemblyConstraint{}
		for _, c := range view.Product.Constraints {
			defs[c.ID] = c
		}
		ids := append([]string{}, request.TargetConstraintIDs...)
		if len(ids) == 0 {
			for _, c := range view.Product.Constraints {
				if activeConflictDefinition(c) {
					ids = append(ids, c.ID)
				}
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			if defs[id].ID == "" {
				return AssemblyConflictReport{}, fmt.Errorf("%w: diagnostic target %s missing", ErrValidation, id)
			}
		}
		raw, _ := json.Marshal(view.Product)
		return AssemblyConflictReport{SchemaVersion: 1, ActorID: actorID, DocumentID: documentID, BaseRevisionID: request.BaseRevisionID, InputDigest: "model-snapshot:" + canonicalModelHash(raw), SolverBuildPolicy: assemblySolverBuildPolicy, ScopeConstraintIDs: ids, ScopeBodyIDs: []string{}, BackgroundConstraintIDs: []string{}, Branches: []AssemblyConflictBranch{}, Status: "UNKNOWN", Items: []AssemblyConflictItem{conflictItem(defs, ids, kind, "DOMAIN_VALIDATION", "UNKNOWN", err.Error(), false)}, Probes: []AssemblyConflictProbe{}, Complete: false}, nil
	}
	report, err := AnalyzeFrozenAssemblyConflicts(ctx, actorID, request, frozen, func(probeCtx context.Context, input AssemblySolveManifest) (geometry.AssemblySolve, error) {
		return service.solveFrozenManifest(probeCtx, "diagnostic/"+input.Digest, input, nil)
	})
	if err != nil {
		return report, err
	}
	// A late answer must never annotate a newer model as though it were current.
	var head string
	if err = service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, documentID).Scan(&head); err != nil {
		return report, err
	}
	if head != request.BaseRevisionID {
		report.Status, report.Complete, report.BudgetReason = "STALE", false, "HEAD_CHANGED"
	}
	return report, nil
}

type AssemblyConflictOracle func(context.Context, AssemblySolveManifest) (geometry.AssemblySolve, error)

func activeConflictDefinition(c AssemblyConstraint) bool {
	return !c.Suppressed && c.Mode != "MEASURED"
}

// Full incidence closure retains ground, cycles, third reference axes and all
// overlapping/nested group motion units. No artificial boundary Fix is added.
func conflictScope(input AssemblySolveManifest, targets []string) ([]string, []string, error) {
	incidence := map[string][]string{}
	groups, err := assemblyGroupMembers(ProductModel{Instances: conflictInstances(input.Bodies), Constraints: input.Definitions})
	if err != nil {
		return nil, nil, err
	}
	for _, c := range input.Definitions {
		if !activeConflictDefinition(c) {
			continue
		}
		units := []string{c.First.InstanceID}
		if c.Second != nil {
			units = append(units, c.Second.InstanceID)
		}
		if c.AngleAxis != nil {
			units = append(units, c.AngleAxis.InstanceID)
		}
		if isAssemblyGroup(c) {
			units = append(units, groups[c.ID]...)
		}
		for _, id := range units {
			if id != "" {
				incidence[c.ID] = append(incidence[c.ID], id)
			}
		}
	}
	if len(targets) == 0 {
		for _, c := range input.Definitions {
			if activeConflictDefinition(c) && c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
				targets = append(targets, c.ID)
			}
		}
		if len(targets) == 0 {
			for id := range incidence {
				targets = append(targets, id)
			}
		}
	}
	selected, bodies := map[string]bool{}, map[string]bool{}
	byBody := map[string][]string{}
	for id, units := range incidence {
		for _, body := range units {
			byBody[body] = append(byBody[body], id)
		}
	}
	queue := append([]string{}, targets...)
	for _, id := range targets {
		if _, ok := incidence[id]; !ok {
			return nil, nil, fmt.Errorf("%w: diagnostic target %s missing/inactive", ErrValidation, id)
		}
		selected[id] = true
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, body := range incidence[id] {
			if bodies[body] {
				continue
			}
			bodies[body] = true
			for _, neighbour := range byBody[body] {
				if !selected[neighbour] {
					selected[neighbour] = true
					queue = append(queue, neighbour)
				}
			}
		}
	}
	return sortedConflictKeys(selected), sortedConflictKeys(bodies), nil
}

func conflictInstances(bodies []geometry.AssemblyBody) []ProductInstance {
	result := []ProductInstance{}
	for _, b := range bodies {
		result = append(result, ProductInstance{ID: b.ID, Translation: b.Pose.Translation, Rotation: b.Pose.Rotation})
	}
	return result
}
func sortedConflictKeys(values map[string]bool) []string {
	result := []string{}
	for id := range values {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

// Three-valued, bounded deletion filtering. UNKNOWN is never promoted to UNSAT,
// and a proven pair is not called irreducible until BOTH deletions have SAT
// witnesses. Minimal cardinality is deliberately not claimed.
func AnalyzeFrozenAssemblyConflicts(ctx context.Context, actorID string, request AssemblyConflictRequest, input AssemblySolveManifest, oracle AssemblyConflictOracle) (AssemblyConflictReport, error) {
	started := time.Now()
	r := AssemblyConflictReport{SchemaVersion: 1, ActorID: actorID, DocumentID: input.RootProductDocumentID, BaseRevisionID: input.RootProductRevisionID, InputDigest: input.Digest, SolverBuildPolicy: input.SolverBuildPolicy, Items: []AssemblyConflictItem{}, Probes: []AssemblyConflictProbe{}, BackgroundConstraintIDs: []string{}, Status: "UNKNOWN"}
	if request.BaseRevisionID != input.RootProductRevisionID {
		return r, fmt.Errorf("%w: frozen diagnostic revision mismatch", ErrValidation)
	}
	ids, bodies, err := conflictScope(input, request.TargetConstraintIDs)
	if err != nil {
		return r, err
	}
	r.ScopeConstraintIDs, r.ScopeBodyIDs = ids, bodies
	if len(ids) > 512 {
		// Do not truncate the connected equations and then claim SAT. Retain
		// the actual scope and honestly decline expensive first-stage search.
		r.Status, r.BudgetReason = "BUDGET_EXHAUSTED", "CONNECTED_SCOPE_EXCEEDS_512_DEFINITIONS"
		r.ElapsedMS = float64(time.Since(started).Microseconds()) / 1000
		return r, nil
	}
	max := request.MaxProbes
	if max == 0 {
		max = 24
	}
	if max < 1 || max > 128 {
		return r, fmt.Errorf("%w: diagnostic probe budget must be 1–128", ErrValidation)
	}
	ms := request.TimeBudgetMS
	if ms == 0 {
		ms = 800
	}
	if ms < 1 || ms > 5000 {
		return r, fmt.Errorf("%w: diagnostic time budget must be 1–5000ms", ErrValidation)
	}
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(ms)*time.Millisecond)
	defer cancel()
	definitions := map[string]AssemblyConstraint{}
	compiled := map[string]bool{}
	equations := map[string]map[string]bool{}
	annotations := []AssemblyConflictItem{}
	unknownKind, unknownEvidence := "LOCALIZED_SUSPECT", "NUMERICAL_UNKNOWN"
	for _, c := range input.Definitions {
		definitions[c.ID] = c
	}
	for _, c := range input.Constraints {
		compiled[c.ID] = true
		r.Branches = append(r.Branches, AssemblyConflictBranch{ConstraintID: c.ID, DirectionRelation: c.DirectionRelation, DistanceRelation: c.DistanceRelation, ContactBranch: c.ContactBranch, AngleBranchState: c.AngleBranchState})
	}
	// Unmet at the displayed pose is evidence about that pose, NOT a proof
	// that the frozen system has no solution elsewhere.
	initialPoses := map[string]geometry.AssemblyPose{}
	initialGeometry := map[string]geometry.AssemblyGeometry{}
	inScope := map[string]bool{}
	for _, id := range ids {
		inScope[id] = true
	}
	for _, b := range input.Bodies {
		initialPoses[b.ID] = b.Pose
	}
	for _, g := range input.Geometry {
		initialGeometry[g.ID] = g
	}
	for _, c := range input.Constraints {
		if !inScope[c.ID] || c.Mode == "MEASURED" {
			continue
		}
		if known, satisfied, reason := conflictGeometryRelation(c, initialGeometry, initialPoses, input.SolverProfile.LengthTolerance, input.SolverProfile.AngleTolerance); known && !satisfied {
			annotations = append(annotations, conflictItem(definitions, []string{c.ID}, "CURRENT_UNSATISFIED", "CURRENT_POSE_GEOMETRY", "UNKNOWN", reason, false))
		}
	}
	for _, s := range input.GroupStages {
		for _, g := range s.Groups {
			compiled[g.GroupID] = true
		}
	}
	for _, id := range ids {
		if !compiled[id] {
			c := definitions[id]
			kind := "SUPPORT_UNAVAILABLE"
			if c.EvaluationStatus == modelcore.AssemblyConstraintImpossible {
				kind = "GEOMETRY_INCOMPATIBLE"
			}
			r.Items = append(r.Items, conflictItem(definitions, []string{id}, kind, "RESOLUTION_EVIDENCE", "UNKNOWN", c.EvaluationSummary, false))
		}
	}
	probe := func(selected []string) (string, string) {
		if err := probeCtx.Err(); err != nil {
			r.BudgetReason = "TIME_OR_CANCEL"
			return "UNKNOWN", err.Error()
		}
		if len(r.Probes) >= max {
			r.BudgetReason = "PROBE_LIMIT"
			return "UNKNOWN", "probe budget exhausted"
		}
		begin := time.Now()
		frozen, e := conflictSubset(input, selected)
		p := AssemblyConflictProbe{ConstraintIDs: append([]string{}, selected...), Oracle: "UNKNOWN"}
		if e != nil {
			p.Reason = e.Error()
		} else if proof := conflictAnalyticProof(frozen); proof != nil {
			p.Oracle, p.Reason = "UNSAT", proof.Reason
		} else {
			unresolved := false
			for _, id := range selected {
				if !compiled[id] {
					unresolved = true
				}
			}
			if unresolved {
				p.Reason = "selected definition has no resolved equations"
			} else if len(selected) == 0 {
				p.Oracle, p.Reason = "SAT", "empty hard set; frozen poses are witness"
			} else if oracle == nil {
				p.Reason = "solver unavailable"
				if len(r.Probes) == 0 {
					unknownKind, unknownEvidence = "INFRASTRUCTURE_FAILURE", "SOLVER_UNAVAILABLE"
				}
			} else {
				result, solveErr := oracle(probeCtx, frozen)
				p.SolverStatus = result.Status
				if len(r.Probes) == 0 {
					for _, rank := range result.ConstraintRanks {
						if strings.Contains(rank.Role, "REDUNDANT") && definitions[rank.ConstraintID].ID != "" {
							annotations = append(annotations, conflictItem(definitions, []string{rank.ConstraintID}, "REDUNDANT", "LOCAL_JACOBIAN_RANK", "UNKNOWN", fmt.Sprintf("local rank: equations=%d effective=%d incremental=%d; dependence is not contradiction", rank.EquationCount, rank.EffectiveRank, rank.IncrementalRank), false))
						}
					}
					for _, diagnostic := range result.Diagnostics {
						if strings.Contains(diagnostic.Code, "DEGENERATE") || strings.Contains(diagnostic.Code, "SINGULAR") {
							annotations = append(annotations, conflictItem(definitions, diagnostic.ConstraintIDs, "DEGENERATE", "LOCAL_NUMERICAL_DIAGNOSTIC", "UNKNOWN", diagnostic.Code+": "+diagnostic.Detail, false))
						}
					}
				}
				for _, row := range result.EquationResiduals {
					owner := row.ConstraintID
					for _, group := range frozen.GroupStages {
						for _, g := range group.Groups {
							if strings.HasPrefix(owner, g.GroupID+"/member/") {
								owner = g.GroupID
							}
						}
					}
					if equations[owner] == nil {
						equations[owner] = map[string]bool{}
					}
					equations[owner][row.EquationID] = true
				}
				if solveErr != nil {
					p.Reason = solveErr.Error()
					if len(r.Probes) == 0 && probeCtx.Err() == nil {
						unknownKind, unknownEvidence = "INFRASTRUCTURE_FAILURE", "TRANSPORT_FAILURE"
						if status.Code(solveErr) == codes.InvalidArgument || status.Code(solveErr) == codes.FailedPrecondition {
							unknownKind, unknownEvidence = "GEOMETRY_INCOMPATIBLE", "WORKER_MODEL_VALIDATION"
						}
					}
				} else if ok, reason := conflictIndependentWitness(frozen, result); ok {
					p.Oracle, p.Reason = "SAT", reason
				} else {
					p.Reason = reason
					if len(r.Probes) == 0 && result.Status == "CONVERGED" && strings.Contains(reason, "certificate") {
						unknownKind, unknownEvidence = "VERIFICATION_UNKNOWN", "INDEPENDENT_CERTIFICATE_UNAVAILABLE"
					}
				}
			}
		}
		p.ElapsedMS = float64(time.Since(begin).Microseconds()) / 1000
		r.Probes = append(r.Probes, p)
		return p.Oracle, p.Reason
	}
	whole, reason := probe(ids)
	if whole == "SAT" {
		r.Status, r.Complete = "SAT", true
		r.Items = append(r.Items, conflictItem(definitions, ids, "SATISFIED", "INDEPENDENT_GEOMETRIC_WITNESS", "SAT", reason, false))
	} else if whole == "UNSAT" {
		current := append([]string{}, ids...)
		allKnown := true
		for _, id := range ids {
			trial := []string{}
			for _, retained := range current {
				if retained != id {
					trial = append(trial, retained)
				}
			}
			if len(trial) == len(current) {
				continue
			}
			answer, _ := probe(trial)
			if answer == "UNSAT" {
				current = trial
			} else if answer == "UNKNOWN" {
				allKnown = false
			}
		}
		r.Status, r.Complete = "UNSAT", allKnown && r.BudgetReason == ""
		evidence := "ANALYTIC_INCOMPATIBILITY"
		if r.Complete {
			evidence = "VERIFIED_IRREDUCIBLE"
		}
		r.Items = append(r.Items, conflictItem(definitions, current, "CONFLICT", evidence, "UNSAT", reason, r.Complete))
	} else {
		// Localize reliable proofs even if an unrelated unresolved/numerical
		// definition prevents deciding the whole connected component.
		if subset, e := conflictSubset(input, ids); e == nil {
			if proof := conflictAnalyticProof(subset); proof != nil {
				r.Items = append(r.Items, conflictItem(definitions, proof.IDs, "CONFLICT", "ANALYTIC_INCOMPATIBILITY", "UNSAT", proof.Reason, false))
			}
		}
		if len(r.Items) == 0 {
			r.Items = append(r.Items, conflictItem(definitions, ids, unknownKind, unknownEvidence, "UNKNOWN", reason, false))
		}
		r.Complete = false
	}
	if probeCtx.Err() != nil {
		r.BudgetReason = "TIME_OR_CANCEL"
		if ctx.Err() != nil {
			r.Status = "CANCELLED"
		} else {
			r.Status = "BUDGET_EXHAUSTED"
		}
	}
	if r.BudgetReason != "" && r.Status == "UNKNOWN" {
		r.Status = "BUDGET_EXHAUSTED"
	}
	r.ProbeCount = len(r.Probes)
	r.Items = append(r.Items, annotations...)
	for i := range r.Items {
		for j := range r.Items[i].Members {
			member := &r.Items[i].Members[j]
			member.EquationIDs = sortedConflictKeys(equations[member.ConstraintID])
		}
	}
	r.ElapsedMS = float64(time.Since(started).Microseconds()) / 1000
	return r, nil
}

func conflictSubset(input AssemblySolveManifest, ids []string) (AssemblySolveManifest, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return input, err
	}
	var frozen AssemblySolveManifest
	if err = json.Unmarshal(raw, &frozen); err != nil {
		return frozen, err
	}
	// The diagnostic orchestrator owns the explicit three-valued subset budget.
	// Its fixed-set Worker oracle must not start a second hidden deletion search.
	// DIAGNOSTIC requests also carry typed disable_conflict_probes so zero does
	// not depend on the legacy profile's default interpretation.
	frozen.SolverProfile.MaxConflictProbes = 0
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	frozen.Constraints = nil
	frozen.Definitions = nil
	frozen.ResolutionEvidence = nil
	// A nested group's membership identity remains readable even when that
	// group's own relationship is omitted in a deletion probe. Mark only the
	// probe-local copy inactive; never mutate the production activation state.
	for _, c := range input.Definitions {
		if isAssemblyGroup(c) && !selected[c.ID] {
			c.Suppressed = true
			frozen.Definitions = append(frozen.Definitions, c)
		}
	}
	for _, c := range input.Constraints {
		if selected[c.ID] && c.Mode != "MEASURED" {
			frozen.Constraints = append(frozen.Constraints, c)
		}
	}
	for _, c := range input.Definitions {
		if selected[c.ID] && activeConflictDefinition(c) {
			frozen.Definitions = append(frozen.Definitions, c)
		}
	}
	for _, evidence := range input.ResolutionEvidence {
		if selected[evidence.ConstraintID] {
			frozen.ResolutionEvidence = append(frozen.ResolutionEvidence, evidence)
		}
	}
	frozen.GroupStages, err = prepareAssemblyGroupStages(frozen, nil)
	if err == nil {
		originalGroups := map[string]AssemblyFrozenGroup{}
		for _, stage := range input.GroupStages {
			for _, group := range stage.Groups {
				originalGroups[group.GroupID] = group
			}
		}
		for i := range frozen.GroupStages {
			for j := range frozen.GroupStages[i].Groups {
				group := &frozen.GroupStages[i].Groups[j]
				if original, ok := originalGroups[group.GroupID]; ok {
					// Changing a deletion probe's internal equation set must not
					// silently recapture a different rigid relationship. The probe
					// input digest is new, but its relationship baseline stays frozen.
					group.CapturePending = original.CapturePending
					group.CapturedRelations = append([]AssemblyGroupRelation{}, original.CapturedRelations...)
				}
			}
		}
	}
	frozen.Purpose = "DIAGNOSTIC"
	frozen.Digest = assemblyManifestDigest(frozen)
	return frozen, err
}

func conflictItem(defs map[string]AssemblyConstraint, ids []string, kind, evidence, oracle, reason string, irreducible bool) AssemblyConflictItem {
	item := AssemblyConflictItem{Kind: kind, Evidence: evidence, Oracle: oracle, ConstraintIDs: append([]string{}, ids...), Reason: reason, Irreducible: irreducible, Members: []AssemblyConflictMember{}}
	for _, id := range ids {
		c := defs[id]
		actions := []string{"EDIT", "SUPPRESS"}
		if c.EvaluationStatus == modelcore.AssemblyConstraintBroken {
			actions = append(actions, "RECONNECT")
		}
		if assemblySupportsMeasurement(c) {
			actions = append(actions, "MEASURE")
		}
		member := AssemblyConflictMember{ConstraintID: id, First: c.First, Second: c.Second, AngleAxis: c.AngleAxis, GroupMembers: c.GroupMembers, RepairActions: actions}
		if isAssemblyGroup(c) {
			member.GroupID = id
		}
		item.Members = append(item.Members, member)
	}
	return item
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// Analysis evidence has its own bounded, rebuildable cache. It never enters
// command solve results, admission, production warm starts or document history.
type assemblyEngineeringCache struct {
	mu     sync.Mutex
	order  []string
	values map[string][]byte
}

func engineeringCacheKey(doc, rev string) string {
	return doc + "/" + rev + "/" + assemblySolverBuildPolicy + "/accepted-pose-v1"
}
func (c *assemblyEngineeringCache) get(key string) (AssemblyEngineeringEvidence, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var e AssemblyEngineeringEvidence
	raw, ok := c.values[key]
	if ok {
		ok = json.Unmarshal(raw, &e) == nil
	}
	return e, ok
}
func (c *assemblyEngineeringCache) put(key string, e AssemblyEngineeringEvidence) {
	raw, err := json.Marshal(e)
	if err != nil || len(raw) > 4<<20 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[string][]byte{}
	}
	if _, ok := c.values[key]; !ok {
		c.order = append(c.order, key)
	}
	c.values[key] = raw
	if len(c.order) > 32 {
		delete(c.values, c.order[0])
		c.order = c.order[1:]
	}
}

type AssemblyEngineeringExclusion struct {
	ConstraintID string `json:"constraintId"`
	Reason       string `json:"reason"`
	Diagnostic   string `json:"diagnostic,omitempty"`
}
type AssemblyEngineeringReference struct {
	InstanceID string `json:"instanceId"`
	DocumentID string `json:"documentId"`
	RevisionID string `json:"revisionId"`
}

func engineeringScope(model ProductModel, e *AssemblyEngineeringEvidence) map[string]bool {
	excluded := map[string]bool{}
	for _, i := range model.Instances {
		e.ScopeBodyIDs = append(e.ScopeBodyIDs, i.ID)
		e.References = append(e.References, AssemblyEngineeringReference{i.ID, i.ReferencedDocumentID, i.ReferencedVersionID})
	}
	for _, c := range model.Constraints {
		reason := ""
		switch {
		case c.Suppressed:
			reason = "SUPPRESSED"
		case c.Mode == "MEASURED":
			reason = "MEASURED"
		case c.EvaluationStatus != modelcore.AssemblyConstraintVerified:
			reason = string(c.EvaluationStatus)
			if reason == "" {
				reason = "NOT_UPDATED"
			}
		}
		if reason == "" {
			e.ConstraintIDs = append(e.ConstraintIDs, c.ID)
		} else {
			excluded[c.ID] = true
			e.Excluded = append(e.Excluded, AssemblyEngineeringExclusion{c.ID, reason, c.EvaluationSummary})
		}
	}
	sort.Strings(e.ScopeBodyIDs)
	sort.Strings(e.ConstraintIDs)
	return excluded
}
func engineeringCompiledScope(f AssemblySolveManifest, ids []string) bool {
	compiled := map[string]bool{}
	for _, c := range f.Constraints {
		if c.Mode != "MEASURED" && c.Mode != "SUPPRESSED" {
			compiled[c.ID] = true
		}
	}
	for _, s := range f.GroupStages {
		for _, g := range s.Groups {
			if g.CapturePending {
				return false
			}
			compiled[g.GroupID] = true
		}
	}
	if len(compiled) != len(ids) {
		return false
	}
	for _, id := range ids {
		if !compiled[id] {
			return false
		}
	}
	return true
}
func engineeringBodies(model ProductModel) []geometry.AssemblyBody {
	out := make([]geometry.AssemblyBody, 0, len(model.Instances))
	for _, i := range model.Instances {
		out = append(out, geometry.AssemblyBody{ID: i.ID, Pose: geometry.AssemblyPose{Translation: i.Translation, Rotation: normalizedInstanceRotation(i.Rotation)}})
	}
	return out
}

// Convergence alone does not prove that a local rank describes the saved pose.
// Gate both the returned bodies and every freedom's actual linearization pose.
func engineeringResult(e *AssemblyEngineeringEvidence, bodies []geometry.AssemblyBody, r geometry.AssemblySolve, p geometry.AssemblySolverProfile) {
	e.Available = false
	e.Status = "UNKNOWN"
	e.Components = []AssemblyEngineeringComponent{}
	e.SolverBuild = r.SolverBuild
	if r.SolverBuild != assemblySolverBuildPolicy {
		e.DiagnosticCode = "ANALYSIS_SOLVER_POLICY_UNCONFIRMED"
		return
	}
	if _, err := json.Marshal(r.Components); err != nil {
		e.DiagnosticCode = "ANALYSIS_NON_FINITE_EVIDENCE"
		return
	}
	if r.Status != "CONVERGED" {
		e.DiagnosticCode = "ANALYSIS_SOLVE_INCOMPLETE"
		e.Diagnostic = r.Status + ": " + r.Diagnostic
		return
	}
	expected := map[string]geometry.AssemblyPose{}
	for _, b := range bodies {
		expected[b.ID] = b.Pose
	}
	returned := map[string]bool{}
	for _, b := range r.Bodies {
		pose, ok := expected[b.ID]
		if !ok || returned[b.ID] || !validMotionPose(InstancePose(b.Pose)) || !conflictPoseClose(b.Pose, pose, p.LengthTolerance, p.AngleTolerance) {
			e.DiagnosticCode = "ANALYSIS_POSE_CHANGED"
			e.Diagnostic = "求解结果不在冻结的已接受位姿，当前构型的自由度未知。"
			return
		}
		returned[b.ID] = true
	}
	if len(returned) != len(expected) {
		e.DiagnosticCode = "ANALYSIS_BODY_COVERAGE_INCOMPLETE"
		return
	}
	covered := map[string]bool{}
	valid := 0
	for _, c := range r.Components {
		out := AssemblyEngineeringComponent{BodyIDs: c.BodyIDs, RelativeDof: c.RelativeDof, GaugeDof: c.GaugeDof, JacobianRank: c.JacobianRank, Solved: c.Solved, Freedoms: []geometry.AssemblyBodyFreedom{}}
		members := map[string]bool{}
		for _, id := range c.BodyIDs {
			_, ok := expected[id]
			if !ok || covered[id] || members[id] {
				e.DiagnosticCode = "ANALYSIS_COMPONENT_COVERAGE_INVALID"
				e.Components = nil
				return
			}
			members[id] = true
			covered[id] = true
		}
		freedoms := map[string]bool{}
		for _, f := range c.Freedoms {
			pose, ok := expected[f.BodyID]
			if !ok || !members[f.BodyID] || freedoms[f.BodyID] || !validMotionPose(InstancePose(f.LinearizationPose)) || !conflictPoseClose(f.LinearizationPose, pose, p.LengthTolerance, p.AngleTolerance) || (f.RelativeToBodyID != "" && !members[f.RelativeToBodyID]) {
				out.Solved = false
				out.DiagnosticCode = "ANALYSIS_LINEARIZATION_MISMATCH"
			}
			freedoms[f.BodyID] = true
		}
		if len(freedoms) != len(members) || len(members) == 0 {
			out.Solved = false
			out.DiagnosticCode = "ANALYSIS_FREEDOM_COVERAGE_INCOMPLETE"
		}
		if out.Solved {
			out.Freedoms = c.Freedoms
			valid++
		}
		e.Components = append(e.Components, out)
	}
	for id := range expected {
		if !covered[id] {
			e.Components = append(e.Components, AssemblyEngineeringComponent{BodyIDs: []string{id}, Freedoms: []geometry.AssemblyBodyFreedom{}, DiagnosticCode: "ANALYSIS_COMPONENT_MISSING"})
		}
	}
	e.Available = valid > 0
	if valid > 0 {
		e.Status = "AVAILABLE"
		if len(e.Components) != valid {
			e.Status = "PARTIAL"
		}
	} else {
		e.DiagnosticCode = "ANALYSIS_EVIDENCE_INCOMPLETE"
	}
}

// Uses the existing compiler and numerical kernel on an immutable accepted set.
// It deliberately never calls admission, persistAssemblySolveManifest or result
// promotion. Even a successful solve's poses and branch updates are discarded.
func (s *Service) RefreshAssemblyEngineeringEvidence(ctx context.Context, doc, rev string) (AssemblyEngineeringEvidence, error) {
	e := AssemblyEngineeringEvidence{DocumentID: doc, RevisionID: rev, Source: "READ_ONLY_ANALYSIS", Status: "UNKNOWN", ReferenceSemantics: "STATIC_RELATIVE", CoordinateFrame: "OWNING_PRODUCT", LengthUnit: "mm", SolverBuildPolicy: assemblySolverBuildPolicy, Components: []AssemblyEngineeringComponent{}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var raw []byte
	var head, kind string
	if err := s.database.QueryRow(ctx, `SELECT d.head_version_id::text,d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, doc, rev).Scan(&head, &kind, &raw); err != nil {
		return e, err
	}
	if head != rev {
		return e, fmt.Errorf("%w: ASSEMBLY_EVIDENCE_STALE", ErrValidation)
	}
	if kind != "PRODUCT" {
		return e, fmt.Errorf("%w: owning Product required", ErrValidation)
	}
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return e, err
	}
	excluded := engineeringScope(model, &e)
	capture := &assemblyDiagnosticCapture{invalid: map[string]bool{}}
	frozen, err := s.freezeAssemblyInput(context.WithValue(ctx, assemblyDiagnosticCaptureKey{}, capture), doc, rev, model, excluded)
	if err != nil {
		return e, err
	}
	e.InputDigest = frozen.Digest
	for _, failure := range capture.failures {
		if !excluded[failure.ConstraintID] {
			e.DiagnosticCode = "ANALYSIS_ACCEPTED_INPUT_UNAVAILABLE"
			e.Diagnostic = failure.ConstraintID + ": " + failure.Error
			return e, nil
		}
	}
	if !engineeringCompiledScope(frozen, e.ConstraintIDs) {
		e.DiagnosticCode = "ANALYSIS_ACCEPTED_SCOPE_INCOMPLETE"
		return e, nil
	}
	result, err := s.solveFrozenManifest(ctx, "engineering/"+frozen.Digest, frozen, nil)
	if err != nil {
		return e, err
	}
	engineeringResult(&e, engineeringBodies(model), result, frozen.SolverProfile)
	if err = ctx.Err(); err != nil {
		return e, err
	}
	if err = s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, doc).Scan(&head); err != nil {
		return e, err
	}
	if head != rev {
		return e, fmt.Errorf("%w: ASSEMBLY_EVIDENCE_STALE", ErrValidation)
	}
	s.assemblyEngineering.put(engineeringCacheKey(doc, rev), e)
	return e, nil
}

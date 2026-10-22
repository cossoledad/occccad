package assistant

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"

	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func (s *Service) verifyGoal(ctx context.Context, r Run) (*GoalVerification, error) {
	snapshot, err := s.automation.ReadSnapshot(ctx, r.ActorID, r.DocumentID, "")
	if err != nil {
		return nil, err
	}
	verification := &GoalVerification{Status: "VERIFIED", GoalVersion: r.GoalVersion, RevisionID: snapshot.RevisionID, Checks: []VerificationCheck{}}
	if r.ActiveTaskID != "" || r.RetiringTaskID != "" {
		verification.Status = "NOT_VERIFIED"
		verification.Checks = append(verification.Checks, VerificationCheck{ID: "execution-boundary", Name: "执行边界", Status: "NOT_VERIFIED", Code: "EXECUTION_PENDING", Message: "尚有计划片段或原操作回执需要处理"})
		return verification, nil
	}
	r.Requirements, err = s.goalRequirements(ctx, r)
	if err != nil {
		return nil, err
	}
	verification.requirements = r.Requirements
	verification.CoverageComplete = len(r.Requirements) > 0
	for _, req := range r.Requirements {
		if req.Coverage == "UNRESOLVED" {
			verification.CoverageComplete = false
			verification.Status = "NOT_VERIFIED"
			verification.Checks = append(verification.Checks, VerificationCheck{ID: req.ID, Name: req.Kind, Status: "NOT_VERIFIED", Code: "GOAL_COVERAGE_UNRESOLVED", Message: coverageMessage(req), Expected: requirementExpectation(req, r.Scope.WorkingBodyID)})
		}
	}
	if snapshot.DocumentType == "PRODUCT" {
		return s.verifyProductGoal(ctx, r, snapshot, verification)
	}
	if snapshot.DocumentType != "PART" {
		verification.Status = "NOT_VERIFIED"
		verification.Message = "当前文档类型尚无目标验收合同"
		return verification, nil
	}
	if r.TargetSelections == nil {
		r.TargetSelections = map[string]string{}
	}
	targets, err := s.goalTargets(ctx, r)
	if err != nil {
		return nil, err
	}
	// Verification targets are requirement-bound receipt outputs. The latest
	// SKETCH / LINEAR_EXTRUDE roles are discovery hints, never goal identities.
	r.Targets = targets
	body := r.Scope.WorkingBodyID
	if body == "" {
		body = r.Targets["BODY"]
	}
	checks := []automation.PartCheck{{ID: "required-body", Kind: "ENTITY_EXISTS", BodyID: body}, {ID: "required-solid", Kind: "BODY_GEOMETRY_VALID", BodyID: body}}
	created := []string{}
	for _, role := range []string{"SKETCH", "LINEAR_EXTRUDE", "FILLET", "CHAMFER"} {
		if id := r.Targets[role]; id != "" {
			created = append(created, id)
		}
	}
	if len(r.Requirements) == 0 {
		verification.Status = "NOT_VERIFIED"
		verification.Checks = append(verification.Checks, VerificationCheck{ID: "goal-contract", Name: "用户目标", Status: "NOT_VERIFIED", Code: "GOAL_CHECK_UNSUPPORTED", Message: "本轮目标尚未映射到应用可执行的必需检查，不能仅凭模型回复宣告完成"})
	}
	for _, requirement := range r.Requirements {
		if requirement.Coverage == "UNRESOLVED" {
			continue
		}
		check := automation.PartCheck{ID: requirement.ID, Kind: requirement.Kind, BodyID: body, Width: requirement.Width, Height: requirement.Height, Length: requirement.Length, Radius: requirement.Radius, Unit: requirement.Unit, RadiusPolicy: requirement.RadiusPolicy}
		if requirement.BodyID != "" {
			check.BodyID = requirement.BodyID
		}
		target := targets[goalTargetKey(requirement.ID)]
		switch requirement.Kind {
		case "RECTANGLE_DIMENSIONS":
			check.SketchID = target
			var original []byte
			if err := s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_inputs WHERE id=$1 AND run_id=$2`, requirement.SourceTurnID, r.ID).Scan(&original); err == nil {
				var source Turn
				if json.Unmarshal(original, &source) == nil {
					upper := strings.ToUpper(source.Text)
					for _, plane := range []string{"XY", "YZ", "XZ"} {
						if strings.Contains(upper, plane) {
							check.Plane = plane
							break
						}
					}
				}
			}
		case "EXTRUSION_LENGTH":
			check.FeatureID = target
			for _, profile := range r.Requirements {
				if profile.Kind == "RECTANGLE_DIMENSIONS" && profile.BodyID == requirement.BodyID {
					check.SketchID = targets[goalTargetKey(profile.ID)]
				}
			}
		case "ALL_EDGES_FILLET":
			check.FeatureID = target
		case "SELECTED_EDGES_FILLET":
			check.FeatureID = target
			token, count, e := s.requiredSelectedEdges(ctx, r, requirement, r.TargetSelections[goalTargetKey(requirement.ID)])
			if e != nil {
				verification.Status = "NOT_VERIFIED"
				verification.Checks = append(verification.Checks, VerificationCheck{ID: requirement.ID, Name: requirement.Kind, Status: "NOT_VERIFIED", Code: asFault(e).Code, Message: e.Error()})
				continue
			}
			check.SelectionSet, check.ExpectedCount = token, count
		case "HOLE_RIM_FILLET":
			check.FeatureID = target
			check.ReferenceFeatureID = targets[goalTargetKey(requirement.ReferenceRequirementID)]
		case "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
			check.FeatureID = target
			check.Plane = requirement.Plane
			for _, reference := range r.Requirements {
				if reference.Kind == "RECTANGLE_DIMENSIONS" && reference.BodyID == requirement.BodyID {
					check.ReferenceSketchID = targets[goalTargetKey(reference.ID)]
				}
			}
		case "NO_FILLET":
			check.Kind = "NO_FEATURE_TYPE"
			check.ExpectedType = "FILLET"
		default:
			if strings.HasPrefix(requirement.Kind, "PROTECTED_SELECTION") && requirement.Selection != nil && requirement.Selection.Selection != nil {
				check.Kind = "PROTECTED_SELECTION"
				ref := requirement.Selection
				source := ref.SourceVersionID
				if source == "" {
					var original []byte
					if err := s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_inputs WHERE id=$1 AND run_id=$2`, requirement.SourceTurnID, r.ID).Scan(&original); err != nil {
						return verification, err
					}
					var turn Turn
					if err := json.Unmarshal(original, &turn); err != nil {
						return verification, err
					}
					source = turn.Scope.RevisionID
				}
				check.Selection = &workspace.FeatureSelection{Selection: *ref.Selection, SourceVersionID: source, SourceFeatureID: ref.FeatureID}
			} else {
				verification.Status = "NOT_VERIFIED"
				verification.Checks = append(verification.Checks, VerificationCheck{ID: requirement.ID, Name: requirement.Kind, Status: "NOT_VERIFIED", Code: "GOAL_CHECK_UNSUPPORTED", Message: "该用户硬约束尚未有应用验证，不能跳过"})
				continue
			}
		}
		if target == "" {
			switch requirement.Kind {
			case "RECTANGLE_DIMENSIONS", "EXTRUSION_LENGTH", "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET", "HOLE_RIM_FILLET", "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
				verification.Status = "NOT_VERIFIED"
				verification.Checks = append(verification.Checks, VerificationCheck{ID: requirement.ID, Name: requirement.Kind, Status: "NOT_VERIFIED", Code: "GOAL_TARGET_PENDING", Message: "尚未从本轮回执定位到可验证对象；这不等于最终几何未完成，请先核对建模路径与当前实体。", Expected: requirementExpectation(requirement, body)})
				continue
			}
		}
		checks = append(checks, check)
	}
	results, err := s.verifyPartGoalChecks(ctx, r, automation.PartVerificationRequest{RevisionID: snapshot.RevisionID, GoalVersion: r.GoalVersion, CreatedEntityIDs: created, Checks: checks})
	if err != nil {
		return verification, err
	}
	for _, check := range results {
		item := VerificationCheck{ID: check.ID, Name: check.Kind, Status: check.Status, Code: check.Code, Message: check.Message, TargetID: targets[goalTargetKey(check.ID)], Observed: check.Observed}
		for _, requirement := range r.Requirements {
			if requirement.ID == check.ID {
				item.Expected = requirementExpectation(requirement, body)
				break
			}
		}
		verification.Checks = append(verification.Checks, item)
		if !check.Passed {
			verification.Status = "NOT_VERIFIED"
		}
	}
	if verification.Status != "VERIFIED" {
		verification.Message = checkEvidenceMessage(verification.Checks)
	}
	return verification, nil
}

func requirementExpectation(r GoalRequirement, body string) map[string]any {
	if r.BodyID != "" {
		body = r.BodyID
	}
	out := map[string]any{"kind": r.Kind, "bodyId": body, "unit": r.Unit, "sourceTurnId": r.SourceTurnID, "targetRole": r.TargetRole}
	switch r.Kind {
	case "RECTANGLE_DIMENSIONS":
		out["width"], out["height"] = r.Width, r.Height
	case "EXTRUSION_LENGTH":
		out["length"], out["role"] = r.Length, "ADDITIVE_BASE_EXTRUSION"
	case "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET", "HOLE_RIM_FILLET", "UNSCOPED_FILLET":
		if r.RadiusPolicy == "DELEGATED" {
			out["radiusPolicy"] = "DELEGATED"
		} else if r.Radius > 0 {
			out["radius"] = r.Radius
		} else {
			out["radiusPolicy"] = "UNRESOLVED"
		}
		if r.ReferenceRequirementID != "" {
			out["referenceRequirementId"] = r.ReferenceRequirementID
		}
	case "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
		out["diameter"], out["through"], out["plane"] = 2*r.Radius, true, r.Plane
	}
	return out
}

// Rebuild the user's original complete subset at the actual fillet input
// snapshot. Using the plan's own subset alone would let an old/unrelated fillet
// satisfy a newly selected edge goal merely because both contain one edge.
func (s *Service) requiredSelectedEdges(ctx context.Context, r Run, requirement GoalRequirement, appliedToken string) (string, int, error) {
	var set struct{ Document, Revision, Body, Kind string }
	raw, err := base64.RawURLEncoding.DecodeString(appliedToken)
	if err != nil || json.Unmarshal(raw, &set) != nil || set.Document != r.DocumentID || set.Revision == "" || set.Body == "" || set.Kind != "EDGE" {
		return "", 0, aiFault("SELECTION_EVIDENCE_MISSING", "尚无该圆角输入快照的完整正式选择证据", false)
	}
	var source Turn
	var payload []byte
	if err = s.db.QueryRow(ctx, `SELECT payload FROM occccad.assistant_inputs WHERE id=$1 AND run_id=$2`, requirement.SourceTurnID, r.ID).Scan(&payload); err != nil {
		return "", 0, err
	}
	if err = json.Unmarshal(payload, &source); err != nil {
		return "", 0, err
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, ref := range source.Scope.Selections {
		if ref.Selection == nil || ref.Selection.ExpectedType != "EDGE" {
			continue
		}
		if ref.Selection.SourceDocumentID != r.DocumentID || ref.Selection.SourceBodyID != set.Body {
			return "", 0, aiFault("SELECTION_SCOPE_MISMATCH", "冻结选边不属于当前圆角的目标实体", false)
		}
		revision := ref.SourceVersionID
		if revision == "" {
			revision = source.Scope.RevisionID
		}
		resolved, e := s.automation.ResolveSelectionForSnapshotWithEvidence(ctx, r.ActorID, r.DocumentID, set.Revision, workspace.FeatureSelection{Selection: *ref.Selection, SourceVersionID: revision, SourceFeatureID: ref.FeatureID})
		if e != nil {
			return "", 0, e
		}
		if !seen[resolved.TopologyID] {
			ids = append(ids, resolved.TopologyID)
			seen[resolved.TopologyID] = true
		}
	}
	if len(ids) == 0 {
		return "", 0, aiFault("SELECTION_EVIDENCE_MISSING", "该必需检查缺少用户发送时冻结的正式选边", false)
	}
	page, err := s.automation.QueryTopology(ctx, r.ActorID, r.DocumentID, automation.TopologyQuery{RevisionID: set.Revision, BodyID: set.Body, Kind: "EDGE", IDs: ids, Limit: 1})
	if err != nil {
		return "", 0, err
	}
	return page["selectionSet"].(string), len(ids), nil
}

func (s *Service) verifyProductGoal(ctx context.Context, r Run, snapshot workspace.ModelSnapshot, result *GoalVerification) (*GoalVerification, error) {
	var product workspace.ProductModel
	if err := json.Unmarshal(snapshot.Model, &product); err != nil {
		return result, err
	}
	matched := false
	for _, requirement := range r.Requirements {
		if requirement.Kind != "ASSEMBLY_FIT" {
			result.Status = "NOT_VERIFIED"
			result.Checks = append(result.Checks, VerificationCheck{ID: requirement.ID, Name: requirement.Kind, Status: "NOT_VERIFIED", Code: "GOAL_CHECK_UNSUPPORTED", Message: "该Product硬约束尚无应用检查"})
			continue
		}
		matched = true
		input, reason := assemblyGoalInput(r, product, snapshot.RevisionID, requirement)
		if reason != "" {
			result.Status = "NOT_VERIFIED"
			result.Checks = append(result.Checks, VerificationCheck{ID: requirement.ID, Name: "轴孔及端面定位", Status: "NOT_VERIFIED", Code: "ASSEMBLY_GOAL_INPUT_REQUIRED", Message: reason})
			continue
		}
		verified, err := s.automation.VerifyAssembly(ctx, r.ActorID, r.DocumentID, input)
		if err != nil {
			return result, err
		}
		check := VerificationCheck{ID: requirement.ID, Name: "权威装配求解、同轴、端面方向、偏置和目标自由度", Status: "PASSED"}
		if !verified.Passed || !verified.CurrentHead {
			result.Status = "NOT_VERIFIED"
			check.Status = "FAILED"
			check.Code = "ASSEMBLY_GOAL_FAILED"
			check.Message = strings.Join(verified.Failures, ", ")
		}
		result.Checks = append(result.Checks, check)
		if strings.Contains(requirement.Quote, "无干涉") || strings.Contains(requirement.Quote, "不干涉") || strings.Contains(strings.ToLower(requirement.Quote), "collision") || strings.Contains(requirement.Quote, "间隙") {
			result.Status = "NOT_VERIFIED"
			result.Checks = append(result.Checks, VerificationCheck{ID: requirement.ID + ":clearance", Name: "用户要求的干涉或间隙检查", Status: "NOT_VERIFIED", Code: "ASSEMBLY_CLEARANCE_UNVERIFIED", Message: "当前轴孔目标检查不能证明完整实体无干涉或指定间隙，尚需相应真实几何检查"})
		}
	}
	if !matched {
		result.Status = "NOT_VERIFIED"
		result.Checks = append(result.Checks, VerificationCheck{ID: "assembly-goal", Name: "装配用户目标", Status: "NOT_VERIFIED", Code: "GOAL_CHECK_UNSUPPORTED", Message: "未得到可执行的明确装配目标检查"})
	}
	return result, nil
}

// Targets identify the constraints actually created/modified by this Run.
// One exact fixed instance determines roles; neither names nor first-candidate
// ordering can select an unrelated pair in a large Product.
func assemblyGoalInput(r Run, product workspace.ProductModel, revision string, requirement GoalRequirement) (automation.AssemblyVerificationInput, string) {
	input := automation.AssemblyVerificationInput{RevisionID: revision}
	// Expected geometry comes from the versioned user requirement. Reading an
	// actual constraint can establish what was applied, never what was wanted.
	if requirement.AxisDirectionRelation != "SAME" && requirement.AxisDirectionRelation != "OPPOSITE" && requirement.AxisDirectionRelation != "UNORIENTED" {
		return input, "用户要求的轴线方向尚未明确，请指定SAME、OPPOSITE或UNORIENTED"
	}
	if requirement.PlaneDirectionRelation != "SAME" && requirement.PlaneDirectionRelation != "OPPOSITE" {
		return input, "用户要求的端面有向法线关系尚未明确，请指定SAME或OPPOSITE"
	}
	if requirement.OffsetMM == nil || math.IsNaN(*requirement.OffsetMM) || math.IsInf(*requirement.OffsetMM, 0) {
		return input, "用户要求的端面有向偏置尚未明确，请指定毫米值（贴合为0）"
	}
	if requirement.TargetDOF == nil || *requirement.TargetDOF < 0 {
		return input, "用户要求的目标自由度尚未明确"
	}
	desiredOffset := *requirement.OffsetMM
	desiredDOF := uint64(*requirement.TargetDOF)
	input.AxisDirectionRelation = requirement.AxisDirectionRelation
	input.DirectionRelation = requirement.PlaneDirectionRelation
	input.OffsetMM = &desiredOffset
	input.TargetDOF = &desiredDOF
	var axis, plane *workspace.AssemblyConstraint
	for i := range product.Constraints {
		c := &product.Constraints[i]
		if c.ID == r.Targets["AXIS_CONSTRAINT"] {
			axis = c
		}
		if c.ID == r.Targets["PLANE_CONSTRAINT"] {
			plane = c
		}
	}
	if axis == nil || plane == nil || axis.Second == nil || plane.Second == nil {
		return input, "尚未找到本运行对应的正式同轴与端面约束回执"
	}
	if axis.DirectionRelation == "" || plane.DirectionRelation == "" {
		return input, "轴线和端面方向仍未明确"
	}
	if requirement.AxisDirectionRelation != "UNORIENTED" && axis.DirectionRelation != requirement.AxisDirectionRelation {
		return input, "实际同轴约束的方向不符合用户要求"
	}
	if axis.DirectionRelation != "SAME" && axis.DirectionRelation != "OPPOSITE" && axis.DirectionRelation != "UNORIENTED" {
		return input, "实际同轴约束没有合法的明确方向合同"
	}
	if plane.DirectionRelation != requirement.PlaneDirectionRelation {
		return input, "实际端面约束的有向法线不符合用户要求"
	}
	pair := map[string]bool{axis.First.InstanceID: true, axis.Second.InstanceID: true}
	if len(pair) != 2 || !pair[plane.First.InstanceID] || !pair[plane.Second.InstanceID] {
		return input, "同轴与端面约束不属于同一对正式实例"
	}
	selected := map[string]bool{}
	for _, ref := range r.Scope.Selections {
		if len(ref.InstancePath) > 0 {
			selected[ref.InstancePath[0]] = true
		}
	}
	if len(selected) > 0 {
		if len(selected) != 2 {
			return input, "选择的装配对象存在歧义，请明确两个目标occurrence"
		}
		for id := range selected {
			if !pair[id] {
				return input, "已创建的配合不作用于用户冻结的目标occurrence"
			}
		}
	}
	fixed := map[string]bool{}
	for _, constraint := range product.Constraints {
		if constraint.Kind == "FIX" && !constraint.Suppressed && constraint.FixMode != "RELATIVE" && pair[constraint.First.InstanceID] {
			fixed[constraint.First.InstanceID] = true
		}
	}
	if len(fixed) != 1 {
		return input, "目标实例中需要一个明确的固定基准，不能猜测谁应移动"
	}
	for id := range fixed {
		input.ReferenceInstanceID = id
	}
	for id := range pair {
		if id != input.ReferenceInstanceID {
			input.MovingInstanceID = id
		}
	}
	firstAxis, secondAxis := axis.First, *axis.Second
	if firstAxis.InstanceID != input.MovingInstanceID {
		firstAxis, secondAxis = secondAxis, firstAxis
	}
	firstPlane, secondPlane := plane.First, *plane.Second
	planeReversed := firstPlane.InstanceID != input.MovingInstanceID
	if firstPlane.InstanceID != input.MovingInstanceID {
		firstPlane, secondPlane = secondPlane, firstPlane
	}
	input.FirstAxis, input.SecondAxis, input.FirstPlane, input.SecondPlane = &firstAxis, &secondAxis, &firstPlane, &secondPlane
	offset := plane.Value
	if plane.Kind == "COINCIDENT" {
		offset = 0
	} else if offset != 0 && plane.DistanceRelation != "SELECTED_PLANE_NORMAL_V1" {
		return input, "现有端面距离没有明确的有向偏置合同，不能猜测符号"
	}
	if planeReversed && plane.DirectionRelation == "SAME" {
		offset = -offset
	}
	// This checks the applied definition; VerifyAssembly separately checks real
	// geometry using its reported solver tolerance and the user's desired value.
	if math.IsNaN(offset) || math.IsInf(offset, 0) || math.Abs(offset-desiredOffset) > 1e-6 {
		return input, "实际端面有向偏置不符合用户要求"
	}
	return input, ""
}

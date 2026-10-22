package assistant

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var cubePattern = regexp.MustCompile(`(?i)` + number + `\s*(mm|毫米|cm|厘米|m|米)?\s*(?:的)?\s*(?:正?立方体|cube)`)
var intentSeparators = regexp.MustCompile(`[，,。；;\n→]+|然后|并且|最后|再|并`)
var continuationOnly = regexp.MustCompile(`(?i)^(继续|好的|可以|谢谢|continue|ok)[。.!！\s]*$`)

// Separate source clauses so one recognized action cannot cover another action
// in the same turn. This is a bounded extension of the existing contracts, not
// a general natural-language compiler. Unsupported clauses stay explicit.
func reviseRequirements(previous []GoalRequirement, turn Turn) []GoalRequirement {
	out := previous
	for _, clause := range intentSeparators.Split(turn.Text, -1) {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		before := map[string]string{}
		for _, r := range out {
			before[r.ID] = string(marshalJSON(r))
		}
		part := turn
		part.Text = clause
		out = reviseRequirementClause(out, part)
		changed := false
		for i := range out {
			r := &out[i]
			if before[r.ID] != string(marshalJSON(*r)) {
				changed = true
				annotateCoverage(r)
			}
		}
		if !changed && !continuationOnly.MatchString(clause) {
			key := "UNRESOLVED_INTENT:" + turn.ID + ":" + clause
			out = append(out, GoalRequirement{ID: uuid.NewSHA1(uuid.MustParse(turn.RunID), []byte(key)).String(), Kind: "UNRESOLVED_INTENT", BodyID: turn.Scope.WorkingBodyID, SourceTurnID: turn.ID, SourceGoalVersion: turn.GoalVersion, IntroducedGoalVersion: turn.GoalVersion, Quote: clause, Coverage: "UNRESOLVED", Unresolved: []string{"INTENT_CONTRACT_UNRESOLVED"}})
		}
	}
	return out
}

func requirementKey(kind string, r GoalRequirement) string {
	prefix := ""
	if r.BodyID != "" {
		prefix = r.BodyID + ":"
	}
	switch kind {
	case "HOLE_RIM_FILLET":
		return prefix + kind + ":" + r.ReferenceRequirementID
	case "SELECTED_EDGES_FILLET":
		return prefix + kind + ":" + r.Sources["selectionScope"]
	case "UNRESOLVED_INTENT":
		return kind + ":" + r.ID
	}
	return prefix + kind
}

func annotateCoverage(r *GoalRequirement) {
	r.Unresolved = nil
	switch r.Kind {
	case "UNSCOPED_FILLET":
		r.Unresolved = append(r.Unresolved, "FILLET_TARGET_REQUIRED")
	case "HOLE_RIM_FILLET":
		if r.ReferenceRequirementID == "" {
			r.Unresolved = append(r.Unresolved, "HOLE_TARGET_REQUIRED")
		}
	case "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
		if r.Radius <= 0 {
			r.Unresolved = append(r.Unresolved, "HOLE_DIAMETER_REQUIRED")
		}
	case "UNRESOLVED_INTENT":
		r.Unresolved = append(r.Unresolved, "INTENT_CONTRACT_UNRESOLVED")
	}
	if strings.Contains(r.Kind, "FILLET") && r.Kind != "NO_FILLET" && r.Radius <= 0 && r.RadiusPolicy != "DELEGATED" {
		r.Unresolved = append(r.Unresolved, "FILLET_RADIUS_REQUIRED")
	}
	r.Coverage = "SUPPORTED"
	if len(r.Unresolved) > 0 {
		r.Coverage = "UNRESOLVED"
	}
}

func completeGoalCoverage(v *GoalVerification) bool {
	if v == nil || !v.CoverageComplete || v.Status != "VERIFIED" || len(v.requirements) == 0 {
		return false
	}
	passed := map[string]bool{}
	for _, c := range v.Checks {
		if c.Status != "PASSED" {
			return false
		}
		passed[c.ID] = c.Status == "PASSED"
	}
	for _, r := range v.requirements {
		if r.Coverage == "UNRESOLVED" || !passed[r.ID] {
			return false
		}
	}
	return true
}

func coverageMessage(r GoalRequirement) string {
	messages := map[string]string{
		"INTENT_CONTRACT_UNRESOLVED":  "这条要求尚未映射到可执行验收，需要明确目标和操作",
		"FILLET_TARGET_REQUIRED":      "尚未明确圆角目标；需要固定选边或明确全部边、指定孔口",
		"FILLET_RADIUS_REQUIRED":      "缺少圆角半径，也没有收到委托选择参数的授权",
		"HOLE_DIAMETER_REQUIRED":      "通孔尚缺少明确直径",
		"HOLE_TARGET_REQUIRED":        "孔口圆角尚未绑定明确的通孔目标",
		"GOAL_VERSION_SOURCE_MISSING": "当前目标版本缺少来源输入，不能用旧检查验收",
	}
	out := []string{}
	for _, reason := range r.Unresolved {
		if message := messages[reason]; message != "" {
			out = append(out, message)
		} else {
			out = append(out, reason)
		}
	}
	return strings.Join(out, "；")
}

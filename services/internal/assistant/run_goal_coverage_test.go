package assistant

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAssistantGoalCoverageKeepsSourcesTargetsAndDelegation(t *testing.T) {
	first := Turn{ID: uuid.NewString(), RunID: uuid.NewString(), GoalVersion: 1, Text: "建立100mm立方体，再对全部边做R10圆角"}
	initial := reviseRequirements(nil, first)
	byKind := func(reqs []GoalRequirement, kind string) GoalRequirement {
		for _, r := range reqs {
			if r.Kind == kind {
				return r
			}
		}
		return GoalRequirement{}
	}
	base := byKind(initial, "ALL_EDGES_FILLET")
	if len(initial) != 3 || base.Radius != 10 || base.Coverage != "SUPPORTED" {
		t.Fatal(initial)
	}
	later := Turn{ID: uuid.NewString(), RunID: first.RunID, GoalVersion: 2, Text: "底面中心直径40通孔，再给孔口两圆边适当倒圆角"}
	revised := reviseRequirements(initial, later)
	all, hole, local := byKind(revised, "ALL_EDGES_FILLET"), byKind(revised, "CENTERED_THROUGH_HOLE"), byKind(revised, "HOLE_RIM_FILLET")
	if len(revised) != 5 || all.ID != base.ID || all.SourceTurnID != first.ID || all.Radius != 10 || hole.Radius != 20 || hole.SourceTurnID != later.ID || local.ReferenceRequirementID != hole.ID || local.RadiusPolicy != "DELEGATED" || local.Radius != 0 || local.Coverage != "SUPPORTED" {
		t.Fatal(revised)
	}
	combined := first
	combined.Text = "100mm立方体→全边R10→底面中心直径40通孔→孔口两圆边适当圆角"
	whole := reviseRequirements(nil, combined)
	if len(whole) != 5 || byKind(whole, "ALL_EDGES_FILLET").Radius != 10 || byKind(whole, "CENTERED_THROUGH_HOLE").Radius != 20 || byKind(whole, "HOLE_RIM_FILLET").RadiusPolicy != "DELEGATED" {
		t.Fatal("one turn's partial requirement masked another", whole)
	}
	if _, ok := requirementExpectation(local, "body")["radius"]; ok {
		t.Fatal("delegated numeric result became a user expectation")
	}
	unscoped := reviseRequirements(initial, Turn{ID: uuid.NewString(), RunID: first.RunID, GoalVersion: 2, Text: "给这两条边做R3圆角"})
	missing := byKind(unscoped, "UNSCOPED_FILLET")
	if missing.Coverage != "UNRESOLVED" || !strings.Contains(strings.Join(missing.Unresolved, ","), "TARGET") || byKind(unscoped, "ALL_EDGES_FILLET").Radius != 10 {
		t.Fatal("missing frozen edges silently became all edges", unscoped)
	}
	partial := reviseRequirements(initial, Turn{ID: uuid.NewString(), RunID: first.RunID, GoalVersion: 2, Text: "打一个通孔，并倒圆角"})
	for _, kind := range []string{"THROUGH_HOLE", "UNSCOPED_FILLET"} {
		if byKind(partial, kind).Coverage != "UNRESOLVED" {
			t.Fatal("partial clause was dropped", partial)
		}
	}
	amended := reviseRequirements(revised, Turn{ID: uuid.NewString(), RunID: first.RunID, GoalVersion: 3, Text: "孔口圆角改成R2"})
	if byKind(amended, "ALL_EDGES_FILLET").Radius != 10 || byKind(amended, "HOLE_RIM_FILLET").Radius != 2 || byKind(amended, "HOLE_RIM_FILLET").RadiusPolicy != "" {
		t.Fatal("local amendment overwrote base or kept delegated expectation", amended)
	}
	if byKind(amended, "HOLE_RIM_FILLET").IntroducedGoalVersion != 2 || byKind(amended, "HOLE_RIM_FILLET").SourceGoalVersion != 3 {
		t.Fatal("amendment lost original target provenance")
	}
	first.Scope.WorkingBodyID = "body-a"
	first.Text = "所有边R10圆角"
	other := Turn{ID: uuid.NewString(), RunID: first.RunID, GoalVersion: 2, Scope: FrozenScope{WorkingBodyID: "body-b"}, Text: "所有边R5圆角"}
	separate := reviseRequirements(reviseRequirements(nil, first), other)
	if len(separate) != 2 || separate[0].BodyID == separate[1].BodyID || separate[0].ID == separate[1].ID {
		t.Fatal("same-kind requirements collapsed across Body targets", separate)
	}
}

func TestAssistantCompletionCoverageIsConjunctive(t *testing.T) {
	requirements := []GoalRequirement{{ID: "base", Coverage: "SUPPORTED"}, {ID: "hole", Coverage: "SUPPORTED"}, {ID: "local", Coverage: "SUPPORTED", RadiusPolicy: "DELEGATED"}}
	v := &GoalVerification{Status: "VERIFIED", CoverageComplete: true, requirements: requirements, Checks: []VerificationCheck{{ID: "base", Status: "PASSED"}, {ID: "hole", Status: "PASSED"}}}
	if completeGoalCoverage(v) {
		t.Fatal("partial checks completed the goal")
	}
	v.Checks = append(v.Checks, VerificationCheck{ID: "local", Status: "PASSED"})
	if !completeGoalCoverage(v) {
		t.Fatal("complete evidence was rejected")
	}
	v.requirements[2].Coverage = "UNRESOLVED"
	if completeGoalCoverage(v) {
		t.Fatal("unresolved intent was hidden by passed checks")
	}
}

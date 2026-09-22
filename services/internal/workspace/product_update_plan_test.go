package workspace

import (
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestAssemblyDiagnosticsDoNotBlockReferenceUpdates(t *testing.T) {
	for _, status := range []modelcore.AssemblyConstraintEvaluationStatus{
		modelcore.AssemblyConstraintBroken, modelcore.AssemblyConstraintImpossible,
		modelcore.AssemblyConstraintNotUpdated, modelcore.AssemblyConstraintVerified,
	} {
		for _, updates := range []bool{false, true} {
			for _, canAccept := range []bool{false, true} {
				t.Run(string(status)+"/"+map[bool]string{true: "changed", false: "current"}[updates]+"/"+map[bool]string{true: "accept", false: "upstream-blocked"}[canAccept], func(t *testing.T) {
					plan := ProductUpdatePlan{CanAccept: canAccept, HasUpdates: updates}
					diagnostic := "support resolution: first=OUTSIDE_CURRENT_TIP second=OUTSIDE_CURRENT_TIP axis=RESOLVED"
					appendAssemblyUpdateDiagnostics(&plan, "revision", []AssemblyConstraint{
						{ID: "active", Kind: "ANGLE", EvaluationStatus: status, EvaluationSummary: diagnostic},
						{ID: "inactive", Kind: "ANGLE", Suppressed: true, EvaluationStatus: modelcore.AssemblyConstraintBroken},
					})
					if plan.CanAccept != canAccept || plan.HasUpdates != updates {
						t.Fatalf("constraint changed reference update policy: %+v", plan)
					}
					affected := updates || status != modelcore.AssemblyConstraintVerified
					if len(plan.AffectedConstraintIDs) != map[bool]int{true: 1, false: 0}[affected] {
						t.Fatalf("wrong affected constraints: %+v", plan)
					}
					if status == modelcore.AssemblyConstraintVerified {
						if len(plan.Entries) != 0 {
							t.Fatalf("verified diagnostic: %+v", plan)
						}
						return
					}
					if len(plan.Entries) != 1 {
						t.Fatalf("inactive constraint included: %+v", plan)
					}
					entry := plan.Entries[0]
					if entry.Currency != map[bool]string{true: "UPDATE_AVAILABLE", false: "CURRENT"}[updates] {
						t.Fatalf("constraint schedules spurious update: %+v", entry)
					}
					if status == modelcore.AssemblyConstraintBroken && (entry.Connection != "BROKEN" || entry.Evaluation != "FAILED" || entry.Diagnostic != diagnostic) {
						t.Fatalf("broken evidence lost: %+v", entry)
					}
					if status == modelcore.AssemblyConstraintImpossible && (entry.Evaluation != "FAILED" || entry.Diagnostic != diagnostic) {
						t.Fatalf("solver failure evidence lost: %+v", entry)
					}
				})
			}
		}
	}
}

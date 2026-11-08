package workspace

import (
	"strings"
	"testing"
)

func TestPatternReexecutionBudgetCountsActiveFormalSlots(t *testing.T) {
	p := &FeaturePattern{SourceKind: "FEATURE_REEXECUTION", PatternDefinition: PatternDefinition{Count: 256}}
	for i := 0; i < 4; i++ {
		p.AdditionalSources = append(p.AdditionalSources, PatternSeed{SourceKind: "FEATURE_REEXECUTION"})
	}
	if err := validatePatternReexecutionBudget(p); err == nil || !strings.Contains(err.Error(), "BUDGET") {
		t.Fatal("excessive replay accepted", err)
	}
	for slot := 205; slot < 256; slot++ {
		p.SkippedSlots = append(p.SkippedSlots, slot)
	}
	if err := validatePatternReexecutionBudget(p); err != nil {
		t.Fatal("inactive slots consume replay budget", err)
	}
	p.SkippedSlots = nil
	p.AdditionalSources = p.AdditionalSources[:3]
	if err := validatePatternReexecutionBudget(p); err != nil {
		t.Fatal("1020 replay calls rejected", err)
	}
}
func TestAssociativeGeneratorAdvertisesExplicitReplayWithoutFrozenTool(t *testing.T) {
	f := Feature{Type: "LINEAR_EXTRUDE", Operation: "REMOVE", Extent: "TO_NEXT"}
	c := replicationSource(f)
	if c.Tool || !c.Reexecution || c.Operation != "REMOVE" {
		t.Fatal(c)
	}
	if replicationSource(Feature{Type: "SHELL", Operation: "ADD"}).Reexecution {
		t.Fatal("arbitrary modifier advertised as replayable")
	}
	if replicationSource(Feature{Type: "LOFT", Operation: "INTERSECT"}).Reexecution {
		t.Fatal("intersection recipe advertised with union semantics")
	}
}

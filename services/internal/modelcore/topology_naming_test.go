package modelcore

import "testing"

func TestPersistentSelectionContractRejectsTransientOrIncompleteIdentity(t *testing.T) {
	t.Parallel()
	selection := PersistentSelection{
		SchemaVersion:    TopologyNamingSchemaVersion,
		SourceDocumentID: "part-1",
		SourceBodyID:     "body-main",
		Anchor: SemanticTopologyRef{
			FeatureID: "extrude-1", OutputSlot: "END_CAP/region-1", SourceIDs: []string{"region-1"},
		},
		ExpectedType: PersistentTopologyFace,
		Selector:     SelectionRecipe{Kind: SelectionDirectSemanticOutput},
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("valid persistent selection was rejected: %v", err)
	}
	selection.Anchor = SemanticTopologyRef{}
	if err := selection.Validate(); err == nil {
		t.Fatal("selection without a semantic anchor was accepted")
	}
}

func TestTopologyResolutionKeepsEndpointAndConstraintStatusSeparate(t *testing.T) {
	t.Parallel()
	if got := SupportingStatusForResolution(SelectionResolved); got != SupportingElementConnected {
		t.Fatalf("resolved selection is not connected: %s", got)
	}
	for _, status := range []SelectionResolutionStatus{SelectionMissing, SelectionAmbiguous, SelectionTypeMismatch, SelectionSourceUnavailable, SelectionContractMismatch, SelectionOutsideCurrentTip} {
		if got := SupportingStatusForResolution(status); got != SupportingElementNotConnected {
			t.Fatalf("%s unexpectedly maps to %s", status, got)
		}
	}
	if AssemblyConstraintBroken == AssemblyConstraintImpossible || AssemblyConstraintNotUpdated == AssemblyConstraintVerified {
		t.Fatal("constraint evaluation statuses collapsed")
	}
}

func TestSplitFixturePreservesEveryCandidateAndReportsAmbiguity(t *testing.T) {
	t.Parallel()
	source := SemanticTopologyRef{FeatureID: "extrude-1", OutputSlot: "END_CAP/region-1"}
	history := TopologyHistory{SchemaVersion: 1, FeatureID: "cut-1", ResultGeometryID: "shape-after-cut",
		Ambiguous: []AmbiguousTopologyLineage{{Sources: []SemanticTopologyRef{source}, Candidates: []SemanticTopologyRef{
			{FeatureID: "cut-1", OutputSlot: "SPLIT_FROM/extrude-1/END_CAP/a"},
			{FeatureID: "cut-1", OutputSlot: "SPLIT_FROM/extrude-1/END_CAP/b"},
		}, DiagnosticCode: "SELECTION_AMBIGUOUS"}}}
	if len(history.Ambiguous) != 1 || len(history.Ambiguous[0].Candidates) != 2 {
		t.Fatalf("split fixture lost candidates: %#v", history)
	}
	resolution := SelectionResolution{Status: SelectionAmbiguous, SupportingElementStatus: SupportingStatusForResolution(SelectionAmbiguous)}
	if resolution.SupportingElementStatus != SupportingElementNotConnected {
		t.Fatalf("ambiguous split must be disconnected: %#v", resolution)
	}
}

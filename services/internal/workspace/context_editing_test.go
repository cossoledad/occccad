package workspace

import "testing"

func TestContextFeatureEditPreservesUnchangedPatternDefinition(t *testing.T) {
	original := Feature{ID: "pattern", Type: "SOLID_PATTERN", Name: "Pattern.1", Pattern: &FeaturePattern{PatternDefinition: PatternDefinition{Count: 3, Spacing: 10, Angle: 45, Phase: 2, Origin: [3]float64{1, 2, 3}, Direction: [3]float64{1, 0, 0}}}}
	base := PartModel{Features: []Feature{original}}
	projected := clonePartModel(base)
	projected.Features[0].Pattern.Count = 5
	projected.Features[0].Pattern.Spacing = 20
	projected.Features[0].Pattern.Origin = [3]float64{10, 20, 30}
	projected.Features[0].Pattern.Direction = [3]float64{0, 1, 0}
	draft := clonePartModel(projected).Features[0]
	draft.Name = "Renamed"
	draft.Pattern.Angle = 60
	retainBaseFeatureValues(base, projected, &draft)
	if draft.Name != "Renamed" || draft.Pattern.Angle != 60 || draft.Pattern.Count != 3 || draft.Pattern.Spacing != 10 || draft.Pattern.Origin != original.Pattern.Origin || draft.Pattern.Direction != original.Pattern.Direction {
		t.Fatalf("accepted projection leaked into base definition: %+v", draft.Pattern)
	}
	if projected.Features[0].Pattern.Count != 5 {
		t.Fatal("normalization mutated the editor's accepted projection")
	}
	draft.Pattern.Spacing = 23
	retainBaseFeatureValues(base, projected, &draft)
	if draft.Pattern.Spacing != 23 {
		t.Fatal("explicit edit was lost")
	}
}

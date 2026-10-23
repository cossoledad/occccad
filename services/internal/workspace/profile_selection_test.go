package workspace

import (
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
)

func TestProfileRegionSelectionIdentityAndModes(t *testing.T) {
	regions := []geometry.ProfileRegion{{ID: "outer-a", Outer: geometry.ProfileLoop{ID: "formal-loop-a"}, Holes: []geometry.ProfileLoop{{ID: "hole"}}}, {ID: "island"}, {ID: "outer-b"}}
	f := Feature{Type: "LINEAR_EXTRUDE", Profile: "sketch"}
	for _, selection := range []*ProfileRegionSelection{nil, {Mode: "WHOLE_SKETCH"}} {
		f.ProfileSelection = selection
		got, err := selectFeatureProfileRegions(f, regions)
		if err != nil || len(got) != 3 {
			t.Fatal("whole-sketch contract", err)
		}
	}
	f.ProfileSelection = &ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{"outer-b", "outer-a"}}
	got, err := selectFeatureProfileRegions(f, regions)
	if err != nil || len(got) != 2 || got[0].ID != "outer-a" || len(got[0].Holes) != 1 {
		t.Fatal("explicit subset changed hole ownership", got, err)
	}
	if f.ProfileSelection.RegionIDs[0] != "outer-b" {
		t.Fatal("resolver mutated definition")
	}
	// A deleted or changed loop cannot be rebound to the nearest/new classified region.
	f.ProfileSelection.RegionIDs = []string{"outer-a", "removed-loop"}
	if _, err = selectFeatureProfileRegions(f, regions); err == nil || !strings.Contains(err.Error(), "PROFILE_REGION_MISSING") {
		t.Fatal("missing input silently discarded", err)
	}
	f.ProfileSelection.RegionIDs = []string{"outer-a"}
	if _, err = selectFeatureProfileRegions(f, append(regions, regions[0])); err == nil || !strings.Contains(err.Error(), "PROFILE_REGION_AMBIGUOUS") {
		t.Fatal("ambiguous input accepted", err)
	}
	for _, selection := range []*ProfileRegionSelection{{Mode: "REGIONS"}, {Mode: "REGIONS", RegionIDs: []string{"outer-a", "outer-a"}}, {Mode: "WHOLE_SKETCH", RegionIDs: []string{"outer-a"}}, {Mode: "unknown"}} {
		f.ProfileSelection = selection
		if err = validateProfileRegionSelection(f); err == nil {
			t.Fatal("invalid set accepted", selection)
		}
	}
}

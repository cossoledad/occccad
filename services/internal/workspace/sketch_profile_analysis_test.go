package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSketchProfileAnalysisRequiresFormalEndpointConnections(t *testing.T) {
	lines := []SketchEntity{
		{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}},
		{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}},
	}
	feature := profileSketch(lines, nil)
	analysis := analyzeSketchProfile(feature)
	if analysis.Status != "OPEN" || len(analysis.Issues) != 4 {
		t.Fatalf("coordinate equality welded endpoints: %#v", analysis)
	}
	for _, issue := range analysis.Issues {
		if issue.Code != "OPEN_ENDPOINT" || issue.Position == nil || len(issue.References) != 1 {
			t.Fatalf("missing explicit endpoint: %#v", issue)
		}
	}
	feature.Sketch.Constraints = []SketchConstraint{{ID: "join", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}}}
	analysis = analyzeSketchProfile(feature)
	if analysis.Status != "OPEN" || len(analysis.Issues) != 2 {
		t.Fatalf("formal connection not reflected: %#v", analysis)
	}
}

func TestSketchProfileAnalysisUsesProductionRegionsAndColdProjection(t *testing.T) {
	feature := profileSketch([]SketchEntity{
		{ID: "outer", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 20},
		{ID: "hole", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 10},
		{ID: "island", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{}, Radius: 3},
		{ID: "separate", Kind: "ELLIPSE", Role: "PROFILE", Center: &SketchPoint2{X: 50}, MajorRadius: 8, MinorRadius: 4},
		{ID: "helper", Kind: "LINE", Role: "CONSTRUCTION", Start: &SketchPoint2{}, End: &SketchPoint2{Y: 50}},
		{ID: "point", Kind: "POINT", Role: "PROFILE", Point: &SketchPoint2{X: 70}},
	}, nil)
	regions, err := buildProfileRegions(feature)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeSketchProfile(feature)
	if analysis.Status != "CLOSED" || analysis.RegionCount != 3 || analysis.LoopCount != 4 || !reflect.DeepEqual(analysis.Regions, regions) {
		t.Fatalf("analysis drifted from production Profile: %#v", analysis)
	}
	model := newPartModel()
	model.Features = append(model.Features, feature)
	before, _ := json.Marshal(model)
	projected := projectSketchAnalyses(model)
	after, _ := json.Marshal(model)
	if string(before) != string(after) {
		t.Fatal("read analysis mutated business model")
	}
	var cold PartModel
	if err = json.Unmarshal(before, &cold); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, projectSketchAnalyses(cold)) {
		t.Fatal("cold read analysis differs")
	}
}

func TestSketchProfileAnalysisLocatesDuplicatesAndBranches(t *testing.T) {
	feature := profileSketch([]SketchEntity{
		{ID: "c1", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 3, Y: 7}, Radius: 4},
		{ID: "c2", Kind: "CIRCLE", Role: "PROFILE", Center: &SketchPoint2{X: 3, Y: 7}, Radius: 4},
	}, nil)
	analysis := analyzeSketchProfile(feature)
	if analysis.Status != "INVALID" || len(analysis.Issues) != 1 || analysis.Issues[0].Code != "DUPLICATE_EDGE" || analysis.Issues[0].Position.X != 3 || analysis.Issues[0].Position.Y != 7 {
		t.Fatalf("duplicate location missing: %#v", analysis)
	}
	feature = profileSketch([]SketchEntity{
		{ID: "a", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}},
		{ID: "b", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 10, Y: 10}},
		{ID: "c", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{X: 10}, End: &SketchPoint2{X: 20}},
	}, []SketchConstraint{
		{ID: "ab", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "b", SubElement: "START"}}},
		{ID: "ac", Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: "a", SubElement: "END"}, {Target: "ENTITY", EntityID: "c", SubElement: "START"}}},
	})
	analysis = analyzeSketchProfile(feature)
	if analysis.Status != "INVALID" {
		t.Fatalf("branch accepted: %#v", analysis)
	}
	found := false
	for _, issue := range analysis.Issues {
		if issue.Code == "BRANCH_POINT" {
			found = true
			if len(issue.References) != 3 || issue.Position.X != 10 {
				t.Fatalf("branch refs/location: %#v", issue)
			}
		}
	}
	if !found {
		t.Fatal("missing branch issue")
	}
}

func TestSketchProfileAnalysisDoesNotRejectSavingOpenSketch(t *testing.T) {
	model := newPartModel()
	feature := profileSketch(nil, nil)
	feature.Sketch.Support.DatumPlaneID = "datum-xy"
	model.Features = append(model.Features, feature)
	before, _ := json.Marshal(model)
	entity := SketchEntity{ID: "line", Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{}, End: &SketchPoint2{X: 10}}
	payload, _ := json.Marshal(editSketchPayload{SketchID: feature.ID, Operations: []SketchOperation{{Type: "ADD_ENTITY", Entity: &entity}}})
	after, _, err := applyEditSketch(before, payload)
	if err != nil {
		t.Fatalf("legal open sketch could not be saved: %v", err)
	}
	var edited PartModel
	if err = json.Unmarshal(after, &edited); err != nil {
		t.Fatal(err)
	}
	analysis := projectSketchAnalyses(edited)[feature.ID]
	if analysis.Status != "OPEN" || len(analysis.Issues) != 2 {
		t.Fatalf("open saved sketch not described: %#v", analysis)
	}
	if _, err = buildProfileRegions(edited.Features[0]); err == nil {
		t.Fatal("open sketch accepted for closed solid extrusion")
	}
}

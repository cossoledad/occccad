package workspace

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestPartDependenciesProjectFormalGraphAndSavedRegionInputs(t *testing.T) {
	m := newPartModel()
	m.Features = append(m.Features, testRectangleSketch("profile", "XY"))
	normalizePartModel(&m)
	body := m.Features[0].BodyID
	m.Features = append(m.Features, Feature{ID: "pad", Name: "Extrude.1", Type: "LINEAR_EXTRUDE", BodyID: body, Profile: "profile", ProfileSelection: &ProfileRegionSelection{Mode: "REGIONS", RegionIDs: []string{"old-region"}}, Operation: "ADD", Extent: "FINITE", Length: 5, EvaluationStatus: "FAILED", Diagnostic: "PROFILE_REGION_MISSING: old-region"})
	ensureFeatureParameters(&m)
	if err := validateAndResolvePartParameters(&m); err != nil {
		t.Fatal(err)
	}
	v := DocumentView{Document: DocumentSummary{ID: "part", VersionID: "head"}, Part: &m, SketchAnalyses: map[string]SketchProfileAnalysis{"profile": {Status: "OPEN", GeometryVerified: true, RegionIDs: []string{"new-region"}}}}
	before, _ := json.Marshal(v)
	projection, err := partDependencyProjection(v)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(m)
	graph, _, err := buildPartEvaluation(m, "head", canonicalModelHash(data), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := graph.Digest()
	if projection.Digest != digest || !reflect.DeepEqual(projection.Edges, graph.Edges) {
		t.Fatal("display invented a second dependency graph")
	}
	if len(projection.Nodes) != len(graph.Nodes) {
		t.Fatal("formal nodes dropped")
	}
	find := func(key modelcore.DependencyKey) PartDependencyNode {
		for _, n := range projection.Nodes {
			if n.Key == key {
				return n
			}
		}
		t.Fatalf("missing %s", key)
		return PartDependencyNode{}
	}
	pad := find("feature:pad")
	if pad.State != "FAILED" || len(pad.Inputs) != 1 || pad.Inputs[0].State != "MISSING" || pad.Inputs[0].RegionID != "old-region" {
		t.Fatal(pad)
	}
	if find("profile:profile").State != "AVAILABLE" {
		t.Fatal("unrelated open curves were presented as the cause of a missing region")
	}
	after, _ := json.Marshal(v)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("read projection changed the model")
	}
	m.Features[1].Name = "Renamed"
	renamed, err := partDependencyProjection(v)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range projection.Nodes {
		if n.Key != renamed.Nodes[i].Key {
			t.Fatal("label defined dependency identity")
		}
	}
}

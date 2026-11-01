package modelcore

import (
	"encoding/json"
	"testing"
)

func TestDerivedFingerprintIgnoresRevisionAndInputOrdering(t *testing.T) {
	input := DerivedInput{Evaluator: "e1", Policy: "exact", Definition: json.RawMessage(`{"kind":"AREA"}`), Reads: []DependencyRead{{DocumentID: "part", EntityID: "body", Facet: IntrinsicGeometry, Digest: "geometry", RevisionID: "r1"}, {DocumentID: "product", EntityID: "member", Facet: Placement, Digest: "pose"}}}
	a, err := input.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	input.Reads[0].RevisionID = "r2"
	input.Reads[0], input.Reads[1] = input.Reads[1], input.Reads[0]
	b, err := input.Fingerprint()
	if err != nil || a != b {
		t.Fatal("provenance or ordering changed currency", err)
	}
	input.Reads[0].Digest = "moved"
	c, _ := input.Fingerprint()
	if c == a {
		t.Fatal("placement ignored")
	}
}

func TestDerivedEvaluationSchedulingAndCycle(t *testing.T) {
	nodes := []DependencyNode{{Key: "shape", Phase: 2}, {Key: "measure", Phase: 2, Derived: true}, {Key: "output", Phase: 1}, {Key: "formula", Phase: 1}, {Key: "consumer", Phase: 2}}
	edges := []DependencyEdge{{Source: "shape", Target: "measure"}, {Source: "measure", Target: "output"}, {Source: "output", Target: "formula"}, {Source: "formula", Target: "consumer"}}
	graph, err := ScheduleEvaluationGraph(nodes, edges)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Nodes["formula"].Phase < 2 || len(graph.DirtyClosure([]DependencyKey{"shape"})) != 5 {
		t.Fatal("derived stage or propagation lost", graph)
	}
	if _, err := ScheduleEvaluationGraph(nodes, append(edges, DependencyEdge{Source: "consumer", Target: "shape"})); err == nil {
		t.Fatal("feedback accepted")
	}
	if _, err := ScheduleEvaluationGraph([]DependencyNode{{Key: "late", Phase: 2}, {Key: "early", Phase: 1}}, []DependencyEdge{{Source: "late", Target: "early"}}); err == nil {
		t.Fatal("ordinary phase violation accepted")
	}
}

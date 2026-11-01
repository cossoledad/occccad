package workspace

import (
	"encoding/json"
	"github.com/google/uuid"
	"reflect"
	"testing"
)

func TestSpatialDefinitionsHistoryIsIndependentFromAssemblyPose(t *testing.T) {
	m := ProductModel{Instances: []ProductInstance{{ID: uuid.NewString(), Name: "Part.1", Translation: [3]float64{1, 2, 3}}}}
	a := SpatialAnalysis{ID: uuid.NewString(), Name: "Section.1", Kind: "SECTION", Result: &SpatialResultReference{Policy: spatialPolicy, ArtifactID: uuid.NewString()}}
	raw, _ := json.Marshal(m)
	payload, _ := json.Marshal([]SpatialAnalysis{a})
	next, changes, e := applySpatialAnalyses(raw, payload)
	if e != nil || len(changes.Changes) != 1 {
		t.Fatal(e, changes)
	}
	before, e := productHistoryValues(raw, changes)
	if e != nil {
		t.Fatal(e)
	}
	after, e := productHistoryValues(next, changes)
	if e != nil {
		t.Fatal(e)
	}
	restored, e := applyProductHistoryValues(next, before)
	if e != nil {
		t.Fatal(e)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if !reflect.DeepEqual(m, undo) {
		t.Fatal("undo lost Product state", undo)
	}
	replay, e := applyProductHistoryValues(restored, after)
	if e != nil || string(replay) != string(next) {
		t.Fatal("redo", e, string(replay), string(next))
	}
	var n ProductModel
	_ = json.Unmarshal(next, &n)
	if !spatialOnlyHistory(m, n, changes) {
		t.Fatal("analysis would invoke assembly solve")
	}
	n.Instances[0].Translation[0] = 42
	if spatialOnlyHistory(m, n, changes) {
		t.Fatal("pose changes bypassed checks")
	}
}
func TestSpatialDefinitionUsesStableUnorderedScope(t *testing.T) {
	p := InstancePath{RootDocumentID: "root", Canonical: "a", Display: "Old name", Segments: []InstancePathSegment{{InstanceID: "a", OwnerVersionID: "before"}}}
	a := SpatialAnalysis{Kind: "SECTION", Scope: "SELECTED", Selected: []InstancePath{p}}
	b := a
	b.Selected = []InstancePath{p}
	b.Selected[0].Display = "New name"
	b.Selected[0].Segments = nil
	b.Name = "new"
	b.Status = "CURRENT"
	b.Result = &SpatialResultReference{}
	if !reflect.DeepEqual(spatialDefinition(a), spatialDefinition(b)) {
		t.Fatal("read projection changed analysis definition")
	}
	for _, a := range []SpatialAnalysis{{Kind: "SECTION", Scope: "BETWEEN"}, {Kind: "SECTION", Scope: "ALL", SectionType: "PLANE", CutMode: "NONE", Size: [3]float64{-1, 1, 1}}, {Kind: "BAND", Scope: "BETWEEN", AccuracyMM: 2, MinimumMM: 5, MaximumMM: 4}} {
		if validateSpatial(a) == nil {
			t.Fatal("illegal definition accepted", a)
		}
	}
}

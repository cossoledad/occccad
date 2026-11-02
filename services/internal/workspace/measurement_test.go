package workspace

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/geometry"
	"reflect"
	"testing"
)

func TestMeasurementDefinitionHistoryForBothDocuments(t *testing.T) {
	m := Measurement{ID: uuid.NewString(), Name: "测量.1", Kind: "ITEM", Supports: []MeasurementSupport{{Kind: "BODY", BodyID: "body-main", Mode: "GEOMETRY"}}, Visible: true, Result: &MeasurementEvidence{SourceRevisionID: "before", SourceStamp: "stamp", Policy: measurementPolicy, MeasurementResult: geometry.MeasurementResult{Complete: true, KernelBuild: "kernel", Values: map[string]float64{"volumeMm3": 10}, Anchors: map[string][3]float64{"centroid": {1, 2, 3}}}}}
	for _, kind := range []string{"PART", "PRODUCT"} {
		t.Run(kind, func(t *testing.T) {
			var raw json.RawMessage
			if kind == "PART" {
				raw, _ = json.Marshal(PartModel{Bodies: []PartBody{{ID: "body-main", Name: "Main"}}, Measurements: nil})
			} else {
				raw, _ = json.Marshal(ProductModel{Instances: []ProductInstance{{ID: "instance"}}})
			}
			payload, _ := json.Marshal([]Measurement{m})
			next, changes, err := applyMeasurements(raw, payload)
			if err != nil {
				t.Fatal(err)
			}
			if !measurementsOnly(changes) || len(changes.ImpactSeeds) != 0 {
				t.Fatal("measurement triggered geometric work", changes)
			}
			changes, err = reconcilePersistedChanges(kind, raw, next, changes)
			if err != nil {
				t.Fatal(err)
			}
			current, err := modelValues(kind, next, changes)
			if err != nil {
				t.Fatal(err)
			}
			undo, err := changes.Compensate(current)
			if err != nil {
				t.Fatal(err)
			}
			undone, err := applyModelValues(kind, next, undo)
			if err != nil {
				t.Fatal(err)
			}
			values, _ := measurementValues(undone)
			if len(values) != 0 {
				t.Fatal("undo retained measurement", values)
			}
			redoCurrent, err := modelValues(kind, undone, changes)
			if err != nil {
				t.Fatal(err)
			}
			redo, err := changes.Reapply(redoCurrent)
			if err != nil {
				t.Fatal(err)
			}
			redone, err := applyModelValues(kind, undone, redo)
			if err != nil {
				t.Fatal(err)
			}
			values, _ = measurementValues(redone)
			if !reflect.DeepEqual(values, []Measurement{m}) {
				t.Fatal("redo lost evidence", values)
			}
			m.Supports[0].LocalID = 12
			bad, _ := json.Marshal([]Measurement{m})
			if _, _, err = applyMeasurements(raw, bad); err == nil {
				t.Fatal("saved local locator")
			}
			m.Supports[0].LocalID = 0
		})
	}
}
func TestMeasurementPathContinuationChecksTypedIdentity(t *testing.T) {
	a := InstancePath{RootDocumentID: "root", Segments: []InstancePathSegment{{OwnerDocumentID: "root", OwnerVersionID: "v1", InstanceID: "member", ReferencedDocumentID: "part", ResolvedVersionID: "p1"}}}
	b := a
	b.Segments = append([]InstancePathSegment{}, a.Segments...)
	b.Segments[0].OwnerVersionID = "v2"
	b.Segments[0].ResolvedVersionID = "p2"
	if !measurementPathMatches(a, b) {
		t.Fatal("snapshot Revision became identity")
	}
	b.Segments[0].ReferencedDocumentID = "replacement"
	if measurementPathMatches(a, b) {
		t.Fatal("replacement silently rebound measurement")
	}
}

func TestMeasurementGroupVisibilityUsesOneReversibleEdit(t *testing.T) {
	for _, kind := range []string{"PART", "PRODUCT"} {
		t.Run(kind, func(t *testing.T) {
			values := []Measurement{{ID: "00000000-0000-7000-8000-000000000001", Name: "First", Kind: "ITEM", Supports: []MeasurementSupport{{Kind: "BODY", BodyID: "body", Mode: "GEOMETRY"}}, Visible: true}, {ID: "00000000-0000-7000-8000-000000000002", Name: "Second", Kind: "ITEM", Supports: []MeasurementSupport{{Kind: "BODY", BodyID: "body", Mode: "GEOMETRY"}}, Visible: false}}
			for i := range values {
				values[i].Result = &MeasurementEvidence{SourceRevisionID: "before", SourceStamp: "stamp", Policy: measurementPolicy, MeasurementResult: geometry.MeasurementResult{Complete: true, KernelBuild: "kernel", Values: map[string]float64{"volumeMm3": 10}, Anchors: map[string][3]float64{"centroid": {1, 2, 3}}}}
			}
			before, _ := json.Marshal(map[string]any{"measurements": values})
			service := &Service{}
			uri, payload, err := service.adaptLegacyCommand(t.Context(), "document", kind, before, CommandRequest{Type: "SET_DEFINITION_VISIBILITY", TargetKind: "MEASUREMENT_GROUP", TargetID: "measurements", Visible: false})
			if err != nil || uri != typeSetMeasurements {
				t.Fatal(uri, err)
			}
			raw, _ := json.Marshal(payload)
			next, changes, err := applyMeasurements(before, raw)
			if err != nil {
				t.Fatal(err)
			}
			hidden, _ := measurementValues(next)
			for _, m := range hidden {
				if m.Visible {
					t.Fatal("group left an annotation visible")
				}
			}
			node := measurementStructure("doc", "rev", "root", hidden, nil)
			if node.EntityID != "measurements" || node.LocalVisible == nil || *node.LocalVisible {
				t.Fatal("group projection disagrees")
			}
			changes, err = reconcilePersistedChanges(kind, before, next, changes)
			if err != nil {
				t.Fatal(err)
			}
			original, err := modelValues(kind, before, changes)
			if err != nil {
				t.Fatal(err)
			}
			undone, err := applyModelValues(kind, next, original)
			if err != nil {
				t.Fatal(err)
			}
			restored, _ := measurementValues(undone)
			if !restored[0].Visible || restored[1].Visible {
				t.Fatal("Undo lost individual display states", restored)
			}
		})
	}
}

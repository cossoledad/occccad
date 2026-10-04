package workspace

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestDatumDeletionProtectsReferencesAndCompensates(t *testing.T) {
	for _, kind := range []string{"DATUM_PLANE", "DATUM_AXIS"} {
		t.Run(kind, func(t *testing.T) {
			model := newPartModel()
			model.DatumPlanes = append(model.DatumPlanes, DatumPlane{ID: "custom", Name: "Custom", Plane: "CUSTOM", Origin: [3]float64{1, 2, 3}, Normal: [3]float64{0, 0, 1}, UDirection: [3]float64{1, 0, 0}, Size: 180})
			model.DatumAxes = append(model.DatumAxes, DatumAxis{ID: "custom", Name: "Custom", Origin: [3]float64{1, 2, 3}, Direction: [3]float64{0, 1, 0}})
			original, _ := json.Marshal(model)
			payload, _ := json.Marshal(deleteNodePayload{TargetKind: kind, TargetID: "custom"})
			next, changes, err := applyDeletePartNode(original, payload)
			if err != nil {
				t.Fatal(err)
			}
			values, err := modelValues("PART", next, changes)
			if err != nil {
				t.Fatal(err)
			}
			undo, err := changes.Compensate(values)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := applyModelValues("PART", next, undo)
			if err != nil {
				t.Fatal(err)
			}
			var result PartModel
			_ = json.Unmarshal(restored, &result)
			if len(result.DatumPlanes) != 4 || len(result.DatumAxes) != 1 {
				t.Fatal("datum definition was not restored")
			}
			model.Features = []Feature{{ID: "consumer", Type: "REVOLVE", AxisEntityID: "DATUM_AXIS:custom", NeutralPlaneID: "custom"}}
			original, _ = json.Marshal(model)
			if _, _, err := applyDeletePartNode(original, payload); err == nil {
				t.Fatal("accepted deletion of referenced datum")
			}
		})
	}
	model := newPartModel()
	original, _ := json.Marshal(model)
	payload, _ := json.Marshal(deleteNodePayload{TargetKind: "DATUM_PLANE", TargetID: "datum-xy"})
	if _, _, err := applyDeletePartNode(original, payload); err == nil {
		t.Fatal("deleted standard plane")
	}
	model.DatumPlanes = append(model.DatumPlanes, DatumPlane{ID: "custom", Plane: "CUSTOM"})
	nodes := partStructureChildren(model, "root", "doc", "revision", true)
	if slices.Contains(nodes[0].Children[0].Capabilities, "DELETE") || !slices.Contains(nodes[0].Children[3].Capabilities, "DELETE") {
		t.Fatal("datum deletion capabilities disagree with model")
	}
}

func TestSolidFeatureSuppressionUsesDefinitionHistory(t *testing.T) {
	model := newPartModel()
	model.Bodies = append(model.Bodies, PartBody{ID: "tool", Name: "Tool", Visible: true})
	model.Features = []Feature{{ID: "base", BodyID: model.ActiveBodyID, Type: "IMPORT_BODY"}, {ID: "tool-stage", BodyID: "tool", Type: "IMPORT_BODY"}, {ID: "boolean", Type: "BOOLEAN", BodyID: model.ActiveBodyID, Operation: "REMOVE", Tools: []FeatureStageRef{{BodyID: "tool", FeatureID: "tool-stage"}}}}
	normalizePartModel(&model)
	original, _ := json.Marshal(model)
	digest, _ := featureDefinitionDigest(model, "boolean")
	payload, _ := json.Marshal(featureSuppressionPayload{FeatureID: "boolean", ExpectedFeatureDigest: digest, Suppressed: true})
	next, changes, err := applyFeatureSuppression(original, payload)
	if err != nil {
		t.Fatal(err)
	}
	var suppressed PartModel
	_ = json.Unmarshal(next, &suppressed)
	if !suppressed.Features[2].Suppressed || suppressed.Bodies[1].Consumed {
		t.Fatal("suppression failed to restore Boolean tool")
	}
	node := featureStructureNode(suppressed.Features[2], "root", "doc", "rev", digest, false, true)
	if !node.Suppressed || !slices.Contains(node.Capabilities, "SUPPRESS") {
		t.Fatal("tree lacks suppression state")
	}
	values, err := modelValues("PART", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	undo, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PART", next, undo)
	if err != nil {
		t.Fatal(err)
	}
	var result PartModel
	_ = json.Unmarshal(restored, &result)
	if result.Features[2].Suppressed || !result.Bodies[1].Consumed {
		t.Fatal("suppression compensation failed")
	}
	if _, _, err := applyFeatureSuppression(next, payload); err == nil {
		t.Fatal("accepted stale digest")
	}
}

func TestModelingFeatureSuppressionProjection(t *testing.T) {
	for _, kind := range []string{"LINEAR_EXTRUDE", "REVOLVE", "LOFT", "BOOLEAN", "FILLET", "CHAMFER", "DRAFT", "SHELL"} {
		feature := Feature{ID: "feature", Type: kind, Suppressed: true}
		for _, editable := range []bool{true, false} {
			node := featureStructureNode(feature, "root", "document", "revision", "digest", false, editable)
			if !node.Suppressed || slices.Contains(node.Capabilities, "SUPPRESS") != editable {
				t.Fatalf("%s suppression projection depends on editing permission: %+v", kind, node)
			}
		}
	}
}

func TestFailedBodyDisplayProvenanceAndRecovery(t *testing.T) {
	model := newPartModel()
	model.Bodies[0].GeometryKey = "success"
	previous := model
	failed := newPartModel()
	failed.Features = []Feature{{ID: "failed", BodyID: failed.ActiveBodyID, Type: "FILLET", EvaluationStatus: "FAILED"}}
	retainFailedBodyDisplay(&failed, previous, "success-revision")
	fallback := failed.Bodies[0].DisplayFallback
	if failed.Bodies[0].GeometryKey != "" || fallback == nil || fallback.GeometryKey != "success" || fallback.SourceVersionID != "success-revision" {
		t.Fatal("fallback must never become authoritative geometry")
	}
	again := failed
	again.Bodies = append([]PartBody(nil), failed.Bodies...)
	again.Bodies[0].DisplayFallback = nil
	retainFailedBodyDisplay(&again, failed, "failed-revision")
	if again.Bodies[0].DisplayFallback.SourceVersionID != "success-revision" {
		t.Fatal("repeated failure lost successful source")
	}
	again.Bodies[0].GeometryKey = "repaired"
	retainFailedBodyDisplay(&again, failed, "failed-revision")
	if again.Bodies[0].DisplayFallback != nil {
		t.Fatal("recovery retained stale display")
	}
	if bodyDefinition(failed.Bodies[0]).DisplayFallback != nil {
		t.Fatal("display provenance leaked into compensable definition")
	}
	a := fallbackVisualArtifact(Artifact{Representations: map[string]Representation{"VISUAL": {ObjectID: "visual"}, "NAMING": {Digest: "naming"}, "BREP": {ObjectID: "exact"}}})
	if len(a.Representations) != 1 || a.VisualNamingDigest != "naming" {
		t.Fatal("fallback exposed authoritative representations or lost visual association")
	}
}

func TestDraftNeutralFaceContractAndDependencies(t *testing.T) {
	model := newPartModel()
	model.Features = []Feature{{ID: "base", Type: "IMPORT_BODY", BodyID: model.ActiveBodyID}}
	pick := FeatureSelection{SourceVersionID: "source", SourceFeatureID: "base", Selection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: "doc", SourceBodyID: model.ActiveBodyID, Anchor: modelcore.SemanticTopologyRef{FeatureID: "base", OutputSlot: "face"}, ExpectedType: modelcore.PersistentTopologyFace, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: modelcore.TopologySelectionEvidence{GeometryType: "PLANE"}}}
	draft := Feature{ID: "draft", Type: "DRAFT", BodyID: model.ActiveBodyID, Angle: 5, NeutralPlane: &pick, Selections: []FeatureSelection{pick}}
	model.Features = append(model.Features, draft)
	normalizePartModel(&model)
	if err := validatePartStructure(model); err != nil {
		t.Fatal(err)
	}
	if _, _, err := buildPartEvaluation(model, "rev", "hash", nil, nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(featureInputIDs(draft), "base") {
		t.Fatal("neutral plane missing dependency")
	}
	draft.NeutralPlaneID = "datum-xy"
	if err := validateSolidStage(draft, map[string]Feature{"base": model.Features[0]}); err == nil {
		t.Fatal("accepted two neutral planes")
	}
	draft.NeutralPlaneID = ""
	pick.Selection.CreationEvidence.GeometryType = "CYLINDER"
	if err := validateSolidStage(draft, map[string]Feature{"base": model.Features[0]}); err == nil {
		t.Fatal("accepted nonplanar neutral face")
	}
}

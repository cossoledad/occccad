//go:build integration

package automation_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationFrozenSelectionRelocatesFormallyAfterHeadChanges(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor, document := access.DefaultUserID, view.Document.ID
	run := func(operation string, args any) automation.ActionResult {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(args)
		action := automation.ActionRequest{OperationID: operation, Version: 1, Arguments: raw, RequestID: uuid.NewString(), ExpectedHead: snapshot.CommandBaseline}
		preview, err := service.Prepare(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(err)
		}
		action.PreviewID = preview.PreviewID
		result, err := service.Commit(t.Context(), actor, document, action)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	sketch := run("sketch.create", map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})
	run("sketch.rectangle", map[string]any{"sketchId": sketch.Output.ID, "width": 80, "height": 50, "unit": "mm"})
	extrude := run("part.extrude", map[string]any{"bodyId": view.Part.ActiveBodyID, "sketchId": sketch.Output.ID, "length": 12, "unit": "mm"})
	topology, err := domain.ReadBodyTopology(t.Context(), document, extrude.Receipt.RevisionID, view.Part.ActiveBodyID, "EDGE")
	if err != nil || len(topology.Items) != 12 {
		t.Fatal(topology, err)
	}
	frozen := topology.Items[0].Selection
	frozenJSON, _ := json.Marshal(frozen)
	faces, err := domain.ReadBodyTopology(t.Context(), document, extrude.Receipt.RevisionID, view.Part.ActiveBodyID, "FACE")
	if err != nil || len(faces.Items) != 6 {
		t.Fatal(faces, err)
	}
	// A source identity is frozen before a later human operation changes Head.
	// The new geometry is evaluated by the original parameter Domain Command.
	model, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	var part workspace.PartModel
	if err = json.Unmarshal(model.Model, &part); err != nil {
		t.Fatal(err)
	}
	parameterID := ""
	for _, parameter := range part.Parameters {
		if parameter.OwnerFeatureID == extrude.Output.ID && parameter.PropertySlot == "length" {
			parameterID = parameter.ParameterID
		}
	}
	if parameterID == "" {
		t.Fatal("length parameter missing")
	}
	modified := run("parameter.set_literal", map[string]any{"parameterId": parameterID, "value": 20, "unit": "mm"})
	resolved, err := service.ResolveSelectionForSnapshotWithEvidence(t.Context(), actor, document, modified.Receipt.RevisionID, frozen)
	if err != nil || resolved.TopologyID == "" || resolved.SourceRevisionID != extrude.Receipt.RevisionID || resolved.TargetRevisionID != modified.Receipt.RevisionID || resolved.Selection.SourceVersionID != modified.Receipt.RevisionID || resolved.ManifestDigest == "" {
		t.Fatal("formal relocation failed", resolved, err)
	}
	unchangedJSON, _ := json.Marshal(frozen)
	if string(frozenJSON) != string(unchangedJSON) {
		t.Fatal("frozen selection changed while resolving")
	}
	page, err := service.QueryTopology(t.Context(), actor, document, automation.TopologyQuery{RevisionID: modified.Receipt.RevisionID, BodyID: view.Part.ActiveBodyID, Kind: "EDGE", IDs: []string{resolved.TopologyID}, IncludeSelections: true})
	if err != nil || page["matchedCount"] != 1 || page["selectionSetComplete"] != true {
		t.Fatal("relocated identity is not a formal current query identity", page, err)
	}
	wrong := frozen
	wrong.Selection.SourceDocumentID = uuid.NewString()
	if _, err = service.ResolveSelectionForSnapshotWithEvidence(t.Context(), actor, document, modified.Receipt.RevisionID, wrong); err == nil {
		t.Fatal("selection from a different owner document was accepted")
	}
	wrong = frozen
	wrong.Selection.Anchor.FeatureID = "missing-formal-anchor"
	if _, err = service.ResolveSelectionForSnapshotWithEvidence(t.Context(), actor, document, modified.Receipt.RevisionID, wrong); err == nil {
		t.Fatal("missing selection anchor was guessed")
	}
	protected := []automation.PartCheck{}
	for _, face := range faces.Items {
		source := face.Selection
		protected = append(protected, automation.PartCheck{ID: "protect-" + face.ID, Kind: "PROTECTED_SELECTION", Selection: &source})
	}
	evidence, err := service.VerifyPart(t.Context(), actor, document, automation.PartVerificationRequest{GoalVersion: 2, RevisionID: modified.Receipt.RevisionID, Checks: protected})
	if err != nil {
		t.Fatal(err)
	}
	passed, failed := 0, 0
	for _, check := range evidence.Checks {
		if check.Passed {
			passed++
		} else {
			failed++
		}
	}
	if passed == 0 || failed == 0 {
		t.Fatal("protected topology checks did not distinguish unchanged mounting geometry from changed geometry", evidence)
	}
	t.Logf("after 12→20 mm edit, protected source faces: %d unchanged, %d changed/unresolved", passed, failed)
	// A one-edge goal uses exactly the frozen selected subset, then verifies
	// formal selections rather than treating the first page as all edges.
	set := page["selectionSet"].(string)
	fillet := run("part.fillet", map[string]any{"bodyId": view.Part.ActiveBodyID, "selectionSet": set, "size": 2, "unit": "mm"})
	selected, err := service.VerifyPart(t.Context(), actor, document, automation.PartVerificationRequest{GoalVersion: 3, CreatedEntityIDs: []string{fillet.Output.ID}, Checks: []automation.PartCheck{{ID: "selected-edge", Kind: "SELECTED_EDGES_FILLET", Radius: 2, Unit: "mm", ExpectedCount: 1, SelectionSet: set}, {ID: "all-edges", Kind: "ALL_EDGES_FILLET", Radius: 2, Unit: "mm", SelectionSet: set}}})
	if err != nil || !selected.Checks[0].Passed || selected.Checks[1].Passed {
		t.Fatal("selected-edge goal and all-edge goal were conflated", selected, err)
	}
	t.Logf("frozen edge from %s formally relocated to %s using manifest %s", extrude.Receipt.RevisionID, modified.Receipt.RevisionID, resolved.ManifestDigest)
}

func TestAutomationHandoffCompletesOnlyOriginalDomainReceipt(t *testing.T) {
	db, domain, view := automationFixture(t)
	service := automation.New(db, domain, access.New(db))
	actor, document := access.DefaultUserID, view.Document.ID
	snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(t.Context(), actor, document, automation.CreateTask{RequestID: uuid.NewString(), Goal: "Use original sketch tool", ExecutionMode: "AUTOMATIC", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "ui.sketch.start", Version: 1, Arguments: json.RawMessage(`{}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	task, err = service.Control(t.Context(), actor, document, task.ID, automation.Control{Action: "handoff", ExpectedVersion: task.Version, StepID: task.Steps[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	handoff := task.Steps[0].Handoff
	input := automation.Control{Action: "ui_complete", ExpectedVersion: task.Version, StepID: task.Steps[0].ID, HandoffID: handoff.ID, DomainRequestID: handoff.DomainRequestID}
	if _, err = service.Control(t.Context(), actor, document, task.ID, input); err == nil {
		t.Fatal("opening tool without a domain receipt completed the stage")
	}
	// Exercise the same native command that the existing workbench commits,
	// carrying the pre-registered request identity and original CAS baseline.
	action := automation.ActionRequest{OperationID: "sketch.create", Version: 1, Arguments: json.RawMessage(`{"bodyId":"` + view.Part.ActiveBodyID + `","plane":"XY"}`), ExpectedHead: handoff.Baseline, RequestID: handoff.DomainRequestID}
	preview, err := service.Prepare(t.Context(), actor, document, action)
	if err != nil {
		t.Fatal(err)
	}
	action.PreviewID = preview.PreviewID
	receipt, err := service.Commit(t.Context(), actor, document, action)
	if err != nil {
		t.Fatal(err)
	}
	task, err = service.Control(t.Context(), actor, document, task.ID, input)
	if err != nil || task.Status != "COMPLETED" || task.Steps[0].Phase != "UI_COMPLETED" || task.Steps[0].Result == nil || task.Steps[0].Result.Receipt.TransactionID != receipt.Receipt.TransactionID || task.Steps[0].Presentation.Status != "NOT_PRESENTED" {
		t.Fatal("exact original receipt was not recognized", task, err)
	}
}

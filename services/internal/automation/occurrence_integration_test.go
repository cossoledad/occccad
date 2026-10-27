//go:build integration

package automation_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

func TestAutomationOccurrenceCandidateRecipesAndInsertedInstanceRecovery(t *testing.T) {
	db, domain, _ := automationFixture(t)
	actor := uuid.NewString()
	if _, err := db.Exec(t.Context(), `INSERT INTO occccad.users(id,email,display_name) VALUES($1,$2,'Product automation acceptance')`, actor, actor+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	s := automation.New(db, domain, access.New(db))
	create := func(kind string) workspace.DocumentView {
		t.Helper()
		v, e := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: kind, Name: "Product automation " + uuid.NewString()})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	run := func(document, operation string, args any) automation.ActionResult {
		t.Helper()
		snapshot, e := domain.ReadModelSnapshot(t.Context(), document, "")
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(args)
		input := automation.ActionRequest{RequestID: uuid.NewString(), OperationID: operation, Version: 1, Arguments: raw, ExpectedHead: snapshot.CommandBaseline}
		preview, e := s.Prepare(t.Context(), actor, document, input)
		if e != nil {
			t.Fatal(operation, e)
		}
		input.PreviewID = preview.PreviewID
		result, e := s.Commit(t.Context(), actor, document, input)
		if e != nil {
			t.Fatal(operation, e)
		}
		return result
	}
	cylinder := func(outer, inner, length float64) workspace.DocumentView {
		t.Helper()
		v := create("PART")
		sketch := run(v.Document.ID, "command.create_sketch", map[string]any{"bodyId": v.Part.ActiveBodyID, "plane": "XY"})
		ops := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: outer}}}
		if inner > 0 {
			ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: inner}})
		}
		run(v.Document.ID, "command.edit_sketch", map[string]any{"sketchId": sketch.Output.ID, "operations": ops})
		run(v.Document.ID, "command.create_solid_feature", map[string]any{"bodyId": v.Part.ActiveBodyID, "sketchId": sketch.Output.ID, "generator": "LINEAR_EXTRUDE", "operation": "ADD", "length": length})
		return v
	}
	shaft, sleeve := cylinder(5, 0, 35), cylinder(12, 5.1, 20)
	product := create("PRODUCT")
	reference := run(product.Document.ID, "command.insert_instance", map[string]any{"referencedDocumentId": sleeve.Document.ID}).Output.ID
	runID := uuid.NewString()
	snapshot, err := domain.ReadModelSnapshot(t.Context(), product.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	insertInput := automation.CreateTask{RequestID: uuid.NewString(), RunID: runID, PlanEpoch: 1, ExecutionMode: "AUTOMATIC", Goal: "Insert shaft, observe geometry, mate to sleeve", ExpectedHead: snapshot.CommandBaseline, Steps: []automation.StepSpec{{ID: uuid.NewString(), OperationID: "command.insert_instance", Version: 1, Arguments: mustOccurrenceJSON(map[string]any{"referencedDocumentId": shaft.Document.ID})}}}
	insertTask, err := s.CreateTask(t.Context(), actor, product.Document.ID, insertInput)
	if err != nil {
		t.Fatal(err)
	}
	finish := func(task automation.Task) automation.Task {
		t.Helper()
		for n := 0; n < 40 && task.Status == "ACTIVE"; n++ {
			var e error
			task, e = s.Advance(t.Context(), actor, product.Document.ID, task.ID, task.Version)
			if e != nil {
				t.Fatal(e)
			}
		}
		if task.Status != "COMPLETED" {
			t.Fatalf("task did not complete: %+v", task)
		}
		return task
	}
	insertTask = finish(insertTask)
	if insertTask.Steps[0].Result == nil || insertTask.Steps[0].Result.Output == nil {
		t.Fatal("insert receipt lost stable instance output")
	}
	moving := insertTask.Steps[0].Result.Output.ID
	recovered, err := s.CreateTask(t.Context(), actor, product.Document.ID, insertInput)
	if err != nil || recovered.ID != insertTask.ID || recovered.Steps[0].Result.Output.ID != moving {
		t.Fatal("insert stage replay did not recover exact occurrence", recovered, err)
	}
	angle := 37 * math.Pi / 180
	run(product.Document.ID, "command.move_instance", map[string]any{"instanceId": moving, "translation": [3]float64{47, -31, 19}, "rotation": [4]float64{0, math.Sin(angle / 2), 0, math.Cos(angle / 2)}})
	snapshot, err = domain.ReadModelSnapshot(t.Context(), product.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	query := automation.OccurrenceQuery{RevisionID: snapshot.RevisionID, InstanceIDs: []string{moving}, Kind: "FACE", Limit: 1}
	firstPage, err := s.QueryOccurrence(t.Context(), actor, product.Document.ID, query)
	if err != nil || len(firstPage.Items) != 1 || !firstPage.HasMore || firstPage.Complete {
		t.Fatal("exact occurrence query did not page", firstPage, err)
	}
	query.Cursor = firstPage.NextCursor
	next, err := s.QueryOccurrence(t.Context(), actor, product.Document.ID, query)
	if err != nil || next.Items[0].ID == firstPage.Items[0].ID {
		t.Fatal("geometry page duplicated support", next, err)
	}
	query.InstanceIDs = []string{reference}
	if _, err = s.QueryOccurrence(t.Context(), actor, product.Document.ID, query); err == nil {
		t.Fatal("cursor crossed occurrence boundary")
	}
	candidates, err := s.MateCandidates(t.Context(), actor, product.Document.ID, automation.MateCandidateQuery{RevisionID: snapshot.RevisionID, FirstInstanceIDs: []string{moving}, SecondInstanceIDs: []string{reference}, AxisDirectionRelation: "SAME", PlaneDirectionRelation: "SAME", Limit: 8})
	if err != nil {
		t.Fatal(err)
	}
	if candidates.MatchedCount != 2 || !candidates.RequiresSelection || !candidates.Complete {
		t.Fatalf("flush-end choices should be explicitly ambiguous: %+v", candidates)
	}
	encodedCandidates, _ := json.Marshal(candidates)
	t.Logf("bounded native mate candidates: %d bytes", len(encodedCandidates))
	if len(encodedCandidates) > 32<<10 {
		t.Fatal("candidate page exceeds the actual Run tool-result budget")
	}
	var wireCandidates automation.MateCandidatePage
	if err = json.Unmarshal(encodedCandidates, &wireCandidates); err != nil {
		t.Fatal(err)
	}
	var chosen *automation.MateCandidate
	for i := range wireCandidates.Items {
		candidate := &wireCandidates.Items[i]
		if math.Abs(candidate.MovingEndCoordinateMM) < 1e-8 && math.Abs(candidate.ReferenceEndCoordinateMM) < 1e-8 {
			if chosen != nil {
				t.Fatal("end choice ambiguous by geometric criterion")
			}
			chosen = candidate
		}
	}
	if chosen == nil || math.Abs(chosen.RadialClearanceMM-.1) > 1e-9 || chosen.FirstMeasuredAxis.MaterialSide != 1 || chosen.SecondMeasuredAxis.MaterialSide != -1 {
		t.Fatal("native candidate facts or original-command recipe invalid", chosen)
	}
	if chosen.InitialAxisDistanceMM < 1 || chosen.InitialAxisAngleRad < .5 {
		t.Fatal("fixture did not start substantially displaced", chosen)
	}
	recipePage, err := s.MateCandidates(t.Context(), actor, product.Document.ID, automation.MateCandidateQuery{RevisionID: snapshot.RevisionID, FirstInstanceIDs: []string{moving}, SecondInstanceIDs: []string{reference}, AxisDirectionRelation: "SAME", PlaneDirectionRelation: "SAME", CandidateID: chosen.ID, IncludeRecipe: true})
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(recipePage)
	if len(wire) > 32<<10 {
		t.Fatal("selected recipe exceeds actual Run tool-result budget")
	}
	var recipeResult struct {
		Items []struct {
			ID           string                               `json:"id"`
			Recipe       []automation.MateRecipeStep          `json:"recipe"`
			Verification automation.AssemblyVerificationInput `json:"verification"`
		} `json:"items"`
	}
	if err = json.Unmarshal(wire, &recipeResult); err != nil || len(recipeResult.Items) != 1 || recipeResult.Items[0].ID != chosen.ID || len(recipeResult.Items[0].Recipe) != 3 {
		t.Fatal("selected original-command recipe did not survive tool JSON", recipeResult, err)
	}
	steps := []automation.StepSpec{}
	for _, recipe := range recipeResult.Items[0].Recipe {
		steps = append(steps, automation.StepSpec{ID: uuid.NewString(), OperationID: recipe.OperationID, Version: recipe.Version, Arguments: mustOccurrenceJSON(recipe.Arguments)})
	}
	mateInput := automation.CreateTask{RequestID: uuid.NewString(), RunID: runID, PlanEpoch: 2, ExecutionMode: "AUTOMATIC", Goal: insertInput.Goal, ExpectedHead: snapshot.CommandBaseline, Steps: steps}
	task, err := s.CreateTask(t.Context(), actor, product.Document.ID, mateInput)
	if err != nil {
		t.Fatal(err)
	}
	task = finish(task)
	verification := recipeResult.Items[0].Verification
	verification.RevisionID = task.Baseline.RevisionID
	result, err := s.VerifyAssembly(t.Context(), actor, product.Document.ID, verification)
	if err != nil || !result.Passed {
		t.Fatalf("native candidate recipe failed production goal verification: %+v %v", result, err)
	}
	view, err := domain.GetDocument(t.Context(), product.Document.ID)
	if err != nil || len(view.Product.Instances) != 2 {
		t.Fatal("insert replay created extra occurrences", view.Product, err)
	}
	for _, step := range task.Steps {
		if step.Result == nil || step.Phase != "APPLIED" || step.Presentation.Status != "NOT_PRESENTED" {
			t.Fatal("headless task fabricated presentation or lost receipt", step)
		}
	}
	t.Logf("two Automation stages share Run %s: insert recovered exact instance %s; %d candidates, explicit geometric end selected; solver=%s DOF=%d", runID, moving, candidates.MatchedCount, result.SolverStatus, result.RelativeDOF)
}

func mustOccurrenceJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}

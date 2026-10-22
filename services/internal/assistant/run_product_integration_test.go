package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/workspace"
)

// The provider is an explicit deterministic fixture. Geometry, transactions,
// run recovery, tool execution and optional verification use production code;
// this test makes no claim about a live model's ability to plan the assembly.
type productRunProvider struct {
	calls                     int
	shaft, reference          string
	rootRun, moving, revision string
	occurrenceCall            string
	candidateCall             string
	recipeCall                string
	verificationCall          string
	selected                  automation.MateCandidate
	verification              automation.AssemblyVerificationInput
}

func (*productRunProvider) Validate(context.Context, string) error { return nil }
func (*productRunProvider) Models(context.Context, string) ([]Model, error) {
	return []Model{{ID: "deepseek-flash"}}, nil
}

type productRunPacket struct {
	Current automation.Context `json:"current"`
	Run     struct {
		ID string `json:"id"`
	} `json:"run"`
	RecentStages []struct {
		Baseline workspace.CommandBaseline `json:"baseline"`
		Steps    []struct {
			OperationID string                 `json:"operationId"`
			Output      *automation.EntityRef  `json:"output"`
			Outputs     []automation.EntityRef `json:"outputs"`
		} `json:"steps"`
	} `json:"recentStages"`
}

func productToolResult(messages []Message, id string, result any) error {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" && messages[i].ToolCallID == id {
			var failure struct{ Code, Message string }
			_ = json.Unmarshal([]byte(messages[i].Content), &failure)
			if failure.Code != "" {
				return fmt.Errorf("production tool returned %s: %s", failure.Code, failure.Message)
			}
			return json.Unmarshal([]byte(messages[i].Content), result)
		}
	}
	return fmt.Errorf("missing real tool response %s", id)
}

func (p *productRunProvider) Complete(_ context.Context, _ string, messages []Message, _ []Tool, _ ...Generation) (Completion, error) {
	p.calls++
	var packet productRunPacket
	if len(messages) < 2 || json.Unmarshal([]byte(messages[1].Content), &packet) != nil || packet.Run.ID == "" || packet.Current.Snapshot.DocumentType != "PRODUCT" {
		return Completion{}, fmt.Errorf("call %d lacks authoritative Product context", p.calls)
	}
	if p.rootRun == "" {
		p.rootRun = packet.Run.ID
	} else if p.rootRun != packet.Run.ID {
		return Completion{}, fmt.Errorf("planning escaped original durable Run")
	}
	respond := func(name string, input any, id *string) (Completion, error) {
		m := toolMessage(name, input)
		if id != nil {
			*id = m.ToolCalls[0].ID
		}
		return Completion{Message: m, Usage: &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}}, nil
	}
	candidateInput := func() automation.MateCandidateQuery {
		return automation.MateCandidateQuery{RevisionID: p.revision, FirstInstanceIDs: []string{p.moving}, SecondInstanceIDs: []string{p.reference}, AxisDirectionRelation: "SAME", PlaneDirectionRelation: "SAME", Limit: 8}
	}
	switch p.calls {
	case 1:
		m := Message{Role: "assistant"}
		for _, operation := range []string{"command.insert_instance", "command.move_instance", "command.add_assembly_constraint"} {
			m.ToolCalls = append(m.ToolCalls, toolMessage("cad_describe_operation", map[string]any{"operationId": operation}).ToolCalls...)
		}
		return Completion{Message: m, Usage: &Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}}, nil
	case 2:
		angle := 37 * math.Pi / 180
		return respond("cad_create_task", map[string]any{"steps": []automation.StepSpec{
			{ID: "insert-shaft", OperationID: "command.insert_instance", Version: 1, Arguments: marshalJSON(map[string]any{"referencedDocumentId": p.shaft})},
			{ID: "displace-shaft", OperationID: "command.move_instance", Version: 1, Arguments: marshalJSON(map[string]any{"translation": [3]float64{47, -31, 19}, "rotation": [4]float64{0, math.Sin(angle / 2), 0, math.Cos(angle / 2)}}), Bindings: map[string]string{"instanceId": "insert-shaft"}},
		}}, nil)
	case 3:
		// The inserted occurrence is taken from the prior stage's actual receipt,
		// after rebuilding the Service, rather than from a fabricated future ID.
		for _, task := range packet.RecentStages {
			if task.Baseline.WorkspaceID != packet.Current.Snapshot.WorkspaceID {
				return Completion{}, fmt.Errorf("stage belongs to another workspace")
			}
			for _, step := range task.Steps {
				if step.OperationID != "command.insert_instance" {
					continue
				}
				outputs := step.Outputs
				if step.Output != nil {
					outputs = append(outputs, *step.Output)
				}
				for _, output := range outputs {
					if output.Kind != "INSTANCE" {
						continue
					}
					if p.moving != "" && p.moving != output.ID {
						return Completion{}, fmt.Errorf("insert output is ambiguous")
					}
					p.moving = output.ID
				}
			}
		}
		if p.moving == "" {
			return Completion{}, fmt.Errorf("inserted instance receipt was absent from bounded durable context")
		}
		p.revision = packet.Current.Snapshot.RevisionID
		return respond("cad_query_occurrence", automation.OccurrenceQuery{RevisionID: p.revision, InstanceIDs: []string{p.moving}, Kind: "FACE", GeometryType: "CYLINDER", Limit: 1}, &p.occurrenceCall)
	case 4:
		var observed automation.OccurrencePage
		if err := productToolResult(messages, p.occurrenceCall, &observed); err != nil {
			return Completion{}, err
		}
		if observed.SourceDocumentID != p.shaft || len(observed.Items) != 1 || observed.Items[0].Kind != "CYLINDER" || observed.Items[0].Reference.PersistentSelection == nil || observed.Items[0].AxialRangeMM == nil || math.Abs(observed.Items[0].Definition.Radius-5) > 1e-8 || observed.WorldPose.Translation != [3]float64{47, -31, 19} {
			return Completion{}, fmt.Errorf("new occurrence did not expose real definition/world geometry and formal reference")
		}
		return respond("cad_mate_candidates", candidateInput(), &p.candidateCall)
	case 5:
		var candidates automation.MateCandidatePage
		if err := productToolResult(messages, p.candidateCall, &candidates); err != nil {
			return Completion{}, err
		}
		if candidates.MatchedCount != 2 || !candidates.Complete || !candidates.RequiresSelection {
			return Completion{}, fmt.Errorf("flush-end ambiguity was not explicit")
		}
		for _, candidate := range candidates.Items {
			if math.Abs(candidate.MovingEndCoordinateMM) < 1e-8 && math.Abs(candidate.ReferenceEndCoordinateMM) < 1e-8 {
				if p.selected.ID != "" {
					return Completion{}, fmt.Errorf("user's explicit zero-end geometric criterion remains ambiguous")
				}
				p.selected = candidate
			}
		}
		if p.selected.ID == "" || p.selected.InitialAxisDistanceMM < 1 || p.selected.InitialAxisAngleRad < .5 || math.Abs(p.selected.RadialClearanceMM-.1) > 1e-9 {
			return Completion{}, fmt.Errorf("zero-end choice lacks measured displaced shaft/hole evidence")
		}
		input := candidateInput()
		input.CandidateID, input.IncludeRecipe = p.selected.ID, true
		return respond("cad_mate_candidates", input, &p.recipeCall)
	case 6:
		var recipe struct {
			Items []struct {
				ID           string                               `json:"id"`
				Recipe       []automation.MateRecipeStep          `json:"recipe"`
				Verification automation.AssemblyVerificationInput `json:"verification"`
			} `json:"items"`
		}
		if err := productToolResult(messages, p.recipeCall, &recipe); err != nil {
			return Completion{}, err
		}
		if len(recipe.Items) != 1 || recipe.Items[0].ID != p.selected.ID || len(recipe.Items[0].Recipe) != 3 {
			return Completion{}, fmt.Errorf("selected candidate did not return original-command recipe")
		}
		p.verification = recipe.Items[0].Verification
		steps := make([]automation.StepSpec, 0, 3)
		for i, step := range recipe.Items[0].Recipe {
			steps = append(steps, automation.StepSpec{ID: fmt.Sprintf("mate-%d", i), OperationID: step.OperationID, Version: step.Version, Arguments: marshalJSON(step.Arguments)})
		}
		return respond("cad_create_task", map[string]any{"steps": steps}, nil)
	case 7:
		if packet.Current.Snapshot.RevisionID == p.revision {
			return Completion{}, fmt.Errorf("mate stage did not advance authoritative Product revision")
		}
		p.verification.RevisionID = packet.Current.Snapshot.RevisionID
		return respond("cad_verify_assembly", p.verification, &p.verificationCall)
	case 8:
		var verified automation.AssemblyVerification
		if err := productToolResult(messages, p.verificationCall, &verified); err != nil {
			return Completion{}, err
		}
		if !verified.Passed || verified.RevisionID != packet.Current.Snapshot.RevisionID {
			return Completion{}, fmt.Errorf("application goal verification rejected actual assembly: %+v", verified)
		}
		return respond("cad_finish", map[string]any{"summary": "已按双方0 mm端面的明确选择完成同轴与同向端面定位，保留绕轴一个旋转自由度。"}, nil)
	default:
		return Completion{}, fmt.Errorf("unexpected fixture provider request %d", p.calls)
	}
}

func TestAssistantProductSameDurableRunInsertObserveMateAndVerify(t *testing.T) {
	s, domain, shaft, actor := assistantFixture(t, "http://127.0.0.1:1")
	create := func(kind string) workspace.DocumentView {
		t.Helper()
		v, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Type: kind, Name: "Assistant Product native fixture " + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	action := func(document, operation string, arguments any) automation.ActionResult {
		t.Helper()
		snapshot, err := domain.ReadModelSnapshot(t.Context(), document, "")
		if err != nil {
			t.Fatal(err)
		}
		input := automation.ActionRequest{RequestID: uuid.NewString(), OperationID: operation, Version: 1, Arguments: marshalJSON(arguments), ExpectedHead: snapshot.CommandBaseline}
		preview, err := s.automation.Prepare(t.Context(), actor, document, input)
		if err != nil {
			t.Fatal(operation, err)
		}
		input.PreviewID = preview.PreviewID
		result, err := s.automation.Commit(t.Context(), actor, document, input)
		if err != nil {
			t.Fatal(operation, err)
		}
		return result
	}
	cylinder := func(v workspace.DocumentView, outer, inner, length float64) {
		t.Helper()
		sketch := action(v.Document.ID, "command.create_sketch", map[string]any{"bodyId": v.Part.ActiveBodyID, "plane": "XY"})
		entities := []workspace.SketchOperation{{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: outer}}}
		if inner > 0 {
			entities = append(entities, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &workspace.SketchEntity{ID: uuid.NewString(), Kind: "CIRCLE", Role: "PROFILE", Center: &workspace.SketchPoint2{}, Radius: inner}})
		}
		action(v.Document.ID, "command.edit_sketch", map[string]any{"sketchId": sketch.Output.ID, "operations": entities})
		action(v.Document.ID, "command.create_solid_feature", map[string]any{"bodyId": v.Part.ActiveBodyID, "sketchId": sketch.Output.ID, "generator": "LINEAR_EXTRUDE", "operation": "ADD", "length": length})
	}
	cylinder(shaft, 5, 0, 35)
	sleeve, product := create("PART"), create("PRODUCT")
	cylinder(sleeve, 12, 5.1, 20)
	reference := action(product.Document.ID, "command.insert_instance", map[string]any{"referencedDocumentId": sleeve.Document.ID}).Output.ID
	p := &productRunProvider{shaft: shaft.Document.ID, reference: reference}
	s.provider = p
	if _, err := s.SaveKey(t.Context(), actor, "owned-deterministic-product-fixture-credential"); err != nil {
		t.Fatal(err)
	}
	input := RunInput{RequestID: uuid.NewString(), Text: fmt.Sprintf("将已有轴零件 %s 插入本Product，固定套筒实例 %s；将新轴与套筒同轴装配，要求轴线方向同向(SAME)、双方轴向坐标0 mm的端面方向同向(SAME)，端面偏置0 mm，目标自由度为1，仅保留绕轴1个旋转自由度。", shaft.Document.ID, reference)}
	r, err := s.CreateRun(t.Context(), actor, product.Document.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	rootRun := r.ID
	if r.AccessMode != "WRITE" || r.Goal != input.Text {
		t.Fatal("original request was not retained", r)
	}
	restarted := false
	for ticks := 0; ticks < 80 && !terminal(r.Status); ticks++ {
		r, err = s.Step(t.Context(), actor, product.Document.ID, rootRun)
		if err != nil {
			t.Fatal("production Step", ticks, r.Status, err)
		}
		if r.ID != rootRun || (!runnable(r.Status) && !terminal(r.Status)) {
			t.Fatalf("same-Run production scheduler stopped before verification: %+v", r)
		}
		if !restarted && r.StageCount == 1 && r.ActiveTaskID == "" && r.StepCount == 2 {
			// Exercise durable recovery between insert and geometry observation.
			// The same request is also replayed through the public Run API.
			s.Close()
			s = New(s.db, s.automation, s.options)
			s.provider = p
			t.Cleanup(s.Close)
			recovered, e := s.CreateRun(t.Context(), actor, product.Document.ID, input)
			if e != nil || recovered.ID != rootRun || recovered.StageCount != 1 || recovered.StepCount != 2 {
				t.Fatal("durable root Run did not recover exact insert checkpoint", recovered, e)
			}
			r, restarted = recovered, true
		}
	}
	if !restarted || p.rootRun != rootRun || p.calls != 8 || r.Status != "COMPLETED" || r.StageCount != 2 || r.StepCount != 5 || r.AppliedCount != 5 || r.PresentedRevisionID != "" {
		t.Fatal("durable assembly Run did not finish with exact receipts and authoritative goal verification", r, "fixture calls", p.calls)
	}
	stages, err := s.Stages(t.Context(), actor, product.Document.ID, rootRun, 0, 0, 10)
	if err != nil || len(stages.Items) != 2 {
		t.Fatal("durable stages missing", stages, err)
	}
	insertCount := 0
	for _, stage := range stages.Items {
		task, e := s.automation.GetTask(t.Context(), actor, product.Document.ID, stage.TaskID)
		if e != nil || task.RunID != rootRun || task.Status != "COMPLETED" {
			t.Fatal("stage not bound to same durable Assistant Run", task, e)
		}
		for _, step := range task.Steps {
			if step.OperationID == "command.insert_instance" {
				insertCount++
			}
			if step.Result == nil || step.Phase != "APPLIED" || !step.ReceiptRecorded || step.Presentation.Status != "NOT_PRESENTED" {
				t.Fatal("native step missing observed receipt or fabricated presentation", step)
			}
		}
	}
	actual, err := domain.GetDocument(t.Context(), product.Document.ID)
	if err != nil || actual.Product == nil || len(actual.Product.Instances) != 2 || len(actual.Product.Constraints) != 3 || insertCount != 1 {
		t.Fatal("restart duplicated insert or failed to persist exact mate constraints", actual.Product, insertCount, err)
	}
	p.verification.RevisionID = r.LatestRevisionID
	verified, err := s.automation.VerifyAssembly(t.Context(), actor, product.Document.ID, p.verification)
	if err != nil || !verified.Passed || !verified.CurrentHead || verified.SolverStatus != "CONVERGED" || verified.RelativeDOF != 1 || verified.TranslationDOF != 0 || verified.RotationDOF != 1 || verified.RotationAxis == nil {
		t.Fatal("independent real geometry/solver verification failed", verified, err)
	}
	t.Logf("DETERMINISTIC_PROVIDER_FIXTURE with native Worker: durable Assistant Run=%s, recovered once, stages=%d, applied=%d, inserts=%d, instances=%d; initial axis distance=%g mm angle=%g rad; candidate=%s chosen by both zero-coordinate ends; solver=%s axis distance=%g mm angle=%g rad plane offset=%g mm, relativeDOF=%d translationDOF=%d rotationDOF=%d; goal=%s revision=%s; presentation not claimed", rootRun, r.StageCount, r.AppliedCount, insertCount, len(actual.Product.Instances), p.selected.InitialAxisDistanceMM, p.selected.InitialAxisAngleRad, p.selected.ID, verified.SolverStatus, verified.AxisDistanceMM, verified.AxisAngleRad, verified.PlaneOffsetMM, verified.RelativeDOF, verified.TranslationDOF, verified.RotationDOF, "on-demand", r.LatestRevisionID)
}

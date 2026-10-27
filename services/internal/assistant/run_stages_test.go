//go:build integration

package assistant

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/automation"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func stageFixture(t *testing.T) (*Service, *workspace.Service, workspace.DocumentView, Run) {
	t.Helper()
	db := testsupport.OpenDevelopmentDatabase(t)
	worker, closeWorker, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(closeWorker)
	store, staging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	domain := workspace.NewWithArtifacts(db, worker, artifact.NewService(db, store, staging))
	view, err := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: access.DefaultUserID, Type: "PART", Name: "Run stages " + uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	s := New(db, automation.New(db, domain, access.New(db)), Options{KeyFile: filepath.Join(t.TempDir(), "key")})
	t.Cleanup(s.Close)
	snapshot, err := domain.ReadModelSnapshot(t.Context(), view.Document.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	r := Run{ID: uuid.NewString(), ActorID: access.DefaultUserID, DocumentID: view.Document.ID, AccessMode: "WRITE", Version: 1, Epoch: 1, GoalVersion: 1, Goal: "在 XY 平面建立 80×50 mm 矩形，拉伸 12 mm，再对全部边做 R2 圆角", Status: "PLANNING", Mode: "AUTOMATIC", Scope: FrozenScope{RootDocumentID: view.Document.ID, DocumentID: view.Document.ID, RevisionID: snapshot.RevisionID, WorkingBodyID: view.Part.ActiveBodyID, Complete: true, Selections: []SelectedReference{}}, LatestRevisionID: snapshot.RevisionID, CreatedAt: timestamp(s.now()), UpdatedAt: timestamp(s.now())}
	turn := Turn{ID: uuid.NewString(), RunID: r.ID, Sequence: 1, RequestID: uuid.NewString(), Text: r.Goal, Kind: "GOAL", Status: "APPLIED", Scope: r.Scope, GoalVersion: 1, CreatedAt: r.CreatedAt}

	r.CurrentTurnID = turn.ID
	turn.Usage = &TurnUsage{}
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.assistant_runs(id,actor_id,document_id,request_id,request_digest,access_mode,version,epoch,status,payload,created_at,updated_at) VALUES($1,$2,$3,$4,$5,'WRITE',$6,$7,$8,$9,$10,$10)`, r.ID, r.ActorID, r.DocumentID, uuid.NewString(), "controlled-stage-fixture", r.Version, r.Epoch, r.Status, marshalJSON(r), r.CreatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(t.Context(), `INSERT INTO occccad.assistant_inputs(id,run_id,sequence,request_id,request_digest,kind,status,payload) VALUES($1,$2,1,$3,$4,'GOAL','APPLIED',$5)`, turn.ID, r.ID, turn.RequestID, "controlled-stage-fixture", marshalJSON(turn)); err != nil {
		t.Fatal(err)
	}
	return s, domain, view, r
}

// This executor supplies synthetic immutable definitions and explicit receipts.
// It tests scheduler capacity and accounting; the native tests above establish
// the actual CAD path. It does not pretend these synthetic sketches are solids.
type syntheticStageWorkspace struct {
	mu         sync.Mutex
	head       workspace.ModelSnapshot
	versions   map[string]workspace.ModelSnapshot
	receipts   map[string]workspace.CommandReceipt
	candidates map[string]workspace.CommandBaseline
	commits    int
}

func (w *syntheticStageWorkspace) ReadModelSnapshot(ctx context.Context, document, revision string) (workspace.ModelSnapshot, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if revision == "" {
		revision = w.head.RevisionID
	}
	snapshot, ok := w.versions[revision]
	if !ok || document != snapshot.DocumentID {
		return workspace.ModelSnapshot{}, workspace.ErrNotFound
	}
	return snapshot, ctx.Err()
}
func (w *syntheticStageWorkspace) PreviewCommand(ctx context.Context, document string, request workspace.CommandRequest) (workspace.CommandPreview, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if request.ExpectedHead == nil || *request.ExpectedHead != w.head.CommandBaseline {
		return workspace.CommandPreview{}, &automation.Fault{Code: "WORKSPACE_HEAD_CONFLICT", Message: "synthetic authoritative baseline changed"}
	}
	id := uuid.NewString()
	w.candidates[id] = *request.ExpectedHead
	return workspace.CommandPreview{PreviewID: id, BaseVersionID: w.head.RevisionID, BaseSequence: w.head.Sequence}, ctx.Err()
}
func (w *syntheticStageWorkspace) ExecuteCommand(ctx context.Context, document string, request workspace.CommandRequest) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := w.receipts[request.RequestID]; ok {
		return nil
	}
	baseline, ok := w.candidates[request.PreviewID]
	if !ok || request.ExpectedHead == nil || baseline != *request.ExpectedHead || baseline != w.head.CommandBaseline {
		return &automation.Fault{Code: "WORKSPACE_HEAD_CONFLICT", Message: "synthetic candidate changed"}
	}
	delete(w.candidates, request.PreviewID)
	var part workspace.PartModel
	if err := json.Unmarshal(w.head.Model, &part); err != nil {
		return err
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(request.RequestID)).String()
	part.Features = append(part.Features, workspace.Feature{ID: id, Type: "SKETCH", BodyID: request.BodyID, Sketch: &workspace.SketchFeature{SchemaVersion: 2, Support: workspace.SketchSupport{Plane: request.Plane}}, Plane: request.Plane})
	next := w.head
	next.RevisionID = uuid.NewString()
	next.Sequence++
	next.Model = marshalJSON(part)
	next.ModelHash = modelcore.ValueDigest(next.Model)
	w.head = next
	w.versions[next.RevisionID] = next
	w.receipts[request.RequestID] = workspace.CommandReceipt{CommandBaseline: next.CommandBaseline, TransactionID: uuid.NewString(), RequestID: request.RequestID, Changes: []workspace.CommandChangeRef{{Kind: "CREATE", EntityID: id, SlotID: "entity"}}}
	w.commits++
	return nil
}
func (w *syntheticStageWorkspace) FindCommandReceipt(_ context.Context, document, actor, requestID string) (workspace.CommandReceipt, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	receipt, ok := w.receipts[requestID]
	if !ok {
		return receipt, workspace.ErrNotFound
	}
	return receipt, nil
}
func (w *syntheticStageWorkspace) DiscardPreview(_, _, id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.candidates, id)
}

func TestAssistantRunStagesHaveNoTotalStepOrRunDeadlineLimit(t *testing.T) {
	s, domain, view, r := stageFixture(t)
	initial, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	controlled := &syntheticStageWorkspace{head: initial, versions: map[string]workspace.ModelSnapshot{initial.RevisionID: initial}, receipts: map[string]workspace.CommandReceipt{}, candidates: map[string]workspace.CommandBaseline{}}
	s.automation = automation.New(s.db, controlled, access.New(s.db))
	clock := s.now()
	s.options.Now = func() time.Time { return clock }
	started := clock
	for segment := 0; segment < 3; segment++ {
		if segment > 0 {
			clock = clock.Add(13 * time.Minute)
		}
		steps := make([]automation.StepSpec, 16)
		for i := range steps {
			steps[i] = automation.StepSpec{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}
		}
		r = stageForTest(t, s, r, steps)
		for r.ActiveTaskID != "" {
			r, err = s.Step(t.Context(), r.ActorID, r.DocumentID, r.ID)
			if err != nil {
				t.Fatal(err)
			}
		}
		if r.Status != "PLANNING" {
			t.Fatal("bounded stage became a total Run termination condition", r)
		}
	}
	if clock.Sub(started) <= 12*time.Minute || r.StageCount != 3 || r.StepCount != 48 || r.AppliedCount != 48 || controlled.commits != 48 || r.Status != "PLANNING" || r.PresentedRevisionID != "" {
		t.Fatal("controlled scheduler prematurely stopped or miscounted", r, controlled.commits, clock.Sub(started))
	}
	t.Logf("controlled execution evidence: one Run, 3 bounded stages, 48 explicit synthetic receipts, elapsed clock %s, still PLANNING; native CAD correctness is established separately", clock.Sub(started))
}

func stageForTest(t *testing.T, s *Service, r Run, steps []automation.StepSpec) Run {
	t.Helper()
	current, err := s.automation.Context(t.Context(), r.ActorID, r.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.automation.CreateTask(t.Context(), r.ActorID, r.DocumentID, automation.CreateTask{RequestID: uuid.NewString(), RunID: r.ID, PlanEpoch: r.Epoch, ExecutionMode: r.Mode, Goal: r.Goal, ExpectedHead: current.Snapshot.CommandBaseline, Steps: steps})
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.mutateRun(t.Context(), r.ID, func(tx database.Tx, current *Run) error {
		current.StageCount++
		current.ActiveTaskID = task.ID
		current.Status = "EXECUTING"
		_, err := tx.Exec(t.Context(), `INSERT INTO occccad.assistant_stages(run_id,sequence,epoch,goal_version,task_id,status) VALUES($1,$2,$3,$4,$5,'ACTIVE')`, r.ID, current.StageCount, current.Epoch, current.GoalVersion, task.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func finishStageForTest(t *testing.T, s *Service, r Run) Run {
	t.Helper()
	for r.ActiveTaskID != "" {
		if err := s.advanceStage(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		var err error
		r, err = loadRun(t.Context(), s.db, r.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != "EXECUTING" && r.Status != "PLANNING" {
			t.Fatalf("unexpected stage wait: %+v", r)
		}
	}
	return r
}

func TestAssistantRunStagesNativeObservationVerificationAndReceiptReplay(t *testing.T) {
	s, _, view, r := stageFixture(t)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: ids[0], OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}, {ID: ids[1], OperationID: "sketch.rectangle", Version: 1, Arguments: json.RawMessage(`{"width":80,"height":50,"unit":"mm"}`), Bindings: map[string]string{"sketchId": ids[0]}}, {ID: ids[2], OperationID: "part.extrude", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "length": 12, "unit": "mm"}), Bindings: map[string]string{"sketchId": ids[0]}}})
	firstTask := r.ActiveTaskID
	r = finishStageForTest(t, s, r)
	if r.Status != "PLANNING" || r.AppliedCount != 3 || r.StepCount != 3 || r.Targets["SKETCH"] == "" || r.Targets["LINEAR_EXTRUDE"] == "" || r.PresentedRevisionID != "" {
		t.Fatal("applied stage prematurely completed the Run or fabricated presentation", r)
	}
	task, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, firstTask)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.checkpointTask(t.Context(), r, task, "PLANNING", "COMPLETED", true, false, ""); err != nil {
		t.Fatal(err)
	}
	r, err = loadRun(t.Context(), s.db, r.ID, false)
	if err != nil || r.AppliedCount != 3 || r.StepCount != 3 {
		t.Fatal("checkpoint replay counted receipts twice", r, err)
	}
	page, err := s.automation.QueryTopology(t.Context(), r.ActorID, r.DocumentID, automation.TopologyQuery{RevisionID: r.LatestRevisionID, BodyID: view.Part.ActiveBodyID, Kind: "EDGE", Limit: 1})
	if err != nil || page["matchedCount"] != 12 {
		t.Fatal(page, err)
	}
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "part.fillet", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "selectionSet": page["selectionSet"], "size": 2, "unit": "mm"})}})
	r = finishStageForTest(t, s, r)
	checked, err := s.automation.VerifyPart(t.Context(), r.ActorID, r.DocumentID, automation.PartVerificationRequest{RevisionID: r.LatestRevisionID, GoalVersion: 1, Checks: []automation.PartCheck{{ID: "fillet", Kind: "ALL_EDGES_FILLET", BodyID: view.Part.ActiveBodyID, FeatureID: r.Targets["FILLET"], Radius: 2, Unit: "mm"}}})
	if err != nil || !checked.Passed || r.AppliedCount != 4 || r.RequestAppliedCount != 4 || r.StepCount != 4 || r.StageCount != 2 || r.PresentedRevisionID != "" {
		t.Fatal("native stage evidence failed", checked, r, err)
	}
}

func TestAssistantRunStageRecoversCommittedTaskBeforeRunCheckpoint(t *testing.T) {
	s, _, view, r := stageFixture(t)
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}})
	task, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, r.ActiveTaskID)
	if err != nil {
		t.Fatal(err)
	}
	for task.Status == "ACTIVE" {
		task, err = s.automation.Advance(t.Context(), r.ActorID, r.DocumentID, task.ID, task.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A service restart loses the caller's stack after the authoritative commit,
	// before any Run aggregate/checkpoint was saved.
	s = New(s.db, s.automation, s.options)
	r = finishStageForTest(t, s, r)
	if r.AppliedCount != 1 || r.StepCount != 1 || r.ActiveTaskID != "" {
		t.Fatal("exact recovered receipt was not counted once", r)
	}
	var commits int
	if err = s.db.QueryRow(context.Background(), `SELECT count(*) FROM occccad.domain_transactions WHERE request_id=$1`, task.Steps[0].Action.RequestID).Scan(&commits); err != nil || commits != 1 {
		t.Fatal("restart repeated the commit", commits, err)
	}
}

func TestAssistantRunRetiredEpochArchivesReceiptWithoutRevivingPlan(t *testing.T) {
	s, _, view, r := stageFixture(t)
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: uuid.NewString(), OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}})
	old := r
	task, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, r.ActiveTaskID)
	if err != nil {
		t.Fatal(err)
	}
	for task.Status == "ACTIVE" {
		task, err = s.automation.Advance(t.Context(), r.ActorID, r.DocumentID, task.ID, task.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err = s.mutateRun(t.Context(), r.ID, func(tx database.Tx, current *Run) error {
		current.Epoch++
		current.GoalVersion++
		current.Goal = "厚度改成20mm，后面不做圆角"
		current.RetiringTaskID = current.ActiveTaskID
		current.ActiveTaskID = ""
		current.Status = "QUEUED"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.checkpointTask(t.Context(), old, task, "PLANNING", "COMPLETED", true, false, ""); err != context.Canceled {
		t.Fatal("late old epoch updated current goal", err)
	}
	if err = s.retireStage(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	r, err = loadRun(t.Context(), s.db, r.ID, false)
	if err != nil || r.Epoch != 2 || r.GoalVersion != 2 || r.Goal != "厚度改成20mm，后面不做圆角" || r.ActiveTaskID != "" || r.RetiringTaskID != "" || r.AppliedCount != 1 || r.StepCount != 1 || r.Status != "PLANNING" || r.LatestAppliedRevisionID != task.Steps[0].Result.Receipt.RevisionID {
		t.Fatal("retired epoch lost receipt or resurrected old plan", r, err)
	}
}

func TestAssistantRunManualHeadChangeDiscardsStageWithoutRebasingArguments(t *testing.T) {
	s, domain, view, r := stageFixture(t)
	first := uuid.NewString()
	r = stageForTest(t, s, r, []automation.StepSpec{{ID: first, OperationID: "sketch.create", Version: 1, Arguments: marshalJSON(map[string]any{"bodyId": view.Part.ActiveBodyID, "plane": "XY"})}, {ID: uuid.NewString(), OperationID: "sketch.rectangle", Version: 1, Arguments: json.RawMessage(`{"width":80,"height":50,"unit":"mm"}`), Bindings: map[string]string{"sketchId": first}}})
	taskID := r.ActiveTaskID
	if err := s.advanceStage(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	r, err := loadRun(t.Context(), s.db, r.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	baseline := prepared.Steps[0].Action.ExpectedHead
	if err = domain.ExecuteCommand(t.Context(), r.DocumentID, workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: view.Part.ActiveBodyID, Plane: "XZ", RequestID: uuid.NewString(), ActorID: r.ActorID}); err != nil {
		t.Fatal(err)
	}
	human, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.advanceStage(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	r, err = loadRun(t.Context(), s.db, r.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := s.automation.GetTask(t.Context(), r.ActorID, r.DocumentID, taskID)
	if err != nil || r.Status != "PLANNING" || r.ActiveTaskID != "" || r.AppliedCount != 0 || failed.Steps[0].Action.ExpectedHead != baseline || failed.Steps[0].Result != nil || failed.Steps[1].Action != nil || failed.Steps[0].Failure == nil || failed.Steps[0].Failure.Code != "WORKSPACE_HEAD_CONFLICT" {
		t.Fatal("manual edit reused old intent on a replaced baseline", r, failed, err)
	}
	current, err := domain.ReadModelSnapshot(t.Context(), r.DocumentID, "")
	if err != nil || current.RevisionID != human.RevisionID {
		t.Fatal("AI overwrote the human commit", current, err)
	}
}

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/testsupport"
	"github.com/occccad/occccad/internal/workspace"
)

func TestMotionAPIIdempotencyAuthorizationFrozenResults(t *testing.T) {
	db, e := database.Open(t.Context(), testsupport.Environment(t).DatabaseURL)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = database.Migrate(t.Context(), db); e != nil {
		t.Fatal(e)
	}
	local, developmentStaging, err := artifact.OpenConfigured(t.Context(), testsupport.Environment(t).DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	objects := artifact.NewService(db, local, developmentStaging)
	domain := workspace.NewWithArtifacts(db, motionAPIWorker(t), objects)
	queue := jobs.New(db)
	actor := access.DefaultUserID
	part, e := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PART", Name: "empty part"})
	if e != nil {
		t.Fatal(e)
	}
	view, e := domain.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: actor, Type: "PRODUCT", Name: "API motion"})
	if e != nil {
		t.Fatal(e)
	}
	command := func(r workspace.CommandRequest) workspace.DocumentView {
		t.Helper()
		r.ActorID = actor
		r.RequestID = uuid.NewString()
		v, e := domain.ApplyCommand(t.Context(), view.Document.ID, r)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	view = command(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	view = command(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	end := func(id string) workspace.JointEndpoint {
		return workspace.JointEndpoint{InstanceID: id, Axis: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}, Plane: &workspace.AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-xy"}, Frame: workspace.InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}
	}
	second := end(view.Product.Instances[1].ID)
	m := workspace.Mechanism{ID: "m", Name: "M", UnitIDs: []string{view.Product.Instances[0].ID, second.InstanceID}, Joints: []workspace.MechanismJoint{{ID: "g", Name: "g", Kind: "GROUND", First: end(view.Product.Instances[0].ID)}, {ID: "r", Name: "r", Kind: "REVOLUTE", First: end(view.Product.Instances[0].ID), Second: &second, Coordinates: []workspace.
		JointCoordinate{{ID: "r" + "-coordinate", Role: "AXIAL_ROTATION", Dimension: "ANGLE", Zero: workspace.MotionQuantity{Unit: "deg"}, Direction: 1, Lower: &workspace.MotionQuantity{Value: -720, Unit: "deg"}, Upper: &workspace.MotionQuantity{Value: 720, Unit: "deg"}}}}}}
	k := workspace.KinematicsDefinitions{Drivers: []workspace.MotionDriver{{ID: "d", Name: "driver", MechanismID: "m", JointID: "r", CoordinateID: "r" + "-coordinate"}}, Mechanisms: []workspace.Mechanism{m}}
	view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	opened, err := domain.BeginMechanismSession(t.Context(), view.Document.ID, actor, workspace.MechanismSessionBegin{OwnerID: "fixture", Generation: 1, BaseRevisionID: view.Document.VersionID, MechanismID: "m", ActiveDriverIDs: []string{"d"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := domain.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 0, workspace.MotionKeyframe{ID: "key0"})
	if err != nil {
		t.Fatal(err)
	}
	qualified, err := domain.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, workspace.MechanismSessionUpdate{Sequence: 1, Values: map[string]workspace.
		MotionQuantity{"d": workspace.MotionQuantity{Value: 45, Unit: "deg"}}})
	if err != nil || !qualified.Accepted {
		t.Fatal(qualified, err)
	}
	last, err := domain.RecordMechanismKey(t.Context(), view.Document.ID, actor, opened.SessionID, 1, workspace.MotionKeyframe{ID: "key1", TimeSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	study := workspace.MotionStudy{ID: "s", Name: "S", MechanismID: "m", Keyframes: []workspace.MotionKeyframe{first, last}, Frames: 3, BudgetMS: 1000, ActiveDriverIDs: []string{"d"}}
	final, err := domain.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, workspace.MechanismSessionFinalize{Sequence: 1, Study: &study})
	if err != nil {
		t.Fatal(err)
	}
	view, err = domain.ApplyCommand(t.Context(), view.Document.ID, final)
	if err != nil {
		t.Fatal(err)
	}
	domain.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)

	k = view.Product.Kinematics
	server := &Server{database: db, workspace: domain, access: access.New(db), artifacts: objects, jobs: queue}
	post := func(req workspace.MotionRunRequest, user string) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/motion-runs", bytes.NewReader(b))
		r.SetPathValue("documentID", view.Document.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		w := httptest.NewRecorder()
		server.startMotionRun(w, r)
		return w
	}
	t.Run("mechanism preview access and stale snapshot", func(t *testing.T) {
		postPreview := func(user string) *httptest.ResponseRecorder {
			b, _ := json.Marshal(workspace.MechanismPreviewRequest{BaseRevisionID: "stale", Mechanism: m})
			r := httptest.NewRequest("POST", "/mechanism-preview", bytes.NewReader(b))
			r.SetPathValue("documentID", view.Document.ID)
			r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
			w := httptest.NewRecorder()
			server.previewMechanism(w, r)
			return w
		}
		if w := postPreview("00000000-0000-7000-8000-000000000099"); w.Code != 403 {
			t.Fatal("unauthorized preview", w.Code)
		}
		if w := postPreview(actor); w.Code != 400 {
			t.Fatal("stale preview", w.Code, w.Body.String())
		}
	})
	req := workspace.MotionRunRequest{BaseRevisionID: view.Document.VersionID, StudyID: "s", RequestID: "api-idempotency"}
	response := post(req, actor)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Code, response.Body.String())
	}
	var job jobs.Job
	json.Unmarshal(response.Body.Bytes(), &job)
	duplicate := post(req, actor)
	var dup jobs.Job
	json.Unmarshal(duplicate.Body.Bytes(), &dup)
	if duplicate.Code != 202 || dup.ID != job.ID {
		t.Fatal("idempotency", duplicate.Code, duplicate.Body.String())
	}
	if r := post(req, "00000000-0000-7000-8000-000000000099"); r.Code != 403 {
		t.Fatal("unauthorized freeze", r.Code)
	}
	k.Studies[0].Keyframes[1].Values["d"] = workspace.MotionQuantity{Value: 60, Unit: "deg"}
	view = command(workspace.CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
	req.BaseRevisionID = view.Document.VersionID
	if r := post(req, actor); r.Code != 409 {
		t.Fatal("requestId accepted another frozen input", r.Code, r.Body.String())
	}
	// Boundary test stores a synthetic result; numerical execution is covered by
	// the real Router/Worker job integration, not this authorization fixture.
	input, r, e := objects.Open(t.Context(), *job.InputObjectID)
	if e != nil {
		t.Fatal(e)
	}
	var snap workspace.MotionSnapshot
	if e = json.NewDecoder(r).Decode(&snap); e != nil {
		t.Fatal(e)
	}
	r.Close()
	if input.Kind != artifact.Kind("MOTION_SNAPSHOT") {
		t.Fatal(input)
	}
	data, _ := json.Marshal(workspace.MotionRun{Schema: 1, Snapshot: snap, Status: "COMPLETED", Frames: []workspace.MotionFrame{}, Completed: true})
	result, e := objects.Put(t.Context(), artifact.Kind("MOTION_RUN"), "application/json", bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	// This API authorization fixture owns only this job. Queue claiming and
	// lease attempts are covered by jobs tests and real Jobs integration.
	if _, e = db.Exec(t.Context(), `UPDATE occccad.jobs SET state='RUNNING',lease_owner='api-fixture',lease_expires_at=now()+interval '1 minute',attempt_count=1 WHERE id=$1`, job.ID); e != nil {
		t.Fatal(e)
	}
	if e = queue.FinishMotion(t.Context(), job.ID, "api-fixture", 1, result.ID, jobs.MotionCompletion{Status: "COMPLETED", Completed: true}); e != nil {
		t.Fatal(e)
	}
	get := func(user string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/motion-run", nil)
		r.SetPathValue("jobID", job.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		w := httptest.NewRecorder()
		server.getMotionRun(w, r)
		return w
	}
	if r := get(actor); r.Code != 200 || !bytes.Contains(r.Body.Bytes(), []byte(snap.RevisionID)) {
		t.Fatal("old frozen result unreadable", r.Code, r.Body.String())
	}
	if r := get("00000000-0000-7000-8000-000000000099"); r.Code != 403 {
		t.Fatal("unauthorized result", r.Code)
	}
	remove := func(user string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("DELETE", "/motion-run", nil)
		r.SetPathValue("jobID", job.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		w := httptest.NewRecorder()
		server.deleteMotionResult(w, r)
		return w
	}
	if r := remove("00000000-0000-7000-8000-000000000099"); r.Code != 403 {
		t.Fatal("foreign result deleted", r.Code)
	}
	if r := remove(actor); r.Code != 200 {
		t.Fatal("delete result", r.Code, r.Body.String())
	}
	if r := remove(actor); r.Code != 200 {
		t.Fatal("repeated delete", r.Code)
	}
	items, err := queue.ListForUser(t.Context(), actor, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == job.ID {
			t.Fatal("deleted result still listed", job.ID)
		}
	}
	if r := get(actor); r.Code != 200 {
		t.Fatal("immutable audit result lost", r.Code)
	}
	planRequest := func(user, base string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"baseRevisionId": base, "jobId": job.ID, "frameIndex": 0})
		r := httptest.NewRequest("POST", "/motion-apply-plan", bytes.NewReader(b))
		r.SetPathValue("documentID", view.Document.ID)
		r = r.WithContext(access.WithPrincipal(r.Context(), access.User{ID: user}))
		w := httptest.NewRecorder()
		server.planMotionApply(w, r)
		return w
	}
	if response := planRequest("00000000-0000-7000-8000-000000000099", view.Document.VersionID); response.Code != 403 {
		t.Fatal("unauthorized conversion plan", response.Code)
	}
	if response := planRequest(actor, "stale"); response.Code < 400 {
		t.Fatal("stale conversion plan admitted")
	}
	if response := planRequest(actor, view.Document.VersionID); response.Code < 400 {
		t.Fatal("synthetic result without qualified frames admitted")
	}

}

// Real numerical validation for definition saves; authorization/result assertions
// still use a synthetic Artifact rather than claiming a full browser workflow.
func motionAPIWorker(t *testing.T) *geometry.Client {
	t.Helper()
	client, close, err := geometry.OpenDevelopmentWorker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(close)
	return client
}

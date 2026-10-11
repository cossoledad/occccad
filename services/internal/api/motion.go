package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) startMotionRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("documentID")
	actor := principal(r).ID
	if _, e := server.access.RequireDocument(r.Context(), id, actor, access.RoleViewer); e != nil {
		writeAccessError(w, e)
		return
	}
	var req workspace.MotionRunRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 128 {
		writeError(w, 400, "requestId is required (max 128)")
		return
	}
	snap, e := server.workspace.FreezeMotionStudy(r.Context(), id, req)
	if e != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, e)
		return
	}
	data, e := json.Marshal(snap)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	o, e := server.artifacts.Put(r.Context(), artifact.Kind("MOTION_SNAPSHOT"), "application/json", bytes.NewReader(data))
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	job, e := server.jobs.Enqueue(r.Context(), jobs.EnqueueRequest{Type: "MOTION_STUDY", DocumentID: id, VersionID: &snap.RevisionID, RequestedBy: actor, InputObjectID: o.ID, IdempotencyKey: actor + "/" + id + "/" + req.RequestID, UserVisible: true, Payload: map[string]any{"inputDigest": snap.Digest, "studyId": snap.Study.ID, "studyName": snap.Study.Name}})
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	var payload struct {
		Digest string `json:"inputDigest"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil || payload.Digest != snap.Digest {
		writeError(w, 409, "requestId already names another frozen input")
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}
func (server *Server) getMotionRun(w http.ResponseWriter, r *http.Request) {
	job, ok := server.authorizedJob(w, r)
	if !ok {
		return
	}
	if job.Type != "MOTION_STUDY" || job.DocumentID == nil || job.ResultObjectID == nil || (job.State != "SUCCEEDED" && job.State != "CANCELED") {
		writeError(w, 409, "motion result is not available")
		return
	}
	if _, e := server.access.RequireDocument(r.Context(), *job.DocumentID, principal(r).ID, access.RoleViewer); e != nil {
		writeAccessError(w, e)
		return
	}
	object, reader, e := server.artifacts.Open(r.Context(), *job.ResultObjectID)
	if e != nil {
		writeError(w, 500, e.Error())
		return
	}
	defer reader.Close()
	b, e := io.ReadAll(io.LimitReader(reader, 16<<20+1))
	if e != nil || len(b) > 16<<20 {
		writeError(w, 500, "invalid motion result size")
		return
	}
	hash := sha256.Sum256(b)
	if object.Kind != artifact.Kind("MOTION_RUN") || hex.EncodeToString(hash[:]) != object.SHA256 {
		writeError(w, 500, "motion result artifact integrity mismatch")
		return
	}
	var result workspace.MotionRun
	if e = json.Unmarshal(b, &result); e != nil {
		writeError(w, 500, e.Error())
		return
	}
	writeJSON(w, 200, result)
}

func (server *Server) planMotionApply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("documentID")
	actor := principal(r).ID
	if _, e := server.access.RequireDocument(r.Context(), id, actor, access.RoleEditor); e != nil {
		writeAccessError(w, e)
		return
	}
	var req struct {
		BaseRevisionID string `json:"baseRevisionId"`
		workspace.MotionApplyRequest
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	plan, e := server.workspace.PlanMotionApply(r.Context(), id, actor, req.BaseRevisionID, req.MotionApplyRequest)
	if e != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, e)
		return
	}
	writeJSON(w, 200, plan)
}
func (server *Server) previewMechanism(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("documentID")
	if _, e := server.access.RequireDocument(r.Context(), id, principal(r).ID, access.RoleViewer); e != nil {
		writeAccessError(w, e)
		return
	}
	var req workspace.MechanismPreviewRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, e := server.workspace.PreviewMechanism(r.Context(), id, req)
	if e != nil {
		writeWorkspaceResult(w, workspace.DocumentView{}, e)
		return
	}
	writeJSON(w, 200, result)
}

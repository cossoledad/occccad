package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/jobs"
	"github.com/occccad/occccad/internal/workspace"
)

func (h handler) executeMotion(ctx context.Context, job jobs.Job) error {
	if job.DocumentID == nil || job.InputObjectID == nil || job.VersionID == nil {
		return fmt.Errorf("missing frozen motion input")
	}
	if _, e := h.access.RequireDocument(ctx, *job.DocumentID, job.RequestedBy, access.RoleViewer); e != nil {
		return e
	}
	object, r, e := h.artifacts.Open(ctx, *job.InputObjectID)
	if e != nil {
		return e
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, 8<<20+1))
	if e != nil || len(b) > 8<<20 {
		return fmt.Errorf("invalid snapshot size")
	}
	digest := sha256.Sum256(b)
	if hex.EncodeToString(digest[:]) != object.SHA256 {
		return fmt.Errorf("snapshot artifact digest mismatch")
	}
	var s workspace.MotionSnapshot
	if e = json.Unmarshal(b, &s); e != nil {
		return e
	}
	if s.DocumentID != *job.DocumentID || s.RevisionID != *job.VersionID {
		return fmt.Errorf("motion job snapshot mismatch")
	}
	var p struct {
		Digest string `json:"inputDigest"`
	}
	if json.Unmarshal(job.Payload, &p) != nil || p.Digest != s.Digest {
		return fmt.Errorf("motion input digest mismatch")
	}
	result, e := h.workspace.RunMotionStudy(ctx, s, func(n int) error { return h.queue.UpdateProgress(ctx, job.ID, h.workerID, 5+90*n/s.Study.Frames) })
	if e != nil {
		return e
	}
	b, e = json.Marshal(result)
	if e != nil || len(b) > 16<<20 {
		return fmt.Errorf("motion result size limit")
	}
	// Cancellation saves accepted frames using a bounded completion context. Lease
	// attempt CAS rejects a late result after takeover; no model revision is written.
	finish, c := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer c()
	o, e := h.artifacts.Put(finish, artifact.Kind("MOTION_RUN"), "application/json", bytes.NewReader(b))
	if e != nil {
		return e
	}
	return h.queue.FinishMotion(finish, job.ID, h.workerID, job.AttemptCount, o.ID)
}

package workspace

import (
	"context"
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

func (service *Service) committedRequestMatches(ctx context.Context, documentID string, request CommandRequest) bool {
	if request.RequestID == "" {
		return false
	}
	var payload []byte
	if request.Type == "UNDO" || request.Type == "REDO" || request.Type == "RESTORE" {
		payload, _ = json.Marshal(map[string]string{"type": request.Type, "versionId": request.VersionID})
	} else {
		payload, _ = json.Marshal(request)
	}
	var digest, actor string
	err := service.database.QueryRow(ctx, `SELECT t.request_digest,t.actor_id::text FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id WHERE w.document_id=$1 AND w.name='main' AND t.request_id=$2 AND t.status='COMMITTED'`, documentID, request.RequestID).Scan(&digest, &actor)
	return err == nil && actor == actorID(request.ActorID) && digest == modelcore.ValueDigest(payload)
}

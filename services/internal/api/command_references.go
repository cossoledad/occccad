package api

import (
	"context"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// Batch commands must authorize every referenced document before any mutation.
func (server *Server) requireCommandReferences(ctx context.Context, actorID, targetID string, input workspace.CommandRequest) error {
	ids := []string{}
	switch strings.ToUpper(input.Type) {
	case "INSERT_INSTANCE":
		ids = append(ids, input.ReferencedDocumentID)
	case "INSERT_INSTANCES":
		ids = append(ids, input.ReferencedDocumentIDs...)
		if input.InstanceID != "" {
			view, err := server.workspace.GetDocument(ctx, targetID, actorID)
			if err != nil {
				return err
			}
			if view.Product != nil {
				for _, instance := range view.Product.Instances {
					if instance.ID == input.InstanceID {
						ids = append(ids, instance.ReferencedDocumentID)
						break
					}
				}
			}
		}
	case "SET_PARAMETER_EXTERNAL":
		ids = append(ids, input.SourceDocumentID)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := server.access.RequireDocument(ctx, id, actorID, access.RoleViewer); err != nil {
			return err
		}
	}
	return nil
}

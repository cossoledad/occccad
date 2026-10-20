package api

import (
	"context"
	"github.com/occccad/occccad/internal/workspace"
)

func (server *Server) requireCommandReferences(ctx context.Context, actor, target string, input workspace.CommandRequest) error {
	return workspace.RequireCommandReferences(ctx, server.access, server.workspace, actor, target, input)
}

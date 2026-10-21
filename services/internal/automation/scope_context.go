package automation

import (
	"context"
	"encoding/json"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/workspace"
)

// ReadSnapshot keeps authority and reference access at the shared tool boundary.
func (s *Service) ReadSnapshot(ctx context.Context, actor, document, revision string) (workspace.ModelSnapshot, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return workspace.ModelSnapshot{}, err
	}
	return s.workspace.ReadModelSnapshot(ctx, document, revision)
}
func (s *Service) ContextAt(ctx context.Context, actor, document, revision string) (Context, error) {
	if revision == "" {
		return s.Context(ctx, actor, document)
	}
	snapshot, err := s.ReadSnapshot(ctx, actor, document, revision)
	if err != nil {
		return Context{}, err
	}
	role, err := s.access.RequireDocument(ctx, document, actor, access.RoleViewer)
	if err != nil {
		return Context{}, err
	}
	out := Context{Snapshot: snapshot, Permission: role, Counts: map[string]int{}, CatalogDigest: s.Catalog.Digest, Scope: "HISTORICAL_DEFINITION"}
	if snapshot.DocumentType == "PART" {
		var p workspace.PartModel
		if err = json.Unmarshal(snapshot.Model, &p); err != nil {
			return out, err
		}
		out.Units = p.Units
		out.WorkingBodyID = p.ActiveBodyID
		out.Counts["bodies"] = len(p.Bodies)
		out.Counts["features"] = len(p.Features)
	}
	return out, nil
}

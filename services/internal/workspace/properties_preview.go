package workspace

import (
	"context"
	"encoding/json"
	"time"

	"github.com/occccad/occccad/internal/modelcore"
)

type propertyPreviewSnapshotKey struct{}
type propertyPreviewSnapshot struct {
	Revision string
	Model    json.RawMessage
}

type PropertyAppearanceProjection struct {
	DocumentID     string             `json:"documentId"`
	OccurrencePath string             `json:"occurrencePath"`
	BodyID         string             `json:"bodyId"`
	Appearance     ResolvedAppearance `json:"appearance"`
}

// The same read-time projection serves a draft and a committed Revision.
// Preview performs no geometry evaluation, persistence, or metadata update.
func (s *Service) previewProperties(ctx context.Context, documentID string, prepared preparedDomainMutation, next json.RawMessage, changes modelcore.ChangeSet) (CommandPreview, error) {
	ctx = context.WithValue(ctx, propertyPreviewSnapshotKey{}, propertyPreviewSnapshot{Revision: prepared.headRevision, Model: next})
	summary := DocumentSummary{ID: documentID, Type: prepared.documentType, VersionID: prepared.headRevision}
	if err := s.database.QueryRow(ctx, `SELECT name FROM occccad.documents WHERE id=$1`, documentID).Scan(&summary.Name); err != nil {
		return CommandPreview{}, err
	}
	var view DocumentView
	var err error
	if prepared.documentType == "PART" {
		view, err = projectPartDocument(ctx, s, summary, next)
	} else {
		view, err = projectProductDocument(ctx, s, summary, next)
	}
	if err != nil {
		return CommandPreview{}, err
	}
	projection := []PropertyAppearanceProjection{}
	if view.Part != nil {
		for _, body := range view.Part.Bodies {
			appearance, ok := view.BodyAppearances[body.ID]
			if !ok {
				appearance = ResolvedAppearance{GeometryKey: body.GeometryKey}
			}
			projection = append(projection, PropertyAppearanceProjection{DocumentID: documentID, BodyID: body.ID, Appearance: appearance})
		}
	}
	for _, body := range view.ResolvedInstances {
		projection = append(projection, PropertyAppearanceProjection{DocumentID: body.DocumentID, OccurrencePath: body.OccurrencePath, BodyID: body.BodyID, Appearance: body.Appearance})
	}
	id := newID("preview")
	expires := time.Now().Add(interactionCandidateTTL)
	s.interactionCandidates.put(interactionCandidate{id: id, documentID: documentID, actorID: prepared.actorID, headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI, payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: next, changes: changes, expiresAt: expires})
	return CommandPreview{PreviewID: id, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence, ModelHash: canonicalModelHash(next), PropertyAppearances: projection, ExpiresAt: expires.UTC().Format(time.RFC3339Nano)}, nil
}

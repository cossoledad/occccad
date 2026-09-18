package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeUpdatePartReferences = "occccad://part/references/update"

type updatePartReferencesPayload struct {
	Model PartModel `json:"model"`
}

func applyUpdatePartReferences(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var before PartModel
	var payload updatePartReferencesPayload
	if err := json.Unmarshal(modelJSON, &before); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	beforeParameters := make(map[string]modelcore.ValueSource, len(before.Parameters))
	for _, parameter := range before.Parameters {
		beforeParameters[parameter.ParameterID] = parameter.Source
	}
	beforeContexts := make(map[string]ContextReference, len(before.ContextReferences))
	for _, reference := range before.ContextReferences {
		beforeContexts[reference.ID] = reference
	}
	changes := []modelcore.ModelChange{}
	seeds := []modelcore.DependencyKey{}
	for _, parameter := range payload.Model.Parameters {
		if prior, ok := beforeParameters[parameter.ParameterID]; ok && resolvedDigest(prior) != resolvedDigest(parameter.Source) {
			change, _ := modelcore.NewChange(modelcore.ChangeBind,
				modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.source"}, prior, parameter.Source)
			changes = append(changes, change)
			seeds = append(seeds, "parameter:"+modelcore.DependencyKey(parameter.ParameterID))
		}
	}
	for _, reference := range payload.Model.ContextReferences {
		if prior, ok := beforeContexts[reference.ID]; ok && resolvedDigest(prior) != resolvedDigest(reference) {
			change, _ := modelcore.NewChange(modelcore.ChangeBind,
				modelcore.PropertyAddress{EntityID: reference.ID, SlotID: "context-reference.entity"}, prior, reference)
			changes = append(changes, change)
			seeds = append(seeds, "context-reference:"+modelcore.DependencyKey(reference.ID))
			if reference.LocalTargetID != "" {
				slot := ""
				if reference.Publication.ExpectedType == "PLANE" {
					slot = "datum.plane"
				}
				if reference.Publication.ExpectedType == "AXIS" {
					slot = "datum.axis"
				}
				if reference.Publication.ExpectedType == "CURVE" {
					slot = "sketch.model"
				}
				if slot != "" {
					marker, _ := modelcore.NewChange(modelcore.ChangeUpdate,
						modelcore.PropertyAddress{EntityID: reference.LocalTargetID, SlotID: slot}, nil, nil)
					changes = append(changes, marker)
					prefix := "datum:"
					if slot == "sketch.model" {
						prefix = "feature:"
					}
					seeds = append(seeds, modelcore.DependencyKey(prefix+reference.LocalTargetID))
				}
			}
		}
	}
	next, _ := json.Marshal(payload.Model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
}

// updatePartReferences builds a fully resolved frozen candidate outside the
// commit transaction. The normal Workspace CAS rejects it if the consumer
// Head changed while source Publications were being resolved.
func (service *Service) updatePartReferences(ctx context.Context, documentID string, model *PartModel) error {
	for index := range model.Parameters {
		parameter := &model.Parameters[index]
		current := parameter.Source.External
		if current == nil || current.Revision.Mode == "PINNED" {
			continue
		}
		updated, err := service.resolveExternalParameterRefWithMode(ctx, documentID, current.SourceDocumentID, "",
			current.PublicationID, current.Revision.Mode, *parameter, *model)
		if err != nil {
			return fmt.Errorf("update external parameter %s: %w", parameter.ParameterID, err)
		}
		parameter.Source.External = &updated
	}
	for index := range model.ContextReferences {
		if model.ContextReferences[index].ReferenceMode == "PINNED" || model.ContextReferences[index].ReferenceMode == "ISOLATED" {
			continue
		}
		updated, err := service.resolveContextReference(ctx, documentID, model.ContextReferences[index], *model, "")
		if err != nil {
			return fmt.Errorf("update context reference %s: %w", model.ContextReferences[index].ID, err)
		}
		if err := materializeContextReference(model, &updated, updated.LocalTargetID); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) projectPartReferenceUpdates(ctx context.Context, model PartModel) []ReferenceUpdate {
	updates := []ReferenceUpdate{}
	appendUpdate := func(kind, id, sourceID, accepted, mode string, publication PublicationRef) {
		if mode == "PINNED" || mode == "ISOLATED" {
			return
		}
		item := ReferenceUpdate{ConsumerKind: kind, ConsumerID: id, SourceDocumentID: sourceID,
			AcceptedRevisionID: accepted, Status: "CURRENT"}
		var head string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, sourceID).Scan(&head); errors.Is(err, pgx.ErrNoRows) {
			item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", "REFERENCE_SOURCE_MISSING", "source document was deleted"
			updates = append(updates, item)
			return
		} else if err != nil {
			item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", "REFERENCE_SOURCE_UNAVAILABLE", err.Error()
			updates = append(updates, item)
			return
		}
		item.CandidateRevisionID = head
		if head != accepted {
			item.Status = "UPDATE_AVAILABLE"
		}
		if publication.PublicationID != "" {
			var raw []byte
			if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1 AND document_id=$2`, head, sourceID).Scan(&raw); err != nil {
				item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", "REFERENCE_CANDIDATE_MISSING", "candidate revision is unavailable"
			} else {
				var source PartModel
				_ = json.Unmarshal(raw, &source)
				found := false
				for _, candidate := range source.Publications {
					if candidate.ID != publication.PublicationID {
						continue
					}
					found = true
					if candidate.Type != publication.ExpectedType ||
						(publication.CompatibilityVersion != "" && candidate.CompatibilityVersion != publication.CompatibilityVersion) {
						item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", "PUBLICATION_CONTRACT_INCOMPATIBLE", "candidate Publication contract is incompatible"
					} else if candidate.Resolution.Status != "CONNECTED" {
						item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", candidate.Resolution.DiagnosticCode, candidate.Resolution.Diagnostic
					}
				}
				if !found {
					item.Status, item.DiagnosticCode, item.Diagnostic = "BROKEN", "PUBLICATION_MISSING", "candidate revision no longer contains the Publication"
				}
			}
		}
		updates = append(updates, item)
	}
	for _, parameter := range model.Parameters {
		if external := parameter.Source.External; external != nil {
			appendUpdate("EXTERNAL_PARAMETER", parameter.ParameterID, external.SourceDocumentID,
				external.ResolvedRevisionID, external.Revision.Mode, PublicationRef{PublicationID: external.PublicationID,
					ExpectedType: "PARAMETER", CompatibilityVersion: external.ContractVersion})
		}
	}
	for _, reference := range model.ContextReferences {
		sourceID, accepted, publication := reference.SourceDocumentID, reference.ResolvedRevisionID, reference.Publication
		if reference.RootProductDocumentID != "" {
			sourceID, accepted, publication = reference.RootProductDocumentID, reference.RootProductRevisionID, PublicationRef{}
		}
		appendUpdate("CONTEXT_REFERENCE", reference.ID, sourceID, accepted, reference.ReferenceMode, publication)
	}
	return updates
}

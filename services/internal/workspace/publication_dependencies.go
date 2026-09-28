package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func pathReferencesDocument(path InstancePath, documentID string) bool {
	segments := path.Segments
	return len(segments) > 0 && segments[len(segments)-1].ReferencedDocumentID == documentID
}

// Report only references owned by current mutable workspaces. Frozen Revision
// references stay readable and do not follow deletion of a newer source Head.
func publicationUseInModel(documentType string, raw []byte, sourceDocumentID, publicationID string) (string, error) {
	if !bytes.Contains(raw, []byte(publicationID)) {
		return "", nil
	}
	if documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(raw, &model); err != nil {
			return "", err
		}
		for _, reference := range model.ContextReferences {
			if reference.SourceDocumentID == sourceDocumentID && reference.Publication.PublicationID == publicationID {
				return "ContextReference " + reference.ID, nil
			}
		}
		for _, parameter := range model.Parameters {
			external := parameter.Source.External
			if external != nil && external.SourceDocumentID == sourceDocumentID && external.PublicationID == publicationID {
				return "Parameter " + parameter.ParameterID, nil
			}
		}
		return "", nil
	}
	if documentType == "PRODUCT" {
		var model ProductModel
		if err := json.Unmarshal(raw, &model); err != nil {
			return "", err
		}
		for _, binding := range model.ContextBindings {
			if binding.Publication.PublicationID == publicationID && pathReferencesDocument(binding.SourceInstancePath, sourceDocumentID) {
				return "ContextBinding " + binding.ID, nil
			}
		}
		for _, forwarding := range model.Publications {
			if forwarding.Target.PublicationID == publicationID && pathReferencesDocument(forwarding.Target.InstancePath, sourceDocumentID) {
				return "Product Publication " + forwarding.ID, nil
			}
		}
	}
	return "", nil
}

func (service *Service) ensurePublicationUnused(ctx context.Context, sourceDocumentID, publicationID string) error {
	rows, err := service.database.Query(ctx, `SELECT d.id::text,d.document_type,v.model_json
        FROM occccad.documents d JOIN occccad.workspaces w ON w.document_id=d.id AND w.name='main'
        JOIN occccad.document_versions v ON v.id=w.head_revision_id
        WHERE d.deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var consumerID, documentType string
		var raw []byte
		if err := rows.Scan(&consumerID, &documentType, &raw); err != nil {
			return err
		}
		use, err := publicationUseInModel(documentType, raw, sourceDocumentID, publicationID)
		if err != nil {
			return err
		}
		if use != "" {
			return fmt.Errorf("%w: PUBLICATION_IN_USE by %s in document %s", ErrValidation, use, consumerID)
		}
	}
	return rows.Err()
}

func rejectPublicationTargetRemoval(model PartModel, bodyID string, removedFeatures map[string]bool) error {
	for _, publication := range model.Publications {
		target := publication.Target
		depends := bodyID != "" && target.Kind == "BODY_RESULT" && target.BodyID == bodyID
		depends = depends || target.Kind == "FEATURE_OUTPUT" && removedFeatures[target.FeatureID]
		if selection := target.PersistentSelection; selection != nil {
			depends = depends || bodyID != "" && selection.SourceBodyID == bodyID || removedFeatures[selection.Anchor.FeatureID]
		}
		if target.Kind == "PARAMETER" {
			for featureID := range removedFeatures {
				if strings.HasPrefix(target.ParameterID, "parameter:"+featureID+":") {
					depends = true
				}
			}
		}
		if depends {
			return fmt.Errorf("%w: Publication %s depends on the selected Body or Feature", ErrValidation, publication.ID)
		}
	}
	return nil
}

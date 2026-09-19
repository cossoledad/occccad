package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

const productOriginPlacement = "PRODUCT_ORIGIN"

// CreatePartComponent creates the Part resource and inserts its first
// occurrence into a Product in one ProductDesignTransaction. If the target is
// nested, each owning Product reference is advanced in the same commit so the
// returned root snapshot already contains the new node.
func (service *Service) CreatePartComponent(ctx context.Context, rootProductDocumentID string, request CreatePartComponentRequest) (DocumentView, error) {
	request.RequestID = requestID(request.RequestID)
	request.ActorID = actorID(request.ActorID)
	request.PlacementMode = strings.ToUpper(strings.TrimSpace(request.PlacementMode))
	if request.PlacementMode == "" {
		request.PlacementMode = productOriginPlacement
	}
	if request.PlacementMode != productOriginPlacement {
		return DocumentView{}, fmt.Errorf("%w: only PRODUCT_ORIGIN placement is currently supported", ErrValidation)
	}
	rawRequest, _ := json.Marshal(request)
	requestDigest := modelcore.ValueDigest(rawRequest)
	var storedDigest, status string
	err := service.database.QueryRow(ctx, `SELECT request_digest,status FROM occccad.product_design_transactions
		WHERE root_product_document_id=$1 AND request_id=$2`, rootProductDocumentID, request.RequestID).Scan(&storedDigest, &status)
	if err == nil {
		if storedDigest != requestDigest {
			return DocumentView{}, fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		if status == "COMMITTED" {
			return service.GetDocument(ctx, rootProductDocumentID, request.ActorID)
		}
		return DocumentView{}, fmt.Errorf("%w: ProductDesignTransaction is %s", ErrValidation, status)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, err
	}

	rootState, err := service.loadProductCandidateState(ctx, rootProductDocumentID, "")
	if err != nil {
		return DocumentView{}, err
	}
	targetDocumentID, targetRevisionID := rootProductDocumentID, rootState.headRevision
	var targetPath *InstancePath
	if request.TargetProductInstancePath != nil && len(request.TargetProductInstancePath.Segments) > 0 {
		path := *request.TargetProductInstancePath
		if err := validateNonRootInstancePath(path, rootProductDocumentID); err != nil {
			return DocumentView{}, err
		}
		items, expandErr := service.expandProductContext(ctx, rootProductDocumentID, rootState.headRevision)
		if expandErr != nil {
			return DocumentView{}, expandErr
		}
		occurrence, ok := occurrenceByCanonical(items, path.Canonical)
		if !ok || occurrence.DocumentType != "PRODUCT" {
			return DocumentView{}, fmt.Errorf("%w: target path does not resolve to a Product occurrence", ErrValidation)
		}
		if err := validateResolvedInstancePath(path, occurrence.Path); err != nil {
			return DocumentView{}, err
		}
		targetDocumentID, targetRevisionID = occurrence.DocumentID, occurrence.RevisionID
		targetPath = &occurrence.Path
	} else if request.TargetProductInstancePath != nil && request.TargetProductInstancePath.RootDocumentID != "" &&
		request.TargetProductInstancePath.RootDocumentID != rootProductDocumentID {
		return DocumentView{}, fmt.Errorf("%w: target Product path belongs to another root Product", ErrValidation)
	}

	partName := strings.TrimSpace(request.Name)
	if partName == "" {
		partName, err = service.nextPartDocumentName(ctx)
		if err != nil {
			return DocumentView{}, err
		}
	}
	partName, description, err := validateNameAndDescription(partName, request.Description, "Part")
	if err != nil {
		return DocumentView{}, err
	}
	var duplicate bool
	if err := service.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occccad.documents
		WHERE document_type='PART' AND name=$1)`, partName).Scan(&duplicate); err != nil {
		return DocumentView{}, err
	}
	if duplicate {
		return DocumentView{}, fmt.Errorf("%w: Part document name already exists", ErrValidation)
	}

	partDocumentUUID, _ := uuid.NewV7()
	partRevisionUUID, _ := uuid.NewV7()
	partDocumentID, partRevisionID := partDocumentUUID.String(), partRevisionUUID.String()
	partModel := newPartModel()
	geometryKey, err := service.ensureVisualizationArtifact(ctx, partModel)
	if err != nil {
		return DocumentView{}, err
	}
	partJSON, _ := json.Marshal(partModel)
	partJSON, partHash, partGraph, partManifest, dependencyDigest, err := prepareInitialEvaluation("PART", partRevisionID, partJSON)
	if err != nil {
		return DocumentView{}, err
	}
	initialPart := initialDocumentCandidate{documentID: partDocumentID, revisionID: partRevisionID, documentType: "PART",
		name: partName, description: description, ownerID: request.ActorID, requestID: request.RequestID + "/part-document",
		modelJSON: partJSON, geometryKey: &geometryKey, modelHash: partHash, dependencyDigest: dependencyDigest,
		graph: partGraph, manifest: partManifest}

	instance := ProductInstance{ID: commandEntityID("instance", request.RequestID), ReferencedDocumentID: partDocumentID,
		ReferencedVersionID: partRevisionID, ResolvedVersionID: partRevisionID, Translation: [3]float64{},
		Rotation: [4]float64{0, 0, 0, 1}, ReferenceMode: "FOLLOW_HEAD"}
	targetState := rootState
	if targetDocumentID != rootProductDocumentID {
		targetState, err = service.loadProductCandidateState(ctx, targetDocumentID, targetRevisionID)
		if err != nil {
			return DocumentView{}, err
		}
	}
	instance.Name = nextInstanceName(targetState.model, partName)
	targetCandidate, err := service.prepareProductInsertionCandidate(request, targetState, instance)
	if err != nil {
		return DocumentView{}, err
	}
	candidates := []atomicDomainCandidate{targetCandidate}

	if targetPath != nil {
		childRevisionID := targetCandidate.revisionID
		seenDocuments := map[string]bool{rootProductDocumentID: true, targetDocumentID: true}
		for pathIndex := len(targetPath.Segments) - 1; pathIndex >= 1; pathIndex-- {
			segment := targetPath.Segments[pathIndex]
			if seenDocuments[segment.OwnerDocumentID] {
				return DocumentView{}, fmt.Errorf("%w: Product occurrence ancestry cannot update one document workspace twice", ErrValidation)
			}
			seenDocuments[segment.OwnerDocumentID] = true
			state, loadErr := service.loadProductCandidateState(ctx, segment.OwnerDocumentID, segment.OwnerVersionID)
			if loadErr != nil {
				return DocumentView{}, loadErr
			}
			candidate, prepareErr := service.prepareProductReferenceCandidate(request, state, segment, childRevisionID, pathIndex)
			if prepareErr != nil {
				return DocumentView{}, prepareErr
			}
			candidates = append(candidates, candidate)
			childRevisionID = candidate.revisionID
		}
		rootSegment := targetPath.Segments[0]
		rootCandidate, prepareErr := service.prepareProductReferenceCandidate(request, rootState, rootSegment, childRevisionID, 0)
		if prepareErr != nil {
			return DocumentView{}, prepareErr
		}
		candidates = append(candidates, rootCandidate)
	}

	groupUUID, _ := uuid.NewV7()
	if err := service.commitProductDesignCandidates(ctx, groupUUID.String(), request.RequestID, requestDigest,
		request.ActorID, rootProductDocumentID, candidates, initialPart); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, rootProductDocumentID, request.ActorID)
}

type productCandidateState struct {
	documentID, workspaceID, headRevision string
	headSequence                          uint64
	modelJSON, manifestJSON               []byte
	model                                 ProductModel
}

func (service *Service) loadProductCandidateState(ctx context.Context, documentID, expectedRevisionID string) (productCandidateState, error) {
	state := productCandidateState{documentID: documentID}
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,
		v.model_json,v.evaluation_manifest FROM occccad.workspaces w
		JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions v ON v.id=w.head_revision_id
		WHERE d.id=$1 AND d.document_type='PRODUCT' AND d.deleted_at IS NULL AND w.name='main'`, documentID).
		Scan(&state.workspaceID, &state.headRevision, &state.headSequence, &state.modelJSON, &state.manifestJSON); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return state, fmt.Errorf("%w: Product does not exist", ErrValidation)
		}
		return state, err
	}
	if expectedRevisionID != "" && state.headRevision != expectedRevisionID {
		return state, fmt.Errorf("%w: target Product occurrence must accept its Head before inserting a Part", ErrValidation)
	}
	if err := json.Unmarshal(state.modelJSON, &state.model); err != nil {
		return state, err
	}
	return state, nil
}

func (service *Service) prepareProductInsertionCandidate(request CreatePartComponentRequest, state productCandidateState, instance ProductInstance) (atomicDomainCandidate, error) {
	payloadJSON, _ := json.Marshal(insertInstancePayload{Instance: instance})
	nextJSON, changes, err := applyInsertInstance(state.modelJSON, payloadJSON)
	if err != nil {
		return atomicDomainCandidate{}, err
	}
	revisionUUID, _ := uuid.NewV7()
	revisionID := revisionUUID.String()
	return buildProductCandidate(state, revisionID, request.RequestID+"/target-product", "CREATE_PART_COMPONENT",
		modelcore.DomainCommand{CommandID: newID("command"), TypeURI: typeInsertInstance, SchemaVersion: 1, Payload: payloadJSON}, nextJSON, changes, request.ActorID)
}

func (service *Service) prepareProductReferenceCandidate(request CreatePartComponentRequest, state productCandidateState,
	segment InstancePathSegment, childRevisionID string, pathIndex int) (atomicDomainCandidate, error) {
	model := state.model
	found := false
	for index := range model.Instances {
		if model.Instances[index].ID == segment.InstanceID {
			model.Instances[index].ReferencedVersionID = childRevisionID
			model.Instances[index].ResolvedVersionID = childRevisionID
			found = true
			break
		}
	}
	if !found {
		return atomicDomainCandidate{}, fmt.Errorf("%w: target Product occurrence disappeared", ErrValidation)
	}
	nextJSON, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeBind,
		modelcore.PropertyAddress{EntityID: segment.InstanceID, SlotID: "instance.reference"}, segment.ResolvedVersionID, childRevisionID)
	changes := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change},
		ImpactSeeds: []modelcore.DependencyKey{"reference:" + modelcore.DependencyKey(segment.InstanceID)}}
	payloadJSON, _ := json.Marshal(map[string]string{"instanceId": segment.InstanceID, "referencedVersionId": childRevisionID})
	revisionUUID, _ := uuid.NewV7()
	return buildProductCandidate(state, revisionUUID.String(), request.RequestID+fmt.Sprintf("/owner-product-%d", pathIndex),
		"ACCEPT_NEW_PART_REVISION", modelcore.DomainCommand{CommandID: newID("command"),
			TypeURI: "occccad://product/reference/accept-new-part", SchemaVersion: 1, Payload: payloadJSON}, nextJSON, changes, request.ActorID)
}

func buildProductCandidate(state productCandidateState, revisionID, requestIDValue, requestType string,
	command modelcore.DomainCommand, nextJSON json.RawMessage, changes modelcore.ChangeSet, actor string) (atomicDomainCandidate, error) {
	modelHash := canonicalModelHash(nextJSON)
	var prior *modelcore.EvaluationManifest
	if len(state.manifestJSON) > 0 {
		var value modelcore.EvaluationManifest
		if json.Unmarshal(state.manifestJSON, &value) == nil {
			prior = &value
		}
	}
	var model ProductModel
	if err := json.Unmarshal(nextJSON, &model); err != nil {
		return atomicDomainCandidate{}, err
	}
	graph, manifest, err := buildProductEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prior)
	if err != nil {
		return atomicDomainCandidate{}, err
	}
	changes, err = reconcilePersistedChanges("PRODUCT", state.modelJSON, nextJSON, changes)
	if err != nil {
		return atomicDomainCandidate{}, err
	}
	return atomicDomainCandidate{documentID: state.documentID, workspaceID: state.workspaceID, headRevision: state.headRevision,
		headSequence: state.headSequence, documentType: "PRODUCT", revisionID: revisionID,
		request: CommandRequest{RequestID: requestIDValue, Type: requestType, ActorID: actor}, command: command,
		nextJSON: nextJSON, modelHash: modelHash, graph: graph, manifest: manifest, changes: changes}, nil
}

func (service *Service) nextPartDocumentName(ctx context.Context) (string, error) {
	// Soft-deleted documents still participate in the current schema's unique
	// (document_type,name) key, so automatic allocation must include Trash.
	rows, err := service.database.Query(ctx, `SELECT name FROM occccad.documents WHERE document_type='PART'`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	used := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		used[strings.ToLower(strings.TrimSpace(name))] = true
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	for ordinal := 1; ; ordinal++ {
		candidate := fmt.Sprintf("Part%d", ordinal)
		if !used[strings.ToLower(candidate)] {
			return candidate, nil
		}
	}
}

func (service *Service) persistInitialDocumentCandidate(ctx context.Context, tx pgx.Tx, candidate *initialDocumentCandidate) error {
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.documents(id,document_type,name,description,folder_id,owner_user_id)
		VALUES($1,$2,$3,$4,$5,$6)`, candidate.documentID, candidate.documentType, candidate.name, candidate.description,
		candidate.folderID, candidate.ownerID); err != nil {
		return fmt.Errorf("create component document: %w", err)
	}
	traceID, spanID := traceIDs(ctx)
	var commandID string
	if err := tx.QueryRow(ctx, `INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id)
		VALUES($1,'CREATE_DOCUMENT',$2,$3,'SUCCEEDED',now(),$4,$5) RETURNING id::text`, candidate.requestID,
		candidate.documentID, candidate.modelJSON, traceID, spanID).Scan(&commandID); err != nil {
		return err
	}
	var geometry any
	if candidate.geometryKey != nil && *candidate.geometryKey != "" {
		geometry = *candidate.geometryKey
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash)
		VALUES($1,$2,1,$3,$4,'READY',$5,$6)`, candidate.revisionID, candidate.documentID, candidate.modelJSON,
		geometry, commandID, candidate.modelHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`,
		candidate.revisionID, candidate.documentID); err != nil {
		return err
	}
	var workspaceID string
	if err := tx.QueryRow(ctx, `INSERT INTO occccad.workspaces(document_id,name,head_revision_id,head_sequence,base_revision_id)
		VALUES($1,'main',$2,1,$2) RETURNING id::text`, candidate.documentID, candidate.revisionID).Scan(&workspaceID); err != nil {
		return err
	}
	if err := persistEvaluationProjection(ctx, tx, candidate.revisionID, candidate.documentType, candidate.modelHash,
		candidate.dependencyDigest, candidate.graph, candidate.manifest); err != nil {
		return err
	}
	if err := persistInitialTransaction(ctx, tx, workspaceID, candidate.documentID, candidate.revisionID, candidate.ownerID,
		candidate.modelHash, "occccad://document/create", candidate.modelJSON); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_history(document_id,position,version_id,command_id)
		VALUES($1,0,$2,$3)`, candidate.documentID, candidate.revisionID, commandID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type)
		VALUES($1,$2,$3,'CREATE_DOCUMENT')`, candidate.documentID, candidate.revisionID, commandID)
	return err
}

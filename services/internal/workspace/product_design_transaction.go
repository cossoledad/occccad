package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

// ProductContextBindingRequest is the first atomic Product design operation:
// it creates a reusable ContextInput in an occurrence Part, advances every
// owning Product reference along that path, and binds the input to a compatible
// Publication reachable through the root Product snapshot.
type ProductContextBindingRequest struct {
	RequestID          string       `json:"requestId"`
	Name               string       `json:"name,omitempty"`
	ContextInputName   string       `json:"contextInputName,omitempty"`
	OwningInstancePath InstancePath `json:"owningInstancePath"`
	SourceInstancePath InstancePath `json:"sourceInstancePath"`
	PublicationID      string       `json:"publicationId"`
	PublicationType    string       `json:"publicationType,omitempty"`
	TargetKind         string       `json:"targetKind"`
	TargetID           string       `json:"targetId"`
	Required           bool         `json:"required,omitempty"`
	ReferenceMode      string       `json:"referenceMode,omitempty"`
	ActorID            string       `json:"-"`
}

type atomicDomainCandidate struct {
	documentID, workspaceID, headRevision, documentType string
	headSequence                                        uint64
	revisionID, transactionID                           string
	kind, rootTransaction, consumedRevert               string
	request                                             CommandRequest
	command                                             modelcore.DomainCommand
	nextJSON                                            json.RawMessage
	geometryKey                                         string
	modelHash                                           string
	graph                                               *modelcore.DependencyGraph
	manifest                                            modelcore.EvaluationManifest
	changes                                             modelcore.ChangeSet
}

func (service *Service) CreateProductContextBinding(ctx context.Context, rootProductDocumentID string, request ProductContextBindingRequest) (DocumentView, error) {
	request.RequestID = requestID(request.RequestID)
	request.ActorID = actorID(request.ActorID)
	rawRequest, _ := json.Marshal(request)
	requestDigest := modelcore.ValueDigest(rawRequest)
	var storedDigest, status string
	if err := service.database.QueryRow(ctx, `SELECT request_digest,status FROM occccad.product_design_transactions WHERE root_product_document_id=$1 AND request_id=$2`, rootProductDocumentID, request.RequestID).Scan(&storedDigest, &status); err == nil {
		if storedDigest != requestDigest {
			return DocumentView{}, fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		if status == "COMMITTED" {
			return service.GetDocument(ctx, rootProductDocumentID, request.ActorID)
		}
		return DocumentView{}, fmt.Errorf("%w: ProductDesignTransaction is %s", ErrValidation, status)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return DocumentView{}, err
	}

	var rootWorkspaceID, rootRevisionID string
	var rootSequence uint64
	var rootJSON []byte
	var rootManifestJSON []byte
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,v.model_json,v.evaluation_manifest FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions v ON v.id=w.head_revision_id WHERE d.id=$1 AND d.document_type='PRODUCT' AND d.deleted_at IS NULL AND w.name='main'`, rootProductDocumentID).Scan(&rootWorkspaceID, &rootRevisionID, &rootSequence, &rootJSON, &rootManifestJSON); err != nil {
		return DocumentView{}, err
	}
	var root ProductModel
	if err := json.Unmarshal(rootJSON, &root); err != nil {
		return DocumentView{}, err
	}
	if err := validateNonRootInstancePath(request.OwningInstancePath, rootProductDocumentID); err != nil {
		return DocumentView{}, err
	}
	if err := validateNonRootInstancePath(request.SourceInstancePath, rootProductDocumentID); err != nil {
		return DocumentView{}, err
	}
	items, err := service.expandProductContext(ctx, rootProductDocumentID, rootRevisionID)
	if err != nil {
		return DocumentView{}, err
	}
	owner, ownerOK := occurrenceByCanonical(items, request.OwningInstancePath.Canonical)
	source, sourceOK := occurrenceByCanonical(items, request.SourceInstancePath.Canonical)
	if !ownerOK || !sourceOK || owner.DocumentType != "PART" {
		return DocumentView{}, fmt.Errorf("%w: binding endpoint is outside the Product context", ErrValidation)
	}
	if err := validateResolvedInstancePath(request.OwningInstancePath, owner.Path); err != nil {
		return DocumentView{}, err
	}
	if err := validateResolvedInstancePath(request.SourceInstancePath, source.Path); err != nil {
		return DocumentView{}, err
	}
	if owner.Path.Canonical == source.Path.Canonical {
		return DocumentView{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	dependencyEdges := map[string][]string{}
	for _, occurrence := range items {
		for _, existing := range occurrence.ContextBindings {
			dependencyEdges[existing.SourceInstancePath.Canonical] = append(dependencyEdges[existing.SourceInstancePath.Canonical], existing.OwningInstancePath.Canonical)
		}
	}
	if contextDependencyReachable(dependencyEdges, owner.Path.Canonical, source.Path.Canonical) {
		return DocumentView{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	var sourcePublication *Publication
	for index := range source.Publications {
		if source.Publications[index].ID == request.PublicationID {
			sourcePublication = &source.Publications[index]
			break
		}
	}
	if sourcePublication == nil || sourcePublication.Resolution.Status != "CONNECTED" {
		return DocumentView{}, fmt.Errorf("%w: source Publication is missing or broken", ErrValidation)
	}

	var ownerWorkspaceID, ownerHead string
	var ownerSequence uint64
	var ownerJSON []byte
	var ownerGeometry *string
	var ownerManifestJSON []byte
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,v.model_json,v.geometry_key,v.evaluation_manifest FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions v ON v.id=w.head_revision_id WHERE d.id=$1 AND d.document_type='PART' AND d.deleted_at IS NULL AND w.name='main'`, owner.DocumentID).Scan(&ownerWorkspaceID, &ownerHead, &ownerSequence, &ownerJSON, &ownerGeometry, &ownerManifestJSON); err != nil {
		return DocumentView{}, err
	}
	if ownerHead != owner.RevisionID {
		return DocumentView{}, fmt.Errorf("%w: owning occurrence must accept its Part Head before binding", ErrValidation)
	}
	var ownerModel PartModel
	if err := json.Unmarshal(ownerJSON, &ownerModel); err != nil {
		return DocumentView{}, err
	}
	inputRequest := CommandRequest{RequestID: request.RequestID + "/context-input", Type: "CREATE_CONTEXT_INPUT", Name: request.ContextInputName,
		TargetKind: request.TargetKind, TargetID: request.TargetID, PublicationType: request.PublicationType, Required: request.Required, ActorID: request.ActorID}
	input, err := contextInputFromRequest(ownerModel, inputRequest)
	if err != nil {
		return DocumentView{}, err
	}
	if !contractsCompatible(input.Contract, input.Type, *sourcePublication) {
		return DocumentView{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
	}
	inputPayload := contextInputPayload{Input: input}
	inputPayloadJSON, _ := json.Marshal(inputPayload)
	ownerNextJSON, ownerChanges, err := applyCreateContextInput(ownerJSON, inputPayloadJSON)
	if err != nil {
		return DocumentView{}, err
	}
	var ownerNext PartModel
	if err := json.Unmarshal(ownerNextJSON, &ownerNext); err != nil {
		return DocumentView{}, err
	}
	if err := validateAndResolvePartParameters(&ownerNext); err != nil {
		return DocumentView{}, err
	}
	ownerRevisionUUID, _ := uuid.NewV7()
	ownerRevisionID := ownerRevisionUUID.String()
	ownerNextJSON, _ = json.Marshal(ownerNext)
	var ownerPrior *modelcore.EvaluationManifest
	if len(ownerManifestJSON) > 0 {
		var value modelcore.EvaluationManifest
		if json.Unmarshal(ownerManifestJSON, &value) == nil {
			ownerPrior = &value
		}
	}
	ownerHash := canonicalModelHash(ownerNextJSON)
	ownerGraph, ownerManifest, err := buildPartEvaluation(ownerNext, ownerRevisionID, ownerHash, ownerChanges.ImpactSeeds, ownerPrior)
	if err != nil {
		return DocumentView{}, err
	}
	ownerChanges, err = reconcilePersistedChanges("PART", ownerJSON, ownerNextJSON, ownerChanges)
	if err != nil {
		return DocumentView{}, err
	}

	ownerPath := owner.Path
	ownerPath.Segments[len(ownerPath.Segments)-1].ResolvedVersionID = ownerRevisionID
	mode := strings.ToUpper(strings.TrimSpace(request.ReferenceMode))
	if mode == "" {
		mode = "FOLLOW_HEAD"
	}
	if mode != "FOLLOW_HEAD" && mode != "FOLLOW_WORKSPACE_WITH_ACCEPT" && mode != "PINNED" {
		return DocumentView{}, fmt.Errorf("%w: unsupported context binding mode", ErrValidation)
	}
	bindingName := strings.TrimSpace(request.Name)
	if bindingName == "" {
		bindingName = input.Name + " <- " + sourcePublication.Name
	}
	inputCommand := modelcore.DomainCommand{CommandID: newID("command"), TypeURI: typeCreateContextInput, SchemaVersion: 1, Payload: inputPayloadJSON}
	ownerCandidate := atomicDomainCandidate{documentID: owner.DocumentID, workspaceID: ownerWorkspaceID, headRevision: ownerHead,
		headSequence: ownerSequence, documentType: "PART", revisionID: ownerRevisionID, request: inputRequest, command: inputCommand,
		nextJSON: ownerNextJSON, modelHash: ownerHash, graph: ownerGraph, manifest: ownerManifest, changes: ownerChanges}
	if ownerGeometry != nil {
		ownerCandidate.geometryKey = *ownerGeometry
	}
	productCandidates := []atomicDomainCandidate{}
	childRevisionID := ownerRevisionID
	seenDocuments := map[string]bool{owner.DocumentID: true, rootProductDocumentID: true}
	for pathIndex := len(owner.Path.Segments) - 1; pathIndex >= 1; pathIndex-- {
		segment := owner.Path.Segments[pathIndex]
		if seenDocuments[segment.OwnerDocumentID] {
			return DocumentView{}, fmt.Errorf("%w: Product occurrence ancestry cannot update one document workspace twice", ErrValidation)
		}
		seenDocuments[segment.OwnerDocumentID] = true
		var workspaceID, headRevision string
		var headSequence uint64
		var modelJSON, manifestJSON []byte
		if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,v.model_json,v.evaluation_manifest FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions v ON v.id=w.head_revision_id WHERE d.id=$1 AND d.document_type='PRODUCT' AND d.deleted_at IS NULL AND w.name='main'`, segment.OwnerDocumentID).Scan(&workspaceID, &headRevision, &headSequence, &modelJSON, &manifestJSON); err != nil {
			return DocumentView{}, err
		}
		if headRevision != segment.OwnerVersionID {
			return DocumentView{}, fmt.Errorf("%w: nested owning Product must accept its Head before binding", ErrValidation)
		}
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return DocumentView{}, err
		}
		found := false
		for index := range model.Instances {
			if model.Instances[index].ID != segment.InstanceID {
				continue
			}
			model.Instances[index].ReferencedVersionID = childRevisionID
			model.Instances[index].ResolvedVersionID = childRevisionID
			found = true
			break
		}
		if !found {
			return DocumentView{}, fmt.Errorf("%w: nested owning occurrence disappeared", ErrValidation)
		}
		nextJSON, _ := json.Marshal(model)
		change, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: segment.InstanceID, SlotID: "instance.reference"}, segment.ResolvedVersionID, childRevisionID)
		changes := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"reference:" + modelcore.DependencyKey(segment.InstanceID)}}
		revisionUUID, _ := uuid.NewV7()
		revisionID := revisionUUID.String()
		modelHash := canonicalModelHash(nextJSON)
		var prior *modelcore.EvaluationManifest
		if len(manifestJSON) > 0 {
			var value modelcore.EvaluationManifest
			if json.Unmarshal(manifestJSON, &value) == nil {
				prior = &value
			}
		}
		graph, manifest, err := buildProductEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prior)
		if err != nil {
			return DocumentView{}, err
		}
		changes, err = reconcilePersistedChanges("PRODUCT", modelJSON, nextJSON, changes)
		if err != nil {
			return DocumentView{}, err
		}
		payload, _ := json.Marshal(map[string]string{"instanceId": segment.InstanceID, "referencedVersionId": childRevisionID})
		candidateRequest := CommandRequest{RequestID: request.RequestID + fmt.Sprintf("/owner-product-%d", pathIndex), Type: "ACCEPT_CONTEXT_INPUT_REVISION", ActorID: request.ActorID}
		productCandidates = append(productCandidates, atomicDomainCandidate{documentID: segment.OwnerDocumentID, workspaceID: workspaceID,
			headRevision: headRevision, headSequence: headSequence, documentType: "PRODUCT", revisionID: revisionID,
			request: candidateRequest, command: modelcore.DomainCommand{CommandID: newID("command"), TypeURI: "occccad://product/reference/accept-context-input", SchemaVersion: 1, Payload: payload},
			nextJSON: nextJSON, modelHash: modelHash, graph: graph, manifest: manifest, changes: changes})
		ownerPath.Segments[pathIndex-1].ResolvedVersionID = revisionID
		ownerPath.Segments[pathIndex].OwnerVersionID = revisionID
		childRevisionID = revisionID
	}
	rootRevisionUUID, _ := uuid.NewV7()
	rootNextRevisionID := rootRevisionUUID.String()
	ownerPath.Segments[0].OwnerVersionID = rootNextRevisionID
	sourcePath := source.Path
	sourcePath.Segments = append([]InstancePathSegment(nil), source.Path.Segments...)
	sourcePath.Segments[0].OwnerVersionID = rootNextRevisionID
	for index := 0; index < len(sourcePath.Segments) && index < len(ownerPath.Segments); index++ {
		if sourcePath.Segments[index].InstanceID != ownerPath.Segments[index].InstanceID {
			break
		}
		sourcePath.Segments[index].ResolvedVersionID = ownerPath.Segments[index].ResolvedVersionID
		if index+1 < len(sourcePath.Segments) && index+1 < len(ownerPath.Segments) {
			sourcePath.Segments[index+1].OwnerVersionID = ownerPath.Segments[index+1].OwnerVersionID
		}
	}
	binding := ContextBinding{ID: commandEntityID("context-binding", request.RequestID), Name: bindingName,
		OwningInstancePath: ownerPath, ContextInputID: input.ID, SourceInstancePath: sourcePath,
		Publication:   PublicationRef{PublicationID: sourcePublication.ID, ExpectedType: sourcePublication.Type, CompatibilityVersion: sourcePublication.CompatibilityVersion},
		ReferenceMode: mode, Transform: inverseRelativePose(source.Pose, owner.Pose), Resolution: sourcePublication.Resolution,
		Accepted: ContextBindingResolutionSnapshot{RootProductRevisionID: rootRevisionID, SourceRevisionID: source.RevisionID,
			OwningRevisionID: ownerRevisionID, ContractDigest: resolvedDigest(sourcePublication.Contract),
			SourceDigest: sourcePublication.Resolution.SourceDigest, Status: "CONNECTED"}}
	rootOwnerInstanceID := ownerPath.Segments[0].InstanceID
	for index := range root.Instances {
		if root.Instances[index].ID == rootOwnerInstanceID {
			root.Instances[index].ReferencedVersionID, root.Instances[index].ResolvedVersionID = childRevisionID, childRevisionID
		}
	}
	rootCandidateJSON, _ := json.Marshal(root)
	bindingPayloadJSON, _ := json.Marshal(contextBindingPayload{Binding: binding})
	rootNextJSON, rootChanges, err := applyCreateContextBinding(rootCandidateJSON, bindingPayloadJSON)
	if err != nil {
		return DocumentView{}, err
	}
	refChange, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: rootOwnerInstanceID, SlotID: "instance.reference"}, owner.Path.Segments[0].ResolvedVersionID, childRevisionID)
	rootChanges.Changes = append(rootChanges.Changes, refChange)
	rootHash := canonicalModelHash(rootNextJSON)
	var rootPrior *modelcore.EvaluationManifest
	if len(rootManifestJSON) > 0 {
		var value modelcore.EvaluationManifest
		if json.Unmarshal(rootManifestJSON, &value) == nil {
			rootPrior = &value
		}
	}
	var rootNext ProductModel
	_ = json.Unmarshal(rootNextJSON, &rootNext)
	rootGraph, rootManifest, err := buildProductEvaluation(rootNext, rootNextRevisionID, rootHash, rootChanges.ImpactSeeds, rootPrior)
	if err != nil {
		return DocumentView{}, err
	}
	rootChanges, err = reconcilePersistedChanges("PRODUCT", rootJSON, rootNextJSON, rootChanges)
	if err != nil {
		return DocumentView{}, err
	}
	bindingCommand := modelcore.DomainCommand{CommandID: newID("command"), TypeURI: typeCreateContextBinding, SchemaVersion: 1, Payload: bindingPayloadJSON}
	rootRequest := CommandRequest{RequestID: request.RequestID + "/context-binding", Type: "CREATE_CONTEXT_BINDING", ActorID: request.ActorID}
	rootCandidate := atomicDomainCandidate{documentID: rootProductDocumentID, workspaceID: rootWorkspaceID, headRevision: rootRevisionID,
		headSequence: rootSequence, documentType: "PRODUCT", revisionID: rootNextRevisionID, request: rootRequest, command: bindingCommand,
		nextJSON: rootNextJSON, modelHash: rootHash, graph: rootGraph, manifest: rootManifest, changes: rootChanges}
	groupUUID, _ := uuid.NewV7()
	candidates := append([]atomicDomainCandidate{ownerCandidate}, productCandidates...)
	candidates = append(candidates, rootCandidate)
	if err := service.commitProductDesignCandidates(ctx, groupUUID.String(), request.RequestID, requestDigest, request.ActorID,
		rootProductDocumentID, candidates); err != nil {
		return DocumentView{}, err
	}
	return service.GetDocument(ctx, rootProductDocumentID, request.ActorID)
}

func (service *Service) commitProductDesignCandidates(ctx context.Context, groupID, requestIDValue, requestDigest, actor, rootDocumentID string, candidates []atomicDomainCandidate) error {
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].workspaceID < candidates[j].workspaceID })
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for index := range candidates {
		var head string
		var sequence uint64
		if err := tx.QueryRow(ctx, `SELECT head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 FOR UPDATE`, candidates[index].workspaceID).Scan(&head, &sequence); err != nil {
			return err
		}
		if head != candidates[index].headRevision || sequence != candidates[index].headSequence {
			return fmt.Errorf("%w: WORKSPACE_HEAD_CONFLICT", ErrValidation)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.product_design_transactions(id,root_product_document_id,actor_id,request_id,request_digest,status,committed_at) VALUES($1,$2,$3,$4,$5,'COMMITTED',now())`, groupID, rootDocumentID, actor, requestIDValue, requestDigest); err != nil {
		return err
	}
	for index := range candidates {
		candidate := &candidates[index]
		transactionUUID, _ := uuid.NewV7()
		candidate.transactionID = transactionUUID.String()
		var revisionSequence uint64
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(sequence),0)+1 FROM occccad.document_versions WHERE document_id=$1`, candidate.documentID).Scan(&revisionSequence); err != nil {
			return err
		}
		transportPayload, _ := json.Marshal(candidate.request)
		traceID, spanID := traceIDs(ctx)
		var auditCommandID string
		if err := tx.QueryRow(ctx, `INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id) VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`, candidate.request.RequestID, candidate.request.Type, candidate.documentID, transportPayload, traceID, spanID).Scan(&auditCommandID); err != nil {
			return err
		}
		var geometry any
		if candidate.geometryKey != "" {
			geometry = candidate.geometryKey
		}
		manifestJSON, _ := json.Marshal(candidate.manifest)
		dependencyDigest, err := candidate.graph.Digest()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,'READY',$7,$8,$9,$10)`, candidate.revisionID, candidate.documentID, candidate.headRevision, revisionSequence, candidate.nextJSON, geometry, auditCommandID, candidate.modelHash, dependencyDigest, manifestJSON); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.revision_parents(revision_id,parent_revision_id,ordinal) VALUES($1,$2,0)`, candidate.revisionID, candidate.headRevision); err != nil {
			return err
		}
		requestJSON, _ := json.Marshal(candidate.request)
		individualDigest := modelcore.ValueDigest(requestJSON)
		kind := candidate.kind
		if kind == "" {
			kind = "DOMAIN"
		}
		var rootTransaction, revertsTransaction, reappliesTransaction any
		if candidate.rootTransaction != "" {
			rootTransaction = candidate.rootTransaction
		}
		if kind == "REVERT" {
			revertsTransaction = candidate.rootTransaction
		} else if kind == "REAPPLY" {
			reappliesTransaction = candidate.consumedRevert
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,base_revision_id,result_revision_id,root_transaction_id,reverts_transaction_id,reapplies_transaction_id,product_design_transaction_id,committed_at) VALUES($1,$2,$3,$4,$5,$6,$7,'COMMITTED',$8,$9,$10,$11,$12,$13,now())`, candidate.transactionID, candidate.workspaceID, candidate.headSequence+1, actor, candidate.request.RequestID, individualDigest, kind, candidate.headRevision, candidate.revisionID, rootTransaction, revertsTransaction, reappliesTransaction, groupID); err != nil {
			return err
		}
		payloadDigest := modelcore.ValueDigest(candidate.command.Payload)
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,$4,$5,$6)`, candidate.transactionID, candidate.command.CommandID, candidate.command.TypeURI, candidate.command.SchemaVersion, candidate.command.Payload, payloadDigest); err != nil {
			return err
		}
		changesJSON, _ := json.Marshal(candidate.changes)
		writes := make([]string, 0, len(candidate.changes.Changes))
		for _, change := range candidate.changes.Changes {
			writes = append(writes, change.Target.Key())
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, candidate.transactionID, changesJSON, candidate.changes.CanonicalDigest, writes, candidate.changes.ImpactSeeds); err != nil {
			return err
		}
		manifestDigest := modelcore.ValueDigest(manifestJSON)
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, candidate.revisionID, strings.ToLower(candidate.documentType), evaluatorVersion, candidate.modelHash, manifestJSON, manifestDigest); err != nil {
			return err
		}
		for _, edge := range candidate.graph.Edges {
			if _, err := tx.Exec(ctx, `INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, candidate.revisionID, edge.Source, edge.Target, edge.Kind); err != nil {
				return err
			}
		}
		eventPayload, _ := json.Marshal(map[string]any{"workspaceId": candidate.workspaceID, "sequence": candidate.headSequence + 1, "revisionId": candidate.revisionID, "transactionId": candidate.transactionID, "productDesignTransactionId": groupID, "modelHash": candidate.modelHash, "changeDigest": candidate.changes.CanonicalDigest})
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, candidate.workspaceID, eventPayload); err != nil {
			return err
		}
		var position int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(position),-1)+1 FROM occccad.document_history WHERE document_id=$1`, candidate.documentID).Scan(&position); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_history(document_id,position,version_id,command_id) VALUES($1,$2,$3,$4)`, candidate.documentID, position, candidate.revisionID, auditCommandID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type) VALUES($1,$2,$3,$4)`, candidate.documentID, candidate.revisionID, auditCommandID, candidate.request.Type); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE occccad.workspaces SET head_revision_id=$1,head_sequence=$2,updated_at=now() WHERE id=$3`, candidate.revisionID, candidate.headSequence+1, candidate.workspaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`, candidate.revisionID, candidate.documentID); err != nil {
			return err
		}
	}
	// Product instance rows carry FKs to referenced revisions. Insert them only
	// after every member revision exists; workspace lock order must not leak into
	// the semantic parent/child dependency order.
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.documentType != "PRODUCT" {
			continue
		}
		var model ProductModel
		_ = json.Unmarshal(candidate.nextJSON, &model)
		if err := insertProductInstances(ctx, tx, candidate.revisionID, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

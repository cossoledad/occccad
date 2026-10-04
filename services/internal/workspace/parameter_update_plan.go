package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// ParameterUpdateNode freezes the entire parameter dependency frontier shown by
// the Product update plan. A node occurs once even when its Part has many poses.
type ParameterUpdateNode struct {
	DocumentID   string   `json:"documentId"`
	RevisionID   string   `json:"revisionId"`
	NeedsUpdate  bool     `json:"needsUpdate"`
	Dependencies []string `json:"dependencies,omitempty"`
}

type parameterUpdateSource struct{ documentID, revisionID string }

func partUpdateSources(model PartModel) []parameterUpdateSource {
	var sources []parameterUpdateSource
	for _, parameter := range model.Parameters {
		ref := parameter.Source.External
		if ref != nil && ref.Revision.Mode != "PINNED" && ref.Revision.Mode != "ISOLATED" {
			sources = append(sources, parameterUpdateSource{ref.SourceDocumentID, ref.ResolvedRevisionID})
		}
	}

	sort.Slice(sources, func(i, j int) bool { return sources[i].documentID < sources[j].documentID })
	return sources
}

func buildParameterUpdateOrder(roots []string, read func(string) (string, PartModel, error)) ([]ParameterUpdateNode, error) {
	state := map[string]int{}
	nodes := map[string]ParameterUpdateNode{}
	var order []ParameterUpdateNode
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("%w: PARAMETER_UPDATE_CYCLE", ErrValidation)
		}
		if state[id] == 2 {
			return nil
		}
		if len(state) >= 256 {
			return fmt.Errorf("%w: PARAMETER_UPDATE_DOCUMENT_BUDGET", ErrValidation)
		}
		state[id] = 1
		revision, model, err := read(id)
		if err != nil {
			return err
		}
		node := ParameterUpdateNode{DocumentID: id, RevisionID: revision}
		seen := map[string]bool{}
		for _, source := range partUpdateSources(model) {
			if err = visit(source.documentID); err != nil {
				return err
			}
			dependency := nodes[source.documentID]
			node.NeedsUpdate = node.NeedsUpdate || dependency.NeedsUpdate || dependency.RevisionID != source.revisionID
			if !seen[source.documentID] {
				node.Dependencies = append(node.Dependencies, source.documentID)
				seen[source.documentID] = true
			}
		}
		nodes[id] = node
		state[id] = 2
		order = append(order, node)
		return nil
	}
	roots = append([]string(nil), roots...)
	sort.Strings(roots)
	for _, id := range roots {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (service *Service) productParameterUpdateOrder(ctx context.Context, product ProductModel) ([]ParameterUpdateNode, error) {
	var roots []string
	for _, instance := range product.Instances {
		if instance.ReferenceMode == "PINNED" || instance.ReferenceMode == "ISOLATED" {
			continue
		}
		var kind string
		if err := service.database.QueryRow(ctx, `SELECT document_type FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, instance.ReferencedDocumentID).Scan(&kind); err != nil {
			return nil, err
		}
		// Nested Products retain their existing explicit leaf-to-root update plans.
		if kind == "PART" {
			roots = append(roots, instance.ReferencedDocumentID)
		}
	}
	return buildParameterUpdateOrder(roots, func(id string) (string, PartModel, error) {
		var revision, kind string
		var raw []byte
		var model PartModel
		err := service.database.QueryRow(ctx, `SELECT d.head_version_id::text,d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1 AND d.deleted_at IS NULL`, id).Scan(&revision, &kind, &raw)
		if err != nil {
			return "", model, err
		}
		if kind != "PART" {
			return "", model, fmt.Errorf("%w: cross-Product parameter propagation requires a nested update plan", ErrValidation)
		}
		err = json.Unmarshal(raw, &model)
		return revision, model, err
	})
}

// Each successful child has its own immutable Revision. On any failure the
// root remains unaccepted and a retry sees the completed children as pending
// references. No browser page or mutable global parameter table drives this.
func (service *Service) AcceptParametricProductUpdate(ctx context.Context, documentID, requestIDValue, digest, actor string, authorize func(string, bool) error) (DocumentView, error) {
	requestID := requestID(requestIDValue)
	receipt := CommandRequest{Type: "UPDATE_REFERENCES", RequestID: requestID, ActorID: actor, UpdatePlanDigest: digest}
	if service.committedRequestMatches(ctx, documentID, receipt) {
		return service.GetDocument(ctx, documentID, actor)
	}
	plan, err := service.GetProductUpdatePlan(ctx, documentID)
	if err != nil {
		return DocumentView{}, err
	}
	if plan.Digest != digest {
		return DocumentView{}, fmt.Errorf("%w: PRODUCT_UPDATE_PLAN_STALE", ErrValidation)
	}
	if !plan.CanAccept {
		return DocumentView{}, fmt.Errorf("%w: PRODUCT_UPDATE_BLOCKED", ErrValidation)
	}
	for _, node := range plan.ParameterUpdates {
		if err = authorize(node.DocumentID, node.NeedsUpdate); err != nil {
			return DocumentView{}, err
		}
	}
	revisions := map[string]string{}
	for _, node := range plan.ParameterUpdates {
		revisions[node.DocumentID] = node.RevisionID
	}
	completed := 0
	for _, node := range plan.ParameterUpdates {
		if !node.NeedsUpdate {
			continue
		}
		request := CommandRequest{Type: "UPDATE_REFERENCES", RequestID: commandEntityID("parameter-update", requestID+"/"+digest+"/"+node.DocumentID), ActorID: actor,
			AcceptedSourceRevisions: revisions, ExpectedUpdateRevision: node.RevisionID}
		preview, e := service.PreviewCommand(ctx, node.DocumentID, request)
		if e == nil {
			request.PreviewID = preview.PreviewID
			e = service.ExecuteCommand(ctx, node.DocumentID, request)
		}
		if e != nil {
			return DocumentView{}, fmt.Errorf("PARAMETER_UPDATE_INCOMPLETE: %d Parts updated; Product not accepted; Part %s: %w", completed, node.DocumentID, e)
		}
		// Read the exact revision produced by this idempotent request, never a
		// later user edit. The receipt is authoritative after commit.
		var revision string
		e = service.database.QueryRow(ctx, `SELECT t.result_revision_id::text FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id WHERE w.document_id=$1 AND w.name='main' AND t.request_id=$2 AND t.status='COMMITTED'`, node.DocumentID, request.RequestID).Scan(&revision)
		if e != nil {
			return DocumentView{}, e
		}
		revisions[node.DocumentID] = revision
		completed++
	}
	request := CommandRequest{Type: "UPDATE_REFERENCES", RequestID: requestID, ActorID: actor, AcceptedSourceRevisions: revisions, ExpectedUpdateRevision: plan.RootProductRevisionID}
	// Refresh the Product's existing context-binding compatibility gates after
	// accepting its leaf Part revisions. They continue to block broken supports.
	current, e := service.GetProductUpdatePlan(ctx, documentID)
	if e != nil {
		return DocumentView{}, e
	}
	if !current.CanAccept {
		return DocumentView{}, fmt.Errorf("%w: PRODUCT_UPDATE_BLOCKED_AFTER_PARAMETERS", ErrValidation)
	}
	request.UpdatePlanDigest = digest
	request.ValidatedUpdatePlanDigest = current.Digest
	return service.ApplyCommand(ctx, documentID, request)
}

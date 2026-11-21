package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workbenchconfig"
)

// DocumentAdapter owns domain semantics. The service owns reads, immutable
// revisions, transactions, CAS, history and explicit opening for every adapter.
type DocumentAdapter struct {
	PrepareCommandModel func(*preparedDomainMutation) error
	EvaluateHistory     func(context.Context, *Service, historyCommit, string) (documentEvaluationResult, error)
	Persist             func(context.Context, database.Tx, string, json.RawMessage) error
	HistoryValues       func(json.RawMessage, modelcore.ChangeSet) (map[modelcore.PropertyAddress]json.RawMessage, error)
	ApplyHistoryValues  func(json.RawMessage, map[modelcore.PropertyAddress]json.RawMessage) (json.RawMessage, error)
	Preview             func(context.Context, *Service, documentPreviewInput) (CommandPreview, *PartModel, error)
	ID                  string
	Initialize          func(context.Context, *Service, CreateDocumentRequest) (any, error)
	Prepare             func(string, []byte) ([]byte, string, *modelcore.DependencyGraph, modelcore.EvaluationManifest, error)
	Project             func(context.Context, *Service, DocumentSummary, []byte) (DocumentView, error)
	Structure           func(context.Context, *Service, DocumentStructureNode, []byte, InstancePath, map[string]bool) (DocumentStructureNode, error)
	Evaluate            func(context.Context, *Service, documentEvaluationInput) (documentEvaluationResult, error)
	AdaptCommand        func(context.Context, *Service, string, json.RawMessage, CommandRequest) (string, any, error)
}
type DocumentRegistry struct{ adapters map[string]DocumentAdapter }

func NewDocumentRegistry(adapters ...DocumentAdapter) (*DocumentRegistry, error) {
	r := &DocumentRegistry{adapters: map[string]DocumentAdapter{}}
	for _, a := range adapters {
		if a.ID == "" || r.adapters[a.ID].ID != "" {
			return nil, fmt.Errorf("duplicate or empty document adapter %q", a.ID)
		}
		if a.PrepareCommandModel == nil || a.EvaluateHistory == nil || a.Persist == nil || a.HistoryValues == nil || a.ApplyHistoryValues == nil || a.Preview == nil || a.Initialize == nil || a.Prepare == nil || a.Project == nil || a.Structure == nil || a.Evaluate == nil || a.AdaptCommand == nil {
			return nil, fmt.Errorf("document adapter %s incomplete", a.ID)
		}
		r.adapters[a.ID] = a
	}
	return r, nil
}
func (r *DocumentRegistry) Lookup(id string) (DocumentAdapter, error) {
	a, ok := r.adapters[id]
	if !ok {
		return DocumentAdapter{}, fmt.Errorf("%w: unregistered document type %s", ErrValidation, id)
	}
	return a, nil
}
func ValidateDocumentCatalog(c workbenchconfig.Catalog) error {
	for _, d := range c.Documents {
		if _, err := documentAdapters.Lookup(d.Adapter); err != nil {
			return err
		}
		if d.ID != d.Adapter {
			return fmt.Errorf("document type and adapter identity differ: %s", d.ID)
		}
	}
	return nil
}

var documentAdapters *DocumentRegistry

func init() { documentAdapters = mustDocumentRegistry() }

func mustDocumentRegistry() *DocumentRegistry {
	r, err := NewDocumentRegistry(
		DocumentAdapter{ID: "PART", PrepareCommandModel: preparePartCommandModel, EvaluateHistory: evaluatePartHistory, Persist: persistPartDocument, HistoryValues: partHistoryValues, ApplyHistoryValues: applyPartHistoryValues, Preview: previewPartDocument, Initialize: initializePartDocument, Prepare: preparePartDocument, Project: projectPartDocument, Structure: projectPartStructure, Evaluate: evaluatePartDocument, AdaptCommand: adaptPartDocumentCommand},
		DocumentAdapter{ID: "PRODUCT", PrepareCommandModel: prepareProductCommandModel, EvaluateHistory: evaluateProductHistory, Persist: persistProductDocument, HistoryValues: productHistoryValues, ApplyHistoryValues: applyProductHistoryValues, Preview: previewProductDocument, Initialize: initializeProductDocument, Prepare: prepareProductDocument, Project: projectProductDocument, Structure: projectProductStructure, Evaluate: evaluateProductDocument, AdaptCommand: adaptProductDocumentCommand},
	)
	if err != nil {
		panic(err)
	}
	return r
}
func initializePartDocument(ctx context.Context, s *Service, request CreateDocumentRequest) (any, error) {
	model := newPartModelFor(request.RequestID)
	if request.hideDefaultReferences {
		for i := range model.DatumPlanes {
			model.DatumPlanes[i].Visible = boolPointer(false)
		}
		for i := range model.AxisSystems {
			model.AxisSystems[i].Visible = boolPointer(false)
		}
	}
	key, err := s.ensureVisualizationArtifact(ctx, model)
	if err != nil {
		return nil, err
	}
	model.Bodies[0].GeometryKey = key
	return model, nil
}
func initializeProductDocument(context.Context, *Service, CreateDocumentRequest) (any, error) {
	return ProductModel{Instances: []ProductInstance{}}, nil
}
func adaptPartDocumentCommand(ctx context.Context, s *Service, id string, model json.RawMessage, request CommandRequest) (string, any, error) {
	return s.adaptLegacyCommand(ctx, id, "PART", model, request)
}
func adaptProductDocumentCommand(ctx context.Context, s *Service, id string, model json.RawMessage, request CommandRequest) (string, any, error) {
	return s.adaptLegacyCommand(ctx, id, "PRODUCT", model, request)
}

func persistPartDocument(context.Context, database.Tx, string, json.RawMessage) error { return nil }
func persistProductDocument(ctx context.Context, tx database.Tx, revisionID string, raw json.RawMessage) error {
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return err
	}
	return insertProductInstances(ctx, tx, revisionID, model)
}

func preparePartCommandModel(p *preparedDomainMutation) error {
	p.basePart = new(PartModel)
	return json.Unmarshal(p.modelJSON, p.basePart)
}
func prepareProductCommandModel(p *preparedDomainMutation) error {
	var model ProductModel
	if err := json.Unmarshal(p.modelJSON, &model); err != nil {
		return err
	}
	return validateAssemblyDefinitionFormat(model)
}

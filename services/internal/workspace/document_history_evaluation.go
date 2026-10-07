package workspace

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/occccad/occccad/internal/modelcore"
	"strings"
)

func evaluatePartHistory(ctx context.Context, service *Service, input historyCommit, revisionID string) (documentEvaluationResult, error) {
	modelHash := canonicalModelHash(input.modelJSON)
	revisionState, evaluationStatus := "READY", "SUCCEEDED"
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	failure := func() error {
		var model PartModel
		if err = json.Unmarshal(input.modelJSON, &model); err != nil {
			return err
		}
		normalizePartModel(&model)
		// Display compensation uses the same frozen geometry as a display command.
		// No parameter/publication preparation or solid evaluation is necessary.
		displayOnly := len(input.changes.Changes) > 0
		for _, change := range input.changes.Changes {
			if !strings.HasPrefix(change.Target.SlotID, "display.") {
				displayOnly = false
				break
			}
		}
		if !displayOnly {
			if err = resolveRuntimeParameters(ctx, &model); err != nil {
				return err
			}
			if err = service.resolvePartPublications(ctx, input.documentID, input.requestID, revisionID, &model); err != nil {
				return err
			}
		}
		if _, failedRevision, failedEvaluation, unresolved := unresolvedExternalRevisionOutcome(model); unresolved {
			revisionState, evaluationStatus = failedRevision, failedEvaluation
			for i := range model.Bodies {
				model.Bodies[i].GeometryKey = ""
			}
		} else if displayOnly {
			if partHasFailedFeature(model) {
				revisionState, evaluationStatus = "FAILED", "FAILED"
			}
		} else {
			err = service.evaluatePartBodies(ctx, input.requestID, &model)
			var failed *solidEvaluationFailure
			if errors.As(err, &failed) {
				revisionState, evaluationStatus = "FAILED", "FAILED"
				err = nil
			}
		}
		if err == nil {
			var previous PartModel
			var beforePartJSON []byte
			if readErr := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, input.headRevision).Scan(&beforePartJSON); readErr != nil {
				return readErr
			}
			if readErr := json.Unmarshal(beforePartJSON, &previous); readErr != nil {
				return readErr
			}
			retainFailedBodyDisplay(&model, previous, input.headRevision)
			input.modelJSON, _ = json.Marshal(model)
			modelHash = canonicalModelHash(input.modelJSON)
			graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, input.changes.ImpactSeeds, nil)
			attachEvaluationRuntime(ctx, &manifest)
		}
		return err
	}()
	return documentEvaluationResult{nextJSON: input.modelJSON, modelHash: modelHash, graph: graph, manifest: manifest, changes: input.changes, revisionState: revisionState, evaluationStatus: evaluationStatus}, failure
}
func evaluateProductHistory(ctx context.Context, service *Service, input historyCommit, revisionID string) (documentEvaluationResult, error) {
	modelHash := canonicalModelHash(input.modelJSON)
	revisionState, evaluationStatus := "READY", "SUCCEEDED"
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	failure := func() error {
		var model ProductModel
		if err = json.Unmarshal(input.modelJSON, &model); err == nil {
			var before json.RawMessage
			if err = service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, input.headRevision).Scan(&before); err != nil {
				return err
			}
			var previous ProductModel
			if err = json.Unmarshal(before, &previous); err != nil {
				return err
			}
			if !constraintVisibilityOnlyHistory(previous, model, input.changes) {
				if err = service.verifyAssemblyHistory(ctx, input.documentID, revisionID, input.requestID, model); err != nil {
					return err
				}
			}
			input.changes = appendAssemblyEvaluationChanges(input.changes, previous, model)
			input.modelJSON, err = json.Marshal(model)
			if err != nil {
				return err
			}
			input.changes, err = reconcilePersistedChanges("PRODUCT", before, input.modelJSON, input.changes)
			if err != nil {
				return err
			}
			modelHash = canonicalModelHash(input.modelJSON)
			graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, input.changes.ImpactSeeds, nil)
		}
		return err
	}()
	return documentEvaluationResult{nextJSON: input.modelJSON, modelHash: modelHash, graph: graph, manifest: manifest, changes: input.changes, revisionState: revisionState, evaluationStatus: evaluationStatus}, failure
}

package workspace

import (
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

func preparePartDocument(revisionID string, modelJSON []byte) ([]byte, string, *modelcore.DependencyGraph, modelcore.EvaluationManifest, error) {
	modelHash := ""
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	var model PartModel
	if err = json.Unmarshal(modelJSON, &model); err != nil {
		return nil, "", nil, manifest, err
	}
	normalizePartModel(&model)
	if err = validateAndResolvePartParameters(&model); err != nil {
		return nil, "", nil, manifest, err
	}
	modelJSON, _ = json.Marshal(model)
	modelHash = canonicalModelHash(modelJSON)
	graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, nil, nil)
	return modelJSON, modelHash, graph, manifest, err
}
func prepareProductDocument(revisionID string, modelJSON []byte) ([]byte, string, *modelcore.DependencyGraph, modelcore.EvaluationManifest, error) {
	modelHash := ""
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	var model ProductModel
	if err = json.Unmarshal(modelJSON, &model); err != nil {
		return nil, "", nil, manifest, err
	}
	modelHash = canonicalModelHash(modelJSON)
	graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, nil, nil)
	return modelJSON, modelHash, graph, manifest, err
}

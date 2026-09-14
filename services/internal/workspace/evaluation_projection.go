package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

func ensureFeatureParameters(model *PartModel) {
	existing := map[string]struct{}{}
	for _, parameter := range model.Parameters {
		existing[parameter.ParameterID] = struct{}{}
	}
	add := func(featureID, slot, key string, value float64) {
		id := "parameter:" + featureID + ":" + slot
		if _, exists := existing[id]; exists {
			return
		}
		quantity, _ := modelcore.NewQuantity(value, "mm")
		model.Parameters = append(model.Parameters, modelcore.ParameterDefinition{ParameterID: id, Key: key, Label: key, ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension, Role: "INPUT", Source: modelcore.ValueSource{Literal: &quantity}})
		existing[id] = struct{}{}
	}
	for _, feature := range model.Features {
		keyPrefix := strings.NewReplacer("-", "_", ":", "_").Replace(feature.ID)
		if isSolidGenerator(feature.Type) && strings.ToUpper(feature.Type) != "REVOLVE" {
			add(feature.ID, "length", keyPrefix+"_length", feature.Length)
		}
	}
	sort.Slice(model.Parameters, func(i, j int) bool { return model.Parameters[i].ParameterID < model.Parameters[j].ParameterID })
}

func validateAndResolvePartParameters(model *PartModel) error {
	if err := validatePartStructure(*model); err != nil {
		return err
	}
	nodes := make([]modelcore.DependencyNode, 0, len(model.Parameters))
	edges := []modelcore.DependencyEdge{}
	definitions := map[string]modelcore.ParameterDefinition{}
	for _, parameter := range model.Parameters {
		if _, exists := definitions[parameter.ParameterID]; exists {
			return fmt.Errorf("%w: duplicate parameter id", ErrValidation)
		}
		definitions[parameter.ParameterID] = parameter
		source, _ := json.Marshal(parameter.Source)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Phase: 1, Type: "PARAMETER", CanonicalInput: source})
		if parameter.Source.Expression != nil {
			for _, read := range parameter.Source.Expression.Reads {
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Kind: modelcore.ReadValue})
			}
		}
	}
	graph, err := modelcore.NewDependencyGraph(nodes, edges)
	if err != nil {
		return err
	}
	values := map[string]modelcore.Quantity{}
	for _, key := range graph.TopologicalOrder() {
		id := strings.TrimPrefix(string(key), "parameter:")
		parameter := definitions[id]
		var value modelcore.Quantity
		if parameter.Source.Literal != nil {
			value = *parameter.Source.Literal
		} else if parameter.Source.Expression != nil {
			value, err = modelcore.EvaluateExpression(*parameter.Source.Expression, values)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%w: parameter %s has no source", ErrValidation, id)
		}
		if !value.Dimension.Equal(parameter.Dimension) {
			return fmt.Errorf("%w: parameter %s dimension mismatch", modelcore.ErrUnitMismatch, id)
		}
		values[id] = value
	}
	for index := range model.Features {
		feature := &model.Features[index]
		if isSolidGenerator(feature.Type) && strings.ToUpper(feature.Type) != "REVOLVE" {
			feature.Length = values["parameter:"+feature.ID+":length"].SIValue * 1000
		}
	}
	return nil
}

func validatePartStructure(model PartModel) error {
	datums := map[string]struct{}{}
	for _, datum := range model.DatumPlanes {
		if datum.ID == "" {
			return fmt.Errorf("%w: datum plane identity is required", ErrValidation)
		}
		if _, exists := datums[datum.ID]; exists {
			return fmt.Errorf("%w: duplicate datum plane identity %s", ErrValidation, datum.ID)
		}
		datums[datum.ID] = struct{}{}
	}
	features := map[string]Feature{}
	for _, feature := range model.Features {
		if feature.ID == "" {
			return fmt.Errorf("%w: feature identity is required", ErrValidation)
		}
		if _, exists := features[feature.ID]; exists {
			return fmt.Errorf("%w: duplicate feature identity %s", ErrValidation, feature.ID)
		}
		if feature.Sketch != nil && len(datums) > 0 && feature.Sketch.Support.DatumPlaneID != "" {
			if _, exists := datums[feature.Sketch.Support.DatumPlaneID]; !exists {
				return fmt.Errorf("%w: sketch %s references unknown datum plane %s", ErrValidation, feature.ID, feature.Sketch.Support.DatumPlaneID)
			}
		}
		if isSolidGenerator(feature.Type) {
			profile, exists := features[feature.Profile]
			if !exists || !strings.Contains(strings.ToUpper(profile.Type), "SKETCH") {
				return fmt.Errorf("%w: solid feature %s requires an earlier sketch profile %s", ErrValidation, feature.ID, feature.Profile)
			}
			operation := strings.ToUpper(feature.Operation)
			if operation != "NEW_BODY" && operation != "ADD" && operation != "REMOVE" && operation != "INTERSECT" {
				return fmt.Errorf("%w: solid feature %s has invalid BodyOperation", ErrValidation, feature.ID)
			}
		}
		features[feature.ID] = feature
	}
	return nil
}

func buildPartEvaluation(model PartModel, revisionID, modelHash string, seeds []modelcore.DependencyKey, prior *modelcore.EvaluationManifest) (*modelcore.DependencyGraph, modelcore.EvaluationManifest, error) {
	if err := validatePartStructure(model); err != nil {
		return nil, modelcore.EvaluationManifest{}, err
	}
	nodes := []modelcore.DependencyNode{}
	edges := []modelcore.DependencyEdge{}
	for _, datum := range model.DatumPlanes {
		data, _ := json.Marshal(datum)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("datum:" + datum.ID), Phase: 1, Type: "DATUM_PLANE", CanonicalInput: data})
	}
	for _, parameter := range model.Parameters {
		source, _ := json.Marshal(parameter.Source)
		key := modelcore.DependencyKey("parameter:" + parameter.ParameterID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 1, Type: "PARAMETER", CanonicalInput: source})
		if parameter.Source.Expression != nil {
			for _, read := range parameter.Source.Expression.Reads {
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: key, Kind: modelcore.ReadValue})
			}
		}
	}
	for _, feature := range model.Features {
		key := modelcore.DependencyKey("feature:" + feature.ID)
		data, _ := json.Marshal(feature)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: feature.Type, CanonicalInput: data})
		prefix := "parameter:" + feature.ID + ":"
		for _, parameter := range model.Parameters {
			if strings.HasPrefix(parameter.ParameterID, prefix) {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Target: key, Kind: modelcore.ReadValue})
			}
		}
		if isSolidGenerator(feature.Type) {
			edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("feature:" + feature.Profile), Target: key, Kind: modelcore.ReadGeometry})
		}
		if feature.Sketch != nil {
			edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("datum:" + feature.Sketch.Support.DatumPlaneID), Target: key, Kind: modelcore.ReadGeometry})
		}
	}
	graph, err := modelcore.NewDependencyGraph(nodes, edges)
	if err != nil {
		return nil, modelcore.EvaluationManifest{}, err
	}
	evaluator := func(node modelcore.DependencyNode, deps map[modelcore.DependencyKey]modelcore.NodeResult) (string, error) {
		data, _ := json.Marshal(struct {
			Node json.RawMessage
			Deps map[modelcore.DependencyKey]modelcore.NodeResult
		}{node.CanonicalInput, deps})
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:]), nil
	}
	manifest, err := graph.Evaluate(revisionID, modelHash, evaluatorVersion, "units-mm-v1", seeds, prior, evaluator)
	return graph, manifest, err
}

func canonicalModelHash(modelJSON []byte) string {
	var value any
	if json.Unmarshal(modelJSON, &value) != nil {
		sum := sha256.Sum256(modelJSON)
		return hex.EncodeToString(sum[:])
	}
	canonical, _ := json.Marshal(value)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func prepareInitialEvaluation(documentType, revisionID string, modelJSON []byte) ([]byte, string, *modelcore.DependencyGraph, modelcore.EvaluationManifest, string, error) {
	modelHash := ""
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var err error
	if documentType == "PART" {
		var model PartModel
		if err = json.Unmarshal(modelJSON, &model); err != nil {
			return nil, "", nil, manifest, "", err
		}
		normalizePartModel(&model)
		if err = validateAndResolvePartParameters(&model); err != nil {
			return nil, "", nil, manifest, "", err
		}
		modelJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(modelJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, nil, nil)
	} else {
		var model ProductModel
		if err = json.Unmarshal(modelJSON, &model); err != nil {
			return nil, "", nil, manifest, "", err
		}
		modelHash = canonicalModelHash(modelJSON)
		graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, nil, nil)
	}
	if err != nil {
		return nil, "", nil, manifest, "", err
	}
	digest, err := graph.Digest()
	return modelJSON, modelHash, graph, manifest, digest, err
}

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
	existing := map[string]int{}
	for index, parameter := range model.Parameters {
		existing[parameter.ParameterID] = index
	}
	managedPrefixes := map[string]struct{}{}
	desired := map[string]struct{}{}
	add := func(featureID, slot, key, label, unit string, value float64, dimension modelcore.Dimension) {
		id := "parameter:" + featureID + ":" + slot
		desired[id] = struct{}{}
		if index, exists := existing[id]; exists {
			parameter := &model.Parameters[index]
			parameter.Dimension = dimension
			parameter.ValueType = modelcore.ValueQuantity
			parameter.DisplayUnit = unit
			if parameter.Label == "" {
				parameter.Label = label
			}
			return
		}
		quantity, _ := modelcore.NewQuantity(value, unit)
		model.Parameters = append(model.Parameters, modelcore.ParameterDefinition{ParameterID: id, Key: key, Label: label,
			ValueType: modelcore.ValueQuantity, Dimension: dimension, DisplayUnit: unit, Role: "INPUT",
			Source: modelcore.ValueSource{Literal: &quantity}, EvaluatedValue: &quantity})
		existing[id] = len(model.Parameters) - 1
	}
	for _, feature := range model.Features {
		managedPrefixes["parameter:"+feature.ID+":"] = struct{}{}
		keyPrefix := strings.NewReplacer("-", "_", ":", "_").Replace(feature.ID)
		if isSolidGenerator(feature.Type) && strings.ToUpper(feature.Type) != "REVOLVE" {
			add(feature.ID, "length", keyPrefix+"_length", "Length", "mm", feature.Length, modelcore.LengthDimension)
		}
		if feature.Sketch != nil {
			for _, constraint := range feature.Sketch.Constraints {
				if !isDimensionalConstraint(constraint.Kind) || constraint.Value == nil {
					continue
				}
				unit, dimension := sketchConstraintUnitAndDimension(constraint.Kind)
				slot := "constraint:" + constraint.ID + ":value"
				key := keyPrefix + "_" + strings.ToLower(constraint.Kind) + "_" + parameterKeyFragment(constraint.ID)
				add(feature.ID, slot, key, constraint.Kind, unit, *constraint.Value, dimension)
			}
		}
	}
	filtered := model.Parameters[:0]
	for _, parameter := range model.Parameters {
		managed := false
		for prefix := range managedPrefixes {
			if strings.HasPrefix(parameter.ParameterID, prefix) {
				managed = true
				break
			}
		}
		if managed {
			if _, keep := desired[parameter.ParameterID]; !keep {
				continue
			}
		}
		filtered = append(filtered, parameter)
	}
	model.Parameters = filtered
	for featureIndex := range model.Features {
		feature := &model.Features[featureIndex]
		if feature.Sketch == nil {
			continue
		}
		for constraintIndex := range feature.Sketch.Constraints {
			constraint := &feature.Sketch.Constraints[constraintIndex]
			if isDimensionalConstraint(constraint.Kind) {
				constraint.ParameterID = sketchConstraintParameterID(feature.ID, constraint.ID)
			}
		}
	}
	sort.Slice(model.Parameters, func(i, j int) bool { return model.Parameters[i].ParameterID < model.Parameters[j].ParameterID })
}

func parameterKeyFragment(value string) string {
	return strings.NewReplacer("-", "_", ":", "_").Replace(value)
}

func sketchConstraintParameterID(sketchID, constraintID string) string {
	return "parameter:" + sketchID + ":constraint:" + constraintID + ":value"
}

func sketchConstraintUnitAndDimension(kind string) (string, modelcore.Dimension) {
	if strings.EqualFold(kind, "ANGLE") {
		return "deg", modelcore.AngleDimension
	}
	return "mm", modelcore.LengthDimension
}

func quantityInDisplayUnit(value modelcore.Quantity, unit string) (float64, error) {
	one, err := modelcore.NewQuantity(1, unit)
	if err != nil {
		return 0, err
	}
	if !one.Dimension.Equal(value.Dimension) {
		return 0, fmt.Errorf("%w: display unit %s has the wrong dimension", modelcore.ErrUnitMismatch, unit)
	}
	return value.SIValue / one.SIValue, nil
}

func validateAndResolvePartParameters(model *PartModel) error {
	if err := validatePartStructure(*model); err != nil {
		return err
	}
	nodes := make([]modelcore.DependencyNode, 0, len(model.Parameters)*2)
	edges := []modelcore.DependencyEdge{}
	definitions := map[string]modelcore.ParameterDefinition{}
	keys := map[string]string{}
	for _, parameter := range model.Parameters {
		if parameter.ParameterID == "" {
			return fmt.Errorf("%w: parameter id is required", ErrValidation)
		}
		if _, exists := definitions[parameter.ParameterID]; exists {
			return fmt.Errorf("%w: duplicate parameter id", ErrValidation)
		}
		if parameter.Key == "" || keys[parameter.Key] != "" {
			return fmt.Errorf("%w: parameter key must be non-empty and unique", ErrValidation)
		}
		sourceCount := 0
		if parameter.Source.Literal != nil {
			sourceCount++
		}
		if parameter.Source.Expression != nil {
			sourceCount++
		}
		if parameter.Source.External != nil {
			sourceCount++
		}
		if sourceCount != 1 {
			return fmt.Errorf("%w: parameter %s must have exactly one source", ErrValidation, parameter.ParameterID)
		}
		if parameter.ValueType != modelcore.ValueQuantity {
			return fmt.Errorf("%w: %w: parameter %s is not a quantity", ErrValidation, modelcore.ErrParameterType, parameter.ParameterID)
		}
		if parameter.Source.Expression != nil && (!parameter.Source.Expression.ResultDimension.Equal(parameter.Dimension) ||
			parameter.Source.Expression.ResultType != parameter.ValueType) {
			return fmt.Errorf("%w: %w: parameter %s expression result type changed", ErrValidation, modelcore.ErrUnitMismatch, parameter.ParameterID)
		}
		if external := parameter.Source.External; external != nil {
			validMode := external.Revision.Mode == "PINNED" || external.Revision.Mode == "FOLLOW_HEAD" || external.Revision.Mode == "FOLLOW_WORKSPACE_WITH_ACCEPT"
			if external.SourceDocumentID == "" || external.PublicationID == "" || external.ContractVersion == "" || !validMode ||
				external.Revision.RevisionID == "" || external.ResolvedRevisionID == "" ||
				external.Revision.RevisionID != external.ResolvedRevisionID {
				return fmt.Errorf("%w: EXTERNAL_PARAMETER_REFERENCE_INCOMPLETE", ErrValidation)
			}
			if external.ExpectedType != parameter.ValueType || !external.ExpectedDimension.Equal(parameter.Dimension) ||
				!external.ResolvedValue.Dimension.Equal(parameter.Dimension) {
				return fmt.Errorf("%w: EXTERNAL_PARAMETER_CONTRACT_INCOMPATIBLE", ErrValidation)
			}
			if external.ResolvedValueDigest == "" || external.ResolvedValueDigest != resolvedDigest(external.ResolvedValue) {
				return fmt.Errorf("%w: EXTERNAL_PARAMETER_VALUE_DIGEST_MISMATCH", ErrValidation)
			}
			snapshot := external.ResolutionSnapshot
			if snapshot.Status != "CONNECTED" || snapshot.SourceRevisionID != external.ResolvedRevisionID ||
				snapshot.PublicationID != external.PublicationID || snapshot.ContractDigest == "" ||
				snapshot.ValueDigest != external.ResolvedValueDigest {
				return fmt.Errorf("%w: EXTERNAL_PARAMETER_RESOLUTION_SNAPSHOT_INCOMPLETE", ErrValidation)
			}
		}
		keys[parameter.Key] = parameter.ParameterID
		definitions[parameter.ParameterID] = parameter
		source, _ := json.Marshal(parameter.Source)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Phase: 1, Type: "PARAMETER", CanonicalInput: source})
		if parameter.Source.External != nil {
			externalKey := modelcore.DependencyKey("external-parameter:" + parameter.ParameterID)
			nodes = append(nodes, modelcore.DependencyNode{Key: externalKey, Phase: 0, Type: "EXTERNAL_PARAMETER_SNAPSHOT", CanonicalInput: source})
			edges = append(edges, modelcore.DependencyEdge{Source: externalKey,
				Target: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Kind: modelcore.ReadValue})
		}
		if parameter.Source.Expression != nil {
			for _, read := range parameter.Source.Expression.Reads {
				readID := strings.TrimPrefix(string(read), "parameter:")
				if _, exists := definitions[readID]; !exists {
					// Definitions may appear later in canonical order; the complete check
					// below distinguishes that from a deleted parameter.
					continue
				}
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Kind: modelcore.ReadValue})
			}
		}
	}
	for _, parameter := range model.Parameters {
		if parameter.Source.Expression == nil {
			continue
		}
		for _, read := range parameter.Source.Expression.Reads {
			readID := strings.TrimPrefix(string(read), "parameter:")
			if _, exists := definitions[readID]; !exists {
				return fmt.Errorf("%w: %w: %s", ErrValidation, modelcore.ErrParameterMissing, readID)
			}
			// The earlier pass only emitted edges to definitions already seen.
			found := false
			for _, edge := range edges {
				if edge.Source == read && edge.Target == modelcore.DependencyKey("parameter:"+parameter.ParameterID) {
					found = true
					break
				}
			}
			if !found {
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: modelcore.DependencyKey("parameter:" + parameter.ParameterID), Kind: modelcore.ReadValue})
			}
		}
	}
	graph, err := modelcore.NewDependencyGraph(nodes, edges)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrValidation, err)
	}
	values := map[string]modelcore.Quantity{}
	for _, key := range graph.TopologicalOrder() {
		if strings.HasPrefix(string(key), "external-parameter:") {
			continue
		}
		id := strings.TrimPrefix(string(key), "parameter:")
		parameter := definitions[id]
		var value modelcore.Quantity
		if parameter.Source.Literal != nil {
			value = *parameter.Source.Literal
		} else if parameter.Source.Expression != nil {
			value, err = modelcore.EvaluateExpression(*parameter.Source.Expression, values)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrValidation, err)
			}
		} else if parameter.Source.External != nil {
			value = parameter.Source.External.ResolvedValue
		} else {
			return fmt.Errorf("%w: parameter %s has no source", ErrValidation, id)
		}
		if !value.Dimension.Equal(parameter.Dimension) {
			return fmt.Errorf("%w: %w: parameter %s dimension mismatch", ErrValidation, modelcore.ErrUnitMismatch, id)
		}
		values[id] = value
		for index := range model.Parameters {
			if model.Parameters[index].ParameterID == id {
				resolved := value
				model.Parameters[index].EvaluatedValue = &resolved
				break
			}
		}
	}
	for index := range model.Features {
		feature := &model.Features[index]
		if isSolidGenerator(feature.Type) && strings.ToUpper(feature.Type) != "REVOLVE" {
			feature.Length = values["parameter:"+feature.ID+":length"].SIValue * 1000
			if !positiveFinite(feature.Length) {
				return fmt.Errorf("%w: extrude length must evaluate to a positive finite value", ErrValidation)
			}
		}
		if feature.Sketch != nil {
			for constraintIndex := range feature.Sketch.Constraints {
				constraint := &feature.Sketch.Constraints[constraintIndex]
				if !isDimensionalConstraint(constraint.Kind) {
					continue
				}
				value, exists := values[constraint.ParameterID]
				if !exists {
					return fmt.Errorf("%w: %w: %s", ErrValidation, modelcore.ErrParameterMissing, constraint.ParameterID)
				}
				display, displayErr := quantityInDisplayUnit(value, constraint.Unit)
				if displayErr != nil {
					return fmt.Errorf("%w: %w", ErrValidation, displayErr)
				}
				constraint.Value = &display
			}
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
		if feature.Sketch != nil {
			if err := validateSketch(*feature.Sketch); err != nil {
				return err
			}
			support := feature.Sketch.Support
			switch support.Type {
			case "DATUM_PLANE":
				if _, exists := datums[support.DatumPlaneID]; !exists {
					return fmt.Errorf("%w: sketch %s references unknown datum plane %s", ErrValidation, feature.ID, support.DatumPlaneID)
				}
			case "PLANAR_FACE":
				if support.PersistentSelection == nil || support.SourceVersionID == "" {
					return fmt.Errorf("%w: sketch %s has an incomplete PLANAR_FACE support", ErrValidation, feature.ID)
				}
				if err := support.PersistentSelection.Validate(); err != nil {
					return fmt.Errorf("%w: sketch %s support: %v", ErrValidation, feature.ID, err)
				}
				if support.PersistentSelection.ExpectedType != modelcore.PersistentTopologyFace ||
					support.PersistentSelection.CreationEvidence.GeometryType != "PLANE" {
					return fmt.Errorf("%w: SUPPORT_TYPE_MISMATCH", ErrValidation)
				}
				if _, exists := features[support.PersistentSelection.Anchor.FeatureID]; !exists {
					return fmt.Errorf("%w: FAILED_SUPPORT: support feature must precede sketch %s", ErrValidation, feature.ID)
				}
				if _, _, _, err := validatedSupportFrame(support.Origin, support.XDirection, support.Normal); err != nil {
					return fmt.Errorf("%w: SUPPORT_FRAME_INVALID: %v", ErrValidation, err)
				}
			default:
				return fmt.Errorf("%w: sketch %s has unsupported support type %s", ErrValidation, feature.ID, support.Type)
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
	contextIDs := map[string]bool{}
	contextInputIDs, contextInputNames := map[string]bool{}, map[string]bool{}
	for _, input := range model.ContextInputs {
		name := scopedNameKey(input.Name)
		if input.ID == "" || contextInputIDs[input.ID] || name == "" || contextInputNames[name] || input.Type == "" || input.Target.Kind == "" || input.Target.TargetID == "" {
			return fmt.Errorf("%w: ContextInput identity, name, target and contract must be complete and unique", ErrValidation)
		}
		contextInputIDs[input.ID], contextInputNames[name] = true, true
	}
	for _, reference := range model.ContextReferences {
		if strings.TrimSpace(reference.ID) == "" || contextIDs[reference.ID] {
			return fmt.Errorf("%w: context reference identity must be unique", ErrValidation)
		}
		contextIDs[reference.ID] = true
		if reference.OwningWorkspace == "" || reference.SourceDocumentID == "" || reference.Publication.PublicationID == "" || reference.ResolvedRevisionID == "" {
			return fmt.Errorf("%w: context reference %s is incomplete", ErrValidation, reference.ID)
		}
		if reference.ReferenceMode != "PINNED" && reference.ReferenceMode != "FOLLOW_HEAD" &&
			reference.ReferenceMode != "FOLLOW_WORKSPACE_WITH_ACCEPT" && reference.ReferenceMode != "ISOLATED" {
			return fmt.Errorf("%w: context reference %s has invalid mode", ErrValidation, reference.ID)
		}
	}
	return validatePublicationDefinitions(model)
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
	for _, datum := range model.AxisSystems {
		data, _ := json.Marshal(datum)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("datum:" + datum.ID), Phase: 1, Type: "AXIS_SYSTEM", CanonicalInput: data})
	}
	for _, datum := range model.DatumAxes {
		data, _ := json.Marshal(datum)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("datum:" + datum.ID), Phase: 1, Type: "DATUM_AXIS", CanonicalInput: data})
	}
	for _, parameter := range model.Parameters {
		source, _ := json.Marshal(parameter.Source)
		key := modelcore.DependencyKey("parameter:" + parameter.ParameterID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 1, Type: "PARAMETER", CanonicalInput: source})
		if parameter.Source.External != nil {
			externalKey := modelcore.DependencyKey("external-parameter:" + parameter.ParameterID)
			nodes = append(nodes, modelcore.DependencyNode{Key: externalKey, Phase: 0,
				Type: "EXTERNAL_PARAMETER_SNAPSHOT", CanonicalInput: source})
			edges = append(edges, modelcore.DependencyEdge{Source: externalKey, Target: key, Kind: modelcore.ReadValue})
		}
		if parameter.Source.Expression != nil {
			for _, read := range parameter.Source.Expression.Reads {
				edges = append(edges, modelcore.DependencyEdge{Source: read, Target: key, Kind: modelcore.ReadValue})
			}
		}
	}
	for _, reference := range model.ContextReferences {
		data, _ := json.Marshal(reference)
		key := modelcore.DependencyKey("context-reference:" + reference.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 0, Type: "CONTEXT_REFERENCE_SNAPSHOT", CanonicalInput: data})
		if reference.LocalTargetID == "" {
			continue
		}
		switch reference.Publication.ExpectedType {
		case "PARAMETER":
			edges = append(edges, modelcore.DependencyEdge{Source: key,
				Target: modelcore.DependencyKey("parameter:" + reference.LocalTargetID), Kind: modelcore.ReadValue})
		case "PLANE", "AXIS":
			edges = append(edges, modelcore.DependencyEdge{Source: key,
				Target: modelcore.DependencyKey("datum:" + reference.LocalTargetID), Kind: modelcore.ReadGeometry})
		case "CURVE":
			edges = append(edges, modelcore.DependencyEdge{Source: key,
				Target: modelcore.DependencyKey("feature:" + reference.LocalTargetID), Kind: modelcore.ReadTopology})
		}
	}
	for _, input := range model.ContextInputs {
		data, _ := json.Marshal(input)
		key := modelcore.DependencyKey("context-input:" + input.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 0, Type: "CONTEXT_INPUT", CanonicalInput: data})
		switch input.Target.Kind {
		case "PARAMETER":
			edges = append(edges, modelcore.DependencyEdge{Source: key, Target: modelcore.DependencyKey("parameter:" + input.Target.TargetID), Kind: modelcore.ReadValue})
		case "DATUM":
			edges = append(edges, modelcore.DependencyEdge{Source: key, Target: modelcore.DependencyKey("datum:" + input.Target.TargetID), Kind: modelcore.ReadGeometry})
		case "SKETCH_EXTERNAL_GEOMETRY", "FEATURE_INPUT":
			edges = append(edges, modelcore.DependencyEdge{Source: key, Target: modelcore.DependencyKey("feature:" + input.Target.TargetID), Kind: modelcore.ReadGeometry})
		}
	}
	bodyTipFeatureID := ""
	featureIDs := map[string]bool{}
	for _, feature := range model.Features {
		featureIDs[feature.ID] = true
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
			if bodyTipFeatureID != "" {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("feature:" + bodyTipFeatureID), Target: key, Kind: modelcore.ReadGeometry})
			}
		}
		if feature.Sketch != nil {
			readsTopology := (feature.Sketch.Support.Type == "PLANAR_FACE" && feature.Sketch.Support.PersistentSelection != nil) || len(feature.Sketch.ExternalGeometry) > 0
			if readsTopology {
				if bodyTipFeatureID != "" {
					edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("feature:" + bodyTipFeatureID), Target: key, Kind: modelcore.ReadTopology})
				}
			} else {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("datum:" + feature.Sketch.Support.DatumPlaneID), Target: key, Kind: modelcore.ReadGeometry})
			}
		}
		if isSolidGenerator(feature.Type) {
			bodyTipFeatureID = feature.ID
		}
	}
	datumIDs := map[string]bool{}
	for _, datum := range model.DatumPlanes {
		datumIDs[datum.ID] = true
	}
	for _, datum := range model.AxisSystems {
		datumIDs[datum.ID] = true
	}
	for _, datum := range model.DatumAxes {
		datumIDs[datum.ID] = true
	}
	parameterIDs := map[string]bool{}
	for _, parameter := range model.Parameters {
		parameterIDs[parameter.ParameterID] = true
	}
	for _, publication := range model.Publications {
		key := modelcore.DependencyKey("publication:" + publication.ID)
		data, _ := json.Marshal(publication)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 3, Type: "PUBLICATION", CanonicalInput: data})
		switch publication.Target.Kind {
		case "DATUM":
			if datumIDs[publication.Target.DatumID] {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("datum:" + publication.Target.DatumID), Target: key, Kind: modelcore.ReadGeometry})
			}
		case "PARAMETER":
			if parameterIDs[publication.Target.ParameterID] {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("parameter:" + publication.Target.ParameterID), Target: key, Kind: modelcore.ReadValue})
			}
		case "FEATURE_OUTPUT":
			if featureIDs[publication.Target.FeatureID] {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("feature:" + publication.Target.FeatureID), Target: key, Kind: modelcore.ReadGeometry})
			}
		case "TOPOLOGY":
			source := bodyTipFeatureID
			if source != "" && featureIDs[source] {
				edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("feature:" + source), Target: key, Kind: modelcore.ReadTopology})
			}
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

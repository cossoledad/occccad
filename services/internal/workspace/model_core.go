package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

const (
	typeCreatePattern          = "occccad://part/pattern/create"
	typeCreateSketch           = "occccad://part/sketch/create"
	typeEditSketch             = "occccad://part/sketch/edit"
	typeCreatePad              = "occccad://part/pad/create"
	typeCreateBooleanFeature   = "occccad://part/boolean/create"
	typeCreateModifierFeature  = "occccad://part/modifier/create"
	typeCreateSolidFeature     = "occccad://part/solid-generator/create"
	typeSetFeatureSuppression  = "occccad://part/feature/suppression/set"
	typeEditFeature            = "occccad://part/feature/edit"
	typeRenameFeature          = "occccad://part/feature/rename"
	typeCreateDatumPlane       = "occccad://part/datum-plane/create"
	typeCreateDatumAxis        = "occccad://part/datum-axis/create"
	typeImportExchange         = "occccad://part/exchange/import"
	typeSetParameterLiteral    = "occccad://parameter/literal/set"
	typeSetParameterExpression = "occccad://parameter/expression/set"
	typeRenameParameter        = "occccad://parameter/key/rename"
	typeDefinitionVisibility   = "occccad://part/display/visibility/set"
	typeOccurrenceVisibility   = "occccad://product/display/visibility/set"
	typeEditParameter          = "occccad://parameter/edit"
	typeCreateUserParameter    = "occccad://parameter/user/create"
	typeDeleteParameter        = "occccad://parameter/delete"
	typeInsertInstance         = "occccad://product/instance/insert"
	typeInsertInstances        = "occccad://product/instance/insert-many"
	typeMoveInstance           = "occccad://product/instance/move"
	typeAddAssemblyConstraint  = "occccad://product/assembly-constraint/add"
	typeEditAssemblyConstraint = "occccad://product/assembly-constraint/edit"
	typeSetReferenceMode       = "occccad://product/instance/reference-mode/set"
	typeUpdateReferences       = "occccad://product/references/update"
	typeDeletePartNode         = "occccad://part/node/delete"
	typeDeleteProductNode      = "occccad://product/node/delete"
	typeDeletePartNodes        = "occccad://part/nodes/delete"
	typeDeleteProductNodes     = "occccad://product/nodes/delete"
)

type commandHandler struct {
	typeURI      string
	documentType string
	apply        func(json.RawMessage, json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error)
}

func (handler commandHandler) TypeURI() string                   { return handler.typeURI }
func (handler commandHandler) SupportedSchemaVersions() []uint32 { return []uint32{1} }
func (handler commandHandler) TargetDocumentTypes() []string     { return []string{handler.documentType} }
func (handler commandHandler) Apply(model, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	return handler.apply(model, payload)
}

var workspaceCommandRegistry = mustWorkspaceRegistry()

func mustWorkspaceRegistry() *modelcore.Registry {
	registry, err := modelcore.NewRegistry(
		commandHandler{typeBodyCommand, "PART", applyBodyCommand},
		commandHandler{typeDefinitionVisibility, "PART", applyDefinitionVisibility},
		commandHandler{typeOccurrenceVisibility, "PRODUCT", applyOccurrenceVisibility},
		commandHandler{typeCreatePattern, "PART", applyCreateFeature},
		commandHandler{typeCreateSketch, "PART", applyCreateFeature},
		commandHandler{typeEditSketch, "PART", applyEditSketch},
		commandHandler{typeCreatePad, "PART", applyCreateFeature},
		commandHandler{typeCreateSolidFeature, "PART", applyCreateFeature},
		commandHandler{typeCreateBooleanFeature, "PART", applyCreateFeature},
		commandHandler{typeCreateModifierFeature, "PART", applyCreateFeature},
		commandHandler{typeEditFeature, "PART", applyEditFeature},
		commandHandler{typeSetFeatureSuppression, "PART", applyFeatureSuppression},
		commandHandler{typeRenameFeature, "PART", applyRenameFeature},
		commandHandler{typeCreateDatumPlane, "PART", applyCreateDatumPlane},
		commandHandler{typeCreateDatumAxis, "PART", applyCreateDatumAxis},
		commandHandler{typeImportExchange, "PART", applyCreateFeature},
		commandHandler{typeRepairImportNaming, "PART", applyRepairImportNaming},
		commandHandler{typeSetParameterLiteral, "PART", applyParameterSource},
		commandHandler{typeSetParameterExpression, "PART", applyParameterSource},
		commandHandler{typeRenameParameter, "PART", applyRenameParameter},
		commandHandler{typeEditParameter, "PART", applyEditParameter},
		commandHandler{typeCreateUserParameter, "PART", applyCreateUserParameter},
		commandHandler{typeDeleteParameter, "PART", applyDeleteParameter},
		commandHandler{typeSetParameterExternal, "PART", applyParameterSource},
		commandHandler{typeCreatePublication, "PART", applyCreatePublication},
		commandHandler{typeEditPublication, "PART", applyEditPublication},
		commandHandler{typeRedirectPublication, "PART", applyRedirectPublication},
		commandHandler{typeDeletePublication, "PART", applyDeletePublication},
		commandHandler{typeUpdatePartReferences, "PART", applyUpdatePartReferences},
		commandHandler{typeCreateContextReference, "PART", applyContextReferenceModel},
		commandHandler{typeDetachContextReference, "PART", applyContextReferenceModel},
		commandHandler{typeCreateContextInput, "PART", applyCreateContextInput},
		commandHandler{typeEditContextInput, "PART", applyEditContextInput},
		commandHandler{typeDeleteContextInput, "PART", applyDeleteContextInput},
		commandHandler{typeInsertInstance, "PRODUCT", applyInsertInstance},
		commandHandler{typeInsertInstances, "PRODUCT", applyInsertInstances},
		commandHandler{typeRenameInstance, "PRODUCT", applyRenameInstance},
		commandHandler{typeReplaceInstance, "PRODUCT", applyReplaceInstance},
		commandHandler{typeCreateProductPublication, "PRODUCT", applyCreateProductPublication},
		commandHandler{typeRedirectProductPublication, "PRODUCT", applyRedirectProductPublication},
		commandHandler{typeEditProductPublication, "PRODUCT", applyEditProductPublication},
		commandHandler{typeDeleteProductPublication, "PRODUCT", applyDeleteProductPublication},
		commandHandler{typeCreateContextBinding, "PRODUCT", applyCreateContextBinding},
		commandHandler{typeDeleteContextBinding, "PRODUCT", applyDeleteContextBinding},
		commandHandler{typeMoveInstance, "PRODUCT", applyMoveInstance},
		commandHandler{typeAddAssemblyConstraint, "PRODUCT", applyAddAssemblyConstraint},
		commandHandler{typeEditAssemblyConstraint, "PRODUCT", applyEditAssemblyConstraint},
		commandHandler{typeSetAssemblyConstraintState, "PRODUCT", applyAssemblyConstraintState},
		commandHandler{typeSetReferenceMode, "PRODUCT", applyReferenceMode},
		commandHandler{typeUpdateReferences, "PRODUCT", applyUpdateReferences},
		commandHandler{typeDeletePartNode, "PART", applyDeletePartNode},
		commandHandler{typeDeleteProductNode, "PRODUCT", applyDeleteProductNode},
		commandHandler{typeDeletePartNodes, "PART", applyDeletePartNodes},
		commandHandler{typeDeleteProductNodes, "PRODUCT", applyDeleteProductNodes},
	)
	if err != nil {
		panic(err)
	}
	return registry
}

type deleteNodePayload struct {
	TargetKind    string `json:"targetKind"`
	TargetID      string `json:"targetId"`
	OwnerEntityID string `json:"ownerEntityId,omitempty"`
}

type deleteNodesPayload struct {
	Targets []deleteNodePayload `json:"targets"`
}

func mergeDeleteChangeSets(changeSets []modelcore.ChangeSet) (modelcore.ChangeSet, error) {
	merged := modelcore.ChangeSet{}
	changeIndexes := map[string]int{}
	seeds := map[modelcore.DependencyKey]bool{}
	for _, changeSet := range changeSets {
		for _, change := range changeSet.Changes {
			key := change.Target.EntityID + "\x00" + change.Target.SlotID
			if index, exists := changeIndexes[key]; exists {
				previous := merged.Changes[index]
				var before, after any
				if len(previous.Before) > 0 {
					before = json.RawMessage(previous.Before)
				}
				if len(change.After) > 0 {
					after = json.RawMessage(change.After)
				}
				combined, err := modelcore.NewChange(previous.Kind, previous.Target, before, after)
				if err != nil {
					return modelcore.ChangeSet{}, err
				}
				merged.Changes[index] = combined
				continue
			}
			changeIndexes[key] = len(merged.Changes)
			merged.Changes = append(merged.Changes, change)
		}
		for _, seed := range changeSet.ImpactSeeds {
			if !seeds[seed] {
				seeds[seed] = true
				merged.ImpactSeeds = append(merged.ImpactSeeds, seed)
			}
		}
	}
	return merged, nil
}

func applyDeleteNodes(modelJSON, payloadJSON json.RawMessage,
	applyOne func(json.RawMessage, json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error)) (json.RawMessage, modelcore.ChangeSet, error) {
	var payload deleteNodesPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(payload.Targets) == 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: delete targets are required", ErrValidation)
	}
	next := modelJSON
	changeSets := make([]modelcore.ChangeSet, 0, len(payload.Targets))
	for _, target := range payload.Targets {
		targetJSON, err := json.Marshal(target)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		var changes modelcore.ChangeSet
		next, changes, err = applyOne(next, targetJSON)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		changeSets = append(changeSets, changes)
	}
	merged, err := mergeDeleteChangeSets(changeSets)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	return next, merged, nil
}

func applyDeletePartNodes(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	return applyDeleteNodes(modelJSON, payloadJSON, applyDeletePartNode)
}

func applyDeleteProductNodes(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	return applyDeleteNodes(modelJSON, payloadJSON, applyDeleteProductNode)
}

func applyDeletePartNode(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload deleteNodePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	switch payload.TargetKind {
	case "DATUM_PLANE", "DATUM_AXIS":
		return deleteDatum(model, payload.TargetKind, payload.TargetID)
	case "FEATURE":
		index := -1
		for i := range model.Features {
			if model.Features[i].ID == payload.TargetID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected feature does not exist", ErrValidation)
		}
		publishedBodyID := ""
		for _, body := range model.Bodies {
			if body.CreatedByFeatureID == payload.TargetID {
				publishedBodyID = body.ID
				break
			}
		}
		if err := rejectPublicationTargetRemoval(model, publishedBodyID, map[string]bool{payload.TargetID: true}); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		for _, dependent := range model.Features {
			if slices.Contains(featureInputIDs(dependent), payload.TargetID) {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: cannot delete feature %s while feature %s depends on it", ErrValidation, payload.TargetID, dependent.ID)
			}
			if dependent.Sketch != nil && dependent.Sketch.Support.Type == "PLANAR_FACE" &&
				dependent.Sketch.Support.PersistentSelection != nil &&
				dependent.Sketch.Support.PersistentSelection.Anchor.FeatureID == payload.TargetID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: FAILED_SUPPORT: sketch %s depends on feature %s", ErrValidation, dependent.ID, payload.TargetID)
			}
		}
		before := featureHistoryDefinition(model.Features[index])
		model.Features = append(model.Features[:index], model.Features[index+1:]...)
		bodyChanges, err := removeFeatureBody(&model, payload.TargetID)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		parameters := model.Parameters[:0]
		for _, parameter := range model.Parameters {
			if !strings.HasPrefix(parameter.ParameterID, "parameter:"+payload.TargetID+":") {
				parameters = append(parameters, parameter)
			}
		}
		model.Parameters = parameters
		ensureFeatureParameters(&model)
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.TargetID, SlotID: "entity"}, before, nil)
		changes, seeds := appendParameterLifecycleChanges(append(bodyChanges, change),
			[]modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(payload.TargetID)}, beforeParameters, model.Parameters)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
	case "SKETCH_PATTERN_DEFINITION":
		return applyEditSketch(modelJSON, mustPatternDeletePayload(payload.OwnerEntityID, payload.TargetID))
	case "SKETCH_ENTITY", "SKETCH_CONSTRAINT":
		for i := range model.Features {
			feature := &model.Features[i]
			if feature.ID != payload.OwnerEntityID || feature.Sketch == nil {
				continue
			}
			var before SketchFeature
			beforeJSON, _ := json.Marshal(feature.Sketch)
			_ = json.Unmarshal(beforeJSON, &before)
			if payload.TargetKind == "SKETCH_ENTITY" {
				found := false
				entities := feature.Sketch.Entities[:0]
				for _, entity := range feature.Sketch.Entities {
					if entity.ID == payload.TargetID {
						found = true
						continue
					}
					entities = append(entities, entity)
				}
				if !found {
					return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected sketch entity does not exist", ErrValidation)
				}
				feature.Sketch.Entities = entities
				constraints := feature.Sketch.Constraints[:0]
				for _, constraint := range feature.Sketch.Constraints {
					referencesDeleted := false
					for _, reference := range constraint.References {
						if reference.Target == "ENTITY" && reference.EntityID == payload.TargetID {
							referencesDeleted = true
							break
						}
					}
					if !referencesDeleted {
						constraints = append(constraints, constraint)
					}
				}
				feature.Sketch.Constraints = constraints
			} else {
				found := false
				constraints := feature.Sketch.Constraints[:0]
				for _, constraint := range feature.Sketch.Constraints {
					if constraint.ID == payload.TargetID {
						found = true
						continue
					}
					constraints = append(constraints, constraint)
				}
				if !found {
					return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected sketch constraint does not exist", ErrValidation)
				}
				feature.Sketch.Constraints = constraints
			}
			if len(feature.Sketch.Entities) == 0 {
				feature.Sketch.Solve = SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY"}
			}
			ensureFeatureParameters(&model)
			if err := validateAndResolvePartParameters(&model); err != nil {
				return nil, modelcore.ChangeSet{}, err
			}
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}, before, *feature.Sketch)
			changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change},
				[]modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(feature.ID)}, beforeParameters, model.Parameters)
			next, _ := json.Marshal(model)
			return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
		}
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: owning sketch does not exist", ErrValidation)
	default:
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: node kind %s is protected from deletion", ErrValidation, payload.TargetKind)
	}
}

func applyDeleteProductNode(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload deleteNodePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if payload.TargetKind != "INSTANCE" && payload.TargetKind != "ASSEMBLY_CONSTRAINT" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: node kind %s is protected from deletion", ErrValidation, payload.TargetKind)
	}
	if payload.TargetKind == "ASSEMBLY_CONSTRAINT" {
		for index := range model.Constraints {
			if model.Constraints[index].ID != payload.TargetID {
				continue
			}
			before := model.Constraints[index]
			model.Constraints = append(model.Constraints[:index], model.Constraints[index+1:]...)
			if isAssemblyGroup(before) {
				return applyAssemblyGroupDelete(modelJSON, payload.TargetID)
			}
			change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.TargetID, SlotID: "assembly-constraint.entity"}, before, nil)
			next, _ := json.Marshal(model)
			return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(payload.TargetID)}}, nil
		}
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected assembly constraint does not exist", ErrValidation)
	}
	index := -1
	for i := range model.Instances {
		if model.Instances[i].ID == payload.TargetID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
	}
	before := model.Instances[index]
	groupChanges := pruneAssemblyGroupsForDeletedInstance(&model, payload.TargetID)
	model.Instances = append(model.Instances[:index], model.Instances[index+1:]...)
	change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.TargetID, SlotID: "entity"}, before, nil)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: append([]modelcore.ModelChange{change}, groupChanges...), ImpactSeeds: []modelcore.DependencyKey{"instance:" + modelcore.DependencyKey(payload.TargetID)}}, nil
}

type editSketchPayload struct {
	SketchID   string            `json:"sketchId"`
	Operations []SketchOperation `json:"operations"`
}

func applyEditSketch(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload editSketchPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	for index := range model.Features {
		feature := &model.Features[index]
		if feature.ID != payload.SketchID {
			continue
		}
		if feature.Type != "SKETCH" || feature.Sketch == nil {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected feature is not a sketch", ErrValidation)
		}
		var before SketchFeature
		beforeJSON, _ := json.Marshal(feature.Sketch)
		_ = json.Unmarshal(beforeJSON, &before)
		beforeSources := map[string]modelcore.ValueSource{}
		beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
		for _, parameter := range model.Parameters {
			beforeSources[parameter.ParameterID] = parameter.Source
		}
		// An internal-constraint copy may not flatten formula-driven dimensions.
		for _, op := range payload.Operations {
			if (op.Copy || op.Type == "COPY_ENTITIES" || op.Type == "MIRROR_ENTITIES") && op.ConstraintPolicy == "INTERNAL" {
				selected := map[string]bool{}
				for _, id := range op.EntityIDs {
					selected[id] = true
				}
				for _, c := range feature.Sketch.Constraints {
					all := len(c.References) > 0
					for _, r := range c.References {
						all = all && selected[r.EntityID] && r.Target == "ENTITY"
					}
					if !all {
						continue
					}
					for _, parameter := range model.Parameters {
						if parameter.ParameterID == c.ParameterID && parameter.Source.Expression != nil {
							return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: copying formula-driven constraint %s requires an explicit parameter dependency policy", ErrValidation, c.ID)
						}
					}
				}
			}
		}
		var cornerErr error
		payload.Operations, cornerErr = resolveSketchCornerOperationValues(model, payload.Operations)
		if cornerErr != nil {
			return nil, modelcore.ChangeSet{}, cornerErr
		}
		if err := applySketchOperations(feature.Sketch, payload.Operations); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		ensureFeatureParameters(&model)
		if err := applySketchPatternParameterSources(&model, feature.ID, before, payload.Operations); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if err := applySketchDimensionLifecycle(&model, payload.SketchID, before, payload.Operations); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}, before, *feature.Sketch)
		changes := []modelcore.ModelChange{change}
		seeds := []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(feature.ID)}
		changes, seeds = appendParameterLifecycleChanges(changes, seeds, beforeParameters, model.Parameters)
		changes, seeds, metadataChanged := appendSketchParameterDefinitionChanges(changes, seeds, beforeParameters, model.Parameters)
		for _, parameter := range model.Parameters {
			if metadataChanged[parameter.ParameterID] {
				continue
			}
			prior, existed := beforeSources[parameter.ParameterID]
			if !existed || reflect.DeepEqual(prior, parameter.Source) {
				continue
			}
			slot := sketchLengthDimensionSlot
			if parameter.Dimension.Equal(modelcore.AngleDimension) {
				slot = sketchAngleDimensionSlot
			}
			parameterChange, _ := modelcore.NewChange(modelcore.ChangeUpdate,
				modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: slot.SlotID}, prior, parameter.Source)
			changes = append(changes, parameterChange)
			seeds = append(seeds, "parameter:"+modelcore.DependencyKey(parameter.ParameterID))
		}
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
}

func appendParameterLifecycleChanges(changes []modelcore.ModelChange, seeds []modelcore.DependencyKey,
	before, after []modelcore.ParameterDefinition) ([]modelcore.ModelChange, []modelcore.DependencyKey) {
	beforeByID := map[string]modelcore.ParameterDefinition{}
	afterByID := map[string]modelcore.ParameterDefinition{}
	for _, parameter := range before {
		beforeByID[parameter.ParameterID] = parameter
	}
	for _, parameter := range after {
		afterByID[parameter.ParameterID] = parameter
	}
	ids := map[string]struct{}{}
	for id := range beforeByID {
		ids[id] = struct{}{}
	}
	for id := range afterByID {
		ids[id] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		prior, hadPrior := beforeByID[id]
		current, hasCurrent := afterByID[id]
		if hadPrior == hasCurrent {
			continue
		}
		kind := modelcore.ChangeCreate
		var beforeValue, afterValue any
		if hadPrior {
			kind, beforeValue = modelcore.ChangeDelete, prior
		} else {
			afterValue = current
		}
		change, _ := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: id, SlotID: "parameter.entity"}, beforeValue, afterValue)
		changes = append(changes, change)
		seeds = append(seeds, "parameter:"+modelcore.DependencyKey(id))
	}
	return changes, seeds
}

// Every compound edit is staged on an isolated model. Callers, previews and
// direct domain tests observe either the complete edit or the original sketch.
func applySketchOperations(sketch *SketchFeature, operations []SketchOperation) error {
	encoded, err := json.Marshal(sketch)
	if err != nil {
		return err
	}
	var candidate SketchFeature
	if err = json.Unmarshal(encoded, &candidate); err != nil {
		return err
	}
	if err = normalizeSketchPointIdentities(&candidate); err != nil {
		return err
	}
	if err = applySketchOperationsCandidate(&candidate, operations); err != nil {
		return err
	}
	if err = normalizeSketchPointIdentities(&candidate); err != nil {
		return err
	}
	if err = ensureSketchDistanceRelations(&candidate); err != nil {
		return err
	}
	if err := validateSketchPatterns(candidate); err != nil {
		return err
	}
	*sketch = candidate
	return nil
}

func applySketchOperationsCandidate(sketch *SketchFeature, operations []SketchOperation) error {
	if len(operations) == 0 {
		return fmt.Errorf("%w: sketch edit requires at least one operation", ErrValidation)
	}
	for _, operation := range operations {
		switch operation.Type {
		case "CREATE_PATTERN", "EDIT_PATTERN", "DELETE_PATTERN", "DETACH_PATTERN":
			if err := applySketchPatternOperation(sketch, operation); err != nil {
				return err
			}
		case "CREATE_POLYGON":
			if err := applySketchPolygon(sketch, operation); err != nil {
				return err
			}
		case "OFFSET_ENTITIES":
			if err := applySketchOffsetEdit(sketch, operation); err != nil {
				return err
			}
		case "FILLET_ENTITIES", "CHAMFER_ENTITIES":
			if err := applySketchCornerEdit(sketch, operation); err != nil {
				return err
			}
		case "APPLY_DRAG_RESULT", "DELETE_ENTITIES", "COPY_ENTITIES", "TRANSFORM_ENTITIES", "MIRROR_ENTITIES", "SPLIT_ENTITY", "REPLACE_CURVE_INTERVALS":
			if err := applySketchGeometryEdit(sketch, operation); err != nil {
				return err
			}
		case "ADD_EXTERNAL_GEOMETRY":
			if operation.ExternalGeometry == nil {
				return fmt.Errorf("%w: ADD_EXTERNAL_GEOMETRY requires a bound external geometry", ErrValidation)
			}
			sketch.ExternalGeometry = append(sketch.ExternalGeometry, *operation.ExternalGeometry)
		case "RECONNECT_EXTERNAL_GEOMETRY":
			if operation.ExternalGeometry == nil || operation.ExternalID == "" {
				return fmt.Errorf("%w: RECONNECT_EXTERNAL_GEOMETRY requires a bound source and ExternalId", ErrValidation)
			}
			found := false
			for index := range sketch.ExternalGeometry {
				if sketch.ExternalGeometry[index].ID == operation.ExternalID {
					replacement := *operation.ExternalGeometry
					replacement.ID = operation.ExternalID
					// Preserve only the prior kind long enough to validate existing
					// references. Resolution runs before solve and either replaces this
					// snapshot or clears it on failure, so it is never consumed as fresh.
					replacement.Snapshot = sketch.ExternalGeometry[index].Snapshot
					replacement.GeometryKind = sketch.ExternalGeometry[index].GeometryKind
					sketch.ExternalGeometry[index] = replacement
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: selected external geometry does not exist", ErrValidation)
			}
		case "DETACH_EXTERNAL_GEOMETRY":
			found := -1
			for index := range sketch.ExternalGeometry {
				if sketch.ExternalGeometry[index].ID == operation.ExternalID {
					found = index
					break
				}
			}
			if found < 0 || sketch.ExternalGeometry[found].Status != "CONNECTED" || sketch.ExternalGeometry[found].Snapshot == nil {
				return fmt.Errorf("%w: only connected external geometry can be detached", ErrValidation)
			}
			external := sketch.ExternalGeometry[found]
			snapshot := external.Snapshot
			entity := SketchEntity{ID: external.ID, Kind: snapshot.Kind, Role: "CONSTRUCTION", Point: snapshot.Point,
				Start: snapshot.Start, End: snapshot.End, Center: snapshot.Center, Radius: snapshot.Radius}
			sketch.Entities = append(sketch.Entities, entity)
			sketch.ExternalGeometry = append(sketch.ExternalGeometry[:found], sketch.ExternalGeometry[found+1:]...)
			for constraintIndex := range sketch.Constraints {
				for referenceIndex := range sketch.Constraints[constraintIndex].References {
					reference := &sketch.Constraints[constraintIndex].References[referenceIndex]
					if reference.Target == "EXTERNAL" && reference.EntityID == external.ID {
						reference.Target = "ENTITY"
					}
				}
			}
		case "ADD_ENTITY":
			if operation.Entity == nil {
				return fmt.Errorf("%w: ADD_ENTITY requires an entity", ErrValidation)
			}
			sketch.Entities = append(sketch.Entities, *operation.Entity)
		case "ADD_CONSTRAINT":
			if operation.Constraint == nil {
				return fmt.Errorf("%w: ADD_CONSTRAINT requires a constraint", ErrValidation)
			}
			sketch.Constraints = append(sketch.Constraints, *operation.Constraint)
		case "UPDATE_ENTITY_ROLE":
			if operation.EntityID == "" || (operation.Role != "PROFILE" && operation.Role != "CONSTRUCTION") {
				return fmt.Errorf("%w: UPDATE_ENTITY_ROLE requires an entity and valid role", ErrValidation)
			}
			found := false
			for index := range sketch.Entities {
				if sketch.Entities[index].ID == operation.EntityID {
					sketch.Entities[index].Role = operation.Role
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: selected sketch entity does not exist", ErrValidation)
			}
		case "EDIT_SPLINE_POINT", "SET_SPLINE_CLOSED", "CONVERT_SPLINE_TO_CONTROL":
			if err := applySketchSplineEdit(sketch, operation); err != nil {
				return err
			}
		case "UPDATE_ENTITY_POINT":
			if operation.EntityID == "" || operation.Point == nil || !finite(operation.Point.X) || !finite(operation.Point.Y) {
				return fmt.Errorf("%w: UPDATE_ENTITY_POINT requires an entity and finite point", ErrValidation)
			}
			found := false
			for index := range sketch.Entities {
				entity := &sketch.Entities[index]
				if entity.ID != operation.EntityID {
					continue
				}
				switch operation.SubElement {
				case "START", "END":
					if entity.Kind == "LINE" {
						if operation.SubElement == "START" {
							entity.Start = operation.Point
						} else {
							entity.End = operation.Point
						}
						found = true
					}
					if entity.Kind == "ARC" && entity.Center != nil {
						angle := math.Atan2(operation.Point.Y-entity.Center.Y, operation.Point.X-entity.Center.X)
						baseline := entity.StartAngle
						if operation.SubElement == "END" {
							baseline = entity.EndAngle
						}
						angle += math.Round((baseline-angle)/(2*math.Pi)) * 2 * math.Pi
						if operation.SubElement == "START" {
							entity.StartAngle = angle
						} else {
							entity.EndAngle = angle
						}
						found = true
					}
					if entity.Kind == "ELLIPTICAL_ARC" && entity.Center != nil {
						dx, dy := operation.Point.X-entity.Center.X, operation.Point.Y-entity.Center.Y
						c, s := math.Cos(entity.Rotation), math.Sin(entity.Rotation)
						angle := math.Atan2((-s*dx+c*dy)/entity.MinorRadius, (c*dx+s*dy)/entity.MajorRadius)
						baseline := entity.StartAngle
						if operation.SubElement == "END" {
							baseline = entity.EndAngle
						}
						angle += math.Round((baseline-angle)/(2*math.Pi)) * 2 * math.Pi
						if operation.SubElement == "START" {
							entity.StartAngle = angle
						} else {
							entity.EndAngle = angle
						}
						found = true
					}
				case "POINT":
					if entity.Kind == "POINT" {
						entity.Point = operation.Point
						found = true
					}
				case "CENTER":
					if entity.Kind == "CIRCLE" || entity.Kind == "ARC" || entity.Kind == "ELLIPSE" || entity.Kind == "ELLIPTICAL_ARC" {
						entity.Center = operation.Point
						found = true
					}
				case "CONTROL":
					if entity.Kind == "SPLINE" {
						points := &entity.ControlPoints
						ids := entity.ControlPointIDs
						if entity.Mode == "CONTROL" {
							points = &entity.Poles
							ids = entity.PoleIDs
						}
						index := -1
						if operation.ControlPointIndex != nil {
							index = *operation.ControlPointIndex
						}
						if operation.ControlPointID != "" {
							index = -1
							for i, id := range ids {
								if id == operation.ControlPointID {
									index = i
									break
								}
							}
						}
						if index >= 0 && index < len(*points) {
							(*points)[index] = *operation.Point
							found = true
							if entity.Mode != "CONTROL" {
								entity.Poles = nil
								entity.PoleIDs = nil
								entity.Knots = nil
								entity.Weights = nil
								entity.Multiplicities = nil
							}
						}
					}

				}
				break
			}
			if !found {
				return fmt.Errorf("%w: selected entity point does not exist", ErrValidation)
			}
		case "UPDATE_ENTITY_SUPPRESSION":
			if operation.EntityID == "" || operation.Suppressed == nil {
				return fmt.Errorf("%w: entity suppression requires a target", ErrValidation)
			}
			found := false
			for index := range sketch.Entities {
				if sketch.Entities[index].ID == operation.EntityID {
					sketch.Entities[index].Suppressed = *operation.Suppressed
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: selected sketch entity does not exist", ErrValidation)
			}
			if *operation.Suppressed {
				for index := range sketch.Constraints {
					for _, reference := range sketch.Constraints[index].References {
						if reference.EntityID == operation.EntityID {
							sketch.Constraints[index].Suppressed = true
							break
						}
					}
				}
			}
		case "UPDATE_CONSTRAINT_SUPPRESSION":
			if operation.ConstraintID == "" || operation.Suppressed == nil {
				return fmt.Errorf("%w: constraint suppression requires a target", ErrValidation)
			}
			found := false
			for index := range sketch.Constraints {
				if sketch.Constraints[index].ID == operation.ConstraintID {
					sketch.Constraints[index].Suppressed = *operation.Suppressed
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%w: selected sketch constraint does not exist", ErrValidation)
			}
		case "DELETE_CONSTRAINT":
			if err := deleteSketchConstraint(sketch, operation); err != nil {
				return err
			}
		case "UPDATE_CONSTRAINT":
			if err := replaceSketchConstraint(sketch, operation); err != nil {
				return err
			}
		case "UPDATE_CONSTRAINT_VALUE":
			if operation.ConstraintID == "" || operation.Value == nil || !finite(*operation.Value) {
				return fmt.Errorf("%w: UPDATE_CONSTRAINT_VALUE requires a finite value", ErrValidation)
			}
			found := false
			for index := range sketch.Constraints {
				if sketch.Constraints[index].ID != operation.ConstraintID {
					continue
				}
				if !isDimensionalConstraint(sketch.Constraints[index].Kind) {
					return fmt.Errorf("%w: only dimensional constraints have editable values", ErrValidation)
				}
				if !validSketchDimensionValue(sketch.Constraints[index].Kind, *operation.Value) {
					return fmt.Errorf("%w: invalid value for this dimension", ErrValidation)
				}
				sketch.Constraints[index].Value = operation.Value
				found = true
				break
			}
			if !found {
				return fmt.Errorf("%w: selected constraint does not exist", ErrValidation)
			}
		case "ADD_RECTANGLE":
			if operation.First == nil || operation.Second == nil {
				return fmt.Errorf("%w: ADD_RECTANGLE requires two points", ErrValidation)
			}
			return fmt.Errorf("%w: rectangle macro must be expanded before command dispatch", ErrValidation)
		default:
			return fmt.Errorf("%w: unsupported sketch operation %s", ErrValidation, operation.Type)
		}
	}
	return nil
}

func isSolidGenerator(featureType string) bool {
	switch strings.ToUpper(featureType) {
	case "PAD", "LINEAR_EXTRUDE", "REVOLVE":
		return true
	default:
		return false
	}
}

func isDimensionalConstraint(kind string) bool {
	return kind == "DISTANCE" || kind == "LENGTH" || kind == "RADIUS" || kind == "DIAMETER" || kind == "ANGLE" || kind == "MAJOR_RADIUS" || kind == "MINOR_RADIUS" || kind == "HORIZONTAL_DISTANCE" || kind == "VERTICAL_DISTANCE"
}

func validateSketch(sketch SketchFeature) error {
	if err := validateSketchPatterns(sketch); err != nil {
		return err
	}
	if sketch.SchemaVersion != SketchSchemaVersion {
		return fmt.Errorf("%w: unsupported sketch schema version", ErrValidation)
	}
	entityKinds := map[string]string{}
	entityControlCounts := map[string]int{}
	for _, entity := range sketch.Entities {
		if entity.ID == "" || entityKinds[entity.ID] != "" {
			return fmt.Errorf("%w: sketch entity ids must be unique", ErrValidation)
		}
		if entity.Role != "PROFILE" && entity.Role != "CONSTRUCTION" {
			return fmt.Errorf("%w: sketch entity %s has invalid role", ErrValidation, entity.ID)
		}
		entityKinds[entity.ID] = entity.Kind
		entityControlCounts[entity.ID] = len(entity.ControlPoints)
		if entity.Mode == "CONTROL" {
			entityControlCounts[entity.ID] = len(entity.Poles)
		}
		switch entity.Kind {
		case "POINT":
			if entity.Point == nil || !finite(entity.Point.X) || !finite(entity.Point.Y) {
				return fmt.Errorf("%w: invalid sketch point", ErrValidation)
			}
		case "LINE":
			if entity.Start == nil || entity.End == nil || !finite(entity.Start.X) || !finite(entity.Start.Y) || !finite(entity.End.X) || !finite(entity.End.Y) || (entity.Start.X == entity.End.X && entity.Start.Y == entity.End.Y) {
				return fmt.Errorf("%w: invalid sketch line", ErrValidation)
			}
		case "CIRCLE":
			if entity.Center == nil || !finite(entity.Center.X) || !finite(entity.Center.Y) || !positiveFinite(entity.Radius) {
				return fmt.Errorf("%w: invalid sketch circle", ErrValidation)
			}
		case "ARC":
			sweep := entity.EndAngle - entity.StartAngle
			if entity.Center == nil || !finite(entity.Center.X) || !finite(entity.Center.Y) || !positiveFinite(entity.Radius) ||
				!finite(entity.StartAngle) || !finite(entity.EndAngle) || math.Abs(sweep) < 1e-9 || math.Abs(sweep) >= 2*math.Pi-1e-9 {
				return fmt.Errorf("%w: invalid sketch arc", ErrValidation)
			}
		case "ELLIPSE", "ELLIPTICAL_ARC":
			if entity.Center == nil || !finite(entity.Center.X) || !finite(entity.Center.Y) || !positiveFinite(entity.MinorRadius) || !finite(entity.MajorRadius) || entity.MajorRadius <= entity.MinorRadius || !finite(entity.Rotation) {
				return fmt.Errorf("%w: ellipse requires major > minor > 0 and finite center/rotation", ErrValidation)
			}
			if entity.Kind == "ELLIPTICAL_ARC" && (!finite(entity.StartAngle) || !finite(entity.EndAngle) || entity.StartAngle == entity.EndAngle || math.Abs(entity.EndAngle-entity.StartAngle) >= 2*math.Pi) {
				return fmt.Errorf("%w: invalid elliptical arc parameter interval", ErrValidation)
			}
		case "SPLINE":
			if (entity.Mode == "" || entity.Mode == "FIT") && len(entity.ControlPoints) < 3 {
				return fmt.Errorf("%w: fitted spline requires at least three fit points", ErrValidation)
			}
			if entity.Mode != "" && entity.Mode != "FIT" && entity.Mode != "CONTROL" {
				return fmt.Errorf("%w: unknown spline mode", ErrValidation)
			}
			if len(entity.Poles) > 0 {
				if err := validateCanonicalSpline(entity); err != nil {
					return err
				}
			} else if entity.Mode == "CONTROL" {
				return fmt.Errorf("%w: control spline requires a canonical basis", ErrValidation)
			}
			if entity.Degree < 1 || entity.Degree > 16 {
				return fmt.Errorf("%w: invalid sketch spline degree or control points", ErrValidation)
			}
			for _, point := range entity.ControlPoints {
				if !finite(point.X) || !finite(point.Y) {
					return fmt.Errorf("%w: invalid sketch spline control point", ErrValidation)
				}
			}
		default:
			return fmt.Errorf("%w: unsupported sketch entity %s", ErrValidation, entity.Kind)
		}
	}
	for _, external := range sketch.ExternalGeometry {
		if external.ID == "" || entityKinds[external.ID] != "" {
			return fmt.Errorf("%w: sketch external ids must be unique across geometry", ErrValidation)
		}
		if external.ProjectionKind != "ORTHOGONAL" || external.SourceVersionID == "" {
			return fmt.Errorf("%w: external geometry %s has an incomplete source contract", ErrValidation, external.ID)
		}
		if err := external.PersistentSelection.Validate(); err != nil {
			return fmt.Errorf("%w: external geometry %s persistent selection: %v", ErrValidation, external.ID, err)
		}
		if external.Status != "PENDING" && external.Status != "CONNECTED" && external.Status != "UNRESOLVED_EXTERNAL" {
			return fmt.Errorf("%w: external geometry %s has invalid status", ErrValidation, external.ID)
		}
		if external.Status == "CONNECTED" && external.Snapshot == nil {
			return fmt.Errorf("%w: connected external geometry %s requires a snapshot", ErrValidation, external.ID)
		}
		if external.Status == "UNRESOLVED_EXTERNAL" && external.Snapshot != nil {
			return fmt.Errorf("%w: unresolved external geometry %s cannot retain a stale snapshot", ErrValidation, external.ID)
		}
		kind := external.GeometryKind
		if external.Snapshot != nil {
			kind = external.Snapshot.Kind
			snapshot := external.Snapshot
			switch kind {
			case "POINT":
				if snapshot.Point == nil || !finite(snapshot.Point.X) || !finite(snapshot.Point.Y) {
					return fmt.Errorf("%w: invalid projected external point", ErrValidation)
				}
			case "LINE":
				if snapshot.Start == nil || snapshot.End == nil || !finite(snapshot.Start.X) || !finite(snapshot.Start.Y) || !finite(snapshot.End.X) || !finite(snapshot.End.Y) || *snapshot.Start == *snapshot.End {
					return fmt.Errorf("%w: invalid projected external line", ErrValidation)
				}
			case "CIRCLE":
				if snapshot.Center == nil || !finite(snapshot.Center.X) || !finite(snapshot.Center.Y) || !positiveFinite(snapshot.Radius) {
					return fmt.Errorf("%w: invalid projected external circle", ErrValidation)
				}
			default:
				return fmt.Errorf("%w: unsupported projected external geometry %s", ErrValidation, kind)
			}
		}
		if kind == "" {
			switch external.PersistentSelection.ExpectedType {
			case "VERTEX":
				kind = "POINT"
			case "EDGE":
				kind = "LINE"
			}
		}
		if (external.PersistentSelection.ExpectedType == modelcore.PersistentTopologyVertex && kind != "POINT") ||
			(external.PersistentSelection.ExpectedType == modelcore.PersistentTopologyEdge && kind != "LINE" && kind != "CIRCLE") {
			return fmt.Errorf("%w: external geometry %s snapshot type does not match its topology source", ErrValidation, external.ID)
		}
		entityKinds[external.ID] = kind
	}
	constraints := map[string]bool{}
	for _, constraint := range sketch.Constraints {
		if constraint.ID == "" || constraints[constraint.ID] {
			return fmt.Errorf("%w: sketch constraint ids must be unique", ErrValidation)
		}
		constraints[constraint.ID] = true
		if constraint.Suppressed {
			continue
		}
		counts := map[string]int{"COINCIDENT": 2, "PARALLEL": 2, "FIXED": 1, "FIXED_POINT": 1,
			"HORIZONTAL": 1, "VERTICAL": 1, "PERPENDICULAR": 2, "TANGENT": 2, "EQUAL": 2,
			"DISTANCE": 2, "LENGTH": 1, "RADIUS": 1, "DIAMETER": 1, "ANGLE": 2,
			"CONCENTRIC": 2, "POINT_ON_OBJECT": 2, "MIDPOINT": 2, "SYMMETRY": 3, "MAJOR_RADIUS": 1, "MINOR_RADIUS": 1, "MIRROR": 3, "HORIZONTAL_DISTANCE": 2, "VERTICAL_DISTANCE": 2, "COLLINEAR": 2, "SAME_SUPPORT": 2}
		expected, supported := counts[constraint.Kind]
		if !supported || len(constraint.References) != expected {
			return fmt.Errorf("%w: constraint %s has unsupported kind or reference count", ErrValidation, constraint.ID)
		}
		if constraint.Kind == "TANGENT" {
			for _, ref := range constraint.References {
				if entityKinds[ref.EntityID] != "SPLINE" {
					continue
				}
				for _, entity := range sketch.Entities {
					if entity.ID != ref.EntityID {
						continue
					}
					if entity.Mode != "CONTROL" || entity.Closed || (ref.SubElement != "START" && ref.SubElement != "END") {
						return fmt.Errorf("%w: spline tangent requires an explicit open CONTROL endpoint; convert solved FIT spline to CONTROL first", ErrValidation)
					}
					if err := validateCanonicalSpline(entity); err != nil {
						return err
					}
					last := len(entity.Knots) - 1
					if entity.Multiplicities[0] != entity.Degree+1 || entity.Multiplicities[last] != entity.Degree+1 || entity.ParameterStart != entity.Knots[0] || entity.ParameterEnd != entity.Knots[last] {
						return fmt.Errorf("%w: endpoint tangent requires a clamped canonical spline on its complete domain", ErrValidation)
					}
				}
			}
		}
		for _, ref := range constraint.References {
			if entityKinds[ref.EntityID] != "SPLINE" || (ref.SubElement != "START" && ref.SubElement != "END") {
				continue
			}
			for _, entity := range sketch.Entities {
				if entity.ID == ref.EntityID && entity.Closed {
					return fmt.Errorf("%w: closed spline has no independent endpoint subelement", ErrValidation)
				}
			}
		}
		if constraint.Reference && !isDimensionalConstraint(constraint.Kind) {
			return fmt.Errorf("%w: only dimensions can be reference measurements", ErrValidation)
		}
		if constraint.Kind == "FIXED_POINT" && (constraint.FixedPoint == nil || !finite(constraint.FixedPoint.X) || !finite(constraint.FixedPoint.Y)) {
			return fmt.Errorf("%w: fixed-point constraint %s requires a finite point", ErrValidation, constraint.ID)
		}
		if constraint.LabelPosition != nil && (!isDimensionalConstraint(constraint.Kind) ||
			!finite(constraint.LabelPosition.X) || !finite(constraint.LabelPosition.Y)) {
			return fmt.Errorf("%w: constraint %s has an invalid dimension placement", ErrValidation, constraint.ID)
		}
		if isDimensionalConstraint(constraint.Kind) {
			if !constraint.Reference && (constraint.Value == nil || !validSketchDimensionValue(constraint.Kind, *constraint.Value)) {
				return fmt.Errorf("%w: dimensional constraint %s requires a valid finite value", ErrValidation, constraint.ID)
			}
			expectedUnit := "mm"
			if constraint.Kind == "ANGLE" {
				expectedUnit = "deg"
			}
			if constraint.Unit != expectedUnit {
				return fmt.Errorf("%w: dimensional constraint %s requires unit %s", ErrValidation, constraint.ID, expectedUnit)
			}
			if constraint.ParameterID == "" {
				return fmt.Errorf("%w: dimensional constraint %s requires a stable ParameterId", ErrValidation, constraint.ID)
			}
		}
		for _, reference := range constraint.References {
			switch reference.Target {
			case "ENTITY", "EXTERNAL":
				kind := entityKinds[reference.EntityID]
				if kind == "" {
					return fmt.Errorf("%w: constraint %s references unknown entity %s", ErrValidation, constraint.ID, reference.EntityID)
				}
				validSubElements := map[string]map[string]bool{
					"POINT": {"POINT": true, "WHOLE": true}, "LINE": {"START": true, "END": true, "DIRECTION": true, "WHOLE": true},
					"CIRCLE": {"CENTER": true, "WHOLE": true}, "ARC": {"START": true, "END": true, "CENTER": true, "WHOLE": true},
					"SPLINE":  {"START": true, "END": true, "CONTROL": true, "WHOLE": true},
					"ELLIPSE": {"CENTER": true, "WHOLE": true}, "ELLIPTICAL_ARC": {"CENTER": true, "START": true, "END": true, "WHOLE": true},
				}
				if !validSubElements[kind][reference.SubElement] {
					return fmt.Errorf("%w: constraint %s uses invalid %s sub-element %s", ErrValidation, constraint.ID, kind, reference.SubElement)
				}
				if reference.SubElement == "CONTROL" && (reference.ControlPointIndex == nil ||
					*reference.ControlPointIndex < 0 || *reference.ControlPointIndex >= entityControlCounts[reference.EntityID]) {
					return fmt.Errorf("%w: constraint %s uses invalid spline control point", ErrValidation, constraint.ID)
				}
			case "SKETCH_ORIGIN":
				if reference.EntityID != "" || reference.SubElement != "POINT" {
					return fmt.Errorf("%w: sketch origin must use the POINT sub-element", ErrValidation)
				}
			case "SKETCH_X_AXIS", "SKETCH_Y_AXIS":
				if reference.EntityID != "" || reference.SubElement != "DIRECTION" {
					return fmt.Errorf("%w: sketch axis must use the DIRECTION sub-element", ErrValidation)
				}
			default:
				return fmt.Errorf("%w: constraint %s has unknown reference target %s", ErrValidation, constraint.ID, reference.Target)
			}
		}
		if !constraintReferencesCompatible(constraint, entityKinds) {
			return fmt.Errorf("%w: constraint %s has incompatible reference types for %s", ErrValidation, constraint.ID, constraint.Kind)
		}
	}
	return nil
}

func constraintReferencesCompatible(constraint SketchConstraint, entityKinds map[string]string) bool {
	point := func(reference SketchGeometryRef) bool {
		if reference.Target == "SKETCH_ORIGIN" {
			return reference.SubElement == "POINT"
		}
		kind := entityKinds[reference.EntityID]
		switch reference.SubElement {
		case "POINT":
			return kind == "POINT"
		case "START", "END":
			return kind == "LINE" || kind == "ARC" || kind == "SPLINE" || kind == "ELLIPTICAL_ARC"
		case "CENTER":
			return kind == "CIRCLE" || kind == "ARC" || kind == "ELLIPSE" || kind == "ELLIPTICAL_ARC"
		case "CONTROL":
			return kind == "SPLINE" && reference.ControlPointIndex != nil
		}
		return false
	}
	line := func(reference SketchGeometryRef) bool {
		return (reference.Target == "SKETCH_X_AXIS" || reference.Target == "SKETCH_Y_AXIS") ||
			((reference.Target == "ENTITY" || reference.Target == "EXTERNAL") && entityKinds[reference.EntityID] == "LINE" &&
				(reference.SubElement == "DIRECTION" || reference.SubElement == "WHOLE"))
	}
	circular := func(reference SketchGeometryRef) bool {
		kind := entityKinds[reference.EntityID]
		return (reference.Target == "ENTITY" || reference.Target == "EXTERNAL") && (kind == "CIRCLE" || kind == "ARC") && reference.SubElement == "WHOLE"
	}
	elliptical := func(reference SketchGeometryRef) bool {
		kind := entityKinds[reference.EntityID]
		return reference.Target == "ENTITY" && (kind == "ELLIPSE" || kind == "ELLIPTICAL_ARC") && reference.SubElement == "WHOLE"
	}
	curve := func(reference SketchGeometryRef) bool {
		return line(reference) || circular(reference) || elliptical(reference)
	}
	refs := constraint.References
	switch constraint.Kind {
	case "SAME_SUPPORT":
		return (line(refs[0]) && line(refs[1])) || (circular(refs[0]) && circular(refs[1])) || (elliptical(refs[0]) && elliptical(refs[1]))
	case "MIRROR":
		if refs[0].EntityID == refs[2].EntityID {
			kind := entityKinds[refs[0].EntityID]
			mode := constraint.SelfMirrorMode
			valid := (kind == "POINT" || kind == "CIRCLE") && mode == "ON_AXIS" || (kind == "LINE" || kind == "SPLINE") && (mode == "ON_AXIS" || mode == "PAIRED") || kind == "ARC" && mode == "PAIRED" || (kind == "ELLIPSE" || kind == "ELLIPTICAL_ARC") && (mode == "MAJOR_PARALLEL" || mode == "MAJOR_PERPENDICULAR")
			if !valid {
				return false
			}
		} else if constraint.SelfMirrorMode != "" {
			return false
		}
		return refs[0].Target == "ENTITY" && refs[2].Target == "ENTITY" && refs[0].SubElement == "WHOLE" && refs[2].SubElement == "WHOLE" && entityKinds[refs[0].EntityID] == entityKinds[refs[2].EntityID] && line(refs[1])
	case "COINCIDENT":
		return point(refs[0]) && point(refs[1])
	case "DISTANCE":
		return (point(refs[0]) && (point(refs[1]) || line(refs[1]))) ||
			(line(refs[0]) && (point(refs[1]) || line(refs[1])))
	case "HORIZONTAL_DISTANCE", "VERTICAL_DISTANCE":
		return point(refs[0]) && point(refs[1])
	case "PARALLEL", "PERPENDICULAR", "ANGLE", "COLLINEAR":
		return line(refs[0]) && line(refs[1])
	case "FIXED":
		return (refs[0].Target == "ENTITY" || refs[0].Target == "EXTERNAL") && refs[0].SubElement == "WHOLE"
	case "FIXED_POINT":
		return point(refs[0])
	case "HORIZONTAL", "VERTICAL", "LENGTH":
		return line(refs[0])
	case "RADIUS", "DIAMETER":
		return circular(refs[0])
	case "CONCENTRIC":
		return (circular(refs[0]) || elliptical(refs[0])) && (circular(refs[1]) || elliptical(refs[1]))
	case "MAJOR_RADIUS", "MINOR_RADIUS":
		return elliptical(refs[0])
	case "TANGENT":
		endpointCurve := func(r SketchGeometryRef) bool {
			return (r.SubElement == "START" || r.SubElement == "END") && (entityKinds[r.EntityID] == "ARC" || entityKinds[r.EntityID] == "ELLIPTICAL_ARC" || entityKinds[r.EntityID] == "SPLINE")
		}
		circularTangent := func(r SketchGeometryRef) bool {
			return circular(r) || ((r.SubElement == "START" || r.SubElement == "END") && entityKinds[r.EntityID] == "ARC")
		}
		ellipticalTangent := func(r SketchGeometryRef) bool {
			return elliptical(r) || ((r.SubElement == "START" || r.SubElement == "END") && entityKinds[r.EntityID] == "ELLIPTICAL_ARC")
		}
		return (line(refs[0]) && (circularTangent(refs[1]) || ellipticalTangent(refs[1]) || endpointCurve(refs[1]))) || (line(refs[1]) && (circularTangent(refs[0]) || ellipticalTangent(refs[0]) || endpointCurve(refs[0]))) || (circularTangent(refs[0]) && circularTangent(refs[1]))
	case "EQUAL":
		return (line(refs[0]) && line(refs[1])) || (circular(refs[0]) && circular(refs[1])) || (elliptical(refs[0]) && elliptical(refs[1]))
	case "POINT_ON_OBJECT":
		return point(refs[0]) && curve(refs[1])
	case "MIDPOINT":
		return point(refs[0]) && line(refs[1])
	case "SYMMETRY":
		return point(refs[0]) && (line(refs[1]) || point(refs[1])) && point(refs[2])
	}
	return false
}

type createFeaturePayload struct {
	Feature          Feature                          `json:"feature"`
	ParameterSources map[string]modelcore.ValueSource `json:"parameterSources,omitempty"`
}

func applyCreateFeature(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload createFeaturePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	payload.Feature.Order = 1
	for _, f := range model.Features {
		if f.Order >= payload.Feature.Order {
			payload.Feature.Order = f.Order + 1
		}
	}

	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	for _, feature := range model.Features {
		if feature.ID == payload.Feature.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate feature identity", ErrValidation)
		}
	}
	var bodyChanges []modelcore.ModelChange
	if payload.Feature.BodyID == "" {
		payload.Feature.BodyID = model.ActiveBodyID
		if isSolidGenerator(payload.Feature.Type) && payload.Feature.Profile != "" {
			for _, feature := range model.Features {
				if feature.ID == payload.Feature.Profile && feature.Sketch != nil {
					payload.Feature.BodyID = feature.BodyID
					break
				}
			}
		}
	}
	if (isSolidGenerator(payload.Feature.Type) || payload.Feature.Type == "LOFT") && payload.Feature.Operation == "NEW_BODY" {
		bodyChanges = createFeatureBody(&model, &payload.Feature)
	}
	if bodyIndex(model, payload.Feature.BodyID) < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: target Body does not exist", ErrValidation)
	}
	model.Features = append(model.Features, payload.Feature)
	normalizePartModel(&model)
	for slot, source := range payload.ParameterSources {
		parameterID := "parameter:" + payload.Feature.ID + ":" + slot
		found := false
		for index := range model.Parameters {
			if model.Parameters[index].ParameterID != parameterID {
				continue
			}
			model.Parameters[index].Source = source
			found = true
			break
		}
		if !found {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: feature parameter slot %s does not exist", ErrValidation, slot)
		}
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	created := model.Features[len(model.Features)-1]
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Feature.ID, SlotID: "entity"}, nil, featureHistoryDefinition(created))
	changes, seeds := appendParameterLifecycleChanges(append(bodyChanges, change),
		[]modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(payload.Feature.ID)}, beforeParameters, model.Parameters)
	set := modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}
	next, _ := json.Marshal(model)
	return next, set, nil
}

type createDatumPlanePayload struct {
	Plane DatumPlane `json:"plane"`
}
type createDatumAxisPayload struct {
	Axis DatumAxis `json:"axis"`
}

func applyCreateDatumPlane(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload createDatumPlanePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	origin, u, normal, err := validatedSupportFrame(payload.Plane.Origin, payload.Plane.UDirection, payload.Plane.Normal)
	if payload.Plane.ID == "" || err != nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid datum plane frame", ErrValidation)
	}
	payload.Plane.Origin, payload.Plane.UDirection, payload.Plane.Normal = origin, u, normal
	for _, plane := range model.DatumPlanes {
		if plane.ID == payload.Plane.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate datum plane identity", ErrValidation)
		}
	}
	model.DatumPlanes = append(model.DatumPlanes, payload.Plane)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Plane.ID, SlotID: "datum.plane"}, nil, payload.Plane)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"datum:" + modelcore.DependencyKey(payload.Plane.ID)}}, nil
}

func applyCreateDatumAxis(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload createDatumAxisPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	direction, ok := normalize3(payload.Axis.Direction)
	if payload.Axis.ID == "" || !ok || !finite(payload.Axis.Origin[0]) || !finite(payload.Axis.Origin[1]) || !finite(payload.Axis.Origin[2]) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid datum axis frame", ErrValidation)
	}
	payload.Axis.Direction = direction
	for _, axis := range model.DatumAxes {
		if axis.ID == payload.Axis.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate datum axis identity", ErrValidation)
		}
	}
	model.DatumAxes = append(model.DatumAxes, payload.Axis)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Axis.ID, SlotID: "datum.axis"}, nil, payload.Axis)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"datum:" + modelcore.DependencyKey(payload.Axis.ID)}}, nil
}

type parameterSourcePayload struct {
	ParameterID string                `json:"parameterId"`
	Source      modelcore.ValueSource `json:"source"`
}

type renameParameterPayload struct {
	ParameterID string `json:"parameterId"`
	Key         string `json:"key"`
}

type linearExtrudeEdit struct {
	Source    modelcore.ValueSource `json:"source"`
	Operation string                `json:"operation"`
	Reversed  bool                  `json:"reversed"`
	Profile   string                `json:"profile"`
}

type editFeaturePayload struct {
	ParameterSources      map[string]modelcore.ValueSource `json:"parameterSources,omitempty"`
	Definition            *Feature                         `json:"definition,omitempty"`
	FeatureID             string                           `json:"featureId"`
	ExpectedFeatureDigest string                           `json:"expectedFeatureDigest"`
	LinearExtrude         linearExtrudeEdit                `json:"linearExtrude"`
}

var padLengthSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/feature/linear-extrude", SlotID: "pad.length",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION", "EXTERNAL"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
}

var sketchLengthDimensionSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/sketch/constraint/dimension", SlotID: "sketch.dimension.value",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION", "EXTERNAL"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
}

var sketchAngleDimensionSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/sketch/constraint/dimension", SlotID: "sketch.dimension.value",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.AngleDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION", "EXTERNAL"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
}

type featureEditFailure struct{ code string }

func (failure featureEditFailure) Error() string { return failure.code }
func (failure featureEditFailure) Unwrap() error { return ErrValidation }
func (failure featureEditFailure) Code() string  { return failure.code }
func (featureEditFailure) Phase() string         { return "FEATURE_EDIT" }
func (featureEditFailure) Retryable() bool       { return false }

func featureDefinitionDigest(model PartModel, featureID string) (string, error) {
	normalizePartModel(&model)
	for _, feature := range model.Features {
		if feature.ID != featureID {
			continue
		}
		sources := map[string]modelcore.ValueSource{}
		for _, parameter := range model.Parameters {
			if parameter.OwnerFeatureID == feature.ID {
				sources[parameter.PropertySlot] = parameter.Source
			}
		}
		value, err := json.Marshal(struct {
			Feature Feature                          `json:"feature"`
			Sources map[string]modelcore.ValueSource `json:"parameterSources"`
		}{geometryFeatureDefinition(feature), sources})
		if err != nil {
			return "", err
		}
		return modelcore.ValueDigest(value), nil
	}
	return "", fmt.Errorf("%w: selected feature does not exist", ErrValidation)
}

func applyEditFeature(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload editFeaturePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	digest, err := featureDefinitionDigest(model, payload.FeatureID)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if payload.ExpectedFeatureDigest == "" || payload.ExpectedFeatureDigest != digest {
		return nil, modelcore.ChangeSet{}, featureEditFailure{code: "FEATURE_EDIT_STALE"}
	}
	var feature *Feature
	for index := range model.Features {
		if model.Features[index].ID == payload.FeatureID {
			feature = &model.Features[index]
			break
		}
	}
	if payload.Definition != nil {
		return editSolidDefinition(model, feature, *payload.Definition, payload.ParameterSources)
	}
	if feature == nil || !isSolidGenerator(feature.Type) || strings.EqualFold(feature.Type, "REVOLVE") {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: only Linear Extrude can be edited", ErrValidation)
	}
	definition := payload.LinearExtrude
	if definition.Profile != feature.Profile || !strings.EqualFold(definition.Operation, feature.Operation) || definition.Reversed != feature.Reversed {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: P1 only permits Linear Extrude length edits", ErrValidation)
	}
	parameterID := "parameter:" + feature.ID + ":length"
	for index := range model.Parameters {
		parameter := &model.Parameters[index]
		if parameter.ParameterID != parameterID {
			continue
		}
		before := parameter.Source
		parameter.Source = definition.Source
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate,
			modelcore.PropertyAddress{EntityID: feature.ID, SlotID: padLengthSlot.SlotID}, before, parameter.Source)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{
			"parameter:" + modelcore.DependencyKey(parameterID), "feature:" + modelcore.DependencyKey(feature.ID),
		}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Linear Extrude length parameter does not exist", ErrValidation)
}

func applyParameterSource(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload parameterSourcePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	for index := range model.Parameters {
		if model.Parameters[index].ParameterID != payload.ParameterID {
			continue
		}
		if model.Parameters[index].Role == "MEASURED" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: reference dimension is measured; switch its driving mode explicitly before editing its preserved source", ErrValidation)
		}
		before := model.Parameters[index].Source
		if before.External != nil && payload.Source.External == nil {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: external parameter requires explicit Detach before source editing", ErrValidation)
		}
		model.Parameters[index].Source = payload.Source
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.ParameterID, SlotID: "parameter.source"}, before, payload.Source)
		set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(payload.ParameterID)}}
		next, _ := json.Marshal(model)
		return next, set, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter does not exist", ErrValidation)
}

type editParameterPayload struct {
	ParameterID string                `json:"parameterId"`
	Key         string                `json:"key"`
	Source      modelcore.ValueSource `json:"source"`
}

// Alias and source share one revision and one validation boundary. Expressions
// retain bound ParameterIds while their readable source text follows aliases.
func applyEditParameter(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload editParameterPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	payload.Key = strings.TrimSpace(payload.Key)
	if !validParameterKey(payload.Key) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter alias must be an ASCII identifier", ErrValidation)
	}
	index := -1
	before := map[string]modelcore.ParameterDefinition{}
	for i, parameter := range model.Parameters {
		before[parameter.ParameterID] = parameter
		if parameter.ParameterID == payload.ParameterID {
			index = i
		} else if parameter.Key == payload.Key {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter alias %s already exists", ErrValidation, payload.Key)
		}
	}
	if index < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter does not exist", ErrValidation)
	}
	if model.Parameters[index].Role == "MEASURED" && !reflect.DeepEqual(model.Parameters[index].Source, payload.Source) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: reference dimension preserves its driving source; use its dimension definition editor to switch mode", ErrValidation)
	}
	if model.Parameters[index].Source.External != nil && payload.Source.External == nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: external parameter requires explicit Detach before source editing", ErrValidation)
	}
	model.Parameters[index].Key = payload.Key
	model.Parameters[index].Source = payload.Source
	keys := map[string]string{}
	for _, parameter := range model.Parameters {
		keys[parameter.ParameterID] = parameter.Key
	}
	for i := range model.Parameters {
		if expression := model.Parameters[i].Source.Expression; expression != nil {
			formatted, err := modelcore.FormatExpression(*expression, keys)
			if err != nil {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: %w", ErrValidation, err)
			}
			expression.SourceText = formatted
		}
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	changes := []modelcore.ModelChange{}
	seeds := []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(payload.ParameterID)}
	for _, parameter := range model.Parameters {
		prior := before[parameter.ParameterID]
		if prior.Key != parameter.Key {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.key"}, prior.Key, parameter.Key)
			changes = append(changes, change)
		}
		if !reflect.DeepEqual(prior.Source, parameter.Source) {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.source"}, prior.Source, parameter.Source)
			changes = append(changes, change)
			seeds = append(seeds, "parameter:"+modelcore.DependencyKey(parameter.ParameterID))
		}
	}
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
}

func applyRenameParameter(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload renameParameterPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	payload.Key = strings.TrimSpace(payload.Key)
	if !validParameterKey(payload.Key) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter key must be an ASCII identifier", ErrValidation)
	}
	for _, parameter := range model.Parameters {
		if parameter.ParameterID != payload.ParameterID && parameter.Key == payload.Key {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter key %s already exists", ErrValidation, payload.Key)
		}
	}
	for index := range model.Parameters {
		parameter := &model.Parameters[index]
		if parameter.ParameterID != payload.ParameterID {
			continue
		}
		before := parameter.Key
		beforeSources := map[string]modelcore.ValueSource{}
		for _, current := range model.Parameters {
			beforeSources[current.ParameterID] = current.Source
		}
		parameter.Key = payload.Key
		keys := map[string]string{}
		for _, current := range model.Parameters {
			keys[current.ParameterID] = current.Key
		}
		for sourceIndex := range model.Parameters {
			expression := model.Parameters[sourceIndex].Source.Expression
			if expression == nil {
				continue
			}
			formatted, formatErr := modelcore.FormatExpression(*expression, keys)
			if formatErr != nil {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: %w", ErrValidation, formatErr)
			}
			expression.SourceText = formatted
		}
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate,
			modelcore.PropertyAddress{EntityID: payload.ParameterID, SlotID: "parameter.key"}, before, payload.Key)
		changes := []modelcore.ModelChange{change}
		seeds := []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(payload.ParameterID)}
		for _, current := range model.Parameters {
			prior := beforeSources[current.ParameterID]
			if reflect.DeepEqual(prior, current.Source) {
				continue
			}
			sourceChange, _ := modelcore.NewChange(modelcore.ChangeUpdate,
				modelcore.PropertyAddress{EntityID: current.ParameterID, SlotID: "parameter.source"}, prior, current.Source)
			changes = append(changes, sourceChange)
			seeds = append(seeds, "parameter:"+modelcore.DependencyKey(current.ParameterID))
		}
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: %s", modelcore.ErrParameterMissing, payload.ParameterID)
}

func validParameterKey(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' ||
			(index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

type insertInstancePayload struct {
	Instance ProductInstance `json:"instance"`
}

type insertInstancesPayload struct {
	Instances []ProductInstance `json:"instances"`
}

func applyInsertInstances(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload insertInstancesPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(payload.Instances) == 0 || len(payload.Instances) > 128 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: instance batch must contain 1..128 items", ErrValidation)
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for _, instance := range model.Instances {
		ids[instance.ID] = true
		names[strings.ToLower(strings.TrimSpace(instance.Name))] = true
	}
	changes := make([]modelcore.ModelChange, 0, len(payload.Instances))
	seeds := make([]modelcore.DependencyKey, 0, len(payload.Instances))
	for _, instance := range payload.Instances {
		name := strings.ToLower(strings.TrimSpace(instance.Name))
		if instance.ID == "" || instance.ReferencedDocumentID == "" || name == "" || ids[instance.ID] || names[name] {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate or incomplete instance identity", ErrValidation)
		}
		for _, coordinate := range instance.Translation {
			if math.IsNaN(coordinate) || math.IsInf(coordinate, 0) {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: non-finite instance translation", ErrValidation)
			}
		}
		ids[instance.ID], names[name] = true, true
		model.Instances = append(model.Instances, instance)
		change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: instance.ID, SlotID: "entity"}, nil, instance)
		changes = append(changes, change)
		seeds = append(seeds, "instance:"+modelcore.DependencyKey(instance.ID))
	}
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}

func applyInsertInstance(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload insertInstancePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for _, instance := range model.Instances {
		if instance.ID == payload.Instance.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate instance identity", ErrValidation)
		}
		if strings.EqualFold(strings.TrimSpace(instance.Name), strings.TrimSpace(payload.Instance.Name)) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate sibling instance name", ErrValidation)
		}
	}
	model.Instances = append(model.Instances, payload.Instance)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Instance.ID, SlotID: "entity"}, nil, payload.Instance)
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"instance:" + modelcore.DependencyKey(payload.Instance.ID)}}
	next, _ := json.Marshal(model)
	return next, set, nil
}

type moveInstancePayload struct {
	SessionID         string                       `json:"sessionId,omitempty"`
	InteractionTarget *geometry.AssemblyDragTarget `json:"interactionTarget,omitempty"`
	InstanceID        string                       `json:"instanceId"`
	Translation       [3]float64                   `json:"translation"`
	Rotation          [4]float64                   `json:"rotation"`
}

type addAssemblyConstraintPayload struct {
	Constraint         AssemblyConstraint `json:"constraint"`
	QuantityExpression *string            `json:"quantityExpression,omitempty"`
	QuantityKey        string             `json:"quantityKey,omitempty"`
	OffsetExpression   *string            `json:"offsetExpression,omitempty"`
	OffsetKey          string             `json:"offsetKey,omitempty"`
}

type editAssemblyConstraintPayload struct {
	GroupName               *string                `json:"groupName,omitempty"`
	GroupMembers            *[]AssemblyGroupMember `json:"groupMembers,omitempty"`
	Family                  string                 `json:"family,omitempty"`
	Subtype                 string                 `json:"subtype,omitempty"`
	ContactKind             string                 `json:"contactKind,omitempty"`
	ContactSide             string                 `json:"contactSide,omitempty"`
	ContactBranch           *int32                 `json:"contactBranch,omitempty"`
	QuantityExpression      *string                `json:"quantityExpression,omitempty"`
	QuantityKey             string                 `json:"quantityKey,omitempty"`
	Mode                    *string                `json:"mode,omitempty"`
	OffsetExpression        *string                `json:"offsetExpression,omitempty"`
	OffsetKey               string                 `json:"offsetKey,omitempty"`
	FixMode                 string                 `json:"fixMode,omitempty"`
	AngleRelation           string                 `json:"angleRelation,omitempty"`
	ConstraintID            string                 `json:"constraintId"`
	Value                   float64                `json:"value"`
	DirectionRelation       string                 `json:"directionRelation"`
	DistanceRelation        string                 `json:"distanceRelation"`
	First                   *AssemblyGeometryRef   `json:"first,omitempty"`
	Second                  *AssemblyGeometryRef   `json:"second,omitempty"`
	AngleAxis               *AssemblyGeometryRef   `json:"angleAxis,omitempty"`
	ReverseAngleAxis        *bool                  `json:"reverseAngleAxis,omitempty"`
	AngleReferenceDirection *[3]float64            `json:"angleReferenceDirection,omitempty"`
	FixedPose               *InstancePose          `json:"fixedPose,omitempty"`
}

func validateInstanceConstraintReferences(constraint AssemblyConstraint) error {
	if isAssemblyGroup(constraint) {
		return validateAssemblyGroupDefinition(constraint)
	}
	if constraint.Kind == "DISTANCE" {
		switch constraint.DistanceRelation {
		case "", "UNSIGNED", "SELECTED_PLANE_NORMAL_V1":
		default:
			return fmt.Errorf("%w: unknown offset sign convention", ErrValidation)
		}
		if (constraint.DistanceRelation == "" || constraint.DistanceRelation == "UNSIGNED") && constraint.Value < 0 {
			return fmt.Errorf("%w: unsigned offset must be nonnegative", ErrValidation)
		}
	}
	switch constraint.Kind {
	case "FIX", "RIGID", "COINCIDENT", "CONCENTRIC", "ANGLE", "DISTANCE", "CONTACT":
	default:
		return fmt.Errorf("%w: unknown assembly constraint kind", ErrValidation)
	}
	if !finite(constraint.Value) || (constraint.Kind == "ANGLE" && (constraint.Value < 0 || constraint.Value > 2*math.Pi)) {
		return fmt.Errorf("%w: invalid assembly quantity", ErrValidation)
	}
	if err := validateContactDefinition(constraint); err != nil {
		return err
	}

	if constraint.Mode != "" && constraint.Mode != "DRIVING" && constraint.Mode != "MEASURED" && constraint.Mode != "CONTROLLED" {
		return fmt.Errorf("%w: activation is independent of constraint mode", ErrValidation)
	}
	if constraint.FixMode != "" && (constraint.Kind != "FIX" || (constraint.FixMode != "SPACE" && constraint.FixMode != "RELATIVE")) {
		return fmt.Errorf("%w: invalid fixed reference mode", ErrValidation)
	}
	if constraint.AngleRelation != "" && (constraint.Kind != "ANGLE" || (constraint.AngleRelation != "FREE" && constraint.AngleRelation != "DIRECTED" && constraint.AngleRelation != "PARALLEL" && constraint.AngleRelation != "PERPENDICULAR")) {
		return fmt.Errorf("%w: invalid angle relation", ErrValidation)
	}
	if constraint.AngleAxis != nil {
		if constraint.Kind != "ANGLE" || constraint.Second == nil || constraint.AngleAxis.InstanceID == "" {
			return fmt.Errorf("%w: angle axis requires an explicit owning occurrence", ErrValidation)
		}
		if constraint.AngleAxis.Kind != "AXIS" && constraint.AngleAxis.Kind != "PLANE" && constraint.AngleAxis.Kind != "FACE" && constraint.AngleAxis.Kind != "EDGE" && constraint.AngleAxis.Kind != "CYLINDER" && constraint.AngleAxis.Kind != "CONE" && constraint.AngleAxis.Kind != "CIRCLE" && constraint.AngleAxis.Kind != "FRAME" {
			return fmt.Errorf("%w: angle axis requires a directional support", ErrValidation)
		}
	}
	if constraint.AngleRelation == "DIRECTED" && constraint.AngleAxis == nil {
		return fmt.Errorf("%w: directed angle requires a stable reference axis", ErrValidation)
	}
	if constraint.Mode == "MEASURED" && !assemblySupportsMeasurement(constraint) {
		return fmt.Errorf("%w: relation does not support measurement", ErrValidation)
	}
	if constraint.FixedPose != nil {
		for _, v := range constraint.FixedPose.Translation {
			if !finite(v) {
				return fmt.Errorf("%w: fixed position must be finite", ErrValidation)
			}
		}
		norm := 0.0
		for _, v := range constraint.FixedPose.Rotation {
			if !finite(v) {
				return fmt.Errorf("%w: fixed rotation must be finite", ErrValidation)
			}
			norm += v * v
		}
		if !finite(norm) || norm < 1e-20 {
			return fmt.Errorf("%w: fixed rotation must be nonzero", ErrValidation)
		}
	}

	if constraint.Kind == "FIX" {
		if constraint.First.Kind != "BODY" || constraint.Second != nil {
			return fmt.Errorf("%w: FIX must reference exactly one instance body", ErrValidation)
		}
		return nil
	}
	if constraint.Kind == "RIGID" {
		if constraint.First.Kind != "BODY" || constraint.Second == nil || constraint.Second.Kind != "BODY" {
			return fmt.Errorf("%w: RIGID must reference two instance bodies", ErrValidation)
		}
	}
	return nil
}

func applyEditAssemblyConstraint(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload editAssemblyConstraintPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if payload.ContactBranch != nil && *payload.ContactBranch != -1 && *payload.ContactBranch != 1 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: explicit contact branch must be -1 or 1", ErrValidation)
	}
	if !finite(payload.Value) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint value must be finite", ErrValidation)
	}
	for index := range model.Constraints {
		if model.Constraints[index].ID != payload.ConstraintID {
			continue
		}
		if model.Constraints[index].Kind == "RIGID" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: unsupported experimental rigid definition; use current Fix Together", ErrValidation)
		}
		if isAssemblyGroup(model.Constraints[index]) {
			return applyAssemblyGroupEdit(modelJSON, payload)
		}
		before := model.Constraints[index]
		if payload.GroupName != nil {
			name := strings.TrimSpace(*payload.GroupName)
			if name == "" {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint name must not be empty", ErrValidation)
			}
			model.Constraints[index].Name = name
		}
		if payload.FixMode != "" {
			model.Constraints[index].FixMode = payload.FixMode
		}
		if payload.AngleAxis != nil {
			model.Constraints[index].AngleAxis = payload.AngleAxis
		}
		if payload.ReverseAngleAxis != nil {
			model.Constraints[index].ReverseAngleAxis = *payload.ReverseAngleAxis
		}
		if payload.AngleRelation != "" {
			model.Constraints[index].AngleRelation = payload.AngleRelation
		}
		model.Constraints[index].Value = payload.Value
		model.Constraints[index].DirectionRelation = payload.DirectionRelation
		model.Constraints[index].DistanceRelation = payload.DistanceRelation
		if payload.Mode != nil {
			model.Constraints[index].Mode = *payload.Mode
		}
		if payload.AngleReferenceDirection != nil {
			model.Constraints[index].AngleReferenceDirection = payload.AngleReferenceDirection
		}
		if payload.AngleRelation != "" && payload.AngleRelation != "DIRECTED" {
			model.Constraints[index].AngleAxis = nil
			model.Constraints[index].AngleReferenceDirection = nil
			model.Constraints[index].ReverseAngleAxis = false
		}
		if payload.FixedPose != nil {
			model.Constraints[index].FixedPose = payload.FixedPose
		}
		if payload.First != nil {
			model.Constraints[index].First = *payload.First
		}
		if payload.Second != nil {
			model.Constraints[index].Second = payload.Second
		}
		sameSupport := func(a, b *AssemblyGeometryRef) bool {
			if a == nil || b == nil {
				return a == b
			}
			x, y := *a, *b
			x.Resolution, y.Resolution = nil, nil
			x.PublicationResolution, y.PublicationResolution = nil, nil
			x.GeometryKey, y.GeometryKey = "", ""
			x.TopologyID, y.TopologyID = 0, 0
			return reflect.DeepEqual(x, y)
		}
		after := &model.Constraints[index]
		if payload.ContactKind != "" {
			after.ContactKind = payload.ContactKind
		}
		if payload.ContactSide != "" {
			after.ContactSide = payload.ContactSide
		}
		if payload.ContactBranch != nil {
			after.ContactBranch = *payload.ContactBranch
		}
		if err := canonicalAssemblyDefinition(after, payload.Family, payload.Subtype); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if payload.AngleRelation == "DIRECTED" || !sameSupport(&before.First, &after.First) || !sameSupport(before.Second, after.Second) {
			after.SpatialAngleBranchDirection = nil
		}
		model.Constraints[index].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
		model.Constraints[index].EvaluationSummary = "constraint definition changed; awaiting authoritative solve"
		if model.Constraints[index].Kind != "FIX" {
			if model.Constraints[index].Second == nil {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: second assembly reference is required", ErrValidation)
			}
			if model.Constraints[index].First.InstanceID == model.Constraints[index].Second.InstanceID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: a binary assembly constraint requires two different instances", ErrValidation)
			}
		}
		expression, key, err := assemblyQuantityInputs(model.Constraints[index].Kind, payload.QuantityExpression, payload.QuantityKey, payload.OffsetExpression, payload.OffsetKey)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if err := editAssemblyQuantity(&model, index, expression, key, payload.Value); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if err := validateInstanceConstraintReferences(model.Constraints[index]); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		model.Constraints[index].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
		model.Constraints[index].EvaluationSummary = "definition changed; evaluation pending"
		model.Constraints[index].EvaluationFailure = nil
		model.Constraints[index].MeasuredValue = nil
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.ConstraintID, SlotID: "assembly-constraint.entity"}, before, model.Constraints[index])
		changes := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(payload.ConstraintID)}}
		if p := assemblyQuantityParameter(model.Constraints[index]); p != nil {
			changes.ImpactSeeds = append(changes.ImpactSeeds, modelcore.DependencyKey("parameter:"+p.ParameterID))
		}
		// Explicit relative-Fix pose editing is a placement edit. Otherwise the
		// next solve would recapture the old nominal placement and discard it.
		c := model.Constraints[index]
		if c.Kind == "FIX" && c.FixMode == "RELATIVE" && payload.FixedPose != nil && !c.Suppressed {
			for i := range model.Instances {
				instance := &model.Instances[i]
				if instance.ID != c.First.InstanceID {
					continue
				}
				old := InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
				instance.Translation, instance.Rotation = c.FixedPose.Translation, c.FixedPose.Rotation
				poseChange, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: instance.ID, SlotID: "instance.pose"}, old, *c.FixedPose)
				changes.Changes = append(changes.Changes, poseChange)
				changes.ImpactSeeds = append(changes.ImpactSeeds, modelcore.DependencyKey("placement:"+instance.ID))
			}
		}
		var priorProduct ProductModel
		if err := json.Unmarshal(modelJSON, &priorProduct); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		changes = appendAssemblyEvaluationChanges(changes, priorProduct, model)
		next, _ := json.Marshal(model)
		changes, err = reconcilePersistedChanges("PRODUCT", modelJSON, next, changes)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		return next, changes, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: assembly constraint does not exist", ErrValidation)
}

func applyAddAssemblyConstraint(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload addAssemblyConstraintPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if isAssemblyGroup(payload.Constraint) || payload.Constraint.Family == "FixTogether" {
		if payload.QuantityExpression != nil || payload.OffsetExpression != nil || payload.QuantityKey != "" || payload.OffsetKey != "" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: group has no Quantity parameter", ErrValidation)
		}
		return applyAssemblyGroupCreate(modelJSON, payload.Constraint)
	}
	for _, existing := range model.Constraints {
		if existing.ID == payload.Constraint.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate assembly constraint identity", ErrValidation)
		}
	}
	model.Constraints = append(model.Constraints, payload.Constraint)
	expression, key, err := assemblyQuantityInputs(payload.Constraint.Kind, payload.QuantityExpression, payload.QuantityKey, payload.OffsetExpression, payload.OffsetKey)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := editAssemblyQuantity(&model, len(model.Constraints)-1, expression, key, payload.Constraint.Value); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	payload.Constraint = model.Constraints[len(model.Constraints)-1]
	if err := validateInstanceConstraintReferences(payload.Constraint); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Constraint.ID, SlotID: "assembly-constraint.entity"}, nil, payload.Constraint)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(payload.Constraint.ID)}}, nil
}

func applyMoveInstance(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload moveInstancePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if !finite(payload.Translation[0]) || !finite(payload.Translation[1]) || !finite(payload.Translation[2]) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: translation must be finite", ErrValidation)
	}
	for index := range model.Instances {
		if model.Instances[index].ID == payload.InstanceID {
			before := model.Instances[index].Translation
			model.Instances[index].Translation = payload.Translation
			model.Instances[index].Rotation = normalizedInstanceRotation(payload.Rotation)
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.InstanceID, SlotID: "instance.translation"}, before, payload.Translation)
			set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"placement:" + modelcore.DependencyKey(payload.InstanceID)}}
			next, _ := json.Marshal(model)
			return next, set, nil
		}
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
}

type referenceModePayload struct{ InstanceID, Mode, PinnedVersionID string }

func applyReferenceMode(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload referenceModePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index := range model.Instances {
		instance := &model.Instances[index]
		if instance.ID != payload.InstanceID {
			continue
		}
		before := struct{ Mode, Version, DocumentID string }{instance.ReferenceMode, instance.ReferencedVersionID, instance.ReferencedDocumentID}
		instance.ReferenceMode = payload.Mode
		if payload.Mode == "PINNED" {
			instance.ReferencedVersionID = payload.PinnedVersionID
		}
		instance.ResolvedVersionID = ""
		instance.HeadChanged = false
		after := struct{ Mode, Version, DocumentID string }{instance.ReferenceMode, instance.ReferencedVersionID, instance.ReferencedDocumentID}
		change, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: payload.InstanceID, SlotID: "instance.reference"}, before, after)
		set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"reference:" + modelcore.DependencyKey(payload.InstanceID)}}
		next, _ := json.Marshal(model)
		return next, set, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
}

type updateReferencesPayload struct {
	Model ProductModel `json:"model"`
}

func applyUpdateReferences(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var before ProductModel
	var payload updateReferencesPayload
	if err := json.Unmarshal(modelJSON, &before); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	changes := []modelcore.ModelChange{}
	beforeInstances := map[string]ProductInstance{}
	for _, value := range before.Instances {
		beforeInstances[value.ID] = value
	}
	for _, value := range payload.Model.Instances {
		if old, ok := beforeInstances[value.ID]; ok && (old.ReferenceMode != value.ReferenceMode || old.ReferencedVersionID != value.ReferencedVersionID) {
			change, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: value.ID, SlotID: "instance.reference"}, struct{ Mode, Version string }{old.ReferenceMode, old.ReferencedVersionID}, struct{ Mode, Version string }{value.ReferenceMode, value.ReferencedVersionID})
			changes = append(changes, change)
		}
	}
	beforeConstraints := map[string]AssemblyConstraint{}
	for _, value := range before.Constraints {
		beforeConstraints[value.ID] = value
	}
	for _, value := range payload.Model.Constraints {
		if old, ok := beforeConstraints[value.ID]; ok && !reflect.DeepEqual(old, value) {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: value.ID, SlotID: "assembly-constraint.entity"}, old, value)
			changes = append(changes, change)
		}
	}
	beforePublications := map[string]ProductPublication{}
	for _, value := range before.Publications {
		beforePublications[value.ID] = value
	}
	for _, value := range payload.Model.Publications {
		if old, ok := beforePublications[value.ID]; ok && !reflect.DeepEqual(old, value) {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate,
				modelcore.PropertyAddress{EntityID: value.ID, SlotID: "product-publication.entity"}, old, value)
			changes = append(changes, change)
		}
	}
	next, _ := json.Marshal(payload.Model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: []modelcore.DependencyKey{"product:references"}}, nil
}

type preparedDomainMutation struct {
	workspaceID, headRevision, documentType, requestDigest, requestID, actorID string
	headSequence                                                               uint64
	modelJSON                                                                  json.RawMessage
	command                                                                    modelcore.DomainCommand
	transactionID                                                              string
	priorManifest                                                              *modelcore.EvaluationManifest
}

func isPartMetadataCommand(command modelcore.DomainCommand) bool {
	if command.TypeURI == typeDefinitionVisibility || command.TypeURI == typeRenameFeature {
		return true
	}
	if command.TypeURI != typeBodyCommand {
		return false
	}
	var payload bodyCommand
	return json.Unmarshal(command.Payload, &payload) == nil && (payload.Action == "RENAME" || payload.Action == "VISIBILITY")
}

func (service *Service) prepareDomainMutation(ctx context.Context, documentID string, request CommandRequest) (preparedDomainMutation, error) {
	var prepared preparedDomainMutation
	request.RequestID = requestID(request.RequestID)
	prepared.requestID = request.RequestID
	prepared.actorID = actorID(request.ActorID)
	var priorManifestJSON []byte
	if err := service.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,d.document_type,v.model_json,v.evaluation_manifest FROM occccad.workspaces w JOIN occccad.documents d ON d.id=w.document_id JOIN occccad.document_versions v ON v.id=w.head_revision_id WHERE w.document_id=$1 AND w.name='main' AND d.deleted_at IS NULL`, documentID).Scan(&prepared.workspaceID, &prepared.headRevision, &prepared.headSequence, &prepared.documentType, &prepared.modelJSON, &priorManifestJSON); errors.Is(err, pgx.ErrNoRows) {
		return prepared, ErrNotFound
	} else if err != nil {
		return prepared, err
	}
	if request.ExpectedUpdateRevision != "" && prepared.headRevision != request.ExpectedUpdateRevision {
		return prepared, fmt.Errorf("%w: PARAMETER_UPDATE_CONSUMER_CHANGED", ErrValidation)
	}
	if len(priorManifestJSON) > 0 {
		var manifest modelcore.EvaluationManifest
		if json.Unmarshal(priorManifestJSON, &manifest) == nil {
			prepared.priorManifest = &manifest
		}
	}
	if prepared.documentType == "PRODUCT" {
		var model ProductModel
		if err := json.Unmarshal(prepared.modelJSON, &model); err != nil {
			return prepared, err
		}
		if err := validateAssemblyDefinitionFormat(model); err != nil {
			return prepared, err
		}
	}
	command, payload, err := service.adaptLegacyCommand(ctx, documentID, prepared.documentType, prepared.modelJSON, request)
	if err != nil {
		return prepared, err
	}
	if command == typeImportExchange || command == typeRepairImportNaming {
		input := payload.(createFeaturePayload)
		if input.Feature.BodyID == "" {
			var m PartModel
			_ = json.Unmarshal(prepared.modelJSON, &m)
			normalizePartModel(&m)
			input.Feature.BodyID = m.ActiveBodyID
		}
		input.Feature.ImportDefinitionID, err = service.allocateImportDefinition(ctx, documentID, input.Feature, request.ImportSource)
		if err != nil {
			return prepared, err
		}
		payload = input
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return prepared, err
	}
	prepared.command = modelcore.DomainCommand{CommandID: newID("command"), TypeURI: command, SchemaVersion: 1, Payload: payloadJSON}
	transactionUUID, err := uuid.NewV7()
	if err != nil {
		return prepared, err
	}
	prepared.transactionID = transactionUUID.String()
	transaction := modelcore.DomainTransaction{TransactionID: prepared.transactionID, RequestID: request.RequestID, DocumentID: documentID, WorkspaceID: prepared.workspaceID, ExpectedHeadSequence: prepared.headSequence, ExpectedHeadRevisionID: prepared.headRevision, Commands: []modelcore.DomainCommand{prepared.command}, EvaluationPolicy: "IMMEDIATE_ALLOW_FEATURE_FAILURE"}
	if _, err = transaction.CanonicalDigest(); err != nil {
		return prepared, err
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return prepared, err
	}
	prepared.requestDigest = modelcore.ValueDigest(requestJSON)
	return prepared, nil
}

func (service *Service) applyDomainMutation(ctx context.Context, documentID string, request CommandRequest) error {
	// Check committed intent before adapting against the new Head: import/repair
	// preconditions intentionally cease to hold after their first successful commit.
	request.RequestID = requestID(request.RequestID)
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return err
	}
	var storedDigest, storedActor string
	err = service.database.QueryRow(ctx, `SELECT t.request_digest,t.actor_id::text FROM occccad.domain_transactions t JOIN occccad.workspaces w ON w.id=t.workspace_id WHERE w.document_id=$1 AND w.name='main' AND t.request_id=$2 AND t.status='COMMITTED'`, documentID, request.RequestID).Scan(&storedDigest, &storedActor)
	if err == nil {
		if storedActor != actorID(request.ActorID) || storedDigest != modelcore.ValueDigest(requestJSON) {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	finishPrepare := perf.Start(ctx, "command-prepare")
	prepared, err := service.prepareDomainMutation(ctx, documentID, request)
	finishPrepare()
	if err != nil {
		return err
	}
	finishPromote := perf.Start(ctx, "candidate-promote")
	if request.SessionID != "" {
		if err := service.validateInteractionCommit(ctx, documentID, prepared, request); err != nil {
			finishPromote()
			return err
		}
	}
	candidate, rejection := service.interactionCandidates.takeDiagnosed(request.PreviewID, documentID, prepared)
	promoted := rejection == ""
	if promoted && candidate.definitionOnly && !retainsAssemblyDefinition(prepared.command.TypeURI) {
		return fmt.Errorf("%w: definition-only candidate cannot commit a move", ErrValidation)
	}
	finishPromote()
	if request.PreviewID != "" && !promoted {
		return fmt.Errorf("%w: PREVIEW_CANDIDATE_STALE_OR_MISMATCHED: %s", ErrValidation, rejection)
	}
	var nextJSON json.RawMessage
	var changes modelcore.ChangeSet
	if promoted {
		nextJSON, changes = candidate.nextJSON, candidate.changes
	} else {
		finishApply := perf.Start(ctx, "command-apply")
		nextJSON, changes, err = workspaceCommandRegistry.Apply(prepared.documentType, prepared.modelJSON, prepared.command)
		finishApply()
		if err != nil {
			return err
		}
	}
	revisionUUID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	revisionID := revisionUUID.String()
	modelHash := canonicalModelHash(nextJSON)
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	revisionState, evaluationStatus := "READY", "SUCCEEDED"
	if prepared.documentType == "PART" {
		var model PartModel
		var beforeModel PartModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		if err := json.Unmarshal(prepared.modelJSON, &beforeModel); err != nil {
			return err
		}
		normalizePartModel(&model)
		normalizePartModel(&beforeModel)
		metadataOnly := isPartMetadataCommand(prepared.command)
		if !promoted && !metadataOnly {
			if err := validateAndResolvePartParameters(&model); err != nil {
				return err
			}
			finishSolve := perf.Start(ctx, "support-resolve-sketch-solve")
			if err := service.resolveAndSolveSketches(ctx, documentID, prepared.requestID, &model); err != nil {
				finishSolve()
				return err
			}
			finishSolve()
		}
		if !metadataOnly {
			if err := validateAndResolvePartParameters(&model); err != nil {
				return err
			}
		}
		if err := rejectExplicitUnresolvedExternal(prepared.command, model); err != nil {
			return err
		}
		if !metadataOnly {
			if err := service.resolvePartPublications(ctx, documentID, prepared.requestID, revisionID, &model); err != nil {
				return err
			}
		}
		if err := rejectExplicitBrokenPublication(prepared.command, model); err != nil {
			return err
		}
		changes = appendEvaluatedSketchChanges(changes, beforeModel, model)
		if _, failedRevision, failedEvaluation, unresolved := unresolvedExternalRevisionOutcome(model); unresolved {
			revisionState, evaluationStatus = failedRevision, failedEvaluation
			for i := range model.Bodies {
				model.Bodies[i].GeometryKey = ""
			}
		} else if metadataOnly {
			if partHasFailedFeature(model) {
				revisionState, evaluationStatus = "FAILED", "FAILED"
			}
			// Display metadata and readable names do not change geometry inputs.
			// Keep the frozen per-Body results without entering the evaluator.
			for i := range model.Bodies {
				for _, previous := range beforeModel.Bodies {
					if previous.ID == model.Bodies[i].ID {
						model.Bodies[i].GeometryKey = previous.GeometryKey
						break
					}
				}
			}
		} else {
			finishGeometry := perf.Start(ctx, "geometry-evaluate")
			err = service.evaluatePartBodies(ctx, prepared.requestID, &model)
			finishGeometry()
			if err != nil {
				var failed *solidEvaluationFailure
				if !errors.As(err, &failed) || (prepared.command.TypeURI != typeEditFeature && prepared.command.TypeURI != typeSetFeatureSuppression && prepared.command.TypeURI != typeSetParameterLiteral && prepared.command.TypeURI != typeSetParameterExpression && prepared.command.TypeURI != typeEditParameter && prepared.command.TypeURI != typeEditSketch) {
					return err
				}
				revisionState, evaluationStatus = "FAILED", "FAILED"
			}
		}
		retainFailedBodyDisplay(&model, beforeModel, prepared.headRevision)
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		if err != nil {
			return err
		}
	} else {
		var model ProductModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		if prepared.command.TypeURI == typeUpdateReferences {
			for index := range model.ContextBindings {
				model.ContextBindings[index].Accepted.RootProductRevisionID = revisionID
				if len(model.ContextBindings[index].SourceInstancePath.Segments) > 0 {
					model.ContextBindings[index].SourceInstancePath.Segments[0].OwnerVersionID = revisionID
				}
				if len(model.ContextBindings[index].OwningInstancePath.Segments) > 0 {
					model.ContextBindings[index].OwningInstancePath.Segments[0].OwnerVersionID = revisionID
				}
			}
		}
		if promoted && !candidate.definitionOnly && (len(model.Constraints) > 0 || request.SessionID != "") {
			if err = service.promoteAssemblySolveManifest(ctx, documentID, revisionID, prepared.requestID, candidate.assemblyPreviewRequestID, canonicalModelHash(nextJSON)); err != nil {
				return err
			}
		}
		if !promoted && prepared.command.TypeURI != typeOccurrenceVisibility {
			finishSolve := perf.Start(ctx, "assembly-solve")
			drivenInstanceID := ""
			var solveIntent *geometry.AssemblySolveIntent
			if prepared.command.TypeURI == typeMoveInstance {
				var payload moveInstancePayload
				_ = json.Unmarshal(prepared.command.Payload, &payload)
				drivenInstanceID = payload.InstanceID
			} else {
				solveIntent = assemblyConstraintSolveIntent(prepared.command, model)
			}
			if err = service.solveAssembly(ctx, documentID, revisionID, prepared.requestID, drivenInstanceID, solveIntent, &model, ""); err != nil {
				allowFailure := retainsAssemblyDefinition(prepared.command.TypeURI) || prepared.command.TypeURI == typeUpdateReferences || prepared.command.TypeURI == typeReplaceInstance
				if err = acceptAssemblyEvaluationFailure(&model, err, allowFailure); err != nil {
					finishSolve()
					return err
				}
			}
			finishSolve()
		}
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		var priorProduct ProductModel
		_ = json.Unmarshal(prepared.modelJSON, &priorProduct)
		changes = appendAssemblyEvaluationChanges(changes, priorProduct, model)
		graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		if err != nil {
			return err
		}
	}
	changes, err = reconcilePersistedChanges(prepared.documentType, prepared.modelJSON, nextJSON, changes)
	if err != nil {
		return err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := modelcore.ValueDigest(manifestJSON)
	dependencyDigest, err := graph.Digest()
	if err != nil {
		return err
	}
	changesJSON, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	payloadDigest := modelcore.ValueDigest(prepared.command.Payload)
	finishCommit := perf.Start(ctx, "commit")
	defer finishCommit()
	tx, err := service.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentHead string
	var currentSequence uint64
	if err := tx.QueryRow(ctx, `SELECT head_revision_id::text,head_sequence FROM occccad.workspaces WHERE id=$1 FOR UPDATE`, prepared.workspaceID).Scan(&currentHead, &currentSequence); err != nil {
		return err
	}
	if currentHead != prepared.headRevision || currentSequence != prepared.headSequence {
		var completedDigest, completedActor string
		completedErr := tx.QueryRow(ctx, `SELECT request_digest,actor_id::text FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, prepared.workspaceID, prepared.requestID).Scan(&completedDigest, &completedActor)
		if completedErr == nil {
			if completedActor != prepared.actorID || completedDigest != prepared.requestDigest {
				return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
			}
			return nil
		}
		if !errors.Is(completedErr, pgx.ErrNoRows) {
			return completedErr
		}

		return fmt.Errorf("%w: WORKSPACE_HEAD_CONFLICT", ErrValidation)
	}
	var revisionSequence uint64
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(sequence),0)+1 FROM occccad.document_versions WHERE document_id=$1`, documentID).Scan(&revisionSequence); err != nil {
		return err
	}
	transportPayload, _ := json.Marshal(request)
	traceID, spanID := traceIDs(ctx)
	var auditCommandID string
	if err := tx.QueryRow(ctx, `INSERT INTO occccad.commands(request_id,command_type,document_id,payload,status,completed_at,trace_id,span_id) VALUES($1,$2,$3,$4,'SUCCEEDED',now(),$5,$6) RETURNING id::text`, prepared.requestID, request.Type, documentID, transportPayload, traceID, spanID).Scan(&auditCommandID); err != nil {
		return err
	}
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, revisionID, documentID, prepared.headRevision, revisionSequence, nextJSON, revisionState, auditCommandID, modelHash, dependencyDigest, manifestJSON)
	batch.Queue(`INSERT INTO occccad.revision_parents(revision_id,parent_revision_id,ordinal) VALUES($1,$2,0)`, revisionID, prepared.headRevision)
	batch.Queue(`INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,base_revision_id,result_revision_id,committed_at) VALUES($1,$2,$3,$4,$5,$6,'DOMAIN','COMMITTED',$7,$8,now())`, prepared.transactionID, prepared.workspaceID, currentSequence+1, prepared.actorID, prepared.requestID, prepared.requestDigest, prepared.headRevision, revisionID)
	batch.Queue(`INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,$4,$5,$6)`, prepared.transactionID, prepared.command.CommandID, prepared.command.TypeURI, prepared.command.SchemaVersion, prepared.command.Payload, payloadDigest)
	writes := make([]string, 0, len(changes.Changes))
	for _, change := range changes.Changes {
		writes = append(writes, change.Target.Key())
	}
	batch.Queue(`INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, prepared.transactionID, changesJSON, changes.CanonicalDigest, writes, changes.ImpactSeeds)
	batch.Queue(`INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,$7,true)`, revisionID, strings.ToLower(prepared.documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest, evaluationStatus)
	for _, edge := range graph.Edges {
		batch.Queue(`INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind)
	}
	eventPayload, _ := json.Marshal(map[string]any{"workspaceId": prepared.workspaceID, "sequence": currentSequence + 1, "revisionId": revisionID, "transactionId": prepared.transactionID, "modelHash": modelHash, "changeDigest": changes.CanonicalDigest})
	batch.Queue(`INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, prepared.workspaceID, eventPayload)
	if err := database.ExecBatch(ctx, tx, batch); err != nil {
		return err
	}

	var position int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(position),-1)+1 FROM occccad.document_history WHERE document_id=$1`, documentID).Scan(&position); err != nil {
		return err
	}
	batch = &pgx.Batch{}
	batch.Queue(`INSERT INTO occccad.document_history(document_id,position,version_id,command_id) VALUES($1,$2,$3,$4)`, documentID, position, revisionID, auditCommandID)
	batch.Queue(`INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type) VALUES($1,$2,$3,$4)`, documentID, revisionID, auditCommandID, request.Type)
	batch.Queue(`UPDATE occccad.workspaces SET head_revision_id=$1,head_sequence=$2,updated_at=now() WHERE id=$3`, revisionID, currentSequence+1, prepared.workspaceID)
	batch.Queue(`UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`, revisionID, documentID)
	if err := database.ExecBatch(ctx, tx, batch); err != nil {
		return err
	}

	if prepared.documentType == "PRODUCT" {
		var model ProductModel
		_ = json.Unmarshal(nextJSON, &model)
		if err := insertProductInstances(ctx, tx, revisionID, model); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func firstUnresolvedExternal(model PartModel) (SketchExternalGeometry, bool) {
	for _, feature := range model.Features {
		if feature.Sketch == nil {
			continue
		}
		for _, external := range feature.Sketch.ExternalGeometry {
			if external.Status != "CONNECTED" || external.Snapshot == nil {
				return external, true
			}
		}
	}
	return SketchExternalGeometry{}, false
}

func unresolvedExternalRevisionOutcome(model PartModel) (geometryKey, revisionState, evaluationStatus string, unresolved bool) {
	if _, unresolved = firstUnresolvedExternal(model); !unresolved {
		return "", "", "", false
	}
	// A dependency snapshot key identifies the upstream prefix used for
	// resolution; it is not the final shape of this Revision. Persisting it as
	// the Revision artifact would violate model/visualization provenance.
	return "", "FAILED", "FAILED", true
}

func assemblyConstraintSolveIntent(command modelcore.DomainCommand, model ProductModel) *geometry.AssemblySolveIntent {
	var first, second string
	switch command.TypeURI {
	case typeAddAssemblyConstraint:
		var payload addAssemblyConstraintPayload
		if json.Unmarshal(command.Payload, &payload) == nil && payload.Constraint.Second != nil {
			first, second = payload.Constraint.First.InstanceID, payload.Constraint.Second.InstanceID
		}
	case typeEditAssemblyConstraint:
		var payload editAssemblyConstraintPayload
		if json.Unmarshal(command.Payload, &payload) == nil {
			for _, constraint := range model.Constraints {
				if constraint.ID == payload.ConstraintID && constraint.Second != nil {
					first, second = constraint.First.InstanceID, constraint.Second.InstanceID
					break
				}
			}
		}
	}
	if first == "" || second == "" {
		return nil
	}
	return &geometry.AssemblySolveIntent{MovingBodyIDs: []string{first}, ReferenceBodyIDs: []string{second},
		PreferencePolicy: "MOVE_FIRST_MINIMIZE_REFERENCE"}
}

// PreviewCommand runs the normal command adapter, typed handler, sketch solver
// and authoritative Part evaluator without creating a Revision, advancing a
// Workspace or appending history. Geometry artifacts remain content-addressed
// rebuildable cache entries and may therefore be reused by the later commit.
func (service *Service) PreviewCommand(ctx context.Context, documentID string, request CommandRequest) (CommandPreview, error) {
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	if request.Type == "UNDO" || request.Type == "REDO" || request.Type == "RESTORE" {
		return CommandPreview{}, fmt.Errorf("%w: history commands cannot be previewed", ErrValidation)
	}
	finishPrepare := perf.Start(ctx, "preview-prepare")
	prepared, err := service.prepareDomainMutation(ctx, documentID, request)
	finishPrepare()
	if err != nil {
		return CommandPreview{}, err
	}
	nextJSON, previewChanges, err := workspaceCommandRegistry.Apply(prepared.documentType, prepared.modelJSON, prepared.command)
	if err != nil {
		return CommandPreview{}, err
	}
	if strings.EqualFold(prepared.documentType, "PRODUCT") {
		var model ProductModel
		if err = json.Unmarshal(nextJSON, &model); err != nil {
			return CommandPreview{}, err
		}
		driven := ""
		var solveIntent *geometry.AssemblySolveIntent
		if prepared.command.TypeURI == typeMoveInstance {
			var payload moveInstancePayload
			_ = json.Unmarshal(prepared.command.Payload, &payload)
			driven = payload.InstanceID
		} else {
			solveIntent = assemblyConstraintSolveIntent(prepared.command, model)
		}
		var assemblyResult geometry.AssemblySolve
		retainedFailure := false
		warmStartKey := ""
		if request.InteractionID != "" {
			warmStartKey = documentID + "|" + prepared.actorID + "|" + request.InteractionID
		}
		if err = service.solveAssembly(ctx, documentID, prepared.headRevision, "preview/"+prepared.requestID, driven, solveIntent, &model, warmStartKey, &assemblyResult); err != nil {
			if retained := acceptAssemblyEvaluationFailure(&model, err, retainsAssemblyDefinition(prepared.command.TypeURI)); retained == nil {
				retainedFailure = true
			} else {
				return CommandPreview{}, err
			}
		}
		nextJSON, err = json.Marshal(model)
		if err != nil {
			return CommandPreview{}, err
		}
		constraintID := ""
		switch prepared.command.TypeURI {
		case typeAddAssemblyConstraint:
			var payload addAssemblyConstraintPayload
			if json.Unmarshal(prepared.command.Payload, &payload) == nil {
				constraintID = payload.Constraint.ID
			}
		case typeEditAssemblyConstraint:
			var payload editAssemblyConstraintPayload
			if json.Unmarshal(prepared.command.Payload, &payload) == nil {
				constraintID = payload.ConstraintID
			}
		}
		unverifiedOffset := false
		for _, constraint := range model.Constraints {
			if constraint.ID == constraintID && constraint.Kind == "DISTANCE" && constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
				// Admission may solve the accepted subset successfully while the
				// edited Offset is isolated as NotUpdated/Broken. That is not a
				// successful candidate for this definition, even if solve converged.
				unverifiedOffset = true
			}
		}
		previewID := newID("preview")
		if !retainedFailure && (assemblyResult.Status != "CONVERGED" || unverifiedOffset) {
			previewID = ""
		}
		if previewID != "" {
			service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
				definitionOnly: retainedFailure,
				headRevision:   prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
				assemblyPreviewRequestID: "preview/" + prepared.requestID,
				payloadDigest:            modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, changes: previewChanges,
				expiresAt: time.Now().Add(interactionCandidateTTL)})
		}
		result := CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence,
			ModelHash: canonicalModelHash(nextJSON), AssemblyComponents: assemblyResult.Components, AssemblySolverBuild: assemblyResult.SolverBuild}
		if retainedFailure {
			result.EvaluationOutcome = "DEFINITION_ONLY"
			result.AssemblyComponents = nil
		}
		for _, constraint := range model.Constraints {
			if constraint.ID != constraintID {
				continue
			}
			preview := AssemblyConstraintPreviewEvaluation{ConstraintID: constraint.ID, Status: constraint.EvaluationStatus,
				Summary: constraint.EvaluationSummary, First: assemblySupportPreview(constraint.First)}
			if constraint.Second != nil {
				second := assemblySupportPreview(*constraint.Second)
				preview.Second = &second
			}
			result.ConstraintEvaluation = &preview
			if retainedFailure {
				result.EvaluationFailure = constraint.EvaluationFailure
			}
			break
		}
		for _, instance := range model.Instances {
			result.InstancePoses = append(result.InstancePoses, struct {
				InstanceID  string     `json:"instanceId"`
				Translation [3]float64 `json:"translation"`
				Rotation    [4]float64 `json:"rotation"`
			}{instance.ID, instance.Translation, normalizedInstanceRotation(instance.Rotation)})
		}
		return result, nil
	}
	var model PartModel
	var beforeModel PartModel
	if err = json.Unmarshal(nextJSON, &model); err != nil {
		return CommandPreview{}, err
	}
	if err = json.Unmarshal(prepared.modelJSON, &beforeModel); err != nil {
		return CommandPreview{}, err
	}
	normalizePartModel(&model)
	normalizePartModel(&beforeModel)
	if err = validateAndResolvePartParameters(&model); err != nil {
		return CommandPreview{}, err
	}
	finishSolve := perf.Start(ctx, "support-resolve-sketch-solve")
	if err = service.resolveAndSolveSketches(ctx, documentID, "preview/"+prepared.requestID, &model); err != nil {
		finishSolve()
		return CommandPreview{}, err
	}
	finishSolve()
	if err = validateAndResolvePartParameters(&model); err != nil {
		return CommandPreview{}, err
	}
	if err = rejectExplicitUnresolvedExternal(prepared.command, model); err != nil {
		return CommandPreview{}, err
	}
	if err = service.resolvePartPublications(ctx, documentID, "preview/"+prepared.requestID,
		prepared.headRevision, &model); err != nil {
		return CommandPreview{}, err
	}
	if err = rejectExplicitBrokenPublication(prepared.command, model); err != nil {
		return CommandPreview{}, err
	}
	previewChanges = appendEvaluatedSketchChanges(previewChanges, beforeModel, model)
	nextJSON, _ = json.Marshal(model)
	previewChanges, err = reconcilePersistedChanges(prepared.documentType, prepared.modelJSON, nextJSON, previewChanges)
	if err != nil {
		return CommandPreview{}, err
	}
	modelHash := canonicalModelHash(nextJSON)
	if _, broken := firstUnresolvedExternal(model); broken {
		previewID := newID("preview")
		service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
			headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
			payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, changes: previewChanges,
			expiresAt: time.Now().Add(interactionCandidateTTL)})
		return CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence, ModelHash: modelHash}, nil
	}
	finishGeometry := perf.Start(ctx, "geometry-evaluate")
	bodyID := previewBodyID(prepared.command.Payload, model)
	geometryKey, err := service.evaluateBodyPrefix(ctx, "preview/"+prepared.requestID, model, bodyID)
	finishGeometry()
	if err != nil {
		return CommandPreview{}, err
	}
	finishArtifact := perf.Start(ctx, "artifact-load")
	artifact, err := service.loadArtifact(ctx, geometryKey)
	finishArtifact()
	if err != nil {
		return CommandPreview{}, err
	}
	artifact.BodyID = bodyID
	bodyName := ""
	bodyAssignment := "TARGET_BODY"
	if index := bodyIndex(model, bodyID); index >= 0 {
		model.Bodies[index].GeometryKey = geometryKey
		bodyName = model.Bodies[index].Name
		if bodyIndex(beforeModel, bodyID) < 0 {
			bodyAssignment = "EXPLICIT_NEW_BODY"
		}
	}
	nextJSON, _ = json.Marshal(model)
	modelHash = canonicalModelHash(nextJSON)
	previewID := newID("preview")
	service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
		headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
		payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, geometryKey: geometryKey, visualObjectID: artifact.Representations["VISUAL"].ObjectID,
		changes: previewChanges, expiresAt: time.Now().Add(interactionCandidateTTL)})
	artifact.RepresentationKind = "TRANSIENT_PREVIEW"

	return CommandPreview{
		PreviewID: previewID, BaseVersionID: prepared.headRevision,
		BaseSequence: prepared.headSequence, ModelHash: modelHash, Artifact: &artifact,
		ResultBodyID: bodyID, ResultBodyName: bodyName, BodyAssignment: bodyAssignment,
		SketchCandidates: solvedSketchPreviewCandidates(prepared.command, model),
	}, nil
}

func assemblySupportPreview(reference AssemblyGeometryRef) AssemblySupportPreviewEvaluation {
	if reference.Kind != "FACE" && reference.Kind != "EDGE" && reference.Kind != "VERTEX" {
		return AssemblySupportPreviewEvaluation{Status: modelcore.SupportingElementConnected}
	}
	if reference.Resolution == nil {
		return AssemblySupportPreviewEvaluation{Status: modelcore.SupportingElementNotConnected,
			DiagnosticCode: "RESOLUTION_UNAVAILABLE", Diagnostic: "persistent support has no resolution snapshot"}
	}
	return AssemblySupportPreviewEvaluation{Status: reference.Resolution.Result.SupportingElementStatus,
		DiagnosticCode: reference.Resolution.Result.DiagnosticCode, Diagnostic: reference.Resolution.Result.Diagnostic}
}

func (service *Service) solveSketches(ctx context.Context, requestID string, model *PartModel) error {
	for featureIndex := range model.Features {
		if err := service.solveSketchFeature(ctx, requestID, model, featureIndex); err != nil {
			return err
		}
	}
	refreshSketchReferenceMeasurements(model)
	return nil
}

func (service *Service) solveSketchFeature(ctx context.Context, requestID string, model *PartModel, featureIndex int, targets ...geometry.SketchDragTarget) error {
	sketch := model.Features[featureIndex].Sketch
	if sketch == nil || (len(sketch.Entities) == 0 && len(sketch.ExternalGeometry) == 0) {
		return nil
	}
	brokenExternal := map[string]bool{}
	brokenDiagnostics := []string{}
	for _, external := range sketch.ExternalGeometry {
		if external.Status != "CONNECTED" || external.Snapshot == nil {
			brokenExternal[external.ID] = true
			brokenDiagnostics = append(brokenDiagnostics, strings.Trim(external.DiagnosticCode+": "+external.Diagnostic, ": "))
		}
	}
	if service.worker == nil {
		return fmt.Errorf("%w: geometry worker is required to solve sketches", ErrValidation)
	}
	if err := normalizeSketchPointIdentities(sketch); err != nil {
		return err
	}
	input := geometry.SketchModel{}
	for _, entity := range sketch.Entities {
		if entity.Suppressed {
			continue
		}
		switch entity.Kind {
		case "POINT":
			input.Points = append(input.Points, geometry.SketchPoint{ID: entity.ID, X: entity.Point.X, Y: entity.Point.Y, Role: entity.Role})
		case "LINE":
			input.Lines = append(input.Lines, geometry.SketchLine{ID: entity.ID, StartX: entity.Start.X, StartY: entity.Start.Y, EndX: entity.End.X, EndY: entity.End.Y, Role: entity.Role})
		case "CIRCLE":
			input.Circles = append(input.Circles, geometry.SketchCircle{ID: entity.ID, CenterX: entity.Center.X, CenterY: entity.Center.Y, Radius: entity.Radius, Role: entity.Role})
		case "ARC":
			input.Arcs = append(input.Arcs, geometry.SketchArc{ID: entity.ID, CenterX: entity.Center.X, CenterY: entity.Center.Y, Radius: entity.Radius, StartAngle: entity.StartAngle, EndAngle: entity.EndAngle, Role: entity.Role})
		case "ELLIPSE":
			input.Ellipses = append(input.Ellipses, geometry.SketchEllipse{ID: entity.ID, Role: entity.Role, CenterX: entity.Center.X, CenterY: entity.Center.Y, MajorRadius: entity.MajorRadius, MinorRadius: entity.MinorRadius, Rotation: entity.Rotation})
		case "ELLIPTICAL_ARC":
			input.EllipticalArcs = append(input.EllipticalArcs, geometry.SketchEllipticalArc{ID: entity.ID, Role: entity.Role, CenterX: entity.Center.X, CenterY: entity.Center.Y, MajorRadius: entity.MajorRadius, MinorRadius: entity.MinorRadius, Rotation: entity.Rotation, StartAngle: entity.StartAngle, EndAngle: entity.EndAngle})
		case "SPLINE":
			value := geometry.SketchSpline{ID: entity.ID, Degree: entity.Degree, Closed: entity.Closed, Role: entity.Role, Mode: entity.Mode, Knots: entity.Knots, Multiplicities: entity.Multiplicities, Weights: entity.Weights, Periodic: entity.Periodic, ParameterStart: entity.ParameterStart, ParameterEnd: entity.ParameterEnd}
			for _, point := range entity.ControlPoints {
				value.ControlPoints = append(value.ControlPoints, [2]float64{point.X, point.Y})
			}
			for _, p := range entity.Poles {
				value.Poles = append(value.Poles, [2]float64{p.X, p.Y})
			}
			input.Splines = append(input.Splines, value)
		}
	}
	for _, external := range sketch.ExternalGeometry {
		if brokenExternal[external.ID] {
			continue
		}
		snapshot := external.Snapshot
		switch snapshot.Kind {
		case "POINT":
			input.Points = append(input.Points, geometry.SketchPoint{ID: external.ID, X: snapshot.Point.X, Y: snapshot.Point.Y, Role: "CONSTRUCTION"})
			input.Constraints = append(input.Constraints, geometry.SketchConstraint{ID: "external-fixed-" + external.ID, Kind: "FIXED_POINT", Internal: true,
				FixedX: snapshot.Point.X, FixedY: snapshot.Point.Y, References: []geometry.SketchReference{{Target: "ENTITY", EntityID: external.ID, SubElement: "POINT"}}})
		case "LINE":
			input.Lines = append(input.Lines, geometry.SketchLine{ID: external.ID, StartX: snapshot.Start.X, StartY: snapshot.Start.Y,
				EndX: snapshot.End.X, EndY: snapshot.End.Y, Role: "CONSTRUCTION"})
			input.Constraints = append(input.Constraints, geometry.SketchConstraint{ID: "external-fixed-" + external.ID, Kind: "FIXED", Internal: true,
				References: []geometry.SketchReference{{Target: "ENTITY", EntityID: external.ID, SubElement: "WHOLE"}}})
		case "CIRCLE":
			input.Circles = append(input.Circles, geometry.SketchCircle{ID: external.ID, CenterX: snapshot.Center.X, CenterY: snapshot.Center.Y,
				Radius: snapshot.Radius, Role: "CONSTRUCTION"})
			input.Constraints = append(input.Constraints, geometry.SketchConstraint{ID: "external-fixed-" + external.ID, Kind: "FIXED", Internal: true,
				References: []geometry.SketchReference{{Target: "ENTITY", EntityID: external.ID, SubElement: "WHOLE"}}})
		}
	}
	for _, constraint := range sketch.Constraints {
		if constraint.Suppressed || constraint.Reference {
			continue
		}
		affected := false
		for _, reference := range constraint.References {
			if reference.Target == "EXTERNAL" && brokenExternal[reference.EntityID] {
				affected = true
				break
			}
		}
		if affected {
			continue
		}
		value := geometry.SketchConstraint{ID: constraint.ID, Kind: constraint.Kind, Unit: constraint.Unit, Internal: constraint.Internal, SelfMirrorMode: constraint.SelfMirrorMode}
		if constraint.Value != nil {
			value.Value = *constraint.Value
		}
		if constraint.FixedPoint != nil {
			value.FixedX, value.FixedY = constraint.FixedPoint.X, constraint.FixedPoint.Y
		}
		for _, reference := range constraint.References {
			target := reference.Target
			if target == "EXTERNAL" {
				target = "ENTITY"
			}
			value.References = append(value.References, geometry.SketchReference{Target: target, EntityID: reference.EntityID,
				SubElement: reference.SubElement, ControlPointIndex: reference.ControlPointIndex})
		}
		input.Constraints = append(input.Constraints, value)
	}
	if len(input.Points)+len(input.Lines)+len(input.Circles)+len(input.Arcs)+len(input.Splines)+len(input.Ellipses)+len(input.EllipticalArcs) == 0 {
		if len(brokenExternal) > 0 {
			sketch.Solve = SketchSolveState{Status: "UNRESOLVED_EXTERNAL", DefinitionStatus: "UNRESOLVED", DegreesOfFreedom: -1,
				Diagnostic: strings.Join(brokenDiagnostics, "; ")}
		} else {
			sketch.Solve = SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}
		}
		refreshSketchReferenceMeasurements(model)
		return nil
	}
	input.DragTargets = targets
	result, err := service.worker.SolveSketch(ctx, requestID+"/"+model.Features[featureIndex].ID, input)
	if err != nil {
		return err
	}
	if result.Status != geometry.SketchSolveFullyConstrained && result.Status != geometry.SketchSolveUnderConstrained && result.Status != geometry.SketchSolveConflicting && result.Status != geometry.SketchSolveRedundant {
		return fmt.Errorf("%w: sketch solve %s: %s", ErrValidation, result.Status, result.Diagnostic)
	}
	byID := map[string]geometry.SketchPoint{}
	lines := map[string]geometry.SketchLine{}
	circles := map[string]geometry.SketchCircle{}
	arcs := map[string]geometry.SketchArc{}
	splines := map[string]geometry.SketchSpline{}
	ellipses := map[string]geometry.SketchEllipse{}
	ellipticalArcs := map[string]geometry.SketchEllipticalArc{}
	for _, point := range result.Model.Points {
		byID[point.ID] = point
	}
	for _, line := range result.Model.Lines {
		lines[line.ID] = line
	}
	for _, circle := range result.Model.Circles {
		circles[circle.ID] = circle
	}
	for _, arc := range result.Model.Arcs {
		arcs[arc.ID] = arc
	}
	for _, spline := range result.Model.Splines {
		splines[spline.ID] = spline
	}
	for _, e := range result.Model.Ellipses {
		ellipses[e.ID] = e
	}
	for _, e := range result.Model.EllipticalArcs {
		ellipticalArcs[e.ID] = e
	}
	for entityIndex := range sketch.Entities {
		entity := &sketch.Entities[entityIndex]
		if entity.Suppressed {
			continue
		}
		if entity.Kind == "POINT" {
			p := byID[entity.ID]
			entity.Point = &SketchPoint2{p.X, p.Y}
		} else if entity.Kind == "LINE" {
			l := lines[entity.ID]
			entity.Start = &SketchPoint2{l.StartX, l.StartY}
			entity.End = &SketchPoint2{l.EndX, l.EndY}
		} else if entity.Kind == "CIRCLE" {
			circle := circles[entity.ID]
			entity.Center, entity.Radius = &SketchPoint2{circle.CenterX, circle.CenterY}, circle.Radius
		} else if entity.Kind == "ARC" {
			arc := arcs[entity.ID]
			entity.Center, entity.Radius = &SketchPoint2{arc.CenterX, arc.CenterY}, arc.Radius
			entity.StartAngle, entity.EndAngle = arc.StartAngle, arc.EndAngle
		} else if entity.Kind == "ELLIPSE" {
			e, ok := ellipses[entity.ID]
			if !ok {
				return fmt.Errorf("%w: solver omitted ellipse %s", ErrValidation, entity.ID)
			}
			entity.Center = &SketchPoint2{X: e.CenterX, Y: e.CenterY}
			entity.MajorRadius = e.MajorRadius
			entity.MinorRadius = e.MinorRadius
			entity.Rotation = e.Rotation
		} else if entity.Kind == "ELLIPTICAL_ARC" {
			e, ok := ellipticalArcs[entity.ID]
			if !ok {
				return fmt.Errorf("%w: solver omitted elliptical arc %s", ErrValidation, entity.ID)
			}
			entity.Center = &SketchPoint2{X: e.CenterX, Y: e.CenterY}
			entity.MajorRadius = e.MajorRadius
			entity.MinorRadius = e.MinorRadius
			entity.Rotation = e.Rotation
			entity.StartAngle = e.StartAngle
			entity.EndAngle = e.EndAngle
		} else if entity.Kind == "SPLINE" {
			spline := splines[entity.ID]
			entity.ControlPoints = entity.ControlPoints[:0]
			for _, point := range spline.ControlPoints {
				entity.ControlPoints = append(entity.ControlPoints, SketchPoint2{X: point[0], Y: point[1]})
			}
			entity.Degree, entity.Closed = spline.Degree, spline.Closed
			entity.Mode = spline.Mode
			entity.Knots = spline.Knots
			entity.Multiplicities = spline.Multiplicities
			entity.Weights = spline.Weights
			entity.Periodic = spline.Periodic
			entity.ParameterStart = spline.ParameterStart
			entity.ParameterEnd = spline.ParameterEnd
			if entity.Mode != "CONTROL" && len(entity.PoleIDs) != len(spline.Poles) {
				entity.PoleIDs = nil
			}
			entity.Poles = nil
			for _, p := range spline.Poles {
				entity.Poles = append(entity.Poles, SketchPoint2{X: p[0], Y: p[1]})
			}
		}
	}
	if err := normalizeSketchPointIdentities(sketch); err != nil {
		return err
	}
	if err := validateSketchCornerGeometry(sketch); err != nil {
		return err
	}
	components, err := service.solveSketchComponents(ctx, requestID+"/"+model.Features[featureIndex].ID, input)
	if err != nil {
		return err
	}
	status, definition, diagnostic := string(result.Status), sketchDefinitionStatus(result.Status, result.DegreesOfFreedom), result.Diagnostic
	if len(brokenExternal) > 0 {
		status, definition, diagnostic = "UNRESOLVED_EXTERNAL", "UNRESOLVED", strings.Join(brokenDiagnostics, "; ")
	}
	sketch.Solve = SketchSolveState{Status: status, DefinitionStatus: definition, DegreesOfFreedom: result.DegreesOfFreedom, Diagnostic: diagnostic,
		ConflictingConstraintIDs: publicSketchConstraintIDs(result.ConflictingConstraintIDs), RedundantConstraintIDs: publicSketchConstraintIDs(result.RedundantConstraintIDs), Components: components}
	refreshSketchReferenceMeasurements(model)
	return resolveSketchPatternReferences(model, featureIndex)
}

func publicSketchConstraintIDs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !strings.HasPrefix(value, "external-fixed-") {
			result = append(result, value)
		}
	}
	return result
}

func (service *Service) solveSketchComponents(ctx context.Context, requestID string, input geometry.SketchModel) ([]SketchSolveComponent, error) {
	parent := map[string]string{}
	for _, value := range input.Points {
		parent[value.ID] = value.ID
	}
	for _, value := range input.Lines {
		parent[value.ID] = value.ID
	}
	for _, value := range input.Circles {
		parent[value.ID] = value.ID
	}
	for _, value := range input.Arcs {
		parent[value.ID] = value.ID
	}
	for _, value := range input.Splines {
		parent[value.ID] = value.ID
	}
	for _, e := range input.Ellipses {
		parent[e.ID] = e.ID
	}
	for _, e := range input.EllipticalArcs {
		parent[e.ID] = e.ID
	}
	var find func(string) string
	find = func(id string) string {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	union := func(a, b string) {
		a, b = find(a), find(b)
		if a != b {
			parent[b] = a
		}
	}
	for _, constraint := range input.Constraints {
		ids := []string{}
		for _, ref := range constraint.References {
			if _, ok := parent[ref.EntityID]; ok {
				ids = append(ids, ref.EntityID)
			}
		}
		for i := 1; i < len(ids); i++ {
			union(ids[0], ids[i])
		}
	}
	groups := map[string]map[string]bool{}
	for id := range parent {
		root := find(id)
		if groups[root] == nil {
			groups[root] = map[string]bool{}
		}
		groups[root][id] = true
	}
	components := []SketchSolveComponent{}
	for root, ids := range groups {
		model := geometry.SketchModel{}
		for _, v := range input.Points {
			if ids[v.ID] {
				model.Points = append(model.Points, v)
			}
		}
		for _, v := range input.Lines {
			if ids[v.ID] {
				model.Lines = append(model.Lines, v)
			}
		}
		for _, v := range input.Circles {
			if ids[v.ID] {
				model.Circles = append(model.Circles, v)
			}
		}
		for _, v := range input.Arcs {
			if ids[v.ID] {
				model.Arcs = append(model.Arcs, v)
			}
		}
		for _, v := range input.Splines {
			if ids[v.ID] {
				model.Splines = append(model.Splines, v)
			}
		}
		for _, e := range input.Ellipses {
			if ids[e.ID] {
				model.Ellipses = append(model.Ellipses, e)
			}
		}
		for _, e := range input.EllipticalArcs {
			if ids[e.ID] {
				model.EllipticalArcs = append(model.EllipticalArcs, e)
			}
		}
		constraintIDs := []string{}
		for _, v := range input.Constraints {
			belongs := false
			for _, ref := range v.References {
				if ids[ref.EntityID] {
					belongs = true
					break
				}
			}
			if belongs {
				model.Constraints = append(model.Constraints, v)
				if !strings.HasPrefix(v.ID, "external-fixed-") {
					constraintIDs = append(constraintIDs, v.ID)
				}
			}
		}
		result, err := service.worker.SolveSketch(ctx, requestID+"/component/"+root, model)
		if err != nil {
			return nil, err
		}
		entityIDs := make([]string, 0, len(ids))
		for id := range ids {
			entityIDs = append(entityIDs, id)
		}
		sort.Strings(entityIDs)
		sort.Strings(constraintIDs)
		components = append(components, SketchSolveComponent{EntityIDs: entityIDs, ConstraintIDs: constraintIDs, Status: string(result.Status), DefinitionStatus: sketchDefinitionStatus(result.Status, result.DegreesOfFreedom), DegreesOfFreedom: result.DegreesOfFreedom})
	}
	sort.Slice(components, func(i, j int) bool {
		return strings.Join(components[i].EntityIDs, "/") < strings.Join(components[j].EntityIDs, "/")
	})
	return components, nil
}

func sketchDefinitionStatus(status geometry.SketchSolveStatus, degreesOfFreedom int) string {
	if status == geometry.SketchSolveConflicting || status == geometry.SketchSolveInvalid || status == geometry.SketchSolveFailed {
		return "UNRESOLVED"
	}
	if degreesOfFreedom == 0 {
		return "FULLY_CONSTRAINED"
	}
	return "UNDER_CONSTRAINED"
}

func buildProductEvaluation(model ProductModel, revisionID, modelHash string, seeds []modelcore.DependencyKey, prior *modelcore.EvaluationManifest) (*modelcore.DependencyGraph, modelcore.EvaluationManifest, error) {
	groupMembers, groupErr := assemblyGroupMembers(model)
	if groupErr != nil {
		return nil, modelcore.EvaluationManifest{}, groupErr
	}
	instanceIDs, instanceNames := map[string]bool{}, map[string]bool{}
	for _, instance := range model.Instances {
		name := scopedNameKey(instance.Name)
		if instance.ID == "" || instanceIDs[instance.ID] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product contains a duplicate or empty InstanceId", ErrValidation)
		}
		if name == "" || instanceNames[name] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product contains a duplicate or empty sibling InstanceName", ErrValidation)
		}
		instanceIDs[instance.ID], instanceNames[name] = true, true
	}
	nodes := make([]modelcore.DependencyNode, 0, len(model.Instances)+len(model.Constraints)+len(model.Publications)+len(model.ContextBindings))
	edges := make([]modelcore.DependencyEdge, 0, len(model.Constraints)*2)
	for _, instance := range model.Instances {
		data, _ := json.Marshal(instance)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("instance:" + instance.ID), Phase: 1, Type: "PRODUCT_INSTANCE", CanonicalInput: data})
	}
	for _, constraint := range model.Constraints {
		if isAssemblyGroup(constraint) {
			data, _ := json.Marshal(constraint)
			key := modelcore.DependencyKey("assembly-constraint:" + constraint.ID)
			nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: "ASSEMBLY_GROUP", CanonicalInput: data})
			if !constraint.Suppressed {
				for _, id := range groupMembers[constraint.ID] {
					edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + id), Target: key, Kind: modelcore.ReadGeometry})
				}
				for _, member := range constraint.GroupMembers {
					if member.GroupID != "" {
						edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("assembly-constraint:" + member.GroupID), Target: key, Kind: modelcore.ReadValue})
					}
				}
				members := stringSet(groupMembers[constraint.ID])
				for _, internal := range model.Constraints {
					if isAssemblyGroup(internal) || internal.Suppressed {
						continue
					}
					second := ""
					if internal.Second != nil {
						second = internal.Second.InstanceID
					}
					if groupContainsConstraint(members, internal.First.InstanceID, second) {
						edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("assembly-constraint:" + internal.ID), Target: key, Kind: modelcore.ReadValue})
					}
				}
			}
			continue
		}
		if !constraint.Suppressed && (!instanceIDs[constraint.First.InstanceID] || (constraint.Second != nil && !instanceIDs[constraint.Second.InstanceID])) {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: assembly constraint references an unknown instance", ErrValidation)
		}
		data, _ := json.Marshal(constraint)
		key := modelcore.DependencyKey("assembly-constraint:" + constraint.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: "ASSEMBLY_CONSTRAINT", CanonicalInput: data})
		if p := assemblyQuantityParameter(constraint); p != nil {
			raw, _ := json.Marshal(p.Source)
			parameterKey := modelcore.DependencyKey("parameter:" + p.ParameterID)
			nodes = append(nodes, modelcore.DependencyNode{Key: parameterKey, Phase: 1, Type: "PARAMETER", CanonicalInput: raw})
			edges = append(edges, modelcore.DependencyEdge{Source: parameterKey, Target: key, Kind: modelcore.ReadValue})
			if p.Source.Expression != nil {
				for _, read := range p.Source.Expression.Reads {
					edges = append(edges, modelcore.DependencyEdge{Source: read, Target: parameterKey, Kind: modelcore.ReadValue})
				}
			}
		}
		if constraint.Suppressed {
			continue
		}
		edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + constraint.First.InstanceID), Target: key, Kind: "READ_GEOMETRY"})
		if constraint.Second != nil {
			edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + constraint.Second.InstanceID), Target: key, Kind: "READ_GEOMETRY"})
		}
	}
	publicationIDs, publicationNames := map[string]bool{}, map[string]bool{}
	for _, publication := range model.Publications {
		name := scopedNameKey(publication.Name)
		if publication.ID == "" || publicationIDs[publication.ID] || name == "" || publicationNames[name] || len(publication.Target.InstancePath.Segments) == 0 || len(publication.Target.InstancePath.Segments) > instancePathMaxDepth {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product Publication identity or relative occurrence path is invalid", ErrValidation)
		}
		publicationIDs[publication.ID], publicationNames[name] = true, true
		instanceID := publication.Target.InstancePath.Segments[0].InstanceID
		if !instanceIDs[instanceID] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product Publication occurrence is missing", ErrValidation)
		}
		data, _ := json.Marshal(publication)
		key := modelcore.DependencyKey("product-publication:" + publication.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 3, Type: "PRODUCT_PUBLICATION", CanonicalInput: data})
		edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + instanceID), Target: key, Kind: modelcore.ReadGeometry})
	}
	bindingIDs, bindingNames, boundInputs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, binding := range model.ContextBindings {
		name := scopedNameKey(binding.Name)
		if binding.ID == "" || bindingIDs[binding.ID] || name == "" || bindingNames[name] ||
			len(binding.OwningInstancePath.Segments) == 0 || len(binding.SourceInstancePath.Segments) == 0 ||
			len(binding.OwningInstancePath.Segments) > instancePathMaxDepth || len(binding.SourceInstancePath.Segments) > instancePathMaxDepth ||
			binding.ContextInputID == "" || binding.Publication.PublicationID == "" {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: ContextBinding identity, name or endpoint is invalid", ErrValidation)
		}
		endpoint := binding.OwningInstancePath.Canonical + "#" + binding.ContextInputID
		if boundInputs[endpoint] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: ContextInput has more than one Product binding", ErrValidation)
		}
		bindingIDs[binding.ID], bindingNames[name], boundInputs[endpoint] = true, true, true
		ownerRoot, sourceRoot := binding.OwningInstancePath.Segments[0].InstanceID, binding.SourceInstancePath.Segments[0].InstanceID
		if !instanceIDs[ownerRoot] || !instanceIDs[sourceRoot] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: ContextBinding occurrence is missing", ErrValidation)
		}
		data, _ := json.Marshal(binding)
		key := modelcore.DependencyKey("context-binding:" + binding.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: "CONTEXT_BINDING", CanonicalInput: data})
		edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + sourceRoot), Target: key, Kind: modelcore.ReadGeometry})
	}
	graph, err := modelcore.NewDependencyGraph(nodes, edges)
	if err != nil {
		return nil, modelcore.EvaluationManifest{}, err
	}
	evaluator := func(node modelcore.DependencyNode, _ map[modelcore.DependencyKey]modelcore.NodeResult) (string, error) {
		return modelcore.ValueDigest(node.CanonicalInput), nil
	}
	manifest, err := graph.Evaluate(revisionID, modelHash, evaluatorVersion, "units-mm-v1", seeds, prior, evaluator)
	return graph, manifest, err
}

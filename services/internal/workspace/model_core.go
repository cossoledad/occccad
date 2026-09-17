package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

const (
	typeCreateSketch           = "occccad://part/sketch/create"
	typeEditSketch             = "occccad://part/sketch/edit"
	typeCreatePad              = "occccad://part/pad/create"
	typeCreateSolidFeature     = "occccad://part/solid-generator/create"
	typeEditFeature            = "occccad://part/feature/edit"
	typeCreateDatumPlane       = "occccad://part/datum-plane/create"
	typeCreateDatumAxis        = "occccad://part/datum-axis/create"
	typeImportExchange         = "occccad://part/exchange/import"
	typeSetParameterLiteral    = "occccad://parameter/literal/set"
	typeSetParameterExpression = "occccad://parameter/expression/set"
	typeRenameParameter        = "occccad://parameter/key/rename"
	typeInsertInstance         = "occccad://product/instance/insert"
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
		commandHandler{typeCreateSketch, "PART", applyCreateFeature},
		commandHandler{typeEditSketch, "PART", applyEditSketch},
		commandHandler{typeCreatePad, "PART", applyCreateFeature},
		commandHandler{typeCreateSolidFeature, "PART", applyCreateFeature},
		commandHandler{typeEditFeature, "PART", applyEditFeature},
		commandHandler{typeCreateDatumPlane, "PART", applyCreateDatumPlane},
		commandHandler{typeCreateDatumAxis, "PART", applyCreateDatumAxis},
		commandHandler{typeImportExchange, "PART", applyCreateFeature},
		commandHandler{typeSetParameterLiteral, "PART", applyParameterSource},
		commandHandler{typeSetParameterExpression, "PART", applyParameterSource},
		commandHandler{typeRenameParameter, "PART", applyRenameParameter},
		commandHandler{typeInsertInstance, "PRODUCT", applyInsertInstance},
		commandHandler{typeMoveInstance, "PRODUCT", applyMoveInstance},
		commandHandler{typeAddAssemblyConstraint, "PRODUCT", applyAddAssemblyConstraint},
		commandHandler{typeEditAssemblyConstraint, "PRODUCT", applyEditAssemblyConstraint},
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
		for _, dependent := range model.Features {
			if dependent.Profile == payload.TargetID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: cannot delete feature %s while feature %s depends on it", ErrValidation, payload.TargetID, dependent.ID)
			}
			if dependent.Sketch != nil && dependent.Sketch.Support.Type == "PLANAR_FACE" &&
				dependent.Sketch.Support.PersistentSelection != nil &&
				dependent.Sketch.Support.PersistentSelection.Anchor.FeatureID == payload.TargetID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: FAILED_SUPPORT: sketch %s depends on feature %s", ErrValidation, dependent.ID, payload.TargetID)
			}
		}
		before := model.Features[index]
		model.Features = append(model.Features[:index], model.Features[index+1:]...)
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
		changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change},
			[]modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(payload.TargetID)}, beforeParameters, model.Parameters)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
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
	model.Instances = append(model.Instances[:index], model.Instances[index+1:]...)
	change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.TargetID, SlotID: "entity"}, before, nil)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"instance:" + modelcore.DependencyKey(payload.TargetID)}}, nil
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
		if err := applySketchOperations(feature.Sketch, payload.Operations); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		ensureFeatureParameters(&model)
		for _, operation := range payload.Operations {
			if operation.Type != "UPDATE_CONSTRAINT_VALUE" || operation.Value == nil {
				continue
			}
			for parameterIndex := range model.Parameters {
				parameter := &model.Parameters[parameterIndex]
				if parameter.ParameterID != sketchConstraintParameterID(feature.ID, operation.ConstraintID) {
					continue
				}
				unit := parameter.DisplayUnit
				if unit == "" {
					unit, _ = sketchConstraintUnitAndDimension(parameter.Label)
				}
				quantity, quantityErr := modelcore.NewQuantity(*operation.Value, unit)
				if quantityErr != nil {
					return nil, modelcore.ChangeSet{}, quantityErr
				}
				parameter.Source = modelcore.ValueSource{Literal: &quantity}
			}
		}
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}, before, *feature.Sketch)
		changes := []modelcore.ModelChange{change}
		seeds := []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(feature.ID)}
		changes, seeds = appendParameterLifecycleChanges(changes, seeds, beforeParameters, model.Parameters)
		for _, parameter := range model.Parameters {
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

func applySketchOperations(sketch *SketchFeature, operations []SketchOperation) error {
	if len(operations) == 0 {
		return fmt.Errorf("%w: sketch edit requires at least one operation", ErrValidation)
	}
	for _, operation := range operations {
		switch operation.Type {
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
				case "POINT":
					if entity.Kind == "POINT" {
						entity.Point = operation.Point
						found = true
					}
				case "CENTER":
					if entity.Kind == "CIRCLE" || entity.Kind == "ARC" {
						entity.Center = operation.Point
						found = true
					}
				case "CONTROL":
					if entity.Kind == "SPLINE" && operation.ControlPointIndex != nil && *operation.ControlPointIndex >= 0 && *operation.ControlPointIndex < len(entity.ControlPoints) {
						entity.ControlPoints[*operation.ControlPointIndex] = *operation.Point
						found = true
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
		case "UPDATE_CONSTRAINT_PLACEMENT":
			if operation.ConstraintID == "" || operation.LabelPosition == nil ||
				!finite(operation.LabelPosition.X) || !finite(operation.LabelPosition.Y) {
				return fmt.Errorf("%w: UPDATE_CONSTRAINT_PLACEMENT requires a constraint and finite position", ErrValidation)
			}
			found := false
			for index := range sketch.Constraints {
				if sketch.Constraints[index].ID != operation.ConstraintID {
					continue
				}
				if !isDimensionalConstraint(sketch.Constraints[index].Kind) {
					return fmt.Errorf("%w: only dimensional constraints have a placement", ErrValidation)
				}
				sketch.Constraints[index].LabelPosition = operation.LabelPosition
				found = true
				break
			}
			if !found {
				return fmt.Errorf("%w: selected constraint does not exist", ErrValidation)
			}
		case "UPDATE_CONSTRAINT_VALUE":
			if operation.ConstraintID == "" || operation.Value == nil || !positiveFinite(*operation.Value) {
				return fmt.Errorf("%w: UPDATE_CONSTRAINT_VALUE requires a positive finite value", ErrValidation)
			}
			found := false
			for index := range sketch.Constraints {
				if sketch.Constraints[index].ID != operation.ConstraintID {
					continue
				}
				if !isDimensionalConstraint(sketch.Constraints[index].Kind) {
					return fmt.Errorf("%w: only dimensional constraints have editable values", ErrValidation)
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
	return kind == "DISTANCE" || kind == "LENGTH" || kind == "RADIUS" || kind == "DIAMETER" || kind == "ANGLE"
}

func validateSketch(sketch SketchFeature) error {
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
		case "SPLINE":
			if entity.Degree < 2 || entity.Degree > 3 || len(entity.ControlPoints) < int(entity.Degree)+1 {
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
			"CONCENTRIC": 2, "POINT_ON_OBJECT": 2, "MIDPOINT": 2, "SYMMETRY": 3}
		expected, supported := counts[constraint.Kind]
		if !supported || len(constraint.References) != expected {
			return fmt.Errorf("%w: constraint %s has unsupported kind or reference count", ErrValidation, constraint.ID)
		}
		if constraint.Kind == "FIXED_POINT" && (constraint.FixedPoint == nil || !finite(constraint.FixedPoint.X) || !finite(constraint.FixedPoint.Y)) {
			return fmt.Errorf("%w: fixed-point constraint %s requires a finite point", ErrValidation, constraint.ID)
		}
		if constraint.LabelPosition != nil && (!isDimensionalConstraint(constraint.Kind) ||
			!finite(constraint.LabelPosition.X) || !finite(constraint.LabelPosition.Y)) {
			return fmt.Errorf("%w: constraint %s has an invalid dimension placement", ErrValidation, constraint.ID)
		}
		if constraint.Kind == "DISTANCE" || constraint.Kind == "LENGTH" || constraint.Kind == "RADIUS" || constraint.Kind == "DIAMETER" || constraint.Kind == "ANGLE" {
			if constraint.Value == nil || !positiveFinite(*constraint.Value) {
				return fmt.Errorf("%w: dimensional constraint %s requires a positive finite value", ErrValidation, constraint.ID)
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
					"SPLINE": {"START": true, "END": true, "CONTROL": true, "WHOLE": true},
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
			return kind == "LINE" || kind == "ARC" || kind == "SPLINE"
		case "CENTER":
			return kind == "CIRCLE" || kind == "ARC"
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
	curve := func(reference SketchGeometryRef) bool { return line(reference) || circular(reference) }
	refs := constraint.References
	switch constraint.Kind {
	case "COINCIDENT":
		return point(refs[0]) && point(refs[1])
	case "DISTANCE":
		return (point(refs[0]) && (point(refs[1]) || line(refs[1]))) ||
			(line(refs[0]) && point(refs[1]))
	case "PARALLEL", "PERPENDICULAR", "ANGLE":
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
		return circular(refs[0]) && circular(refs[1])
	case "TANGENT":
		return curve(refs[0]) && curve(refs[1]) && !(line(refs[0]) && line(refs[1]))
	case "EQUAL":
		return (line(refs[0]) && line(refs[1])) || (circular(refs[0]) && circular(refs[1]))
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
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	for _, feature := range model.Features {
		if feature.ID == payload.Feature.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate feature identity", ErrValidation)
		}
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
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Feature.ID, SlotID: "entity"}, nil, created)
	changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change},
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
	Length    modelcore.Quantity `json:"length"`
	Operation string             `json:"operation"`
	Reversed  bool               `json:"reversed"`
	Profile   string             `json:"profile"`
}

type editFeaturePayload struct {
	FeatureID             string            `json:"featureId"`
	ExpectedFeatureDigest string            `json:"expectedFeatureDigest"`
	LinearExtrude         linearExtrudeEdit `json:"linearExtrude"`
}

var padLengthSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/feature/linear-extrude", SlotID: "pad.length",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
}

var sketchLengthDimensionSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/sketch/constraint/dimension", SlotID: "sketch.dimension.value",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.LengthDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
}

var sketchAngleDimensionSlot = modelcore.PropertySlotDescriptor{
	OwnerTypeURI: "occccad://part/sketch/constraint/dimension", SlotID: "sketch.dimension.value",
	ValueType: modelcore.ValueQuantity, Dimension: modelcore.AngleDimension,
	AllowedSources: []string{"LITERAL", "EXPRESSION"}, Affects: "GEOMETRY", EvaluatorPhase: 2,
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
		var source modelcore.ValueSource
		parameterID := "parameter:" + feature.ID + ":length"
		for _, parameter := range model.Parameters {
			if parameter.ParameterID == parameterID {
				source = parameter.Source
				break
			}
		}
		value, err := json.Marshal(struct {
			Feature Feature               `json:"feature"`
			Source  modelcore.ValueSource `json:"lengthSource"`
		}{feature, source})
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
	if feature == nil || !isSolidGenerator(feature.Type) || strings.EqualFold(feature.Type, "REVOLVE") {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: only Linear Extrude can be edited", ErrValidation)
	}
	definition := payload.LinearExtrude
	if definition.Profile != feature.Profile || !strings.EqualFold(definition.Operation, feature.Operation) || definition.Reversed != feature.Reversed {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: P1 only permits Linear Extrude length edits", ErrValidation)
	}
	if !definition.Length.Dimension.Equal(modelcore.LengthDimension) || !positiveFinite(definition.Length.SIValue) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Linear Extrude length must be a positive finite length", ErrValidation)
	}
	parameterID := "parameter:" + feature.ID + ":length"
	for index := range model.Parameters {
		parameter := &model.Parameters[index]
		if parameter.ParameterID != parameterID {
			continue
		}
		if parameter.Source.Expression != nil {
			return nil, modelcore.ChangeSet{}, featureEditFailure{code: "FEATURE_LENGTH_EXPRESSION_DRIVEN"}
		}
		before := parameter.Source
		length := definition.Length
		parameter.Source = modelcore.ValueSource{Literal: &length}
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
		before := model.Parameters[index].Source
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
	InstanceID  string     `json:"instanceId"`
	Translation [3]float64 `json:"translation"`
	Rotation    [4]float64 `json:"rotation"`
}

type addAssemblyConstraintPayload struct {
	Constraint AssemblyConstraint `json:"constraint"`
}

type editAssemblyConstraintPayload struct {
	ConstraintID            string               `json:"constraintId"`
	Value                   float64              `json:"value"`
	DirectionRelation       string               `json:"directionRelation"`
	DistanceRelation        string               `json:"distanceRelation"`
	First                   *AssemblyGeometryRef `json:"first,omitempty"`
	Second                  *AssemblyGeometryRef `json:"second,omitempty"`
	AngleReferenceDirection *[3]float64          `json:"angleReferenceDirection,omitempty"`
	FixedPose               *InstancePose        `json:"fixedPose,omitempty"`
}

func validateInstanceConstraintReferences(constraint AssemblyConstraint) error {
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
	if !finite(payload.Value) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint value must be finite", ErrValidation)
	}
	for index := range model.Constraints {
		if model.Constraints[index].ID != payload.ConstraintID {
			continue
		}
		before := model.Constraints[index]
		if before.Kind == "ANGLE" && (payload.Value < 0 || payload.Value > 2*math.Pi) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: assembly angle must be in [0, 2pi]", ErrValidation)
		}
		model.Constraints[index].Value = payload.Value
		model.Constraints[index].DirectionRelation = payload.DirectionRelation
		model.Constraints[index].DistanceRelation = payload.DistanceRelation
		if payload.AngleReferenceDirection != nil {
			model.Constraints[index].AngleReferenceDirection = payload.AngleReferenceDirection
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
		if err := validateInstanceConstraintReferences(model.Constraints[index]); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.ConstraintID, SlotID: "assembly-constraint.entity"}, before, model.Constraints[index])
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(payload.ConstraintID)}}, nil
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
	if err := validateInstanceConstraintReferences(payload.Constraint); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for _, existing := range model.Constraints {
		if existing.ID == payload.Constraint.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate assembly constraint identity", ErrValidation)
		}
	}
	model.Constraints = append(model.Constraints, payload.Constraint)
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
		before := struct{ Mode, Version string }{instance.ReferenceMode, instance.ReferencedVersionID}
		instance.ReferenceMode = payload.Mode
		if payload.Mode == "PINNED" {
			instance.ReferencedVersionID = payload.PinnedVersionID
		}
		instance.ResolvedVersionID = ""
		instance.HeadChanged = false
		after := struct{ Mode, Version string }{instance.ReferenceMode, instance.ReferencedVersionID}
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
	if len(priorManifestJSON) > 0 {
		var manifest modelcore.EvaluationManifest
		if json.Unmarshal(priorManifestJSON, &manifest) == nil {
			prepared.priorManifest = &manifest
		}
	}
	command, payload, err := service.adaptLegacyCommand(ctx, documentID, prepared.documentType, prepared.modelJSON, request)
	if err != nil {
		return prepared, err
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
	finishPrepare := perf.Start(ctx, "command-prepare")
	prepared, err := service.prepareDomainMutation(ctx, documentID, request)
	finishPrepare()
	if err != nil {
		return err
	}
	var storedDigest string
	if err := service.database.QueryRow(ctx, `SELECT request_digest FROM occccad.domain_transactions WHERE workspace_id=$1 AND request_id=$2 AND status='COMMITTED'`, prepared.workspaceID, prepared.requestID).Scan(&storedDigest); err == nil {
		if storedDigest != prepared.requestDigest {
			return fmt.Errorf("%w: IDEMPOTENCY_KEY_REUSED", ErrValidation)
		}
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	finishPromote := perf.Start(ctx, "candidate-promote")
	candidate, promoted := service.interactionCandidates.take(request.PreviewID, documentID, prepared)
	finishPromote()
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
	geometryKey := candidate.geometryKey
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
		if !promoted {
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
		if err := validateAndResolvePartParameters(&model); err != nil {
			return err
		}
		changes = appendEvaluatedSketchChanges(changes, beforeModel, model)
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		if err != nil {
			return err
		}
		if broken, unresolved := firstUnresolvedExternal(model); unresolved {
			geometryKey, revisionState, evaluationStatus = "", "FAILED", "FAILED"
			if broken.DependencySnapshot != nil {
				geometryKey = broken.DependencySnapshot.GeometryKey
			}
		} else if !promoted {
			finishGeometry := perf.Start(ctx, "geometry-evaluate")
			geometryKey, err = service.evaluatePart(ctx, prepared.requestID, model)
			finishGeometry()
			if err != nil {
				return err
			}
		}
	} else {
		var model ProductModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		if !promoted {
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
			if err = service.solveAssembly(ctx, documentID, prepared.requestID, drivenInstanceID, solveIntent, &model, ""); err != nil {
				if prepared.command.TypeURI != typeUpdateReferences {
					finishSolve()
					return err
				}
				var failure *assemblySolveFailure
				if !errors.As(err, &failure) || failure.retryable {
					finishSolve()
					return err
				}
				for index := range model.Constraints {
					if model.Constraints[index].EvaluationStatus != modelcore.AssemblyConstraintBroken {
						model.Constraints[index].EvaluationStatus = modelcore.AssemblyConstraintImpossible
						model.Constraints[index].EvaluationSummary = failure.code + ": " + failure.diagnostic
					}
				}
				err = nil
			}
			finishSolve()
		}
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		var priorProduct ProductModel
		_ = json.Unmarshal(prepared.modelJSON, &priorProduct)
		priorPoses := map[string]InstancePose{}
		for _, instance := range priorProduct.Instances {
			priorPoses[instance.ID] = InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
		}
		for _, instance := range model.Instances {
			pose := InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
			if prior, exists := priorPoses[instance.ID]; exists && prior != pose {
				marker, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: instance.ID, SlotID: "instance.pose"}, nil, nil)
				changes.Changes = append(changes.Changes, marker)
			}
		}
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
	var nullableGeometry any
	if geometryKey != "" {
		nullableGeometry = geometryKey
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, revisionID, documentID, prepared.headRevision, revisionSequence, nextJSON, nullableGeometry, revisionState, auditCommandID, modelHash, dependencyDigest, manifestJSON); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.revision_parents(revision_id,parent_revision_id,ordinal) VALUES($1,$2,0)`, revisionID, prepared.headRevision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,base_revision_id,result_revision_id,committed_at) VALUES($1,$2,$3,$4,$5,$6,'DOMAIN','COMMITTED',$7,$8,now())`, prepared.transactionID, prepared.workspaceID, currentSequence+1, prepared.actorID, prepared.requestID, prepared.requestDigest, prepared.headRevision, revisionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,$4,$5,$6)`, prepared.transactionID, prepared.command.CommandID, prepared.command.TypeURI, prepared.command.SchemaVersion, prepared.command.Payload, payloadDigest); err != nil {
		return err
	}
	writes := make([]string, 0, len(changes.Changes))
	for _, change := range changes.Changes {
		writes = append(writes, change.Target.Key())
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, prepared.transactionID, changesJSON, changes.CanonicalDigest, writes, changes.ImpactSeeds); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,$7,true)`, revisionID, strings.ToLower(prepared.documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest, evaluationStatus); err != nil {
		return err
	}
	for _, edge := range graph.Edges {
		if _, err := tx.Exec(ctx, `INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind); err != nil {
			return err
		}
	}
	eventPayload, _ := json.Marshal(map[string]any{"workspaceId": prepared.workspaceID, "sequence": currentSequence + 1, "revisionId": revisionID, "transactionId": prepared.transactionID, "modelHash": modelHash, "changeDigest": changes.CanonicalDigest})
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, prepared.workspaceID, eventPayload); err != nil {
		return err
	}
	var position int
	if err := tx.QueryRow(ctx, `SELECT coalesce(max(position),-1)+1 FROM occccad.document_history WHERE document_id=$1`, documentID).Scan(&position); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_history(document_id,position,version_id,command_id) VALUES($1,$2,$3,$4)`, documentID, position, revisionID, auditCommandID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_changes(document_id,version_id,command_id,change_type) VALUES($1,$2,$3,$4)`, documentID, revisionID, auditCommandID, request.Type); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE occccad.workspaces SET head_revision_id=$1,head_sequence=$2,updated_at=now() WHERE id=$3`, revisionID, currentSequence+1, prepared.workspaceID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE occccad.documents SET head_version_id=$1,updated_at=now() WHERE id=$2`, revisionID, documentID); err != nil {
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

func restoreMovePreviewOnSolveFailure(typeURI string, solveErr error, baseJSON []byte, model *ProductModel) (bool, error) {
	var failure *assemblySolveFailure
	if typeURI != typeMoveInstance || !errors.As(solveErr, &failure) {
		return false, nil
	}
	if err := json.Unmarshal(baseJSON, model); err != nil {
		return false, err
	}
	return true, nil
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
		constraintLimited := false
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
		warmStartKey := ""
		if request.InteractionID != "" {
			warmStartKey = documentID + "|" + prepared.actorID + "|" + request.InteractionID
		}
		if err = service.solveAssembly(ctx, documentID, "preview/"+prepared.requestID, driven, solveIntent, &model, warmStartKey, &assemblyResult); err != nil {
			restored, restoreErr := restoreMovePreviewOnSolveFailure(prepared.command.TypeURI, err, prepared.modelJSON, &model)
			if restoreErr != nil {
				return CommandPreview{}, restoreErr
			}
			if !restored {
				return CommandPreview{}, err
			}
			constraintLimited = true
			// A manipulator target is an ephemeral preference, not a new hard
			// constraint. Until the solver exposes closest-feasible projection,
			// an unreachable target previews the unchanged authoritative poses.
			nextJSON = prepared.modelJSON
		}
		if !constraintLimited {
			nextJSON, err = json.Marshal(model)
			if err != nil {
				return CommandPreview{}, err
			}
		}
		previewID := newID("preview")
		if !constraintLimited {
			service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
				headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
				payloadDigest: modelcore.ValueDigest(prepared.command.Payload), nextJSON: nextJSON, changes: previewChanges,
				expiresAt: time.Now().Add(interactionCandidateTTL)})
		}
		result := CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence,
			ModelHash: canonicalModelHash(nextJSON), ConstraintLimited: constraintLimited, AssemblyComponents: assemblyResult.Components, AssemblySolverBuild: assemblyResult.SolverBuild}
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
			payloadDigest: modelcore.ValueDigest(prepared.command.Payload), nextJSON: nextJSON, changes: previewChanges,
			expiresAt: time.Now().Add(interactionCandidateTTL)})
		return CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence, ModelHash: modelHash}, nil
	}
	finishGeometry := perf.Start(ctx, "geometry-evaluate")
	geometryKey, err := service.evaluatePart(ctx, "preview/"+prepared.requestID, model)
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
	previewID := newID("preview")
	service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
		headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
		payloadDigest: modelcore.ValueDigest(prepared.command.Payload), nextJSON: nextJSON, geometryKey: geometryKey,
		changes: previewChanges, expiresAt: time.Now().Add(interactionCandidateTTL)})
	return CommandPreview{
		PreviewID: previewID, BaseVersionID: prepared.headRevision,
		BaseSequence: prepared.headSequence, ModelHash: modelHash, Artifact: &artifact,
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
	return nil
}

func (service *Service) solveSketchFeature(ctx context.Context, requestID string, model *PartModel, featureIndex int) error {
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
		case "SPLINE":
			value := geometry.SketchSpline{ID: entity.ID, Degree: entity.Degree, Closed: entity.Closed, Role: entity.Role}
			for _, point := range entity.ControlPoints {
				value.ControlPoints = append(value.ControlPoints, [2]float64{point.X, point.Y})
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
		if constraint.Suppressed {
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
		value := geometry.SketchConstraint{ID: constraint.ID, Kind: constraint.Kind, Unit: constraint.Unit, Internal: constraint.Internal}
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
	if len(input.Points)+len(input.Lines)+len(input.Circles)+len(input.Arcs)+len(input.Splines) == 0 {
		if len(brokenExternal) > 0 {
			sketch.Solve = SketchSolveState{Status: "UNRESOLVED_EXTERNAL", DefinitionStatus: "UNRESOLVED", DegreesOfFreedom: -1,
				Diagnostic: strings.Join(brokenDiagnostics, "; ")}
		} else {
			sketch.Solve = SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}
		}
		return nil
	}
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
		} else if entity.Kind == "SPLINE" {
			spline := splines[entity.ID]
			entity.ControlPoints = entity.ControlPoints[:0]
			for _, point := range spline.ControlPoints {
				entity.ControlPoints = append(entity.ControlPoints, SketchPoint2{X: point[0], Y: point[1]})
			}
			entity.Degree, entity.Closed = spline.Degree, spline.Closed
		}
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
	return nil
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
	instanceIDs, instanceNames := map[string]bool{}, map[string]bool{}
	for _, instance := range model.Instances {
		name := strings.ToLower(strings.TrimSpace(instance.Name))
		if instance.ID == "" || instanceIDs[instance.ID] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product contains a duplicate or empty InstanceId", ErrValidation)
		}
		if name == "" || instanceNames[name] {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: Product contains a duplicate or empty sibling InstanceName", ErrValidation)
		}
		instanceIDs[instance.ID], instanceNames[name] = true, true
	}
	nodes := make([]modelcore.DependencyNode, 0, len(model.Instances)+len(model.Constraints))
	edges := make([]modelcore.DependencyEdge, 0, len(model.Constraints)*2)
	for _, instance := range model.Instances {
		data, _ := json.Marshal(instance)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("instance:" + instance.ID), Phase: 1, Type: "PRODUCT_INSTANCE", CanonicalInput: data})
	}
	for _, constraint := range model.Constraints {
		if !instanceIDs[constraint.First.InstanceID] || (constraint.Second != nil && !instanceIDs[constraint.Second.InstanceID]) {
			return nil, modelcore.EvaluationManifest{}, fmt.Errorf("%w: assembly constraint references an unknown instance", ErrValidation)
		}
		data, _ := json.Marshal(constraint)
		key := modelcore.DependencyKey("assembly-constraint:" + constraint.ID)
		nodes = append(nodes, modelcore.DependencyNode{Key: key, Phase: 2, Type: "ASSEMBLY_CONSTRAINT", CanonicalInput: data})
		edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + constraint.First.InstanceID), Target: key, Kind: "READ_GEOMETRY"})
		if constraint.Second != nil {
			edges = append(edges, modelcore.DependencyEdge{Source: modelcore.DependencyKey("instance:" + constraint.Second.InstanceID), Target: key, Kind: "READ_GEOMETRY"})
		}
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

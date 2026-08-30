package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	typeCreateSketch           = "occccad://part/sketch/create"
	typeEditSketch             = "occccad://part/sketch/edit"
	typeCreatePad              = "occccad://part/pad/create"
	typeCreateSolidFeature     = "occccad://part/solid-generator/create"
	typeCreateDatumPlane       = "occccad://part/datum-plane/create"
	typeCreateDatumAxis        = "occccad://part/datum-axis/create"
	typeImportExchange         = "occccad://part/exchange/import"
	typeSetParameterLiteral    = "occccad://parameter/literal/set"
	typeSetParameterExpression = "occccad://parameter/expression/set"
	typeInsertInstance         = "occccad://product/instance/insert"
	typeMoveInstance           = "occccad://product/instance/move"
	typeSetReferenceMode       = "occccad://product/instance/reference-mode/set"
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
		commandHandler{typeCreateDatumPlane, "PART", applyCreateDatumPlane},
		commandHandler{typeCreateDatumAxis, "PART", applyCreateDatumAxis},
		commandHandler{typeImportExchange, "PART", applyCreateFeature},
		commandHandler{typeSetParameterLiteral, "PART", applyParameterSource},
		commandHandler{typeSetParameterExpression, "PART", applyParameterSource},
		commandHandler{typeInsertInstance, "PRODUCT", applyInsertInstance},
		commandHandler{typeMoveInstance, "PRODUCT", applyMoveInstance},
		commandHandler{typeSetReferenceMode, "PRODUCT", applyReferenceMode},
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
		if err := validateAndResolvePartParameters(&model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.TargetID, SlotID: "entity"}, before, nil)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(payload.TargetID)}}, nil
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
			if err := validateSketch(*feature.Sketch); err != nil {
				return nil, modelcore.ChangeSet{}, err
			}
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}, before, *feature.Sketch)
			next, _ := json.Marshal(model)
			return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(feature.ID)}}, nil
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
	if payload.TargetKind != "INSTANCE" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: node kind %s is protected from deletion", ErrValidation, payload.TargetKind)
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
		before := *feature.Sketch
		if err := applySketchOperations(feature.Sketch, payload.Operations); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		if err := validateSketch(*feature.Sketch); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}, before, *feature.Sketch)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(feature.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
}

func applySketchOperations(sketch *SketchFeature, operations []SketchOperation) error {
	if len(operations) == 0 {
		return fmt.Errorf("%w: sketch edit requires at least one operation", ErrValidation)
	}
	for _, operation := range operations {
		switch operation.Type {
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
	if sketch.SchemaVersion != 1 {
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
		}
		for _, reference := range constraint.References {
			switch reference.Target {
			case "ENTITY":
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
			(reference.Target == "ENTITY" && entityKinds[reference.EntityID] == "LINE" &&
				(reference.SubElement == "DIRECTION" || reference.SubElement == "WHOLE"))
	}
	circular := func(reference SketchGeometryRef) bool {
		kind := entityKinds[reference.EntityID]
		return reference.Target == "ENTITY" && (kind == "CIRCLE" || kind == "ARC") && reference.SubElement == "WHOLE"
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
		return refs[0].Target == "ENTITY" && refs[0].SubElement == "WHOLE"
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
	Feature Feature `json:"feature"`
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
	for _, feature := range model.Features {
		if feature.ID == payload.Feature.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate feature identity", ErrValidation)
		}
	}
	model.Features = append(model.Features, payload.Feature)
	ensureFeatureParameters(&model)
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Feature.ID, SlotID: "entity"}, nil, payload.Feature)
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(payload.Feature.ID)}}
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
	prepared, err := service.prepareDomainMutation(ctx, documentID, request)
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
	nextJSON, changes, err := workspaceCommandRegistry.Apply(prepared.documentType, prepared.modelJSON, prepared.command)
	if err != nil {
		return err
	}
	revisionUUID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	revisionID := revisionUUID.String()
	modelHash := canonicalModelHash(nextJSON)
	geometryKey := ""
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	if prepared.documentType == "PART" {
		var model PartModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		normalizePartModel(&model)
		if err := service.solveSketches(ctx, prepared.requestID, &model); err != nil {
			return err
		}
		if err := validateAndResolvePartParameters(&model); err != nil {
			return err
		}
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		if err != nil {
			return err
		}
		geometryKey, err = service.evaluatePart(ctx, prepared.requestID, model)
		if err != nil {
			return err
		}
	} else {
		var model ProductModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
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
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.document_versions(id,document_id,parent_version_id,sequence,model_json,geometry_key,state,created_by_command_id,model_hash,dependency_snapshot_digest,evaluation_manifest) VALUES($1,$2,$3,$4,$5,$6,'READY',$7,$8,$9,$10)`, revisionID, documentID, prepared.headRevision, revisionSequence, nextJSON, nullableGeometry, auditCommandID, modelHash, dependencyDigest, manifestJSON); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, revisionID, strings.ToLower(prepared.documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest); err != nil {
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

// PreviewCommand runs the normal command adapter, typed handler, sketch solver
// and authoritative Part evaluator without creating a Revision, advancing a
// Workspace or appending history. Geometry artifacts remain content-addressed
// rebuildable cache entries and may therefore be reused by the later commit.
func (service *Service) PreviewCommand(ctx context.Context, documentID string, request CommandRequest) (CommandPreview, error) {
	request.Type = strings.ToUpper(strings.TrimSpace(request.Type))
	if request.Type == "UNDO" || request.Type == "REDO" || request.Type == "RESTORE" {
		return CommandPreview{}, fmt.Errorf("%w: history commands cannot be previewed", ErrValidation)
	}
	prepared, err := service.prepareDomainMutation(ctx, documentID, request)
	if err != nil {
		return CommandPreview{}, err
	}
	if !strings.EqualFold(prepared.documentType, "PART") {
		return CommandPreview{}, fmt.Errorf("%w: command preview currently requires a Part document", ErrValidation)
	}
	nextJSON, _, err := workspaceCommandRegistry.Apply(prepared.documentType, prepared.modelJSON, prepared.command)
	if err != nil {
		return CommandPreview{}, err
	}
	var model PartModel
	if err = json.Unmarshal(nextJSON, &model); err != nil {
		return CommandPreview{}, err
	}
	normalizePartModel(&model)
	if err = service.solveSketches(ctx, "preview/"+prepared.requestID, &model); err != nil {
		return CommandPreview{}, err
	}
	if err = validateAndResolvePartParameters(&model); err != nil {
		return CommandPreview{}, err
	}
	nextJSON, _ = json.Marshal(model)
	modelHash := canonicalModelHash(nextJSON)
	geometryKey, err := service.evaluatePart(ctx, "preview/"+prepared.requestID, model)
	if err != nil {
		return CommandPreview{}, err
	}
	artifact, err := service.loadArtifact(ctx, geometryKey)
	if err != nil {
		return CommandPreview{}, err
	}
	identity := sha256.Sum256([]byte(prepared.headRevision + "|" + modelHash + "|" + geometryKey))
	return CommandPreview{
		PreviewID: "sha256:" + hex.EncodeToString(identity[:]), BaseVersionID: prepared.headRevision,
		BaseSequence: prepared.headSequence, ModelHash: modelHash, Artifact: &artifact,
	}, nil
}

func (service *Service) solveSketches(ctx context.Context, requestID string, model *PartModel) error {
	for featureIndex := range model.Features {
		sketch := model.Features[featureIndex].Sketch
		if sketch == nil || len(sketch.Entities) == 0 {
			continue
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
		for _, constraint := range sketch.Constraints {
			if constraint.Suppressed {
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
				value.References = append(value.References, geometry.SketchReference{Target: reference.Target, EntityID: reference.EntityID,
					SubElement: reference.SubElement, ControlPointIndex: reference.ControlPointIndex})
			}
			input.Constraints = append(input.Constraints, value)
		}
		if len(input.Points)+len(input.Lines)+len(input.Circles)+len(input.Arcs)+len(input.Splines) == 0 {
			sketch.Solve = SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}
			continue
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
		components, err := service.solveSketchComponents(ctx, requestID+"/"+model.Features[featureIndex].ID, input, sketch)
		if err != nil {
			return err
		}
		sketch.Solve = SketchSolveState{Status: string(result.Status), DefinitionStatus: sketchDefinitionStatus(result.Status, result.DegreesOfFreedom), DegreesOfFreedom: result.DegreesOfFreedom, Diagnostic: result.Diagnostic,
			ConflictingConstraintIDs: result.ConflictingConstraintIDs, RedundantConstraintIDs: result.RedundantConstraintIDs, Components: components}
	}
	return nil
}

func (service *Service) solveSketchComponents(ctx context.Context, requestID string, input geometry.SketchModel, sketch *SketchFeature) ([]SketchSolveComponent, error) {
	parent := map[string]string{}
	for _, entity := range sketch.Entities {
		if !entity.Suppressed {
			parent[entity.ID] = entity.ID
		}
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
	for _, constraint := range sketch.Constraints {
		if constraint.Suppressed {
			continue
		}
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
				constraintIDs = append(constraintIDs, v.ID)
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
	nodes := make([]modelcore.DependencyNode, 0, len(model.Instances))
	for _, instance := range model.Instances {
		data, _ := json.Marshal(instance)
		nodes = append(nodes, modelcore.DependencyNode{Key: modelcore.DependencyKey("instance:" + instance.ID), Phase: 1, Type: "PRODUCT_INSTANCE", CanonicalInput: data})
	}
	graph, err := modelcore.NewDependencyGraph(nodes, nil)
	if err != nil {
		return nil, modelcore.EvaluationManifest{}, err
	}
	evaluator := func(node modelcore.DependencyNode, _ map[modelcore.DependencyKey]modelcore.NodeResult) (string, error) {
		return modelcore.ValueDigest(node.CanonicalInput), nil
	}
	manifest, err := graph.Evaluate(revisionID, modelHash, evaluatorVersion, "units-mm-v1", seeds, prior, evaluator)
	return graph, manifest, err
}

func (service *Service) adaptLegacyCommand(ctx context.Context, documentID, documentType string, modelJSON json.RawMessage, request CommandRequest) (string, any, error) {
	switch request.Type {
	case "CREATE_SKETCH":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		plane := strings.ToUpper(request.Plane)
		datumID := strings.TrimSpace(request.DatumPlaneID)
		if datumID == "" {
			if plane != "XY" && plane != "XZ" && plane != "YZ" {
				return "", nil, fmt.Errorf("%w: select a datum plane", ErrValidation)
			}
			datumID = "datum-" + strings.ToLower(plane)
		} else {
			found := false
			for _, datum := range model.DatumPlanes {
				if datum.ID == datumID {
					plane, found = datum.Plane, true
					break
				}
			}
			if !found {
				return "", nil, fmt.Errorf("%w: selected datum plane does not exist", ErrValidation)
			}
		}
		return typeCreateSketch, createFeaturePayload{Feature: Feature{ID: newID("sketch"), Type: "SKETCH", Name: numberedFeatureName(model.Features, "SKETCH", "Sketch"), Plane: plane, Sketch: &SketchFeature{SchemaVersion: 1, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: datumID, Plane: plane}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}}}}, nil
	case "EDIT_SKETCH":
		if documentType != "PART" {
			break
		}
		operations := make([]SketchOperation, 0, len(request.Operations)+12)
		for index, operation := range request.Operations {
			if operation.Type != "ADD_RECTANGLE" {
				operations = append(operations, operation)
				continue
			}
			if operation.First == nil || operation.Second == nil {
				return "", nil, fmt.Errorf("%w: rectangle requires two points", ErrValidation)
			}
			expanded, err := rectangleMacro(request.RequestID+fmt.Sprintf("/%d", index), *operation.First, *operation.Second)
			if err != nil {
				return "", nil, err
			}
			operations = append(operations, expanded...)
			for _, snapped := range []struct {
				suffix    string
				point     SketchPoint2
				reference *SketchGeometryRef
			}{
				{"first", *operation.First, operation.FirstReference},
				{"second", *operation.Second, operation.SecondReference},
			} {
				if snapped.reference == nil {
					continue
				}
				found := false
				for _, candidate := range expanded {
					if candidate.Entity == nil || candidate.Entity.Kind != "LINE" {
						continue
					}
					for _, endpoint := range []struct {
						sub   string
						point *SketchPoint2
					}{{"START", candidate.Entity.Start}, {"END", candidate.Entity.End}} {
						if endpoint.point != nil && endpoint.point.X == snapped.point.X && endpoint.point.Y == snapped.point.Y {
							constraint := SketchConstraint{ID: macroID(request.RequestID+fmt.Sprintf("/%d", index), "snap-"+snapped.suffix), Kind: "COINCIDENT",
								References: []SketchGeometryRef{{Target: "ENTITY", EntityID: candidate.Entity.ID, SubElement: endpoint.sub}, *snapped.reference}}
							operations = append(operations, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &constraint})
							found = true
							break
						}
					}
					if found {
						break
					}
				}
			}
		}
		return typeEditSketch, editSketchPayload{SketchID: request.SketchID, Operations: operations}, nil
	case "PAD_SKETCH":
		if documentType != "PART" {
			break
		}
		if !positiveFinite(request.Length) {
			return "", nil, fmt.Errorf("%w: pad length must be a positive finite value", ErrValidation)
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		found := false
		for _, feature := range model.Features {
			if feature.ID == request.SketchID && strings.Contains(strings.ToUpper(feature.Type), "SKETCH") {
				found = true
			}
		}
		if !found {
			return "", nil, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		return typeCreatePad, createFeaturePayload{Feature: Feature{ID: commandEntityID("extrude", request.RequestID), Type: "PAD", Name: numberedFeatureName(model.Features, "PAD", "Extrude"), Profile: request.SketchID, Length: request.Length, Operation: "ADD"}}, nil
	case "CREATE_SOLID_FEATURE":
		if documentType != "PART" {
			break
		}
		generator := strings.ToUpper(strings.TrimSpace(request.Generator))
		if generator != "LINEAR_EXTRUDE" && generator != "REVOLVE" {
			return "", nil, fmt.Errorf("%w: generator must be LINEAR_EXTRUDE or REVOLVE", ErrValidation)
		}
		operation := strings.ToUpper(strings.TrimSpace(request.Operation))
		if operation != "NEW_BODY" && operation != "ADD" && operation != "REMOVE" && operation != "INTERSECT" {
			return "", nil, fmt.Errorf("%w: invalid BodyOperation %s", ErrValidation, operation)
		}
		if generator == "LINEAR_EXTRUDE" && !positiveFinite(request.Length) {
			return "", nil, fmt.Errorf("%w: extrude length must be a positive finite value", ErrValidation)
		}
		if generator == "REVOLVE" && (!positiveFinite(request.Angle) || request.Angle > 360) {
			return "", nil, fmt.Errorf("%w: revolve angle must be in (0, 360] degrees", ErrValidation)
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		var sketch *Feature
		for index := range model.Features {
			if model.Features[index].ID == request.SketchID && model.Features[index].Sketch != nil {
				sketch = &model.Features[index]
				break
			}
		}
		if sketch == nil {
			return "", nil, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		if generator == "REVOLVE" {
			if _, _, err := resolveRevolveAxis(model, *sketch, request.AxisEntityID); err != nil {
				return "", nil, err
			}
		}
		prefix, label := "extrude", "Extrude"
		if generator == "REVOLVE" {
			prefix, label = "revolve", "Revolve"
		}
		feature := Feature{ID: commandEntityID(prefix, request.RequestID), Type: generator,
			Name: numberedFeatureName(model.Features, generator, label), Profile: request.SketchID,
			Length: request.Length, Angle: request.Angle, Operation: operation,
			AxisEntityID: request.AxisEntityID, Reversed: request.Reversed}
		return typeCreateSolidFeature, createFeaturePayload{Feature: feature}, nil
	case "CREATE_DATUM_PLANE":
		if documentType != "PART" {
			break
		}
		finiteVector := func(value [3]float64) bool { return finite(value[0]) && finite(value[1]) && finite(value[2]) }
		length := func(value [3]float64) float64 {
			return math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
		}
		if !finiteVector(request.Origin) || !finiteVector(request.Normal) || !finiteVector(request.UDirection) ||
			length(request.Normal) < 1e-9 || length(request.UDirection) < 1e-9 {
			return "", nil, fmt.Errorf("%w: datum plane requires finite origin, normal and U direction", ErrValidation)
		}
		dot := request.Normal[0]*request.UDirection[0] + request.Normal[1]*request.UDirection[1] + request.Normal[2]*request.UDirection[2]
		if math.Abs(dot/(length(request.Normal)*length(request.UDirection))) > 1e-8 {
			return "", nil, fmt.Errorf("%w: datum plane U direction must be perpendicular to its normal", ErrValidation)
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = "Plane"
		}
		plane := DatumPlane{ID: commandEntityID("plane", request.RequestID), Name: name, Plane: "CUSTOM",
			Origin: request.Origin, Normal: request.Normal, UDirection: request.UDirection, Size: 180}
		return typeCreateDatumPlane, createDatumPlanePayload{Plane: plane}, nil
	case "CREATE_DATUM_AXIS":
		if documentType != "PART" {
			break
		}
		magnitude := math.Sqrt(request.Direction[0]*request.Direction[0] + request.Direction[1]*request.Direction[1] + request.Direction[2]*request.Direction[2])
		if !finite(request.Origin[0]) || !finite(request.Origin[1]) || !finite(request.Origin[2]) || !finite(magnitude) || magnitude < 1e-9 {
			return "", nil, fmt.Errorf("%w: datum axis requires finite origin and non-zero direction", ErrValidation)
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = "Axis"
		}
		axis := DatumAxis{ID: commandEntityID("axis", request.RequestID), Name: name, Origin: request.Origin,
			Direction: [3]float64{request.Direction[0] / magnitude, request.Direction[1] / magnitude, request.Direction[2] / magnitude}}
		return typeCreateDatumAxis, createDatumAxisPayload{Axis: axis}, nil
	case "IMPORT_EXCHANGE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		if strings.TrimSpace(request.GeometryKey) == "" || len(model.Features) != 0 {
			return "", nil, fmt.Errorf("%w: STEP import requires an empty Part and geometry key", ErrValidation)
		}
		name := strings.TrimSpace(request.FileName)
		if name == "" {
			name = "Imported STEP"
		}
		format := strings.ToUpper(strings.TrimSpace(request.SourceFormat))
		if format != "STEP" && format != "BREP" {
			return "", nil, fmt.Errorf("%w: exchange format must be STEP or BREP", ErrValidation)
		}
		return typeImportExchange, createFeaturePayload{Feature: Feature{ID: newID("import"), Type: "IMPORT_BODY", Name: "Import " + name, GeometryKey: request.GeometryKey, FileName: name, SourceFormat: format}}, nil
	case "SET_PARAMETER_VALUE":
		if documentType != "PART" {
			break
		}
		quantity, err := modelcore.NewQuantity(request.Value, request.Unit)
		if err != nil {
			return "", nil, err
		}
		return typeSetParameterLiteral, parameterSourcePayload{ParameterID: request.ParameterID, Source: modelcore.ValueSource{Literal: &quantity}}, nil
	case "SET_PARAMETER_EXPRESSION":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		names := map[string]modelcore.ParameterBinding{}
		var expected *modelcore.ParameterDefinition
		for index := range model.Parameters {
			parameter := &model.Parameters[index]
			names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
			if parameter.ParameterID == request.ParameterID {
				expected = parameter
			}
		}
		if expected == nil {
			return "", nil, fmt.Errorf("%w: parameter does not exist", ErrValidation)
		}
		expression, err := modelcore.CompileExpression(request.Expression, names, expected.Dimension)
		if err != nil {
			return "", nil, err
		}
		return typeSetParameterExpression, parameterSourcePayload{ParameterID: request.ParameterID, Source: modelcore.ValueSource{Expression: &expression}}, nil
	case "INSERT_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		var referenceID, versionID, name string
		if err := service.database.QueryRow(ctx, `SELECT id::text,head_version_id::text,name FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, request.ReferencedDocumentID).Scan(&referenceID, &versionID, &name); errors.Is(err, pgx.ErrNoRows) {
			return "", nil, fmt.Errorf("%w: referenced document does not exist", ErrValidation)
		} else if err != nil {
			return "", nil, err
		}
		if referenceID == documentID {
			return "", nil, fmt.Errorf("%w: a Product cannot contain itself", ErrValidation)
		}
		var cycle bool
		if err := service.database.QueryRow(ctx, `WITH RECURSIVE graph(document_id) AS (SELECT $1::uuid UNION SELECT pi.referenced_document_id FROM graph g JOIN occccad.documents d ON d.id=g.document_id JOIN occccad.product_instances pi ON pi.product_version_id=d.head_version_id) SELECT EXISTS(SELECT 1 FROM graph WHERE document_id=$2::uuid)`, referenceID, documentID).Scan(&cycle); err != nil {
			return "", nil, err
		}
		if cycle {
			return "", nil, fmt.Errorf("%w: Product reference would create a cycle", ErrValidation)
		}
		instanceName := strings.TrimSpace(request.Name)
		if instanceName == "" {
			instanceName = name
		}
		return typeInsertInstance, insertInstancePayload{Instance: ProductInstance{ID: newID("instance"), Name: instanceName, ReferencedDocumentID: referenceID, ReferencedVersionID: versionID, Translation: request.Translation, ReferenceMode: "FOLLOW_HEAD"}}, nil
	case "MOVE_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		return typeMoveInstance, moveInstancePayload{request.InstanceID, request.Translation}, nil
	case "SET_REFERENCE_MODE":
		if documentType != "PRODUCT" {
			break
		}
		mode := strings.ToUpper(request.ReferenceMode)
		if mode != "FOLLOW_HEAD" && mode != "PINNED" {
			return "", nil, fmt.Errorf("%w: reference mode must be FOLLOW_HEAD or PINNED", ErrValidation)
		}
		var model ProductModel
		_ = json.Unmarshal(modelJSON, &model)
		var referenced string
		for _, instance := range model.Instances {
			if instance.ID == request.InstanceID {
				referenced = instance.ReferencedDocumentID
			}
		}
		if referenced == "" {
			return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
		pinned := ""
		if mode == "PINNED" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, referenced).Scan(&pinned); err != nil {
				return "", nil, err
			}
		}
		return typeSetReferenceMode, referenceModePayload{request.InstanceID, mode, pinned}, nil
	case "DELETE_NODE":
		kind := strings.ToUpper(strings.TrimSpace(request.TargetKind))
		id := strings.TrimSpace(request.TargetID)
		owner := strings.TrimSpace(request.OwnerEntityID)
		if id == "" {
			return "", nil, fmt.Errorf("%w: delete target identity is required", ErrValidation)
		}
		if documentType == "PART" {
			if kind != "FEATURE" && kind != "SKETCH_ENTITY" && kind != "SKETCH_CONSTRAINT" {
				break
			}
			if (kind == "SKETCH_ENTITY" || kind == "SKETCH_CONSTRAINT") && owner == "" {
				return "", nil, fmt.Errorf("%w: sketch child deletion requires its owning sketch", ErrValidation)
			}
			return typeDeletePartNode, deleteNodePayload{TargetKind: kind, TargetID: id, OwnerEntityID: owner}, nil
		}
		if documentType == "PRODUCT" && kind == "INSTANCE" {
			return typeDeleteProductNode, deleteNodePayload{TargetKind: kind, TargetID: id}, nil
		}
	case "DELETE_NODES":
		if len(request.Targets) == 0 {
			return "", nil, fmt.Errorf("%w: delete targets are required", ErrValidation)
		}
		targets := make([]deleteNodePayload, 0, len(request.Targets))
		seen := map[string]bool{}
		valid := true
		for _, requested := range request.Targets {
			kind := strings.ToUpper(strings.TrimSpace(requested.TargetKind))
			id := strings.TrimSpace(requested.TargetID)
			owner := strings.TrimSpace(requested.OwnerEntityID)
			if id == "" {
				return "", nil, fmt.Errorf("%w: delete target identity is required", ErrValidation)
			}
			if documentType == "PART" {
				if kind != "FEATURE" && kind != "SKETCH_ENTITY" && kind != "SKETCH_CONSTRAINT" {
					valid = false
					break
				}
				if (kind == "SKETCH_ENTITY" || kind == "SKETCH_CONSTRAINT") && owner == "" {
					return "", nil, fmt.Errorf("%w: sketch child deletion requires its owning sketch", ErrValidation)
				}
			} else if documentType != "PRODUCT" || kind != "INSTANCE" {
				valid = false
				break
			}
			key := kind + "\x00" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, deleteNodePayload{TargetKind: kind, TargetID: id, OwnerEntityID: owner})
		}
		if !valid || len(targets) == 0 {
			break
		}
		if documentType == "PART" {
			return typeDeletePartNodes, deleteNodesPayload{Targets: targets}, nil
		}
		return typeDeleteProductNodes, deleteNodesPayload{Targets: targets}, nil
	}
	return "", nil, fmt.Errorf("%w: command %s is not valid for a %s", ErrValidation, request.Type, documentType)
}

func macroID(seed, slot string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("occccad/sketch/"+seed+"/"+slot)).String()
}

func commandEntityID(prefix, requestID string) string {
	return prefix + "-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte("occccad/command/"+requestID+"/"+prefix)).String()
}

func rectangleMacro(seed string, first, second SketchPoint2) ([]SketchOperation, error) {
	if !finite(first.X) || !finite(first.Y) || !finite(second.X) || !finite(second.Y) || first.X == second.X || first.Y == second.Y {
		return nil, fmt.Errorf("%w: rectangle corners must define positive area", ErrValidation)
	}
	minX, maxX, minY, maxY := first.X, second.X, first.Y, second.Y
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	lineIDs := []string{macroID(seed, "line-bottom"), macroID(seed, "line-right"), macroID(seed, "line-top"), macroID(seed, "line-left")}
	lines := []SketchEntity{
		{ID: lineIDs[0], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{minX, minY}, End: &SketchPoint2{maxX, minY}},
		{ID: lineIDs[1], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{maxX, minY}, End: &SketchPoint2{maxX, maxY}},
		{ID: lineIDs[2], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{maxX, maxY}, End: &SketchPoint2{minX, maxY}},
		{ID: lineIDs[3], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{minX, maxY}, End: &SketchPoint2{minX, minY}},
	}
	operations := make([]SketchOperation, 0, 12)
	for index := range lines {
		entity := lines[index]
		operations = append(operations, SketchOperation{Type: "ADD_ENTITY", Entity: &entity})
	}
	endpoint := func(id, sub string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	axis := func(target string) SketchGeometryRef {
		return SketchGeometryRef{Target: target, SubElement: "DIRECTION"}
	}
	constraints := []SketchConstraint{
		{ID: macroID(seed, "coincident-0"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[0], "END"), endpoint(lineIDs[1], "START")}},
		{ID: macroID(seed, "coincident-1"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[1], "END"), endpoint(lineIDs[2], "START")}},
		{ID: macroID(seed, "coincident-2"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[2], "END"), endpoint(lineIDs[3], "START")}},
		{ID: macroID(seed, "coincident-3"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[3], "END"), endpoint(lineIDs[0], "START")}},
		{ID: macroID(seed, "parallel-x-0"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[0], "DIRECTION"), axis("SKETCH_X_AXIS")}},
		{ID: macroID(seed, "parallel-y-0"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[1], "DIRECTION"), axis("SKETCH_Y_AXIS")}},
		{ID: macroID(seed, "parallel-x-1"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[2], "DIRECTION"), axis("SKETCH_X_AXIS")}},
		{ID: macroID(seed, "parallel-y-1"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[3], "DIRECTION"), axis("SKETCH_Y_AXIS")}},
	}
	for index := range constraints {
		constraint := constraints[index]
		operations = append(operations, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &constraint})
	}
	return operations, nil
}

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

func persistEvaluationProjection(ctx context.Context, tx pgx.Tx, revisionID, documentType, modelHash, dependencyDigest string, graph *modelcore.DependencyGraph, manifest modelcore.EvaluationManifest) error {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDigest := modelcore.ValueDigest(manifestJSON)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.evaluation_runs(revision_id,capability,evaluator_digest,input_digest,manifest,manifest_digest,status,authoritative) VALUES($1,$2,$3,$4,$5,$6,'SUCCEEDED',true)`, revisionID, strings.ToLower(documentType), evaluatorVersion, modelHash, manifestJSON, manifestDigest); err != nil {
		return err
	}
	for _, edge := range graph.Edges {
		if _, err = tx.Exec(ctx, `INSERT INTO occccad.dependency_edges(revision_id,source_key,target_key,edge_kind) VALUES($1,$2,$3,$4)`, revisionID, edge.Source, edge.Target, edge.Kind); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE occccad.document_versions SET dependency_snapshot_digest=$1,evaluation_manifest=$2 WHERE id=$3`, dependencyDigest, manifestJSON, revisionID)
	return err
}

func persistInitialTransaction(ctx context.Context, tx pgx.Tx, workspaceID, documentID, revisionID, actor, modelHash, typeURI string, modelJSON []byte) error {
	transactionUUID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	transactionID := transactionUUID.String()
	requestID := "initial:" + documentID
	requestDigest := modelcore.ValueDigest(modelJSON)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.domain_transactions(id,workspace_id,sequence,actor_id,request_id,request_digest,kind,status,result_revision_id,committed_at) VALUES($1,$2,1,$3,$4,$5,'CREATE','COMMITTED',$6,now())`, transactionID, workspaceID, actor, requestID, requestDigest, revisionID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.transaction_commands(transaction_id,ordinal,command_id,type_uri,schema_version,payload,payload_digest) VALUES($1,0,$2,$3,1,$4,$5)`, transactionID, newID("command"), typeURI, modelJSON, requestDigest); err != nil {
		return err
	}
	change, err := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: documentID, SlotID: "document.model"}, nil, json.RawMessage(modelJSON))
	if err != nil {
		return err
	}
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"document:" + modelcore.DependencyKey(documentID)}}
	if err = set.Finalize(); err != nil {
		return err
	}
	setJSON, _ := json.Marshal(set)
	if _, err = tx.Exec(ctx, `INSERT INTO occccad.change_sets(transaction_id,canonical_blob,canonical_digest,write_set,impact_seeds) VALUES($1,$2,$3,$4,$5)`, transactionID, setJSON, set.CanonicalDigest, []string{change.Target.Key()}, set.ImpactSeeds); err != nil {
		return err
	}
	event, _ := json.Marshal(map[string]any{"workspaceId": workspaceID, "sequence": 1, "revisionId": revisionID, "transactionId": transactionID, "modelHash": modelHash})
	_, err = tx.Exec(ctx, `INSERT INTO occccad.outbox_events(aggregate_type,aggregate_id,event_type,schema_version,payload) VALUES('WORKSPACE',$1,'workspace.transaction.committed.v1',1,$2)`, workspaceID, event)
	return err
}

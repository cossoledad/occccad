package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

type referencePointPayload struct {
	Point            DatumPoint                       `json:"point"`
	ParameterSources map[string]modelcore.ValueSource `json:"parameterSources,omitempty"`
	Action           string                           `json:"action"`
}

func applyReferencePoint(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var input referencePointPayload
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	index := -1
	for i, p := range model.DatumPoints {
		if p.ID == input.Point.ID {
			index = i
		}
	}
	var before any
	changes := []modelcore.ModelChange{}
	kind := modelcore.ChangeCreate
	if input.Point.ID == "" || input.Point.Construction == nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: point definition is required", ErrValidation)
	}
	if input.Action == "EDIT" {
		if index < 0 {
			return nil, modelcore.ChangeSet{}, ErrNotFound
		}
		before = model.DatumPoints[index]
		input.Point.Visible = model.DatumPoints[index].Visible
		kind = modelcore.ChangeUpdate
	} else if input.Action != "CREATE" || index >= 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: point identity exists", ErrValidation)
	}
	if input.Point.Name == "" {
		names := []string{}
		for _, p := range model.DatumPoints {
			names = append(names, p.Name)
		}
		input.Point.Name = nextScopedName("参考点.", names)
	}
	if validateScopedName(input.Point.Name) != nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid point name", ErrValidation)
	}
	containerChanges, err := referenceContainer(&model, &input.Point.GeometricalSetID, input.Point.ID)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	changes = append(changes, containerChanges...)
	if index < 0 {
		model.DatumPoints = append(model.DatumPoints, input.Point)
	} else {
		model.DatumPoints[index] = input.Point
	}
	ensureFeatureParameters(&model)
	for i := range model.Parameters {
		if model.Parameters[i].OwnerFeatureID == input.Point.ID {
			if source, ok := input.ParameterSources[model.Parameters[i].PropertySlot]; ok {
				model.Parameters[i].Source = source
			}
		}
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, err := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: input.Point.ID, SlotID: "datum.point"}, before, input.Point)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	changes = append(changes, change)
	changes, seeds := appendParameterLifecycleChanges(changes, []modelcore.DependencyKey{modelcore.DependencyKey("datum:" + input.Point.ID)}, beforeParameters, model.Parameters)
	for _, before := range beforeParameters {
		for _, after := range model.Parameters {
			if before.ParameterID == after.ParameterID && resolvedDigest(before.Source) != resolvedDigest(after.Source) {
				change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: after.ParameterID, SlotID: "parameter.source"}, before.Source, after.Source)
				changes = append(changes, change)
				seeds = append(seeds, modelcore.DependencyKey("parameter:"+after.ParameterID))
			}
		}
	}
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}

func deleteReferencePoint(model PartModel, id string) (json.RawMessage, modelcore.ChangeSet, error) {
	index := -1
	for i, p := range model.DatumPoints {
		if p.ID == id {
			index = i
		}
	}
	if index < 0 {
		return nil, modelcore.ChangeSet{}, ErrNotFound
	}
	for _, publication := range model.Publications {
		if publication.Target.DatumID == id {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: reference point is published", ErrValidation)
		}
	}
	for owner, c := range referenceConstructions(&model) {
		if owner == id {
			continue
		}
		for _, ref := range c.Inputs {
			if ref.EntityID == id {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: reference point is used by %s", ErrValidation, owner)
			}
		}
	}
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	point := model.DatumPoints[index]
	model.DatumPoints = append(model.DatumPoints[:index], model.DatumPoints[index+1:]...)
	filtered := model.Parameters[:0]
	for _, p := range model.Parameters {
		if p.OwnerFeatureID != id {
			filtered = append(filtered, p)
		}
	}
	model.Parameters = filtered
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, err := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: id, SlotID: "datum.point"}, point, nil)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change}, nil, beforeParameters, model.Parameters)
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}

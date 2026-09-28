package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

type deleteParameterPayload struct {
	ParameterID string `json:"parameterId"`
}

type createUserParameterPayload struct {
	Parameter modelcore.ParameterDefinition `json:"parameter"`
}

func applyCreateUserParameter(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload createUserParameterPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	parameter := payload.Parameter
	if parameter.ParameterID == "" || !validParameterKey(parameter.Key) || parameter.Lifecycle != "USER" ||
		parameter.OwnerFeatureID != "" || parameter.Source.Literal == nil || parameter.Source.Expression != nil || parameter.Source.External != nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid user parameter definition", ErrValidation)
	}
	for _, current := range model.Parameters {
		if current.ParameterID == parameter.ParameterID || current.Key == parameter.Key {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate parameter identity or alias", ErrValidation)
		}
	}
	model.Parameters = append(model.Parameters, parameter)
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.entity"}, nil, model.Parameters[len(model.Parameters)-1])
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(parameter.ParameterID)}}, nil
}

func applyDeleteParameter(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload deleteParameterPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	index := -1
	for i := range model.Parameters {
		if model.Parameters[i].ParameterID == payload.ParameterID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter does not exist", ErrValidation)
	}
	parameter := model.Parameters[index]
	if parameter.Lifecycle == "SKETCH_DIMENSION" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: sketch dimension parameter must be removed through its constraint", ErrValidation)
	}
	if parameter.Lifecycle != "USER" || parameter.OwnerFeatureID != "" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: required feature parameter cannot be deleted independently", ErrValidation)
	}
	read := modelcore.DependencyKey("parameter:" + payload.ParameterID)
	for _, other := range model.Parameters {
		if other.ParameterID == payload.ParameterID || other.Source.Expression == nil {
			continue
		}
		for _, dependency := range other.Source.Expression.Reads {
			if dependency == read {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter is used by %s", ErrValidation, other.ParameterID)
			}
		}
	}
	for _, publication := range model.Publications {
		if publication.Target.ParameterID == payload.ParameterID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter is used by Publication %s", ErrValidation, publication.ID)
		}
	}
	for _, input := range model.ContextInputs {
		if input.Target.Kind == "PARAMETER" && input.Target.TargetID == payload.ParameterID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: parameter is used by ContextInput %s", ErrValidation, input.ID)
		}
	}
	model.Parameters = append(model.Parameters[:index], model.Parameters[index+1:]...)
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.ParameterID, SlotID: "parameter.entity"}, parameter, nil)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{read}}, nil
}

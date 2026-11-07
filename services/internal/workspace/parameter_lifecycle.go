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
	var payload createUserParameterPayload
	model, err := readParameterDocument(modelJSON)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
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
	if err := model.resolve(); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, _ := json.Marshal(model)
	for _, p := range model.Parameters {
		if p.ParameterID == parameter.ParameterID {
			parameter = p
			break
		}
	}
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.entity"}, nil, parameterHistoryDefinition(parameter))
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"parameter:" + modelcore.DependencyKey(parameter.ParameterID)}}, nil
}

func applyDeleteParameter(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var payload deleteParameterPayload
	model, err := readParameterDocument(modelJSON)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
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
	if err := dependencyBlocked(DependencyObject{Kind: "PARAMETER", ID: parameter.ParameterID, Name: parameter.Key}, model.parameterConsumers(map[modelcore.DependencyKey]bool{read: true})); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}

	model.Parameters = append(model.Parameters[:index], model.Parameters[index+1:]...)
	if err := model.resolve(); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: payload.ParameterID, SlotID: "parameter.entity"}, parameterHistoryDefinition(parameter), nil)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{read}}, nil
}

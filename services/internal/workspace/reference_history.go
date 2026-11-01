package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

func referenceObjectHistory(model *PartModel, address modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	if address.SlotID != "geometrical-set.entity" && address.SlotID != "datum.point" {
		return nil, false, nil
	}
	if address.SlotID == "geometrical-set.entity" {
		index := -1
		for i, p := range model.GeometricalSets {
			if p.ID == address.EntityID {
				index = i
			}
		}
		if !write {
			if index < 0 {
				return nil, true, nil
			}
			raw, err := json.Marshal(model.GeometricalSets[index])
			return raw, true, err
		}
		if len(value) == 0 || string(value) == "null" {
			if index >= 0 {
				model.GeometricalSets = append(model.GeometricalSets[:index], model.GeometricalSets[index+1:]...)
			}
			return nil, true, nil
		}
		var p GeometricalSet
		if err := json.Unmarshal(value, &p); err != nil {
			return nil, true, err
		}
		if p.ID != address.EntityID {
			return nil, true, fmt.Errorf("%w: container history identity", ErrValidation)
		}
		if index < 0 {
			model.GeometricalSets = append(model.GeometricalSets, p)
		} else {
			model.GeometricalSets[index] = p
		}
		return nil, true, nil
	}
	index := -1
	for i, p := range model.DatumPoints {
		if p.ID == address.EntityID {
			index = i
		}
	}
	if !write {
		if index < 0 {
			return nil, true, nil
		}
		raw, err := json.Marshal(model.DatumPoints[index])
		return raw, true, err
	}
	if len(value) == 0 || string(value) == "null" {
		if index >= 0 {
			model.DatumPoints = append(model.DatumPoints[:index], model.DatumPoints[index+1:]...)
		}
		return nil, true, nil
	}
	var p DatumPoint
	if err := json.Unmarshal(value, &p); err != nil {
		return nil, true, err
	}
	if p.ID != address.EntityID {
		return nil, true, fmt.Errorf("%w: point history identity", ErrValidation)
	}
	if index < 0 {
		model.DatumPoints = append(model.DatumPoints, p)
	} else {
		model.DatumPoints[index] = p
	}
	return nil, true, nil
}

package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

type definitionVisibilityPayload struct {
	EntityKind    string `json:"entityKind"`
	EntityID      string `json:"entityId"`
	OwnerEntityID string `json:"ownerEntityId,omitempty"`
	Visible       bool   `json:"visible"`
}

func applyDefinitionVisibility(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var input definitionVisibilityPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	kind := strings.ToUpper(input.EntityKind)
	var before, after any
	address := modelcore.PropertyAddress{}
	var found bool
	switch kind {
	case "BODY":
		for i := range model.Bodies {
			if model.Bodies[i].ID == input.EntityID {
				before, found = bodyDefinition(model.Bodies[i]), true
				model.Bodies[i].Visible = input.Visible
				after = bodyDefinition(model.Bodies[i])
				address = modelcore.PropertyAddress{EntityID: input.EntityID, SlotID: "body.entity"}
				break
			}
		}
	case "SKETCH":
		for i := range model.Features {
			if model.Features[i].ID == input.EntityID && model.Features[i].Sketch != nil {
				before, found = model.Features[i], true
				model.Features[i].Visible = &input.Visible
				after = model.Features[i]
				address = modelcore.PropertyAddress{EntityID: input.EntityID, SlotID: "entity"}
				break
			}
		}
	case "SKETCH_ENTITY":
		for i := range model.Features {
			if model.Features[i].ID == input.OwnerEntityID && model.Features[i].Sketch != nil {
				for j := range model.Features[i].Sketch.Entities {
					if model.Features[i].Sketch.Entities[j].ID == input.EntityID {
						entity := &model.Features[i].Sketch.Entities[j]
						priorJSON, _ := json.Marshal(model.Features[i])
						var prior Feature
						_ = json.Unmarshal(priorJSON, &prior)
						before, found = prior, true
						entity.Visible = &input.Visible
						after = model.Features[i]
						address = modelcore.PropertyAddress{EntityID: input.OwnerEntityID, SlotID: "entity"}
						break
					}
				}
			}
		}
	default:
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: %s has no independent display result", ErrValidation, kind)
	}
	if !found {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: display target does not exist", ErrValidation)
	}
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, before, after)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, nil
}

func visibleOrDefault(value *bool) bool { return value == nil || *value }

func definitionVisibilityTargetExists(model PartModel, kind, id, ownerID string) bool {
	switch strings.ToUpper(kind) {
	case "BODY":
		return bodyIndex(model, id) >= 0
	case "SKETCH":
		for _, feature := range model.Features {
			if feature.ID == id && feature.Sketch != nil {
				return true
			}
		}
	case "SKETCH_ENTITY":
		for _, feature := range model.Features {
			if feature.ID == ownerID && feature.Sketch != nil {
				for _, entity := range feature.Sketch.Entities {
					if entity.ID == id {
						return true
					}
				}
			}
		}
	}
	return false
}

type occurrenceVisibilityPayload struct {
	InstancePath InstancePath `json:"instancePath"`
	EntityKind   string       `json:"entityKind"`
	EntityID     string       `json:"entityId"`
	Mode         string       `json:"mode"`
}

func applyOccurrenceVisibility(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var input occurrenceVisibilityPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if input.InstancePath.Canonical == "" || input.EntityID == "" ||
		(input.Mode != "INHERIT" && input.Mode != "SHOW" && input.Mode != "HIDE") {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid occurrence display address or mode", ErrValidation)
	}
	index := -1
	for i, current := range model.VisibilityOverrides {
		if current.InstancePath.Canonical == input.InstancePath.Canonical && current.EntityKind == input.EntityKind && current.EntityID == input.EntityID {
			index = i
			break
		}
	}
	if input.Mode == "INHERIT" {
		if index >= 0 {
			model.VisibilityOverrides = append(model.VisibilityOverrides[:index], model.VisibilityOverrides[index+1:]...)
		}
	} else {
		value := OccurrenceVisibility{InstancePath: input.InstancePath, EntityKind: input.EntityKind, EntityID: input.EntityID, Mode: input.Mode}
		if index >= 0 {
			model.VisibilityOverrides[index] = value
		} else {
			model.VisibilityOverrides = append(model.VisibilityOverrides, value)
		}
	}
	next, _ := json.Marshal(model)
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "product", SlotID: "document.model"}, json.RawMessage(modelJSON), json.RawMessage(next))
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, nil
}

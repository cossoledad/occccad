package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

type definitionVisibilityPayload struct {
	Axis          string `json:"axis,omitempty"`
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
	case "ORIGIN", "PLANE", "DATUM_AXIS", "AXIS_SYSTEM", "DATUM_POINT", "AXIS":
		slot := "display." + kind
		if kind == "AXIS" {
			slot += "." + input.Axis
		}
		address = modelcore.PropertyAddress{EntityID: input.EntityID, SlotID: slot}
		value, exists := partReferenceVisibility(&model, address, nil, false)
		if !exists {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: display target does not exist", ErrValidation)
		}
		before, found = value, true
		partReferenceVisibility(&model, address, &input.Visible, true)
		after = &input.Visible
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

// Display slots address the existing definition; X/Y/Z are typed child slots of
// an AxisSystem rather than new object IDs or tree paths.
func partReferenceVisibility(model *PartModel, address modelcore.PropertyAddress, value *bool, write bool) (*bool, bool) {
	kind := strings.TrimPrefix(address.SlotID, "display.")
	if kind == "ORIGIN" && address.EntityID == "origin" {
		prior := model.OriginVisible
		if write {
			model.OriginVisible = value
		}
		return prior, true
	}
	for i := range model.DatumPlanes {
		if kind == "PLANE" && model.DatumPlanes[i].ID == address.EntityID {
			prior := model.DatumPlanes[i].Visible
			if write {
				model.DatumPlanes[i].Visible = value
			}
			return prior, true
		}
	}
	for i := range model.DatumAxes {
		if kind == "DATUM_AXIS" && model.DatumAxes[i].ID == address.EntityID {
			prior := model.DatumAxes[i].Visible
			if write {
				model.DatumAxes[i].Visible = value
			}
			return prior, true
		}
	}
	for i := range model.AxisSystems {
		a := &model.AxisSystems[i]
		if a.ID != address.EntityID {
			continue
		}
		switch kind {
		case "AXIS_SYSTEM":
			prior := a.Visible
			if write {
				a.Visible = value
			}
			return prior, true
		case "DATUM_POINT":
			prior := a.PointVisible
			if write {
				a.PointVisible = value
			}
			return prior, true
		case "AXIS.X", "AXIS.Y", "AXIS.Z":
			axis := strings.TrimPrefix(kind, "AXIS.")
			var prior *bool
			if v, ok := a.AxisVisibility[axis]; ok {
				prior = &v
			}
			if write {
				if value == nil {
					delete(a.AxisVisibility, axis)
				} else {
					if a.AxisVisibility == nil {
						a.AxisVisibility = map[string]bool{}
					}
					a.AxisVisibility[axis] = *value
				}
			}
			return prior, true
		}
	}
	return nil, false
}
func applyConstraintVisibility(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var input definitionVisibilityPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	wholeSet := input.EntityKind == "ASSEMBLY_CONSTRAINT_SET" && input.EntityID == "assembly-constraints"
	if input.EntityKind != "ASSEMBLY_CONSTRAINT" && !wholeSet {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: unsupported Product display target", ErrValidation)
	}
	set := modelcore.ChangeSet{}
	for i := range model.Constraints {
		if wholeSet || model.Constraints[i].ID == input.EntityID {
			before := model.Constraints[i]
			model.Constraints[i].Visible = &input.Visible
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: before.ID, SlotID: "assembly-constraint.entity"}, before, model.Constraints[i])
			set.Changes = append(set.Changes, change)
		}
	}
	if len(set.Changes) > 0 {
		next, _ := json.Marshal(model)
		return next, set, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint does not exist", ErrValidation)
}

func boolPointer(value bool) *bool { return &value }
func axisDisplayVisible(axis AxisSystem, direction string) bool {
	value, ok := axis.AxisVisibility[direction]
	return !ok || value
}

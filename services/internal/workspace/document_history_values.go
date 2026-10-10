package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
	"slices"
	"strings"
)

func partHistoryValues(modelJSON json.RawMessage, set modelcore.ChangeSet) (map[modelcore.PropertyAddress]json.RawMessage, error) {
	result := map[modelcore.PropertyAddress]json.RawMessage{}
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	for _, change := range set.Changes {
		if strings.HasPrefix(change.Target.SlotID, "display.") {
			value, exists := partReferenceVisibility(&model, change.Target, nil, false)
			if exists {
				result[change.Target], _ = json.Marshal(value)
			}
			continue
		}
		switch change.Target.SlotID {
		case "body.entity":
			for _, b := range model.Bodies {
				if b.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(bodyDefinition(b))
				}
			}
		case "active-body":
			result[change.Target], _ = json.Marshal(model.ActiveBodyID)

		case "context-input.entity":
			for _, input := range model.ContextInputs {
				if input.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(input)
				}
			}
		case "context-reference.entity":
			for _, reference := range model.ContextReferences {
				if reference.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(reference)
				}
			}
		case "publication.entity":
			for _, publication := range model.Publications {
				if publication.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(publication)
				}
			}
		case "entity":
			for _, feature := range model.Features {
				if feature.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(featureHistoryDefinition(feature))
				}
			}
		case "sketch.model":
			for _, feature := range model.Features {
				if feature.ID == change.Target.EntityID && feature.Sketch != nil {
					result[change.Target], _ = json.Marshal(feature.Sketch)
				}
			}
		case "parameter.source", "sketch.dimension.value":
			for _, parameter := range model.Parameters {
				if parameter.ParameterID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(parameter.Source)
				}
			}
		case "parameter.entity":
			for _, parameter := range model.Parameters {
				if parameter.ParameterID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(parameter)
				}
			}
		case "parameter.key":
			for _, parameter := range model.Parameters {
				if parameter.ParameterID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(parameter.Key)
				}
			}
		case "pad.length":
			parameterID := "parameter:" + change.Target.EntityID + ":length"
			for _, parameter := range model.Parameters {
				if parameter.ParameterID == parameterID {
					result[change.Target], _ = json.Marshal(parameter.Source)
					break
				}
			}
		case "datum.plane":
			for _, plane := range model.DatumPlanes {
				if plane.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(plane)
				}
			}
		case "datum.axis":
			for _, axis := range model.DatumAxes {
				if axis.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(axis)
				}
			}
		case "document.model":
			result[change.Target] = append(json.RawMessage(nil), modelJSON...)
		}
	}
	return result, nil
}
func productHistoryValues(modelJSON json.RawMessage, set modelcore.ChangeSet) (map[modelcore.PropertyAddress]json.RawMessage, error) {
	result := map[modelcore.PropertyAddress]json.RawMessage{}
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	for _, change := range set.Changes {
		if change.Target.SlotID == "product.kinematics" {
			result[change.Target], _ = json.Marshal(model.Kinematics)
			continue
		}
		if change.Target.SlotID == "context-binding.entity" {
			for _, binding := range model.ContextBindings {
				if binding.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(binding)
					break
				}
			}
			continue
		}
		if change.Target.SlotID == "product-publication.entity" {
			for _, publication := range model.Publications {
				if publication.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(publication)
					break
				}
			}
			continue
		}
		if change.Target.SlotID == "assembly-constraint.entity" {
			for _, constraint := range model.Constraints {
				if constraint.ID == change.Target.EntityID {
					result[change.Target], _ = json.Marshal(constraint)
					break
				}
			}
			continue
		}
		for _, instance := range model.Instances {
			if instance.ID != change.Target.EntityID {
				continue
			}
			switch change.Target.SlotID {
			case "entity":
				result[change.Target], _ = json.Marshal(instance)
			case "instance.translation":
				result[change.Target], _ = json.Marshal(instance.Translation)
			case "instance.pose":
				result[change.Target], _ = json.Marshal(InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)})
			case "instance.reference":
				result[change.Target], _ = json.Marshal(struct{ Mode, Version, DocumentID string }{instance.ReferenceMode, instance.ReferencedVersionID, instance.ReferencedDocumentID})
			case "instance.name":
				result[change.Target], _ = json.Marshal(instance.Name)
			}
		}
		if change.Target.SlotID == "document.model" {
			result[change.Target] = append(json.RawMessage(nil), modelJSON...)
		}
	}
	return result, nil
}
func applyPartHistoryValues(modelJSON json.RawMessage, values map[modelcore.PropertyAddress]json.RawMessage) (json.RawMessage, error) {
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	for address, value := range values {
		if strings.HasPrefix(address.SlotID, "display.") {
			var visible *bool
			if err := json.Unmarshal(value, &visible); err != nil {
				return nil, err
			}
			if _, ok := partReferenceVisibility(&model, address, visible, true); !ok {
				return nil, fmt.Errorf("%w: display target deleted", ErrValidation)
			}
			continue
		}
		switch address.SlotID {
		case "active-body":
			if err := json.Unmarshal(value, &model.ActiveBodyID); err != nil {
				return nil, err
			}
		case "body.entity":
			index := bodyIndex(model, address.EntityID)
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.Bodies = append(model.Bodies[:index], model.Bodies[index+1:]...)
				}
			} else {
				var b PartBody
				if err := json.Unmarshal(value, &b); err != nil {
					return nil, err
				}
				if index >= 0 {
					b.GeometryKey = model.Bodies[index].GeometryKey
					model.Bodies[index] = b
				} else {
					model.Bodies = append(model.Bodies, b)
				}
			}

		case "document.model":
			return append(json.RawMessage(nil), value...), nil
		case "publication.entity":
			index := slices.IndexFunc(model.Publications, func(item Publication) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.Publications = append(model.Publications[:index], model.Publications[index+1:]...)
				}
			} else {
				var publication Publication
				if err := json.Unmarshal(value, &publication); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.Publications[index] = publication
				} else {
					model.Publications = append(model.Publications, publication)
				}
			}
		case "context-input.entity":
			index := slices.IndexFunc(model.ContextInputs, func(item ContextInput) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.ContextInputs = append(model.ContextInputs[:index], model.ContextInputs[index+1:]...)
				}
			} else {
				var input ContextInput
				if err := json.Unmarshal(value, &input); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.ContextInputs[index] = input
				} else {
					model.ContextInputs = append(model.ContextInputs, input)
				}
			}
		case "context-reference.entity":
			index := slices.IndexFunc(model.ContextReferences, func(item ContextReference) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.ContextReferences = append(model.ContextReferences[:index], model.ContextReferences[index+1:]...)
				}
			} else {
				var reference ContextReference
				if err := json.Unmarshal(value, &reference); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.ContextReferences[index] = reference
				} else {
					model.ContextReferences = append(model.ContextReferences, reference)
				}
			}
		case "entity":
			index := -1
			for i := range model.Features {
				if model.Features[i].ID == address.EntityID {
					index = i
				}
			}
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					for _, dependent := range model.Features {
						if dependent.Profile == address.EntityID {
							if pending, ok := values[modelcore.PropertyAddress{EntityID: dependent.ID, SlotID: "entity"}]; ok && (len(pending) == 0 || string(pending) == "null") {
								continue
							}
							return nil, fmt.Errorf("%w: cannot remove %s while feature %s depends on it", ErrValidation, address.EntityID, dependent.ID)
						}
					}
					model.Features = append(model.Features[:index], model.Features[index+1:]...)
				}
				filtered := model.Parameters[:0]
				for _, parameter := range model.Parameters {
					if !strings.HasPrefix(parameter.ParameterID, "parameter:"+address.EntityID+":") {
						filtered = append(filtered, parameter)
					}
				}
				model.Parameters = filtered
			} else {
				var feature Feature
				if err := json.Unmarshal(value, &feature); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.Features[index] = feature
				} else {
					model.Features = append(model.Features, feature)
				}
			}
		case "sketch.model":
			index := -1
			for i := range model.Features {
				if model.Features[i].ID == address.EntityID {
					index = i
					break
				}
			}
			if index < 0 || model.Features[index].Type != "SKETCH" || model.Features[index].Sketch == nil {
				return nil, fmt.Errorf("%w: sketch %s was deleted", ErrValidation, address.EntityID)
			}
			if len(value) == 0 || string(value) == "null" {
				return nil, fmt.Errorf("%w: sketch.model cannot be removed independently", ErrValidation)
			}
			var sketch SketchFeature
			if err := json.Unmarshal(value, &sketch); err != nil {
				return nil, err
			}
			model.Features[index].Sketch = &sketch
		case "parameter.source", "sketch.dimension.value":
			for i := range model.Parameters {
				if model.Parameters[i].ParameterID == address.EntityID {
					if err := json.Unmarshal(value, &model.Parameters[i].Source); err != nil {
						return nil, err
					}
				}
			}
		case "parameter.entity":
			index := slices.IndexFunc(model.Parameters, func(item modelcore.ParameterDefinition) bool { return item.ParameterID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.Parameters = append(model.Parameters[:index], model.Parameters[index+1:]...)
				}
			} else {
				var parameter modelcore.ParameterDefinition
				if err := json.Unmarshal(value, &parameter); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.Parameters[index] = parameter
				} else {
					model.Parameters = append(model.Parameters, parameter)
				}
			}
		case "parameter.key":
			found := false
			for i := range model.Parameters {
				if model.Parameters[i].ParameterID == address.EntityID {
					if err := json.Unmarshal(value, &model.Parameters[i].Key); err != nil {
						return nil, err
					}
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%w: parameter was deleted", ErrValidation)
			}
		case "pad.length":
			parameterID := "parameter:" + address.EntityID + ":length"
			found := false
			for i := range model.Parameters {
				if model.Parameters[i].ParameterID == parameterID {
					if err := json.Unmarshal(value, &model.Parameters[i].Source); err != nil {
						return nil, err
					}
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%w: Linear Extrude length parameter was deleted", ErrValidation)
			}
		case "datum.plane":
			index := slices.IndexFunc(model.DatumPlanes, func(item DatumPlane) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.DatumPlanes = append(model.DatumPlanes[:index], model.DatumPlanes[index+1:]...)
				}
			} else {
				var plane DatumPlane
				if err := json.Unmarshal(value, &plane); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.DatumPlanes[index] = plane
				} else {
					model.DatumPlanes = append(model.DatumPlanes, plane)
				}
			}
		case "datum.axis":
			index := slices.IndexFunc(model.DatumAxes, func(item DatumAxis) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.DatumAxes = append(model.DatumAxes[:index], model.DatumAxes[index+1:]...)
				}
			} else {
				var axis DatumAxis
				if err := json.Unmarshal(value, &axis); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.DatumAxes[index] = axis
				} else {
					model.DatumAxes = append(model.DatumAxes, axis)
				}
			}
		}
	}
	slices.SortStableFunc(model.Features, func(a, b Feature) int { return a.Order - b.Order })
	slices.SortStableFunc(model.Bodies, func(a, b PartBody) int { return a.Order - b.Order })
	normalizePartModel(&model)
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, err
	}
	return json.Marshal(model)
}
func applyProductHistoryValues(modelJSON json.RawMessage, values map[modelcore.PropertyAddress]json.RawMessage) (json.RawMessage, error) {

	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	for address, value := range values {
		if address.SlotID == "document.model" {
			return append(json.RawMessage(nil), value...), nil
		}
		index := -1
		for i := range model.Instances {
			if model.Instances[i].ID == address.EntityID {
				index = i
			}
		}
		switch address.SlotID {
		case "product.kinematics":
			model.Kinematics = KinematicsDefinitions{}
			if err := json.Unmarshal(value, &model.Kinematics); err != nil {
				return nil, err
			}
		case "context-binding.entity":
			bindingIndex := slices.IndexFunc(model.ContextBindings, func(item ContextBinding) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if bindingIndex >= 0 {
					model.ContextBindings = append(model.ContextBindings[:bindingIndex], model.ContextBindings[bindingIndex+1:]...)
				}
			} else {
				var binding ContextBinding
				if err := json.Unmarshal(value, &binding); err != nil {
					return nil, err
				}
				if bindingIndex >= 0 {
					model.ContextBindings[bindingIndex] = binding
				} else {
					model.ContextBindings = append(model.ContextBindings, binding)
				}
			}
		case "product-publication.entity":
			publicationIndex := slices.IndexFunc(model.Publications, func(item ProductPublication) bool { return item.ID == address.EntityID })
			if len(value) == 0 || string(value) == "null" {
				if publicationIndex >= 0 {
					model.Publications = append(model.Publications[:publicationIndex], model.Publications[publicationIndex+1:]...)
				}
			} else {
				var publication ProductPublication
				if err := json.Unmarshal(value, &publication); err != nil {
					return nil, err
				}
				if publicationIndex >= 0 {
					model.Publications[publicationIndex] = publication
				} else {
					model.Publications = append(model.Publications, publication)
				}
			}
		case "entity":
			if len(value) == 0 || string(value) == "null" {
				if index >= 0 {
					model.Instances = append(model.Instances[:index], model.Instances[index+1:]...)
				}
			} else {
				var instance ProductInstance
				if err := json.Unmarshal(value, &instance); err != nil {
					return nil, err
				}
				if index >= 0 {
					model.Instances[index] = instance
				} else {
					model.Instances = append(model.Instances, instance)
				}
			}
		case "instance.translation":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			if err := json.Unmarshal(value, &model.Instances[index].Translation); err != nil {
				return nil, err
			}
		case "instance.name":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			if err := json.Unmarshal(value, &model.Instances[index].Name); err != nil {
				return nil, err
			}
		case "instance.reference":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			var reference struct{ Mode, Version, DocumentID string }
			if err := json.Unmarshal(value, &reference); err != nil {
				return nil, err
			}
			model.Instances[index].ReferenceMode = reference.Mode
			model.Instances[index].ReferencedVersionID = reference.Version
			if reference.DocumentID != "" {
				model.Instances[index].ReferencedDocumentID = reference.DocumentID
			}
		case "instance.pose":
			if index < 0 {
				return nil, fmt.Errorf("%w: instance was deleted", ErrValidation)
			}
			var pose InstancePose
			if err := json.Unmarshal(value, &pose); err != nil {
				return nil, err
			}
			model.Instances[index].Translation, model.Instances[index].Rotation = pose.Translation, pose.Rotation
		case "assembly-constraint.entity":
			constraintIndex := -1
			for candidate := range model.Constraints {
				if model.Constraints[candidate].ID == address.EntityID {
					constraintIndex = candidate
					break
				}
			}
			if len(value) == 0 || string(value) == "null" {
				if constraintIndex >= 0 {
					model.Constraints = append(model.Constraints[:constraintIndex], model.Constraints[constraintIndex+1:]...)
				}
			} else {
				var constraint AssemblyConstraint
				if err := json.Unmarshal(value, &constraint); err != nil {
					return nil, err
				}
				if constraintIndex >= 0 {
					model.Constraints[constraintIndex] = constraint
				} else {
					model.Constraints = append(model.Constraints, constraint)
				}
			}
		}
	}
	return json.Marshal(model)
}

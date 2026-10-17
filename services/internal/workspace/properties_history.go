package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

func propertyHistoryValue(target any, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	if target == nil {
		return nil, false, nil
	}
	if write {
		if err := json.Unmarshal(value, target); err != nil {
			return nil, true, err
		}
	}
	raw, err := json.Marshal(target)
	return raw, true, err
}

func partPropertyHistory(model *PartModel, a modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	var field any
	switch a.SlotID {
	case "document.metadata":
		field = &model.Metadata
	case "document.appearance":
		field = &model.DefaultAppearance
	case "body.name":
		for i := range model.Bodies {
			if model.Bodies[i].ID == a.EntityID {
				field = &model.Bodies[i].Name
			}
		}
	case "feature.name":
		for i := range model.Features {
			if model.Features[i].ID == a.EntityID {
				field = &model.Features[i].Name
			}
		}
	case "datum-plane.name":
		for i := range model.DatumPlanes {
			if model.DatumPlanes[i].ID == a.EntityID {
				field = &model.DatumPlanes[i].Name
			}
		}
	case "datum-axis.name":
		for i := range model.DatumAxes {
			if model.DatumAxes[i].ID == a.EntityID {
				field = &model.DatumAxes[i].Name
			}
		}
	default:
		return nil, false, nil
	}
	if field == nil {
		return nil, true, fmt.Errorf("%w: property target deleted", ErrValidation)
	}
	return propertyHistoryValue(field, value, write)
}

func productPropertyHistory(model *ProductModel, a modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	var field any
	switch a.SlotID {
	case "document.metadata":
		field = &model.Metadata
	case "document.appearance":
		field = &model.DefaultAppearance
	case "product.appearance-overrides":
		field = &model.AppearanceOverrides
	default:
		return nil, false, nil
	}
	return propertyHistoryValue(field, value, write)
}

func seedDocumentMetadata(raw json.RawMessage, name, description string) (json.RawMessage, error) {
	var model map[string]json.RawMessage
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, err
	}
	model["metadata"], _ = json.Marshal(DocumentMetadata{Name: name, Description: description})
	return json.Marshal(model)
}

// Restoring geometry/model history keeps the live library title. Metadata has
// its own explicit compensating property actions in the same history log.
func preserveDocumentMetadata(target, current json.RawMessage) (json.RawMessage, error) {
	var to, from map[string]json.RawMessage
	if err := json.Unmarshal(target, &to); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(current, &from); err != nil {
		return nil, err
	}
	if m, ok := from["metadata"]; ok {
		to["metadata"] = m
	} else {
		delete(to, "metadata")
	}
	return json.Marshal(to)
}

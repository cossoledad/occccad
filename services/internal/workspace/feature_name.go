package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

type renameFeaturePayload struct {
	FeatureID string `json:"featureId"`
	Name      string `json:"name"`
}

func applyRenameFeature(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload renameFeaturePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Feature name required", ErrValidation)
	}
	normalizePartModel(&model)
	for i := range model.Features {
		if model.Features[i].ID != payload.FeatureID {
			continue
		}
		before := model.Features[i]
		model.Features[i].Name = name
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate,
			modelcore.PropertyAddress{EntityID: payload.FeatureID, SlotID: "entity"}, before, model.Features[i])
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Feature does not exist", ErrValidation)
}

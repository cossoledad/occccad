package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

func isStandardDatumPlane(id string) bool {
	return id == "datum-xy" || id == "datum-yz" || id == "datum-xz"
}

// Reference geometry deletion shares the ordinary node command and its compensation slots.
func deleteDatum(model PartModel, kind, id string) (json.RawMessage, modelcore.ChangeSet, error) {
	raw, _ := json.Marshal(model)
	return applyPartObjectDeletion(raw, deleteNodesPayload{Targets: []deleteNodePayload{{TargetKind: kind, TargetID: id}}})
}

type featureSuppressionPayload struct {
	FeatureID             string `json:"featureId"`
	ExpectedFeatureDigest string `json:"expectedFeatureDigest"`
	Suppressed            bool   `json:"suppressed"`
}

func applyFeatureSuppression(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload featureSuppressionPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&model)
	digest, err := featureDefinitionDigest(model, payload.FeatureID)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if payload.ExpectedFeatureDigest == "" || payload.ExpectedFeatureDigest != digest {
		return nil, modelcore.ChangeSet{}, featureEditFailure{code: "FEATURE_EDIT_STALE"}
	}
	for i := range model.Features {
		f := &model.Features[i]
		if f.ID != payload.FeatureID {
			continue
		}
		if (!isBodyFeature(f.Type) && f.Type != "SKETCH_PATTERN" && f.Type != "GEOMETRY") || f.Type == "IMPORT_BODY" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: feature does not support suppression", ErrValidation)
		}
		before := featureHistoryDefinition(*f)
		f.Suppressed = payload.Suppressed
		f.EvaluationStatus, f.Diagnostic = "", ""
		normalizeBodies(&model)
		change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: f.ID, SlotID: "entity"}, before, featureHistoryDefinition(*f))
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		next, err := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{modelcore.DependencyKey("feature:" + f.ID)}}, err
	}
	return nil, modelcore.ChangeSet{}, ErrNotFound
}

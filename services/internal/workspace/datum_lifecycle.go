package workspace

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

func isStandardDatumPlane(id string) bool {
	return id == "datum-xy" || id == "datum-yz" || id == "datum-xz"
}

// Reference geometry deletion shares the ordinary node command and its compensation slots.
func deleteDatum(model PartModel, kind, id string) (json.RawMessage, modelcore.ChangeSet, error) {
	for _, publication := range model.Publications {
		if publication.Target.Kind == "DATUM" && publication.Target.DatumID == id {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is published by %s", ErrValidation, publication.ID)
		}
	}
	for _, ref := range model.ContextReferences {
		if ref.LocalTargetID == id {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: detach context reference before deleting its datum", ErrValidation)
		}
	}
	for owner, d := range datumDefinitions(&model) {
		for _, ref := range datumReferences(d) {
			if ref.EntityID == id {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is referenced by %s", ErrValidation, owner)
			}
		}
	}
	for _, f := range model.Features {
		if f.Sketch != nil {
			for _, external := range f.Sketch.ExternalGeometry {
				if external.DatumReference != nil && external.DatumReference.EntityID == id {
					return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is projected by %s", ErrValidation, f.ID)
				}
			}
		}

		definitions := []PatternDefinition{}
		if f.Pattern != nil {
			definitions = append(definitions, f.Pattern.PatternDefinition)
		}
		if f.Sketch != nil {
			for _, p := range f.Sketch.Patterns {
				definitions = append(definitions, p.PatternDefinition)
			}
		}
		for _, p := range definitions {
			if p.MirrorPlaneID == id || p.AxisEntityID == "DATUM_AXIS:"+id || p.CenterReference != nil && p.CenterReference.AxisEntityID == "DATUM_AXIS:"+id {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is used by pattern %s", ErrValidation, p.ID)
			}
		}

		for _, section := range f.Sections {
			if section.Point != nil && section.Point.AxisEntityID == "DATUM_AXIS:"+id {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is used by loft %s", ErrValidation, f.ID)
			}
		}
		if f.NeutralPlaneID == id || f.AxisEntityID == "DATUM_AXIS:"+id || f.Sketch != nil && f.Sketch.Support.DatumPlaneID == id {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum is used by feature %s", ErrValidation, f.ID)
		}
	}
	var before any
	slot := "datum.axis"
	if kind == "DATUM_PLANE" {
		if isStandardDatumPlane(id) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: standard datum planes are protected", ErrValidation)
		}
		index := slices.IndexFunc(model.DatumPlanes, func(p DatumPlane) bool { return p.ID == id })
		if index < 0 {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum plane missing", ErrValidation)
		}
		before, slot = model.DatumPlanes[index], "datum.plane"
		model.DatumPlanes = slices.Delete(model.DatumPlanes, index, index+1)
	} else {
		index := slices.IndexFunc(model.DatumAxes, func(a DatumAxis) bool { return a.ID == id })
		if index < 0 {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum axis missing", ErrValidation)
		}
		before = model.DatumAxes[index]
		model.DatumAxes = slices.Delete(model.DatumAxes, index, index+1)
	}
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	for i := len(model.Parameters) - 1; i >= 0; i-- {
		if model.Parameters[i].OwnerFeatureID == id && model.Parameters[i].Lifecycle == "DATUM_REQUIRED" {
			model.Parameters = append(model.Parameters[:i], model.Parameters[i+1:]...)
		}
	}
	change, err := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: id, SlotID: slot}, before, nil)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, err := json.Marshal(model)
	changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change}, []modelcore.DependencyKey{modelcore.DependencyKey("datum:" + id)}, beforeParameters, model.Parameters)
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
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
		if (!isBodyFeature(f.Type) && f.Type != "SKETCH_PATTERN") || f.Type == "IMPORT_BODY" {
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

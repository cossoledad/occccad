package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"
)

// A linked offset is derived from solved ordinary geometry. Its curves are not
// additional solver variables. Detachment explicitly turns them into editable
// ordinary geometry, retaining their stable identities and connectivity.
type SketchOffset struct {
	ID         string   `json:"id"`
	Name       string   `json:"name,omitempty"`
	EntityIDs  []string `json:"entityIds"`
	Distance   float64  `json:"distance"`
	Join       string   `json:"join"`
	Suppressed bool     `json:"suppressed,omitempty"`
}

func validateSketchOffsets(sketch SketchFeature) error {
	ids, sources := map[string]bool{}, map[string]bool{}
	for _, e := range sketch.Entities {
		sources[e.ID] = true
	}
	budget := 0
	for _, offset := range sketch.Offsets {
		if offset.ID == "" || ids[offset.ID] || len(offset.EntityIDs) == 0 || !finite(offset.Distance) || offset.Distance == 0 || (offset.Join != "MITER" && offset.Join != "ROUND") {
			return fmt.Errorf("%w: invalid linked offset identity, source, distance or join", ErrValidation)
		}
		ids[offset.ID] = true
		for _, p := range sketch.Patterns {
			if p.ID == offset.ID {
				return fmt.Errorf("%w: duplicate derived operation identity", ErrValidation)
			}
		}
		seen := map[string]bool{}
		for _, id := range offset.EntityIDs {
			if !sources[id] || seen[id] {
				return fmt.Errorf("%w: linked offset requires unique ordinary local source geometry", ErrValidation)
			}
			seen[id] = true
		}
		budget += len(offset.EntityIDs) * 2
		if budget > 4096 {
			return fmt.Errorf("%w: linked offset generated geometry budget exceeded", ErrValidation)
		}
	}
	return nil
}

func evaluateSketchOffset(sketch SketchFeature, offset SketchOffset) ([]SketchEntity, []SketchConstraint, error) {
	if offset.Suppressed {
		return nil, nil, nil
	}
	// Use the exact same chain, branch, intersection and radius guards as the
	// independent offset operation. No geometric guessing repairs references.
	copySketch := sketch
	copySketch.Entities = append([]SketchEntity(nil), sketch.Entities...)
	copySketch.Constraints = append([]SketchConstraint(nil), sketch.Constraints...)
	startEntities, startConstraints := len(copySketch.Entities), len(copySketch.Constraints)
	err := applySketchOffsetEdit(&copySketch, SketchOperation{OperationID: offset.ID, EntityIDs: offset.EntityIDs, Value: &offset.Distance, Mode: offset.Join})
	if err != nil {
		return nil, nil, err
	}
	return copySketch.Entities[startEntities:], copySketch.Constraints[startConstraints:], nil
}

func sketchOffsetIssues(sketch SketchFeature) []SketchProfileIssue {
	var issues []SketchProfileIssue
	for _, offset := range sketch.Offsets {
		if _, _, err := evaluateSketchOffset(sketch, offset); err != nil {
			issues = append(issues, SketchProfileIssue{Code: "OFFSET_FAILED", Message: fmt.Sprintf("关联偏移 %s: %v", offset.ID, err), EntityIDs: append([]string(nil), offset.EntityIDs...)})
		}
	}
	return issues
}

func evaluatedSketchEntities(sketch SketchFeature) ([]SketchEntity, error) {
	entities, err := evaluatedSketchPatternEntities(sketch)
	if err != nil {
		return nil, err
	}
	for _, offset := range sketch.Offsets {
		generated, _, failure := evaluateSketchOffset(sketch, offset)
		if failure == nil {
			entities = append(entities, generated...)
		}
	}
	return entities, nil
}

func sketchDerivedConnectivity(sketch SketchFeature) []SketchConstraint {
	var constraints []SketchConstraint
	for _, pattern := range sketch.Patterns {
		constraints = append(constraints, sketchPatternConnectivity(sketch, pattern)...)
	}
	for _, offset := range sketch.Offsets {
		_, joins, err := evaluateSketchOffset(sketch, offset)
		if err == nil {
			constraints = append(constraints, joins...)
		}
	}
	return constraints
}

func applySketchOffsetOperation(sketch *SketchFeature, op SketchOperation) error {
	index := -1
	for i, offset := range sketch.Offsets {
		if offset.ID == op.OffsetID {
			index = i
		}
	}
	switch op.Type {
	case "CREATE_OFFSET":
		if op.Offset == nil {
			return fmt.Errorf("%w: linked offset definition required", ErrValidation)
		}
		for _, offset := range sketch.Offsets {
			if offset.ID == op.Offset.ID {
				return fmt.Errorf("%w: linked offset already exists", ErrValidation)
			}
		}
		sketch.Offsets = append(sketch.Offsets, *op.Offset)
	case "EDIT_OFFSET":
		if index < 0 || op.Offset == nil || op.Offset.ID != op.OffsetID {
			return fmt.Errorf("%w: linked offset identity missing", ErrValidation)
		}
		sketch.Offsets[index] = *op.Offset
	case "DELETE_OFFSET", "DETACH_OFFSET":
		if index < 0 {
			return fmt.Errorf("%w: linked offset missing", ErrValidation)
		}
		if op.Type == "DETACH_OFFSET" {
			if sketch.Offsets[index].Suppressed {
				return fmt.Errorf("%w: unsuppress linked offset before detaching", ErrValidation)
			}
			entities, joins, err := evaluateSketchOffset(*sketch, sketch.Offsets[index])
			if err != nil {
				return err
			}
			for i := range entities {
				entities[i].CreatedByOperationID, entities[i].SourceEntityID = "", ""
			}
			for i := range joins {
				joins[i].Internal = false
			}
			sketch.Entities = append(sketch.Entities, entities...)
			sketch.Constraints = append(sketch.Constraints, joins...)
		}
		sketch.Offsets = append(sketch.Offsets[:index], sketch.Offsets[index+1:]...)
	default:
		return fmt.Errorf("%w: unsupported linked offset operation", ErrValidation)
	}
	if err := validateSketchOffsets(*sketch); err != nil {
		return err
	}
	if op.Offset != nil {
		_, _, err := evaluateSketchOffset(*sketch, *op.Offset)
		return err
	}
	return nil
}

func applySketchOffsetParameterSources(model *PartModel, featureID string, before SketchFeature, operations []SketchOperation) error {
	for _, op := range operations {
		if (op.Type != "CREATE_OFFSET" && op.Type != "EDIT_OFFSET") || op.Offset == nil {
			continue
		}
		for i := range model.Parameters {
			parameter := &model.Parameters[i]
			if parameter.ParameterID != "parameter:"+featureID+":offset:"+op.Offset.ID+":distance" {
				continue
			}
			if op.ParameterSource != nil {
				source, err := compileSketchDimensionSource(*model, *op.ParameterSource, modelcore.LengthDimension)
				if err != nil {
					return err
				}
				parameter.Source = source
			} else {
				unchanged := false
				for _, old := range before.Offsets {
					if old.ID == op.Offset.ID && old.Distance == op.Offset.Distance {
						unchanged = true
					}
				}
				if !unchanged {
					q, _ := modelcore.NewQuantity(op.Offset.Distance, "mm")
					parameter.Source = modelcore.ValueSource{Literal: &q}
				}
			}
		}
	}
	return nil
}

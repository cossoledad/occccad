package workspace

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/modelcore"
)

func applySketchPatternParameterSources(model *PartModel, featureID string, before SketchFeature, operations []SketchOperation) error {
	previous := map[string]map[string]float64{}
	for _, p := range before.Patterns {
		previous[p.ID] = map[string]float64{}
		for _, v := range patternParameters(&p.PatternDefinition) {
			previous[p.ID][v.slot] = v.value
		}
	}
	for _, op := range operations {
		if (op.Type != "CREATE_PATTERN" && op.Type != "EDIT_PATTERN") || op.Pattern == nil {
			continue
		}
		valid := map[string]bool{}
		for _, v := range patternParameters(&op.Pattern.PatternDefinition) {
			valid[v.slot] = true
			id := "parameter:" + featureID + ":pattern:" + op.Pattern.ID + ":" + v.slot
			for i := range model.Parameters {
				p := &model.Parameters[i]
				if p.ParameterID != id {
					continue
				}
				if source, ok := op.PatternParameterSources[v.slot]; ok {
					p.Source = source
				} else if previous[op.Pattern.ID] == nil || previous[op.Pattern.ID][v.slot] != v.value {
					quantity, err := modelcore.NewQuantity(v.value, v.unit)
					if err != nil {
						return err
					}
					p.Source = modelcore.ValueSource{Literal: &quantity}
				}
			}
		}
		for slot := range op.PatternParameterSources {
			if !valid[slot] {
				return fmt.Errorf("%w: unknown pattern parameter %s", ErrValidation, slot)
			}
		}
	}
	return nil
}

type SketchPattern struct {
	PatternDefinition
	EntityIDs []string `json:"entityIds"`
}

func validateSketchPatterns(sketch SketchFeature) error {
	ids, sources := map[string]bool{}, map[string]bool{}
	for _, e := range sketch.Entities {
		sources[e.ID] = true
	}
	owned := map[string]bool{}
	budget := 0
	for _, p := range sketch.Patterns {
		if p.Kind == "MIRROR" {
 return fmt.Errorf("%w: sketch mirror unsupported", ErrValidation)
}
if p.ID == "" || ids[p.ID] || len(p.EntityIDs) == 0 {
			return fmt.Errorf("%w: invalid sketch pattern identity or source", ErrValidation)
		}
		ids[p.ID] = true
		if p.Suppressed {
			continue
		}
		placements, err := patternPlacements(p.PatternDefinition)
		if err != nil {
			return err
		}
		budget += len(placements) * len(p.EntityIDs)
		if budget > 4096 {
			return fmt.Errorf("%w: PATTERN_GEOMETRY_BUDGET", ErrValidation)
		}
		if (p.Kind == "CIRCULAR" && (math.Abs(p.Direction[0])+math.Abs(p.Direction[1]) > 1e-12 || p.Direction[2] == 0)) || (p.Kind == "LINEAR" && math.Abs(p.Direction[2]) > 1e-12) {
			return fmt.Errorf("%w: sketch pattern must preserve its plane", ErrValidation)
		}
		for _, id := range p.EntityIDs {
			if !sources[id] || owned[id] {
				return fmt.Errorf("%w: PATTERN_SOURCE_MISSING_OR_ALREADY_OWNED: %s", ErrValidation, id)
			}
			owned[id] = true
		}
	}
	return nil
}

// Seeds remain the only solver variables. Output geometry is a disposable
// projection, never a collection of independent dimensions or fixed constraints.
func evaluatedSketchPatternEntities(sketch SketchFeature) ([]SketchEntity, error) {
	if err := validateSketchPatterns(sketch); err != nil {
		return nil, err
	}
	sources := map[string]SketchEntity{}
	consumed := map[string]bool{}
	for _, e := range sketch.Entities {
		sources[e.ID] = e
	}
	var generated []SketchEntity
	for _, p := range sketch.Patterns {
		if p.Suppressed {
			continue
		}
		placements, _ := patternPlacements(p.PatternDefinition)
		for _, id := range p.EntityIDs {
			consumed[id] = true
		}
		for _, placement := range placements {
			for _, id := range p.EntityIDs {
				raw, _ := json.Marshal(sources[id])
				var e SketchEntity
				_ = json.Unmarshal(raw, &e)
				e.ID = patternMemberID(p.ID, placement.Slot, id)
				e.CreatedByOperationID, e.SourceEntityID = p.ID, id
				e.StartPointID, e.EndPointID = patternMemberID(p.ID, placement.Slot, e.StartPointID), patternMemberID(p.ID, placement.Slot, e.EndPointID)
				for i, id := range e.ControlPointIDs {
					e.ControlPointIDs[i] = patternMemberID(p.ID, placement.Slot, id)
				}
				for i, id := range e.PoleIDs {
					e.PoleIDs[i] = patternMemberID(p.ID, placement.Slot, id)
				}
				transform := func(v SketchPoint2) SketchPoint2 {
					out := placement.point([3]float64{v.X, v.Y, 0})
					return SketchPoint2{X: out[0], Y: out[1]}
				}
				for _, v := range []*SketchPoint2{e.Point, e.Start, e.End, e.Center} {
					if v != nil {
						*v = transform(*v)
					}
				}
				for i := range e.ControlPoints {
					e.ControlPoints[i] = transform(e.ControlPoints[i])
				}
				for i := range e.Poles {
					e.Poles[i] = transform(e.Poles[i])
				}
				angle := math.Atan2(placement.Matrix[4], placement.Matrix[0])
				if e.Kind == "ARC" {
					e.StartAngle += angle
					e.EndAngle += angle
				}
				e.Rotation += angle
				generated = append(generated, e)
			}
		}
	}
	result := make([]SketchEntity, 0, len(sketch.Entities)+len(generated))
	for _, e := range sketch.Entities {
		if !consumed[e.ID] {
			result = append(result, e)
		}
	}
	return append(result, generated...), nil
}

// Only connectivity is copied into derived profiles or explicitly detached
// members. Driving dimensions remain on the seed while the pattern is linked.
func sketchPatternConnectivity(sketch SketchFeature, pattern SketchPattern) []SketchConstraint {
	if pattern.Suppressed {
		return nil
	}
	selected := map[string]bool{}
	for _, id := range pattern.EntityIDs {
		selected[id] = true
	}
	placements, err := patternPlacements(pattern.PatternDefinition)
	if err != nil {
		return nil
	} // callers validate the complete definition first
	var result []SketchConstraint
	for _, placement := range placements {
		for _, constraint := range sketch.Constraints {
			if constraint.Kind != "COINCIDENT" || len(constraint.References) != 2 || !selected[constraint.References[0].EntityID] || !selected[constraint.References[1].EntityID] {
				continue
			}
			copyConstraint := constraint
			copyConstraint.ID = patternMemberID(pattern.ID, placement.Slot, "constraint:"+constraint.ID)
			copyConstraint.References = append([]SketchGeometryRef(nil), constraint.References...)
			for i := range copyConstraint.References {
				ref := &copyConstraint.References[i]
				ref.EntityID = patternMemberID(pattern.ID, placement.Slot, ref.EntityID)
				if ref.PointID != "" {
					ref.PointID = patternMemberID(pattern.ID, placement.Slot, ref.PointID)
				}
			}
			result = append(result, copyConstraint)
		}
	}
	return result
}

func applySketchPatternOperation(sketch *SketchFeature, op SketchOperation) error {
	index := -1
	for i, p := range sketch.Patterns {
		if p.ID == op.PatternID {
			index = i
		}
	}
	switch op.Type {
	case "CREATE_PATTERN":
		if op.Pattern == nil {
			return fmt.Errorf("%w: pattern definition required", ErrValidation)
		}
		sketch.Patterns = append(sketch.Patterns, *op.Pattern)
	case "EDIT_PATTERN":
		if index < 0 || op.Pattern == nil || op.Pattern.ID != op.PatternID {
			return fmt.Errorf("%w: pattern identity missing", ErrValidation)
		}
		sketch.Patterns[index] = *op.Pattern
	case "DELETE_PATTERN", "DETACH_PATTERN":
		if index < 0 {
			return fmt.Errorf("%w: pattern missing", ErrValidation)
		}
		if op.Type == "DETACH_PATTERN" {
			evaluated, err := evaluatedSketchPatternEntities(*sketch)
			if err != nil {
				return err
			}
			for _, e := range evaluated {
				if e.CreatedByOperationID == op.PatternID {
					e.CreatedByOperationID, e.SourceEntityID = "", ""
					sketch.Entities = append(sketch.Entities, e)
				}
			}
			sketch.Constraints = append(sketch.Constraints, sketchPatternConnectivity(*sketch, sketch.Patterns[index])...)
			// Keep the original solver seed as construction geometry on detachment.
			for i := range sketch.Entities {
				for _, id := range sketch.Patterns[index].EntityIDs {
					if sketch.Entities[i].ID == id {
						sketch.Entities[i].Role = "CONSTRUCTION"
					}
				}
			}
		}
		sketch.Patterns = append(sketch.Patterns[:index], sketch.Patterns[index+1:]...)
	default:
		return fmt.Errorf("%w: unknown pattern operation", ErrValidation)
	}
	return validateSketchPatterns(*sketch)
}

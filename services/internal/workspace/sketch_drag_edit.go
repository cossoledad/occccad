package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
)

func sketchDragDigest(sketch *SketchFeature) string {
	raw, _ := json.Marshal(struct {
		Entities    []SketchEntity
		Constraints []SketchConstraint
		External    []SketchExternalGeometry
	}{sketch.Entities, sketch.Constraints, sketch.ExternalGeometry})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// A drag owns a frozen revision and absolute intent. Targets are solver-only
// objectives: no temporary constraints, coordinate edits or formula changes persist.
func (service *Service) prepareSketchDragEdits(ctx context.Context, raw json.RawMessage, sketchID, requestID string, ops []SketchOperation) ([]SketchOperation, error) {
	hasDrag := false
	for _, op := range ops {
		if op.Type == "APPLY_DRAG_RESULT" {
			return nil, fmt.Errorf("%w: computed drag results cannot be supplied", ErrValidation)
		}
		hasDrag = hasDrag || op.Type == "DRAG_ENTITIES"
	}
	if !hasDrag {
		return ops, nil
	}
	if len(ops) != 1 {
		return nil, fmt.Errorf("%w: a drag must be one atomic operation", ErrValidation)
	}
	op := ops[0]
	if op.OperationID == "" || len(op.EntityIDs) == 0 || op.Copy || len(op.DetachConstraintIDs) != 0 || op.Scale != nil {
		return nil, fmt.Errorf("%w: drag preserves relationships and requires local geometry", ErrValidation)
	}
	var model PartModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, err
	}
	index := -1
	for i, f := range model.Features {
		if f.ID == sketchID && f.Sketch != nil {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("%w: choose a sketch", ErrValidation)
	}
	sketch := model.Features[index].Sketch
	if err := validateSketch(*sketch); err != nil {
		return nil, err
	}
	digest := sketchDragDigest(sketch)
	selected := map[string]bool{}
	for _, id := range op.EntityIDs {
		if id == "" || selected[id] {
			return nil, fmt.Errorf("%w: duplicate drag entity", ErrValidation)
		}
		selected[id] = true
	}
	origin, translation := SketchPoint2{}, SketchPoint2{}
	if op.Origin != nil {
		origin = *op.Origin
	}
	if op.Translation != nil {
		translation = *op.Translation
	}
	if !finite(origin.X) || !finite(origin.Y) || !finite(translation.X) || !finite(translation.Y) || !finite(op.Angle) {
		return nil, fmt.Errorf("%w: invalid drag target", ErrValidation)
	}
	transform := func(p SketchPoint2) SketchPoint2 {
		x, y := p.X-origin.X, p.Y-origin.Y
		c, s := math.Cos(op.Angle), math.Sin(op.Angle)
		return SketchPoint2{X: origin.X + c*x - s*y + translation.X, Y: origin.Y + s*x + c*y + translation.Y}
	}
	targets := []geometry.SketchDragTarget{}
	add := func(id, sub string, p SketchPoint2, index *int) {
		p = transform(p)
		targets = append(targets, geometry.SketchDragTarget{Reference: geometry.SketchReference{Target: "ENTITY", EntityID: id, SubElement: sub, ControlPointIndex: index}, X: p.X, Y: p.Y})
	}
	for _, e := range sketch.Entities {
		if !selected[e.ID] {
			continue
		}
		if e.Suppressed {
			return nil, fmt.Errorf("%w: suppressed geometry cannot be dragged", ErrValidation)
		}
		delete(selected, e.ID)
		switch e.Kind {
		case "POINT":
			add(e.ID, "POINT", *e.Point, nil)
		case "LINE":
			add(e.ID, "START", *e.Start, nil)
			add(e.ID, "END", *e.End, nil)
		case "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC":
			add(e.ID, "CENTER", *e.Center, nil)
			if e.Kind == "ARC" || e.Kind == "ELLIPTICAL_ARC" {
				for _, sub := range []string{"START", "END"} {
					p, ok := constraintReferencePoint(SketchGeometryRef{Target: "ENTITY", EntityID: e.ID, SubElement: sub}, map[string]SketchEntity{e.ID: e})
					if !ok {
						return nil, fmt.Errorf("%w: invalid curve endpoint", ErrValidation)
					}
					add(e.ID, sub, p, nil)
				}
			}
			if e.Kind == "ELLIPSE" || e.Kind == "ELLIPTICAL_ARC" {
				f := math.Sqrt(e.MajorRadius*e.MajorRadius - e.MinorRadius*e.MinorRadius)
				add(e.ID, "DIRECTION", SketchPoint2{X: e.Center.X + f*math.Cos(e.Rotation), Y: e.Center.Y + f*math.Sin(e.Rotation)}, nil)
			}
		case "SPLINE":
			points := e.ControlPoints
			if e.Mode == "CONTROL" {
				points = e.Poles
			}
			for i, p := range points {
				n := i
				add(e.ID, "CONTROL", p, &n)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported drag geometry", ErrValidation)
		}
	}
	if len(selected) != 0 || len(targets) > 4096 {
		return nil, fmt.Errorf("%w: drag targets must be bounded local geometry", ErrValidation)
	}
	if err := service.solveSketchFeature(ctx, requestID+"/drag", &model, index, targets...); err != nil {
		return nil, err
	}
	sketch = model.Features[index].Sketch
	if sketch.Solve.Status == "CONFLICTING" || sketch.Solve.Status == "FAILED" {
		return nil, fmt.Errorf("%w: drag solve %s: %s", ErrValidation, sketch.Solve.Status, sketch.Solve.Diagnostic)
	}
	op.Type = "APPLY_DRAG_RESULT"
	op.CurveSourceDigest = digest
	op.ComputedEntities = sketch.Entities
	return []SketchOperation{op}, nil
}

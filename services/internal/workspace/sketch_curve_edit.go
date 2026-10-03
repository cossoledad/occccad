package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/occccad/occccad/internal/geometry"
)

func sketchCurveDigest(e SketchEntity) string {
	raw, _ := json.Marshal(e)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
func sketchCurveDomain(e SketchEntity) (float64, float64, error) {
	switch e.Kind {
	case "LINE":
		return 0, math.Hypot(e.End.X-e.Start.X, e.End.Y-e.Start.Y), nil
	case "CIRCLE", "ELLIPSE":
		return 0, 2 * math.Pi, nil
	case "ARC", "ELLIPTICAL_ARC":
		return e.StartAngle, e.EndAngle, nil
	case "SPLINE":
		if len(e.Poles) > 0 {
			return e.ParameterStart, e.ParameterEnd, nil
		}
	}
	return 0, 0, fmt.Errorf("%w: curve has no precise parameter domain", ErrValidation)
}
func entityFromProfileCurve(c geometry.ProfileCurve, source SketchEntity) SketchEntity {
	e := source
	e.Kind = c.Kind
	e.Start = nil
	e.End = nil
	e.Center = nil
	e.ControlPoints = nil
	e.Poles = nil
	e.ControlPointIDs = nil
	e.PoleIDs = nil
	e.Radius = c.Radius
	e.MajorRadius = c.MajorRadius
	e.MinorRadius = c.MinorRadius
	e.Rotation = c.Rotation
	e.StartAngle = c.StartAngle
	e.EndAngle = c.EndAngle
	if c.Kind == "LINE" {
		e.Start = &SketchPoint2{X: c.Start[0], Y: c.Start[1]}
		e.End = &SketchPoint2{X: c.End[0], Y: c.End[1]}
	}
	if c.Kind == "ARC" || c.Kind == "CIRCLE" || c.Kind == "ELLIPSE" || c.Kind == "ELLIPTICAL_ARC" {
		e.Center = &SketchPoint2{X: c.Center[0], Y: c.Center[1]}
	}
	e.Mode = c.Mode
	e.Degree = c.Degree
	e.Closed = c.Closed
	e.Knots = c.Knots
	e.Multiplicities = c.Multiplicities
	e.Weights = c.Weights
	e.Periodic = c.Periodic
	e.ParameterStart = c.ParameterStart
	e.ParameterEnd = c.ParameterEnd
	for _, p := range c.ControlPoints {
		e.ControlPoints = append(e.ControlPoints, SketchPoint2{X: p[0], Y: p[1]})
	}
	for _, p := range c.Poles {
		e.Poles = append(e.Poles, SketchPoint2{X: p[0], Y: p[1]})
	}
	return e
}

// Expensive exact geometry runs outside the model transaction on the frozen
// input revision. Only its deterministic, source-bound candidate is dispatched.
func (service *Service) prepareSketchCurveEdits(ctx context.Context, modelJSON json.RawMessage, sketchID, requestID string, operations []SketchOperation) ([]SketchOperation, error) {
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, err
	}
	var cornerErr error
	operations, cornerErr = resolveSketchCornerOperationValues(model, operations)
	if cornerErr != nil {
		return nil, cornerErr
	}
	var sketch *SketchFeature
	for _, f := range model.Features {
		if f.ID == sketchID {
			sketch = f.Sketch
		}
	}
	needs := false
	for _, op := range operations {
		needs = needs || op.Type == "TRIM_ENTITY" || op.Type == "QUICK_TRIM" || op.Type == "SPLIT_ENTITY" || op.Type == "EXTEND_ENTITY" || op.Type == "ARC_COMPLEMENT" || op.Type == "CLOSE_CURVE"
	}
	if !needs {
		for _, op := range operations {
			if op.Type == "REPLACE_CURVE_INTERVALS" {
				return nil, fmt.Errorf("%w: computed curve replacements cannot be supplied by the client", ErrValidation)
			}
		}
		return operations, nil
	}
	if sketch == nil {
		return nil, fmt.Errorf("%w: select an existing sketch", ErrValidation)
	}
	raw, _ := json.Marshal(sketch)
	var staged SketchFeature
	_ = json.Unmarshal(raw, &staged)
	if err := normalizeSketchPointIdentities(&staged); err != nil {
		return nil, err
	}
	prepared := []SketchOperation{}
	for index, op := range operations {
		if op.Type == "REPLACE_CURVE_INTERVALS" {
			return nil, fmt.Errorf("%w: computed curve replacements cannot be supplied by the client", ErrValidation)
		}
		if op.Type == "EXTEND_ENTITY" || op.Type == "ARC_COMPLEMENT" || op.Type == "CLOSE_CURVE" {
			candidate, err := service.prepareSketchExtension(ctx, &staged, op, requestID+fmt.Sprintf("/extend/%d", index))
			if err != nil {
				return nil, err
			}
			if err = applySketchOperations(&staged, []SketchOperation{candidate}); err != nil {
				return nil, err
			}
			prepared = append(prepared, candidate)
			continue
		}
		if op.Type != "TRIM_ENTITY" && op.Type != "QUICK_TRIM" && op.Type != "SPLIT_ENTITY" {
			prepared = append(prepared, op)
			if err := applySketchOperations(&staged, []SketchOperation{op}); err != nil {
				return nil, err
			}
			continue
		}
		if len(op.EntityIDs) == 0 && op.EntityID != "" {
			op.EntityIDs = []string{op.EntityID}
		}
		if len(op.EntityIDs) != 1 || op.OperationID == "" {
			return nil, fmt.Errorf("%w: precise edit requires one local curve and an operation identity", ErrValidation)
		}
		entities := map[string]SketchEntity{}
		for _, e := range staged.Entities {
			entities[e.ID] = e
		}
		source, exists := entities[op.EntityIDs[0]]
		if !exists {
			return nil, fmt.Errorf("%w: external geometry is read only", ErrValidation)
		}
		start, end, err := sketchCurveDomain(source)
		if err != nil {
			return nil, err
		}
		intervals := []SketchCurveInterval{}
		if op.Type == "TRIM_ENTITY" {
			if len(op.Parameters) != 2 || !finite(op.Parameters[0]) || !finite(op.Parameters[1]) || op.Parameters[0] < 0 || op.Parameters[1] > 1 || op.Parameters[0] >= op.Parameters[1] {
				return nil, fmt.Errorf("%w: trim requires an ordered normalized interval", ErrValidation)
			}
			intervals = append(intervals, SketchCurveInterval{Start: op.Parameters[0], End: op.Parameters[1]})
		} else {
			cuts := []float64{0, 1}
			seamIntersection := false
			if op.Type == "SPLIT_ENTITY" {
				cuts = append(cuts, op.Parameters...)
			} else {
				if op.HitParameter == nil || !finite(*op.HitParameter) || *op.HitParameter < 0 || *op.HitParameter > 1 {
					return nil, fmt.Errorf("%w: quick trim requires a finite hit interval", ErrValidation)
				}
				if service.worker == nil {
					return nil, fmt.Errorf("%w: exact curve worker is unavailable", ErrValidation)
				}
				for _, id := range op.BoundaryIDs {
					boundary, ok := entities[id]
					if !ok {
						for _, external := range staged.ExternalGeometry {
							if external.ID == id && external.Status == "CONNECTED" && external.Snapshot != nil {
								s := external.Snapshot
								boundary = SketchEntity{ID: id, Kind: s.Kind, Start: s.Start, End: s.End, Center: s.Center, Radius: s.Radius}
								ok = true
								break
							}
						}
					}
					if !ok || id == source.ID {
						return nil, fmt.Errorf("%w: invalid trim boundary", ErrValidation)
					}
					result, computeErr := service.worker.ComputeSketchCurves(ctx, requestID+fmt.Sprintf("/curve/%d/intersect/%s", index, id), "INTERSECT", []geometry.ProfileCurve{profileCurve(source, false), profileCurve(boundary, false)}, nil, nil)
					if computeErr != nil {
						return nil, fmt.Errorf("%w: %v", ErrValidation, computeErr)
					}
					if len(result.Overlaps) > 0 {
						return nil, fmt.Errorf("%w: overlapping boundaries require an explicit overlap edit", ErrValidation)
					}
					for _, p := range result.Intersections {
						t := (unwrapSketchCurveParameter(source, p.FirstParameter) - start) / (end - start)
						if sketchBoundaryIntersectionEndpoint(source, p.FirstParameter, p.Point) != "" || math.Abs(t) <= 1e-10 || math.Abs(t-1) <= 1e-10 {
							seamIntersection = true
							continue // An endpoint contact is not an interior interval cut.
						}
						if t > 0 && t < 1 {
							sub := sketchBoundaryIntersectionEndpoint(boundary, p.SecondParameter, p.Point)
							if sub != "" {
								target := "ENTITY"
								if _, local := entities[boundary.ID]; !local {
									target = "EXTERNAL"
								}
								op.CutConnections = append(op.CutConnections, SketchCurveCutConnection{Parameter: t, Reference: SketchGeometryRef{Target: target, EntityID: boundary.ID, SubElement: sub}})
							}
						}
					}
					for _, p := range result.Intersections {
						t := (unwrapSketchCurveParameter(source, p.FirstParameter) - start) / (end - start)
						if sketchBoundaryIntersectionEndpoint(source, p.FirstParameter, p.Point) != "" || math.Abs(t) <= 1e-10 || math.Abs(t-1) <= 1e-10 {
							seamIntersection = true
							continue // An endpoint contact is not an interior interval cut.
						}
						if t > 0 && t < 1 {
							cuts = append(cuts, t)
						}
					}
				}
			}
			sort.Float64s(cuts)
			unique := []float64{}
			for _, t := range cuts {
				if !finite(t) || t < 0 || t > 1 {
					return nil, fmt.Errorf("%w: invalid split parameter", ErrValidation)
				}
				if len(unique) == 0 || t != unique[len(unique)-1] {
					unique = append(unique, t)
				}
			}
			if len(unique) < 3 && !(op.Type == "QUICK_TRIM" && op.TrimMode == "DELETE_HIT") {
				return nil, fmt.Errorf("%w: no interior curve intersection or split", ErrValidation)
			}
			cyclicHit := op.Type == "QUICK_TRIM" && !seamIntersection && (source.Kind == "CIRCLE" || source.Kind == "ELLIPSE" || source.Closed) && op.HitParameter != nil && (*op.HitParameter < unique[1] || *op.HitParameter >= unique[len(unique)-2])
			for i := 1; i < len(unique); i++ {
				hit := op.HitParameter != nil && *op.HitParameter >= unique[i-1] && (*op.HitParameter < unique[i] || (i == len(unique)-1 && *op.HitParameter == 1))
				if cyclicHit && (i == 1 || i == len(unique)-1) {
					hit = true
				}
				keep := op.Type == "SPLIT_ENTITY" || op.TrimMode == "BREAK" || (op.TrimMode == "DELETE_HIT" && !hit) || (op.TrimMode == "KEEP_HIT" && hit)
				if keep {
					intervals = append(intervals, SketchCurveInterval{Start: unique[i-1], End: unique[i]})
				}
			}
			if op.Type == "QUICK_TRIM" && op.TrimMode != "BREAK" && op.TrimMode != "DELETE_HIT" && op.TrimMode != "KEEP_HIT" {
				return nil, fmt.Errorf("%w: choose DELETE_HIT, KEEP_HIT or BREAK", ErrValidation)
			}
		}
		// A conic has no topological endpoint at its arbitrary parameter seam.
		// Two interior split picks produce two cyclic arcs, not three pieces.
		if op.Type == "SPLIT_ENTITY" && (source.Kind == "CIRCLE" || source.Kind == "ELLIPSE") && len(intervals) >= 3 && intervals[0].Start == 0 && intervals[len(intervals)-1].End == 1 {
			explicitSeam := false
			for _, cut := range op.Parameters {
				explicitSeam = explicitSeam || cut == 0 || cut == 1
			}
			if !explicitSeam {
				wrap := SketchCurveInterval{Start: intervals[len(intervals)-1].Start, End: intervals[0].End + 1}
				intervals = append(append([]SketchCurveInterval(nil), intervals[1:len(intervals)-1]...), wrap)
			}
		}
		if len(intervals) > 64 {
			return nil, fmt.Errorf("%w: curve edit exceeds segment limit", ErrValidation)
		}
		if service.worker == nil {
			return nil, fmt.Errorf("%w: exact curve worker is unavailable", ErrValidation)
		}
		computed := []SketchEntity{}
		for segment, interval := range intervals {
			a, b := start+(end-start)*interval.Start, start+(end-start)*interval.End
			result, computeErr := service.worker.ComputeSketchCurves(ctx, requestID+fmt.Sprintf("/curve/%d/segment/%d", index, segment), "TRIM", []geometry.ProfileCurve{profileCurve(source, false)}, &a, &b)
			if computeErr != nil || len(result.Curves) != 1 {
				return nil, fmt.Errorf("%w: exact curve trim failed: %v", ErrValidation, computeErr)
			}
			e := entityFromProfileCurve(result.Curves[0], source)
			e.ID = source.ID
			if segment > 0 {
				e.ID = macroID(op.OperationID, fmt.Sprintf("segment/%s/%d", source.ID, segment))
			}
			e.StartPointID = macroID(op.OperationID, fmt.Sprintf("interval/%d/start", segment))
			e.EndPointID = macroID(op.OperationID, fmt.Sprintf("interval/%d/end", segment))
			if interval.Start == 0 && source.StartPointID != "" {
				e.StartPointID = source.StartPointID
			}
			if interval.End == 1 && source.EndPointID != "" {
				e.EndPointID = source.EndPointID
			}
			e.CreatedByOperationID = op.OperationID
			e.SourceEntityID = source.ID
			computed = append(computed, e)
		}
		op.Type = "REPLACE_CURVE_INTERVALS"
		op.CurveSourceDigest = sketchCurveDigest(source)
		op.ComputedEntities = computed
		op.Intervals = intervals
		if err := applySketchOperations(&staged, []SketchOperation{op}); err != nil {
			return nil, err
		}
		prepared = append(prepared, op)
	}
	return prepared, nil
}

func applyComputedSketchIntervals(sketch *SketchFeature, op SketchOperation, entities map[string]SketchEntity, detached map[string]bool) error {
	if len(op.EntityIDs) != 1 || len(op.Intervals) != len(op.ComputedEntities) {
		return fmt.Errorf("%w: malformed precise curve candidate", ErrValidation)
	}
	source := entities[op.EntityIDs[0]]
	if op.CurveSourceDigest != sketchCurveDigest(source) {
		return fmt.Errorf("%w: precise edit source changed", ErrValidation)
	}
	// A formal SAME_SUPPORT sibling is an exact replacement for shared
	// circular center/radius references when an entire split arc is removed.
	// Endpoint or arbitrary whole-curve relations never use this migration.
	sharedCircularSupport := ""
	if len(op.ComputedEntities) == 0 && (source.Kind == "CIRCLE" || source.Kind == "ARC") {
		var siblings []string
		for _, relation := range sketch.Constraints {
			if relation.Kind == "SAME_SUPPORT" && !relation.Suppressed && !detached[relation.ID] && len(relation.References) == 2 {
				for i, r := range relation.References {
					if r.Target == "ENTITY" && r.EntityID == source.ID {
						other := relation.References[1-i]
						candidate := entities[other.EntityID]
						if other.Target == "ENTITY" && (candidate.Kind == "CIRCLE" || candidate.Kind == "ARC") {
							siblings = append(siblings, candidate.ID)
						}
					}
				}
			}
		}
		sort.Strings(siblings)
		if len(siblings) > 0 {
			sharedCircularSupport = siblings[0]
		}
	}
	constraints := []SketchConstraint{}
	for _, c := range sketch.Constraints {
		if len(op.ComputedEntities) == 0 && c.Internal {
			dependent := false
			for _, r := range c.References {
				dependent = dependent || (r.Target == "ENTITY" && r.EntityID == source.ID)
			}
			if dependent {
				continue
			}
		}
		if detached[c.ID] {
			continue
		}
		clone := c
		clone.References = append([]SketchGeometryRef(nil), c.References...)
		for i, r := range c.References {
			if r.Target != "ENTITY" || r.EntityID != source.ID {
				continue
			}
			keep := -1
			switch r.SubElement {
			case "START":
				for segment, interval := range op.Intervals {
					if interval.Start == 0 && source.StartPointID != "" {
						keep = segment
						break
					}
				}
			case "END":
				for segment, interval := range op.Intervals {
					if interval.End == 1 && source.EndPointID != "" {
						keep = segment
						break
					}
				}
			case "CENTER":
				if len(op.ComputedEntities) > 0 {
					keep = 0
				}
			case "WHOLE", "DIRECTION":
				if len(op.ComputedEntities) > 0 && (c.Kind == "RADIUS" || c.Kind == "DIAMETER" || c.Kind == "MAJOR_RADIUS" || c.Kind == "MINOR_RADIUS" || c.Kind == "SAME_SUPPORT" || c.Kind == "HORIZONTAL" || c.Kind == "VERTICAL" || c.Kind == "PARALLEL" || c.Kind == "PERPENDICULAR" || c.Kind == "COLLINEAR" || c.Kind == "POINT_ON_OBJECT") {
					keep = 0
				}
			}
			if keep < 0 && sharedCircularSupport != "" && (r.SubElement == "CENTER" || (r.SubElement == "WHOLE" && (c.Kind == "RADIUS" || c.Kind == "DIAMETER" || c.Kind == "CONCENTRIC"))) {
				clone.References[i].EntityID = sharedCircularSupport
				clone.References[i].PointID = ""
				continue
			}
			if keep < 0 {
				return fmt.Errorf("%w: topology edit changes constraint %s (%s); explicitly release or replace this reference", ErrValidation, c.ID, r.SubElement)
			}
			clone.References[i].EntityID = op.ComputedEntities[keep].ID
		}
		constraints = append(constraints, clone)
	}
	for i, interval := range op.Intervals {
		for cutIndex, cut := range op.CutConnections {
			for _, endpoint := range []struct {
				sub string
				t   float64
			}{{"START", interval.Start}, {"END", interval.End}} {
				if endpoint.t == cut.Parameter || ((source.Kind == "CIRCLE" || source.Kind == "ELLIPSE") && endpoint.t == cut.Parameter+1) {
					kind := cut.Kind
					if kind == "" {
						kind = "COINCIDENT"
					}
					constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("boundary-join/%d/%d/%s", i, cutIndex, endpoint.sub)), Kind: kind, Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: op.ComputedEntities[i].ID, SubElement: endpoint.sub}, cut.Reference}})
				}
			}
		}
	}
	for i, e := range op.ComputedEntities {
		if e.ID == "" || (i == 0 && e.ID != source.ID) || (i > 0 && entities[e.ID].ID != "") {
			return fmt.Errorf("%w: invalid precise output identity", ErrValidation)
		}
		for j := 0; j < i; j++ {
			if op.Intervals[j].End == op.Intervals[i].Start {
				constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("join/%d/%d", j, i)), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: op.ComputedEntities[j].ID, SubElement: "END"}, {Target: "ENTITY", EntityID: e.ID, SubElement: "START"}}})
			}
		}
	}
	if source.Kind == "LINE" || source.Kind == "CIRCLE" || source.Kind == "ARC" || source.Kind == "ELLIPSE" || source.Kind == "ELLIPTICAL_ARC" {
		for i := 1; i < len(op.ComputedEntities); i++ {
			constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("same-support/%d", i)), Kind: "SAME_SUPPORT", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: op.ComputedEntities[0].ID, SubElement: "WHOLE"}, {Target: "ENTITY", EntityID: op.ComputedEntities[i].ID, SubElement: "WHOLE"}}})
		}
	}
	if (source.Kind == "CIRCLE" || source.Kind == "ELLIPSE" || source.Closed) && len(op.Intervals) > 1 && op.Intervals[len(op.Intervals)-1].End == op.Intervals[0].Start+1 {
		constraints = append(constraints, SketchConstraint{ID: macroID(op.OperationID, "periodic-join"), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{{Target: "ENTITY", EntityID: op.ComputedEntities[0].ID, SubElement: "START"}, {Target: "ENTITY", EntityID: op.ComputedEntities[len(op.ComputedEntities)-1].ID, SubElement: "END"}}})
	}
	kept := sketch.Entities[:0]
	for _, e := range sketch.Entities {
		if e.ID != source.ID {
			kept = append(kept, e)
		}
	}
	sketch.Entities = append(kept, op.ComputedEntities...)
	sketch.Constraints = constraints
	return nil
}

func unwrapSketchCurveParameter(e SketchEntity, p float64) float64 {
	if e.Kind != "ARC" && e.Kind != "ELLIPTICAL_ARC" {
		return p
	}
	lo, hi := math.Min(e.StartAngle, e.EndAngle), math.Max(e.StartAngle, e.EndAngle)
	shifted := p + 2*math.Pi*math.Ceil((lo-p)/(2*math.Pi))
	if shifted <= hi {
		return shifted
	}
	return p
}

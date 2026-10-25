package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"sort"
)

type SketchIntersectionsRequest struct {
	VersionID string   `json:"versionId"`
	SketchID  string   `json:"sketchId"`
	EntityIDs []string `json:"entityIds"`
}
type SketchIntersection struct {
	Point               [2]float64           `json:"point"`
	Parameters          [2]float64           `json:"parameters"`
	References          [2]SketchGeometryRef `json:"references"`
	ConstraintSupported bool                 `json:"constraintSupported"`
}
type SketchIntersections struct {
	VersionID     string               `json:"versionId"`
	SketchID      string               `json:"sketchId"`
	SourceDigests [2]string            `json:"sourceDigests"`
	Intersections []SketchIntersection `json:"intersections"`
}

func (s *Service) SketchIntersections(ctx context.Context, documentID string, r SketchIntersectionsRequest) (SketchIntersections, error) {
	result := SketchIntersections{VersionID: r.VersionID, SketchID: r.SketchID, Intersections: []SketchIntersection{}}
	v, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return result, err
	}
	if v.Part == nil || v.Document.VersionID != r.VersionID {
		return result, fmt.Errorf("%w: sketch intersection revision changed", ErrValidation)
	}
	for _, f := range v.Part.Features {
		if f.ID == r.SketchID && f.Sketch != nil {
			return s.exactSketchIntersections(ctx, f.Sketch, r.EntityIDs, documentID+"/intersections/"+r.VersionID, result)
		}
	}
	return result, fmt.Errorf("%w: sketch missing", ErrValidation)
}
func (s *Service) exactSketchIntersections(ctx context.Context, sketch *SketchFeature, ids []string, requestID string, result SketchIntersections) (SketchIntersections, error) {
	if len(ids) != 2 || ids[0] == ids[1] {
		return result, fmt.Errorf("%w: select two distinct curves", ErrValidation)
	}
	var curves [2]SketchEntity
	var targets [2]string
	for i, id := range ids {
		e, ok := sketchEditBoundary(sketch, id)
		if !ok || e.Suppressed || e.Kind == "POINT" {
			return result, fmt.Errorf("%w: intersection requires supported finite curves", ErrValidation)
		}
		curves[i] = e
		targets[i] = "EXTERNAL"
		for _, local := range sketch.Entities {
			if local.ID == id {
				targets[i] = "ENTITY"
			}
		}
		result.SourceDigests[i] = sketchCurveDigest(e)
	}
	if s.worker == nil {
		return result, fmt.Errorf("%w: exact curve worker unavailable", ErrValidation)
	}
	computed, err := s.worker.ComputeSketchCurves(ctx, requestID, "INTERSECT", []geometry.ProfileCurve{profileCurve(curves[0], false), profileCurve(curves[1], false)}, nil, nil)
	if err != nil {
		return result, err
	}
	if len(computed.Overlaps) > 0 {
		return result, fmt.Errorf("%w: overlapping curves have no isolated intersection", ErrValidation)
	}
	for _, p := range computed.Intersections {
		hit := SketchIntersection{Point: p.Point, ConstraintSupported: true}
		for i, parameter := range []float64{p.FirstParameter, p.SecondParameter} {
			a, b, err := sketchCurveDomain(curves[i])
			if err != nil {
				return result, err
			}
			parameter = unwrapSketchCurveParameter(curves[i], parameter)
			t := (parameter - a) / (b - a)
			if t < -1e-9 || t > 1+1e-9 {
				continue
			}
			hit.Parameters[i] = math.Max(0, math.Min(1, t))
			sub := sketchBoundaryIntersectionEndpoint(curves[i], parameter, p.Point)
			if sub == "" {
				sub = "WHOLE"
				if curves[i].Kind == "SPLINE" {
					hit.ConstraintSupported = false
				}
			}
			hit.References[i] = SketchGeometryRef{Target: targets[i], EntityID: curves[i].ID, SubElement: sub}
		}
		if hit.References[0].EntityID != "" && hit.References[1].EntityID != "" {
			result.Intersections = append(result.Intersections, hit)
		}
	}
	sort.Slice(result.Intersections, func(i, j int) bool {
		return result.Intersections[i].Parameters[0] < result.Intersections[j].Parameters[0]
	})
	return result, nil
}

// Hints select an exact branch in this frozen operation; they never identify a
// persistent point. The accepted point and relationships use ordinary stable IDs.
func (s *Service) prepareSketchIntersection(ctx context.Context, model PartModel, sketchID string, staged *SketchFeature, op SketchOperation, requestID string) ([]SketchOperation, error) {
	if op.OperationID == "" || (len(op.Parameters) != 2 && len(op.Parameters) != 4) {
		return nil, fmt.Errorf("%w: select an exact intersection", ErrValidation)
	}
	analysis, err := s.exactSketchIntersections(ctx, staged, op.EntityIDs, requestID, SketchIntersections{Intersections: []SketchIntersection{}})
	if err != nil {
		return nil, err
	}
	hits := []SketchIntersection{}
	for i := 0; i < len(op.Parameters); i += 2 {
		var selected *SketchIntersection
		for _, h := range analysis.Intersections {
			if math.Abs(h.Parameters[0]-op.Parameters[i]) < 1e-9 && math.Abs(h.Parameters[1]-op.Parameters[i+1]) < 1e-9 {
				if selected != nil {
					return nil, fmt.Errorf("%w: intersection is ambiguous", ErrValidation)
				}
				copy := h
				selected = &copy
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("%w: intersection changed or missing", ErrValidation)
		}
		hits = append(hits, *selected)
	}
	if op.Type == "CREATE_INTERSECTION" {
		if len(hits) != 1 || !hits[0].ConstraintSupported {
			return nil, fmt.Errorf("%w: intersection support is unavailable to the sketch solver", ErrValidation)
		}
		id := macroID(op.OperationID, "point")
		point := SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: "POINT"}
		p := hits[0].Point
		ops := []SketchOperation{{Type: "ADD_ENTITY", Entity: &SketchEntity{ID: id, Kind: "POINT", Role: "CONSTRUCTION", Point: &SketchPoint2{X: p[0], Y: p[1]}, CreatedByOperationID: op.OperationID}}}
		for _, r := range hits[0].References {
			kind := "COINCIDENT"
			if r.SubElement == "WHOLE" {
				kind = "POINT_ON_OBJECT"
			}
			if len(ops) > 1 && kind == "COINCIDENT" && sketchPointsJoined(staged, hits[0].References[0], r) {
				continue
			}
			ops = append(ops, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &SketchConstraint{ID: macroID(op.OperationID, "support/"+r.EntityID), Kind: kind, References: []SketchGeometryRef{point, r}}})
		}
		return ops, nil
	}
	if op.Mode != "FIRST" && op.Mode != "BOTH" {
		return nil, fmt.Errorf("%w: choose one or both curves", ErrValidation)
	}
	splits := []SketchOperation{}
	for i, id := range op.EntityIDs {
		if i == 1 && op.Mode == "FIRST" {
			break
		}
		var source SketchEntity
		for _, e := range staged.Entities {
			if e.ID == id {
				source = e
			}
		}
		if source.ID == "" {
			return nil, fmt.Errorf("%w: external geometry is read only", ErrValidation)
		}
		closed := source.Kind == "CIRCLE" || source.Kind == "ELLIPSE" || source.Closed
		cuts := []float64{}
		for _, h := range hits {
			v := h.Parameters[i]
			if closed {
				if v > 1-1e-10 {
					v = 0
				}
				cuts = append(cuts, v)
			} else if v > 1e-10 && v < 1-1e-10 {
				cuts = append(cuts, v)
			}
		}
		sort.Float64s(cuts)
		if len(cuts) == 2 && math.Abs(cuts[0]-cuts[1]) < 1e-9 {
			return nil, fmt.Errorf("%w: select different intersections", ErrValidation)
		}
		if closed && len(cuts) < 2 {
			return nil, fmt.Errorf("%w: closed curve needs two distinct intersection positions", ErrValidation)
		}

		if len(cuts) > 0 {
			splits = append(splits, SketchOperation{Type: "SPLIT_ENTITY", OperationID: macroID(op.OperationID, id), EntityIDs: []string{id}, Parameters: cuts, DetachConstraintIDs: op.DetachConstraintIDs})
		}
	}
	if len(splits) == 0 {
		return nil, fmt.Errorf("%w: intersections are already endpoints", ErrValidation)
	}
	for i := range model.Features {
		if model.Features[i].ID == sketchID {
			model.Features[i].Sketch = staged
		}
	}
	raw, _ := json.Marshal(model)
	prepared, err := s.prepareSketchCurveEdits(ctx, raw, sketchID, requestID+"/split", splits)
	if err != nil {
		return nil, err
	}
	for _, hit := range hits {
		refs := hit.References
		for i, id := range op.EntityIDs {
			for _, p := range prepared {
				if p.Type != "REPLACE_CURVE_INTERVALS" || p.EntityIDs[0] != id {
					continue
				}
				for j, interval := range p.Intervals {
					t := hit.Parameters[i]
					if math.Abs(interval.Start-t) < 1e-9 {
						refs[i] = SketchGeometryRef{Target: "ENTITY", EntityID: p.ComputedEntities[j].ID, SubElement: "START", PointID: p.ComputedEntities[j].StartPointID}
						break
					}
					if math.Abs(interval.End-t) < 1e-9 {
						refs[i] = SketchGeometryRef{Target: "ENTITY", EntityID: p.ComputedEntities[j].ID, SubElement: "END", PointID: p.ComputedEntities[j].EndPointID}
						break
					}
				}
			}
		}
		if refs[0].SubElement == "WHOLE" {
			refs[0], refs[1] = refs[1], refs[0]
		}
		kind := "COINCIDENT"
		if refs[1].SubElement == "WHOLE" {
			kind = "POINT_ON_OBJECT"
		}
		if kind == "COINCIDENT" && sketchPointsJoined(staged, refs[0], refs[1]) {
			continue
		}
		prepared = append(prepared, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &SketchConstraint{ID: macroID(op.OperationID, fmt.Sprintf("join/%g/%g", hit.Parameters[0], hit.Parameters[1])), Kind: kind, References: refs[:]}})
	}
	return prepared, nil
}
func sketchPointsJoined(sketch *SketchFeature, a, b SketchGeometryRef) bool {
	key := func(r SketchGeometryRef) string {
		if r.PointID != "" {
			return r.Target + "/point/" + r.PointID
		}
		for _, e := range sketch.Entities {
			if e.ID == r.EntityID {
				if r.SubElement == "START" && e.StartPointID != "" {
					return r.Target + "/point/" + e.StartPointID
				}
				if r.SubElement == "END" && e.EndPointID != "" {
					return r.Target + "/point/" + e.EndPointID
				}
			}
		}
		return fmt.Sprintf("%s/%s/%s/%s", r.Target, r.EntityID, r.SubElement, r.ControlPointID)
	}
	seen := map[string]bool{key(a): true}
	for changed := true; changed; {
		changed = false
		for _, c := range sketch.Constraints {
			if c.Suppressed || c.Kind != "COINCIDENT" || len(c.References) != 2 {
				continue
			}
			x, y := key(c.References[0]), key(c.References[1])
			if seen[x] != seen[y] {
				seen[x], seen[y] = true, true
				changed = true
			}
		}
	}
	return seen[key(b)]
}

package automation

import (
	"context"
	"encoding/json"
	"math"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// Acceptance evidence for an explicitly bound circular cut, not a general hole
// locator. Uses the same current Naming observations, predicates and full set
// matching as QueryTopology. Ambiguous/split/unsupported walls stay unresolved.
func (s *Service) requiredHoleRims(ctx context.Context, document string, part workspace.PartModel, fillet workspace.Feature, holeID, revision string, edges workspace.BodyTopology, tolerance float64) ([]workspace.BodyTopologyItem, string, error) {
	var hole *workspace.Feature
	for i := range part.Features {
		if part.Features[i].ID == holeID {
			hole = &part.Features[i]
			break
		}
	}
	if hole == nil || hole.BodyID != fillet.BodyID || hole.Suppressed || hole.Order >= fillet.Order {
		return nil, "HOLE_RIM_TARGET_UNRESOLVED", nil
	}
	generator, sketch, circle := circularCutProfile(part, *hole)
	if generator == nil || sketch == nil || circle == nil || circle.Center == nil || generator.Extent != "THROUGH_ALL" {
		return nil, "HOLE_RIM_STRATEGY_UNSUPPORTED", nil
	}
	source, err := s.workspace.ReadModelSnapshot(ctx, document, revision)
	if err != nil {
		return nil, "", err
	}
	// The cut must already exist at the actual fillet input, not only at the
	// final Head. A later unrelated cut cannot lend a semantic target backwards.
	var sourcePart workspace.PartModel
	if err = json.Unmarshal(source.Model, &sourcePart); err != nil {
		return nil, "", err
	}
	found := false
	for _, f := range sourcePart.Features {
		if f.ID == holeID && !f.Suppressed {
			found = true
			break
		}
	}
	if !found {
		return nil, "HOLE_RIM_TARGET_UNRESOLVED", nil
	}
	faces, err := s.workspace.(topologyReader).ReadBodyTopology(ctx, document, revision, fillet.BodyID, "FACE")
	if err != nil {
		return nil, "", err
	}
	center := sketchPointWorld(sketch.Sketch.Support, *circle.Center)
	normal := sketch.Sketch.Support.Normal
	wallID := ""
	for _, face := range faces.Items {
		g := face.Geometry
		if face.GeometryType != "CYLINDER" || g.RadiusMM == nil || g.MaterialSide == nil || *g.MaterialSide != -1 || g.OriginMM == nil || g.Direction == nil {
			continue
		}
		if closeNumber(*g.RadiusMM, circle.Radius, tolerance) && parallelVectors(*g.Direction, normal, tolerance) && onAxis(*g.OriginMM, center, normal, tolerance) {
			if wallID != "" {
				return nil, "HOLE_RIM_WALL_AMBIGUOUS", nil
			}
			wallID = face.ID
		}
	}
	if wallID == "" {
		return nil, "HOLE_RIM_WALL_UNAVAILABLE", nil
	}
	matches, err := matchTopology(edges, nil, &TopologyFilter{GeometryType: "CIRCLE", Radius: &TopologyRadiusFilter{EqualMM: &circle.Radius, ToleranceMM: tolerance}, AdjacentFaceID: wallID})
	if err != nil {
		return nil, "", err
	}
	if matches.unknown != 0 || !completeRimCircles(matches.items, center, normal, tolerance) {
		return nil, "HOLE_RIM_BOUNDARY_UNRESOLVED", nil
	}
	return matches.items, "", nil
}

func completeRimCircles(items []workspace.BodyTopologyItem, center, normal [3]float64, toleranceMM float64) bool {
	if len(items) != 2 {
		return false
	}
	for _, item := range items {
		g := item.Geometry
		if g.OriginMM == nil || g.Direction == nil || g.Range == nil || g.Range.Convention != "ANGLE_RADIANS" {
			return false
		}
		if math.Abs(math.Abs(g.Range.End-g.Range.Start)-2*math.Pi) > modelcore.TopologyNamingAngularToleranceRadians || !onAxis(*g.OriginMM, center, normal, toleranceMM) || !parallelVectors(*g.Direction, normal, 1e-9) {
			return false
		}
	}
	a, b := items[0].Geometry.OriginMM, items[1].Geometry.OriginMM
	distanceSquared := 0.0
	for i := 0; i < 3; i++ {
		distanceSquared += (a[i] - b[i]) * (a[i] - b[i])
	}
	return distanceSquared > toleranceMM*toleranceMM
}

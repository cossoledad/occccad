package workspace

import (
	"context"
	"fmt"
	"sort"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type ThinWallDefinition struct {
	Input      string                `json:"input"` // REGIONS | CHAINS
	Mode       string                `json:"mode"`  // ONE_SIDED | TWO_SIDED | SYMMETRIC (total thickness)
	Side       string                `json:"side"`  // LEFT | RIGHT, along the formal directed chain
	Join       string                `json:"join"`  // MITER | ROUND
	Thickness  float64               `json:"thickness"`
	Thickness2 float64               `json:"thickness2,omitempty"`
	EntityIDs  []string              `json:"entityIds,omitempty"`
	Reversed   bool                  `json:"reversed,omitempty"`
	Directions []CurveChainDirection `json:"directions,omitempty"`
}

// Freeze formal direction anchors on an explicit Feature creation/definition
// edit. Upstream sketch edits retain these anchors and cannot silently reverse
// a wall when UUID ordering or chain membership changes.
func resolveThinDirections(model PartModel, feature *Feature) error {
	if feature.ThinWall == nil || feature.ThinWall.Input != "CHAINS" || len(feature.ThinWall.Directions) > 0 {
		return nil
	}
	earlier := map[string]Feature{}
	for _, f := range model.Features {
		if f.ID == feature.ID {
			break
		}
		earlier[f.ID] = f
	}
	source, err := resolvePatternSketch(model, earlier, feature.Profile, feature.ProfileMemberSlot)
	if err != nil {
		return err
	}
	chains, err := sketchCurveChains(source, feature.ThinWall.EntityIDs)
	if err != nil {
		return err
	}
	definition := *feature.ThinWall
	definition.Directions = nil
	for _, chain := range chains {
		definition.Directions = append(definition.Directions, CurveChainDirection{EntityID: chain.Curves[0].EntityID, Reversed: chain.Curves[0].Reversed})
	}
	sort.Slice(definition.Directions, func(i, j int) bool { return definition.Directions[i].EntityID < definition.Directions[j].EntityID })
	feature.ThinWall = &definition
	return nil
}

func validateThinWall(f Feature) error {
	d := f.ThinWall
	if d == nil {
		return nil
	}
	if !linearExtrudeFeature(f) || !positiveFinite(d.Thickness) || (d.Input != "REGIONS" && d.Input != "CHAINS") || (d.Mode != "ONE_SIDED" && d.Mode != "TWO_SIDED" && d.Mode != "SYMMETRIC") || (d.Side != "LEFT" && d.Side != "RIGHT") || (d.Join != "MITER" && d.Join != "ROUND") {
		return fmt.Errorf("%w: THIN_DEFINITION_INVALID", ErrValidation)
	}
	if d.Mode == "TWO_SIDED" {
		if !positiveFinite(d.Thickness2) {
			return fmt.Errorf("%w: THIN_SECOND_THICKNESS_INVALID", ErrValidation)
		}
	} else if d.Thickness2 != 0 {
		return fmt.Errorf("%w: THIN_INACTIVE_SECOND_THICKNESS", ErrValidation)
	}
	if d.Input == "CHAINS" {
		if len(d.EntityIDs) == 0 || len(d.EntityIDs) > 256 || f.ProfileSelection != nil {
			return fmt.Errorf("%w: THIN_CHAIN_SELECTION_REQUIRED", ErrValidation)
		}
		seen := map[string]bool{}
		for _, id := range d.EntityIDs {
			if id == "" || seen[id] {
				return fmt.Errorf("%w: THIN_CHAIN_SELECTION_INVALID", ErrValidation)
			}
			seen[id] = true
		}
		anchors := map[string]bool{}
		for _, a := range d.Directions {
			if !seen[a.EntityID] || anchors[a.EntityID] {
				return fmt.Errorf("%w: CURVE_CHAIN_DIRECTION_INVALID", ErrValidation)
			}
			anchors[a.EntityID] = true
		}
	} else if len(d.EntityIDs) > 0 {
		return fmt.Errorf("%w: THIN_REGIONS_CANNOT_DECLARE_CHAINS", ErrValidation)
	} else if len(d.Directions) > 0 {
		return fmt.Errorf("%w: THIN_REGIONS_CANNOT_DECLARE_DIRECTIONS", ErrValidation)
	}
	return nil
}

func thinWallParameters(f Feature) []extrudeParameter {
	if f.ThinWall == nil {
		return nil
	}
	out := []extrudeParameter{{"thin:thickness", "Thin thickness", f.ThinWall.Thickness}}
	if f.ThinWall.Mode == "TWO_SIDED" {
		out = append(out, extrudeParameter{"thin:thickness2", "Second thin thickness", f.ThinWall.Thickness2})
	}
	return out
}

func thinEntity(c geometry.ProfileCurve) (SketchEntity, error) {
	e := SketchEntity{ID: c.EntityID, Kind: c.Kind, Role: "PROFILE", Radius: c.Radius, StartAngle: c.StartAngle, EndAngle: c.EndAngle}
	switch c.Kind {
	case "LINE":
		e.Start = &SketchPoint2{X: c.Start[0], Y: c.Start[1]}
		e.End = &SketchPoint2{X: c.End[0], Y: c.End[1]}
	case "ARC", "CIRCLE":
		e.Center = &SketchPoint2{X: c.Center[0], Y: c.Center[1]}
	default:
		return e, fmt.Errorf("%w: THIN_UNSUPPORTED_CURVE: %s; exact thin sections support Line, Arc and Circle", ErrValidation, c.Kind)
	}
	if c.Reversed && c.Kind != "CIRCLE" {
		e = offsetReverse(e)
	}
	return e, nil
}

// This is an analytic section construction from accepted geometry. It introduces
// no new sketch variables, hidden constraints, saved coordinates or model edits.
// The native generator receives exact curves and an explicit formal origin map.
func thinProfileOutline(feature Feature, chains []sketchCurveChain) (Feature, map[string][]string, error) {
	d := feature.ThinWall
	if err := validateThinWall(feature); err != nil {
		return Feature{}, nil, err
	}
	outline := SketchFeature{SchemaVersion: SketchSchemaVersion}
	origins := map[string][]string{}
	join := func(a, as, b, bs string) {
		outline.Constraints = append(outline.Constraints, SketchConstraint{ID: macroID(feature.ID, "thin/join/"+a+"/"+as+"/"+b+"/"+bs), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{cornerRef(a, as), cornerRef(b, bs)}})
	}
	low, high := 0.0, d.Thickness
	if d.Mode == "SYMMETRIC" {
		low, high = -d.Thickness/2, d.Thickness/2
	} else if d.Mode == "TWO_SIDED" {
		low = -d.Thickness2
	}
	if d.Side == "RIGHT" && d.Mode != "SYMMETRIC" {
		low, high = -high, -low
	}
	for _, chain := range chains {
		if d.Reversed {
			chain = reverseCurveChain(chain)
		}
		source := SketchFeature{SchemaVersion: SketchSchemaVersion}
		ids := []string{}
		for _, c := range chain.Curves {
			e, err := thinEntity(c)
			if err != nil {
				return Feature{}, nil, err
			}
			source.Entities = append(source.Entities, e)
			ids = append(ids, e.ID)
		}
		if len(ids) == 0 {
			return Feature{}, nil, fmt.Errorf("%w: THIN_EMPTY_CHAIN", ErrValidation)
		}
		for i, e := range source.Entities {
			if i+1 < len(ids) || chain.Closed && e.Kind != "CIRCLE" {
				next := ids[(i+1)%len(ids)]
				source.Constraints = append(source.Constraints, SketchConstraint{ID: macroID(feature.ID, "thin/source-join/"+e.ID+"/"+next), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{cornerRef(e.ID, "END"), cornerRef(next, "START")}})
			}
		}
		sides := [][]SketchEntity{}
		for _, side := range []struct {
			key      string
			distance float64
		}{{"RIGHT", low}, {"LEFT", high}} {
			distance := side.distance
			operation := macroID(feature.ID, "thin/side/"+side.key)
			copy := source
			copy.Entities = append([]SketchEntity(nil), source.Entities...)
			copy.Constraints = append([]SketchConstraint(nil), source.Constraints...)
			count := len(copy.Entities)
			constraintCount := len(copy.Constraints)
			if len(chain.Curves) == 1 && chain.Curves[0].Kind == "CIRCLE" && chain.Curves[0].Reversed {
				distance = -distance
			}
			if distance != 0 {
				if err := applySketchOffsetEdit(&copy, SketchOperation{OperationID: operation, EntityIDs: ids, Value: &distance, Mode: d.Join}); err != nil {
					return Feature{}, nil, fmt.Errorf("%w: THIN_OFFSET_FAILED: %v", ErrValidation, err)
				}
			} else {
				for _, e := range source.Entities {
					e.SourceEntityID = e.ID
					copy.Entities = append(copy.Entities, e)
				}
				for i := range source.Entities {
					if i+1 < len(ids) || chain.Closed && source.Entities[i].Kind != "CIRCLE" {
						copy.Constraints = append(copy.Constraints, SketchConstraint{ID: macroID(operation, "join/"+ids[i]+"/"+ids[(i+1)%len(ids)]), Kind: "COINCIDENT", Internal: true, References: []SketchGeometryRef{cornerRef(copy.Entities[count+i].ID, "END"), cornerRef(copy.Entities[count+(i+1)%len(ids)].ID, "START")}})
					}
				}
			}
			generated := copy.Entities[count:]
			sides = append(sides, generated)
			outline.Entities = append(outline.Entities, generated...)
			outline.Constraints = append(outline.Constraints, copy.Constraints[constraintCount:]...)
			for _, e := range generated {
				origins[e.ID] = []string{e.SourceEntityID}
			}
			joints := len(ids) - 1
			if chain.Closed {
				joints++
			}
			for i := 0; i < joints; i++ {
				a, b := ids[i], ids[(i+1)%len(ids)]
				id := macroID(operation, "offset/round/"+a+"/"+b)
				if _, ok := origins[id]; ok {
					origins[id] = []string{a, b}
				}
			}
		}
		if !chain.Closed {
			for _, end := range []bool{false, true} {
				i, j, sub := 0, 0, "START"
				if end {
					i, j, sub = len(sides[0])-1, len(sides[1])-1, "END"
				}
				a, b := sides[0][i], sides[1][j]
				p, q := cornerEndpoint(a, sub), cornerEndpoint(b, sub)
				endpoint := chain.Curves[0].EntityID + "/START"
				if chain.Curves[0].Reversed {
					endpoint = chain.Curves[0].EntityID + "/END"
				}
				if end {
					last := chain.Curves[len(chain.Curves)-1]
					endpoint = last.EntityID + "/END"
					if last.Reversed {
						endpoint = last.EntityID + "/START"
					}
				}
				id := macroID(feature.ID, "thin/cap/"+endpoint)
				cap := SketchEntity{ID: id, Kind: "LINE", Role: "PROFILE", Start: &p, End: &q}
				outline.Entities = append(outline.Entities, cap)
				join(a.ID, sub, id, "START")
				join(b.ID, sub, id, "END")
				original := ids[0]
				if end {
					original = ids[len(ids)-1]
				}
				origins[id] = []string{original}
			}
		}
		if len(outline.Entities) > 4096 {
			return Feature{}, nil, fmt.Errorf("%w: THIN_GENERATED_CURVE_BUDGET", ErrValidation)
		}
	}
	if len(outline.Entities) == 0 {
		return Feature{}, nil, fmt.Errorf("%w: THIN_EMPTY_CHAIN", ErrValidation)
	}
	return Feature{ID: feature.Profile, Type: "SKETCH", Sketch: &outline}, origins, nil
}

func (s *Service) thinProfileRegions(ctx context.Context, model PartModel, feature Feature, requestID string) (Feature, []geometry.ProfileRegion, []geometry.ProfileOrigin, error) {
	earlier := map[string]Feature{}
	for _, f := range model.Features {
		if f.ID == feature.ID {
			break
		}
		earlier[f.ID] = f
	}
	sketch, err := resolvePatternSketch(model, earlier, feature.Profile, feature.ProfileMemberSlot)
	if err != nil {
		return Feature{}, nil, nil, err
	}
	var chains []sketchCurveChain
	if feature.ThinWall.Input == "CHAINS" {
		chains, err = sketchCurveChains(sketch, feature.ThinWall.EntityIDs)
		if err == nil {
			chains, err = orientCurveChains(chains, feature.ThinWall.Directions)
		}
	} else {
		ordinary := feature
		ordinary.ThinWall = nil
		var regions []geometry.ProfileRegion
		_, regions, err = s.featureProfileRegions(ctx, model, ordinary, requestID+"/source")
		for _, r := range regions {
			for _, loop := range append([]geometry.ProfileLoop{r.Outer}, r.Holes...) {
				chains = append(chains, sketchCurveChain{ID: loop.ID, Curves: loop.Curves, Closed: true})
			}
		}
	}
	if err != nil {
		return sketch, nil, nil, err
	}
	regions := []geometry.ProfileRegion{}
	mapped := []geometry.ProfileOrigin{}
	estimate := 0
	for _, chain := range chains {
		if len(chain.Curves) > 512 {
			return sketch, nil, nil, fmt.Errorf("%w: THIN_SOURCE_CURVE_BUDGET", ErrValidation)
		}
		estimate += 4 * len(chain.Curves)
		if !chain.Closed {
			estimate += 2
		}
		if estimate > 4096 {
			return sketch, nil, nil, fmt.Errorf("%w: THIN_GENERATED_CURVE_BUDGET", ErrValidation)
		}
	}
	// Classify each wall separately. Global even/odd loop classification would
	// erase overlapping wall material (e.g. nested circular thin sections).
	// Their exact union and composed history belong to the native generator.
	for _, chain := range chains {
		if err := ctx.Err(); err != nil {
			return sketch, nil, nil, err
		}
		outline, origins, err := thinProfileOutline(feature, []sketchCurveChain{chain})
		if err != nil {
			return sketch, nil, nil, err
		}
		loops, err := buildProfileLoops(outline, true)
		if err != nil {
			return sketch, nil, nil, err
		}
		section, err := s.classifyExactProfileLoops(ctx, "thin:"+feature.ID, loops, requestID+"/thin-section/"+chain.ID)
		if err != nil {
			return sketch, nil, nil, err
		}
		regions = append(regions, section...)
		mapped = append(mapped, thinSectionOrigins(sketch.ID, outline, origins, section)...)
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].ID < regions[j].ID })
	sort.Slice(mapped, func(i, j int) bool { return mapped[i].DerivedID < mapped[j].DerivedID })
	return sketch, regions, mapped, nil
}

func thinSectionOrigins(sketchID string, outline Feature, origins map[string][]string, regions []geometry.ProfileRegion) []geometry.ProfileOrigin {
	graph := profileConnectivity(outline)
	endpointSources := map[string]map[string]bool{}
	for _, e := range outline.Sketch.Entities {
		for _, sub := range []string{"START", "END"} {
			key := graph.find(e.ID + "/" + sub)
			if endpointSources[key] == nil {
				endpointSources[key] = map[string]bool{}
			}
			for _, id := range origins[e.ID] {
				endpointSources[key][id] = true
			}
		}
	}
	sourceRefs := func(ids []string) []modelcore.SemanticTopologyRef {
		sort.Strings(ids)
		refs := []modelcore.SemanticTopologyRef{}
		seen := map[string]bool{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				refs = append(refs, modelcore.SemanticTopologyRef{FeatureID: sketchID, OutputSlot: "PROFILE_EDGE/" + id, SourceIDs: []string{id}})
			}
		}
		return refs
	}
	mapped := []geometry.ProfileOrigin{}
	for _, e := range outline.Sketch.Entities {
		mapped = append(mapped, geometry.ProfileOrigin{DerivedID: e.ID, Sources: sourceRefs(origins[e.ID])})
		for _, sub := range []string{"START", "END"} {
			ids := []string{}
			for id := range endpointSources[graph.find(e.ID+"/"+sub)] {
				ids = append(ids, id)
			}
			mapped = append(mapped, geometry.ProfileOrigin{DerivedID: e.ID + "/" + sub, Sources: sourceRefs(ids)})
		}
	}
	for _, region := range regions {
		ids := []string{}
		for _, loop := range append([]geometry.ProfileLoop{region.Outer}, region.Holes...) {
			for _, c := range loop.Curves {
				ids = append(ids, origins[c.EntityID]...)
			}
		}
		mapped = append(mapped, geometry.ProfileOrigin{DerivedID: region.ID, Sources: sourceRefs(ids)})
	}
	sort.Slice(mapped, func(i, j int) bool { return mapped[i].DerivedID < mapped[j].DerivedID })
	return mapped
}

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// PatternDefinition is shared by sketch geometry, sketch frames and solid tools.
// Coordinates use the owner's frame (mm); angles are degrees. Count includes slot
// zero. Skipping a slot never changes the identity or placement of another slot.
type PatternDefinition struct {
	MirrorPlaneID      string                 `json:"mirrorPlaneId,omitempty"`
	MirrorPlane        *FeatureSelection      `json:"mirrorPlane,omitempty"`
	DirectionReference *SketchGeometryRef     `json:"directionReference,omitempty"`
	CenterReference    *PatternPointReference `json:"centerReference,omitempty"`
	Reversed           bool                   `json:"reversed,omitempty"`
	AxisEntityID       string                 `json:"axisEntityId,omitempty"`
	ID                 string                 `json:"id"`
	Kind               string                 `json:"kind"`
	Distribution       string                 `json:"distribution"`
	Count              float64                `json:"count"`
	Spacing            float64                `json:"spacing,omitempty"`
	Angle              float64                `json:"angle,omitempty"`
	Phase              float64                `json:"phase,omitempty"`
	Origin             [3]float64             `json:"origin"`
	Direction          [3]float64             `json:"direction"`
	SkippedSlots       []int                  `json:"skippedSlots,omitempty"`
	Suppressed         bool                   `json:"suppressed,omitempty"`
}

const maxPatternMembers = 256

type FeaturePattern struct {
	ResultMode string `json:"resultMode,omitempty"` // COMBINE | INDEPENDENT
	PatternDefinition
	Source         FeatureStageRef `json:"source"`
	SourceKind     string          `json:"sourceKind"`               // SKETCH_FRAME, GENERATOR_TOOL, BODY_STAGE, FEATURE_DELTA
	StartFeatureID string          `json:"startFeatureId,omitempty"` // inclusive start of additive material range
}

func resolvePatternAxisDefinition(model PartModel, p PatternDefinition) (PatternDefinition, error) {
	if p.AxisEntityID == "" {
		return p, nil
	}
	parts := strings.Split(p.AxisEntityID, ":")
	if len(parts) == 2 && parts[0] == "DATUM_AXIS" {
		for _, axis := range model.DatumAxes {
			if axis.ID == parts[1] {
				p.Origin, p.Direction = axis.Origin, axis.Direction
				return p, nil
			}
		}
	}
	if len(parts) == 3 && parts[0] == "AXIS_SYSTEM" {
		for _, axis := range model.AxisSystems {
			if axis.ID == parts[1] {
				p.Origin = axis.Origin
				switch parts[2] {
				case "X":
					p.Direction = axis.XDirection
				case "Y":
					p.Direction = axis.YDirection
				case "Z":
					p.Direction = axis.ZDirection
				default:
					return p, fmt.Errorf("%w: invalid pattern axis", ErrValidation)
				}
				return p, nil
			}
		}
	}
	if len(parts) == 3 && parts[0] == "SKETCH_LINE" {
		for _, feature := range model.Features {
			if feature.ID != parts[1] || feature.Sketch == nil || feature.Suppressed {
				continue
			}
			origin, u, n, ok := supportFrame(model, feature.Sketch.Support)
			if !ok {
				break
			}
			v := [3]float64{n[1]*u[2] - n[2]*u[1], n[2]*u[0] - n[0]*u[2], n[0]*u[1] - n[1]*u[0]}
			for _, entity := range feature.Sketch.Entities {
				if entity.ID != parts[2] || entity.Kind != "LINE" || entity.Start == nil || entity.End == nil || entity.Suppressed {
					continue
				}
				for i := 0; i < 3; i++ {
					p.Origin[i] = origin[i] + u[i]*entity.Start.X + v[i]*entity.Start.Y
					p.Direction[i] = u[i]*(entity.End.X-entity.Start.X) + v[i]*(entity.End.Y-entity.Start.Y)
				}
				return p, nil
			}
		}
	}
	return p, fmt.Errorf("%w: PATTERN_AXIS_UNAVAILABLE", ErrValidation)
}

func validateFeaturePattern(f Feature, earlier map[string]Feature) error {
	if f.Type != "SKETCH_PATTERN" && f.Type != "SOLID_PATTERN" {
		if f.Pattern != nil || f.ProfileMemberSlot != nil && !isSolidGenerator(f.Type) {
			return fmt.Errorf("%w: unexpected pattern definition", ErrValidation)
		}
		return nil
	}
	if f.Pattern == nil || f.Pattern.ID != f.ID || f.Sketch != nil || f.Pattern.Suppressed {
		return fmt.Errorf("%w: pattern identity required", ErrValidation)
	}
	p := f.Pattern
	for _, id := range patternReferenceFeatures(p.PatternDefinition) {
		if _, ok := earlier[id]; !ok {
			return fmt.Errorf("%w: pattern reference must precede pattern", ErrValidation)
		}
	}
	if axis := strings.Split(p.AxisEntityID, ":"); len(axis) == 3 && axis[0] == "SKETCH_LINE" {
		if source, ok := earlier[axis[1]]; !ok || source.Sketch == nil {
			return fmt.Errorf("%w: pattern axis must precede pattern", ErrValidation)
		}
	}
	source, ok := earlier[p.Source.FeatureID]
	if !ok || source.BodyID != p.Source.BodyID || source.Suppressed {
		return fmt.Errorf("%w: PATTERN_SOURCE_STAGE_UNAVAILABLE", ErrValidation)
	}
	if p.SourceKind != "FEATURE_DELTA" && p.StartFeatureID != "" {
		return fmt.Errorf("%w: PATTERN_RANGE_UNEXPECTED", ErrValidation)
	}
	if p.Kind == "MIRROR" && f.Type != "SOLID_PATTERN" {
		return fmt.Errorf("%w: MIRROR_REQUIRES_SOLID_FEATURE", ErrValidation)
	}
	if p.Kind == "MIRROR" {
		if (p.MirrorPlaneID == "") == (p.MirrorPlane == nil) {
			return fmt.Errorf("%w: MIRROR_PLANE_REQUIRED", ErrValidation)
		}
		if err := validateReferencedPlane(p.MirrorPlaneID, p.MirrorPlane, earlier); err != nil {
			return err
		}
	} else if p.MirrorPlaneID != "" || p.MirrorPlane != nil {
		return fmt.Errorf("%w: MIRROR_PLANE_UNEXPECTED", ErrValidation)
	}
	if f.Type == "SKETCH_PATTERN" {
		if p.SourceKind != "SKETCH_FRAME" || source.Sketch == nil {
			return fmt.Errorf("%w: pattern requires an upstream sketch", ErrValidation)
		}
	} else {
		if p.ResultMode != "" && p.ResultMode != "COMBINE" && p.ResultMode != "INDEPENDENT" {
			return fmt.Errorf("%w: invalid pattern result mode", ErrValidation)
		}
		if p.ResultMode == "INDEPENDENT" && f.Operation != "ADD" {
			return fmt.Errorf("%w: independent pattern requires additive source geometry", ErrValidation)
		}
		if p.Phase != 0 {
			return fmt.Errorf("%w: set solid pattern phase on the seed; seed relocation requires feature replay", ErrValidation)
		}
		for _, slot := range p.SkippedSlots {
			if slot == 0 {
				return fmt.Errorf("%w: solid seed member cannot be skipped", ErrValidation)
			}
		}
		if source.BodyID != f.BodyID {
			return fmt.Errorf("%w: solid pattern requires a stage in its own Body", ErrValidation)
		}
		if p.SourceKind == "GENERATOR_TOOL" {
			if !replicationSource(source).Tool {
				return fmt.Errorf("%w: PATTERN_REEXECUTION_UNSUPPORTED", ErrValidation)
			}
			if f.Operation != source.Operation && !(source.Operation == "NEW_BODY" && f.Operation == "ADD") {
				return fmt.Errorf("%w: pattern operation must match seed tool", ErrValidation)
			}
		} else if p.SourceKind == "FEATURE_DELTA" {
			start, valid := earlier[p.StartFeatureID]
			if !valid || start.Suppressed || start.BodyID != source.BodyID || start.Order > source.Order || !replicationSource(start).RangeStart || !isBodyFeature(source.Type) || f.Operation != "ADD" {
				return fmt.Errorf("%w: PATTERN_RANGE_INVALID", ErrValidation)
			}
			for _, member := range earlier {
				if member.BodyID != source.BodyID || member.Order < start.Order || member.Order > source.Order || member.Suppressed || !isBodyFeature(member.Type) {
					continue
				}
				if member.ID != start.ID && !replicationSource(member).RangeModifier {
					return fmt.Errorf("%w: PATTERN_RANGE_REEXECUTION_UNSUPPORTED", ErrValidation)
				}
			}
		} else if p.SourceKind != "BODY_STAGE" || !isBodyFeature(source.Type) || f.Operation != "ADD" {
			return fmt.Errorf("%w: invalid pattern source or operation", ErrValidation)
		}
	}
	return nil
}

// Resolve only an explicit member of an earlier spatial pattern. There is no
// fallback from a vanished slot to a neighboring member or a final Body tip.
func resolvePatternSketch(model PartModel, earlier map[string]Feature, id string, slot *int) (Feature, error) {
	source, ok := earlier[id]
	if !ok || source.Suppressed {
		return Feature{}, fmt.Errorf("%w: sketch source unavailable", ErrValidation)
	}
	if source.Type != "SKETCH_PATTERN" {
		if slot != nil || source.Sketch == nil {
			return Feature{}, fmt.Errorf("%w: invalid sketch member reference", ErrValidation)
		}
		return source, nil
	}
	if slot == nil || source.Pattern == nil {
		return Feature{}, fmt.Errorf("%w: explicit pattern member slot required", ErrValidation)
	}
	p := source.Pattern
	seed, ok := earlier[p.Source.FeatureID]
	if !ok || seed.Sketch == nil || seed.Suppressed {
		return Feature{}, fmt.Errorf("%w: pattern seed unavailable", ErrValidation)
	}
	definition, err := resolvedPatternDefinition(model, p.PatternDefinition)
	if err != nil {
		return Feature{}, err
	}
	placements, err := patternPlacements(definition)
	if err != nil {
		return Feature{}, err
	}
	for _, placement := range placements {
		if placement.Slot != *slot {
			continue
		}
		origin, u, n, valid := supportFrame(model, seed.Sketch.Support)
		if !valid {
			return Feature{}, fmt.Errorf("%w: pattern seed frame unavailable", ErrValidation)
		}
		vector := func(v [3]float64) [3]float64 {
			zero := placement.point([3]float64{})
			out := placement.point(v)
			for i := range out {
				out[i] -= zero[i]
			}
			return out
		}
		copySketch := *seed.Sketch
		copySketch.Support = SketchSupport{Type: "PATTERN_FRAME", Origin: placement.point(origin), XDirection: vector(u), Normal: vector(n), Status: "CONNECTED"}
		seed.ID = patternMemberID(p.ID, *slot, seed.ID)
		seed.Sketch = &copySketch
		return seed, nil
	}
	return Feature{}, fmt.Errorf("%w: PATTERN_MEMBER_MISSING: %s/%d", ErrValidation, id, *slot)
}

type PatternPlacement struct {
	Slot int
	// Row-major orthogonal isometry, in the same length unit as the source geometry.
	Matrix [12]float64
}

func visualizationSketchFeatures(model PartModel) []Feature {
	var result []Feature
	earlier := map[string]Feature{}
	for _, feature := range model.Features {
		earlier[feature.ID] = feature
		if feature.Suppressed {
			continue
		}
		if feature.Type != "SKETCH_PATTERN" {
			result = append(result, feature)
			continue
		}
		if feature.Pattern == nil {
			continue
		}
		placements, err := patternPlacements(feature.Pattern.PatternDefinition)
		if err != nil {
			continue
		}
		for _, placement := range placements {
			member, err := resolvePatternSketch(model, earlier, feature.ID, &placement.Slot)
			if err != nil {
				continue
			}
			entities, err := evaluatedSketchPatternEntities(*member.Sketch)
			if err != nil {
				continue
			}
			// Entity IDs stay in the seed namespace; member FeatureID scopes display
			// identity and downstream seam references still address the source geometry.
			// Keep the stable member identity for separate visual/selection groups.
			member.BodyID = feature.BodyID
			member.Visible = feature.Visible
			member.Sketch.Entities = entities
			member.Sketch.Patterns = nil
			member.Sketch.Constraints = nil
			result = append(result, member)
		}
	}
	return result
}

func patternMemberID(patternID string, slot int, sourceID string) string {
	data, _ := json.Marshal([]any{patternID, slot, sourceID})
	digest := sha256.Sum256(data)
	return "pattern-member-" + hex.EncodeToString(digest[:16])
}

func patternPlacements(p PatternDefinition) ([]PatternPlacement, error) {
	fail := func(message string) ([]PatternPlacement, error) {
		return nil, fmt.Errorf("%w: PATTERN_%s", ErrValidation, message)
	}
	if p.ID == "" || math.IsNaN(p.Count) || math.IsInf(p.Count, 0) || p.Count < 1 || p.Count > maxPatternMembers || math.Trunc(p.Count) != p.Count {
		return fail("COUNT_REQUIRES_INTEGER_1_TO_256")
	}
	for _, value := range append(append([]float64{p.Spacing, p.Angle, p.Phase}, p.Origin[:]...), p.Direction[:]...) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fail("NON_FINITE")
		}
	}
	if p.Kind != "LINEAR" && p.Kind != "CIRCULAR" && p.Kind != "MIRROR" {
		return fail("KIND_UNSUPPORTED")
	}
	if p.Kind == "MIRROR" && (p.Count != 2 || p.Distribution != "FIXED_STEP" || p.Phase != 0 || p.Angle != 0 || p.Spacing != 0 || p.Reversed || len(p.SkippedSlots) != 0 || p.AxisEntityID != "" || p.CenterReference != nil || p.DirectionReference != nil) {
		return fail("MIRROR_PARAMETERS_INVALID")
	}
	if p.Distribution != "FIXED_STEP" && p.Distribution != "TOTAL_SPAN" && !(p.Kind == "CIRCULAR" && p.Distribution == "FULL_CIRCLE") {
		return fail("DISTRIBUTION_UNSUPPORTED")
	}
	d := p.Direction
	norm := math.Hypot(math.Hypot(d[0], d[1]), d[2])
	if norm < 1e-12 {
		return fail("DIRECTION_REQUIRED")
	}
	for i := range d {
		d[i] /= norm
	}
	if p.Kind == "LINEAR" && (p.Spacing == 0 && p.Count > 1 || p.Phase != 0) {
		return fail("LINEAR_PARAMETERS_INVALID")
	}
	if p.Kind == "CIRCULAR" && p.Distribution != "FULL_CIRCLE" && p.Count > 1 {
		span := p.Angle
		if p.Distribution == "FIXED_STEP" {
			span *= p.Count - 1
		}
		if span == 0 || math.Abs(span) >= 360 {
			return fail("ANGLE_OVERLAPS_MEMBERS")
		}
	}
	skipped := map[int]bool{}
	for _, slot := range p.SkippedSlots {
		// Retain skipped slots outside the current count for a subsequent increase.
		if slot < 0 || slot >= maxPatternMembers || skipped[slot] {
			return fail("SKIPPED_SLOT_INVALID")
		}
		skipped[slot] = true
	}
	step := p.Spacing
	if p.Kind == "CIRCULAR" {
		step = p.Angle
	}
	if p.Distribution == "TOTAL_SPAN" && p.Count > 1 {
		step /= p.Count - 1
	}
	if p.Distribution == "FULL_CIRCLE" {
		step = 360 / p.Count
	}
	if p.Reversed {
		step = -step
	}
	result := make([]PatternPlacement, 0, int(p.Count))
	for slot := 0; slot < int(p.Count); slot++ {
		if skipped[slot] {
			continue
		}
		m := [12]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0}
		if p.Kind == "MIRROR" && slot == 1 {
			for row := 0; row < 3; row++ {
				for col := 0; col < 3; col++ {
					m[row*4+col] -= 2 * d[row] * d[col]
				}
				m[row*4+3] = 2 * d[row] * (d[0]*p.Origin[0] + d[1]*p.Origin[1] + d[2]*p.Origin[2])
			}
		} else if p.Kind == "MIRROR" {
			// Slot zero is the unchanged seed.
		} else if p.Kind == "LINEAR" {
			for row := 0; row < 3; row++ {
				m[row*4+3] = float64(slot) * step * d[row]
			}
		} else {
			a := (p.Phase + float64(slot)*step) * math.Pi / 180
			c, s := math.Cos(a), math.Sin(a)
			x, y, z := d[0], d[1], d[2]
			m = [12]float64{c + x*x*(1-c), x*y*(1-c) - z*s, x*z*(1-c) + y*s, 0, y*x*(1-c) + z*s, c + y*y*(1-c), y*z*(1-c) - x*s, 0, z*x*(1-c) - y*s, z*y*(1-c) + x*s, c + z*z*(1-c), 0}
			for row := 0; row < 3; row++ {
				m[row*4+3] = p.Origin[row]
				for col := 0; col < 3; col++ {
					m[row*4+3] -= m[row*4+col] * p.Origin[col]
				}
			}
		}
		for _, value := range m {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fail("TRANSFORM_NON_FINITE")
			}
		}
		result = append(result, PatternPlacement{slot, m})
	}
	if len(result) == 0 {
		return fail("ALL_MEMBERS_SKIPPED")
	}
	return result, nil
}

func (p PatternPlacement) point(value [3]float64) [3]float64 {
	var out [3]float64
	for row := range out {
		out[row] = p.Matrix[row*4+3]
		for col := range value {
			out[row] += p.Matrix[row*4+col] * value[col]
		}
	}
	return out
}

type patternParameter struct {
	slot, unit string
	value      float64
	dimension  modelcore.Dimension
}

func patternParameters(p *PatternDefinition) []patternParameter {
	if p == nil || p.Kind == "MIRROR" {
		return nil
	}
	result := []patternParameter{{"count", "1", p.Count, modelcore.Dimensionless}}
	if p.Kind == "LINEAR" {
		return append(result, patternParameter{"spacing", "mm", p.Spacing, modelcore.LengthDimension})
	}
	result = append(result, patternParameter{"phase", "deg", p.Phase, modelcore.AngleDimension})
	if p.Distribution != "FULL_CIRCLE" {
		result = append(result, patternParameter{"angle", "deg", p.Angle, modelcore.AngleDimension})
	}
	return result
}

func resolvePatternParameters(featureID, prefix string, p *PatternDefinition, values map[string]modelcore.Quantity) error {
	for _, parameter := range patternParameters(p) {
		q, ok := values["parameter:"+featureID+":"+prefix+parameter.slot]
		if !ok {
			return fmt.Errorf("%w: pattern parameter missing", ErrValidation)
		}
		v, err := quantityInDisplayUnit(q, parameter.unit)
		if err != nil {
			return err
		}
		switch parameter.slot {
		case "count":
			p.Count = v
		case "spacing":
			p.Spacing = v
		case "angle":
			p.Angle = v
		case "phase":
			p.Phase = v
		}
	}
	_, err := patternPlacements(*p)
	return err
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type datumFrame struct {
	origin, direction, u [3]float64
	plane                bool
}

func datumReferences(d *DatumTransform) []DatumReference {
	if d == nil {
		return nil
	}
	refs := []DatumReference{d.Source}
	if d.RotationAxis != nil {
		refs = append(refs, *d.RotationAxis)
	}
	if d.TranslationDirection != nil {
		refs = append(refs, *d.TranslationDirection)
	}
	return refs
}
func datumDefinitions(m *PartModel) map[string]*DatumTransform {
	defs := map[string]*DatumTransform{}
	for i := range m.DatumPlanes {
		if d := m.DatumPlanes[i].Definition; d != nil {
			defs[m.DatumPlanes[i].ID] = d
		}
	}
	for i := range m.DatumAxes {
		if d := m.DatumAxes[i].Definition; d != nil {
			defs[m.DatumAxes[i].ID] = d
		}
	}
	return defs
}
func datumReferenceDependency(ref DatumReference) modelcore.DependencyKey {
	if ref.Selection != nil {
		return modelcore.DependencyKey("feature:" + ref.FeatureID)
	}
	if ref.Kind == "SKETCH_LINE" {
		return modelcore.DependencyKey("feature:" + ref.FeatureID)
	}
	return modelcore.DependencyKey("datum:" + ref.EntityID)
}

func validateDatumReference(ref DatumReference) error {
	invalid := func() error { return fmt.Errorf("%w: DATUM_REFERENCE_INVALID", ErrValidation) }
	if ref.Kind == "TOPOLOGY" {
		if ref.Selection == nil || ref.SourceVersionID == "" || ref.EntityID != "" || ref.FeatureID == "" || ref.Axis != "" {
			return invalid()
		}
		if err := ref.Selection.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrValidation, err)
		}
		return nil
	}
	if ref.Selection != nil || ref.SourceVersionID != "" || ref.EntityID == "" {
		return invalid()
	}
	switch ref.Kind {
	case "SKETCH_LINE":
		if ref.FeatureID == "" || ref.Axis != "" {
			return invalid()
		}
	case "AXIS_SYSTEM":
		if ref.FeatureID != "" || (ref.Axis != "X" && ref.Axis != "Y" && ref.Axis != "Z") {
			return invalid()
		}
	case "PLANE", "AXIS", "POINT":
		if ref.FeatureID != "" || ref.Axis != "" {
			return invalid()
		}
	default:
		return invalid()
	}
	return nil
}

// Historical consumers may only use datums derived from their upstream stages.
// Walk the existing graph so indirect datum chains obey the same rule.
func validateDatumFeatureOrder(m PartModel, graph *modelcore.DependencyGraph) error {
	indices := map[modelcore.DependencyKey]int{}
	incoming := map[modelcore.DependencyKey][]modelcore.DependencyKey{}
	for i, f := range m.Features {
		indices[modelcore.DependencyKey("feature:"+f.ID)] = i
	}
	for _, edge := range graph.Edges {
		incoming[edge.Target] = append(incoming[edge.Target], edge.Source)
	}
	for target, index := range indices {
		seen := map[modelcore.DependencyKey]bool{}
		var visit func(modelcore.DependencyKey) error
		visit = func(key modelcore.DependencyKey) error {
			if seen[key] {
				return nil
			}
			seen[key] = true
			for _, source := range incoming[key] {
				if sourceIndex, ok := indices[source]; ok {
					if sourceIndex >= index {
						return fmt.Errorf("%w: DATUM_DOWNSTREAM_REFERENCE: %s reads %s", ErrValidation, target, source)
					}
				} else if strings.HasPrefix(string(source), "datum:") {
					if err := visit(source); err != nil {
						return err
					}
				}
			}
			return nil
		}
		for _, source := range incoming[target] {
			if strings.HasPrefix(string(source), "datum:") {
				if err := visit(source); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Numerical frames are derived on the existing Datum; definitions and stable
// parameter ASTs remain authoritative. Rotate first, then translate in the
// selected direction (or the rotated source direction when none is selected).
func datumRotate(v, axis [3]float64, angle float64) [3]float64 {
	c, s := math.Cos(angle), math.Sin(angle)
	cross := patternCross(axis, v)
	dot := dot3(axis, v)
	for i := range v {
		v[i] = v[i]*c + cross[i]*s + axis[i]*dot*(1-c)
	}
	return v
}
func (s *Service) resolveDatumDefinitions(ctx context.Context, documentID, requestID string, m *PartModel, limit int, targets ...string) error {
	defs := datumDefinitions(m)
	if len(defs) == 0 {
		return nil
	}
	for _, d := range defs {
		for _, ref := range datumReferences(d) {
			if err := validateDatumReference(ref); err != nil {
				return err
			}
			if ref.Kind == "POINT" {
				return fmt.Errorf("%w: DATUM_DIRECTION_REQUIRES_LINE", ErrValidation)
			}
		}
	}
	state := map[string]int{}
	cache := map[string]datumFrame{}
	indices := map[string]int{}
	for i, f := range m.Features {
		indices[f.ID] = i
	}
	var resolve func(DatumReference) (datumFrame, error)
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 2 {
			return nil
		}
		if state[id] == 1 {
			return fmt.Errorf("%w: DATUM_REFERENCE_CYCLE: %s", ErrValidation, id)
		}
		state[id] = 1
		d := defs[id]
		if d == nil {
			state[id] = 2
			return nil
		}
		for _, ref := range datumReferences(d) {
			source := ref.FeatureID
			if ref.Selection != nil {
				source = ref.FeatureID
			}
			if source != "" {
				index, ok := indices[source]
				if !ok {
					return fmt.Errorf("%w: DATUM_REFERENCE_MISSING: %s", ErrValidation, source)
				}
				if index >= limit {
					state[id] = 0
					return nil
				}
			}
		}
		base, err := resolve(d.Source)
		if err != nil {
			return err
		}
		if d.RotationAxis != nil {
			axis, e := resolve(*d.RotationAxis)
			if e != nil {
				return e
			}
			if axis.plane {
				return fmt.Errorf("%w: DATUM_ROTATION_REQUIRES_LINE", ErrValidation)
			}
			direction, ok := normalize3(axis.direction)
			if !ok {
				return fmt.Errorf("%w: DATUM_DIRECTION_DEGENERATE", ErrValidation)
			}
			relative := base.origin
			for i := range relative {
				relative[i] -= axis.origin[i]
			}
			relative = datumRotate(relative, direction, d.Angle*math.Pi/180)
			for i := range relative {
				base.origin[i] = axis.origin[i] + relative[i]
			}
			base.direction = datumRotate(base.direction, direction, d.Angle*math.Pi/180)
			base.u = datumRotate(base.u, direction, d.Angle*math.Pi/180)
		} else if d.Angle != 0 {
			return fmt.Errorf("%w: DATUM_ROTATION_AXIS_REQUIRED", ErrValidation)
		}
		direction := base.direction
		if d.TranslationDirection != nil {
			axis, e := resolve(*d.TranslationDirection)
			if e != nil {
				return e
			}
			if axis.plane {
				return fmt.Errorf("%w: DATUM_TRANSLATION_REQUIRES_LINE", ErrValidation)
			}
			direction = axis.direction
		}
		direction, ok := normalize3(direction)
		if !ok {
			return fmt.Errorf("%w: DATUM_DIRECTION_DEGENERATE", ErrValidation)
		}
		if !finite(d.Distance) || !finite(d.Angle) {
			return fmt.Errorf("%w: DATUM_TRANSFORM_NOT_FINITE", ErrValidation)
		}
		for i := range base.origin {
			base.origin[i] += direction[i] * d.Distance
		}
		found := false
		for i := range m.DatumPlanes {
			p := &m.DatumPlanes[i]
			if p.ID == id {
				if !base.plane {
					return fmt.Errorf("%w: DATUM_PLANE_SOURCE_REQUIRED", ErrValidation)
				}
				p.Origin, p.Normal, p.UDirection = base.origin, base.direction, base.u
				found = true
			}
		}
		for i := range m.DatumAxes {
			p := &m.DatumAxes[i]
			if p.ID == id {
				if base.plane {
					return fmt.Errorf("%w: DATUM_AXIS_SOURCE_REQUIRED", ErrValidation)
				}
				p.Origin, p.Direction = base.origin, base.direction
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: DATUM_REFERENCE_MISSING", ErrValidation)
		}
		cache[id] = base
		state[id] = 2
		recordEvaluation(ctx, "datum:"+id, false, "", 0)
		return nil
	}
	resolve = func(ref DatumReference) (datumFrame, error) {
		if ref.Kind == "PLANE" || ref.Kind == "AXIS" {
			if err := visit(ref.EntityID); err != nil {
				return datumFrame{}, err
			}
			if v, ok := cache[ref.EntityID]; ok {
				return v, nil
			}
		}
		switch ref.Kind {
		case "PLANE":
			for _, p := range m.DatumPlanes {
				if p.ID == ref.EntityID {
					return datumFrame{p.Origin, p.Normal, p.UDirection, true}, nil
				}
			}
		case "AXIS":
			for _, p := range m.DatumAxes {
				if p.ID == ref.EntityID {
					return datumFrame{origin: p.Origin, direction: p.Direction}, nil
				}
			}
		case "AXIS_SYSTEM", "POINT":
			for _, p := range m.AxisSystems {
				if p.ID == ref.EntityID {
					v := p.ZDirection
					switch ref.Axis {
					case "X":
						v = p.XDirection
					case "Y":
						v = p.YDirection
					case "Z", "":
					default:
						return datumFrame{}, fmt.Errorf("%w: DATUM_AXIS_INVALID", ErrValidation)
					}
					return datumFrame{origin: p.Origin, direction: v}, nil
				}
			}
		case "SKETCH_LINE":
			for _, f := range m.Features {
				if f.ID != ref.FeatureID || f.Sketch == nil || f.Suppressed {
					continue
				}
				for _, e := range patternSketchReferenceEntities(*f.Sketch) {
					if e.ID != ref.EntityID || e.Kind != "LINE" || e.Suppressed || e.Start == nil || e.End == nil {
						continue
					}
					o, u, n, ok := supportFrame(*m, f.Sketch.Support)
					if !ok {
						break
					}
					v := patternCross(n, u)
					direction := [3]float64{}
					for i := range o {
						o[i] += u[i]*e.Start.X + v[i]*e.Start.Y
						direction[i] = u[i]*(e.End.X-e.Start.X) + v[i]*(e.End.Y-e.Start.Y)
					}
					direction, ok = normalize3(direction)
					if ok {
						return datumFrame{origin: o, direction: direction}, nil
					}
				}
			}
		case "TOPOLOGY":
			if ref.Selection == nil || ref.SourceVersionID == "" || ref.Selection.SourceDocumentID != documentID {
				return datumFrame{}, fmt.Errorf("%w: DATUM_TOPOLOGY_REFERENCE_INCOMPLETE", ErrValidation)
			}
			index, ok := indices[ref.FeatureID]
			if !ok || index >= limit {
				return datumFrame{}, fmt.Errorf("%w: DATUM_DOWNSTREAM_REFERENCE", ErrValidation)
			}
			anchorIndex, ok := indices[ref.Selection.Anchor.FeatureID]
			if !ok || anchorIndex > index || m.Features[index].BodyID != ref.Selection.SourceBodyID || !isBodyFeature(m.Features[index].Type) || m.Features[index].Suppressed {
				return datumFrame{}, fmt.Errorf("%w: DATUM_TOPOLOGY_STAGE_INVALID", ErrValidation)
			}
			prefix := *m
			prefix.Features = append([]Feature(nil), m.Features[:index+1]...)
			key, err := s.evaluateBodyPrefix(ctx, requestID+"/datum/"+ref.FeatureID, prefix, ref.Selection.SourceBodyID)
			if err != nil {
				return datumFrame{}, err
			}
			r, _, err := s.resolveSelectionAgainstGeometry(ctx, documentID, ref.SourceVersionID, key, *ref.Selection)
			if err != nil {
				return datumFrame{}, err
			}
			if r.Status != modelcore.SelectionResolved || len(r.Candidates) != 1 {
				return datumFrame{}, fmt.Errorf("%w: DATUM_REFERENCE_UNRESOLVED: %s", ErrValidation, r.DiagnosticCode)
			}
			evidence := r.Candidates[0].Evidence
			if evidence.GeometryType == "LINE" && r.Candidates[0].Type == modelcore.PersistentTopologyEdge {
				return datumFrame{origin: evidence.Origin, direction: evidence.Direction}, nil
			}
			if evidence.GeometryType == "PLANE" && r.Candidates[0].Type == modelcore.PersistentTopologyFace {
				u, ok := stableSupportX(evidence.Direction, evidence.XDirection)
				if ok {
					return datumFrame{evidence.Origin, evidence.Direction, u, true}, nil
				}
			}
			return datumFrame{}, fmt.Errorf("%w: DATUM_SOURCE_TYPE_MISMATCH", ErrValidation)
		}
		return datumFrame{}, fmt.Errorf("%w: DATUM_REFERENCE_MISSING: %s", ErrValidation, ref.EntityID)
	}
	if len(targets) > 0 {
		return visit(targets[0])
	}
	for _, p := range m.DatumPlanes {
		if err := visit(p.ID); err != nil {
			return err
		}
	}
	for _, p := range m.DatumAxes {
		if err := visit(p.ID); err != nil {
			return err
		}
	}
	return nil
}

func datumParameterSources(m PartModel, d *DatumTransform, angle, distance string, ownerID string) (map[string]modelcore.ValueSource, error) {
	sources := map[string]modelcore.ValueSource{}
	if d == nil {
		return sources, nil
	}
	names := map[string]modelcore.ParameterBinding{}
	for _, p := range m.Parameters {
		names[p.Key] = modelcore.ParameterBinding{ParameterID: p.ParameterID, Dimension: p.Dimension}
	}
	for _, input := range []struct {
		slot, text, unit string
		value            float64
		dimension        modelcore.Dimension
	}{{"datum:angle", angle, "deg", d.Angle, modelcore.AngleDimension}, {"datum:distance", distance, "mm", d.Distance, modelcore.LengthDimension}} {
		source := modelcore.ValueSource{}
		if strings.TrimSpace(input.text) != "" {
			for _, p := range m.Parameters {
				if p.OwnerFeatureID == ownerID && p.PropertySlot == input.slot && p.Source.Expression != nil && p.Source.Expression.SourceText == input.text {
					sources[input.slot] = p.Source
					break
				}
			}
			if _, ok := sources[input.slot]; ok {
				continue
			}
			expr, err := modelcore.CompileExpression(input.text, names, input.dimension)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", ErrValidation, err)
			}
			source.Expression = &expr
		} else {
			q, err := modelcore.NewQuantity(input.value, input.unit)
			if err != nil {
				return nil, err
			}
			source.Literal = &q
		}
		sources[input.slot] = source
	}
	return sources, nil
}
func applyDatumParameterSources(m *PartModel, id string, sources map[string]modelcore.ValueSource) {
	ensureFeatureParameters(m)
	for i := range m.Parameters {
		p := &m.Parameters[i]
		if p.OwnerFeatureID == id {
			if source, ok := sources[p.PropertySlot]; ok {
				p.Source = source
			}
		}
	}
}
func appendEvaluatedDatumChanges(changes modelcore.ChangeSet, before, after PartModel) modelcore.ChangeSet {
	add := func(id, slot string, a, b any) {
		if reflect.DeepEqual(a, b) {
			return
		}
		address := modelcore.PropertyAddress{EntityID: id, SlotID: slot}
		for _, c := range changes.Changes {
			if c.Target == address {
				return
			}
		}
		c, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, a, b)
		changes.Changes = append(changes.Changes, c)
	}
	for _, b := range after.DatumPlanes {
		for _, a := range before.DatumPlanes {
			if a.ID == b.ID {
				add(b.ID, "datum.plane", a, b)
			}
		}
	}
	for _, b := range after.DatumAxes {
		for _, a := range before.DatumAxes {
			if a.ID == b.ID {
				add(b.ID, "datum.axis", a, b)
			}
		}
	}
	return changes
}

type editDatumPayload struct {
	Plane            *DatumPlane                      `json:"plane,omitempty"`
	Axis             *DatumAxis                       `json:"axis,omitempty"`
	ParameterSources map[string]modelcore.ValueSource `json:"parameterSources,omitempty"`
}

func applyEditDatum(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var m PartModel
	var p editDatumPayload
	if err := json.Unmarshal(modelJSON, &m); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&m)
	oldParameters := append([]modelcore.ParameterDefinition(nil), m.Parameters...)
	if (p.Plane == nil) == (p.Axis == nil) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum edit requires one definition", ErrValidation)
	}
	var before, after any
	var id, slot string
	if p.Plane != nil {
		id = p.Plane.ID
		slot = "datum.plane"
		index := slices.IndexFunc(m.DatumPlanes, func(v DatumPlane) bool { return v.ID == id })
		if index < 0 || isStandardDatumPlane(id) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum plane edit not allowed", ErrValidation)
		}
		o, u, n, err := validatedSupportFrame(p.Plane.Origin, p.Plane.UDirection, p.Plane.Normal)
		if err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		p.Plane.Origin, p.Plane.UDirection, p.Plane.Normal = o, u, n
		p.Plane.Size = m.DatumPlanes[index].Size
		p.Plane.Plane = "CUSTOM"
		before = m.DatumPlanes[index]
		m.DatumPlanes[index] = *p.Plane
		after = *p.Plane
	} else {
		id = p.Axis.ID
		slot = "datum.axis"
		index := slices.IndexFunc(m.DatumAxes, func(v DatumAxis) bool { return v.ID == id })
		d, ok := normalize3(p.Axis.Direction)
		if index < 0 || !ok || !finite(p.Axis.Origin[0]) || !finite(p.Axis.Origin[1]) || !finite(p.Axis.Origin[2]) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: datum axis edit not allowed", ErrValidation)
		}
		p.Axis.Direction = d
		before = m.DatumAxes[index]
		m.DatumAxes[index] = *p.Axis
		after = *p.Axis
	}
	applyDatumParameterSources(&m, id, p.ParameterSources)
	c, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: id, SlotID: slot}, before, after)
	changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{c}, []modelcore.DependencyKey{modelcore.DependencyKey("datum:" + id)}, oldParameters, m.Parameters)
	for _, old := range oldParameters {
		for _, next := range m.Parameters {
			if next.ParameterID == old.ParameterID && !reflect.DeepEqual(old.Source, next.Source) {
				c, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: next.ParameterID, SlotID: "parameter.source"}, old.Source, next.Source)
				changes = append(changes, c)
				seeds = append(seeds, modelcore.DependencyKey("parameter:"+next.ParameterID))
			}
		}
	}
	raw, err := json.Marshal(m)
	return raw, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}

func datumProjectionSource(m PartModel, ref DatumReference) (geometry.ExternalProjectionSource, error) {
	if err := validateDatumReference(ref); err != nil {
		return geometry.ExternalProjectionSource{}, err
	}
	source := geometry.ExternalProjectionSource{GeometryID: ref.EntityID, GeometryType: "AXIS"}
	if ref.Selection != nil || ref.FeatureID != "" || ref.SourceVersionID != "" {
		return source, fmt.Errorf("%w: DATUM_PROJECTION_REFERENCE_INVALID", ErrValidation)
	}
	found := false
	if ref.Kind == "AXIS" {
		for _, a := range m.DatumAxes {
			if a.ID == ref.EntityID {
				source.Origin, source.Direction = a.Origin, a.Direction
				found = true
			}
		}
	} else if ref.Kind == "AXIS_SYSTEM" || ref.Kind == "POINT" {
		for _, a := range m.AxisSystems {
			if a.ID == ref.EntityID {
				source.Origin = a.Origin
				switch ref.Axis {
				case "X":
					source.Direction = a.XDirection
				case "Y":
					source.Direction = a.YDirection
				case "Z", "":
					source.Direction = a.ZDirection
				default:
					return source, fmt.Errorf("%w: DATUM_AXIS_INVALID", ErrValidation)
				}
				if ref.Kind == "POINT" {
					source.GeometryType = "POINT"
					source.Direction = [3]float64{}
				}
				found = true
			}
		}
	}
	if !found {
		return source, fmt.Errorf("%w: DATUM_REFERENCE_MISSING", ErrValidation)
	}
	source.EvidenceDigest = resolvedDigest(source)
	return source, nil
}

func datumPreviewReferences(c modelcore.DomainCommand, m PartModel) *ReferenceGeometry {
	if c.TypeURI != typeCreateDatumPlane && c.TypeURI != typeCreateDatumAxis && c.TypeURI != typeEditDatum {
		return nil
	}
	r := referenceGeometry(m)
	return &r
}
func parameterPreviewCandidates(c modelcore.DomainCommand, m PartModel) []modelcore.ParameterDefinition {
	if c.TypeURI != typeEditParameter && c.TypeURI != typeCreateDatumPlane && c.TypeURI != typeCreateDatumAxis && c.TypeURI != typeEditDatum {
		return nil
	}
	return m.Parameters
}

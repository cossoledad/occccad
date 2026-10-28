package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/occccad/occccad/internal/geometry"
)

// Capabilities describe implemented consumers, not everything a Publication
// can legally expose (e.g. assembly SURFACE/BODY publications remain valid).
func contextConsumerSupported(kind, typ string) bool {
	return kind == "PARAMETER" && typ == "PARAMETER" || kind == "DATUM" && (typ == "PLANE" || typ == "AXIS") || kind == "SKETCH_EXTERNAL_GEOMETRY" && (typ == "POINT" || typ == "CURVE")
}
func contextPublicationSupported(p Publication) bool {
	if p.Type != "CURVE" {
		return true
	}
	if p.Resolution.NativeSketch != nil {
		kind := p.Resolution.NativeSketch.Geometry.Kind
		return kind == "LINE" || kind == "CIRCLE" || kind == "ARC"
	}
	kind := p.Resolution.GeometryKind
	return p.Resolution.TopologyKind == "EDGE" && (kind == "LINE" || kind == "CIRCLE") && p.Resolution.ParameterStart != nil && p.Resolution.ParameterEnd != nil && *p.Resolution.ParameterEnd > *p.Resolution.ParameterStart
}
func contextExternalID(inputID string) string { return macroID(inputID, "external") }
func nativeSketchPublication(model PartModel, target PublicationTarget, revision string) (PublicationResolution, error) {
	for _, f := range model.Features {
		if f.ID != target.SketchID || f.Sketch == nil || f.Suppressed {
			continue
		}
		if f.Sketch.Solve.Status == "CONFLICTING" || f.Sketch.Solve.Status == "FAILED" || f.Sketch.Solve.Status == "UNRESOLVED_EXTERNAL" {
			break
		}
		origin, x, n, ok := supportFrame(model, f.Sketch.Support)
		if !ok || f.Sketch.Support.Status == "FAILED_SUPPORT" {
			break
		}
		for _, e := range f.Sketch.Entities {
			if e.ID != target.EntityID || e.Suppressed || e.Kind != target.EntityKind {
				continue
			}
			if e.Kind != "POINT" && e.Kind != "LINE" && e.Kind != "CIRCLE" && e.Kind != "ARC" {
				break
			}
			source := &NativeSketchPublication{SketchID: f.ID, EntityID: e.ID, Origin: origin, XDirection: x, Normal: n, Geometry: SketchExternalGeometrySnapshot{Kind: e.Kind, Point: e.Point, Start: e.Start, End: e.End, Center: e.Center, Radius: e.Radius, StartAngle: e.StartAngle, EndAngle: e.EndAngle}}
			return nativePublicationDescriptor(PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revision, LengthUnit: "mm", NativeSketch: source, SourceDigest: resolvedDigest(source)})
		}
	}
	return PublicationResolution{}, fmt.Errorf("%w: published native sketch element is missing, suppressed, unsupported or has invalid support", ErrValidation)
}

// Both consumers describe geometry in the publication owner's coordinates.
// NativeSketch retains its 2D definition/frame; the outer analytic descriptor
// carries the same exact geometry for other existing Publication consumers.
func nativePublicationDescriptor(r PublicationResolution) (PublicationResolution, error) {
	n := r.NativeSketch
	r.XDirection, r.YDirection, r.ZDirection = n.XDirection, cross3(n.Normal, n.XDirection), n.Normal
	p, err := publicationProjectionSource(r, InstancePose{Rotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		return PublicationResolution{}, err
	}
	r.Origin, r.ZDirection, r.GeometryKind = p.Origin, p.Direction, p.GeometryType
	r.ParameterStart, r.ParameterEnd = p.ParameterStart, p.ParameterEnd
	if p.RadiusMM != nil {
		r.Radius = *p.RadiusMM
	}
	return r, nil
}
func publicationProjectionSource(r PublicationResolution, pose InstancePose) (geometry.ExternalProjectionSource, error) {
	source := geometry.ExternalProjectionSource{EvidenceDigest: r.SourceDigest, Origin: pointByPose(pose, r.Origin), Direction: rotateByPose(pose, r.ZDirection), XDirection: rotateByPose(pose, r.XDirection), GeometryType: r.GeometryKind}
	if r.NativeSketch != nil {
		native := r.NativeSketch
		e := native.Geometry
		source.SourceKind = "SKETCH_ENTITY"
		x := native.XDirection
		y := cross3(native.Normal, x)
		world := func(p *SketchPoint2) [3]float64 {
			v := native.Origin
			for i := range v {
				v[i] += x[i]*p.X + y[i]*p.Y
			}
			return pointByPose(pose, v)
		}
		switch e.Kind {
		case "POINT":
			if e.Point == nil {
				break
			}
			source.GeometryType = "POINT"
			source.Origin = world(e.Point)
			return source, nil
		case "LINE":
			if e.Start == nil || e.End == nil {
				break
			}
			a, b := world(e.Start), world(e.End)
			length := 0.
			for i := range a {
				source.Direction[i] = b[i] - a[i]
				length += source.Direction[i] * source.Direction[i]
			}
			length = math.Sqrt(length)
			if length <= 1e-7 {
				break
			}
			for i := range a {
				source.Direction[i] /= length
			}
			zero := 0.
			source.GeometryType = "LINE"
			source.Origin = a
			source.ParameterStart = &zero
			source.ParameterEnd = &length
			return source, nil
		case "CIRCLE", "ARC":
			if e.Center == nil || e.Radius <= 1e-7 {
				break
			}
			source.GeometryType = "CIRCLE"
			source.Origin = world(e.Center)
			source.Direction = rotateByPose(pose, native.Normal)
			source.XDirection = rotateByPose(pose, native.XDirection)
			radius := e.Radius
			source.RadiusMM = &radius
			start, end := 0., 2*math.Pi
			if e.Kind == "ARC" {
				start, end = e.StartAngle, e.EndAngle
			}
			source.ParameterStart = &start
			source.ParameterEnd = &end
			return source, nil
		}
		return source, fmt.Errorf("%w: native sketch projection contract invalid", ErrValidation)
	}
	if r.TopologyKind == "EDGE" && r.GeometryKey != "" && r.LocalID != 0 {
		source.GeometryID, source.GeometryKey, source.TopologyType, source.LocalID = r.GeometryID, r.GeometryKey, r.TopologyKind, r.LocalID
		source.ParameterStart, source.ParameterEnd = r.ParameterStart, r.ParameterEnd
		if r.Radius > 0 {
			radius := r.Radius
			source.RadiusMM = &radius
		}
		return source, nil
	}
	if r.GeometryKind == "POINT" || r.GeometryKind == "AXIS" {
		return source, nil
	}
	return source, fmt.Errorf("%w: this publication has no supported native sketch projection", ErrValidation)
}
func externalProjectionSnapshot(p geometry.ProjectedExternalGeometry) *SketchExternalGeometrySnapshot {
	s := &SketchExternalGeometrySnapshot{Kind: p.Kind, Radius: p.Radius, MajorRadius: p.MajorRadius, MinorRadius: p.MinorRadius, Rotation: p.Rotation, StartAngle: p.StartAngle, EndAngle: p.EndAngle}
	point := func(v [2]float64) *SketchPoint2 { return &SketchPoint2{X: v[0], Y: v[1]} }
	switch p.Kind {
	case "POINT":
		s.Point = point(p.Point)
	case "LINE":
		s.Start, s.End = point(p.Start), point(p.End)
	case "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC":
		s.Center = point(p.Center)
	}
	return s
}
func (s *Service) projectInputPublication(ctx context.Context, request string, model PartModel, sketchID, inputID string, r PublicationResolution, pose InstancePose) (*SketchExternalGeometrySnapshot, error) {
	for _, f := range model.Features {
		if f.ID != sketchID || f.Sketch == nil {
			continue
		}
		origin, x, n, ok := supportFrame(model, f.Sketch.Support)
		if !ok {
			break
		}
		source, err := publicationProjectionSource(r, pose)
		if err != nil {
			return nil, err
		}
		value, err := s.projectExternalGeometry(ctx, request, sketchID, contextExternalID(inputID), source, geometry.ExternalProjectionFrame{Origin: origin, XDirection: x, Normal: n})
		if err != nil {
			return nil, err
		}
		if value.Status != "CONNECTED" {
			return nil, fmt.Errorf("%w: %s: %s", ErrValidation, value.DiagnosticCode, value.Diagnostic)
		}
		return externalProjectionSnapshot(value), nil
	}
	return nil, fmt.Errorf("%w: context input sketch support unavailable", ErrValidation)
}
func captureContextInputDefault(model PartModel, input *ContextInput) error {
	if !contextConsumerSupported(input.Target.Kind, input.Type) {
		return fmt.Errorf("%w: CONTEXT_CONSUMER_UNSUPPORTED", ErrValidation)
	}
	if input.Default == nil {
		input.Default = &ContextInputDefault{}
	}
	switch input.Target.Kind {
	case "PARAMETER":
		for _, p := range model.Parameters {
			if p.ParameterID == input.Target.TargetID {
				source := p.Source
				input.Default.ParameterSource = &source
				return nil
			}
		}
	case "DATUM":
		if input.Type == "PLANE" {
			for _, p := range model.DatumPlanes {
				if p.ID == input.Target.TargetID {
					copy := p
					input.Default.Plane = &copy
					return nil
				}
			}
		}
		if input.Type == "AXIS" {
			for _, p := range model.DatumAxes {
				if p.ID == input.Target.TargetID {
					copy := p
					input.Default.Axis = &copy
					return nil
				}
			}
		}
	case "SKETCH_EXTERNAL_GEOMETRY":
		if input.Default.External != nil {
			return nil
		}
	}
	return fmt.Errorf("%w: context input requires an explicit local default", ErrValidation)
}
func installContextInput(model *PartModel, input ContextInput) error {
	if input.Target.Kind != "SKETCH_EXTERNAL_GEOMETRY" {
		return nil
	}
	if input.Default == nil || input.Default.External == nil {
		return fmt.Errorf("%w: missing local projection default", ErrValidation)
	}
	for i := range model.Features {
		f := &model.Features[i]
		if f.ID == input.Target.TargetID && f.Sketch != nil {
			id := contextExternalID(input.ID)
			for _, e := range f.Sketch.ExternalGeometry {
				if e.ID == id {
					return nil
				}
			}
			f.Sketch.ExternalGeometry = append(f.Sketch.ExternalGeometry, SketchExternalGeometry{ID: id, Name: input.Name, ContextInputID: input.ID, ProjectionKind: "ORTHOGONAL", SourceVersionID: "LOCAL_INPUT", Status: "CONNECTED", GeometryKind: input.Default.External.Kind, Snapshot: input.Default.External})
			return nil
		}
	}
	return fmt.Errorf("%w: input sketch missing", ErrValidation)
}

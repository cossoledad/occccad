package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
)

// Geometric supports are the definition. Frames and solver IDs are derived.
// CapturedX is an explicit body-local angular datum: two axial planes cannot
// define rotation about their common normal.
func motionFrameFromSupports(axis, plane geometry.AssemblyGeometry, x [3]float64) (InstancePose, error) {
	if axis.Kind != "AXIS" || plane.Kind != "PLANE" {
		return InstancePose{}, fmt.Errorf("%w: joint needs an axis and a planar support", ErrValidation)
	}
	z := axis.Direction
	nz := conflictNorm(z)
	np := conflictNorm(plane.Direction)
	if nz < 1e-12 || np < 1e-12 {
		return InstancePose{}, fmt.Errorf("%w: degenerate joint support", ErrValidation)
	}
	z = conflictScale(z, 1/nz)
	n := conflictScale(plane.Direction, 1/np)
	if conflictNorm(conflictCross(z, n)) > 1e-8 {
		return InstancePose{}, fmt.Errorf("%w: locating plane must be normal to joint axis", ErrValidation)
	}
	x = conflictSub(x, conflictScale(z, conflictDot(x, z)))
	nx := conflictNorm(x)
	if nx < 1e-10 {
		return InstancePose{}, fmt.Errorf("%w: invalid captured angular reference", ErrValidation)
	}
	x = conflictScale(x, 1/nx)
	y := conflictCross(z, x)
	origin := conflictSub(axis.Origin, conflictScale(z, conflictDot(conflictSub(axis.Origin, plane.Origin), n)/conflictDot(z, n)))
	// Rotation matrix columns x,y,z -> normalized quaternion (x,y,z,w).
	r := [3][3]float64{{x[0], y[0], z[0]}, {x[1], y[1], z[1]}, {x[2], y[2], z[2]}}
	q := [4]float64{}
	tr := r[0][0] + r[1][1] + r[2][2]
	if tr > 0 {
		v := math.Sqrt(tr+1) * 2
		q = [4]float64{(r[2][1] - r[1][2]) / v, (r[0][2] - r[2][0]) / v, (r[1][0] - r[0][1]) / v, v / 4}
	} else {
		i := 0
		if r[1][1] > r[i][i] {
			i = 1
		}
		if r[2][2] > r[i][i] {
			i = 2
		}
		j, k := (i+1)%3, (i+2)%3
		v := math.Sqrt(1+r[i][i]-r[j][j]-r[k][k]) * 2
		q[i] = v / 4
		q[j] = (r[j][i] + r[i][j]) / v
		q[k] = (r[k][i] + r[i][k]) / v
		q[3] = (r[k][j] - r[j][k]) / v
	}
	return InstancePose{Translation: origin, Rotation: normalizedInstanceRotation(q)}, nil
}
func (service *Service) resolveMechanismGeometry(ctx context.Context, model ProductModel, m Mechanism, capture bool) (Mechanism, error) {
	raw, _ := json.Marshal(m)
	_ = json.Unmarshal(raw, &m)
	resolver := newAssemblySupportResolver(withAssemblyReadCache(ctx), service, &model)

	for i := range m.Joints {
		j := &m.Joints[i]
		if j.Kind == "GROUND" {
			continue
		}
		if j.Second == nil {
			return m, fmt.Errorf("%w: joint has no second endpoint", ErrValidation)
		}
		if j.Kind == "RIGID" {
			continue
		}
		firstNormalSign := 1.0
		if (j.Kind == "PRISMATIC" || j.Kind == "CYLINDRICAL") && j.AxisDirection == "" {
			j.AxisDirection = "SAME"
		}
		if j.AxialDirection == "" {
			j.AxialDirection = "SAME"
		}
		for side, end := range []*JointEndpoint{&j.First, j.Second} {
			refs := []*AssemblyGeometryRef{}
			switch j.Kind {
			case "REVOLUTE", "PRISMATIC":
				refs = []*AssemblyGeometryRef{end.Axis, end.Plane}
			case "CYLINDRICAL":
				refs = []*AssemblyGeometryRef{end.Axis}
			case "PLANAR":
				refs = []*AssemblyGeometryRef{end.Plane}
			case "SPHERICAL":
				refs = []*AssemblyGeometryRef{end.Point}
			default:
				return m, fmt.Errorf("%w: unsupported joint", ErrValidation)
			}
			// Explicit frames are the numerical conformance API, not UI picks.
			allEmpty := true
			for _, ref := range refs {
				allEmpty = allEmpty && ref == nil
			}
			if allEmpty {
				continue
			}
			values := []geometry.AssemblyGeometry{}
			for _, ref := range refs {
				if ref == nil || ref.InstanceID != end.InstanceID {
					return m, fmt.Errorf("%w: incomplete or foreign joint support", ErrValidation)
				}
				if capture {
					if err := service.bindAssemblyPick(ctx, &model, ref); err != nil {
						return m, err
					}
				}
				v, err := resolver.resolve(*ref)
				if err != nil {
					return m, err
				}
				values = append(values, v)
			}
			if j.Kind == "SPHERICAL" {
				if values[0].Kind != "POINT" {
					return m, fmt.Errorf("%w: spherical joint requires exact points", ErrValidation)
				}
				end.Frame = InstancePose{Translation: values[0].Origin, Rotation: [4]float64{0, 0, 0, 1}}
				continue
			}
			var axis, plane geometry.AssemblyGeometry
			if j.Kind == "PLANAR" {
				plane = values[0]
				axis = geometry.AssemblyGeometry{Kind: "AXIS", Origin: plane.Origin, Direction: plane.Direction}
			} else {
				axis = values[0]
				plane = geometry.AssemblyGeometry{Kind: "PLANE", Origin: axis.Origin, Direction: axis.Direction}
				if j.Kind == "REVOLUTE" {
					plane = values[1]
				}
			}
			if axis.Kind != "AXIS" || plane.Kind != "PLANE" || conflictNorm(axis.Direction) < 1e-12 {
				return m, fmt.Errorf("%w: invalid joint supports", ErrValidation)
			}
			axis.Direction = conflictScale(axis.Direction, 1/conflictNorm(axis.Direction))
			if side == 1 && (j.Kind == "PRISMATIC" || j.Kind == "CYLINDRICAL") && j.AxisDirection == "OPPOSITE" {
				axis.Direction = conflictScale(axis.Direction, -1)
			}
			if j.Kind == "REVOLUTE" {
				if side == 0 {
					firstNormalSign = conflictDot(axis.Direction, plane.Direction)
				} else if (firstNormalSign*conflictDot(axis.Direction, plane.Direction) < 0) != (j.AxialDirection == "OPPOSITE") {
					axis.Direction = conflictScale(axis.Direction, -1)
				}
			}
			if j.Kind == "PRISMATIC" {
				if values[1].Kind != "PLANE" {
					return m, fmt.Errorf("%w: orientation needs a plane", ErrValidation)
				}
				x := conflictSub(values[1].Direction, conflictScale(axis.Direction, conflictDot(values[1].Direction, axis.Direction)))
				if conflictNorm(x) < 1e-10 {
					return m, fmt.Errorf("%w: orientation plane normal is parallel to the axis", ErrValidation)
				}
				end.CapturedX = &x
			} else if end.CapturedX == nil {
				if !capture {
					return m, fmt.Errorf("%w: missing captured frame basis", ErrValidation)
				}
				// Each endpoint captures its own Part-local basis. The accepted
				// coordinate origin is captured after physical alignment, so an
				// initially perpendicular second axis is still a valid input.
				x := [3]float64{1, 0, 0}
				if j.Kind == "CYLINDRICAL" && end.Axis != nil && end.Axis.Kind == "FRAME" && conflictNorm(values[0].XDirection) > 1e-10 {
					x = values[0].XDirection
					if math.Abs(conflictDot(x, axis.Direction)) > .9 {
						x = rotateByPose(InstancePose{Rotation: values[0].Rotation}, [3]float64{0, 1, 0})
					}
				} else if math.Abs(conflictDot(x, axis.Direction)) > .9 {
					x = [3]float64{0, 1, 0}
				}
				x = conflictSub(x, conflictScale(axis.Direction, conflictDot(x, axis.Direction)))
				x = conflictScale(x, 1/conflictNorm(x))
				end.CapturedX = &x
			}
			var err error
			end.Frame, err = motionFrameFromSupports(axis, plane, *end.CapturedX)
			if err != nil {
				return m, err
			}
		}
		if (j.Kind == "REVOLUTE" && j.First.Axis != nil || j.Kind == "PLANAR" && j.First.Plane != nil) && j.AxialOffset.Unit != "" {
			offset, err := motionValue(j.AxialOffset, false)
			if err != nil {
				return m, err
			}
			j.First.Frame.Translation = conflictSub(j.First.Frame.Translation, conflictScale(rotateByPose(j.First.Frame, [3]float64{0, 0, 1}), -offset))
		}
	}
	return m, nil
}
func (service *Service) bindKinematics(ctx context.Context, documentID, revisionID string, raw json.RawMessage, k KinematicsDefinitions) (KinematicsDefinitions, error) {
	if err := normalizeMotionRecordings(&k); err != nil {
		return k, err
	}
	var model ProductModel
	if e := json.Unmarshal(raw, &model); e != nil {
		return k, e
	}
	if err := validateMotionDriverChanges(model.Kinematics, k); err != nil {
		return k, err
	}
	for _, mapping := range k.Associations {
		if !slices.ContainsFunc(model.Kinematics.Associations, func(old MotionAssemblyMapping) bool { return reflect.DeepEqual(old, mapping) }) {
			return k, fmt.Errorf("%w: publishing mappings may only be created by Apply Motion Frame", ErrValidation)
		}
	}
	if len(k.Analyses) != 0 {
		return k, fmt.Errorf("%w: interference analysis is not part of mechanism editing", ErrValidation)
	}
	for i, m := range k.Mechanisms {
		if len(m.SupplementalConstraintIDs) != 0 {
			return k, fmt.Errorf("%w: mechanism uses only its own constraints", ErrValidation)
		}
		for _, j := range m.Joints {
			if !slices.Contains([]string{"GROUND", "REVOLUTE", "PRISMATIC", "CYLINDRICAL", "PLANAR", "RIGID", "SPHERICAL"}, j.Kind) {
				return k, fmt.Errorf("%w: unsupported joint", ErrValidation)
			}
			if len(j.Sources) != 0 {
				return k, fmt.Errorf("%w: importing assembly constraints is not supported", ErrValidation)
			}
			if err := validateJointSupports(j); err != nil {
				return k, err
			}
		}
		old := slices.IndexFunc(model.Kinematics.Mechanisms, func(prev Mechanism) bool { return prev.ID == m.ID })
		if old >= 0 {
			// A removed relationship has no continuous coordinate to carry. Keep
			// live winding intent; unknown IDs still fail normal validation.
			for id := range m.Coordinates {
				previouslyKnown := mechanismHasCoordinate(model.Kinematics.Mechanisms[old], id)
				stillKnown := mechanismHasCoordinate(m, id)
				if previouslyKnown && !stillKnown {
					delete(m.Coordinates, id)
				}
			}
		}
		driversFor := func(list []MotionDriver) []MotionDriver {
			out := []MotionDriver{}
			for _, d := range list {
				if d.MechanismID == m.ID {
					out = append(out, d)
				}
			}
			return out
		}
		if old >= 0 {
			previous := model.Kinematics.Mechanisms[old]
			metadata := m
			metadata.Name = previous.Name
			metadata.Joints = append([]MechanismJoint(nil), m.Joints...)
			for i, j := range metadata.Joints {
				if n := slices.IndexFunc(previous.Joints, func(v MechanismJoint) bool { return v.ID == j.ID }); n >= 0 {
					metadata.Joints[i].Name = previous.Joints[n].Name
				}
			}
			if reflect.DeepEqual(previous, metadata) && reflect.DeepEqual(driversFor(model.Kinematics.Drivers), driversFor(k.Drivers)) {
				continue
			}
		}
		if old >= 0 {
			m.ConstraintNameCounters = map[string]int{}
			for name, n := range model.Kinematics.Mechanisms[old].ConstraintNameCounters {
				m.ConstraintNameCounters[name] = n
			}
			for i, j := range m.Joints {
				for _, prev := range model.Kinematics.Mechanisms[old].Joints {
					if prev.ID == j.ID {
						if prev.Kind != j.Kind {
							return k, fmt.Errorf("%w: joint type is immutable", ErrValidation)
						}
						if prev.First.InstanceID != j.First.InstanceID {
							continue
						}
						if prev.Second != nil && (j.Second == nil || prev.Second.InstanceID != j.Second.InstanceID) {
							continue
						}
						m.Joints[i].Constraints = prev.Constraints
					}
				}
			}
		}
		if len(m.Poses) == 0 {
			m.Poses = map[string]InstancePose{}
			for _, v := range model.Instances {
				if slices.Contains(m.UnitIDs, v.ID) {
					m.Poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
				}
			}
		}
		if len(m.Joints) == 0 && len(m.UnitIDs) == 0 {
			m.Evaluation = &MechanismEvaluation{}
			k.Mechanisms[i] = m
			continue
		}
		// The authoritative solve occurs before CAS. Client poses are initial intent,
		// never accepted as a numerical witness. Every owning relation is retained.
		draftID := ""
		for _, j := range m.Joints {
			if old < 0 || !slices.ContainsFunc(model.Kinematics.Mechanisms[old].Joints, func(prev MechanismJoint) bool { return reflect.DeepEqual(prev, j) }) {
				draftID = j.ID
			}
		}
		result, err := service.PreviewMechanism(ctx, documentID, MechanismPreviewRequest{BaseRevisionID: revisionID, Mechanism: m, DraftJointID: draftID, Drivers: append([]MotionDriver{}, k.Drivers...)})
		if err != nil {
			return k, err
		}
		k.Mechanisms[i] = result.Mechanism
	}

	if err := bindMotionDrivers(&k); err != nil {
		return k, err
	}
	if err := service.validateChangedRecordings(ctx, documentID, revisionID, model, k); err != nil {
		return k, err
	}
	return k, nil
}

func capturedAssemblyDirection(v geometry.AssemblyGeometry, ref AssemblyGeometryRef) (geometry.AssemblyGeometry, error) {
	if ref.CapturedDirection == nil {
		return v, nil
	}
	d := *ref.CapturedDirection
	if v.Kind != "AXIS" || !finiteMotion(d[0]) || !finiteMotion(d[1]) || !finiteMotion(d[2]) || math.Abs(conflictNorm(d)-1) > 1e-8 || math.Abs(conflictDot(d, v.Direction)) > 1e-8 {
		return v, fmt.Errorf("%w: captured angular datum no longer matches its supporting axis", ErrValidation)
	}
	v.Direction = d
	return v, nil
}

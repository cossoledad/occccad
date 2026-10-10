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
	poses := map[string]InstancePose{}
	for _, v := range model.Instances {
		poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
	}
	for i := range m.Joints {
		j := &m.Joints[i]
		if j.Kind == "GROUND" {
			continue
		}
		if j.Second == nil {
			return m, fmt.Errorf("%w: joint has no second endpoint", ErrValidation)
		}
		var worldX [3]float64
		for side, end := range []*JointEndpoint{&j.First, j.Second} {
			if end.Axis == nil && end.Plane == nil {
				continue
			} // explicit local-frame definitions remain available to API clients.
			if end.Axis == nil || end.Plane == nil {
				return m, fmt.Errorf("%w: both geometric supports are required", ErrValidation)
			}
			for _, ref := range []*AssemblyGeometryRef{end.Axis, end.Plane} {
				if ref.InstanceID != end.InstanceID {
					return m, fmt.Errorf("%w: joint support belongs to another motion unit", ErrValidation)
				}
				if capture {
					if err := service.bindAssemblyPick(ctx, &model, ref); err != nil {
						return m, err
					}
				}
			}
			axis, e := resolver.resolve(*end.Axis)
			if e != nil {
				return m, fmt.Errorf("joint %s geometry: %w", j.ID, e)
			}
			plane, e := resolver.resolve(*end.Plane)
			if e != nil {
				return m, fmt.Errorf("joint %s geometry: %w", j.ID, e)
			}
			if end.CapturedX == nil {
				if !capture {
					return m, fmt.Errorf("%w: missing captured angular baseline", ErrValidation)
				}
				x := [3]float64{1, 0, 0}
				if side == 0 {
					if math.Abs(conflictDot(x, axis.Direction)) > .9 {
						x = [3]float64{0, 1, 0}
					}
					x = conflictSub(x, conflictScale(axis.Direction, conflictDot(x, axis.Direction)))
					x = conflictScale(x, 1/conflictNorm(x))
					worldX = rotateByPose(poses[end.InstanceID], x)
				} else {
					inv := inverseRelativePose(InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, poses[end.InstanceID])
					x = rotateByPose(inv, worldX)
				}
				end.CapturedX = &x
			}
			end.Frame, e = motionFrameFromSupports(axis, plane, *end.CapturedX)
			if e != nil {
				return m, e
			}
			if side == 0 {
				worldX = rotateByPose(poses[end.InstanceID], rotateByPose(end.Frame, [3]float64{1, 0, 0}))
			}
		}
		if j.First.Axis != nil && j.AxialOffset.Unit != "" {
			offset, e := motionValue(j.AxialOffset, false)
			if e != nil {
				return m, e
			}
			j.First.Frame.Translation = conflictSub(j.First.Frame.Translation, conflictScale(rotateByPose(j.First.Frame, [3]float64{0, 0, 1}), -offset))
		}
	}
	return m, nil
}
func (service *Service) bindKinematics(ctx context.Context, raw json.RawMessage, k KinematicsDefinitions) (KinematicsDefinitions, error) {
	var model ProductModel
	if e := json.Unmarshal(raw, &model); e != nil {
		return k, e
	}
	for _, mapping := range k.Associations {
		if !slices.ContainsFunc(model.Kinematics.Associations, func(old MotionAssemblyMapping) bool { return reflect.DeepEqual(old, mapping) }) {
			return k, fmt.Errorf("%w: publishing mappings may only be created by Apply Motion Frame", ErrValidation)
		}
	}
	for i, m := range k.Mechanisms {
		for j, joint := range m.Joints {
			unchanged := false
			for _, old := range model.Kinematics.Mechanisms {
				for _, prev := range old.Joints {
					if prev.ID == joint.ID && reflect.DeepEqual(prev, joint) {
						unchanged = true
					}
				}
			}
			if unchanged {
				continue
			}
			for _, source := range joint.Sources {
				idx := slices.IndexFunc(model.Constraints, func(c AssemblyConstraint) bool { return c.ID == source.ConstraintID })
				if idx < 0 || semanticConstraint(model.Constraints[idx]) != source.Baseline {
					return k, fmt.Errorf("%w: joint source changed before confirmation", ErrValidation)
				}
			}
			single := Mechanism{ID: m.ID, Joints: []MechanismJoint{joint}}
			resolved, e := service.resolveMechanismGeometry(ctx, model, single, true)
			if e != nil {
				return k, e
			}
			k.Mechanisms[i].Joints[j] = resolved.Joints[0]
		}
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

package workspace

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
)

// Editing poses and constraints belong to the mechanism. The Product is only
// a geometry/occurrence snapshot; its assembly equations never enter this input.
func mechanismPoseModel(model ProductModel, m Mechanism) (ProductModel, error) {
	input, err := cloneAssemblyProduct(model)
	if err != nil {
		return input, err
	}
	if len(m.Poses) > 0 {
		for i, v := range input.Instances {
			if !slices.Contains(m.UnitIDs, v.ID) {
				continue
			}
			p, ok := m.Poses[v.ID]
			if !ok || !validMotionPose(p) {
				return input, fmt.Errorf("%w: invalid mechanism editing pose", ErrValidation)
			}
			input.Instances[i].Translation, input.Instances[i].Rotation = p.Translation, p.Rotation
		}
	}
	input.Constraints = nil
	return input, nil
}

// Accepted mechanisms own only their members. Nonmembers always come from the
// current Product snapshot, including when adopting a historical run frame.
func mechanismMemberPoses(m Mechanism, poses map[string]InstancePose) map[string]InstancePose {
	result := make(map[string]InstancePose, len(m.UnitIDs))
	for _, id := range m.UnitIDs {
		if p, ok := poses[id]; ok {
			result[id] = p
		}
	}
	return result
}

func (service *Service) bindMechanismConstraints(ctx context.Context, model ProductModel, m Mechanism) (Mechanism, error) {
	resolver := newAssemblySupportResolver(ctx, service, &model)
	for i := range m.Joints {
		j := &m.Joints[i]
		var relations []AssemblyConstraint
		if j.Kind == "GROUND" {
			var p InstancePose
			found := false
			for _, c := range j.Constraints {
				if c.Kind == "FIX" && c.First.InstanceID == j.First.InstanceID && c.FixedPose != nil {
					p = *c.FixedPose
					found = true
				}
			}
			if !found {
				for _, v := range model.Instances {
					if v.ID == j.First.InstanceID {
						p = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
						found = true
					}
				}
			}
			if !found || !validMotionPose(p) {
				return m, fmt.Errorf("%w: missing Ground pose", ErrValidation)
			}
			relations = []AssemblyConstraint{{RelationRole: "ground", Kind: "FIX", FixMode: "SPACE", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: j.First.InstanceID}, FixedPose: &p}}
		} else if j.Second != nil {
			switch j.Kind {
			case "RIGID":
				for _, c := range j.Constraints {
					if c.Kind == "FIX_TOGETHER" && len(c.GroupRelations) == 2 {
						relations = []AssemblyConstraint{c}
					}
				}
				if len(relations) == 0 {
					poses := map[string]InstancePose{}
					for _, v := range model.Instances {
						poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
					}
					relative := inverseRelativePose(poses[j.Second.InstanceID], poses[j.First.InstanceID])
					relations = []AssemblyConstraint{{Kind: "FIX_TOGETHER", GroupMembers: []AssemblyGroupMember{{InstanceID: j.First.InstanceID}, {InstanceID: j.Second.InstanceID}}, GroupRelations: []AssemblyGroupRelation{{InstanceID: j.First.InstanceID, RelativePose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}, {InstanceID: j.Second.InstanceID, RelativePose: relative}}, GroupCaptureDigest: motionSemanticDigest(relative)}}
				}
				relations[0].RelationRole = "rigid"
			case "REVOLUTE", "PRISMATIC", "CYLINDRICAL":
				if j.First.Axis == nil || j.Second.Axis == nil {
					continue
				}
				a, b := *j.First.Axis, *j.Second.Axis
				axisB, err := resolver.resolve(b)
				if err != nil {
					return m, err
				}
				direction := "SAME"
				if conflictDot(axisB.Direction, rotateByPose(j.Second.Frame, [3]float64{0, 0, 1})) < 0 {
					direction = "OPPOSITE"
				}
				relations = []AssemblyConstraint{{RelationRole: "axis", Kind: "COINCIDENT", First: a, Second: &b, DirectionRelation: direction}}
				if j.Kind == "REVOLUTE" {
					if j.First.Plane == nil || j.Second.Plane == nil {
						continue
					}
					planeA, planeB := *j.First.Plane, *j.Second.Plane
					plane, err := resolver.resolve(planeA)
					if err != nil {
						return m, err
					}
					offset := 0.0
					if j.AxialOffset.Unit != "" {
						offset, err = motionValue(j.AxialOffset, false)
						if err != nil {
							return m, err
						}
					}
					z := rotateByPose(j.First.Frame, [3]float64{0, 0, 1})
					relations = append(relations, AssemblyConstraint{RelationRole: "axial-location", Kind: "DISTANCE", First: planeA, Second: &planeB, DirectionRelation: j.AxialDirection, DistanceRelation: selectedPlaneNormalV1, Value: -offset * conflictDot(z, plane.Direction)})
				}
				if j.Kind == "PRISMATIC" {
					if j.First.Plane == nil || j.Second.Plane == nil {
						continue
					}
					angle, err := motionValue(j.OrientationAngle, true)
					if err != nil {
						return m, err
					}
					planeA, planeB := *j.First.Plane, *j.Second.Plane
					axis := *j.First.Axis
					relations = append(relations, AssemblyConstraint{RelationRole: "orientation", Kind: "ANGLE", First: planeA, Second: &planeB, AngleRelation: "DIRECTED", AngleAxis: &axis, Value: wrappedMotionAngle(angle)})
				}
			case "PLANAR":
				if j.First.Plane == nil || j.Second.Plane == nil {
					continue
				}
				a, b := *j.First.Plane, *j.Second.Plane
				c := AssemblyConstraint{RelationRole: "plane", Kind: "COINCIDENT", First: a, Second: &b, DirectionRelation: j.AxialDirection}
				if j.AxialOffset.Unit != "" {
					offset, err := motionValue(j.AxialOffset, false)
					if err != nil {
						return m, err
					}
					if offset != 0 {
						c.Kind = "DISTANCE"
						c.DistanceRelation = selectedPlaneNormalV1
						c.Value = -offset
					}
				}
				relations = []AssemblyConstraint{c}
			case "SPHERICAL":
				if j.First.Point == nil || j.Second.Point == nil {
					continue
				}
				a, b := *j.First.Point, *j.Second.Point
				relations = []AssemblyConstraint{{RelationRole: "center", Kind: "COINCIDENT", First: a, Second: &b}}
			default:
				return m, fmt.Errorf("%w: unsupported joint", ErrValidation)
			}
		} else {
			continue
		}

		for n := range relations {
			c := &relations[n]
			role := mechanismConstraintRole(*c)
			c.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte(m.ID+"/"+j.ID+"/"+role)).String()
			for _, previous := range j.Constraints {
				if mechanismConstraintRole(previous) == role {
					c.ID, c.Name, c.QuantityParameter = previous.ID, previous.Name, previous.QuantityParameter
				}
			}
			c.DefinitionVersion = 2
			c.Mode = "DRIVING"
			c.ConnectionID = j.ID
			if err := canonicalAssemblyDefinition(c, "", ""); err != nil {
				return m, err
			}
			assignConstraintName(c, &m.ConstraintNameCounters, nil)
			if err := service.validatePublicAssemblySupports(ctx, &model, c); err != nil {
				return m, err
			}
			c.EvaluationStatus = "VERIFIED"
		}
		j.Constraints = relations
	}
	quantityModel := model
	quantityModel.Constraints = nil
	for _, j := range m.Joints {
		quantityModel.Constraints = append(quantityModel.Constraints, j.Constraints...)
	}
	for i, c := range quantityModel.Constraints {
		if c.Kind == "DISTANCE" || c.Kind == "ANGLE" {
			literal := ""
			if err := editAssemblyQuantity(&quantityModel, i, &literal, "", c.Value); err != nil {
				return m, err
			}
		}
	}
	for i := range m.Joints {
		for n, c := range m.Joints[i].Constraints {
			for _, resolved := range quantityModel.Constraints {
				if resolved.ID == c.ID {
					m.Joints[i].Constraints[n] = resolved
				}
			}
		}
	}
	return m, nil
}

// Reuse the ordinary Product compiler. Joint Frames only define the remaining
// motion coordinate, not a second implementation of the same hard relations.
func appendMechanismConstraints(input *ProductModel, m Mechanism) {
	for _, j := range m.Joints {
		if j.Kind != "RIGID" {
			input.Constraints = append(input.Constraints, j.Constraints...)
		}
	}
}

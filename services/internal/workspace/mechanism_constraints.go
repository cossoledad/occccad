package workspace

import (
	"context"
	"fmt"
)

// Editing poses and constraints belong to the mechanism. The Product is only
// a geometry/occurrence snapshot; its assembly equations never enter this input.
func mechanismPoseModel(model ProductModel, m Mechanism) (ProductModel, error) {
	input, err := cloneAssemblyProduct(model)
	if err != nil {
		return input, err
	}
	if len(m.Poses) > 0 {
		if len(m.Poses) != len(input.Instances) {
			return input, fmt.Errorf("%w: incomplete mechanism editing pose", ErrValidation)
		}
		for i, v := range input.Instances {
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
			relations = []AssemblyConstraint{{ID: j.ID + "/ground", Name: j.Name, Kind: "FIX", FixMode: "SPACE", First: AssemblyGeometryRef{Kind: "BODY", InstanceID: j.First.InstanceID}, FixedPose: &p}}
		} else if j.First.Axis != nil && j.Second != nil && j.Second.Axis != nil && j.First.Plane != nil && j.Second.Plane != nil {
			a, b := *j.First.Axis, *j.Second.Axis
			axis := AssemblyConstraint{ID: j.ID + "/axis", Name: "同心", Kind: "COINCIDENT", First: a, Second: &b, DirectionRelation: "SAME"}
			planeA, planeB := *j.First.Plane, *j.Second.Plane
			plane, err := resolver.resolve(planeA)
			if err != nil {
				return m, err
			}
			z := rotateByPose(j.First.Frame, [3]float64{0, 0, 1})
			offset, err := motionValue(j.AxialOffset, false)
			if j.AxialOffset.Unit == "" {
				offset, err = 0, nil
			}
			if err != nil {
				return m, err
			}
			location := AssemblyConstraint{ID: j.ID + "/axial-location", Name: "偏移", Kind: "DISTANCE", First: planeA, Second: &planeB, DirectionRelation: "UNORIENTED", DistanceRelation: selectedPlaneNormalV1, Value: -offset * conflictDot(z, plane.Direction)}
			relations = []AssemblyConstraint{axis, location}
		} else {
			continue
		} // pure frame fixtures are used by numerical conformance tests only
		for n := range relations {
			c := &relations[n]
			c.DefinitionVersion = 2
			c.Mode = "DRIVING"
			c.ConnectionID = j.ID
			if err := canonicalAssemblyDefinition(c, "", ""); err != nil {
				return m, err
			}
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
		if c.Kind == "DISTANCE" {
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
		input.Constraints = append(input.Constraints, j.Constraints...)
	}
}

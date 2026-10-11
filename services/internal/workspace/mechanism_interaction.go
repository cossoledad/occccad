package workspace

import (
	"fmt"
	"slices"
)

func mechanismInteractionModel(product ProductModel, id string, sources []string) (ProductModel, error) {
	if len(sources) > 16 {
		return product, fmt.Errorf("%w: application source scope limit", ErrValidation)
	}
	model, err := cloneAssemblyProduct(product)
	if err != nil {
		return model, err
	}
	order := append([]string(nil), sources...)
	if !slices.Contains(order, id) {
		order = append(order, id)
	}
	for _, source := range order {
		index := slices.IndexFunc(model.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == source })
		if index < 0 {
			return model, fmt.Errorf("%w: missing mechanism", ErrValidation)
		}
		m := model.Kinematics.Mechanisms[index]
		if len(m.UnitIDs) > 0 && len(m.Poses) != len(m.UnitIDs) {
			return model, fmt.Errorf("%w: missing accepted mechanism poses", ErrValidation)
		}
		model, err = mechanismPoseModel(model, model.Kinematics.Mechanisms[index])
		if err != nil {
			return model, err
		}
	}
	index := slices.IndexFunc(model.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == id })
	appendMechanismConstraints(&model, model.Kinematics.Mechanisms[index])
	return model, nil
}

func mechanismInteractionCandidate(original, solved ProductModel, id, bodyID string, coordinates map[string]float64) (ProductModel, error) {
	next, err := cloneAssemblyProduct(original)
	if err != nil {
		return next, err
	}
	index := slices.IndexFunc(next.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == id })
	if index < 0 || !slices.ContainsFunc(next.Instances, func(v ProductInstance) bool { return v.ID == bodyID }) {
		return next, fmt.Errorf("%w: missing mechanism motion unit", ErrValidation)
	}
	m := &next.Kinematics.Mechanisms[index]
	if !slices.Contains(m.UnitIDs, bodyID) {
		m.UnitIDs = append(m.UnitIDs, bodyID)
		m.Evaluation = nil
	}
	slices.Sort(m.UnitIDs)
	poses := map[string]InstancePose{}
	for _, v := range solved.Instances {
		poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
	}
	m.Poses = mechanismMemberPoses(*m, poses)
	if len(m.Poses) != len(m.UnitIDs) {
		return next, fmt.Errorf("%w: incomplete mechanism move", ErrValidation)
	}
	for _, p := range m.Poses {
		if !validMotionPose(p) {
			return next, fmt.Errorf("%w: invalid mechanism move", ErrValidation)
		}
	}
	// The qualified drag transports both placement and continuous joint state.
	// Reusing the base coordinates would fail final acceptance after any motion
	// and would lose winding when a moved joint crosses a principal-angle seam.
	m.Coordinates, err = verifyMechanismJoints(*m, poses, coordinates, "", 0)
	if err != nil {
		return next, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	if err = motionLimits(*m, m.Coordinates); err != nil {
		return next, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return next, nil
}

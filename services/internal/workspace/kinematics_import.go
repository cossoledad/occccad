package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
)

// Proposals are read-only. Confirmation saves a joint definition and references
// its source equations; those references are never re-added as joint equations.
func (service *Service) MotionJointProposals(ctx context.Context, doc string) ([]MechanismJoint, error) {
	view, e := service.GetDocument(ctx, doc)
	if e != nil {
		return nil, e
	}
	if view.Product == nil {
		return nil, fmt.Errorf("%w: owning Product required", ErrValidation)
	}
	model := *view.Product
	resolver := newAssemblySupportResolver(withAssemblyReadCache(ctx), service, &model)
	out := []MechanismJoint{}
	identity := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	for _, c := range model.Constraints {
		if c.Suppressed || c.Mode == "MEASURED" {
			continue
		}
		if c.Kind == "FIX" && c.FixMode != "RELATIVE" {
			out = append(out, MechanismJoint{ID: uuid.NewString(), Name: c.Name, Kind: "GROUND", First: JointEndpoint{InstanceID: c.First.InstanceID, Frame: identity}, Direction: 1, Sources: []JointSource{{c.ID, semanticConstraint(c)}}})
			continue
		}
		if c.Second == nil || (c.Kind != "COINCIDENT" && c.Kind != "CONCENTRIC") {
			continue
		}
		a, e1 := resolver.resolve(c.First)
		b, e2 := resolver.resolve(*c.Second)
		if e1 != nil || e2 != nil || a.Kind != "AXIS" || b.Kind != "AXIS" {
			continue
		}
		for _, loc := range model.Constraints {
			if loc.Suppressed || loc.Mode == "MEASURED" || loc.Kind != "DISTANCE" || loc.Second == nil || loc.First.InstanceID != c.First.InstanceID || loc.Second.InstanceID != c.Second.InstanceID {
				continue
			}
			pa, e1 := resolver.resolve(loc.First)
			pb, e2 := resolver.resolve(*loc.Second)
			if e1 != nil || e2 != nil || pa.Kind != "PLANE" || pb.Kind != "PLANE" {
				continue
			}
			if loc.DistanceRelation != selectedPlaneNormalV1 || math.Abs(conflictDot(pa.Direction, a.Direction)) < 1-1e-8 {
				continue
			}
			axisA, axisB, planeA, planeB := c.First, *c.Second, loc.First, *loc.Second
			j := MechanismJoint{ID: uuid.NewString(), Name: "旋转接合 — " + c.Name, Kind: "REVOLUTE", First: JointEndpoint{InstanceID: c.First.InstanceID, Frame: identity, Axis: &axisA, Plane: &planeA}, Second: &JointEndpoint{InstanceID: c.Second.InstanceID, Frame: identity, Axis: &axisB, Plane: &planeB}, Zero: MotionQuantity{Unit: "deg"}, Direction: 1, AxialOffset: MotionQuantity{Value: -loc.Value / conflictDot(pa.Direction, a.Direction), Unit: "mm"}, Sources: []JointSource{{c.ID, semanticConstraint(c)}, {loc.ID, semanticConstraint(loc)}}}
			out = append(out, j)
		}
	}
	return out, nil
}

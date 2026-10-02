package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"math"
)

type conflictProof struct {
	IDs    []string
	Reason string
}

// Certificates use only frozen definitions/descriptor parameters. Numerical
// failure, rank dependence and preference failure are never UNSAT certificates.
func conflictAnalyticProof(input AssemblySolveManifest) *conflictProof {
	geo := map[string]geometry.AssemblyGeometry{}
	for _, g := range input.Geometry {
		geo[g.ID] = g
	}
	lt, at := input.SolverProfile.LengthTolerance, input.SolverProfile.AngleTolerance
	for i, a := range input.Constraints {
		if a.Mode == "MEASURED" {
			continue
		}
		if a.Kind == "CONTACT" {
			if reason := contactDefinitionInfeasibility(a, geo[a.FirstGeometryID], geo[a.SecondGeometryID]); reason != "" {
				return &conflictProof{[]string{a.ID}, reason}
			}
		}
		for _, b := range input.Constraints[i+1:] {
			if b.Mode == "MEASURED" {
				continue
			}
			same := a.FirstBodyID == b.FirstBodyID && a.SecondBodyID == b.SecondBodyID && a.FirstGeometryID == b.FirstGeometryID && a.SecondGeometryID == b.SecondGeometryID
			reversed := a.FirstBodyID == b.SecondBodyID && a.SecondBodyID == b.FirstBodyID && a.FirstGeometryID == b.SecondGeometryID && a.SecondGeometryID == b.FirstGeometryID
			if a.Kind == "DISTANCE" && b.Kind == "DISTANCE" && ((same && a.DistanceRelation == b.DistanceRelation) || (reversed && conflictUnsigned(a.DistanceRelation) && conflictUnsigned(b.DistanceRelation))) && math.Abs(a.Value-b.Value) > 2*lt {
				return &conflictProof{[]string{a.ID, b.ID}, "identical exact distance function has incompatible disjoint target intervals"}
			}
			if (same || reversed) && ((a.Kind == "PARALLEL" && b.Kind == "PERPENDICULAR") || (a.Kind == "PERPENDICULAR" && b.Kind == "PARALLEL")) && at < 0.25 {
				return &conflictProof{[]string{a.ID, b.ID}, "same unit directions cannot be simultaneously parallel and perpendicular"}
			}
			if a.Kind == "FIX" && b.Kind == "FIX" && a.FirstBodyID == b.FirstBodyID && a.FixedPose != nil && b.FixedPose != nil && !conflictPoseClose(*a.FixedPose, *b.FixedPose, 2*lt, 2*at) {
				return &conflictProof{[]string{a.ID, b.ID}, "same motion unit has incompatible fixed poses"}
			}
		}
	}
	// Once every participating body is physically grounded by real SPACE Fix,
	// a failed independently evaluable relation has no movable degrees of freedom.
	fixed := map[string]geometry.AssemblyPose{}
	fixIDs := map[string]string{}
	for _, c := range input.Constraints {
		if c.Kind == "FIX" && c.Mode != "MEASURED" && c.FixedPose != nil {
			fixed[c.FirstBodyID] = *c.FixedPose
			fixIDs[c.FirstBodyID] = c.ID
		}
	}
	for _, c := range input.Constraints {
		if c.Kind == "FIX" || c.Mode == "MEASURED" {
			continue
		}
		// Infinite line distances can change discontinuously under arbitrarily
		// small direction perturbations; projected angles can be ill-conditioned.
		// They do not receive a fixed-pose UNSAT certificate here.
		ga, gb := geo[c.FirstGeometryID], geo[c.SecondGeometryID]
		if c.Kind == "ANGLE" || (c.Kind == "DISTANCE" && (conflictLine(ga.Kind) || conflictLine(gb.Kind))) {
			continue
		}
		ids := []string{c.FirstBodyID, c.SecondBodyID, c.AngleReferenceBodyID}
		covered := true
		proofIDs := []string{c.ID}
		for _, id := range ids {
			if id == "" {
				continue
			}
			if _, ok := fixed[id]; !ok {
				covered = false
			}
			if fixIDs[id] != "" {
				proofIDs = append(proofIDs, fixIDs[id])
			}
		}
		if covered {
			worldA, worldB := conflictWorld(ga, fixed[c.FirstBodyID]), conflictWorld(gb, fixed[c.SecondBodyID])
			// Conservative propagation of the real Fix acceptance tolerances:
			// translation and bounded rotation of local lever arms/support normals.
			margin := 4*lt + 4*at*(conflictNorm(ga.Origin)+conflictNorm(gb.Origin)+conflictNorm(conflictSub(worldA.Origin, worldB.Origin))+1)
			known, satisfied, _ := conflictGeometryRelation(c, geo, fixed, margin, 4*at)
			if known && !satisfied {
				return &conflictProof{uniqueGroupIDs(proofIDs), "independent geometry contradicts frozen physical Fix poses"}
			}
		}
	}
	return nil
}

func conflictUnsigned(s string) bool { return s == "" || s == "UNSIGNED" }
func conflictSub(a, b [3]float64) [3]float64 {
	return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}
func conflictDot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func conflictNorm(a [3]float64) float64   { return math.Sqrt(conflictDot(a, a)) }
func conflictCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func conflictScale(a [3]float64, s float64) [3]float64 {
	return [3]float64{a[0] * s, a[1] * s, a[2] * s}
}
func conflictQuaternionAngle(a, b [4]float64) float64 {
	a, b = normalizedInstanceRotation(a), normalizedInstanceRotation(b)
	dot := 0.
	for i := 0; i < 4; i++ {
		dot += a[i] * b[i]
	}
	return 2 * math.Acos(math.Min(1, math.Abs(dot)))
}
func conflictPoseClose(a, b geometry.AssemblyPose, lt, at float64) bool {
	return conflictNorm(conflictSub(a.Translation, b.Translation)) <= lt && conflictQuaternionAngle(a.Rotation, b.Rotation) <= at
}

func conflictWorld(g geometry.AssemblyGeometry, p geometry.AssemblyPose) geometry.AssemblyGeometry {
	pose := InstancePose{Translation: p.Translation, Rotation: p.Rotation}
	g.Origin = pointByPose(pose, g.Origin)
	g.Direction = rotateByPose(pose, g.Direction)
	g.XDirection = rotateByPose(pose, g.XDirection)
	return g
}
func conflictLine(k string) bool { return k == "AXIS" || k == "CYLINDER" }

// This oracle is independent of Worker residuals/status. Unsupported certificate
// primitives remain UNKNOWN, rather than accepting a Converged enum as proof.
func conflictIndependentWitness(input AssemblySolveManifest, result geometry.AssemblySolve) (bool, string) {
	poses := map[string]geometry.AssemblyPose{}
	geo := map[string]geometry.AssemblyGeometry{}
	for _, b := range result.Bodies {
		for _, v := range b.Pose.Translation {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false, "nonfinite witness pose"
			}
		}
		n := 0.
		for _, v := range b.Pose.Rotation {
			n += v * v
		}
		if math.Abs(n-1) > 1e-8 {
			return false, "invalid witness quaternion"
		}
		poses[b.ID] = b.Pose
	}
	for _, b := range input.Bodies {
		if _, ok := poses[b.ID]; !ok {
			return false, "solver omitted frozen body"
		}
	}
	for _, g := range input.Geometry {
		geo[g.ID] = g
	}
	constraints := append([]geometry.AssemblyConstraint{}, input.Constraints...)
	for _, stage := range input.GroupStages {
		for _, group := range stage.Groups {
			if group.CapturePending {
				return false, "group capture pending: no frozen group relation certificate"
			}
			links, err := groupRigidConstraints(group.GroupID, group.MemberIDs, group.CapturedRelations)
			if err != nil {
				return false, err.Error()
			}
			constraints = append(constraints, links...)
		}
	}
	for _, c := range constraints {
		if c.Mode == "MEASURED" {
			continue
		}
		known, ok, reason := conflictGeometryRelation(c, geo, poses, input.SolverProfile.LengthTolerance, input.SolverProfile.AngleTolerance)
		if !known || !ok {
			return false, c.ID + ": " + reason
		}
	}
	return true, "independent final-pose geometry satisfies every frozen hard relation"
}

func conflictGeometryRelation(c geometry.AssemblyConstraint, local map[string]geometry.AssemblyGeometry, poses map[string]geometry.AssemblyPose, lt, at float64) (bool, bool, string) {
	if c.Kind == "FIX" {
		if c.FixedPose == nil {
			return false, false, "missing frozen Fix pose"
		}
		return true, conflictPoseClose(poses[c.FirstBodyID], *c.FixedPose, lt, at), "fixed pose mismatch"
	}
	if c.Kind == "RIGID" {
		if c.FixedPose == nil {
			return false, false, "missing frozen rigid relation"
		}
		a, b := poses[c.FirstBodyID], poses[c.SecondBodyID]
		relative := inverseRelativePose(InstancePose{Translation: a.Translation, Rotation: a.Rotation}, InstancePose{Translation: b.Translation, Rotation: b.Rotation})
		return true, conflictPoseClose(geometry.AssemblyPose{Translation: relative.Translation, Rotation: relative.Rotation}, *c.FixedPose, lt, at), "rigid relation mismatch"
	}
	a, okA := local[c.FirstGeometryID]
	b, okB := local[c.SecondGeometryID]
	if !okA || !okB {
		return false, false, "missing exact support descriptor"
	}
	a = conflictWorld(a, poses[c.FirstBodyID])
	b = conflictWorld(b, poses[c.SecondBodyID])
	delta := conflictSub(a.Origin, b.Origin)
	parallel := conflictNorm(conflictCross(a.Direction, b.Direction)) <= at
	directional := func() bool {
		dot := conflictDot(a.Direction, b.Direction)
		switch c.DirectionRelation {
		case "SAME":
			return conflictNorm(conflictSub(a.Direction, b.Direction)) <= at
		case "OPPOSITE":
			return conflictNorm(conflictSub(a.Direction, conflictScale(b.Direction, -1))) <= at
		default:
			return math.Abs(math.Abs(dot)-1) <= at
		}
	}
	distance := 0.
	knownDistance := true
	constant := true
	switch {
	case a.Kind == "POINT" && b.Kind == "POINT":
		distance = conflictNorm(delta)
	case a.Kind == "POINT" && conflictLine(b.Kind):
		distance = conflictNorm(conflictCross(delta, b.Direction))
	case conflictLine(a.Kind) && b.Kind == "POINT":
		distance = conflictNorm(conflictCross(delta, a.Direction))
	case a.Kind == "POINT" && b.Kind == "PLANE":
		distance = conflictDot(delta, b.Direction)
	case a.Kind == "PLANE" && b.Kind == "POINT":
		distance = conflictDot(delta, a.Direction)
	case conflictLine(a.Kind) && conflictLine(b.Kind):
		normal := conflictCross(a.Direction, b.Direction)
		if conflictNorm(normal) <= 1e-12 {
			distance = conflictNorm(conflictCross(delta, b.Direction))
		} else {
			distance = math.Abs(conflictDot(delta, normal)) / conflictNorm(normal)
		}
	case a.Kind == "PLANE" && b.Kind == "PLANE":
		constant = parallel && directional()
		normal := b.Direction
		if c.DistanceRelation == selectedPlaneNormalV1 {
			normal = a.Direction
		}
		distance = conflictDot(delta, normal)
	case conflictLine(a.Kind) && b.Kind == "PLANE":
		constant = math.Abs(conflictDot(a.Direction, b.Direction)) <= at
		distance = conflictDot(delta, b.Direction)
	case a.Kind == "PLANE" && conflictLine(b.Kind):
		constant = math.Abs(conflictDot(a.Direction, b.Direction)) <= at
		distance = conflictDot(delta, a.Direction)
		if c.DistanceRelation != selectedPlaneNormalV1 {
			distance = -distance
		}
	default:
		knownDistance = false
	}
	if c.DistanceRelation == "OPPOSITE_SECOND_NORMAL" {
		distance = -distance
	}
	if conflictUnsigned(c.DistanceRelation) {
		distance = math.Abs(distance)
	}
	switch c.Kind {
	case "DISTANCE":
		if !knownDistance {
			return false, false, "no independent distance certificate for this exact pair"
		}
		return true, constant && math.Abs(distance-c.Value) <= lt, "distance or constant-offset direction mismatch"
	case "PARALLEL":
		return true, parallel && directional(), "parallel orientation mismatch"
	case "PERPENDICULAR":
		return true, math.Abs(conflictDot(a.Direction, b.Direction)) <= at, "perpendicular mismatch"
	case "ANGLE":
		av, bv := a.Direction, b.Direction
		if c.DirectionRelation == "OPPOSITE" {
			av = conflictScale(av, -1)
		}
		if c.AngleReferenceDirection != nil || c.AngleReferenceGeometryID != "" {
			axis := [3]float64{}
			if c.AngleReferenceDirection != nil {
				axis = *c.AngleReferenceDirection
			} else {
				g, ok := local[c.AngleReferenceGeometryID]
				if !ok {
					return false, false, "third occurrence axis missing"
				}
				axis = conflictWorld(g, poses[c.AngleReferenceBodyID]).Direction
			}
			if c.ReverseAngleReference {
				axis = conflictScale(axis, -1)
			}
			av = conflictSub(av, conflictScale(axis, conflictDot(av, axis)))
			bv = conflictSub(bv, conflictScale(axis, conflictDot(bv, axis)))
			if conflictNorm(av) <= 1e-8 || conflictNorm(bv) <= 1e-8 {
				return false, false, "projected angle degeneracy"
			}
			angle := math.Atan2(conflictDot(axis, conflictCross(av, bv)), conflictDot(av, bv))
			error := math.Atan2(math.Sin(angle-c.Value), math.Cos(angle-c.Value))
			return true, math.Abs(error) <= at, "projected angle mismatch"
		}
		angle := math.Acos(math.Max(-1, math.Min(1, conflictDot(av, bv))))
		sense := 1.0
		if c.Value > math.Pi {
			sense = -1
		}
		if c.SpatialAngleBranchDirection != nil {
			side := conflictDot(*c.SpatialAngleBranchDirection, conflictCross(av, bv))
			if math.Abs(side) > 1e-12 {
				sense = 1
				if side < 0 {
					sense = -1
				}
			}
		}
		angle *= sense
		error := math.Atan2(math.Sin(angle-c.Value), math.Cos(angle-c.Value))
		return true, math.Abs(error) <= at, "spatial angle mismatch"
	case "CONCENTRIC":
		return true, parallel && directional() && conflictNorm(conflictCross(delta, a.Direction)) <= lt, "coaxial mismatch"
	case "COINCIDENT":
		if a.Kind == "POINT" && b.Kind == "POINT" {
			return true, conflictNorm(delta) <= lt, "point coincidence mismatch"
		}
		if a.Kind == "PLANE" && b.Kind == "PLANE" {
			return true, parallel && directional() && math.Abs(conflictDot(delta, a.Direction)) <= lt, "plane coincidence mismatch"
		}
		if conflictLine(a.Kind) && conflictLine(b.Kind) {
			radii := true
			if a.Kind == "CYLINDER" && b.Kind == "CYLINDER" {
				radii = math.Abs(a.Radius-b.Radius) <= lt
			}
			return true, parallel && directional() && conflictNorm(conflictCross(delta, a.Direction)) <= lt && radii, "line/surface coincidence mismatch"
		}
		if knownDistance {
			return true, constant && math.Abs(distance) <= lt, "point incidence mismatch"
		}
	case "CONTACT":
		if a.Kind == "PLANE" && b.Kind == "PLANE" && c.ContactKind == "FACE" {
			orient := 1.
			if c.ContactSide == "EXTERNAL" {
				orient = -1
			}
			return true, conflictNorm(conflictSub(a.Direction, conflictScale(b.Direction, orient))) <= at && math.Abs(conflictDot(delta, a.Direction)) <= lt, "plane face contact mismatch"
		}
		plane, other := a, b
		if b.Kind == "PLANE" {
			plane, other = b, a
		}
		if plane.Kind == "PLANE" && (other.Kind == "SPHERE" || other.Kind == "CYLINDER") {
			d := conflictDot(conflictSub(other.Origin, plane.Origin), plane.Direction)
			alignment := true
			if other.Kind == "CYLINDER" {
				alignment = math.Abs(conflictDot(plane.Direction, other.Direction)) <= at
			}
			return true, alignment && math.Abs(d-float64(c.ContactBranch)*other.Radius) <= lt, "plane analytic tangency mismatch"
		}
	}
	return false, false, fmt.Sprintf("independent certificate unavailable for %s %s/%s; numerical result remains UNKNOWN", c.Kind, a.Kind, b.Kind)
}

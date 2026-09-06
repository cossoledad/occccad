package geometry

import workerv1 "github.com/occccad/occccad/gen/worker/v1"

// Typed solver evidence; these are not persistent geometric constraints.
type PreferenceStatus int32

const (
	PreferenceNotEvaluated PreferenceStatus = iota
	PreferenceConverged
	PreferenceIterationLimit
	PreferenceStalled
)

type MotionRole int32

const (
	MotionMoving MotionRole = iota
	MotionReference
	MotionNeutral
)

type FreedomKind int32

const (
	FreedomFixed FreedomKind = iota
	FreedomRevolute
	FreedomPrismatic
	FreedomCylindrical
	FreedomPlanar
	FreedomSpherical
	FreedomFree
	FreedomCoupled
)

type AssemblyBodyMotion struct {
	BodyID      string     `json:"bodyId"`
	Role        MotionRole `json:"role"`
	Translation float64    `json:"translation"`
	Rotation    float64    `json:"rotation"`
}
type AssemblyMotionPreference struct {
	Status                PreferenceStatus     `json:"status"`
	GeometricallyFeasible bool                 `json:"geometricallyFeasible"`
	ReferenceObjective    float64              `json:"referenceObjective"`
	TotalObjective        float64              `json:"totalObjective"`
	ReferenceOptimality   float64              `json:"referenceOptimality"`
	TotalOptimality       float64              `json:"totalOptimality"`
	LengthScale           float64              `json:"lengthScale"`
	AngleScale            float64              `json:"angleScale"`
	Iterations            uint64               `json:"iterations"`
	Bodies                []AssemblyBodyMotion `json:"bodies"`
}
type AssemblyScrewFreedom struct {
	Direction [3]float64 `json:"direction"`
	AxisPoint [3]float64 `json:"axisPoint"`
	Pitch     float64    `json:"pitch"`
}
type AssemblyBodyFreedom struct {
	BodyID                string                 `json:"bodyId"`
	RelativeToBodyID      string                 `json:"relativeToBodyId,omitempty"`
	LinearizationPose     AssemblyPose           `json:"linearizationPose"`
	Kind                  FreedomKind            `json:"kind"`
	TranslationDof        uint64                 `json:"translationDof"`
	RotationDof           uint64                 `json:"rotationDof"`
	AllowedBasis          [][]float64            `json:"allowedBasis"`
	BlockedBasis          [][]float64            `json:"blockedBasis"`
	TranslationDirections [][3]float64           `json:"translationDirections"`
	Rotations             []AssemblyScrewFreedom `json:"rotations"`
	RankThreshold         float64                `json:"rankThreshold"`
}

func assemblyVector(v *workerv1.Vec3) [3]float64 { return [3]float64{v.GetX(), v.GetY(), v.GetZ()} }
func assemblyPose(p *workerv1.RigidPose) AssemblyPose {
	r := p.GetRotation()
	return AssemblyPose{Translation: assemblyVector(p.GetTranslation()), Rotation: [4]float64{r.GetX(), r.GetY(), r.GetZ(), r.GetW()}}
}

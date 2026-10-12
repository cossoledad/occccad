package workspace

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

const typeSetKinematics = "occccad://product/kinematics/set"

type MotionQuantity struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}
type JointEndpoint struct {
	InstanceID string               `json:"instanceId"`
	Frame      InstancePose         `json:"frame"` // derived; rebuilt from supports for geometry-defined joints
	Axis       *AssemblyGeometryRef `json:"axis,omitempty"`
	Plane      *AssemblyGeometryRef `json:"plane,omitempty"`
	CapturedX  *[3]float64          `json:"capturedX,omitempty"`
}
type MechanismJoint struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	Kind           string               `json:"kind"` // GROUND | REVOLUTE
	First          JointEndpoint        `json:"first"`
	Second         *JointEndpoint       `json:"second,omitempty"`
	Zero           MotionQuantity       `json:"zero"`
	Direction      int                  `json:"direction"`      // +1/-1 along first frame Z
	AxialDirection string               `json:"axialDirection"` // plane normals SAME | OPPOSITE
	AxialOffset    MotionQuantity       `json:"axialOffset"`
	Sources        []JointSource        `json:"sources,omitempty"`
	Lower          *MotionQuantity      `json:"lower,omitempty"`
	Upper          *MotionQuantity      `json:"upper,omitempty"`
	Constraints    []AssemblyConstraint `json:"constraints,omitempty"` // owning mechanism relations, same public definition as Product
}
type Mechanism struct {
	Evaluation                *MechanismEvaluation    `json:"evaluation,omitempty"`
	ConstraintNameCounters    map[string]int          `json:"constraintNameCounters,omitempty"`
	ID                        string                  `json:"id"`
	Name                      string                  `json:"name"`
	UnitIDs                   []string                `json:"unitIds"` // direct owning Product instances; descendants frozen
	Joints                    []MechanismJoint        `json:"joints"`
	Poses                     map[string]InstancePose `json:"poses,omitempty"`       // accepted editing baseline, independent of Product assembly poses
	Coordinates               map[string]float64      `json:"coordinates,omitempty"` // canonical continuous joint coordinates, including winding
	SupplementalConstraintIDs []string                `json:"supplementalConstraintIds,omitempty"`
}
type MechanismEvaluation struct {
	RelativeDOF uint64 `json:"relativeDof"`
	GaugeDOF    uint64 `json:"gaugeDof"`
	DriverCount int    `json:"driverCount"`
}
type DMUAddress struct {
	InstancePath InstancePath `json:"instancePath"`
	BodyID       string       `json:"bodyId"`
}
type MotionStudy struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	MechanismID     string  `json:"mechanismId"`
	DriverID        string  `json:"driverId,omitempty"`
	DriverJointID   string  `json:"driverJointId,omitempty"`
	DurationSeconds float64 `json:"durationSeconds"`
	Frames          int     `json:"frames"`
	// Only an exactly matching existing coordinate equation may be replaced.
	ReplaceDriverConstraintID string                  `json:"replaceDriverConstraintId,omitempty"`
	IncludeSameUnit           bool                    `json:"includeSameUnit"`
	CheckDMU                  bool                    `json:"checkDmu"`
	Scope                     []DMUAddress            `json:"scope,omitempty"` // empty means all exact geometry
	Clearance                 MotionQuantity          `json:"clearance"`
	BudgetMS                  int                     `json:"budgetMs"`
	SourceRevisionID          string                  `json:"sourceRevisionId,omitempty"`
	BasisDigest               string                  `json:"basisDigest,omitempty"`
	GeometryDigest            string                  `json:"geometryDigest,omitempty"`
	Reference                 *MotionQuantity         `json:"reference,omitempty"`
	InitialPoses              map[string]InstancePose `json:"initialPoses,omitempty"`
	InitialCoordinates        map[string]float64      `json:"initialCoordinates,omitempty"`
	EnvironmentPoses          map[string]InstancePose `json:"environmentPoses,omitempty"` // frozen display environment, never mechanism ownership
	Keyframes                 []MotionKeyframe        `json:"keyframes,omitempty"`
	AnimateViewpoint          bool                    `json:"animateViewpoint,omitempty"`
}
type MotionViewpoint struct {
	Position [3]float64 `json:"position"`
	Rotation [4]float64 `json:"rotation"`
	Up       [3]float64 `json:"up"`
	Target   [3]float64 `json:"target"`
	Zoom     float64    `json:"zoom"`
	Top      float64    `json:"top"`
	Bottom   float64    `json:"bottom"`
}
type MotionKeyframe struct {
	ID          string           `json:"id"`
	TimeSeconds float64          `json:"timeSeconds"`
	DriverID    string           `json:"driverId"`
	Value       MotionQuantity   `json:"value"`
	Viewpoint   *MotionViewpoint `json:"viewpoint,omitempty"`
}
type MotionDriver struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	MechanismID     string          `json:"mechanismId"`
	JointID         string          `json:"jointId"`
	Reference       *MotionQuantity `json:"reference,omitempty"`
	ReferenceDigest string          `json:"referenceDigest,omitempty"`
}
type InterferenceAnalysis struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	StudyID         string         `json:"studyId,omitempty"`
	Scope           []DMUAddress   `json:"scope,omitempty"`
	Clearance       MotionQuantity `json:"clearance"`
	IncludeSameUnit bool           `json:"includeSameUnit"`
}
type JointSource struct {
	ConstraintID string `json:"constraintId"`
	Baseline     string `json:"baseline"`
}
type MotionAssemblyMapping struct {
	MechanismID        string `json:"mechanismId"`
	JointID            string `json:"jointId"`
	Role               string `json:"role"`
	ConstraintID       string `json:"constraintId"`
	JointBaseline      string `json:"jointBaseline"`
	ConstraintBaseline string `json:"constraintBaseline"`
}
type KinematicsDefinitions struct {
	Drivers      []MotionDriver          `json:"drivers,omitempty"`
	Analyses     []InterferenceAnalysis  `json:"analyses,omitempty"`
	Associations []MotionAssemblyMapping `json:"associations,omitempty"`
	Mechanisms   []Mechanism             `json:"mechanisms"`
	Studies      []MotionStudy           `json:"studies"`
}

func normalizeMotionRecordings(k *KinematicsDefinitions) error {
	for i := range k.Studies {
		study := &k.Studies[i]
		if len(study.Keyframes) == 0 {
			return fmt.Errorf("%w: simulation requires recorded keyframes", ErrValidation)
		}
		study.DurationSeconds = study.Keyframes[len(study.Keyframes)-1].TimeSeconds
	}
	return nil
}

func motionValue(q MotionQuantity, angular bool) (float64, error) {
	if !finiteMotion(q.Value) {
		return 0, fmt.Errorf("%w: non-finite coordinate", ErrValidation)
	}
	if angular {
		switch q.Unit {
		case "rad":
			return q.Value, nil
		case "deg":
			return q.Value * (math.Pi / 180), nil
		}
	} else {
		switch q.Unit {
		case "mm":
			return q.Value, nil
		case "m":
			if value := q.Value * 1000; finiteMotion(value) {
				return value, nil
			}
			return 0, fmt.Errorf("%w: coordinate unit conversion overflow", ErrValidation)
		}
	}
	return 0, fmt.Errorf("%w: coordinate unit %q does not match joint", ErrValidation, q.Unit)
}
func finiteMotion(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func validMotionPose(p InstancePose) bool {
	norm := 0.0
	for _, v := range p.Translation {
		if !finiteMotion(v) {
			return false
		}
	}
	for _, v := range p.Rotation {
		if !finiteMotion(v) {
			return false
		}
		norm += v * v
	}
	return math.Abs(norm-1) < 1e-8
}
func validateKinematics(model ProductModel, k KinematicsDefinitions) error {
	if len(k.Analyses) > 0 {
		return fmt.Errorf("%w: interference analysis is outside mechanism editing", ErrValidation)
	}
	if len(k.Mechanisms) > 16 || len(k.Studies) > 64 || len(k.Drivers) > 64 || len(k.Analyses) > 64 || len(k.Associations) > 1024 {
		return fmt.Errorf("%w: study resource limit", ErrValidation)
	}
	mechanisms := map[string]Mechanism{}
	ids := map[string]bool{}
	claim := func(id string) bool {
		if id == "" || len(id) > 128 || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, m := range k.Mechanisms {
		if !claim(m.ID) || m.Name == "" || len(m.UnitIDs) > 32 || len(m.Joints) > 64 {
			return fmt.Errorf("%w: invalid mechanism identity/size", ErrValidation)
		}
		if len(m.SupplementalConstraintIDs) > 0 {
			return fmt.Errorf("%w: assembly inheritance is not supported", ErrValidation)
		}
		if len(m.Poses) > 0 && len(m.Poses) != len(m.UnitIDs) {
			return fmt.Errorf("%w: incomplete mechanism poses", ErrValidation)
		}
		for id, p := range m.Poses {
			if !validMotionPose(p) || !slices.Contains(m.UnitIDs, id) || !slices.ContainsFunc(model.Instances, func(v ProductInstance) bool { return v.ID == id }) {
				return fmt.Errorf("%w: invalid mechanism pose", ErrValidation)
			}
		}
		for id, q := range m.Coordinates {
			if !finiteMotion(q) || math.Abs(q) > 1e9 || !slices.ContainsFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == id && j.Kind == "REVOLUTE" }) {
				return fmt.Errorf("%w: invalid continuous mechanism coordinate", ErrValidation)
			}
		}
		member := map[string]bool{}
		for _, id := range m.UnitIDs {
			if id == "" || member[id] {
				return fmt.Errorf("%w: invalid rigid motion unit %s", ErrValidation, id)
			}
			member[id] = true
		}
		for _, j := range m.Joints {
			if !claim(j.ID) || !member[j.First.InstanceID] || !validMotionPose(j.First.Frame) {
				return fmt.Errorf("%w: invalid joint first frame", ErrValidation)
			}
			if j.AxialDirection != "" && j.AxialDirection != "SAME" && j.AxialDirection != "OPPOSITE" {
				return fmt.Errorf("%w: invalid axial plane direction", ErrValidation)
			}
			if j.AxialOffset.Unit != "" {
				if _, e := motionValue(j.AxialOffset, false); e != nil {
					return e
				}
			}
			if len(j.Sources) > 0 {
				return fmt.Errorf("%w: joint sources are unsupported", ErrValidation)
			}
			switch j.Kind {
			case "GROUND":
				if j.Second != nil {
					return fmt.Errorf("%w: Ground has one endpoint", ErrValidation)
				}
			case "REVOLUTE":
				if j.Second == nil || j.Second.InstanceID == j.First.InstanceID || !member[j.Second.InstanceID] || !validMotionPose(j.Second.Frame) {
					return fmt.Errorf("%w: invalid joint second frame", ErrValidation)
				}
			default:
				return fmt.Errorf("%w: unsupported joint %s", ErrValidation, j.Kind)
			}
			if j.Direction != 1 && j.Direction != -1 {
				return fmt.Errorf("%w: joint direction must be +1/-1", ErrValidation)
			}
			if j.Kind == "REVOLUTE" {
				angular := j.Kind == "REVOLUTE"
				if _, err := motionValue(j.Zero, angular); err != nil {
					return err
				}
				if j.Lower != nil {
					if _, err := motionValue(*j.Lower, angular); err != nil {
						return err
					}
				}
				if j.Upper != nil {
					if _, err := motionValue(*j.Upper, angular); err != nil {
						return err
					}
				}
				if j.Lower != nil && j.Upper != nil {
					lo, _ := motionValue(*j.Lower, angular)
					hi, _ := motionValue(*j.Upper, angular)
					if lo >= hi {
						return fmt.Errorf("%w: reversed joint limits", ErrValidation)
					}
				}
			}
		}
		mechanisms[m.ID] = m
	}
	drivers := map[string]MotionDriver{}
	drivenCoordinates := map[string]bool{}
	for _, d := range k.Drivers {
		m, ok := mechanisms[d.MechanismID]
		i := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID })
		if !ok || !claim(d.ID) || d.Name == "" || drivenCoordinates[d.MechanismID+"/"+d.JointID] || i < 0 || m.Joints[i].Kind != "REVOLUTE" {
			return fmt.Errorf("%w: invalid driver", ErrValidation)
		}
		drivers[d.ID] = d
		drivenCoordinates[d.MechanismID+"/"+d.JointID] = true
		joint := m.Joints[i]
		if joint.Lower == nil || joint.Upper == nil {
			return fmt.Errorf("%w: driven joint requires lower and upper limits", ErrValidation)
		}
		if d.Reference != nil {
			if _, err := motionValue(*d.Reference, true); err != nil {
				return err
			}
		}
	}
	if err := normalizeMotionRecordings(&k); err != nil {
		return err
	}
	for _, study := range k.Studies {
		if err := validateMotionRecording(study, mechanisms, drivers, claim); err != nil {
			return err
		}
		for id, pose := range study.EnvironmentPoses {
			if !slices.ContainsFunc(model.Instances, func(i ProductInstance) bool { return i.ID == id }) || slices.Contains(mechanisms[study.MechanismID].UnitIDs, id) || !validMotionPose(pose) {
				return fmt.Errorf("%w: invalid recorded environment", ErrValidation)
			}
		}
	}

	return nil
}
func applyKinematicsDefinition(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var k KinematicsDefinitions
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &k); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := validateMotionDriverChanges(model.Kinematics, k); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := normalizeMotionRecordings(&k); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for i := range k.Studies {
		s := &k.Studies[i]
		s.DriverJointID = ""
		if s.CheckDMU || len(s.Scope) > 0 || s.Clearance.Value != 0 || s.IncludeSameUnit {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: interference is outside mechanism definitions", ErrValidation)
		}
	}
	if err := validateKinematics(model, k); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	before := model.Kinematics
	model.Kinematics = k
	if reflect.DeepEqual(before, k) {
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{}, nil
	}
	change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "product", SlotID: "product.kinematics"}, before, k)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, err
}

func kinematicsOnlyHistory(before, after ProductModel, changes modelcore.ChangeSet) bool {
	if len(changes.Changes) == 0 || len(changes.ImpactSeeds) != 0 {
		return false
	}
	for _, c := range changes.Changes {
		if c.Target.EntityID != "product" || c.Target.SlotID != "product.kinematics" {
			return false
		}
	}
	before.Kinematics = KinematicsDefinitions{}
	after.Kinematics = KinematicsDefinitions{}
	return reflect.DeepEqual(before, after)
}

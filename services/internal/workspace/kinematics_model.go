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
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Kind        string          `json:"kind"` // GROUND | RIGID | REVOLUTE | PRISMATIC
	First       JointEndpoint   `json:"first"`
	Second      *JointEndpoint  `json:"second,omitempty"`
	Zero        MotionQuantity  `json:"zero"`
	Direction   int             `json:"direction"` // +1/-1 along first frame Z
	AxialOffset MotionQuantity  `json:"axialOffset"`
	Sources     []JointSource   `json:"sources,omitempty"`
	Lower       *MotionQuantity `json:"lower,omitempty"`
	Upper       *MotionQuantity `json:"upper,omitempty"`
}
type Mechanism struct {
	ID                        string           `json:"id"`
	Name                      string           `json:"name"`
	UnitIDs                   []string         `json:"unitIds"` // direct owning Product instances; descendants frozen
	Joints                    []MechanismJoint `json:"joints"`
	SupplementalConstraintIDs []string         `json:"supplementalConstraintIds,omitempty"`
}
type DMUAddress struct {
	InstancePath InstancePath `json:"instancePath"`
	BodyID       string       `json:"bodyId"`
}
type MotionStudy struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	MechanismID     string         `json:"mechanismId"`
	DriverID        string         `json:"driverId,omitempty"`
	DriverJointID   string         `json:"driverJointId,omitempty"`
	Start           MotionQuantity `json:"start"`
	End             MotionQuantity `json:"end"`
	DurationSeconds float64        `json:"durationSeconds"`
	Frames          int            `json:"frames"`
	// Only an exactly matching existing coordinate equation may be replaced.
	ReplaceDriverConstraintID string         `json:"replaceDriverConstraintId,omitempty"`
	IncludeSameUnit           bool           `json:"includeSameUnit"`
	CheckDMU                  bool           `json:"checkDmu"`
	Scope                     []DMUAddress   `json:"scope,omitempty"` // empty means all exact geometry
	Clearance                 MotionQuantity `json:"clearance"`
	BudgetMS                  int            `json:"budgetMs"`
}
type MotionDriver struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MechanismID string `json:"mechanismId"`
	JointID     string `json:"jointId"`
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
			if j.AxialOffset.Unit != "" {
				if _, e := motionValue(j.AxialOffset, false); e != nil {
					return e
				}
			}
			if len(j.Sources) > 64 {
				return fmt.Errorf("%w: joint sources limit", ErrValidation)
			}
			switch j.Kind {
			case "GROUND":
				if j.Second != nil {
					return fmt.Errorf("%w: Ground has one endpoint", ErrValidation)
				}
			case "RIGID", "REVOLUTE", "PRISMATIC":
				if j.Second == nil || j.Second.InstanceID == j.First.InstanceID || !member[j.Second.InstanceID] || !validMotionPose(j.Second.Frame) {
					return fmt.Errorf("%w: invalid joint second frame", ErrValidation)
				}
			default:
				return fmt.Errorf("%w: unsupported joint %s", ErrValidation, j.Kind)
			}
			if j.Direction != 1 && j.Direction != -1 {
				return fmt.Errorf("%w: joint direction must be +1/-1", ErrValidation)
			}
			if j.Kind == "REVOLUTE" || j.Kind == "PRISMATIC" {
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
					if lo > hi {
						return fmt.Errorf("%w: reversed joint limits", ErrValidation)
					}
				}
			}
		}
		mechanisms[m.ID] = m
	}
	drivers := map[string]MotionDriver{}
	for _, d := range k.Drivers {
		m, ok := mechanisms[d.MechanismID]
		i := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID })
		if !ok || !claim(d.ID) || d.Name == "" || i < 0 || (m.Joints[i].Kind != "REVOLUTE" && m.Joints[i].Kind != "PRISMATIC") {
			return fmt.Errorf("%w: invalid driver", ErrValidation)
		}
		drivers[d.ID] = d
	}
	for _, s := range k.Studies {
		if s.DriverID == "" {
			return fmt.Errorf("%w: motion study requires an independent Driver reference", ErrValidation)
		}
		if s.DriverID != "" {
			d, ok := drivers[s.DriverID]
			if !ok || d.MechanismID != s.MechanismID {
				return fmt.Errorf("%w: study driver", ErrValidation)
			}
			s.DriverJointID = d.JointID
		}
		m, ok := mechanisms[s.MechanismID]
		if !ok || !claim(s.ID) || s.Name == "" || s.Frames < 2 || s.Frames > 500 || !finiteMotion(s.DurationSeconds) || s.DurationSeconds <= 0 || s.DurationSeconds > 3600 || s.BudgetMS < 100 || s.BudgetMS > 120000 {
			return fmt.Errorf("%w: invalid motion study", ErrValidation)
		}
		idx := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == s.DriverJointID })
		if idx < 0 {
			return fmt.Errorf("%w: unknown driver joint", ErrValidation)
		}
		j := m.Joints[idx]
		if j.Kind != "REVOLUTE" && j.Kind != "PRISMATIC" {
			return fmt.Errorf("%w: drive requires one Revolute/Prismatic coordinate", ErrValidation)
		}
		for _, v := range []MotionQuantity{s.Start, s.End} {
			if _, err := motionValue(v, j.Kind == "REVOLUTE"); err != nil {
				return err
			}
		}
		start, _ := motionValue(s.Start, j.Kind == "REVOLUTE")
		end, _ := motionValue(s.End, j.Kind == "REVOLUTE")
		zero, _ := motionValue(j.Zero, j.Kind == "REVOLUTE")
		if !finiteMotion(end-start) || !finiteMotion(zero+float64(j.Direction)*start) || !finiteMotion(zero+float64(j.Direction)*end) {
			return fmt.Errorf("%w: motion law coordinate overflow", ErrValidation)
		}
		gap, err := motionValue(s.Clearance, false)
		if err != nil {
			return err
		}
		if gap < 0 || gap > 1e6 {
			return fmt.Errorf("%w: invalid clearance", ErrValidation)
		}
		if len(s.Scope) > 128 {
			return fmt.Errorf("%w: DMU scope resource limit", ErrValidation)
		}
		for _, a := range s.Scope {
			if a.BodyID == "" || len(a.InstancePath.Segments) == 0 {
				return fmt.Errorf("%w: DMU scope needs occurrence and Body", ErrValidation)
			}
		}
		if s.ReplaceDriverConstraintID != "" {
			i := slices.IndexFunc(model.Constraints, func(c AssemblyConstraint) bool { return c.ID == s.ReplaceDriverConstraintID })
			if i < 0 || model.Constraints[i].Suppressed || model.Constraints[i].Mode == "MEASURED" {
				return fmt.Errorf("%w: unknown/inactive coordinate replacement", ErrValidation)
			}
		}
	}
	for _, a := range k.Analyses {
		gap, e := motionValue(a.Clearance, false)
		if !claim(a.ID) || a.Name == "" || e != nil || gap < 0 || gap > 1e6 || len(a.Scope) > 128 {
			return fmt.Errorf("%w: invalid interference analysis", ErrValidation)
		}
		if a.StudyID != "" && slices.IndexFunc(k.Studies, func(s MotionStudy) bool { return s.ID == a.StudyID }) < 0 {
			return fmt.Errorf("%w: analysis study", ErrValidation)
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
	for i := range k.Studies {
		s := &k.Studies[i]
		s.DriverJointID = ""
		if s.CheckDMU || len(s.Scope) > 0 || s.Clearance.Value != 0 || s.IncludeSameUnit {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: save DMU scope/settings in an InterferenceAnalysis", ErrValidation)
		}
	}
	if err := validateKinematics(model, k); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	before := model.Kinematics
	model.Kinematics = k
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

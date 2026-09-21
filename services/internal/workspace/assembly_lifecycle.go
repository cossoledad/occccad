package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeSetAssemblyConstraintState = "occccad://product/assembly-constraint/state"

type assemblyConstraintStatePayload struct {
	ConstraintIDs []string `json:"constraintIds"`
	Suppressed    *bool    `json:"suppressed,omitempty"`
	Mode          *string  `json:"mode,omitempty"`
}

// Entity compensation restores mode, activation and evaluation evidence together.
func applyAssemblyConstraintState(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var payload assemblyConstraintStatePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if len(payload.ConstraintIDs) == 0 || (payload.Suppressed == nil && payload.Mode == nil) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: constraint identities and state change required", ErrValidation)
	}
	wanted := map[string]bool{}
	for _, id := range payload.ConstraintIDs {
		if id == "" || wanted[id] {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: empty or duplicate constraint identity", ErrValidation)
		}
		wanted[id] = true
	}
	changes := modelcore.ChangeSet{}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if !wanted[c.ID] {
			continue
		}
		before := *c
		if payload.Mode != nil {
			if *payload.Mode != "DRIVING" && *payload.Mode != "MEASURED" {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: unsupported user constraint mode", ErrValidation)
			}
			if *payload.Mode == "MEASURED" && !assemblySupportsMeasurement(*c) {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: only angle and offset quantities support measurement", ErrValidation)
			}
			c.Mode = *payload.Mode
		}
		if payload.Suppressed != nil {
			c.Suppressed = *payload.Suppressed
		}
		if c.Mode != before.Mode || c.Suppressed != before.Suppressed {
			if !c.Suppressed {
				c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
				c.EvaluationSummary = "constraint activated or mode changed; authoritative resolution required"
			}
			change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, before, *c)
			if err != nil {
				return nil, modelcore.ChangeSet{}, err
			}
			changes.Changes = append(changes.Changes, change)
			changes.ImpactSeeds = append(changes.ImpactSeeds, modelcore.DependencyKey("assembly-constraint:"+c.ID))
		}
		delete(wanted, c.ID)
	}
	if len(wanted) > 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: assembly constraint does not exist", ErrValidation)
	}
	next, err := json.Marshal(model)
	return next, changes, err
}

// Measurement is evidence about the accepted pose, never a driving value.
func applyAssemblyMeasurements(model *ProductModel, result geometry.AssemblySolve, profile geometry.AssemblySolverProfile) {
	for i := range model.Constraints {
		c := &model.Constraints[i]
		c.MeasuredValue = nil
		if c.Suppressed || c.Mode != "MEASURED" {
			continue
		}
		valid, found := true, false
		measured := 0.0
		for _, equation := range result.EquationResiduals {
			if equation.ConstraintID != c.ID {
				continue
			}
			if strings.Contains(equation.EquationID, "/NORMAL_") || strings.Contains(equation.EquationID, "/LINE_PLANE_DIRECTION") {
				if math.Abs(equation.NormalizedValue) > profile.AngleTolerance {
					valid = false
				}
				continue
			}
			// The Geometry client freezes both residual scales at 1 (mm and radians).
			measured = c.Value + equation.NormalizedValue
			found = true
		}
		if valid && found && finite(measured) {
			if c.Kind == "ANGLE" {
				measured = math.Mod(measured, 2*math.Pi)
				if measured < 0 {
					measured += 2 * math.Pi
				}
			}
			c.MeasuredValue = &measured
			c.EvaluationSummary = "measurement evaluated at accepted placement; does not drive motion"
		} else {
			c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			c.EvaluationSummary = "measurement undefined for the accepted supports; no numeric value"
		}
	}
}

// Mark evaluator-owned writes before rebuilding the ChangeSet from final values.
func appendAssemblyEvaluationChanges(set modelcore.ChangeSet, before, after ProductModel) modelcore.ChangeSet {
	existing := map[modelcore.PropertyAddress]bool{}
	for _, change := range set.Changes {
		existing[change.Target] = true
	}
	mark := func(id, slot string) {
		address := modelcore.PropertyAddress{EntityID: id, SlotID: slot}
		if existing[address] {
			return
		}
		existing[address] = true
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, nil, nil)
		set.Changes = append(set.Changes, change)
	}
	poses := map[string]InstancePose{}
	for _, v := range before.Instances {
		poses[v.ID] = InstancePose{Translation: v.Translation, Rotation: normalizedInstanceRotation(v.Rotation)}
	}
	for _, v := range after.Instances {
		if old, ok := poses[v.ID]; ok && old != (InstancePose{Translation: v.Translation, Rotation: normalizedInstanceRotation(v.Rotation)}) {
			mark(v.ID, "instance.pose")
		}
	}
	constraints := map[string]AssemblyConstraint{}
	for _, c := range before.Constraints {
		constraints[c.ID] = c
	}
	for _, c := range after.Constraints {
		if old, ok := constraints[c.ID]; ok && !reflect.DeepEqual(old, c) {
			mark(c.ID, "assembly-constraint.entity")
		}
	}
	return set
}

func assemblySupportsMeasurement(c AssemblyConstraint) bool {
	return c.Kind == "DISTANCE" || (c.Kind == "ANGLE" && (c.AngleRelation == "" || c.AngleRelation == "FREE" || c.AngleRelation == "DIRECTED"))
}

// Source updates and replay of an already failed Revision retain deterministic
// model failure evidence. Transport/infrastructure failures never become history.
func acceptAssemblyEvaluationFailure(model *ProductModel, err error, allow bool) error {
	if err == nil {
		return nil
	}
	var failure *assemblySolveFailure
	if !allow || !errors.As(err, &failure) || failure.retryable {
		return err
	}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if c.Suppressed || c.EvaluationStatus == modelcore.AssemblyConstraintBroken {
			continue
		}
		c.EvaluationStatus = modelcore.AssemblyConstraintImpossible
		c.EvaluationSummary = failure.code + ": " + failure.diagnostic
	}
	return nil
}

// Angle relations compile to distinct equations without inventing a rotation axis.
func applyAssemblyAngleRelation(c *AssemblyConstraint, value *geometry.AssemblyConstraint) error {
	if c.Kind != "ANGLE" {
		return nil
	}
	switch c.AngleRelation {
	case "", "DIRECTED":
		return nil
	case "FREE", "PARALLEL", "PERPENDICULAR":
		c.AngleAxis, c.AngleReferenceDirection = nil, nil
		c.ReverseAngleAxis = false
		value.AngleReferenceDirection = nil
		if c.AngleRelation == "FREE" {
			return nil
		}
		value.Kind, value.Value = c.AngleRelation, 0
		if c.AngleRelation == "PERPENDICULAR" {
			c.Value = math.Pi / 2
			if c.DirectionRelation == "OPPOSITE" {
				c.Value = 3 * math.Pi / 2
			}
			value.Kind, value.Value, value.DirectionRelation = "ANGLE", c.Value, "SAME"
		}
		return nil
	default:
		return fmt.Errorf("%w: unknown angle relation", ErrValidation)
	}
}

// The selector follows the accepted normals; it is never an alignment equation.
func spatialAngleBranchDirection(a, b [3]float64, first, second *ProductInstance, sense float64) *[3]float64 {
	if first == nil || second == nil {
		return nil
	}
	aw := rotateByPose(InstancePose{Rotation: first.Rotation}, a)
	bw := rotateByPose(InstancePose{Rotation: second.Rotation}, b)
	cross := cross3(aw, bw)
	length := math.Sqrt(cross[0]*cross[0] + cross[1]*cross[1] + cross[2]*cross[2])
	if length < 1e-10 {
		return nil
	}
	for i := range cross {
		cross[i] *= sense / length
	}
	q := normalizedInstanceRotation(second.Rotation)
	local := rotateByPose(InstancePose{Rotation: [4]float64{-q[0], -q[1], -q[2], q[3]}}, cross)
	return &local
}

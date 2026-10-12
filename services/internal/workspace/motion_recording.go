package workspace

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/valuecopy"
)

func validateMotionDriverChanges(before, next KinematicsDefinitions) error {
	for _, m := range next.Mechanisms {
		for _, old := range before.Mechanisms {
			if old.ID != m.ID {
				continue
			}
			for _, joint := range m.Joints {
				for _, prior := range old.Joints {
					if prior.ID != joint.ID {
						continue
					}
					if prior.Kind != joint.Kind {
						return fmt.Errorf("%w: joint type is immutable", ErrValidation)
					}
					for _, coordinate := range prior.Coordinates {
						match := slices.IndexFunc(joint.Coordinates, func(c JointCoordinate) bool { return c.Role == coordinate.Role })
						if match < 0 || joint.Coordinates[match].ID != coordinate.ID || joint.Coordinates[match].Dimension != coordinate.Dimension {
							return fmt.Errorf("%w: joint coordinate identity is immutable", ErrValidation)
						}
						priorZero, err := motionValue(coordinate.Zero, coordinate.angular())
						nextZero, nextErr := motionValue(joint.Coordinates[match].Zero, coordinate.angular())
						if err != nil || nextErr != nil || math.Abs(priorZero-nextZero) > 1e-12 {
							return fmt.Errorf("%w: joint coordinate zero is immutable", ErrValidation)
						}
					}
				}
			}
		}
	}
	for _, driver := range next.Drivers {
		old := slices.IndexFunc(before.Drivers, func(d MotionDriver) bool { return d.ID == driver.ID })
		if old >= 0 && (before.Drivers[old].JointID != driver.JointID || before.Drivers[old].MechanismID != driver.MechanismID || before.Drivers[old].CoordinateID != driver.CoordinateID) {
			return fmt.Errorf("%w: 命令所属接合不能更改", ErrValidation)
		}
	}
	for _, study := range next.Studies {
		for _, id := range study.ActiveDriverIDs {
			if !slices.ContainsFunc(next.Drivers, func(d MotionDriver) bool { return d.ID == id && d.MechanismID == study.MechanismID }) {
				return fmt.Errorf("%w: 命令被模拟引用", ErrValidation)
			}
		}
	}

	return nil
}

// Preserve every physical target while presenting the recording in today's
// command zero. This is an owned draft; opening it never edits the saved record.
func rebaseMotionRecording(study MotionStudy, references map[string]MotionQuantity) (MotionStudy, error) {
	copy, err := valuecopy.Clone(study)
	if err != nil || len(study.References) != len(references) {
		return copy, fmt.Errorf("%w: invalid recording references", ErrValidation)
	}
	for _, id := range study.ActiveDriverIDs {
		old, ok := study.References[id]
		current, found := references[id]
		if !ok || !found {
			return copy, fmt.Errorf("%w: recording reference identity mismatch", ErrValidation)
		}
		angular := old.Unit == "rad" || old.Unit == "deg"
		a, err := motionValue(old, angular)
		if err != nil {
			return copy, err
		}
		b, err := motionValue(current, angular)
		if err != nil {
			return copy, err
		}
		unit := "mm"
		if angular {
			unit = "rad"
		}
		for i, key := range copy.Keyframes {
			q, err := motionValue(key.Values[id], angular)
			if err != nil {
				return copy, err
			}
			copy.Keyframes[i].Values[id] = MotionQuantity{Value: a + q - b, Unit: unit}
		}
	}
	copy.References, err = valuecopy.Clone(references)
	return copy, err
}

type qualifiedRecordingContext struct{}

// This witness can only originate in the actor-bound session finalizer. It
// avoids solving the same pinned keys again, but does not skip Domain Command,
// mechanism validation, Preview promotion, Revision or CAS checks.
func withQualifiedRecording(ctx context.Context, study *MotionStudy) context.Context {
	if study == nil {
		return ctx
	}
	return context.WithValue(ctx, qualifiedRecordingContext{}, motionSemanticDigest(*study))
}

func (service *Service) validateChangedRecordings(ctx context.Context, document, revision string, model ProductModel, k KinematicsDefinitions) error {
	var view DocumentView
	for _, study := range k.Studies {
		if len(study.Keyframes) == 0 {
			continue
		}
		old := slices.IndexFunc(model.Kinematics.Studies, func(s MotionStudy) bool { return s.ID == study.ID })
		if old >= 0 && reflect.DeepEqual(model.Kinematics.Studies[old], study) {
			continue
		}
		if digest, ok := ctx.Value(qualifiedRecordingContext{}).(string); ok && digest == motionSemanticDigest(study) {
			continue
		}
		if view.Product == nil {
			var err error
			view, err = service.GetDocument(ctx, document)
			if err != nil {
				return err
			}
			if view.Document.VersionID != revision {
				return fmt.Errorf("%w: KINEMATICS_BASE_REVISION_CHANGED", ErrValidation)
			}
			product, err := cloneAssemblyProduct(model)
			if err != nil {
				return err
			}
			product.Kinematics = k
			view.Product = &product
			if err := validateKinematics(product, k); err != nil {
				return err
			}
		}
		mi := slices.IndexFunc(k.Mechanisms, func(m Mechanism) bool { return m.ID == study.MechanismID })
		if mi < 0 {
			return fmt.Errorf("%w: missing recording owner", ErrValidation)
		}
		m := k.Mechanisms[mi]
		ds, err := snapshotDrivers(MotionSnapshot{Mechanism: m, Drivers: k.Drivers}, study.ActiveDriverIDs)
		if err != nil {
			return err
		}
		if study.BasisDigest != recordingBasis(m, ds) || study.GeometryDigest != recordingGeometry(view, m) {
			return fmt.Errorf("%w: SIMULATION_REVALIDATION_REQUIRED", ErrValidation)
		}

		m.Poses, m.Coordinates = study.InitialPoses, study.InitialCoordinates
		snapshot := MotionSnapshot{Schema: motionSchema, Drivers: ds, Policy: motionPolicy, DocumentID: document, RevisionID: revision, View: view, Mechanism: m, Study: study, SolverProfile: defaultAssemblySolverProfile()}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = service.compileMotionSnapshot(bounded, &snapshot)
		if err == nil {
			snapshot.Digest = motionDigest(snapshot)
			var engine *mechanismMotionSolver
			engine, err = newMechanismMotionSolver(bounded, service, snapshot)
			if err == nil {
				for id, pose := range study.InitialPoses {
					if !conflictPoseClose(geometry.AssemblyPose(pose), geometry.AssemblyPose(engine.state.Frame.UnitPoses[id]), snapshot.SolverProfile.LengthTolerance, snapshot.SolverProfile.AngleTolerance) {
						err = fmt.Errorf("recorded initial configuration is not qualified")
						break
					}
				}
				for index, key := range study.Keyframes {
					if err != nil {
						break
					}
					var targets map[string]float64
					targets, err = physicalTargets(m, ds, key.Values, study.References)
					if err != nil {
						break
					}
					if index == 0 {
						for id, q := range targets {
							if math.Abs(engine.state.Frame.Coordinates[id]-q) > 1e-8 {
								err = fmt.Errorf("initial key does not match recording configuration")
							}
						}
					}
					if err == nil {
						_, err = engine.Advance(bounded, targets)
					}
				}
			}
		}
		cancel()
		if err != nil {
			return fmt.Errorf("%w: recording has no qualified checkpoint: %v", ErrValidation, err)
		}
	}
	return nil
}

func validMotionViewpoint(v *MotionViewpoint) bool {
	if v == nil {
		return true
	}
	if !validMotionPose(InstancePose{Translation: v.Position, Rotation: v.Rotation}) {
		return false
	}
	for _, x := range append(append(v.Up[:], v.Target[:]...), v.Zoom, v.Top, v.Bottom) {
		if !finiteMotion(x) {
			return false
		}
	}
	return v.Zoom > 0 && v.Zoom <= 1e8 && v.Top > v.Bottom && math.Abs(conflictNorm(v.Up)-1) < 1e-6
}

func validateMotionRecording(s MotionStudy, mechanisms map[string]Mechanism, drivers map[string]MotionDriver, claim func(string) bool) error {
	m, ok := mechanisms[s.MechanismID]
	if !ok || !claim(s.ID) || s.Name == "" || len(s.Keyframes) == 0 || len(s.Keyframes) > 500 || len(s.ActiveDriverIDs) == 0 || len(s.References) != len(s.ActiveDriverIDs) || s.SourceRevisionID == "" || s.BasisDigest == "" || s.GeometryDigest == "" || s.BudgetMS < 100 || s.BudgetMS > 120000 || s.Frames < 2 || s.Frames > 500 || len(s.InitialPoses) != len(m.UnitIDs) {
		return fmt.Errorf("%w: invalid recorded simulation", ErrValidation)
	}
	ds := []MotionDriver{}
	seen := map[string]bool{}
	for _, id := range s.ActiveDriverIDs {
		d, ok := drivers[id]
		if !ok || d.MechanismID != m.ID || seen[id] {
			return fmt.Errorf("%w: invalid recording active set", ErrValidation)
		}
		seen[id] = true
		ds = append(ds, d)
	}
	for _, id := range m.UnitIDs {
		p, ok := s.InitialPoses[id]
		if !ok || !validMotionPose(p) {
			return fmt.Errorf("%w: incomplete recording configuration", ErrValidation)
		}
	}
	expected := 0
	for _, j := range m.Joints {
		expected += len(j.Coordinates)
	}
	if len(s.InitialCoordinates) != expected {
		return fmt.Errorf("%w: incomplete recording coordinates", ErrValidation)
	}
	for id, q := range s.InitialCoordinates {
		if !finiteMotion(q) || math.Abs(q) > 1e9 || !mechanismHasCoordinate(m, id) {
			return fmt.Errorf("%w: invalid recording coordinate", ErrValidation)
		}
	}
	for i, key := range s.Keyframes {
		if _, err := physicalTargets(m, ds, key.Values, s.References); err != nil {
			return err
		}
		if !claim(key.ID) || !finiteMotion(key.TimeSeconds) || key.TimeSeconds < 0 || key.TimeSeconds > 3600 || i == 0 && key.TimeSeconds != 0 || i > 0 && key.TimeSeconds <= s.Keyframes[i-1].TimeSeconds || !validMotionViewpoint(key.Viewpoint) {
			return fmt.Errorf("%w: invalid keyframe identity or time", ErrValidation)
		}
	}
	if s.ReplaceDriverConstraintID != "" || s.CheckDMU || len(s.Scope) > 0 || s.Clearance.Value != 0 || s.IncludeSameUnit {
		return fmt.Errorf("%w: recording contains unrelated analysis", ErrValidation)
	}
	return nil
}

func recordingBasis(m Mechanism, ds []MotionDriver) string {
	units := append([]string(nil), m.UnitIDs...)
	slices.Sort(units)
	joints := make([]struct{ ID, Digest string }, 0, len(m.Joints))
	for _, j := range m.Joints {
		joints = append(joints, struct{ ID, Digest string }{j.ID, jointDefinitionDigest(j)})
	}
	slices.SortFunc(joints, func(a, b struct{ ID, Digest string }) int { return strings.Compare(a.ID, b.ID) })
	type binding struct{ ID, JointID, CoordinateID string }
	bindings := []binding{}
	for _, d := range ds {
		bindings = append(bindings, binding{d.ID, d.JointID, d.CoordinateID})
	}
	slices.SortFunc(bindings, func(a, b binding) int { return strings.Compare(a.ID, b.ID) })
	return motionSemanticDigest(struct {
		MechanismID string
		Units       []string
		Joints      any
		Bindings    []binding
	}{m.ID, units, joints, bindings})
}
func commandCoordinateBasis(j MechanismJoint, id string) string {
	j.Coordinates = append([]JointCoordinate(nil), j.Coordinates...)
	for i := range j.Coordinates {
		j.Coordinates[i].Lower = nil
		j.Coordinates[i].Upper = nil
	}
	return motionSemanticDigest(struct {
		Joint        string
		CoordinateID string
	}{jointDefinitionDigest(j), id})
}
func motionEnvironment(m Mechanism, poses map[string]InstancePose) map[string]InstancePose {
	result := map[string]InstancePose{}
	for id, p := range poses {
		if !slices.Contains(m.UnitIDs, id) {
			result[id] = p
		}
	}
	return result
}

// Referenced geometry is part of recording provenance; a unchanged joint ID
// does not make a recording valid after its supporting Part has changed.
func recordingGeometry(view DocumentView, m Mechanism) string {
	type dependency struct{ InstanceID, DocumentID, RevisionID, Path, BodyID, GeometryKey string }
	items := []dependency{}
	for _, i := range view.Product.Instances {
		if !slices.Contains(m.UnitIDs, i.ID) {
			continue
		}
		revision := i.ResolvedVersionID
		if revision == "" {
			revision = i.ReferencedVersionID
		}
		items = append(items, dependency{InstanceID: i.ID, DocumentID: i.ReferencedDocumentID, RevisionID: revision})
		for _, r := range view.ResolvedInstances {
			if r.ID == i.ID || len(r.InstancePath.Segments) > 0 && r.InstancePath.Segments[0].InstanceID == i.ID {
				items = append(items, dependency{InstanceID: i.ID, DocumentID: r.DocumentID, Path: r.InstancePath.Canonical, BodyID: r.BodyID, GeometryKey: r.GeometryKey})
			}
		}
	}
	slices.SortFunc(items, func(a, b dependency) int { return strings.Compare(motionSemanticDigest(a), motionSemanticDigest(b)) })
	return motionSemanticDigest(items)
}
func bindMotionDrivers(k *KinematicsDefinitions) error {
	for i := range k.Drivers {
		d := &k.Drivers[i]
		mi := slices.IndexFunc(k.Mechanisms, func(m Mechanism) bool { return m.ID == d.MechanismID })
		if mi < 0 {
			return fmt.Errorf("%w: missing command mechanism", ErrValidation)
		}
		m := k.Mechanisms[mi]
		j, c, err := driverCoordinate(m, *d)
		if err != nil {
			return err
		}
		if d.Reference == nil {
			var prior *float64
			if q, ok := m.Coordinates[c.ID]; ok {
				prior = &q
			}
			q, err := motionJointCoordinate(j, c, m.Poses, prior)
			if err != nil {
				return err
			}
			d.Reference = &MotionQuantity{Value: q, Unit: c.unit()}
			d.ReferenceDigest = commandCoordinateBasis(j, c.ID)
		}
	}
	return nil
}

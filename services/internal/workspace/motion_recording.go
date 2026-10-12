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
	for _, driver := range next.Drivers {
		old := slices.IndexFunc(before.Drivers, func(d MotionDriver) bool { return d.ID == driver.ID })
		if old >= 0 && (before.Drivers[old].JointID != driver.JointID || before.Drivers[old].MechanismID != driver.MechanismID) {
			return fmt.Errorf("%w: 命令所属接合不能更改", ErrValidation)
		}
	}
	for _, study := range next.Studies {
		if !slices.ContainsFunc(next.Drivers, func(d MotionDriver) bool { return d.ID == study.DriverID && d.MechanismID == study.MechanismID }) {
			return fmt.Errorf("%w: 命令被模拟引用", ErrValidation)
		}
	}
	return nil
}

// Preserve every physical target while presenting the recording in today's
// command zero. This is an owned draft; opening it never edits the saved record.
func rebaseMotionRecording(study MotionStudy, reference MotionQuantity) (MotionStudy, error) {
	copy, err := valuecopy.Clone(study)
	if err != nil || study.Reference == nil {
		return copy, fmt.Errorf("%w: invalid recording reference", ErrValidation)
	}
	old, err := motionValue(*study.Reference, true)
	if err != nil {
		return copy, err
	}
	current, err := motionValue(reference, true)
	if err != nil {
		return copy, err
	}
	for i, key := range copy.Keyframes {
		value, err := motionValue(key.Value, true)
		if err != nil {
			return copy, err
		}
		copy.Keyframes[i].Value = MotionQuantity{Value: old + value - current, Unit: "rad"}
	}
	copy.Reference = &reference
	return copy, nil
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
		di := slices.IndexFunc(k.Drivers, func(d MotionDriver) bool { return d.ID == study.DriverID })
		if mi < 0 || di < 0 {
			return fmt.Errorf("%w: missing recording owner", ErrValidation)
		}
		m, d := k.Mechanisms[mi], k.Drivers[di]
		if study.BasisDigest != recordingBasis(m, d) || study.GeometryDigest != recordingGeometry(view, m) {
			return fmt.Errorf("%w: SIMULATION_REVALIDATION_REQUIRED", ErrValidation)
		}
		reference, err := motionValue(*study.Reference, true)
		if err != nil {
			return err
		}
		m.Poses, m.Coordinates = study.InitialPoses, study.InitialCoordinates
		study.DriverJointID = d.JointID
		snapshot := MotionSnapshot{Schema: 1, Policy: motionPolicy, DocumentID: document, RevisionID: revision, View: view, Mechanism: m, Study: study, SolverProfile: defaultAssemblySolverProfile()}
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
				first, _ := motionValue(study.Keyframes[0].Value, true)
				if math.Abs(engine.state.Frame.DriverValue-reference-first) > 1e-8 {
					err = fmt.Errorf("initial key does not match recording configuration")
				}
				for _, key := range study.Keyframes {
					if err != nil {
						break
					}
					q, _ := motionValue(key.Value, true)
					_, err = engine.Advance(bounded, reference+q)
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
	d, driven := drivers[s.DriverID]
	if !ok || !driven || d.MechanismID != m.ID || !claim(s.ID) || s.Name == "" || len(s.Keyframes) > 500 ||
		s.SourceRevisionID == "" || s.BasisDigest == "" || s.GeometryDigest == "" || s.Reference == nil ||
		s.BudgetMS < 100 || s.BudgetMS > 120000 || s.Frames < 2 || s.Frames > 500 ||
		len(s.InitialPoses) != len(m.UnitIDs) {
		return fmt.Errorf("%w: invalid recorded simulation", ErrValidation)
	}
	reference, err := motionValue(*s.Reference, true)
	if err != nil {
		return err
	}
	for _, id := range m.UnitIDs {
		p, found := s.InitialPoses[id]
		if !found || !validMotionPose(p) {
			return fmt.Errorf("%w: incomplete simulation initial configuration", ErrValidation)
		}
	}
	for _, j := range m.Joints {
		if j.Kind == "REVOLUTE" {
			if _, ok := s.InitialCoordinates[j.ID]; !ok {
				return fmt.Errorf("%w: incomplete recording branch intent", ErrValidation)
			}
		}
	}
	for id, q := range s.InitialCoordinates {
		if !finiteMotion(q) || math.Abs(q) > 1e9 || !slices.ContainsFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == id && j.Kind == "REVOLUTE" }) {
			return fmt.Errorf("%w: invalid recording branch intent", ErrValidation)
		}
	}
	for i, key := range s.Keyframes {
		value, err := motionValue(key.Value, true)
		if err != nil {
			return err
		}
		if !claim(key.ID) || key.DriverID != d.ID || !finiteMotion(reference+value) || !finiteMotion(key.TimeSeconds) ||
			key.TimeSeconds < 0 || key.TimeSeconds > 3600 || i == 0 && key.TimeSeconds != 0 ||
			i > 0 && key.TimeSeconds <= s.Keyframes[i-1].TimeSeconds || !validMotionViewpoint(key.Viewpoint) {
			return fmt.Errorf("%w: invalid keyframe identity, time or coordinate", ErrValidation)
		}
	}
	if s.ReplaceDriverConstraintID != "" || s.CheckDMU || len(s.Scope) > 0 || s.Clearance.Value != 0 || s.IncludeSameUnit {
		return fmt.Errorf("%w: recorded simulation contains unrelated analysis", ErrValidation)
	}
	return nil
}

func recordingBasis(m Mechanism, d MotionDriver) string {
	units := append([]string(nil), m.UnitIDs...)
	slices.Sort(units)
	joints := make([]struct{ ID, Digest string }, 0, len(m.Joints))
	for _, j := range m.Joints {
		joints = append(joints, struct{ ID, Digest string }{j.ID, jointDefinitionDigest(j)})
	}
	slices.SortFunc(joints, func(a, b struct{ ID, Digest string }) int { return strings.Compare(a.ID, b.ID) })
	return motionSemanticDigest(struct {
		MechanismID       string
		Units             []string
		Joints            any
		DriverID, JointID string
	}{m.ID, units, joints, d.ID, d.JointID})
}
func commandCoordinateBasis(j MechanismJoint) string {
	j.Lower, j.Upper = nil, nil
	return jointDefinitionDigest(j)
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
		ji := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID && j.Kind == "REVOLUTE" })
		if ji < 0 {
			return fmt.Errorf("%w: missing command joint", ErrValidation)
		}
		j := m.Joints[ji]
		if d.Reference == nil {
			var prior *float64
			if q, ok := m.Coordinates[j.ID]; ok {
				prior = &q
			}
			q, err := motionJointCoordinate(j, m.Poses, prior)
			if err != nil {
				return err
			}
			d.Reference = &MotionQuantity{Value: q, Unit: "rad"}
			d.ReferenceDigest = commandCoordinateBasis(j)
		}
	}
	return nil
}

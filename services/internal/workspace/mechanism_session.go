package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/valuecopy"
	"google.golang.org/grpc/codes"
)

const mechanismSessionTTL = 15 * time.Minute

type MechanismSessionBegin struct {
	OwnerID            string   `json:"ownerId"`
	Generation         uint64   `json:"generation"`
	BaseRevisionID     string   `json:"baseRevisionId"`
	MechanismID        string   `json:"mechanismId"`
	DriverID           string   `json:"driverId"`
	StudyID            string   `json:"studyId,omitempty"`
	Revalidate         bool     `json:"revalidate,omitempty"`
	SourceMechanismIDs []string `json:"sourceMechanismIds,omitempty"`
}
type MechanismSessionFrame struct {
	ActiveDriver   *MotionDriver                         `json:"activeDriver,omitempty"`
	SessionID      string                                `json:"sessionId"`
	BaseRevisionID string                                `json:"baseRevisionId"`
	InputDigest    string                                `json:"inputDigest"`
	Sequence       uint64                                `json:"sequence"`
	Accepted       bool                                  `json:"accepted"`
	Frame          MotionFrame                           `json:"frame"`
	Failure        *MechanismMotionFailure               `json:"failure,omitempty"`
	SolveMS        float64                               `json:"solveMs"`
	Interaction    *geometry.AssemblyInteractionEvidence `json:"interaction,omitempty"`
}
type MechanismMotionFailure struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	DiagnosticID string `json:"diagnosticId,omitempty"`
}
type MechanismSessionOpened struct {
	MechanismSessionFrame
	Driver         MotionDriver            `json:"driver"`
	BasisDigest    string                  `json:"basisDigest"`
	GeometryDigest string                  `json:"geometryDigest"`
	InitialPoses   map[string]InstancePose `json:"initialPoses"`
	Study          *MotionStudy            `json:"study,omitempty"`
}
type MechanismSessionUpdate struct {
	DriverID      string                       `json:"driverId,omitempty"`
	Sequence      uint64                       `json:"sequence"`
	Value         *MotionQuantity              `json:"value,omitempty"`
	SpatialTarget *geometry.AssemblyDragTarget `json:"spatialTarget,omitempty"`
}
type MechanismSessionFinalize struct {
	Sequence uint64       `json:"sequence"`
	Study    *MotionStudy `json:"study,omitempty"`
}
type mechanismSession struct {
	id, actor, document string
	owner               string
	generation          uint64
	view                DocumentView
	driver              MotionDriver
	basis, geometry     string
	initial             MotionFrame
	engine              *mechanismMotionSolver
	sequence            uint64
	expires             time.Time
	cancel              context.CancelFunc
	busy, finalizing    bool
	previewID           string
	study               *MotionStudy
	keys                map[string]MotionKeyframe
}
type mechanismSessionCache struct {
	mu      sync.Mutex
	entries map[string]*mechanismSession
	owners  map[mechanismSessionOwner]mechanismOwnerFence
}
type mechanismSessionOwner struct{ document, actor, owner string }
type mechanismOwnerFence struct {
	generation uint64
	expires    time.Time
}

func (service *Service) mechanismHead(ctx context.Context, document, revision string) error {
	var head string
	if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, document).Scan(&head); err != nil {
		return err
	}
	if head != revision {
		return fmt.Errorf("%w: KINEMATICS_BASE_REVISION_CHANGED", ErrValidation)
	}
	return nil
}
func (service *Service) mechanismSessionLocked(document, actor, id string) (*mechanismSession, error) {
	s := service.mechanismSessions.entries[id]
	if s == nil || s.document != document || s.actor != actor || time.Now().After(s.expires) {
		return nil, fmt.Errorf("%w: MECHANISM_SESSION_EXPIRED", ErrValidation)
	}
	s.expires = time.Now().Add(mechanismSessionTTL)
	key := mechanismSessionOwner{document, actor, s.owner}
	if fence := service.mechanismSessions.owners[key]; fence.generation == s.generation {
		fence.expires = s.expires
		service.mechanismSessions.owners[key] = fence
	}
	return s, nil
}
func (s *mechanismSession) frame(accepted bool) MechanismSessionFrame {
	f, _ := valuecopy.Clone(s.engine.state.Frame)
	ref, _ := motionValue(*s.driver.Reference, true)
	f.DriverValue -= ref
	d, _ := valuecopy.Clone(s.driver)
	return MechanismSessionFrame{ActiveDriver: &d, SessionID: s.id, BaseRevisionID: s.view.Document.VersionID, InputDigest: s.engine.snapshot.Digest, Sequence: s.sequence, Accepted: accepted, Frame: f}
}
func (service *Service) BeginMechanismSession(parent context.Context, document, actor string, req MechanismSessionBegin) (_ MechanismSessionOpened, returnErr error) {
	ctx, cancel := context.WithTimeout(withAssemblyReadCache(parent), 30*time.Second)
	defer cancel()
	diagnostic := service.newOperationDiagnostic(ctx, document, CommandRequest{RequestID: newID("mechanism-begin"), Type: "MECHANISM_MOTION", VersionID: req.BaseRevisionID}, "MECHANISM_MOTION")
	var retainedEngine *mechanismMotionSolver
	defer func() {
		if diagnostic != nil {
			diagnostic.MotionBegin = &req
			diagnostic.BaseRevisionID = req.BaseRevisionID
			if retainedEngine != nil {
				diagnostic.MotionSnapshot = &retainedEngine.snapshot
				if len(retainedEngine.replay) > 0 {
					diagnostic.recordAssemblyAttempt(retainedEngine.replay)
				}
			}
		}
		service.finishOperationDiagnostic(ctx, diagnostic, nil, nil, &returnErr)
	}()
	view, err := service.GetDocument(ctx, document)
	if err != nil {
		return MechanismSessionOpened{}, err
	}
	if view.Product == nil || req.BaseRevisionID == "" || req.BaseRevisionID != view.Document.VersionID || len(view.Product.Instances) > 64 {
		return MechanismSessionOpened{}, fmt.Errorf("%w: invalid mechanism session revision or scope", ErrValidation)
	}
	if req.OwnerID == "" || len(req.OwnerID) > 128 || req.Generation == 0 {
		return MechanismSessionOpened{}, fmt.Errorf("%w: invalid mechanism session owner", ErrValidation)
	}
	// Reserve the dialog generation before computing. An aborted HTTP response,
	// StrictMode remount, or late Begin can never recreate an older private owner.
	c := &service.mechanismSessions
	owner := mechanismSessionOwner{document, actor, req.OwnerID}
	c.mu.Lock()
	if c.owners == nil {
		c.owners = map[mechanismSessionOwner]mechanismOwnerFence{}
	}
	for key, fence := range c.owners {
		if time.Now().After(fence.expires) {
			delete(c.owners, key)
		}
	}
	if fence, exists := c.owners[owner]; exists && fence.generation >= req.Generation {
		c.mu.Unlock()
		return MechanismSessionOpened{}, fmt.Errorf("%w: MECHANISM_SESSION_SUPERSEDED", ErrValidation)
	}
	if _, exists := c.owners[owner]; !exists && len(c.owners) >= 128 {
		c.mu.Unlock()
		return MechanismSessionOpened{}, fmt.Errorf("%w: mechanism owner capacity", ErrValidation)
	}
	c.owners[owner] = mechanismOwnerFence{req.Generation, time.Now().Add(mechanismSessionTTL)}
	for id, old := range c.entries {
		if time.Now().After(old.expires) || (old.actor == actor && old.document == document && (old.owner == req.OwnerID || old.view.Document.VersionID != req.BaseRevisionID)) {
			if old.cancel != nil {
				old.cancel()
			}
			service.DiscardPreview(old.document, old.actor, old.previewID)
			delete(c.entries, id)
		}
	}
	c.mu.Unlock()
	if diagnostic != nil {
		diagnostic.BaseModel, _ = json.Marshal(view.Product)
	}
	mi := slices.IndexFunc(view.Product.Kinematics.Mechanisms, func(m Mechanism) bool { return m.ID == req.MechanismID })
	di := slices.IndexFunc(view.Product.Kinematics.Drivers, func(d MotionDriver) bool { return d.ID == req.DriverID && d.MechanismID == req.MechanismID })
	if mi < 0 || di < 0 {
		return MechanismSessionOpened{}, fmt.Errorf("%w: select a mechanism command", ErrValidation)
	}
	m, d := view.Product.Kinematics.Mechanisms[mi], view.Product.Kinematics.Drivers[di]
	ji := slices.IndexFunc(m.Joints, func(j MechanismJoint) bool { return j.ID == d.JointID })
	if ji < 0 || d.Reference == nil || d.ReferenceDigest != commandCoordinateBasis(m.Joints[ji]) {
		return MechanismSessionOpened{}, fmt.Errorf("%w: COMMAND_REFERENCE_REVALIDATION_REQUIRED", ErrValidation)
	}
	s := &mechanismSession{id: newID("mechanism-session"), actor: actor, document: document, owner: req.OwnerID, generation: req.Generation, view: view, driver: d, basis: recordingBasis(m, d), geometry: recordingGeometry(view, m), expires: time.Now().Add(mechanismSessionTTL), keys: map[string]MotionKeyframe{}}
	study := MotionStudy{MechanismID: m.ID, DriverID: d.ID, DriverJointID: d.JointID, Reference: d.Reference, BudgetMS: 30000, Frames: 121}
	if req.StudyID != "" {
		si := slices.IndexFunc(view.Product.Kinematics.Studies, func(x MotionStudy) bool {
			return x.ID == req.StudyID && x.MechanismID == m.ID && x.DriverID == d.ID && len(x.Keyframes) > 0
		})
		if si < 0 {
			return MechanismSessionOpened{}, fmt.Errorf("%w: unknown recorded simulation", ErrValidation)
		}
		study, err = rebaseMotionRecording(view.Product.Kinematics.Studies[si], *d.Reference)
		if err != nil {
			return MechanismSessionOpened{}, err
		}
		study.DriverJointID = d.JointID
		if !req.Revalidate && (study.BasisDigest != s.basis || study.GeometryDigest != s.geometry) {
			return MechanismSessionOpened{}, fmt.Errorf("%w: SIMULATION_REVALIDATION_REQUIRED", ErrValidation)
		}
		s.study = &study
		for _, key := range study.Keyframes {
			s.keys[key.ID] = key
		}
		if req.Revalidate {
			// This is an explicit adoption of today's mechanism and geometry.
			// Keep the authored operation values; never silently reset a command zero.
			study.EnvironmentPoses = nil
		} else {
			m.Poses = study.InitialPoses
			m.Coordinates = study.InitialCoordinates
		}
	}
	compileView, err := valuecopy.Clone(view)
	if err != nil {
		return MechanismSessionOpened{}, err
	}
	nominal, err := mechanismInteractionModel(*compileView.Product, m.ID, req.SourceMechanismIDs)
	if err != nil {
		return MechanismSessionOpened{}, err
	}
	compileView.Product = &nominal
	snapshot := MotionSnapshot{Schema: 1, Policy: motionPolicy, DocumentID: document, RevisionID: req.BaseRevisionID, View: compileView, Mechanism: m, Study: study, SolverProfile: defaultAssemblySolverProfile()}
	if err = service.compileMotionSnapshot(ctx, &snapshot); err != nil {
		return MechanismSessionOpened{}, err
	}
	snapshot.Digest = motionDigest(snapshot)
	if data, err := json.Marshal(snapshot); err != nil || len(data) > 8<<20 {
		return MechanismSessionOpened{}, fmt.Errorf("%w: mechanism snapshot size limit", ErrValidation)
	}
	s.engine, err = newMechanismMotionSolver(ctx, service, snapshot)
	retainedEngine = s.engine
	if err != nil {
		return MechanismSessionOpened{}, err
	}
	if req.Revalidate && s.study != nil {
		reference, _ := motionValue(*d.Reference, true)
		for index, key := range study.Keyframes {
			value, valueErr := motionValue(key.Value, true)
			if valueErr != nil {
				return MechanismSessionOpened{}, valueErr
			}
			if _, err = s.engine.Advance(ctx, reference+value); err != nil {
				return MechanismSessionOpened{}, fmt.Errorf("%w: SIMULATION_REVALIDATION_FAILED: key %d: %v", ErrValidation, index+1, err)
			}
			if index == 0 {
				s.initial, _ = valuecopy.Clone(s.engine.state.Frame)
			}
		}
		study.InitialPoses = mechanismMemberPoses(m, s.initial.UnitPoses)
		study.InitialCoordinates, _ = valuecopy.Clone(s.initial.Coordinates)
		study.EnvironmentPoses = motionEnvironment(m, s.initial.UnitPoses)
		study.BasisDigest, study.GeometryDigest = s.basis, s.geometry
		study.SourceRevisionID, study.Reference = req.BaseRevisionID, d.Reference
		snapshot.Mechanism.Poses, snapshot.Mechanism.Coordinates = study.InitialPoses, study.InitialCoordinates
		snapshot.Study = study
		if err = service.compileMotionSnapshot(ctx, &snapshot); err != nil {
			return MechanismSessionOpened{}, err
		}
		snapshot.Digest = motionDigest(snapshot)
		s.engine, err = newMechanismMotionSolver(ctx, service, snapshot)
		if err != nil {
			return MechanismSessionOpened{}, err
		}
		s.study = &study
	}
	s.initial, _ = valuecopy.Clone(s.engine.state.Frame)
	if err = service.mechanismHead(ctx, document, req.BaseRevisionID); err != nil {
		return MechanismSessionOpened{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.owners[owner].generation != req.Generation {
		return MechanismSessionOpened{}, fmt.Errorf("%w: MECHANISM_SESSION_SUPERSEDED", ErrValidation)
	}
	if c.entries == nil {
		c.entries = map[string]*mechanismSession{}
	}
	for id, old := range c.entries {
		if time.Now().After(old.expires) {
			if old.cancel != nil {
				old.cancel()
			}
			service.DiscardPreview(old.document, old.actor, old.previewID)
			delete(c.entries, id)
		}
	}
	actorCount := 0
	for _, old := range c.entries {
		if old.actor == actor {
			actorCount++
		}
	}
	if len(c.entries) >= 32 || actorCount >= 4 {
		return MechanismSessionOpened{}, fmt.Errorf("%w: mechanism session capacity", ErrValidation)
	}
	c.entries[s.id] = s
	return MechanismSessionOpened{MechanismSessionFrame: s.frame(true), Driver: d, BasisDigest: s.basis, GeometryDigest: s.geometry, InitialPoses: mechanismMemberPoses(m, s.initial.UnitPoses), Study: s.study}, nil
}

// An idle dialog renews its private lease without changing a coordinate or Head.
func (service *Service) TouchMechanismSession(ctx context.Context, document, actor, id string) error {
	c := &service.mechanismSessions
	c.mu.Lock()
	s, err := service.mechanismSessionLocked(document, actor, id)
	var revision string
	if err == nil {
		revision = s.view.Document.VersionID
	}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return service.mechanismHead(ctx, document, revision)
}

func (service *Service) UpdateMechanismSession(parent context.Context, document, actor, id string, req MechanismSessionUpdate) (MechanismSessionFrame, error) {
	inputs := 0
	if req.Value != nil {
		inputs++
	}
	if req.SpatialTarget != nil {
		inputs++
	}
	if req.DriverID != "" {
		inputs++
	}
	if req.Sequence == 0 || inputs != 1 {
		return MechanismSessionFrame{}, fmt.Errorf("%w: one motion input required", ErrValidation)
	}
	c := &service.mechanismSessions
	c.mu.Lock()
	s, err := service.mechanismSessionLocked(document, actor, id)
	if err != nil {
		c.mu.Unlock()
		return MechanismSessionFrame{}, err
	}
	if req.Sequence <= s.sequence || s.finalizing {
		c.mu.Unlock()
		return MechanismSessionFrame{}, fmt.Errorf("%w: stale or finalizing mechanism input", ErrValidation)
	}
	driver := s.driver
	if req.DriverID != "" {
		i := slices.IndexFunc(s.view.Product.Kinematics.Drivers, func(d MotionDriver) bool { return d.ID == req.DriverID && d.MechanismID == s.driver.MechanismID })
		if i < 0 || s.study != nil || len(s.keys) > 0 {
			c.mu.Unlock()
			return MechanismSessionFrame{}, fmt.Errorf("%w: command switch outside a trial", ErrValidation)
		}
		driver = s.view.Product.Kinematics.Drivers[i]
	}
	if req.SpatialTarget != nil && !slices.Contains(s.engine.snapshot.Mechanism.UnitIDs, req.SpatialTarget.BodyID) {
		c.mu.Unlock()
		return MechanismSessionFrame{}, fmt.Errorf("%w: spatial target outside mechanism", ErrValidation)
	}
	if s.cancel != nil {
		s.cancel()
	}
	service.DiscardPreview(document, actor, s.previewID)
	s.previewID = ""
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	s.cancel = cancel
	s.sequence = req.Sequence
	s.busy = true
	e := *s.engine
	e.state, err = valuecopy.Clone(s.engine.state)
	e.calls = 0
	e.last = nil
	e.replay = nil
	c.mu.Unlock()
	started := time.Now()
	if err == nil {
		err = service.mechanismHead(ctx, document, s.view.Document.VersionID)
	}
	if err == nil {
		if req.DriverID != "" {
			err = e.SelectDriver(ctx, driver)
		} else if req.Value != nil {
			var q, reference float64
			q, err = motionValue(*req.Value, true)
			reference, _ = motionValue(*driver.Reference, true)
			if err == nil {
				_, err = e.Advance(ctx, reference+q)
			}
		} else {
			_, err = e.Drag(ctx, *req.SpatialTarget)
		}
	}
	if err == nil {
		err = service.mechanismHead(ctx, document, s.view.Document.VersionID)
	}
	c.mu.Lock()
	if c.entries[id] != s || s.sequence != req.Sequence || parent.Err() != nil {
		if c.entries[id] == s && s.sequence == req.Sequence {
			s.busy = false
			s.cancel = nil
		}
		c.mu.Unlock()
		if parent.Err() != nil {
			return MechanismSessionFrame{}, parent.Err()
		}
		if ctx.Err() != nil {
			return MechanismSessionFrame{}, ctx.Err()
		}
		return MechanismSessionFrame{}, fmt.Errorf("%w: superseded mechanism input", ErrValidation)
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("BUDGET_EXHAUSTED: command solve deadline")
	}
	s.busy = false
	s.cancel = nil
	if err == nil && req.DriverID != "" {
		prior := s.initial.Coordinates[e.driver.ID]
		var q float64
		q, err = motionJointCoordinate(e.driver, s.initial.UnitPoses, &prior)
		if err == nil {
			s.initial.DriverValue = q
		}
	}
	if err == nil {
		s.engine = &e
		s.driver = driver
		s.basis = recordingBasis(e.snapshot.Mechanism, driver)
	}
	out := s.frame(err == nil)
	if req.SpatialTarget != nil && e.last != nil {
		out.Interaction = e.last.Interaction
	}
	checkpoint, _ := valuecopy.Clone(s.engine.state.Frame)
	c.mu.Unlock()
	out.SolveMS = float64(time.Since(started).Microseconds()) / 1000
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return out, err
		}
		code := "LOCAL_SOLVE_FAILED"
		if e.transport == codes.Unavailable || e.transport == codes.Internal || e.transport == codes.InvalidArgument {
			code = "EXECUTION_FAILURE"
		}
		if strings.HasPrefix(err.Error(), "LIMIT_REACHED") {
			code = "LIMIT_REACHED"
		}
		if strings.HasPrefix(err.Error(), "UNSUPPORTED_DOF") {
			code = "UNSUPPORTED_DOF"
		}
		if strings.HasPrefix(err.Error(), "BUDGET_EXHAUSTED") {
			code = "BUDGET_EXHAUSTED"
		}
		out.Failure = &MechanismMotionFailure{Code: code, Message: err.Error()}
		diagnostic := service.newOperationDiagnostic(ctx, document, CommandRequest{RequestID: fmt.Sprintf("%s/%d", id, req.Sequence), Type: "MECHANISM_MOTION"}, "MECHANISM_MOTION")
		if diagnostic != nil {
			diagnostic.BaseRevisionID = s.view.Document.VersionID
			diagnostic.BaseModel, _ = json.Marshal(s.view.Product)
			diagnostic.Stage = "SOLVE"
			diagnostic.MotionInput = &req
			diagnostic.MotionSnapshot = &e.snapshot
			diagnostic.MotionCheckpoint = &checkpoint
			if len(e.replay) > 0 {
				diagnostic.recordAssemblyAttempt(e.replay)
			}
			service.finishOperationDiagnostic(ctx, diagnostic, nil, nil, &err)
			var retained interface{ DiagnosticID() string }
			if errors.As(err, &retained) {
				out.Failure.DiagnosticID = retained.DiagnosticID()
			}
		}
	}
	return out, nil
}

// Pin an actual published checkpoint. A client supplies time/camera and stable
// identity, never a pose or an unchecked coordinate as a recording witness.
func (service *Service) RecordMechanismKey(ctx context.Context, document, actor, id string, sequence uint64, key MotionKeyframe) (MotionKeyframe, error) {
	c := &service.mechanismSessions
	c.mu.Lock()
	defer c.mu.Unlock()
	s, err := service.mechanismSessionLocked(document, actor, id)
	if err != nil {
		return key, err
	}
	if s.busy || s.finalizing || sequence != s.sequence || key.ID == "" || len(key.ID) > 128 || key.TimeSeconds < 0 || key.TimeSeconds > 3600 || !finiteMotion(key.TimeSeconds) || !validMotionViewpoint(key.Viewpoint) {
		return key, fmt.Errorf("%w: recording requires a settled checkpoint", ErrValidation)
	}
	if _, exists := s.keys[key.ID]; !exists && len(s.keys) >= 500 {
		return key, fmt.Errorf("%w: keyframe limit", ErrValidation)
	}
	if err = service.mechanismHead(ctx, document, s.view.Document.VersionID); err != nil {
		return key, err
	}
	key.DriverID = s.driver.ID
	key.Value = MotionQuantity{Value: s.frame(true).Frame.DriverValue, Unit: "rad"}
	s.keys[key.ID] = key
	return key, nil
}

func (service *Service) FinalizeMechanismSession(parent context.Context, document, actor, id string, req MechanismSessionFinalize) (CommandRequest, error) {
	c := &service.mechanismSessions
	c.mu.Lock()
	s, err := service.mechanismSessionLocked(document, actor, id)
	if err != nil {
		c.mu.Unlock()
		return CommandRequest{}, err
	}
	if s.busy || s.finalizing || req.Sequence != s.sequence {
		c.mu.Unlock()
		return CommandRequest{}, fmt.Errorf("%w: settle the latest motion before confirmation", ErrValidation)
	}
	definition, err := valuecopy.Clone(s.view.Product.Kinematics)
	if err != nil {
		c.mu.Unlock()
		return CommandRequest{}, err
	}
	mi := slices.IndexFunc(definition.Mechanisms, func(m Mechanism) bool { return m.ID == s.engine.snapshot.Mechanism.ID })
	definition.Mechanisms[mi].Poses = mechanismMemberPoses(definition.Mechanisms[mi], s.engine.state.Frame.UnitPoses)
	definition.Mechanisms[mi].Coordinates, _ = valuecopy.Clone(s.engine.state.Frame.Coordinates)
	if req.Study != nil {
		study, cloneErr := valuecopy.Clone(*req.Study)
		if cloneErr != nil {
			c.mu.Unlock()
			return CommandRequest{}, cloneErr
		}
		if study.ID == "" || len(study.Keyframes) == 0 || study.MechanismID != s.engine.snapshot.Mechanism.ID || study.DriverID != s.driver.ID {
			c.mu.Unlock()
			return CommandRequest{}, fmt.Errorf("%w: invalid recording owner", ErrValidation)
		}
		if s.study != nil && study.ID != s.study.ID {
			c.mu.Unlock()
			return CommandRequest{}, fmt.Errorf("%w: recording identity changed", ErrValidation)
		}
		if s.study != nil && study.Keyframes[0].ID != s.study.Keyframes[0].ID {
			c.mu.Unlock()
			return CommandRequest{}, fmt.Errorf("%w: simulation initial key identity changed", ErrValidation)
		}
		if s.study == nil && slices.ContainsFunc(definition.Studies, func(x MotionStudy) bool { return x.ID == study.ID }) {
			c.mu.Unlock()
			return CommandRequest{}, fmt.Errorf("%w: recording already exists", ErrValidation)
		}
		for _, key := range study.Keyframes {
			witness, ok := s.keys[key.ID]
			q, qe := motionValue(key.Value, true)
			w, we := motionValue(witness.Value, true)
			if !ok || qe != nil || we != nil || math.Abs(q-w) > 1e-9 || key.DriverID != s.driver.ID {
				c.mu.Unlock()
				return CommandRequest{}, fmt.Errorf("%w: keyframe has no qualified checkpoint", ErrValidation)
			}
		}
		ref, _ := motionValue(*s.driver.Reference, true)
		initial := s.initial.DriverValue - ref
		first, _ := motionValue(study.Keyframes[0].Value, true)
		if study.Keyframes[0].TimeSeconds != 0 || math.Abs(first-initial) > 1e-8 {
			c.mu.Unlock()
			return CommandRequest{}, fmt.Errorf("%w: initial keyframe changed", ErrValidation)
		}
		study.SourceRevisionID = s.view.Document.VersionID
		study.BasisDigest = s.basis
		study.GeometryDigest = s.geometry
		study.Reference = s.driver.Reference
		study.InitialPoses = mechanismMemberPoses(s.engine.snapshot.Mechanism, s.initial.UnitPoses)
		study.InitialCoordinates, _ = valuecopy.Clone(s.initial.Coordinates)
		study.EnvironmentPoses = motionEnvironment(s.engine.snapshot.Mechanism, s.initial.UnitPoses)
		study.DriverJointID = s.driver.JointID
		study.DurationSeconds = study.Keyframes[len(study.Keyframes)-1].TimeSeconds
		i := slices.IndexFunc(definition.Studies, func(x MotionStudy) bool { return x.ID == study.ID })
		if i < 0 {
			definition.Studies = append(definition.Studies, study)
		} else {
			definition.Studies[i] = study
		}
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	s.cancel = cancel
	s.finalizing = true
	service.DiscardPreview(document, actor, s.previewID)
	s.previewID = ""
	c.mu.Unlock()
	command := CommandRequest{Type: "SAVE_KINEMATICS", VersionID: s.view.Document.VersionID, RequestID: newID("mechanism-confirm"), ActorID: actor, Kinematics: &definition}
	if req.Study != nil {
		i := slices.IndexFunc(definition.Studies, func(study MotionStudy) bool { return study.ID == req.Study.ID })
		ctx = withQualifiedRecording(ctx, &definition.Studies[i])
	}
	preview, err := service.PreviewCommand(ctx, document, command)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries[id] == s {
		// A deadline or disconnected confirmation may be retried against this
		// same checkpoint; it must release the confirmation barrier as well.
		s.finalizing = false
		s.cancel = nil
	}
	if c.entries[id] != s || ctx.Err() != nil {
		service.DiscardPreview(document, actor, preview.PreviewID)
		if ctx.Err() != nil {
			return CommandRequest{}, ctx.Err()
		}
		return CommandRequest{}, fmt.Errorf("%w: canceled confirmation", ErrValidation)
	}
	if err != nil {
		return CommandRequest{}, err
	}
	s.previewID = preview.PreviewID
	command.PreviewID = preview.PreviewID
	return command, nil
}

func (service *Service) CancelMechanismSession(document, actor, id string) {
	c := &service.mechanismSessions
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.entries[id]
	if s == nil || s.document != document || s.actor != actor {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	service.DiscardPreview(document, actor, s.previewID)
	delete(c.entries, id)
}

// Finite recorded paths can be evaluated without a persistent Job. Only
// checkpoints pinned by this session can enter the draft evaluation.
func (service *Service) PreviewMechanismRecording(parent context.Context, document, actor, id string, study MotionStudy) (MotionRun, error) {
	c := &service.mechanismSessions
	c.mu.Lock()
	s, err := service.mechanismSessionLocked(document, actor, id)
	if err != nil {
		c.mu.Unlock()
		return MotionRun{}, err
	}
	if s.busy || s.finalizing || len(study.Keyframes) == 0 || len(study.Keyframes) > 500 {
		c.mu.Unlock()
		return MotionRun{}, fmt.Errorf("%w: unsettled recording", ErrValidation)
	}
	for _, key := range study.Keyframes {
		w, ok := s.keys[key.ID]
		q, qe := motionValue(key.Value, true)
		v, ve := motionValue(w.Value, true)
		if !ok || qe != nil || ve != nil || math.Abs(q-v) > 1e-9 {
			c.mu.Unlock()
			return MotionRun{}, fmt.Errorf("%w: unqualified recording key", ErrValidation)
		}
	}
	snapshot, err := valuecopy.Clone(s.engine.snapshot)
	if err != nil {
		c.mu.Unlock()
		return MotionRun{}, err
	}
	study.MechanismID = snapshot.Mechanism.ID
	study.DriverID = s.driver.ID
	study.DriverJointID = s.driver.JointID
	study.Reference = s.driver.Reference
	study.SourceRevisionID = s.view.Document.VersionID
	study.BasisDigest = s.basis
	study.GeometryDigest = s.geometry
	study.InitialPoses = mechanismMemberPoses(snapshot.Mechanism, s.initial.UnitPoses)
	study.InitialCoordinates, _ = valuecopy.Clone(s.initial.Coordinates)
	study.EnvironmentPoses = motionEnvironment(snapshot.Mechanism, s.initial.UnitPoses)
	study.DurationSeconds = study.Keyframes[len(study.Keyframes)-1].TimeSeconds
	snapshot.Study = study
	snapshot.Mechanism.Poses = study.InitialPoses
	snapshot.Mechanism.Coordinates = study.InitialCoordinates
	driver := s.driver
	c.mu.Unlock()
	if err = validateKinematics(*snapshot.View.Product, KinematicsDefinitions{Mechanisms: []Mechanism{snapshot.Mechanism}, Drivers: []MotionDriver{driver}, Studies: []MotionStudy{study}}); err != nil {
		return MotionRun{}, err
	}
	if err = service.mechanismHead(parent, document, snapshot.RevisionID); err != nil {
		return MotionRun{}, err
	}
	if err = service.compileMotionSnapshot(parent, &snapshot); err != nil {
		return MotionRun{}, err
	}
	snapshot.Digest = motionDigest(snapshot)
	run, err := service.RunMotionStudy(parent, snapshot, nil)
	if err == nil {
		err = service.mechanismHead(parent, document, snapshot.RevisionID)
	}
	return run, err
}

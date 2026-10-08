package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/valuecopy"
)

const assemblyInteractionTTL = 2 * time.Minute
const assemblyInteractionCapacity = 128

// Session owns frozen computation, never a persistent model or a DB transaction.
type assemblyInteraction struct {
	actorID, documentID, workspaceID, baseRevisionID, inputDigest, bodyID string
	sequence, baseSequence                                                uint64
	manifest                                                              AssemblySolveManifest
	model                                                                 ProductModel
	nominalByID                                                           map[string]InstancePoseEntry
	manifestConstraintIndex                                               map[string]int
	definitionConstraintIndex                                             map[string]int
	geometryByID                                                          map[string]geometry.AssemblyGeometry
	localGrabPoint                                                        [3]float64
	frameRotation                                                         [4]float64
	occurrencePath                                                        *InstancePath
	editContext                                                           *AssemblyInteractionEditContext
	expires                                                               time.Time
	inFlight                                                              bool
	cancel                                                                context.CancelFunc
	previewID                                                             string
	finalTarget                                                           *geometry.AssemblyDragTarget
	goalSequence                                                          uint64
	goalDigest                                                            string
}
type assemblyInteractionCache struct {
	mu     sync.Mutex
	values map[string]*assemblyInteraction
}
type AssemblyInteractionEditContext struct {
	RootDocumentID   string        `json:"rootDocumentId"`
	RootRevisionID   string        `json:"rootRevisionId"`
	ActiveDocumentID string        `json:"activeDocumentId,omitempty"`
	ActiveRevisionID string        `json:"activeRevisionId,omitempty"`
	InstancePath     *InstancePath `json:"instancePath,omitempty"`
}
type AssemblyInteractionBegin struct {
	BaseRevisionID string                          `json:"baseRevisionId"`
	InstanceID     string                          `json:"instanceId"`
	OccurrencePath *InstancePath                   `json:"occurrencePath,omitempty"`
	LocalGrabPoint [3]float64                      `json:"localGrabPoint"`
	FrameRotation  [4]float64                      `json:"frameRotation"`
	EditContext    *AssemblyInteractionEditContext `json:"editContext,omitempty"`
}
type InstancePoseEntry struct {
	InstanceID  string     `json:"instanceId"`
	Translation [3]float64 `json:"translation"`
	Rotation    [4]float64 `json:"rotation"`
}
type AssemblyInteractionOpened struct {
	SessionID      string              `json:"sessionId"`
	BodyID         string              `json:"bodyId"`
	BaseRevisionID string              `json:"baseRevisionId"`
	InputDigest    string              `json:"inputDigest"`
	NominalPoses   []InstancePoseEntry `json:"nominalPoses"`
}
type AssemblyInteractionUpdate struct {
	GoalSequence uint64                      `json:"goalSequence,omitempty"`
	SessionID    string                      `json:"sessionId"`
	Sequence     uint64                      `json:"sequence"`
	Final        bool                        `json:"final"`
	Target       geometry.AssemblyDragTarget `json:"target"`
}
type AssemblyInteractionFrame struct {
	GoalSequence uint64 `json:"goalSequence"`
	CommandPreview
	SessionID     string                                `json:"sessionId"`
	InputDigest   string                                `json:"inputDigest"`
	Sequence      uint64                                `json:"sequence"`
	RequestID     string                                `json:"requestId"`
	Interaction   *geometry.AssemblyInteractionEvidence `json:"interaction"`
	Unchanged     bool                                  `json:"unchanged"`
	CommitCommand *CommandRequest                       `json:"commitCommand,omitempty"`
	SolveMS       float64                               `json:"solveMs"`
}

func validInteractionTarget(t geometry.AssemblyDragTarget) bool {
	if t.BodyID == "" || t.TargetSequence == 0 {
		return false
	}
	active := false
	for i := 0; i < 3; i++ {
		if (t.TranslationComponents[i] && t.HoldTranslationComponents[i]) || (t.RotationComponents[i] && t.HoldRotationComponents[i]) {
			return false
		}
		active = active || t.TranslationComponents[i] || t.RotationComponents[i]
	}
	if !active {
		return false
	}
	values := append(append(append([]float64{}, t.LocalGrabPoint[:]...), t.TargetPose.Translation[:]...), t.TargetPose.Rotation[:]...)
	values = append(values, t.FrameRotation[:]...)
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	for _, q := range [][4]float64{t.TargetPose.Rotation, t.FrameRotation} {
		norm := 0.0
		for _, v := range q {
			norm += v * v
		}
		if math.Abs(norm-1) > 1e-6 {
			return false
		}
	}
	return true
}

func interactionFrozenConstraints(input AssemblySolveManifest) ([]geometry.AssemblyConstraint, error) {
	out := append([]geometry.AssemblyConstraint(nil), input.Constraints...)
	for _, stage := range input.GroupStages {
		for _, group := range stage.Groups {
			if group.CapturePending {
				return nil, fmt.Errorf("%w: group %s has no accepted capture", ErrValidation, group.GroupID)
			}
			links, err := groupRigidConstraints(group.GroupID, group.MemberIDs, group.CapturedRelations)
			if err != nil {
				return nil, err
			}
			out = append(out, links...)
		}
	}
	if len(input.RelativeFixUpdates) > 0 {
		updates := map[string]bool{}
		for _, id := range input.RelativeFixUpdates {
			updates[id] = true
		}
		filtered := out[:0]
		for _, c := range out {
			if !updates[c.ID] {
				filtered = append(filtered, c)
			}
		}
		out = filtered
	}
	return out, nil
}

func (service *Service) BeginAssemblyInteraction(ctx context.Context, documentID, actor string, input AssemblyInteractionBegin) (AssemblyInteractionOpened, error) {
	// Retained caller pointers must not mutate the Session snapshot or indices.
	owned, err := valuecopy.Clone(input)
	if err != nil {
		return AssemblyInteractionOpened{}, fmt.Errorf("%w: invalid Begin snapshot: %v", ErrValidation, err)
	}
	input = owned

	// Reuse the production command adapter and authoritative Workspace snapshot.
	prepared, err := service.prepareDomainMutation(ctx, documentID, CommandRequest{Type: "MOVE_INSTANCE", ActorID: actor, InstanceID: input.InstanceID, Rotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		return AssemblyInteractionOpened{}, err
	}
	if prepared.documentType != "PRODUCT" || input.BaseRevisionID == "" || input.BaseRevisionID != prepared.headRevision {
		return AssemblyInteractionOpened{}, fmt.Errorf("%w: ASSEMBLY_SESSION_BASE_CHANGED", ErrValidation)
	}
	bodyID := input.InstanceID
	if input.OccurrencePath != nil {
		p := input.OccurrencePath
		if err = validateNonRootInstancePath(*p, documentID); err != nil {
			return AssemblyInteractionOpened{}, err
		}
		expansion, e := service.expandProductContext(ctx, documentID, prepared.headRevision, mustInteractionModel(prepared.modelJSON))
		if e != nil {
			return AssemblyInteractionOpened{}, e
		}
		occurrence, ok := occurrenceByCanonical(expansion, p.Canonical)
		if !ok {
			return AssemblyInteractionOpened{}, fmt.Errorf("%w: occurrence missing", ErrValidation)
		}
		if e = validateResolvedInstancePath(*p, occurrence.Path); e != nil {
			return AssemblyInteractionOpened{}, e
		}
		bodyID = p.Segments[0].InstanceID
	}
	if err = service.validateInteractionEditContext(ctx, documentID, prepared.headRevision, input.EditContext); err != nil {
		return AssemblyInteractionOpened{}, err
	}
	var model ProductModel
	if err = json.Unmarshal(prepared.modelJSON, &model); err != nil {
		return AssemblyInteractionOpened{}, err
	}
	found := false
	nominal := make([]InstancePoseEntry, 0, len(model.Instances))
	for _, i := range model.Instances {
		found = found || i.ID == bodyID
		nominal = append(nominal, InstancePoseEntry{i.ID, i.Translation, normalizedInstanceRotation(i.Rotation)})
	}
	if !found {
		return AssemblyInteractionOpened{}, fmt.Errorf("%w: assembly motion unit missing", ErrValidation)
	}
	compile := mustInteractionModel(prepared.modelJSON)
	// Admission is frozen once. Isolated definitions remain in the original model
	// and persisted Definitions; they never become physical equations in a drag.
	for i := range compile.Constraints {
		c := &compile.Constraints[i]
		if c.Mode != "MEASURED" && c.EvaluationStatus != modelcore.AssemblyConstraintVerified {
			c.Suppressed = true
		}
	}
	frozen, err := service.FreezeAssemblyInput(ctx, documentID, prepared.headRevision, compile)
	if err != nil {
		return AssemblyInteractionOpened{}, err
	}
	frozen.Definitions = append([]AssemblyConstraint(nil), model.Constraints...)
	// An explicit Move may edit RELATIVE baselines. This authorization is frozen
	// before numerical work, not admission, suppression, or a mouse-target Fix.
	movable := map[string]bool{bodyID: true}
	for changed := true; changed; {
		changed = false
		for _, stage := range frozen.GroupStages {
			connected := false
			for _, id := range stage.MemberIDs {
				connected = connected || movable[id]
			}
			if connected {
				for _, id := range stage.MemberIDs {
					if !movable[id] {
						movable[id] = true
						changed = true
					}
				}
			}
		}
	}
	for _, c := range model.Constraints {
		if c.Kind == "FIX" && c.FixMode == "RELATIVE" && !c.Suppressed && c.EvaluationStatus == modelcore.AssemblyConstraintVerified && movable[c.First.InstanceID] {
			frozen.RelativeFixUpdates = append(frozen.RelativeFixUpdates, c.ID)
		}
	}
	frozen.Purpose = "PREVIEW"
	frozen.Intent = nil
	frozen.AffectedBodyIDs = nil
	frozen.Digest = assemblyManifestDigest(frozen)
	if _, err = interactionFrozenConstraints(frozen); err != nil {
		return AssemblyInteractionOpened{}, err
	}
	// The complete connected component includes third reference bodies and every
	// frozen group edge. It is not a fixed-neighbour approximation.
	frozen, err = interactionComponent(frozen, bodyID)
	if err != nil {
		return AssemblyInteractionOpened{}, err
	}
	frozen.Digest = assemblyManifestDigest(frozen)
	q := input.FrameRotation
	if q == ([4]float64{}) {
		q = [4]float64{0, 0, 0, 1}
	}
	probe := geometry.AssemblyDragTarget{BodyID: bodyID, LocalGrabPoint: input.LocalGrabPoint, FrameRotation: q, TargetSequence: 1, TargetPose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, TranslationComponents: [3]bool{true, true, true}}
	if !validInteractionTarget(probe) {
		return AssemblyInteractionOpened{}, fmt.Errorf("%w: invalid frozen drag frame", ErrValidation)
	}
	session := &assemblyInteraction{actorID: actor, documentID: documentID, workspaceID: prepared.workspaceID, baseRevisionID: prepared.headRevision, baseSequence: prepared.headSequence, inputDigest: frozen.Digest, bodyID: bodyID, manifest: frozen, model: model, localGrabPoint: input.LocalGrabPoint, frameRotation: q, occurrencePath: input.OccurrencePath, editContext: input.EditContext, expires: time.Now().Add(assemblyInteractionTTL)}
	id := newID("assembly-session")
	session.nominalByID = make(map[string]InstancePoseEntry, len(nominal))
	for _, pose := range nominal {
		session.nominalByID[pose.InstanceID] = pose
	}
	session.manifestConstraintIndex = make(map[string]int, len(frozen.Constraints))
	for i, c := range frozen.Constraints {
		session.manifestConstraintIndex[c.ID] = i
	}
	session.definitionConstraintIndex = make(map[string]int, len(model.Constraints))
	for i, c := range model.Constraints {
		session.definitionConstraintIndex[c.ID] = i
	}
	session.geometryByID = make(map[string]geometry.AssemblyGeometry, len(frozen.Geometry))
	for _, g := range frozen.Geometry {
		session.geometryByID[g.ID] = g
	}
	cache := &service.assemblyInteractions
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values == nil {
		cache.values = map[string]*assemblyInteraction{}
	}
	for key, value := range cache.values {
		if time.Now().After(value.expires) {
			if value.cancel != nil {
				value.cancel()
			}
			service.DiscardPreview(value.documentID, value.actorID, value.previewID)
			delete(cache.values, key)
		}
	}
	if len(cache.values) >= assemblyInteractionCapacity {
		return AssemblyInteractionOpened{}, fmt.Errorf("%w: ASSEMBLY_SESSION_CAPACITY", ErrValidation)
	}
	cache.values[id] = session
	return AssemblyInteractionOpened{id, bodyID, prepared.headRevision, frozen.Digest, nominal}, nil
}
func mustInteractionModel(raw []byte) ProductModel {
	var m ProductModel
	_ = json.Unmarshal(raw, &m)
	return m
}

func interactionComponent(input AssemblySolveManifest, bodyID string) (AssemblySolveManifest, error) {
	constraints, err := interactionFrozenConstraints(input)
	if err != nil {
		return input, err
	}
	members := map[string]bool{bodyID: true}
	for changed := true; changed; {
		changed = false
		for _, c := range constraints {
			ids := []string{c.FirstBodyID, c.SecondBodyID, c.AngleReferenceBodyID}
			connected := false
			for _, id := range ids {
				connected = connected || members[id]
			}
			if connected {
				for _, id := range ids {
					if id != "" && !members[id] {
						members[id] = true
						changed = true
					}
				}
			}
		}
	}
	out := input
	out.Bodies = nil
	for _, b := range input.Bodies {
		if members[b.ID] {
			out.Bodies = append(out.Bodies, b)
		}
	}
	if len(out.Bodies) == 0 {
		return input, fmt.Errorf("%w: motion unit outside frozen input", ErrValidation)
	}
	out.Constraints = nil
	geometryIDs := map[string]bool{}
	constraintIDs := map[string]bool{}
	for _, c := range input.Constraints {
		if members[c.FirstBodyID] {
			out.Constraints = append(out.Constraints, c)
			constraintIDs[c.ID] = true
			geometryIDs[c.FirstGeometryID] = true
			geometryIDs[c.SecondGeometryID] = true
			geometryIDs[c.AngleReferenceGeometryID] = true
		}
	}
	out.GroupStages = nil
	for _, stage := range input.GroupStages {
		if len(stage.MemberIDs) > 0 && members[stage.MemberIDs[0]] {
			out.GroupStages = append(out.GroupStages, stage)
		}
	}
	out.Geometry = nil
	for _, g := range input.Geometry {
		if geometryIDs[g.ID] {
			out.Geometry = append(out.Geometry, g)
		}
	}
	out.ResolutionEvidence = nil
	for _, e := range input.ResolutionEvidence {
		if constraintIDs[e.ConstraintID] {
			out.ResolutionEvidence = append(out.ResolutionEvidence, e)
		}
	}
	return out, nil
}

func (service *Service) validateInteractionEditContext(ctx context.Context, doc, revision string, c *AssemblyInteractionEditContext) error {
	defer perf.Start(ctx, "assembly-edit-context")()
	if c == nil {
		return nil
	}
	canonical := ""
	if c.InstancePath != nil {
		canonical = c.InstancePath.Canonical
	}
	// This check needs snapshot/path/owner evidence, not a publication catalog.
	// Keep mutable root Head and document liveness reads fresh on every check.
	rootRevision, items, err := service.productContextRoot(withAssemblyReadCache(ctx), c.RootDocumentID)
	if err != nil {
		return err
	}
	active, ok := occurrenceByCanonical(items, canonical)
	if !ok {
		return fmt.Errorf("%w: active occurrence is outside the Product context", ErrValidation)
	}
	activeDoc, activeRevision := c.ActiveDocumentID, c.ActiveRevisionID
	if activeDoc == "" {
		activeDoc, activeRevision = doc, revision
	}
	if rootRevision != c.RootRevisionID || active.DocumentID != activeDoc || active.RevisionID != activeRevision || (doc == c.RootDocumentID && revision != c.RootRevisionID) {
		return fmt.Errorf("%w: ASSEMBLY_SESSION_EDIT_CONTEXT_CHANGED", ErrValidation)
	}
	ownerFound := doc == c.RootDocumentID
	for _, item := range items {
		if item.DocumentID == doc && item.RevisionID == revision {
			ownerFound = true
		}
		if doc != c.RootDocumentID && item.Path.Canonical != "" && (canonical == item.Path.Canonical || (len(canonical) > len(item.Path.Canonical) && canonical[:len(item.Path.Canonical)+1] == item.Path.Canonical+"/")) && item.ReferenceMode == "PINNED" {
			return fmt.Errorf("%w: PINNED edit context is read-only", ErrValidation)
		}
	}
	if !ownerFound {
		return fmt.Errorf("%w: solver owner outside edit context snapshot", ErrValidation)
	}
	if c.InstancePath != nil {
		if err = validateResolvedInstancePath(*c.InstancePath, active.Path); err != nil {
			return err
		}
	}
	return nil
}
func (service *Service) checkInteractionBase(ctx context.Context, s *assemblyInteraction) error {
	defer perf.Start(ctx, "assembly-base-check")()
	var head, workspace string
	var seq uint64
	err := service.database.QueryRow(ctx, `SELECT id::text,head_revision_id::text,head_sequence FROM occccad.workspaces WHERE document_id=$1 AND name='main'`, s.documentID).Scan(&workspace, &head, &seq)
	if err != nil {
		return err
	}
	if head != s.baseRevisionID || seq != s.baseSequence || workspace != s.workspaceID {
		return fmt.Errorf("%w: ASSEMBLY_SESSION_BASE_CHANGED", ErrValidation)
	}
	return service.validateInteractionEditContext(ctx, s.documentID, s.baseRevisionID, s.editContext)
}

func (service *Service) UpdateAssemblyInteraction(ctx context.Context, documentID, actor string, input AssemblyInteractionUpdate) (AssemblyInteractionFrame, error) {
	defer perf.Start(ctx, "assembly-update")()
	cache := &service.assemblyInteractions
	finishLock := perf.Start(ctx, "assembly-session-lock-wait")
	cache.mu.Lock()
	finishLock()
	finishPrepare := perf.Start(ctx, "assembly-update-prepare")
	defer func() {
		if finishPrepare != nil {
			finishPrepare()
		}
	}()
	s := cache.values[input.SessionID]
	if s == nil || s.actorID != actor || s.documentID != documentID || time.Now().After(s.expires) {
		cache.mu.Unlock()
		return AssemblyInteractionFrame{}, fmt.Errorf("%w: ASSEMBLY_SESSION_EXPIRED_OR_SCOPE_MISMATCH", ErrValidation)
	}
	if s.inFlight || input.Sequence <= s.sequence || input.Sequence != input.Target.TargetSequence || !validInteractionTarget(input.Target) || input.Target.BodyID != s.bodyID || input.Target.LocalGrabPoint != s.localGrabPoint || input.Target.FrameRotation != s.frameRotation {
		cache.mu.Unlock()
		return AssemblyInteractionFrame{}, fmt.Errorf("%w: ASSEMBLY_SESSION_TARGET_INVALID_OR_BUSY", ErrValidation)
	}
	s.inFlight = true
	goal := input.GoalSequence
	if goal == 0 {
		goal = input.Sequence
	}
	semanticTarget := input.Target
	semanticTarget.TargetSequence = 0
	rawGoal, _ := json.Marshal(semanticTarget)
	goalDigest := canonicalModelHash(rawGoal)
	if goal > input.Sequence || goal < s.goalSequence || goal == s.goalSequence && goalDigest != s.goalDigest {
		s.inFlight = false
		cache.mu.Unlock()
		return AssemblyInteractionFrame{}, fmt.Errorf("%w: ASSEMBLY_SESSION_GOAL_MISMATCH", ErrValidation)
	}
	s.goalSequence, s.goalDigest = goal, goalDigest
	s.sequence = input.Sequence
	s.expires = time.Now().Add(assemblyInteractionTTL)
	service.DiscardPreview(documentID, actor, s.previewID)
	s.previewID = ""
	s.finalTarget = nil
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	s.cancel = cancel
	frozen := s.manifest
	frozen.Bodies = append([]geometry.AssemblyBody(nil), s.manifest.Bodies...)
	frozen.Constraints = append([]geometry.AssemblyConstraint(nil), s.manifest.Constraints...)
	target := input.Target
	frozen.DragTarget = &target
	frozen.InteractionGoalSequence = goal
	cache.mu.Unlock()
	frozen.Digest = assemblyManifestDigest(frozen)
	finishPrepare()
	finishPrepare = nil
	defer func() {
		cancel()
		cache.mu.Lock()
		if cache.values[input.SessionID] == s {
			s.inFlight = false
			s.cancel = nil
		}
		cache.mu.Unlock()
	}()
	if err := service.checkInteractionBase(work, s); err != nil {
		return AssemblyInteractionFrame{}, err
	}
	started := time.Now()
	requestID := newID("assembly-drag")
	result, err := service.solveInteractionTrajectory(work, requestID, frozen)
	if err != nil {
		return AssemblyInteractionFrame{}, err
	}
	if err = service.checkInteractionBase(work, s); err != nil {
		return AssemblyInteractionFrame{}, err
	}
	finishPublishWait := perf.Start(ctx, "assembly-session-lock-wait")
	cache.mu.Lock()
	finishPublishWait()
	current := cache.values[input.SessionID] == s && work.Err() == nil
	cache.mu.Unlock()
	if !current {
		return AssemblyInteractionFrame{}, fmt.Errorf("%w: ASSEMBLY_SESSION_CANCELLED", ErrValidation)
	}
	// inFlight grants this update sole ownership of model/warm/branch state.
	// The global lock protects lifecycle and tokens only; no DB/RPC/JSON work
	// below holds it. Cancellation can revoke membership at any point.
	defer perf.Start(ctx, "assembly-update-publish")()
	frame := AssemblyInteractionFrame{CommandPreview: CommandPreview{BaseVersionID: s.baseRevisionID, BaseSequence: s.baseSequence, AssemblySolverBuild: result.SolverBuild, AssemblyComponents: result.Components}, SessionID: input.SessionID, InputDigest: s.inputDigest, Sequence: input.Sequence, RequestID: requestID, Interaction: result.Interaction, SolveMS: float64(time.Since(started).Microseconds()) / 1000}
	if result.Interaction == nil {
		frame.GoalSequence = goal
		return frame, fmt.Errorf("%w: missing interaction evidence", ErrValidation)
	}
	frame.GoalSequence = goal
	if result.SolverBuild != assemblySolverBuildPolicy || result.Interaction.TargetSequence != input.Sequence {
		return frame, fmt.Errorf("%w: ASSEMBLY_SESSION_POLICY_OR_TARGET_MISMATCH", ErrValidation)
	}
	if !result.Interaction.HardFeasible || result.Interaction.Status == "FAILED" || result.Interaction.Status == "CANCELLED" {
		return frame, nil
	}
	if !result.Interaction.TargetConverged || !result.Interaction.EligibleForCommit {
		// Displayable physical witness is not a qualified continuation checkpoint.
		for _, b := range result.Bodies {
			frame.InstancePoses = append(frame.InstancePoses, struct {
				InstanceID  string     `json:"instanceId"`
				Translation [3]float64 `json:"translation"`
				Rotation    [4]float64 `json:"rotation"`
			}{b.ID, b.Pose.Translation, b.Pose.Rotation})
		}
		return frame, nil
	}
	// Only current, feasible accepted frames transport warm state. Nominal is immutable.
	byID := map[string]geometry.AssemblyPose{}
	for _, b := range result.Bodies {
		byID[b.ID] = b.Pose
	}
	finishCopy := perf.Start(ctx, "assembly-model-copy")
	// Copy only the two value slices written below. Nested definitions,
	// paths, groups and context bindings remain read-only frozen values.
	next := s.model
	next.Instances = append([]ProductInstance(nil), s.model.Instances...)
	next.Constraints = append([]AssemblyConstraint(nil), s.model.Constraints...)
	finishCopy()
	for i := range next.Instances {
		if p, ok := byID[next.Instances[i].ID]; ok {
			next.Instances[i].Translation = p.Translation
			next.Instances[i].Rotation = p.Rotation
		}
	}
	for _, id := range frozen.RelativeFixUpdates {
		for i := range next.Constraints {
			c := &next.Constraints[i]
			if c.ID == id {
				if pose, ok := byID[c.First.InstanceID]; ok {
					p := InstancePose{Translation: pose.Translation, Rotation: pose.Rotation}
					c.FixedPose = &p
				}
			}
		}
	}
	for i := range s.manifest.Bodies {
		if p, ok := byID[s.manifest.Bodies[i].ID]; ok {
			pCopy := p
			s.manifest.Bodies[i].InitialGuess = &pCopy
		}
	}
	for _, branch := range result.AngleBranches {
		if i, ok := s.manifestConstraintIndex[branch.ConstraintID]; ok {
			state := branch.State
			s.manifest.Constraints[i].AngleBranchState = &state
		}
	}
	resolved := s.geometryByID
	instances := map[string]*ProductInstance{}
	for i := range next.Instances {
		instances[next.Instances[i].ID] = &next.Instances[i]
	}
	for i := range next.Constraints {
		c := &next.Constraints[i]
		if c.Suppressed || c.Mode == "MEASURED" {
			continue
		}
		if j, ok := s.manifestConstraintIndex[c.ID]; ok {
			compiled := s.manifest.Constraints[j]
			if compiled.Kind != "ANGLE" || compiled.AngleReferenceDirection != nil || compiled.AngleReferenceGeometryID != "" {
				continue
			}
			sense := 1.0
			if c.Value > math.Pi {
				sense = -1
			}
			if branch := spatialAngleBranchDirection(resolved[compiled.FirstGeometryID].Direction, resolved[compiled.SecondGeometryID].Direction, instances[compiled.FirstBodyID], instances[compiled.SecondBodyID], sense); branch != nil {
				c.SpatialAngleBranchDirection = branch
				s.manifest.Constraints[j].SpatialAngleBranchDirection = branch
			}
		}
	}
	measurementModel := ProductModel{}
	for _, c := range next.Constraints {
		if _, ok := s.manifestConstraintIndex[c.ID]; ok && c.Mode == "MEASURED" {
			measurementModel.Constraints = append(measurementModel.Constraints, c)
		}
	}
	applyAssemblyMeasurements(&measurementModel, result, frozen.SolverProfile, nil)
	for _, c := range measurementModel.Constraints {
		if i, ok := s.definitionConstraintIndex[c.ID]; ok {
			next.Constraints[i] = c
		}
	}
	finishNominal := perf.Start(ctx, "assembly-nominal-compare")
	unchanged := true
	for _, i := range next.Instances {
		frame.InstancePoses = append(frame.InstancePoses, struct {
			InstanceID  string     `json:"instanceId"`
			Translation [3]float64 `json:"translation"`
			Rotation    [4]float64 `json:"rotation"`
		}{i.ID, i.Translation, i.Rotation})
		if n, ok := s.nominalByID[i.ID]; ok && !interactionPoseEqual(n, InstancePoseEntry{i.ID, i.Translation, i.Rotation}) {
			unchanged = false
		}
	}
	finishNominal()
	finishSerialize := perf.Start(ctx, "assembly-model-serialize")
	raw, err := json.Marshal(next)
	finishSerialize()
	if err != nil {
		return frame, err
	}
	cache.mu.Lock()
	current = cache.values[input.SessionID] == s && work.Err() == nil
	if current {
		s.model = next
	}
	cache.mu.Unlock()
	if !current {
		return frame, fmt.Errorf("%w: ASSEMBLY_SESSION_CANCELLED", ErrValidation)
	}
	frame.ModelHash = canonicalModelHash(raw)
	frame.Unchanged = unchanged
	frame.ConstraintLimited = result.Interaction.Status == "CONSTRAINED"
	if !input.Final || !result.Interaction.EligibleForCommit || unchanged {
		return frame, nil
	}
	command := CommandRequest{Type: "MOVE_INSTANCE", ActorID: actor, RequestID: requestID, InstanceID: s.bodyID, Translation: target.TargetPose.Translation, Rotation: target.TargetPose.Rotation, SessionID: input.SessionID, InteractionTarget: &target}
	prepared, err := service.prepareDomainMutation(work, documentID, command)
	if err != nil {
		return frame, err
	}
	if prepared.headRevision != s.baseRevisionID || prepared.headSequence != s.baseSequence {
		return frame, fmt.Errorf("%w: ASSEMBLY_SESSION_BASE_CHANGED", ErrValidation)
	}
	// Persist the complete immutable replay input/result before offering a token.
	frozen.Purpose = "PREVIEW"
	frozen.Definitions = append([]AssemblyConstraint(nil), next.Constraints...)
	frozen.ResolvedAlignmentBranches = append([]geometry.AssemblySolvedAlignmentBranch(nil), result.AlignmentBranches...)
	frozen.ResolvedDistanceBranches = append([]geometry.AssemblySolvedDistanceBranch(nil), result.DistanceBranches...)
	frozen.Digest = assemblyManifestDigest(frozen)
	previewRequestID := "preview/" + requestID
	if err = service.persistAssemblySolveManifest(work, frozen); err != nil {
		return frame, err
	}
	if err = service.recordAssemblySolveResult(work, frozen, previewRequestID, result, nil); err != nil {
		return frame, err
	}
	if err = service.checkInteractionBase(work, s); err != nil {
		return frame, err
	}
	previewID := newID("preview")
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values[input.SessionID] != s || work.Err() != nil {
		return frame, fmt.Errorf("%w: ASSEMBLY_SESSION_CANCELLED", ErrValidation)
	}
	service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: actor, headRevision: s.baseRevisionID, headSequence: s.baseSequence, commandType: prepared.command.TypeURI, payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: raw, assemblyPreviewRequestID: previewRequestID, expiresAt: time.Now().Add(interactionCandidateTTL)})
	s.previewID = previewID
	s.finalTarget = &target
	command.PreviewID = previewID
	frame.PreviewID = previewID
	frame.CommitCommand = &command
	return frame, nil
}
func interactionPoseEqual(a, b InstancePoseEntry) bool {
	for i := 0; i < 3; i++ {
		if math.Abs(a.Translation[i]-b.Translation[i]) > 1e-7 {
			return false
		}
	}
	same, opposite := 0.0, 0.0
	for i := 0; i < 4; i++ {
		same += math.Pow(a.Rotation[i]-b.Rotation[i], 2)
		opposite += math.Pow(a.Rotation[i]+b.Rotation[i], 2)
	}
	return math.Min(same, opposite) < 1e-16
}
func (service *Service) CancelAssemblyInteraction(documentID, actor, id string) {
	cache := &service.assemblyInteractions
	cache.mu.Lock()
	defer cache.mu.Unlock()
	s := cache.values[id]
	if s == nil || s.documentID != documentID || s.actorID != actor {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	service.DiscardPreview(documentID, actor, s.previewID)
	delete(cache.values, id)
}
func (service *Service) validateInteractionCommit(ctx context.Context, doc string, prepared preparedDomainMutation, request CommandRequest) error {
	cache := &service.assemblyInteractions
	cache.mu.Lock()
	s := cache.values[request.SessionID]
	if s == nil || s.actorID != prepared.actorID || s.documentID != doc || time.Now().After(s.expires) || s.inFlight || s.previewID == "" || s.previewID != request.PreviewID || s.finalTarget == nil || request.InteractionTarget == nil || !reflect.DeepEqual(*s.finalTarget, *request.InteractionTarget) || prepared.headRevision != s.baseRevisionID || prepared.headSequence != s.baseSequence {
		cache.mu.Unlock()
		return fmt.Errorf("%w: ASSEMBLY_SESSION_FINAL_CANDIDATE_STALE_OR_MISMATCHED", ErrValidation)
	}
	cache.mu.Unlock()
	if err := service.checkInteractionBase(ctx, s); err != nil {
		return err
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values[request.SessionID] != s || s.inFlight || s.previewID != request.PreviewID || s.finalTarget == nil || !reflect.DeepEqual(*s.finalTarget, *request.InteractionTarget) {
		return fmt.Errorf("%w: ASSEMBLY_SESSION_FINAL_CANDIDATE_STALE_OR_MISMATCHED", ErrValidation)
	}
	return ctx.Err()
}

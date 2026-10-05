package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"github.com/qmuntal/stateless"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func inverseRelativePose(first, second InstancePose) InstancePose {
	q := normalizedInstanceRotation(second.Rotation)
	n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	q = [4]float64{q[0] / n, q[1] / n, q[2] / n, q[3] / n}
	qi := [4]float64{-q[0], -q[1], -q[2], q[3]}
	mul := func(a, b [4]float64) [4]float64 {
		return [4]float64{a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1], a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0], a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3], a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2]}
	}
	d := [4]float64{first.Translation[0] - second.Translation[0], first.Translation[1] - second.Translation[1], first.Translation[2] - second.Translation[2], 0}
	r := mul(mul(qi, d), q)
	return InstancePose{Translation: [3]float64{r[0], r[1], r[2]}, Rotation: mul(qi, normalizedInstanceRotation(first.Rotation))}
}

func normalizedInstanceRotation(value [4]float64) [4]float64 {
	if value == [4]float64{} {
		return [4]float64{0, 0, 0, 1}
	}
	return value
}

type assemblyConstraintCapabilities struct {
	direction     bool
	distanceSide  bool
	directedAngle bool
}

type assemblySolveFailure struct {
	definitionPersistable bool
	status                string
	diagnostic            string
	code                  string
	phase                 string
	retryable             bool
}

func validatePersistentAssemblyReference(reference AssemblyGeometryRef) error {
	if reference.PersistentSelection == nil || reference.SourceVersionID == "" {
		return fmt.Errorf("%w: persistent topology reference is required", ErrValidation)
	}
	if reference.GeometryKey != "" || reference.TopologyID != 0 {
		return fmt.Errorf("%w: revision-local pick evidence cannot be persisted", ErrValidation)
	}
	return reference.PersistentSelection.Validate()
}

func (failure *assemblySolveFailure) Error() string {
	return fmt.Sprintf("assembly solve %s: %s", failure.status, failure.diagnostic)
}

func (failure *assemblySolveFailure) Unwrap() error   { return ErrValidation }
func (failure *assemblySolveFailure) Code() string    { return failure.code }
func (failure *assemblySolveFailure) Phase() string   { return failure.phase }
func (failure *assemblySolveFailure) Retryable() bool { return failure.retryable }

const (
	assemblySolveResolving = "RESOLVING_GEOMETRY"
	assemblySolveSolving   = "SOLVING"
	assemblySolveApplying  = "APPLYING_RESULT"
	assemblySolveCompleted = "COMPLETED"
	assemblySolveFailed    = "FAILED"
	assemblySolveAdvance   = "ADVANCE"
	assemblySolveFail      = "FAIL"
)

type assemblySolveWorkflow struct{ machine *stateless.StateMachine }

func newAssemblySolveWorkflow() *assemblySolveWorkflow {
	machine := stateless.NewStateMachine(assemblySolveResolving)
	machine.Configure(assemblySolveResolving).
		Permit(assemblySolveAdvance, assemblySolveSolving).
		Permit(assemblySolveFail, assemblySolveFailed)
	machine.Configure(assemblySolveSolving).
		Permit(assemblySolveAdvance, assemblySolveApplying).
		Permit(assemblySolveFail, assemblySolveFailed)
	machine.Configure(assemblySolveApplying).
		Permit(assemblySolveAdvance, assemblySolveCompleted).
		Permit(assemblySolveFail, assemblySolveFailed)
	return &assemblySolveWorkflow{machine: machine}
}

func (workflow *assemblySolveWorkflow) advance(ctx context.Context) error {
	return workflow.machine.FireCtx(ctx, assemblySolveAdvance)
}

func (workflow *assemblySolveWorkflow) failure(ctx context.Context, status, code, diagnostic string, retryable bool) error {
	phase := fmt.Sprint(workflow.machine.MustState())
	if err := workflow.machine.FireCtx(ctx, assemblySolveFail); err != nil {
		return fmt.Errorf("assembly solve workflow transition from %s: %w", phase, err)
	}
	return &assemblySolveFailure{status: status, diagnostic: diagnostic, code: code,
		phase: phase, retryable: retryable, definitionPersistable: phase == assemblySolveSolving && status != "INVALID_MODEL"}
}

// assemblyCapabilities is the authoritative application-layer geometry-pair
// matrix. It operates on exact descriptors resolved from topology, rather than
// on the browser's FACE/EDGE pick category.
func assemblyCapabilities(kind, firstKind, secondKind string) assemblyConstraintCapabilities {
	planePair := firstKind == "PLANE" && secondKind == "PLANE"
	directional := func(k string) bool { return k == "AXIS" || k == "PLANE" || k == "CYLINDER" }
	axisLike := func(k string) bool { return k == "AXIS" || k == "CYLINDER" }
	axisPair := axisLike(firstKind) && axisLike(secondKind)
	return assemblyConstraintCapabilities{
		direction:     ((kind == "PARALLEL" || kind == "PERPENDICULAR") && directional(firstKind) && directional(secondKind)) || (kind == "COINCIDENT" && (planePair || axisPair)) || (kind == "CONCENTRIC" && axisPair) || (kind == "DISTANCE" && planePair),
		distanceSide:  kind == "DISTANCE" && (firstKind == "PLANE" || secondKind == "PLANE"),
		directedAngle: kind == "ANGLE" && directional(firstKind) && directional(secondKind),
	}
}

func (service *Service) solveAssemblySet(ctx context.Context, documentID, rootRevisionID, requestID, drivenInstanceID string, intent *geometry.AssemblySolveIntent, model *ProductModel, warmStartKey string, excluded map[string]bool, probe bool, evidence ...*geometry.AssemblySolve) (returnErr error) {
	diagnostic := service.newOperationDiagnostic(ctx, documentID, CommandRequest{RequestID: requestID, Type: "ASSEMBLY_SOLVE"}, "ASSEMBLY")
	if diagnostic != nil {
		diagnostic.BaseRevisionID = rootRevisionID
		diagnostic.BaseModel, _ = json.Marshal(model)
		for id, skip := range excluded {
			if skip {
				diagnostic.ExcludedConstraintIDs = append(diagnostic.ExcludedConstraintIDs, id)
			}
		}
		sort.Strings(diagnostic.ExcludedConstraintIDs)
	}
	defer func() {
		if diagnostic != nil {
			diagnostic.Stage = "ASSEMBLY_SOLVE"
			if f, ok := returnErr.(*assemblySolveFailure); ok {
				diagnostic.Stage = f.phase
			}
			raw, _ := json.Marshal(model)
			candidate := json.RawMessage(raw)
			service.finishOperationDiagnostic(ctx, diagnostic, &candidate, nil, &returnErr)
		}
	}()
	if err := resolveAssemblyQuantities(model); err != nil {
		return err
	}
	if len(model.Constraints) == 0 {
		return nil
	}
	finishPrepare := perf.Start(ctx, "assembly-prepare")
	preparing := true
	defer func() {
		if preparing {
			finishPrepare()
		}
	}()
	workflow := newAssemblySolveWorkflow()
	defer func() {
		if returnErr == nil {
			return
		}
		// Cancellation/deadline are request transport semantics. Preserve them so
		// the HTTP boundary can return its established timeout response instead of
		// misclassifying them as an assembly-model failure.
		if errors.Is(returnErr, context.Canceled) || errors.Is(returnErr, context.DeadlineExceeded) || errors.Is(returnErr, database.ErrBusy) {
			return
		}
		var failure *assemblySolveFailure
		if errors.As(returnErr, &failure) {
			return
		}
		deterministic := errors.Is(returnErr, ErrValidation) || errors.Is(returnErr, ErrNotFound)
		returnErr = workflow.failure(context.Background(), "INVALID_MODEL",
			"ASSEMBLY_GEOMETRY_RESOLUTION_FAILED", returnErr.Error(), !deterministic)
	}()
	if service.worker == nil {
		return workflow.failure(context.Background(), "NUMERICAL_FAILURE",
			"ASSEMBLY_SOLVER_UNAVAILABLE", "assembly solver is unavailable", true)
	}
	if err := service.resolveAssemblySupports(ctx, model, excluded); err != nil {
		return err
	}
	instances := make(map[string]*ProductInstance, len(model.Instances))
	bodies := make([]geometry.AssemblyBody, 0, len(model.Instances))
	warmStarts := service.assemblyWarmStarts.get(warmStartKey)
	for index := range model.Instances {
		instance := &model.Instances[index]
		instance.Rotation = normalizedInstanceRotation(instance.Rotation)
		instances[instance.ID] = instance
		body := geometry.AssemblyBody{ID: instance.ID, Pose: geometry.AssemblyPose{Translation: instance.Translation, Rotation: instance.Rotation}}
		if warm, ok := warmStarts[instance.ID]; ok {
			guess := geometry.AssemblyPose{Translation: warm.Translation, Rotation: warm.Rotation}
			body.InitialGuess = &guess
		}
		bodies = append(bodies, body)
	}
	contextVariants, err := service.contextVariantsForProductModel(ctx, *model)
	if err != nil {
		return err
	}
	applyVariantPublication := func(reference *AssemblyGeometryRef) error {
		if reference == nil || reference.PublicationRef == nil {
			return nil
		}
		if reference.InstancePath != nil && len(reference.InstancePath.Segments) > 1 {
			return nil // nested source is resolved at its own accepted occurrence
		}
		publication, ok, err := service.variantPublicationForAssembly(ctx, *model, reference.InstanceID,
			reference.PublicationRef.PublicationID, contextVariants)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if !publicationReferenceCompatible(*reference.PublicationRef, publication) {
			return fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
		}
		if err := applyPublicationDescriptor(reference, publication); err != nil {
			return err
		}
		resolution := publication.Resolution
		reference.PublicationResolution = &resolution
		if publication.Target.PersistentSelection != nil {
			selection := *publication.Target.PersistentSelection
			reference.PublicationRef.PersistentSelection = &selection
		}
		return nil
	}
	for index := range model.Constraints {
		if model.Constraints[index].Suppressed || excluded[model.Constraints[index].ID] {
			continue
		}
		if err := applyVariantPublication(&model.Constraints[index].First); err != nil {
			return err
		}
		if err := applyVariantPublication(model.Constraints[index].Second); err != nil {
			return err
		}
		if model.Constraints[index].AngleRelation == "DIRECTED" {
			if err := applyVariantPublication(model.Constraints[index].AngleAxis); err != nil {
				return err
			}
		}
	}
	resolver := newAssemblySupportResolver(ctx, service, model)
	geometryValues := make([]geometry.AssemblyGeometry, 0)
	seenGeometry := map[string]bool{}
	resolvedGeometry := map[string]geometry.AssemblyGeometry{}
	resolveRef := func(reference AssemblyGeometryRef) (string, error) {
		value, err := resolver.resolve(reference)
		if err != nil {
			return "", err
		}
		if value.Kind == "BODY" {
			return "", nil
		}
		if !seenGeometry[value.ID] {
			seenGeometry[value.ID] = true
			resolvedGeometry[value.ID] = value
			geometryValues = append(geometryValues, value)
		}
		return value.ID, nil
	}
	constraints := make([]geometry.AssemblyConstraint, 0, len(model.Constraints))
	hasUnresolvedActiveConstraint := false
	resolutionEvidence := make([]AssemblyResolutionEvidence, 0, len(model.Constraints)*2)
	appendResolutionEvidence := func(constraintID, endpoint, geometryKey string, reference AssemblyGeometryRef) {
		evidence := AssemblyResolutionEvidence{ConstraintID: constraintID, Endpoint: endpoint, InstanceID: reference.InstanceID,
			PublicationRef: reference.PublicationRef, Publication: reference.PublicationResolution,
			Persistent: reference.Resolution, SourceVersionID: reference.SourceVersionID,
			DescriptorDigest: resolvedDigest(resolvedGeometry[geometryKey])}
		resolutionEvidence = append(resolutionEvidence, evidence)
	}
	if drivenInstanceID != "" {
		if driven := instances[drivenInstanceID]; driven != nil {
			pose := geometry.AssemblyPose{Translation: driven.Translation, Rotation: normalizedInstanceRotation(driven.Rotation)}
			constraints = append(constraints, geometry.AssemblyConstraint{ID: "interaction-driver", Kind: "FIX", FirstBodyID: drivenInstanceID, FixedPose: &pose})
		}
	}
	for constraintIndex := range model.Constraints {
		constraint := &model.Constraints[constraintIndex]
		if isAssemblyGroup(*constraint) {
			continue
		}
		if constraint.Suppressed || excluded[constraint.ID] {
			continue
		}
		if constraint.EvaluationStatus == modelcore.AssemblyConstraintBroken {
			hasUnresolvedActiveConstraint = true
			// Broken definitions remain in the manifest and fail the active Release
			// gate; unrelated connected constraints can still be solved.
			continue
		}
		firstGeometry, err := resolveRef(constraint.First)
		if err != nil {
			if markAssemblyImpossible(constraint, err) {
				hasUnresolvedActiveConstraint = true
				continue
			}
			return err
		}
		appendResolutionEvidence(constraint.ID, "FIRST", firstGeometry, constraint.First)
		value := geometry.AssemblyConstraint{ID: constraint.ID, ConnectionID: constraint.ConnectionID, Kind: constraint.Kind, Mode: constraint.Mode, FirstBodyID: constraint.First.InstanceID, FirstGeometryID: firstGeometry, Value: constraint.Value, DirectionRelation: constraint.DirectionRelation, DistanceRelation: constraint.DistanceRelation,
			ContactKind: constraint.ContactKind, ContactSide: constraint.ContactSide, ContactBranch: constraint.ContactBranch,
			AngleReferenceDirection: constraint.AngleReferenceDirection, SpatialAngleBranchDirection: constraint.SpatialAngleBranchDirection}
		if err := applyAssemblyAngleRelation(constraint, &value); err != nil {
			return err
		}
		if constraint.Kind == "FIX" || constraint.Kind == "RIGID" {
			fixedValue := constraint.FixedPose
			if constraint.Kind == "FIX" && (fixedValue == nil || constraint.FixMode == "RELATIVE") {
				instance := instances[constraint.First.InstanceID]
				fixedValue = &InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
				constraint.FixedPose = fixedValue
			}
			if fixedValue == nil {
				return fmt.Errorf("%w: rigid constraint is missing its captured relative pose", ErrValidation)
			}
			fixed := geometry.AssemblyPose{Translation: fixedValue.Translation, Rotation: normalizedInstanceRotation(fixedValue.Rotation)}
			value.FixedPose = &fixed
		}
		if constraint.Kind != "FIX" && constraint.Second != nil {
			secondGeometry, resolveErr := resolveRef(*constraint.Second)
			if resolveErr != nil {
				if markAssemblyImpossible(constraint, resolveErr) {
					hasUnresolvedActiveConstraint = true
					continue
				}
				return resolveErr
			}
			appendResolutionEvidence(constraint.ID, "SECOND", secondGeometry, *constraint.Second)
			value.SecondBodyID, value.SecondGeometryID = constraint.Second.InstanceID, secondGeometry
		}
		if value.Kind == "ANGLE" && constraint.AngleAxis != nil {
			axisKey, err := resolveRef(*constraint.AngleAxis)
			if err != nil {
				if markAssemblyImpossible(constraint, err) {
					hasUnresolvedActiveConstraint = true
					continue
				}
				return err
			}
			axis := resolvedGeometry[axisKey]
			if axis.Kind != "AXIS" && axis.Kind != "PLANE" && axis.Kind != "CYLINDER" {
				markAssemblyImpossible(constraint, fmt.Errorf("%w: reference axis has no exact direction", ErrValidation))
				hasUnresolvedActiveConstraint = true
				continue
			}
			value.AngleReferenceBodyID, value.AngleReferenceGeometryID, value.ReverseAngleReference = axis.BodyID, axisKey, constraint.ReverseAngleAxis
			constraint.AngleReferenceDirection, value.AngleReferenceDirection = nil, nil
			appendResolutionEvidence(constraint.ID, "ANGLE_AXIS", axisKey, *constraint.AngleAxis)
		}
		if constraint.Kind != "FIX" && constraint.Kind != "RIGID" {
			firstKind := resolvedGeometry[firstGeometry].Kind
			secondKind := resolvedGeometry[value.SecondGeometryID].Kind
			if constraint.Family == "Coincidence" &&
				(firstKind == "AXIS" || firstKind == "CYLINDER") && (secondKind == "AXIS" || secondKind == "CYLINDER") {
				value.Kind = "CONCENTRIC" // public coaxial relation never equates radii
			}
			if constraint.Family == "Coincidence" && constraint.Subtype == "point-surface" &&
				((firstKind == "POINT" && secondKind == "CYLINDER") || (secondKind == "POINT" && firstKind == "CYLINDER")) {
				value.Kind = "SURFACE_INCIDENCE" // not the legacy point-to-cylinder-axis shortcut
			}
			if err := compileAssemblyOffset(*constraint, &value, resolvedGeometry[firstGeometry], resolvedGeometry[value.SecondGeometryID]); err != nil {
				return err
			}
			capabilities := assemblyCapabilities(value.Kind, firstKind, secondKind)
			if constraint.AngleRelation == "PERPENDICULAR" {
				value.DirectionRelation = "SAME"
			} else if value.Kind == "ANGLE" && (value.AngleReferenceDirection != nil || value.AngleReferenceGeometryID != "") {
				value.DirectionRelation = "SAME" // compiled primitive, not persisted user branch intent
			} else if !capabilities.direction {
				constraint.DirectionRelation, value.DirectionRelation = "UNORIENTED", "UNORIENTED"
			} else if constraint.DirectionRelation == "" {
				constraint.DirectionRelation, value.DirectionRelation = "UNORIENTED", "UNORIENTED"
			}
			if !capabilities.distanceSide {
				constraint.DistanceRelation, value.DistanceRelation = "UNSIGNED", "UNSIGNED"
			}
			if value.Kind == "ANGLE" && (value.AngleReferenceDirection != nil || value.AngleReferenceGeometryID != "") && !capabilities.directedAngle {
				return fmt.Errorf("%w: directed angle requires two directional supports", ErrValidation)
			}

		}
		if value.Kind == "ANGLE" && value.Value == 2*math.Pi {
			constraint.Value, value.Value = 0, 0
		}
		if value.Kind == "ANGLE" && value.AngleReferenceDirection == nil && value.AngleReferenceGeometryID == "" {
			if constraint.SpatialAngleBranchDirection == nil {
				constraint.SpatialAngleBranchDirection = spatialAngleBranchDirection(resolvedGeometry[firstGeometry].Direction, resolvedGeometry[value.SecondGeometryID].Direction,
					instances[constraint.First.InstanceID], instances[constraint.Second.InstanceID], 1)
			}
			value.SpatialAngleBranchDirection = constraint.SpatialAngleBranchDirection
		}
		if reason := incompatibleAssemblyGeometry(value, resolvedGeometry[value.FirstGeometryID], resolvedGeometry[value.SecondGeometryID]); reason != "" {
			constraint.EvaluationStatus = modelcore.AssemblyConstraintImpossible
			constraint.EvaluationSummary = reason
			hasUnresolvedActiveConstraint = true
			if _, diagnosticCapture := ctx.Value(assemblyFrozenInputKey{}).(*AssemblySolveManifest); diagnosticCapture {
				// A read-only diagnostic must retain the incompatible target and
				// its exact descriptors; the production admission path still
				// isolates it, and this capture never sends it to that path.
				constraints = append(constraints, value)
			}
			continue
		}
		constraints = append(constraints, value)
	}
	// A failed resolution is not an unconstrained solve. Only a genuinely
	// empty active set receives empty-set solver evidence.
	if len(constraints) == 0 && hasUnresolvedActiveConstraint && !hasActiveAssemblyGroups(*model, excluded) {
		return nil
	}
	if err := workflow.advance(context.Background()); err != nil {
		return err
	}
	modelJSON, err := json.Marshal(model)
	if err != nil {
		return err
	}
	included := map[string]bool{}
	for _, c := range constraints {
		included[c.ID] = true
	}
	filteredEvidence := resolutionEvidence[:0]
	for _, e := range resolutionEvidence {
		if included[e.ConstraintID] {
			filteredEvidence = append(filteredEvidence, e)
		}
	}
	resolutionEvidence = filteredEvidence
	manifest, err := newAssemblySolveManifest(documentID, rootRevisionID, canonicalModelHash(modelJSON), bodies, geometryValues,
		constraints, intent, nil, resolutionEvidence, model.Constraints)
	if err != nil {
		return err
	}
	manifest.GroupStages, err = prepareAssemblyGroupStages(manifest, excluded)
	if err != nil {
		return err
	}
	manifest.Digest = assemblyManifestDigest(manifest)
	if probe || strings.HasPrefix(requestID, "preview/") {
		manifest.Purpose = "PREVIEW"
		if probe {
			manifest.Purpose = "PROBE"
		}
		manifest.Digest = assemblyManifestDigest(manifest)
	}
	finishPrepare()
	preparing = false
	// Read-only M4/M5 compilation captures exact values before persistence,
	// numerical work, admission or production warm-start promotion.
	if frozen, ok := ctx.Value(assemblyFrozenInputKey{}).(*AssemblySolveManifest); ok {
		*frozen = manifest
		return nil
	}
	if err := service.persistAssemblySolveManifest(ctx, manifest); err != nil {
		return err
	}
	if diagnostic != nil {
		diagnostic.AssemblyManifestDigest = manifest.Digest
	}
	var result geometry.AssemblySolve
	if existing, lookupErr := service.GetAssemblySolveResult(ctx, documentID, requestID); lookupErr == nil {
		if existing.ManifestDigest != manifest.Digest {
			return fmt.Errorf("%w: request id was already used for another SolveManifest", ErrValidation)
		}
		result = existing.Result
		if existing.Status == "FAILED" {
			err = fmt.Errorf("%s", existing.Diagnostic)
		}
	} else if !errors.Is(lookupErr, ErrNotFound) {
		return lookupErr
	} else {
		work, stop := assemblyNumericalContext(ctx)
		result, err = service.solveFrozenManifest(work, requestID, manifest, func(data []byte, e error) {
			if diagnostic != nil && e == nil {
				diagnostic.AssemblyReplay = append(json.RawMessage(nil), data...)
			}
			service.captureAssemblyReplay(ctx, documentID, requestID)(data, e)
		})
		stop()
		// Never cache a canceled/invalid/unauthorized RPC as a retryable solve
		// failure: replaying an untyped error string must not change its policy.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		switch status.Code(err) {
		case codes.Canceled, codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition:
			return err
		}
		if recordErr := service.recordAssemblySolveResult(ctx, manifest, requestID, result, err); recordErr != nil {
			return recordErr
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
			return err
		}
		switch status.Code(err) {
		case codes.InvalidArgument, codes.PermissionDenied, codes.Unauthenticated, codes.FailedPrecondition:
			return err
		}
		return workflow.failure(context.Background(), "NUMERICAL_FAILURE",
			"ASSEMBLY_SOLVER_UNAVAILABLE", err.Error(), true)
	}
	if result.Status != "CONVERGED" {
		code, retryable := "ASSEMBLY_SOLVER_UNSATISFIED", false
		switch result.Status {
		case "MAX_ITERATIONS":
			code = "ASSEMBLY_SOLVER_NON_CONVERGENT"
			retryable = true
		case "INCONSISTENT":
			code = "ASSEMBLY_SOLVER_INCONSISTENT"
		case "INVALID_MODEL":
			code = "ASSEMBLY_SOLVER_INVALID_MODEL"
		case "NUMERICAL_FAILURE":
			code, retryable = "ASSEMBLY_SOLVER_NUMERICAL_FAILURE", true
		}
		return workflow.failure(context.Background(), result.Status, code, result.Diagnostic, retryable)
	}
	if err := validateAssemblyPreference(result); err != nil {
		return workflow.failure(context.Background(), "PREFERENCE_NOT_CONVERGED", "ASSEMBLY_PREFERENCE_NOT_CONVERGED", err.Error(), false)
	}
	promoteAssemblyGroupEvidence(model, result)
	for _, target := range evidence {
		if target != nil {
			*target = result
		}
	}
	if err := workflow.advance(context.Background()); err != nil {
		return err
	}
	for _, solved := range result.Bodies {
		if instance := instances[solved.ID]; instance != nil {
			instance.Translation, instance.Rotation = solved.Pose.Translation, solved.Pose.Rotation
		}
	}
	// Transport the sector along the accepted pose, preserving its positive sense.
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if c.Suppressed || c.Second == nil || c.Mode == "MEASURED" {
			continue
		}
		for _, compiled := range constraints {
			if compiled.ID != c.ID || compiled.Kind != "ANGLE" || compiled.AngleReferenceDirection != nil || compiled.AngleReferenceGeometryID != "" {
				continue
			}
			sense := 1.0
			if c.Value > math.Pi {
				sense = -1
			}
			if next := spatialAngleBranchDirection(resolvedGeometry[compiled.FirstGeometryID].Direction, resolvedGeometry[compiled.SecondGeometryID].Direction,
				instances[c.First.InstanceID], instances[c.Second.InstanceID], sense); next != nil {
				c.SpatialAngleBranchDirection = next
			}
		}
	}
	for index := range model.Constraints {
		if !excluded[model.Constraints[index].ID] && !model.Constraints[index].Suppressed && model.Constraints[index].EvaluationStatus != modelcore.AssemblyConstraintBroken && model.Constraints[index].EvaluationStatus != modelcore.AssemblyConstraintImpossible {
			model.Constraints[index].EvaluationStatus = modelcore.AssemblyConstraintVerified
			model.Constraints[index].EvaluationFailure = nil
			model.Constraints[index].EvaluationSummary = "resolved supports satisfy the accepted assembly solution"
		}
	}
	applyAssemblyMeasurements(model, result, manifest.SolverProfile, excluded)
	service.assemblyWarmStarts.put(warmStartKey, *model)
	if err := workflow.advance(context.Background()); err != nil {
		return err
	}
	return nil
}

// A feasible pose is insufficient for a command promising minimum reference motion.
func validateAssemblyPreference(result geometry.AssemblySolve) error {
	if len(result.Components) == 0 {
		return fmt.Errorf("assembly solver returned no motion evidence")
	}
	for _, component := range result.Components {
		if component.Solved && (component.Preference.Status != geometry.PreferenceConverged || !component.Preference.GeometricallyFeasible) {
			return fmt.Errorf("component %s is geometrically feasible but motion preference did not converge", component.ComponentID)
		}
	}
	return nil
}

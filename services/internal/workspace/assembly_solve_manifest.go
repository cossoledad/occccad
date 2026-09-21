package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	assemblySolveManifestSchema = 1
	assemblySolverBuildPolicy   = "assembly-m3-lifecycle-v3"
	maxManifestBodies           = 4096
	maxManifestGeometry         = 16384
	maxManifestConstraints      = 16384
)

// AssemblySolveManifest is the immutable, Worker-ready M3 input. The solver
// receives only these frozen descriptors and never resolves Product, database,
// Publication, PersistentSelection or B-Rep state itself.
type AssemblySolveManifest struct {
	Definitions           []AssemblyConstraint           `json:"definitions,omitempty"`
	SchemaVersion         int                            `json:"schemaVersion"`
	Digest                string                         `json:"digest"`
	RootProductDocumentID string                         `json:"rootProductDocumentId"`
	RootProductRevisionID string                         `json:"rootProductRevisionId"`
	ModelHash             string                         `json:"modelHash"`
	Purpose               string                         `json:"purpose"`
	Bodies                []geometry.AssemblyBody        `json:"bodies"`
	Geometry              []geometry.AssemblyGeometry    `json:"geometry"`
	Constraints           []geometry.AssemblyConstraint  `json:"constraints"`
	Intent                *geometry.AssemblySolveIntent  `json:"intent,omitempty"`
	AffectedBodyIDs       []string                       `json:"affectedBodyIds,omitempty"`
	SolverProfile         geometry.AssemblySolverProfile `json:"solverProfile"`
	SolverBuildPolicy     string                         `json:"solverBuildPolicy"`
	ResolutionEvidence    []AssemblyResolutionEvidence   `json:"resolutionEvidence,omitempty"`
}

type AssemblyResolutionEvidence struct {
	ConstraintID     string                 `json:"constraintId"`
	Endpoint         string                 `json:"endpoint"`
	InstanceID       string                 `json:"instanceId"`
	PublicationRef   *PublicationRef        `json:"publicationRef,omitempty"`
	Publication      *PublicationResolution `json:"publicationResolution,omitempty"`
	Persistent       *ResolutionSnapshot    `json:"persistentSelectionResolution,omitempty"`
	SourceVersionID  string                 `json:"sourceVersionId,omitempty"`
	DescriptorDigest string                 `json:"descriptorDigest"`
}

type AssemblySolveManifestResult struct {
	ManifestDigest string                 `json:"manifestDigest"`
	RequestID      string                 `json:"requestId"`
	ResultDigest   string                 `json:"resultDigest,omitempty"`
	Status         string                 `json:"status"`
	Diagnostic     string                 `json:"diagnostic,omitempty"`
	Result         geometry.AssemblySolve `json:"result"`
}

func defaultAssemblySolverProfile() geometry.AssemblySolverProfile {
	preferenceIterations := uint64(100)
	return geometry.AssemblySolverProfile{SchemaVersion: 2, MaxIterations: 100,
		LengthTolerance: 1e-7, AngleTolerance: 1e-8,
		ClassificationLengthTolerance: 1e-7, ClassificationAngleTolerance: 1e-8,
		TranslationStepTolerance: 1e-9, RotationStepTolerance: 1e-10, DegeneracyTolerance: 1e-8,
		TranslationFiniteDifferenceStep: 1e-6, RotationFiniteDifferenceStep: 1e-7,
		InitialDamping: 1e-4, RankAbsoluteTolerance: 1e-10, RankRelativeTolerance: 1e-8,
		GradientTolerance: 1e-8, MotionLengthScale: 1, MotionAngleScale: 1,
		PreferenceTolerance: 1e-8, ObjectiveTolerance: 1e-12, MaxPreferenceIterations: &preferenceIterations,
		MaxConflictProbes: 16, JacobianCheckTolerance: 1e-5}
}

func newAssemblySolveManifest(documentID, revisionID, modelHash string, bodies []geometry.AssemblyBody,
	geometryValues []geometry.AssemblyGeometry, constraints []geometry.AssemblyConstraint, intent *geometry.AssemblySolveIntent,
	affected []string, evidence []AssemblyResolutionEvidence, definitions ...[]AssemblyConstraint) (AssemblySolveManifest, error) {
	bodies = append([]geometry.AssemblyBody(nil), bodies...)
	geometryValues = append([]geometry.AssemblyGeometry(nil), geometryValues...)
	constraints = append([]geometry.AssemblyConstraint(nil), constraints...)
	evidence = append([]AssemblyResolutionEvidence(nil), evidence...)
	sort.Slice(bodies, func(i, j int) bool { return bodies[i].ID < bodies[j].ID })
	sort.Slice(geometryValues, func(i, j int) bool { return geometryValues[i].ID < geometryValues[j].ID })
	sort.Slice(constraints, func(i, j int) bool { return constraints[i].ID < constraints[j].ID })
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].ConstraintID == evidence[j].ConstraintID {
			return evidence[i].Endpoint < evidence[j].Endpoint
		}
		return evidence[i].ConstraintID < evidence[j].ConstraintID
	})
	if len(affected) == 0 {
		for _, body := range bodies {
			affected = append(affected, body.ID)
		}
	} else {
		affected = append([]string(nil), affected...)
		sort.Strings(affected)
	}
	if intent != nil {
		copy := *intent
		copy.MovingBodyIDs = append([]string(nil), intent.MovingBodyIDs...)
		copy.ReferenceBodyIDs = append([]string(nil), intent.ReferenceBodyIDs...)
		sort.Strings(copy.MovingBodyIDs)
		sort.Strings(copy.ReferenceBodyIDs)
		intent = &copy
	}
	manifest := AssemblySolveManifest{SchemaVersion: assemblySolveManifestSchema, RootProductDocumentID: documentID,
		RootProductRevisionID: revisionID, ModelHash: modelHash, Purpose: "COMMIT", Bodies: bodies, Geometry: geometryValues,
		Constraints: constraints, Intent: intent, AffectedBodyIDs: affected, SolverProfile: defaultAssemblySolverProfile(),
		SolverBuildPolicy: assemblySolverBuildPolicy, ResolutionEvidence: evidence}
	if len(definitions) > 0 {
		manifest.Definitions = append([]AssemblyConstraint(nil), definitions[0]...)
		sort.Slice(manifest.Definitions, func(i, j int) bool { return manifest.Definitions[i].ID < manifest.Definitions[j].ID })
	}
	if err := validateAssemblySolveManifest(manifest); err != nil {
		return AssemblySolveManifest{}, err
	}
	// Freeze the same value representation that persistence/replay consumes.
	// Slice copies above protect canonical ordering, but nested poses, branch
	// state and resolution evidence still contain caller-owned pointers/slices.
	raw, err := json.Marshal(manifest)
	if err != nil {
		return AssemblySolveManifest{}, fmt.Errorf("%w: encode assembly SolveManifest: %v", ErrValidation, err)
	}
	var frozen AssemblySolveManifest
	if err := json.Unmarshal(raw, &frozen); err != nil {
		return AssemblySolveManifest{}, fmt.Errorf("%w: freeze assembly SolveManifest: %v", ErrValidation, err)
	}
	frozen.Digest = resolvedDigest(frozen)
	return frozen, nil
}

func validateAssemblySolveManifest(manifest AssemblySolveManifest) error {
	if manifest.SchemaVersion != assemblySolveManifestSchema || manifest.RootProductDocumentID == "" ||
		manifest.RootProductRevisionID == "" || manifest.ModelHash == "" ||
		(manifest.Purpose != "COMMIT" && manifest.Purpose != "PREVIEW") {
		return fmt.Errorf("%w: incomplete assembly SolveManifest identity", ErrValidation)
	}
	if len(manifest.Bodies) == 0 || len(manifest.Bodies) > maxManifestBodies || len(manifest.Geometry) > maxManifestGeometry ||
		len(manifest.Definitions) > maxManifestConstraints || len(manifest.Constraints) > maxManifestConstraints {
		return fmt.Errorf("%w: assembly SolveManifest resource limits exceeded", ErrValidation)
	}
	if manifest.SolverProfile.SchemaVersion != 2 || manifest.SolverBuildPolicy != assemblySolverBuildPolicy {
		return fmt.Errorf("%w: unsupported assembly solver profile or build policy", ErrValidation)
	}
	bodies, geometryIDs, constraints := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, body := range manifest.Bodies {
		if body.ID == "" || bodies[body.ID] {
			return fmt.Errorf("%w: duplicate or empty body identity in SolveManifest", ErrValidation)
		}
		bodies[body.ID] = true
	}
	for _, item := range manifest.Geometry {
		if item.ID == "" || geometryIDs[item.ID] || !bodies[item.BodyID] {
			return fmt.Errorf("%w: invalid geometry identity or body reference in SolveManifest", ErrValidation)
		}
		geometryIDs[item.ID] = true
	}
	for _, constraint := range manifest.Constraints {
		if constraint.ID == "" || constraints[constraint.ID] || !bodies[constraint.FirstBodyID] ||
			(constraint.SecondBodyID != "" && !bodies[constraint.SecondBodyID]) ||
			(constraint.FirstGeometryID != "" && !geometryIDs[constraint.FirstGeometryID]) ||
			(constraint.SecondGeometryID != "" && !geometryIDs[constraint.SecondGeometryID]) {
			return fmt.Errorf("%w: invalid constraint reference in SolveManifest", ErrValidation)
		}
		constraints[constraint.ID] = true
	}
	for _, evidence := range manifest.ResolutionEvidence {
		if evidence.ConstraintID == "" || !constraints[evidence.ConstraintID] || evidence.DescriptorDigest == "" ||
			(evidence.Publication != nil && evidence.Publication.Status != "CONNECTED") ||
			(evidence.Persistent != nil && evidence.Persistent.Result.Status != modelcore.SelectionResolved) {
			return fmt.Errorf("%w: unresolved or incomplete resolution evidence in SolveManifest", ErrValidation)
		}
	}
	return nil
}

func (service *Service) persistAssemblySolveManifest(ctx context.Context, manifest AssemblySolveManifest) error {
	stored := manifest
	stored.Digest = ""
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	_, err = service.database.Exec(ctx, `INSERT INTO occccad.product_solve_manifests(
		digest,schema_version,root_product_document_id,root_product_revision_id,manifest,solver_build_policy)
		VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (digest) DO NOTHING`, manifest.Digest, manifest.SchemaVersion,
		manifest.RootProductDocumentID, manifest.RootProductRevisionID, raw, manifest.SolverBuildPolicy)
	return err
}

func (service *Service) recordAssemblySolveResult(ctx context.Context, manifest AssemblySolveManifest, requestID string,
	result geometry.AssemblySolve, solveErr error) error {
	status, diagnostic, solverBuild := result.Status, result.Diagnostic, result.SolverBuild
	if solveErr != nil {
		status, diagnostic = "FAILED", solveErr.Error()
	}
	resultRaw, _ := json.Marshal(result)
	resultDigest := resolvedDigest(result)
	_, err := service.database.Exec(ctx, `INSERT INTO occccad.product_solve_results(
		manifest_digest,request_id,result,result_digest,status,diagnostic,solver_build,completed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,now()) ON CONFLICT (request_id) DO NOTHING`, manifest.Digest, requestID,
		resultRaw, resultDigest, status, diagnostic, solverBuild)
	return err
}

func (service *Service) solveFrozenManifest(ctx context.Context, requestID string, manifest AssemblySolveManifest,
	capture func([]byte, error)) (geometry.AssemblySolve, error) {
	if err := validateAssemblySolveManifest(manifest); err != nil {
		return geometry.AssemblySolve{}, err
	}
	return service.worker.SolveAssemblyWithOptions(ctx, requestID, manifest.Bodies, manifest.Geometry, manifest.Constraints,
		geometry.AssemblySolveOptions{Intent: manifest.Intent, AffectedBodyIDs: manifest.AffectedBodyIDs,
			SolverProfile: &manifest.SolverProfile, CaptureReplay: capture})
}

func (service *Service) GetAssemblySolveResult(ctx context.Context, documentID, requestID string) (AssemblySolveManifestResult, error) {
	var value AssemblySolveManifestResult
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT r.manifest_digest,r.request_id,r.result_digest,r.status,
		COALESCE(r.diagnostic,''),r.result FROM occccad.product_solve_results r
		JOIN occccad.product_solve_manifests m ON m.digest=r.manifest_digest
		WHERE m.root_product_document_id=$1 AND r.request_id=$2`, documentID, requestID).
		Scan(&value.ManifestDigest, &value.RequestID, &value.ResultDigest, &value.Status, &value.Diagnostic, &raw); errors.Is(err, pgx.ErrNoRows) {
		return AssemblySolveManifestResult{}, ErrNotFound
	} else if err != nil {
		return AssemblySolveManifestResult{}, err
	}
	if err := json.Unmarshal(raw, &value.Result); err != nil {
		return AssemblySolveManifestResult{}, err
	}
	return value, nil
}

func (service *Service) ReplayAssemblySolveManifest(ctx context.Context, documentID, digest, requestID string) (AssemblySolveManifestResult, error) {
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT manifest FROM occccad.product_solve_manifests
		WHERE digest=$1 AND root_product_document_id=$2`, digest, documentID).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return AssemblySolveManifestResult{}, ErrNotFound
	} else if err != nil {
		return AssemblySolveManifestResult{}, err
	}
	var manifest AssemblySolveManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return AssemblySolveManifestResult{}, err
	}
	manifest.Digest = digest
	if resolvedDigest(func() AssemblySolveManifest { value := manifest; value.Digest = ""; return value }()) != digest {
		return AssemblySolveManifestResult{}, fmt.Errorf("%w: SolveManifest digest mismatch", ErrValidation)
	}
	requestID = requestID + "/solve-manifest-replay"
	if existing, lookupErr := service.GetAssemblySolveResult(ctx, documentID, requestID); lookupErr == nil {
		if existing.ManifestDigest != digest {
			return AssemblySolveManifestResult{}, fmt.Errorf("%w: request id was already used for another SolveManifest", ErrValidation)
		}
		return existing, nil
	} else if !errors.Is(lookupErr, ErrNotFound) {
		return AssemblySolveManifestResult{}, lookupErr
	}
	result, solveErr := service.solveFrozenManifest(ctx, requestID, manifest, service.captureAssemblyReplay(ctx, documentID, requestID))
	if recordErr := service.recordAssemblySolveResult(ctx, manifest, requestID, result, solveErr); recordErr != nil && solveErr == nil {
		return AssemblySolveManifestResult{}, recordErr
	}
	if solveErr != nil {
		return AssemblySolveManifestResult{}, solveErr
	}
	return AssemblySolveManifestResult{ManifestDigest: digest, RequestID: requestID,
		ResultDigest: resolvedDigest(result), Status: result.Status, Diagnostic: result.Diagnostic, Result: result}, nil
}

// A promoted preview reuses immutable solve evidence but binds it to the new
// committed revision. It must not leave Release dependent on a PREVIEW record.
func (service *Service) promoteAssemblySolveManifest(ctx context.Context, documentID, revisionID, requestID, previewRequestID, modelHash string) error {
	result, err := service.GetAssemblySolveResult(ctx, documentID, previewRequestID)
	if err != nil {
		return err
	}
	if result.Status != "CONVERGED" {
		return fmt.Errorf("%w: only converged assembly evidence can be promoted", ErrValidation)
	}
	var raw []byte
	if err = service.database.QueryRow(ctx, `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1 AND root_product_document_id=$2`, result.ManifestDigest, documentID).Scan(&raw); err != nil {
		return err
	}
	var manifest AssemblySolveManifest
	if err = json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	manifest.RootProductRevisionID, manifest.ModelHash, manifest.Purpose = revisionID, modelHash, "COMMIT"
	manifest.Digest = ""
	manifest.Digest = resolvedDigest(manifest)
	if err = service.persistAssemblySolveManifest(ctx, manifest); err != nil {
		return err
	}
	return service.recordAssemblySolveResult(ctx, manifest, requestID, result.Result, nil)
}

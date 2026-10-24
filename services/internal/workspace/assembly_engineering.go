package workspace

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
)

type AssemblyEngineeringComponent struct {
	JacobianRank   uint64                         `json:"jacobianRank"`
	DiagnosticCode string                         `json:"diagnosticCode,omitempty"`
	BodyIDs        []string                       `json:"bodyIds"`
	RelativeDof    uint64                         `json:"relativeDof"`
	GaugeDof       uint64                         `json:"gaugeDof"`
	Solved         bool                           `json:"solved"`
	Freedoms       []geometry.AssemblyBodyFreedom `json:"freedoms"`
}
type AssemblyEngineeringEvidence struct {
	Source             string                         `json:"source"`
	Status             string                         `json:"status"`
	InputDigest        string                         `json:"inputDigest,omitempty"`
	SolverBuildPolicy  string                         `json:"solverBuildPolicy,omitempty"`
	SolverBuild        string                         `json:"solverBuild,omitempty"`
	ScopeBodyIDs       []string                       `json:"scopeBodyIds"`
	ConstraintIDs      []string                       `json:"constraintIds"`
	Excluded           []AssemblyEngineeringExclusion `json:"excluded"`
	References         []AssemblyEngineeringReference `json:"references"`
	DiagnosticCode     string                         `json:"diagnosticCode,omitempty"`
	Diagnostic         string                         `json:"diagnostic,omitempty"`
	DocumentID         string                         `json:"documentId"`
	RevisionID         string                         `json:"revisionId"`
	Available          bool                           `json:"available"`
	Components         []AssemblyEngineeringComponent `json:"components"`
	ReferenceSemantics string                         `json:"referenceSemantics"`
	CoordinateFrame    string                         `json:"coordinateFrame"`
	LengthUnit         string                         `json:"lengthUnit"`
}

// Read the exact Revision's independent analysis cache or command result.
// Never run diagnostics, re-solve, or reuse the most recent result of a different
// revision. A metadata-only command without matching evidence stays unknown.
func (service *Service) GetAssemblyEngineeringEvidence(ctx context.Context, documentID, revisionID string) (AssemblyEngineeringEvidence, error) {
	evidence := AssemblyEngineeringEvidence{DocumentID: documentID, RevisionID: revisionID, Source: "COMMAND", Status: "NOT_COMPUTED", Components: []AssemblyEngineeringComponent{}}
	var requestID string
	var raw []byte
	err := service.database.QueryRow(ctx, `SELECT c.request_id FROM occccad.document_versions v
 JOIN occccad.commands c ON c.id=v.created_by_command_id WHERE v.document_id=$1 AND v.id=$2`, documentID, revisionID).Scan(&requestID)
	if errors.Is(err, database.ErrNoRows) {
		return evidence, ErrNotFound
	}
	if err != nil {
		return evidence, err
	}
	if cached, ok := service.assemblyEngineering.get(engineeringCacheKey(documentID, revisionID)); ok {
		return cached, nil
	}
	if err = service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, documentID, revisionID).Scan(&raw); err != nil {
		return evidence, err
	}
	var model ProductModel
	if err = json.Unmarshal(raw, &model); err != nil {
		return evidence, err
	}
	engineeringScope(model, &evidence)
	result, err := service.GetAssemblySolveResult(ctx, documentID, requestID)
	if errors.Is(err, ErrNotFound) {
		return evidence, nil
	}
	if err != nil {
		return evidence, err
	}
	if result.Status != "CONVERGED" {
		evidence.Status = "UNKNOWN"
		evidence.DiagnosticCode = "COMMAND_SOLVE_INCOMPLETE"
		evidence.Diagnostic = result.Status + ": " + result.Diagnostic
		return evidence, nil
	}
	// The exact result's manifest distinguishes physical-world drag freedom
	// from static freedom relative to the numerical gauge anchor. Never infer
	// this from an empty reference or borrow another Revision's manifest.
	var interactive bool
	if err = service.database.QueryRow(ctx, `SELECT COALESCE(manifest->'dragTarget', 'null'::jsonb) <> 'null'::jsonb FROM occccad.product_solve_manifests WHERE digest=$1 AND root_product_document_id=$2`, result.ManifestDigest, documentID).Scan(&interactive); err != nil {
		return evidence, err
	}
	evidence.ReferenceSemantics = "STATIC_RELATIVE"
	if interactive {
		evidence.ReferenceSemantics = "INTERACTION_PHYSICAL"
	}
	evidence.CoordinateFrame = "OWNING_PRODUCT"
	evidence.LengthUnit = "mm"
	var manifestRaw []byte
	if err = service.database.QueryRow(ctx, `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1 AND root_product_document_id=$2`, result.ManifestDigest, documentID).Scan(&manifestRaw); err != nil {
		return evidence, err
	}
	var frozen AssemblySolveManifest
	if err = json.Unmarshal(manifestRaw, &frozen); err != nil {
		return evidence, err
	}
	evidence.InputDigest = result.ManifestDigest
	evidence.SolverBuildPolicy = frozen.SolverBuildPolicy
	if frozen.SolverBuildPolicy != assemblySolverBuildPolicy || !engineeringCompiledScope(frozen, evidence.ConstraintIDs) {
		evidence.Status = "UNKNOWN"
		evidence.DiagnosticCode = "ANALYSIS_ACCEPTED_SCOPE_INCOMPLETE"
		return evidence, nil
	}
	engineeringResult(&evidence, engineeringBodies(model), result.Result, frozen.SolverProfile)
	return evidence, nil
}

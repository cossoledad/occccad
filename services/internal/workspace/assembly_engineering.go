package workspace

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/geometry"
)

type AssemblyEngineeringComponent struct {
	BodyIDs     []string                       `json:"bodyIds"`
	RelativeDof uint64                         `json:"relativeDof"`
	GaugeDof    uint64                         `json:"gaugeDof"`
	Solved      bool                           `json:"solved"`
	Freedoms    []geometry.AssemblyBodyFreedom `json:"freedoms"`
}
type AssemblyEngineeringEvidence struct {
	DocumentID string                         `json:"documentId"`
	RevisionID string                         `json:"revisionId"`
	Available  bool                           `json:"available"`
	Components []AssemblyEngineeringComponent `json:"components"`
}

// Read the existing result attached to the exact immutable Revision's command.
// Never run diagnostics, re-solve, or reuse the most recent result of a different
// revision. A metadata-only command without matching evidence stays unknown.
func (service *Service) GetAssemblyEngineeringEvidence(ctx context.Context, documentID, revisionID string) (AssemblyEngineeringEvidence, error) {
	evidence := AssemblyEngineeringEvidence{DocumentID: documentID, RevisionID: revisionID, Components: []AssemblyEngineeringComponent{}}
	var requestID string
	err := service.database.QueryRow(ctx, `SELECT c.request_id FROM occccad.document_versions v
 JOIN occccad.commands c ON c.id=v.created_by_command_id WHERE v.document_id=$1 AND v.id=$2`, documentID, revisionID).Scan(&requestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return evidence, ErrNotFound
	}
	if err != nil {
		return evidence, err
	}
	result, err := service.GetAssemblySolveResult(ctx, documentID, requestID)
	if errors.Is(err, ErrNotFound) {
		return evidence, nil
	}
	if err != nil {
		return evidence, err
	}
	if result.Status != "CONVERGED" {
		return evidence, nil
	}
	evidence.Available = true
	for _, component := range result.Result.Components {
		evidence.Components = append(evidence.Components, AssemblyEngineeringComponent{BodyIDs: component.BodyIDs, RelativeDof: component.RelativeDof, GaugeDof: component.GaugeDof, Solved: component.Solved, Freedoms: component.Freedoms})
	}
	return evidence, nil
}

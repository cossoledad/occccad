package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

type FeatureRepairRequest struct {
	PartDependencyRequest
	FeatureID        string `json:"featureId"`
	CompareVersionID string `json:"compareVersionId,omitempty"`
	MaxRevisions     int    `json:"maxRevisions,omitempty"`
	ActorID          string `json:"-"`
}
type FeatureRepairSnapshot struct {
	VersionID             string                          `json:"versionId"`
	RootProductRevisionID string                          `json:"rootProductRevisionId,omitempty"`
	ContextVariantID      string                          `json:"contextVariantId,omitempty"`
	CreatedAt             string                          `json:"createdAt"`
	Feature               Feature                         `json:"feature"`
	Parameters            []modelcore.ParameterDefinition `json:"parameters"`
	Inputs                []PartDependencyInput           `json:"inputs"`
	Artifacts             []Artifact                      `json:"artifacts"`
	SketchFrame           *DatumPlane                     `json:"sketchFrame,omitempty"`
	EvaluatorVersion      string                          `json:"evaluatorVersion"`
	Reconstructed         bool                            `json:"reconstructed"`
}
type FeatureRepairContext struct {
	ContextReadScope
	DocumentID        string                          `json:"documentId"`
	BaseVersionID     string                          `json:"baseVersionId"`
	FeatureID         string                          `json:"featureId"`
	State             string                          `json:"state"`
	Examined          int                             `json:"examined"`
	SearchLimit       int                             `json:"searchLimit"`
	SearchDocumentID  string                          `json:"searchDocumentId"`
	DisplayID         string                          `json:"displayId,omitempty"`
	CurrentInputs     []PartDependencyInput           `json:"currentInputs"`
	CurrentParameters []modelcore.ParameterDefinition `json:"currentParameters"`
	Snapshot          *FeatureRepairSnapshot          `json:"snapshot,omitempty"`
	Downstream        []PartDependencyNode            `json:"downstream"`
}
type repairRevision struct {
	id      string
	raw     []byte
	created string
}

// This is an immutable read. Historical geometry is reconstructed using the
// current evaluator and issued as display-only evidence, never as a pick source.
func (s *Service) GetFeatureRepairContext(ctx context.Context, documentID string, r FeatureRepairRequest) (FeatureRepairContext, error) {
	limit := r.MaxRevisions
	if limit == 0 {
		limit = 64
	}
	if limit < 1 || limit > 256 {
		return FeatureRepairContext{}, fmt.Errorf("%w: history search limit must be in [1,256]", ErrValidation)
	}
	view, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return FeatureRepairContext{}, err
	}
	if view.Part == nil || view.Document.VersionID != r.VersionID {
		return FeatureRepairContext{}, fmt.Errorf("%w: repair snapshot changed", ErrValidation)
	}
	if r.RootProductDocumentID == "" && (r.RootProductRevisionID != "" || r.ContextVariantID != "" || r.InstancePath != nil) {
		return FeatureRepairContext{}, fmt.Errorf("%w: complete comparison scope required", ErrValidation)
	}
	if err = s.acceptedContextPart(ctx, documentID, &view, r.ContextReadScope); err != nil {
		return FeatureRepairContext{}, err
	}
	index := slices.IndexFunc(view.Part.Features, func(f Feature) bool { return f.ID == r.FeatureID })
	if index < 0 {
		return FeatureRepairContext{}, fmt.Errorf("%w: repair Feature missing", ErrValidation)
	}
	current := view.Part.Features[index]
	result := FeatureRepairContext{ContextReadScope: r.ContextReadScope, DocumentID: documentID, BaseVersionID: r.VersionID, FeatureID: r.FeatureID, State: "NO_HEALTHY_HISTORY", SearchLimit: limit, SearchDocumentID: documentID, Downstream: []PartDependencyNode{}}
	result.CurrentInputs = partFeatureDependencyInputs(view, current)
	result.CurrentParameters = []modelcore.ParameterDefinition{}
	for _, p := range view.Part.Parameters {
		if p.OwnerFeatureID == current.ID {
			result.CurrentParameters = append(result.CurrentParameters, p)
		}
	}
	graph, err := partDependencyProjection(view)
	if err != nil {
		return result, err
	}
	keys := map[modelcore.DependencyKey]bool{modelcore.DependencyKey("feature:" + r.FeatureID): true}
	for changed := true; changed; {
		changed = false
		for _, e := range graph.Edges {
			if keys[e.Source] && !keys[e.Target] {
				keys[e.Target] = true
				changed = true
			}
		}
	}
	for _, n := range graph.Nodes {
		if keys[n.Key] && n.EntityID != r.FeatureID && strings.HasPrefix(string(n.Key), "feature:") {
			result.Downstream = append(result.Downstream, n)
		}
	}
	sourceDocument, head := documentID, r.VersionID
	if r.RootProductDocumentID != "" {
		sourceDocument, head = r.RootProductDocumentID, r.RootProductRevisionID
		result.SearchDocumentID = sourceDocument
	}
	revisions := []repairRevision{}
	if r.CompareVersionID != "" {
		if _, e := uuid.Parse(r.CompareVersionID); e != nil {
			return result, fmt.Errorf("%w: invalid comparison revision", ErrValidation)
		}
		var row repairRevision
		err = s.database.QueryRow(ctx, `SELECT id::text,model_json,created_at::text FROM occccad.document_versions WHERE document_id=$1 AND id=$2 AND state='READY' AND sequence<=(SELECT sequence FROM occccad.document_versions WHERE document_id=$1 AND id=$3)`, sourceDocument, r.CompareVersionID, head).Scan(&row.id, &row.raw, &row.created)
		if errors.Is(err, database.ErrNoRows) {
			return result, fmt.Errorf("%w: comparison revision does not belong to snapshot owner", ErrValidation)
		}
		if err != nil {
			return result, err
		}
		revisions = append(revisions, row)
	} else {
		rows, e := s.database.Query(ctx, `WITH RECURSIVE ancestry AS (
 SELECT id,parent_version_id,model_json,created_at,state,0 AS depth FROM occccad.document_versions WHERE document_id=$1 AND id=$2
 UNION ALL SELECT v.id,v.parent_version_id,v.model_json,v.created_at,v.state,a.depth+1 FROM occccad.document_versions v JOIN ancestry a ON v.id=a.parent_version_id WHERE a.depth<$3 AND v.document_id=$1
 ) SELECT id::text,model_json,created_at::text FROM ancestry WHERE depth>0 AND state='READY' ORDER BY depth LIMIT $3`, sourceDocument, head, limit)
		if e != nil {
			return result, e
		}
		for rows.Next() {
			var row repairRevision
			if e = rows.Scan(&row.id, &row.raw, &row.created); e != nil {
				rows.Close()
				return result, e
			}
			revisions = append(revisions, row)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return result, e
		}
	}
	for _, revision := range revisions {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		result.Examined++
		var model PartModel
		partRevision, variant := revision.id, ""
		if r.RootProductDocumentID == "" {
			if err = json.Unmarshal(revision.raw, &model); err != nil {
				return result, err
			}
			normalizePartModel(&model)
		} else {
			var root ProductModel
			if err = json.Unmarshal(revision.raw, &root); err != nil {
				return result, err
			}
			items, e := s.expandProductContext(ctx, sourceDocument, revision.id, root)
			if e != nil {
				return result, e
			}
			owner, ok := occurrenceByCanonical(items, r.InstancePath.Canonical)
			if !ok || owner.DocumentID != documentID {
				continue
			}
			bindings := []ContextBinding{}
			for _, item := range items {
				for _, b := range item.ContextBindings {
					if b.OwningInstancePath.Canonical == r.InstancePath.Canonical {
						bindings = append(bindings, b)
					}
				}
			}
			v, e := s.contextVariantSnapshot(ctx, owner, bindings)
			if e != nil {
				return result, e
			}
			if v.Status != "READY" || v.Part == nil {
				continue
			}
			model = *v.Part
			partRevision, variant = v.BaseRevisionID, v.VariantKey
		}
		i := slices.IndexFunc(model.Features, func(f Feature) bool { return f.ID == r.FeatureID })
		if i < 0 {
			continue
		}
		feature := model.Features[i]
		if feature.Type != current.Type || feature.BodyID != current.BodyID || !healthyRepairFeature(feature) {
			continue
		}
		snapshot := FeatureRepairSnapshot{VersionID: partRevision, ContextVariantID: variant, CreatedAt: revision.created, Feature: feature, Parameters: []modelcore.ParameterDefinition{}, Inputs: partFeatureDependencyInputs(DocumentView{Part: &model}, feature), Artifacts: []Artifact{}, EvaluatorVersion: evaluatorVersion}
		if r.RootProductDocumentID != "" {
			snapshot.RootProductRevisionID = revision.id
		}
		for j := range snapshot.Inputs {
			snapshot.Inputs[j].State = "DEFINED"
		}
		for _, p := range model.Parameters {
			if p.OwnerFeatureID == feature.ID {
				snapshot.Parameters = append(snapshot.Parameters, p)
			}
		}
		prefix := model
		prefix.Features = append([]Feature(nil), model.Features[:i+1]...)
		if feature.Sketch != nil {
			o, x, n, ok := supportFrame(model, feature.Sketch.Support)
			if !ok {
				continue
			}
			snapshot.SketchFrame = &DatumPlane{Origin: o, UDirection: x, Normal: n}
			// The stored solved sketch is the historical definition. No current
			// support or topology is substituted into its display frame.
		} else if isBodyFeature(feature.Type) {
			key, e := s.evaluateBodyPrefix(ctx, "repair-comparison/"+revision.id+"/"+feature.ID, prefix, feature.BodyID, true)
			if e != nil {
				return result, fmt.Errorf("historical comparison rebuild: %w", e)
			}
			a, e := s.loadArtifact(ctx, key)
			if e != nil {
				return result, e
			}
			a.BodyID = feature.BodyID
			a.DisplayStageFeatureID = feature.ID
			a = fallbackVisualArtifact(a)
			a.Naming = NamingAvailability{Status: "DISPLAY_ONLY", CanBind: false, DiagnosticCode: "HISTORICAL_COMPARISON", Diagnostic: "历史对照仅供查看，请从当前输入重新选择。"}
			a.RepresentationKind = "HISTORICAL_COMPARISON"
			snapshot.Artifacts = append(snapshot.Artifacts, a)
			snapshot.Reconstructed = true
		}
		result.State = "AVAILABLE"
		result.Snapshot = &snapshot
		break
	}
	// Expensive reconstruction cannot publish a comparison against a new Head.
	var accepted string
	if err = s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, documentID).Scan(&accepted); err != nil {
		return result, err
	}
	if accepted != r.VersionID {
		return result, fmt.Errorf("%w: repair snapshot changed", ErrValidation)
	}
	if r.RootProductDocumentID != "" {
		if err = s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, r.RootProductDocumentID).Scan(&accepted); err != nil {
			return result, err
		}
		if accepted != r.RootProductRevisionID {
			return result, fmt.Errorf("%w: repair root snapshot changed", ErrValidation)
		}
	}
	if result.Snapshot != nil {
		result.DisplayID = newID("repair-display")
		objects := map[string]bool{}
		for _, a := range result.Snapshot.Artifacts {
			if id := a.Representations["VISUAL"].ObjectID; id != "" {
				objects[id] = true
			}
		}
		s.interactionCandidates.put(interactionCandidate{id: result.DisplayID, displayOnly: true, documentID: documentID, actorID: actorID(r.ActorID), headRevision: r.VersionID, visualObjectIDs: objects, expiresAt: time.Now().Add(interactionCandidateTTL)})
	}
	return result, nil
}

func healthyRepairFeature(f Feature) bool {
	if f.Suppressed || f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
		return false
	}
	if f.Sketch != nil {
		if len(sketchOffsetIssues(*f.Sketch)) > 0 {
			return false
		}
		if slices.Contains([]string{"FAILED_SUPPORT", "BLOCKED"}, f.Sketch.Support.Status) || f.Sketch.Solve.DefinitionStatus == "UNRESOLVED" || len(f.Sketch.Solve.ConflictingConstraintIDs) > 0 || slices.Contains([]string{"INVALID", "CONFLICTING", "EXECUTION_FAILURE", "UNRESOLVED_EXTERNAL", "FAILED"}, f.Sketch.Solve.Status) {
			return false
		}
		for _, e := range f.Sketch.ExternalGeometry {
			if slices.Contains([]string{"MISSING", "BROKEN", "UNAVAILABLE", "AMBIGUOUS"}, e.Status) {
				return false
			}
		}
	}
	return f.Sketch != nil || isBodyFeature(f.Type)
}

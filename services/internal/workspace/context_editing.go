package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/modelcore"
	"reflect"
)

func (s *Service) resolveVariantModel(ctx context.Context, documentID, key string, m *PartModel, bindings []ContextBinding) error {
	for _, b := range bindings {
		if err := materializeContextBindingVariant(m, b); err != nil {
			return err
		}
	}
	if err := validateAndResolvePartParameters(m); err != nil {
		return err
	}
	if err := s.resolveAndSolveSketches(ctx, documentID, "context-projection/"+key, m); err != nil {
		return err
	}
	return s.resolvePartPublications(ctx, documentID, "context-projection/"+key, key, m)
}

// Resolve the accepted owning Product, never the source's latest Head. The
// caller's exact scope is also checked at preview promotion and final CAS.
func (s *Service) contextEditBindings(ctx context.Context, documentID string, r CommandRequest) ([]ContextBinding, string, error) {
	if r.RootProductDocumentID == "" {
		return nil, "", nil
	}
	if r.InstancePath == nil || len(r.InstancePath.Segments) != 1 || r.InstancePath.RootDocumentID != r.RootProductDocumentID {
		return nil, "", fmt.Errorf("%w: direct Product occurrence is required", ErrValidation)
	}
	revision, items, err := s.productContextRoot(ctx, r.RootProductDocumentID)
	if err != nil {
		return nil, "", err
	}
	if revision != r.RootProductRevisionID {
		return nil, "", fmt.Errorf("%w: CONTEXT_PRODUCT_REVISION_CHANGED", ErrValidation)
	}
	owner, ok := occurrenceByCanonical(items, r.InstancePath.Canonical)
	if !ok || owner.DocumentID != documentID || owner.ReferenceMode == "PINNED" {
		return nil, "", fmt.Errorf("%w: context edit occurrence unavailable or fixed", ErrValidation)
	}
	if err := validateResolvedInstancePath(*r.InstancePath, owner.Path); err != nil {
		return nil, "", err
	}
	var bindings []ContextBinding
	for _, item := range items {
		for _, b := range item.ContextBindings {
			if b.OwningInstancePath.Canonical == owner.Path.Canonical {
				bindings = append(bindings, b)
			}
		}
	}
	key := ""
	if len(bindings) > 0 || len(owner.ContextInputs) > 0 {
		key = contextVariantKey(owner.RevisionID, contextVariantBindingDigest(bindings))
	}
	if r.ContextVariantID != key {
		return nil, "", fmt.Errorf("%w: CONTEXT_VARIANT_CHANGED", ErrValidation)
	}
	return bindings, revision, nil
}

// Only explicit field changes from the projected editor are applied to the
// base definition. Solved coordinates and bound values are not shared edits.
func retainBaseFeatureValues(base, projected PartModel, f *Feature) {
	if f == nil {
		return
	}
	var original, shown *Feature
	for i := range base.Features {
		if base.Features[i].ID == f.ID {
			original = &base.Features[i]
		}
	}
	for i := range projected.Features {
		if projected.Features[i].ID == f.ID {
			shown = &projected.Features[i]
		}
	}
	if original == nil || shown == nil {
		return
	}
	patch := func(dst *float64, visible, value float64) {
		if *dst == visible {
			*dst = value
		}
	}
	patch(&f.Length, shown.Length, original.Length)
	patch(&f.Length2, shown.Length2, original.Length2)
	patch(&f.Angle, shown.Angle, original.Angle)
	patch(&f.Diameter, shown.Diameter, original.Diameter)
	patch(&f.Depth, shown.Depth, original.Depth)
	patch(&f.RecessDiameter, shown.RecessDiameter, original.RecessDiameter)
	patch(&f.RecessDepth, shown.RecessDepth, original.RecessDepth)
	patch(&f.CountersinkAngle, shown.CountersinkAngle, original.CountersinkAngle)
	if f.Pattern != nil && shown.Pattern != nil && original.Pattern != nil {
		pattern := *f.Pattern
		f.Pattern = &pattern // the caller's editor draft remains immutable
		patch(&pattern.Spacing, shown.Pattern.Spacing, original.Pattern.Spacing)
		patch(&pattern.Angle, shown.Pattern.Angle, original.Pattern.Angle)
		patch(&pattern.Phase, shown.Pattern.Phase, original.Pattern.Phase)
		if pattern.Count == shown.Pattern.Count {
			pattern.Count = original.Pattern.Count
		}
		for i := range pattern.Origin {
			patch(&pattern.Origin[i], shown.Pattern.Origin[i], original.Pattern.Origin[i])
			patch(&pattern.Direction[i], shown.Pattern.Direction[i], original.Pattern.Direction[i])
		}
	}

}
func clonePartModel(m PartModel) PartModel {
	raw, _ := json.Marshal(m)
	var out PartModel
	_ = json.Unmarshal(raw, &out)
	return out
}

// Reuse the existing multi-workspace design transaction: the edited base and
// its owning Product reference advance together, while source inputs stay at
// their explicitly accepted revisions.
func (s *Service) commitContextPartEdit(ctx context.Context, documentID, revisionID string, p preparedDomainMutation, r CommandRequest, e documentEvaluationResult) error {
	if e.revisionState != "READY" {
		return fmt.Errorf("%w: independent Part defaults must remain valid", ErrValidation)
	}
	var root ProductModel
	var raw, priorRaw []byte
	var ws, head string
	var seq uint64
	if err := s.database.QueryRow(ctx, `SELECT w.id::text,w.head_revision_id::text,w.head_sequence,v.model_json,v.evaluation_manifest FROM occccad.workspaces w JOIN occccad.document_versions v ON v.id=w.head_revision_id WHERE w.document_id=$1 AND w.name='main'`, p.contextRootID).Scan(&ws, &head, &seq, &raw, &priorRaw); err != nil {
		return err
	}
	if head != p.contextRootRevision {
		return fmt.Errorf("%w: CONTEXT_PRODUCT_REVISION_CHANGED", ErrValidation)
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return err
	}
	for i := range root.Instances {
		f := &root.Instances[i]
		if f.ReferencedDocumentID == documentID && f.ReferenceMode != "PINNED" {
			f.ReferencedVersionID, f.ResolvedVersionID = revisionID, revisionID
		}
	}
	rootUUID, _ := uuid.NewV7()
	rootRevision := rootUUID.String()
	var base PartModel
	if err := json.Unmarshal(e.nextJSON, &base); err != nil {
		return err
	}
	for i := range root.ContextBindings {
		b := &root.ContextBindings[i]
		affected := false
		for _, instance := range root.Instances {
			if len(b.OwningInstancePath.Segments) == 1 && instance.ID == b.OwningInstancePath.Segments[0].InstanceID && instance.ReferencedVersionID == revisionID {
				affected = true
			}
		}
		if !affected {
			continue
		}
		if len(b.OwningInstancePath.Segments) == 1 && b.OwningInstancePath.Segments[0].ReferencedDocumentID == documentID {
			leaf := &b.OwningInstancePath.Segments[0]
			leaf.ResolvedVersionID = revisionID
			leaf.OwnerVersionID = rootRevision
			b.Accepted.OwningRevisionID = revisionID
		}
	}
	if err := s.validateContextPartOccurrences(ctx, documentID, base, root, r, p.command); err != nil {
		return err
	}
	for i := range root.Constraints {
		if root.Constraints[i].EvaluationStatus != modelcore.AssemblyConstraintBroken {
			root.Constraints[i].EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
		}
	}
	next, _ := json.Marshal(root)
	hash := canonicalModelHash(next)
	graph, manifest, err := buildProductEvaluation(root, rootRevision, hash, nil, nil)
	if err != nil {
		return err
	}
	changes, err := reconcilePersistedChanges("PRODUCT", raw, next, modelcore.ChangeSet{})
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(updateReferencesPayload{Model: root})
	command := modelcore.DomainCommand{CommandID: newID("command"), TypeURI: typeUpdateReferences, SchemaVersion: 1, Payload: payload}
	owner := atomicDomainCandidate{documentID: documentID, workspaceID: p.workspaceID, headRevision: p.headRevision, headSequence: p.headSequence, documentType: "PART", revisionID: revisionID, request: r, command: p.command, nextJSON: e.nextJSON, modelHash: e.modelHash, graph: e.graph, manifest: e.manifest, changes: e.changes}
	parent := atomicDomainCandidate{documentID: p.contextRootID, workspaceID: ws, headRevision: head, headSequence: seq, documentType: "PRODUCT", revisionID: rootRevision, request: CommandRequest{Type: "UPDATE_REFERENCES", RequestID: r.RequestID + "/owning-product", ActorID: p.actorID}, command: command, nextJSON: next, modelHash: hash, graph: graph, manifest: manifest, changes: changes}
	groupUUID, _ := uuid.NewV7()
	return s.commitProductDesignCandidates(ctx, groupUUID.String(), r.RequestID, p.requestDigest, p.actorID, p.contextRootID, []atomicDomainCandidate{owner, parent})
}

// Check every affected direct, following occurrence with its own accepted
// inputs. No source Head is consulted, and fixed occurrences keep their version.
func (s *Service) validateContextPartOccurrences(ctx context.Context, documentID string, base PartModel, root ProductModel, r CommandRequest, c modelcore.DomainCommand) error {
	for _, instance := range root.Instances {
		if instance.ReferencedDocumentID != documentID || instance.ReferenceMode == "PINNED" {
			continue
		}
		var bindings []ContextBinding
		for _, b := range root.ContextBindings {
			if len(b.OwningInstancePath.Segments) == 1 && b.OwningInstancePath.Segments[0].InstanceID == instance.ID {
				bindings = append(bindings, b)
			}
		}
		projection := clonePartModel(base)
		if err := s.resolveVariantModel(ctx, documentID, instance.ID, &projection, bindings); err != nil {
			return err
		}
		var err error
		if requiresOrderedEvaluation(c) {
			err = s.evaluateOrderedBodies(ctx, documentID, r.RequestID+"/context/"+instance.ID, &projection, orderMutationBlocker(c))
		} else {
			err = s.evaluatePartBodies(ctx, r.RequestID+"/context/"+instance.ID, &projection)
		}
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrValidation, instance.Name, err)
		}
	}
	return nil
}

// A historical context preview must be legal in the independent definition
// and all affected occurrences before it can become a confirmable candidate.
func (s *Service) validateContextInsertionPreview(ctx context.Context, documentID string, p preparedDomainMutation, r CommandRequest, raw json.RawMessage) error {
	if p.contextRootID == "" || !requiresOrderedEvaluation(p.command) {
		return nil
	}
	var base PartModel
	if err := json.Unmarshal(raw, &base); err != nil {
		return err
	}
	if err := resolveRuntimeParameters(ctx, &base); err != nil {
		return err
	}
	if err := s.resolveAndSolveSketches(ctx, documentID, r.RequestID+"/definition", &base); err != nil {
		return err
	}
	if err := resolveRuntimeParameters(ctx, &base); err != nil {
		return err
	}
	if err := s.resolvePartPublications(ctx, documentID, r.RequestID+"/definition", p.headRevision, &base); err != nil {
		return err
	}
	if err := s.evaluateOrderedBodies(ctx, documentID, r.RequestID+"/definition", &base, orderMutationBlocker(p.command)); err != nil {
		return err
	}
	root, err := s.GetDocument(ctx, p.contextRootID)
	if err != nil {
		return err
	}
	if root.Document.VersionID != p.contextRootRevision {
		return fmt.Errorf("%w: CONTEXT_PRODUCT_REVISION_CHANGED", ErrValidation)
	}
	if root.Product == nil {
		return fmt.Errorf("%w: Product required", ErrValidation)
	}
	return s.validateContextPartOccurrences(ctx, documentID, base, *root.Product, r, p.command)
}

// A read scope selects the exact accepted occurrence projection. It never
// resolves source Head and does not require edit permission or activate it.
type ContextReadScope struct {
	RootProductDocumentID string        `json:"rootProductDocumentId,omitempty"`
	RootProductRevisionID string        `json:"rootProductRevisionId,omitempty"`
	InstancePath          *InstancePath `json:"instancePath,omitempty"`
	ContextVariantID      string        `json:"contextVariantId,omitempty"`
}

func commandReadScope(r CommandRequest) ContextReadScope {
	return ContextReadScope{RootProductDocumentID: r.RootProductDocumentID, RootProductRevisionID: r.RootProductRevisionID, ContextVariantID: r.ContextVariantID, InstancePath: r.InstancePath}
}

func (s *Service) acceptedContextPart(ctx context.Context, documentID string, view *DocumentView, scope ContextReadScope) error {
	if scope.RootProductDocumentID == "" {
		return nil
	}
	if scope.InstancePath == nil || len(scope.InstancePath.Segments) != 1 || scope.InstancePath.RootDocumentID != scope.RootProductDocumentID {
		return fmt.Errorf("%w: direct Product occurrence is required", ErrValidation)
	}
	root, err := s.GetDocument(ctx, scope.RootProductDocumentID)
	if err != nil {
		return err
	}
	if root.Document.VersionID != scope.RootProductRevisionID {
		return fmt.Errorf("%w: CONTEXT_PRODUCT_REVISION_CHANGED", ErrValidation)
	}
	for _, variant := range root.ContextVariants {
		if variant.OwningInstancePath.Canonical != scope.InstancePath.Canonical || variant.VariantKey != scope.ContextVariantID || variant.BaseDocumentID != documentID || variant.BaseRevisionID != view.Document.VersionID {
			continue
		}
		if err := validateResolvedInstancePath(*scope.InstancePath, variant.OwningInstancePath); err != nil {
			return err
		}
		if variant.Status != "READY" || variant.Part == nil {
			return fmt.Errorf("%w: accepted context unavailable", ErrValidation)
		}
		view.Part = variant.Part
		view.SketchAnalyses = variant.SketchAnalyses
		return nil
	}
	return fmt.Errorf("%w: CONTEXT_VARIANT_CHANGED", ErrValidation)
}

func contextInputBound(model PartModel, bindings []ContextBinding, kind, target string) bool {
	for _, input := range model.ContextInputs {
		if input.Target.Kind == kind && input.Target.TargetID == target {
			for _, binding := range bindings {
				if binding.ContextInputID == input.ID {
					return true
				}
			}
		}
	}
	return false
}

// Check the canonical candidate, after every command adapter. This covers
// managed Feature parameters and sketch dimensions as well as direct edits.
// Accepted projection values are never used as writable defaults.
func validateContextInputWrites(beforeJSON, afterJSON json.RawMessage, bindings []ContextBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	var before, after PartModel
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		return err
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		return err
	}
	protected := map[string]string{}
	for _, input := range before.ContextInputs {
		if !contextInputBound(before, bindings, input.Target.Kind, input.Target.TargetID) {
			continue
		}
		for _, p := range before.Parameters {
			if input.Target.Kind == "PARAMETER" && p.ParameterID == input.Target.TargetID || input.Target.Kind == "DATUM" && p.OwnerFeatureID == input.Target.TargetID {
				protected[p.ParameterID] = input.Name
			}
		}
	}
	for id, name := range protected {
		var old, next *modelcore.ParameterDefinition
		for i := range before.Parameters {
			if before.Parameters[i].ParameterID == id {
				old = &before.Parameters[i]
			}
		}
		for i := range after.Parameters {
			if after.Parameters[i].ParameterID == id {
				next = &after.Parameters[i]
			}
		}
		if old == nil || next == nil || !contextSourceEqual(old.Source, next.Source) || old.Role != next.Role {
			return fmt.Errorf("%w: CONTEXT_INPUT_VALUE_READ_ONLY: %s", ErrValidation, name)
		}
	}
	return nil
}

// A definition dialog may rename or change references without editing its
// displayed, accepted dimension. Retain its independent source/value.
func retainBaseSketchValues(base, projected PartModel, sketchID string, operations []SketchOperation) []SketchOperation {
	out := append([]SketchOperation(nil), operations...)
	constraints := func(model PartModel) map[string]SketchConstraint {
		result := map[string]SketchConstraint{}
		for _, f := range model.Features {
			if f.ID == sketchID && f.Sketch != nil {
				for _, c := range f.Sketch.Constraints {
					result[c.ID] = c
				}
			}
		}
		return result
	}
	original, shown := constraints(base), constraints(projected)
	for i := range out {
		op := &out[i]
		if op.Type != "UPDATE_CONSTRAINT" || op.Constraint == nil || op.ParameterSource != nil {
			continue
		}
		old, a := original[op.ConstraintID]
		visible, b := shown[op.ConstraintID]
		if a && b && old.Value != nil && visible.Value != nil && op.Constraint.Value != nil && *op.Constraint.Value == *visible.Value {
			copy := *op.Constraint
			value := *old.Value
			copy.Value = &value
			op.Constraint = &copy
		}
	}
	return out
}

func contextSourceEqual(a, b modelcore.ValueSource) bool {
	// Alias renaming rewrites display text; the checked AST and stable reads
	// remain the authority for whether an associated default source changed.
	if a.Expression != nil {
		e := *a.Expression
		e.SourceText = ""
		a.Expression = &e
	}
	if b.Expression != nil {
		e := *b.Expression
		e.SourceText = ""
		b.Expression = &e
	}
	return reflect.DeepEqual(a, b)
}

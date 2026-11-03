package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

// Replacing one occurrence accepts only the explicitly chosen snapshot. It does
// not advance other references or solve poses. Public interfaces are reconnected
// against these two exact snapshots; private geometry is never shape-matched.
func (s *Service) prepareInstanceReplacement(ctx context.Context, documentID string, raw json.RawMessage, r CommandRequest) (string, any, error) {
	fail := func(code string) (string, any, error) { return "", nil, fmt.Errorf("%w: %s", ErrValidation, code) }
	if r.VersionID == "" || r.ReplacementVersionID == "" {
		return fail("REPLACEMENT_VERSION_REQUIRED")
	}
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return "", nil, err
	}
	index := slices.IndexFunc(model.Instances, func(i ProductInstance) bool { return i.ID == r.InstanceID })
	if index < 0 {
		return fail("REPLACEMENT_DIRECT_INSTANCE_REQUIRED")
	}
	var kind string
	if err := s.database.QueryRow(ctx, `SELECT d.document_type FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, r.ReferencedDocumentID, r.ReplacementVersionID).Scan(&kind); err != nil {
		return "", nil, err
	}
	if kind != "PART" && kind != "PRODUCT" {
		return fail("REPLACEMENT_TYPE_MISMATCH")
	}
	items, err := s.expandProductContext(ctx, r.ReferencedDocumentID, r.ReplacementVersionID)
	if err != nil {
		return "", nil, err
	}
	for _, item := range items {
		if item.DocumentID == documentID {
			return fail("REPLACEMENT_CYCLE")
		}
		if item.DocumentType == "PART" {
			if err = s.requireReadyReferencedPart(ctx, item.DocumentID, item.RevisionID); err != nil {
				return "", nil, err
			}
		}
	}
	old := model.Instances[index]
	model.Instances[index].ReferencedDocumentID = r.ReferencedDocumentID
	model.Instances[index].ReferencedVersionID = r.ReplacementVersionID
	model.Instances[index].ResolvedVersionID = r.ReplacementVersionID
	model.Instances[index].HeadChanged = false
	invalidateReplacedRelations(&model, documentID, r.VersionID, old, model.Instances[index])
	if old.ReferencedDocumentID != r.ReferencedDocumentID {
		ports, err := s.replacementPublicationMap(ctx, old, model.Instances[index])
		if err != nil {
			return "", nil, err
		}
		reconnectReplacedPublications(&model, old, model.Instances[index], ports)
	}
	return typeReplaceInstance, replaceInstancePayload{InstanceID: old.ID, ReferencedDocumentID: r.ReferencedDocumentID, ReferencedVersionID: r.ReplacementVersionID, Model: model}, nil
}

// Preserve typed references to the old source for explicit repair and Undo.
// Whole-occurrence relationships still identify the same occurrence; geometry
// relationships must never reconnect to similarly named elements in another Part.
func invalidateReplacedRelations(model *ProductModel, root, revision string, old, replacement ProductInstance) {
	uses := func(path InstancePath) bool { return len(path.Segments) > 0 && path.Segments[0].InstanceID == old.ID }
	changedDocument := old.ReferencedDocumentID != replacement.ReferencedDocumentID
	rebindOccurrence := func(path *InstancePath) *InstancePath {
		if path == nil || len(path.Segments) == 0 {
			return nil
		}
		copy := *path
		copy.Segments = slices.Clone(path.Segments)
		copy.Segments[0].ReferencedDocumentID = replacement.ReferencedDocumentID
		copy.Segments[0].ResolvedVersionID = replacement.ReferencedVersionID
		return &copy
	}

	for i := range model.Constraints {
		c := &model.Constraints[i]
		affected, broken := false, false
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref == nil || !(ref.InstanceID == old.ID || ref.InstancePath != nil && uses(*ref.InstancePath)) {
				continue
			}
			affected = true
			geometric := ref.Kind != "BODY" || ref.InstancePath != nil && len(ref.InstancePath.Segments) > 1 || ref.PublicationRef != nil || ref.CapturedFrame != nil

			if changedDocument && !geometric {
				ref.InstancePath = rebindOccurrence(ref.InstancePath)
				ref.SourceVersionID = replacement.ReferencedVersionID
				ref.Resolution = nil
			}
			if changedDocument && geometric {
				broken = true
				if ref.InstancePath == nil {
					p := appendInstancePath(InstancePath{RootDocumentID: root}, InstancePathSegment{OwnerDocumentID: root, OwnerVersionID: revision, InstanceID: old.ID, InstanceName: old.Name, ReferencedDocumentID: old.ReferencedDocumentID, ResolvedVersionID: old.ReferencedVersionID})
					ref.InstancePath = &p
				}
				ref.Resolution = &ResolutionSnapshot{PolicyDigest: modelcore.TopologyNamingPolicyDigest, Result: unavailableSelectionResolution("COMPONENT_REPLACED", "组件已替换，请重新选择支撑")}
			}
		}
		for j := range c.GroupMembers {
			m := &c.GroupMembers[j]
			if m.InstanceID == old.ID || m.InstancePath != nil && uses(*m.InstancePath) {
				affected = true
				if changedDocument && m.InstancePath != nil && len(m.InstancePath.Segments) > 1 {
					broken = true
				} else if changedDocument && m.InstancePath != nil {
					m.InstancePath = rebindOccurrence(m.InstancePath)
				}
			}
		}
		if affected {
			c.MeasuredValue = nil
			c.EvaluationFailure = nil
			c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			c.EvaluationSummary = "COMPONENT_REPLACED"
			if broken {
				c.EvaluationStatus = modelcore.AssemblyConstraintBroken
			}
		}
	}
	if !changedDocument {
		return
	}
	for i := range model.ContextBindings {
		b := &model.ContextBindings[i]
		if uses(b.OwningInstancePath) || uses(b.SourceInstancePath) {
			b.Accepted.Status = "BROKEN"
			b.Resolution.Status = "BROKEN"
			b.Resolution.DiagnosticCode = "COMPONENT_REPLACED"
			b.Resolution.Diagnostic = "组件已替换，请重新指定关联输入"
		}
	}
	for i := range model.Publications {
		p := &model.Publications[i]
		if uses(p.Target.InstancePath) {
			p.Resolution.Status = "BROKEN_PUBLICATION"
			p.Resolution.DiagnosticCode = "COMPONENT_REPLACED"
			p.Resolution.Diagnostic = "组件已替换，请重新指定公开接口"
		}
	}
}

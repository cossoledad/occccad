package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// Replacement is scoped to one accepted occurrence. In particular it never
// invokes UPDATE_REFERENCES, whose contract advances all following references.
func (s *Service) prepareInstanceReplacement(ctx context.Context, documentID string, raw json.RawMessage, r CommandRequest) (string, any, error) {
	fail := func(reason string) (string, any, error) { return "", nil, fmt.Errorf("%w: %s", ErrValidation, reason) }
	if r.VersionID == "" || r.ReplacementVersionID == "" {
		return fail("REPLACEMENT_VERSION_REQUIRED: 替换需要明确的装配和新组件版本，请重新选择组件")
	}
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return "", nil, err
	}
	i := slices.IndexFunc(model.Instances, func(v ProductInstance) bool { return v.ID == r.InstanceID })
	if i < 0 {
		return fail("REPLACEMENT_DIRECT_INSTANCE_REQUIRED: 只能替换当前装配的直接组件")
	}
	old := model.Instances[i]
	var oldKind, newKind string
	if err := s.database.QueryRow(ctx, `SELECT document_type FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, old.ReferencedDocumentID).Scan(&oldKind); err != nil {
		return "", nil, err
	}
	if err := s.database.QueryRow(ctx, `SELECT d.document_type FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, r.ReferencedDocumentID, r.ReplacementVersionID).Scan(&newKind); err != nil {
		return "", nil, err
	}
	if newKind != oldKind || (newKind != "PART" && newKind != "PRODUCT") {
		return fail("REPLACEMENT_TYPE_MISMATCH: 请选择同类型的替换组件")
	}
	if r.ReferencedDocumentID == documentID {
		return fail("REPLACEMENT_CYCLE: 替换会形成循环装配引用")
	}
	// Traverse the exact replacement snapshot, never a graph of newer Heads.
	items, err := s.expandProductContext(ctx, r.ReferencedDocumentID, r.ReplacementVersionID)
	if err != nil {
		return "", nil, err
	}
	checked := map[string]bool{}
	for _, item := range items {
		if item.DocumentID == documentID {
			return fail("REPLACEMENT_CYCLE: 替换会形成循环装配引用")
		}
		key := item.DocumentID + "@" + item.RevisionID
		if item.DocumentType == "PART" && !checked[key] {
			if err = s.requireReadyReferencedPart(ctx, item.DocumentID, item.RevisionID); err != nil {
				return "", nil, err
			}
			checked[key] = true
		}
	}

	receiver := false
	outgoing := false
	for _, b := range model.ContextBindings {
		if len(b.OwningInstancePath.Segments) > 0 && b.OwningInstancePath.Segments[0].InstanceID == old.ID {
			receiver = true
		}
		if len(b.SourceInstancePath.Segments) > 0 && b.SourceInstancePath.Segments[0].InstanceID == old.ID {
			outgoing = true
		}
	}
	if receiver && outgoing {
		return fail("REPLACEMENT_CHAIN_UNSUPPORTED")
	}
	if receiver && oldKind != "PART" {
		return fail("REPLACEMENT_RECEIVER_UNSUPPORTED")
	}
	receiverIDs := []string{}
	frozen := map[string]InstancePose{}
	if oldKind == "PART" {
		part, readErr := s.readAssemblyPartModel(ctx, r.ReferencedDocumentID, r.ReplacementVersionID)
		if readErr != nil {
			return "", nil, readErr
		}
		receiverIDs, frozen, err = s.receiverReplacementInputs(ctx, documentID, model, old, part.model, r.ContextInputMapping)
		if err != nil {
			return "", nil, err
		}
	} else if len(r.ContextInputMapping) > 0 {
		return fail("REPLACEMENT_IMPACT_CHANGED")
	}
	used := map[string]bool{}
	bindingIDs, constraintIDs, publicationIDs := []string{}, []string{}, []string{}
	uses := func(p InstancePath) bool { return len(p.Segments) > 0 && p.Segments[0].InstanceID == old.ID }
	direct := func(p InstancePath) bool {
		return uses(p) && len(p.Segments) == 1 && p.RootDocumentID == documentID && p.Segments[0].ReferencedDocumentID == old.ReferencedDocumentID && p.Segments[0].ResolvedVersionID == old.ReferencedVersionID
	}
	for _, b := range model.ContextBindings {
		if uses(b.OwningInstancePath) {
			bindingIDs = append(bindingIDs, b.ID)
			continue
		}
		if !uses(b.SourceInstancePath) {
			continue
		}
		if !direct(b.SourceInstancePath) {
			return fail("REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED: 请先将深层来源转发为组件公开接口")
		}
		if b.ReferenceMode == "PINNED" {
			return fail("REPLACEMENT_PINNED_INPUT: 组件提供了固定输入，请先明确处理该关联后再替换")
		}
		used[b.Publication.PublicationID] = true
		bindingIDs = append(bindingIDs, b.ID)
	}
	for _, p := range model.Publications {
		if uses(p.Target.InstancePath) {
			if !direct(p.Target.InstancePath) {
				return fail("请先将深层转发来源整理为组件公开接口")
			}
			used[p.Target.PublicationID] = true
			publicationIDs = append(publicationIDs, p.ID)
		}
	}
	for _, c := range model.Constraints {
		affected := false
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref == nil || !(ref.InstanceID == old.ID || ref.InstancePath != nil && uses(*ref.InstancePath)) {
				continue
			}
			affected = true
			if ref.InstancePath != nil && !direct(*ref.InstancePath) {
				return fail("REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED: 约束“" + c.Name + "”使用了深层来源，不能直接迁移")
			}
			if ref.PublicationRef == nil {
				if c.Kind == "FIX" && ref.Kind == "BODY" && ref.CapturedFrame == nil {
					continue
				}
				return fail("REPLACEMENT_RAW_SUPPORT_UNSUPPORTED: 约束“" + c.Name + "”未使用公开接口，请先重新指定支撑")
			}
			used[ref.PublicationRef.PublicationID] = true
		}
		for _, member := range c.GroupMembers {
			if member.InstanceID == old.ID || member.InstancePath != nil && uses(*member.InstancePath) {
				affected = true
			}
			if member.InstancePath != nil && uses(*member.InstancePath) && !direct(*member.InstancePath) {
				return fail("REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED: 固联组使用了组件内的深层成员，不能直接迁移")
			}
		}
		if affected {
			constraintIDs = append(constraintIDs, c.ID)
		}
	}
	for _, m := range model.Kinematics.Mechanisms {
		if slices.Contains(m.UnitIDs, old.ID) {
			return fail("REPLACEMENT_MECHANISM_UNSUPPORTED: 组件属于运动机构，本轮不迁移机构关系")
		}
	}
	publications := map[string]Publication{}
	for id := range used {
		replacementID := r.PublicationMapping[id]
		if replacementID == "" {
			return fail("REPLACEMENT_MAPPING_REQUIRED: 仍有已使用接口未指定对应接口")
		}
		before, e := s.publicationAtRevision(ctx, old.ReferencedDocumentID, old.ReferencedVersionID, id)
		if e != nil {
			return "", nil, e
		}
		after, e := s.publicationAtRevision(ctx, r.ReferencedDocumentID, r.ReplacementVersionID, replacementID)
		if e != nil {
			return "", nil, e
		}
		if before.CompatibilityVersion != after.CompatibilityVersion || !publicationContractsCompatible(before, after) || before.Contract.UnitPolicy != after.Contract.UnitPolicy {
			return fail("REPLACEMENT_CONTRACT_INCOMPATIBLE: 接口“" + before.Name + "”的类型、契约或量纲不兼容")
		}
		if after.Resolution.Status != "CONNECTED" {
			return fail("REPLACEMENT_PUBLICATION_UNAVAILABLE: 新接口“" + after.Name + "”尚不可用")
		}
		publications[id] = after
	}
	for id := range r.PublicationMapping {
		if !used[id] {
			return fail("REPLACEMENT_IMPACT_CHANGED: 接口影响范围已变化，请重新预览")
		}
	}
	model.Instances[i].ReferencedDocumentID = r.ReferencedDocumentID
	model.Instances[i].ReferencedVersionID = r.ReplacementVersionID
	model.Instances[i].ResolvedVersionID = r.ReplacementVersionID
	model.Instances[i].HeadChanged = false
	path := func(p InstancePath) InstancePath {
		p.Segments = append([]InstancePathSegment(nil), p.Segments...)
		p.Segments[0].ReferencedDocumentID = r.ReferencedDocumentID
		p.Segments[0].ResolvedVersionID = r.ReplacementVersionID
		return p
	}
	for i := range model.ContextBindings {
		b := &model.ContextBindings[i]
		if receiver && slices.Contains(receiverIDs, b.ID) {
			b.OwningInstancePath = path(b.OwningInstancePath)
			b.ContextInputID = r.ContextInputMapping[b.ContextInputID]
			b.Accepted.OwningRevisionID = r.ReplacementVersionID
			continue
		}
		if !uses(b.SourceInstancePath) {
			continue
		}
		p := publications[b.Publication.PublicationID]

		b.SourceInstancePath = path(b.SourceInstancePath)
		b.Publication = PublicationRef{PublicationID: p.ID, ExpectedType: p.Type, CompatibilityVersion: p.CompatibilityVersion, PersistentSelection: p.Target.PersistentSelection, SelectionSourceVersionID: p.Target.SourceVersionID}
		b.Resolution = p.Resolution
		b.Accepted.SourceRevisionID = r.ReplacementVersionID
		b.Accepted.SourceDigest = p.Resolution.SourceDigest
		b.Accepted.ContractDigest = resolvedDigest(p.Contract)
		b.Accepted.Status = "CONNECTED"
	}
	for i := range model.Publications {
		p := &model.Publications[i]
		if uses(p.Target.InstancePath) {
			p.Target.PublicationID = publications[p.Target.PublicationID].ID
			p.Target.InstancePath = path(p.Target.InstancePath)
		}
	}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref == nil || !(ref.InstanceID == old.ID || ref.InstancePath != nil && uses(*ref.InstancePath)) {
				continue
			}
			if ref.InstancePath != nil {
				p := path(*ref.InstancePath)
				ref.InstancePath = &p
			}
			if ref.PublicationRef != nil {
				p := publications[ref.PublicationRef.PublicationID]
				ref.PublicationRef = &PublicationRef{PublicationID: p.ID, ExpectedType: p.Type, CompatibilityVersion: p.CompatibilityVersion}
				ref.PersistentSelection = nil
				ref.Resolution = nil
				ref.PublicationResolution = nil
				ref.SourceVersionID = ""
			}
		}
		for j := range c.GroupMembers {
			m := &c.GroupMembers[j]
			if m.InstancePath != nil && uses(*m.InstancePath) {
				p := path(*m.InstancePath)
				m.InstancePath = &p
			}
		}
	}
	replacement := replaceInstancePayload{InstanceID: old.ID, AffectedBindingIDs: bindingIDs, Receiver: receiver, FrozenSourcePoses: frozen, ReferencedDocumentID: r.ReferencedDocumentID, ReferencedVersionID: r.ReplacementVersionID}
	if _, err = s.refreshReplacementFrames(&model, replacement, true); err != nil {
		return "", nil, err
	}
	if !receiver {
		if err = s.resolveAssemblySupports(ctx, &model); err != nil {
			return "", nil, err
		}
		variants, err := s.contextVariantsForProductModel(ctx, model)
		if err != nil {
			return "", nil, fmt.Errorf("REPLACEMENT_GEOMETRY_FAILED: %w", err)
		}
		for _, v := range variants {
			if v.Status != "READY" {
				return fail("REPLACEMENT_GEOMETRY_FAILED: 关联零件无法完成替换后的求值，请检查输入及下游特征")
			}
		}
		s.resolveProductPublications(ctx, &model, variants)
		for _, p := range model.Publications {
			if uses(p.Target.InstancePath) && p.Resolution.Status != "CONNECTED" {
				return fail("REPLACEMENT_FORWARDING_FAILED: 对外接口“" + p.Name + "”无法恢复")
			}
		}
	}

	slices.Sort(bindingIDs)
	slices.Sort(constraintIDs)
	slices.Sort(publicationIDs)
	return typeReplaceInstance, replaceInstancePayload{Receiver: receiver, ContextInputMapping: r.ContextInputMapping, FrozenSourcePoses: frozen, RootRevisionID: r.VersionID, SourceDocumentID: old.ReferencedDocumentID, SourceRevisionID: old.ReferencedVersionID, PublicationMapping: r.PublicationMapping, AffectedBindingIDs: bindingIDs, AffectedConstraintIDs: constraintIDs, AffectedPublicationIDs: publicationIDs, InstanceID: old.ID, ReferencedDocumentID: r.ReferencedDocumentID, ReferencedVersionID: r.ReplacementVersionID, Model: model}, nil
}

func validateReplacementConstraints(model ProductModel, instanceID string) error {
	for _, c := range model.Constraints {
		affected := false
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref != nil && (ref.InstanceID == instanceID || ref.InstancePath != nil && len(ref.InstancePath.Segments) > 0 && ref.InstancePath.Segments[0].InstanceID == instanceID) {
				affected = true
			}
		}
		for _, m := range c.GroupMembers {
			if m.InstanceID == instanceID || m.InstancePath != nil && len(m.InstancePath.Segments) > 0 && m.InstancePath.Segments[0].InstanceID == instanceID {
				affected = true
			}
		}
		if affected && (c.EvaluationStatus == modelcore.AssemblyConstraintBroken || c.EvaluationStatus == modelcore.AssemblyConstraintNotUpdated || c.EvaluationStatus == modelcore.AssemblyConstraintImpossible) {
			return fmt.Errorf("%w: REPLACEMENT_CONSTRAINT_FAILED: 约束“%s”无法迁移到新组件", ErrValidation, c.Name)
		}
	}
	return nil
}

// Only migrated relations accept this new owning snapshot. Unrelated accepted
// inputs remain byte-for-byte unchanged, including their acceptance evidence.
func stampReplacementSnapshot(model *ProductModel, p replaceInstancePayload, revisionID string) {
	stamp := func(path *InstancePath) {
		if path != nil && len(path.Segments) > 0 {
			path.Segments[0].OwnerVersionID = revisionID
		}
	}
	for i := range model.ContextBindings {
		b := &model.ContextBindings[i]
		if slices.Contains(p.AffectedBindingIDs, b.ID) {
			b.Accepted.RootProductRevisionID = revisionID
			stamp(&b.OwningInstancePath)
			if !p.Receiver {
				stamp(&b.SourceInstancePath)
			}
		}
	}
	for i := range model.Publications {
		v := &model.Publications[i]
		if slices.Contains(p.AffectedPublicationIDs, v.ID) {
			stamp(&v.Target.InstancePath)
		}
	}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if !slices.Contains(p.AffectedConstraintIDs, c.ID) {
			continue
		}
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref != nil && ref.InstancePath != nil && len(ref.InstancePath.Segments) > 0 && ref.InstancePath.Segments[0].InstanceID == p.InstanceID {
				stamp(ref.InstancePath)
			}
		}
		for j := range c.GroupMembers {
			m := &c.GroupMembers[j]
			if m.InstancePath != nil && len(m.InstancePath.Segments) > 0 && m.InstancePath.Segments[0].InstanceID == p.InstanceID {
				stamp(m.InstancePath)
			}
		}
	}
}

// Stable occurrence identity, rather than the order of bindings, determines both
// ends. Validate the complete accepted edge before deriving a relative frame.
func replacementBindingFrames(model ProductModel, p replaceInstancePayload) (map[string]InstancePose, error) {
	instances := map[string]ProductInstance{}
	for _, v := range model.Instances {
		instances[v.ID] = v
	}
	frames := map[string]InstancePose{}
	resolve := func(path InstancePath, sourceSnapshot bool) (ProductInstance, error) {
		if len(path.Segments) != 1 {
			return ProductInstance{}, fmt.Errorf("%w: REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED", ErrValidation)
		}
		edge := path.Segments[0]
		v, ok := instances[edge.InstanceID]
		if !ok || edge.ReferencedDocumentID != v.ReferencedDocumentID || (sourceSnapshot && edge.ResolvedVersionID != v.ReferencedVersionID) {
			return ProductInstance{}, fmt.Errorf("%w: REPLACEMENT_DIRECT_INSTANCE_REQUIRED", ErrValidation)
		}
		return v, nil
	}
	for _, b := range model.ContextBindings {
		if !slices.Contains(p.AffectedBindingIDs, b.ID) {
			continue
		}
		if b.ReferenceMode == "PINNED" {
			return nil, fmt.Errorf("%w: REPLACEMENT_PINNED_INPUT", ErrValidation)
		}
		owner, err := resolve(b.OwningInstancePath, false)
		if err != nil {
			return nil, err
		}
		if p.Receiver {
			pose, ok := p.FrozenSourcePoses[b.ID]
			if !ok || owner.ID != p.InstanceID {
				return nil, fmt.Errorf("%w: REPLACEMENT_IMPACT_CHANGED", ErrValidation)
			}
			frames[b.ID] = inverseRelativePose(pose, InstancePose{Translation: owner.Translation, Rotation: owner.Rotation})
		} else {
			source, err := resolve(b.SourceInstancePath, true)
			if err != nil {
				return nil, err
			}
			if source.ID != p.InstanceID || b.SourceInstancePath.RootDocumentID != b.OwningInstancePath.RootDocumentID {
				return nil, fmt.Errorf("%w: REPLACEMENT_IMPACT_CHANGED", ErrValidation)
			}
			frames[b.ID] = inverseRelativePose(InstancePose{Translation: source.Translation, Rotation: source.Rotation}, InstancePose{Translation: owner.Translation, Rotation: owner.Rotation})
		}
	}
	if len(frames) != len(p.AffectedBindingIDs) {
		return nil, fmt.Errorf("%w: REPLACEMENT_IMPACT_CHANGED", ErrValidation)
	}
	return frames, nil
}

// Translation tolerance is in the workbench's millimetres; orientation is in
// radians, matching the existing accepted-pose comparison policy.
func (s *Service) refreshReplacementFrames(model *ProductModel, p replaceInstancePayload, write bool) (bool, error) {
	frames, err := replacementBindingFrames(*model, p)
	if err != nil {
		return false, err
	}
	changed := false
	for i := range model.ContextBindings {
		b := &model.ContextBindings[i]
		expected, ok := frames[b.ID]
		if !ok {
			continue
		}
		if b.Transform != expected && !conflictPoseClose(geometry.AssemblyPose{Translation: b.Transform.Translation, Rotation: b.Transform.Rotation}, geometry.AssemblyPose{Translation: expected.Translation, Rotation: expected.Rotation}, 1e-7, 1e-8) {
			changed = true
		}
		if write {
			b.Transform = expected
		}
	}
	return changed, nil
}

func (s *Service) checkReplacementCandidate(ctx context.Context, model *ProductModel, p replaceInstancePayload) error {
	changed, err := s.refreshReplacementFrames(model, p, false)
	if err != nil {
		return err
	}
	if changed {
		return fmt.Errorf("%w: REPLACEMENT_FRAME_INCONSISTENT", ErrValidation)
	}
	if err = validateReplacementConstraints(*model, p.InstanceID); err != nil {
		return err
	}
	variants, err := s.contextVariantsForProductModel(ctx, *model)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			return fmt.Errorf("REPLACEMENT_GEOMETRY_FAILED: %w", err)
		}
		return err
	}
	s.resolveProductPublications(ctx, model, variants)
	for _, pub := range model.Publications {
		if slices.Contains(p.AffectedPublicationIDs, pub.ID) && pub.Resolution.Status != "CONNECTED" {
			return fmt.Errorf("%w: REPLACEMENT_FORWARDING_FAILED: %s", ErrValidation, pub.Name)
		}
	}
	return nil
}

// A bounded replacement-only alternation of the existing evaluator and solver.
// Each round consumes the previous accepted candidate pose. The final numerical
// evidence and displayed variants must describe that same physical frame.
func (s *Service) settleInstanceReplacement(ctx context.Context, documentID, revisionID, requestID string, model *ProductModel, p replaceInstancePayload, evidence *geometry.AssemblySolve) (string, error) {
	const maxRounds = 4
	for round := 0; round < maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if _, err := s.refreshReplacementFrames(model, p, true); err != nil {
			return "", err
		}
		// This also covers the no-constraint path, where solveAssembly is a no-op.
		if _, err := s.contextVariantsForProductModel(ctx, *model); err != nil {
			var failure *contextVariantEvaluationFailure
			if round != 0 || !p.Receiver || !errors.As(err, &failure) || failure.Code != "CONTEXT_VARIANT_GEOMETRY_EVALUATION_FAILED" || failure.Occurrence != p.InstanceID {
				if errors.Is(err, ErrValidation) {
					return "", fmt.Errorf("REPLACEMENT_GEOMETRY_FAILED: %w", err)
				}
				return "", err
			}
			if placementErr := s.placeReceiverBeforeEvaluation(ctx, documentID, revisionID, requestID, model, p); placementErr != nil {
				return "", fmt.Errorf("%w: initial placement: %v", placementErr, err)
			}
			if _, err := s.refreshReplacementFrames(model, p, true); err != nil {
				return "", err
			}
			if _, err := s.contextVariantsForProductModel(ctx, *model); err != nil {
				return "", err
			}
		}
		solveID := fmt.Sprintf("%s/replacement/%d", requestID, round)
		if err := s.solveAssembly(ctx, documentID, revisionID, solveID, "", nil, model, "", evidence); err != nil {
			return "", err
		}
		changed, err := s.refreshReplacementFrames(model, p, false)
		if err != nil {
			return "", err
		}
		if !changed {
			return solveID, s.checkReplacementCandidate(ctx, model, p)
		}
	}
	return "", fmt.Errorf("%w: REPLACEMENT_FRAME_INCONSISTENT: replacement did not settle within %d evaluations", ErrValidation, maxRounds)
}

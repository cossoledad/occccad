package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

func (s *Service) receiverReplacementInputs(ctx context.Context, root string, before ProductModel, instance ProductInstance, newPart PartModel, mapping map[string]string) ([]string, map[string]InstancePose, error) {
	fail := func(code string) ([]string, map[string]InstancePose, error) {
		return nil, nil, fmt.Errorf("%w: %s", ErrValidation, code)
	}
	old, err := s.readAssemblyPartModel(ctx, instance.ReferencedDocumentID, instance.ReferencedVersionID)
	if err != nil {
		return nil, nil, err
	}
	oldInputs, newInputs := map[string]ContextInput{}, map[string]ContextInput{}
	for _, i := range old.model.ContextInputs {
		oldInputs[i.ID] = i
	}
	for _, i := range newPart.ContextInputs {
		newInputs[i.ID] = i
	}
	used, targets, assigned := map[string]bool{}, map[string]bool{}, map[string]bool{}
	ids := []string{}
	frozen := map[string]InstancePose{}
	snapshots := map[string]ProductModel{}
	for _, b := range before.ContextBindings {
		if len(b.OwningInstancePath.Segments) == 0 || b.OwningInstancePath.Segments[0].InstanceID != instance.ID {
			continue
		}
		if len(b.OwningInstancePath.Segments) != 1 || len(b.SourceInstancePath.Segments) != 1 {
			return fail("REPLACEMENT_DEEP_REFERENCE_UNSUPPORTED")
		}
		if b.OwningInstancePath.RootDocumentID != root || b.SourceInstancePath.RootDocumentID != root || b.OwningInstancePath.Segments[0].ReferencedDocumentID != instance.ReferencedDocumentID {
			return fail("REPLACEMENT_IMPACT_CHANGED")
		}
		if b.ReferenceMode == "PINNED" {
			return fail("REPLACEMENT_PINNED_INPUT")
		}
		a, ok := oldInputs[b.ContextInputID]
		n, nok := newInputs[mapping[b.ContextInputID]]
		if !ok || !nok {
			return fail("REPLACEMENT_INPUT_MAPPING_REQUIRED")
		}
		if a.Type != n.Type || b.Publication.ExpectedType != n.Type || !publicationContractsCompatible(Publication{Type: a.Type, Contract: a.Contract}, Publication{Type: n.Type, Contract: n.Contract}) || a.Contract.UnitPolicy != n.Contract.UnitPolicy || !contextConsumerSupported(n.Target.Kind, n.Type) {
			return fail("REPLACEMENT_INPUT_INCOMPATIBLE")
		}
		target := n.Target.Kind + ":" + n.Target.TargetID
		if n.Target.Kind == "SKETCH_EXTERNAL_GEOMETRY" {
			target = n.ID
		}
		if targets[target] {
			return fail("REPLACEMENT_INPUT_DUPLICATE")
		}
		targets[target] = true
		assigned[n.ID] = true
		used[b.ContextInputID] = true
		if b.Accepted.Status != "CONNECTED" || b.Resolution.Status != "CONNECTED" {
			return fail("REPLACEMENT_INPUT_UNAVAILABLE")
		}
		accepted, ok := snapshots[b.Accepted.RootProductRevisionID]
		if !ok {
			var raw []byte
			if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, root, b.Accepted.RootProductRevisionID).Scan(&raw); err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(raw, &accepted); err != nil {
				return nil, nil, err
			}
			snapshots[b.Accepted.RootProductRevisionID] = accepted
		}
		owner := slices.IndexFunc(accepted.Instances, func(i ProductInstance) bool {
			return i.ID == instance.ID && i.ReferencedDocumentID == instance.ReferencedDocumentID
		})
		if owner < 0 {
			return fail("REPLACEMENT_INPUT_UNAVAILABLE")
		}
		v := accepted.Instances[owner]
		frozen[b.ID] = composeInstancePose(InstancePose{Translation: v.Translation, Rotation: normalizedInstanceRotation(v.Rotation)}, b.Transform)
		ids = append(ids, b.ID)
	}
	for oldID := range mapping {
		if !used[oldID] {
			return fail("REPLACEMENT_IMPACT_CHANGED")
		}
	}
	for _, n := range newPart.ContextInputs {
		if n.Required && !assigned[n.ID] {
			return fail("REPLACEMENT_REQUIRED_INPUT_MISSING")
		}
	}
	return ids, frozen, nil
}

// Placement may use only supports whose dependency closure is independent of
// incoming ports and solid/sketch evaluation. It uses the existing solver with
// the receiver's valid standalone defaults; no temporary geometry is committed.
func (s *Service) placeReceiverBeforeEvaluation(ctx context.Context, doc, rev, request string, model *ProductModel, p replaceInstancePayload) error {
	if !p.Receiver {
		return fmt.Errorf("%w: REPLACEMENT_GEOMETRY_FAILED", ErrValidation)
	}
	source, err := s.readAssemblyPartModel(ctx, p.ReferencedDocumentID, p.ReferencedVersionID)
	if err != nil {
		return err
	}
	graph, _, err := buildPartEvaluation(source.model, p.ReferencedVersionID, "replacement-placement", nil, nil)
	if err != nil {
		return err
	}
	bound := map[modelcore.DependencyKey]bool{}
	for _, b := range model.ContextBindings {
		if !slices.Contains(p.AffectedBindingIDs, b.ID) {
			continue
		}
		for _, i := range source.model.ContextInputs {
			if i.ID == b.ContextInputID {
				prefix := "datum:"
				if i.Target.Kind == "PARAMETER" {
					prefix = "parameter:"
				}
				if i.Target.Kind == "SKETCH_EXTERNAL_GEOMETRY" {
					prefix = "feature:"
				}
				bound[modelcore.DependencyKey(prefix+i.Target.TargetID)] = true
			}
		}
	}
	seen := map[modelcore.DependencyKey]bool{}
	valid := map[modelcore.DependencyKey]bool{}
	var independent func(modelcore.DependencyKey) bool
	independent = func(k modelcore.DependencyKey) bool {
		if bound[k] || strings.HasPrefix(string(k), "feature:") {
			return false
		}
		if seen[k] {
			return valid[k]
		}
		seen[k] = true
		for _, e := range graph.Edges {
			if e.Target == k && !independent(e.Source) {
				return false
			}
		}
		valid[k] = true
		return true
	}
	hasSupport := false
	for _, c := range model.Constraints {
		if c.Suppressed || c.Mode == "MEASURED" {
			continue
		}
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref == nil || ref.InstanceID != p.InstanceID {
				continue
			}
			if c.Kind == "FIX" && ref.Kind == "BODY" && ref.CapturedFrame == nil {
				hasSupport = true
				continue
			}
			if ref.PublicationRef == nil {
				return fmt.Errorf("%w: REPLACEMENT_PLACEMENT_UNAVAILABLE", ErrValidation)
			}
			var pub *Publication
			for i := range source.model.Publications {
				if source.model.Publications[i].ID == ref.PublicationRef.PublicationID {
					pub = &source.model.Publications[i]
				}
			}
			if pub == nil || pub.Target.Kind != "DATUM" || !independent(modelcore.DependencyKey("datum:"+pub.Target.DatumID)) {
				return fmt.Errorf("%w: REPLACEMENT_PLACEMENT_UNAVAILABLE", ErrValidation)
			}
			hasSupport = true
		}
	}
	if !hasSupport {
		return fmt.Errorf("%w: REPLACEMENT_PLACEMENT_UNAVAILABLE", ErrValidation)
	}
	placement, err := cloneAssemblyProduct(*model)
	if err != nil {
		return err
	}
	placement.ContextBindings = slices.DeleteFunc(placement.ContextBindings, func(b ContextBinding) bool { return slices.Contains(p.AffectedBindingIDs, b.ID) })
	if err = s.solveAssembly(ctx, doc, rev, request+"/installation", "", nil, &placement, ""); err != nil {
		return err
	}
	if err = validateReplacementConstraints(placement, p.InstanceID); err != nil {
		return err
	}
	for i := range model.Instances {
		model.Instances[i].Translation = placement.Instances[i].Translation
		model.Instances[i].Rotation = placement.Instances[i].Rotation
	}
	return nil
}

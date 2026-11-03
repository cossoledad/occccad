package workspace

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

// A publication name is a replacement-interface convention, not an identity.
// Discover a unique compatible port in the explicitly accepted replacement,
// then persist its ID. Renaming an already connected port never breaks its ID.
func replacementPublicationCorrespondence(before, after []Publication) map[string]Publication {
	oldCounts, newCounts := map[string]int{}, map[string]int{}
	nextByName := map[string]Publication{}
	for _, p := range before {
		oldCounts[p.Name]++
	}
	for _, p := range after {
		newCounts[p.Name]++
		nextByName[p.Name] = p
	}
	result := map[string]Publication{}
	for _, old := range before {
		if old.Name == "" || oldCounts[old.Name] != 1 || newCounts[old.Name] != 1 {
			continue
		}
		next := nextByName[old.Name]
		if next.Resolution.Status == "CONNECTED" && old.CompatibilityVersion == next.CompatibilityVersion && old.Contract.UnitPolicy == next.Contract.UnitPolicy && publicationContractsCompatible(old, next) {
			result[old.ID] = next
		}
	}
	return result
}

func (s *Service) replacementPublicationMap(ctx context.Context, old, next ProductInstance) (map[string]Publication, error) {
	read := func(i ProductInstance, resolve bool) ([]Publication, error) {
		var kind string
		var raw []byte
		if err := s.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.document_id=d.id WHERE d.id=$1 AND v.id=$2 AND d.deleted_at IS NULL`, i.ReferencedDocumentID, i.ReferencedVersionID).Scan(&kind, &raw); err != nil {
			return nil, err
		}
		if kind == "PART" {
			var part PartModel
			if err := json.Unmarshal(raw, &part); err != nil {
				return nil, err
			}
			if resolve {
				if err := s.projectFrozenPublications(ctx, i.ReferencedDocumentID, i.ReferencedVersionID, &part); err != nil {
					return nil, err
				}
			}
			return part.Publications, nil
		}
		var product ProductModel
		if err := json.Unmarshal(raw, &product); err != nil {
			return nil, err
		}
		ports := make([]Publication, 0, len(product.Publications))
		for _, p := range product.Publications {
			ports = append(ports, Publication{ID: p.ID, Name: p.Name, Type: p.Type, CompatibilityVersion: p.CompatibilityVersion, Contract: p.Contract, Resolution: p.Resolution})
		}
		return ports, nil
	}
	// The old component may itself be broken. Its interface definitions are
	// sufficient; replacing it must not require rebuilding its missing geometry.
	before, err := read(old, false)
	if err != nil {
		return nil, err
	}
	after, err := read(next, true)
	if err != nil {
		return nil, err
	}
	return replacementPublicationCorrespondence(before, after), nil
}

// Reconnect only ports on the replaced component's public boundary. Descendant
// private paths and receiving ContextInputs belong to other definitions and
// require explicit repair. Other occurrences of the same Part are untouched.
func reconnectReplacedPublications(model *ProductModel, old, next ProductInstance, ports map[string]Publication) {
	accepts := func(path *InstancePath) bool {
		return path == nil || len(path.Segments) == 1 && path.Segments[0].InstanceID == old.ID && path.Segments[0].ReferencedDocumentID == old.ReferencedDocumentID
	}
	rebind := func(path InstancePath) InstancePath {
		path.Segments = slices.Clone(path.Segments)
		path.Segments[0].ReferencedDocumentID = next.ReferencedDocumentID
		path.Segments[0].ResolvedVersionID = next.ReferencedVersionID
		return path
	}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		changed := false
		for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
			if ref == nil || ref.InstanceID != old.ID || ref.PublicationRef == nil || ref.PublicationRef.Scope == "LOCAL" || !accepts(ref.InstancePath) {
				continue
			}
			port, ok := ports[ref.PublicationRef.PublicationID]
			if !ok || ref.PublicationRef.ExpectedType != port.Type || ref.PublicationRef.CompatibilityVersion != "" && ref.PublicationRef.CompatibilityVersion != port.CompatibilityVersion {
				continue
			}
			updated := *ref
			if ref.InstancePath != nil {
				path := rebind(*ref.InstancePath)
				updated.InstancePath = &path
			}
			updated.PublicationRef = &PublicationRef{Scope: ref.PublicationRef.Scope, OwnerPath: updated.InstancePath, PublicationID: port.ID, ExpectedType: port.Type, CompatibilityVersion: port.CompatibilityVersion}
			updated.PersistentSelection, updated.Resolution, updated.PublicationResolution = nil, nil, nil
			updated.CapturedFrame, updated.CapturedDirection = nil, nil
			updated.GeometryKey, updated.TopologyID, updated.SourceVersionID = "", 0, ""
			if err := applyPublicationDescriptor(&updated, port); err != nil {
				continue
			}
			*ref = updated
			changed = true
		}
		if changed {
			c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			// Reconnecting one endpoint must not hide another broken support.
			for _, ref := range []*AssemblyGeometryRef{&c.First, c.Second, c.AngleAxis} {
				if ref != nil && (ref.Resolution != nil && ref.Resolution.Result.Status != "" && ref.Resolution.Result.Status != modelcore.SelectionResolved || ref.PublicationResolution != nil && ref.PublicationResolution.Status != "" && ref.PublicationResolution.Status != "CONNECTED") {
					c.EvaluationStatus = modelcore.AssemblyConstraintBroken
				}
			}
		}
	}
	for i := range model.Publications {
		p := &model.Publications[i]
		if !accepts(&p.Target.InstancePath) || p.Target.Element != nil {
			continue
		}
		port, ok := ports[p.Target.PublicationID]
		if !ok || p.CompatibilityVersion != port.CompatibilityVersion || !publicationContractsCompatible(Publication{Type: p.Type, Contract: p.Contract}, port) {
			continue
		}
		p.Target.InstancePath = rebind(p.Target.InstancePath)
		p.Target.PublicationID = port.ID
		// Resolution is recomputed by the normal Product publication evaluator.
	}
	for i := range model.ContextBindings {
		b := &model.ContextBindings[i]
		if !accepts(&b.SourceInstancePath) || len(b.OwningInstancePath.Segments) > 0 && b.OwningInstancePath.Segments[0].InstanceID == old.ID {
			continue
		}
		port, ok := ports[b.Publication.PublicationID]
		if !ok || b.Publication.ExpectedType != port.Type || b.Publication.CompatibilityVersion != "" && b.Publication.CompatibilityVersion != port.CompatibilityVersion {
			continue
		}
		b.SourceInstancePath = rebind(b.SourceInstancePath)
		b.Publication = PublicationRef{PublicationID: port.ID, ExpectedType: port.Type, CompatibilityVersion: port.CompatibilityVersion}
		b.Resolution = port.Resolution
		b.Accepted.SourceRevisionID, b.Accepted.SourceDigest, b.Accepted.ContractDigest, b.Accepted.Status = next.ReferencedVersionID, port.Resolution.SourceDigest, resolvedDigest(port.Contract), "CONNECTED"
	}
}

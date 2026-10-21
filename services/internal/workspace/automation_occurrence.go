package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/access"
)

// AutomationOccurrence separates an immutable definition from an occurrence's
// derived location. Definition is deliberately excluded from tool JSON.
type AutomationOccurrence struct {
	actor                  string
	acl                    CommandAuthorizer
	Root                   ModelSnapshot `json:"root"`
	Path                   InstancePath  `json:"instancePath"`
	SourceDocumentID       string        `json:"sourceDocumentId"`
	SourceRevisionID       string        `json:"sourceRevisionId"`
	SourceDocumentType     string        `json:"sourceDocumentType"`
	ReferenceMode          string        `json:"referenceMode"`
	WorldPose              InstancePose  `json:"worldPose"`
	ContextVariantRequired bool          `json:"contextVariantRequired"`
	Definition             ModelSnapshot `json:"-"`
}

// ReadAutomationOccurrence follows only the accepted revisions recorded in the
// requested root snapshot. FOLLOW_HEAD is an update policy, not permission to
// mix a new source Head into an already accepted Product revision.
func (s *Service) ReadAutomationOccurrence(ctx context.Context, acl CommandAuthorizer, actor, root, revision string, requested InstancePath) (AutomationOccurrence, error) {
	var out AutomationOccurrence
	if acl == nil || revision == "" || requested.RootDocumentID != root || len(requested.Segments) < 1 || len(requested.Segments) > instancePathMaxDepth {
		return out, fmt.Errorf("%w: root revision and stable occurrence path required", ErrValidation)
	}
	if _, err := acl.RequireDocument(ctx, root, actor, access.RoleViewer); err != nil {
		return out, err
	}
	snapshot, err := s.ReadModelSnapshot(ctx, root, revision)
	if err != nil {
		return out, err
	}
	if snapshot.DocumentType != "PRODUCT" {
		return out, fmt.Errorf("%w: occurrence root must be a Product", ErrValidation)
	}
	out.Root = snapshot
	out.actor, out.acl = actor, acl
	out.Path = InstancePath{RootDocumentID: root}
	out.WorldPose = InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	visiting := map[string]bool{root + "@" + revision: true}
	for depth, expected := range requested.Segments {
		if err := ctx.Err(); err != nil {
			return AutomationOccurrence{}, err
		}
		if snapshot.DocumentType != "PRODUCT" {
			return out, fmt.Errorf("%w: occurrence path traverses a Part", ErrValidation)
		}
		var product ProductModel
		if err := json.Unmarshal(snapshot.Model, &product); err != nil {
			return out, err
		}
		for _, binding := range product.ContextBindings {
			owner := binding.OwningInstancePath.Segments
			remaining := requested.Segments[depth:]
			if len(owner) == 0 {
				continue
			}
			shared := len(owner)
			if len(remaining) < shared {
				shared = len(remaining)
			}
			matches := true
			for i := 0; i < shared; i++ {
				if owner[i].InstanceID != remaining[i].InstanceID {
					matches = false
					break
				}
			}
			if matches {
				out.ContextVariantRequired = true
			}
		}
		var selected *ProductInstance
		for i := range product.Instances {
			if product.Instances[i].ID == expected.InstanceID && expected.InstanceID != "" {
				if selected != nil {
					return out, fmt.Errorf("%w: duplicate stable instance identity", ErrValidation)
				}
				selected = &product.Instances[i]
			}
		}
		if selected == nil {
			return out, ErrNotFound
		}
		actual := InstancePathSegment{OwnerDocumentID: snapshot.DocumentID, OwnerVersionID: snapshot.RevisionID, InstanceID: selected.ID, InstanceName: selected.Name, ReferencedDocumentID: selected.ReferencedDocumentID, ResolvedVersionID: selected.ReferencedVersionID}
		if (expected.OwnerDocumentID != "" && expected.OwnerDocumentID != actual.OwnerDocumentID) || (expected.OwnerVersionID != "" && expected.OwnerVersionID != actual.OwnerVersionID) || (expected.ReferencedDocumentID != "" && expected.ReferencedDocumentID != actual.ReferencedDocumentID) || (expected.ResolvedVersionID != "" && expected.ResolvedVersionID != actual.ResolvedVersionID) || actual.ResolvedVersionID == "" {
			return out, fmt.Errorf("%w: OCCURRENCE_SNAPSHOT_MISMATCH", ErrValidation)
		}
		// Authorize before reading the referenced definition, including intermediate
		// Products. Permission decisions are never cached with geometry.
		if _, err := acl.RequireDocument(ctx, actual.ReferencedDocumentID, actor, access.RoleViewer); err != nil {
			return AutomationOccurrence{}, err
		}
		key := actual.ReferencedDocumentID + "@" + actual.ResolvedVersionID
		if visiting[key] {
			return out, fmt.Errorf("%w: PRODUCT_REFERENCE_CYCLE", ErrValidation)
		}
		visiting[key] = true
		out.Path = appendInstancePath(out.Path, actual)
		out.WorldPose = composeInstancePose(out.WorldPose, InstancePose{Translation: selected.Translation, Rotation: normalizedInstanceRotation(selected.Rotation)})
		out.ReferenceMode = selected.ReferenceMode
		snapshot, err = s.ReadModelSnapshot(ctx, actual.ReferencedDocumentID, actual.ResolvedVersionID)
		if err != nil {
			return AutomationOccurrence{}, err
		}
	}
	out.SourceDocumentID, out.SourceRevisionID, out.SourceDocumentType = snapshot.DocumentID, snapshot.RevisionID, snapshot.DocumentType
	out.Definition = snapshot
	return out, nil
}

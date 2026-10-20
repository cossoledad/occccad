package workspace

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/occccad/occccad/internal/access"
)

type CommandAuthorizer interface {
	RequireDocument(context.Context, string, string, access.Role) (access.Role, error)
}
type CommandSnapshotReader interface {
	ReadModelSnapshot(context.Context, string, string) (ModelSnapshot, error)
}

// RequireCommandReferences is shared by ordinary commands and automation. The
// caller separately authorizes edits to target. New source references may never
// bypass read permission merely because the target is writable.
func RequireCommandReferences(ctx context.Context, acl CommandAuthorizer, reader CommandSnapshotReader, actor, target string, input CommandRequest) error {
	ids := []string{input.ReferencedDocumentID, input.SourceDocumentID, input.RootProductDocumentID}
	ids = append(ids, input.ReferencedDocumentIDs...)
	for _, path := range []*InstancePath{input.InstancePath, input.SourceInstancePath, input.OwningInstancePath} {
		if path != nil {
			ids = append(ids, path.RootDocumentID)
		}
	}
	if strings.ToUpper(input.Type) == "INSERT_INSTANCES" && input.InstanceID != "" {
		revision := ""
		if input.ExpectedHead != nil {
			revision = input.ExpectedHead.RevisionID
		}
		snapshot, err := reader.ReadModelSnapshot(ctx, target, revision)
		if err != nil {
			return err
		}
		var product ProductModel
		if err = json.Unmarshal(snapshot.Model, &product); err != nil {
			return err
		}
		for _, instance := range product.Instances {
			if instance.ID == input.InstanceID {
				ids = append(ids, instance.ReferencedDocumentID)
				break
			}
		}
	}
	seen := map[string]bool{target: true}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := acl.RequireDocument(ctx, id, actor, access.RoleViewer); err != nil {
			return err
		}
	}
	return nil
}

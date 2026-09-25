package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// CommitImportedGraph reuses each document definition and creates Products in
// dependency order. Each batch is an ordinary idempotent domain transaction;
// this does not promise atomic publication of the entire import job.
func (service *Service) CommitImportedGraph(ctx context.Context, actor, folderID, requestID, fallback string, graph *geometry.ExchangeGraph, documents map[string]DocumentView) (DocumentView, error) {
	if err := ValidateImportGraph(graph); err != nil {
		return DocumentView{}, err
	}
	if documents == nil {
		documents = map[string]DocumentView{}
	}
	definitions := map[string]*workerv1.ExchangeDefinition{}
	for _, def := range graph.Definitions {
		definitions[def.Id] = def
	}
	var build func(string) (DocumentView, error)
	var instances func(string, []*workerv1.ExchangeOccurrence) ([]ProductInstance, error)
	instances = func(owner string, children []*workerv1.ExchangeOccurrence) ([]ProductInstance, error) {
		result := make([]ProductInstance, 0, len(children))
		used := map[string]bool{}
		for _, child := range children {
			target, err := build(child.DefinitionId)
			if err != nil {
				return nil, err
			}
			name := child.Name
			if strings.TrimSpace(name) == "" {
				name = target.Document.Name
			}
			sourceName := name
			for suffix := 1; used[scopedNameKey(name)]; suffix++ {
				name = fmt.Sprintf("%s.%d", sourceName, suffix)
			}
			used[scopedNameKey(name)] = true
			var importedName *ImportedOccurrenceName
			if name != sourceName {
				importedName = &ImportedOccurrenceName{Source: sourceName, Assigned: name}
			}
			result = append(result, ProductInstance{ImportedName: importedName, ID: commandEntityID("instance", requestID+"/"+owner+"/"+child.Id), Name: name, ReferencedDocumentID: target.Document.ID, ReferencedVersionID: target.Document.VersionID, ReferenceMode: "PINNED", Translation: [3]float64{child.Translation.GetX(), child.Translation.GetY(), child.Translation.GetZ()}, Rotation: [4]float64{child.Rotation.GetX(), child.Rotation.GetY(), child.Rotation.GetZ(), child.Rotation.GetW()}})
		}
		return result, nil
	}
	build = func(id string) (DocumentView, error) {
		if view, ok := documents[id]; ok {
			return view, nil
		}
		def := definitions[id]
		if def.Kind != "PRODUCT" {
			return DocumentView{}, fmt.Errorf("missing evaluated Part definition %s", id)
		}
		children, err := instances(id, def.Children)
		if err != nil {
			return DocumentView{}, err
		}
		name := def.Name
		if strings.TrimSpace(name) == "" {
			name = fallback
		}
		view, err := service.CommitImportedProduct(ctx, actor, folderID, requestID+"/definition/"+id, name, children)
		if err == nil {
			documents[id] = view
		}
		return view, err
	}
	if len(graph.Roots) == 1 {
		root := graph.Roots[0]
		if root.Translation.GetX() == 0 && root.Translation.GetY() == 0 && root.Translation.GetZ() == 0 && root.Rotation.GetX() == 0 && root.Rotation.GetY() == 0 && root.Rotation.GetZ() == 0 && (root.Rotation.GetW() == 1 || root.Rotation.GetW() == -1) {
			return build(root.DefinitionId)
		}
	}
	roots, err := instances("source-roots", graph.Roots)
	if err != nil {
		return DocumentView{}, err
	}
	return service.CommitImportedProduct(ctx, actor, folderID, requestID+"/source-roots", fallback, roots)
}

// ExchangeExportGraph walks accepted immutable revisions, not the flattened
// render list. Geometry overrides carry existing context/release evaluation.
func (service *Service) ExchangeExportGraph(ctx context.Context, documentID, releaseID string, expectedRevision ...string) (*geometry.ExchangeGraph, error) {
	var revision string
	overrides := map[string]string{}
	if releaseID != "" {
		release, err := service.GetProductRelease(ctx, documentID, releaseID)
		if err != nil {
			return nil, err
		}
		revision = release.Manifest.RootProductRevisionID
		for _, o := range release.Manifest.Occurrences {
			if o.GeometryKey != "" {
				overrides[o.InstancePath.Canonical] = o.GeometryKey
			}
		}
	} else {
		view, err := service.GetDocument(ctx, documentID)
		if err != nil {
			return nil, err
		}
		if len(expectedRevision) > 0 && expectedRevision[0] != "" && view.Document.VersionID != expectedRevision[0] {
			return nil, fmt.Errorf("%w: export Head changed", ErrValidation)
		}
		if view.Document.Type == "PART" && (view.Artifact == nil || view.Artifact.Volume <= 0) {
			return nil, fmt.Errorf("%w: Part has no solid geometry to export", ErrValidation)
		}
		revision = view.Document.VersionID
		if view.Artifact != nil {
			overrides[""] = view.Artifact.GeometryKey
		}
		for _, o := range view.ResolvedInstances {
			overrides[o.InstancePath.Canonical] = o.GeometryKey
		}
	}
	graph := &geometry.ExchangeGraph{}
	type snapshot struct {
		name, kind, key string
		model           []byte
	}
	snapshots := map[string]snapshot{}
	built := map[string]bool{}
	visiting := map[string]bool{}
	var build func(string, string, string, int) (string, error)
	build = func(docID, rev, path string, depth int) (string, error) {
		identity := docID + "@" + rev
		if depth > 128 || visiting[identity] {
			return "", fmt.Errorf("%w: cyclic/deep exchange graph", ErrValidation)
		}
		visiting[identity] = true
		defer delete(visiting, identity)
		value, ok := snapshots[identity]
		if !ok {
			err := service.database.QueryRow(ctx, `SELECT d.name,d.document_type,v.model_json,COALESCE(v.geometry_key,'') FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, docID, rev).Scan(&value.name, &value.kind, &value.model, &value.key)
			if err != nil {
				return "", err
			}
			snapshots[identity] = value
		}
		def := &workerv1.ExchangeDefinition{Id: identity, Name: value.name, Kind: value.kind}
		if value.kind == "PART" {
			key := value.key
			if override := overrides[path]; override != "" {
				key = override
			}
			if key == "" {
				return "", fmt.Errorf("%w: Part has no exportable geometry", ErrValidation)
			}
			def.Id += "/" + key
			if !built[def.Id] {
				ref, err := service.brepArtifactReference(ctx, key)
				if err != nil {
					return "", err
				}
				def.Brep = &workerv1.ArtifactReference{Backend: ref.Backend, ObjectKey: ref.ObjectKey, Sha256: ref.SHA256, SizeBytes: uint64(ref.Size), ContentType: ref.ContentType}
			}
		} else {
			var model ProductModel
			if err := json.Unmarshal(value.model, &model); err != nil {
				return "", err
			}
			for _, instance := range model.Instances {
				childPath := instance.ID
				if path != "" {
					childPath = path + "/" + instance.ID
				}
				target, err := build(instance.ReferencedDocumentID, instance.ReferencedVersionID, childPath, depth+1)
				if err != nil {
					return "", err
				}
				q := normalizedInstanceRotation(instance.Rotation)
				instanceName := exchangeInstanceName(instance)
				def.Children = append(def.Children, &workerv1.ExchangeOccurrence{Id: instance.ID, Name: instanceName, DefinitionId: target, Translation: &workerv1.Vec3{X: instance.Translation[0], Y: instance.Translation[1], Z: instance.Translation[2]}, Rotation: &workerv1.Quaternion{X: q[0], Y: q[1], Z: q[2], W: q[3]}})
			}
			// Equal business definition + equal accepted child evaluation shares XDE
			// identity. Context variants never silently overwrite another occurrence.
			encoded, _ := json.Marshal(def.Children)
			def.Id += "/" + modelcore.ValueDigest(encoded)
		}
		if !built[def.Id] {
			built[def.Id] = true
			graph.Definitions = append(graph.Definitions, def)
		}
		return def.Id, nil
	}
	root, err := build(documentID, revision, "", 0)
	if err != nil {
		return nil, err
	}
	graph.Roots = []*workerv1.ExchangeOccurrence{{Id: "root", DefinitionId: root, Rotation: &workerv1.Quaternion{W: 1}}}
	if err := geometry.ValidateExchangeGraph(graph); err != nil {
		return nil, err
	}
	return graph, nil
}

// ValidateImportGraph applies existing Product working-set limits before any
// Part is evaluated/committed, including expansion of shared subassemblies.
func ValidateImportGraph(graph *geometry.ExchangeGraph) error {
	if err := geometry.ValidateExchangeGraph(graph); err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	defs := map[string]*workerv1.ExchangeDefinition{}
	for _, def := range graph.Definitions {
		defs[def.Id] = def
	}
	type cost struct{ members, depth int }
	costs := map[string]cost{}
	var visit func(string) (cost, error)
	visit = func(id string) (cost, error) {
		if c, ok := costs[id]; ok {
			return c, nil
		}
		c := cost{members: 1}
		for _, child := range defs[id].Children {
			next, err := visit(child.DefinitionId)
			if err != nil {
				return cost{}, err
			}
			c.members += next.members
			c.depth = max(c.depth, next.depth+1)
			if c.members > productContextMaxMembers || c.depth > instancePathMaxDepth {
				return cost{}, fmt.Errorf("%w: imported Product exceeds context size/depth budget", ErrValidation)
			}
		}
		costs[id] = c
		return c, nil
	}
	total := 0
	for _, root := range graph.Roots {
		c, err := visit(root.DefinitionId)
		if err != nil {
			return err
		}
		total += c.members
		if total+1 > productContextMaxMembers {
			return fmt.Errorf("%w: imported Product exceeds context size budget", ErrValidation)
		}
	}
	return nil
}

func exchangeInstanceName(instance ProductInstance) string {
	if instance.ImportedName != nil && instance.ImportedName.Assigned == instance.Name {
		return instance.ImportedName.Source
	}
	return instance.Name
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// BodyTopology is an immutable Naming projection. No OCCT local locator escapes
// this boundary. Each result carries the same formal selection as a native pick.
type BodyTopologyItem struct {
	ID                  string           `json:"id"`
	GeometryType        string           `json:"geometryType"`
	Selection           FeatureSelection `json:"selection"`
	Geometry            TopologyGeometry `json:"geometry"`
	FaceIDs             []string         `json:"faceIds"`
	RelationsComplete   bool             `json:"relationsComplete"`
	UnresolvedRelations int              `json:"unresolvedRelations"`
}
type BodyTopology struct {
	GeometryKey    string             `json:"geometryKey"`
	ManifestDigest string             `json:"manifestDigest"`
	Items          []BodyTopologyItem `json:"items"`
}

func (service *Service) ReadBodyTopology(ctx context.Context, document, revision, body, kind string) (BodyTopology, error) {
	result := BodyTopology{Items: []BodyTopologyItem{}}
	topologyKind := modelcore.PersistentTopologyType(strings.ToUpper(kind))
	if revision == "" || body == "" || (topologyKind != "EDGE" && topologyKind != "FACE" && topologyKind != "VERTEX") {
		return result, fmt.Errorf("%w: exact revision, Body and topology kind required", ErrValidation)
	}
	manifest, key, digest, err := service.topologyManifestForVersion(ctx, document, revision, body)
	if err != nil {
		return result, err
	}
	if !topologyHistoryComplete(manifest) {
		return result, fmt.Errorf("%w: TOPOLOGY_HISTORY_INCOMPLETE", ErrValidation)
	}
	tip := manifest.Tips[body]
	if tip == nil {
		return result, ErrNotFound
	}
	if len(tip.SemanticOutputs) > 200000 {
		return result, fmt.Errorf("%w: TOPOLOGY_QUERY_BUDGET", ErrValidation)
	}
	result.GeometryKey = key
	result.ManifestDigest = digest
	result.Items, err = projectBodyTopology(ctx, document, revision, body, topologyKind, tip.SemanticOutputs)
	return result, err
}

// Index exact current semantic references, including cross-type adjacency. No
// lineage, output-slot spelling or caller-provided creation evidence is used as
// a current geometric measurement. Formal selection hashing stays unchanged.
func projectBodyTopology(ctx context.Context, document, revision, body string, kind modelcore.PersistentTopologyType, outputs []*workerv1.SemanticTopologyOutput) ([]BodyTopologyItem, error) {
	type currentRef struct {
		kind modelcore.PersistentTopologyType
		id   string
	}
	index := map[string]currentRef{}
	items := []BodyTopologyItem{}
	evidence := []*workerv1.SelectionEvidence{}
	seen := map[string]bool{}
	refKey := func(ref *workerv1.SemanticTopologyRef) string {
		raw, _ := json.Marshal(semanticRef(ref))
		return string(raw)
	}
	for _, output := range outputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		outputKind := topologyType(output.GetTopologyType())
		key := refKey(output.GetSemanticRef())
		if _, exists := index[key]; exists {
			return nil, fmt.Errorf("%w: AMBIGUOUS_TOPOLOGY_IDENTITY", ErrValidation)
		}
		current := currentRef{kind: outputKind}
		if outputKind != kind && outputKind != "FACE" {
			index[key] = current
			continue
		}
		selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: document, SourceBodyID: body, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: outputKind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}
		if err := selection.Validate(); err != nil {
			return nil, err
		}
		raw, err := json.Marshal(selection)
		if err != nil {
			return nil, fmt.Errorf("%w: INVALID_NAMING_EVIDENCE", ErrValidation)
		}
		id := modelcore.ValueDigest(raw)
		if seen[id] {
			return nil, fmt.Errorf("%w: AMBIGUOUS_TOPOLOGY_IDENTITY", ErrValidation)
		}
		seen[id] = true
		current.id = id
		index[key] = current
		if outputKind == kind {
			items = append(items, BodyTopologyItem{ID: id, GeometryType: observedGeometryType(output.GetEvidence().GetGeometryType()), Geometry: observeTopologyGeometry(output.GetEvidence()), Selection: FeatureSelection{Selection: selection, SourceVersionID: revision}, FaceIDs: []string{}, RelationsComplete: true})
			evidence = append(evidence, output.GetEvidence())
		}
	}
	for i, e := range evidence {
		faces := map[string]bool{}
		// Repeated protobuf fields cannot distinguish absent evidence from an
		// asserted empty adjacency list. Keep that case conservative rather than
		// declaring that no face can be associated with this element.
		if len(e.GetAdjacent()) == 0 {
			items[i].RelationsComplete = false
		}
		for _, adjacent := range e.GetAdjacent() {
			ref, exists := index[refKey(adjacent)]
			if !exists {
				items[i].UnresolvedRelations++
				items[i].RelationsComplete = false
				continue
			}
			if ref.kind == "FACE" {
				faces[ref.id] = true
			}
		}
		for id := range faces {
			items[i].FaceIDs = append(items[i].FaceIDs, id)
		}
		sort.Strings(items[i].FaceIDs)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

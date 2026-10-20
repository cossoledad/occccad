package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// BodyTopology is an immutable Naming projection. No OCCT local locator escapes
// this boundary. Each result carries the same formal selection as a native pick.
type BodyTopologyItem struct {
	ID           string           `json:"id"`
	GeometryType string           `json:"geometryType"`
	Selection    FeatureSelection `json:"selection"`
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
	seen := map[string]bool{}
	for _, output := range tip.SemanticOutputs {
		if topologyType(output.GetTopologyType()) != topologyKind {
			continue
		}
		if err := ctx.Err(); err != nil {
			return BodyTopology{}, err
		}
		selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: document, SourceBodyID: body, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: topologyKind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}
		if err := selection.Validate(); err != nil {
			return BodyTopology{}, err
		}
		raw, _ := json.Marshal(selection)
		id := modelcore.ValueDigest(raw)
		if seen[id] {
			return BodyTopology{}, fmt.Errorf("%w: AMBIGUOUS_TOPOLOGY_IDENTITY", ErrValidation)
		}
		seen[id] = true
		result.Items = append(result.Items, BodyTopologyItem{ID: id, GeometryType: output.GetEvidence().GetGeometryType(), Selection: FeatureSelection{Selection: selection, SourceVersionID: revision}})
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].ID < result.Items[j].ID })
	return result, nil
}

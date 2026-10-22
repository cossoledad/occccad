package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// Entire serialized tool content, including scope, set token and cursor. Shared
// with the Host normalizer; this is not a task-wide token or call limit.
const MaxToolResponseBytes = 32 << 10

type TopologyQuery struct {
	RevisionID        string          `json:"revisionId"`
	BodyID            string          `json:"bodyId"`
	Kind              string          `json:"kind"`
	IDs               []string        `json:"ids,omitempty"`
	Filter            *TopologyFilter `json:"filter,omitempty"`
	Limit             int             `json:"limit,omitempty"`
	Cursor            string          `json:"cursor,omitempty"`
	IncludeSelections bool            `json:"includeSelections,omitempty"`
}
type topologySet struct {
	Document    string          `json:"document"`
	Revision    string          `json:"revision"`
	Body        string          `json:"body"`
	Kind        string          `json:"kind"`
	Digest      string          `json:"digest"`
	IDs         []string        `json:"ids,omitempty"`
	Filter      *TopologyFilter `json:"filter,omitempty"`
	Count       int             `json:"count"`
	Complete    bool            `json:"complete"`
	MatchDigest string          `json:"matchDigest"`
}
type topologyReader interface {
	ReadBodyTopology(context.Context, string, string, string, string) (workspace.BodyTopology, error)
}
type TopologyFaceRelations struct {
	IDs             []string `json:"ids"`
	KnownCount      int      `json:"knownCount"`
	Complete        bool     `json:"complete"`
	UnresolvedCount int      `json:"unresolvedCount,omitempty"`
}
type TopologyObservation struct {
	ID            string                      `json:"id"`
	GeometryType  string                      `json:"geometryType"`
	Geometry      workspace.TopologyGeometry  `json:"geometry"`
	AdjacentFaces TopologyFaceRelations       `json:"adjacentFaces"`
	Selection     *workspace.FeatureSelection `json:"selection,omitempty"`
}

func topologyMatchDigest(items []workspace.BodyTopologyItem) string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return modelcore.ValueDigest(marshal(ids))
}

func (s *Service) validateTopologyFace(ctx context.Context, document, revision, body string, filter *TopologyFilter, data workspace.BodyTopology, kind string) error {
	if filter == nil || filter.AdjacentFaceID == "" {
		return nil
	}
	faces := data
	if kind != "FACE" {
		var err error
		faces, err = s.workspace.(topologyReader).ReadBodyTopology(ctx, document, revision, body, "FACE")
		if err != nil {
			return err
		}
		if faces.ManifestDigest != data.ManifestDigest {
			return fault("TOPOLOGY_SELECTION_STALE", "adjacency scope manifest mismatch")
		}
	}
	for _, face := range faces.Items {
		if face.ID == filter.AdjacentFaceID {
			return nil
		}
	}
	return fault("TOPOLOGY_NOT_FOUND", "adjacentFaceId is not a current face in this fixed scope")
}

func (s *Service) QueryTopology(ctx context.Context, actor, document string, input TopologyQuery) (map[string]any, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return nil, err
	}
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.RevisionID == "" || input.BodyID == "" || (input.Kind != "EDGE" && input.Kind != "FACE" && input.Kind != "VERTEX") || input.Limit < 1 || input.Limit > 20 || len(input.IDs) > 100 || len(input.Cursor) > 2048 {
		return nil, fault("INVALID_QUERY", "exact revision/Body/kind, limit 1..20 and at most 100 stable IDs required")
	}
	if err := input.Filter.validate(); err != nil {
		return nil, err
	}
	reader, ok := s.workspace.(topologyReader)
	if !ok {
		return nil, fault("TOPOLOGY_UNAVAILABLE", "authoritative topology reader is unavailable")
	}
	data, err := reader.ReadBodyTopology(ctx, document, input.RevisionID, input.BodyID, input.Kind)
	if err != nil {
		return nil, err
	}
	if err = s.validateTopologyFace(ctx, document, input.RevisionID, input.BodyID, input.Filter, data, input.Kind); err != nil {
		return nil, err
	}
	matches, err := matchTopology(data, input.IDs, input.Filter)
	if err != nil {
		return nil, err
	}
	ids := append([]string(nil), input.IDs...)
	sort.Strings(ids)
	set := topologySet{Document: document, Revision: input.RevisionID, Body: input.BodyID, Kind: input.Kind, Digest: data.ManifestDigest, IDs: ids, Filter: input.Filter, Count: len(matches.items), Complete: matches.unknown == 0, MatchDigest: topologyMatchDigest(matches.items)}
	token := base64.RawURLEncoding.EncodeToString(marshal(set))
	cursorSet := modelcore.ValueDigest(marshal(struct {
		Token    string
		Limit    int
		Evidence bool
	}{token, input.Limit, input.IncludeSelections}))
	after := ""
	if input.Cursor != "" {
		var cursor struct{ Set, After string }
		raw, e := base64.RawURLEncoding.DecodeString(input.Cursor)
		if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Set != cursorSet {
			return nil, fault("INVALID_CURSOR", "topology cursor belongs to another snapshot, filter or projection")
		}
		after = cursor.After
	}
	start := 0
	if after != "" {
		start = sort.Search(len(matches.items), func(i int) bool { return matches.items[i].ID >= after })
		if start == len(matches.items) || matches.items[start].ID != after {
			return nil, fault("INVALID_CURSOR", "topology cursor is outside the matching set")
		}
		start++
	}
	rows := []TopologyObservation{}
	for _, item := range matches.items[start:] {
		if len(rows) == input.Limit {
			break
		}
		faceIDs := item.FaceIDs
		if faceIDs == nil {
			faceIDs = []string{}
		}
		complete := item.RelationsComplete
		if len(faceIDs) > 16 {
			faceIDs = faceIDs[:16]
			complete = false
		}
		row := TopologyObservation{ID: item.ID, GeometryType: item.GeometryType, Geometry: item.Geometry, AdjacentFaces: TopologyFaceRelations{IDs: faceIDs, KnownCount: len(item.FaceIDs), Complete: complete, UnresolvedCount: item.UnresolvedRelations}}
		if input.IncludeSelections {
			pick := item.Selection
			row.Selection = &pick
		}
		rows = append(rows, row)
	}
	out := map[string]any{
		"scope":  map[string]any{"documentId": document, "revisionId": input.RevisionID, "bodyId": input.BodyID, "kind": input.Kind, "geometryKey": data.GeometryKey, "manifestDigest": data.ManifestDigest, "coordinateSpace": "PART_LOCAL", "lengthUnit": "mm", "angleUnit": "rad", "observationSource": "CURRENT_NAMING_BODY_TIP"},
		"filter": input.Filter, "scopeCount": len(data.Items), "candidateCount": matches.candidates, "matchedCount": len(matches.items), "unknownCount": matches.unknown, "matchComplete": set.Complete,
		"selectionSet": token, "selectionSetCount": len(matches.items), "selectionSetComplete": set.Complete, "byteLimit": MaxToolResponseBytes,
	}
	// Account for the entire response after set generation, including the cursor.
	// A shorter page advances only past emitted items. Nested JSON is never sliced.
	for {
		more := start+len(rows) < len(matches.items)
		next := ""
		if more && len(rows) > 0 {
			next = base64.RawURLEncoding.EncodeToString(marshal(struct{ Set, After string }{cursorSet, rows[len(rows)-1].ID}))
		}
		out["items"], out["returnedCount"], out["hasMore"], out["nextCursor"] = rows, len(rows), more, next
		out["pageComplete"] = !more && len(rows) == len(matches.items)
		if len(marshal(out)) <= MaxToolResponseBytes {
			return out, nil
		}
		if len(rows) <= 1 {
			return nil, fault("TOPOLOGY_PAGE_RESOURCE", "one topology observation/evidence and its envelope exceed the tool byte limit; omit includeSelections or narrow explicit IDs")
		}
		rows = rows[:len(rows)-1]
	}
}

func (s *Service) resolveTopologySet(ctx context.Context, document string, set topologySet, data workspace.BodyTopology) ([]workspace.BodyTopologyItem, error) {
	if !set.Complete {
		return nil, fault("TOPOLOGY_FILTER_INCOMPLETE", "unknown filter membership cannot be executed")
	}
	if set.Count == 0 {
		return nil, fault("TOPOLOGY_SELECTION_EMPTY", "zero matches cannot select the whole Body")
	}
	if set.Count < 1 || set.Count > 4096 || len(set.IDs) > 100 || data.ManifestDigest != set.Digest {
		return nil, fault("TOPOLOGY_SELECTION_STALE", "selection manifest/count changed or is out of bounds")
	}
	if err := s.validateTopologyFace(ctx, document, set.Revision, set.Body, set.Filter, data, set.Kind); err != nil {
		return nil, err
	}
	matches, err := matchTopology(data, set.IDs, set.Filter)
	if err != nil {
		return nil, err
	}
	if matches.unknown != 0 || len(matches.items) != set.Count || topologyMatchDigest(matches.items) != set.MatchDigest {
		return nil, fault("TOPOLOGY_SELECTION_STALE", "selection membership changed or is incomplete")
	}
	return matches.items, nil
}

func (s *Service) resolveSelections(ctx context.Context, document string, a ActionRequest, command *workspace.CommandRequest) error {
	args, err := DecodeObject(a.Arguments)
	if err != nil {
		return err
	}
	token := textArg(args, "selectionSet")
	if token == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	var set topologySet
	if err != nil || len(raw) > 12<<10 || json.Unmarshal(raw, &set) != nil || set.Document != document || set.Revision != a.ExpectedHead.RevisionID || set.Body == "" {
		return fault("TOPOLOGY_SELECTION_STALE", "selectionSet must name the exact command baseline and Body")
	}
	if command.Feature == nil || command.Feature.BodyID != set.Body || len(command.Feature.Selections) != 0 {
		return fault("TOPOLOGY_SCOPE_MISMATCH", "selectionSet must belong to the feature Body and replace no other selections")
	}
	kind := "FACE"
	if command.Feature.Type == "FILLET" || command.Feature.Type == "CHAMFER" {
		kind = "EDGE"
	}
	if set.Kind != kind {
		return fault("TOPOLOGY_SCOPE_MISMATCH", "modifier topology kind mismatch")
	}
	reader, ok := s.workspace.(topologyReader)
	if !ok {
		return fault("TOPOLOGY_UNAVAILABLE", "authoritative topology reader is unavailable")
	}
	data, err := reader.ReadBodyTopology(ctx, document, set.Revision, set.Body, set.Kind)
	if err != nil {
		return err
	}
	items, err := s.resolveTopologySet(ctx, document, set, data)
	if err != nil {
		return err
	}
	for _, item := range items {
		command.Feature.Selections = append(command.Feature.Selections, item.Selection)
	}
	return nil
}

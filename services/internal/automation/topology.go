package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type TopologyQuery struct {
	RevisionID        string   `json:"revisionId"`
	BodyID            string   `json:"bodyId"`
	Kind              string   `json:"kind"`
	IDs               []string `json:"ids,omitempty"`
	Limit             int      `json:"limit,omitempty"`
	Cursor            string   `json:"cursor,omitempty"`
	IncludeSelections bool     `json:"includeSelections,omitempty"`
}
type topologySet struct {
	Document string   `json:"document"`
	Revision string   `json:"revision"`
	Body     string   `json:"body"`
	Kind     string   `json:"kind"`
	Digest   string   `json:"digest"`
	IDs      []string `json:"ids,omitempty"`
	Count    int      `json:"count"`
}
type topologyReader interface {
	ReadBodyTopology(context.Context, string, string, string, string) (workspace.BodyTopology, error)
}

func (s *Service) QueryTopology(ctx context.Context, actor, document string, input TopologyQuery) (map[string]any, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return nil, err
	}
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.Limit < 1 || input.Limit > 20 || len(input.IDs) > 100 || len(input.Cursor) > 2048 {
		return nil, fault("INVALID_QUERY", "Topology pages require limit 1..20 and at most 100 explicit stable IDs")
	}
	reader, ok := s.workspace.(topologyReader)
	if !ok {
		return nil, fault("TOPOLOGY_UNAVAILABLE", "authoritative topology reader is unavailable")
	}
	data, err := reader.ReadBodyTopology(ctx, document, input.RevisionID, input.BodyID, input.Kind)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, id := range input.IDs {
		if id == "" || wanted[id] {
			return nil, fault("INVALID_QUERY", "unique stable topology IDs required")
		}
		wanted[id] = true
	}
	matches := []workspace.BodyTopologyItem{}
	for _, item := range data.Items {
		if len(wanted) == 0 || wanted[item.ID] {
			matches = append(matches, item)
		}
	}
	if len(wanted) > 0 && len(matches) != len(wanted) {
		return nil, fault("TOPOLOGY_NOT_FOUND", "some selected topology IDs do not belong to this snapshot")
	}
	set := topologySet{Document: document, Revision: input.RevisionID, Body: input.BodyID, Kind: input.Kind, Digest: data.ManifestDigest, IDs: input.IDs, Count: len(matches)}
	token := base64.RawURLEncoding.EncodeToString(marshal(set))
	cursorSet := modelcore.ValueDigest([]byte(token))
	after := ""
	if input.Cursor != "" {
		var cursor struct {
			Set   string
			After string
		}
		raw, e := base64.RawURLEncoding.DecodeString(input.Cursor)
		if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Set != cursorSet {
			return nil, fault("INVALID_CURSOR", "topology cursor belongs to another snapshot or selection")
		}
		after = cursor.After
	}
	items := []map[string]any{}
	last := ""
	more := false
	for _, item := range matches {
		if item.ID <= after {
			continue
		}
		if len(items) >= input.Limit {
			more = true
			break
		}
		row := map[string]any{"id": item.ID, "geometryType": item.GeometryType}
		if input.IncludeSelections {
			row["selection"] = item.Selection
		}
		items = append(items, row)
		last = item.ID
	}
	cursor := ""
	if more {
		cursor = base64.RawURLEncoding.EncodeToString(marshal(struct {
			Set   string
			After string
		}{cursorSet, last}))
	}
	return map[string]any{"revisionId": input.RevisionID, "bodyId": input.BodyID, "kind": input.Kind, "matchedCount": len(matches), "items": items, "hasMore": more, "nextCursor": cursor, "selectionSet": token, "selectionSetCount": len(matches), "selectionSetComplete": true, "pageComplete": !more && len(items) == len(matches)}, nil
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
	if err != nil || len(raw) > 12<<10 || json.Unmarshal(raw, &set) != nil || set.Document != document || set.Revision != a.ExpectedHead.RevisionID || set.Body == "" || set.Count < 1 || set.Count > 4096 || len(set.IDs) > 100 {
		return fault("TOPOLOGY_SELECTION_STALE", "selectionSet must name 1..4096 exact topology elements in the current task baseline")
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
	if data.ManifestDigest != set.Digest {
		return fault("TOPOLOGY_SELECTION_STALE", "selection manifest changed")
	}
	wanted := map[string]bool{}
	for _, id := range set.IDs {
		if wanted[id] {
			return fault("INVALID_ARGUMENTS", "duplicate topology identity")
		}
		wanted[id] = true
	}
	for _, item := range data.Items {
		if len(wanted) == 0 || wanted[item.ID] {
			command.Feature.Selections = append(command.Feature.Selections, item.Selection)
		}
	}
	if len(command.Feature.Selections) != set.Count || len(wanted) > 0 && len(command.Feature.Selections) != len(wanted) {
		return fault("TOPOLOGY_SELECTION_STALE", "selection set is incomplete")
	}
	return nil
}

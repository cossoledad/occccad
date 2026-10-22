package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type topologyTestWorkspace struct {
	Workspace
	data     map[string]workspace.BodyTopology
	revision string
}

func (w *topologyTestWorkspace) ReadBodyTopology(_ context.Context, _, revision, body, kind string) (workspace.BodyTopology, error) {
	if revision != w.revision || body != "body" {
		return workspace.BodyTopology{}, workspace.ErrNotFound
	}
	return w.data[kind], nil
}

type topologyTestACL struct{}

func (topologyTestACL) RequireDocument(context.Context, string, string, access.Role) (access.Role, error) {
	return access.RoleEditor, nil
}
func radius(v float64) *float64 { return &v }

func TestTopologyFilterSetPaginationAndExecution(t *testing.T) {
	actor, doc, rev := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ws := &topologyTestWorkspace{revision: rev, data: map[string]workspace.BodyTopology{
		"EDGE": {ManifestDigest: "manifest", Items: []workspace.BodyTopologyItem{
			{ID: "a", GeometryType: "LINE", RelationsComplete: true},
			{ID: "b", GeometryType: "CIRCLE", Geometry: workspace.TopologyGeometry{RadiusMM: radius(3)}, FaceIDs: []string{"wall"}, RelationsComplete: true},
			{ID: "c", GeometryType: "CIRCLE", Geometry: workspace.TopologyGeometry{RadiusMM: radius(3.0000005)}, FaceIDs: []string{"wall"}, RelationsComplete: true},
			{ID: "d", GeometryType: "CIRCLE", Geometry: workspace.TopologyGeometry{RadiusMM: radius(3)}, FaceIDs: []string{"boss"}, RelationsComplete: true},
		}},
		"FACE": {ManifestDigest: "manifest", Items: []workspace.BodyTopologyItem{{ID: "wall"}, {ID: "boss"}}},
	}}
	s := New(nil, ws, topologyTestACL{})
	input := TopologyQuery{RevisionID: rev, BodyID: "body", Kind: "EDGE", Limit: 1, Filter: &TopologyFilter{GeometryType: "CIRCLE", Radius: &TopologyRadiusFilter{EqualMM: radius(3), ToleranceMM: 1e-6}, AdjacentFaceID: "wall"}}
	first, err := s.QueryTopology(t.Context(), actor, doc, input)
	if err != nil || first["matchedCount"] != 2 || first["selectionSetCount"] != 2 || first["selectionSetComplete"] != true || first["pageComplete"] != false {
		t.Fatal(first, err)
	}
	if first["items"].([]TopologyObservation)[0].ID != "b" {
		t.Fatal("filter applied after pagination", first)
	}
	token := first["selectionSet"].(string)
	input.Cursor = first["nextCursor"].(string)
	last, err := s.QueryTopology(t.Context(), actor, doc, input)
	if err != nil || last["hasMore"] != false || last["pageComplete"] != false || last["selectionSet"] != token || last["items"].([]TopologyObservation)[0].ID != "c" {
		t.Fatal(last, err)
	}
	for _, mutate := range []func(*TopologyQuery){
		func(q *TopologyQuery) { q.Filter = &TopologyFilter{GeometryType: "LINE"} },
		func(q *TopologyQuery) { q.IncludeSelections = true },
		func(q *TopologyQuery) { q.Limit = 2 },
	} {
		changed := input
		mutate(&changed)
		if _, e := s.QueryTopology(t.Context(), actor, doc, changed); e == nil || !strings.Contains(e.Error(), "INVALID_CURSOR") {
			t.Fatal("cursor accepted another query", e)
		}
	}
	resolve := func(token, revision string) (workspace.CommandRequest, error) {
		command := workspace.CommandRequest{Feature: &workspace.Feature{BodyID: "body", Type: "CHAMFER"}}
		err := s.resolveSelections(t.Context(), doc, ActionRequest{ExpectedHead: workspace.CommandBaseline{RevisionID: revision}, Arguments: marshal(map[string]any{"selectionSet": token})}, &command)
		return command, err
	}
	command, err := resolve(token, rev)
	if err != nil || len(command.Feature.Selections) != 2 {
		t.Fatal("command did not use the complete filtered set", command, err)
	}
	if _, err = resolve(token, uuid.NewString()); err == nil {
		t.Fatal("set revision silently replaced")
	}
	querySubset := input
	querySubset.Cursor = ""
	querySubset.IDs = []string{"a"}
	subset, subsetErr := s.QueryTopology(t.Context(), actor, doc, querySubset)
	if subsetErr != nil || subset["candidateCount"] != 1 || subset["matchedCount"] != 0 {
		t.Fatal("IDs must define scope before geometric filtering", subset, subsetErr)
	}
	missingFace := input
	missingFace.Cursor = ""
	missingFace.Filter = &TopologyFilter{AdjacentFaceID: "not-a-face-in-this-revision"}
	if _, err = s.QueryTopology(t.Context(), actor, doc, missingFace); err == nil || !strings.Contains(err.Error(), "TOPOLOGY_NOT_FOUND") {
		t.Fatal("out-of-scope face filter accepted", err)
	}
	// Count alone cannot authorize a different matching collection.
	raw, _ := base64.RawURLEncoding.DecodeString(token)
	var forged topologySet
	_ = json.Unmarshal(raw, &forged)
	forged.MatchDigest = "other-members"
	if _, err = resolve(base64.RawURLEncoding.EncodeToString(marshal(forged)), rev); err == nil {
		t.Fatal("set digest unchecked")
	}
	ws.data["EDGE"] = workspace.BodyTopology{ManifestDigest: "changed", Items: ws.data["EDGE"].Items}
	if _, err = resolve(token, rev); err == nil {
		t.Fatal("manifest change ignored")
	}
}

func TestTopologyUnknownEmptyAndBoundedOutput(t *testing.T) {
	actor, doc, rev := uuid.NewString(), uuid.NewString(), uuid.NewString()
	data := workspace.BodyTopology{ManifestDigest: "manifest", Items: []workspace.BodyTopologyItem{{ID: "known", GeometryType: "CIRCLE", Geometry: workspace.TopologyGeometry{RadiusMM: radius(3)}, RelationsComplete: true}, {ID: "unknown", GeometryType: "CIRCLE"}}}
	ws := &topologyTestWorkspace{revision: rev, data: map[string]workspace.BodyTopology{"EDGE": data}}
	s := New(nil, ws, topologyTestACL{})
	query := TopologyQuery{RevisionID: rev, BodyID: "body", Kind: "EDGE", Filter: &TopologyFilter{GeometryType: "CIRCLE", Radius: &TopologyRadiusFilter{EqualMM: radius(3), ToleranceMM: 1e-6}}}
	out, err := s.QueryTopology(t.Context(), actor, doc, query)
	if err != nil || out["matchedCount"] != 1 || out["unknownCount"] != 1 || out["matchComplete"] != false || out["selectionSetComplete"] != false {
		t.Fatal(out, err)
	}
	parse := func(out map[string]any) topologySet {
		raw, _ := base64.RawURLEncoding.DecodeString(out["selectionSet"].(string))
		var set topologySet
		_ = json.Unmarshal(raw, &set)
		return set
	}
	if _, err = s.resolveTopologySet(t.Context(), doc, parse(out), data); err == nil || !strings.Contains(err.Error(), "INCOMPLETE") {
		t.Fatal("unknown membership executed", err)
	}
	query.Filter = &TopologyFilter{GeometryType: "LINE"}
	empty, err := s.QueryTopology(t.Context(), actor, doc, query)
	if err != nil || empty["matchedCount"] != 0 || empty["selectionSetCount"] != 0 || empty["pageComplete"] != true {
		t.Fatal(empty, err)
	}
	if _, err = s.resolveTopologySet(t.Context(), doc, parse(empty), data); err == nil || !strings.Contains(err.Error(), "EMPTY") {
		t.Fatal("zero matches selected the whole scope", err)
	}
	for _, r := range []*TopologyRadiusFilter{{EqualMM: radius(3)}, {EqualMM: radius(3), MinMM: radius(2), ToleranceMM: 1e-6}, {MinMM: radius(4), MaxMM: radius(3), ToleranceMM: 1e-6}} {
		query.Filter = &TopologyFilter{Radius: r}
		if _, err = s.QueryTopology(t.Context(), actor, doc, query); err == nil {
			t.Fatal("invalid radius filter accepted", r)
		}
	}
	query.Filter = nil
	query.IncludeSelections = true
	query.Limit = 20
	data.Items = nil
	for i := 0; i < 8; i++ {
		data.Items = append(data.Items, workspace.BodyTopologyItem{ID: fmt.Sprintf("edge-%d", i), GeometryType: "LINE", Selection: workspace.FeatureSelection{Selection: modelcore.PersistentSelection{Anchor: modelcore.SemanticTopologyRef{SourceIDs: []string{strings.Repeat("证据", 1000)}}}}})
	}
	ws.data["EDGE"] = data
	page, err := s.QueryTopology(t.Context(), actor, doc, query)
	if err != nil || len(marshal(page)) > MaxToolResponseBytes || page["hasMore"] != true || page["returnedCount"].(int) >= 8 || page["returnedCount"].(int) < 1 {
		t.Fatal("whole response not byte-bounded", len(marshal(page)), page, err)
	}
	count := page["returnedCount"].(int)
	for page["hasMore"] == true {
		query.Cursor = page["nextCursor"].(string)
		page, err = s.QueryTopology(t.Context(), actor, doc, query)
		if err != nil || len(marshal(page)) > MaxToolResponseBytes {
			t.Fatal(err)
		}
		count += page["returnedCount"].(int)
	}
	if count != 8 {
		t.Fatal("byte-based pagination lost items", count)
	}
	query.Cursor = ""
	query.IncludeSelections = false
	compact, err := s.QueryTopology(t.Context(), actor, doc, query)
	if err != nil || strings.Contains(string(marshal(compact)), "sourceIds") || strings.Contains(string(marshal(compact)), "creationEvidence") {
		t.Fatal("evidence leaked by default", err)
	}
	data.Items[0].Selection.Selection.Anchor.SourceIDs = []string{strings.Repeat("证据", 10000)}
	ws.data["EDGE"] = data
	query.IncludeSelections = true
	query.Limit = 1
	if _, err = s.QueryTopology(t.Context(), actor, doc, query); err == nil || !strings.Contains(err.Error(), "TOPOLOGY_PAGE_RESOURCE") {
		t.Fatal("oversized indivisible evidence was returned", err)
	}
}

func TestTopologyFilterUsesFullRelations(t *testing.T) {
	ids := []string{}
	for i := 0; i < 30; i++ {
		ids = append(ids, fmt.Sprintf("face-%02d", i))
	}
	item := workspace.BodyTopologyItem{ID: "edge", GeometryType: "CIRCLE", FaceIDs: ids, RelationsComplete: true}
	matched, known := (&TopologyFilter{AdjacentFaceID: ids[29]}).match(item)
	if !matched || !known {
		t.Fatal("relations limited before filtering")
	}
	data := workspace.BodyTopology{ManifestDigest: "manifest", Items: []workspace.BodyTopologyItem{item}}
	faces := workspace.BodyTopology{ManifestDigest: "manifest"}
	for _, id := range ids {
		faces.Items = append(faces.Items, workspace.BodyTopologyItem{ID: id})
	}
	actor, doc, revision := uuid.NewString(), uuid.NewString(), uuid.NewString()
	ws := &topologyTestWorkspace{revision: revision, data: map[string]workspace.BodyTopology{"EDGE": data, "FACE": faces}}
	s := New(nil, ws, topologyTestACL{})
	page, err := s.QueryTopology(t.Context(), actor, doc, TopologyQuery{RevisionID: revision, BodyID: "body", Kind: "EDGE", Filter: &TopologyFilter{AdjacentFaceID: ids[29]}})
	if err != nil || page["matchedCount"] != 1 || page["selectionSetComplete"] != true {
		t.Fatal("query truncated before matching", page, err)
	}
	row := page["items"].([]TopologyObservation)[0]
	if len(row.AdjacentFaces.IDs) != 16 || row.AdjacentFaces.Complete || row.AdjacentFaces.KnownCount != 30 {
		t.Fatal("shallow relation completeness not explicit", row)
	}
	raw, _ := base64.RawURLEncoding.DecodeString(page["selectionSet"].(string))
	var set topologySet
	_ = json.Unmarshal(raw, &set)
	resolved, err := s.resolveTopologySet(t.Context(), doc, set, data)
	if err != nil || len(resolved) != 1 || resolved[0].ID != item.ID {
		t.Fatal("resolver used displayed relations instead of full set", resolved, err)
	}
	item.FaceIDs = nil
	item.RelationsComplete = false
	matched, known = (&TopologyFilter{AdjacentFaceID: ids[29]}).match(item)
	if matched || known {
		t.Fatal("missing relations became known non-match")
	}
}

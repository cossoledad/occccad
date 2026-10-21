package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type OccurrenceQuery struct {
	RevisionID   string                  `json:"revisionId"`
	InstanceIDs  []string                `json:"instanceIds,omitempty"`
	InstancePath *workspace.InstancePath `json:"instancePath,omitempty"`
	BodyID       string                  `json:"bodyId,omitempty"`
	Kind         string                  `json:"kind,omitempty"`
	GeometryType string                  `json:"geometryType,omitempty"`
	Cursor       string                  `json:"cursor,omitempty"`
	Limit        int                     `json:"limit,omitempty"`
}
type OccurrenceBody struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GeometryKey string `json:"geometryKey,omitempty"`
}
type OccurrencePage struct {
	Snapshot           workspace.CommandBaseline      `json:"snapshot"`
	InstancePath       workspace.InstancePath         `json:"instancePath"`
	SourceDocumentID   string                         `json:"sourceDocumentId"`
	SourceRevisionID   string                         `json:"sourceRevisionId"`
	SourceDocumentType string                         `json:"sourceDocumentType"`
	ReferenceMode      string                         `json:"referenceMode"`
	WorldPose          workspace.InstancePose         `json:"worldPose"`
	Units              string                         `json:"units"`
	Bodies             []OccurrenceBody               `json:"bodies,omitempty"`
	BodyCount          int                            `json:"bodyCount"`
	Instances          []workspace.ProductInstance    `json:"instances,omitempty"`
	Items              []workspace.AutomationGeometry `json:"items"`
	MatchedCount       int                            `json:"matchedCount"`
	UnsupportedCount   int                            `json:"unsupportedCount"`
	HasMore            bool                           `json:"hasMore"`
	NextCursor         string                         `json:"nextCursor,omitempty"`
	Complete           bool                           `json:"complete"`
	Completeness       string                         `json:"completeness"`
	GeometryScope      string                         `json:"geometryScope"`
}
type occurrenceReader interface {
	ReadAutomationOccurrence(context.Context, workspace.CommandAuthorizer, string, string, string, workspace.InstancePath) (workspace.AutomationOccurrence, error)
	ReadAutomationOccurrenceGeometry(context.Context, workspace.AutomationOccurrence, string, string, string, string, int) (workspace.AutomationGeometryPage, error)
}

func occurrencePath(root string, ids []string, input *workspace.InstancePath) (workspace.InstancePath, error) {
	path := workspace.InstancePath{RootDocumentID: root}
	if input != nil {
		path = *input
	}
	if len(ids) > 0 {
		if input != nil && len(input.Segments) != len(ids) {
			return path, fault("INVALID_OCCURRENCE", "instanceIds and formal path differ")
		}
		for i, id := range ids {
			if id == "" || len(id) > 160 {
				return path, fault("INVALID_OCCURRENCE", "stable instance IDs required")
			}
			if input == nil {
				path.Segments = append(path.Segments, workspace.InstancePathSegment{InstanceID: id})
			} else if path.Segments[i].InstanceID != id {
				return path, fault("INVALID_OCCURRENCE", "instanceIds and formal path differ")
			}
		}
	}
	if path.RootDocumentID != root || len(path.Segments) == 0 || len(path.Segments) > 32 {
		return path, fault("INVALID_OCCURRENCE", "root-bound path of 1..32 stable instance IDs required")
	}
	return path, nil
}

// QueryOccurrence never accepts an arbitrary source document: the source and
// accepted Revision are derived from the root Product's stable occurrence path.
func (s *Service) QueryOccurrence(ctx context.Context, actor, document string, input OccurrenceQuery) (OccurrencePage, error) {
	var out OccurrencePage
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return out, err
	}
	if input.RevisionID == "" || len(input.Cursor) > 2048 {
		return out, fault("INVALID_QUERY", "exact root revision and bounded cursor required")
	}
	if input.Limit == 0 {
		input.Limit = 10
	}
	if input.Limit < 1 || input.Limit > 20 {
		return out, fault("INVALID_QUERY", "occurrence page limit must be 1..20")
	}
	path, err := occurrencePath(document, input.InstanceIDs, input.InstancePath)
	if err != nil {
		return out, err
	}
	reader, ok := s.workspace.(occurrenceReader)
	if !ok {
		return out, fault("OCCURRENCE_UNAVAILABLE", "authoritative occurrence reader is unavailable")
	}
	occurrence, err := reader.ReadAutomationOccurrence(ctx, s.access, actor, document, input.RevisionID, path)
	if err != nil {
		return out, err
	}
	out = OccurrencePage{Snapshot: occurrence.Root.CommandBaseline, InstancePath: occurrence.Path, SourceDocumentID: occurrence.SourceDocumentID, SourceRevisionID: occurrence.SourceRevisionID, SourceDocumentType: occurrence.SourceDocumentType, ReferenceMode: occurrence.ReferenceMode, WorldPose: occurrence.WorldPose, Units: "mm,rad; definition local -> worldPose -> root Product", Items: []workspace.AutomationGeometry{}, Complete: true, Completeness: "accepted immutable definition; requested page only"}
	out.GeometryScope = "ACCEPTED_DEFINITION"
	if occurrence.ContextVariantRequired {
		out.GeometryScope = "CONTEXT_VARIANT_NOT_AVAILABLE"
	}
	input.Kind, input.GeometryType = strings.ToUpper(input.Kind), strings.ToUpper(input.GeometryType)
	scope := modelcore.ValueDigest(marshal(struct {
		Revision             string
		Path                 workspace.InstancePath
		Body, Kind, Geometry string
	}{input.RevisionID, occurrence.Path, input.BodyID, input.Kind, input.GeometryType}))
	after := ""
	if input.Cursor != "" {
		var cursor struct{ Scope, After string }
		raw, e := base64.RawURLEncoding.DecodeString(input.Cursor)
		if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Scope != scope {
			return out, fault("INVALID_CURSOR", "occurrence cursor belongs to another snapshot or geometry query")
		}
		after = cursor.After
	}
	if occurrence.SourceDocumentType == "PART" {
		var part workspace.PartModel
		if err = json.Unmarshal(occurrence.Definition.Model, &part); err != nil {
			return out, err
		}
		out.BodyCount = len(part.Bodies)
		sort.Slice(part.Bodies, func(i, j int) bool { return part.Bodies[i].ID < part.Bodies[j].ID })
		pageBodies := input.Kind == "" || input.Kind == "BODY"
		found := after == "" || !pageBodies
		for _, body := range part.Bodies {
			if pageBodies {
				if body.ID == after {
					found = true
					continue
				}
				if body.ID <= after {
					continue
				}
			}
			if len(out.Bodies) >= input.Limit {
				if pageBodies {
					out.HasMore = true
					out.Complete = false
				}
				break
			}
			out.Bodies = append(out.Bodies, OccurrenceBody{ID: body.ID, Name: body.Name, GeometryKey: body.GeometryKey})
		}
		if !found {
			return out, fault("INVALID_CURSOR", "Body cursor is outside this occurrence snapshot")
		}
		if pageBodies {
			out.MatchedCount = out.BodyCount
			if out.HasMore {
				out.NextCursor = base64.RawURLEncoding.EncodeToString(marshal(struct{ Scope, After string }{scope, out.Bodies[len(out.Bodies)-1].ID}))
			}
		}
	} else if input.Kind == "" || input.Kind == "INSTANCE" {
		var product workspace.ProductModel
		if err = json.Unmarshal(occurrence.Definition.Model, &product); err != nil {
			return out, err
		}
		sort.Slice(product.Instances, func(i, j int) bool { return product.Instances[i].ID < product.Instances[j].ID })
		out.MatchedCount = len(product.Instances)
		found := after == ""
		for _, instance := range product.Instances {
			if instance.ID == after {
				found = true
				continue
			}
			if instance.ID <= after {
				continue
			}
			if len(out.Instances) == input.Limit {
				out.HasMore = true
				out.Complete = false
				break
			}
			out.Instances = append(out.Instances, instance)
		}
		if !found {
			return out, fault("INVALID_CURSOR", "occurrence child cursor is outside this snapshot")
		}
		if out.HasMore {
			out.NextCursor = base64.RawURLEncoding.EncodeToString(marshal(struct{ Scope, After string }{scope, out.Instances[len(out.Instances)-1].ID}))
		}
	}
	if input.Kind == "" || input.Kind == "BODY" || input.Kind == "INSTANCE" {
		out.Completeness = "definition children page only; request FACE, EDGE, VERTEX, DATUM or PUBLICATION for geometry"
		return out, nil
	}
	out.Instances = nil
	page, err := reader.ReadAutomationOccurrenceGeometry(ctx, occurrence, input.BodyID, input.Kind, input.GeometryType, after, input.Limit)
	if err != nil {
		return out, err
	}
	out.Items, out.MatchedCount, out.UnsupportedCount, out.Complete = page.Items, page.Total, page.Unsupported, page.Complete
	out.HasMore = page.NextID != ""
	if out.HasMore {
		out.NextCursor = base64.RawURLEncoding.EncodeToString(marshal(struct{ Scope, After string }{scope, page.NextID}))
	}
	if page.Unsupported > 0 {
		out.Completeness = "unsupported analytic supports omitted with explicit count; requested analytic page only"
	}
	return out, nil
}

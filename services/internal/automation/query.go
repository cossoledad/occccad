package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type EntityRef struct {
	DocumentID string `json:"documentId"`
	RevisionID string `json:"revisionId"`
	Kind       string `json:"kind"`
	ID         string `json:"id"`
}
type Query struct {
	RevisionID string   `json:"revisionId,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	OwnerID    string   `json:"ownerId,omitempty"`
	BodyID     string   `json:"bodyId,omitempty"`
	ID         string   `json:"id,omitempty"`
	Name       string   `json:"name,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Limit      int      `json:"limit,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
}
type Entity struct {
	Ref        EntityRef      `json:"ref"`
	Properties map[string]any `json:"properties"`
}
type QueryResult struct {
	Snapshot      workspace.ModelSnapshot `json:"snapshot"`
	Items         []Entity                `json:"items"`
	MatchedCount  int                     `json:"matchedCount"`
	ReturnedCount int                     `json:"returnedCount"`
	HasMore       bool                    `json:"hasMore"`
	NextCursor    string                  `json:"nextCursor,omitempty"`
	Complete      bool                    `json:"complete"`
	Scope         string                  `json:"scope"`
}
type queryCursor struct {
	RevisionID string `json:"revisionId"`
	Digest     string `json:"digest"`
	After      string `json:"after"`
}

var queryFields = map[string]bool{"definition": true, "definitionDigest": true, "name": true, "type": true, "bodyId": true, "ownerId": true, "length": true, "plane": true, "status": true, "parameterAlias": true, "value": true, "displayValue": true, "unit": true, "referencedDocumentId": true, "referencedRevisionId": true, "referenceMode": true, "geometryKey": true, "profileId": true}

func (s *Service) Query(ctx context.Context, actor, document string, q Query) (QueryResult, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return QueryResult{}, err
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if q.Limit < 1 || q.Limit > 100 || len(q.Name) > 256 || len(q.Fields) > 16 || len(q.Cursor) > 2048 {
		return QueryResult{}, fault("INVALID_QUERY", "bounded query requires limit 1..100 and at most 16 fields")
	}
	if len(q.Fields) == 0 {
		q.Fields = []string{"name", "type", "bodyId", "status"}
	}
	for _, f := range q.Fields {
		if !queryFields[f] {
			return QueryResult{}, fault("INVALID_QUERY", "unknown field: "+f)
		}
		if f == "definitionDigest" && q.ID == "" {
			return QueryResult{}, fault("INVALID_QUERY", "definitionDigest requires one exact feature id")
		}
	}
	sort.Strings(q.Fields)
	after := ""
	cursorText := q.Cursor
	q.Cursor = ""
	revision := q.RevisionID
	q.RevisionID = ""
	digest := modelcore.ValueDigest(marshal(q))
	q.RevisionID = revision
	if cursorText != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursorText)
		if err != nil {
			return QueryResult{}, fault("INVALID_CURSOR", "invalid cursor")
		}
		var c queryCursor
		if json.Unmarshal(raw, &c) != nil || c.Digest != digest || revision != "" && revision != c.RevisionID {
			return QueryResult{}, fault("INVALID_CURSOR", "cursor belongs to a different query")
		}
		q.RevisionID = c.RevisionID
		after = c.After
	}
	if q.RevisionID != "" {
		if _, err := uuid.Parse(q.RevisionID); err != nil {
			return QueryResult{}, fault("INVALID_QUERY", "invalid revision identity")
		}
	}
	snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, q.RevisionID)
	if err != nil {
		return QueryResult{}, err
	}
	rows, err := definitionEntities(snapshot, q.Kind)
	if err != nil {
		return QueryResult{}, err
	}
	for _, field := range q.Fields {
		if field != "definitionDigest" {
			continue
		}
		if snapshot.DocumentType != "PART" {
			return QueryResult{}, fault("INVALID_QUERY", "definitionDigest is a Part feature edit baseline")
		}
		var part workspace.PartModel
		if err = json.Unmarshal(snapshot.Model, &part); err != nil {
			return QueryResult{}, err
		}
		digest, e := workspace.FeatureDefinitionDigest(part, q.ID)
		if e != nil {
			return QueryResult{}, e
		}
		for i := range rows {
			if rows[i].Ref.ID == q.ID && (rows[i].Ref.Kind == "FEATURE" || rows[i].Ref.Kind == "SKETCH") {
				rows[i].Properties[field] = digest
			}
		}
	}
	result := QueryResult{Snapshot: snapshot, Items: []Entity{}, Scope: "DOCUMENT_DEFINITION"}
	encodedBytes := 0
	last := ""
	for _, row := range rows {
		if q.OwnerID != "" && row.Properties["ownerId"] != q.OwnerID || q.Kind != "" && row.Ref.Kind != q.Kind || q.ID != "" && row.Ref.ID != q.ID || q.BodyID != "" && row.Properties["bodyId"] != q.BodyID {
			continue
		}
		name, _ := row.Properties["name"].(string)
		if !strings.Contains(strings.ToLower(name), strings.ToLower(q.Name)) {
			continue
		}
		result.MatchedCount++
		key := row.Ref.Kind + ":" + row.Ref.ID
		if key <= after {
			continue
		}
		if result.HasMore {
			continue
		}
		projected := Entity{Ref: row.Ref, Properties: map[string]any{}}
		for _, f := range q.Fields {
			if value, ok := row.Properties[f]; ok {
				projected.Properties[f] = value
			}
		}
		size := len(marshal(projected))
		if len(result.Items) >= q.Limit || encodedBytes+size > 32<<10 {
			if len(result.Items) == 0 {
				return QueryResult{}, fault("QUERY_OUTPUT_BUDGET_EXCEEDED", "requested fields exceed the query output budget")
			}
			result.HasMore = true
			continue
		}
		result.Items = append(result.Items, projected)
		encodedBytes += size
		last = key
	}
	result.ReturnedCount = len(result.Items)
	result.Complete = after == "" && !result.HasMore
	if result.HasMore {
		result.NextCursor = base64.RawURLEncoding.EncodeToString(marshal(queryCursor{snapshot.RevisionID, digest, last}))
	}
	return result, nil
}

func definitionEntities(snapshot workspace.ModelSnapshot, requestedKind string) ([]Entity, error) {
	rows := []Entity{}
	add := func(kind, id string, p map[string]any) {
		if len(rows) > 100000 {
			return
		}
		rows = append(rows, Entity{EntityRef{snapshot.DocumentID, snapshot.RevisionID, kind, id}, p})
	}
	if snapshot.DocumentType == "PART" {
		var part workspace.PartModel
		if err := json.Unmarshal(snapshot.Model, &part); err != nil {
			return nil, err
		}
		for _, b := range part.Bodies {
			add("BODY", b.ID, map[string]any{"name": b.Name, "type": "BODY", "bodyId": b.ID, "geometryKey": b.GeometryKey, "definition": b})
		}
		ownerBodies := map[string]string{}
		for _, f := range part.Features {
			ownerBodies[f.ID] = f.BodyID
			kind := "FEATURE"
			if f.Sketch != nil {
				kind = "SKETCH"
			}
			status := f.EvaluationStatus
			if f.Sketch != nil {
				status = f.Sketch.Solve.Status
			}
			properties := map[string]any{"name": f.Name, "type": f.Type, "bodyId": f.BodyID, "plane": f.Plane, "status": status, "profileId": f.Profile, "definition": f}
			if f.Sketch == nil {
				properties["length"] = f.Length
				properties["unit"] = "mm"
			}
			add(kind, f.ID, properties)
			if f.Sketch != nil {
				if requestedKind == "SKETCH_ENTITY" {
					for _, entity := range f.Sketch.Entities {
						add("SKETCH_ENTITY", entity.ID, map[string]any{"name": entity.ID, "type": entity.Kind, "bodyId": f.BodyID, "ownerId": f.ID, "definition": entity})
					}
				}
				if requestedKind == "SKETCH_CONSTRAINT" {
					for _, constraint := range f.Sketch.Constraints {
						add("SKETCH_CONSTRAINT", constraint.ID, map[string]any{"name": constraint.ID, "type": constraint.Kind, "bodyId": f.BodyID, "ownerId": f.ID, "definition": constraint})
					}
				}
			}
		}
		for _, p := range part.Parameters {
			name := p.DisplayName
			if name == "" {
				name = p.Label
			}
			if name == "" {
				name = p.Key
			}
			properties := map[string]any{"name": name, "type": p.ValueType, "parameterAlias": p.Key, "ownerId": p.OwnerFeatureID, "bodyId": ownerBodies[p.OwnerFeatureID], "value": p.EvaluatedValue, "unit": p.DisplayUnit, "definition": p}
			if p.EvaluatedValue != nil {
				if scale, err := modelcore.NewQuantity(1, p.DisplayUnit); err == nil && scale.Dimension.Equal(p.EvaluatedValue.Dimension) {
					properties["displayValue"] = p.EvaluatedValue.SIValue / scale.SIValue
				}
			}
			add("PARAMETER", p.ParameterID, properties)
		}
		for _, p := range part.DatumPlanes {
			add("DATUM_PLANE", p.ID, map[string]any{"name": p.Name, "plane": p.Plane, "type": "DATUM_PLANE", "definition": p})
		}
		for _, item := range part.DatumAxes {
			add("DATUM_AXIS", item.ID, map[string]any{"name": item.Name, "type": "DATUM_AXIS", "definition": item})
		}
		for _, item := range part.Publications {
			add("PUBLICATION", item.ID, map[string]any{"name": item.Name, "type": item.Type, "definition": item})
		}
		for _, item := range part.ContextInputs {
			add("CONTEXT_INPUT", item.ID, map[string]any{"name": item.Name, "type": item.Type, "definition": item})
		}
		for _, item := range part.ContextReferences {
			add("CONTEXT_REFERENCE", item.ID, map[string]any{"name": item.Name, "type": "CONTEXT_REFERENCE", "definition": item})
		}
	} else if snapshot.DocumentType == "PRODUCT" {
		var product workspace.ProductModel
		if err := json.Unmarshal(snapshot.Model, &product); err != nil {
			return nil, err
		}
		for _, i := range product.Instances {
			add("INSTANCE", i.ID, map[string]any{"name": i.Name, "type": "INSTANCE", "referencedDocumentId": i.ReferencedDocumentID, "referencedRevisionId": i.ReferencedVersionID, "referenceMode": i.ReferenceMode, "definition": i})
		}
		for _, item := range product.Constraints {
			add("ASSEMBLY_CONSTRAINT", item.ID, map[string]any{"name": item.Name, "type": item.Kind, "definition": item})
		}
		for _, item := range product.Publications {
			add("PUBLICATION", item.ID, map[string]any{"name": item.Name, "type": item.Type, "definition": item})
		}
		for _, item := range product.ContextBindings {
			add("CONTEXT_BINDING", item.ID, map[string]any{"name": item.Name, "type": "CONTEXT_BINDING", "definition": item})
		}
		for _, item := range product.Kinematics.Mechanisms {
			add("MECHANISM", item.ID, map[string]any{"name": item.Name, "type": "MECHANISM", "definition": item})
		}
	} else {
		return nil, fault("DOCUMENT_TYPE_UNSUPPORTED", "definition query supports Part and Product")
	}
	if len(rows) > 100000 {
		return nil, fault("MODEL_QUERY_BUDGET_EXCEEDED", "definition projection exceeds 100000 entities; narrow the document scope")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ref.Kind+":"+rows[i].Ref.ID < rows[j].Ref.Kind+":"+rows[j].Ref.ID })
	return rows, nil
}

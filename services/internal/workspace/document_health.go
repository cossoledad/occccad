package workspace

import "context"

type HealthSeverity string

const (
	HealthNormal  HealthSeverity = "NORMAL"
	HealthWarning HealthSeverity = "WARNING"
	HealthError   HealthSeverity = "ERROR"
)

// DocumentHealth is a read projection of a particular revision and its accepted references.
type DocumentHealth struct {
	Severity HealthSeverity `json:"severity"`
	Message  string         `json:"message,omitempty"`
}

func (service *Service) revisionHealth(ctx context.Context, versions []string) (map[string]*DocumentHealth, error) {
	result := map[string]*DocumentHealth{}
	if len(versions) == 0 {
		return result, nil
	}
	rows, err := service.database.Query(ctx, `WITH RECURSIVE included(id) AS (
 SELECT id FROM occccad.document_versions WHERE id=ANY($1)
 UNION SELECT p.referenced_version_id FROM included r JOIN occccad.product_instances p ON p.product_version_id=r.id
 )
 SELECT v.id::text,v.state,p.referenced_version_id::text,i.message
 FROM included r JOIN occccad.document_versions v ON v.id=r.id
 LEFT JOIN occccad.product_instances p ON p.product_version_id=v.id
 LEFT JOIN occccad.import_document_issues i ON i.document_id=v.document_id AND
 (i.code<>'EXCHANGE_COMPONENT_DISPLAY_UNSUPPORTED' OR NOT EXISTS (
    SELECT 1 FROM occccad.jobs j JOIN occccad.import_definitions d ON d.source_object_id=j.input_object_id
    WHERE j.id=i.job_id AND d.document_id=i.document_id AND d.definition->'source'->>'definitionId'=i.definition_id
    AND EXISTS (SELECT 1 FROM jsonb_array_elements(v.model_json->'features') f WHERE f->>'importDefinitionId'=d.id)
 ))`, versions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := map[string][]string{}
	local := map[string]DocumentHealth{}
	for rows.Next() {
		var id, state string
		var child, message *string
		if err := rows.Scan(&id, &state, &child, &message); err != nil {
			return nil, err
		}
		if child != nil {
			children[id] = append(children[id], *child)
		}
		health := local[id]
		switch state {
		case "FAILED":
			health = DocumentHealth{HealthError, "此版本计算失败"}
		case "EVALUATING", "PENDING":
			if health.Severity == "" {
				health = DocumentHealth{HealthWarning, "此版本等待计算完成"}
			}
		}
		if message != nil && health.Severity != HealthError {
			if health.Message == "" || *message < health.Message {
				health = DocumentHealth{HealthWarning, *message}
			}
		}
		if health.Severity == "" {
			health.Severity = HealthNormal
		}
		local[id] = health
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	visiting := map[string]bool{}
	var project func(string) *DocumentHealth
	project = func(id string) *DocumentHealth {
		if h := result[id]; h != nil {
			return h
		}
		if visiting[id] {
			return &DocumentHealth{HealthError, "装配引用形成循环"}
		}
		visiting[id] = true
		h := local[id]
		for _, child := range children[id] {
			c := project(child)
			if c.Severity == HealthError && h.Severity != HealthError {
				h = DocumentHealth{HealthError, "装配包含有错误的零件或子装配"}
			}
			if c.Severity == HealthWarning && h.Severity == HealthNormal {
				h = DocumentHealth{HealthWarning, "装配包含有警告的零件或子装配"}
			}
		}
		delete(visiting, id)
		result[id] = &h
		return &h
	}
	for _, id := range versions {
		project(id)
	}
	return result, nil
}

func applyDocumentHealth(view *DocumentView, health map[string]*DocumentHealth) {
	view.Document.Health = health[view.Document.VersionID]
	if view.Product != nil {
		for i := range view.Product.Instances {
			inst := &view.Product.Instances[i]
			inst.Health = health[inst.ReferencedVersionID]
		}
	}
	var apply func(*DocumentStructureNode)
	apply = func(n *DocumentStructureNode) {
		if n == nil {
			return
		}
		switch n.Kind {
		case "PART", "PRODUCT", "INSTANCE":
			n.Health = health[n.VersionID]
		default:
			return
		}
		for i := range n.Children {
			apply(&n.Children[i])
		}
	}
	apply(view.StructureTree)
}

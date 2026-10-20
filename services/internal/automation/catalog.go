// Package automation exposes bounded, provider-independent CAD operations.
// It compiles checked inputs into the existing workspace command pipeline.
package automation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type Fault struct {
	Phase        string `json:"phase,omitempty"`
	DiagnosticID string `json:"diagnosticId,omitempty"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	Retryable    bool   `json:"retryable"`
}

func (e *Fault) Error() string         { return e.Code + ": " + e.Message }
func fault(code, message string) error { return &Fault{Code: code, Message: message} }

type Field struct {
	Name     string   `json:"name"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Enum     []string `json:"enum,omitempty"`
	Positive bool     `json:"-"`
}

type Operation struct {
	ID            string         `json:"id"`
	Version       int            `json:"version"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	DocumentType  string         `json:"documentType"`
	Effect        string         `json:"effect"`
	Preconditions []string       `json:"preconditions"`
	Fields        []Field        `json:"fields"`
	InputSchema   map[string]any `json:"inputSchema"`
	OutputSchema  map[string]any `json:"outputSchema"`
	compile       func(map[string]any) workspace.CommandRequest
}

func stringField(name, title string, values ...string) Field {
	return Field{Name: name, Title: title, Type: "string", Required: true, Enum: values}
}
func numberField(name, title string) Field {
	return Field{Name: name, Title: title, Type: "number", Required: true, Positive: true}
}
func textArg(args map[string]any, key string) string { value, _ := args[key].(string); return value }
func numericArg(args map[string]any, key string) float64 {
	value, _ := args[key].(float64)
	return value
}

func builtinOperations() []Operation {
	body := stringField("bodyId", "目标实体")
	sketch := stringField("sketchId", "草图")
	unit := stringField("unit", "长度单位", "mm")
	parameter := stringField("parameterId", "参数")
	return []Operation{
		{ID: "sketch.create", Name: "创建草图", Description: "Create a sketch on a standard datum plane. XY/XZ/YZ; explicit target Body.", Fields: []Field{body, stringField("plane", "基准平面", "XY", "XZ", "YZ")}, compile: func(a map[string]any) workspace.CommandRequest {
			return workspace.CommandRequest{Type: "CREATE_SKETCH", BodyID: textArg(a, "bodyId"), Plane: textArg(a, "plane")}
		}},
		{ID: "sketch.rectangle", Name: "绘制矩形", Description: "Add an axis-aligned rectangle from the sketch origin. Width/height in mm; existing sketch constraints are preserved.", Fields: []Field{sketch, numberField("width", "宽度"), numberField("height", "高度"), unit}, compile: func(a map[string]any) workspace.CommandRequest {
			return workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: textArg(a, "sketchId"), Operations: []workspace.SketchOperation{{Type: "ADD_RECTANGLE", First: &workspace.SketchPoint2{}, Second: &workspace.SketchPoint2{X: numericArg(a, "width"), Y: numericArg(a, "height")}, FirstReference: &workspace.SketchGeometryRef{Target: "SKETCH_ORIGIN", SubElement: "POINT"}}}}
		}},
		{ID: "part.extrude", Name: "拉伸", Description: "Extrude a closed sketch profile into the specified Body (ADD / Pad). Length in mm; exact profile and solid validation remain authoritative.", Fields: []Field{body, sketch, numberField("length", "长度"), unit}, compile: func(a map[string]any) workspace.CommandRequest {
			return workspace.CommandRequest{Type: "PAD_SKETCH", BodyID: textArg(a, "bodyId"), SketchID: textArg(a, "sketchId"), Length: numericArg(a, "length")}
		}},
		{ID: "parameter.set_literal", Name: "修改参数值", Description: "Set a literal quantity with an explicit unit; preserve dimensional validation and dependent feature evaluation.", Fields: []Field{parameter, {Name: "value", Title: "数值", Type: "number", Required: true}, stringField("unit", "单位", "mm", "cm", "m", "in", "deg", "rad")}, compile: func(a map[string]any) workspace.CommandRequest {
			return workspace.CommandRequest{Type: "SET_PARAMETER_VALUE", ParameterID: textArg(a, "parameterId"), Value: numericArg(a, "value"), Unit: textArg(a, "unit")}
		}},
		{ID: "parameter.set_expression", Name: "修改参数表达式", Description: "Set a quantity expression using the Part parameter aliases; bind stable ParameterIds and enforce units, cycles and cost limits.", Fields: []Field{parameter, stringField("expression", "表达式")}, compile: func(a map[string]any) workspace.CommandRequest {
			return workspace.CommandRequest{Type: "SET_PARAMETER_EXPRESSION", ParameterID: textArg(a, "parameterId"), Expression: textArg(a, "expression")}
		}},
	}
}

type Catalog struct {
	operations map[string]Operation
	ordered    []Operation
	Digest     string
}

func NewCatalog() *Catalog {
	c := &Catalog{operations: map[string]Operation{}}
	for _, op := range builtinOperations() {
		op.Version = 1
		op.DocumentType = "PART"
		op.Effect = "MODEL_MUTATION"
		op.Preconditions = []string{"Editor permission", "Exact expected workspace head", "Standalone Part definition; no implicit occurrence activation", "Successful current preview before commit"}
		properties := map[string]any{}
		required := []string{}
		for _, f := range op.Fields {
			p := map[string]any{"type": f.Type, "description": f.Title}
			if f.Type == "string" {
				p["minLength"] = 1
				p["maxLength"] = 4096
			}
			if len(f.Enum) > 0 {
				p["enum"] = f.Enum
			}
			if f.Positive {
				p["exclusiveMinimum"] = 0
				p["maximum"] = 1e6
			}
			properties[f.Name] = p
			if f.Required {
				required = append(required, f.Name)
			}
		}
		op.InputSchema = map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
		op.OutputSchema = map[string]any{"type": "object", "properties": map[string]any{"receipt": map[string]any{"type": "object"}, "output": map[string]any{"type": "object"}}, "required": []string{"receipt"}}
		if c.operations[op.ID].ID != "" {
			panic("duplicate automation operation")
		}
		c.operations[op.ID] = op
		c.ordered = append(c.ordered, op)
	}
	sort.Slice(c.ordered, func(i, j int) bool { return c.ordered[i].ID < c.ordered[j].ID })
	raw, _ := json.Marshal(c.ordered)
	c.Digest = modelcore.ValueDigest(raw)
	return c
}

type OperationSummary struct {
	ID           string `json:"id"`
	Version      int    `json:"version"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DocumentType string `json:"documentType"`
}

func (c *Catalog) Search(query, documentType string, limit int) ([]OperationSummary, error) {
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 20 || len(query) > 256 {
		return nil, fault("INVALID_QUERY", "operation search limit is 1..20; query is at most 256 bytes")
	}
	items := []OperationSummary{}
	terms := strings.Fields(strings.ToLower(query))
	for _, op := range c.ordered {
		if documentType != "" && documentType != op.DocumentType {
			continue
		}
		haystack := strings.ToLower(op.ID + " " + op.Name + " " + op.Description)
		matched := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matched = false
				break
			}
		}
		if matched {
			items = append(items, OperationSummary{op.ID, op.Version, op.Name, op.Description, op.DocumentType})
			if len(items) == limit {
				break
			}
		}
	}
	return items, nil
}
func (c *Catalog) Describe(id string) (Operation, error) {
	op, ok := c.operations[id]
	if !ok {
		return op, fault("OPERATION_UNSUPPORTED", "operation is not registered")
	}
	return op, nil
}

// Validate and compile share the same field descriptors that produce schemas.
// Domain handlers remain responsible for CAD semantics and authoritative checks.
func (c *Catalog) Compile(id string, version int, raw json.RawMessage) (workspace.CommandRequest, error) {
	op, err := c.Describe(id)
	if err != nil {
		return workspace.CommandRequest{}, err
	}
	if version != op.Version {
		return workspace.CommandRequest{}, fault("OPERATION_VERSION_UNSUPPORTED", "operation version changed")
	}
	args, err := DecodeObject(raw)
	if err != nil {
		return workspace.CommandRequest{}, err
	}
	if err = validateFields(op, args, false); err != nil {
		return workspace.CommandRequest{}, err
	}
	return op.compile(args), nil
}
func DecodeObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > 8192 {
		return nil, fault("INVALID_ARGUMENTS", "arguments require an object of at most 8192 bytes")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, fault("INVALID_ARGUMENTS", "arguments must be a JSON object")
	}
	args := map[string]any{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, fault("INVALID_ARGUMENTS", "invalid argument field")
		}
		name, ok := key.(string)
		if !ok {
			return nil, fault("INVALID_ARGUMENTS", "invalid argument field")
		}
		if _, duplicate := args[name]; duplicate {
			return nil, fault("INVALID_ARGUMENTS", "duplicate field: "+name)
		}
		var value any
		if err = d.Decode(&value); err != nil {
			return nil, fault("INVALID_ARGUMENTS", "invalid argument value")
		}
		args[name] = value
	}
	if _, err = d.Token(); err != nil {
		return nil, fault("INVALID_ARGUMENTS", "invalid argument object")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fault("INVALID_ARGUMENTS", "trailing JSON is not allowed")
	}
	return args, nil
}
func validateFields(op Operation, args map[string]any, allowMissing bool) error {
	fields := map[string]Field{}
	for _, f := range op.Fields {
		fields[f.Name] = f
		if _, ok := args[f.Name]; !ok && f.Required && !allowMissing {
			return fault("INVALID_ARGUMENTS", "missing field: "+f.Name)
		}
	}
	for key, value := range args {
		f, ok := fields[key]
		if !ok {
			return fault("INVALID_ARGUMENTS", "unknown field: "+key)
		}
		switch f.Type {
		case "string":
			s, ok := value.(string)
			if !ok || strings.TrimSpace(s) == "" || len(s) > 4096 {
				return fault("INVALID_ARGUMENTS", "invalid string: "+key)
			}
			if len(f.Enum) > 0 {
				found := false
				for _, v := range f.Enum {
					found = found || s == v
				}
				if !found {
					return fault("INVALID_ARGUMENTS", "unsupported value for "+key)
				}
			}
		case "number":
			n, ok := value.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || f.Positive && (n <= 0 || n > 1e6) {
				return fault("INVALID_ARGUMENTS", "invalid finite quantity: "+key)
			}
		default:
			return fmt.Errorf("invalid descriptor type %s", f.Type)
		}
	}
	return nil
}

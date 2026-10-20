package automation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/occccad/occccad/internal/workbenchconfig"
	"github.com/occccad/occccad/internal/workspace"
)

// commandContracts maps every existing public Workspace adapter command to its
// actual JSON fields. A source-coverage test prevents new commands going missing.
// Request IDs, actor, head and previews are always assigned by the host.
type commandContract struct{ command, document, name, fields, required, handoff string }

var commandContracts = []commandContract{
	{"SET_PROPERTIES", "ANY", "修改属性", "properties", "properties", ""},
	{"SET_BODY_COLOR", "PART", "设置实体颜色", "bodyId color", "bodyId color", ""},
	{"ACTIVATE_MECHANISM", "PRODUCT", "激活机构", "mechanismApplication versionId", "mechanismApplication", ""},
	{"EXIT_MECHANISM", "PRODUCT", "退出机构", "mechanismApplication versionId", "mechanismApplication", ""},
	{"APPLY_MOTION_FRAME", "PRODUCT", "应用运动帧", "motionApply versionId", "motionApply", ""},
	{"SAVE_KINEMATICS", "PRODUCT", "保存机构定义", "kinematics sessionId versionId", "kinematics sessionId", ""},
	{"SET_DEFINITION_VISIBILITY", "ANY", "显示或隐藏对象", "axis ownerEntityId targetId targetKind visible", "targetKind targetId visible", ""},
	{"SET_BODY_VISIBILITY", "PART", "显示或隐藏实体", "bodyId visible", "bodyId visible", ""},
	{"SET_OCCURRENCE_VISIBILITY", "PRODUCT", "显示或隐藏实例", "instancePath ownerEntityId targetId targetKind visibilityMode", "instancePath targetKind targetId visibilityMode", ""},
	{"CREATE_BODY", "PART", "创建实体", "name visible", "", ""},
	{"DELETE_BODY", "PART", "删除实体", "bodyId name visible", "bodyId", ""},
	{"RENAME_BODY", "PART", "重命名实体", "bodyId name visible", "bodyId name", ""},
	{"SET_ACTIVE_BODY", "PART", "激活实体", "bodyId name visible", "bodyId", ""},
	{"RENAME_FEATURE", "PART", "重命名特征", "name targetId", "targetId name", ""},
	{"CREATE_PARAMETER", "PART", "创建参数", "name unit value", "value unit", ""},
	{"CREATE_SKETCH", "PART", "创建草图", "bodyId datumPlaneId geometryKey plane targetKind versionId", "bodyId", ""},
	{"EDIT_SKETCH", "PART", "编辑草图几何、尺寸和约束", "operations sketchId", "sketchId operations", ""},
	{"PAD_SKETCH", "PART", "拉伸草图", "bodyId length sketchId", "bodyId sketchId length", ""},
	{"CREATE_MODIFY_FEATURE", "PART", "创建圆角、倒角、拔模或抽壳", "feature parameterExpressions", "feature", ""},
	{"CREATE_BOOLEAN_FEATURE", "PART", "创建布尔特征", "bodyId keepTools operation tools", "bodyId operation tools", ""},
	{"CREATE_SOLID_FEATURE", "PART", "创建拉伸、切除、旋转或放样", "angle axisEntityId bodyId extent feature generator length length2 lengthExpression operation parameterExpressions profileMemberSlot reversed sketchId", "", ""},
	{"SET_FEATURE_SUPPRESSION", "PART", "停用或启用特征", "expectedFeatureDigest suppressed targetId", "targetId suppressed", ""},
	{"CREATE_PATTERN", "PART", "创建阵列或镜像", "feature parameterExpressions", "feature", ""},
	{"EDIT_FEATURE", "PART", "编辑特征", "expectedFeatureDigest feature length lengthExpression parameterExpressions targetId unit", "targetId", ""},
	{"CREATE_DATUM_PLANE", "PART", "创建基准面", "datumAngleExpression datumDefinition datumDistanceExpression name normal origin targetId uDirection", "origin normal uDirection", ""},
	{"EDIT_DATUM_PLANE", "PART", "编辑基准面", "datumAngleExpression datumDefinition datumDistanceExpression name normal origin targetId uDirection", "targetId origin normal uDirection", ""},
	{"CREATE_DATUM_AXIS", "PART", "创建基准轴", "datumAngleExpression datumDefinition datumDistanceExpression direction name origin targetId", "origin direction", ""},
	{"EDIT_DATUM_AXIS", "PART", "编辑基准轴", "datumAngleExpression datumDefinition datumDistanceExpression direction name origin targetId", "targetId origin direction", ""},
	{"REPAIR_IMPORT_NAMING", "PART", "修复导入拓扑命名", "targetId", "targetId", ""},
	{"IMPORT_EXCHANGE", "PART", "导入几何", "bodyId fileName geometryKey sourceFormat", "geometryKey sourceFormat", "File import must use the application upload/import flow; geometryKey and ImportSource are verified application-owned artifacts."},
	{"SET_PARAMETER_VALUE", "PART", "修改参数值", "parameterId unit value", "parameterId value unit", ""},
	{"EDIT_PARAMETER", "PART", "编辑参数", "expression name parameterId unit value", "parameterId", ""},
	{"DELETE_PARAMETER", "PART", "删除参数", "parameterId", "parameterId", ""},
	{"SET_PARAMETER_EXPRESSION", "PART", "修改参数表达式", "expression parameterId", "parameterId expression", ""},
	{"RENAME_PARAMETER", "PART", "重命名参数", "name parameterId", "parameterId name", ""},
	{"SET_PARAMETER_EXTERNAL", "PART", "关联外部参数", "parameterId publicationId referenceMode sourceDocumentId versionId", "parameterId sourceDocumentId publicationId referenceMode", ""},
	{"CREATE_PUBLICATION", "PART", "创建发布", "axis compatibilityVersion geometryKey name publicationId publicationType semanticPurpose targetId targetKind versionId", "targetKind targetId", ""},
	{"REDIRECT_PUBLICATION", "PART", "重定向发布", "axis compatibilityVersion geometryKey name publicationId publicationType semanticPurpose targetId targetKind versionId", "publicationId targetKind targetId", ""},
	{"EDIT_PUBLICATION", "PART", "编辑发布", "compatibilityVersion name publicationId semanticPurpose", "publicationId", ""},
	{"DELETE_PUBLICATION", "PART", "删除发布", "publicationId", "publicationId", ""},
	{"CREATE_CONTEXT_INPUT", "PART", "创建上下文输入", "compatibilityVersion name publicationType required semanticPurpose targetId targetKind", "targetKind targetId", ""},
	{"EDIT_CONTEXT_INPUT", "PART", "编辑上下文输入", "contextInputId name required", "contextInputId", ""},
	{"DELETE_CONTEXT_INPUT", "PART", "删除上下文输入", "contextInputId", "contextInputId", ""},
	{"CREATE_CONTEXT_REFERENCE", "PART", "创建上下文引用", "compatibilityVersion contextVariantId instancePath name owningInstancePath parameterId publicationId publicationType referenceMode rootProductDocumentId sketchId sourceDocumentId versionId", "", ""},
	{"DETACH_CONTEXT_REFERENCE", "PART", "断开上下文引用", "contextReferenceId", "contextReferenceId", ""},
	{"ISOLATE_CONTEXT_REFERENCE", "PART", "隔离上下文引用", "contextReferenceId", "contextReferenceId", ""},
	{"IMPORT_PRODUCT_INSTANCES", "PRODUCT", "导入产品实例", "", "", "Use ui.product.insert after uploading an exchange file; importInstances is an application-owned verified graph."},
	{"INSERT_INSTANCES", "PRODUCT", "插入多个实例", "instanceId patternAxis patternCount patternReversed patternSpacing referenceMode referencedDocumentIds", "", ""},
	{"INSERT_INSTANCE", "PRODUCT", "插入实例", "referencedDocumentId translation", "referencedDocumentId", ""},
	{"RENAME_INSTANCE", "PRODUCT", "重命名实例", "instanceId name", "instanceId name", ""},
	{"REPLACE_INSTANCE", "PRODUCT", "替换实例", "instanceId referencedDocumentId", "instanceId referencedDocumentId", ""},
	{"CREATE_PRODUCT_PUBLICATION", "PRODUCT", "创建产品发布", "instanceId instancePath name publicationId semanticPurpose targetId targetKind", "instancePath publicationId", ""},
	{"DELETE_PRODUCT_PUBLICATION", "PRODUCT", "删除产品发布", "publicationId", "publicationId", ""},
	{"REDIRECT_PRODUCT_PUBLICATION", "PRODUCT", "重定向产品发布", "instancePath publicationId targetId", "publicationId instancePath targetId", ""},
	{"EDIT_PRODUCT_PUBLICATION", "PRODUCT", "编辑产品发布", "name publicationId semanticPurpose", "publicationId", ""},
	{"CREATE_CONTEXT_BINDING", "PRODUCT", "创建上下文绑定", "contextInputId contextVariantId name owningInstancePath publicationId referenceMode sourceDocumentId sourceInstancePath versionId", "contextInputId owningInstancePath sourceInstancePath publicationId", ""},
	{"DELETE_CONTEXT_BINDING", "PRODUCT", "删除上下文绑定", "contextBindingId", "contextBindingId", ""},
	{"MOVE_INSTANCE", "PRODUCT", "移动实例", "instanceId interactionTarget rotation sessionId translation", "instanceId translation rotation", ""},
	{"SET_ASSEMBLY_CONSTRAINT_STATE", "PRODUCT", "修改装配约束状态", "constraintIds constraintMode suppressed", "constraintIds", ""},
	{"ADD_ASSEMBLY_CONSTRAINT", "PRODUCT", "添加装配约束", "angleAxis angleReferenceDirection angleRelation constraintFamily constraintKind constraintMode constraintSubtype contactBranch contactKind contactSide directionRelation distanceRelation firstAssemblyRef fixMode fixedPose groupMembers name offsetExpression offsetKey quantityExpression quantityKey reverseAngleAxis secondAssemblyRef value", "", ""},
	{"EDIT_ASSEMBLY_CONSTRAINT", "PRODUCT", "编辑装配约束", "angleAxis angleReferenceDirection angleRelation constraintFamily constraintMode constraintSubtype contactBranch contactKind contactSide directionRelation distanceRelation firstAssemblyRef fixMode fixedPose groupMembers name offsetExpression offsetKey quantityExpression quantityKey reverseAngleAxis secondAssemblyRef targetId value", "targetId", ""},
	{"SET_REFERENCE_MODE", "PRODUCT", "修改引用模式", "instanceId referenceMode", "instanceId referenceMode", ""},
	{"UPDATE_REFERENCES", "PRODUCT", "更新引用", "updatePlanDigest", "", "Use ui.tree.refresh to obtain and accept the authoritative update plan digest; do not fabricate it."},
	{"DELETE_NODE", "ANY", "删除对象", "ownerEntityId targetId targetKind", "targetId targetKind", ""},
	{"DELETE_NODES", "ANY", "删除多个对象", "targets", "targets", ""},
}

func commandOperations() []Operation {
	result := []Operation{}
	request := reflect.TypeOf(workspace.CommandRequest{})
	for _, contract := range commandContracts {
		contract := contract
		op := Operation{ID: "command." + strings.ToLower(contract.command), Name: contract.name, DocumentType: contract.document, CommandType: contract.command, Handoff: contract.handoff,
			Description: contract.command + ". Existing versioned domain command; lengths mm, angles degrees unless an explicit unit is provided. Nested types: cad_describe_type. " + contract.handoff}
		required := map[string]bool{}
		for _, name := range strings.Fields(contract.required) {
			required[name] = true
		}
		for _, name := range strings.Fields(contract.fields) {
			field, ok := jsonField(request, name)
			if !ok {
				panic("unknown command field " + name)
			}
			schema := typeSchema(field.Type)
			kind, _ := schema["type"].(string)
			if kind == "" {
				kind = "object"
			}
			op.Fields = append(op.Fields, Field{Name: name, Title: name, Type: kind, Required: required[name], Schema: schema})
		}
		if contract.command == "CREATE_MODIFY_FEATURE" || contract.command == "EDIT_FEATURE" {
			op.Fields = append(op.Fields, Field{Name: "selectionSet", MaxLength: 16384, Title: "Revision-bound topology set from cad_query_topology; resolves to formal PersistentSelections", Type: "string"})
		}
		op.validate = func(args map[string]any) error {
			for name, value := range args {
				if name == "selectionSet" {
					continue
				}
				field, _ := jsonField(request, name)
				if err := validateTyped(field.Type, value, 0); err != nil {
					return fault("INVALID_ARGUMENTS", name+": "+err.Error())
				}
			}
			return nil
		}
		op.compile = func(args map[string]any) workspace.CommandRequest {
			values := map[string]any{}
			for name, value := range args {
				if name != "selectionSet" {
					values[name] = value
				}
			}
			var command workspace.CommandRequest
			_ = json.Unmarshal(marshal(values), &command)
			command.Type = contract.command
			return command
		}
		result = append(result, op)
	}
	// UI contracts are projected from the same catalog used by CommandRegistry.
	catalog, err := workbenchconfig.Read()
	if err != nil {
		panic(err)
	}
	for _, command := range catalog.Commands {
		result = append(result, Operation{ID: "ui." + command.ID, Name: command.Name, DocumentType: "ANY", Effect: "UI_COMMAND", UICommand: command.ID,
			Description: command.HelpText + " Open/activate the existing workbench command. Runtime visibility and availability apply. Interactive tools/forms require user input; dispatch is not a modeling completion. Use a domain operation when full arguments are known.",
			Fields:      []Field{{Name: "targetId", Title: "Optional stable entity ID already queried in this document; selects the object before opening the tool", Type: "string"}}})
	}
	for _, kind := range []string{"FILLET", "CHAMFER"} {
		kind := kind
		name := "圆角"
		quantity := "半径"
		if kind == "CHAMFER" {
			name = "倒角"
			quantity = "距离"
		}
		result = append(result, Operation{ID: "part." + strings.ToLower(kind), Name: name, CommandType: "CREATE_MODIFY_FEATURE",
			Description: "Create a 3D " + kind + " on the revision-bound edge selectionSet returned by cad_query_topology. Select all edges without transmitting their individual references. Dimension in mm.",
			Fields:      []Field{stringField("bodyId", "目标实体"), Field{Name: "selectionSet", Title: "精确拓扑选择集", Type: "string", Required: true, MaxLength: 16384}, numberField("size", quantity), stringField("unit", "长度单位", "mm")},
			compile: func(args map[string]any) workspace.CommandRequest {
				return workspace.CommandRequest{Type: "CREATE_MODIFY_FEATURE", Feature: &workspace.Feature{Type: kind, BodyID: textArg(args, "bodyId"), Length: numericArg(args, "size")}}
			}})
	}
	return result
}

func bindingKind(name string) string {
	return map[string]string{"sketchId": "SKETCH", "parameterId": "PARAMETER", "bodyId": "BODY", "instanceId": "INSTANCE", "publicationId": "PUBLICATION", "contextInputId": "CONTEXT_INPUT", "contextReferenceId": "CONTEXT_REFERENCE", "contextBindingId": "CONTEXT_BINDING", "datumPlaneId": "DATUM_PLANE", "targetId": "ENTITY"}[name]
}
func copySchema(schema map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range schema {
		result[k] = v
	}
	return result
}
func dereference(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
func jsonField(t reflect.Type, name string) (reflect.StructField, bool) {
	t = dereference(t)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		if key == "" && !f.Anonymous {
			key = f.Name
		}
		if f.PkgPath == "" && key == name && key != "-" {
			return f, true
		}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath == "" && f.Anonymous && strings.Split(f.Tag.Get("json"), ",")[0] == "" && dereference(f.Type).Kind() == reflect.Struct {
			if field, ok := jsonField(f.Type, name); ok {
				return field, true
			}
		}
	}
	return reflect.StructField{}, false
}
func jsonFields(t reflect.Type) map[string]reflect.StructField {
	t = dereference(t)
	fields := map[string]reflect.StructField{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		if key == "-" {
			continue
		}
		if f.Anonymous && key == "" && dereference(f.Type).Kind() == reflect.Struct {
			for name, child := range jsonFields(f.Type) {
				fields[name] = child
			}
			continue
		}
		if key == "" {
			key = f.Name
		}
		fields[key] = f
	}
	return fields
}
func typeSchema(t reflect.Type) map[string]any {
	t = dereference(t)
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string", "maxLength": 4096}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Float64, reflect.Float32:
		return map[string]any{"type": "number"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Array, reflect.Slice:
		if t == reflect.TypeOf(json.RawMessage{}) {
			return map[string]any{"type": "object", "additionalProperties": true}
		}
		schema := map[string]any{"type": "array", "items": typeSchema(t.Elem()), "maxItems": 4096}
		if t.Kind() == reflect.Array {
			schema["minItems"] = t.Len()
			schema["maxItems"] = t.Len()
		}
		return schema
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": typeSchema(t.Elem())}
	case reflect.Struct:
		return map[string]any{"$ref": "cad://types/" + t.String()}
	default:
		return map[string]any{"type": "object", "additionalProperties": true}
	}
}

var knownTypes = sync.OnceValue(func() map[string]reflect.Type {
	types := map[string]reflect.Type{}
	var visit func(reflect.Type)
	visit = func(t reflect.Type) {
		t = dereference(t)
		switch t.Kind() {
		case reflect.Array, reflect.Slice, reflect.Map:
			visit(t.Elem())
		case reflect.Struct:
			if _, ok := types[t.String()]; ok {
				return
			}
			types[t.String()] = t
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath == "" && strings.Split(f.Tag.Get("json"), ",")[0] != "-" {
					visit(f.Type)
				}
			}
		}
	}
	visit(reflect.TypeOf(workspace.CommandRequest{}))
	return types
})

func DescribeType(name string) (map[string]any, error) {
	t, ok := knownTypes()[name]
	if !ok {
		return nil, fault("TYPE_UNSUPPORTED", "unknown command contract type")
	}
	properties := map[string]any{}
	for key, f := range jsonFields(t) {
		properties[key] = typeSchema(f.Type)
		if values := contractEnums[t.String()+"."+key]; len(values) > 0 {
			properties[key] = map[string]any{"type": "string", "enum": values}
		}
		if (name == "workspace.FeaturePattern" || name == "workspace.SketchPattern") && len(contractEnums["workspace.PatternDefinition."+key]) > 0 {
			properties[key] = map[string]any{"type": "string", "enum": contractEnums["workspace.PatternDefinition."+key]}
		}
	}
	description := "Existing command DTO. CAD required fields, enum combinations, units and geometry are validated by authoritative domain handlers. Load referenced nested types only when needed."
	if name == "workspace.SketchEntity" {
		description += " Coordinates and radii in mm; rotation/startAngle/endAngle in radians. A new entity needs a fresh unique id, kind and role PROFILE or CONSTRUCTION. Supply the geometry fields required by its kind, e.g. a CIRCLE needs center and radius."
	}
	if name == "workspace.Feature" {
		description += " Lengths including fillet radius/chamfer distance/thickness use length in mm; angle in degrees. For CREATE_* the application assigns id/name/order; editing references the existing stable targetId."
	}
	if name == "workspace.SketchOperation" {
		description += " ADD_ENTITY requires entity; ADD_CONSTRAINT requires constraint. Existing entity/constraint references must be queried by ownerId from the sketch. Never use display names or list indices."
	}
	required := []string{}
	switch name {
	case "workspace.SketchEntity":
		required = []string{"id", "kind", "role"}
	case "workspace.SketchOperation":
		required = []string{"type"}
	case "workspace.Feature":
		required = []string{"type"}
	}
	return map[string]any{"id": name, "type": "object", "properties": properties, "required": required, "additionalProperties": false, "description": description}, nil
}

// These discriminator values mirror the existing handlers, not CAD recipes.
// All nested geometry/constraint fields still come from their actual DTO types.
var contractEnums = map[string][]string{
	"workspace.PatternDefinition.kind":         {"LINEAR", "CIRCULAR", "MIRROR"},
	"workspace.PatternDefinition.distribution": {"FIXED_STEP", "TOTAL_SPAN", "FULL_CIRCLE"},
	"workspace.SketchOperation.type":           {"ADD_ENTITY", "ADD_CONSTRAINT", "ADD_RECTANGLE", "CREATE_POLYGON", "CREATE_PATTERN", "EDIT_PATTERN", "DELETE_PATTERN", "DETACH_PATTERN", "OFFSET_ENTITIES", "FILLET_ENTITIES", "CHAMFER_ENTITIES", "APPLY_DRAG_RESULT", "DELETE_ENTITIES", "COPY_ENTITIES", "TRANSFORM_ENTITIES", "MIRROR_ENTITIES", "SPLIT_ENTITY", "REPLACE_CURVE_INTERVALS", "ADD_EXTERNAL_GEOMETRY", "RECONNECT_EXTERNAL_GEOMETRY", "DETACH_EXTERNAL_GEOMETRY", "UPDATE_ENTITY_ROLE", "EDIT_SPLINE_POINT", "SET_SPLINE_CLOSED", "CONVERT_SPLINE_TO_CONTROL", "UPDATE_ENTITY_POINT", "UPDATE_ENTITY_SUPPRESSION", "UPDATE_CONSTRAINT_SUPPRESSION", "DELETE_CONSTRAINT", "UPDATE_CONSTRAINT", "UPDATE_CONSTRAINT_VALUE"},
	"workspace.SketchEntity.role":              {"PROFILE", "CONSTRUCTION"},
	"workspace.SketchEntity.kind":              {"POINT", "LINE", "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC", "SPLINE"},
	"workspace.Feature.type":                   {"SKETCH", "PAD", "LINEAR_EXTRUDE", "REVOLVE", "FILLET", "CHAMFER", "DRAFT", "SHELL", "LOFT", "BOOLEAN", "SOLID_PATTERN", "SKETCH_PATTERN"},
	"workspace.Feature.operation":              {"NEW_BODY", "ADD", "REMOVE", "INTERSECT"},
}

func validateTyped(t reflect.Type, value any, depth int) error {
	if depth > 32 {
		return fmt.Errorf("nested input exceeds depth budget")
	}
	if value == nil {
		if t.Kind() == reflect.Pointer || t.Kind() == reflect.Map || t.Kind() == reflect.Slice {
			return nil
		}
		return fmt.Errorf("null is not allowed")
	}
	t = dereference(t)
	if t == reflect.TypeOf(json.RawMessage{}) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("expected object")
		}
		for key, v := range obj {
			f, ok := jsonField(t, key)
			if !ok {
				return fmt.Errorf("unknown field %s", key)
			}
			if err := validateTyped(f.Type, v, depth+1); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case reflect.Slice, reflect.Array:
		values, ok := value.([]any)
		if !ok || len(values) > 4096 || t.Kind() == reflect.Array && len(values) != t.Len() {
			return fmt.Errorf("invalid array length")
		}
		for _, v := range values {
			if err := validateTyped(t.Elem(), v, depth+1); err != nil {
				return err
			}
		}
	case reflect.Map:
		values, ok := value.(map[string]any)
		if !ok || len(values) > 4096 {
			return fmt.Errorf("invalid map")
		}
		for _, v := range values {
			if err := validateTyped(t.Elem(), v, depth+1); err != nil {
				return err
			}
		}
	default:
		target := reflect.New(t).Interface()
		if err := json.Unmarshal(marshal(value), target); err != nil {
			return fmt.Errorf("invalid %s", t.Kind())
		}
	}
	return nil
}

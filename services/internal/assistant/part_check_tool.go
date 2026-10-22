package assistant

// This exposes existing explicit CAD checks, without compiling natural language
// or persisting a mandatory checklist on the Run. Results remain tool evidence.
func partCheckTool() Tool {
	fields := map[string]any{}
	for _, name := range []string{"id", "bodyId", "featureId", "sketchId", "referenceFeatureId", "referenceSketchId", "parameterId", "expectedType", "plane", "unit"} {
		fields[name] = stringSchema()
	}
	fields["kind"] = map[string]any{"type": "string", "description": "An explicit existing Part check kind: ENTITY_EXISTS, FEATURE_VALID, BODY_GEOMETRY_VALID, PARAMETER_VALUE, RECTANGLE_DIMENSIONS, EXTRUSION_LENGTH, ALL_EDGES_FILLET, SELECTED_EDGES_FILLET, HOLE_RIM_FILLET, CENTERED_THROUGH_HOLE, THROUGH_HOLE, NO_FEATURE_TYPE, PROTECTED_SELECTION."}
	for _, name := range []string{"width", "height", "length", "radius", "value", "tolerance"} {
		fields[name] = map[string]any{"type": "number"}
	}
	fields["radiusPolicy"] = map[string]any{"type": "string", "enum": []string{"DELEGATED"}}
	fields["expectedCount"] = map[string]any{"type": "integer", "minimum": 0}
	fields["selectionSet"] = map[string]any{"type": "string", "maxLength": 32768}
	fields["selection"] = map[string]any{"type": "object", "description": "An exact existing FeatureSelection for PROTECTED_SELECTION; read its contract with cad_describe_type."}
	return Tool{"cad_verify_part", "Optional read-only checks at an explicit Revision. Supply exact feature/body/selection and expected dimensions in mm; a DELEGATED radius checks the actual positive radius without inventing a user numeric requirement. FAILED/NOT_VERIFIED describes only these supplied checks and never blocks ending the request. Result omits the full model; use cad_query_model for details.", object(map[string]any{
		"revisionId":       stringSchema(),
		"createdEntityIds": map[string]any{"type": "array", "items": stringSchema(), "maxItems": 4096},
		"checks":           map[string]any{"type": "array", "minItems": 1, "maxItems": 64, "items": object(fields, "id", "kind")},
	}, "revisionId", "checks")}
}

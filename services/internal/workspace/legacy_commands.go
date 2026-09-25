package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

func (service *Service) adaptLegacyCommand(ctx context.Context, documentID, documentType string, modelJSON json.RawMessage, request CommandRequest) (string, any, error) {
	switch request.Type {
	case "CREATE_SKETCH":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		if strings.EqualFold(request.TargetKind, "FACE") {
			sourceVersionID := strings.TrimSpace(request.VersionID)
			if sourceVersionID == "" {
				if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&sourceVersionID); err != nil {
					return "", nil, err
				}
			}
			selection, err := service.BindPersistentSelection(ctx, documentID, BindPersistentSelectionRequest{
				SourceVersionID: sourceVersionID, GeometryKey: request.GeometryKey, Kind: "FACE", LocalID: request.TopologyID,
			})
			if err != nil {
				return "", nil, err
			}
			if selection.CreationEvidence.GeometryType != "PLANE" {
				return "", nil, fmt.Errorf("%w: SUPPORT_TYPE_MISMATCH: selected face is not planar", ErrValidation)
			}
			// Persistent evidence identifies the face; its immutable source
			// manifest may predate orientation fixes. Read the exact B-Rep
			// frame for sketch placement, just as normal view does.
			properties, err := service.getTopologyElementPropertiesFromArtifact(ctx, request.GeometryKey, "FACE", request.TopologyID)
			if err != nil {
				return "", nil, err
			}
			normal, ok := properties.Properties["normal"].([3]float64)
			if !ok {
				return "", nil, fmt.Errorf("%w: SUPPORT_FRAME_INVALID", ErrValidation)
			}
			xDirection, ok := stableSupportX(normal, [3]float64{})
			if !ok {
				return "", nil, fmt.Errorf("%w: SUPPORT_FRAME_INVALID", ErrValidation)
			}
			support := SketchSupport{Type: "PLANAR_FACE", Plane: "CUSTOM", PersistentSelection: &selection,
				SourceVersionID: sourceVersionID, Origin: selection.CreationEvidence.Origin, XDirection: xDirection,
				Normal: normal, OrientationRule: sketchSupportOrientationRule, Status: "CONNECTED"}
			return typeCreateSketch, createFeaturePayload{Feature: Feature{ID: newID("sketch"), Type: "SKETCH",
				Name: numberedFeatureName(model.Features, "SKETCH", "Sketch"), Plane: "CUSTOM",
				Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: support, Entities: []SketchEntity{},
					Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}}}}, nil
		}
		plane := strings.ToUpper(request.Plane)
		datumID := strings.TrimSpace(request.DatumPlaneID)
		if datumID == "" {
			if plane != "XY" && plane != "XZ" && plane != "YZ" {
				return "", nil, fmt.Errorf("%w: select a datum plane", ErrValidation)
			}
			datumID = "datum-" + strings.ToLower(plane)
		} else {
			found := false
			for _, datum := range model.DatumPlanes {
				if datum.ID == datumID {
					plane, found = datum.Plane, true
					break
				}
			}
			if !found {
				return "", nil, fmt.Errorf("%w: selected datum plane does not exist", ErrValidation)
			}
		}
		return typeCreateSketch, createFeaturePayload{Feature: Feature{ID: newID("sketch"), Type: "SKETCH", Name: numberedFeatureName(model.Features, "SKETCH", "Sketch"), Plane: plane, Sketch: &SketchFeature{SchemaVersion: SketchSchemaVersion, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: datumID, Plane: plane, Status: "CONNECTED"}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}}}}, nil
	case "EDIT_SKETCH":
		if documentType != "PART" {
			break
		}
		operations := make([]SketchOperation, 0, len(request.Operations)+12)
		for index, operation := range request.Operations {
			if operation.Type == "ADD_EXTERNAL_GEOMETRY" || operation.Type == "RECONNECT_EXTERNAL_GEOMETRY" {
				sourceVersionID := strings.TrimSpace(operation.SourceVersionID)
				if sourceVersionID == "" {
					if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&sourceVersionID); err != nil {
						return "", nil, err
					}
				}
				kind := strings.ToUpper(strings.TrimSpace(operation.TopologyKind))
				if kind != "EDGE" && kind != "VERTEX" {
					return "", nil, fmt.Errorf("%w: external geometry source must be an edge or vertex", ErrValidation)
				}
				selection, err := service.BindPersistentSelection(ctx, documentID, BindPersistentSelectionRequest{
					SourceVersionID: sourceVersionID, GeometryKey: operation.GeometryKey, Kind: kind, LocalID: operation.TopologyID,
				})
				if err != nil {
					return "", nil, err
				}
				externalID := strings.TrimSpace(operation.ExternalID)
				if externalID == "" {
					externalID = newID("external")
				}
				operation.ExternalID = externalID
				operation.ExternalGeometry = &SketchExternalGeometry{ID: externalID, ProjectionKind: "ORTHOGONAL",
					PersistentSelection: selection, SourceVersionID: sourceVersionID, Status: "PENDING"}
				operations = append(operations, operation)
				continue
			}
			if operation.Type != "ADD_RECTANGLE" {
				operations = append(operations, operation)
				continue
			}
			if operation.First == nil || operation.Second == nil {
				return "", nil, fmt.Errorf("%w: rectangle requires two points", ErrValidation)
			}
			expanded, err := rectangleMacro(request.RequestID+fmt.Sprintf("/%d", index), *operation.First, *operation.Second)
			if err != nil {
				return "", nil, err
			}
			operations = append(operations, expanded...)
			for _, snapped := range []struct {
				suffix    string
				point     SketchPoint2
				reference *SketchGeometryRef
			}{
				{"first", *operation.First, operation.FirstReference},
				{"second", *operation.Second, operation.SecondReference},
			} {
				if snapped.reference == nil {
					continue
				}
				found := false
				for _, candidate := range expanded {
					if candidate.Entity == nil || candidate.Entity.Kind != "LINE" {
						continue
					}
					for _, endpoint := range []struct {
						sub   string
						point *SketchPoint2
					}{{"START", candidate.Entity.Start}, {"END", candidate.Entity.End}} {
						if endpoint.point != nil && endpoint.point.X == snapped.point.X && endpoint.point.Y == snapped.point.Y {
							constraint := SketchConstraint{ID: macroID(request.RequestID+fmt.Sprintf("/%d", index), "snap-"+snapped.suffix), Kind: "COINCIDENT",
								References: []SketchGeometryRef{{Target: "ENTITY", EntityID: candidate.Entity.ID, SubElement: endpoint.sub}, *snapped.reference}}
							operations = append(operations, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &constraint})
							found = true
							break
						}
					}
					if found {
						break
					}
				}
			}
		}
		return typeEditSketch, editSketchPayload{SketchID: request.SketchID, Operations: operations}, nil
	case "PAD_SKETCH":
		if documentType != "PART" {
			break
		}
		if !positiveFinite(request.Length) {
			return "", nil, fmt.Errorf("%w: pad length must be a positive finite value", ErrValidation)
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		found := false
		for _, feature := range model.Features {
			if feature.ID == request.SketchID && strings.Contains(strings.ToUpper(feature.Type), "SKETCH") {
				found = true
			}
		}
		if !found {
			return "", nil, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		return typeCreatePad, createFeaturePayload{Feature: Feature{ID: commandEntityID("extrude", request.RequestID), Type: "PAD", Name: numberedFeatureName(model.Features, "PAD", "Extrude"), Profile: request.SketchID, Length: request.Length, Operation: "ADD"}}, nil
	case "CREATE_SOLID_FEATURE":
		if documentType != "PART" {
			break
		}
		generator := strings.ToUpper(strings.TrimSpace(request.Generator))
		if generator != "LINEAR_EXTRUDE" && generator != "REVOLVE" {
			return "", nil, fmt.Errorf("%w: generator must be LINEAR_EXTRUDE or REVOLVE", ErrValidation)
		}
		operation := strings.ToUpper(strings.TrimSpace(request.Operation))
		if operation != "NEW_BODY" && operation != "ADD" && operation != "REMOVE" && operation != "INTERSECT" {
			return "", nil, fmt.Errorf("%w: invalid BodyOperation %s", ErrValidation, operation)
		}
		if generator == "LINEAR_EXTRUDE" && strings.TrimSpace(request.LengthExpression) == "" && !positiveFinite(request.Length) {
			return "", nil, fmt.Errorf("%w: extrude length must be a positive finite value", ErrValidation)
		}
		if generator == "REVOLVE" && (!positiveFinite(request.Angle) || request.Angle > 360) {
			return "", nil, fmt.Errorf("%w: revolve angle must be in (0, 360] degrees", ErrValidation)
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		normalizePartModel(&model)
		var sketch *Feature
		for index := range model.Features {
			if model.Features[index].ID == request.SketchID && model.Features[index].Sketch != nil {
				sketch = &model.Features[index]
				break
			}
		}
		if sketch == nil {
			return "", nil, fmt.Errorf("%w: selected sketch does not exist", ErrValidation)
		}
		if generator == "REVOLVE" {
			if _, _, err := resolveRevolveAxis(model, *sketch, request.AxisEntityID); err != nil {
				return "", nil, err
			}
		}
		prefix, label := "extrude", "Extrude"
		if generator == "REVOLVE" {
			prefix, label = "revolve", "Revolve"
		}
		feature := Feature{ID: commandEntityID(prefix, request.RequestID), Type: generator,
			Name: numberedFeatureName(model.Features, generator, label), Profile: request.SketchID,
			Length: request.Length, Angle: request.Angle, Operation: operation,
			AxisEntityID: request.AxisEntityID, Reversed: request.Reversed}
		parameterSources := map[string]modelcore.ValueSource{}
		if generator == "LINEAR_EXTRUDE" && strings.TrimSpace(request.LengthExpression) != "" {
			names := map[string]modelcore.ParameterBinding{}
			for _, parameter := range model.Parameters {
				names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
			}
			expression, compileErr := modelcore.CompileExpression(request.LengthExpression, names, modelcore.LengthDimension)
			if compileErr != nil {
				return "", nil, fmt.Errorf("%w: %w", ErrValidation, compileErr)
			}
			parameterSources["length"] = modelcore.ValueSource{Expression: &expression}
			// The generated parameter is replaced by the expression in the typed handler;
			// keep the transient feature value structurally valid until parameter evaluation.
			feature.Length = 1
		}
		return typeCreateSolidFeature, createFeaturePayload{Feature: feature, ParameterSources: parameterSources}, nil
	case "EDIT_FEATURE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		var feature *Feature
		for index := range model.Features {
			if model.Features[index].ID == request.TargetID {
				feature = &model.Features[index]
				break
			}
		}
		if feature == nil {
			return "", nil, fmt.Errorf("%w: selected feature does not exist", ErrValidation)
		}
		var source modelcore.ValueSource
		if strings.TrimSpace(request.LengthExpression) != "" {
			names := map[string]modelcore.ParameterBinding{}
			for _, parameter := range model.Parameters {
				names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
			}
			expression, err := modelcore.CompileExpression(request.LengthExpression, names, modelcore.LengthDimension)
			if err != nil {
				return "", nil, fmt.Errorf("%w: %w", ErrValidation, err)
			}
			source = modelcore.ValueSource{Expression: &expression}
		} else {
			unit := request.Unit
			if unit == "" {
				unit = "mm"
			} // REST feature lengths use model millimeters.
			quantity, err := modelcore.NewQuantity(request.Length, unit)
			if err != nil {
				return "", nil, err
			}
			source = modelcore.ValueSource{Literal: &quantity}
		}
		return typeEditFeature, editFeaturePayload{FeatureID: feature.ID, ExpectedFeatureDigest: request.ExpectedFeatureDigest,
			LinearExtrude: linearExtrudeEdit{Source: source, Operation: feature.Operation, Reversed: feature.Reversed, Profile: feature.Profile}}, nil
	case "CREATE_DATUM_PLANE":
		if documentType != "PART" {
			break
		}
		finiteVector := func(value [3]float64) bool { return finite(value[0]) && finite(value[1]) && finite(value[2]) }
		length := func(value [3]float64) float64 {
			return math.Sqrt(value[0]*value[0] + value[1]*value[1] + value[2]*value[2])
		}
		if !finiteVector(request.Origin) || !finiteVector(request.Normal) || !finiteVector(request.UDirection) ||
			length(request.Normal) < 1e-9 || length(request.UDirection) < 1e-9 {
			return "", nil, fmt.Errorf("%w: datum plane requires finite origin, normal and U direction", ErrValidation)
		}
		dot := request.Normal[0]*request.UDirection[0] + request.Normal[1]*request.UDirection[1] + request.Normal[2]*request.UDirection[2]
		if math.Abs(dot/(length(request.Normal)*length(request.UDirection))) > 1e-8 {
			return "", nil, fmt.Errorf("%w: datum plane U direction must be perpendicular to its normal", ErrValidation)
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = "Plane"
		}
		plane := DatumPlane{ID: commandEntityID("plane", request.RequestID), Name: name, Plane: "CUSTOM",
			Origin: request.Origin, Normal: request.Normal, UDirection: request.UDirection, Size: 180}
		return typeCreateDatumPlane, createDatumPlanePayload{Plane: plane}, nil
	case "CREATE_DATUM_AXIS":
		if documentType != "PART" {
			break
		}
		magnitude := math.Sqrt(request.Direction[0]*request.Direction[0] + request.Direction[1]*request.Direction[1] + request.Direction[2]*request.Direction[2])
		if !finite(request.Origin[0]) || !finite(request.Origin[1]) || !finite(request.Origin[2]) || !finite(magnitude) || magnitude < 1e-9 {
			return "", nil, fmt.Errorf("%w: datum axis requires finite origin and non-zero direction", ErrValidation)
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			name = "Axis"
		}
		axis := DatumAxis{ID: commandEntityID("axis", request.RequestID), Name: name, Origin: request.Origin,
			Direction: [3]float64{request.Direction[0] / magnitude, request.Direction[1] / magnitude, request.Direction[2] / magnitude}}
		return typeCreateDatumAxis, createDatumAxisPayload{Axis: axis}, nil
	case "REPAIR_IMPORT_NAMING":
		var model PartModel
		if documentType != "PART" {
			break
		}
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		for _, feature := range model.Features {
			if feature.ID == request.TargetID && feature.Type == "IMPORT_BODY" {
				if feature.ImportDefinitionID != "" {
					return "", nil, fmt.Errorf("%w: import already has frozen naming", ErrValidation)
				}
				return typeRepairImportNaming, createFeaturePayload{Feature: feature}, nil
			}
		}
		return "", nil, fmt.Errorf("%w: imported feature not found", ErrValidation)
	case "IMPORT_EXCHANGE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
		if strings.TrimSpace(request.GeometryKey) == "" || len(model.Features) != 0 {
			return "", nil, fmt.Errorf("%w: STEP import requires an empty Part and geometry key", ErrValidation)
		}
		name := strings.TrimSpace(request.FileName)
		if name == "" {
			name = "Imported STEP"
		}
		format := strings.ToUpper(strings.TrimSpace(request.SourceFormat))
		if format != "STEP" && format != "BREP" {
			return "", nil, fmt.Errorf("%w: exchange format must be STEP or BREP", ErrValidation)
		}
		return typeImportExchange, createFeaturePayload{Feature: Feature{ID: commandEntityID("import", documentID+"/"+request.RequestID), Type: "IMPORT_BODY", Name: "Import " + name, GeometryKey: request.GeometryKey, FileName: name, SourceFormat: format}}, nil
	case "SET_PARAMETER_VALUE":
		if documentType != "PART" {
			break
		}
		quantity, err := modelcore.NewQuantity(request.Value, request.Unit)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", ErrValidation, err)
		}
		return typeSetParameterLiteral, parameterSourcePayload{ParameterID: request.ParameterID, Source: modelcore.ValueSource{Literal: &quantity}}, nil
	case "SET_PARAMETER_EXPRESSION":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		names := map[string]modelcore.ParameterBinding{}
		var expected *modelcore.ParameterDefinition
		for index := range model.Parameters {
			parameter := &model.Parameters[index]
			names[parameter.Key] = modelcore.ParameterBinding{ParameterID: parameter.ParameterID, Dimension: parameter.Dimension}
			if parameter.ParameterID == request.ParameterID {
				expected = parameter
			}
		}
		if expected == nil {
			return "", nil, fmt.Errorf("%w: parameter does not exist", ErrValidation)
		}
		expression, err := modelcore.CompileExpression(request.Expression, names, expected.Dimension)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %w", ErrValidation, err)
		}
		return typeSetParameterExpression, parameterSourcePayload{ParameterID: request.ParameterID, Source: modelcore.ValueSource{Expression: &expression}}, nil
	case "RENAME_PARAMETER":
		if documentType != "PART" {
			break
		}
		return typeRenameParameter, renameParameterPayload{ParameterID: request.ParameterID, Key: request.Name}, nil
	case "SET_PARAMETER_EXTERNAL":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		var parameter *modelcore.ParameterDefinition
		for index := range model.Parameters {
			if model.Parameters[index].ParameterID == request.ParameterID {
				parameter = &model.Parameters[index]
				break
			}
		}
		if parameter == nil {
			return "", nil, fmt.Errorf("%w: parameter does not exist", ErrValidation)
		}
		mode := strings.ToUpper(strings.TrimSpace(request.ReferenceMode))
		if mode == "" {
			mode = "FOLLOW_HEAD"
		}
		reference, err := service.resolveExternalParameterRefWithMode(ctx, documentID, request.SourceDocumentID,
			request.VersionID, request.PublicationID, mode, *parameter, model)
		if err != nil {
			return "", nil, err
		}
		return typeSetParameterExternal, parameterSourcePayload{ParameterID: request.ParameterID,
			Source: modelcore.ValueSource{External: &reference}}, nil
	case "CREATE_PUBLICATION", "REDIRECT_PUBLICATION":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		normalizePartModel(&model)
		publicationID := strings.TrimSpace(request.PublicationID)
		if request.Type == "CREATE_PUBLICATION" {
			publicationID = commandEntityID("publication", request.RequestID)
		} else if publicationID == "" {
			return "", nil, fmt.Errorf("%w: publication id is required", ErrValidation)
		}
		publication, err := service.publicationFromRequest(ctx, documentID, model, request, publicationID)
		if err != nil {
			return "", nil, err
		}
		command := typeCreatePublication
		if request.Type == "REDIRECT_PUBLICATION" {
			command = typeRedirectPublication
		}
		return command, publicationPayload{Publication: publication}, nil
	case "EDIT_PUBLICATION":
		if documentType != "PART" {
			break
		}
		return typeEditPublication, publicationEditPayload{PublicationID: request.PublicationID, Name: request.Name,
			SemanticPurpose: request.SemanticPurpose, CompatibilityVersion: request.CompatibilityVersion}, nil
	case "DELETE_PUBLICATION":
		if documentType != "PART" {
			break
		}
		return typeDeletePublication, publicationDeletePayload{PublicationID: request.PublicationID}, nil
	case "CREATE_CONTEXT_INPUT":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		input, err := contextInputFromRequest(model, request)
		if err != nil {
			return "", nil, err
		}
		return typeCreateContextInput, contextInputPayload{Input: input}, nil
	case "EDIT_CONTEXT_INPUT":
		if documentType != "PART" {
			break
		}
		return typeEditContextInput, contextInputEditPayload{ContextInputID: request.ContextInputID, Name: request.Name, Required: request.Required}, nil
	case "DELETE_CONTEXT_INPUT":
		if documentType != "PART" {
			break
		}
		return typeDeleteContextInput, contextInputDeletePayload{ContextInputID: request.ContextInputID}, nil
	case "CREATE_CONTEXT_REFERENCE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		mode := strings.ToUpper(strings.TrimSpace(request.ReferenceMode))
		if mode == "" {
			mode = "FOLLOW_HEAD"
		}
		reference := ContextReference{ID: newID("context-reference"), Name: strings.TrimSpace(request.Name),
			OwningWorkspace: "main", SourceDocumentID: request.SourceDocumentID, ReferenceMode: mode,
			RootProductDocumentID: request.RootProductDocumentID, SourceInstancePath: request.InstancePath,
			OwningInstancePath: request.OwningInstancePath,
			ContextVariantID:   request.ContextVariantID, LocalTargetID: request.ParameterID,
			Publication: PublicationRef{PublicationID: request.PublicationID, ExpectedType: strings.ToUpper(request.PublicationType),
				CompatibilityVersion: request.CompatibilityVersion}}
		if reference.Name == "" {
			reference.Name = "Context Reference"
		}
		if reference.LocalTargetID == "" {
			reference.LocalTargetID = request.SketchID
		}
		resolved, err := service.resolveContextReference(ctx, documentID, reference, model, request.VersionID)
		if err != nil {
			return "", nil, err
		}
		if resolved.Publication.ExpectedType == "PARAMETER" {
			var parameter *modelcore.ParameterDefinition
			for index := range model.Parameters {
				if model.Parameters[index].ParameterID == request.ParameterID {
					parameter = &model.Parameters[index]
					break
				}
			}
			if parameter == nil {
				return "", nil, fmt.Errorf("%w: local parameter does not exist", ErrValidation)
			}
			external, err := service.resolveExternalParameterRefWithMode(ctx, documentID, resolved.SourceDocumentID,
				resolved.ResolvedRevisionID, request.PublicationID, mode, *parameter, model)
			if err != nil {
				return "", nil, err
			}
			parameter.Source = modelcore.ValueSource{External: &external}
		}
		if err := materializeContextReference(&model, &resolved, request.ParameterID); err != nil {
			return "", nil, err
		}
		return typeCreateContextReference, contextReferencePayload{Model: model, Reference: resolved}, nil
	case "DETACH_CONTEXT_REFERENCE", "ISOLATE_CONTEXT_REFERENCE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		for index := range model.ContextReferences {
			if model.ContextReferences[index].ID != request.ContextReferenceID {
				continue
			}
			before := model.ContextReferences[index]
			model.ContextReferences[index].ReferenceMode = "ISOLATED"
			if err := isolateContextCurve(&model, before); err != nil {
				return "", nil, err
			}
			if target := model.ContextReferences[index].LocalTargetID; target != "" {
				for parameterIndex := range model.Parameters {
					parameter := &model.Parameters[parameterIndex]
					if parameter.ParameterID == target && parameter.Source.External != nil {
						value := parameter.Source.External.ResolvedValue
						parameter.Source = modelcore.ValueSource{Literal: &value}
					}
				}
			}
			return typeDetachContextReference, contextReferencePayload{Model: model,
				Reference: model.ContextReferences[index], Before: &before}, nil
		}
		return "", nil, fmt.Errorf("%w: context reference does not exist", ErrValidation)
	case "INSERT_INSTANCES":
		if documentType != "PRODUCT" {
			break
		}
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return "", nil, err
		}
		selected := request.InstanceID != ""
		if selected == (len(request.ReferencedDocumentIDs) > 0) {
			return "", nil, fmt.Errorf("%w: select documents or one source instance", ErrValidation)
		}
		result := insertInstancesPayload{Instances: make([]ProductInstance, 0)}
		if selected {
			var source *ProductInstance
			for i := range product.Instances {
				if product.Instances[i].ID == request.InstanceID {
					source = &product.Instances[i]
					break
				}
			}
			if source == nil {
				return "", nil, fmt.Errorf("%w: source instance does not exist", ErrValidation)
			}
			axis := strings.ToUpper(request.PatternAxis)
			if request.PatternCount < 2 || request.PatternCount > 128 || !(axis == "X" || axis == "Y" || axis == "Z") || math.IsNaN(request.PatternSpacing) || math.IsInf(request.PatternSpacing, 0) || request.PatternSpacing <= 0 || request.PatternSpacing > 1e6 {
				return "", nil, fmt.Errorf("%w: pattern requires X/Y/Z, total count 2..128 and positive finite spacing", ErrValidation)
			}
			coordinate := map[string]int{"X": 0, "Y": 1, "Z": 2}[axis]
			baseName := source.Name
			if service.database != nil {
				if err := service.database.QueryRow(ctx, `SELECT name FROM occccad.documents WHERE id=$1`, source.ReferencedDocumentID).Scan(&baseName); err != nil {
					return "", nil, err
				}
			} else {
				baseName = instanceReferenceName(source.Name)
			}
			sign := 1.0
			if request.PatternReversed {
				sign = -1
			}
			for i := 1; i < request.PatternCount; i++ {
				clone := *source
				clone.ID = newID("instance")
				clone.Name = nextInstanceName(product, baseName)
				clone.Translation[coordinate] += sign * float64(i) * request.PatternSpacing
				clone.ResolvedVersionID = ""
				clone.HeadChanged = false
				product.Instances = append(product.Instances, clone)
				result.Instances = append(result.Instances, clone)
			}
		} else {
			if len(request.ReferencedDocumentIDs) > 128 {
				return "", nil, fmt.Errorf("%w: at most 128 documents can be inserted", ErrValidation)
			}
			mode := request.ReferenceMode
			if mode == "" {
				mode = "FOLLOW_HEAD"
			}
			if mode != "FOLLOW_HEAD" && mode != "PINNED" {
				return "", nil, fmt.Errorf("%w: invalid reference mode", ErrValidation)
			}
			seen := map[string]bool{}
			for _, id := range request.ReferencedDocumentIDs {
				if seen[id] {
					return "", nil, fmt.Errorf("%w: duplicate selected document", ErrValidation)
				}
				seen[id] = true
				var referenceID, versionID, name string
				err := service.database.QueryRow(ctx, `SELECT id::text,head_version_id::text,name FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&referenceID, &versionID, &name)
				if errors.Is(err, pgx.ErrNoRows) {
					return "", nil, fmt.Errorf("%w: referenced document does not exist", ErrValidation)
				}
				if err != nil {
					return "", nil, err
				}
				if referenceID == documentID {
					return "", nil, fmt.Errorf("%w: a Product cannot contain itself", ErrValidation)
				}
				var cycle bool
				err = service.database.QueryRow(ctx, `WITH RECURSIVE graph(document_id) AS (SELECT $1::uuid UNION SELECT pi.referenced_document_id FROM graph g JOIN occccad.documents d ON d.id=g.document_id JOIN occccad.product_instances pi ON pi.product_version_id=d.head_version_id) SELECT EXISTS(SELECT 1 FROM graph WHERE document_id=$2::uuid)`, referenceID, documentID).Scan(&cycle)
				if err != nil {
					return "", nil, err
				}
				if cycle {
					return "", nil, fmt.Errorf("%w: Product reference would create a cycle", ErrValidation)
				}
				instance := ProductInstance{ID: newID("instance"), Name: nextInstanceName(product, name), ReferencedDocumentID: referenceID, ReferencedVersionID: versionID, Rotation: [4]float64{0, 0, 0, 1}, ReferenceMode: mode}
				product.Instances = append(product.Instances, instance)
				result.Instances = append(result.Instances, instance)
			}
		}
		return typeInsertInstances, result, nil
	case "INSERT_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		var referenceID, versionID, name string
		if err := service.database.QueryRow(ctx, `SELECT id::text,head_version_id::text,name FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, request.ReferencedDocumentID).Scan(&referenceID, &versionID, &name); errors.Is(err, pgx.ErrNoRows) {
			return "", nil, fmt.Errorf("%w: referenced document does not exist", ErrValidation)
		} else if err != nil {
			return "", nil, err
		}
		if referenceID == documentID {
			return "", nil, fmt.Errorf("%w: a Product cannot contain itself", ErrValidation)
		}
		var cycle bool
		if err := service.database.QueryRow(ctx, `WITH RECURSIVE graph(document_id) AS (SELECT $1::uuid UNION SELECT pi.referenced_document_id FROM graph g JOIN occccad.documents d ON d.id=g.document_id JOIN occccad.product_instances pi ON pi.product_version_id=d.head_version_id) SELECT EXISTS(SELECT 1 FROM graph WHERE document_id=$2::uuid)`, referenceID, documentID).Scan(&cycle); err != nil {
			return "", nil, err
		}
		if cycle {
			return "", nil, fmt.Errorf("%w: Product reference would create a cycle", ErrValidation)
		}
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return "", nil, err
		}
		instanceName := nextInstanceName(product, name)
		return typeInsertInstance, insertInstancePayload{Instance: ProductInstance{ID: newID("instance"), Name: instanceName, ReferencedDocumentID: referenceID, ReferencedVersionID: versionID, Translation: request.Translation, Rotation: [4]float64{0, 0, 0, 1}, ReferenceMode: "FOLLOW_HEAD"}}, nil
	case "RENAME_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		return typeRenameInstance, renameInstancePayload{InstanceID: request.InstanceID, Name: request.Name}, nil
	case "REPLACE_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		var replacementID, replacementVersion string
		if err := service.database.QueryRow(ctx, `SELECT id::text,head_version_id::text FROM occccad.documents
			WHERE id=$1 AND deleted_at IS NULL`, request.ReferencedDocumentID).Scan(&replacementID, &replacementVersion); errors.Is(err, pgx.ErrNoRows) {
			return "", nil, fmt.Errorf("%w: replacement document does not exist", ErrValidation)
		} else if err != nil {
			return "", nil, err
		}
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return "", nil, err
		}
		found := false
		for index := range product.Instances {
			if product.Instances[index].ID != request.InstanceID {
				continue
			}
			found = true
			product.Instances[index].ReferencedDocumentID = replacementID
			product.Instances[index].ReferencedVersionID = replacementVersion
			product.Instances[index].ResolvedVersionID = replacementVersion
		}
		if !found {
			return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
		if err := service.updateProductReferences(ctx, &product); err != nil {
			return "", nil, err
		}
		return typeReplaceInstance, replaceInstancePayload{InstanceID: request.InstanceID,
			ReferencedDocumentID: replacementID, ReferencedVersionID: replacementVersion, Model: product}, nil
	case "CREATE_PRODUCT_PUBLICATION":
		if documentType != "PRODUCT" {
			break
		}
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return "", nil, err
		}
		path := InstancePath{RootDocumentID: documentID}
		if request.InstancePath != nil {
			path = *request.InstancePath
		} else {
			var rootRevisionID string
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&rootRevisionID); err != nil {
				return "", nil, err
			}
			for _, instance := range product.Instances {
				if instance.ID == request.InstanceID {
					path = appendInstancePath(path, InstancePathSegment{OwnerDocumentID: documentID,
						OwnerVersionID: rootRevisionID,
						InstanceID:     instance.ID, InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
						ResolvedVersionID: instance.ReferencedVersionID})
					break
				}
			}
		}
		if path.RootDocumentID != documentID {
			return "", nil, fmt.Errorf("%w: Product Publication path belongs to another root Product", ErrValidation)
		}
		child, canonicalPath, err := service.publicationAtRelativePath(ctx, product, path, request.PublicationID)
		if err != nil {
			return "", nil, err
		}
		if child.Resolution.Status != "CONNECTED" {
			return "", nil, fmt.Errorf("%w: child Publication is broken", ErrValidation)
		}
		name := strings.TrimSpace(request.Name)
		if name == "" {
			existing := make([]string, 0, len(product.Publications))
			for _, item := range product.Publications {
				existing = append(existing, item.Name)
			}
			name = child.Name
			if err := ensureUniqueScopedName(name, existing, ""); err != nil {
				name = nextScopedName(defaultPublicationBase(child.Target.Kind, child.Type), existing)
			}
		}
		publication := productPublicationFromPath(canonicalPath, child, commandEntityID("product-publication", request.RequestID), name, request.SemanticPurpose)
		return typeCreateProductPublication, productPublicationPayload{Publication: publication}, nil
	case "DELETE_PRODUCT_PUBLICATION":
		if documentType != "PRODUCT" {
			break
		}
		return typeDeleteProductPublication, publicationDeletePayload{PublicationID: request.PublicationID}, nil
	case "EDIT_PRODUCT_PUBLICATION":
		if documentType != "PRODUCT" {
			break
		}
		return typeEditProductPublication, productPublicationEditPayload{PublicationID: request.PublicationID,
			Name: request.Name, SemanticPurpose: request.SemanticPurpose}, nil
	case "CREATE_CONTEXT_BINDING":
		if documentType != "PRODUCT" {
			break
		}
		var rootRevisionID string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND document_type='PRODUCT' AND deleted_at IS NULL`, documentID).Scan(&rootRevisionID); err != nil {
			return "", nil, err
		}
		binding, err := service.contextBindingFromRequest(ctx, documentID, rootRevisionID, request)
		if err != nil {
			return "", nil, err
		}
		return typeCreateContextBinding, contextBindingPayload{Binding: binding}, nil
	case "DELETE_CONTEXT_BINDING":
		if documentType != "PRODUCT" {
			break
		}
		return typeDeleteContextBinding, contextBindingDeletePayload{ContextBindingID: request.ContextBindingID}, nil
	case "MOVE_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		return typeMoveInstance, moveInstancePayload{request.InstanceID, request.Translation, request.Rotation}, nil
	case "SET_ASSEMBLY_CONSTRAINT_STATE":
		if documentType != "PRODUCT" {
			break
		}
		return typeSetAssemblyConstraintState, assemblyConstraintStatePayload{ConstraintIDs: request.ConstraintIDs, Suppressed: request.Suppressed, Mode: request.ConstraintMode}, nil
	case "ADD_ASSEMBLY_CONSTRAINT":
		if documentType != "PRODUCT" {
			break
		}
		kind := strings.ToUpper(strings.TrimSpace(request.ConstraintKind))
		if kind != "FIX" && kind != "RIGID" && kind != "COINCIDENT" && kind != "CONCENTRIC" && kind != "ANGLE" && kind != "DISTANCE" {
			return "", nil, fmt.Errorf("%w: unsupported assembly constraint kind", ErrValidation)
		}
		if request.FirstAssemblyRef == nil {
			return "", nil, fmt.Errorf("%w: first assembly reference is required", ErrValidation)
		}
		if kind != "FIX" && request.SecondAssemblyRef == nil {
			return "", nil, fmt.Errorf("%w: second assembly reference is required", ErrValidation)
		}
		if (kind == "FIX" || kind == "RIGID") && request.FirstAssemblyRef.Kind != "BODY" {
			return "", nil, fmt.Errorf("%w: %s operates on an instance body, not transient geometry", ErrValidation, kind)
		}
		if kind == "RIGID" && request.SecondAssemblyRef.Kind != "BODY" {
			return "", nil, fmt.Errorf("%w: RIGID operates on two instance bodies", ErrValidation)
		}
		if kind != "FIX" && request.SecondAssemblyRef != nil && request.FirstAssemblyRef.InstanceID == request.SecondAssemblyRef.InstanceID {
			return "", nil, fmt.Errorf("%w: a binary assembly constraint requires two different instances", ErrValidation)
		}
		if kind == "ANGLE" && (request.Value < 0 || request.Value > 2*math.Pi) {
			return "", nil, fmt.Errorf("%w: assembly angle must be in [0, 2pi]", ErrValidation)
		}
		var product ProductModel
		if err := json.Unmarshal(modelJSON, &product); err != nil {
			return "", nil, err
		}
		if err := service.bindAssemblyPick(ctx, &product, request.FirstAssemblyRef); err != nil {
			return "", nil, err
		}
		if request.SecondAssemblyRef != nil {
			if err := service.bindAssemblyPick(ctx, &product, request.SecondAssemblyRef); err != nil {
				return "", nil, err
			}
		}
		if request.AngleAxis != nil {
			if err := service.bindAssemblyPick(ctx, &product, request.AngleAxis); err != nil {
				return "", nil, err
			}
		}
		constraint := AssemblyConstraint{ID: commandEntityID("assembly-constraint", request.RequestID), ConnectionID: commandEntityID("assembly-connection", request.RequestID), Kind: kind, FixMode: request.FixMode, AngleRelation: request.AngleRelation, Mode: "DRIVING", First: *request.FirstAssemblyRef,
			Second: request.SecondAssemblyRef, Value: request.Value, DirectionRelation: strings.ToUpper(request.DirectionRelation), DistanceRelation: strings.ToUpper(request.DistanceRelation),
			AngleAxis: request.AngleAxis, ReverseAngleAxis: request.ReverseAngleAxis != nil && *request.ReverseAngleAxis, AngleReferenceDirection: request.AngleReferenceDirection, EvaluationStatus: modelcore.AssemblyConstraintVerified}
		if constraint.DirectionRelation == "" {
			constraint.DirectionRelation = "UNORIENTED"
		}
		if constraint.DistanceRelation == "" {
			constraint.DistanceRelation = "UNSIGNED"
		}
		if kind == "FIX" || kind == "RIGID" {
			var product ProductModel
			if err := json.Unmarshal(modelJSON, &product); err != nil {
				return "", nil, err
			}
			var first, second *ProductInstance
			for index := range product.Instances {
				instance := &product.Instances[index]
				if instance.ID == constraint.First.InstanceID {
					first = instance
				}
				if constraint.Second != nil && instance.ID == constraint.Second.InstanceID {
					second = instance
				}
			}
			if first == nil || (kind == "RIGID" && second == nil) {
				return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
			}
			firstPose := InstancePose{Translation: first.Translation, Rotation: normalizedInstanceRotation(first.Rotation)}
			if kind == "FIX" {
				constraint.FixedPose = &firstPose
			} else {
				secondPose := InstancePose{Translation: second.Translation, Rotation: normalizedInstanceRotation(second.Rotation)}
				relative := inverseRelativePose(firstPose, secondPose)
				constraint.FixedPose = &relative
			}
		}
		return typeAddAssemblyConstraint, addAssemblyConstraintPayload{Constraint: constraint}, nil
	case "EDIT_ASSEMBLY_CONSTRAINT":
		if documentType != "PRODUCT" {
			break
		}
		id := strings.TrimSpace(request.TargetID)
		if id == "" {
			return "", nil, fmt.Errorf("%w: constraint identity is required", ErrValidation)
		}
		if request.FirstAssemblyRef != nil && request.SecondAssemblyRef != nil &&
			request.FirstAssemblyRef.InstanceID == request.SecondAssemblyRef.InstanceID {
			return "", nil, fmt.Errorf("%w: a binary assembly constraint requires two different instances", ErrValidation)
		}
		var bindingModel ProductModel
		if err := json.Unmarshal(modelJSON, &bindingModel); err != nil {
			return "", nil, err
		}
		if request.FirstAssemblyRef != nil {
			if err := service.bindAssemblyPick(ctx, &bindingModel, request.FirstAssemblyRef); err != nil {
				return "", nil, err
			}
		}
		if request.SecondAssemblyRef != nil {
			if err := service.bindAssemblyPick(ctx, &bindingModel, request.SecondAssemblyRef); err != nil {
				return "", nil, err
			}
		}
		if request.AngleAxis != nil {
			if err := service.bindAssemblyPick(ctx, &bindingModel, request.AngleAxis); err != nil {
				return "", nil, err
			}
		}
		var editedFixedPose *InstancePose
		var product ProductModel
		if json.Unmarshal(modelJSON, &product) == nil {
			for _, existing := range product.Constraints {
				if existing.ID != id || (existing.Kind != "FIX" && existing.Kind != "RIGID") {
					continue
				}
				firstRef := existing.First
				if request.FirstAssemblyRef != nil {
					firstRef = *request.FirstAssemblyRef
				}
				secondRef := existing.Second
				if request.SecondAssemblyRef != nil {
					secondRef = request.SecondAssemblyRef
				}
				var first, second *ProductInstance
				for index := range product.Instances {
					instance := &product.Instances[index]
					if instance.ID == firstRef.InstanceID {
						first = instance
					}
					if secondRef != nil && instance.ID == secondRef.InstanceID {
						second = instance
					}
				}
				if first == nil || (existing.Kind == "RIGID" && second == nil) {
					return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
				}
				firstPose := InstancePose{Translation: first.Translation, Rotation: normalizedInstanceRotation(first.Rotation)}
				if existing.Kind == "FIX" {
					editedFixedPose = &firstPose
				} else {
					secondPose := InstancePose{Translation: second.Translation, Rotation: normalizedInstanceRotation(second.Rotation)}
					relative := inverseRelativePose(firstPose, secondPose)
					editedFixedPose = &relative
				}
				break
			}
		}
		if request.FixedPose != nil {
			editedFixedPose = request.FixedPose
		}
		return typeEditAssemblyConstraint, editAssemblyConstraintPayload{FixMode: request.FixMode, AngleRelation: request.AngleRelation, ConstraintID: id, Value: request.Value,
			DirectionRelation: strings.ToUpper(request.DirectionRelation), DistanceRelation: strings.ToUpper(request.DistanceRelation),
			First: request.FirstAssemblyRef, Second: request.SecondAssemblyRef, AngleAxis: request.AngleAxis, ReverseAngleAxis: request.ReverseAngleAxis, AngleReferenceDirection: request.AngleReferenceDirection,
			FixedPose: editedFixedPose}, nil
	case "SET_REFERENCE_MODE":
		if documentType != "PRODUCT" {
			break
		}
		mode := strings.ToUpper(request.ReferenceMode)
		if mode != "FOLLOW_HEAD" && mode != "FOLLOW_WORKSPACE_WITH_ACCEPT" && mode != "PINNED" {
			return "", nil, fmt.Errorf("%w: reference mode must be FOLLOW_HEAD or PINNED", ErrValidation)
		}
		var model ProductModel
		_ = json.Unmarshal(modelJSON, &model)
		var referenced, acceptedVersion string
		for _, instance := range model.Instances {
			if instance.ID == request.InstanceID {
				referenced = instance.ReferencedDocumentID
				acceptedVersion = instance.ReferencedVersionID
			}
		}
		if referenced == "" {
			return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
		pinned := ""
		if mode == "PINNED" {
			// Pin the revision currently accepted by this Product snapshot. Using
			// the source Head here would make "pin" silently accept a pending
			// update before freezing it.
			pinned = acceptedVersion
			if pinned == "" {
				if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, referenced).Scan(&pinned); err != nil {
					return "", nil, err
				}
			}
		}
		return typeSetReferenceMode, referenceModePayload{request.InstanceID, mode, pinned}, nil
	case "UPDATE_REFERENCES":
		if documentType == "PART" {
			var model PartModel
			if err := json.Unmarshal(modelJSON, &model); err != nil {
				return "", nil, err
			}
			if err := service.updatePartReferences(ctx, documentID, &model); err != nil {
				return "", nil, err
			}
			return typeUpdatePartReferences, updatePartReferencesPayload{Model: model}, nil
		}
		if documentType != "PRODUCT" {
			break
		}
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
		if len(model.ContextBindings) > 0 && request.UpdatePlanDigest == "" {
			return "", nil, fmt.Errorf("%w: PRODUCT_UPDATE_PLAN_DIGEST_REQUIRED", ErrValidation)
		}
		if request.UpdatePlanDigest != "" {
			plan, err := service.GetProductUpdatePlan(ctx, documentID)
			if err != nil {
				return "", nil, err
			}
			if plan.Digest != request.UpdatePlanDigest {
				return "", nil, fmt.Errorf("%w: PRODUCT_UPDATE_PLAN_STALE", ErrValidation)
			}
			if !plan.CanAccept {
				return "", nil, fmt.Errorf("%w: PRODUCT_UPDATE_BLOCKED", ErrValidation)
			}
		}
		if err := service.updateProductReferences(ctx, &model); err != nil {
			return "", nil, err
		}
		return typeUpdateReferences, updateReferencesPayload{Model: model}, nil
	case "DELETE_NODE":
		kind := strings.ToUpper(strings.TrimSpace(request.TargetKind))
		id := strings.TrimSpace(request.TargetID)
		owner := strings.TrimSpace(request.OwnerEntityID)
		if id == "" {
			return "", nil, fmt.Errorf("%w: delete target identity is required", ErrValidation)
		}
		if documentType == "PART" {
			if kind != "FEATURE" && kind != "SKETCH_ENTITY" && kind != "SKETCH_CONSTRAINT" {
				break
			}
			if (kind == "SKETCH_ENTITY" || kind == "SKETCH_CONSTRAINT") && owner == "" {
				return "", nil, fmt.Errorf("%w: sketch child deletion requires its owning sketch", ErrValidation)
			}
			return typeDeletePartNode, deleteNodePayload{TargetKind: kind, TargetID: id, OwnerEntityID: owner}, nil
		}
		if documentType == "PRODUCT" && (kind == "INSTANCE" || kind == "ASSEMBLY_CONSTRAINT") {
			return typeDeleteProductNode, deleteNodePayload{TargetKind: kind, TargetID: id}, nil
		}
	case "DELETE_NODES":
		if len(request.Targets) == 0 {
			return "", nil, fmt.Errorf("%w: delete targets are required", ErrValidation)
		}
		targets := make([]deleteNodePayload, 0, len(request.Targets))
		seen := map[string]bool{}
		valid := true
		for _, requested := range request.Targets {
			kind := strings.ToUpper(strings.TrimSpace(requested.TargetKind))
			id := strings.TrimSpace(requested.TargetID)
			owner := strings.TrimSpace(requested.OwnerEntityID)
			if id == "" {
				return "", nil, fmt.Errorf("%w: delete target identity is required", ErrValidation)
			}
			if documentType == "PART" {
				if kind != "FEATURE" && kind != "SKETCH_ENTITY" && kind != "SKETCH_CONSTRAINT" {
					valid = false
					break
				}
				if (kind == "SKETCH_ENTITY" || kind == "SKETCH_CONSTRAINT") && owner == "" {
					return "", nil, fmt.Errorf("%w: sketch child deletion requires its owning sketch", ErrValidation)
				}
			} else if documentType != "PRODUCT" || (kind != "INSTANCE" && kind != "ASSEMBLY_CONSTRAINT") {
				valid = false
				break
			}
			key := kind + "\x00" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, deleteNodePayload{TargetKind: kind, TargetID: id, OwnerEntityID: owner})
		}
		if !valid || len(targets) == 0 {
			break
		}
		if documentType == "PART" {
			return typeDeletePartNodes, deleteNodesPayload{Targets: targets}, nil
		}
		return typeDeleteProductNodes, deleteNodesPayload{Targets: targets}, nil
	}
	return "", nil, fmt.Errorf("%w: command %s is not valid for a %s", ErrValidation, request.Type, documentType)
}

func macroID(seed, slot string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("occccad/sketch/"+seed+"/"+slot)).String()
}

func commandEntityID(prefix, requestID string) string {
	return prefix + "-" + uuid.NewSHA1(uuid.NameSpaceURL, []byte("occccad/command/"+requestID+"/"+prefix)).String()
}

func rectangleMacro(seed string, first, second SketchPoint2) ([]SketchOperation, error) {
	if !finite(first.X) || !finite(first.Y) || !finite(second.X) || !finite(second.Y) || first.X == second.X || first.Y == second.Y {
		return nil, fmt.Errorf("%w: rectangle corners must define positive area", ErrValidation)
	}
	minX, maxX, minY, maxY := first.X, second.X, first.Y, second.Y
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	lineIDs := []string{macroID(seed, "line-bottom"), macroID(seed, "line-right"), macroID(seed, "line-top"), macroID(seed, "line-left")}
	lines := []SketchEntity{
		{ID: lineIDs[0], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{minX, minY}, End: &SketchPoint2{maxX, minY}},
		{ID: lineIDs[1], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{maxX, minY}, End: &SketchPoint2{maxX, maxY}},
		{ID: lineIDs[2], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{maxX, maxY}, End: &SketchPoint2{minX, maxY}},
		{ID: lineIDs[3], Kind: "LINE", Role: "PROFILE", Start: &SketchPoint2{minX, maxY}, End: &SketchPoint2{minX, minY}},
	}
	operations := make([]SketchOperation, 0, 12)
	for index := range lines {
		entity := lines[index]
		operations = append(operations, SketchOperation{Type: "ADD_ENTITY", Entity: &entity})
	}
	endpoint := func(id, sub string) SketchGeometryRef {
		return SketchGeometryRef{Target: "ENTITY", EntityID: id, SubElement: sub}
	}
	axis := func(target string) SketchGeometryRef {
		return SketchGeometryRef{Target: target, SubElement: "DIRECTION"}
	}
	constraints := []SketchConstraint{
		{ID: macroID(seed, "coincident-0"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[0], "END"), endpoint(lineIDs[1], "START")}},
		{ID: macroID(seed, "coincident-1"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[1], "END"), endpoint(lineIDs[2], "START")}},
		{ID: macroID(seed, "coincident-2"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[2], "END"), endpoint(lineIDs[3], "START")}},
		{ID: macroID(seed, "coincident-3"), Kind: "COINCIDENT", References: []SketchGeometryRef{endpoint(lineIDs[3], "END"), endpoint(lineIDs[0], "START")}},
		{ID: macroID(seed, "parallel-x-0"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[0], "DIRECTION"), axis("SKETCH_X_AXIS")}},
		{ID: macroID(seed, "parallel-y-0"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[1], "DIRECTION"), axis("SKETCH_Y_AXIS")}},
		{ID: macroID(seed, "parallel-x-1"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[2], "DIRECTION"), axis("SKETCH_X_AXIS")}},
		{ID: macroID(seed, "parallel-y-1"), Kind: "PARALLEL", References: []SketchGeometryRef{endpoint(lineIDs[3], "DIRECTION"), axis("SKETCH_Y_AXIS")}},
	}
	for index := range constraints {
		constraint := constraints[index]
		operations = append(operations, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &constraint})
	}
	return operations, nil
}

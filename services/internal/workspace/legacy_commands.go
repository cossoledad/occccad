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
		return typeCreateSketch, createFeaturePayload{Feature: Feature{ID: newID("sketch"), Type: "SKETCH", Name: numberedFeatureName(model.Features, "SKETCH", "Sketch"), Plane: plane, Sketch: &SketchFeature{SchemaVersion: 1, Support: SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: datumID, Plane: plane}, Entities: []SketchEntity{}, Constraints: []SketchConstraint{}, Solve: SketchSolveState{Status: "EMPTY", DefinitionStatus: "EMPTY", DegreesOfFreedom: 0}}}}, nil
	case "EDIT_SKETCH":
		if documentType != "PART" {
			break
		}
		operations := make([]SketchOperation, 0, len(request.Operations)+12)
		for index, operation := range request.Operations {
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
		if generator == "LINEAR_EXTRUDE" && !positiveFinite(request.Length) {
			return "", nil, fmt.Errorf("%w: extrude length must be a positive finite value", ErrValidation)
		}
		if generator == "REVOLVE" && (!positiveFinite(request.Angle) || request.Angle > 360) {
			return "", nil, fmt.Errorf("%w: revolve angle must be in (0, 360] degrees", ErrValidation)
		}
		var model PartModel
		_ = json.Unmarshal(modelJSON, &model)
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
		return typeCreateSolidFeature, createFeaturePayload{Feature: feature}, nil
	case "EDIT_FEATURE":
		if documentType != "PART" {
			break
		}
		var model PartModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
		}
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
		quantity, err := modelcore.NewQuantity(request.Length, request.Unit)
		if err != nil {
			return "", nil, err
		}
		return typeEditFeature, editFeaturePayload{FeatureID: feature.ID, ExpectedFeatureDigest: request.ExpectedFeatureDigest,
			LinearExtrude: linearExtrudeEdit{Length: quantity, Operation: feature.Operation, Reversed: feature.Reversed, Profile: feature.Profile}}, nil
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
		return typeImportExchange, createFeaturePayload{Feature: Feature{ID: newID("import"), Type: "IMPORT_BODY", Name: "Import " + name, GeometryKey: request.GeometryKey, FileName: name, SourceFormat: format}}, nil
	case "SET_PARAMETER_VALUE":
		if documentType != "PART" {
			break
		}
		quantity, err := modelcore.NewQuantity(request.Value, request.Unit)
		if err != nil {
			return "", nil, err
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
			return "", nil, err
		}
		return typeSetParameterExpression, parameterSourcePayload{ParameterID: request.ParameterID, Source: modelcore.ValueSource{Expression: &expression}}, nil
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
	case "MOVE_INSTANCE":
		if documentType != "PRODUCT" {
			break
		}
		return typeMoveInstance, moveInstancePayload{request.InstanceID, request.Translation, request.Rotation}, nil
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
		constraint := AssemblyConstraint{ID: newID("assembly-constraint"), ConnectionID: newID("assembly-connection"), Kind: kind, Mode: "DRIVING", First: *request.FirstAssemblyRef,
			Second: request.SecondAssemblyRef, Value: request.Value, DirectionRelation: strings.ToUpper(request.DirectionRelation), DistanceRelation: strings.ToUpper(request.DistanceRelation),
			AngleReferenceDirection: request.AngleReferenceDirection, EvaluationStatus: modelcore.AssemblyConstraintVerified}
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
		return typeEditAssemblyConstraint, editAssemblyConstraintPayload{ConstraintID: id, Value: request.Value,
			DirectionRelation: strings.ToUpper(request.DirectionRelation), DistanceRelation: strings.ToUpper(request.DistanceRelation),
			First: request.FirstAssemblyRef, Second: request.SecondAssemblyRef, AngleReferenceDirection: request.AngleReferenceDirection,
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
		var referenced string
		for _, instance := range model.Instances {
			if instance.ID == request.InstanceID {
				referenced = instance.ReferencedDocumentID
			}
		}
		if referenced == "" {
			return "", nil, fmt.Errorf("%w: selected instance does not exist", ErrValidation)
		}
		pinned := ""
		if mode == "PINNED" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, referenced).Scan(&pinned); err != nil {
				return "", nil, err
			}
		}
		return typeSetReferenceMode, referenceModePayload{request.InstanceID, mode, pinned}, nil
	case "UPDATE_REFERENCES":
		if documentType != "PRODUCT" {
			break
		}
		var model ProductModel
		if err := json.Unmarshal(modelJSON, &model); err != nil {
			return "", nil, err
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

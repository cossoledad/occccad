package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
)

func inverseRelativePose(first, second InstancePose) InstancePose {
	q := normalizedInstanceRotation(second.Rotation)
	n := math.Sqrt(q[0]*q[0] + q[1]*q[1] + q[2]*q[2] + q[3]*q[3])
	q = [4]float64{q[0] / n, q[1] / n, q[2] / n, q[3] / n}
	qi := [4]float64{-q[0], -q[1], -q[2], q[3]}
	mul := func(a, b [4]float64) [4]float64 {
		return [4]float64{a[3]*b[0] + a[0]*b[3] + a[1]*b[2] - a[2]*b[1], a[3]*b[1] - a[0]*b[2] + a[1]*b[3] + a[2]*b[0], a[3]*b[2] + a[0]*b[1] - a[1]*b[0] + a[2]*b[3], a[3]*b[3] - a[0]*b[0] - a[1]*b[1] - a[2]*b[2]}
	}
	d := [4]float64{first.Translation[0] - second.Translation[0], first.Translation[1] - second.Translation[1], first.Translation[2] - second.Translation[2], 0}
	r := mul(mul(qi, d), q)
	return InstancePose{Translation: [3]float64{r[0], r[1], r[2]}, Rotation: mul(qi, normalizedInstanceRotation(first.Rotation))}
}

func normalizedInstanceRotation(value [4]float64) [4]float64 {
	if value == [4]float64{} {
		return [4]float64{0, 0, 0, 1}
	}
	return value
}

type assemblyConstraintCapabilities struct {
	direction     bool
	distanceSide  bool
	directedAngle bool
}

type assemblySolveFailure struct {
	status     string
	diagnostic string
}

func (failure *assemblySolveFailure) Error() string {
	return fmt.Sprintf("assembly solve %s: %s", failure.status, failure.diagnostic)
}

func (failure *assemblySolveFailure) Unwrap() error { return ErrValidation }

// assemblyCapabilities is the authoritative application-layer geometry-pair
// matrix. It operates on exact descriptors resolved from topology, rather than
// on the browser's FACE/EDGE pick category.
func assemblyCapabilities(kind, firstKind, secondKind string) assemblyConstraintCapabilities {
	planePair := firstKind == "PLANE" && secondKind == "PLANE"
	return assemblyConstraintCapabilities{
		direction: planePair && (kind == "COINCIDENT" || kind == "DISTANCE"),
		distanceSide: kind == "DISTANCE" && ((firstKind == "POINT" && secondKind == "PLANE") ||
			(firstKind == "PLANE" && secondKind == "POINT") || planePair),
		directedAngle: kind == "ANGLE" && planePair,
	}
}

func (service *Service) solveAssembly(ctx context.Context, documentID, requestID, drivenInstanceID string, intent *geometry.AssemblySolveIntent, model *ProductModel) error {
	if len(model.Constraints) == 0 {
		return nil
	}
	if service.worker == nil {
		return fmt.Errorf("%w: assembly solver is unavailable", ErrValidation)
	}
	instances := make(map[string]*ProductInstance, len(model.Instances))
	bodies := make([]geometry.AssemblyBody, 0, len(model.Instances))
	for index := range model.Instances {
		instance := &model.Instances[index]
		instance.Rotation = normalizedInstanceRotation(instance.Rotation)
		instances[instance.ID] = instance
		bodies = append(bodies, geometry.AssemblyBody{ID: instance.ID, Pose: geometry.AssemblyPose{Translation: instance.Translation, Rotation: instance.Rotation}})
	}
	type resolvedPart struct {
		model       PartModel
		geometryKey string
	}
	resolved := map[string]resolvedPart{}
	resolvePart := func(instance *ProductInstance) (resolvedPart, error) {
		versionID := instance.ReferencedVersionID
		if strings.EqualFold(instance.ReferenceMode, "FOLLOW_HEAD") || versionID == "" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, instance.ReferencedDocumentID).Scan(&versionID); err != nil {
				return resolvedPart{}, err
			}
		}
		if value, ok := resolved[versionID]; ok {
			return value, nil
		}
		var documentType, geometryKey string
		var modelJSON []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json,COALESCE(v.geometry_key,'') FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE v.id=$1`, versionID).Scan(&documentType, &modelJSON, &geometryKey); err != nil {
			return resolvedPart{}, err
		}
		if documentType != "PART" {
			return resolvedPart{}, fmt.Errorf("%w: assembly geometry currently requires a direct Part instance", ErrValidation)
		}
		var part PartModel
		if err := json.Unmarshal(modelJSON, &part); err != nil {
			return resolvedPart{}, err
		}
		normalizePartModel(&part)
		value := resolvedPart{model: part, geometryKey: geometryKey}
		resolved[versionID] = value
		return value, nil
	}
	geometryValues := make([]geometry.AssemblyGeometry, 0)
	seenGeometry := map[string]bool{}
	resolvedGeometry := map[string]geometry.AssemblyGeometry{}
	resolveRef := func(reference AssemblyGeometryRef) (string, error) {
		instance := instances[reference.InstanceID]
		if instance == nil {
			return "", fmt.Errorf("%w: assembly constraint references an unknown instance", ErrValidation)
		}
		if reference.Kind == "BODY" {
			return "", nil
		}
		key := reference.InstanceID + ":" + reference.Kind + ":" + reference.GeometryID + ":" + reference.Axis + ":" + reference.GeometryKey + fmt.Sprintf(":%d", reference.TopologyID)
		if seenGeometry[key] {
			return key, nil
		}
		value := geometry.AssemblyGeometry{ID: key, BodyID: reference.InstanceID, Kind: reference.Kind}
		switch reference.Kind {
		case "FACE", "EDGE", "VERTEX":
			if reference.GeometryKey == "" || reference.TopologyID == 0 {
				return "", fmt.Errorf("%w: a topology reference requires geometryKey and topologyId", ErrValidation)
			}
			part, err := resolvePart(instance)
			if err != nil {
				return "", err
			}
			if part.geometryKey == "" || part.geometryKey != reference.GeometryKey {
				return "", fmt.Errorf("%w: selected face does not belong to the resolved instance revision", ErrValidation)
			}
			properties, err := service.GetTopologyElementProperties(ctx, documentID, reference.GeometryKey, reference.Kind, reference.TopologyID)
			if err != nil {
				return "", err
			}
			if reference.Kind == "VERTEX" {
				if properties.Point == nil {
					return "", fmt.Errorf("%w: selected vertex is missing its exact point", ErrValidation)
				}
				value.Kind, value.Origin = "POINT", *properties.Point
				break
			}
			origin, originOK := properties.Properties["origin"].([3]float64)
			if reference.Kind == "EDGE" {
				if properties.GeometryType != "LINE" {
					return "", fmt.Errorf("%w: %s edges are not supported by this assembly constraint slice", ErrValidation, properties.GeometryType)
				}
				direction, ok := properties.Properties["direction"].([3]float64)
				if !originOK || !ok {
					return "", fmt.Errorf("%w: linear edge is missing its exact line", ErrValidation)
				}
				value.Kind, value.Origin, value.Direction = "AXIS", origin, direction
				break
			}
			switch properties.GeometryType {
			case "PLANE":
				direction, ok := properties.Properties["normal"].([3]float64)
				if !originOK || !ok {
					return "", fmt.Errorf("%w: planar face is missing its exact frame", ErrValidation)
				}
				value.Kind, value.Origin, value.Direction = "PLANE", origin, direction
			case "CYLINDER":
				direction, ok := properties.Properties["axis"].([3]float64)
				radius, radiusOK := properties.Properties["radius"].(float64)
				if !originOK || !ok || !radiusOK {
					return "", fmt.Errorf("%w: cylindrical face is missing its exact frame", ErrValidation)
				}
				value.Kind, value.Origin, value.Direction, value.Radius = "CYLINDER", origin, direction, radius
			default:
				return "", fmt.Errorf("%w: %s faces are not supported by this assembly constraint slice", ErrValidation, properties.GeometryType)
			}
		case "PLANE":
			resolved, err := resolvePart(instance)
			if err != nil {
				return "", err
			}
			part := resolved.model
			found := false
			for _, plane := range part.DatumPlanes {
				if plane.ID == reference.GeometryID {
					value.Origin, value.Direction, found = plane.Origin, plane.Normal, true
					break
				}
			}
			if !found {
				return "", fmt.Errorf("%w: referenced datum plane does not exist", ErrValidation)
			}
		case "AXIS", "POINT":
			resolved, err := resolvePart(instance)
			if err != nil {
				return "", err
			}
			part := resolved.model
			found := false
			for _, axis := range part.DatumAxes {
				if axis.ID == reference.GeometryID && reference.Kind == "AXIS" {
					value.Origin, value.Direction, found = axis.Origin, axis.Direction, true
					break
				}
			}
			for _, system := range part.AxisSystems {
				if system.ID != reference.GeometryID {
					continue
				}
				value.Origin, found = system.Origin, true
				if reference.Kind == "AXIS" {
					switch reference.Axis {
					case "X":
						value.Direction = system.XDirection
					case "Y":
						value.Direction = system.YDirection
					case "Z":
						value.Direction = system.ZDirection
					default:
						return "", fmt.Errorf("%w: axis-system direction must be X, Y, or Z", ErrValidation)
					}
				}
				break
			}
			if !found {
				return "", fmt.Errorf("%w: referenced axis geometry does not exist", ErrValidation)
			}
		default:
			return "", fmt.Errorf("%w: assembly references support BODY, POINT, AXIS, PLANE, VERTEX, linear EDGE, and planar/cylindrical FACE", ErrValidation)
		}
		seenGeometry[key] = true
		resolvedGeometry[key] = value
		geometryValues = append(geometryValues, value)
		return key, nil
	}
	constraints := make([]geometry.AssemblyConstraint, 0, len(model.Constraints))
	if drivenInstanceID != "" {
		if driven := instances[drivenInstanceID]; driven != nil {
			pose := geometry.AssemblyPose{Translation: driven.Translation, Rotation: normalizedInstanceRotation(driven.Rotation)}
			constraints = append(constraints, geometry.AssemblyConstraint{ID: "interaction-driver", Kind: "FIX", FirstBodyID: drivenInstanceID, FixedPose: &pose})
		}
	}
	for constraintIndex := range model.Constraints {
		constraint := &model.Constraints[constraintIndex]
		firstGeometry, err := resolveRef(constraint.First)
		if err != nil {
			return err
		}
		value := geometry.AssemblyConstraint{ID: constraint.ID, ConnectionID: constraint.ConnectionID, Kind: constraint.Kind, Mode: constraint.Mode, FirstBodyID: constraint.First.InstanceID, FirstGeometryID: firstGeometry, Value: constraint.Value, DirectionRelation: constraint.DirectionRelation, DistanceRelation: constraint.DistanceRelation,
			AngleReferenceDirection: constraint.AngleReferenceDirection}
		if constraint.Kind == "FIX" || constraint.Kind == "RIGID" {
			fixedValue := constraint.FixedPose
			if fixedValue == nil && constraint.Kind == "FIX" {
				instance := instances[constraint.First.InstanceID]
				fixedValue = &InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
			}
			if fixedValue == nil {
				return fmt.Errorf("%w: rigid constraint is missing its captured relative pose", ErrValidation)
			}
			fixed := geometry.AssemblyPose{Translation: fixedValue.Translation, Rotation: normalizedInstanceRotation(fixedValue.Rotation)}
			value.FixedPose = &fixed
		}
		if constraint.Kind != "FIX" && constraint.Second != nil {
			secondGeometry, resolveErr := resolveRef(*constraint.Second)
			if resolveErr != nil {
				return resolveErr
			}
			value.SecondBodyID, value.SecondGeometryID = constraint.Second.InstanceID, secondGeometry
		}
		if constraint.Kind != "FIX" && constraint.Kind != "RIGID" {
			firstKind := resolvedGeometry[firstGeometry].Kind
			secondKind := resolvedGeometry[value.SecondGeometryID].Kind
			capabilities := assemblyCapabilities(constraint.Kind, firstKind, secondKind)
			if constraint.Kind == "ANGLE" && constraint.AngleReferenceDirection != nil {
				constraint.DirectionRelation, value.DirectionRelation = "SAME", "SAME"
			} else if !capabilities.direction {
				constraint.DirectionRelation, value.DirectionRelation = "UNORIENTED", "UNORIENTED"
			} else if constraint.DirectionRelation == "" || constraint.DirectionRelation == "UNORIENTED" {
				// Persist a deterministic branch instead of delegating a mutable
				// "undefined" orientation to every future solve.
				constraint.DirectionRelation, value.DirectionRelation = "SAME", "SAME"
			}
			if !capabilities.distanceSide {
				constraint.DistanceRelation, value.DistanceRelation = "UNSIGNED", "UNSIGNED"
			}
			if constraint.Kind == "ANGLE" && constraint.AngleReferenceDirection != nil && !capabilities.directedAngle {
				return fmt.Errorf("%w: directed angle requires two planar supports", ErrValidation)
			}
			if constraint.Kind == "ANGLE" && constraint.Value > math.Pi &&
				(!capabilities.directedAngle || constraint.AngleReferenceDirection == nil) {
				return fmt.Errorf("%w: an angle above 180 degrees requires a persisted reference direction", ErrValidation)
			}
		}
		constraints = append(constraints, value)
	}
	result, err := service.worker.SolveAssemblyWithOptions(ctx, requestID, bodies, geometryValues, constraints, geometry.AssemblySolveOptions{Intent: intent})
	if err != nil {
		return err
	}
	if result.Status != "CONVERGED" {
		return &assemblySolveFailure{status: result.Status, diagnostic: result.Diagnostic}
	}
	for _, solved := range result.Bodies {
		if instance := instances[solved.ID]; instance != nil {
			instance.Translation, instance.Rotation = solved.Pose.Translation, solved.Pose.Rotation
		}
	}
	return nil
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
)

func normalizedInstanceRotation(value [4]float64) [4]float64 {
	if value == [4]float64{} {
		return [4]float64{0, 0, 0, 1}
	}
	return value
}

func (service *Service) solveAssembly(ctx context.Context, requestID string, model *ProductModel) error {
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
	resolved := map[string]PartModel{}
	resolvePart := func(instance *ProductInstance) (PartModel, error) {
		versionID := instance.ReferencedVersionID
		if strings.EqualFold(instance.ReferenceMode, "FOLLOW_HEAD") || versionID == "" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, instance.ReferencedDocumentID).Scan(&versionID); err != nil {
				return PartModel{}, err
			}
		}
		if value, ok := resolved[versionID]; ok {
			return value, nil
		}
		var documentType string
		var modelJSON []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE v.id=$1`, versionID).Scan(&documentType, &modelJSON); err != nil {
			return PartModel{}, err
		}
		if documentType != "PART" {
			return PartModel{}, fmt.Errorf("%w: assembly geometry currently requires a direct Part instance", ErrValidation)
		}
		var part PartModel
		if err := json.Unmarshal(modelJSON, &part); err != nil {
			return PartModel{}, err
		}
		normalizePartModel(&part)
		resolved[versionID] = part
		return part, nil
	}
	geometryValues := make([]geometry.AssemblyGeometry, 0)
	seenGeometry := map[string]bool{}
	resolveRef := func(reference AssemblyGeometryRef) (string, error) {
		instance := instances[reference.InstanceID]
		if instance == nil {
			return "", fmt.Errorf("%w: assembly constraint references an unknown instance", ErrValidation)
		}
		if reference.Kind == "BODY" {
			return "", nil
		}
		key := reference.InstanceID + ":" + reference.Kind + ":" + reference.GeometryID + ":" + reference.Axis
		if seenGeometry[key] {
			return key, nil
		}
		part, err := resolvePart(instance)
		if err != nil {
			return "", err
		}
		value := geometry.AssemblyGeometry{ID: key, BodyID: reference.InstanceID, Kind: reference.Kind}
		switch reference.Kind {
		case "PLANE":
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
			return "", fmt.Errorf("%w: first Product slice supports BODY, POINT, AXIS, and PLANE references", ErrValidation)
		}
		seenGeometry[key] = true
		geometryValues = append(geometryValues, value)
		return key, nil
	}
	constraints := make([]geometry.AssemblyConstraint, 0, len(model.Constraints))
	for _, constraint := range model.Constraints {
		firstGeometry, err := resolveRef(constraint.First)
		if err != nil {
			return err
		}
		value := geometry.AssemblyConstraint{ID: constraint.ID, Kind: constraint.Kind, FirstBodyID: constraint.First.InstanceID, FirstGeometryID: firstGeometry, Value: constraint.Value, DirectionRelation: constraint.DirectionRelation, DistanceRelation: constraint.DistanceRelation}
		if constraint.Kind == "FIX" {
			fixedValue := constraint.FixedPose
			if fixedValue == nil {
				instance := instances[constraint.First.InstanceID]
				fixedValue = &InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
			}
			fixed := geometry.AssemblyPose{Translation: fixedValue.Translation, Rotation: normalizedInstanceRotation(fixedValue.Rotation)}
			value.FixedPose = &fixed
		} else if constraint.Second != nil {
			secondGeometry, resolveErr := resolveRef(*constraint.Second)
			if resolveErr != nil {
				return resolveErr
			}
			value.SecondBodyID, value.SecondGeometryID = constraint.Second.InstanceID, secondGeometry
		}
		constraints = append(constraints, value)
	}
	result, err := service.worker.SolveAssembly(ctx, requestID, bodies, geometryValues, constraints)
	if err != nil {
		return err
	}
	if result.Status != "CONVERGED" {
		return fmt.Errorf("%w: assembly solve %s: %s", ErrValidation, result.Status, result.Diagnostic)
	}
	for _, solved := range result.Bodies {
		if instance := instances[solved.ID]; instance != nil {
			instance.Translation, instance.Rotation = solved.Pose.Translation, solved.Pose.Rotation
		}
	}
	return nil
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/geometry"
)

// Resolve stable occurrence IDs against accepted revisions, never the latest Head.
// Geometry belongs to the leaf Part, while motion belongs to the direct child body.
func (service *Service) assemblyReferenceOccurrence(ctx context.Context, product ProductModel, reference AssemblyGeometryRef) (*ProductInstance, InstancePose, error) {
	return resolveAssemblyOccurrence(product, reference, func(selected ProductInstance) (ProductModel, error) {
		var kind string
		var raw []byte
		if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v JOIN occccad.documents d ON d.id=v.document_id WHERE d.id=$1 AND v.id=$2`, selected.ReferencedDocumentID, selected.ReferencedVersionID).Scan(&kind, &raw); err != nil {
			return ProductModel{}, err
		}
		if kind != "PRODUCT" {
			return ProductModel{}, fmt.Errorf("%w: assembly occurrence traverses a Part", ErrValidation)
		}
		var child ProductModel
		err := json.Unmarshal(raw, &child)
		return child, err
	})
}

func resolveAssemblyOccurrence(product ProductModel, reference AssemblyGeometryRef, load func(ProductInstance) (ProductModel, error)) (*ProductInstance, InstancePose, error) {
	pose := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	ids := []string{reference.InstanceID}
	if reference.InstancePath != nil {
		if len(reference.InstancePath.Segments) == 0 || len(reference.InstancePath.Segments) > instancePathMaxDepth || reference.InstancePath.Segments[0].InstanceID != reference.InstanceID {
			return nil, pose, fmt.Errorf("%w: invalid assembly occurrence path", ErrValidation)
		}
		ids = nil
		for _, segment := range reference.InstancePath.Segments {
			ids = append(ids, segment.InstanceID)
		}
	}
	current := product
	for depth, id := range ids {
		var selected *ProductInstance
		for i := range current.Instances {
			if current.Instances[i].ID == id {
				copy := current.Instances[i]
				selected = &copy
				break
			}
		}
		if selected == nil {
			return nil, pose, fmt.Errorf("%w: assembly occurrence disappeared", ErrValidation)
		}
		if reference.InstancePath != nil {
			expected := reference.InstancePath.Segments[depth].ReferencedDocumentID
			if expected != "" && expected != selected.ReferencedDocumentID {
				return nil, pose, fmt.Errorf("%w: assembly occurrence document changed", ErrValidation)
			}
		}
		if depth > 0 {
			pose = composeInstancePose(pose, InstancePose{Translation: selected.Translation, Rotation: normalizedInstanceRotation(selected.Rotation)})
		}
		if depth == len(ids)-1 {
			return selected, pose, nil
		}
		child, err := load(*selected)
		if err != nil {
			return nil, pose, err
		}
		current = child
	}
	return nil, pose, fmt.Errorf("%w: empty assembly occurrence", ErrValidation)
}

func assemblyGeometryInBody(value geometry.AssemblyGeometry, pose InstancePose) geometry.AssemblyGeometry {
	value.Origin = rotateByPose(pose, value.Origin)
	for i := range value.Origin {
		value.Origin[i] += pose.Translation[i]
	}
	value.Direction = rotateByPose(pose, value.Direction)
	return value
}

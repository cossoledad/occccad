package assistant

import "github.com/occccad/occccad/internal/workspace"

// Enrich only from the selected definition snapshot. UI kinds such as PAD and
// VISUAL are presentation categories, not the feature's real CAD type.
func selectionMetadata(ref *SelectedReference, part *workspace.PartModel) {
	if part == nil {
		return
	}
	for _, feature := range part.Features {
		if feature.ID == ref.ID && (ref.Kind == "PAD" || ref.Kind == "IMPORT" || ref.Kind == "FEATURE" || ref.Kind == "SKETCH" || ref.Kind == "TREE") {
			ref.Kind = "FEATURE"
			if feature.Sketch != nil {
				ref.Kind = "SKETCH"
			}
			ref.EntityType, ref.BodyID, ref.OwnerID, ref.Name = feature.Type, feature.BodyID, feature.BodyID, feature.Name
			return
		}
		if feature.ID != ref.FeatureID || feature.Sketch == nil {
			continue
		}
		if ref.SketchReference != nil && (ref.SketchReference.Target == "SKETCH_ORIGIN" || ref.SketchReference.Target == "SKETCH_X_AXIS" || ref.SketchReference.Target == "SKETCH_Y_AXIS") {
			ref.Kind = "SKETCH_REFERENCE"
			ref.OwnerID = feature.ID
			ref.BodyID = feature.BodyID
			ref.ID = ""
			return
		}
		if ref.Kind == "VISUAL" || ref.Kind == "SKETCH_ENTITY" {
			for _, entity := range feature.Sketch.Entities {
				if entity.ID == ref.ID {
					ref.Kind = "SKETCH_ENTITY"
					ref.Role = entity.Role
					ref.EntityType = entity.Kind
					ref.OwnerID = feature.ID
					ref.BodyID = feature.BodyID
					return
				}
			}
			for _, entity := range feature.Sketch.ExternalGeometry {
				if entity.ID == ref.ID {
					ref.Kind = "SKETCH_ENTITY"
					ref.EntityType = entity.GeometryKind
					ref.OwnerID = feature.ID
					ref.BodyID = feature.BodyID
					return
				}
			}
		}
		if ref.Kind == "SKETCH-CONSTRAINT" || ref.Kind == "SKETCH_CONSTRAINT" {
			for _, constraint := range feature.Sketch.Constraints {
				if constraint.ID == ref.ID {
					ref.Kind = "SKETCH_CONSTRAINT"
					ref.EntityType = constraint.Kind
					ref.OwnerID = feature.ID
					ref.BodyID = feature.BodyID
					return
				}
			}
		}
	}
}

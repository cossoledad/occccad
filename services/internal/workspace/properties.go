package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
)

const typeSetProperties = "occccad://part/properties/set"
const typeSetProductProperties = "occccad://product/properties/set"

// Metadata is a frozen participant snapshot for compensation. The live title
// remains Document metadata; historical viewing never renames a document.
type DocumentMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type OccurrenceAppearance struct {
	InstancePath InstancePath       `json:"instancePath"`
	BodyID       string             `json:"bodyId,omitempty"`
	Name         *string            `json:"name,omitempty"`
	Appearance   *SurfaceAppearance `json:"appearance,omitempty"`
}

type PropertyTarget struct {
	Kind         string                         `json:"kind"`
	ID           string                         `json:"id"`
	InstancePath *InstancePath                  `json:"instancePath,omitempty"`
	BodyID       string                         `json:"bodyId,omitempty"`
	GeometryKey  string                         `json:"geometryKey,omitempty"`
	LocalID      uint64                         `json:"localId,omitempty"`
	Anchor       *modelcore.SemanticTopologyRef `json:"anchor,omitempty"`
}

type PropertyUpdateRequest struct {
	Targets            []PropertyTarget           `json:"targets"`
	Name               *string                    `json:"name,omitempty"`
	Description        *string                    `json:"description,omitempty"`
	Appearance         map[string]json.RawMessage `json:"appearance,omitempty"`
	ClearFaceOverrides bool                       `json:"clearFaceOverrides,omitempty"`
}

func rebaseCopiedPropertyPaths(raw []byte, sourceID, documentID, revisionID string) ([]byte, error) {
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, err
	}
	for i := range model.AppearanceOverrides {
		path := &model.AppearanceOverrides[i].InstancePath
		if path.RootDocumentID != sourceID {
			return nil, fmt.Errorf("%w: copied appearance owner mismatch", ErrValidation)
		}
		path.RootDocumentID = documentID
		path.Segments = append([]InstancePathSegment(nil), path.Segments...)
		if len(path.Segments) > 0 {
			path.Segments[0].OwnerDocumentID = documentID
			path.Segments[0].OwnerVersionID = revisionID
		}
	}
	return json.Marshal(model)
}

func (s *Service) adaptProperties(ctx context.Context, documentID, documentType string, raw json.RawMessage, request CommandRequest) (string, any, error) {
	if request.Properties == nil || request.VersionID == "" {
		return "", nil, fmt.Errorf("%w: properties require targets and a base Revision", ErrValidation)
	}
	input := *request.Properties
	input.Targets = append([]PropertyTarget(nil), input.Targets...)
	if len(input.Targets) == 0 || len(input.Targets) > 256 {
		return "", nil, fmt.Errorf("%w: select between 1 and 256 property targets", ErrValidation)
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if err := validateScopedName(name); err != nil {
			return "", nil, err
		}
		if len(input.Targets) != 1 {
			return "", nil, fmt.Errorf("%w: rename requires one object", ErrValidation)
		}
		input.Name = &name
	}
	if input.Description != nil {
		v := strings.TrimSpace(*input.Description)
		if utf8.RuneCountInString(v) > 500 {
			return "", nil, fmt.Errorf("%w: description is limited to 500 characters", ErrValidation)
		}
		input.Description = &v
	}
	var occurrences []expandedOccurrence
	if documentType == "PRODUCT" {
		var product ProductModel
		if err := json.Unmarshal(raw, &product); err != nil {
			return "", nil, err
		}
		var err error
		occurrences, err = s.expandProductContext(ctx, documentID, request.VersionID, product)
		if err != nil {
			return "", nil, err
		}
	}
	for i := range input.Targets {
		t := &input.Targets[i]
		t.Kind = strings.ToUpper(t.Kind)
		if input.Description != nil && t.Kind != "DOCUMENT" {
			return "", nil, fmt.Errorf("%w: description belongs to a document", ErrValidation)
		}
		if t.Kind == "DOCUMENT" {
			if t.ID != documentID {
				return "", nil, fmt.Errorf("%w: document property owner mismatch", ErrValidation)
			}
			t.InstancePath, t.Anchor = nil, nil
			t.BodyID, t.GeometryKey, t.LocalID = "", "", 0
			continue
		}
		sourceID, sourceVersion := documentID, request.VersionID
		if documentType == "PRODUCT" {
			if t.InstancePath == nil {
				return "", nil, fmt.Errorf("%w: occurrence properties require InstancePath", ErrValidation)
			}
			if err := validateNonRootInstancePath(*t.InstancePath, documentID); err != nil {
				return "", nil, err
			}
			o, ok := occurrenceByCanonical(occurrences, t.InstancePath.Canonical)
			if !ok {
				return "", nil, fmt.Errorf("%w: occurrence no longer exists", ErrValidation)
			}
			if err := validateResolvedInstancePath(*t.InstancePath, o.Path); err != nil {
				return "", nil, err
			}
			sourceID, sourceVersion = o.DocumentID, o.RevisionID
			if t.Kind == "INSTANCE" {
				if t.ID != o.Path.Segments[len(o.Path.Segments)-1].InstanceID {
					return "", nil, fmt.Errorf("%w: occurrence target mismatch", ErrValidation)
				}
			} else if o.DocumentType != "PART" {
				return "", nil, fmt.Errorf("%w: graphical subelement must belong to a Part", ErrValidation)
			}
			if t.Kind == "BODY" {
				var model PartModel
				var bodyRaw []byte
				if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1`, sourceVersion).Scan(&bodyRaw); err != nil {
					return "", nil, err
				}
				if err := json.Unmarshal(bodyRaw, &model); err != nil {
					return "", nil, err
				}
				if bodyIndex(model, t.ID) < 0 {
					return "", nil, fmt.Errorf("%w: occurrence Body does not exist", ErrValidation)
				}
			}
		}
		if t.Kind == "FACE" {
			if input.Name != nil {
				return "", nil, fmt.Errorf("%w: topology does not have an editable name", ErrValidation)
			}
			selection, err := s.BindPersistentSelection(ctx, sourceID, BindPersistentSelectionRequest{SourceVersionID: sourceVersion, BodyID: t.BodyID, GeometryKey: t.GeometryKey, Kind: "FACE", LocalID: t.LocalID})
			if err != nil {
				return "", nil, err
			}
			t.Anchor = &selection.Anchor
			t.BodyID = selection.SourceBodyID
			t.ID = selection.SourceBodyID
			t.GeometryKey = ""
			t.LocalID = 0 // exchange/pick locators never enter the Domain Command
		} else {
			t.Anchor = nil
			t.GeometryKey = ""
			t.LocalID = 0
		}
	}
	if documentType == "PRODUCT" {
		return typeSetProductProperties, input, nil
	}
	return typeSetProperties, input, nil
}

func applyPartProperties(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var input PropertyUpdateRequest
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	set := modelcore.ChangeSet{}
	add := func(id, slot string, before, after any) { addPropertyChange(&set, id, slot, before, after) }
	for _, t := range input.Targets {
		if t.InstancePath != nil {
			return nil, set, fmt.Errorf("%w: Part properties cannot address an occurrence", ErrValidation)
		}
		switch t.Kind {
		case "DOCUMENT":
			if input.Name != nil || input.Description != nil {
				before := model.Metadata
				after, err := patchDocumentMetadata(before, input)
				if err != nil {
					return nil, set, err
				}
				model.Metadata = after
				add(t.ID, "document.metadata", before, after)
			}
			if len(input.Appearance) > 0 || input.ClearFaceOverrides {
				before := model.DefaultAppearance
				after, err := patchAppearance(before, input.Appearance, input.ClearFaceOverrides)
				if err != nil {
					return nil, set, err
				}
				model.DefaultAppearance = after
				add(t.ID, "document.appearance", before, after)
			}
		case "BODY", "FACE":
			id := t.ID
			if t.Kind == "FACE" {
				id = t.BodyID
			}
			i := bodyIndex(model, id)
			if i < 0 {
				return nil, set, fmt.Errorf("%w: property Body does not exist", ErrValidation)
			}
			if input.Name != nil {
				before := model.Bodies[i].Name
				model.Bodies[i].Name = *input.Name
				add(id, "body.name", before, *input.Name)
			}
			if len(input.Appearance) > 0 || input.ClearFaceOverrides {
				before := model.Bodies[i].Appearance
				after, err := patchTargetAppearance(before, t, input)
				if err != nil {
					return nil, set, err
				}
				model.Bodies[i].Appearance = after
				add(id, "body.appearance", before, after)
				set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("appearance:"+id))
			}
		case "FEATURE", "SKETCH", "PLANE", "DATUM_AXIS":
			if len(input.Appearance) > 0 || input.ClearFaceOverrides {
				return nil, set, fmt.Errorf("%w: this object has no independent surface appearance", ErrValidation)
			}
			found := false
			if t.Kind == "FEATURE" || t.Kind == "SKETCH" {
				for i := range model.Features {
					if model.Features[i].ID == t.ID {
						found = true
						if input.Name != nil {
							before := model.Features[i].Name
							model.Features[i].Name = *input.Name
							add(t.ID, "feature.name", before, *input.Name)
						}
						break
					}
				}
			}
			if t.Kind == "PLANE" {
				for i := range model.DatumPlanes {
					if model.DatumPlanes[i].ID == t.ID && model.DatumPlanes[i].Definition != nil {
						found = true
						if input.Name != nil {
							before := model.DatumPlanes[i].Name
							model.DatumPlanes[i].Name = *input.Name
							add(t.ID, "datum-plane.name", before, *input.Name)
						}
						break
					}
				}
			}
			if t.Kind == "DATUM_AXIS" {
				for i := range model.DatumAxes {
					if model.DatumAxes[i].ID == t.ID {
						found = true
						if input.Name != nil {
							before := model.DatumAxes[i].Name
							model.DatumAxes[i].Name = *input.Name
							add(t.ID, "datum-axis.name", before, *input.Name)
						}
						break
					}
				}
			}
			if !found {
				return nil, set, fmt.Errorf("%w: property target does not exist or is protected", ErrValidation)
			}
		default:
			return nil, set, fmt.Errorf("%w: unsupported property target %s", ErrValidation, t.Kind)
		}
	}
	if len(set.Changes) == 0 {
		return nil, set, fmt.Errorf("%w: properties have not changed", ErrValidation)
	}
	next, err := json.Marshal(model)
	return next, set, err
}

func applyProductProperties(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	var input PropertyUpdateRequest
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	set := modelcore.ChangeSet{}
	add := func(id, slot string, before, after any) { addPropertyChange(&set, id, slot, before, after) }
	beforeOverrides := append([]OccurrenceAppearance(nil), model.AppearanceOverrides...)
	for _, t := range input.Targets {
		if t.Kind == "DOCUMENT" {
			if input.Name != nil || input.Description != nil {
				before := model.Metadata
				after, err := patchDocumentMetadata(before, input)
				if err != nil {
					return nil, set, err
				}
				model.Metadata = after
				add(t.ID, "document.metadata", before, after)
			}
			if len(input.Appearance) > 0 || input.ClearFaceOverrides {
				before := model.DefaultAppearance
				after, err := patchAppearance(before, input.Appearance, input.ClearFaceOverrides)
				if err != nil {
					return nil, set, err
				}
				model.DefaultAppearance = after
				add(t.ID, "document.appearance", before, after)
			}
			continue
		}
		if t.InstancePath == nil || t.InstancePath.Canonical == "" || (t.Kind != "INSTANCE" && t.Kind != "BODY" && t.Kind != "FACE") {
			return nil, set, fmt.Errorf("%w: invalid occurrence property target", ErrValidation)
		}
		bodyID := t.BodyID
		if t.Kind == "BODY" {
			bodyID = t.ID
		}
		if t.Kind == "INSTANCE" {
			bodyID = ""
		}
		index := slices.IndexFunc(model.AppearanceOverrides, func(o OccurrenceAppearance) bool {
			return o.InstancePath.Canonical == t.InstancePath.Canonical && o.BodyID == bodyID
		})
		value := OccurrenceAppearance{InstancePath: *t.InstancePath, BodyID: bodyID}
		if index >= 0 {
			value = model.AppearanceOverrides[index]
			old, current := value.InstancePath.Segments, t.InstancePath.Segments
			if bodyID != "" && len(old) > 0 && len(current) > 0 && old[len(old)-1].ReferencedDocumentID != current[len(current)-1].ReferencedDocumentID {
				// An explicit edit of the replacement starts from inheritance.
				value = OccurrenceAppearance{BodyID: bodyID}
			}
		}
		value.InstancePath = *t.InstancePath
		if input.Name != nil {
			if t.Kind != "INSTANCE" {
				return nil, set, fmt.Errorf("%w: rename the original Body in its Part", ErrValidation)
			}
			if len(t.InstancePath.Segments) == 1 {
				i := slices.IndexFunc(model.Instances, func(v ProductInstance) bool { return v.ID == t.ID })
				if i < 0 {
					return nil, set, fmt.Errorf("%w: instance deleted", ErrValidation)
				}
				for _, other := range model.Instances {
					if other.ID != t.ID && scopedNameKey(other.Name) == scopedNameKey(*input.Name) {
						return nil, set, fmt.Errorf("%w: sibling instance name already exists", ErrValidation)
					}
				}
				before := model.Instances[i].Name
				model.Instances[i].Name = *input.Name
				add(t.ID, "instance.name", before, *input.Name)
			} else {
				value.Name = input.Name
			}
		}
		if len(input.Appearance) > 0 || input.ClearFaceOverrides {
			after, err := patchTargetAppearance(value.Appearance, t, input)
			if err != nil {
				return nil, set, err
			}
			value.Appearance = after
		}
		if value.Appearance == nil && value.Name == nil {
			if index >= 0 {
				model.AppearanceOverrides = slices.Delete(model.AppearanceOverrides, index, index+1)
			}
		} else if index >= 0 {
			model.AppearanceOverrides[index] = value
		} else {
			model.AppearanceOverrides = append(model.AppearanceOverrides, value)
		}
	}
	slices.SortFunc(model.AppearanceOverrides, func(a, b OccurrenceAppearance) int {
		return strings.Compare(a.InstancePath.Canonical+"/"+a.BodyID, b.InstancePath.Canonical+"/"+b.BodyID)
	})
	if len(model.AppearanceOverrides) == 0 {
		model.AppearanceOverrides = nil
	}
	add("document", "product.appearance-overrides", beforeOverrides, model.AppearanceOverrides)
	if len(set.Changes) == 0 {
		return nil, set, fmt.Errorf("%w: properties have not changed", ErrValidation)
	}
	set.ImpactSeeds = []modelcore.DependencyKey{"appearance:document"}
	next, err := json.Marshal(model)
	return next, set, err
}

func patchDocumentMetadata(before *DocumentMetadata, input PropertyUpdateRequest) (*DocumentMetadata, error) {
	if before == nil {
		return nil, fmt.Errorf("%w: document metadata snapshot missing", ErrValidation)
	}
	after := *before
	if input.Name != nil {
		after.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		after.Description = strings.TrimSpace(*input.Description)
	}
	if err := validateScopedName(after.Name); err != nil {
		return nil, err
	}
	if utf8.RuneCountInString(after.Description) > 500 {
		return nil, fmt.Errorf("%w: description is limited to 500 characters", ErrValidation)
	}
	return &after, nil
}

func patchTargetAppearance(before *SurfaceAppearance, t PropertyTarget, input PropertyUpdateRequest) (*SurfaceAppearance, error) {
	if t.Kind != "FACE" {
		return patchAppearance(before, input.Appearance, input.ClearFaceOverrides)
	}
	for key, value := range input.Appearance {
		if string(value) != "null" && key != "color" && key != "opacity" && key != "roughness" && key != "metalness" && key != "pickable" {
			return nil, fmt.Errorf("%w: %s is not a face surface property", ErrValidation, key)
		}
	}
	if t.Anchor == nil || t.Anchor.FeatureID == "" || t.Anchor.OutputSlot == "" {
		return nil, fmt.Errorf("%w: stable face anchor required", ErrValidation)
	}
	if input.ClearFaceOverrides {
		return nil, fmt.Errorf("%w: clear face overrides addresses a whole Body", ErrValidation)
	}
	var out SurfaceAppearance
	if before != nil {
		out = *before
		out.Faces = append([]FaceAppearance(nil), before.Faces...)
	}
	i := slices.IndexFunc(out.Faces, func(f FaceAppearance) bool { return refKey(f.Anchor) == refKey(*t.Anchor) })
	var face *SurfaceAppearance
	if i >= 0 {
		face = &SurfaceAppearance{GraphicProperties: out.Faces[i].GraphicProperties, Color: out.Faces[i].Color}
	}
	value, err := patchAppearance(face, input.Appearance, false)
	if err != nil {
		return nil, err
	}
	inherited := []string{}
	if i >= 0 {
		inherited = append(inherited, out.Faces[i].InheritedFields...)
	}
	for key, raw := range input.Appearance {
		if key != "color" && key != "opacity" && key != "roughness" && key != "metalness" && key != "pickable" {
			continue
		}
		if string(raw) == "null" {
			if !slices.Contains(inherited, key) {
				inherited = append(inherited, key)
			}
		} else {
			inherited = slices.DeleteFunc(inherited, func(v string) bool { return v == key })
		}
	}
	slices.Sort(inherited)
	if len(inherited) == 0 {
		inherited = nil
	}
	if value == nil && len(inherited) == 0 {
		if i >= 0 {
			out.Faces = slices.Delete(out.Faces, i, i+1)
		}
	} else {
		f := FaceAppearance{Anchor: *t.Anchor, InheritedFields: inherited}
		if value != nil {
			f.GraphicProperties = value.GraphicProperties
			f.Color = value.Color
		}
		if i >= 0 {
			out.Faces[i] = f
		} else {
			out.Faces = append(out.Faces, f)
		}
	}

	if len(out.Faces) == 0 {
		out.Faces = nil
	}
	if reflect.DeepEqual(out, SurfaceAppearance{}) {
		return nil, nil
	}
	return &out, nil
}

// This participant runs inside the same short CAS transaction as model history.
// Comparing the captured metadata protects against out-of-band metadata writes.
func persistMetadataChanges(ctx context.Context, tx database.Tx, documentID string, set modelcore.ChangeSet) error {
	for _, c := range set.Changes {
		if c.Target.SlotID != "document.metadata" {
			continue
		}
		var before, after DocumentMetadata
		if err := json.Unmarshal(c.Before, &before); err != nil {
			return err
		}
		if err := json.Unmarshal(c.After, &after); err != nil {
			return err
		}
		r, err := tx.Exec(ctx, `UPDATE occccad.documents SET name=$1,description=$2,updated_at=now() WHERE id=$3 AND name=$4 AND description=$5`, after.Name, after.Description, documentID, before.Name, before.Description)
		if err != nil {
			return err
		}
		if r.RowsAffected() != 1 {
			return fmt.Errorf("%w: DOCUMENT_METADATA_CONFLICT", ErrValidation)
		}
	}
	return nil
}

func displayPropertiesOnly(set modelcore.ChangeSet) bool {
	if len(set.Changes) == 0 {
		return false
	}
	for _, c := range set.Changes {
		switch c.Target.SlotID {
		case "document.metadata", "document.appearance", "body.appearance", "body.name", "feature.name", "datum-plane.name", "datum-axis.name", "product.appearance-overrides", "instance.name":
		default:
			return false
		}
	}
	return true
}

func addPropertyChange(set *modelcore.ChangeSet, id, slot string, before, after any) {
	address := modelcore.PropertyAddress{EntityID: id, SlotID: slot}
	for i := range set.Changes {
		if set.Changes[i].Target == address {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, set.Changes[i].Before, after)
			set.Changes[i] = change
			return
		}
	}
	if reflect.DeepEqual(before, after) {
		return
	}
	change, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, before, after)
	set.Changes = append(set.Changes, change)
}

package workspace

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

func partContainerField(m *PartModel, kind, id string) *string {
	switch kind {
	case "FEATURE":
		for i := range m.Features {
			if m.Features[i].ID == id {
				return &m.Features[i].GeometricalSetID
			}
		}
	case "DATUM_PLANE":
		for i := range m.DatumPlanes {
			if m.DatumPlanes[i].ID == id {
				return &m.DatumPlanes[i].GeometricalSetID
			}
		}
	case "DATUM_AXIS":
		for i := range m.DatumAxes {
			if m.DatumAxes[i].ID == id {
				return &m.DatumAxes[i].GeometricalSetID
			}
		}
	case "REFERENCE_POINT":
		for i := range m.DatumPoints {
			if m.DatumPoints[i].ID == id {
				return &m.DatumPoints[i].GeometricalSetID
			}
		}
	}
	return nil
}
func partContainerHistory(m *PartModel, a modelcore.PropertyAddress, value json.RawMessage, write bool) (json.RawMessage, bool, error) {
	if a.SlotID == "feature.body" {
		for i := range m.Features {
			if m.Features[i].ID == a.EntityID {
				return propertyHistoryValue(&m.Features[i].BodyID, value, write)
			}
		}
		return nil, true, fmt.Errorf("%w: missing feature", ErrValidation)
	}
	for kind, prefix := range map[string]string{"FEATURE": "feature", "DATUM_PLANE": "datum-plane", "DATUM_AXIS": "datum-axis", "REFERENCE_POINT": "reference-point"} {
		if a.SlotID == prefix+".container" {
			field := partContainerField(m, kind, a.EntityID)
			if field == nil {
				return nil, true, fmt.Errorf("%w: missing container member", ErrValidation)
			}
			return propertyHistoryValue(field, value, write)
		}
	}
	return nil, false, nil
}
func applyPartContainerChange(m PartModel, p geometricalSetPayload) (json.RawMessage, modelcore.ChangeSet, error) {
	set := modelcore.ChangeSet{}
	destination := p.Set.ID
	targets := p.Targets
	if p.Action == "DISSOLVE" {
		index := slices.IndexFunc(m.GeometricalSets, func(v GeometricalSet) bool { return v.ID == p.Set.ID })
		if index < 0 {
			return nil, set, ErrNotFound
		}
		source := m.GeometricalSets[index]
		destination = p.Set.ParentID
		if destination == "" {
			destination = source.ParentID
		}
		if destination == "" || destination == source.ID {
			return nil, set, fmt.Errorf("%w: select a destination geometrical set", ErrValidation)
		}
		for _, o := range partObjects(m) {
			if objectContainer(o) == objectKey("GEOMETRICAL_SET", source.ID) {
				targets = append(targets, deleteNodePayload{TargetKind: o.Kind, TargetID: o.ID})
			}
		}
		addPropertyChange(&set, source.ID, "geometrical-set.entity", source, nil)
		m.GeometricalSets = slices.Delete(m.GeometricalSets, index, index+1)
	}
	if !slices.ContainsFunc(m.GeometricalSets, func(v GeometricalSet) bool { return v.ID == destination }) {
		return nil, set, fmt.Errorf("%w: destination geometrical set missing", ErrValidation)
	}
	for _, t := range targets {
		if t.TargetKind == "GEOMETRICAL_SET" {
			i := slices.IndexFunc(m.GeometricalSets, func(v GeometricalSet) bool { return v.ID == t.TargetID })
			if i < 0 || t.TargetID == destination {
				return nil, set, fmt.Errorf("%w: invalid set destination", ErrValidation)
			}
			before := m.GeometricalSets[i]
			m.GeometricalSets[i].ParentID = destination
			addPropertyChange(&set, t.TargetID, "geometrical-set.entity", before, m.GeometricalSets[i])
			continue
		}
		kind := t.TargetKind
		if kind == "DATUM_PLANE" && isStandardDatumPlane(t.TargetID) {
			return nil, set, fmt.Errorf("%w: standard plane cannot move", ErrValidation)
		}
		if kind == "FEATURE" {
			found := false
			for i := range m.Features {
				f := &m.Features[i]
				if f.ID == t.TargetID {
					found = true
					if f.Type != "GEOMETRY" && f.Sketch == nil {
						return nil, set, fmt.Errorf("%w: solid material features require a Body", ErrValidation)
					}
					if f.BodyID != "" {
						addPropertyChange(&set, f.ID, "feature.body", f.BodyID, "")
						f.BodyID = ""
					}
				}
			}
			if !found {
				return nil, set, ErrNotFound
			}
		}
		field := partContainerField(&m, kind, t.TargetID)
		object, ok := partPropertyObjectFor(&m, kind, t.TargetID)
		if field == nil || !ok {
			return nil, set, fmt.Errorf("%w: object cannot move to a set", ErrValidation)
		}
		before := *field
		*field = destination
		addPropertyChange(&set, t.TargetID, object.prefix+".container", before, destination)
	}
	if err := validateAndResolvePartParameters(&m); err != nil {
		return nil, set, err
	}
	if len(set.Changes) == 0 {
		return nil, set, fmt.Errorf("%w: selected objects are already in this set", ErrValidation)
	}
	raw, err := json.Marshal(m)
	return raw, set, err
}

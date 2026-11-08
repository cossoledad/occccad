package workspace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

type GeometricalSet struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
	Visible  *bool  `json:"visible,omitempty"`
}

type ReferenceConstruction struct {
	Mode         string           `json:"mode"`
	Inputs       []DatumReference `json:"inputs,omitempty"`
	Coordinates  [3]float64       `json:"coordinates"`
	Direction    [3]float64       `json:"direction"`
	Ratio        float64          `json:"ratio"`
	Offset       float64          `json:"offset"`
	Angle        float64          `json:"angle"`
	Coefficients [4]float64       `json:"coefficients"`
}

type DatumPoint struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	GeometricalSetID string                 `json:"geometricalSetId"`
	Visible          *bool                  `json:"visible,omitempty"`
	Construction     *ReferenceConstruction `json:"construction"`
	Position         [3]float64             `json:"position"`
}

const typeGeometricalSet = "occccad://part/geometrical-set/edit"
const typeReferencePoint = "occccad://part/reference-point/edit"

func geometricalSetChangesOnly(changes modelcore.ChangeSet) bool {
	if len(changes.Changes) == 0 {
		return false
	}
	for _, c := range changes.Changes {
		if c.Target.SlotID != "geometrical-set.entity" {
			return false
		}
	}
	return true
}

type geometricalSetPayload struct {
	Action   string         `json:"action"`
	Set      GeometricalSet `json:"set"`
	EntityID string         `json:"entityId,omitempty"`
}

func validateGeometricalSets(model PartModel) error {
	identities := map[string]bool{}
	checkIdentity := func(id string) error {
		if id == "" || identities[id] {
			return fmt.Errorf("%w: REFERENCE_IDENTITY_INVALID", ErrValidation)
		}
		identities[id] = true
		return nil
	}
	for _, p := range model.DatumPlanes {
		if err := checkIdentity(p.ID); err != nil {
			return err
		}
		if p.Definition != nil && p.Construction != nil {
			return fmt.Errorf("%w: REFERENCE_DEFINITION_AMBIGUOUS", ErrValidation)
		}
	}
	for _, p := range model.DatumAxes {
		if err := checkIdentity(p.ID); err != nil {
			return err
		}
		if p.Definition != nil && p.Construction != nil {
			return fmt.Errorf("%w: REFERENCE_DEFINITION_AMBIGUOUS", ErrValidation)
		}
	}
	for _, p := range model.AxisSystems {
		if err := checkIdentity(p.ID); err != nil {
			return err
		}
	}
	for _, p := range model.DatumPoints {
		if err := checkIdentity(p.ID); err != nil {
			return err
		}
		if p.Construction == nil || !strings.HasPrefix(p.Construction.Mode, "POINT_") {
			return fmt.Errorf("%w: POINT_CONSTRUCTION_INVALID", ErrValidation)
		}
	}
	sets := map[string]GeometricalSet{}
	names := map[string]bool{}
	for _, set := range model.GeometricalSets {
		key := set.ParentID + "\x00" + scopedNameKey(set.Name)
		if set.ID == "" || validateScopedName(set.Name) != nil || names[key] {
			return fmt.Errorf("%w: GEOMETRICAL_SET_IDENTITY_INVALID", ErrValidation)
		}
		if _, exists := sets[set.ID]; exists {
			return fmt.Errorf("%w: GEOMETRICAL_SET_IDENTITY_INVALID", ErrValidation)
		}
		sets[set.ID] = set
		names[key] = true
	}
	for _, set := range model.GeometricalSets {
		seen := map[string]bool{set.ID: true}
		parent := set.ParentID
		for parent != "" {
			if seen[parent] {
				return fmt.Errorf("%w: GEOMETRICAL_SET_CYCLE", ErrValidation)
			}
			seen[parent] = true
			p, exists := sets[parent]
			if !exists {
				return fmt.Errorf("%w: GEOMETRICAL_SET_PARENT_MISSING", ErrValidation)
			}
			parent = p.ParentID
		}
	}
	check := func(id string) error {
		if id != "" {
			if _, exists := sets[id]; !exists {
				return fmt.Errorf("%w: GEOMETRICAL_SET_MISSING", ErrValidation)
			}
		}
		return nil
	}
	for _, p := range model.DatumPlanes {
		if err := check(p.GeometricalSetID); err != nil {
			return err
		}
	}
	for _, p := range model.DatumAxes {
		if err := check(p.GeometricalSetID); err != nil {
			return err
		}
	}
	for _, p := range model.DatumPoints {
		if err := check(p.GeometricalSetID); err != nil {
			return err
		}
	}
	return nil
}

func referenceContainer(model *PartModel, id *string, seed string) ([]modelcore.ModelChange, error) {
	if *id != "" {
		for _, set := range model.GeometricalSets {
			if set.ID == *id {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("%w: GEOMETRICAL_SET_MISSING", ErrValidation)
	}
	if len(model.GeometricalSets) > 0 {
		*id = model.GeometricalSets[0].ID
		return nil, nil
	}
	set := GeometricalSet{ID: commandEntityID("geometrical-set", seed), Name: "几何图形集.1"}
	model.GeometricalSets = append(model.GeometricalSets, set)
	*id = set.ID
	change, err := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: set.ID, SlotID: "geometrical-set.entity"}, nil, set)
	return []modelcore.ModelChange{change}, err
}

func applyGeometricalSet(raw, payload json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var input geometricalSetPayload
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	index := -1
	for i, set := range model.GeometricalSets {
		if set.ID == input.Set.ID {
			index = i
		}
	}
	var before, after any
	switch input.Action {
	case "CREATE":
		if index >= 0 {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate set", ErrValidation)
		}
		if strings.TrimSpace(input.Set.Name) == "" {
			names := []string{}
			for _, set := range model.GeometricalSets {
				if set.ParentID == input.Set.ParentID {
					names = append(names, set.Name)
				}
			}
			input.Set.Name = nextScopedName("几何图形集.", names)
		}
		model.GeometricalSets = append(model.GeometricalSets, input.Set)
		after = input.Set
	case "EDIT":
		if index < 0 {
			return nil, modelcore.ChangeSet{}, ErrNotFound
		}
		before = model.GeometricalSets[index]
		if input.Set.Name == "" {
			input.Set.Name = model.GeometricalSets[index].Name
		}
		input.Set.Visible = model.GeometricalSets[index].Visible
		model.GeometricalSets[index] = input.Set
		after = input.Set
	case "DELETE":
		if index < 0 {
			return nil, modelcore.ChangeSet{}, ErrNotFound
		}
		for _, set := range model.GeometricalSets {
			if set.ParentID == input.Set.ID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: GEOMETRICAL_SET_NOT_EMPTY", ErrValidation)
			}
		}
		for _, id := range referenceContainerIDs(model) {
			if id == input.Set.ID {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: GEOMETRICAL_SET_NOT_EMPTY", ErrValidation)
			}
		}
		before = model.GeometricalSets[index]
		model.GeometricalSets = append(model.GeometricalSets[:index], model.GeometricalSets[index+1:]...)
	default:
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid geometrical set action", ErrValidation)
	}
	if err := validateGeometricalSets(model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	kind := modelcore.ChangeUpdate
	if before == nil {
		kind = modelcore.ChangeCreate
	}
	if after == nil {
		kind = modelcore.ChangeDelete
	}
	change, err := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: input.Set.ID, SlotID: "geometrical-set.entity"}, before, after)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}}, err
}

func referenceContainerIDs(model PartModel) []string {
	out := []string{}
	for _, p := range model.DatumPlanes {
		out = append(out, p.GeometricalSetID)
	}
	for _, p := range model.DatumAxes {
		out = append(out, p.GeometricalSetID)
	}
	for _, f := range model.Features {
		if f.GeometricalSetID != "" {
			out = append(out, f.GeometricalSetID)
		}
	}
	for _, p := range model.DatumPoints {
		out = append(out, p.GeometricalSetID)
	}
	return out
}

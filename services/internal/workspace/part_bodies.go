package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// GeometryKey is a derived result frozen with the Revision, not a mutable
// business property. Body commands/history compare only the definition.
type PartBody struct {
	CreatedByFeatureID string `json:"createdByFeatureId,omitempty"`
	Order              int    `json:"order"`
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Visible            bool   `json:"visible"`
	GeometryKey        string `json:"geometryKey,omitempty"`
}

func bodyDefinition(b PartBody) PartBody { b.GeometryKey = ""; return b }
func bodyIndex(m PartModel, id string) int {
	return slices.IndexFunc(m.Bodies, func(b PartBody) bool { return b.ID == id })
}
func normalizeBodies(m *PartModel) {
	if m.Bodies == nil {
		m.Bodies = []PartBody{{ID: "body-main", Name: "Body.1", Visible: true}}
	}
	if m.ActiveBodyID == "" && len(m.Bodies) > 0 {
		m.ActiveBodyID = m.Bodies[0].ID
	}
	for i := range m.Bodies {
		if m.Bodies[i].Order == 0 {
			m.Bodies[i].Order = i + 1
		}
	}
	for i := range m.Features {
		if m.Features[i].Order == 0 {
			m.Features[i].Order = i + 1
		}
		if m.Features[i].BodyID == "" {
			m.Features[i].BodyID = m.ActiveBodyID
		}
	}
}
func nextBodyName(m PartModel) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf("Body.%d", n)
		if !slices.ContainsFunc(m.Bodies, func(b PartBody) bool { return b.Name == name }) {
			return name
		}
	}
}
func bodyModel(m PartModel, id string) PartModel {
	out := m
	out.Features = nil
	out.Bodies = make([]PartBody, 0, 1)
	out.ActiveBodyID = id
	needed := map[string]bool{}
	for _, f := range m.Features {
		if f.BodyID == id {
			if f.Profile != "" {
				needed[f.Profile] = true
			}
			if axis := strings.Split(f.AxisEntityID, ":"); len(axis) == 3 && axis[0] == "SKETCH_LINE" {
				needed[axis[1]] = true
			}
		}
	}
	for _, f := range m.Features {
		if f.BodyID == id || needed[f.ID] {
			out.Features = append(out.Features, f)
		}
	}
	if i := bodyIndex(m, id); i >= 0 {
		out.Bodies = append(out.Bodies, m.Bodies[i])
	}
	return out
}

// A Body artifact may also contain sketches borrowed as inputs by features in
// this Body. Only these IDs are display-owned here in a Product occurrence.
func ownedSketchIDs(m PartModel, bodyID string) []string {
	var ids []string
	for _, feature := range m.Features {
		if feature.BodyID == bodyID && feature.Sketch != nil {
			ids = append(ids, feature.ID)
		}
	}
	return ids
}
func (s *Service) evaluatePartBodies(ctx context.Context, requestID string, m *PartModel) error {
	normalizePartModel(m)
	for i := range m.Bodies {
		key, err := s.evaluateBody(ctx, requestID+"/body/"+m.Bodies[i].ID, bodyModel(*m, m.Bodies[i].ID))
		if err != nil {
			return err
		}
		m.Bodies[i].GeometryKey = key
	}
	return nil
}
func (s *Service) evaluateBodyPrefix(ctx context.Context, requestID string, m PartModel, bodyID string) (string, error) {
	if bodyID == "" {
		bodyID = m.ActiveBodyID
	}
	return s.evaluateBody(ctx, requestID, bodyModel(m, bodyID))
}
func (s *Service) bodyArtifacts(ctx context.Context, m PartModel) (map[string]Artifact, error) {
	result := map[string]Artifact{}
	for _, b := range m.Bodies {
		if b.GeometryKey == "" {
			continue
		}
		a, err := s.loadArtifact(ctx, b.GeometryKey)
		if err != nil {
			return nil, err
		}
		a.BodyID = b.ID
		result[b.GeometryKey] = a
	}
	return result, nil
}

const typeBodyCommand = "occccad://part/body/change"

type bodyCommand struct {
	Action  string `json:"action"`
	BodyID  string `json:"bodyId"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
}

func applyBodyCommand(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var m PartModel
	var p bodyCommand
	if err := json.Unmarshal(modelJSON, &m); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&m)
	beforeActive := m.ActiveBodyID
	beforeParams := append([]modelcore.ParameterDefinition(nil), m.Parameters...)
	var before, after any
	var changes []modelcore.ModelChange
	index := bodyIndex(m, p.BodyID)
	if p.Action == "CREATE" {
		if index >= 0 || p.BodyID == "" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid Body identity", ErrValidation)
		}
		if strings.TrimSpace(p.Name) == "" {
			p.Name = nextBodyName(m)
		}
		order := 1
		for _, b := range m.Bodies {
			if b.Order >= order {
				order = b.Order + 1
			}
		}
		b := PartBody{Order: order, ID: p.BodyID, Name: p.Name, Visible: true}
		m.Bodies = append(m.Bodies, b)
		after = b
		m.ActiveBodyID = b.ID
	} else {
		if index < 0 {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Body does not exist", ErrValidation)
		}
		before = bodyDefinition(m.Bodies[index])
		switch p.Action {
		case "RENAME":
			if strings.TrimSpace(p.Name) == "" {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Body name required", ErrValidation)
			}
			m.Bodies[index].Name = p.Name
		case "ACTIVATE":
			m.ActiveBodyID = p.BodyID
		case "VISIBILITY":
			m.Bodies[index].Visible = p.Visible
		case "DELETE":
			removed := map[string]bool{}
			for _, f := range m.Features {
				if f.BodyID == p.BodyID {
					removed[f.ID] = true
				}
			}
			if err := rejectPublicationTargetRemoval(m, p.BodyID, removed); err != nil {
				return nil, modelcore.ChangeSet{}, err
			}
			for _, f := range m.Features {
				if f.BodyID != p.BodyID && removed[f.Profile] {
					return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Body is used by feature %s", ErrValidation, f.ID)
				}
			}
			kept := []Feature{}
			for _, f := range m.Features {
				if !removed[f.ID] {
					kept = append(kept, f)
				} else {
					c, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: f.ID, SlotID: "entity"}, f, nil)
					changes = append(changes, c)
				}
			}
			m.Features = kept
			params := []modelcore.ParameterDefinition{}
			for _, p := range m.Parameters {
				drop := false
				for id := range removed {
					drop = drop || strings.HasPrefix(p.ParameterID, "parameter:"+id+":")
				}
				if !drop {
					params = append(params, p)
				}
			}
			m.Parameters = params
			m.Bodies = append(m.Bodies[:index], m.Bodies[index+1:]...)
			if m.ActiveBodyID == p.BodyID {
				m.ActiveBodyID = ""
				if len(m.Bodies) > 0 {
					m.ActiveBodyID = m.Bodies[0].ID
				}
			}
		default:
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid Body action", ErrValidation)
		}
		if p.Action != "DELETE" {
			after = bodyDefinition(m.Bodies[index])
		}
	}
	if p.Action != "ACTIVATE" {
		kind := modelcore.ChangeUpdate
		if before == nil {
			kind = modelcore.ChangeCreate
		}
		if after == nil {
			kind = modelcore.ChangeDelete
		}
		c, _ := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: p.BodyID, SlotID: "body.entity"}, before, after)
		changes = append(changes, c)
	}
	if beforeActive != m.ActiveBodyID {
		c, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "part", SlotID: "active-body"}, beforeActive, m.ActiveBodyID)
		changes = append(changes, c)
	}
	changes, seeds := appendParameterLifecycleChanges(changes, []modelcore.DependencyKey{modelcore.DependencyKey("body:" + p.BodyID)}, beforeParams, m.Parameters)
	data, _ := json.Marshal(m)
	return data, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
}

func previewBodyID(payload json.RawMessage, m PartModel) string {
	var p struct {
		Feature   Feature `json:"feature"`
		FeatureID string  `json:"featureId"`
		SketchID  string  `json:"sketchId"`
	}
	_ = json.Unmarshal(payload, &p)
	for _, f := range m.Features {
		if f.ID == p.Feature.ID || f.ID == p.FeatureID || f.ID == p.SketchID {
			return f.BodyID
		}
	}
	return m.ActiveBodyID
}

// createFeatureBody records the explicit NEW_BODY intent in the same command
// as the feature. One command creates one Body, regardless of its solid count.
func createFeatureBody(m *PartModel, f *Feature) []modelcore.ModelChange {
	order := 1
	for _, b := range m.Bodies {
		if b.Order >= order {
			order = b.Order + 1
		}
	}
	b := PartBody{ID: "body-" + f.ID, Name: nextBodyName(*m), Order: order, Visible: true, CreatedByFeatureID: f.ID}
	m.Bodies = append(m.Bodies, b)
	created, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: b.ID, SlotID: "body.entity"}, nil, b)
	active, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "part", SlotID: "active-body"}, m.ActiveBodyID, b.ID)
	m.ActiveBodyID = b.ID
	f.BodyID = b.ID
	f.Operation = "ADD"
	return []modelcore.ModelChange{created, active}
}

// A feature-owned Body disappears with its originating feature. Do not silently
// discard later features: users must delete those dependents first.
func removeFeatureBody(m *PartModel, featureID string) ([]modelcore.ModelChange, error) {
	for i, b := range m.Bodies {
		if b.CreatedByFeatureID != featureID {
			continue
		}
		for _, f := range m.Features {
			if f.BodyID == b.ID {
				return nil, fmt.Errorf("%w: cannot delete feature %s while Body feature %s depends on it", ErrValidation, featureID, f.ID)
			}
		}
		deleted, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: b.ID, SlotID: "body.entity"}, bodyDefinition(b), nil)
		changes := []modelcore.ModelChange{deleted}
		m.Bodies = append(m.Bodies[:i], m.Bodies[i+1:]...)
		if m.ActiveBodyID == b.ID {
			next := ""
			if len(m.Bodies) > 0 {
				next = m.Bodies[0].ID
			}
			active, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: "part", SlotID: "active-body"}, b.ID, next)
			changes = append(changes, active)
			m.ActiveBodyID = next
		}
		return changes, nil
	}
	return nil, nil
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

// Retired references are unavailable typed descriptors, never live geometry or
// a second history. Only descriptors still referenced by a surviving definition
// are retained. They let a valid definition remain editable after input removal.
type RetiredPartObject struct {
	Kind      string                         `json:"kind"`
	ID        string                         `json:"id"`
	Name      string                         `json:"name"`
	Feature   *Feature                       `json:"feature,omitempty"`
	Body      *PartBody                      `json:"body,omitempty"`
	Set       *GeometricalSet                `json:"set,omitempty"`
	Plane     *DatumPlane                    `json:"plane,omitempty"`
	Axis      *DatumAxis                     `json:"axis,omitempty"`
	Point     *DatumPoint                    `json:"point,omitempty"`
	Parameter *modelcore.ParameterDefinition `json:"parameter,omitempty"`
}
type DeleteOptions struct {
	DeleteDependents       bool `json:"deleteDependents,omitempty"`
	DeleteExclusiveParents bool `json:"deleteExclusiveParents,omitempty"`
}
type DeleteImpactItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}
type PartDeletePlanRequest struct {
	VersionID string             `json:"versionId"`
	Targets   []DeleteNodeTarget `json:"targets"`
	Options   DeleteOptions      `json:"options"`
}
type PartDeletePlan struct {
	VersionID    string             `json:"versionId"`
	Digest       string             `json:"digest"`
	Options      DeleteOptions      `json:"options"`
	Deleted      []DeleteImpactItem `json:"deleted"`
	Affected     []DeleteImpactItem `json:"affected"`
	Publications []DeleteImpactItem `json:"publications"`
}

func managedPartObject(kind string) bool {
	return slices.Contains([]string{"BODY", "GEOMETRICAL_SET", "FEATURE", "DATUM_PLANE", "DATUM_AXIS", "REFERENCE_POINT"}, kind)
}
func objectKey(kind, id string) string  { return kind + "\x00" + id }
func (o RetiredPartObject) key() string { return objectKey(o.Kind, o.ID) }
func (o RetiredPartObject) dependencyKeys() []modelcore.DependencyKey {
	switch o.Kind {
	case "FEATURE":
		out := []modelcore.DependencyKey{modelcore.DependencyKey("feature:" + o.ID)}
		if o.Feature != nil && o.Feature.Sketch != nil {
			out = append(out, modelcore.DependencyKey("profile:"+o.ID))
		}
		return out
	case "PARAMETER":
		return []modelcore.DependencyKey{modelcore.DependencyKey("parameter:" + o.ID)}
	case "BODY":
		return []modelcore.DependencyKey{modelcore.DependencyKey("body:" + o.ID)}
	case "DATUM_PLANE", "DATUM_AXIS", "REFERENCE_POINT":
		return []modelcore.DependencyKey{modelcore.DependencyKey("datum:" + o.ID)}
	}
	return nil
}
func partObjects(m PartModel) map[string]RetiredPartObject {
	out := map[string]RetiredPartObject{}
	put := func(o RetiredPartObject) { out[o.key()] = o }
	for _, v := range m.Features {
		f := featureHistoryDefinition(v)
		put(RetiredPartObject{Kind: "FEATURE", ID: v.ID, Name: v.Name, Feature: &f})
	}
	for _, v := range m.Bodies {
		b := bodyDefinition(v)
		put(RetiredPartObject{Kind: "BODY", ID: v.ID, Name: v.Name, Body: &b})
	}
	for _, v := range m.GeometricalSets {
		put(RetiredPartObject{Kind: "GEOMETRICAL_SET", ID: v.ID, Name: v.Name, Set: &v})
	}
	for _, v := range m.DatumPlanes {
		put(RetiredPartObject{Kind: "DATUM_PLANE", ID: v.ID, Name: v.Name, Plane: &v})
	}
	for _, v := range m.DatumAxes {
		put(RetiredPartObject{Kind: "DATUM_AXIS", ID: v.ID, Name: v.Name, Axis: &v})
	}
	for _, v := range m.DatumPoints {
		put(RetiredPartObject{Kind: "REFERENCE_POINT", ID: v.ID, Name: v.Name, Point: &v})
	}
	for _, v := range m.Parameters {
		put(RetiredPartObject{Kind: "PARAMETER", ID: v.ParameterID, Name: v.Label, Parameter: &v})
	}
	return out
}
func objectContainer(o RetiredPartObject) string {
	if o.Feature != nil {
		if o.Feature.GeometricalSetID != "" {
			return objectKey("GEOMETRICAL_SET", o.Feature.GeometricalSetID)
		}
		return objectKey("BODY", o.Feature.BodyID)
	}
	if o.Set != nil && o.Set.ParentID != "" {
		return objectKey("GEOMETRICAL_SET", o.Set.ParentID)
	}
	id := ""
	if o.Plane != nil {
		id = o.Plane.GeometricalSetID
	}
	if o.Axis != nil {
		id = o.Axis.GeometricalSetID
	}
	if o.Point != nil {
		id = o.Point.GeometricalSetID
	}
	if id != "" {
		return objectKey("GEOMETRICAL_SET", id)
	}
	if o.Parameter != nil && o.Parameter.OwnerFeatureID != "" {
		return objectKey("FEATURE", o.Parameter.OwnerFeatureID)
	}
	return ""
}
func deletionPlan(m PartModel, p deleteNodesPayload) (PartDeletePlan, map[string]RetiredPartObject, error) {
	plan := PartDeletePlan{Options: p.Options, Deleted: []DeleteImpactItem{}, Affected: []DeleteImpactItem{}, Publications: []DeleteImpactItem{}}
	if len(p.Targets) == 0 || len(p.Targets) > 256 {
		return plan, nil, fmt.Errorf("%w: select between 1 and 256 objects", ErrValidation)
	}
	all := partObjects(m)
	removed := map[string]RetiredPartObject{}
	reasons := map[string]string{}
	add := func(o RetiredPartObject, reason string) bool {
		if _, ok := removed[o.key()]; ok {
			return false
		}
		removed[o.key()] = o
		reasons[o.key()] = reason
		return true
	}
	for _, t := range p.Targets {
		o, ok := all[objectKey(t.TargetKind, t.TargetID)]
		if !ok || !managedPartObject(t.TargetKind) {
			return plan, nil, fmt.Errorf("%w: delete target does not exist", ErrValidation)
		}
		if o.Plane != nil && isStandardDatumPlane(o.ID) {
			return plan, nil, fmt.Errorf("%w: standard datum planes are protected", ErrValidation)
		}
		add(o, "SELECTED")
	}
	graph, _, err := buildPartEvaluation(m, "", "", nil, nil)
	if err != nil {
		return plan, nil, err
	}
	// Evaluation skips suppressed consumers. Deletion examines their stored
	// design references as well, so exclusive-input cleanup cannot orphan them.
	for _, f := range m.Features {
		if !f.Suppressed {
			continue
		}
		target := modelcore.DependencyKey("feature:" + f.ID)
		for _, input := range featureInputs(f) {
			prefix := "feature:"
			if input.EntityKind != "FEATURE" {
				prefix = "datum:"
			}
			source := modelcore.DependencyKey(prefix + input.FeatureID)
			if _, ok := graph.Nodes[source]; ok {
				graph.Edges = append(graph.Edges, modelcore.DependencyEdge{Source: source, Target: target, Kind: modelcore.ReadGeometry})
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for _, o := range all {
			_, owned := removed[objectContainer(o)]
			if o.Parameter != nil {
				for _, kind := range []string{"FEATURE", "REFERENCE_POINT", "DATUM_PLANE", "DATUM_AXIS"} {
					if _, ok := removed[objectKey(kind, o.Parameter.OwnerFeatureID)]; ok {
						owned = true
					}
				}
			}
			if owned {
				changed = add(o, "CONTAINED") || changed
			}
		}
		seeds := []modelcore.DependencyKey{}
		for _, o := range removed {
			seeds = append(seeds, o.dependencyKeys()...)
		}
		affected := map[modelcore.DependencyKey]bool{}
		for _, k := range graph.DirtyClosure(seeds) {
			affected[k] = true
		}
		if p.Options.DeleteDependents {
			for _, o := range all {
				if o.Kind == "BODY" || o.Kind == "GEOMETRICAL_SET" || o.Kind == "PARAMETER" {
					continue
				}
				for _, key := range o.dependencyKeys() {
					if affected[key] {
						changed = add(o, "DEPENDENT") || changed
						break
					}
				}
			}
		}
		if p.Options.DeleteExclusiveParents {
			candidates := map[string]bool{}
			for _, o := range removed {
				if o.Feature != nil {
					for _, i := range featureInputs(*o.Feature) {
						if i.Role == "STAGE" {
							continue
						}
						kind := "FEATURE"
						if i.EntityKind != "FEATURE" {
							kind = ""
							for _, k := range []string{"REFERENCE_POINT", "DATUM_AXIS", "DATUM_PLANE"} {
								if _, ok := all[objectKey(k, i.FeatureID)]; ok {
									kind = k
									break
								}
							}
						}
						if kind != "" {
							candidates[objectKey(kind, i.FeatureID)] = true
						}
					}
				}
			}
			for key := range candidates {
				o, ok := all[key]
				if !ok || o.Feature != nil && o.Feature.Sketch == nil && o.Feature.Type != "GEOMETRY" || o.Plane != nil && isStandardDatumPlane(o.ID) {
					continue
				}
				// The candidate and its owned parameters/profile form one input.
				// Any surviving reader, including a datum, formula or measurement,
				// makes that input shared. Stop at removed readers: they disappear.
				ownKeys := map[modelcore.DependencyKey]bool{}
				goneKeys := map[modelcore.DependencyKey]bool{}
				for _, k := range o.dependencyKeys() {
					ownKeys[k] = true
				}
				for _, v := range all {
					if objectContainer(v) == o.key() && v.Kind == "PARAMETER" {
						for _, k := range v.dependencyKeys() {
							ownKeys[k] = true
						}
					}
				}
				for _, v := range removed {
					for _, k := range v.dependencyKeys() {
						goneKeys[k] = true
					}
				}
				shared := false
				for _, edge := range graph.Edges {
					if ownKeys[edge.Source] && !ownKeys[edge.Target] && !goneKeys[edge.Target] {
						shared = true
						break
					}
				}
				for _, pub := range m.Publications {
					if publicationTargetsObject(pub, o) {
						shared = true
					}
				}
				for _, port := range m.ContextInputs {
					if contextInputTargetsObject(port, o) {
						shared = true
					}
					for _, v := range all {
						if v.Kind == "PARAMETER" && objectContainer(v) == o.key() && contextInputTargetsObject(port, v) {
							shared = true
						}
					}
				}
				if !shared {
					changed = add(o, "EXCLUSIVE_INPUT") || changed
				}
			}
		}
		for _, b := range m.Bodies {
			if b.CreatedByFeatureID == "" {
				continue
			}
			if _, gone := removed[objectKey("FEATURE", b.CreatedByFeatureID)]; !gone {
				continue
			}
			hasSurvivor := false
			for _, f := range m.Features {
				if f.BodyID == b.ID {
					if _, gone := removed[objectKey("FEATURE", f.ID)]; !gone {
						hasSurvivor = true
					}
				}
			}
			if !hasSurvivor {
				changed = add(all[objectKey("BODY", b.ID)], "EMPTY_OWNED_BODY") || changed
			}
		}
	}
	seeds := []modelcore.DependencyKey{}
	for _, o := range removed {
		plan.Deleted = append(plan.Deleted, DeleteImpactItem{o.Kind, o.ID, o.Name, "DELETE", reasons[o.key()]})
		seeds = append(seeds, o.dependencyKeys()...)
	}
	affected := map[modelcore.DependencyKey]bool{}
	for _, k := range graph.DirtyClosure(seeds) {
		affected[k] = true
	}
	for _, o := range all {
		if _, gone := removed[o.key()]; gone || o.Kind == "GEOMETRICAL_SET" {
			continue
		}
		for _, k := range o.dependencyKeys() {
			if affected[k] {
				plan.Affected = append(plan.Affected, DeleteImpactItem{o.Kind, o.ID, o.Name, "RECOMPUTE", "DEPENDENCY_CHANGED"})
				break
			}
		}
	}
	for _, pub := range m.Publications {
		for _, o := range removed {
			if publicationTargetsObject(pub, o) {
				plan.Publications = append(plan.Publications, DeleteImpactItem{"PUBLICATION", pub.ID, pub.Name, "UNRESOLVED", "TARGET_REMOVED"})
				break
			}
		}
	}

	for _, measurement := range m.Measurements {
		if affected[modelcore.DependencyKey("measurement:"+measurement.ID)] {
			plan.Affected = append(plan.Affected, DeleteImpactItem{"MEASUREMENT", measurement.ID, measurement.Name, "BLOCKED", "SUPPORT_CHANGED"})
		}
	}
	for _, port := range m.ContextInputs {
		for _, o := range removed {
			if contextInputTargetsObject(port, o) {
				plan.Publications = append(plan.Publications, DeleteImpactItem{"CONTEXT_INPUT", port.ID, port.Name, "UNRESOLVED", "TARGET_REMOVED"})
				break
			}
		}
	}

	// Material order can be spliced. Explicit geometry/value references really
	// disappear, so distinguish them from the wider recomputation closure.
	removedKeys := map[modelcore.DependencyKey]bool{}
	for _, o := range removed {
		for _, key := range o.dependencyKeys() {
			removedKeys[key] = true
		}
	}
	directMissing := map[modelcore.DependencyKey]bool{}
	for _, f := range m.Features {
		if _, gone := removed[objectKey("FEATURE", f.ID)]; gone {
			continue
		}
		for _, input := range featureInputs(f) {
			prefix := "feature:"
			if input.EntityKind != "FEATURE" {
				prefix = "datum:"
			}
			if removedKeys[modelcore.DependencyKey(prefix+input.FeatureID)] {
				directMissing[modelcore.DependencyKey("feature:"+f.ID)] = true
			}
		}
	}
	for _, e := range graph.Edges {
		if removedKeys[e.Source] && (strings.HasPrefix(string(e.Target), "datum:") || strings.HasPrefix(string(e.Target), "parameter:")) {
			directMissing[e.Target] = true
		}
	}
	missingSeeds := []modelcore.DependencyKey{}
	for key := range directMissing {
		missingSeeds = append(missingSeeds, key)
	}
	blockedAfterRemoval := map[modelcore.DependencyKey]bool{}
	for _, key := range graph.DirtyClosure(missingSeeds) {
		blockedAfterRemoval[key] = true
	}
	for i := range plan.Affected {
		item := &plan.Affected[i]
		o := all[objectKey(item.Kind, item.ID)]
		for _, key := range o.dependencyKeys() {
			if directMissing[key] {
				item.State = "INPUT_MISSING"
				break
			}
			if blockedAfterRemoval[key] {
				item.State = "BLOCKED"
			}
		}
	}
	order := func(values []DeleteImpactItem) {
		sort.Slice(values, func(i, j int) bool {
			return objectKey(values[i].Kind, values[i].ID) < objectKey(values[j].Kind, values[j].ID)
		})
	}
	order(plan.Deleted)
	order(plan.Affected)
	order(plan.Publications)
	raw, _ := json.Marshal(struct {
		Model   PartModel
		Options DeleteOptions
		Deleted []DeleteImpactItem
	}{m, p.Options, plan.Deleted})
	plan.Digest = modelcore.ValueDigest(raw)
	return plan, removed, nil
}
func contextInputTargetsObject(input ContextInput, o RetiredPartObject) bool {
	if input.Target.TargetID != o.ID {
		return false
	}
	switch input.Target.Kind {
	case "PARAMETER":
		return o.Kind == "PARAMETER"
	case "DATUM":
		return o.Plane != nil || o.Axis != nil || o.Point != nil
	case "SKETCH_EXTERNAL_GEOMETRY", "FEATURE_INPUT":
		return o.Feature != nil
	}
	return false
}

func publicationTargetsObject(p Publication, o RetiredPartObject) bool {
	t := p.Target
	switch o.Kind {
	case "FEATURE":
		return t.FeatureID == o.ID || t.SketchID == o.ID || t.PersistentSelection != nil && (t.PersistentSelection.Anchor.FeatureID == o.ID || t.PersistentSelection.SourceResult != nil && t.PersistentSelection.SourceResult.FeatureID == o.ID)
	case "BODY":
		return t.BodyID == o.ID || t.PersistentSelection != nil && t.PersistentSelection.SourceBodyID == o.ID
	case "PARAMETER":
		return t.ParameterID == o.ID
	case "DATUM_AXIS", "DATUM_PLANE", "REFERENCE_POINT":
		return t.DatumID == o.ID || t.EntityID == o.ID
	}
	return false
}

func (s *Service) GetPartDeletePlan(ctx context.Context, documentID string, request PartDeletePlanRequest) (PartDeletePlan, error) {
	snapshot, err := s.ReadModelSnapshot(ctx, documentID, "")
	if err != nil {
		return PartDeletePlan{}, err
	}
	if snapshot.DocumentType != "PART" || request.VersionID == "" || snapshot.RevisionID != request.VersionID {
		return PartDeletePlan{}, fmt.Errorf("%w: DELETE_PLAN_STALE", ErrValidation)
	}
	var m PartModel
	if err = json.Unmarshal(snapshot.Model, &m); err != nil {
		return PartDeletePlan{}, err
	}
	normalizePartModel(&m)
	targets := make([]deleteNodePayload, 0, len(request.Targets))
	for _, t := range request.Targets {
		targets = append(targets, deleteNodePayload{TargetKind: t.TargetKind, TargetID: t.TargetID, OwnerEntityID: t.OwnerEntityID})
	}
	plan, _, err := deletionPlan(m, deleteNodesPayload{Targets: targets, Options: request.Options})
	plan.VersionID = snapshot.RevisionID
	return plan, err
}

func applyPartObjectDeletion(raw json.RawMessage, p deleteNodesPayload) (json.RawMessage, modelcore.ChangeSet, error) {
	var m PartModel
	set := modelcore.ChangeSet{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, set, err
	}
	normalizePartModel(&m)
	plan, removed, err := deletionPlan(m, p)
	if err != nil {
		return nil, set, err
	}
	if p.PlanDigest != "" && p.PlanDigest != plan.Digest {
		return nil, set, fmt.Errorf("%w: DELETE_PLAN_STALE", ErrValidation)
	}
	gone := func(kind, id string) bool { _, ok := removed[objectKey(kind, id)]; return ok }
	beforeRetired := append([]RetiredPartObject(nil), m.RetiredReferences...)
	beforeActive := m.ActiveBodyID
	add := func(id, slot string, before, after any) { addPropertyChange(&set, id, slot, before, after) }
	for _, o := range removed {
		var value any
		slot := ""
		switch o.Kind {
		case "FEATURE":
			slot = "entity"
			value = *o.Feature
		case "BODY":
			slot = "body.entity"
			value = *o.Body
		case "GEOMETRICAL_SET":
			slot = "geometrical-set.entity"
			value = *o.Set
		case "DATUM_PLANE":
			slot = "datum.plane"
			value = *o.Plane
		case "DATUM_AXIS":
			slot = "datum.axis"
			value = *o.Axis
		case "REFERENCE_POINT":
			slot = "datum.point"
			value = *o.Point
		case "PARAMETER":
			slot = "parameter.entity"
			value = *o.Parameter
		}
		add(o.ID, slot, value, nil)
		set.ImpactSeeds = append(set.ImpactSeeds, o.dependencyKeys()...)
		m.RetiredReferences = append(m.RetiredReferences, o)
	}
	m.Features = slices.DeleteFunc(m.Features, func(v Feature) bool { return gone("FEATURE", v.ID) })
	m.Bodies = slices.DeleteFunc(m.Bodies, func(v PartBody) bool { return gone("BODY", v.ID) })
	m.GeometricalSets = slices.DeleteFunc(m.GeometricalSets, func(v GeometricalSet) bool { return gone("GEOMETRICAL_SET", v.ID) })
	m.DatumPlanes = slices.DeleteFunc(m.DatumPlanes, func(v DatumPlane) bool { return gone("DATUM_PLANE", v.ID) })
	m.DatumAxes = slices.DeleteFunc(m.DatumAxes, func(v DatumAxis) bool { return gone("DATUM_AXIS", v.ID) })
	m.DatumPoints = slices.DeleteFunc(m.DatumPoints, func(v DatumPoint) bool { return gone("REFERENCE_POINT", v.ID) })
	m.Parameters = slices.DeleteFunc(m.Parameters, func(v modelcore.ParameterDefinition) bool { return gone("PARAMETER", v.ParameterID) })
	for i := range m.Bodies {
		b := &m.Bodies[i]
		if gone("FEATURE", b.CreatedByFeatureID) {
			before := bodyDefinition(*b)
			b.CreatedByFeatureID = ""
			add(b.ID, "body.entity", before, bodyDefinition(*b))
		}
	}
	if gone("BODY", m.ActiveBodyID) {
		m.ActiveBodyID = ""
		if len(m.Bodies) > 0 {
			m.ActiveBodyID = m.Bodies[0].ID
		}
	}
	add("part", "active-body", beforeActive, m.ActiveBodyID)
	pruneRetiredReferences(&m)
	add("part", "part.retired-references", beforeRetired, m.RetiredReferences)
	if err = validateAndResolvePartParameters(&m); err != nil {
		return nil, set, err
	}
	out, err := json.Marshal(m)
	return out, set, err
}

func pruneRetiredReferences(m *PartModel) {
	needed := map[modelcore.DependencyKey]bool{}
	for _, f := range m.Features {
		for _, i := range featureInputs(f) {
			prefix := "feature:"
			if i.EntityKind != "FEATURE" {
				prefix = "datum:"
			}
			needed[modelcore.DependencyKey(prefix+i.FeatureID)] = true
		}
	}
	for _, d := range datumDefinitions(m) {
		for _, r := range datumReferences(d) {
			needed[datumReferenceDependency(r)] = true
		}
	}
	for _, c := range referenceConstructions(m) {
		for _, r := range c.Inputs {
			needed[datumReferenceDependency(r)] = true
		}
	}
	for _, p := range m.Parameters {
		if p.Source.Expression != nil {
			for _, r := range p.Source.Expression.Reads {
				needed[r] = true
			}
		}
	}
	kept := []RetiredPartObject{}
	seen := map[string]bool{}
	for _, o := range m.RetiredReferences {
		keep := false
		for _, key := range o.dependencyKeys() {
			keep = keep || needed[key]
		}
		for _, p := range m.Publications {
			keep = keep || publicationTargetsObject(p, o)
		}
		for _, port := range m.ContextInputs {
			keep = keep || contextInputTargetsObject(port, o)
		}
		if keep && !seen[o.key()] {
			kept = append(kept, o)
			seen[o.key()] = true
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].key() < kept[j].key() })
	m.RetiredReferences = kept
	if len(kept) == 0 {
		m.RetiredReferences = nil
	}
}

// The copy is for type/shape validation only. Runtime resolution always uses
// active objects, and unavailable graph nodes block exact computation.
func retiredValidationContext(m PartModel) PartModel {
	m.Features = append([]Feature(nil), m.Features...)
	m.DatumPlanes = append([]DatumPlane(nil), m.DatumPlanes...)
	m.DatumAxes = append([]DatumAxis(nil), m.DatumAxes...)
	m.DatumPoints = append([]DatumPoint(nil), m.DatumPoints...)
	for _, o := range m.RetiredReferences {
		if o.Feature != nil {
			m.Features = append(m.Features, *o.Feature)
		}
		if o.Plane != nil {
			m.DatumPlanes = append(m.DatumPlanes, *o.Plane)
		}
		if o.Axis != nil {
			m.DatumAxes = append(m.DatumAxes, *o.Axis)
		}
		if o.Point != nil {
			m.DatumPoints = append(m.DatumPoints, *o.Point)
		}
	}
	return m
}
func retiredBlockedNodes(m PartModel, graph *modelcore.DependencyGraph) map[modelcore.DependencyKey]bool {
	seeds := []modelcore.DependencyKey{}
	for _, o := range m.RetiredReferences {
		seeds = append(seeds, o.dependencyKeys()...)
	}
	out := map[modelcore.DependencyKey]bool{}
	for _, k := range graph.DirtyClosure(seeds) {
		out[k] = true
	}
	return out
}
func retiredReferenceNames(m PartModel) string {
	names := []string{}
	for _, o := range m.RetiredReferences {
		if o.Name != "" {
			names = append(names, o.Name)
		} else {
			names = append(names, o.ID)
		}
	}
	if len(names) > 4 {
		names = names[:4]
	}
	return strings.Join(names, ", ")
}
func partObjectDeletionCommand(typeURI string, payload json.RawMessage) bool {
	if typeURI == typeDeletePartNode || typeURI == typeDeletePartNodes {
		return true
	}
	if typeURI == typeBodyCommand || typeURI == typeGeometricalSet {
		var p struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(payload, &p)
		return p.Action == "DELETE"
	}
	return false
}

func resolvePartParameterDefinitions(m *PartModel) (map[string]modelcore.Quantity, error) {
	if len(m.RetiredReferences) == 0 {
		return resolveParameterDefinitions(m.Parameters)
	}
	values := append([]modelcore.ParameterDefinition(nil), m.Parameters...)
	for _, o := range m.RetiredReferences {
		if o.Parameter != nil {
			p := *o.Parameter
			p.Source = modelcore.ValueSource{}
			p.Role = "MEASURED"
			p.EvaluatedValue = nil
			p.CurrencyStatus = "MISSING"
			p.Key = "missing_" + parameterKeyFragment(p.ParameterID)
			values = append(values, p)
		}
	}
	resolved, err := resolveParameterDefinitions(values)
	if err != nil {
		return nil, err
	}
	copy(m.Parameters, values[:len(m.Parameters)])
	return resolved, nil
}

func markRetiredReferenceFailures(m *PartModel, graph *modelcore.DependencyGraph) map[modelcore.DependencyKey]bool {
	blocked := retiredBlockedNodes(*m, graph)
	missing := map[modelcore.DependencyKey]bool{}
	for _, o := range m.RetiredReferences {
		for _, k := range o.dependencyKeys() {
			missing[k] = true
		}
	}
	direct := map[modelcore.DependencyKey]bool{}
	for _, e := range graph.Edges {
		if missing[e.Source] {
			direct[e.Target] = true
		}
	}
	referenceState := func(id string, status, diagnostic *string) {
		*status = ""
		*diagnostic = ""
		key := modelcore.DependencyKey("datum:" + id)
		if blocked[key] {
			*status = "BLOCKED"
			*diagnostic = "UPSTREAM_INPUT_MISSING"
			if direct[key] {
				*status = "FAILED"
				*diagnostic = "INPUT_REFERENCE_MISSING: " + retiredReferenceNames(*m)
			}
		}
	}
	for i := range m.DatumPoints {
		v := &m.DatumPoints[i]
		referenceState(v.ID, &v.EvaluationStatus, &v.Diagnostic)
	}
	for i := range m.DatumPlanes {
		v := &m.DatumPlanes[i]
		referenceState(v.ID, &v.EvaluationStatus, &v.Diagnostic)
	}
	for i := range m.DatumAxes {
		v := &m.DatumAxes[i]
		referenceState(v.ID, &v.EvaluationStatus, &v.Diagnostic)
	}
	for i := range m.Features {
		f := &m.Features[i]
		key := modelcore.DependencyKey("feature:" + f.ID)
		if !blocked[key] || f.Suppressed {
			continue
		}
		f.EvaluationStatus = "BLOCKED"
		f.Diagnostic = "UPSTREAM_INPUT_MISSING"
		if direct[key] {
			f.EvaluationStatus = "FAILED"
			f.Diagnostic = "INPUT_REFERENCE_MISSING: " + retiredReferenceNames(*m)
		}
		f.Result = nil
		if f.Sketch != nil {
			f.Sketch.Support.Status = "BLOCKED"
			if direct[key] {
				f.Sketch.Support.Status = "FAILED_SUPPORT"
			}
			f.Sketch.Support.DiagnosticCode = "INPUT_REFERENCE_MISSING"
			f.Sketch.Support.Diagnostic = f.Diagnostic
		}
	}
	return blocked
}

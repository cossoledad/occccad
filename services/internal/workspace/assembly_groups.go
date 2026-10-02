package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type AssemblyGroupRelation struct {
	InstanceID   string       `json:"instanceId"`
	RelativePose InstancePose `json:"relativePose"`
}

type AssemblyFrozenGroup struct {
	GroupID           string                  `json:"groupId"`
	MemberIDs         []string                `json:"memberIds"`
	InputDigest       string                  `json:"inputDigest"`
	CapturePending    bool                    `json:"capturePending"`
	CapturedRelations []AssemblyGroupRelation `json:"capturedRelations,omitempty"`
}

// Overlapping/nested active groups share one inner stage, then become rigid
// units only for the outer solve. This input is frozen; replay never reads Head.
type AssemblyGroupStage struct {
	ID                    string                `json:"id"`
	MemberIDs             []string              `json:"memberIds"`
	InternalConstraintIDs []string              `json:"internalConstraintIds"`
	UnresolvedInternalIDs []string              `json:"unresolvedInternalIds,omitempty"`
	Groups                []AssemblyFrozenGroup `json:"groups"`
}

func isAssemblyGroup(c AssemblyConstraint) bool { return c.Kind == "FIX_TOGETHER" }

func hasActiveAssemblyGroups(model ProductModel, excluded map[string]bool) bool {
	for _, c := range model.Constraints {
		if isAssemblyGroup(c) && !c.Suppressed && !excluded[c.ID] {
			return true
		}
	}
	return false
}

func groupMemberUnit(member AssemblyGroupMember) (string, error) {
	if member.GroupID != "" {
		if member.InstanceID != "" || member.InstancePath != nil {
			return "", fmt.Errorf("%w: group and occurrence identities are exclusive", ErrValidation)
		}
		return "", nil
	}
	id := member.InstanceID
	if member.InstancePath != nil {
		if len(member.InstancePath.Segments) == 0 {
			return "", fmt.Errorf("%w: group occurrence path is empty", ErrValidation)
		}
		root := member.InstancePath.Segments[0].InstanceID
		if id != "" && id != root {
			return "", fmt.Errorf("%w: group occurrence does not match its rigid motion unit", ErrValidation)
		}
		id = root
	}
	if id == "" {
		return "", fmt.Errorf("%w: empty group member identity", ErrValidation)
	}
	return id, nil
}

// A nested rigid Product remains ONE solver body, even when a member has a
// descendant path. CAD Body IDs are not membership or motion-unit identities.
func assemblyGroupMembers(model ProductModel) (map[string][]string, error) {
	instances := map[string]bool{}
	for _, instance := range model.Instances {
		instances[instance.ID] = true
	}
	groups := map[string]AssemblyConstraint{}
	names := map[string]string{}
	for _, c := range model.Constraints {
		if !isAssemblyGroup(c) {
			continue
		}
		if c.ID == "" || groups[c.ID].ID != "" {
			return nil, fmt.Errorf("%w: duplicate/empty GroupId", ErrValidation)
		}
		if err := validateAssemblyGroupDefinition(c); err != nil {
			return nil, err
		}
		name := strings.ToLower(strings.TrimSpace(c.Name))
		if prior := names[name]; prior != "" {
			return nil, fmt.Errorf("%w: duplicate group name (%s, %s)", ErrValidation, prior, c.ID)
		}
		names[name], groups[c.ID] = c.ID, c
	}
	resolved, visiting := map[string][]string{}, map[string]bool{}
	var expand func(string) ([]string, error)
	expand = func(id string) ([]string, error) {
		if result, exists := resolved[id]; exists {
			return result, nil
		}
		group, exists := groups[id]
		if !exists {
			return nil, fmt.Errorf("%w: GROUP_MEMBER_MISSING: %s", ErrValidation, id)
		}
		if visiting[id] {
			return nil, fmt.Errorf("%w: GROUP_REFERENCE_CYCLE: %s", ErrValidation, id)
		}
		visiting[id] = true
		units, direct := map[string]bool{}, map[string]bool{}
		for _, member := range group.GroupMembers {
			unit, err := groupMemberUnit(member)
			if err != nil {
				return nil, err
			}
			identity := "instance:" + unit
			if member.GroupID != "" {
				identity = "group:" + member.GroupID
			}
			if direct[identity] {
				return nil, fmt.Errorf("%w: duplicate direct group member %s", ErrValidation, identity)
			}
			direct[identity] = true
			if member.GroupID != "" {
				children, err := expand(member.GroupID)
				if err != nil {
					return nil, err
				}
				for _, child := range children {
					units[child] = true
				}
			} else {
				if !instances[unit] {
					return nil, fmt.Errorf("%w: GROUP_MEMBER_MISSING: %s", ErrValidation, unit)
				}
				units[unit] = true
			}
		}
		if len(units) < 2 {
			return nil, fmt.Errorf("%w: group must contain at least two distinct motion units", ErrValidation)
		}
		result := make([]string, 0, len(units))
		for unit := range units {
			result = append(result, unit)
		}
		sort.Strings(result)
		visiting[id], resolved[id] = false, result
		return result, nil
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := expand(id); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func validateAssemblyGroupDefinition(c AssemblyConstraint) error {
	if !isAssemblyGroup(c) {
		return nil
	}
	wrappedGroup := len(c.GroupMembers) == 1 && c.GroupMembers[0].GroupID != ""
	if c.ID == "" || (len(c.GroupMembers) < 2 && !wrappedGroup) || len(c.GroupMembers) > maxManifestBodies || strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("%w: group requires stable identity, name and 2..N members", ErrValidation)
	}
	if c.Mode != "" && c.Mode != "DRIVING" {
		return fmt.Errorf("%w: Fix Together does not support measurement/controlled mode", ErrValidation)
	}
	if c.First.InstanceID != "" || c.Second != nil || c.QuantityParameter != nil {
		return fmt.Errorf("%w: a multi-member group is not a binary/Quantity definition", ErrValidation)
	}
	return nil
}

// Explicit occurrence deletion removes that motion unit atomically. Groups
// reduced below two units dissolve; parents retain surviving stable members.
func pruneAssemblyGroupsForDeletedInstance(model *ProductModel, id string) []modelcore.ModelChange {
	before := map[string]AssemblyConstraint{}
	for i := range model.Constraints {
		c := &model.Constraints[i]
		if !isAssemblyGroup(*c) {
			continue
		}
		before[c.ID] = *c
		members := []AssemblyGroupMember{}
		for _, m := range c.GroupMembers {
			unit, _ := groupMemberUnit(m)
			if unit != id {
				members = append(members, m)
			}
		}
		if len(members) != len(c.GroupMembers) {
			c.GroupMembers = members
			c.GroupCapturePending = true
			c.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
		}
	}
	for {
		dissolve := -1
		for i, c := range model.Constraints {
			if isAssemblyGroup(c) && (len(c.GroupMembers) == 0 || (len(c.GroupMembers) == 1 && c.GroupMembers[0].GroupID == "")) {
				dissolve = i
				break
			}
		}
		if dissolve < 0 {
			break
		}
		c := model.Constraints[dissolve]
		model.Constraints = append(model.Constraints[:dissolve], model.Constraints[dissolve+1:]...)
		for i := range model.Constraints {
			parent := &model.Constraints[i]
			if !isAssemblyGroup(*parent) {
				continue
			}
			members := []AssemblyGroupMember{}
			changed := false
			for _, m := range parent.GroupMembers {
				if m.GroupID == c.ID {
					members = append(members, c.GroupMembers...)
					changed = true
				} else {
					members = append(members, m)
				}
			}
			if changed {
				parent.GroupMembers = deduplicateGroupMembers(members)
				parent.GroupCapturePending = true
				parent.EvaluationStatus = modelcore.AssemblyConstraintNotUpdated
			}
		}
	}
	after := map[string]AssemblyConstraint{}
	for _, c := range model.Constraints {
		if isAssemblyGroup(c) {
			after[c.ID] = c
		}
	}
	changes := []modelcore.ModelChange{}
	ids := []string{}
	for key := range before {
		ids = append(ids, key)
	}
	sort.Strings(ids)
	for _, key := range ids {
		c, exists := after[key]
		if exists && reflect.DeepEqual(c, before[key]) {
			continue
		}
		kind, value := modelcore.ChangeUpdate, any(c)
		if !exists {
			kind, value = modelcore.ChangeDelete, nil
		}
		change, _ := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: key, SlotID: "assembly-constraint.entity"}, before[key], value)
		changes = append(changes, change)
	}
	return changes
}

func applyAssemblyGroupCreate(raw json.RawMessage, c AssemblyConstraint) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if c.Name == "" {
		c.Name = "FixTogether_" + parameterKeyFragment(c.ID)
	}
	c.Kind, c.Family, c.DefinitionVersion = "FIX_TOGETHER", "FixTogether", 2
	c.GroupMembers = canonicalGroupMembers(c.GroupMembers)
	c.GroupRelations, c.GroupCaptureDigest, c.GroupCapturePending = nil, "", true
	c.EvaluationStatus, c.EvaluationSummary = modelcore.AssemblyConstraintNotUpdated, "group awaits inner and outer authoritative solve"
	for _, existing := range model.Constraints {
		if existing.ID == c.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate GroupId", ErrValidation)
		}
	}
	model.Constraints = append(model.Constraints, c)
	if _, err := assemblyGroupMembers(model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, nil, c)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(c.ID)}}, nil
}

func applyAssemblyGroupEdit(raw json.RawMessage, payload editAssemblyConstraintPayload) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index := range model.Constraints {
		c := &model.Constraints[index]
		if c.ID != payload.ConstraintID || !isAssemblyGroup(*c) {
			continue
		}
		before := *c
		if payload.GroupName != nil {
			c.Name = *payload.GroupName
		}
		if payload.GroupMembers != nil && !reflect.DeepEqual(canonicalGroupMembers(c.GroupMembers), canonicalGroupMembers(*payload.GroupMembers)) {
			c.GroupMembers = canonicalGroupMembers(*payload.GroupMembers)
			c.GroupCapturePending = true
		}
		if payload.Mode != nil {
			c.Mode = *payload.Mode
		}
		if payload.First != nil || payload.Second != nil || payload.QuantityExpression != nil || payload.OffsetExpression != nil || payload.QuantityKey != "" || payload.OffsetKey != "" || payload.FixedPose != nil || payload.AngleAxis != nil || payload.Value != 0 || payload.DirectionRelation != "" || payload.DistanceRelation != "" || payload.ContactKind != "" || payload.ContactSide != "" || payload.ContactBranch != nil || payload.AngleRelation != "" || payload.FixMode != "" || (payload.Family != "" && payload.Family != "FixTogether") || payload.Subtype != "" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: invalid binary/Quantity group edit", ErrValidation)
		}
		c.EvaluationStatus, c.EvaluationSummary = modelcore.AssemblyConstraintNotUpdated, "group definition changed; awaiting staged solve"
		if _, err := assemblyGroupMembers(model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, before, *c)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(c.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: group does not exist", ErrValidation)
}

func canonicalGroupMembers(members []AssemblyGroupMember) []AssemblyGroupMember {
	result := append([]AssemblyGroupMember(nil), members...)
	sort.Slice(result, func(i, j int) bool { return resolvedDigest(result[i]) < resolvedDigest(result[j]) })
	return result
}

func deduplicateGroupMembers(members []AssemblyGroupMember) []AssemblyGroupMember {
	result := []AssemblyGroupMember{}
	seen := map[string]bool{}
	for _, m := range canonicalGroupMembers(members) {
		unit, _ := groupMemberUnit(m)
		key := m.GroupID + ":" + unit
		if !seen[key] {
			result = append(result, m)
			seen[key] = true
		}
	}
	return result
}

func applyAssemblyGroupDelete(raw json.RawMessage, id string) (json.RawMessage, modelcore.ChangeSet, error) {
	var model ProductModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	var selected *AssemblyConstraint
	for i := range model.Constraints {
		if model.Constraints[i].ID == id && isAssemblyGroup(model.Constraints[i]) {
			selected = &model.Constraints[i]
			break
		}
	}
	if selected == nil {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: group does not exist", ErrValidation)
	}
	before := map[string]AssemblyConstraint{}
	for _, c := range model.Constraints {
		before[c.ID] = c
	}
	replacement := append([]AssemblyGroupMember(nil), selected.GroupMembers...)
	constraints := []AssemblyConstraint{}
	for _, c := range model.Constraints {
		if c.ID == id {
			continue
		}
		if isAssemblyGroup(c) {
			members := []AssemblyGroupMember{}
			changed := false
			for _, m := range c.GroupMembers {
				if m.GroupID == id {
					members = append(members, replacement...)
					changed = true
				} else {
					members = append(members, m)
				}
			}
			if changed {
				unique := map[string]bool{}
				dedup := []AssemblyGroupMember{}
				for _, m := range members {
					unit, _ := groupMemberUnit(m)
					key := m.GroupID + ":" + unit
					if !unique[key] {
						dedup = append(dedup, m)
						unique[key] = true
					}
				}
				c.GroupMembers, c.GroupCapturePending = canonicalGroupMembers(dedup), true
				c.EvaluationStatus, c.EvaluationSummary = modelcore.AssemblyConstraintNotUpdated, "nested group dissolved; stable members retained"
			}
		}
		constraints = append(constraints, c)
	}
	model.Constraints = constraints
	if _, err := assemblyGroupMembers(model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	deleted, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: id, SlotID: "assembly-constraint.entity"}, before[id], nil)
	set := modelcore.ChangeSet{Changes: []modelcore.ModelChange{deleted}, ImpactSeeds: []modelcore.DependencyKey{"assembly-constraint:" + modelcore.DependencyKey(id)}}
	for _, c := range constraints {
		if !reflect.DeepEqual(c, before[c.ID]) {
			change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: c.ID, SlotID: "assembly-constraint.entity"}, before[c.ID], c)
			set.Changes = append(set.Changes, change)
			set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("assembly-constraint:"+c.ID))
		}
	}
	next, _ := json.Marshal(model)
	return next, set, nil
}

func prepareAssemblyGroupStages(manifest AssemblySolveManifest, excluded map[string]bool) ([]AssemblyGroupStage, error) {
	model := ProductModel{Constraints: manifest.Definitions}
	for _, b := range manifest.Bodies {
		model.Instances = append(model.Instances, ProductInstance{ID: b.ID})
	}
	members, err := assemblyGroupMembers(model)
	if err != nil {
		return nil, err
	}
	active := []AssemblyConstraint{}
	for _, c := range model.Constraints {
		if isAssemblyGroup(c) && !c.Suppressed && !excluded[c.ID] {
			active = append(active, c)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	stages := []AssemblyGroupStage{}
	for _, c := range active {
		stage := AssemblyGroupStage{ID: "group-stage:" + c.ID, MemberIDs: append([]string(nil), members[c.ID]...), Groups: []AssemblyFrozenGroup{{GroupID: c.ID, MemberIDs: append([]string(nil), members[c.ID]...)}}}
		// Merge the overlap closure, independent of declaration/member order.
		for i := 0; i < len(stages); {
			intersects := false
			for _, a := range stage.MemberIDs {
				for _, b := range stages[i].MemberIDs {
					if a == b {
						intersects = true
					}
				}
			}
			if !intersects {
				i++
				continue
			}
			stage.MemberIDs = append(stage.MemberIDs, stages[i].MemberIDs...)
			stage.Groups = append(stage.Groups, stages[i].Groups...)
			stages = append(stages[:i], stages[i+1:]...)
		}
		stage.MemberIDs = uniqueGroupIDs(stage.MemberIDs)
		sort.Slice(stage.Groups, func(i, j int) bool { return stage.Groups[i].GroupID < stage.Groups[j].GroupID })
		stage.ID = "group-stage:" + stage.Groups[0].GroupID
		stages = append(stages, stage)
	}
	byID := map[string]AssemblyConstraint{}
	numeric := map[string]geometry.AssemblyConstraint{}
	for _, c := range model.Constraints {
		byID[c.ID] = c
	}
	for _, c := range manifest.Constraints {
		numeric[c.ID] = c
	}
	for i := range stages {
		stage := &stages[i]
		set := stringSet(stage.MemberIDs)
		for _, c := range manifest.Constraints {
			if c.ID != "interaction-driver" && groupContainsConstraint(set, c.FirstBodyID, c.SecondBodyID, c.AngleReferenceBodyID) {
				stage.InternalConstraintIDs = append(stage.InternalConstraintIDs, c.ID)
			}
		}
		sort.Strings(stage.InternalConstraintIDs)
		for _, c := range model.Constraints {
			if isAssemblyGroup(c) || c.Suppressed || excluded[c.ID] {
				continue
			}
			second := ""
			if c.Second != nil {
				second = c.Second.InstanceID
			}
			third := ""
			if c.AngleAxis != nil {
				third = c.AngleAxis.InstanceID
			}
			if groupContainsConstraint(set, c.First.InstanceID, second, third) && numeric[c.ID].ID == "" {
				stage.UnresolvedInternalIDs = append(stage.UnresolvedInternalIDs, c.ID)
			}
		}
		sort.Strings(stage.UnresolvedInternalIDs)
		for j := range stage.Groups {
			group := &stage.Groups[j]
			c := byID[group.GroupID]
			group.InputDigest = assemblyGroupInputDigest(manifest, group.MemberIDs)
			group.CapturePending = c.GroupCapturePending || c.GroupCaptureDigest != group.InputDigest || len(c.GroupRelations) != len(group.MemberIDs)
			group.CapturedRelations = append([]AssemblyGroupRelation(nil), c.GroupRelations...)
		}
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i].ID < stages[j].ID })
	return stages, nil
}

func assemblyGroupInputDigest(manifest AssemblySolveManifest, members []string) string {
	own := stringSet(members)
	inputs := []geometry.AssemblyConstraint{}
	supportIDs := map[string]bool{}
	for _, primitive := range manifest.Constraints {
		if primitive.ID == "interaction-driver" || !groupContainsConstraint(own, primitive.FirstBodyID, primitive.SecondBodyID, primitive.AngleReferenceBodyID) {
			continue
		}
		inputs = append(inputs, primitive)
		supportIDs[primitive.FirstGeometryID], supportIDs[primitive.SecondGeometryID] = true, true
		if primitive.AngleReferenceGeometryID != "" {
			supportIDs[primitive.AngleReferenceGeometryID] = true
		}
	}
	descriptors := []geometry.AssemblyGeometry{}
	for _, descriptor := range manifest.Geometry {
		if supportIDs[descriptor.ID] {
			descriptors = append(descriptors, descriptor)
		}
	}
	input := struct {
		Members     []string
		Constraints []geometry.AssemblyConstraint
		Geometry    []geometry.AssemblyGeometry
	}{members, inputs, descriptors}
	// Like frozen manifests, this newly introduced identity must survive JSONB
	// numeric normalization; -0 is not a different physical group input.
	raw, _ := json.Marshal([]any{input})
	return assemblyManifestCanonicalJSON + ":" + canonicalAssemblyManifestJSONDigest(raw)
}

func uniqueGroupIDs(values []string) []string {
	set := stringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func stringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, v := range values {
		set[v] = true
	}
	return set
}
func groupContainsConstraint(members map[string]bool, first, second string, additional ...string) bool {
	if !members[first] || (second != "" && !members[second]) {
		return false
	}
	for _, body := range additional {
		if body != "" && !members[body] {
			return false
		}
	}
	return true
}

func groupRigidConstraints(groupID string, members []string, relations []AssemblyGroupRelation) ([]geometry.AssemblyConstraint, error) {
	if len(members) < 2 || len(relations) != len(members) {
		return nil, fmt.Errorf("%w: incomplete captured group relation", ErrValidation)
	}
	byID := map[string]InstancePose{}
	for _, relation := range relations {
		if byID[relation.InstanceID].Rotation != ([4]float64{}) {
			return nil, fmt.Errorf("%w: duplicate captured group member", ErrValidation)
		}
		if err := validateInstanceConstraintReferences(AssemblyConstraint{ID: "group-pose", Kind: "FIX", First: AssemblyGeometryRef{InstanceID: relation.InstanceID, Kind: "BODY"}, FixedPose: &relation.RelativePose}); err != nil {
			return nil, err
		}
		byID[relation.InstanceID] = relation.RelativePose
	}
	anchor, ok := byID[members[0]]
	if !ok {
		return nil, fmt.Errorf("%w: group anchor missing", ErrValidation)
	}
	result := []geometry.AssemblyConstraint{}
	for _, id := range members[1:] {
		member, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: group member missing", ErrValidation)
		}
		relative := inverseRelativePose(member, anchor)
		pose := geometry.AssemblyPose{Translation: relative.Translation, Rotation: relative.Rotation}
		result = append(result, geometry.AssemblyConstraint{ID: groupID + "/member/" + id, GroupID: groupID, ConnectionID: groupID, Kind: "RIGID", Mode: "DRIVING", FirstBodyID: id, SecondBodyID: members[0], FixedPose: &pose})
	}
	return result, nil
}

// No database, topology or Head access here: all inner and outer calls consume
// the exact frozen manifest. Failed stages never become captured definitions.
func (service *Service) solveFrozenAssemblyGroups(ctx context.Context, requestID string, manifest AssemblySolveManifest, capture func([]byte, error)) (geometry.AssemblySolve, error) {
	if err := validateFrozenAssemblyGroupStages(manifest); err != nil {
		return geometry.AssemblySolve{}, err
	}
	bodies := append([]geometry.AssemblyBody(nil), manifest.Bodies...)
	outer := append([]geometry.AssemblyConstraint(nil), manifest.Constraints...)
	evidence := []geometry.AssemblyGroupSolveEvidence{}
	for _, stage := range manifest.GroupStages {
		if len(stage.UnresolvedInternalIDs) > 0 {
			return geometry.AssemblySolve{Status: "UNSATISFIED", Diagnostic: "GROUP_INTERNAL_UNRESOLVED", Diagnostics: []geometry.AssemblySolveDiagnostic{{Code: "GROUP_INTERNAL_UNRESOLVED", ConstraintIDs: stage.UnresolvedInternalIDs, Detail: "internal definitions could not resolve; no group relationship captured"}}}, nil
		}
		members := stringSet(stage.MemberIDs)
		innerBodies := []geometry.AssemblyBody{}
		innerGeometry := []geometry.AssemblyGeometry{}
		inner := []geometry.AssemblyConstraint{}
		for _, b := range bodies {
			if members[b.ID] {
				b.InitialGuess = nil
				innerBodies = append(innerBodies, b)
			}
		}
		if len(innerBodies) != len(stage.MemberIDs) {
			return geometry.AssemblySolve{}, fmt.Errorf("%w: frozen group member body missing", ErrValidation)
		}
		for _, g := range manifest.Geometry {
			if members[g.BodyID] {
				innerGeometry = append(innerGeometry, g)
			}
		}
		internal := stringSet(stage.InternalConstraintIDs)
		for _, c := range manifest.Constraints {
			if internal[c.ID] {
				inner = append(inner, c)
			}
		}
		if len(inner) != len(internal) {
			return geometry.AssemblySolve{}, fmt.Errorf("%w: frozen group internal constraint mapping missing", ErrValidation)
		}
		for _, group := range stage.Groups {
			if group.CapturePending {
				continue
			}
			links, err := groupRigidConstraints(group.GroupID, group.MemberIDs, group.CapturedRelations)
			if err != nil {
				return geometry.AssemblySolve{}, err
			}
			inner = append(inner, links...)
		}
		result, err := service.worker.SolveAssemblyWithOptions(ctx, requestID+"/"+stage.ID, innerBodies, innerGeometry, inner, geometry.AssemblySolveOptions{AffectedBodyIDs: stage.MemberIDs, SolverProfile: &manifest.SolverProfile, DisableConflictProbes: manifest.Purpose == "DIAGNOSTIC"})
		if err != nil {
			return result, err
		}
		if result.Status != "CONVERGED" {
			for _, group := range stage.Groups {
				evidence = append(evidence, geometry.AssemblyGroupSolveEvidence{GroupID: group.GroupID, InputDigest: group.InputDigest, MemberIDs: append([]string(nil), group.MemberIDs...), InternalConstraintIDs: append([]string(nil), stage.InternalConstraintIDs...), StageStatus: result.Status, StageBuild: result.SolverBuild, StageResultDigest: resolvedDigest(result)})
			}
			result.GroupEvidence = evidence
			result.Diagnostic = "GROUP_INTERNAL_FAILED: " + result.Diagnostic
			return result, nil
		}
		if err := validateAssemblyPreference(result); err != nil {
			result.Status, result.Diagnostic = "MAX_ITERATIONS", "GROUP_INTERNAL_PREFERENCE_NOT_CONVERGED: "+err.Error()
			for _, group := range stage.Groups {
				evidence = append(evidence, geometry.AssemblyGroupSolveEvidence{GroupID: group.GroupID, InputDigest: group.InputDigest, MemberIDs: append([]string(nil), group.MemberIDs...), InternalConstraintIDs: append([]string(nil), stage.InternalConstraintIDs...), StageStatus: result.Status, StageBuild: result.SolverBuild, StageResultDigest: resolvedDigest(result)})
			}
			result.GroupEvidence = evidence
			return result, nil
		}
		accepted := map[string]geometry.AssemblyPose{}
		for _, b := range result.Bodies {
			accepted[b.ID] = b.Pose
		}
		for i := range bodies {
			if p, ok := accepted[bodies[i].ID]; ok {
				bodies[i].Pose = p
				bodies[i].InitialGuess = nil
			}
		}
		for _, group := range stage.Groups {
			relations := append([]AssemblyGroupRelation(nil), group.CapturedRelations...)
			if group.CapturePending {
				anchor, ok := accepted[group.MemberIDs[0]]
				if !ok {
					return result, fmt.Errorf("%w: inner solver omitted group anchor", ErrValidation)
				}
				relations = nil
				for _, id := range group.MemberIDs {
					p, ok := accepted[id]
					if !ok {
						return result, fmt.Errorf("%w: inner solver omitted group member", ErrValidation)
					}
					relative := inverseRelativePose(InstancePose{Translation: p.Translation, Rotation: p.Rotation}, InstancePose{Translation: anchor.Translation, Rotation: anchor.Rotation})
					relations = append(relations, AssemblyGroupRelation{InstanceID: id, RelativePose: relative})
				}
			}
			links, err := groupRigidConstraints(group.GroupID, group.MemberIDs, relations)
			if err != nil {
				return result, err
			}
			outer = append(outer, links...)
			proof := geometry.AssemblyGroupSolveEvidence{GroupID: group.GroupID, InputDigest: group.InputDigest, MemberIDs: append([]string(nil), group.MemberIDs...), InternalConstraintIDs: append([]string(nil), stage.InternalConstraintIDs...), StageStatus: result.Status, StageBuild: result.SolverBuild, StageResultDigest: resolvedDigest(result), CompiledConstraints: links}
			for _, r := range relations {
				proof.Relations = append(proof.Relations, geometry.AssemblyGroupSolvedRelation{InstanceID: r.InstanceID, RelativePose: geometry.AssemblyPose{Translation: r.RelativePose.Translation, Rotation: r.RelativePose.Rotation}})
			}
			evidence = append(evidence, proof)
		}
	}
	result, err := service.worker.SolveAssemblyWithOptions(ctx, requestID, bodies, manifest.Geometry, outer, geometry.AssemblySolveOptions{Intent: manifest.Intent, AffectedBodyIDs: manifest.AffectedBodyIDs, SolverProfile: &manifest.SolverProfile, CaptureReplay: capture, DisableConflictProbes: manifest.Purpose == "DIAGNOSTIC"})
	result.GroupEvidence = evidence
	return result, err
}

func promoteAssemblyGroupEvidence(model *ProductModel, result geometry.AssemblySolve) {
	if result.Status != "CONVERGED" || validateAssemblyPreference(result) != nil {
		return
	}
	for _, proof := range result.GroupEvidence {
		for i := range model.Constraints {
			c := &model.Constraints[i]
			if c.ID != proof.GroupID || !isAssemblyGroup(*c) {
				continue
			}
			c.GroupRelations = nil
			for _, r := range proof.Relations {
				c.GroupRelations = append(c.GroupRelations, AssemblyGroupRelation{InstanceID: r.InstanceID, RelativePose: InstancePose{Translation: r.RelativePose.Translation, Rotation: r.RelativePose.Rotation}})
			}
			c.GroupCaptureDigest, c.GroupCapturePending = proof.InputDigest, false
		}
	}
}

func validateFrozenAssemblyGroupStages(manifest AssemblySolveManifest) error {
	model := ProductModel{Constraints: manifest.Definitions}
	for _, b := range manifest.Bodies {
		model.Instances = append(model.Instances, ProductInstance{ID: b.ID})
	}
	members, err := assemblyGroupMembers(model)
	if err != nil {
		return err
	}
	stages, groups, units := map[string]bool{}, map[string]bool{}, map[string]string{}
	for _, stage := range manifest.GroupStages {
		if stage.ID == "" || stages[stage.ID] || len(stage.Groups) == 0 || len(stage.MemberIDs) < 2 || !reflect.DeepEqual(stage.MemberIDs, uniqueGroupIDs(stage.MemberIDs)) {
			return fmt.Errorf("%w: invalid frozen group stage identity/membership", ErrValidation)
		}
		stages[stage.ID] = true
		for _, id := range stage.MemberIDs {
			if previous := units[id]; previous != "" {
				return fmt.Errorf("%w: overlapping groups were not merged into one inner stage", ErrValidation)
			}
			units[id] = stage.ID
		}
		set := stringSet(stage.MemberIDs)
		expected := []string{}
		for _, c := range manifest.Constraints {
			if c.ID != "interaction-driver" && groupContainsConstraint(set, c.FirstBodyID, c.SecondBodyID, c.AngleReferenceBodyID) {
				expected = append(expected, c.ID)
			}
		}
		sort.Strings(expected)
		if !reflect.DeepEqual(expected, stage.InternalConstraintIDs) && !(len(expected) == 0 && len(stage.InternalConstraintIDs) == 0) {
			return fmt.Errorf("%w: frozen group internal mapping does not match real primitives", ErrValidation)
		}
		for _, group := range stage.Groups {
			if groups[group.GroupID] || members[group.GroupID] == nil || !reflect.DeepEqual(members[group.GroupID], group.MemberIDs) || group.InputDigest != assemblyGroupInputDigest(manifest, group.MemberIDs) {
				return fmt.Errorf("%w: invalid frozen group definition mapping", ErrValidation)
			}
			groups[group.GroupID] = true
			for _, id := range group.MemberIDs {
				if !set[id] {
					return fmt.Errorf("%w: group member escaped its inner stage", ErrValidation)
				}
			}
		}
	}
	return nil
}

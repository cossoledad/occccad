package workspace

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func groupModelFixture() ProductModel {
	return ProductModel{Instances: []ProductInstance{{ID: "a", Name: "A", Rotation: [4]float64{0, 0, 0, 1}}, {ID: "b", Name: "B", Translation: [3]float64{3, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}, {ID: "c", Name: "C", Translation: [3]float64{0, 4, 0}, Rotation: [4]float64{0, 0, 0, 1}}}}
}
func groupDefinition(id string, members ...AssemblyGroupMember) AssemblyConstraint {
	return AssemblyConstraint{ID: id, Kind: "FIX_TOGETHER", Family: "FixTogether", Name: id, GroupMembers: members, Mode: "DRIVING"}
}
func groupRaw(t *testing.T, model ProductModel) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestFixTogetherThirdAxisDefinesInternalBoundaryAndCaptureInput(t *testing.T) {
	m := AssemblySolveManifest{Bodies: []geometry.AssemblyBody{{ID: "a"}, {ID: "b"}, {ID: "c"}}, Definitions: []AssemblyConstraint{groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"})}, Constraints: []geometry.AssemblyConstraint{{ID: "directed", Kind: "ANGLE", FirstBodyID: "a", SecondBodyID: "b", AngleReferenceBodyID: "c", AngleReferenceGeometryID: "third-axis"}}, Geometry: []geometry.AssemblyGeometry{{ID: "third-axis", BodyID: "c", Kind: "AXIS", Direction: [3]float64{0, 0, 1}}}}
	stages, err := prepareAssemblyGroupStages(m, nil)
	if err != nil || len(stages) != 1 || len(stages[0].InternalConstraintIDs) != 0 {
		t.Fatal("external third-axis relation was sent to member-only inner solve", stages, err)
	}
	before := assemblyGroupInputDigest(m, []string{"a", "b"})
	m.Geometry[0].Direction = [3]float64{0, 1, 0}
	if before != assemblyGroupInputDigest(m, []string{"a", "b"}) {
		t.Fatal("external axis changed captured internal definition")
	}
	m.Definitions[0].GroupMembers = append(m.Definitions[0].GroupMembers, AssemblyGroupMember{InstanceID: "c"})
	stages, err = prepareAssemblyGroupStages(m, nil)
	if err != nil || len(stages) != 1 || !reflect.DeepEqual(stages[0].InternalConstraintIDs, []string{"directed"}) {
		t.Fatal("member third axis was lost from internal solve", stages, err)
	}
	before = assemblyGroupInputDigest(m, []string{"a", "b", "c"})
	m.Geometry[0].Direction = [3]float64{1, 0, 0}
	if before == assemblyGroupInputDigest(m, []string{"a", "b", "c"}) {
		t.Fatal("internal axis geometry omitted from group capture identity")
	}
}

func TestFixTogetherFailedTrialCannotCaptureAndInvalidParametersAreAtomic(t *testing.T) {
	model := groupModelFixture()
	model.Constraints = []AssemblyConstraint{groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"})}
	model.Constraints[0].GroupCapturePending = true
	before := groupRaw(t, model)
	proof := geometry.AssemblyGroupSolveEvidence{GroupID: "g", InputDigest: "trial", Relations: []geometry.AssemblyGroupSolvedRelation{{InstanceID: "a", RelativePose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}}}}
	for _, result := range []geometry.AssemblySolve{
		{Status: "UNSATISFIED", GroupEvidence: []geometry.AssemblyGroupSolveEvidence{proof}},
		{Status: "CONVERGED", GroupEvidence: []geometry.AssemblyGroupSolveEvidence{proof}},
		{Status: "CONVERGED", Components: []geometry.AssemblyComponentDof{{Solved: true, Preference: geometry.AssemblyMotionPreference{Status: geometry.PreferenceIterationLimit, GeometricallyFeasible: true}}}, GroupEvidence: []geometry.AssemblyGroupSolveEvidence{proof}},
	} {
		promoteAssemblyGroupEvidence(&model, result)
		if !reflect.DeepEqual(before, groupRaw(t, model)) {
			t.Fatal("failed outer or preference trial promoted group capture", result)
		}
	}
	for _, edit := range []editAssemblyConstraintPayload{{QuantityKey: "invalid"}, {Family: "Offset"}, {Value: 4}, {FixedPose: &InstancePose{}}, {DirectionRelation: "SAME"}} {
		edit.ConstraintID = "g"
		output, changes, err := applyAssemblyGroupEdit(before, edit)
		if err == nil || output != nil || len(changes.Changes) != 0 {
			t.Fatal("unsupported group parameter silently accepted", edit, err)
		}
	}
}

func TestFixTogetherExplicitLegacyPairPromotionAndCompensation(t *testing.T) {
	model := groupModelFixture()
	pose := InstancePose{Translation: [3]float64{-3, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}
	old := AssemblyConstraint{ID: "legacy-pair", ConnectionID: "legacy-connection", Kind: "RIGID", Mode: "DRIVING", First: AssemblyGeometryRef{InstanceID: "a", Kind: "BODY"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "BODY"}, FixedPose: &pose, Suppressed: true}
	model.Constraints = []AssemblyConstraint{old}
	raw := groupRaw(t, model)
	payload, _ := json.Marshal(editAssemblyConstraintPayload{ConstraintID: old.ID, Family: "FixTogether"})
	next, changes, err := applyEditAssemblyConstraint(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var after ProductModel
	_ = json.Unmarshal(next, &after)
	c := after.Constraints[0]
	if c.ID != old.ID || c.ConnectionID != old.ConnectionID || c.Kind != "FIX_TOGETHER" || c.DefinitionVersion != 2 || !c.Suppressed || len(c.GroupMembers) != 2 || !c.GroupCapturePending || c.First.InstanceID != "" || c.Second != nil || c.FixedPose != nil {
		t.Fatal("explicit promotion lost identity or retained a parallel primitive", c)
	}
	values, err := modelValues("PRODUCT", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if !reflect.DeepEqual(undo.Constraints[0], old) {
		t.Fatal("Undo reinterpreted the legacy immutable definition", undo.Constraints[0])
	}
	if !reflect.DeepEqual(undo.Instances, model.Instances) {
		t.Fatal("promotion moved nominal poses without authoritative solve")
	}
}

func TestFixTogetherMembersNestingOrderingAndAtomicErrors(t *testing.T) {
	model := groupModelFixture()
	group := groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"}, AssemblyGroupMember{InstanceID: "c"})
	raw, set, err := applyAssemblyGroupCreate(groupRaw(t, model), group)
	if err != nil || len(set.Changes) != 1 {
		t.Fatal(err, set)
	}
	_ = json.Unmarshal(raw, &model)
	c := &model.Constraints[0]
	if c.ID != "g" || c.First.InstanceID != "" || c.Second != nil || c.DefinitionVersion != 2 || len(c.GroupMembers) != 3 || !c.GroupCapturePending {
		t.Fatal(c)
	}
	// Ordering changes neither group identity nor captured baseline.
	c.GroupCapturePending = false
	c.GroupCaptureDigest = "baseline"
	c.GroupRelations = []AssemblyGroupRelation{{InstanceID: "a", RelativePose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}}
	raw = groupRaw(t, model)
	reordered := []AssemblyGroupMember{c.GroupMembers[2], c.GroupMembers[1], c.GroupMembers[0]}
	next, _, err := applyAssemblyGroupEdit(raw, editAssemblyConstraintPayload{ConstraintID: "g", GroupMembers: &reordered})
	if err != nil {
		t.Fatal(err)
	}
	var same ProductModel
	_ = json.Unmarshal(next, &same)
	if same.Constraints[0].ID != "g" || same.Constraints[0].GroupCapturePending || !reflect.DeepEqual(same.Constraints[0].GroupRelations, c.GroupRelations) {
		t.Fatal(same)
	}
	parent := groupDefinition("parent", AssemblyGroupMember{GroupID: "g"}, AssemblyGroupMember{InstanceID: "a"})
	raw, _, err = applyAssemblyGroupCreate(raw, parent)
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(raw, &model)
	units, err := assemblyGroupMembers(model)
	if err != nil || !reflect.DeepEqual(units["parent"], []string{"a", "b", "c"}) {
		t.Fatal(units, err)
	}
	for _, members := range [][]AssemblyGroupMember{
		{{InstanceID: "a"}, {InstanceID: "a"}}, {{InstanceID: "a"}, {InstanceID: "missing"}}, {{GroupID: "parent"}, {InstanceID: "b"}}, {{InstanceID: "a"}},
	} {
		candidate, changes, err := applyAssemblyGroupEdit(raw, editAssemblyConstraintPayload{ConstraintID: "g", GroupMembers: &members})
		if err == nil || candidate != nil || len(changes.Changes) != 0 {
			t.Fatal("illegal/cyclic group edit committed", members, err)
		}
	}
	measured := "MEASURED"
	if _, _, err := applyAssemblyGroupEdit(raw, editAssemblyConstraintPayload{ConstraintID: "g", Mode: &measured}); err == nil {
		t.Fatal("group measurement incorrectly supported")
	}
}

func TestFixTogetherSuppressionDissolutionAndCompensation(t *testing.T) {
	model := groupModelFixture()
	internal := AssemblyConstraint{ID: "internal", Kind: "DISTANCE", Value: 3, First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}}
	model.Constraints = []AssemblyConstraint{internal}
	raw, _, err := applyAssemblyGroupCreate(groupRaw(t, model), groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"}))
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err = applyAssemblyGroupCreate(raw, groupDefinition("parent", AssemblyGroupMember{GroupID: "g"}, AssemblyGroupMember{InstanceID: "c"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, suppressed := range []bool{true, false} {
		p, _ := json.Marshal(assemblyConstraintStatePayload{ConstraintIDs: []string{"g"}, Suppressed: &suppressed})
		raw, _, err = applyAssemblyConstraintState(raw, p)
		if err != nil {
			t.Fatal(err)
		}
		var state ProductModel
		_ = json.Unmarshal(raw, &state)
		if state.Constraints[0].Suppressed || !reflect.DeepEqual(state.Constraints[0], internal) {
			t.Fatal("group suppression changed independent internal constraint", state)
		}
	}
	next, changes, err := applyAssemblyGroupDelete(raw, "g")
	if err != nil {
		t.Fatal(err)
	}
	var after ProductModel
	_ = json.Unmarshal(next, &after)
	if len(after.Constraints) != 2 || len(after.Constraints[1].GroupMembers) != 3 {
		t.Fatal(after)
	}
	for _, m := range after.Constraints[1].GroupMembers {
		if m.GroupID == "g" {
			t.Fatal("dissolution retained a dangling group identity")
		}
	}
	values, err := modelValues("PRODUCT", next, changes)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := changes.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if len(undo.Constraints) != 3 {
		t.Fatal("compensation did not restore dissolved group", undo)
	}
	units, err := assemblyGroupMembers(undo)
	if err != nil || len(units["parent"]) != 3 {
		t.Fatal(units, err)
	}
}

func TestFixTogetherStageBoundaryCaptureInvalidationAndGraph(t *testing.T) {
	model := groupModelFixture()
	model.Constraints = []AssemblyConstraint{
		groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"}),
		groupDefinition("overlap", AssemblyGroupMember{InstanceID: "b"}, AssemblyGroupMember{InstanceID: "c"}),
		{ID: "inner", Kind: "DISTANCE", Value: 3, First: AssemblyGeometryRef{InstanceID: "a", Kind: "POINT"}, Second: &AssemblyGeometryRef{InstanceID: "b", Kind: "POINT"}},
	}
	body := []geometry.AssemblyBody{}
	for _, i := range model.Instances {
		body = append(body, geometry.AssemblyBody{ID: i.ID, Pose: geometry.AssemblyPose{Translation: i.Translation, Rotation: i.Rotation}})
	}
	primitive := geometry.AssemblyConstraint{ID: "inner", Kind: "DISTANCE", FirstBodyID: "a", FirstGeometryID: "pa", SecondBodyID: "b", SecondGeometryID: "pb", Value: 3}
	manifest, err := newAssemblySolveManifest("product", "revision", "hash", body, []geometry.AssemblyGeometry{{ID: "pa", BodyID: "a", Kind: "POINT"}, {ID: "pb", BodyID: "b", Kind: "POINT"}}, []geometry.AssemblyConstraint{primitive}, nil, nil, nil, model.Constraints)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := prepareAssemblyGroupStages(manifest, nil)
	if err != nil || len(stages) != 1 || len(stages[0].Groups) != 2 || !reflect.DeepEqual(stages[0].MemberIDs, []string{"a", "b", "c"}) || !reflect.DeepEqual(stages[0].InternalConstraintIDs, []string{"inner"}) {
		t.Fatal(stages, err)
	}
	manifest.GroupStages = stages
	if err := validateFrozenAssemblyGroupStages(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.GroupStages[0].Groups[0].InputDigest = "forged"
	if err := validateFrozenAssemblyGroupStages(manifest); err == nil {
		t.Fatal("frozen input identity was not checked against real primitives")
	}
	manifest.GroupStages[0].Groups[0].InputDigest = assemblyGroupInputDigest(manifest, stages[0].Groups[0].MemberIDs)
	for i := range manifest.Definitions {
		c := &manifest.Definitions[i]
		if !isAssemblyGroup(*c) {
			continue
		}
		for _, g := range stages[0].Groups {
			if c.ID != g.GroupID {
				continue
			}
			c.GroupCaptureDigest = g.InputDigest
			c.GroupCapturePending = false
			for _, id := range g.MemberIDs {
				c.GroupRelations = append(c.GroupRelations, AssemblyGroupRelation{InstanceID: id, RelativePose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}})
			}
		}
	}
	manifest.Bodies[1].Pose.Translation = [3]float64{99, 72, -3}
	stable, err := prepareAssemblyGroupStages(manifest, nil)
	if err != nil || stable[0].Groups[0].CapturePending || stable[0].Groups[1].CapturePending {
		t.Fatal("nominal placement/read silently recaptured relationships", stable, err)
	}
	manifest.Constraints[0].Value = 4
	changed, err := prepareAssemblyGroupStages(manifest, nil)
	if err != nil || !changed[0].Groups[0].CapturePending || changed[0].Groups[1].CapturePending {
		t.Fatal("inner parameter invalidation was not scoped to its group", changed, err)
	}
	graph, _, err := buildProductEvaluation(model, "revision", "hash", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closure := graph.DirtyClosure([]modelcore.DependencyKey{"assembly-constraint:inner"})
	found := false
	for _, key := range closure {
		if key == "assembly-constraint:g" {
			found = true
		}
	}
	if !found {
		t.Fatal("group capture missed internal constraint dependency", closure)
	}
	manifest.GroupStages[0].InternalConstraintIDs = []string{"missing"}
	if err := validateFrozenAssemblyGroupStages(manifest); err == nil {
		t.Fatal("invalid frozen stage mapping accepted")
	}
}

func TestFixTogetherRigidCompilationHasStableIdentityAndRelativePose(t *testing.T) {
	half := math.Sqrt(.5)
	relations := []AssemblyGroupRelation{{InstanceID: "a", RelativePose: InstancePose{Rotation: [4]float64{0, 0, 0, 1}}}, {InstanceID: "b", RelativePose: InstancePose{Translation: [3]float64{2, 3, 4}, Rotation: [4]float64{0, 0, half, half}}}, {InstanceID: "c", RelativePose: InstancePose{Translation: [3]float64{-5, 1, 3}, Rotation: [4]float64{0, 0, 0, 1}}}}
	links, err := groupRigidConstraints("group", []string{"a", "b", "c"}, relations)
	if err != nil || len(links) != 2 {
		t.Fatal(links, err)
	}
	for _, c := range links {
		if c.GroupID != "group" || !strings.HasPrefix(c.ID, "group/member/") || c.Kind != "RIGID" || c.SecondBodyID != "a" {
			t.Fatal(c)
		}
	}
	if links[0].FixedPose.Translation != relations[1].RelativePose.Translation {
		t.Fatal("relative pose compilation changed baseline", links)
	}
}

func TestFixTogetherMemberDeletionDissolvesDependentGroupsAndCompensates(t *testing.T) {
	model := groupModelFixture()
	model.Constraints = []AssemblyConstraint{
		groupDefinition("g", AssemblyGroupMember{InstanceID: "a"}, AssemblyGroupMember{InstanceID: "b"}),
		groupDefinition("parent", AssemblyGroupMember{GroupID: "g"}, AssemblyGroupMember{InstanceID: "a"}),
	}
	raw := groupRaw(t, model)
	payload, _ := json.Marshal(deleteNodePayload{TargetKind: "INSTANCE", TargetID: "b"})
	next, set, err := applyDeleteProductNode(raw, payload)
	if err != nil {
		t.Fatal(err)
	}
	var after ProductModel
	_ = json.Unmarshal(next, &after)
	if len(after.Instances) != 2 || len(after.Constraints) != 0 || len(set.Changes) != 3 {
		t.Fatal("member deletion left a degenerate/dangling group", after, set)
	}
	values, err := modelValues("PRODUCT", next, set)
	if err != nil {
		t.Fatal(err)
	}
	desired, err := set.Compensate(values)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := applyModelValues("PRODUCT", next, desired)
	if err != nil {
		t.Fatal(err)
	}
	var undo ProductModel
	_ = json.Unmarshal(restored, &undo)
	if len(undo.Instances) != 3 || len(undo.Constraints) != 2 {
		t.Fatal(undo)
	}
	if _, err := assemblyGroupMembers(undo); err != nil {
		t.Fatal("Undo restored invalid nested groups", err)
	}
}

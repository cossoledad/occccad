package control

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

// Authority is the versioned profile frozen alongside the tested command, not
// an independently hand-picked epsilon. LengthTolerance is already physical
// millimetres and AngleTolerance radians. LengthScale conditions equations and
// preferences; it must NOT be used to loosen these physical acceptance bounds.
func compositionAcceptanceProfile(t *testing.T, f *compositionControlFixture, revision string) geometry.AssemblySolverProfile {
	t.Helper()
	var raw []byte
	if e := f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, revision).Scan(&raw); e != nil {
		t.Fatal("frozen acceptance profile", e)
	}
	var manifest workspace.AssemblySolveManifest
	if e := json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	p := manifest.SolverProfile
	if p.LengthTolerance <= 0 || p.AngleTolerance <= 0 || math.IsNaN(p.LengthTolerance) || math.IsNaN(p.AngleTolerance) || math.IsInf(p.LengthTolerance, 0) || math.IsInf(p.AngleTolerance, 0) {
		t.Fatal("missing physical tolerance contract", p)
	}
	return p
}

func TestCompositionIndependentConstrainedThirdAxisThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	product, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Independent constrained third axis"})
	if e != nil {
		t.Fatal(e)
	}
	id, seq := product.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-third-axis-%d", id, seq)
		var err error
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	for _, name := range []string{"Moving", "Reference", "Axis carrier"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	a, b, c := product.Product.Instances[0].ID, product.Product.Instances[1].ID, product.Product.Instances[2].ID
	qref := [4]float64{0, math.Sin(.17), 0, math.Cos(.17)}
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{7, -11, 5}, Rotation: qref})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: c, Translation: [3]float64{31, -8, 17}, Rotation: [4]float64{0, 0, math.Sin(.3), math.Cos(.3)}})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{19, -7, 24}, Rotation: [4]float64{math.Sin(.2), 0, 0, math.Cos(.2)}})
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "BODY"}})
	carrier := workspace.AssemblyGeometryRef{InstanceID: c, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-axis-x"}
	referenceAxis := carrier
	referenceAxis.InstanceID = b
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", AngleRelation: "PARALLEL", FirstAssemblyRef: &carrier, SecondAssemblyRef: &referenceAxis, DirectionRelation: "SAME"})
	carrierConstraint := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	first, second := f.support(t, part, "CYLINDER"), f.support(t, part, "PLANE")
	first.InstanceID, second.InstanceID = a, b
	first.DerivedRole = "cylinder-axis"
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", AngleRelation: "DIRECTED", FirstAssemblyRef: &first, SecondAssemblyRef: &second, AngleAxis: &carrier, Value: .7})
	target := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	check := func(value float64) {
		t.Helper()
		profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
		inspected, err := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{first, second, carrier})
		if err != nil || len(inspected.Supports) != 3 {
			t.Fatal(err)
		}
		poses := map[string]workspace.ProductInstance{}
		for _, p := range product.Product.Instances {
			poses[p.ID] = p
		}
		var directions [3][3]float64
		for i, ref := range []workspace.AssemblyGeometryRef{first, second, carrier} {
			if inspected.Supports[i].Descriptor == nil {
				t.Fatal(inspected.Supports[i])
			}
			directions[i] = compositionRotate(poses[ref.InstanceID].Rotation, inspected.Supports[i].Descriptor.Direction)
		}
		u, v, k := directions[0], directions[1], directions[2]
		uk, vk := compositionDot(u, k), compositionDot(v, k)
		for i := range u {
			u[i] -= uk * k[i]
			v[i] -= vk * k[i]
		}
		if compositionNorm(u) < 1e-8 || compositionNorm(v) < 1e-8 {
			t.Fatal("undefined projected-angle fixture")
		}
		cross := [3]float64{u[1]*v[2] - u[2]*v[1], u[2]*v[0] - u[0]*v[2], u[0]*v[1] - u[1]*v[0]}
		observed := math.Atan2(compositionDot(k, cross), compositionDot(u, v))
		error := math.Atan2(math.Sin(observed-value), math.Cos(observed-value))
		if math.Abs(error) > profile.AngleTolerance {
			t.Fatal("independent third-axis angle", observed, value, profile.AngleTolerance)
		}
	}
	check(.7)
	var raw []byte
	var digest string
	if e = f.db.QueryRow(t.Context(), `SELECT manifest,digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&raw, &digest); e != nil {
		t.Fatal(e)
	}
	var manifest workspace.AssemblySolveManifest
	if e = json.Unmarshal(raw, &manifest); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, primitive := range manifest.Constraints {
		if primitive.ID == target {
			found = true
			if primitive.AngleReferenceBodyID != c || primitive.AngleReferenceDirection != nil {
				t.Fatal("third source collapsed to stale vector or binary endpoint", primitive)
			}
		}
	}
	if !found {
		t.Fatal("directed primitive absent")
	}
	replay, e := f.service.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-third-replay")
	if e != nil || replay.Status != "CONVERGED" || len(replay.Result.Components) != 1 || replay.Result.Components[0].TangentVariableCount != 12 || replay.Result.Components[0].JacobianRank != 3 || replay.Result.Components[0].RelativeDof != 9 {
		t.Fatal("constrained third axis rank/variable participation", e, replay)
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{carrierConstraint}, Suppressed: &suppressed})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: c, Translation: [3]float64{31, -8, 17}, Rotation: [4]float64{0, 0, math.Sin(.2), math.Cos(.2)}})
	check(.7)
	suppressed = false
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{carrierConstraint}, Suppressed: &suppressed})
	check(.7)
	// A modern DIRECTED constraint is ternary for stage ownership even
	// though its primary support relation has two endpoints. An external
	// reference axis must never be sent to a two-member inner solve.
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}})
	groupID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	checkStage := func(internal bool) {
		t.Helper()
		check(.7)
		for _, definition := range product.Product.Constraints {
			if !definition.Suppressed && definition.EvaluationStatus != "VERIFIED" {
				t.Fatal("third-axis staged definition not accepted", definition)
			}
		}
		var raw []byte
		var digest string
		if err := f.db.QueryRow(t.Context(), `SELECT manifest,digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&raw, &digest); err != nil {
			t.Fatal(err)
		}
		var frozen workspace.AssemblySolveManifest
		if err := json.Unmarshal(raw, &frozen); err != nil {
			t.Fatal(err)
		}
		if len(frozen.GroupStages) != 1 {
			t.Fatal("missing deterministic group stage", frozen.GroupStages)
		}
		found := false
		for _, id := range frozen.GroupStages[0].InternalConstraintIDs {
			found = found || id == target
		}
		if found != internal {
			t.Fatal("third axis misclassified inner/outer", internal, frozen.GroupStages[0])
		}
		var thirdGeometry string
		for _, primitive := range frozen.Constraints {
			if primitive.ID == target {
				thirdGeometry = primitive.AngleReferenceGeometryID
			}
		}
		thirdFrozen := false
		for _, descriptor := range frozen.Geometry {
			thirdFrozen = thirdFrozen || descriptor.ID == thirdGeometry && descriptor.BodyID == c
		}
		if thirdGeometry == "" || !thirdFrozen {
			t.Fatal("third descriptor not frozen for replay", thirdGeometry, frozen.Geometry)
		}
		r, err := f.service.ReplayAssemblySolveManifest(t.Context(), id, digest, fmt.Sprintf("%s-third-group-replay-%d", id, seq))
		if err != nil || r.Status != "CONVERGED" || len(r.Result.GroupEvidence) != 1 {
			t.Fatal("ternary group frozen replay", err, r)
		}
	}
	checkStage(false)
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: groupID, GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}, {InstanceID: c}}})
	checkStage(true)
}

// Both Fix modes capture in the immediate owning Product. Changing a parent
// occurrence is not an explicit move of the child, and must not recapture it.
func TestCompositionNestedOwningProductFixThroughRouter(t *testing.T) {
	for _, mode := range []string{"SPACE", "RELATIVE"} {
		t.Run(mode, func(t *testing.T) {
			f := newCompositionControlFixture(t)
			part := f.importPart(t, "cylinder-r6-h12")
			child, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Child owning fix " + mode})
			if e != nil {
				t.Fatal(e)
			}
			seq := 0
			command := func(id string, r workspace.CommandRequest) workspace.DocumentView {
				t.Helper()
				seq++
				r.ActorID = f.actor
				r.RequestID = fmt.Sprintf("%s-owning-fix-%d", id, seq)
				v, err := f.service.ApplyCommand(t.Context(), id, r)
				if err != nil {
					t.Fatal(r.Type, err)
				}
				return v
			}
			child = command(child.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			leaf := child.Product.Instances[0].ID
			child = command(child.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{4, -3, 9}, Rotation: [4]float64{0, math.Sin(.23), 0, math.Cos(.23)}})
			child = command(child.Document.ID, workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FixMode: mode, FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: leaf, Kind: "BODY"}})
			if mode == "RELATIVE" {
				child = command(child.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: leaf, Translation: [3]float64{6, -2, 11}, Rotation: [4]float64{0, math.Sin(.31), 0, math.Cos(.31)}})
			}
			captured, childPose := *child.Product.Constraints[0].FixedPose, child.Product.Instances[0]
			captureProfile := compositionAcceptanceProfile(t, f, child.Document.VersionID)
			if captured.Translation != childPose.Translation || captured.Rotation != childPose.Rotation || child.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
				t.Fatal("local explicit capture", captured, childPose)
			}
			parent, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Outer placement " + mode})
			if e != nil {
				t.Fatal(e)
			}
			parent = command(parent.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: child.Document.ID})
			outer := parent.Product.Instances[0].ID
			for _, theta := range []float64{.4, .8} {
				parent = command(parent.Document.ID, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: outer, Translation: [3]float64{31, -14, 7}, Rotation: [4]float64{math.Sin(theta / 2), 0, 0, math.Cos(theta / 2)}})
				cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
				read, err := cold.GetDocument(t.Context(), child.Document.ID)
				if err != nil || read.Document.VersionID != child.Document.VersionID || !reflect.DeepEqual(read.Product.Constraints[0].FixedPose, &captured) || !reflect.DeepEqual(read.Product.Instances[0], childPose) {
					t.Fatal("outer move recaptured owning Product fix", err, read)
				}
				child = command(child.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
				if !reflect.DeepEqual(child.Product.Constraints[0].FixedPose, &captured) || child.Product.Instances[0].Translation != childPose.Translation || child.Product.Instances[0].Rotation != childPose.Rotation {
					t.Fatal("ordinary child evaluation recaptured Fix into outer world", child.Product.Constraints[0], child.Product.Instances[0])
				}
				parent = command(parent.Document.ID, workspace.CommandRequest{Type: "UPDATE_REFERENCES"})
				path := workspace.InstancePath{RootDocumentID: parent.Document.ID, Segments: []workspace.InstancePathSegment{{OwnerDocumentID: parent.Document.ID, OwnerVersionID: parent.Document.VersionID, InstanceID: outer, ReferencedDocumentID: child.Document.ID, ResolvedVersionID: child.Document.VersionID}, {OwnerDocumentID: child.Document.ID, OwnerVersionID: child.Document.VersionID, InstanceID: leaf, ReferencedDocumentID: part.Document.ID, ResolvedVersionID: part.Document.VersionID}}}
				ref := workspace.AssemblyGeometryRef{InstanceID: outer, InstancePath: &path, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-origin"}
				inspection, err := cold.InspectAssemblySupports(t.Context(), parent.Document.ID, []workspace.AssemblyGeometryRef{ref})
				if err != nil || inspection.Supports[0].Descriptor == nil {
					t.Fatal(err, inspection)
				}
				expected := compositionPoint(parent.Product.Instances[0], childPose.Translation)
				observed := compositionPoint(parent.Product.Instances[0], inspection.Supports[0].Descriptor.Origin)
				// Parent has no equations and therefore need not emit a solve
				// manifest. Use the physical tolerance of the actual child Fix
				// solve whose captured local support we are transporting.
				if compositionNorm(compositionSub(expected, observed)) > captureProfile.LengthTolerance {
					t.Fatal("nested support transformed other than once", expected, observed)
				}
			}
		})
	}
}

func TestCompositionBrokenReactivationAndMeasuredEmptySetThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	product, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Broken and measured empty set"})
	if e != nil {
		t.Fatal(e)
	}
	id, seq := product.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-quality-life-%d", id, seq)
		var err error
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	for range 2 {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	}
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: b, Kind: "BODY"}})
	fixID := product.Product.Constraints[0].ID
	first := workspace.AssemblyGeometryRef{InstanceID: a, Kind: "PLANE", GeometryID: "datum-yz"}
	second := first
	second.InstanceID = b
	axis := workspace.AssemblyGeometryRef{InstanceID: b, Kind: "PLANE", GeometryID: "datum-xy"}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "ANGLE", AngleRelation: "DIRECTED", FirstAssemblyRef: &first, SecondAssemblyRef: &second, AngleAxis: &axis, Value: .7})
	angleID := product.Product.Constraints[1].ID
	definition := func() workspace.AssemblyConstraint {
		for _, c := range product.Product.Constraints {
			if c.ID == angleID {
				return c
			}
		}
		t.Fatal("lost definition")
		return workspace.AssemblyConstraint{}
	}
	if definition().EvaluationStatus != "VERIFIED" {
		t.Fatal("initial directed", definition())
	}
	missing := axis
	missing.GeometryID = "deliberately-removed-axis"
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: angleID, AngleRelation: "DIRECTED", AngleAxis: &missing, Value: .7})
	if definition().EvaluationStatus != "BROKEN" {
		t.Fatal("missing precise source not Broken", definition())
	}
	poses := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{angleID}, Suppressed: &suppressed})
	suppressed = false
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{angleID}, Suppressed: &suppressed})
	if definition().Suppressed || definition().EvaluationStatus != "BROKEN" || !reflect.DeepEqual(poses, product.Product.Instances) || product.Product.Constraints[0].EvaluationStatus != "VERIFIED" {
		t.Fatal("reactivated unavailable source reused Verified or moved accepted pose", definition(), product.Product.Instances)
	}
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: angleID, AngleRelation: "DIRECTED", AngleAxis: &axis, Value: .7})
	if definition().EvaluationStatus != "VERIFIED" {
		t.Fatal("explicit source reconnect", definition())
	}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}})
	groupID := product.Product.Constraints[2].ID
	groupRelations := append([]workspace.AssemblyGroupRelation(nil), product.Product.Constraints[2].GroupRelations...)
	all := []string{fixID, angleID, groupID}
	suppressed = true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: all, Suppressed: &suppressed})
	checkEmpty := func(measured bool) {
		t.Helper()
		var raw []byte
		var digest string
		if err := f.db.QueryRow(t.Context(), `SELECT manifest,digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, product.Document.VersionID).Scan(&raw, &digest); err != nil {
			t.Fatal(err)
		}
		var m workspace.AssemblySolveManifest
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		for _, c := range m.Constraints {
			if c.Mode != "MEASURED" {
				t.Fatal("driving rows in empty accepted set", c)
			}
		}
		if len(m.GroupStages) != 0 || (!measured && len(m.Constraints) != 0) {
			t.Fatal("suppressed group emitted stage or equation", m.GroupStages, m.Constraints)
		}
		r, err := f.service.ReplayAssemblySolveManifest(t.Context(), id, digest, fmt.Sprintf("%s-empty-replay-%d", id, seq))
		if err != nil || r.Status != "CONVERGED" {
			t.Fatal("empty cold replay", err, r)
		}
		for _, component := range r.Result.Components {
			if component.JacobianRank != 0 {
				t.Fatal("empty/measured deducted freedom", component)
			}
		}
	}
	checkEmpty(false)
	mode := "MEASURED"
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{angleID}, ConstraintMode: &mode})
	poses = append([]workspace.ProductInstance(nil), product.Product.Instances...)
	suppressed = false
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{angleID}, Suppressed: &suppressed})
	if definition().Mode != "MEASURED" || definition().MeasuredValue == nil || !reflect.DeepEqual(poses, product.Product.Instances) {
		t.Fatal("only measured moved poses or lost mode", definition())
	}
	if math.Abs(*definition().MeasuredValue-.7) > compositionAcceptanceProfile(t, f, product.Document.VersionID).AngleTolerance || definition().Value != .7 {
		t.Fatal("only measured replaced driving definition or measured another branch", definition())
	}
	checkEmpty(true)
	suppressed = true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{angleID}, Suppressed: &suppressed})
	suppressed = false
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: all, Suppressed: &suppressed})
	if definition().Mode != "MEASURED" || product.Product.Constraints[2].GroupCaptureDigest == "" || product.Product.Constraints[2].EvaluationStatus != "VERIFIED" {
		t.Fatal("group/measurement restore changed definitions", product.Product.Constraints)
	}
	profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
	// DRIVING -> MEASURED is an explicit internal input change: its stage
	// digest must be allowed to change. It is not an ordinary-read recapture.
	// The accepted physical relative poses must nevertheless remain unchanged.
	for i, relation := range product.Product.Constraints[2].GroupRelations {
		before := groupRelations[i]
		if relation.InstanceID != before.InstanceID || compositionNorm(compositionSub(relation.RelativePose.Translation, before.RelativePose.Translation)) > profile.LengthTolerance {
			t.Fatal("mode/activation drifted group relative positions", before, relation)
		}
		positive, negative := 0.0, 0.0
		for j, v := range relation.RelativePose.Rotation {
			positive += math.Pow(v-before.RelativePose.Rotation[j], 2)
			negative += math.Pow(v+before.RelativePose.Rotation[j], 2)
		}
		if math.Sqrt(math.Min(positive, negative)) > 2*math.Sin(profile.AngleTolerance/4) {
			t.Fatal("mode/activation drifted group relative orientations", before, relation)
		}
	}
	acceptedGroup := product.Product.Constraints[2]
	read, err := workspace.NewWithArtifacts(f.db, f.client, f.store).GetDocument(t.Context(), id)
	if err != nil || !reflect.DeepEqual(read.Product.Constraints[2], acceptedGroup) {
		t.Fatal("ordinary cold read recaptured group", err, read)
	}
}

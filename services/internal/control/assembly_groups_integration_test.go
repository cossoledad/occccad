package control

import (
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/debugartifact"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

// Actual immutable BREP support, Router/Worker and isolated PostgreSQL commands;
// model-only group tests do not stand in for this stage/capture/history evidence.
func TestFixTogetherStagedInternalUpdateAndHistoryThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Staged Fix Together"})
	if err != nil {
		t.Fatal(err)
	}
	id, sequence := product.Document.ID, 0
	apply := func(request workspace.CommandRequest) {
		t.Helper()
		sequence++
		request.ActorID = f.actor
		if request.RequestID == "" {
			request.RequestID = fmt.Sprintf("%s-group-%d", id, sequence)
		}
		product, err = f.service.ApplyCommand(t.Context(), id, request)
		if err != nil {
			t.Fatal(request.Type, err)
		}
	}
	for _, name := range []string{"A", "B", "C", "Ground"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	a, b, c, d := product.Product.Instances[0].ID, product.Product.Instances[1].ID, product.Product.Instances[2].ID, product.Product.Instances[3].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{0, 0, 20}, Rotation: [4]float64{0, 0, 0, 1}})
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: c, Translation: [3]float64{30, 5, 7}, Rotation: [4]float64{0, 0, 0, 1}})
	first, second := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	first.InstanceID, second.InstanceID = b, a
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &first, SecondAssemblyRef: &second, Value: 8, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	innerID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	if product.Product.Constraints[len(product.Product.Constraints)-1].EvaluationStatus != "VERIFIED" {
		t.Fatal("initial inner Offset failed", product.Product.Constraints)
	}
	members := []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}, {InstanceID: c}}
	request := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-group-preview", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupName: "Three members", GroupMembers: members}
	head := product.Document.VersionID
	preview, err := f.service.PreviewCommand(t.Context(), id, request)
	if err != nil || preview.PreviewID == "" {
		t.Fatal("real group preview", err, preview)
	}
	read, err := f.service.GetDocument(t.Context(), id)
	if err != nil || read.Document.VersionID != head {
		t.Fatal("preview promoted group before commit", err)
	}
	request.PreviewID = preview.PreviewID
	apply(request)
	groupID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	group := func() workspace.AssemblyConstraint {
		t.Helper()
		for _, g := range product.Product.Constraints {
			if g.ID == groupID {
				return g
			}
		}
		t.Fatal("GroupId disappeared")
		return workspace.AssemblyConstraint{}
	}
	check := func() {
		t.Helper()
		profile := compositionAcceptanceProfile(t, f, product.Document.VersionID)
		g := group()
		if g.Kind != "FIX_TOGETHER" || g.Family != "FixTogether" || g.EvaluationStatus != "VERIFIED" || g.GroupCapturePending || g.GroupCaptureDigest == "" || len(g.GroupRelations) != len(g.GroupMembers) {
			t.Fatal("group capture/status incomplete", g)
		}
		poses := map[string]workspace.ProductInstance{}
		for _, p := range product.Product.Instances {
			poses[p.ID] = p
		}
		ids := []string{}
		for _, m := range g.GroupMembers {
			ids = append(ids, m.InstanceID)
		}
		sort.Strings(ids)
		anchor := poses[ids[0]]
		inverse := [4]float64{-anchor.Rotation[0], -anchor.Rotation[1], -anchor.Rotation[2], anchor.Rotation[3]}
		for _, relation := range g.GroupRelations {
			p := poses[relation.InstanceID]
			local := compositionRotate(inverse, compositionSub(p.Translation, anchor.Translation))
			if compositionNorm(compositionSub(local, relation.RelativePose.Translation)) > profile.LengthTolerance {
				t.Fatal("actual member pose violates captured group relation", relation, local)
			}
			q := p.Rotation
			relativeRotation := [4]float64{
				inverse[3]*q[0] + inverse[0]*q[3] + inverse[1]*q[2] - inverse[2]*q[1],
				inverse[3]*q[1] - inverse[0]*q[2] + inverse[1]*q[3] + inverse[2]*q[0],
				inverse[3]*q[2] + inverse[0]*q[1] - inverse[1]*q[0] + inverse[2]*q[3],
				inverse[3]*q[3] - inverse[0]*q[0] - inverse[1]*q[1] - inverse[2]*q[2],
			}
			positiveError, negativeError := 0.0, 0.0
			for i, v := range relativeRotation {
				positiveError += math.Pow(v-relation.RelativePose.Rotation[i], 2)
				negativeError += math.Pow(v+relation.RelativePose.Rotation[i], 2)
			}
			if math.Sqrt(math.Min(positiveError, negativeError)) > 2*math.Sin(profile.AngleTolerance/4) {
				t.Fatal("actual member orientation violates captured group relation", relation, relativeRotation)
			}
		}
	}
	check()
	bundle, err := f.service.ExportAssemblyDiagnostic(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	output, err := workspace.ReplayAssemblyDiagnostic(t.Context(), f.client, bundle, false)
	if err != nil {
		t.Fatal(err)
	}
	var file geometry.AssemblyReplay
	if err := json.Unmarshal(output, &file); err != nil {
		t.Fatal(err)
	}
	var diagnosticResult geometry.AssemblySolve
	if err := json.Unmarshal(file.AssemblyResult, &diagnosticResult); err != nil {
		t.Fatal(err)
	}
	if diagnosticResult.Status != "CONVERGED" || len(diagnosticResult.GroupEvidence) != 1 || len(file.Stages) < 2 {
		t.Fatal("complete diagnostic lost staged group solve", diagnosticResult)
	}
	read, err = f.service.GetDocument(t.Context(), id)
	if err != nil || read.Document.VersionID != product.Document.VersionID {
		t.Fatal("group export/replay changed history", err)
	}

	old := group().GroupCaptureDigest
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: innerID, Value: 12, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	check()
	if group().GroupCaptureDigest == old {
		t.Fatal("inner parameter edit was rigid-merged before internal solve")
	}
	// Independently evaluate d = n_first dot (p_first - p_second) from
	// authoritative local supports and adopted occurrence poses.
	poses := map[string]workspace.ProductInstance{}
	for _, p := range product.Product.Instances {
		poses[p.ID] = p
	}
	inspection, inspectErr := f.service.InspectAssemblySupports(t.Context(), id, []workspace.AssemblyGeometryRef{first, second})
	if inspectErr != nil || len(inspection.Supports) != 2 || inspection.Supports[0].Descriptor == nil || inspection.Supports[1].Descriptor == nil {
		t.Fatal("inner Offset exact supports", inspectErr, inspection)
	}
	firstGeometry, secondGeometry := inspection.Supports[0].Descriptor, inspection.Supports[1].Descriptor
	normal := compositionRotate(poses[b].Rotation, firstGeometry.Direction)
	firstPoint := compositionPoint(poses[b], firstGeometry.Origin)
	secondPoint := compositionPoint(poses[a], secondGeometry.Origin)
	delta := compositionSub(firstPoint, secondPoint)
	signed := normal[0]*delta[0] + normal[1]*delta[1] + normal[2]*delta[2]
	if math.Abs(signed-12) > compositionAcceptanceProfile(t, f, product.Document.VersionID).LengthTolerance {
		t.Fatal("inner Offset violates first-normal signed geometry", signed, poses)
	}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: d, Kind: "BODY"}})
	outerFirst, outerSecond := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	outerFirst.InstanceID, outerSecond.InstanceID = a, d
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &outerFirst, SecondAssemblyRef: &outerSecond, Value: 5, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	check()
	// Explicit member MOVE must move the entire group, without recapturing its
	// accepted relative pose or replacing independent inner Offset semantics.
	frozenRelations := append([]workspace.AssemblyGroupRelation(nil), group().GroupRelations...)
	for _, p := range product.Product.Instances {
		if p.ID == a {
			apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: a, Translation: [3]float64{p.Translation[0] + 7, p.Translation[1], p.Translation[2]}, Rotation: p.Rotation})
			break
		}
	}
	check()
	if !reflect.DeepEqual(frozenRelations, group().GroupRelations) {
		t.Fatal("ordinary whole-group motion recaptured member baselines")
	}
	for _, suppressed := range []bool{true, false} {
		apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{groupID}, Suppressed: &suppressed})
	}
	check()
	for _, g := range product.Product.Constraints {
		if g.ID == innerID && g.Suppressed {
			t.Fatal("group suppression silently disabled independent inner Offset")
		}
	}
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: groupID, GroupName: "Two members", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}})
	check()
	if len(group().GroupMembers) != 2 {
		t.Fatal(group())
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	check()
	if len(group().GroupMembers) != 3 {
		t.Fatal("Undo lost N-member group identity", group())
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	check()
	if len(group().GroupMembers) != 2 {
		t.Fatal("Redo lost member removal", group())
	}
	apply(workspace.CommandRequest{Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: groupID, GroupName: "Three members", GroupMembers: members})
	check()
	// An existing group and an overlapping subgroup share the inner stage.
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupName: "Nested", GroupMembers: []workspace.AssemblyGroupMember{{GroupID: groupID}, {InstanceID: d}}})
	nestedID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupName: "Overlap", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: b}, {InstanceID: c}}})
	for _, g := range product.Product.Constraints {
		if g.Kind == "FIX_TOGETHER" && g.EvaluationStatus != "VERIFIED" {
			t.Fatal("nested/overlap group did not verify", g)
		}
	}
	head = product.Document.VersionID
	cyclic := workspace.CommandRequest{ActorID: f.actor, Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: groupID, GroupMembers: []workspace.AssemblyGroupMember{{GroupID: nestedID}, {InstanceID: a}}}
	if _, err := f.service.ApplyCommand(t.Context(), id, cyclic); err == nil {
		t.Fatal("cyclic group edit committed")
	}
	cold := workspace.NewWithArtifacts(f.db, f.client, f.store)
	read, err = cold.GetDocument(t.Context(), id)
	if err != nil || read.Document.VersionID != head || !reflect.DeepEqual(read.Product.Constraints, product.Product.Constraints) {
		t.Fatal("cold read mutated captured group definitions", err)
	}
	var digest string
	if err = f.db.QueryRow(t.Context(), `SELECT digest FROM occccad.product_solve_manifests WHERE root_product_revision_id=$1 AND manifest->>'purpose'='COMMIT' ORDER BY created_at DESC LIMIT 1`, head).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	replayed, err := cold.ReplayAssemblySolveManifest(t.Context(), id, digest, id+"-group-cold-replay")
	if err != nil || replayed.Status != "CONVERGED" || len(replayed.Result.GroupEvidence) != 3 {
		t.Fatal("group cold staged replay", err, replayed)
	}
	for _, proof := range replayed.Result.GroupEvidence {
		if proof.StageStatus != "CONVERGED" || proof.StageResultDigest == "" || len(proof.CompiledConstraints) != len(proof.MemberIDs)-1 {
			t.Fatal("inner/outer boundary evidence missing", proof)
		}
	}
	var raw []byte
	if err = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, digest).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var manifest workspace.AssemblySolveManifest
	if err = json.Unmarshal(raw, &manifest); err != nil || len(manifest.GroupStages) != 1 {
		t.Fatal("overlap stages were not frozen as one deterministic boundary", err, manifest)
	}
	release, err := cold.CreateProductRelease(t.Context(), id, workspace.CreateProductReleaseRequest{RequestID: id + "-group-release", Name: "Frozen groups", ActorID: f.actor})
	if err != nil {
		t.Fatal(err)
	}
	suppressed := true
	apply(workspace.CommandRequest{Type: "SET_ASSEMBLY_CONSTRAINT_STATE", ConstraintIDs: []string{groupID, nestedID}, Suppressed: &suppressed})
	frozen, err := cold.ReplayProductRelease(t.Context(), id, release.ID, id+"-group-release-replay")
	if err != nil || frozen.Assembly == nil {
		t.Fatal("group Release replay lost frozen evidence", err)
	}
	if err = f.db.QueryRow(t.Context(), `SELECT manifest FROM occccad.product_solve_manifests WHERE digest=$1`, frozen.Assembly.ManifestDigest).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, definition := range manifest.Definitions {
		if definition.ID == groupID && definition.Suppressed {
			t.Fatal("new Head activation state altered frozen Release")
		}
	}
}

func TestFixTogetherFailedInternalTrialIdentityAndDissolutionThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	debugStore, err := debugartifact.NewStore(t.TempDir(), 50, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	f.service.SetDiagnosticArtifactStore(debugStore)
	part := f.importPart(t, "cylinder-r6-h12")
	product, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "Group failure and identity"})
	if err != nil {
		t.Fatal(err)
	}
	id, seq := product.Document.ID, 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		seq++
		r.ActorID = f.actor
		if r.RequestID == "" {
			r.RequestID = fmt.Sprintf("%s-group-failure-%d", id, seq)
		}
		product, err = f.service.ApplyCommand(t.Context(), id, r)
		if err != nil {
			t.Fatal(r.Type, err)
		}
	}
	for _, name := range []string{"Left", "Right"} {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Name: name})
	}
	a, b := product.Product.Instances[0].ID, product.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{0, 0, 20}, Rotation: [4]float64{0, 0, 0, 1}})
	for _, member := range []string{a, b} {
		apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: member, Kind: "BODY"}})
	}
	r := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-identity", Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", GroupName: "Grounded pair", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}}
	apply(r)
	groupID := product.Product.Constraints[len(product.Product.Constraints)-1].ID
	groupBefore := product.Product.Constraints[len(product.Product.Constraints)-1]
	posesBefore := append([]workspace.ProductInstance(nil), product.Product.Instances...)
	head := product.Document.VersionID
	idempotent, e := f.service.ApplyCommand(t.Context(), id, r)
	if e != nil || idempotent.Document.VersionID != head || len(idempotent.Product.Constraints) != len(product.Product.Constraints) {
		t.Fatal("group create idempotency", e)
	}
	stale := workspace.CommandRequest{ActorID: f.actor, RequestID: id + "-stale", Type: "EDIT_ASSEMBLY_CONSTRAINT", TargetID: groupID, GroupName: "Stale"}
	stalePreview, e := f.service.PreviewCommand(t.Context(), id, stale)
	if e != nil || stalePreview.BaseVersionID != head {
		t.Fatal("group stale candidate setup", e)
	}
	stale.PreviewID = stalePreview.PreviewID
	first, second := f.support(t, part, "PLANE"), f.support(t, part, "PLANE")
	first.InstanceID, second.InstanceID = b, a
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "DISTANCE", FirstAssemblyRef: &first, SecondAssemblyRef: &second, Value: 100, DirectionRelation: "SAME", DistanceRelation: "SELECTED_PLANE_NORMAL_V1"})
	failed := product.Product.Constraints[len(product.Product.Constraints)-1]
	if failed.EvaluationStatus == "VERIFIED" || failed.Suppressed {
		t.Fatal("internal impossible definition faked verification or suppression", failed)
	}
	bundle, err := f.service.ExportAssemblyDiagnostic(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic geometry.AssemblyReplay
	var snapshot workspace.AssemblyDiagnosticSnapshot
	_ = json.Unmarshal(bundle, &diagnostic)
	if err = json.Unmarshal(diagnostic.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Attempts) != 1 || len(snapshot.Attempts[0].Missing) != 0 || len(snapshot.Attempts[0].Sequence) == 0 {
		t.Fatal("group failure lost actual ordered numerical stages", string(bundle))
	}
	original, err := workspace.ReplayAssemblyDiagnosticWithOptions(t.Context(), f.client, bundle, workspace.AssemblyReplayOptions{Mode: "original", TargetConstraintID: failed.ID})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(original, &diagnostic)
	if diagnostic.Outcome == "INPUT_ERROR" || len(diagnostic.Stages) != len(snapshot.Attempts[0].Sequence) {
		t.Fatal("group original sequence could not replay", string(original))
	}
	for _, g := range product.Product.Constraints {
		if g.ID == groupID && (!reflect.DeepEqual(g.GroupRelations, groupBefore.GroupRelations) || g.GroupCaptureDigest != groupBefore.GroupCaptureDigest) {
			t.Fatal("failed internal candidate overwrote captured group", g)
		}
	}
	for i, p := range product.Product.Instances {
		if p.Translation != posesBefore[i].Translation || p.Rotation != posesBefore[i].Rotation {
			t.Fatal("failed candidate pose entered Revision", p, posesBefore[i])
		}
	}
	current := product.Document.VersionID
	if _, e = f.service.ApplyCommand(t.Context(), id, stale); e == nil {
		t.Fatal("stale group CAS accepted")
	}
	read, e := f.service.GetDocument(t.Context(), id)
	if e != nil || read.Document.VersionID != current {
		t.Fatal("stale CAS advanced Head", e)
	}
	apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetID: failed.ID, TargetKind: "ASSEMBLY_CONSTRAINT"})
	apply(workspace.CommandRequest{Type: "DELETE_NODE", TargetID: groupID, TargetKind: "ASSEMBLY_CONSTRAINT"})
	for _, definition := range product.Product.Constraints {
		if definition.Kind != "FIX" {
			t.Fatal("group dissolution removed or retained wrong independent definitions", definition)
		}
	}
	if len(product.Product.Constraints) != 2 {
		t.Fatal("group deletion removed independent inner constraints")
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	if len(product.Product.Constraints) != 3 {
		t.Fatal("group dissolution Undo did not restore identity")
	}
	for _, definition := range product.Product.Constraints {
		if definition.ID == groupID && (definition.Kind != "FIX_TOGETHER" || len(definition.GroupMembers) != 2) {
			t.Fatal("Undo changed group identity", definition)
		}
	}
}

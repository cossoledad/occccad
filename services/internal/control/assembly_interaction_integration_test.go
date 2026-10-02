package control

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
)

// Every frame crosses the real Router and current Worker. The fixture only
// accepts the explicitly configured disposable database, never the app DB.
func TestAssemblyInteractionRouterCommitHistory(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "M4 frozen Session"})
	if err != nil {
		t.Fatal(err)
	}
	serial := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		serial++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-m4-%d", p.Document.ID, serial)
		p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
	selected := p.Product.Instances[0].ID
	other := p.Product.Instances[1]
	head := p.Document.VersionID
	session, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: head, InstanceID: selected, LocalGrabPoint: [3]float64{2, 3, 4}, FrameRotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	target := geometry.AssemblyDragTarget{BodyID: selected, LocalGrabPoint: [3]float64{2, 3, 4}, FrameRotation: [4]float64{0, 0, 0, 1}, TargetPose: geometry.AssemblyPose{Translation: [3]float64{12, -7, 5}, Rotation: [4]float64{0, 0, 0, 1}}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: 1}
	assertGrab := func(value workspace.AssemblyInteractionFrame) {
		t.Helper()
		found := false
		for _, pose := range value.InstancePoses {
			if pose.InstanceID != selected {
				continue
			}
			found = true
			q, v := pose.Rotation, target.LocalGrabPoint
			cross := func(a, b [3]float64) [3]float64 {
				return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
			}
			axis := [3]float64{q[0], q[1], q[2]}
			uv := cross(axis, v)
			uuv := cross(axis, uv)
			for k := 0; k < 3; k++ {
				if math.Abs(pose.Translation[k]+v[k]+2*(q[3]*uv[k]+uuv[k])-(target.TargetPose.Translation[k]+v[k])) > 1e-7 {
					t.Fatal("independent accepted grab mismatch", pose)
				}
			}
		}
		if !found {
			t.Fatal("accepted frame omitted selected unit")
		}
	}
	frame, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: 1, Target: target})
	if err != nil || frame.Interaction == nil || !frame.Interaction.HardFeasible || frame.PreviewID != "" {
		t.Fatal(frame, err)
	}
	assertGrab(frame)
	current, err := f.service.GetDocument(t.Context(), p.Document.ID)
	if err != nil || current.Document.VersionID != head {
		t.Fatal("Preview wrote Head", err)
	}
	// Controlled synchronous RPC samples: one selected free unit and one
	// unrelated unit; no browser/network-queue or rendering claim. Each target
	// actually changes and every sample must be qualified and independently
	// checked before it contributes to the measurements.
	rpcSamples := []float64{frame.SolveMS}
	for sequence := uint64(2); sequence <= 10; sequence++ {
		target.TargetSequence = sequence
		target.TargetPose.Translation[0] += 0.1
		interim, solveErr := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: sequence, Target: target})
		if solveErr != nil || interim.Interaction == nil || !interim.Interaction.EligibleForCommit || interim.PreviewID != "" || interim.Interaction.TargetError > 1e-7 {
			t.Fatal("unqualified RPC timing sample", interim, solveErr)
		}
		assertGrab(interim)
		rpcSamples = append(rpcSamples, interim.SolveMS)
	}
	target.TargetSequence = 11
	final, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: 11, Target: target, Final: true})
	if err != nil || final.CommitCommand == nil || !final.Interaction.EligibleForCommit {
		t.Fatal("final not eligible", final, err)
	}
	if _, err = f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, "wrong-actor", workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: 3, Target: target}); err == nil {
		t.Fatal("wrong actor accepted")
	}
	command := *final.CommitCommand
	p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, command)
	if err != nil {
		t.Fatal("promotion", err)
	}
	committed := p.Document.VersionID
	var accepted workspace.InstancePose
	for _, i := range p.Product.Instances {
		if i.ID == selected {
			accepted = workspace.InstancePose{Translation: i.Translation, Rotation: i.Rotation}
			q := i.Rotation
			v := target.LocalGrabPoint
			cross := func(a, b [3]float64) [3]float64 {
				return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
			}
			axis := [3]float64{q[0], q[1], q[2]}
			uv := cross(axis, v)
			uuv := cross(axis, uv)
			for k := 0; k < 3; k++ {
				actual := i.Translation[k] + v[k] + 2*(q[3]*uv[k]+uuv[k])
				if math.Abs(actual-(target.TargetPose.Translation[k]+v[k])) > 1e-7 {
					t.Fatal("independent target mismatch", i)
				}
			}
		}
		if i.ID == other.ID && (i.Translation != other.Translation || i.Rotation != other.Rotation) {
			t.Fatal("unrelated moved")
		}
	}
	again, err := f.service.ApplyCommand(t.Context(), p.Document.ID, command)
	if err != nil || again.Document.VersionID != committed {
		t.Fatal("retry duplicated or rejected", err)
	}
	// Immutable final evidence remains available independently of Session TTL.
	evidence, err := f.service.GetAssemblySolveResult(t.Context(), p.Document.ID, command.RequestID)
	if err != nil || evidence.Result.Interaction == nil || !evidence.Result.Interaction.EligibleForCommit {
		t.Fatal("persistent interaction evidence", err)
	}
	// Cold replay uses the immutable final target and frozen geometry, not a
	// live Session or the current Head. Compare actual poses, not just status.
	replayed, err := f.service.ReplayAssemblySolveManifest(t.Context(), p.Document.ID, evidence.ManifestDigest, "m4-cold-"+p.Document.ID)
	if err != nil || replayed.Result.Interaction == nil || !replayed.Result.Interaction.EligibleForCommit {
		t.Fatal("cold interaction replay", err)
	}
	for _, body := range replayed.Result.Bodies {
		if body.ID == selected {
			for k := range body.Pose.Translation {
				if math.Abs(body.Pose.Translation[k]-accepted.Translation[k]) > 1e-7 {
					t.Fatal("cold replay changed final pose", body)
				}
			}
		}
	}
	release, err := f.service.CreateProductRelease(t.Context(), p.Document.ID, workspace.CreateProductReleaseRequest{RequestID: "m4-release-" + p.Document.ID, Name: "M4 frozen result", ActorID: f.actor})
	if err != nil {
		t.Fatal("M4 Release", err)
	}
	historyBefore, err := f.service.ListHistory(t.Context(), p.Document.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, session.SessionID)
	apply(workspace.CommandRequest{Type: "UNDO"})
	for _, i := range p.Product.Instances {
		if i.ID == selected && i.Translation != ([3]float64{}) {
			t.Fatal("Undo failed", i)
		}
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	for _, i := range p.Product.Instances {
		if i.ID == selected && (i.Translation != accepted.Translation || i.Rotation != accepted.Rotation) {
			t.Fatal("Redo failed", i)
		}
	}
	// Zero change is an honest eligible frame without a candidate/Revision.
	before, _ := json.Marshal(p.Product)
	zero, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: selected, FrameRotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	target.LocalGrabPoint = [3]float64{}
	target.TargetPose = geometry.AssemblyPose{Translation: accepted.Translation, Rotation: accepted.Rotation}
	target.TargetSequence = 1
	noChange, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: zero.SessionID, Sequence: 1, Target: target, Final: true})
	if err != nil || !noChange.Unchanged || noChange.CommitCommand != nil {
		t.Fatal("zero change", noChange, err)
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, zero.SessionID)
	read, err := f.service.GetDocument(t.Context(), p.Document.ID)
	after, _ := json.Marshal(read.Product)
	if err != nil || string(before) != string(after) {
		t.Fatal("cancel modified Product", err)
	}
	stale, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: selected, FrameRotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: selected, Kind: "BODY"}, FixMode: "SPACE"})
	if _, err = f.service.ReplayProductRelease(t.Context(), p.Document.ID, release.ID, "m4-release-replay-"+p.Document.ID); err != nil {
		t.Fatal("frozen Release replay after Head change", err)
	}
	if _, err = f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: stale.SessionID, Sequence: 1, Target: target, Final: true}); err == nil {
		t.Fatal("external Head did not invalidate Session")
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, stale.SessionID)
	// SPACE remains physical. An unreachable target is a qualified, unchanged
	// feasible result, not NotUpdated and not a new Revision.
	blocked, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: selected, FrameRotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	target.TargetPose.Translation[0] += 10
	fixedFrame, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: blocked.SessionID, Sequence: 1, Target: target, Final: true})
	if err != nil || fixedFrame.Interaction.Status != "CONSTRAINED" || !fixedFrame.Interaction.EligibleForCommit || !fixedFrame.Unchanged || fixedFrame.CommitCommand != nil {
		t.Fatal("SPACE restricted", fixedFrame, err)
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, blocked.SessionID)
	sort.Float64s(rpcSamples)
	t.Logf("M4 actual Router/Worker final %s eligible; one Move history entry, receipt retry unchanged; history entries before Undo=%d; Debug solve RPC+adapter sample_count=%d p50_ms=%.3f p95_ms=%.3f final_ms=%.3f; synchronous queue=none, browser/transport separation/render unmeasured", final.Interaction.Status, len(historyBefore), len(rpcSamples), rpcSamples[4], rpcSamples[9], final.SolveMS)
}

func TestAssemblyInteractionRelativeAndGroupThroughRouter(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "M4 accepted group and relative baseline"})
	if err != nil {
		t.Fatal(err)
	}
	serial := 0
	apply := func(r workspace.CommandRequest) {
		t.Helper()
		serial++
		r.ActorID = f.actor
		r.RequestID = fmt.Sprintf("%s-m4-group-%d", p.Document.ID, serial)
		p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, r)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID, Translation: [3]float64{float64(i) * 7, 0, 0}})
	}
	a, b := p.Product.Instances[0].ID, p.Product.Instances[1].ID
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX_TOGETHER", ConstraintFamily: "FixTogether", GroupMembers: []workspace.AssemblyGroupMember{{InstanceID: a}, {InstanceID: b}}})
	group := p.Product.Constraints[0]
	if group.EvaluationStatus != "VERIFIED" || group.GroupCapturePending {
		t.Fatal("group not accepted", group)
	}
	apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: a, Kind: "BODY"}, FixMode: "RELATIVE"})
	before, _ := json.Marshal(p.Product.Constraints[0].GroupRelations)
	session, err := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: b, FrameRotation: [4]float64{0, 0, 0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	initial := map[string]workspace.InstancePose{}
	for _, i := range p.Product.Instances {
		initial[i.ID] = workspace.InstancePose{Translation: i.Translation, Rotation: i.Rotation}
	}
	target := geometry.AssemblyDragTarget{BodyID: b, FrameRotation: [4]float64{0, 0, 0, 1}, TargetPose: geometry.AssemblyPose{Translation: [3]float64{initial[b].Translation[0], initial[b].Translation[1] + 10, initial[b].Translation[2]}, Rotation: initial[b].Rotation}, TranslationComponents: [3]bool{true, true, true}, RotationComponents: [3]bool{true, true, true}, TargetSequence: 1}
	frame, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: 1, Target: target, Final: true})
	if err != nil || frame.CommitCommand == nil {
		t.Fatal("relative group final", frame, err)
	}
	p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, *frame.CommitCommand)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p.Product.Constraints[0].GroupRelations)
	if string(before) != string(after) {
		t.Fatal("drag recaptured group relation")
	}
	for _, i := range p.Product.Instances {
		if i.ID == a || i.ID == b {
			if math.Abs(i.Translation[1]-(initial[i.ID].Translation[1]+10)) > 1e-7 {
				t.Fatal("group did not move coherently", i)
			}
		} else if i.Translation != initial[i.ID].Translation {
			t.Fatal("unrelated moved")
		}
		if i.ID == a {
			fix := p.Product.Constraints[1]
			if fix.FixedPose == nil || fix.FixedPose.Translation != i.Translation {
				t.Fatal("relative baseline not atomic", fix)
			}
		}
	}
	f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, session.SessionID)
	acceptedPoses := map[string]workspace.InstancePose{}
	for _, i := range p.Product.Instances {
		acceptedPoses[i.ID] = workspace.InstancePose{Translation: i.Translation, Rotation: i.Rotation}
	}
	apply(workspace.CommandRequest{Type: "UNDO"})
	for _, i := range p.Product.Instances {
		if i.Translation != initial[i.ID].Translation || i.Rotation != initial[i.ID].Rotation {
			t.Fatal("group Undo did not restore every motion unit", i)
		}
	}
	if p.Product.Constraints[1].FixedPose == nil || p.Product.Constraints[1].FixedPose.Translation != initial[a].Translation {
		t.Fatal("group Undo did not restore relative baseline")
	}
	apply(workspace.CommandRequest{Type: "REDO"})
	for _, i := range p.Product.Instances {
		if i.Translation != acceptedPoses[i.ID].Translation || i.Rotation != acceptedPoses[i.ID].Rotation {
			t.Fatal("group Redo did not restore every motion unit", i)
		}
	}
	t.Log("frozen multi-member group moved as one, independent unit unchanged; explicit RELATIVE baseline updated only with final Move")
}

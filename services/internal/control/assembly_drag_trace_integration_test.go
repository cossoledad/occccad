package control

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/workspace"
	"math"
	"testing"
)

func TestAssemblyInteractionCumulativeIntentTrace(t *testing.T) {
	f := newCompositionControlFixture(t)
	part := f.importPart(t, "cylinder-r6-h12")
	for _, plane := range []bool{false, true} {
		t.Run(fmt.Sprintf("plane-%v", plane), func(t *testing.T) {
			p, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: "cumulative trace"})
			if err != nil {
				t.Fatal(err)
			}
			serial := 0
			apply := func(command workspace.CommandRequest) {
				t.Helper()
				serial++
				command.ActorID = f.actor
				command.RequestID = fmt.Sprintf("%s-trace-%d", p.Document.ID, serial)
				p, err = f.service.ApplyCommand(t.Context(), p.Document.ID, command)
				if err != nil {
					t.Fatal(err)
				}
			}
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			apply(workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			moving, ground := p.Product.Instances[0].ID, p.Product.Instances[1].ID
			if plane {
				apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "FIX", FirstAssemblyRef: &workspace.AssemblyGeometryRef{InstanceID: ground, Kind: "BODY"}, FixMode: "SPACE"})
				first := f.support(t, part, "PLANE")
				second := first
				first.InstanceID = moving
				second.InstanceID = ground
				apply(workspace.CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: &first, SecondAssemblyRef: &second, DirectionRelation: "SAME"})
			}
			head := p.Document.VersionID
			opened, e := f.service.BeginAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: head, InstanceID: moving, LocalGrabPoint: [3]float64{0, 50, 0}, FrameRotation: [4]float64{0, 0, 0, 1}})
			if e != nil {
				t.Fatal(e)
			}
			target := geometry.AssemblyDragTarget{BodyID: moving, LocalGrabPoint: [3]float64{0, 50, 0}, FrameRotation: [4]float64{0, 0, 0, 1}, TargetPose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, TranslationComponents: [3]bool{true, false, false}, HoldTranslationComponents: [3]bool{false, true, true}, HoldRotationComponents: [3]bool{true, true, true}}
			firstSequence := uint64(1)
			if !plane {
				// Reproduce the legacy numerical failure through the real Worker.
				// No fault result is fabricated: continuation is bounded globally,
				// and the next held target must recover in the SAME Session.
				legacy := target
				legacy.TargetSequence = 1
				legacy.TargetPose.Translation[0] = 100
				legacy.HoldTranslationComponents, legacy.HoldRotationComponents = [3]bool{}, [3]bool{}
				limited, err := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: opened.SessionID, Sequence: 1, Target: legacy})
				if err != nil || limited.Interaction == nil || !limited.Interaction.HardFeasible || limited.Interaction.ContinuationSteps == 0 || limited.Interaction.Iterations > 300 || limited.PreviewID != "" {
					t.Fatal("real bounded continuation evidence", limited, err)
				}
				t.Logf("BUDGET_RECOVERY %+v", limited.Interaction)
				firstSequence = 2
			}
			var final workspace.AssemblyInteractionFrame
			for sequence := firstSequence; sequence <= 61; sequence++ {
				target.TargetSequence = sequence
				u := float64(sequence) * .2
				if sequence > 30 {
					u = 6 - float64(sequence-30)*.15
				}
				target.TargetPose.Translation = [3]float64{u, 0, 0}
				if sequence == 20 {
					cancelled, cancel := context.WithCancel(t.Context())
					cancel()
					_, e = f.service.UpdateAssemblyInteraction(cancelled, p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: opened.SessionID, Sequence: sequence, Target: target})
					if e == nil {
						t.Fatal("cancelled update must not claim qualification")
					}
					continue
				}
				frame, e := f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: opened.SessionID, Sequence: sequence, Target: target, Final: sequence == 61})
				if e != nil || frame.Interaction == nil || !frame.Interaction.EligibleForCommit {
					t.Fatal("unqualified trace", sequence, frame, e)
				}
				for _, pose := range frame.InstancePoses {
					if pose.InstanceID == moving {
						if math.Abs(pose.Translation[0]-u) > 1e-7 || math.Abs(pose.Translation[1])+math.Abs(pose.Translation[2]) > 1e-7 || math.Abs(pose.Rotation[0])+math.Abs(pose.Rotation[1])+math.Abs(pose.Rotation[2]) > 1e-8 {
							t.Fatal("independent pure translation", pose)
						}
					} else if pose.Translation != [3]float64{} || pose.Rotation != [4]float64{0, 0, 0, 1} {
						t.Fatal("unrelated/reference drift", pose)
					}
				}
				current, e := f.service.GetDocument(t.Context(), p.Document.ID)
				if e != nil || current.Document.VersionID != head {
					t.Fatal("intermediate Head changed", e)
				}
				data, _ := json.Marshal(struct {
					Sequence uint64                             `json:"sequence"`
					Target   geometry.AssemblyDragTarget        `json:"target"`
					Frame    workspace.AssemblyInteractionFrame `json:"frame"`
				}{sequence, target, frame})
				t.Logf("DRAG_TRACE %s", data)
				final = frame
			}
			target.TargetSequence = 62
			final, e = f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: opened.SessionID, Sequence: 62, GoalSequence: 61, Target: target, Final: true})
			if e != nil || final.GoalSequence != 61 {
				t.Fatal("same final goal, new transport attempt", e)
			}
			changed := target
			changed.TargetSequence = 63
			changed.TargetPose.Translation[0]++
			if _, e = f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: opened.SessionID, Sequence: 63, GoalSequence: 61, Target: changed, Final: true}); e == nil {
				t.Fatal("retry cannot change final intent")
			}
			if final.CommitCommand == nil {
				t.Fatal("final candidate missing")
			}
			command := *final.CommitCommand
			updated, e := f.service.ApplyCommand(t.Context(), p.Document.ID, command)
			if e != nil || updated.Document.VersionID == head {
				t.Fatal("single promotion", e)
			}
			again, e := f.service.ApplyCommand(t.Context(), p.Document.ID, command)
			if e != nil || again.Document.VersionID != updated.Document.VersionID {
				t.Fatal("duplicate commit", e)
			}
			p = updated
			apply(workspace.CommandRequest{Type: "UNDO"})
			if p.Product.Instances[0].Translation != [3]float64{} {
				t.Fatal("Undo full baseline")
			}
			apply(workspace.CommandRequest{Type: "REDO"})
			if math.Abs(p.Product.Instances[0].Translation[0]-target.TargetPose.Translation[0]) > 1e-7 {
				t.Fatal("Redo qualified pose")
			}
		})
	}
}

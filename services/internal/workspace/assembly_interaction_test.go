package workspace

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/geometry"
)

func TestAssemblyInteractionTypedTarget(t *testing.T) {
	target := geometry.AssemblyDragTarget{BodyID: "body", TargetSequence: 1, TargetPose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, FrameRotation: [4]float64{0, 0, 0, 1}, TranslationComponents: [3]bool{true, false, false}}
	if !validInteractionTarget(target) {
		t.Fatal("valid target rejected")
	}
	bad := target
	bad.LocalGrabPoint[0] = math.NaN()
	if validInteractionTarget(bad) {
		t.Fatal("NaN accepted")
	}
	bad = target
	bad.FrameRotation[3] = 2
	if validInteractionTarget(bad) {
		t.Fatal("nonunit frame accepted")
	}
	bad = target
	bad.TranslationComponents = [3]bool{}
	if validInteractionTarget(bad) {
		t.Fatal("zero control accepted")
	}
	if !interactionPoseEqual(InstancePoseEntry{Rotation: [4]float64{0, 0, 0, 1}}, InstancePoseEntry{Rotation: [4]float64{0, 0, 0, -1}}) {
		t.Fatal("equivalent quaternion sign considered move")
	}
}

func TestAssemblyInteractionCompleteComponent(t *testing.T) {
	input := AssemblySolveManifest{Bodies: []geometry.AssemblyBody{{ID: "moving"}, {ID: "mate"}, {ID: "axis-reference"}, {ID: "unrelated"}}, Constraints: []geometry.AssemblyConstraint{{ID: "directed", Kind: "ANGLE", FirstBodyID: "moving", SecondBodyID: "mate", AngleReferenceBodyID: "axis-reference"}, {ID: "unrelated-fix", Kind: "FIX", FirstBodyID: "unrelated"}}}
	out, err := interactionComponent(input, "moving")
	if err != nil || len(out.Bodies) != 3 || len(out.Constraints) != 1 {
		t.Fatal("third occurrence/whole connected component lost", out, err)
	}
	for _, body := range out.Bodies {
		if body.ID == "unrelated" {
			t.Fatal("unrelated retained")
		}
	}
}

func TestAssemblyInteractionCancelAndStrictFinalScope(t *testing.T) {
	service := &Service{}
	target := geometry.AssemblyDragTarget{BodyID: "moving", TargetSequence: 3, TargetPose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, FrameRotation: [4]float64{0, 0, 0, 1}, TranslationComponents: [3]bool{true, true, true}}
	s := &assemblyInteraction{actorID: "actor", documentID: "doc", previewID: "candidate", finalTarget: &target}
	work, cancel := context.WithCancel(t.Context())
	s.cancel = cancel
	s.inFlight = true
	service.assemblyInteractions.values = map[string]*assemblyInteraction{"session": s}
	service.CancelAssemblyInteraction("doc", "wrong", "session")
	if service.assemblyInteractions.values["session"] == nil {
		t.Fatal("wrong actor cancelled Session")
	}
	service.CancelAssemblyInteraction("doc", "actor", "session")
	if len(service.assemblyInteractions.values) != 0 {
		t.Fatal("cancel did not revoke")
	}
	if work.Err() != context.Canceled {
		t.Fatal("cancel did not reach numerical work context")
	}
	if err := service.validateInteractionCommit(t.Context(), "doc", preparedDomainMutation{actorID: "actor"}, CommandRequest{SessionID: "session", PreviewID: "candidate", InteractionTarget: &target}); err == nil {
		t.Fatal("cancelled final accepted")
	}
}

func TestAssemblyInteractionBusyAndSequenceGuard(t *testing.T) {
	target := geometry.AssemblyDragTarget{BodyID: "moving", TargetSequence: 2, TargetPose: geometry.AssemblyPose{Rotation: [4]float64{0, 0, 0, 1}}, FrameRotation: [4]float64{0, 0, 0, 1}, TranslationComponents: [3]bool{true, true, true}}
	service := &Service{assemblyInteractions: assemblyInteractionCache{values: map[string]*assemblyInteraction{"session": {actorID: "actor", documentID: "doc", bodyID: "moving", sequence: 1, inFlight: true, frameRotation: target.FrameRotation, expires: time.Now().Add(time.Minute)}}}}
	input := AssemblyInteractionUpdate{SessionID: "session", Sequence: 2, Target: target}
	if _, err := service.UpdateAssemblyInteraction(t.Context(), "doc", "actor", input); err == nil {
		t.Fatal("second in-flight solve accepted")
	}
	s := service.assemblyInteractions.values["session"]
	s.inFlight = false
	input.Sequence = 1
	if _, err := service.UpdateAssemblyInteraction(t.Context(), "doc", "actor", input); err == nil {
		t.Fatal("old sequence accepted")
	}
	input.Sequence = 2
	input.Target.FrameRotation = [4]float64{1, 0, 0, 0}
	if _, err := service.UpdateAssemblyInteraction(t.Context(), "doc", "actor", input); err == nil {
		t.Fatal("frozen frame changed")
	}
	if s.sequence != 1 || s.inFlight {
		t.Fatal("rejected request mutated Session")
	}
}

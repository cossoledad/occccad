package workspace

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestReplacementFramesUseStableEdgesAndPreserveUnrelatedInputs(t *testing.T) {
	path := func(id, doc string) InstancePath {
		return InstancePath{RootDocumentID: "root", Segments: []InstancePathSegment{{InstanceID: id, ReferencedDocumentID: doc, ResolvedVersionID: doc + "-r1"}}}
	}
	model := ProductModel{Instances: []ProductInstance{{ID: "other", ReferencedDocumentID: "other", ReferencedVersionID: "other-r1"}, {ID: "source", ReferencedDocumentID: "source", ReferencedVersionID: "source-r1", Translation: [3]float64{10, 5, 3}, Rotation: [4]float64{0, 0, 0, 1}}, {ID: "receiver", ReferencedDocumentID: "receiver", ReferencedVersionID: "receiver-r1", Translation: [3]float64{-5, 0, 1}, Rotation: [4]float64{0, 0, 0, 1}}}}
	payload := replaceInstancePayload{InstanceID: "source"}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		model.ContextBindings = append(model.ContextBindings, ContextBinding{ID: id, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT", SourceInstancePath: path("source", "source"), OwningInstancePath: path("receiver", "receiver")})
		payload.AffectedBindingIDs = append(payload.AffectedBindingIDs, id)
	}
	model.ContextBindings = append(model.ContextBindings, ContextBinding{ID: "untouched", ReferenceMode: "PINNED", SourceInstancePath: path("other", "other"), OwningInstancePath: path("receiver", "receiver")})
	before, _ := json.Marshal(model.ContextBindings[6])
	slices.Reverse(model.ContextBindings)
	s := &Service{}
	changed, err := s.refreshReplacementFrames(&model, payload, true)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	for _, b := range model.ContextBindings {
		if b.ID == "untouched" {
			after, _ := json.Marshal(b)
			if string(before) != string(after) {
				t.Fatal("unrelated frozen input changed")
			}
			continue
		}
		if b.Transform.Translation != [3]float64{15, 5, 2} {
			t.Fatal("wrong occurrence", b.ID, b.Transform)
		}
	}
	// The binding's acceptance snapshot may predate a later edit of the same
	// receiving definition. Its accepted Product edge still owns the pose.
	for i := range model.ContextBindings {
		model.ContextBindings[i].OwningInstancePath.Segments[0].ResolvedVersionID = "receiver-old-input-snapshot"
	}
	if changed, err := s.refreshReplacementFrames(&model, payload, false); err != nil || changed {
		t.Fatal("receiving snapshot confused with source snapshot", err)
	}

	model.Instances[1].Translation[2] = 4
	if err := s.checkReplacementCandidate(t.Context(), &model, payload); err == nil || !strings.Contains(err.Error(), "REPLACEMENT_FRAME_INCONSISTENT") {
		t.Fatal("stale promoted candidate accepted", err)
	}
	model.ContextBindings[1].SourceInstancePath.Segments[0].ResolvedVersionID = "different"
	if _, err := s.refreshReplacementFrames(&model, payload, true); err == nil {
		t.Fatal("stale path silently accepted")
	}
}

func TestReceiverReplacementFramesUseAcceptedPoseAndPreserveSourceReceipt(t *testing.T) {
	ownerPath := InstancePath{RootDocumentID: "root", Segments: []InstancePathSegment{{OwnerVersionID: "old-root", InstanceID: "receiver", ReferencedDocumentID: "new-part", ResolvedVersionID: "new-r1"}}}
	sourcePath := InstancePath{RootDocumentID: "root", Segments: []InstancePathSegment{{OwnerVersionID: "accepted-root", InstanceID: "source", ReferencedDocumentID: "source", ResolvedVersionID: "accepted-source"}}}
	frozen := InstancePose{Translation: [3]float64{25, 12, 0}, Rotation: [4]float64{0, 0, 0, 1}}
	m := ProductModel{Instances: []ProductInstance{{ID: "source", ReferencedDocumentID: "source", ReferencedVersionID: "pending-source", Translation: [3]float64{100, 0, 0}}, {ID: "receiver", ReferencedDocumentID: "new-part", ReferencedVersionID: "new-r1", Translation: [3]float64{5, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}, ContextBindings: []ContextBinding{{ID: "b", OwningInstancePath: ownerPath, SourceInstancePath: sourcePath, ReferenceMode: "FOLLOW_WORKSPACE_WITH_ACCEPT", Accepted: ContextBindingResolutionSnapshot{SourceRevisionID: "accepted-source"}}}}
	p := replaceInstancePayload{Receiver: true, InstanceID: "receiver", AffectedBindingIDs: []string{"b"}, FrozenSourcePoses: map[string]InstancePose{"b": frozen}}
	s := &Service{}
	if _, e := s.refreshReplacementFrames(&m, p, true); e != nil {
		t.Fatal(e)
	}
	if m.ContextBindings[0].Transform.Translation != [3]float64{20, 12, 0} {
		t.Fatal("used current source pose instead of accepted source frame", m.ContextBindings[0].Transform)
	}
	stampReplacementSnapshot(&m, p, "committed-root")
	raw, _ := json.Marshal(m.ContextBindings[0].SourceInstancePath)
	want, _ := json.Marshal(sourcePath)
	if string(raw) != string(want) || m.ContextBindings[0].Accepted.SourceRevisionID != "accepted-source" {
		t.Fatal("accepted source receipt changed")
	}
	if m.ContextBindings[0].OwningInstancePath.Segments[0].OwnerVersionID != "committed-root" {
		t.Fatal("owner receipt not committed")
	}
}

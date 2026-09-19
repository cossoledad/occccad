package workspace

import (
	"encoding/json"
	"testing"
)

func TestCreatePartComponentCandidateUsesStableIdentityAndProductOrigin(t *testing.T) {
	t.Parallel()
	before := ProductModel{Instances: []ProductInstance{{ID: "existing", Name: "Part1.1"}}}
	beforeJSON, _ := json.Marshal(before)
	state := productCandidateState{documentID: "product", workspaceID: "workspace", headRevision: "product-r1",
		headSequence: 1, modelJSON: beforeJSON, model: before}
	instance := ProductInstance{ID: commandEntityID("instance", "new-part-request"), Name: nextInstanceName(before, "Part1"),
		ReferencedDocumentID: "part", ReferencedVersionID: "part-r1", ResolvedVersionID: "part-r1",
		Translation: [3]float64{}, Rotation: [4]float64{0, 0, 0, 1}, ReferenceMode: "FOLLOW_HEAD"}
	candidate, err := (&Service{}).prepareProductInsertionCandidate(CreatePartComponentRequest{
		RequestID: "new-part-request", ActorID: "actor",
	}, state, instance)
	if err != nil {
		t.Fatal(err)
	}
	var after ProductModel
	if err := json.Unmarshal(candidate.nextJSON, &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Instances) != 2 || after.Instances[1].ID != instance.ID || after.Instances[1].Name != "Part1.2" ||
		after.Instances[1].Translation != [3]float64{} || after.Instances[1].Rotation != [4]float64{0, 0, 0, 1} {
		t.Fatalf("unexpected inserted component: %+v", after.Instances)
	}
	if candidate.request.Type != "CREATE_PART_COMPONENT" || len(candidate.changes.Changes) != 1 ||
		candidate.changes.Changes[0].Target.EntityID != instance.ID {
		t.Fatalf("unexpected Product candidate: request=%+v changes=%+v", candidate.request, candidate.changes)
	}
}

func TestCreatePartComponentAncestorCandidateAdvancesOnlyTargetOccurrence(t *testing.T) {
	t.Parallel()
	before := ProductModel{Instances: []ProductInstance{
		{ID: "target", Name: "SubProduct.1", ReferencedVersionID: "sub-r1", ResolvedVersionID: "sub-r1"},
		{ID: "sibling", Name: "SubProduct.2", ReferencedVersionID: "sub-r1", ResolvedVersionID: "sub-r1"},
	}}
	beforeJSON, _ := json.Marshal(before)
	state := productCandidateState{documentID: "root", workspaceID: "workspace", headRevision: "root-r1",
		headSequence: 1, modelJSON: beforeJSON, model: before}
	segment := InstancePathSegment{OwnerDocumentID: "root", OwnerVersionID: "root-r1", InstanceID: "target",
		ReferencedDocumentID: "sub-product", ResolvedVersionID: "sub-r1"}
	candidate, err := (&Service{}).prepareProductReferenceCandidate(CreatePartComponentRequest{
		RequestID: "new-part-request", ActorID: "actor",
	}, state, segment, "sub-r2", 0)
	if err != nil {
		t.Fatal(err)
	}
	var after ProductModel
	if err := json.Unmarshal(candidate.nextJSON, &after); err != nil {
		t.Fatal(err)
	}
	if after.Instances[0].ReferencedVersionID != "sub-r2" || after.Instances[0].ResolvedVersionID != "sub-r2" ||
		after.Instances[1].ReferencedVersionID != "sub-r1" {
		t.Fatalf("ancestor update escaped its typed occurrence: %+v", after.Instances)
	}
}

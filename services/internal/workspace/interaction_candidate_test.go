package workspace

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/modelcore"
)

func TestInteractionCandidatePromotionRequiresExactAuthorityBoundary(t *testing.T) {
	prepared := preparedDomainMutation{actorID: "actor", headRevision: "revision", headSequence: 7,
		command: modelcore.DomainCommand{TypeURI: "type", Payload: json.RawMessage(`{"value":42}`)}}
	value := interactionCandidate{id: "candidate", documentID: "document", actorID: prepared.actorID,
		headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
		payloadDigest: modelcore.ValueDigest(prepared.command.Payload), nextJSON: json.RawMessage(`{"solved":true}`),
		changes: modelcore.ChangeSet{ImpactSeeds: []modelcore.DependencyKey{"feature:preview-stable-id"}}, expiresAt: time.Now().Add(time.Minute)}
	var cache interactionCandidateCache
	cache.put(value)
	got, ok := cache.take("candidate", "document", prepared)
	if !ok || string(got.nextJSON) != `{"solved":true}` {
		t.Fatalf("promotion = %#v, %v", got, ok)
	}
	if len(got.changes.ImpactSeeds) != 1 || got.changes.ImpactSeeds[0] != "feature:preview-stable-id" {
		t.Fatal("promotion lost preview changeset")
	}
	if _, ok = cache.take("candidate", "document", prepared); ok {
		t.Fatal("candidate token was reusable")
	}

	cache.put(value)
	changed := prepared
	changed.command.Payload = json.RawMessage(`{"value":43}`)
	if _, ok = cache.take("candidate", "document", changed); ok {
		t.Fatal("changed command promoted stale candidate")
	}

	value.expiresAt = time.Now().Add(-time.Second)
	cache.put(value)
	if _, ok = cache.take("candidate", "document", prepared); ok {
		t.Fatal("expired candidate was promoted")
	}
}

func TestPreviewAccessAndCancellationAreActorScoped(t *testing.T) {
	service := &Service{}
	value := interactionCandidate{id: "preview", documentID: "doc", actorID: "actor", geometryKey: "geometry", expiresAt: time.Now().Add(time.Minute)}
	service.interactionCandidates.put(value)
	if _, ok := service.PreviewGeometryKey("doc", "other", "preview"); ok {
		t.Fatal("preview grant crossed actor")
	}
	service.DiscardPreview("doc", "other", "preview")
	if key, ok := service.PreviewGeometryKey("doc", "actor", "preview"); !ok || key != "geometry" {
		t.Fatal("other actor revoked preview")
	}
	service.DiscardPreview("doc", "actor", "preview")
	if _, ok := service.PreviewGeometryKey("doc", "actor", "preview"); ok {
		t.Fatal("canceled preview grant remained")
	}
	value.expiresAt = time.Now().Add(-time.Second)
	service.interactionCandidates.put(value)
	if _, ok := service.PreviewGeometryKey("doc", "actor", "preview"); ok {
		t.Fatal("expired preview grant remained")
	}
}

func TestPreviewVisualMembershipIsExactAndRevocable(t *testing.T) {
	service := &Service{}
	value := interactionCandidate{id: "preview", documentID: "doc", actorID: "actor", visualObjectID: "visual", expiresAt: time.Now().Add(time.Minute)}
	service.interactionCandidates.put(value)
	if !service.PreviewVisualAllowed("doc", "actor", "preview", "visual") {
		t.Fatal("valid visual rejected")
	}
	for _, args := range [][4]string{{"doc", "other", "preview", "visual"}, {"other", "actor", "preview", "visual"}, {"doc", "actor", "other", "visual"}, {"doc", "actor", "preview", "brep"}, {"doc", "actor", "preview", ""}} {
		if service.PreviewVisualAllowed(args[0], args[1], args[2], args[3]) {
			t.Fatal("invalid visual grant", args)
		}
	}
	service.DiscardPreview("doc", "actor", "preview")
	if service.PreviewVisualAllowed("doc", "actor", "preview", "visual") {
		t.Fatal("canceled grant remained")
	}
	value.expiresAt = time.Now().Add(-time.Second)
	service.interactionCandidates.put(value)
	if service.PreviewVisualAllowed("doc", "actor", "preview", "visual") {
		t.Fatal("expired grant remained")
	}
}

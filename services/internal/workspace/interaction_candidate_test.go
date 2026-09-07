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

package workspace

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/occccad/occccad/internal/modelcore"
)

const (
	interactionCandidateTTL = 45 * time.Second
	interactionCandidateCap = 256
)

// interactionCandidate is an authoritative, already-evaluated transient. It
// may be promoted only while its exact base and typed command still match.
// Explicit missing tokens are rejected; only tokenless commands evaluate anew.
type interactionCandidate struct {
	assemblyPreviewRequestID                                          string
	id, documentID, actorID, headRevision, commandType, payloadDigest string
	headSequence                                                      uint64
	nextJSON                                                          json.RawMessage
	geometryKey                                                       string
	visualObjectID                                                    string
	changes                                                           modelcore.ChangeSet
	intentPayload                                                     json.RawMessage
	expiresAt                                                         time.Time
}

type interactionCandidateCache struct {
	mu     sync.Mutex
	values map[string]interactionCandidate
}

type assemblyWarmStartCache struct {
	mu     sync.Mutex
	values map[string]struct {
		poses     map[string]InstancePose
		expiresAt time.Time
	}
}

func (cache *assemblyWarmStartCache) get(key string) map[string]InstancePose {
	if key == "" {
		return nil
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, ok := cache.values[key]
	if !ok || time.Now().After(value.expiresAt) {
		delete(cache.values, key)
		return nil
	}
	return value.poses
}

func (cache *assemblyWarmStartCache) put(key string, model ProductModel) {
	if key == "" {
		return
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values == nil {
		cache.values = map[string]struct {
			poses     map[string]InstancePose
			expiresAt time.Time
		}{}
	}
	now := time.Now()
	for existing, value := range cache.values {
		if now.After(value.expiresAt) {
			delete(cache.values, existing)
		}
	}
	if len(cache.values) >= interactionCandidateCap {
		var oldestKey string
		var oldest time.Time
		for existing, value := range cache.values {
			if oldestKey == "" || value.expiresAt.Before(oldest) {
				oldestKey, oldest = existing, value.expiresAt
			}
		}
		delete(cache.values, oldestKey)
	}
	poses := make(map[string]InstancePose, len(model.Instances))
	for _, instance := range model.Instances {
		poses[instance.ID] = InstancePose{Translation: instance.Translation, Rotation: normalizedInstanceRotation(instance.Rotation)}
	}
	cache.values[key] = struct {
		poses     map[string]InstancePose
		expiresAt time.Time
	}{poses, now.Add(interactionCandidateTTL)}
}

func (cache *interactionCandidateCache) put(value interactionCandidate) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.values == nil {
		cache.values = map[string]interactionCandidate{}
	}
	now := time.Now()
	for key, item := range cache.values {
		if now.After(item.expiresAt) {
			delete(cache.values, key)
		}
	}
	if len(cache.values) >= interactionCandidateCap {
		var oldestKey string
		var oldest time.Time
		for key, item := range cache.values {
			if oldestKey == "" || item.expiresAt.Before(oldest) {
				oldestKey, oldest = key, item.expiresAt
			}
		}
		delete(cache.values, oldestKey)
	}
	value.nextJSON = append(json.RawMessage(nil), value.nextJSON...)
	value.intentPayload = append(json.RawMessage(nil), value.intentPayload...)
	value.changes = cloneCandidateChanges(value.changes)
	cache.values[value.id] = value
}

func (cache *interactionCandidateCache) take(id, documentID string, prepared preparedDomainMutation) (interactionCandidate, bool) {
	value, reason := cache.takeDiagnosed(id, documentID, prepared)
	return value, reason == ""
}

// Report only identity digests and changed paths, never geometry or parameter
// contents. A mismatched actor/request must not consume another valid token.
func (cache *interactionCandidateCache) takeDiagnosed(id, documentID string, prepared preparedDomainMutation) (interactionCandidate, string) {
	if id == "" {
		return interactionCandidate{}, "candidate_missing"
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, ok := cache.values[id]
	if !ok {
		return interactionCandidate{}, "candidate_missing_or_consumed"
	}
	if time.Now().After(value.expiresAt) {
		delete(cache.values, id)
		return interactionCandidate{}, "candidate_expired"
	}
	for _, check := range []struct {
		mismatch bool
		reason   string
	}{
		{value.documentID != documentID, "target_document_mismatch"},
		{value.actorID != prepared.actorID, "actor_scope_mismatch"},
		{value.headRevision != prepared.headRevision || value.headSequence != prepared.headSequence, "base_revision_mismatch"},
		{value.commandType != prepared.command.TypeURI, "command_type_mismatch"},
	} {
		if check.mismatch {
			return interactionCandidate{}, check.reason
		}
	}
	actual := modelcore.ValueDigest(prepared.command.Payload)
	if value.payloadDigest != actual {
		return interactionCandidate{}, fmt.Sprintf("intent_digest_mismatch expected=%s actual=%s paths=%s", value.payloadDigest, actual,
			strings.Join(candidateChangedPaths(value.intentPayload, prepared.command.Payload), ","))
	}
	delete(cache.values, id)
	value.nextJSON = append(json.RawMessage(nil), value.nextJSON...)
	value.changes = cloneCandidateChanges(value.changes)
	return value, ""
}

func candidateChangedPaths(expected, actual []byte) []string {
	var a, b any
	if json.Unmarshal(expected, &a) != nil || json.Unmarshal(actual, &b) != nil {
		return []string{"$"}
	}
	paths := []string{}
	var walk func(any, any, string)
	walk = func(a, b any, path string) {
		if len(paths) >= 16 || reflect.DeepEqual(a, b) {
			return
		}
		am, aok := a.(map[string]any)
		bm, bok := b.(map[string]any)
		if aok && bok {
			keys := map[string]bool{}
			for k := range am {
				keys[k] = true
			}
			for k := range bm {
				keys[k] = true
			}
			ordered := []string{}
			for k := range keys {
				ordered = append(ordered, k)
			}
			sort.Strings(ordered)
			for _, k := range ordered {
				av, presentA := am[k]
				bv, presentB := bm[k]
				if presentA != presentB && len(paths) < 16 {
					paths = append(paths, path+"."+k)
				} else {
					walk(av, bv, path+"."+k)
				}
			}
			return
		}
		paths = append(paths, path)
	}
	walk(a, b, "$")
	return paths
}

func cloneCandidateChanges(value modelcore.ChangeSet) modelcore.ChangeSet {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var result modelcore.ChangeSet
	if json.Unmarshal(data, &result) != nil {
		return value
	}
	return result
}

// DiscardPreview revokes both promotion and transient artifact access.
func (service *Service) DiscardPreview(documentID, actorID, id string) {
	cache := &service.interactionCandidates
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if value, ok := cache.values[id]; ok && value.documentID == documentID && value.actorID == actorID {
		delete(cache.values, id)
	}
}

func (service *Service) PreviewGeometryKey(documentID, actorID, id string) (string, bool) {
	cache := &service.interactionCandidates
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, ok := cache.values[id]
	if !ok || value.documentID != documentID || value.actorID != actorID || time.Now().After(value.expiresAt) {
		return "", false
	}
	return value.geometryKey, value.geometryKey != ""
}

// PreviewVisualAllowed uses the immutable evaluated candidate, avoiding a
// second database lookup for geometry membership on every preview download.
func (service *Service) PreviewVisualAllowed(documentID, actorID, id, objectID string) bool {
	cache := &service.interactionCandidates
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, ok := cache.values[id]
	return ok && objectID != "" && value.visualObjectID == objectID && value.documentID == documentID && value.actorID == actorID && time.Now().Before(value.expiresAt)
}

package workspace

import (
	"encoding/json"
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
// Losing this cache is safe: commit falls back to normal evaluation.
type interactionCandidate struct {
	id, documentID, actorID, headRevision, commandType, payloadDigest string
	headSequence                                                      uint64
	nextJSON                                                          json.RawMessage
	geometryKey                                                       string
	changes                                                           modelcore.ChangeSet
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
	value.changes = cloneCandidateChanges(value.changes)
	cache.values[value.id] = value
}

func (cache *interactionCandidateCache) take(id, documentID string, prepared preparedDomainMutation) (interactionCandidate, bool) {
	if id == "" {
		return interactionCandidate{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	value, ok := cache.values[id]
	if !ok {
		return interactionCandidate{}, false
	}
	delete(cache.values, id)
	if time.Now().After(value.expiresAt) || value.documentID != documentID || value.actorID != prepared.actorID ||
		value.headRevision != prepared.headRevision || value.headSequence != prepared.headSequence ||
		value.commandType != prepared.command.TypeURI || value.payloadDigest != modelcore.ValueDigest(prepared.command.Payload) {
		return interactionCandidate{}, false
	}
	value.nextJSON = append(json.RawMessage(nil), value.nextJSON...)
	value.changes = cloneCandidateChanges(value.changes)
	return value, true
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

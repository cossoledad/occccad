package modelcore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrChangeConflict = errors.New("CHANGESET_CONFLICT")

type ChangeKind string

const (
	ChangeCreate ChangeKind = "CREATE"
	ChangeUpdate ChangeKind = "UPDATE"
	ChangeDelete ChangeKind = "DELETE"
	ChangeMove   ChangeKind = "MOVE"
	ChangeBind   ChangeKind = "BIND"
)

type PropertyAddress struct {
	EntityID string `json:"entityId"`
	SlotID   string `json:"slotId"`
}

func (address PropertyAddress) Key() string { return address.EntityID + ":" + address.SlotID }

type ModelChange struct {
	Kind         ChangeKind      `json:"kind"`
	Target       PropertyAddress `json:"target"`
	Before       json.RawMessage `json:"before,omitempty"`
	After        json.RawMessage `json:"after,omitempty"`
	BeforeDigest string          `json:"beforeDigest"`
	AfterDigest  string          `json:"afterDigest"`
	Tombstone    json.RawMessage `json:"tombstone,omitempty"`
}

type ChangeSet struct {
	Changes         []ModelChange   `json:"changes"`
	ImpactSeeds     []DependencyKey `json:"impactSeeds"`
	CanonicalDigest string          `json:"canonicalDigest"`
}

func NewChange(kind ChangeKind, target PropertyAddress, before, after any) (ModelChange, error) {
	change := ModelChange{Kind: kind, Target: target}
	var err error
	if before != nil {
		change.Before, err = json.Marshal(before)
		if err != nil {
			return ModelChange{}, err
		}
	}
	if after != nil {
		change.After, err = json.Marshal(after)
		if err != nil {
			return ModelChange{}, err
		}
	}
	change.BeforeDigest = ValueDigest(change.Before)
	change.AfterDigest = ValueDigest(change.After)
	if kind == ChangeDelete {
		change.Tombstone = append(json.RawMessage(nil), change.Before...)
	}
	return change, nil
}

func ValueDigest(value json.RawMessage) string {
	if len(value) == 0 {
		return ""
	}
	canonical := value
	if json.Valid(value) {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if decoder.Decode(&decoded) == nil {
			if encoded, err := json.Marshal(decoded); err == nil {
				canonical = encoded
			}
		}
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func (set *ChangeSet) Finalize() error {
	seen := map[string]struct{}{}
	for index := range set.Changes {
		change := &set.Changes[index]
		if strings.TrimSpace(change.Target.EntityID) == "" || strings.TrimSpace(change.Target.SlotID) == "" {
			return fmt.Errorf("change target requires stable entity and slot IDs")
		}
		if _, exists := seen[change.Target.Key()]; exists {
			return fmt.Errorf("multiple changes target %s", change.Target.Key())
		}
		seen[change.Target.Key()] = struct{}{}
		if change.BeforeDigest != ValueDigest(change.Before) || change.AfterDigest != ValueDigest(change.After) {
			return fmt.Errorf("digest mismatch for %s", change.Target.Key())
		}
	}
	sort.Slice(set.ImpactSeeds, func(i, j int) bool { return set.ImpactSeeds[i] < set.ImpactSeeds[j] })
	payload, err := json.Marshal(struct {
		Changes []ModelChange   `json:"changes"`
		Seeds   []DependencyKey `json:"impactSeeds"`
	}{set.Changes, set.ImpactSeeds})
	if err != nil {
		return err
	}
	set.CanonicalDigest = ValueDigest(payload)
	return nil
}

// Compensate verifies field-level digests before producing the values a caller
// must apply. It never overwrites a later edit silently.
func (set ChangeSet) Compensate(current map[PropertyAddress]json.RawMessage) (map[PropertyAddress]json.RawMessage, error) {
	result := map[PropertyAddress]json.RawMessage{}
	for index := len(set.Changes) - 1; index >= 0; index-- {
		change := set.Changes[index]
		value, exists := current[change.Target]
		if !exists {
			value = nil
		}
		if ValueDigest(value) != change.AfterDigest {
			return nil, fmt.Errorf("%w: %s no longer has the original after value", ErrChangeConflict, change.Target.Key())
		}
		result[change.Target] = append(json.RawMessage(nil), change.Before...)
	}
	return result, nil
}

func (set ChangeSet) Reapply(current map[PropertyAddress]json.RawMessage) (map[PropertyAddress]json.RawMessage, error) {
	result := map[PropertyAddress]json.RawMessage{}
	for _, change := range set.Changes {
		value, exists := current[change.Target]
		if !exists {
			value = nil
		}
		if ValueDigest(value) != change.BeforeDigest {
			return nil, fmt.Errorf("%w: %s no longer has the original before value", ErrChangeConflict, change.Target.Key())
		}
		result[change.Target] = append(json.RawMessage(nil), change.After...)
	}
	return result, nil
}

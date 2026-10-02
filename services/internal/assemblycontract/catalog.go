// Package assemblycontract owns the single versioned public capability contract.
// Test execution evidence is intentionally not an authority for product availability.
package assemblycontract

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed catalog.json
var catalogJSON []byte
var once sync.Once
var immutable Catalog
var byFamily map[string][]Capability
var byID map[string]Capability
var parseCount int

type Role struct {
	Role                 string   `json:"role"`
	Descriptor           string   `json:"descriptor"`
	SupportedDescriptors []string `json:"supportedDescriptors,omitempty"`
}

type Capability struct {
	ID      string `json:"capabilityId"`
	Family  string `json:"family"`
	Subtype string `json:"subtype"`
	Roles   []Role `json:"roles"`
	Policy  string `json:"policy"`
	Arity   struct {
		Kind string `json:"kind"`
		Min  int    `json:"min"`
		Max  *int   `json:"max"`
	} `json:"arity"`
}

type Catalog struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	ContractVersion string                     `json:"contractVersion"`
	Families        []string                   `json:"families"`
	Capabilities    []Capability               `json:"capabilities"`
	Policies        map[string]json.RawMessage `json:"policies"`
	Descriptors     map[string]json.RawMessage `json:"descriptors"`
	DerivedSupports []json.RawMessage          `json:"derivedSupports"`
}

func Read() Catalog {
	initialize()
	result := immutable
	result.Families = append([]string(nil), immutable.Families...)
	result.Capabilities = cloneCapabilities(immutable.Capabilities)
	result.Policies = cloneObjects(immutable.Policies)
	result.Descriptors = cloneObjects(immutable.Descriptors)
	result.DerivedSupports = make([]json.RawMessage, len(immutable.DerivedSupports))
	for i, v := range immutable.DerivedSupports {
		result.DerivedSupports[i] = bytes.Clone(v)
	}
	return result
}

func initialize() {
	once.Do(func() {
		parseCount++
		if err := json.Unmarshal(catalogJSON, &immutable); err != nil {
			panic(err)
		}
		byFamily = map[string][]Capability{}
		byID = map[string]Capability{}
		ids := map[string]bool{}
		for _, c := range immutable.Capabilities {
			if c.ID == "" || ids[c.ID] || len(c.Roles) == 0 || immutable.Policies[c.Policy] == nil {
				panic(fmt.Sprintf("invalid capability %s", c.ID))
			}
			ids[c.ID] = true
			byID[c.ID] = c
			byFamily[c.Family] = append(byFamily[c.Family], c)
		}
	})
}

// Return detached values, never mutable pointers into the shared authority.
func ForFamily(family string) []Capability { initialize(); return cloneCapabilities(byFamily[family]) }
func ForCapability(id string) (Capability, bool) {
	initialize()
	c, ok := byID[id]
	if !ok {
		return Capability{}, false
	}
	return cloneCapabilities([]Capability{c})[0], true
}
func cloneCapabilities(source []Capability) []Capability {
	out := append([]Capability(nil), source...)
	for i := range out {
		out[i].Roles = append([]Role(nil), source[i].Roles...)
		if source[i].Arity.Max != nil {
			max := *source[i].Arity.Max
			out[i].Arity.Max = &max
		}
		for j := range out[i].Roles {
			out[i].Roles[j].SupportedDescriptors = append([]string(nil), source[i].Roles[j].SupportedDescriptors...)
		}
	}
	return out
}
func cloneObjects(source map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(source))
	for k, v := range source {
		out[k] = bytes.Clone(v)
	}
	return out
}

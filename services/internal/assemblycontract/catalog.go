// Package assemblycontract owns the single versioned public capability contract.
// Test execution evidence is intentionally not an authority for product availability.
package assemblycontract

import (
	_ "embed"
	"encoding/json"
)

//go:embed catalog.json
var catalogJSON []byte

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
	var result Catalog
	if err := json.Unmarshal(catalogJSON, &result); err != nil {
		panic("invalid embedded assembly contract: " + err.Error())
	}
	return result
}

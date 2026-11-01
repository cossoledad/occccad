package modelcore

import (
	"encoding/json"
	"fmt"
	"sort"
)

type DependencyFacet string

const (
	IntrinsicGeometry DependencyFacet = "INTRINSIC_GEOMETRY"
	TopologySupport   DependencyFacet = "TOPOLOGY_SUPPORT"
	Placement         DependencyFacet = "PLACEMENT"
	ScopeMembership   DependencyFacet = "SCOPE_MEMBERSHIP"
	Configuration     DependencyFacet = "CONFIGURATION"
	ParameterValue    DependencyFacet = "PARAMETER_VALUE"
)

// Address is stable identity; Digest describes one frozen input. Revision is
// provenance, intentionally excluded from the semantic input fingerprint.
type DependencyRead struct {
	DocumentID string          `json:"documentId"`
	Occurrence string          `json:"occurrence,omitempty"`
	EntityID   string          `json:"entityId"`
	Facet      DependencyFacet `json:"facet"`
	Slot       string          `json:"slot,omitempty"`
	Digest     string          `json:"digest"`
	RevisionID string          `json:"revisionId,omitempty"`
}

type DerivedInput struct {
	Evaluator  string           `json:"evaluator"`
	Policy     string           `json:"policy"`
	Definition json.RawMessage  `json:"definition"`
	Reads      []DependencyRead `json:"reads"`
}

func (input DerivedInput) Fingerprint() (string, error) {
	if input.Evaluator == "" || input.Policy == "" {
		return "", fmt.Errorf("derived evaluator and policy are required")
	}
	reads := append([]DependencyRead(nil), input.Reads...)
	for i := range reads {
		if reads[i].DocumentID == "" || reads[i].EntityID == "" || reads[i].Facet == "" || reads[i].Digest == "" {
			return "", fmt.Errorf("incomplete dependency read")
		}
		reads[i].RevisionID = ""
	}
	sort.Slice(reads, func(i, j int) bool {
		a, _ := json.Marshal(reads[i])
		b, _ := json.Marshal(reads[j])
		return string(a) < string(b)
	})
	input.Reads = reads
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return ValueDigest(raw), nil
}

func DerivedCurrency(accepted, current string, available bool) string {
	if !available {
		return "MISSING"
	}
	if current == "" {
		return "UNKNOWN"
	}
	if accepted == current {
		return "CURRENT"
	}
	return "STALE"
}

package workspace

import (
	"encoding/json"
	"fmt"
)

// Experimental parameter representations are rejected, never read-repaired.
func (c *AssemblyConstraint) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if old, ok := fields["offsetParameter"]; ok && string(old) != "null" {
		return fmt.Errorf("%w: unsupported experimental Offset definition; rebuild development data", ErrValidation)
	}
	type current AssemblyConstraint
	return json.Unmarshal(raw, (*current)(c))
}
func validateAssemblyDefinitionFormat(model ProductModel) error {
	for _, c := range model.Constraints {
		if c.DefinitionVersion != 2 {
			return fmt.Errorf("%w: unsupported assembly definition version; rebuild development data", ErrValidation)
		}
		if c.Kind == "RIGID" {
			return fmt.Errorf("%w: unsupported experimental rigid definition; rebuild development data", ErrValidation)
		}
		if c.DistanceRelation == "ALONG_SECOND_NORMAL" || c.DistanceRelation == "OPPOSITE_SECOND_NORMAL" {
			return fmt.Errorf("%w: unsupported experimental Offset sign convention; rebuild development data", ErrValidation)
		}
	}
	return nil
}

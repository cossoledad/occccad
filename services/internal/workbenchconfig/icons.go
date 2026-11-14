package workbenchconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// IconSource is authoring data. The API continues to return flat Icon elements.
type IconSource struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Symbols       map[string][]Element `json:"symbols"`
	Icons         map[string]struct {
		Family   string    `json:"family"`
		Parts    []string  `json:"parts,omitempty"`
		Elements []Element `json:"elements,omitempty"`
		Resource string    `json:"resource,omitempty"`
	} `json:"icons"`
}

func compileIcons(data []byte) (map[string]Icon, error) {
	if err := validateJSONKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return nil, err
	}
	var source IconSource
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&source); err != nil {
		return nil, err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF || source.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported icon source or trailing data")
	}
	families := map[string]bool{"view": true, "solid": true, "surface": true, "curve": true, "sketch": true, "constraint": true, "assembly": true, "reference": true, "status": true}
	icons := make(map[string]Icon, len(source.Icons))
	for id, definition := range source.Icons {
		if id == "" || !families[definition.Family] || len(definition.Parts) > 8 {
			return nil, fmt.Errorf("icon %s: invalid family or composition", id)
		}
		icon := Icon{ClassName: "cad-command-icon family-" + definition.Family, Resource: definition.Resource}
		for _, part := range definition.Parts {
			elements, ok := source.Symbols[part]
			if !ok {
				return nil, fmt.Errorf("icon %s: missing symbol %s", id, part)
			}
			icon.Elements = append(icon.Elements, elements...)
		}
		icon.Elements = append(icon.Elements, definition.Elements...)
		if len(icon.Elements) == 0 && icon.Resource == "" {
			return nil, fmt.Errorf("icon %s: empty graphic", id)
		}
		icons[id] = icon
	}
	return icons, nil
}

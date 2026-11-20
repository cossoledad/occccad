package workspace

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// All ordinals are local to this Visual artifact. Zero denotes an empty list.
// A row contains kind, locator, mapping status, origins, primary, modifiers,
// supports, and pattern members. Members are pairs of symbol ordinal and slot.
type featureAssociationTable struct {
	Version  int                 `json:"version"`
	Symbols  []string            `json:"symbols"`
	Features []uint32            `json:"features"`
	Lists    associationOrdinals `json:"lists,omitempty"`
	Members  associationOrdinals `json:"members,omitempty"`
	Rows     associationOrdinals `json:"rows"`
}

// The writer emits compact arrays of unsigned decimal integers. Decode that
// common case into one backing array instead of growing a slice per row and
// invoking the reflective JSON decoder for every ordinal. Other JSON inputs
// use the standard decoder, including its null, whitespace and overflow rules.
type associationOrdinals [][]uint32

func (rows *associationOrdinals) UnmarshalJSON(data []byte) error {
	if packed, ok := compactAssociationOrdinals(data); ok {
		*rows = packed
		return nil
	}
	return json.Unmarshal(data, (*[][]uint32)(rows))
}

func compactAssociationOrdinals(data []byte) (associationOrdinals, bool) {
	if len(data) < 4 || data[0] != '[' || data[1] != '[' || data[len(data)-1] != ']' {
		return nil, false
	}
	// Estimate compact rows in bulk. Bound speculative allocation by input
	// size even when a non-numeric JSON string contains many delimiters.
	values := make([]uint32, 0, min(bytes.Count(data, []byte(","))+1, len(data)/4+1))
	rows := make(associationOrdinals, 0, min(bytes.Count(data, []byte("["))-1, len(data)/24+1))
	for i := 1; i < len(data)-1; {
		if data[i] != '[' {
			return nil, false
		}
		i++
		start := len(values)
		if data[i] != ']' {
			for {
				first := i
				var value uint64
				for i < len(data) && data[i] >= '0' && data[i] <= '9' {
					value = value*10 + uint64(data[i]-'0')
					if value > math.MaxUint32 {
						return nil, false
					}
					i++
				}
				if i == first || i-first > 1 && data[first] == '0' || i >= len(data) {
					return nil, false
				}
				values = append(values, uint32(value))
				if data[i] != ',' {
					break
				}
				i++
			}
		}
		if data[i] != ']' {
			return nil, false
		}
		rows = append(rows, values[start:len(values):len(values)])
		i++
		if i == len(data)-1 {
			return rows, true
		}
		if i >= len(data)-1 || data[i] != ',' {
			return nil, false
		}
		i++
	}
	return nil, false
}

var associationKinds = [...]string{"FACE", "EDGE", "VERTEX"}
var associationStatuses = [...]string{"MAPPED", "MAPPING_MISSING"}

func packFeatureAssociations(index *FeatureAssociationIndex) (*featureAssociationTable, error) {
	if index == nil {
		return nil, nil
	}
	table := &featureAssociationTable{Version: 1, Symbols: []string{}, Features: []uint32{}, Rows: make([][]uint32, 0, len(index.Elements))}
	symbols := make(map[string]uint32, len(index.Features))
	symbol := func(id string) uint32 {
		if ordinal := symbols[id]; ordinal != 0 {
			return ordinal
		}
		table.Symbols = append(table.Symbols, id)
		ordinal := uint32(len(table.Symbols))
		symbols[id] = ordinal
		return ordinal
	}
	for _, id := range index.Features {
		table.Features = append(table.Features, symbol(id))
	}
	lists, members := map[string]uint32{}, map[string]uint32{}
	// Reuse scratch for all rows. Each distinct immutable list is copied once.
	var key []byte
	var values []uint32
	intern := func(values []uint32, known map[string]uint32, pool *associationOrdinals) uint32 {
		if len(values) == 0 {
			return 0
		}
		key = key[:0]
		for _, value := range values {
			key = binary.AppendUvarint(key, uint64(value))
		}
		if ordinal := known[string(key)]; ordinal != 0 {
			return ordinal
		}
		*pool = append(*pool, append([]uint32(nil), values...))
		ordinal := uint32(len(*pool))
		known[string(key)] = ordinal
		return ordinal
	}
	// Restored and carried rows borrow immutable lists. Only identical slices
	// skip the value interner; equal independently allocated lists still deduplicate.
	type listSlice struct {
		first  *string
		length int
	}
	shared := map[listSlice]uint32{}
	list := func(ids []string) uint32 {
		if len(ids) == 0 {
			return 0
		}
		key := listSlice{&ids[0], len(ids)}
		if ordinal := shared[key]; ordinal != 0 {
			return ordinal
		}
		values = values[:0]
		for _, id := range ids {
			values = append(values, symbol(id))
		}
		ordinal := intern(values, lists, &table.Lists)
		shared[key] = ordinal
		return ordinal
	}
	for _, row := range index.Elements {
		var kind, status uint32
		switch row.Kind {
		case "FACE":
			kind = 0
		case "EDGE":
			kind = 1
		case "VERTEX":
			kind = 2
		default:
			return nil, fmt.Errorf("invalid display association topology kind")
		}
		switch row.Status {
		case "MAPPED":
			status = 0
		case "MAPPING_MISSING":
			status = 1
		default:
			return nil, fmt.Errorf("invalid display association status")
		}
		if row.LocalID == 0 || row.LocalID > math.MaxUint32 {
			return nil, fmt.Errorf("invalid display association locator")
		}
		origins, primary := list(row.Origins), list(row.Primary)
		modifiers, supports := list(row.Modifiers), list(row.Supports)
		values = values[:0]
		for _, member := range row.Members {
			if member.Slot < 0 || uint64(member.Slot) > math.MaxUint32 {
				return nil, fmt.Errorf("invalid display association pattern slot")
			}
			values = append(values, symbol(member.PatternID), uint32(member.Slot))
		}
		memberList := intern(values, members, &table.Members)
		table.Rows = append(table.Rows, []uint32{kind, uint32(row.LocalID), status, origins, primary, modifiers, supports, memberList})
	}
	return table, nil
}

func (table *featureAssociationTable) unpack() (*FeatureAssociationIndex, error) {
	if table.Version != 1 {
		return nil, fmt.Errorf("unsupported display association table")
	}
	resolve := func(ids []uint32) ([]string, error) {
		values := make([]string, len(ids))
		for i, id := range ids {
			if id == 0 || uint64(id) > uint64(len(table.Symbols)) || table.Symbols[id-1] == "" {
				return nil, fmt.Errorf("invalid display association symbol")
			}
			values[i] = table.Symbols[id-1]
		}
		return values, nil
	}
	features, err := resolve(table.Features)
	if err != nil {
		return nil, err
	}
	ids := make([][]string, len(table.Lists)+1)
	ids[0] = []string{}
	for i, list := range table.Lists {
		if ids[i+1], err = resolve(list); err != nil {
			return nil, err
		}
	}
	members := make([][]FeatureAssociationMember, len(table.Members)+1)
	for i, pairs := range table.Members {
		if len(pairs)%2 != 0 {
			return nil, fmt.Errorf("invalid display association member list")
		}
		if len(pairs) > 0 {
			members[i+1] = make([]FeatureAssociationMember, len(pairs)/2)
		}
		for j := 0; j < len(pairs); j += 2 {
			id := pairs[j]
			if id == 0 || uint64(id) > uint64(len(table.Symbols)) || table.Symbols[id-1] == "" {
				return nil, fmt.Errorf("invalid display association member symbol")
			}
			members[i+1][j/2] = FeatureAssociationMember{PatternID: table.Symbols[id-1], Slot: int(pairs[j+1])}
		}
	}
	index := &FeatureAssociationIndex{Features: features, Elements: make([]FeatureTopologyAssociation, 0, len(table.Rows))}
	locators := make(map[uint64]bool, len(table.Rows))
	for _, row := range table.Rows {
		if len(row) != 8 || row[0] >= uint32(len(associationKinds)) || row[1] == 0 || row[2] >= uint32(len(associationStatuses)) ||
			row[3] >= uint32(len(ids)) || row[4] >= uint32(len(ids)) || row[5] >= uint32(len(ids)) || row[6] >= uint32(len(ids)) || row[7] >= uint32(len(members)) {
			return nil, fmt.Errorf("invalid display association row")
		}
		locator := uint64(row[0])<<32 | uint64(row[1])
		if locators[locator] {
			return nil, fmt.Errorf("duplicate display association locator")
		}
		locators[locator] = true
		index.Elements = append(index.Elements, FeatureTopologyAssociation{
			Kind: associationKinds[row[0]], LocalID: uint64(row[1]), Status: associationStatuses[row[2]],
			Origins: ids[row[3]], Primary: ids[row[4]], Modifiers: ids[row[5]], Supports: ids[row[6]], Members: members[row[7]],
		})
	}
	return index, nil
}

// unpack allocates one slice per dictionary list. All references to a symbol
// share the same decoded string bytes, including across different lists.
func (table *featureAssociationTable) decodedListBytes() int64 {
	bytes := int64(len(table.Features)) * 16
	for _, symbol := range table.Symbols {
		bytes += int64(len(symbol))
	}
	for _, ids := range table.Lists {
		bytes += int64(len(ids)) * 16
	}
	for _, pairs := range table.Members {
		bytes += int64(len(pairs)/2) * 24
	}
	return bytes
}

type visualizationManifestJSON VisualizationManifest

type visualizationJSON struct {
	*visualizationManifestJSON
	Table *featureAssociationTable `json:"featureAssociationTable,omitempty"`
}

func (v *VisualizationManifest) UnmarshalJSON(data []byte) error {
	wire := visualizationJSON{visualizationManifestJSON: (*visualizationManifestJSON)(v)}
	if err := json.Unmarshal(data, &wire); err != nil || wire.Table == nil {
		return err
	}
	index, err := wire.Table.unpack()
	if err == nil {
		v.FeatureAssociations = index
	}
	return err
}

package workspace

import (
	"container/heap"
	"context"
	"encoding/binary"
	"sort"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// This is a derived, immutable query plan for one frozen manifest. Symbols and
// event positions are local accelerators, not persistent topology identities.
type namingOwner struct {
	body      string
	result    modelcore.GeometryResultReference
	geometric bool
}
type namingEvent struct {
	sources, targets []int
	deleted          bool
}
type namingOwnerSearch struct {
	splitParent map[int]*workerv1.SemanticTopologyRef
	seen        map[int]bool
	events      []namingEvent
	bySource    map[int][]int
	outputs     []*workerv1.SemanticTopologyOutput
	byOutput    map[int][]int
}
type namingSearchIndex struct {
	symbols  map[string]int
	atoms    map[string]uint64
	owners   map[namingOwner]*namingOwnerSearch
	complete bool
}
type namingQueryStats struct{ Events, Outputs int }

func (index *namingSearchIndex) retainedBytes() int {
	bytes := 256
	for symbol := range index.symbols {
		bytes += len(symbol) + 64
	}
	for text := range index.atoms {
		bytes += len(text) + 64
	}
	for _, owner := range index.owners {
		bytes += 256 + len(owner.splitParent)*64 + len(owner.seen)*32 + cap(owner.events)*56 + cap(owner.outputs)*8
		bytes += splitParentCharge(owner.splitParent)
		for _, event := range owner.events {
			bytes += (cap(event.sources) + cap(event.targets)) * 8
		}
		for _, entries := range owner.bySource {
			bytes += 64 + cap(entries)*8
		}
		for _, entries := range owner.byOutput {
			bytes += 64 + cap(entries)*8
		}
	}
	return bytes
}

// Pack the complete ordered tuple using immutable, manifest-local string
// ordinals. This has the same equality as comparing all the original strings.
func (index *namingSearchIndex) symbolKey(data []byte, ref modelcore.SemanticTopologyRef, insert bool) ([]byte, bool) {
	atom := func(text string) uint64 {
		id := index.atoms[text]
		if id == 0 && insert {
			id = uint64(len(index.atoms) + 1)
			index.atoms[text] = id
		}
		return id
	}
	feature, slot := atom(ref.FeatureID), atom(ref.OutputSlot)
	if feature == 0 || slot == 0 {
		return nil, false
	}
	data = binary.AppendUvarint(data, feature)
	data = binary.AppendUvarint(data, slot)
	for _, source := range ref.SourceIDs {
		id := atom(source)
		if id == 0 {
			return nil, false
		}
		data = binary.AppendUvarint(data, id)
	}
	return data, true
}

func (index *namingSearchIndex) findSymbol(ref modelcore.SemanticTopologyRef) (int, bool) {
	var scratch [96]byte
	key, ok := index.symbolKey(scratch[:0], ref, false)
	if !ok {
		return 0, false
	}
	id, found := index.symbols[string(key)]
	return id, found
}

func namingSelectionOwner(selection modelcore.PersistentSelection) namingOwner {
	if selection.SourceResult != nil {
		return namingOwner{result: *selection.SourceResult, geometric: true}
	}
	return namingOwner{body: selection.SourceBodyID}
}
func namingFeatureOwner(feature *workerv1.FeatureResult) (namingOwner, bool) {
	if feature.Result != nil {
		return namingOwner{result: modelcore.GeometryResultReference{FeatureID: feature.Result.FeatureId, ResultID: feature.Result.ResultId}, geometric: true}, feature.BodyId == ""
	}
	return namingOwner{body: feature.BodyId}, true
}

func buildNamingSearchIndex(ctx context.Context, manifest *topologyManifest) (*namingSearchIndex, error) {
	index := &namingSearchIndex{symbols: map[string]int{}, atoms: map[string]uint64{}, owners: map[namingOwner]*namingOwnerSearch{}, complete: len(manifest.GetFeatureResults()) > 0}
	pointers := map[*workerv1.SemanticTopologyRef]int{}
	intern := func(ref *workerv1.SemanticTopologyRef) int {
		if id, ok := pointers[ref]; ok {
			return id
		}
		// Borrow the immutable source slice only for canonicalization. Exported
		// selections continue to copy their source IDs and optional evidence.
		var scratch [96]byte
		key, _ := index.symbolKey(scratch[:0], modelcore.SemanticTopologyRef{FeatureID: ref.GetFeatureId(), OutputSlot: ref.GetOutputSlot(), SourceIDs: ref.GetSourceIds()}, true)
		id, ok := index.symbols[string(key)]
		if !ok {
			id = len(index.symbols)
			index.symbols[string(key)] = id
		}
		pointers[ref] = id
		return id
	}
	ownerFor := func(key namingOwner) *namingOwnerSearch {
		owner := index.owners[key]
		if owner == nil {
			owner = &namingOwnerSearch{seen: map[int]bool{}, bySource: map[int][]int{}, byOutput: map[int][]int{}}
			index.owners[key] = owner
		}
		return owner
	}
	add := func(owner *namingOwnerSearch, event namingEvent) {
		if len(event.sources) == 0 {
			return
		}
		id := len(owner.events)
		owner.events = append(owner.events, event)
		for _, source := range event.sources {
			owner.bySource[source] = append(owner.bySource[source], id)
		}
	}
	visited := 0
	for _, feature := range manifest.GetFeatureResults() {
		if !feature.GetTopologyHistoryComplete() {
			index.complete = false
		}
		key, valid := namingFeatureOwner(feature)
		if !valid {
			continue
		}
		owner := ownerFor(key)
		for _, lineage := range feature.GetTopologyHistory().GetLineage() {
			visited++
			if visited%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			target := intern(lineage.Result)
			if lineage.Kind == workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_SPLIT && len(lineage.Sources) == 1 {
				if owner.splitParent == nil {
					owner.splitParent = map[int]*workerv1.SemanticTopologyRef{}
				}
				owner.splitParent[target] = lineage.Sources[0]
			}
			owner.seen[target] = true
			var sources []int
			for _, source := range lineage.Sources {
				id := intern(source)
				owner.seen[id] = true
				// A self edge cannot activate an absent reference. Its presence
				// is still recorded for exact outside-tip diagnostics.
				if id != target && lineage.Kind != workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_GENERATED {
					sources = append(sources, id)
				}
			}
			if len(sources) > 0 {
				add(owner, namingEvent{sources: sources, targets: []int{target}})
			}
		}
		for _, ambiguous := range feature.GetTopologyHistory().GetAmbiguous() {
			var sources, targets []int
			for _, source := range ambiguous.Sources {
				id := intern(source)
				owner.seen[id] = true
				sources = append(sources, id)
			}
			for _, target := range ambiguous.Candidates {
				targets = append(targets, intern(target))
			}
			if len(targets) > 0 {
				add(owner, namingEvent{sources: sources, targets: targets})
			}
		}
		for _, deleted := range feature.GetTopologyHistory().GetDeleted() {
			id := intern(deleted.Source)
			owner.seen[id] = true
			add(owner, namingEvent{sources: []int{id}, deleted: true})
		}
		for _, output := range feature.GetSemanticOutputs() {
			owner.seen[intern(output.SemanticRef)] = true
		}
	}
	installTip := func(key namingOwner, tip *workerv1.FeatureResult) {
		owner := ownerFor(key)
		owner.outputs = tip.GetSemanticOutputs()
		for i, output := range owner.outputs {
			id := intern(output.SemanticRef)
			owner.byOutput[id] = append(owner.byOutput[id], i)
		}
	}
	for body, tip := range manifest.Tips {
		installTip(namingOwner{body: body}, tip)
	}
	for result, tip := range manifest.Results {
		installTip(namingOwner{result: result, geometric: true}, tip)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

func namingSearch(manifest *topologyManifest) *namingSearchIndex {
	if manifest.search != nil {
		return manifest.search
	}
	// Mutable, manually constructed domain fixtures get a fresh view. Only a
	// validated immutable artifact installs and shares an index across reads.
	index, _ := buildNamingSearchIndex(context.Background(), manifest)
	return index
}

type namingEvents []int

func (q namingEvents) Len() int           { return len(q) }
func (q namingEvents) Less(i, j int) bool { return q[i] < q[j] }
func (q namingEvents) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *namingEvents) Push(value any)    { *q = append(*q, value.(int)) }
func (q *namingEvents) Pop() any {
	last := len(*q) - 1
	value := (*q)[last]
	*q = (*q)[:last]
	return value
}

func (index *namingSearchIndex) semanticOutput(selection modelcore.PersistentSelection) *workerv1.SemanticTopologyOutput {
	id, ok := index.findSymbol(selection.Anchor)
	if !ok {
		return nil
	}
	owner := index.owners[namingSelectionOwner(selection)]
	if owner == nil {
		return nil
	}
	for _, position := range owner.byOutput[id] {
		output := owner.outputs[position]
		if topologyType(output.TopologyType) == selection.ExpectedType {
			return output
		}
	}
	return nil
}

// Only rules incident to live references are visited, in original temporal
// order. An unordered transitive closure would resurrect deleted references or
// follow a rule that occurred before its source became live.
func (index *namingSearchIndex) resolve(selection modelcore.PersistentSelection, geometryKey, geometryID string, stats *namingQueryStats) modelcore.SelectionResolution {
	return index.resolveForUse(selection, geometryKey, geometryID, stats, finiteElement)
}
func (index *namingSearchIndex) resolveForUse(selection modelcore.PersistentSelection, geometryKey, geometryID string, stats *namingQueryStats, use namingUse) modelcore.SelectionResolution {
	result := modelcore.SelectionResolution{Status: modelcore.SelectionMissing, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_MISSING", Diagnostic: "semantic anchor has no result in the target body tip"}
	id, ok := index.findSymbol(selection.Anchor)
	owner := index.owners[namingSelectionOwner(selection)]
	if !ok || owner == nil || !owner.seen[id] {
		result.Status = modelcore.SelectionOutsideCurrentTip
		result.DiagnosticCode = "PERSISTENT_SELECTION_OUTSIDE_CURRENT_TIP"
		result.Diagnostic = "anchor feature is not part of the target body tip"
		return result
	}
	active := map[int]bool{id: true}
	queued := map[int]bool{}
	scheduled := map[int]bool{}
	queue := namingEvents{}
	schedule := func(symbol, after int) {
		// First activation queues every future incident event. Reactivation
		// cannot make an earlier event eligible, and all later ones are already
		// queued. Scan each symbol's incidence list once, even after tombstones.
		if scheduled[symbol] {
			return
		}
		scheduled[symbol] = true
		events := owner.bySource[symbol]
		for _, event := range events[sort.SearchInts(events, after+1):] {
			if !queued[event] {
				queued[event] = true
				heap.Push(&queue, event)
			}
		}
	}
	schedule(id, -1)
	for queue.Len() > 0 {
		position := heap.Pop(&queue).(int)
		event := owner.events[position]
		if stats != nil {
			stats.Events++
		}
		if event.deleted {
			delete(active, event.sources[0])
			continue
		}
		matched := false
		for _, source := range event.sources {
			if active[source] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, target := range event.targets {
			if !active[target] {
				active[target] = true
				schedule(target, position)
			}
		}
	}
	var positions []int
	for symbol := range active {
		positions = append(positions, owner.byOutput[symbol]...)
	}
	sort.Ints(positions)
	for _, position := range positions {
		if stats != nil {
			stats.Outputs++
		}
		output := owner.outputs[position]
		if topologyType(output.GetTopologyType()) != selection.ExpectedType {
			result.Status = modelcore.SelectionTypeMismatch
			result.DiagnosticCode = "PERSISTENT_SELECTION_TYPE_MISMATCH"
			continue
		}
		ref, evidence := semanticRef(output.SemanticRef), selectionEvidence(output.Evidence)
		if use == analyticSupport && selection.Selector.Kind == modelcore.SelectionLineageDescendant && selection.CreationEvidence.GeometryType != "" && evidence.GeometryType != selection.CreationEvidence.GeometryType {
			return modelcore.SelectionResolution{Status: modelcore.SelectionTypeMismatch, SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "PERSISTENT_SELECTION_TYPE_MISMATCH", Diagnostic: "descendant geometry is incompatible with the requested support"}
		}
		if !(use == descendantSet && selection.Selector.Kind == modelcore.SelectionLineageDescendant) && !candidateMatchesRecipe(selection, ref, evidence) {
			continue
		}
		result.Candidates = append(result.Candidates, modelcore.ResolvedTopologyElement{GeometryID: geometryID, GeometryKey: geometryKey, Type: selection.ExpectedType, LocalID: output.LocalId, SemanticRef: ref, Evidence: evidence})
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		return refKey(result.Candidates[i].SemanticRef) < refKey(result.Candidates[j].SemanticRef)
	})
	switch len(result.Candidates) {
	case 1:
		result.Status = modelcore.SelectionResolved
		result.SupportingElementStatus = modelcore.SupportingElementConnected
		result.DiagnosticCode = ""
		result.Diagnostic = ""
		result.EvidenceDigest = result.Candidates[0].Evidence.EvidenceDigest
	case 0:
		if result.Status != modelcore.SelectionTypeMismatch {
			result.Status = modelcore.SelectionMissing
		}
	default:
		result.Status = modelcore.SelectionAmbiguous
		result.DiagnosticCode = "PERSISTENT_SELECTION_AMBIGUOUS"
		result.Diagnostic = "lineage produced multiple valid candidates; reconnect or use a more specific recipe"
	}
	return resolveNamingUse(result, use, selection)
}

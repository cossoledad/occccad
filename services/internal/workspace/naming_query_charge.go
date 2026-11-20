package workspace

import (
	"unsafe"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/proto"
)

// The frozen root stores interned strings, source lists and evidence once.
// Charge its bytes once for payloads, then the Go objects retained by the tip.
// A wire-size multiplier overcharges long strings and undercharges short rows.
func namingTipCharge(root *workerv1.PartTopologyManifest, wireBytes, locators int) int {
	const pointer = int(unsafe.Sizeof(uintptr(0)))
	const stringHeader = int(unsafe.Sizeof(""))
	const refSize = int(unsafe.Sizeof(workerv1.SemanticTopologyRef{}))
	bytes := wireBytes + 1024
	if table := root.ReferenceDictionary; table != nil {
		bytes += len(table.Rows) * refSize
		for _, list := range table.SourceLists {
			bytes += len(list.Symbols) * stringHeader
		}
	} else {
		for _, ref := range root.SemanticRefs {
			bytes += refSize + cap(ref.SourceIds)*stringHeader
		}
	}
	// Output pointers, locator map capacity and key/value storage. The index
	// holds pointers to the very same output objects and is charged separately.
	const entry = int(unsafe.Sizeof(namingLocatorKey{}) + unsafe.Sizeof(namingLocator{}))
	bytes += locators * (int(unsafe.Sizeof(workerv1.SemanticTopologyOutput{})) + 2*pointer + 2*(entry+8))
	// Each decoded evidence can retain centroid, origin, direction, X axis
	// and optional scalars. Allow space for all, including the point zero axis.
	const evidence = int(unsafe.Sizeof(workerv1.SelectionEvidence{})+4*unsafe.Sizeof(workerv1.Vec3{})) + 8*8
	for _, row := range root.Evidence {
		bytes += evidence + 2*len(row.Adjacent)*pointer
	}
	bytes += len(root.History) * (int(unsafe.Sizeof(workerv1.NamingHistoryReference{})) + pointer)
	return bytes + bytes/4
}

// Split-parent witnesses can outlive the owning decoded history blocks.
// Siblings often share the same reference object in a given frozen block.
func splitParentCharge(parents map[int]*workerv1.SemanticTopologyRef) int {
	bytes := 0
	seen := map[*workerv1.SemanticTopologyRef]bool{}
	for _, ref := range parents {
		if seen[ref] {
			continue
		}
		seen[ref] = true
		bytes += int(unsafe.Sizeof(*ref)) + int(unsafe.Sizeof(""))*cap(ref.SourceIds) + proto.Size(ref)
	}
	return bytes
}

// Charge the immutable query graph rather than its hypothetical serialization:
// repeated row references share one decoded SemanticTopologyRef. The allowance
// also covers allocator/index overhead. Like other cache charges this estimates
// retained ownership; reader leases and process RSS are not bounded by it.
func namingQueryBlockCharge(feature *workerv1.FeatureResult) int {
	pointerBytes := int(unsafe.Sizeof((*workerv1.SemanticTopologyRef)(nil)))
	stringBytes := int(unsafe.Sizeof(""))
	history := feature.TopologyHistory
	bytes := int(unsafe.Sizeof(workerv1.FeatureResult{}) + unsafe.Sizeof(workerv1.TopologyHistory{}))
	for _, text := range []string{feature.FeatureId, feature.BodyId, feature.InputFeatureId, feature.ProfileFeatureId, feature.ResultGeometryId, history.FeatureId, history.InputGeometryId, history.ResultGeometryId, history.PolicyDigest, history.EvidenceDigest} {
		bytes += len(text)
	}
	bytes += cap(feature.Diagnostics) * stringBytes
	for _, text := range feature.Diagnostics {
		bytes += len(text)
	}
	if feature.Result != nil {
		bytes += int(unsafe.Sizeof(workerv1.GeometryResultReference{})) + proto.Size(feature.Result)
	}
	seen := map[*workerv1.SemanticTopologyRef]bool{}
	ref := func(value *workerv1.SemanticTopologyRef) {
		if value == nil || seen[value] {
			return
		}
		seen[value] = true
		bytes += int(unsafe.Sizeof(workerv1.SemanticTopologyRef{})) + proto.Size(value) + cap(value.SourceIds)*stringBytes
	}
	bytes += (cap(history.Lineage) + cap(history.Deleted) + cap(history.Ambiguous)) * pointerBytes
	for _, row := range history.Lineage {
		bytes += int(unsafe.Sizeof(workerv1.TopologyLineage{})) + cap(row.Sources)*pointerBytes
		ref(row.Result)
		for _, source := range row.Sources {
			ref(source)
		}
	}
	for _, row := range history.Deleted {
		bytes += int(unsafe.Sizeof(workerv1.TopologyTombstone{})) + len(row.Reason)
		ref(row.Source)
	}
	for _, row := range history.Ambiguous {
		bytes += int(unsafe.Sizeof(workerv1.AmbiguousLineage{})) + len(row.DiagnosticCode) + (cap(row.Sources)+cap(row.Candidates))*pointerBytes + len(row.ProtoReflect().GetUnknown())
		for _, source := range row.Sources {
			ref(source)
		}
		for _, candidate := range row.Candidates {
			ref(candidate)
		}
	}
	return bytes * 2
}

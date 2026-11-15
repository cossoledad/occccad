package workspace

import (
	"unsafe"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/proto"
)

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

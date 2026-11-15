package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

type decodedNamingBlock struct {
	view                   *topologyManifest
	layout                 []*workerv1.NamingHistorySpan
	previous               string
	previousRows, fullRows uint64
	retainedBytes          int
	policyID, evaluator    string
	queryOnly              bool
}
type namingHistoryLoader func(context.Context, *workerv1.NamingHistoryReference) (*decodedNamingBlock, error)
type namingHistoryLoadError struct{ error }

func decodeNamingBlock(ctx context.Context, data []byte, expected *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if uint64(len(data)) != expected.SizeBytes || hex.EncodeToString(sum[:]) != expected.Sha256 {
		return nil, fmt.Errorf("naming history block integrity mismatch")
	}
	block := &workerv1.NamingHistoryBlock{}
	if err := proto.Unmarshal(data, block); err != nil {
		return nil, err
	}
	v := block.Values
	if block.SchemaVersion != 1 || v == nil || len(v.History) != 0 || len(v.Transitions) != 1 || block.FullLineageCount != expected.FullLineageCount {
		return nil, fmt.Errorf("invalid naming history block contract")
	}
	if len(v.Bodies)+len(v.Results) != 1 || len(v.Bodies) == 1 && len(v.Bodies[0].Tip) != 0 || len(v.Results) == 1 && len(v.Results[0].Tip) != 0 {
		return nil, fmt.Errorf("history block contains a locator snapshot")
	}
	stage := v.Transitions[0]
	if stage.FeatureId != expected.FeatureId || stage.ResultGeometryId != expected.ResultGeometryId || v.GeometryId != expected.ResultGeometryId {
		return nil, fmt.Errorf("history block stage mismatch")
	}
	// Literals retain original event order; carries are inert self-continuations.
	// Validate runs without expanding the potentially large inherited row set.
	var total, literal uint64
	type interval struct{ start, end uint64 }
	var inherited []interval
	for i, span := range block.Layout {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		limit := uint64(len(stage.Lineage))
		if span.Carry {
			limit = block.PreviousLineageCount
		}
		if span.Count == 0 || span.Offset > limit || span.Count > limit-span.Offset || span.Count > block.FullLineageCount || total > block.FullLineageCount-span.Count {
			return nil, fmt.Errorf("invalid naming history span")
		}
		if span.Carry {
			inherited = append(inherited, interval{span.Offset, span.Offset + span.Count})
		} else {
			if span.Offset != literal {
				return nil, fmt.Errorf("naming literal order mismatch")
			}
			literal += span.Count
		}
		total += span.Count
	}
	if total != block.FullLineageCount || literal != uint64(len(stage.Lineage)) {
		return nil, fmt.Errorf("incomplete naming history layout")
	}
	sort.Slice(inherited, func(i, j int) bool { return inherited[i].start < inherited[j].start })
	for i := 1; i < len(inherited); i++ {
		if inherited[i].start < inherited[i-1].end {
			return nil, fmt.Errorf("overlapping naming carry ranges")
		}
	}
	view, err := unpackNamingValues(v)
	if err != nil {
		return nil, err
	}
	return &decodedNamingBlock{view: view, layout: block.Layout, previous: block.PreviousSha256, previousRows: block.PreviousLineageCount, fullRows: block.FullLineageCount, retainedBytes: len(data) * 16, policyID: v.PolicyId, evaluator: v.EvaluatorVersion}, nil
}

// Query consumers use ordered semantic events and current-tip evidence. Do not
// retain historical evidence/adjacency tables or carry layouts in their cache:
// those remain in the fully validated immutable artifact, available to explicit
// full-history loaders. Never mutate the decoded block or its protobuf values.
func namingBlockForQueries(block *decodedNamingBlock) *decodedNamingBlock {
	if block.queryOnly {
		return block
	}
	f := block.view.FeatureResults[0]
	h := f.TopologyHistory
	rows := make([]*workerv1.TopologyLineage, len(h.Lineage))
	for i, row := range h.Lineage {
		rows[i] = &workerv1.TopologyLineage{Result: row.Result, Sources: row.Sources, Kind: row.Kind}
	}
	deleted := make([]*workerv1.TopologyTombstone, len(h.Deleted))
	for i, row := range h.Deleted {
		deleted[i] = &workerv1.TopologyTombstone{Source: row.Source, Reason: row.Reason}
	}
	history := &workerv1.TopologyHistory{SchemaVersion: h.SchemaVersion, FeatureId: h.FeatureId, InputGeometryId: h.InputGeometryId, ResultGeometryId: h.ResultGeometryId, PolicyDigest: h.PolicyDigest, EvidenceDigest: h.EvidenceDigest, Lineage: rows, Deleted: deleted, Ambiguous: h.Ambiguous}
	feature := &workerv1.FeatureResult{FeatureId: f.FeatureId, BodyId: f.BodyId, Result: f.Result, InputFeatureId: f.InputFeatureId, ProfileFeatureId: f.ProfileFeatureId, ResultGeometryId: f.ResultGeometryId, TopologyHistory: history, TopologyHistoryComplete: f.TopologyHistoryComplete, Diagnostics: f.Diagnostics}
	projected := *block
	projected.view = &topologyManifest{GeometryID: block.view.GeometryID, FeatureResults: []*workerv1.FeatureResult{feature}}
	projected.layout = nil
	projected.queryOnly = true
	projected.retainedBytes = namingQueryBlockCharge(feature) + len(projected.previous) + len(projected.policyID) + len(projected.evaluator) + 512
	return &projected
}

func unpackNamingRoot(ctx context.Context, m *workerv1.PartTopologyManifest, load namingHistoryLoader) (*topologyManifest, error) {
	if m.SchemaVersion != 3 || len(m.Transitions) != 0 || len(m.History) == 0 || len(m.Bodies)+len(m.Results) != 1 {
		return nil, fmt.Errorf("invalid naming root contract")
	}
	var body string
	var result *workerv1.GeometryResultReference
	var tip string
	var complete bool
	if len(m.Bodies) == 1 {
		b := m.Bodies[0]
		body = b.BodyId
		tip = b.TipFeatureId
		complete = b.Complete
		if body == "" || len(b.Transitions) != 0 || b.TipTransition != 0 {
			return nil, fmt.Errorf("invalid naming body root")
		}
	} else {
		r := m.Results[0]
		result = r.Reference
		tip = r.TipFeatureId
		complete = r.Complete
		if result == nil || result.FeatureId == "" || result.ResultId == "" || len(r.Transitions) != 0 || r.TipTransition != 0 {
			return nil, fmt.Errorf("invalid naming result root")
		}
	}
	if load == nil {
		return nil, fmt.Errorf("naming history loader required")
	}
	out := &topologyManifest{GeometryID: m.GeometryId, BRepSHA256: m.BrepSha256, historyReferences: m.History}
	seen := map[string]bool{}
	features := map[string]bool{}
	previous := ""
	var previousRows uint64
	allComplete := true
	for _, reference := range m.History {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(reference.Sha256) != 64 || seen[reference.Sha256] || reference.SizeBytes == 0 || reference.FeatureId == "" || features[reference.FeatureId] {
			return nil, fmt.Errorf("invalid naming history reference")
		}
		seen[reference.Sha256] = true
		features[reference.FeatureId] = true
		block, err := load(ctx, reference)
		if err != nil {
			return nil, namingHistoryLoadError{err}
		}
		if block.previous != previous || block.previousRows != previousRows {
			return nil, fmt.Errorf("naming history predecessor mismatch")
		}
		feature := block.view.FeatureResults[0]
		if feature.BodyId != body || !proto.Equal(feature.Result, result) || feature.TopologyHistory.PolicyDigest != m.PolicyDigest || block.policyID != m.PolicyId || block.evaluator != m.EvaluatorVersion {
			return nil, fmt.Errorf("naming history owner or policy mismatch")
		}
		if feature.FeatureId != reference.FeatureId || feature.ResultGeometryId != reference.ResultGeometryId || block.fullRows != reference.FullLineageCount {
			return nil, fmt.Errorf("naming history reference mismatch")
		}
		allComplete = allComplete && feature.TopologyHistoryComplete
		out.FeatureResults = append(out.FeatureResults, feature)
		out.history = append(out.history, block)
		previous = reference.Sha256
		previousRows = block.fullRows
		out.retainedBytes += block.retainedBytes
	}
	last := out.FeatureResults[len(out.FeatureResults)-1]
	if last.FeatureId != tip || last.ResultGeometryId != m.GeometryId || allComplete != complete {
		return nil, fmt.Errorf("naming root tip mismatch")
	}
	// Decode the independent current locator table, then attach it to a fresh
	// tip view. Cached historical records remain immutable across revisions.
	meta := &workerv1.NamingTransition{FeatureId: last.FeatureId, BodyId: last.BodyId, Result: last.Result, Complete: last.TopologyHistoryComplete, ResultGeometryId: last.ResultGeometryId}
	values := &workerv1.PartTopologyManifest{SchemaVersion: 3, GeometryId: m.GeometryId, BrepSha256: m.BrepSha256, PolicyDigest: m.PolicyDigest, CoordinateSpace: m.CoordinateSpace, LengthUnit: m.LengthUnit, SemanticRefs: m.SemanticRefs, Evidence: m.Evidence, Transitions: []*workerv1.NamingTransition{meta}}
	if result == nil {
		values.Bodies = []*workerv1.NamingBody{{BodyId: body, Transitions: []uint32{1}, TipTransition: 1, Tip: m.Bodies[0].Tip}}
	} else {
		values.Results = []*workerv1.NamingGeometryResult{{Reference: result, Transitions: []uint32{1}, TipTransition: 1, Tip: m.Results[0].Tip}}
	}
	locators, err := unpackNamingValues(values)
	if err != nil {
		return nil, err
	}
	latest := &workerv1.FeatureResult{FeatureId: last.FeatureId, BodyId: last.BodyId, Result: last.Result, InputFeatureId: last.InputFeatureId, ProfileFeatureId: last.ProfileFeatureId, ResultGeometryId: last.ResultGeometryId, TopologyHistory: last.TopologyHistory, TopologyHistoryComplete: last.TopologyHistoryComplete, Diagnostics: last.Diagnostics, SemanticOutputs: locators.FeatureResults[0].SemanticOutputs}
	out.FeatureResults[len(out.FeatureResults)-1] = latest
	out.Locators = locators.Locators
	out.Tips = map[string]*workerv1.FeatureResult{}
	out.Results = map[modelcore.GeometryResultReference]*workerv1.FeatureResult{}
	if result == nil {
		out.Tips[body] = latest
	} else {
		out.Results[modelcore.GeometryResultReference{FeatureID: result.FeatureId, ResultID: result.ResultId}] = latest
	}
	return out, nil
}

// Explicit diagnostic/test expansion. Production selection, styling and
// contribution queries use the literal projection and never allocate these rows.
func expandNamingHistory(ctx context.Context, m *topologyManifest, maxRows uint64) ([]*workerv1.FeatureResult, error) {
	var out []*workerv1.FeatureResult
	var used uint64
	for i, block := range m.history {
		if block.queryOnly {
			return nil, fmt.Errorf("full history expansion requires full block loading")
		}
		if block.fullRows > maxRows-used {
			return nil, fmt.Errorf("naming history expansion budget exceeded")
		}
		used += block.fullRows
		f := proto.Clone(block.view.FeatureResults[0]).(*workerv1.FeatureResult)
		literals := f.TopologyHistory.Lineage
		f.TopologyHistory.Lineage = nil
		for _, span := range block.layout {
			for n := uint64(0); n < span.Count; n++ {
				if n%1024 == 0 {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
				}
				if span.Carry {
					old := out[i-1].TopologyHistory.Lineage[span.Offset+n]
					f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, &workerv1.TopologyLineage{Result: old.Result, Sources: []*workerv1.SemanticTopologyRef{old.Result}, Kind: workerv1.TopologyLineageKind_TOPOLOGY_LINEAGE_UNCHANGED, Evidence: old.Evidence})
				} else {
					f.TopologyHistory.Lineage = append(f.TopologyHistory.Lineage, literals[span.Offset+n])
				}
			}
		}
		out = append(out, f)
	}
	if len(out) > 0 {
		out[len(out)-1].SemanticOutputs = m.FeatureResults[len(m.FeatureResults)-1].SemanticOutputs
	}
	return out, nil
}

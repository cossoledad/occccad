package workspace

import (
	"fmt"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

// Consumer-local domain view, never marshaled to an artifact or database.
// Historical features carry transitions only; full output locators exist at explicit body tips.
type topologyManifest struct {
	resolutionPrefix  string
	historyTipDigest  string
	BRepSHA256        string
	GeometryID        string
	Locators          map[namingLocatorKey]namingLocator
	FeatureResults    []*workerv1.FeatureResult
	Tips              map[string]*workerv1.FeatureResult
	Results           map[modelcore.GeometryResultReference]*workerv1.FeatureResult
	search            *namingSearchIndex
	retainedBytes     int
	history           []*decodedNamingBlock
	historyReferences []*workerv1.NamingHistoryReference
	associations      *FeatureAssociationIndex
}

type namingLocatorKey struct {
	kind workerv1.PersistentTopologyType
	id   uint64
}
type namingLocator struct {
	BodyID string
	Result *modelcore.GeometryResultReference
	Output *workerv1.SemanticTopologyOutput
}

func (m *topologyManifest) GetFeatureResults() []*workerv1.FeatureResult {
	if m == nil {
		return nil
	}
	return m.FeatureResults
}
func (m *topologyManifest) bodyTips() map[string]*workerv1.FeatureResult { return m.Tips }

func namingReferences(m *workerv1.PartTopologyManifest) ([]*workerv1.SemanticTopologyRef, error) {
	table := m.ReferenceDictionary
	if table == nil {
		return m.SemanticRefs, nil
	}
	if len(m.SemanticRefs) != 0 {
		return nil, fmt.Errorf("ambiguous naming reference encoding")
	}
	lists := make([][]string, len(table.SourceLists)+1)
	for i, list := range table.SourceLists {
		if len(list.Symbols) == 0 {
			return nil, fmt.Errorf("empty naming source list")
		}
		values := make([]string, len(list.Symbols))
		for j, id := range list.Symbols {
			if id == 0 || int(id) > len(table.Symbols) {
				return nil, fmt.Errorf("invalid naming source symbol %d", id)
			}
			values[j] = table.Symbols[id-1]
		}
		lists[i+1] = values
	}
	refs := make([]*workerv1.SemanticTopologyRef, len(table.Rows))
	for i, row := range table.Rows {
		if row.Feature == 0 || int(row.Feature) > len(table.Symbols) || row.Slot == 0 || int(row.Slot) > len(table.Symbols) || int(row.Sources) >= len(lists) {
			return nil, fmt.Errorf("invalid naming reference row %d", i+1)
		}
		refs[i] = &workerv1.SemanticTopologyRef{FeatureId: table.Symbols[row.Feature-1], OutputSlot: table.Symbols[row.Slot-1], SourceIds: lists[row.Sources]}
	}
	return refs, nil
}

func unpackNamingValues(m *workerv1.PartTopologyManifest) (*topologyManifest, error) {
	return unpackNamingValuesWithEvidence(m, true, nil)
}

type predecessorAdjacency struct {
	position int
	row      uint32
}

type namingEvidencePredecessor struct {
	count uint64
	uses  map[*workerv1.SelectionEvidence][]predecessorAdjacency
}

func unpackNamingValuesWithEvidence(m *workerv1.PartTopologyManifest, includeEvidence bool, predecessor *namingEvidencePredecessor) (*topologyManifest, error) {
	if len(m.Bodies)+len(m.Results) != 1 || m.GeometryId == "" || m.SchemaVersion != 3 || m.CoordinateSpace != "PART_LOCAL" || m.LengthUnit != "mm" {
		return nil, fmt.Errorf("unsupported naming artifact contract")
	}
	var ownerBody string
	var ownerResult *workerv1.GeometryResultReference
	var transitions []uint32
	var tipTransition uint32
	var tipOutputs []*workerv1.NamingTopologyLocator
	if len(m.Bodies) == 1 {
		b := m.Bodies[0]
		ownerBody = b.BodyId
		transitions = b.Transitions
		tipTransition = b.TipTransition
		tipOutputs = b.Tip
		if ownerBody == "" {
			return nil, fmt.Errorf("invalid naming body identity")
		}
	} else {
		r := m.Results[0]
		ownerResult = r.Reference
		transitions = r.Transitions
		tipTransition = r.TipTransition
		tipOutputs = r.Tip
		if ownerResult == nil || ownerResult.FeatureId == "" || ownerResult.ResultId == "" {
			return nil, fmt.Errorf("invalid naming result identity")
		}
	}
	matchesOwner := func(body string, r *workerv1.GeometryResultReference) bool {
		return body == ownerBody && ((ownerResult == nil && r == nil) || (ownerResult != nil && r != nil && ownerResult.FeatureId == r.FeatureId && ownerResult.ResultId == r.ResultId))
	}
	out := &topologyManifest{GeometryID: m.GeometryId, BRepSHA256: m.BrepSha256, Locators: map[namingLocatorKey]namingLocator{}, Tips: map[string]*workerv1.FeatureResult{}, Results: map[modelcore.GeometryResultReference]*workerv1.FeatureResult{}}
	refs, err := namingReferences(m)
	if err != nil {
		return nil, err
	}
	var failure error
	ref := func(id uint32) *workerv1.SemanticTopologyRef {
		if id == 0 || uint64(id) > uint64(len(refs)) {
			failure = fmt.Errorf("invalid naming semantic table index %d", id)
			return nil
		}
		return refs[id-1]
	}
	adjacency := func(e *workerv1.SelectionEvidence, indices []uint32) {
		for position, id := range indices {
			if predecessor != nil {
				index := id >> 1
				if id&1 != 0 {
					if index == 0 || uint64(index) > predecessor.count {
						failure = fmt.Errorf("invalid naming predecessor adjacency index %d", index)
					}
					if e != nil {
						predecessor.uses[e] = append(predecessor.uses[e], predecessorAdjacency{position: position, row: index})
						e.Adjacent = append(e.Adjacent, nil)
					}
					continue
				}
				id = index
			}
			value := ref(id)
			if e != nil {
				e.Adjacent = append(e.Adjacent, value)
			}
		}
	}
	var evidence []*workerv1.SelectionEvidence
	if includeEvidence {
		evidence = make([]*workerv1.SelectionEvidence, len(m.Evidence))
	}
	for i, e := range m.Evidence {
		var frame *workerv1.NamingFrame
		var point *workerv1.Vec3
		var r *workerv1.NamingCurveRange
		var kind string
		isPoint := false
		switch g := e.Geometry.(type) {
		case *workerv1.NamingEvidence_Plane:
			kind, frame = "PLANE", g.Plane
		case *workerv1.NamingEvidence_Cylinder:
			kind, frame = "CYLINDER", g.Cylinder
		case *workerv1.NamingEvidence_Point:
			kind, point = "POINT", g.Point.Position
			isPoint = true
		case *workerv1.NamingEvidence_Line:
			kind, frame, r = "LINE", g.Line.Frame, g.Line.Range
		case *workerv1.NamingEvidence_Circle:
			kind, frame, r = "CIRCLE", g.Circle.Frame, g.Circle.Range
		case *workerv1.NamingEvidence_Spline:
			kind, frame, r = g.Spline.Family, g.Spline.Frame, g.Spline.Range
		case *workerv1.NamingEvidence_Other:
			kind, frame, r = g.Other.Family, g.Other.Frame, g.Other.Range
		default:
			return nil, fmt.Errorf("missing typed naming evidence")
		}
		if r != nil {
			expected := "NATIVE_CURVE_PARAMETER"
			if kind == "LINE" {
				expected = "LINE_MM"
			} else if kind == "CIRCLE" || kind == "ELLIPSE" {
				expected = "ANGLE_RADIANS"
			}
			if r.Convention != expected {
				return nil, fmt.Errorf("unknown curve parameter convention")
			}
		}
		if !includeEvidence {
			adjacency(nil, e.Adjacent)
			continue
		}
		v := &workerv1.SelectionEvidence{Centroid: e.Centroid, MeasureSi: e.MeasureSi, MeasureDimension: e.MeasureDimension, EvidenceDigest: e.EvidenceDigest, EndpointRole: e.EndpointRole, GeometryType: kind}
		v.MinorRadiusMm = e.MinorRadiusMm
		v.RadiusMm, v.HalfAngleRadians, v.ConeLeaf, v.MaterialSide, v.XDirection = e.RadiusMm, e.HalfAngleRadians, e.ConeLeaf, e.MaterialSide, e.XDirection
		adjacency(v, e.Adjacent)
		if isPoint {
			v.Origin, v.Direction = point, &workerv1.Vec3{}
		}
		if frame != nil {
			v.Origin, v.Direction = frame.Origin, frame.Direction
		}
		if r != nil {
			v.ParameterStart, v.ParameterEnd = r.Start, r.End
		}
		evidence[i] = v
	}
	ev := func(id uint32) *workerv1.SelectionEvidence {
		if id == 0 || uint64(id) > uint64(len(m.Evidence)) {
			failure = fmt.Errorf("invalid naming evidence table index %d", id)
			return nil
		}
		if includeEvidence {
			return evidence[id-1]
		}
		return nil
	}
	for _, t := range m.Transitions {
		if !matchesOwner(t.BodyId, t.Result) {
			return nil, fmt.Errorf("cross-body naming transition")
		}
		h := &workerv1.TopologyHistory{SchemaVersion: 1, FeatureId: t.FeatureId, InputGeometryId: t.InputGeometryId, ResultGeometryId: t.ResultGeometryId, EvidenceDigest: t.EvidenceDigest, PolicyDigest: m.PolicyDigest}
		for _, l := range t.Lineage {
			v := &workerv1.TopologyLineage{Result: ref(l.Result), Kind: l.Kind, Evidence: ev(l.Evidence)}
			for _, id := range l.Sources {
				v.Sources = append(v.Sources, ref(id))
			}
			h.Lineage = append(h.Lineage, v)
		}
		for _, d := range t.Deleted {
			h.Deleted = append(h.Deleted, &workerv1.TopologyTombstone{Source: ref(d.Source), Reason: d.Reason, Evidence: ev(d.Evidence)})
		}
		for _, a := range t.Ambiguous {
			v := &workerv1.AmbiguousLineage{DiagnosticCode: a.DiagnosticCode}
			for _, id := range a.Sources {
				v.Sources = append(v.Sources, ref(id))
			}
			for _, id := range a.Candidates {
				v.Candidates = append(v.Candidates, ref(id))
			}
			h.Ambiguous = append(h.Ambiguous, v)
		}
		out.FeatureResults = append(out.FeatureResults, &workerv1.FeatureResult{FeatureId: t.FeatureId, BodyId: t.BodyId, Result: t.Result, InputFeatureId: t.InputFeatureId, ProfileFeatureId: t.ProfileFeatureId, ResultGeometryId: t.ResultGeometryId, TopologyHistory: h, TopologyHistoryComplete: t.Complete, Diagnostics: t.Diagnostics})
	}
	seen := map[uint32]bool{}

	{
		if tipTransition == 0 || uint64(tipTransition) > uint64(len(out.FeatureResults)) {
			return nil, fmt.Errorf("invalid naming body tip")
		}
		tip := out.FeatureResults[tipTransition-1]
		if !matchesOwner(tip.BodyId, tip.Result) {
			return nil, fmt.Errorf("tip belongs to another body")
		}
		var previous uint32
		for _, id := range transitions {
			if id <= previous || uint64(id) > uint64(len(out.FeatureResults)) || seen[id] || !matchesOwner(out.FeatureResults[id-1].BodyId, out.FeatureResults[id-1].Result) {
				return nil, fmt.Errorf("invalid body transition chain")
			}
			seen[id] = true
			previous = id
		}
		if previous != tipTransition {
			return nil, fmt.Errorf("body tip not at end of transition chain")
		}
		var resultRef *modelcore.GeometryResultReference
		if ownerResult != nil {
			resultRef = &modelcore.GeometryResultReference{FeatureID: ownerResult.FeatureId, ResultID: ownerResult.ResultId}
			out.Results[*resultRef] = tip
		} else {
			out.Tips[ownerBody] = tip
		}
		for _, o := range tipOutputs {
			key := namingLocatorKey{o.TopologyType, o.LocalId}
			if _, exists := out.Locators[key]; exists || o.LocalId == 0 || o.TopologyType < 1 || o.TopologyType > 3 {
				return nil, fmt.Errorf("invalid or duplicate tip locator")
			}
			output := &workerv1.SemanticTopologyOutput{TopologyType: o.TopologyType, LocalId: o.LocalId, SemanticRef: ref(o.SemanticRef), Evidence: ev(o.Evidence)}
			tip.SemanticOutputs = append(tip.SemanticOutputs, output)
			out.Locators[key] = namingLocator{BodyID: ownerBody, Result: resultRef, Output: output}
		}
	}
	if len(seen) != len(m.Transitions) {
		return nil, fmt.Errorf("unowned naming transition")
	}
	if failure != nil {
		return nil, failure
	}
	return out, nil
}

func (m *topologyManifest) selectionTip(selection modelcore.PersistentSelection) *workerv1.FeatureResult {
	if selection.SourceResult != nil {
		return m.Results[*selection.SourceResult]
	}
	return m.Tips[selection.SourceBodyID]
}
func selectionOwnsFeature(selection modelcore.PersistentSelection, f *workerv1.FeatureResult) bool {
	if r := selection.SourceResult; r != nil {
		return f.Result != nil && f.BodyId == "" && f.Result.FeatureId == r.FeatureID && f.Result.ResultId == r.ResultID
	}
	return f.Result == nil && f.BodyId == selection.SourceBodyID
}

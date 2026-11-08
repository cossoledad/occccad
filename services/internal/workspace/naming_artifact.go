package workspace

import (
	"fmt"
	"github.com/occccad/occccad/internal/modelcore"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
)

// Consumer-local domain view, never marshaled to an artifact or database.
// Historical features carry transitions only; full output locators exist at explicit body tips.
type topologyManifest struct {
	BRepSHA256     string
	GeometryID     string
	Locators       map[string]namingLocator
	FeatureResults []*workerv1.FeatureResult
	Tips           map[string]*workerv1.FeatureResult
	Results        map[modelcore.GeometryResultReference]*workerv1.FeatureResult
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

func unpackNaming(m *workerv1.PartTopologyManifest) (*topologyManifest, error) {
	if len(m.Bodies)+len(m.Results) != 1 || m.GeometryId == "" || m.SchemaVersion != 2 || m.CoordinateSpace != "PART_LOCAL" || m.LengthUnit != "mm" {
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
	out := &topologyManifest{GeometryID: m.GeometryId, BRepSHA256: m.BrepSha256, Locators: map[string]namingLocator{}, Tips: map[string]*workerv1.FeatureResult{}, Results: map[modelcore.GeometryResultReference]*workerv1.FeatureResult{}}
	var failure error
	ref := func(id uint32) *workerv1.SemanticTopologyRef {
		if id == 0 || uint64(id) > uint64(len(m.SemanticRefs)) {
			failure = fmt.Errorf("invalid naming semantic table index %d", id)
			return nil
		}
		return m.SemanticRefs[id-1]
	}
	evidence := make([]*workerv1.SelectionEvidence, len(m.Evidence))
	for i, e := range m.Evidence {
		v := &workerv1.SelectionEvidence{Centroid: e.Centroid, MeasureSi: e.MeasureSi, MeasureDimension: e.MeasureDimension, EvidenceDigest: e.EvidenceDigest, EndpointRole: e.EndpointRole}
		v.MinorRadiusMm = e.MinorRadiusMm
		v.RadiusMm, v.HalfAngleRadians, v.ConeLeaf, v.MaterialSide, v.XDirection = e.RadiusMm, e.HalfAngleRadians, e.ConeLeaf, e.MaterialSide, e.XDirection
		for _, id := range e.Adjacent {
			v.Adjacent = append(v.Adjacent, ref(id))
		}
		var frame *workerv1.NamingFrame
		var r *workerv1.NamingCurveRange
		switch g := e.Geometry.(type) {
		case *workerv1.NamingEvidence_Plane:
			v.GeometryType = "PLANE"
			frame = g.Plane
		case *workerv1.NamingEvidence_Cylinder:
			v.GeometryType = "CYLINDER"
			frame = g.Cylinder
		case *workerv1.NamingEvidence_Point:
			v.GeometryType = "POINT"
			v.Origin = g.Point.Position
			v.Direction = &workerv1.Vec3{}
		case *workerv1.NamingEvidence_Line:
			v.GeometryType = "LINE"
			frame = g.Line.Frame
			r = g.Line.Range
		case *workerv1.NamingEvidence_Circle:
			v.GeometryType = "CIRCLE"
			frame = g.Circle.Frame
			r = g.Circle.Range
		case *workerv1.NamingEvidence_Spline:
			v.GeometryType = g.Spline.Family
			frame = g.Spline.Frame
			r = g.Spline.Range
		case *workerv1.NamingEvidence_Other:
			v.GeometryType = g.Other.Family
			frame = g.Other.Frame
			r = g.Other.Range
		default:
			return nil, fmt.Errorf("missing typed naming evidence")
		}
		if frame != nil {
			v.Origin = frame.Origin
			v.Direction = frame.Direction
		}
		if r != nil {
			expected := "NATIVE_CURVE_PARAMETER"
			if v.GeometryType == "LINE" {
				expected = "LINE_MM"
			} else if v.GeometryType == "CIRCLE" || v.GeometryType == "ELLIPSE" {
				expected = "ANGLE_RADIANS"
			}
			if r.Convention != expected {
				return nil, fmt.Errorf("unknown curve parameter convention")
			}
			v.ParameterStart = r.Start
			v.ParameterEnd = r.End
		}
		evidence[i] = v
	}
	ev := func(id uint32) *workerv1.SelectionEvidence {
		if id == 0 || uint64(id) > uint64(len(evidence)) {
			failure = fmt.Errorf("invalid naming evidence table index %d", id)
			return nil
		}
		return evidence[id-1]
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
	locators := map[string]bool{}
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
		for _, o := range tipOutputs {
			key := fmt.Sprintf("%d:%d", o.TopologyType, o.LocalId)
			if o.LocalId == 0 || o.TopologyType < 1 || o.TopologyType > 3 || locators[key] {
				return nil, fmt.Errorf("invalid or duplicate tip locator")
			}
			locators[key] = true
			tip.SemanticOutputs = append(tip.SemanticOutputs, &workerv1.SemanticTopologyOutput{TopologyType: o.TopologyType, LocalId: o.LocalId, SemanticRef: ref(o.SemanticRef), Evidence: ev(o.Evidence)})
		}
		for _, o := range tip.SemanticOutputs {
			out.Locators[fmt.Sprintf("%d:%d", o.TopologyType, o.LocalId)] = namingLocator{BodyID: ownerBody, Output: o}
		}
		if ownerResult != nil {
			r := modelcore.GeometryResultReference{FeatureID: ownerResult.FeatureId, ResultID: ownerResult.ResultId}
			out.Results[r] = tip
			for _, o := range tip.SemanticOutputs {
				locator := out.Locators[fmt.Sprintf("%d:%d", o.TopologyType, o.LocalId)]
				locator.Result = &r
				out.Locators[fmt.Sprintf("%d:%d", o.TopologyType, o.LocalId)] = locator
			}
		} else {
			out.Tips[ownerBody] = tip
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

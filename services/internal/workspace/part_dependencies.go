package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

type PartDependencyRequest struct {
	ContextReadScope
	VersionID string `json:"versionId"`
}

// Inputs explain the saved contract. DEFINED topology is not a claim that a
// selector was re-resolved by this read-only graph projection.
type PartDependencyInput struct {
	Role       string                         `json:"role"`
	Kind       string                         `json:"kind"`
	SourceID   string                         `json:"sourceId,omitempty"`
	SourceName string                         `json:"sourceName,omitempty"`
	RegionID   string                         `json:"regionId,omitempty"`
	EntityID   string                         `json:"entityId,omitempty"`
	State      string                         `json:"state"`
	Selection  *modelcore.PersistentSelection `json:"selection,omitempty"`
}
type PartDependencyNode struct {
	Key            modelcore.DependencyKey `json:"key"`
	Type           string                  `json:"type"`
	Label          string                  `json:"label"`
	EntityID       string                  `json:"entityId,omitempty"`
	OwnerFeatureID string                  `json:"ownerFeatureId,omitempty"`
	BodyID         string                  `json:"bodyId,omitempty"`
	State          string                  `json:"state"`
	Diagnostic     string                  `json:"diagnostic,omitempty"`
	Inputs         []PartDependencyInput   `json:"inputs,omitempty"`
}
type PartDependencies struct {
	DocumentID            string                     `json:"documentId"`
	VersionID             string                     `json:"versionId"`
	ContextVariantID      string                     `json:"contextVariantId,omitempty"`
	RootProductDocumentID string                     `json:"rootProductDocumentId,omitempty"`
	RootProductRevisionID string                     `json:"rootProductRevisionId,omitempty"`
	InstancePath          *InstancePath              `json:"instancePath,omitempty"`
	Digest                string                     `json:"digest"`
	Nodes                 []PartDependencyNode       `json:"nodes"`
	Edges                 []modelcore.DependencyEdge `json:"edges"`
}

func (s *Service) GetPartDependencies(ctx context.Context, documentID string, request PartDependencyRequest) (PartDependencies, error) {
	if request.RootProductDocumentID == "" && (request.RootProductRevisionID != "" || request.ContextVariantID != "" || request.InstancePath != nil) {
		return PartDependencies{}, fmt.Errorf("%w: complete dependency read scope required", ErrValidation)
	}
	view, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return PartDependencies{}, err
	}
	if view.Part == nil || request.VersionID != view.Document.VersionID {
		return PartDependencies{}, fmt.Errorf("%w: dependency snapshot changed", ErrValidation)
	}
	if err = s.acceptedContextPart(ctx, documentID, &view, request.ContextReadScope); err != nil {
		return PartDependencies{}, err
	}
	result, err := partDependencyProjection(view)
	result.ContextVariantID = request.ContextVariantID
	result.InstancePath = request.InstancePath
	result.RootProductDocumentID = request.RootProductDocumentID
	result.RootProductRevisionID = request.RootProductRevisionID
	return result, err
}

func partDependencyProjection(view DocumentView) (PartDependencies, error) {
	if view.Part == nil {
		return PartDependencies{}, fmt.Errorf("%w: Part required", ErrValidation)
	}
	m := *view.Part
	data, _ := json.Marshal(m)
	graph, manifest, err := buildPartEvaluation(m, view.Document.VersionID, canonicalModelHash(data), nil, nil)
	if err != nil {
		return PartDependencies{}, err
	}
	digest, err := graph.Digest()
	if err != nil {
		return PartDependencies{}, err
	}
	result := PartDependencies{DocumentID: view.Document.ID, VersionID: view.Document.VersionID, Digest: digest, Nodes: []PartDependencyNode{}, Edges: graph.Edges}
	for key, definition := range graph.Nodes {
		n := PartDependencyNode{Key: key, Type: definition.Type, Label: definition.Type, State: manifest.NodeResults[key].Status}
		if n.State == "" {
			n.State = "CURRENT"
		}
		parts := strings.SplitN(string(key), ":", 2)
		if len(parts) == 2 {
			n.EntityID = parts[1]
		}
		switch parts[0] {
		case "feature":
			for _, f := range m.Features {
				if f.ID != n.EntityID {
					continue
				}
				n.Label = f.Name
				n.BodyID = f.BodyID
				n.Diagnostic = f.Diagnostic
				n.Inputs = partFeatureDependencyInputs(view, f)
				if f.Suppressed {
					n.State = "SUPPRESSED"
				}
				if f.Sketch != nil && f.Sketch.Support.Status == "FAILED_SUPPORT" {
					n.State = "FAILED"
					n.Diagnostic = f.Sketch.Support.Diagnostic
				}
				if f.Sketch != nil {
					if issues := sketchOffsetIssues(*f.Sketch); len(issues) > 0 {
						n.State = "FAILED"
						n.Diagnostic = issues[0].Message
					}
				}
				break
			}
		case "profile":
			n.OwnerFeatureID = n.EntityID
			for _, f := range m.Features {
				if f.ID == n.EntityID {
					n.Label = f.Name + " · 轮廓"
					n.BodyID = f.BodyID
					break
				}
			}
			if a, ok := view.SketchAnalyses[n.EntityID]; ok {
				n.State = a.Status
				if a.GeometryVerified && len(a.RegionIDs) > 0 {
					n.State = "AVAILABLE"
				}
			}
		case "parameter":
			for _, p := range m.Parameters {
				if p.ParameterID == n.EntityID {
					n.Label = p.DisplayAlias
					if n.Label == "" {
						n.Label = p.Label
					}
					if n.Label == "" {
						n.Label = p.Key
					}
					n.OwnerFeatureID = p.OwnerFeatureID
					if p.CurrencyStatus != "" && p.CurrencyStatus != "CURRENT" {
						n.State = p.CurrencyStatus
					}
					break
				}
			}
		case "datum":
			for _, p := range m.DatumPlanes {
				if p.ID == n.EntityID {
					n.Label = p.Name
				}
			}
			for _, p := range m.DatumAxes {
				if p.ID == n.EntityID {
					n.Label = p.Name
				}
			}
			for _, p := range m.DatumPoints {
				if p.ID == n.EntityID {
					n.Label = p.Name
				}
			}
			for _, p := range m.AxisSystems {
				if p.ID == n.EntityID {
					n.Label = p.Name
				}
			}
		case "body":
			for _, b := range m.Bodies {
				if b.ID == n.EntityID {
					n.Label = b.Name
					n.BodyID = b.ID
				}
			}
		case "projection":
			ids := strings.SplitN(n.EntityID, "/", 2)
			n.OwnerFeatureID = ids[0]
			if len(ids) == 2 {
				n.EntityID = ids[1]
				for _, f := range m.Features {
					if f.ID == ids[0] && f.Sketch != nil {
						for _, e := range f.Sketch.ExternalGeometry {
							if e.ID == ids[1] {
								n.Label = f.Name + " · 投影"
								n.State = e.Status
								n.Diagnostic = e.Diagnostic
							}
						}
					}
				}
			}
		}
		if n.Label == "" {
			n.Label = n.Type
		}
		result.Nodes = append(result.Nodes, n)
	}
	slices.SortFunc(result.Nodes, func(a, b PartDependencyNode) int { return strings.Compare(string(a.Key), string(b.Key)) })
	return result, nil
}

func partFeatureDependencyInputs(view DocumentView, f Feature) []PartDependencyInput {
	inputs := []PartDependencyInput{}
	addSelection := func(role string, p *modelcore.PersistentSelection) {
		if p != nil {
			inputs = append(inputs, PartDependencyInput{Role: role, Kind: "TOPOLOGY", SourceID: p.Anchor.FeatureID, State: "DEFINED", Selection: p})
		}
	}
	if f.Profile != "" {
		if f.ProfileSelection != nil && f.ProfileSelection.Mode == "REGIONS" {
			a := view.SketchAnalyses[f.Profile]
			for _, id := range f.ProfileSelection.RegionIDs {
				state := "MISSING"
				if a.GeometryVerified && slices.Contains(a.RegionIDs, id) {
					state = "AVAILABLE"
				}
				inputs = append(inputs, PartDependencyInput{Role: "轮廓区域", Kind: "PROFILE_REGION", SourceID: f.Profile, RegionID: id, State: state})
			}
		} else {
			inputs = append(inputs, PartDependencyInput{Role: "整张草图", Kind: "PROFILE", SourceID: f.Profile, State: "DEFINED"})
		}
	}
	for _, p := range loftBoundaryPicks(f) {
		copy := p.Selection
		addSelection("放样连续性支撑", &copy)
	}
	for _, p := range f.Selections {
		selection := p.Selection
		addSelection("局部拓扑输入", &selection)
	}
	if f.TerminationPlane != nil {
		p := f.TerminationPlane.Selection
		addSelection("终止平面", &p)
	}
	if f.TerminationSurface != nil {
		p := f.TerminationSurface.Selection
		addSelection("终止曲面", &p)
	}
	if f.NeutralPlane != nil {
		p := f.NeutralPlane.Selection
		addSelection("中性平面", &p)
	}
	if f.ThinWall != nil {
		for _, id := range f.ThinWall.EntityIDs {
			inputs = append(inputs, PartDependencyInput{Role: "薄壁线链", Kind: "SKETCH_CURVE", SourceID: f.Profile, EntityID: id, State: "DEFINED"})
		}
	}
	if f.Sweep != nil {
		for _, id := range f.Sweep.Path.EntityIDs {
			inputs = append(inputs, PartDependencyInput{Role: "扫掠路径", Kind: "SKETCH_CURVE", SourceID: f.Sweep.Path.SketchID, EntityID: id, State: "DEFINED"})
		}
	}
	for _, p := range holePositions(f) {
		inputs = append(inputs, PartDependencyInput{Role: "孔位", Kind: "SKETCH_POINT", SourceID: p.SketchID, EntityID: p.Reference.EntityID, State: "DEFINED"})
	}
	if f.Sketch != nil {
		addSelection("草图支撑", f.Sketch.Support.PersistentSelection)
		if f.Sketch.Support.Type == "DATUM_PLANE" {
			inputs = append(inputs, PartDependencyInput{Role: "草图支撑", Kind: "DATUM", SourceID: f.Sketch.Support.DatumPlaneID, State: f.Sketch.Support.Status})
		}
		for _, e := range f.Sketch.ExternalGeometry {
			p := e.PersistentSelection
			if p.SourceDocumentID != "" {
				addSelection("投影来源", &p)
			}
		}
	}
	if view.Part != nil {
		for i := range inputs {
			for _, source := range view.Part.Features {
				if source.ID == inputs[i].SourceID {
					inputs[i].SourceName = source.Name
				}
			}
			for _, source := range view.Part.DatumPlanes {
				if source.ID == inputs[i].SourceID {
					inputs[i].SourceName = source.Name
				}
			}
		}
	}
	return inputs
}

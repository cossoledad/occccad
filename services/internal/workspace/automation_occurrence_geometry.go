package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

type AutomationGeometry struct {
	ID             string              `json:"id"`
	Kind           string              `json:"kind"`
	Source         string              `json:"source"` // TOPOLOGY | DATUM | PUBLICATION
	BodyID         string              `json:"bodyId,omitempty"`
	Name           string              `json:"name,omitempty"`
	Reference      AssemblyGeometryRef `json:"reference"`
	Definition     AutomationSupport   `json:"definition"`
	World          AutomationSupport   `json:"world"`
	AxialRangeMM   *[2]float64         `json:"axialRangeMM,omitempty"`
	Bounds         map[string]any      `json:"definitionBoundsMM,omitempty"`
	ManifestDigest string              `json:"manifestDigest,omitempty"`
}

type AutomationSupport struct {
	Kind           string     `json:"kind"`
	Origin         [3]float64 `json:"originMM"`
	Direction      [3]float64 `json:"direction,omitempty"`
	Radius         float64    `json:"radiusMM,omitempty"`
	MaterialSide   int32      `json:"materialSide,omitempty"`
	ParameterStart *float64   `json:"parameterStart,omitempty"`
	ParameterEnd   *float64   `json:"parameterEnd,omitempty"`
}

func automationSupport(value geometry.AssemblyGeometry) AutomationSupport {
	return AutomationSupport{Kind: value.Kind, Origin: value.Origin, Direction: value.Direction, Radius: value.Radius, MaterialSide: value.MaterialSide, ParameterStart: value.ParameterStart, ParameterEnd: value.ParameterEnd}
}

type AutomationGeometryPage struct {
	Items       []AutomationGeometry `json:"items"`
	Total       int                  `json:"total"`
	Unsupported int                  `json:"unsupported"`
	NextID      string               `json:"nextId,omitempty"`
	Complete    bool                 `json:"complete"`
}

// ReadAutomationOccurrenceGeometry consumes the authorized immutable occurrence
// returned above. Only selected page elements require precise B-Rep RPCs.
func (s *Service) ReadAutomationOccurrenceGeometry(ctx context.Context, occurrence AutomationOccurrence, body, kind, geometryType, after string, limit int) (AutomationGeometryPage, error) {
	out := AutomationGeometryPage{Items: []AutomationGeometry{}}
	if limit < 1 || limit > 20 || occurrence.SourceRevisionID == "" || occurrence.acl == nil {
		return out, fmt.Errorf("%w: bounded occurrence geometry page required", ErrValidation)
	}
	if occurrence.ContextVariantRequired {
		return out, fmt.Errorf("%w: OCCURRENCE_CONTEXT_VARIANT_UNAVAILABLE: precise occurrence queries require its accepted ContextVariant geometry; definition geometry was not substituted", ErrValidation)
	}
	kind, geometryType = strings.ToUpper(kind), strings.ToUpper(geometryType)
	path := occurrence.Path
	base := AssemblyGeometryRef{InstanceID: path.Segments[0].InstanceID, InstancePath: &path, SourceVersionID: occurrence.SourceRevisionID}
	makeItem := func(id, source, name string, ref AssemblyGeometryRef, descriptor geometry.AssemblyGeometry) AutomationGeometry {
		return AutomationGeometry{ID: id, Kind: descriptor.Kind, Source: source, Name: name, Reference: ref, Definition: automationSupport(descriptor), World: automationSupport(assemblyGeometryInBody(descriptor, occurrence.WorldPose))}
	}
	items := []AutomationGeometry{}
	if kind == "DATUM" || kind == "PUBLICATION" {
		if occurrence.SourceDocumentType == "PART" {
			part, err := decodeAssemblyPartModel(occurrence.Definition.Model)
			if err != nil {
				return out, err
			}
			if kind == "DATUM" {
				for _, system := range part.AxisSystems {
					for _, entry := range []struct {
						axis      string
						direction [3]float64
					}{{"X", system.XDirection}, {"Y", system.YDirection}, {"Z", system.ZDirection}} {
						ref := base
						ref.Kind, ref.GeometryID, ref.Axis = "AXIS", system.ID, entry.axis
						items = append(items, makeItem("axis-system:"+system.ID+"/"+entry.axis, "DATUM", system.Name+" "+entry.axis, ref, geometry.AssemblyGeometry{Kind: "AXIS", Origin: system.Origin, Direction: entry.direction, LengthUnit: "mm"}))
					}
				}
				for _, axis := range part.DatumAxes {
					ref := base
					ref.Kind, ref.GeometryID = "AXIS", axis.ID
					items = append(items, makeItem("axis:"+axis.ID, "DATUM", axis.Name, ref, geometry.AssemblyGeometry{Kind: "AXIS", Origin: axis.Origin, Direction: axis.Direction, LengthUnit: "mm"}))
				}
				for _, plane := range part.DatumPlanes {
					ref := base
					ref.Kind, ref.GeometryID = "PLANE", plane.ID
					items = append(items, makeItem("plane:"+plane.ID, "DATUM", plane.Name, ref, geometry.AssemblyGeometry{Kind: "PLANE", Origin: plane.Origin, Direction: plane.Normal, LengthUnit: "mm"}))
				}
			} else {
				for _, publication := range part.Publications {
					if publication.Resolution.Status != "CONNECTED" {
						out.Unsupported++
						continue
					}
					ref := base
					ref.PublicationRef = &PublicationRef{PublicationID: publication.ID, ExpectedType: publication.Type, CompatibilityVersion: publication.CompatibilityVersion}
					if err := applyPublicationDescriptor(&ref, publication); err != nil {
						out.Unsupported++
						continue
					}
					value, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: ref.Kind}, publication.Resolution)
					if err != nil {
						out.Unsupported++
						continue
					}
					items = append(items, makeItem("publication:"+publication.ID, "PUBLICATION", publication.Name, ref, value))
				}
			}
		} else if occurrence.SourceDocumentType == "PRODUCT" && kind == "PUBLICATION" {
			var product ProductModel
			if err := json.Unmarshal(occurrence.Definition.Model, &product); err != nil {
				return out, err
			}
			for _, publication := range product.Publications {
				if publication.Resolution.Status != "CONNECTED" {
					out.Unsupported++
					continue
				}
				ref := base
				ref.PublicationRef = &PublicationRef{PublicationID: publication.ID, ExpectedType: publication.Type, CompatibilityVersion: publication.CompatibilityVersion}
				if err := applyPublicationDescriptor(&ref, Publication{Type: publication.Type, Resolution: publication.Resolution}); err != nil {
					out.Unsupported++
					continue
				}
				value, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: ref.Kind}, publication.Resolution)
				if err != nil {
					out.Unsupported++
					continue
				}
				items = append(items, makeItem("publication:"+publication.ID, "PUBLICATION", publication.Name, ref, value))
			}
		}
		filtered := items[:0]
		for _, item := range items {
			if geometryType == "" || item.Kind == geometryType {
				filtered = append(filtered, item)
			}
		}
		items = filtered
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		out.Total = len(items)
		found := after == ""
		for _, item := range items {
			if item.ID == after {
				found = true
				continue
			}
			if item.ID <= after {
				continue
			}
			if len(out.Items) == limit {
				out.NextID = out.Items[len(out.Items)-1].ID
				break
			}
			if item.Source == "PUBLICATION" {
				publication, err := s.readAutomationPublication(ctx, occurrence, item.Reference.PublicationRef.PublicationID, map[string]bool{})
				if err != nil {
					return out, err
				}
				value, err := assemblyGeometryFromPublication(geometry.AssemblyGeometry{Kind: item.Reference.Kind}, publication.Resolution)
				if err != nil {
					return out, err
				}
				item.Definition, item.World = automationSupport(value), automationSupport(assemblyGeometryInBody(value, occurrence.WorldPose))
			}
			out.Items = append(out.Items, item)
		}
		if !found {
			return out, fmt.Errorf("%w: occurrence cursor is outside this query", ErrValidation)
		}
		out.Complete = out.NextID == ""
		return out, nil
	}
	if occurrence.SourceDocumentType != "PART" || (kind != "FACE" && kind != "EDGE" && kind != "VERTEX") {
		return out, fmt.Errorf("%w: topology requires a Part occurrence and FACE/EDGE/VERTEX", ErrValidation)
	}
	part, err := decodeAssemblyPartModel(occurrence.Definition.Model)
	if err != nil {
		return out, err
	}
	if body == "" && len(part.Bodies) == 1 {
		body = part.Bodies[0].ID
	}
	if body == "" || bodyIndex(part, body) < 0 {
		return out, fmt.Errorf("%w: select an explicit Body for a multi-Body occurrence", ErrValidation)
	}
	topology, err := s.ReadBodyTopology(ctx, occurrence.SourceDocumentID, occurrence.SourceRevisionID, body, kind)
	if err != nil {
		return out, err
	}
	named := []BodyTopologyItem{}
	for _, item := range topology.Items {
		if geometryType != "" && geometryType != item.GeometryType {
			continue
		}
		switch item.GeometryType {
		case "PLANE", "CYLINDER", "CIRCLE", "LINE", "POINT":
			named = append(named, item)
		default:
			out.Unsupported++
		}
	}
	out.Total = len(named)
	found := after == ""
	for _, item := range named {
		if item.ID == after {
			found = true
			continue
		}
		if item.ID <= after {
			continue
		}
		if len(out.Items) == limit {
			out.NextID = out.Items[len(out.Items)-1].ID
			break
		}
		resolved, err := s.resolvedAssemblyTopologyProperties(ctx, occurrence.SourceDocumentID, ResolvePersistentSelectionRequest{Selection: item.Selection.Selection, SourceVersionID: occurrence.SourceRevisionID, TargetVersionID: occurrence.SourceRevisionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest})
		if err != nil {
			return out, err
		}
		if resolved.Resolution.Status != modelcore.SelectionResolved || resolved.Properties == nil {
			return out, fmt.Errorf("%w: precise occurrence support is %s", ErrValidation, resolved.Resolution.Status)
		}
		ref := base
		ref.Kind = kind
		selection := item.Selection.Selection
		ref.PersistentSelection = &selection
		value, err := assemblyGeometryFromProperties(geometry.AssemblyGeometry{}, *resolved.Properties)
		if err != nil {
			return out, err
		}
		entry := makeItem(item.ID, "TOPOLOGY", "", ref, value)
		entry.BodyID, entry.Bounds, entry.ManifestDigest = body, resolved.Properties.BBox, topology.ManifestDigest
		if value.Kind == "CYLINDER" {
			a, oka := resolved.Properties.Properties["vFirst"].(float64)
			b, okb := resolved.Properties.Properties["vLast"].(float64)
			if oka && okb && !math.IsInf(a, 0) && !math.IsInf(b, 0) && !math.IsNaN(a) && !math.IsNaN(b) {
				entry.AxialRangeMM = &[2]float64{a, b}
			}
		}
		out.Items = append(out.Items, entry)
	}
	if !found {
		return out, fmt.Errorf("%w: occurrence cursor is outside this query", ErrValidation)
	}
	out.Complete = out.NextID == ""
	return out, nil
}

// A forwarding Product Publication must not disclose a hidden descendant's
// descriptors merely because the intermediate Product is readable.
func (s *Service) readAutomationPublication(ctx context.Context, occurrence AutomationOccurrence, id string, visiting map[string]bool) (Publication, error) {
	key := occurrence.SourceDocumentID + "@" + occurrence.SourceRevisionID + "#" + id
	if visiting[key] || len(visiting) > instancePathMaxDepth {
		return Publication{}, fmt.Errorf("%w: Product Publication cycle/depth", ErrValidation)
	}
	visiting[key] = true
	if occurrence.SourceDocumentType == "PART" {
		part, err := decodeAssemblyPartModel(occurrence.Definition.Model)
		if err != nil {
			return Publication{}, err
		}
		for _, p := range part.Publications {
			if p.ID == id && p.Resolution.Status == "CONNECTED" {
				return p, nil
			}
		}
		return Publication{}, ErrNotFound
	}
	var product ProductModel
	if err := json.Unmarshal(occurrence.Definition.Model, &product); err != nil {
		return Publication{}, err
	}
	for _, p := range product.Publications {
		if p.ID != id {
			continue
		}
		path := InstancePath{RootDocumentID: occurrence.Root.DocumentID}
		for _, segment := range occurrence.Path.Segments {
			path.Segments = append(path.Segments, InstancePathSegment{InstanceID: segment.InstanceID})
		}
		for _, segment := range p.Target.InstancePath.Segments {
			path.Segments = append(path.Segments, InstancePathSegment{InstanceID: segment.InstanceID})
		}
		if len(path.Segments) == len(occurrence.Path.Segments) {
			return Publication{}, fmt.Errorf("%w: forwarding Publication target is empty", ErrValidation)
		}
		child, err := s.ReadAutomationOccurrence(ctx, occurrence.acl, occurrence.actor, occurrence.Root.DocumentID, occurrence.Root.RevisionID, path)
		if err != nil {
			return Publication{}, err
		}
		publication, err := s.readAutomationPublication(ctx, child, p.Target.PublicationID, visiting)
		if err != nil {
			return Publication{}, err
		}
		if !publicationReferenceCompatible(PublicationRef{PublicationID: publication.ID, ExpectedType: p.Type, CompatibilityVersion: p.CompatibilityVersion}, publication) {
			return Publication{}, fmt.Errorf("%w: forwarding Publication contract changed", ErrValidation)
		}
		if !publicationContractsCompatible(Publication{Type: p.Type, Contract: p.Contract}, publication) {
			return Publication{}, fmt.Errorf("%w: forwarding Publication geometry contract changed", ErrValidation)
		}
		// Stored forwarding resolution is a rebuildable projection and can
		// predate a pose-only edit. Derive from the accepted descendant's exact
		// definition and the immutable occurrence transforms, without updating
		// either definition or any current Head.
		publication = publicationThroughRigidPose(publication, inverseRelativePose(child.WorldPose, occurrence.WorldPose))
		publication.ID, publication.Name, publication.Type = p.ID, p.Name, p.Type
		publication.CompatibilityVersion, publication.Contract = p.CompatibilityVersion, p.Contract
		return publication, nil
	}
	return Publication{}, ErrNotFound
}

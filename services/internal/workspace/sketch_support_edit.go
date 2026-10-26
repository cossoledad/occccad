package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

// This is the existing Feature input-stage contract, also usable when the old
// sketch support cannot resolve. No final failed Body or displayFallback is read.
func (s *Service) sketchSupportInput(ctx context.Context, view DocumentView, r FeatureInputRequest, index int) (FeatureInput, error) {
	if r.BodyID != "" && bodyIndex(*view.Part, r.BodyID) < 0 {
		return FeatureInput{}, fmt.Errorf("%w: input Body missing", ErrValidation)
	}
	prefix := *view.Part
	prefix.Features = append([]Feature(nil), prefix.Features[:index]...)
	result := FeatureInput{VersionID: r.VersionID, Picks: []FeatureInputPick{}}
	for _, body := range prefix.Bodies {
		if r.BodyID != "" && body.ID != r.BodyID {
			continue
		}
		stage := ""
		for _, f := range prefix.Features {
			if f.BodyID == body.ID && isBodyFeature(f.Type) && !f.Suppressed {
				stage = f.ID
			}
		}
		if stage == "" {
			continue
		}
		key, err := s.evaluateBodyPrefix(ctx, "support-input/"+r.FeatureID+"/"+body.ID, prefix, body.ID, true)
		if err != nil {
			return result, featureInputStageError(prefix, stage, err)
		}
		a, err := s.loadArtifact(ctx, key)
		if err != nil {
			return result, err
		}
		a.BodyID = body.ID
		a.DisplayStageFeatureID = stage
		result.Artifacts = append(result.Artifacts, a)
		if r.GeometryKey != "" && r.GeometryKey != key {
			continue
		}
		if result.Artifact.GeometryKey == "" || r.GeometryKey == key {
			result.Artifact = a
			result.SourceFeatureID = stage
		}
		if r.Kind != "" || r.LocalID != 0 {
			if r.Kind != "FACE" || r.LocalID == 0 || r.GeometryKey != key {
				return result, fmt.Errorf("%w: exact upstream FACE required", ErrValidation)
			}
			manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
			if err != nil {
				return result, err
			}
			if !topologyHistoryComplete(manifest) || manifest.Tips[body.ID] == nil || manifest.Tips[body.ID].FeatureId != stage {
				return result, fmt.Errorf("%w: upstream support stage lacks complete Naming", ErrValidation)
			}
			output, _ := manifestOutput(manifest, modelcore.PersistentTopologyFace, r.LocalID)
			if output == nil {
				return result, fmt.Errorf("%w: face missing from upstream stage", ErrValidation)
			}
			evidence := selectionEvidence(output.GetEvidence())
			if evidence.GeometryType != "PLANE" {
				return result, fmt.Errorf("%w: SUPPORT_TYPE_MISMATCH: selected face is not planar", ErrValidation)
			}
			selection := modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: view.Document.ID, SourceBodyID: body.ID, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: modelcore.PersistentTopologyFace, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: evidence}
			if err := selection.Validate(); err != nil {
				return result, fmt.Errorf("%w: %v", ErrValidation, err)
			}
			pick := FeatureSelection{Selection: selection, SourceVersionID: r.VersionID, SourceFeatureID: stage}
			if err := s.contextDefinitionPick(ctx, r.ContextReadScope, &pick, key, r.LocalID); err != nil {
				return result, err
			}
			result.Selection = &pick
		}
	}
	if r.GeometryKey != "" && (result.Artifact.GeometryKey != r.GeometryKey || r.Kind != "" && result.Selection == nil) {
		return result, fmt.Errorf("%w: geometry is outside sketch upstream stages", ErrValidation)
	}
	return result, nil
}

func (s *Service) bindSketchReplacementSupport(ctx context.Context, documentID string, r CommandRequest) (SketchSupport, error) {
	view, err := s.GetDocument(ctx, documentID)
	if err != nil {
		return SketchSupport{}, err
	}
	if view.Part == nil || r.VersionID == "" || r.VersionID != view.Document.VersionID {
		return SketchSupport{}, fmt.Errorf("%w: sketch support editing revision changed", ErrValidation)
	}
	index := -1
	for i, f := range view.Part.Features {
		if f.ID == r.SketchID && f.Sketch != nil {
			index = i
		}
	}
	if index < 0 {
		return SketchSupport{}, fmt.Errorf("%w: sketch missing", ErrValidation)
	}
	if r.TargetKind != "" && r.TargetKind != "PLANE" && r.TargetKind != "FACE" {
		return SketchSupport{}, fmt.Errorf("%w: unsupported sketch support kind", ErrValidation)
	}
	if r.TargetKind != "FACE" {
		id := r.DatumPlaneID
		if id == "" {
			switch r.Plane {
			case "XY":
				id = "datum-xy"
			case "XZ":
				id = "datum-xz"
			case "YZ":
				id = "datum-yz"
			}
		}
		for _, d := range view.Part.DatumPlanes {
			if d.ID == id {
				return SketchSupport{Type: "DATUM_PLANE", DatumPlaneID: id, Plane: d.Plane, Origin: d.Origin, XDirection: d.UDirection, Normal: d.Normal, OrientationRule: sketchSupportOrientationRule, Status: "CONNECTED"}, nil
			}
		}
		return SketchSupport{}, fmt.Errorf("%w: datum plane missing", ErrValidation)
	}
	input, err := s.sketchSupportInput(ctx, view, FeatureInputRequest{VersionID: r.VersionID, FeatureID: r.SketchID, BodyID: r.BodyID, GeometryKey: r.GeometryKey, Kind: "FACE", LocalID: r.TopologyID}, index)
	if err != nil {
		return SketchSupport{}, err
	}
	if input.Selection == nil {
		return SketchSupport{}, fmt.Errorf("%w: upstream plane missing", ErrValidation)
	}
	pick := input.Selection
	properties, err := s.getTopologyElementPropertiesFromArtifact(ctx, input.Artifact.GeometryKey, "FACE", r.TopologyID)
	if err != nil {
		return SketchSupport{}, err
	}
	normal, ok := properties.Properties["normal"].([3]float64)
	if !ok {
		return SketchSupport{}, fmt.Errorf("%w: SUPPORT_FRAME_INVALID", ErrValidation)
	}
	x, ok := stableSupportX(normal, view.Part.Features[index].Sketch.Support.XDirection)
	if !ok {
		return SketchSupport{}, fmt.Errorf("%w: SUPPORT_FRAME_INVALID", ErrValidation)
	}
	return SketchSupport{Type: "PLANAR_FACE", Plane: "CUSTOM", PersistentSelection: &pick.Selection, SourceVersionID: pick.SourceVersionID, SourceFeatureID: pick.SourceFeatureID, Origin: pick.Selection.CreationEvidence.Origin, XDirection: x, Normal: normal, OrientationRule: sketchSupportOrientationRule, Status: "CONNECTED"}, nil
}

func rejectExplicitBrokenSupport(c modelcore.DomainCommand, m PartModel) error {
	ids := map[string]bool{}
	if c.TypeURI == typeCreateSketch {
		var p createFeaturePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return err
		}
		ids[p.Feature.ID] = true
	}
	if c.TypeURI == typeEditSketch {
		var p editSketchPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return err
		}
		for _, op := range p.Operations {
			if op.Type == "REASSIGN_SUPPORT" {
				ids[p.SketchID] = true
			}
		}
	}
	for _, f := range m.Features {
		if !ids[f.ID] || f.Sketch == nil {
			continue
		}
		support := f.Sketch.Support
		if support.Status != "CONNECTED" {
			return &sketchSupportFailure{diagnosticCode: support.DiagnosticCode, diagnostic: support.Diagnostic}
		}
		for _, e := range f.Sketch.ExternalGeometry {
			if e.Status != "CONNECTED" || e.Snapshot == nil {
				return &externalGeometryProjectionFailure{diagnosticCode: e.DiagnosticCode, diagnostic: e.Diagnostic}
			}
		}
	}
	return nil
}

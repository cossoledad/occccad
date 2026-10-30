package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/occccad/occccad/internal/modelcore"
)

func insertionBlocked(f Feature, reason string) error {
	return fmt.Errorf("%w: INSERT_FEATURE_BLOCKED[%s] %s: %s", ErrValidation, f.ID, f.Name, reason)
}

func insertionAnchor(m PartModel, body, id string) (int, error) {
	i, err := insertionBoundary(m, body, id)
	if err != nil {
		return -1, err
	}
	if !isBodyFeature(m.Features[i].Type) {
		return -1, insertionBlocked(m.Features[i], "anchor must be an active solid stage of the specified Body")
	}
	return i, nil
}

// A position is global. Its target Body's solid tip can precede that position
// (notably when the position is a sketch), and must retain its own identity.
func insertionBoundary(m PartModel, body, id string) (int, error) {
	i := slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == id })
	if i < 0 {
		return -1, fmt.Errorf("%w: INSERT_STAGE_MISSING", ErrValidation)
	}
	f := m.Features[i]
	if body == "" || f.BodyID != body || (!isBodyFeature(f.Type) && f.Sketch == nil) || f.Suppressed {
		return -1, insertionBlocked(f, "anchor must be an active solid or sketch of the specified Body")
	}
	return i, nil
}

func protectFeatureRanges(before, after PartModel, blocked func(Feature, string) error) error {
	for _, f := range before.Features {
		if f.Pattern != nil {
			for _, seed := range patternSeeds(f.Pattern) {
				p := *f.Pattern
				p.Source = seed.Source
				p.SourceKind = seed.SourceKind
				p.StartFeatureID = seed.StartFeatureID
				if p.SourceKind == "FEATURE_DELTA" && !slices.Equal(rangeMembers(before, &p), rangeMembers(after, &p)) {
					return blocked(f, "FEATURE_DELTA range membership/order is protected")
				}
			}
		}
	}
	return nil
}

func insertFeatureCandidate(m *PartModel, p createFeaturePayload) error {
	i, err := insertionBoundary(*m, p.Feature.BodyID, p.AfterFeatureID)
	if err != nil {
		return err
	}
	if p.InsertionVersionID == "" || p.InsertionGeometryKey == "" {
		return insertionBlocked(p.Feature, "Revision and exact stage GeometryKey are required")
	}
	anchor := m.Features[i]
	switch {
	case isLocalModifier(p.Feature.Type), p.Feature.Type == "SKETCH" && p.Feature.Sketch != nil:
		if !isBodyFeature(anchor.Type) {
			return insertionBlocked(anchor, "sketch/modifier creation requires a solid boundary")
		}
	case p.Feature.Type == "LINEAR_EXTRUDE":
		if anchor.Sketch == nil || p.Feature.Profile != anchor.ID || (p.Feature.Operation != "ADD" && p.Feature.Operation != "REMOVE") || (p.Feature.Extent != "" && p.Feature.Extent != "FINITE") || p.Feature.ProfileMemberSlot != nil {
			return insertionBlocked(p.Feature, "only finite ADD/REMOVE extrusion immediately after its sketch is supported")
		}
	default:
		return insertionBlocked(p.Feature, "historical creation type is unsupported")
	}
	for _, pick := range p.Feature.Selections {
		if pick.SourceFeatureID != p.AfterFeatureID || pick.SourceVersionID != p.InsertionVersionID || pick.Selection.SourceBodyID != p.Feature.BodyID {
			return insertionBlocked(p.Feature, "selection must be bound to the insertion stage")
		}
	}
	m.Features = slices.Insert(m.Features, i+1, p.Feature)
	for j := range m.Features {
		m.Features[j].Order = j + 1
	}
	return nil
}

func insertionPayload(c modelcore.DomainCommand) (createFeaturePayload, bool) {
	switch c.TypeURI {
	case typeCreateModifierFeature, typeCreateSolidFeature, typeCreatePad, typeCreateSketch, typeCreatePattern, typeCreateBooleanFeature, typeImportExchange:
	default:
		return createFeaturePayload{}, false
	}
	var p createFeaturePayload
	_ = json.Unmarshal(c.Payload, &p)
	return p, p.AfterFeatureID != ""
}
func requiresOrderedEvaluation(c modelcore.DomainCommand) bool {
	_, insert := insertionPayload(c)
	return c.TypeURI == typeMoveFeature || insert
}
func orderMutationBlocker(c modelcore.DomainCommand) func(Feature, string) error {
	if _, ok := insertionPayload(c); ok {
		return insertionBlocked
	}
	return moveBlocked
}

// Check the frozen pre-command stage, never a prefix containing the new feature.
// Keeping this at the service evaluation boundary also covers typed commands.
func (s *Service) validateInsertionSource(ctx context.Context, documentID, head string, c modelcore.DomainCommand, before PartModel, scope ContextReadScope) error {
	p, insert := insertionPayload(c)
	if !insert {
		return nil
	}
	validType := c.TypeURI == typeCreateModifierFeature && isLocalModifier(p.Feature.Type) || c.TypeURI == typeCreateSketch && p.Feature.Type == "SKETCH" || c.TypeURI == typeCreateSolidFeature && p.Feature.Type == "LINEAR_EXTRUDE"
	if !validType || p.InsertionVersionID != head {
		return insertionBlocked(p.Feature, "insertion Revision or command type is invalid")
	}
	if scope.RootProductDocumentID == "" && (scope.RootProductRevisionID != "" || scope.ContextVariantID != "" || scope.InstancePath != nil) {
		return insertionBlocked(p.Feature, "CONTEXT_VARIANT_CHANGED: incomplete context scope")
	}
	// Read the exact accepted occurrence, not a newly resolved source Head. The
	// independent model remains the authority for persisted Naming/evidence.
	view := DocumentView{Part: &before}
	view.Document.ID = documentID
	view.Document.VersionID = head
	if err := s.acceptedContextPart(ctx, documentID, &view, scope); err != nil {
		return err
	}
	shown := *view.Part
	key, targetStage, err := s.insertionStage(ctx, before, head, p.AfterFeatureID, p.Feature.BodyID, p.Feature.BodyID)
	if err != nil {
		return err
	}
	shownKey := key
	if scope.RootProductDocumentID != "" {
		shownKey, _, err = s.insertionStage(ctx, shown, head, p.AfterFeatureID, p.Feature.BodyID, p.Feature.BodyID)
		if err != nil {
			return err
		}
	}
	if shownKey != p.InsertionGeometryKey {
		return insertionBlocked(p.Feature, "GeometryKey does not belong to the insertion stage")
	}
	if p.Feature.Sketch != nil {
		support := p.Feature.Sketch.Support
		if support.Type == "PLANAR_FACE" {
			if support.PersistentSelection == nil || support.SourceFeatureID != targetStage || support.SourceVersionID != head || support.PersistentSelection.SourceDocumentID != documentID || support.PersistentSelection.SourceBodyID != p.Feature.BodyID || support.PersistentSelection.ExpectedType != modelcore.PersistentTopologyFace {
				return insertionBlocked(p.Feature, "sketch support must belong to its exact upstream Body stage")
			}
			resolution, err := s.resolveFeaturePick(ctx, FeatureSelection{Selection: *support.PersistentSelection, SourceVersionID: head, SourceFeatureID: targetStage}, key)
			if err != nil {
				return err
			}
			state, err := featureInputResolution(resolution, "PLANE")
			if err != nil {
				return err
			}
			if state.Status != modelcore.SelectionResolved {
				return insertionBlocked(p.Feature, string(state.Status)+": "+state.Diagnostic)
			}
			pick := FeatureSelection{Selection: *support.PersistentSelection, SourceVersionID: head, SourceFeatureID: targetStage}
			expected, e := s.definitionSketchSupport(ctx, pick, key)
			if e != nil {
				return e
			}
			if support.Origin != expected.Origin || support.Normal != expected.Normal || support.XDirection != expected.XDirection || support.OrientationRule != expected.OrientationRule {
				return insertionBlocked(p.Feature, "sketch support must retain the independent definition frame")
			}
			projected, e := s.resolveFeaturePick(ctx, pick, shownKey)
			if e != nil {
				return e
			}
			if projected.Status != modelcore.SelectionResolved || len(projected.Candidates) != 1 {
				return insertionBlocked(p.Feature, "sketch support does not uniquely correspond in the accepted context")
			}
			canonical := pick
			if e = s.contextDefinitionPick(ctx, scope, &canonical, shownKey, projected.Candidates[0].LocalID); e != nil {
				return e
			}
			if !reflect.DeepEqual(canonical.Selection.CreationEvidence, pick.Selection.CreationEvidence) {
				return insertionBlocked(p.Feature, "sketch support must retain independent definition evidence")
			}
		} else if support.Type != "DATUM_PLANE" {
			return insertionBlocked(p.Feature, "sketch requires a datum or upstream planar face")
		}
	}
	validateProjection := func(pick FeatureSelection, displayKey string, plane bool) error {
		if scope.RootProductDocumentID == "" {
			return nil
		}
		resolution, e := s.resolveFeaturePick(ctx, pick, displayKey)
		if e != nil {
			return e
		}
		state, e := featureInputResolution(resolution, func() string {
			if plane {
				return "PLANE"
			}
			return ""
		}())
		if e != nil {
			return e
		}
		if state.Status != modelcore.SelectionResolved {
			return insertionBlocked(p.Feature, "context input does not uniquely resolve")
		}
		canonical := pick
		if e = s.contextDefinitionPick(ctx, scope, &canonical, displayKey, resolution.Candidates[0].LocalID); e != nil {
			return e
		}
		if !reflect.DeepEqual(canonical.Selection.CreationEvidence, pick.Selection.CreationEvidence) {
			return insertionBlocked(p.Feature, "context input must retain independent definition evidence")
		}
		return nil
	}
	for _, pick := range p.Feature.Selections {
		kind := modelcore.PersistentTopologyFace
		if p.Feature.Type == "FILLET" || p.Feature.Type == "CHAMFER" {
			kind = modelcore.PersistentTopologyEdge
		}
		if pick.Selection.ExpectedType != kind {
			return insertionBlocked(p.Feature, "modifier input topology type mismatch")
		}
		if pick.Selection.SourceDocumentID != documentID || pick.SourceVersionID != head || pick.SourceFeatureID != p.AfterFeatureID || pick.Selection.SourceBodyID != p.Feature.BodyID {
			return insertionBlocked(p.Feature, "selection must belong to the insertion Revision, Body and stage")
		}
		resolution, err := s.resolveFeaturePick(ctx, pick, key)
		if err != nil {
			return err
		}
		if resolution.Status != modelcore.SelectionResolved {
			return insertionBlocked(p.Feature, string(resolution.Status)+": "+resolution.Diagnostic)
		}
		if err := validateProjection(pick, shownKey, false); err != nil {
			return err
		}
	}
	if pick := p.Feature.NeutralPlane; pick != nil {
		planeKey, stage, err := s.insertionStage(ctx, before, head, p.AfterFeatureID, p.Feature.BodyID, pick.Selection.SourceBodyID)
		if err != nil {
			return err
		}
		if pick.Selection.SourceDocumentID != documentID || pick.SourceVersionID != head || pick.SourceFeatureID != stage || pick.Selection.ExpectedType != modelcore.PersistentTopologyFace {
			return insertionBlocked(p.Feature, "neutral plane must belong to its exact upstream insertion stage")
		}
		resolution, err := s.resolveFeaturePick(ctx, *pick, planeKey)
		if err != nil {
			return err
		}
		state, err := featureInputResolution(resolution, "PLANE")
		if err != nil {
			return err
		}
		if state.Status != modelcore.SelectionResolved {
			return insertionBlocked(p.Feature, string(state.Status)+": "+state.Diagnostic)
		}
		if scope.RootProductDocumentID != "" {
			displayKey, _, e := s.insertionStage(ctx, shown, head, p.AfterFeatureID, p.Feature.BodyID, pick.Selection.SourceBodyID)
			if e != nil {
				return e
			}
			if e = validateProjection(*pick, displayKey, true); e != nil {
				return e
			}
		}
	}
	return nil
}

// Both target and reference Bodies are evaluated at the same global prefix.
// A semantic anchor is not the actual source stage, and another Body's final
// output must not leak backwards across the insertion point.
func (s *Service) insertionStage(ctx context.Context, model PartModel, revision, after, targetBody, sourceBody string) (string, string, error) {
	i, err := insertionBoundary(model, targetBody, after)
	if err != nil {
		return "", "", err
	}
	prefix := model
	prefix.Features = append([]Feature(nil), model.Features[:i+1]...)
	stage := ""
	for _, f := range prefix.Features {
		if f.BodyID == sourceBody && isBodyFeature(f.Type) && !f.Suppressed {
			stage = f.ID
		}
	}
	if stage == "" {
		return "", "", fmt.Errorf("%w: reference Body has no solid at the insertion stage", ErrValidation)
	}
	key, err := s.evaluateBodyPrefix(ctx, "insertion-input/"+revision+"/"+after, prefix, sourceBody, true)
	if err != nil {
		return "", "", featureInputStageError(prefix, stage, err)
	}
	return key, stage, nil
}

// Insertion is an explicit pick context, separate from display-only ResultStage.
func (s *Service) insertionInput(ctx context.Context, documentID string, request FeatureInputRequest, view DocumentView) (FeatureInput, error) {
	if request.ResultStage || request.FeatureID != "" {
		return FeatureInput{}, fmt.Errorf("%w: insertion and result/edit contexts cannot be combined", ErrValidation)
	}
	bodyID := request.BodyID
	if request.ReferenceBodyID != "" {
		bodyID = request.ReferenceBodyID
	}
	key, stage, err := s.insertionStage(ctx, *view.Part, request.VersionID, request.AfterFeatureID, request.BodyID, bodyID)
	if err != nil {
		return FeatureInput{}, err
	}
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	a.BodyID = bodyID
	a.DisplayStageFeatureID = stage
	result := FeatureInput{VersionID: request.VersionID, SourceFeatureID: stage, Artifact: a, Artifacts: []Artifact{a}, Picks: []FeatureInputPick{}}
	index, _ := insertionBoundary(*view.Part, request.BodyID, request.AfterFeatureID)
	refs, err := referenceGeometryBefore(*view.Part, index+1)
	if err != nil {
		return FeatureInput{}, err
	}
	result.ReferenceGeometry = &refs
	if request.LocalID == 0 && request.Kind == "" && request.GeometryKey == "" {
		return result, nil
	}
	if request.LocalID == 0 || (request.Kind != "EDGE" && request.Kind != "FACE") || request.ReferenceBodyID != "" && request.Kind != "FACE" || request.GeometryKey != key {
		return FeatureInput{}, fmt.Errorf("%w: INSERT_STAGE_PICK_OUTSIDE_INPUT", ErrValidation)
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	kind := modelcore.PersistentTopologyType(request.Kind)
	output, body := manifestOutput(manifest, kind, request.LocalID)
	if !topologyHistoryComplete(manifest) || output == nil || body != bodyID {
		return FeatureInput{}, fmt.Errorf("%w: insertion topology unavailable", ErrValidation)
	}
	pick := FeatureSelection{SourceVersionID: request.VersionID, SourceFeatureID: stage, Selection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceBodyID: body, Anchor: semanticRef(output.GetSemanticRef()), ExpectedType: kind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(output.GetEvidence())}}
	if request.ReferenceBodyID != "" && pick.Selection.CreationEvidence.GeometryType != "PLANE" {
		return FeatureInput{}, fmt.Errorf("%w: insertion reference must be a planar face", ErrValidation)
	}
	if err := pick.Selection.Validate(); err != nil {
		return FeatureInput{}, err
	}
	if err := s.contextDefinitionPick(ctx, request.ContextReadScope, &pick, key, request.LocalID); err != nil {
		return FeatureInput{}, err
	}
	result.Selection = &pick
	return result, nil
}

// Use the same recursive datum dependency edges as the creation/order gate.
// Numerical frames are read from this Revision, never from displayFallback.
func referenceGeometryBefore(model PartModel, limit int) (ReferenceGeometry, error) {
	graph, _, err := buildPartEvaluation(model, "", "", nil, nil)
	if err != nil {
		return ReferenceGeometry{}, err
	}
	indices := map[modelcore.DependencyKey]int{}
	incoming := map[modelcore.DependencyKey][]modelcore.DependencyKey{}
	for i, f := range model.Features {
		indices[modelcore.DependencyKey("feature:"+f.ID)] = i
	}
	for _, e := range graph.Edges {
		incoming[e.Target] = append(incoming[e.Target], e.Source)
	}
	var legal func(modelcore.DependencyKey, map[modelcore.DependencyKey]bool) bool
	legal = func(key modelcore.DependencyKey, seen map[modelcore.DependencyKey]bool) bool {
		if i, ok := indices[key]; ok {
			return i < limit
		}
		if seen[key] {
			return true
		}
		seen[key] = true
		for _, source := range incoming[key] {
			if (strings.HasPrefix(string(source), "feature:") || strings.HasPrefix(string(source), "datum:")) && !legal(source, seen) {
				return false
			}
		}
		return true
	}
	refs := ReferenceGeometry{DatumPlanes: []DatumPlane{}, DatumAxes: []DatumAxis{}, AxisSystems: []AxisSystem{}}
	for _, p := range model.DatumPlanes {
		if legal(modelcore.DependencyKey("datum:"+p.ID), map[modelcore.DependencyKey]bool{}) {
			refs.DatumPlanes = append(refs.DatumPlanes, p)
		}
	}
	for _, a := range model.DatumAxes {
		if legal(modelcore.DependencyKey("datum:"+a.ID), map[modelcore.DependencyKey]bool{}) {
			refs.DatumAxes = append(refs.DatumAxes, a)
		}
	}
	refs.AxisSystems = append(refs.AxisSystems, model.AxisSystems...)
	return refs, nil
}

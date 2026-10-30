package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math"
	"reflect"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const sketchSupportOrientationRule = "PROJECT_STORED_X_FACE_NORMAL_V2"

type sketchSupportFailure struct {
	diagnosticCode string
	diagnostic     string
}

func (failure *sketchSupportFailure) Error() string {
	detail := strings.TrimSpace(failure.diagnosticCode + ": " + failure.diagnostic)
	return "FAILED_SUPPORT: " + strings.Trim(detail, ": ")
}
func (failure *sketchSupportFailure) Unwrap() error   { return ErrValidation }
func (failure *sketchSupportFailure) Code() string    { return "FAILED_SUPPORT" }
func (failure *sketchSupportFailure) Phase() string   { return "SKETCH_SUPPORT" }
func (failure *sketchSupportFailure) Retryable() bool { return false }

// externalGeometryProjectionFailure is returned only for an explicit ADD or
// RECONNECT whose authoritative projection cannot be created. That command is
// a structural precondition failure and must not advance the Workspace Head.
// Existing connected references that become unresolved after an upstream edit
// continue to use the persisted FAILED Revision path.
type externalGeometryProjectionFailure struct {
	diagnosticCode string
	diagnostic     string
}

func (failure *externalGeometryProjectionFailure) Error() string {
	code := strings.TrimSpace(failure.diagnosticCode)
	if code == "" {
		code = "EXTERNAL_GEOMETRY_UNRESOLVED"
	}
	detail := strings.TrimSpace(failure.diagnostic)
	if detail == "" {
		return "external geometry projection failed: " + code
	}
	return "external geometry projection failed: " + code + ": " + detail
}
func (failure *externalGeometryProjectionFailure) Unwrap() error { return ErrValidation }
func (failure *externalGeometryProjectionFailure) Code() string {
	if code := strings.TrimSpace(failure.diagnosticCode); code != "" {
		return code
	}
	return "EXTERNAL_GEOMETRY_UNRESOLVED"
}
func (failure *externalGeometryProjectionFailure) Phase() string {
	return "EXTERNAL_GEOMETRY_PROJECTION"
}
func (failure *externalGeometryProjectionFailure) Retryable() bool { return false }

func (service *Service) rejectExplicitUnresolvedExternal(ctx context.Context, documentID string, command modelcore.DomainCommand, model PartModel) error {
	if command.TypeURI != typeEditSketch {
		return nil
	}
	var payload editSketchPayload
	if err := json.Unmarshal(command.Payload, &payload); err != nil {
		return err
	}
	requested := map[string]struct{}{}
	for _, operation := range payload.Operations {
		switch operation.Type {
		case "ADD_EXTERNAL_GEOMETRY":
			if operation.ExternalGeometry != nil {
				requested[operation.ExternalGeometry.ID] = struct{}{}
			}
		case "RECONNECT_EXTERNAL_GEOMETRY":
			requested[operation.ExternalID] = struct{}{}
		}
	}
	if len(requested) == 0 {
		return nil
	}
	for _, feature := range model.Features {
		if feature.ID != payload.SketchID || feature.Sketch == nil {
			continue
		}
		for _, external := range feature.Sketch.ExternalGeometry {
			if _, ok := requested[external.ID]; !ok || (external.Status == "CONNECTED" && external.Snapshot != nil) {
				continue
			}
			return &externalGeometryProjectionFailure{
				diagnosticCode: external.DiagnosticCode,
				diagnostic:     external.Diagnostic,
			}
		}
	}
	// Typed commands carry formal selections instead of viewport pick arguments.
	// Validate their actual source witnesses as well, without imposing atomic
	// downstream geometry success on ordinary, individually committed drawing.
	check := model
	check.Features = append([]Feature(nil), model.Features...)
	for i, f := range check.Features {
		if f.ID != payload.SketchID || f.Sketch == nil {
			continue
		}
		sketch := *f.Sketch
		sketch.Support.PersistentSelection = nil
		sketch.ExternalGeometry = nil
		for _, e := range f.Sketch.ExternalGeometry {
			if _, ok := requested[e.ID]; ok && e.ContextInputID == "" {
				sketch.ExternalGeometry = append(sketch.ExternalGeometry, e)
			}
		}
		check.Features[i].Sketch = &sketch
	}
	if err := service.validateOrderHistoricalStages(ctx, documentID, check, func(f Feature, _ string) error {
		return &externalGeometryProjectionFailure{diagnosticCode: "EXTERNAL_SOURCE_STAGE_INVALID", diagnostic: f.Name + ": projection source must precede the sketch"}
	}, payload.SketchID); err != nil {
		return err
	}
	return nil
}

func dot3(left, right [3]float64) float64 {
	return left[0]*right[0] + left[1]*right[1] + left[2]*right[2]
}

func normalize3(value [3]float64) ([3]float64, bool) {
	length := math.Sqrt(dot3(value, value))
	if length < 1e-12 || math.IsNaN(length) || math.IsInf(length, 0) {
		return [3]float64{}, false
	}
	return [3]float64{value[0] / length, value[1] / length, value[2] / length}, true
}

func stableSupportX(normal, preferred [3]float64) ([3]float64, bool) {
	normal, ok := normalize3(normal)
	if !ok {
		return [3]float64{}, false
	}
	project := func(value [3]float64) ([3]float64, bool) {
		scale := dot3(value, normal)
		return normalize3([3]float64{value[0] - scale*normal[0], value[1] - scale*normal[1], value[2] - scale*normal[2]})
	}
	if value, valid := project(preferred); valid {
		return value, true
	}
	candidates := [][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if math.Abs(dot3(candidate, normal)) < math.Abs(dot3(best, normal)) {
			best = candidate
		}
	}
	return project(best)
}

func validatedSupportFrame(origin, xDirection, normal [3]float64) ([3]float64, [3]float64, [3]float64, error) {
	for _, value := range append(append(origin[:], xDirection[:]...), normal[:]...) {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return origin, xDirection, normal, fmt.Errorf("support frame contains a non-finite value")
		}
	}
	normal, ok := normalize3(normal)
	if !ok {
		return origin, xDirection, normal, fmt.Errorf("support normal is degenerate")
	}
	xDirection, ok = stableSupportX(normal, xDirection)
	if !ok || math.Abs(dot3(xDirection, normal)) > 1e-9 {
		return origin, xDirection, normal, fmt.Errorf("support X direction is degenerate")
	}
	return origin, xDirection, normal, nil
}

func datumSupportFrame(model PartModel, support SketchSupport) ([3]float64, [3]float64, [3]float64, bool) {
	for _, datum := range model.DatumPlanes {
		if datum.ID == support.DatumPlaneID {
			return datum.Origin, datum.UDirection, datum.Normal, true
		}
	}
	return [3]float64{}, [3]float64{}, [3]float64{}, false
}

func supportFrame(model PartModel, support SketchSupport) ([3]float64, [3]float64, [3]float64, bool) {
	if support.Status == "FAILED_SUPPORT" || support.Status == "BLOCKED" {
		return [3]float64{}, [3]float64{}, [3]float64{}, false
	}
	if support.Type == "PLANAR_FACE" || support.Type == "PATTERN_FRAME" {
		origin, xDirection, normal, err := validatedSupportFrame(support.Origin, support.XDirection, support.Normal)
		return origin, xDirection, normal, err == nil && support.Status != "FAILED_SUPPORT" && support.Status != "BLOCKED"
	}
	return datumSupportFrame(model, support)
}

func (service *Service) topologyManifestForGeometryKey(ctx context.Context, geometryKey string) (*topologyManifest, string, error) {
	objectID, digest, geometryID, brepDigest, err := service.namingReference(ctx, geometryKey)
	if err != nil {
		return nil, "", err
	}
	manifest, actualDigest, err := service.readTopologyManifest(ctx, nil, objectID, digest)
	if err != nil {
		return nil, "", err
	}
	if manifest.GeometryID != geometryID || manifest.BRepSHA256 != brepDigest {
		return nil, "", namingError("CORRUPT", "TOPOLOGY_MANIFEST_GEOMETRY_MISMATCH", "Naming artifact belongs to a different frozen geometry")
	}
	return manifest, actualDigest, nil
}

func (service *Service) resolveSelectionAgainstGeometry(ctx context.Context, documentID, sourceVersionID, geometryKey string,
	selection modelcore.PersistentSelection) (modelcore.SelectionResolution, string, error) {
	source, _, _, err := service.topologyManifestForVersion(ctx, documentID, sourceVersionID, selection.SourceBodyID)
	if err != nil {
		return modelcore.SelectionResolution{}, "", err
	}
	sourceOutput := manifestSemanticOutput(source, selection)
	if !topologyHistoryComplete(source) || sourceOutput == nil {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "source support has no complete semantic topology history"), "", nil
	}
	if selection.CreationEvidence.EvidenceDigest != "" && selection.CreationEvidence.EvidenceDigest != sourceOutput.GetEvidence().GetEvidenceDigest() {
		return modelcore.SelectionResolution{Status: modelcore.SelectionContractMismatch,
			SupportingElementStatus: modelcore.SupportingElementNotConnected, DiagnosticCode: "CREATION_EVIDENCE_MISMATCH"}, "", nil
	}
	target, digest, err := service.topologyManifestForGeometryKey(ctx, geometryKey)
	if err != nil {
		return modelcore.SelectionResolution{}, "", err
	}
	if !topologyHistoryComplete(target) {
		return unavailableSelectionResolution("TOPOLOGY_HISTORY_INCOMPLETE", "target support has no complete semantic topology history"), digest, nil
	}
	return resolveManifest(selection, geometryKey, target), digest, nil
}

func (service *Service) resolveAndSolveSketches(ctx context.Context, documentID, requestID string, model *PartModel) error {
	// Validate real dependency cycles before evaluating any geometry prefix.
	graph, _, err := buildPartEvaluation(*model, "", "", nil, nil)
	if err != nil {
		return err
	}
	if err := validateDatumFeatureOrder(*model, graph); err != nil {
		return err
	}
	featureIndex := map[string]int{}
	for index, feature := range model.Features {
		featureIndex[feature.ID] = index
	}
	blocked := map[string]bool{}
	blockDownstream := func(id string) {
		for _, key := range graph.DirtyClosure([]modelcore.DependencyKey{modelcore.DependencyKey("feature:" + id)}) {
			target := strings.TrimPrefix(string(key), "feature:")
			if target != id {
				blocked[target] = true
			}
		}
	}
	failSupport := func(feature *Feature, code, diagnostic string) {
		support := &feature.Sketch.Support
		support.Status, support.DiagnosticCode, support.Diagnostic = "FAILED_SUPPORT", code, diagnostic
		for i := range feature.Sketch.ExternalGeometry {
			e := &feature.Sketch.ExternalGeometry[i]
			e.Status, e.DiagnosticCode, e.Diagnostic = "UNRESOLVED_EXTERNAL", "UPSTREAM_SUPPORT_FAILED", diagnostic
			e.Snapshot = nil
			e.DependencySnapshot = nil
			e.ResolvedSourceDigest = ""
		}
		blockDownstream(feature.ID)
	}
	// The existing graph orders real datum/projection/sketch preparation.
	// Solid execution stays a coarse Body RPC, invoked only by stage readers.
	for _, node := range graph.TopologicalOrder() {
		if strings.HasPrefix(string(node), "datum:") {
			if err := service.resolveDatumDefinitions(ctx, documentID, requestID, model, len(model.Features), strings.TrimPrefix(string(node), "datum:")); err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(string(node), "feature:") {
			continue
		}
		index, ok := featureIndex[strings.TrimPrefix(string(node), "feature:")]
		if !ok {
			continue
		}
		feature := &model.Features[index]
		if feature.Sketch == nil {
			continue
		}
		support := &feature.Sketch.Support
		if blocked[feature.ID] {
			support.Status = "BLOCKED"
			support.DiagnosticCode, support.Diagnostic = "UPSTREAM_INPUT_FAILED", "an upstream sketch input failed"
			for i := range feature.Sketch.ExternalGeometry {
				feature.Sketch.ExternalGeometry[i].Status = "UNRESOLVED_EXTERNAL"
				feature.Sketch.ExternalGeometry[i].DiagnosticCode = "UPSTREAM_BLOCKED"
				feature.Sketch.ExternalGeometry[i].Snapshot = nil
			}
			continue
		}
		if feature.Suppressed {
			continue
		}
		if support.Type == "" || support.Type == "DATUM_PLANE" {
			origin, xDirection, normal, ok := datumSupportFrame(*model, *support)
			if !ok {
				failSupport(feature, "SUPPORT_MISSING", "datum plane is missing")
				continue
			}
			support.Type, support.Origin, support.XDirection, support.Normal = "DATUM_PLANE", origin, xDirection, normal
			support.OrientationRule, support.Status = sketchSupportOrientationRule, "CONNECTED"
			support.DiagnosticCode, support.Diagnostic = "", ""
			if err := service.resolveExternalGeometry(ctx, documentID, requestID, model, index, ""); err != nil {
				return err
			}
			if err := service.solveSketchFeature(ctx, requestID, model, index); err != nil {
				return err
			}
			if feature.Sketch.Solve.Status == "UNRESOLVED_EXTERNAL" {
				blockDownstream(feature.ID)
			}
			continue
		}
		if support.Type != "PLANAR_FACE" || support.PersistentSelection == nil || support.SourceVersionID == "" {
			return &sketchSupportFailure{diagnosticCode: "SUPPORT_CONTRACT_INCOMPLETE", diagnostic: "planar face support contract is incomplete"}
		}
		if support.PersistentSelection.SourceDocumentID != documentID {
			return fmt.Errorf("%w: cross-document sketch support is unsupported", ErrValidation)
		}
		anchorIndex, exists := featureIndex[support.PersistentSelection.Anchor.FeatureID]
		if !exists || anchorIndex >= index {
			support.Status, support.DiagnosticCode = "FAILED_SUPPORT", "SUPPORT_SOURCE_ORDER_INVALID"
			support.Diagnostic = "support face must belong to an earlier feature"
			return &sketchSupportFailure{diagnosticCode: support.DiagnosticCode, diagnostic: support.Diagnostic}
		}
		// Resolve against the complete body state immediately before this sketch.
		// The semantic anchor may originate from an older feature whose face has
		// since been modified by intervening Boolean operations.
		prefix := *model
		prefix.Features = append([]Feature(nil), model.Features[:index]...)
		stage := ""
		for _, f := range prefix.Features {
			if f.BodyID == support.PersistentSelection.SourceBodyID && isBodyFeature(f.Type) && !f.Suppressed {
				stage = f.ID
			}
		}
		if stage == "" {
			failSupport(feature, "SUPPORT_SOURCE_BODY_EMPTY", "source Body has no solid before sketch")
			continue
		}
		geometryKey, err := service.evaluateBodyPrefix(ctx, requestID+"/support/"+feature.ID, prefix, support.PersistentSelection.SourceBodyID)
		if err != nil {
			matches := featureFailurePattern.FindAllStringSubmatch(err.Error(), -1)
			modelFailure := status.Code(err) == codes.InvalidArgument || errors.Is(err, ErrValidation) && (strings.Contains(err.Error(), "PROFILE_REGION_MISSING") || strings.Contains(err.Error(), "PROFILE_REGION_AMBIGUOUS") || strings.Contains(err.Error(), "PERSISTENT_SELECTION_OUTSIDE_CURRENT_TIP"))
			if len(matches) > 0 && modelFailure {
				support.Status, support.DiagnosticCode, support.Diagnostic = "BLOCKED", "SUPPORT_UPSTREAM_FAILED", err.Error()
				blockDownstream(matches[len(matches)-1][1])
				for i := range feature.Sketch.ExternalGeometry {
					e := &feature.Sketch.ExternalGeometry[i]
					e.Status, e.DiagnosticCode = "UNRESOLVED_EXTERNAL", "UPSTREAM_BLOCKED"
					e.Snapshot = nil
					e.DependencySnapshot = nil
					e.ResolvedSourceDigest = ""
				}
				continue
			}
			return err
		}
		resolution, err := service.resolveFeaturePick(ctx, FeatureSelection{Selection: *support.PersistentSelection, SourceVersionID: support.SourceVersionID, SourceFeatureID: support.SourceFeatureID}, geometryKey)
		_, digest, manifestErr := service.topologyManifestForGeometryKey(ctx, geometryKey)
		if manifestErr != nil {
			return manifestErr
		}
		if err != nil {
			return err
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			support.Status, support.DiagnosticCode, support.Diagnostic = "FAILED_SUPPORT", resolution.DiagnosticCode, resolution.Diagnostic
			if support.DiagnosticCode == "" {
				support.DiagnosticCode = "FAILED_SUPPORT"
			}
			failSupport(feature, support.DiagnosticCode, support.Diagnostic)
			continue
		}
		evidence := resolution.Candidates[0].Evidence
		if evidence.GeometryType != "PLANE" {
			support.Status, support.DiagnosticCode, support.Diagnostic = "FAILED_SUPPORT", "SUPPORT_TYPE_MISMATCH", "resolved support is not planar"
			failSupport(feature, support.DiagnosticCode, support.Diagnostic)
			continue
		}
		normal := evidence.Direction
		origin, xDirection, normal, frameErr := validatedSupportFrame(evidence.Origin, support.XDirection, normal)
		if frameErr != nil {
			failSupport(feature, "SUPPORT_FRAME_INVALID", frameErr.Error())
			continue
		}
		support.Origin, support.XDirection, support.Normal = origin, xDirection, normal
		support.OrientationRule, support.Status = sketchSupportOrientationRule, "CONNECTED"
		support.DiagnosticCode, support.Diagnostic = "", ""
		support.DependencySnapshot = &SketchSupportDependencySnapshot{GeometryKey: geometryKey, ManifestDigest: digest,
			PolicyDigest: modelcore.TopologyNamingPolicyDigest, EvidenceDigest: resolution.EvidenceDigest}
		if err := service.resolveExternalGeometry(ctx, documentID, requestID, model, index, geometryKey); err != nil {
			return err
		}
		if err := service.solveSketchFeature(ctx, requestID, model, index); err != nil {
			return err
		}
		if feature.Sketch.Solve.Status == "UNRESOLVED_EXTERNAL" {
			blockDownstream(feature.ID)
		}
	}
	return nil
}

func (service *Service) resolveExternalGeometry(ctx context.Context, documentID, requestID string, model *PartModel, featureIndex int, geometryKey string) error {
	sketch := model.Features[featureIndex].Sketch
	if sketch == nil || len(sketch.ExternalGeometry) == 0 {
		return nil
	}
	if service.worker == nil {
		return fmt.Errorf("%w: geometry worker is required to project external geometry", ErrValidation)
	}
	bodyGeometries := map[string]string{}
	if geometryKey != "" && sketch.Support.PersistentSelection != nil {
		bodyGeometries[sketch.Support.PersistentSelection.SourceBodyID] = geometryKey
	}
	markBroken := func(external *SketchExternalGeometry, code, diagnostic string) {
		external.Status, external.DiagnosticCode, external.Diagnostic = "UNRESOLVED_EXTERNAL", code, diagnostic
		external.Snapshot, external.DependencySnapshot, external.ResolvedSourceDigest = nil, nil, ""
		external.AffectedConstraintIDs = external.AffectedConstraintIDs[:0]
		for _, constraint := range sketch.Constraints {
			for _, reference := range constraint.References {
				if reference.Target == "EXTERNAL" && reference.EntityID == external.ID {
					external.AffectedConstraintIDs = append(external.AffectedConstraintIDs, constraint.ID)
					break
				}
			}
		}
		external.AffectedProfileRegionIDs = external.AffectedProfileRegionIDs[:0]
		if regions, err := buildProfileRegions(model.Features[featureIndex]); err == nil {
			for _, region := range regions {
				external.AffectedProfileRegionIDs = append(external.AffectedProfileRegionIDs, region.ID)
			}
		}
		external.DownstreamFeatureIDs = external.DownstreamFeatureIDs[:0]
		if graph, _, err := buildPartEvaluation(*model, "", "", nil, nil); err == nil {
			affected := map[modelcore.DependencyKey]bool{}
			for _, key := range graph.DirtyClosure([]modelcore.DependencyKey{modelcore.DependencyKey("feature:" + model.Features[featureIndex].ID)}) {
				affected[key] = true
			}
			for _, f := range model.Features[featureIndex+1:] {
				if affected[modelcore.DependencyKey("feature:"+f.ID)] {
					external.DownstreamFeatureIDs = append(external.DownstreamFeatureIDs, f.ID)
				}
			}
		}
	}
	for index := range sketch.ExternalGeometry {
		external := &sketch.ExternalGeometry[index]
		if external.ContextInputID != "" {
			var input *ContextInput
			for i := range model.ContextInputs {
				if model.ContextInputs[i].ID == external.ContextInputID && model.ContextInputs[i].Target.TargetID == model.Features[featureIndex].ID {
					input = &model.ContextInputs[i]
					break
				}
			}
			if input == nil || input.Default == nil || input.Default.External == nil {
				return fmt.Errorf("%w: CONTEXT_INPUT_DEFAULT_MISSING", ErrValidation)
			}
			snapshot := input.Default.External
			digest := resolvedDigest(snapshot)
			if external.LocalInput != nil {
				var err error
				snapshot, err = service.projectLocalInput(ctx, requestID, *model, model.Features[featureIndex].ID, input.ID, external.LocalInput)
				if err != nil {
					return err
				}
				digest = resolvedDigest(external.LocalInput)
			}
			for _, ref := range model.ContextReferences {
				if external.LocalInput == nil && ref.ID == input.ID {
					if ref.Resolution.Status != "CONNECTED" {
						return fmt.Errorf("%w: CONTEXT_INPUT_BINDING_BROKEN", ErrValidation)
					}
					var err error
					snapshot, err = service.projectInputPublication(ctx, requestID, *model, model.Features[featureIndex].ID, input.ID, ref.Resolution, ref.Transform)
					if err != nil {
						return err
					}
					digest = ref.Resolution.SourceDigest
				}
			}
			external.Snapshot = snapshot
			external.GeometryKind = snapshot.Kind
			external.Status = "CONNECTED"
			external.ResolvedSourceDigest = digest
			external.DiagnosticCode, external.Diagnostic = "", ""
			continue
		}
		if external.DatumReference != nil {
			source, err := datumProjectionSource(*model, *external.DatumReference)
			if err != nil {
				markBroken(external, "EXTERNAL_DATUM_MISSING", err.Error())
				continue
			}
			projected, err := service.projectExternalGeometry(ctx, requestID, model.Features[featureIndex].ID, external.ID, source, geometry.ExternalProjectionFrame{Origin: sketch.Support.Origin, XDirection: sketch.Support.XDirection, Normal: sketch.Support.Normal})
			if err != nil {
				return err
			}
			if projected.Status != "CONNECTED" {
				markBroken(external, projected.DiagnosticCode, projected.Diagnostic)
				continue
			}
			snapshot := &SketchExternalGeometrySnapshot{Kind: projected.Kind}
			if projected.Kind == "POINT" {
				snapshot.Point = &SketchPoint2{X: projected.Point[0], Y: projected.Point[1]}
			} else if projected.Kind == "LINE" {
				snapshot.Start = &SketchPoint2{X: projected.Start[0], Y: projected.Start[1]}
				snapshot.End = &SketchPoint2{X: projected.End[0], Y: projected.End[1]}
			} else {
				return fmt.Errorf("%w: datum projection kind invalid", ErrValidation)
			}
			external.Status, external.GeometryKind, external.Snapshot = "CONNECTED", projected.Kind, snapshot
			external.DiagnosticCode, external.Diagnostic = "", ""
			external.ResolvedSourceDigest = source.EvidenceDigest
			external.DependencySnapshot = nil
			external.AffectedConstraintIDs, external.AffectedProfileRegionIDs, external.DownstreamFeatureIDs = nil, nil, nil
			continue
		}
		sourceDocumentID := external.SourceDocumentID
		if sourceDocumentID == "" {
			sourceDocumentID = documentID
		}
		sourceGeometryKey := geometryKey
		sourcePose := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
		if external.ContextReferenceID != "" {
			for _, reference := range model.ContextReferences {
				if reference.ID == external.ContextReferenceID {
					sourceGeometryKey, sourcePose = reference.Resolution.GeometryKey, reference.Transform
					break
				}
			}
		} else {
			anchorIndex := -1
			for candidateIndex, feature := range model.Features {
				if feature.ID == external.PersistentSelection.Anchor.FeatureID {
					anchorIndex = candidateIndex
					break
				}
			}
			if anchorIndex < 0 || anchorIndex >= featureIndex {
				markBroken(external, "EXTERNAL_SOURCE_ORDER_INVALID", "external geometry must originate from an earlier feature")
				continue
			}
			bodyID := external.PersistentSelection.SourceBodyID
			sourceGeometryKey = bodyGeometries[bodyID]
			if sourceGeometryKey == "" {
				prefix := *model
				prefix.Features = append([]Feature(nil), model.Features[:featureIndex]...)
				var err error
				sourceGeometryKey, err = service.evaluateBodyPrefix(ctx, requestID+"/external-source/"+external.ID, prefix, bodyID)
				if err != nil {
					return err
				}
				bodyGeometries[bodyID] = sourceGeometryKey
			}

		}
		if sourceGeometryKey == "" {
			markBroken(external, "EXTERNAL_SOURCE_UNAVAILABLE", "context Publication has no resolved geometry")
			continue
		}
		var resolution modelcore.SelectionResolution
		var manifestDigest string
		var err error
		if external.SourceFeatureID != "" && external.ContextReferenceID == "" && sourceDocumentID == documentID {
			resolution, err = service.resolveFeaturePick(ctx, FeatureSelection{Selection: external.PersistentSelection, SourceVersionID: external.SourceVersionID, SourceFeatureID: external.SourceFeatureID}, sourceGeometryKey)
			if err == nil {
				_, manifestDigest, err = service.topologyManifestForGeometryKey(ctx, sourceGeometryKey)
			}
		} else {
			resolution, manifestDigest, err = service.resolveSelectionAgainstGeometry(ctx, sourceDocumentID, external.SourceVersionID, sourceGeometryKey, external.PersistentSelection)
		}
		if err != nil {
			return err
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			code := map[modelcore.SelectionResolutionStatus]string{
				modelcore.SelectionMissing: "EXTERNAL_SOURCE_MISSING", modelcore.SelectionAmbiguous: "EXTERNAL_SOURCE_AMBIGUOUS",
				modelcore.SelectionTypeMismatch: "EXTERNAL_SOURCE_TYPE_MISMATCH", modelcore.SelectionSourceUnavailable: "EXTERNAL_SOURCE_UNAVAILABLE",
				modelcore.SelectionContractMismatch: "EXTERNAL_SOURCE_CONTRACT_MISMATCH", modelcore.SelectionOutsideCurrentTip: "EXTERNAL_SOURCE_OUTSIDE_CURRENT_TIP",
			}[resolution.Status]
			if code == "" {
				code = "EXTERNAL_SOURCE_UNRESOLVED"
			}
			markBroken(external, code, resolution.Diagnostic)
			external.DependencySnapshot = &SketchSupportDependencySnapshot{GeometryKey: sourceGeometryKey, ManifestDigest: manifestDigest,
				PolicyDigest: modelcore.TopologyNamingPolicyDigest, EvidenceDigest: resolution.EvidenceDigest}
			continue
		}
		candidate := resolution.Candidates[0]
		origin, direction := pointByPose(sourcePose, candidate.Evidence.Origin), rotateByPose(sourcePose, candidate.Evidence.Direction)
		projected, err := service.projectExternalGeometry(ctx, requestID, model.Features[featureIndex].ID, external.ID,
			geometry.ExternalProjectionSource{GeometryID: candidate.GeometryID, GeometryKey: candidate.GeometryKey,
				TopologyType: string(candidate.Type), LocalID: candidate.LocalID, GeometryType: candidate.Evidence.GeometryType,
				EvidenceDigest: candidate.Evidence.EvidenceDigest, MeasureSI: candidate.Evidence.MeasureSI,
				ParameterStart: candidate.Evidence.ParameterStart, ParameterEnd: candidate.Evidence.ParameterEnd,
				Origin: origin, Direction: direction, XDirection: rotateByPose(sourcePose, candidate.Evidence.XDirection), RadiusMM: candidate.Evidence.RadiusMM, MinorRadiusMM: candidate.Evidence.MinorRadiusMM},
			geometry.ExternalProjectionFrame{Origin: sketch.Support.Origin, XDirection: sketch.Support.XDirection, Normal: sketch.Support.Normal})
		if err != nil {
			return err
		}
		if projected.Status != "CONNECTED" {
			markBroken(external, projected.DiagnosticCode, projected.Diagnostic)
			external.DependencySnapshot = &SketchSupportDependencySnapshot{GeometryKey: sourceGeometryKey, ManifestDigest: manifestDigest,
				PolicyDigest: modelcore.TopologyNamingPolicyDigest, EvidenceDigest: resolution.EvidenceDigest}
			continue
		}
		snapshot := &SketchExternalGeometrySnapshot{Kind: projected.Kind, Radius: projected.Radius, MajorRadius: projected.MajorRadius, MinorRadius: projected.MinorRadius, Rotation: projected.Rotation, StartAngle: projected.StartAngle, EndAngle: projected.EndAngle}
		switch projected.Kind {
		case "POINT":
			snapshot.Point = &SketchPoint2{X: projected.Point[0], Y: projected.Point[1]}
		case "LINE":
			snapshot.Start, snapshot.End = &SketchPoint2{X: projected.Start[0], Y: projected.Start[1]}, &SketchPoint2{X: projected.End[0], Y: projected.End[1]}
		case "CIRCLE", "ARC", "ELLIPSE", "ELLIPTICAL_ARC":
			snapshot.Center = &SketchPoint2{X: projected.Center[0], Y: projected.Center[1]}
		default:
			markBroken(external, "EXTERNAL_PROJECTION_TYPE_MISMATCH", "worker returned an unsupported projected geometry kind")
			continue
		}
		candidateSketch := *sketch
		candidateSketch.ExternalGeometry = append([]SketchExternalGeometry(nil), sketch.ExternalGeometry...)
		candidateSketch.ExternalGeometry[index].Status = "CONNECTED"
		candidateSketch.ExternalGeometry[index].GeometryKind = projected.Kind
		candidateSketch.ExternalGeometry[index].Snapshot = snapshot
		if validationErr := validateSketch(candidateSketch); validationErr != nil {
			markBroken(external, "EXTERNAL_SOURCE_TYPE_MISMATCH", validationErr.Error())
			external.DependencySnapshot = &SketchSupportDependencySnapshot{GeometryKey: sourceGeometryKey, ManifestDigest: manifestDigest,
				PolicyDigest: modelcore.TopologyNamingPolicyDigest, EvidenceDigest: resolution.EvidenceDigest}
			continue
		}
		external.Status, external.GeometryKind, external.DiagnosticCode, external.Diagnostic = "CONNECTED", projected.Kind, "", ""
		external.Snapshot, external.ResolvedSourceDigest = snapshot, projected.SourceDigest
		external.DependencySnapshot = &SketchSupportDependencySnapshot{GeometryKey: sourceGeometryKey, ManifestDigest: manifestDigest,
			PolicyDigest: modelcore.TopologyNamingPolicyDigest, EvidenceDigest: resolution.EvidenceDigest}
		external.AffectedConstraintIDs, external.AffectedProfileRegionIDs, external.DownstreamFeatureIDs = nil, nil, nil
	}
	return nil
}

func appendEvaluatedSketchChanges(changes modelcore.ChangeSet, before, after PartModel) modelcore.ChangeSet {
	existing := map[string]struct{}{}
	for _, change := range changes.Changes {
		existing[change.Target.Key()] = struct{}{}
	}
	beforeSketches := map[string]*SketchFeature{}
	for index := range before.Features {
		if before.Features[index].Sketch != nil {
			beforeSketches[before.Features[index].ID] = before.Features[index].Sketch
		}
	}
	for index := range after.Features {
		feature := &after.Features[index]
		if feature.Sketch == nil || beforeSketches[feature.ID] == nil || reflect.DeepEqual(*beforeSketches[feature.ID], *feature.Sketch) {
			continue
		}
		address := modelcore.PropertyAddress{EntityID: feature.ID, SlotID: "sketch.model"}
		if _, exists := existing[address.Key()]; exists {
			continue
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, address, *beforeSketches[feature.ID], *feature.Sketch)
		changes.Changes = append(changes.Changes, change)
		changes.ImpactSeeds = append(changes.ImpactSeeds, "feature:"+modelcore.DependencyKey(feature.ID))
		existing[address.Key()] = struct{}{}
	}
	return changes
}

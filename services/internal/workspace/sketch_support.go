package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
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

func rejectExplicitUnresolvedExternal(command modelcore.DomainCommand, model PartModel) error {
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
	if support.Type == "PLANAR_FACE" {
		origin, xDirection, normal, err := validatedSupportFrame(support.Origin, support.XDirection, support.Normal)
		return origin, xDirection, normal, err == nil && support.Status != "FAILED_SUPPORT"
	}
	return datumSupportFrame(model, support)
}

func (service *Service) topologyManifestForGeometryKey(ctx context.Context, geometryKey string) (*workerv1.PartTopologyManifest, string, error) {
	var digest string
	var inline []byte
	var objectID *string
	err := service.database.QueryRow(ctx, `SELECT COALESCE(topology_manifest_data,''::bytea),topology_manifest_object_id::text,topology_manifest_digest FROM occccad.geometry_artifacts WHERE geometry_key=$1`, geometryKey).Scan(&inline, &objectID, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	data := inline
	if len(data) == 0 && objectID != nil && service.artifacts != nil {
		_, reader, openErr := service.artifacts.Open(ctx, *objectID)
		if openErr != nil {
			return nil, "", openErr
		}
		data, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, "", err
		}
	}
	if len(data) == 0 {
		return nil, "", ErrNotFound
	}
	hash := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), strings.TrimSpace(digest)) {
		return nil, "", fmt.Errorf("topology manifest digest mismatch")
	}
	manifest := &workerv1.PartTopologyManifest{}
	if err := proto.Unmarshal(data, manifest); err != nil {
		return nil, "", err
	}
	return manifest, strings.TrimSpace(digest), nil
}

func (service *Service) resolveSelectionAgainstGeometry(ctx context.Context, documentID, sourceVersionID, geometryKey string,
	selection modelcore.PersistentSelection) (modelcore.SelectionResolution, string, error) {
	source, _, _, err := service.topologyManifestForVersion(ctx, documentID, sourceVersionID)
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
	featureIndex := map[string]int{}
	for index, feature := range model.Features {
		featureIndex[feature.ID] = index
	}
	for index := range model.Features {
		feature := &model.Features[index]
		if feature.Sketch == nil {
			continue
		}
		support := &feature.Sketch.Support
		if support.Type == "" || support.Type == "DATUM_PLANE" {
			origin, xDirection, normal, ok := datumSupportFrame(*model, *support)
			if !ok {
				return &sketchSupportFailure{diagnosticCode: "SUPPORT_MISSING", diagnostic: "datum plane is missing"}
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
				return nil
			}
			continue
		}
		if support.Type != "PLANAR_FACE" || support.PersistentSelection == nil || support.SourceVersionID == "" {
			return &sketchSupportFailure{diagnosticCode: "SUPPORT_CONTRACT_INCOMPLETE", diagnostic: "planar face support contract is incomplete"}
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
		geometryKey, err := service.evaluatePart(ctx, requestID+"/support/"+feature.ID, prefix)
		if err != nil {
			return err
		}
		resolution, digest, err := service.resolveSelectionAgainstGeometry(ctx, documentID, support.SourceVersionID, geometryKey, *support.PersistentSelection)
		if err != nil {
			return err
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			support.Status, support.DiagnosticCode, support.Diagnostic = "FAILED_SUPPORT", resolution.DiagnosticCode, resolution.Diagnostic
			if support.DiagnosticCode == "" {
				support.DiagnosticCode = "FAILED_SUPPORT"
			}
			return &sketchSupportFailure{diagnosticCode: support.DiagnosticCode, diagnostic: support.Diagnostic}
		}
		evidence := resolution.Candidates[0].Evidence
		if evidence.GeometryType != "PLANE" {
			support.Status, support.DiagnosticCode, support.Diagnostic = "FAILED_SUPPORT", "SUPPORT_TYPE_MISMATCH", "resolved support is not planar"
			return &sketchSupportFailure{diagnosticCode: support.DiagnosticCode, diagnostic: support.Diagnostic}
		}
		normal := evidence.Direction
		origin, xDirection, normal, frameErr := validatedSupportFrame(evidence.Origin, support.XDirection, normal)
		if frameErr != nil {
			return &sketchSupportFailure{diagnosticCode: "SUPPORT_FRAME_INVALID", diagnostic: frameErr.Error()}
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
			return nil
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
	if geometryKey == "" {
		prefix := *model
		prefix.Features = append([]Feature(nil), model.Features[:featureIndex]...)
		var err error
		geometryKey, err = service.evaluatePart(ctx, requestID+"/external-source/"+model.Features[featureIndex].ID, prefix)
		if err != nil {
			return err
		}
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
		for _, downstream := range model.Features[featureIndex+1:] {
			external.DownstreamFeatureIDs = append(external.DownstreamFeatureIDs, downstream.ID)
		}
	}
	for index := range sketch.ExternalGeometry {
		external := &sketch.ExternalGeometry[index]
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
		}
		if sourceGeometryKey == "" {
			markBroken(external, "EXTERNAL_SOURCE_UNAVAILABLE", "context Publication has no resolved geometry")
			continue
		}
		resolution, manifestDigest, err := service.resolveSelectionAgainstGeometry(ctx, sourceDocumentID, external.SourceVersionID, sourceGeometryKey, external.PersistentSelection)
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
		projected, err := service.worker.ProjectExternalGeometry(ctx, requestID+"/external/"+external.ID,
			geometry.ExternalProjectionSource{GeometryID: candidate.GeometryID, GeometryKey: candidate.GeometryKey,
				TopologyType: string(candidate.Type), LocalID: candidate.LocalID, GeometryType: candidate.Evidence.GeometryType,
				EvidenceDigest: candidate.Evidence.EvidenceDigest, MeasureSI: candidate.Evidence.MeasureSI,
				ParameterStart: candidate.Evidence.ParameterStart, ParameterEnd: candidate.Evidence.ParameterEnd,
				Origin: origin, Direction: direction},
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
		snapshot := &SketchExternalGeometrySnapshot{Kind: projected.Kind, Radius: projected.Radius}
		switch projected.Kind {
		case "POINT":
			snapshot.Point = &SketchPoint2{X: projected.Point[0], Y: projected.Point[1]}
		case "LINE":
			snapshot.Start, snapshot.End = &SketchPoint2{X: projected.Start[0], Y: projected.Start[1]}, &SketchPoint2{X: projected.End[0], Y: projected.End[1]}
		case "CIRCLE":
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

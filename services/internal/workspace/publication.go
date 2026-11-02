package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	typeCreatePublication    = "occccad://part/publication/create"
	typeEditPublication      = "occccad://part/publication/edit"
	typeRedirectPublication  = "occccad://part/publication/redirect"
	typeDeletePublication    = "occccad://part/publication/delete"
	typeSetParameterExternal = "occccad://parameter/external/set"
)

type publicationPayload struct {
	Publication Publication `json:"publication"`
}

type publicationEditPayload struct {
	PublicationID        string       `json:"publicationId"`
	Name                 string       `json:"name"`
	SemanticPurpose      string       `json:"semanticPurpose"`
	CompatibilityVersion string       `json:"compatibilityVersion"`
	Target               *Publication `json:"target,omitempty"`
}

type publicationDeletePayload struct {
	PublicationID string `json:"publicationId"`
}

func applyCreatePublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload publicationPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for _, item := range model.Publications {
		if item.ID == payload.Publication.ID {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: duplicate PublicationId", ErrValidation)
		}
	}
	model.Publications = append(model.Publications, payload.Publication)
	if err := validatePublicationDefinitions(model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, _ := modelcore.NewChange(modelcore.ChangeCreate, modelcore.PropertyAddress{EntityID: payload.Publication.ID, SlotID: "publication.entity"}, nil, payload.Publication)
	next, _ := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"publication:" + modelcore.DependencyKey(payload.Publication.ID)}}, nil
}

func applyEditPublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload publicationEditPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index := range model.Publications {
		if model.Publications[index].ID != payload.PublicationID {
			continue
		}
		before := model.Publications[index]
		if payload.Target != nil {
			if payload.Target.ID != before.ID || !publicationContractsCompatible(before, *payload.Target) {
				return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
			}
			model.Publications[index] = *payload.Target
			model.Publications[index].CompatibilityVersion = before.CompatibilityVersion
		}
		model.Publications[index].Name = strings.TrimSpace(payload.Name)
		model.Publications[index].SemanticPurpose = strings.TrimSpace(payload.SemanticPurpose)
		if version := strings.TrimSpace(payload.CompatibilityVersion); version != "" {
			model.Publications[index].CompatibilityVersion = version
		}
		if err := validatePublicationDefinitions(model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: payload.PublicationID, SlotID: "publication.entity"}, before, model.Publications[index])
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"publication:" + modelcore.DependencyKey(payload.PublicationID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: publication does not exist", ErrValidation)
}

func applyRedirectPublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload publicationPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index := range model.Publications {
		if model.Publications[index].ID != payload.Publication.ID {
			continue
		}
		before := model.Publications[index]
		if !publicationContractsCompatible(before, payload.Publication) {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
		}
		payload.Publication.Name = before.Name
		payload.Publication.SemanticPurpose = before.SemanticPurpose
		payload.Publication.CompatibilityVersion = before.CompatibilityVersion
		model.Publications[index] = payload.Publication
		if err := validatePublicationDefinitions(model); err != nil {
			return nil, modelcore.ChangeSet{}, err
		}
		change, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: payload.Publication.ID, SlotID: "publication.entity"}, before, payload.Publication)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"publication:" + modelcore.DependencyKey(payload.Publication.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: publication does not exist", ErrValidation)
}

func applyDeletePublication(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload publicationDeletePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for index, item := range model.Publications {
		if item.ID != payload.PublicationID {
			continue
		}
		model.Publications = append(model.Publications[:index], model.Publications[index+1:]...)
		change, _ := modelcore.NewChange(modelcore.ChangeDelete, modelcore.PropertyAddress{EntityID: item.ID, SlotID: "publication.entity"}, item, nil)
		next, _ := json.Marshal(model)
		return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"publication:" + modelcore.DependencyKey(item.ID)}}, nil
	}
	return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: publication does not exist", ErrValidation)
}

func publicationContractsCompatible(left, right Publication) bool {
	if left.Type != right.Type || left.Contract.GeometryKind != right.Contract.GeometryKind || left.Contract.ValueType != right.Contract.ValueType || left.Contract.Symmetry != right.Contract.Symmetry {
		return false
	}
	if (left.Contract.Dimension == nil) != (right.Contract.Dimension == nil) {
		return false
	}
	if left.Contract.Dimension != nil && !left.Contract.Dimension.Equal(*right.Contract.Dimension) {
		return false
	}
	return resolvedDigest(left.Contract.Bounds) == resolvedDigest(right.Contract.Bounds)
}

func validatePublicationDefinitions(model PartModel) error {
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, publication := range model.Publications {
		if strings.TrimSpace(publication.ID) == "" || strings.TrimSpace(publication.Name) == "" || strings.TrimSpace(publication.CompatibilityVersion) == "" {
			return fmt.Errorf("%w: publication identity, name and compatibility version are required", ErrValidation)
		}
		if ids[publication.ID] {
			return fmt.Errorf("%w: duplicate PublicationId", ErrValidation)
		}
		ids[publication.ID] = true
		nameKey := scopedNameKey(publication.Name)
		if names[nameKey] {
			return fmt.Errorf("%w: Publication name must be unique in its Part", ErrValidation)
		}
		names[nameKey] = true
		switch publication.Type {
		case "POINT", "AXIS", "PLANE", "FRAME", "CURVE", "SURFACE", "BODY", "PARAMETER":
		default:
			return fmt.Errorf("%w: unsupported publication type %s", ErrValidation, publication.Type)
		}
		switch publication.Target.Kind {
		case "SKETCH_ENTITY":
			symmetry := "NONE"
			if publication.Type == "CURVE" {
				symmetry = "PARAMETER_DIRECTION"
			}
			if publication.Contract.GeometryKind != publication.Type || publication.Contract.Symmetry != symmetry {
				return fmt.Errorf("%w: native sketch publication contract invalid", ErrValidation)
			}
			if publication.Target.SketchID == "" || publication.Target.EntityID == "" || !((publication.Type == "POINT" && publication.Target.EntityKind == "POINT") || (publication.Type == "CURVE" && (publication.Target.EntityKind == "LINE" || publication.Target.EntityKind == "CIRCLE" || publication.Target.EntityKind == "ARC"))) {
				return fmt.Errorf("%w: native sketch publication contract invalid", ErrValidation)
			}
		case "DATUM":
			if publication.Target.DatumID == "" {
				return fmt.Errorf("%w: datum publication target is incomplete", ErrValidation)
			}
			if publication.Type != "POINT" && publication.Type != "AXIS" && publication.Type != "PLANE" && publication.Type != "FRAME" {
				return fmt.Errorf("%w: datum publication type is incompatible", ErrValidation)
			}
			if publication.Contract.GeometryKind != publication.Type {
				return fmt.Errorf("%w: datum publication geometry contract is incompatible", ErrValidation)
			}
		case "TOPOLOGY":
			if publication.Target.PersistentSelection == nil || publication.Target.SourceVersionID == "" {
				return fmt.Errorf("%w: topology publication target is incomplete", ErrValidation)
			}
			if err := publication.Target.PersistentSelection.Validate(); err != nil {
				return fmt.Errorf("%w: publication selection: %v", ErrValidation, err)
			}
			expectedType := modelcore.PersistentTopologyFace
			expectedPublication := "SURFACE"
			if publication.Type == "CURVE" {
				expectedType, expectedPublication = modelcore.PersistentTopologyEdge, "CURVE"
			}
			if publication.Type == "POINT" {
				expectedType, expectedPublication = modelcore.PersistentTopologyVertex, "POINT"
			}
			if publication.Type != expectedPublication || publication.Contract.GeometryKind != expectedPublication ||
				publication.Target.PersistentSelection.ExpectedType != expectedType {
				return fmt.Errorf("%w: topology publication contract is incompatible", ErrValidation)
			}
		case "FEATURE_OUTPUT":
			if publication.Target.FeatureID == "" || publication.Target.OutputSlot != "BODY" {
				return fmt.Errorf("%w: feature output publication target is incomplete", ErrValidation)
			}
			if publication.Type != "BODY" || publication.Contract.GeometryKind != "BODY" {
				return fmt.Errorf("%w: feature output publication contract is incompatible", ErrValidation)
			}
		case "BODY_RESULT":
			if publication.Target.BodyID == "" || publication.Type != "BODY" || publication.Contract.GeometryKind != "BODY" {
				return fmt.Errorf("%w: Body result publication target is incomplete", ErrValidation)
			}
		case "PARAMETER":
			if publication.Target.ParameterID == "" || publication.Type != "PARAMETER" {
				return fmt.Errorf("%w: parameter publication target is incomplete", ErrValidation)
			}
			if publication.Contract.ValueType == "" || publication.Contract.Dimension == nil || publication.Contract.UnitPolicy == "" {
				return fmt.Errorf("%w: parameter publication contract is incomplete", ErrValidation)
			}
			if bounds := publication.Contract.Bounds; bounds != nil {
				for _, bound := range []*modelcore.Quantity{bounds.Minimum, bounds.Maximum} {
					if bound != nil && !bound.Dimension.Equal(*publication.Contract.Dimension) {
						return fmt.Errorf("%w: parameter publication bound dimension mismatch", ErrValidation)
					}
				}
				if bounds.Minimum != nil && bounds.Maximum != nil && bounds.Minimum.SIValue > bounds.Maximum.SIValue {
					return fmt.Errorf("%w: parameter publication bounds are inverted", ErrValidation)
				}
			}
		default:
			return fmt.Errorf("%w: unsupported publication target %s", ErrValidation, publication.Target.Kind)
		}
	}
	return nil
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func resolvedDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (service *Service) resolvePartPublications(ctx context.Context, documentID, requestID, revisionID string, model *PartModel, frozenBodies ...bool) error {
	bodyKey := func(bodyID string) (string, error) {
		if len(frozenBodies) > 0 && frozenBodies[0] {
			for _, b := range model.Bodies {
				if b.ID == bodyID {
					return b.GeometryKey, nil
				}
			}
			return "", nil
		}
		return service.evaluateBodyPrefix(ctx, requestID+"/publication-body", *model, bodyID, true)
	}
	for index := range model.Publications {
		recordEvaluation(ctx, "publication:"+model.Publications[index].ID, false, "", 0)
		publication := &model.Publications[index]
		element := partElementPresentation(*model, publication.Target)
		broken := func(code, diagnostic string) {
			publication.Resolution = PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: code, Diagnostic: diagnostic, Element: &element}
		}
		switch publication.Target.Kind {
		case "SKETCH_ENTITY":
			value, err := nativeSketchPublication(*model, publication.Target, revisionID)
			if err != nil {
				broken("PUBLICATION_SKETCH_ENTITY_UNAVAILABLE", err.Error())
				continue
			}
			publication.Resolution = value
		case "BODY_RESULT":
			if bodyIndex(*model, publication.Target.BodyID) < 0 {
				broken("PUBLICATION_BODY_MISSING", "published Body no longer exists")
				continue
			}
			key, err := bodyKey(publication.Target.BodyID)
			if err != nil {
				return err
			}
			if key == "" {
				broken("PUBLICATION_BODY_EMPTY", "published Body has no geometry result")
				continue
			}
			artifact, err := service.loadArtifact(ctx, key)
			if err != nil {
				return err
			}
			if artifact.Volume <= 1.0e-9 {
				broken("PUBLICATION_BODY_EMPTY", "published Body has no solid result")
				continue
			}
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID,
				GeometryKey: key, GeometryID: artifact.GeometryID,
				SourceDigest:     resolvedDigest(struct{ Body, Key string }{publication.Target.BodyID, key}),
				EvaluatorVersion: artifact.EvaluatorVersion, WorkerID: artifact.WorkerID, OCCTVersion: artifact.OCCTVersion}
		case "DATUM":
			if !resolveDatumPublication(model, publication, revisionID) {
				broken("PUBLICATION_TARGET_MISSING", "datum target no longer exists")
			}
		case "PARAMETER":
			publication.Resolution = parameterPublicationResolution(model.Parameters, publication.Target.ParameterID, publication.Contract, revisionID)
		case "FEATURE_OUTPUT":
			prefix, ok := publicationFeaturePrefix(*model, publication.Target.FeatureID)
			if !ok {
				broken("PUBLICATION_FEATURE_OUTPUT_MISSING", "published feature output no longer exists")
				continue
			}
			key, err := service.evaluateBodyPrefix(ctx, requestID+"/publication/"+publication.ID, prefix, prefix.Features[len(prefix.Features)-1].BodyID, true)
			if err != nil {
				return err
			}
			artifact, err := service.loadArtifact(ctx, key)
			if err != nil {
				return err
			}
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID,
				GeometryKey: key, GeometryID: artifact.GeometryID,
				SourceDigest:     resolvedDigest(struct{ Feature, Key string }{publication.Target.FeatureID, key}),
				EvaluatorVersion: artifact.EvaluatorVersion, WorkerID: artifact.WorkerID, OCCTVersion: artifact.OCCTVersion}
		case "TOPOLOGY":
			// The resolved topology and the viewport must share one immutable
			// artifact. Exact-only and display artifacts have distinct keys.
			if bodyIndex(*model, publication.Target.PersistentSelection.SourceBodyID) < 0 {
				broken("PUBLICATION_BODY_MISSING", "published Body no longer exists")
				continue
			}
			finalGeometryKey, err := bodyKey(publication.Target.PersistentSelection.SourceBodyID)
			if err != nil {
				return err
			}
			if finalGeometryKey == "" {
				broken("PUBLICATION_BODY_EMPTY", "published Body has no geometry")
				continue
			}
			resolution, manifestDigest, err := service.resolvePublicationSelection(ctx, documentID, publication.Target, finalGeometryKey)
			if err != nil {
				return err
			}
			if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
				broken("PUBLICATION_TARGET_"+string(resolution.Status), resolution.Diagnostic)
				continue
			}
			candidate := resolution.Candidates[0]
			artifact, err := service.loadArtifact(ctx, candidate.GeometryKey)
			if err != nil {
				return err
			}
			properties, err := service.getTopologyElementPropertiesFromArtifact(ctx, candidate.GeometryKey, string(candidate.Type), candidate.LocalID)
			if err != nil {
				return err
			}
			exact, descriptorErr := assemblyGeometryFromProperties(geometry.AssemblyGeometry{}, properties)
			if descriptorErr != nil {
				exact = geometry.AssemblyGeometry{Kind: properties.GeometryType, Direction: [3]float64{0, 0, 1}, XDirection: [3]float64{1, 0, 0}}
			}
			z := exact.Direction
			if exact.Kind == "SPHERE" || exact.Kind == "POINT" {
				z = [3]float64{0, 0, 1}
			} // Cosmetic local frame, not a sphere axis.
			x, xOK := stableSupportX(z, exact.XDirection)
			if !xOK {
				broken("PUBLICATION_LOCAL_FRAME_UNAVAILABLE", "resolved topology does not provide a stable local frame")
				continue
			}
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID,
				GeometryKey: candidate.GeometryKey, GeometryID: candidate.GeometryID, TopologyKind: string(candidate.Type), GeometryKind: properties.GeometryType,
				LocalID: candidate.LocalID, Origin: exact.Origin, XDirection: x, YDirection: cross3(z, x), ZDirection: z,
				Radius: exact.Radius, HalfAngle: exact.HalfAngle, ConeLeaf: exact.ConeLeaf, MaterialSide: exact.MaterialSide,
				ParameterStart: exact.ParameterStart, ParameterEnd: exact.ParameterEnd, LengthUnit: exact.LengthUnit,
				SourceDigest: resolvedDigest(struct {
					Selection modelcore.PersistentSelection
					Candidate modelcore.ResolvedTopologyElement
				}{*publication.Target.PersistentSelection, candidate}), ManifestDigest: manifestDigest,
				NamingPolicyDigest: modelcore.TopologyNamingPolicyDigest, EvaluatorVersion: artifact.EvaluatorVersion,
				WorkerID: artifact.WorkerID, OCCTVersion: artifact.OCCTVersion}
		}
		publication.Resolution.Element = &element
		if publication.Resolution.Status == "CONNECTED" {
			if publication.Target.Kind != "TOPOLOGY" {
				publication.Resolution.GeometryKind = publication.Contract.GeometryKind
			}
			publication.Resolution.Symmetry = publication.Contract.Symmetry
		}
	}
	return nil
}

func resolveDatumPublication(model *PartModel, publication *Publication, revisionID string) bool {
	for _, point := range model.DatumPoints {
		if point.ID == publication.Target.DatumID && publication.Type == "POINT" {
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, GeometryKind: "POINT", Origin: point.Position, SourceDigest: resolvedDigest(point.Position)}
			return true
		}
	}
	for _, plane := range model.DatumPlanes {
		if plane.ID != publication.Target.DatumID || publication.Type != "PLANE" {
			continue
		}
		n, ok := normalize3(plane.Normal)
		if !ok {
			return false
		}
		x, ok := stableSupportX(n, plane.UDirection)
		if !ok {
			return false
		}
		publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: plane.Origin, XDirection: x, YDirection: cross3(n, x), ZDirection: n, SourceDigest: resolvedDigest(struct{ Origin, X, Normal [3]float64 }{plane.Origin, x, n})}
		return true
	}
	for _, axis := range model.DatumAxes {
		if axis.ID != publication.Target.DatumID || publication.Type != "AXIS" {
			continue
		}
		z, ok := normalize3(axis.Direction)
		if !ok {
			return false
		}
		x, ok := stableSupportX(z, [3]float64{})
		if !ok {
			return false
		}
		publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: axis.Origin, XDirection: x, YDirection: cross3(z, x), ZDirection: z, SourceDigest: resolvedDigest(struct{ Origin, Direction [3]float64 }{axis.Origin, z})}
		return true
	}
	for _, frame := range model.AxisSystems {
		if frame.ID != publication.Target.DatumID {
			continue
		}
		switch publication.Type {
		case "POINT":
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: frame.Origin, SourceDigest: resolvedDigest(frame.Origin)}
		case "FRAME":
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: frame.Origin, XDirection: frame.XDirection, YDirection: frame.YDirection, ZDirection: frame.ZDirection, SourceDigest: resolvedDigest(frame)}
		case "AXIS":
			var direction [3]float64
			switch publication.Target.Axis {
			case "X":
				direction = frame.XDirection
			case "Y":
				direction = frame.YDirection
			case "Z":
				direction = frame.ZDirection
			default:
				return false
			}
			z, zOK := normalize3(direction)
			x, xOK := stableSupportX(z, [3]float64{})
			if !zOK || !xOK {
				return false
			}
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID,
				Origin: frame.Origin, XDirection: x, YDirection: cross3(z, x), ZDirection: z,
				SourceDigest: resolvedDigest(struct{ ID, Axis string }{frame.ID, publication.Target.Axis})}
		default:
			return false
		}
		return true
	}
	return false
}

func publicationFeaturePrefix(model PartModel, featureID string) (PartModel, bool) {
	for index, feature := range model.Features {
		if feature.ID != featureID || !isSolidGenerator(feature.Type) {
			continue
		}
		model.Features = append([]Feature(nil), model.Features[:index+1]...)
		return model, true
	}
	return PartModel{}, false
}

func rejectExplicitBrokenPublication(command modelcore.DomainCommand, model PartModel) error {
	if command.TypeURI != typeCreatePublication && command.TypeURI != typeRedirectPublication && command.TypeURI != typeEditPublication {
		return nil
	}
	var payload publicationPayload
	if json.Unmarshal(command.Payload, &payload) != nil {
		return nil
	}
	if command.TypeURI == typeEditPublication {
		var edit publicationEditPayload
		if err := json.Unmarshal(command.Payload, &edit); err != nil || edit.Target == nil {
			return nil
		}
		payload.Publication = *edit.Target
	}
	for _, publication := range model.Publications {
		if publication.ID == payload.Publication.ID && publication.Resolution.Status != "CONNECTED" {
			return fmt.Errorf("%w: %s: %s", ErrValidation, publication.Resolution.DiagnosticCode, publication.Resolution.Diagnostic)
		}
	}
	return nil
}

func (service *Service) publicationFromRequest(ctx context.Context, documentID string, model PartModel, request CommandRequest, publicationID string) (Publication, error) {
	if request.SourceDocumentID != "" && request.SourceDocumentID != documentID {
		return Publication{}, fmt.Errorf("%w: PUBLICATION_TARGET_DOCUMENT_MISMATCH", ErrValidation)
	}
	if request.VersionID != "" {
		var head string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, documentID).Scan(&head); err != nil {
			return Publication{}, err
		}
		if request.VersionID != head {
			return Publication{}, fmt.Errorf("%w: PUBLICATION_TARGET_REVISION_CHANGED", ErrValidation)
		}
	}
	if request.InstancePath != nil {
		_, occurrences, err := service.productContextRoot(ctx, request.InstancePath.RootDocumentID)
		if err != nil {
			return Publication{}, err
		}
		owner, ok := occurrenceByCanonical(occurrences, request.InstancePath.Canonical)
		if !ok || owner.DocumentID != documentID || (request.VersionID != "" && owner.RevisionID != request.VersionID) {
			return Publication{}, fmt.Errorf("%w: PUBLICATION_TARGET_OCCURRENCE_MISMATCH", ErrValidation)
		}
		if err := validateResolvedInstancePath(*request.InstancePath, owner.Path); err != nil {
			return Publication{}, err
		}
	}
	publicationType := strings.ToUpper(strings.TrimSpace(request.PublicationType))
	if publicationType == "" {
		return Publication{}, fmt.Errorf("%w: publication type is required", ErrValidation)
	}
	version := strings.TrimSpace(request.CompatibilityVersion)
	if version == "" {
		version = "1.0.0"
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		existing := make([]string, 0, len(model.Publications))
		for _, item := range model.Publications {
			existing = append(existing, item.Name)
		}
		name = nextScopedName(defaultPublicationBase(request.TargetKind, publicationType), existing)
	}
	publication := Publication{ID: publicationID, Name: name, Type: publicationType, SemanticPurpose: strings.TrimSpace(request.SemanticPurpose), CompatibilityVersion: version, Resolution: PublicationResolution{Status: "PENDING"}}
	switch strings.ToUpper(strings.TrimSpace(request.TargetKind)) {
	case "SKETCH_ENTITY":
		target := PublicationTarget{Kind: "SKETCH_ENTITY", SketchID: request.SketchID, EntityID: request.TargetID}
		for _, f := range model.Features {
			if f.ID == target.SketchID && f.Sketch != nil {
				for _, e := range f.Sketch.Entities {
					if e.ID == target.EntityID {
						target.EntityKind = e.Kind
					}
				}
			}
		}
		expected := "CURVE"
		if target.EntityKind == "POINT" {
			expected = "POINT"
		}
		if publicationType != expected {
			return Publication{}, fmt.Errorf("%w: native sketch publication type mismatch", ErrValidation)
		}
		if _, err := nativeSketchPublication(model, target, request.VersionID); err != nil {
			return Publication{}, err
		}
		publication.Target = target
		symmetry := "NONE"
		if publicationType == "CURVE" {
			symmetry = "PARAMETER_DIRECTION"
		}
		publication.Contract = PublicationContract{GeometryKind: publicationType, Symmetry: symmetry}
	case "PLANE":
		publication.Target = PublicationTarget{Kind: "DATUM", DatumID: request.TargetID}
		publication.Contract = PublicationContract{GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"}
	case "REFERENCE_POINT":
		publication.Target = PublicationTarget{Kind: "DATUM", DatumID: request.TargetID}
		publication.Contract = PublicationContract{GeometryKind: "POINT", Symmetry: "NONE"}
	case "DATUM_AXIS", "AXIS":
		publication.Target = PublicationTarget{Kind: "DATUM", DatumID: request.TargetID, Axis: strings.ToUpper(request.Axis)}
		publication.Contract = PublicationContract{GeometryKind: "AXIS", Symmetry: "DIRECTION_UNORIENTED"}
	case "AXIS_SYSTEM":
		publication.Target = PublicationTarget{Kind: "DATUM", DatumID: request.TargetID, Axis: strings.ToUpper(request.Axis)}
		symmetry := "RIGHT_HANDED_FRAME"
		if publicationType == "POINT" {
			symmetry = "NONE"
		} else if publicationType == "AXIS" {
			symmetry = "DIRECTION_UNORIENTED"
		}
		publication.Contract = PublicationContract{GeometryKind: publicationType, Symmetry: symmetry}
	case "FACE", "EDGE", "VERTEX":
		sourceVersionID := strings.TrimSpace(request.VersionID)
		if sourceVersionID == "" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&sourceVersionID); err != nil {
				return Publication{}, err
			}
		}
		selection, err := service.bindPublicationGeometry(ctx, documentID, model, request)
		if err != nil {
			return Publication{}, err
		}
		expected := "SURFACE"
		geometryKind := "SURFACE"
		symmetry := "NONE"
		if strings.EqualFold(request.TargetKind, "EDGE") {
			expected, geometryKind, symmetry = "CURVE", "CURVE", "PARAMETER_DIRECTION"
		}
		if strings.EqualFold(request.TargetKind, "VERTEX") {
			expected, geometryKind = "POINT", "POINT"
		}
		if publicationType != expected {
			return Publication{}, fmt.Errorf("%w: publication type %s is incompatible with %s", ErrValidation, publicationType, request.TargetKind)
		}
		publication.Target = PublicationTarget{Kind: "TOPOLOGY", PersistentSelection: &selection, SourceVersionID: sourceVersionID, SourceGeometryKey: request.GeometryKey}
		publication.Contract = PublicationContract{GeometryKind: geometryKind, Symmetry: symmetry}
	case "BODY":
		if publicationType != "BODY" {
			return Publication{}, fmt.Errorf("%w: BODY output requires BODY publication type", ErrValidation)
		}
		if bodyIndex(model, request.TargetID) < 0 {
			return Publication{}, fmt.Errorf("%w: published Body does not exist", ErrValidation)
		}
		publication.Target = PublicationTarget{Kind: "BODY_RESULT", BodyID: request.TargetID}
		publication.Contract = PublicationContract{GeometryKind: "BODY", Symmetry: "NONE"}
	case "FEATURE_OUTPUT":
		if publicationType != "BODY" {
			return Publication{}, fmt.Errorf("%w: feature output requires BODY publication type", ErrValidation)
		}
		if _, ok := publicationFeaturePrefix(model, request.TargetID); !ok {
			return Publication{}, fmt.Errorf("%w: published feature output does not exist", ErrValidation)
		}
		publication.Target = PublicationTarget{Kind: "FEATURE_OUTPUT", FeatureID: request.TargetID, OutputSlot: "BODY"}
		publication.Contract = PublicationContract{GeometryKind: "BODY", Symmetry: "NONE"}
	case "PARAMETER":
		if publicationType != "PARAMETER" {
			return Publication{}, fmt.Errorf("%w: parameter target requires PARAMETER publication type", ErrValidation)
		}
		for _, parameter := range model.Parameters {
			if parameter.ParameterID != request.TargetID {
				continue
			}
			dimension := parameter.Dimension
			publication.Target = PublicationTarget{Kind: "PARAMETER", ParameterID: parameter.ParameterID}
			publication.Contract = PublicationContract{ValueType: parameter.ValueType, Dimension: &dimension, UnitPolicy: "SI_CANONICAL"}
			return publication, nil
		}
		return Publication{}, fmt.Errorf("%w: published parameter does not exist", ErrValidation)
	default:
		return Publication{}, fmt.Errorf("%w: unsupported publication target", ErrValidation)
	}
	return publication, nil
}

func (service *Service) resolveExternalParameterRef(ctx context.Context, consumerDocumentID, sourceDocumentID,
	sourceVersionID, publicationID string, expected modelcore.ParameterDefinition, consumerModel PartModel) (modelcore.ExternalParameterRef, error) {
	return service.resolveExternalParameterRefWithMode(ctx, consumerDocumentID, sourceDocumentID,
		sourceVersionID, publicationID, "PINNED", expected, consumerModel)
}

func (service *Service) resolveExternalParameterRefWithMode(ctx context.Context, consumerDocumentID, sourceDocumentID,
	sourceVersionID, publicationID, referenceMode string, expected modelcore.ParameterDefinition, consumerModel PartModel) (modelcore.ExternalParameterRef, error) {
	sourceDocumentID = strings.TrimSpace(sourceDocumentID)
	publicationID = strings.TrimSpace(publicationID)
	if sourceDocumentID == "" || publicationID == "" {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: source document and PublicationId are required", ErrValidation)
	}
	if sourceDocumentID == consumerDocumentID {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_CYCLE: a Part cannot import its own parameter", ErrValidation)
	}
	if strings.TrimSpace(sourceVersionID) == "" {
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`,
			sourceDocumentID).Scan(&sourceVersionID); errors.Is(err, database.ErrNoRows) {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_SOURCE_DELETED", ErrValidation)
		} else if err != nil {
			return modelcore.ExternalParameterRef{}, err
		}
	}
	var sourceType string
	var sourceModelJSON []byte
	if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
		JOIN occccad.documents d ON d.id=v.document_id WHERE v.id=$1 AND d.id=$2 AND d.deleted_at IS NULL`,
		sourceVersionID, sourceDocumentID).Scan(&sourceType, &sourceModelJSON); errors.Is(err, database.ErrNoRows) {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_REVISION_MISSING", ErrValidation)
	} else if err != nil {
		return modelcore.ExternalParameterRef{}, err
	}
	if sourceType != "PART" && sourceType != "PRODUCT" {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_SOURCE_UNSUPPORTED", ErrValidation)
	}
	var sourceModel PartModel
	if err := json.Unmarshal(sourceModelJSON, &sourceModel); err != nil {
		return modelcore.ExternalParameterRef{}, err
	}
	if sourceType == "PRODUCT" {
		var product ProductModel
		if err := json.Unmarshal(sourceModelJSON, &product); err != nil {
			return modelcore.ExternalParameterRef{}, err
		}
		if err := service.refreshProductMeasurementParameters(ctx, sourceDocumentID, sourceVersionID, sourceVersionID, &product); err != nil {
			return modelcore.ExternalParameterRef{}, err
		}
		sourceDoc := &parameterDocument{product: &product, Parameters: product.Parameters}
		for i := range product.Publications {
			if product.Publications[i].Type == "PARAMETER" {
				product.Publications[i].Resolution.ResolvedVersionID = sourceVersionID
			}
		}
		sourceModel = parameterPartContext(sourceDoc)
	}
	for _, publication := range sourceModel.Publications {
		if publication.ID != publicationID {
			continue
		}
		if publication.Type != "PARAMETER" || publication.Contract.ValueType != expected.ValueType ||
			publication.Contract.Dimension == nil || !publication.Contract.Dimension.Equal(expected.Dimension) {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_CONTRACT_INCOMPATIBLE", ErrValidation)
		}
		if publication.Resolution.Status != "CONNECTED" || publication.Resolution.Value == nil {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: BROKEN_PUBLICATION: %s", ErrValidation, publication.Resolution.Diagnostic)
		}
		if currency := publication.Resolution.CurrencyStatus; currency != "" && currency != "CURRENT" {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_SOURCE_UPDATE_REQUIRED: %s", ErrValidation, currency)
		}
		if publication.Resolution.ResolvedVersionID != sourceVersionID {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: PUBLICATION_PROVENANCE_MISMATCH", ErrValidation)
		}
		if cycle, err := service.externalParameterTargetReaches(ctx, sourceDocumentID, sourceModel,
			publication.Target.ParameterID, consumerDocumentID, expected.ParameterID, consumerModel, map[string]bool{}, 0); err != nil {
			return modelcore.ExternalParameterRef{}, err
		} else if cycle {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_CYCLE", ErrValidation)
		}
		value := *publication.Resolution.Value
		if !value.Dimension.Equal(expected.Dimension) {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_DIMENSION_MISMATCH", ErrValidation)
		}
		digest := resolvedDigest(value)
		if publication.Resolution.ValueDigest != "" && publication.Resolution.ValueDigest != digest {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_VALUE_DIGEST_MISMATCH", ErrValidation)
		}
		if referenceMode == "" {
			referenceMode = "FOLLOW_HEAD"
		}
		if referenceMode != "PINNED" && referenceMode != "FOLLOW_HEAD" && referenceMode != "FOLLOW_WORKSPACE_WITH_ACCEPT" {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: unsupported reference mode", ErrValidation)
		}
		contractDigest := resolvedDigest(publication.Contract)
		return modelcore.ExternalParameterRef{SourceDocumentID: sourceDocumentID,
			Revision:      modelcore.ReferenceSelector{Mode: referenceMode, RevisionID: sourceVersionID},
			PublicationID: publicationID, ExpectedType: expected.ValueType, ExpectedDimension: expected.Dimension,
			ContractVersion: publication.CompatibilityVersion, ResolvedRevisionID: sourceVersionID,
			ResolvedValue: value, ResolvedValueDigest: digest,
			ResolutionSnapshot: modelcore.ReferenceResolutionSnapshot{SourceRevisionID: sourceVersionID,
				PublicationID: publicationID, ContractDigest: contractDigest, ValueDigest: digest, Status: "CONNECTED"}}, nil
	}
	return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_PUBLICATION_MISSING", ErrValidation)
}

func (service *Service) externalParameterTargetReaches(ctx context.Context, currentDocumentID string, model PartModel,
	parameterID, targetDocumentID, targetParameterID string, targetModel PartModel, visited map[string]bool, depth int) (bool, error) {
	if depth > 64 {
		return false, fmt.Errorf("%w: EXTERNAL_PARAMETER_GRAPH_TOO_DEEP", ErrValidation)
	}
	if currentDocumentID == targetDocumentID && parameterID == targetParameterID {
		return true, nil
	}
	nodeKey := currentDocumentID + "#" + parameterID
	if visited[nodeKey] {
		return false, nil
	}
	visited[nodeKey] = true
	for _, parameter := range model.Parameters {
		if parameter.ParameterID != parameterID {
			continue
		}
		if produced := parameter.Source.Produced; produced != nil && produced.Kind == "MEASUREMENT" {
			for _, measurement := range model.Measurements {
				if measurement.ID != produced.EntityID {
					continue
				}
				for _, support := range measurement.Supports {
					owner, source := currentDocumentID, model
					if path := support.InstancePath; path != nil && len(path.Segments) > 0 {
						segment := path.Segments[len(path.Segments)-1]
						owner = segment.ReferencedDocumentID
						if owner == targetDocumentID {
							source = targetModel
						} else {
							_, raw, err := service.measurementModel(ctx, owner, segment.ResolvedVersionID)
							if err != nil {
								return false, err
							}
							doc, err := readParameterDocument(raw)
							if err != nil {
								return false, err
							}
							source = parameterPartContext(doc)
						}
					}
					bodyID := support.BodyID
					if support.Selection != nil {
						bodyID = support.Selection.SourceBodyID
					}
					owners := map[string]bool{}
					for _, feature := range source.Features {
						if !feature.Suppressed && (support.Kind == "OCCURRENCE" || feature.BodyID == bodyID) {
							owners[feature.ID] = true
						}
					}
					for _, input := range source.Parameters {
						if !owners[input.OwnerFeatureID] {
							continue
						}
						reaches, err := service.externalParameterTargetReaches(ctx, owner, source, input.ParameterID, targetDocumentID, targetParameterID, targetModel, visited, depth+1)
						if err != nil || reaches {
							return reaches, err
						}
					}
				}
			}
			return false, nil
		}
		if expression := parameter.Source.Expression; expression != nil {
			for _, read := range expression.Reads {
				reaches, err := service.externalParameterTargetReaches(ctx, currentDocumentID, model,
					strings.TrimPrefix(string(read), "parameter:"), targetDocumentID, targetParameterID, targetModel, visited, depth+1)
				if err != nil || reaches {
					return reaches, err
				}
			}
			return false, nil
		}
		reference := parameter.Source.External
		if reference == nil {
			return false, nil
		}
		key := reference.SourceDocumentID + "@" + reference.ResolvedRevisionID + "#" + reference.PublicationID
		if visited[key] {
			return false, nil
		}
		visited[key] = true
		var nested PartModel
		if reference.SourceDocumentID == targetDocumentID {
			nested = targetModel
		} else {
			var nestedJSON []byte
			if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions
				WHERE id=$1 AND document_id=$2`, reference.ResolvedRevisionID, reference.SourceDocumentID).Scan(&nestedJSON); errors.Is(err, database.ErrNoRows) {
				return false, fmt.Errorf("%w: EXTERNAL_PARAMETER_REVISION_MISSING", ErrValidation)
			} else if err != nil {
				return false, err
			}
			doc, err := readParameterDocument(nestedJSON)
			if err != nil {
				return false, err
			}
			nested = parameterPartContext(doc)
		}
		for _, publication := range nested.Publications {
			if publication.ID != reference.PublicationID || publication.Type != "PARAMETER" {
				continue
			}
			return service.externalParameterTargetReaches(ctx, reference.SourceDocumentID, nested,
				publication.Target.ParameterID, targetDocumentID, targetParameterID, targetModel, visited, depth+1)
		}
		return false, fmt.Errorf("%w: EXTERNAL_PARAMETER_PUBLICATION_MISSING", ErrValidation)
	}
	return false, nil
}

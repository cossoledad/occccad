package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
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
	PublicationID        string `json:"publicationId"`
	Name                 string `json:"name"`
	SemanticPurpose      string `json:"semanticPurpose"`
	CompatibilityVersion string `json:"compatibilityVersion"`
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
	for _, publication := range model.Publications {
		if strings.TrimSpace(publication.ID) == "" || strings.TrimSpace(publication.Name) == "" || strings.TrimSpace(publication.CompatibilityVersion) == "" {
			return fmt.Errorf("%w: publication identity, name and compatibility version are required", ErrValidation)
		}
		if ids[publication.ID] {
			return fmt.Errorf("%w: duplicate PublicationId", ErrValidation)
		}
		ids[publication.ID] = true
		switch publication.Type {
		case "POINT", "AXIS", "PLANE", "FRAME", "CURVE", "SURFACE", "BODY", "PARAMETER":
		default:
			return fmt.Errorf("%w: unsupported publication type %s", ErrValidation, publication.Type)
		}
		switch publication.Target.Kind {
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

func (service *Service) resolvePartPublications(ctx context.Context, documentID, requestID, revisionID string, model *PartModel) error {
	var finalGeometryKey string
	for index := range model.Publications {
		publication := &model.Publications[index]
		broken := func(code, diagnostic string) {
			publication.Resolution = PublicationResolution{Status: "BROKEN_PUBLICATION", DiagnosticCode: code, Diagnostic: diagnostic}
		}
		switch publication.Target.Kind {
		case "DATUM":
			if !resolveDatumPublication(model, publication, revisionID) {
				broken("PUBLICATION_TARGET_MISSING", "datum target no longer exists")
			}
		case "PARAMETER":
			found := false
			for _, parameter := range model.Parameters {
				if parameter.ParameterID != publication.Target.ParameterID || parameter.EvaluatedValue == nil {
					continue
				}
				if publication.Contract.Dimension == nil || parameter.ValueType != publication.Contract.ValueType ||
					!parameter.Dimension.Equal(*publication.Contract.Dimension) {
					broken("PUBLICATION_PARAMETER_CONTRACT_INCOMPATIBLE", "published parameter type or dimension changed")
					found = true
					continue
				}
				value := *parameter.EvaluatedValue
				if bounds := publication.Contract.Bounds; bounds != nil &&
					((bounds.Minimum != nil && value.SIValue < bounds.Minimum.SIValue) ||
						(bounds.Maximum != nil && value.SIValue > bounds.Maximum.SIValue)) {
					broken("PUBLICATION_PARAMETER_OUT_OF_BOUNDS", "published parameter value is outside its contract bounds")
					found = true
					continue
				}
				publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Value: &value, ValueDigest: resolvedDigest(value), SourceDigest: resolvedDigest(struct {
					ID    string
					Value modelcore.Quantity
				}{parameter.ParameterID, value})}
				found = true
			}
			if !found {
				broken("PUBLICATION_PARAMETER_MISSING", "published parameter no longer exists")
			}
		case "FEATURE_OUTPUT":
			prefix, ok := publicationFeaturePrefix(*model, publication.Target.FeatureID)
			if !ok {
				broken("PUBLICATION_FEATURE_OUTPUT_MISSING", "published feature output no longer exists")
				continue
			}
			key, err := service.evaluatePart(ctx, requestID+"/publication/"+publication.ID, prefix)
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
			if finalGeometryKey == "" {
				var err error
				finalGeometryKey, err = service.evaluatePart(ctx, requestID+"/publication-body", *model)
				if err != nil {
					return err
				}
			}
			resolution, manifestDigest, err := service.resolveSelectionAgainstGeometry(ctx, documentID, publication.Target.SourceVersionID, finalGeometryKey, *publication.Target.PersistentSelection)
			if err != nil {
				return err
			}
			if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
				broken("PUBLICATION_TARGET_"+string(resolution.Status), resolution.Diagnostic)
				continue
			}
			candidate := resolution.Candidates[0]
			evidence := candidate.Evidence
			x, xOK := stableSupportX(evidence.Direction, [3]float64{})
			z, zOK := normalize3(evidence.Direction)
			if !xOK || !zOK {
				broken("PUBLICATION_LOCAL_FRAME_UNAVAILABLE", "resolved topology does not provide a stable local frame")
				continue
			}
			artifact, err := service.loadArtifact(ctx, candidate.GeometryKey)
			if err != nil {
				return err
			}
			publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID,
				GeometryKey: candidate.GeometryKey, GeometryID: candidate.GeometryID, TopologyKind: string(candidate.Type),
				LocalID: candidate.LocalID, Origin: evidence.Origin, XDirection: x, YDirection: cross3(z, x), ZDirection: z,
				SourceDigest: resolvedDigest(struct {
					Selection modelcore.PersistentSelection
					Candidate modelcore.ResolvedTopologyElement
				}{*publication.Target.PersistentSelection, candidate}), ManifestDigest: manifestDigest,
				NamingPolicyDigest: modelcore.TopologyNamingPolicyDigest, EvaluatorVersion: artifact.EvaluatorVersion,
				WorkerID: artifact.WorkerID, OCCTVersion: artifact.OCCTVersion}
		}
		if publication.Resolution.Status == "CONNECTED" {
			publication.Resolution.GeometryKind = publication.Contract.GeometryKind
			publication.Resolution.Symmetry = publication.Contract.Symmetry
		}
	}
	return nil
}

func resolveDatumPublication(model *PartModel, publication *Publication, revisionID string) bool {
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
		publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: plane.Origin, XDirection: x, YDirection: cross3(n, x), ZDirection: n, SourceDigest: resolvedDigest(plane)}
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
		publication.Resolution = PublicationResolution{Status: "CONNECTED", ResolvedVersionID: revisionID, Origin: axis.Origin, XDirection: x, YDirection: cross3(z, x), ZDirection: z, SourceDigest: resolvedDigest(axis)}
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
	if command.TypeURI != typeCreatePublication && command.TypeURI != typeRedirectPublication {
		return nil
	}
	var payload publicationPayload
	if json.Unmarshal(command.Payload, &payload) != nil {
		return nil
	}
	for _, publication := range model.Publications {
		if publication.ID == payload.Publication.ID && publication.Resolution.Status != "CONNECTED" {
			return fmt.Errorf("%w: %s: %s", ErrValidation, publication.Resolution.DiagnosticCode, publication.Resolution.Diagnostic)
		}
	}
	return nil
}

func (service *Service) publicationFromRequest(ctx context.Context, documentID string, model PartModel, request CommandRequest, publicationID string) (Publication, error) {
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
		name = publicationType
	}
	publication := Publication{ID: publicationID, Name: name, Type: publicationType, SemanticPurpose: strings.TrimSpace(request.SemanticPurpose), CompatibilityVersion: version, Resolution: PublicationResolution{Status: "PENDING"}}
	switch strings.ToUpper(strings.TrimSpace(request.TargetKind)) {
	case "PLANE":
		publication.Target = PublicationTarget{Kind: "DATUM", DatumID: request.TargetID}
		publication.Contract = PublicationContract{GeometryKind: "PLANE", Symmetry: "NORMAL_UNORIENTED"}
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
	case "FACE", "EDGE":
		sourceVersionID := strings.TrimSpace(request.VersionID)
		if sourceVersionID == "" {
			if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&sourceVersionID); err != nil {
				return Publication{}, err
			}
		}
		selection, err := service.BindPersistentSelection(ctx, documentID, BindPersistentSelectionRequest{SourceVersionID: sourceVersionID, GeometryKey: request.GeometryKey, Kind: strings.ToUpper(request.TargetKind), LocalID: request.TopologyID})
		if err != nil {
			return Publication{}, err
		}
		expected := "SURFACE"
		geometryKind := "SURFACE"
		symmetry := "NONE"
		if strings.EqualFold(request.TargetKind, "EDGE") {
			expected, geometryKind, symmetry = "CURVE", "CURVE", "PARAMETER_DIRECTION"
		}
		if publicationType != expected {
			return Publication{}, fmt.Errorf("%w: publication type %s is incompatible with %s", ErrValidation, publicationType, request.TargetKind)
		}
		publication.Target = PublicationTarget{Kind: "TOPOLOGY", PersistentSelection: &selection, SourceVersionID: sourceVersionID}
		publication.Contract = PublicationContract{GeometryKind: geometryKind, Symmetry: symmetry}
	case "BODY":
		if publicationType != "BODY" {
			return Publication{}, fmt.Errorf("%w: BODY output requires BODY publication type", ErrValidation)
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
			sourceDocumentID).Scan(&sourceVersionID); errors.Is(err, pgx.ErrNoRows) {
			return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_SOURCE_DELETED", ErrValidation)
		} else if err != nil {
			return modelcore.ExternalParameterRef{}, err
		}
	}
	var sourceType string
	var sourceModelJSON []byte
	if err := service.database.QueryRow(ctx, `SELECT d.document_type,v.model_json FROM occccad.document_versions v
		JOIN occccad.documents d ON d.id=v.document_id WHERE v.id=$1 AND d.id=$2 AND d.deleted_at IS NULL`,
		sourceVersionID, sourceDocumentID).Scan(&sourceType, &sourceModelJSON); errors.Is(err, pgx.ErrNoRows) {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_REVISION_MISSING", ErrValidation)
	} else if err != nil {
		return modelcore.ExternalParameterRef{}, err
	}
	if sourceType != "PART" {
		return modelcore.ExternalParameterRef{}, fmt.Errorf("%w: EXTERNAL_PARAMETER_SOURCE_NOT_PART", ErrValidation)
	}
	var sourceModel PartModel
	if err := json.Unmarshal(sourceModelJSON, &sourceModel); err != nil {
		return modelcore.ExternalParameterRef{}, err
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
		return modelcore.ExternalParameterRef{SourceDocumentID: sourceDocumentID,
			Revision:      modelcore.ReferenceSelector{Mode: "PINNED", RevisionID: sourceVersionID},
			PublicationID: publicationID, ExpectedType: expected.ValueType, ExpectedDimension: expected.Dimension,
			ContractVersion: publication.CompatibilityVersion, ResolvedRevisionID: sourceVersionID,
			ResolvedValue: value, ResolvedValueDigest: digest}, nil
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
				WHERE id=$1 AND document_id=$2`, reference.ResolvedRevisionID, reference.SourceDocumentID).Scan(&nestedJSON); errors.Is(err, pgx.ErrNoRows) {
				return false, fmt.Errorf("%w: EXTERNAL_PARAMETER_REVISION_MISSING", ErrValidation)
			} else if err != nil {
				return false, err
			}
			if err := json.Unmarshal(nestedJSON, &nested); err != nil {
				return false, err
			}
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

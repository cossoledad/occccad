package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/occccad/occccad/internal/modelcore"
)

const (
	typeCreateContextReference = "occccad://part/context-reference/create"
	typeDetachContextReference = "occccad://part/context-reference/detach"
)

type contextReferencePayload struct {
	Model     PartModel         `json:"model"`
	Reference ContextReference  `json:"reference"`
	Before    *ContextReference `json:"before,omitempty"`
}

func applyContextReferenceModel(modelJSON json.RawMessage, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var current PartModel
	if err := json.Unmarshal(modelJSON, &current); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	var payload contextReferencePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, _ := json.Marshal(payload.Model)
	kind := modelcore.ChangeCreate
	var before any
	if payload.Before != nil {
		kind, before = modelcore.ChangeBind, *payload.Before
	}
	change, _ := modelcore.NewChange(kind, modelcore.PropertyAddress{EntityID: payload.Reference.ID,
		SlotID: "context-reference.entity"}, before, payload.Reference)
	changes := []modelcore.ModelChange{change}
	seeds := []modelcore.DependencyKey{"context-reference:" + modelcore.DependencyKey(payload.Reference.ID)}
	if target := payload.Reference.LocalTargetID; target != "" {
		beforeParameters := map[string]modelcore.ValueSource{}
		for _, parameter := range current.Parameters {
			beforeParameters[parameter.ParameterID] = parameter.Source
		}
		for _, parameter := range payload.Model.Parameters {
			if prior, ok := beforeParameters[parameter.ParameterID]; ok && resolvedDigest(prior) != resolvedDigest(parameter.Source) {
				item, _ := modelcore.NewChange(modelcore.ChangeBind, modelcore.PropertyAddress{EntityID: parameter.ParameterID,
					SlotID: "parameter.source"}, prior, parameter.Source)
				changes, seeds = append(changes, item), append(seeds, "parameter:"+modelcore.DependencyKey(parameter.ParameterID))
			}
		}
		beforePlanes := map[string]DatumPlane{}
		for _, plane := range current.DatumPlanes {
			beforePlanes[plane.ID] = plane
		}
		for _, plane := range payload.Model.DatumPlanes {
			if plane.ID == target && resolvedDigest(beforePlanes[target]) != resolvedDigest(plane) {
				item, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: target, SlotID: "datum.plane"}, beforePlanes[target], plane)
				changes, seeds = append(changes, item), append(seeds, "datum:"+modelcore.DependencyKey(target))
			}
		}
		beforeAxes := map[string]DatumAxis{}
		for _, axis := range current.DatumAxes {
			beforeAxes[axis.ID] = axis
		}
		for _, axis := range payload.Model.DatumAxes {
			if axis.ID == target && resolvedDigest(beforeAxes[target]) != resolvedDigest(axis) {
				item, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: target, SlotID: "datum.axis"}, beforeAxes[target], axis)
				changes, seeds = append(changes, item), append(seeds, "datum:"+modelcore.DependencyKey(target))
			}
		}
		if payload.Reference.Publication.ExpectedType == "CURVE" {
			item, _ := modelcore.NewChange(modelcore.ChangeUpdate,
				modelcore.PropertyAddress{EntityID: target, SlotID: "sketch.model"}, nil, nil)
			changes, seeds = append(changes, item), append(seeds, "feature:"+modelcore.DependencyKey(target))
		}
	}
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, nil
}

func rotateByPose(pose InstancePose, vector [3]float64) [3]float64 {
	q := normalizedInstanceRotation(pose.Rotation)
	u := [3]float64{q[0], q[1], q[2]}
	dot := u[0]*vector[0] + u[1]*vector[1] + u[2]*vector[2]
	cross := [3]float64{u[1]*vector[2] - u[2]*vector[1], u[2]*vector[0] - u[0]*vector[2], u[0]*vector[1] - u[1]*vector[0]}
	uu := u[0]*u[0] + u[1]*u[1] + u[2]*u[2]
	return [3]float64{2*dot*u[0] + (q[3]*q[3]-uu)*vector[0] + 2*q[3]*cross[0],
		2*dot*u[1] + (q[3]*q[3]-uu)*vector[1] + 2*q[3]*cross[1],
		2*dot*u[2] + (q[3]*q[3]-uu)*vector[2] + 2*q[3]*cross[2]}
}

func pointByPose(pose InstancePose, point [3]float64) [3]float64 {
	rotated := rotateByPose(pose, point)
	return [3]float64{rotated[0] + pose.Translation[0], rotated[1] + pose.Translation[1], rotated[2] + pose.Translation[2]}
}

func (service *Service) contextSource(ctx context.Context, consumerDocumentID string, reference ContextReference) (string, string, InstancePose, *InstancePath, string, string, error) {
	if reference.RootProductDocumentID == "" {
		var head string
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`,
			reference.SourceDocumentID).Scan(&head); errors.Is(err, pgx.ErrNoRows) {
			return "", "", InstancePose{}, nil, "", "", fmt.Errorf("%w: CONTEXT_SOURCE_MISSING", ErrValidation)
		} else if err != nil {
			return "", "", InstancePose{}, nil, "", "", err
		}
		return reference.SourceDocumentID, head, InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, nil, "", "", nil
	}
	rootRevision := reference.RootProductRevisionID
	if rootRevision == "" || reference.ReferenceMode != "PINNED" {
		if err := service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`,
			reference.RootProductDocumentID).Scan(&rootRevision); err != nil {
			return "", "", InstancePose{}, nil, "", "", err
		}
	}
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1 AND document_id=$2`,
		rootRevision, reference.RootProductDocumentID).Scan(&raw); err != nil {
		return "", "", InstancePose{}, nil, "", "", err
	}
	var product ProductModel
	if err := json.Unmarshal(raw, &product); err != nil {
		return "", "", InstancePose{}, nil, "", "", err
	}
	matches := []ProductInstance{}
	for _, instance := range product.Instances {
		if instance.ReferencedDocumentID == reference.SourceDocumentID {
			matches = append(matches, instance)
		}
	}
	if len(matches) == 0 {
		return "", "", InstancePose{}, nil, "", "", fmt.Errorf("%w: CONTEXT_OCCURRENCE_MISSING", ErrValidation)
	}
	var selected *ProductInstance
	if reference.SourceInstancePath != nil && len(reference.SourceInstancePath.Segments) == 1 {
		for index := range matches {
			if matches[index].ID == reference.SourceInstancePath.Segments[0].InstanceID {
				selected = &matches[index]
				break
			}
		}
	} else if len(matches) == 1 {
		selected = &matches[0]
	}
	if selected == nil {
		return "", "", InstancePose{}, nil, "", "", fmt.Errorf("%w: CONTEXT_OCCURRENCE_AMBIGUOUS", ErrValidation)
	}
	if len(matches) > 1 && strings.TrimSpace(reference.ContextVariantID) == "" {
		return "", "", InstancePose{}, nil, "", "", fmt.Errorf("%w: CONTEXT_VARIANT_REQUIRED", ErrValidation)
	}
	ownerPose := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	owners := []ProductInstance{}
	for _, instance := range product.Instances {
		if instance.ReferencedDocumentID == consumerDocumentID {
			owners = append(owners, instance)
		}
	}
	if len(owners) > 0 {
		var owner *ProductInstance
		if reference.OwningInstancePath != nil && len(reference.OwningInstancePath.Segments) == 1 {
			for index := range owners {
				if owners[index].ID == reference.OwningInstancePath.Segments[0].InstanceID {
					owner = &owners[index]
					break
				}
			}
		} else if len(owners) == 1 {
			owner = &owners[0]
		}
		if owner == nil {
			return "", "", InstancePose{}, nil, "", "", fmt.Errorf("%w: CONTEXT_VARIANT_REQUIRED", ErrValidation)
		}
		ownerPose = InstancePose{Translation: owner.Translation, Rotation: normalizedInstanceRotation(owner.Rotation)}
	}
	path := appendInstancePath(InstancePath{RootDocumentID: reference.RootProductDocumentID}, InstancePathSegment{
		OwnerDocumentID: reference.RootProductDocumentID, OwnerVersionID: rootRevision, InstanceID: selected.ID,
		InstanceName: selected.Name, ReferencedDocumentID: selected.ReferencedDocumentID, ResolvedVersionID: selected.ReferencedVersionID})
	sourcePose := InstancePose{Translation: selected.Translation, Rotation: normalizedInstanceRotation(selected.Rotation)}
	return selected.ReferencedDocumentID, selected.ReferencedVersionID, inverseRelativePose(sourcePose, ownerPose), &path,
		rootRevision, resolvedDigest(json.RawMessage(raw)), nil
}

func (service *Service) resolveContextReference(ctx context.Context, consumerDocumentID string, reference ContextReference,
	consumer PartModel, forcedSourceRevision string) (ContextReference, error) {
	if reference.SourceDocumentID == consumerDocumentID {
		return ContextReference{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	sourceDocument, sourceRevision, pose, path, rootRevision, rootDigest, err := service.contextSource(ctx, consumerDocumentID, reference)
	if err != nil {
		return ContextReference{}, err
	}
	if forcedSourceRevision != "" {
		sourceRevision = forcedSourceRevision
	}
	if cycle, err := service.contextDocumentReaches(ctx, sourceDocument, sourceRevision, consumerDocumentID, map[string]bool{}, 0); err != nil {
		return ContextReference{}, err
	} else if cycle {
		return ContextReference{}, fmt.Errorf("%w: CONTEXT_REFERENCE_CYCLE", ErrValidation)
	}
	publication, err := service.publicationAtRevision(ctx, sourceDocument, sourceRevision, reference.Publication.PublicationID)
	if err != nil {
		return ContextReference{}, err
	}
	if reference.Publication.ExpectedType == "" {
		reference.Publication.ExpectedType, reference.Publication.CompatibilityVersion = publication.Type, publication.CompatibilityVersion
	}
	if !publicationReferenceCompatible(reference.Publication, publication) {
		return ContextReference{}, fmt.Errorf("%w: PUBLICATION_CONTRACT_INCOMPATIBLE", ErrValidation)
	}
	if publication.Target.PersistentSelection != nil {
		selection := *publication.Target.PersistentSelection
		reference.Publication.PersistentSelection = &selection
		reference.Publication.SelectionSourceVersionID = publication.Target.SourceVersionID
	}
	if publication.Resolution.Status != "CONNECTED" {
		return ContextReference{}, fmt.Errorf("%w: BROKEN_PUBLICATION", ErrValidation)
	}
	reference.SourceDocumentID, reference.ResolvedRevisionID = sourceDocument, sourceRevision
	reference.RootProductRevisionID, reference.RootProductSnapshotDigest = rootRevision, rootDigest
	reference.SourceInstancePath, reference.Transform = path, pose
	reference.Resolution = publication.Resolution
	reference.Resolution.Origin = pointByPose(pose, publication.Resolution.Origin)
	reference.Resolution.XDirection = rotateByPose(pose, publication.Resolution.XDirection)
	reference.Resolution.YDirection = rotateByPose(pose, publication.Resolution.YDirection)
	reference.Resolution.ZDirection = rotateByPose(pose, publication.Resolution.ZDirection)
	if reference.OwningWorkspace == "" {
		reference.OwningWorkspace = "main"
	}
	if reference.ReferenceMode == "" {
		reference.ReferenceMode = "FOLLOW_HEAD"
	}
	return reference, nil
}

func (service *Service) contextDocumentReaches(ctx context.Context, documentID, revisionID, target string,
	visited map[string]bool, depth int) (bool, error) {
	if documentID == target {
		return true, nil
	}
	if depth > 64 {
		return false, fmt.Errorf("%w: CONTEXT_REFERENCE_GRAPH_TOO_DEEP", ErrValidation)
	}
	key := documentID + "@" + revisionID
	if visited[key] {
		return false, nil
	}
	visited[key] = true
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE id=$1 AND document_id=$2`,
		revisionID, documentID).Scan(&raw); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var model PartModel
	if err := json.Unmarshal(raw, &model); err != nil {
		return false, nil
	}
	for _, reference := range model.ContextReferences {
		if reference.ReferenceMode == "ISOLATED" {
			continue
		}
		if reference.SourceDocumentID == target {
			return true, nil
		}
		if reaches, err := service.contextDocumentReaches(ctx, reference.SourceDocumentID, reference.ResolvedRevisionID, target, visited, depth+1); err != nil || reaches {
			return reaches, err
		}
	}
	for _, parameter := range model.Parameters {
		if external := parameter.Source.External; external != nil {
			if external.SourceDocumentID == target {
				return true, nil
			}
			if reaches, err := service.contextDocumentReaches(ctx, external.SourceDocumentID, external.ResolvedRevisionID, target, visited, depth+1); err != nil || reaches {
				return reaches, err
			}
		}
	}
	return false, nil
}

func materializeContextReference(model *PartModel, reference *ContextReference, parameterID string) error {
	typeName := reference.Publication.ExpectedType
	switch typeName {
	case "PLANE":
		id := reference.LocalTargetID
		if id == "" {
			id = "context-plane-" + reference.ID
		}
		reference.LocalTargetID = id
		plane := DatumPlane{ID: id, Name: reference.Name, Plane: "CUSTOM", Origin: reference.Resolution.Origin,
			Normal: reference.Resolution.ZDirection, UDirection: reference.Resolution.XDirection, Size: 200}
		found := false
		for index := range model.DatumPlanes {
			if model.DatumPlanes[index].ID == id {
				model.DatumPlanes[index] = plane
				found = true
			}
		}
		if !found {
			model.DatumPlanes = append(model.DatumPlanes, plane)
		}
	case "AXIS":
		id := reference.LocalTargetID
		if id == "" {
			id = "context-axis-" + reference.ID
		}
		reference.LocalTargetID = id
		axis := DatumAxis{ID: id, Name: reference.Name, Origin: reference.Resolution.Origin, Direction: reference.Resolution.ZDirection}
		found := false
		for index := range model.DatumAxes {
			if model.DatumAxes[index].ID == id {
				model.DatumAxes[index] = axis
				found = true
			}
		}
		if !found {
			model.DatumAxes = append(model.DatumAxes, axis)
		}
	case "PARAMETER":
		if parameterID == "" {
			parameterID = reference.LocalTargetID
		}
		if parameterID == "" {
			return fmt.Errorf("%w: PARAMETER context requires a local parameter", ErrValidation)
		}
		reference.LocalTargetID = parameterID
	case "CURVE":
		if reference.LocalTargetID == "" {
			return fmt.Errorf("%w: CURVE context requires a target sketch", ErrValidation)
		}
		if reference.Publication.PersistentSelection == nil {
			return fmt.Errorf("%w: CURVE context has no PersistentSelection", ErrValidation)
		}
		for featureIndex := range model.Features {
			feature := &model.Features[featureIndex]
			if feature.ID != reference.LocalTargetID || feature.Sketch == nil {
				continue
			}
			externalID := "context-external-" + reference.ID
			external := SketchExternalGeometry{ID: externalID, ProjectionKind: "ORTHOGONAL",
				PersistentSelection: *reference.Publication.PersistentSelection,
				SourceVersionID:     reference.Publication.SelectionSourceVersionID,
				SourceDocumentID:    reference.SourceDocumentID, ContextReferenceID: reference.ID, Status: "PENDING"}
			for index := range feature.Sketch.ExternalGeometry {
				if feature.Sketch.ExternalGeometry[index].ID == externalID {
					feature.Sketch.ExternalGeometry[index] = external
					goto stored
				}
			}
			feature.Sketch.ExternalGeometry = append(feature.Sketch.ExternalGeometry, external)
		stored:
			for index := range model.ContextReferences {
				if model.ContextReferences[index].ID == reference.ID {
					model.ContextReferences[index] = *reference
					return nil
				}
			}
			model.ContextReferences = append(model.ContextReferences, *reference)
			return nil
		}
		return fmt.Errorf("%w: target sketch for CURVE context does not exist", ErrValidation)
	default:
		// CURVE/SURFACE/BODY are retained as exact frozen Publication descriptors.
		// Their feature adapters consume the ContextReference instead of inventing
		// a local OCCT identity.
	}
	for index := range model.ContextReferences {
		if model.ContextReferences[index].ID == reference.ID {
			model.ContextReferences[index] = *reference
			return nil
		}
	}
	model.ContextReferences = append(model.ContextReferences, *reference)
	return nil
}

func isolateContextCurve(model *PartModel, reference ContextReference) error {
	if reference.Publication.ExpectedType != "CURVE" {
		return nil
	}
	externalID := "context-external-" + reference.ID
	for featureIndex := range model.Features {
		feature := &model.Features[featureIndex]
		if feature.ID != reference.LocalTargetID || feature.Sketch == nil {
			continue
		}
		for externalIndex, external := range feature.Sketch.ExternalGeometry {
			if external.ID != externalID {
				continue
			}
			if external.Status != "CONNECTED" || external.Snapshot == nil {
				return fmt.Errorf("%w: unresolved context curve cannot be isolated", ErrValidation)
			}
			snapshot := external.Snapshot
			entity := SketchEntity{ID: external.ID, Kind: snapshot.Kind, Role: "CONSTRUCTION", Point: snapshot.Point,
				Start: snapshot.Start, End: snapshot.End, Center: snapshot.Center, Radius: snapshot.Radius}
			feature.Sketch.Entities = append(feature.Sketch.Entities, entity)
			feature.Sketch.ExternalGeometry = append(feature.Sketch.ExternalGeometry[:externalIndex], feature.Sketch.ExternalGeometry[externalIndex+1:]...)
			for constraintIndex := range feature.Sketch.Constraints {
				for refIndex := range feature.Sketch.Constraints[constraintIndex].References {
					geometryRef := &feature.Sketch.Constraints[constraintIndex].References[refIndex]
					if geometryRef.Target == "EXTERNAL" && geometryRef.EntityID == external.ID {
						geometryRef.Target = "ENTITY"
					}
				}
			}
			return nil
		}
	}
	return fmt.Errorf("%w: context curve target no longer exists", ErrValidation)
}

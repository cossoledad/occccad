package workspace

import (
	"context"
	"encoding/json"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"time"
)

type documentPreviewInput struct {
	documentID     string
	prepared       preparedDomainMutation
	request        CommandRequest
	nextJSON       json.RawMessage
	previewChanges modelcore.ChangeSet
	diagnostic     *OperationDiagnostic
}

func previewPartDocument(ctx context.Context, service *Service, input documentPreviewInput) (CommandPreview, *PartModel, error) {
	documentID, prepared, request, nextJSON, previewChanges, diagnostic := input.documentID, input.prepared, input.request, input.nextJSON, input.previewChanges, input.diagnostic
	var diagnosticModel *PartModel
	var err error
	result, failure := func() (CommandPreview, error) {

		var model PartModel
		var beforeModel PartModel
		if err = json.Unmarshal(nextJSON, &model); err != nil {
			return CommandPreview{}, err
		}
		if err = json.Unmarshal(prepared.modelJSON, &beforeModel); err != nil {
			return CommandPreview{}, err
		}
		diagnosticModel = &model
		diagnostic.stage("PARAMETERS")
		normalizePartModel(&model)
		normalizePartModel(&beforeModel)
		if err = resolveRuntimeParameters(ctx, &model); err != nil {
			return CommandPreview{}, err
		}
		diagnostic.stage("SKETCH_INPUTS")
		finishSolve := perf.Start(ctx, "support-resolve-sketch-solve")
		if err = service.resolveAndSolveSketches(ctx, documentID, "preview/"+prepared.requestID, &model); err != nil {
			finishSolve()
			return CommandPreview{}, err
		}
		finishSolve()
		if err = resolveRuntimeParameters(ctx, &model); err != nil {
			return CommandPreview{}, err
		}
		if err = rejectExplicitUnresolvedExternal(prepared.command, model); err != nil {
			return CommandPreview{}, err
		}
		if err = service.resolvePartPublications(ctx, documentID, "preview/"+prepared.requestID,
			prepared.headRevision, &model); err != nil {
			return CommandPreview{}, err
		}
		if err = rejectExplicitBrokenPublication(prepared.command, model); err != nil {
			return CommandPreview{}, err
		}
		previewChanges = appendEvaluatedDatumChanges(appendEvaluatedSketchChanges(previewChanges, beforeModel, model), beforeModel, model)
		nextJSON, _ = json.Marshal(model)
		previewChanges, err = reconcilePersistedChanges(prepared.documentType, prepared.modelJSON, nextJSON, previewChanges)
		if err != nil {
			return CommandPreview{}, err
		}
		modelHash := canonicalModelHash(nextJSON)
		if _, broken := firstUnresolvedExternal(model); broken {
			previewID := newID("preview")
			service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
				headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
				payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, changes: previewChanges,
				expiresAt: time.Now().Add(interactionCandidateTTL)})
			return CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence, ModelHash: modelHash}, nil
		}
		diagnostic.stage("GEOMETRY")
		finishGeometry := perf.Start(ctx, "geometry-evaluate")
		bodyID := previewBodyID(prepared.command.Payload, model)
		geometryKey, err := service.evaluateBodyPrefix(ctx, "preview/"+prepared.requestID, model, bodyID, true)
		finishGeometry()
		if err != nil {
			return CommandPreview{}, err
		}
		diagnostic.stage("ARTIFACT")
		finishArtifact := perf.Start(ctx, "artifact-load")
		artifact, err := service.loadArtifact(ctx, geometryKey)
		finishArtifact()
		if err != nil {
			return CommandPreview{}, err
		}
		artifact.BodyID = bodyID
		bodyName := ""
		bodyAssignment := "TARGET_BODY"
		if index := bodyIndex(model, bodyID); index >= 0 {
			model.Bodies[index].GeometryKey = geometryKey
			bodyName = model.Bodies[index].Name
			if bodyIndex(beforeModel, bodyID) < 0 {
				bodyAssignment = "EXPLICIT_NEW_BODY"
			}
		}
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		previewID := newID("preview")
		service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
			headRevision: prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
			payloadDigest: modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, geometryKey: geometryKey, visualObjectID: artifact.Representations["VISUAL"].ObjectID,
			changes: previewChanges, expiresAt: time.Now().Add(interactionCandidateTTL)})
		artifact.RepresentationKind = "TRANSIENT_PREVIEW"
		var loftSections []LoftSection
		var loftConnections [][][3]float64
		if request.Feature != nil && request.Feature.Type == "LOFT" {
			target := request.TargetID
			if target == "" {
				target = commandEntityID("loft", request.RequestID)
			}
			earlier := map[string]Feature{}
			for _, f := range model.Features {
				if f.ID == target {
					loftSections = f.Sections
					inputs, e := service.loftGeometrySections(ctx, request.RequestID+"/connections", model, f, earlier)
					if e != nil {
						return CommandPreview{}, e
					}
					resolved, e := service.worker.ResolveLoftCorrespondence(ctx, request.RequestID+"/connections", inputs)
					if e != nil {
						return CommandPreview{}, e
					}
					if len(resolved) > 0 {
						for j := range resolved[0].BoundaryPoints {
							line := make([][3]float64, len(resolved))
							for i := range resolved {
								line[i] = resolved[i].BoundaryPoints[j]
							}
							loftConnections = append(loftConnections, line)
						}
					}
					break
				}
				earlier[f.ID] = f
			}
		}
		return CommandPreview{
			LoftSections: loftSections, LoftConnections: loftConnections,
			ReferenceGeometry: datumPreviewReferences(prepared.command, model), ParameterCandidates: parameterPreviewCandidates(prepared.command, model),
			PreviewID: previewID, BaseVersionID: prepared.headRevision,
			BaseSequence: prepared.headSequence, ModelHash: modelHash, Artifact: &artifact,
			ResultBodyID: bodyID, ResultBodyName: bodyName, BodyAssignment: bodyAssignment,
			SketchCandidates: solvedSketchPreviewCandidates(prepared.command, model),
		}, nil
	}()
	return result, diagnosticModel, failure
}
func previewProductDocument(ctx context.Context, service *Service, input documentPreviewInput) (CommandPreview, *PartModel, error) {
	documentID, prepared, request, nextJSON, previewChanges := input.documentID, input.prepared, input.request, input.nextJSON, input.previewChanges
	var diagnosticModel *PartModel
	var err error
	result, failure := func() (CommandPreview, error) {
		var model ProductModel
		if err = json.Unmarshal(nextJSON, &model); err != nil {
			return CommandPreview{}, err
		}
		driven := ""
		var solveIntent *geometry.AssemblySolveIntent
		if prepared.command.TypeURI == typeMoveInstance {
			var payload moveInstancePayload
			_ = json.Unmarshal(prepared.command.Payload, &payload)
			driven = payload.InstanceID
		} else {
			solveIntent = assemblyConstraintSolveIntent(prepared.command, model)
		}
		var assemblyResult geometry.AssemblySolve
		retainedFailure := false
		warmStartKey := ""
		if request.InteractionID != "" {
			warmStartKey = documentID + "|" + prepared.actorID + "|" + request.InteractionID
		}
		if err = service.solveAssembly(ctx, documentID, prepared.headRevision, "preview/"+prepared.requestID, driven, solveIntent, &model, warmStartKey, &assemblyResult); err != nil {
			if retained := acceptAssemblyEvaluationFailure(&model, err, retainsAssemblyDefinition(prepared.command.TypeURI)); retained == nil {
				retainedFailure = true
			} else {
				return CommandPreview{}, err
			}
		}
		nextJSON, err = json.Marshal(model)
		if err != nil {
			return CommandPreview{}, err
		}
		constraintID := ""
		switch prepared.command.TypeURI {
		case typeAddAssemblyConstraint:
			var payload addAssemblyConstraintPayload
			if json.Unmarshal(prepared.command.Payload, &payload) == nil {
				constraintID = payload.Constraint.ID
			}
		case typeEditAssemblyConstraint:
			var payload editAssemblyConstraintPayload
			if json.Unmarshal(prepared.command.Payload, &payload) == nil {
				constraintID = payload.ConstraintID
			}
		}
		unverifiedOffset := false
		for _, constraint := range model.Constraints {
			if constraint.ID == constraintID && constraint.Kind == "DISTANCE" && constraint.EvaluationStatus != modelcore.AssemblyConstraintVerified {
				// Admission may solve the accepted subset successfully while the
				// edited Offset is isolated as NotUpdated/Broken. That is not a
				// successful candidate for this definition, even if solve converged.
				unverifiedOffset = true
			}
		}
		previewID := newID("preview")
		if !retainedFailure && (assemblyResult.Status != "CONVERGED" || unverifiedOffset) {
			previewID = ""
		}
		if previewID != "" {
			service.interactionCandidates.put(interactionCandidate{id: previewID, documentID: documentID, actorID: prepared.actorID,
				definitionOnly: retainedFailure,
				headRevision:   prepared.headRevision, headSequence: prepared.headSequence, commandType: prepared.command.TypeURI,
				assemblyPreviewRequestID: "preview/" + prepared.requestID,
				payloadDigest:            modelcore.ValueDigest(prepared.command.Payload), intentPayload: prepared.command.Payload, nextJSON: nextJSON, changes: previewChanges,
				expiresAt: time.Now().Add(interactionCandidateTTL)})
		}
		result := CommandPreview{PreviewID: previewID, BaseVersionID: prepared.headRevision, BaseSequence: prepared.headSequence,
			ModelHash: canonicalModelHash(nextJSON), AssemblyComponents: assemblyResult.Components, AssemblySolverBuild: assemblyResult.SolverBuild}
		if retainedFailure {
			result.EvaluationOutcome = "DEFINITION_ONLY"
			result.AssemblyComponents = nil
		}
		for _, constraint := range model.Constraints {
			if constraint.ID != constraintID {
				continue
			}
			preview := AssemblyConstraintPreviewEvaluation{ConstraintID: constraint.ID, Status: constraint.EvaluationStatus,
				Summary: constraint.EvaluationSummary, First: assemblySupportPreview(constraint.First), Failure: constraint.EvaluationFailure}
			if constraint.Second != nil {
				second := assemblySupportPreview(*constraint.Second)
				preview.Second = &second
			}
			result.ConstraintEvaluation = &preview
			if constraint.EvaluationFailure != nil {
				result.EvaluationFailure = constraint.EvaluationFailure
			}
			break
		}
		for _, instance := range model.Instances {
			result.InstancePoses = append(result.InstancePoses, struct {
				InstanceID  string     `json:"instanceId"`
				Translation [3]float64 `json:"translation"`
				Rotation    [4]float64 `json:"rotation"`
			}{instance.ID, instance.Translation, normalizedInstanceRotation(instance.Rotation)})
		}
		return result, nil
	}()
	return result, diagnosticModel, failure
}

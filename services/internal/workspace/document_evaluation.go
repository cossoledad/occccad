package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
)

type documentEvaluationInput struct {
	documentID, revisionID string
	prepared               preparedDomainMutation
	request                CommandRequest
	candidate              interactionCandidate
	promoted               bool
	nextJSON               json.RawMessage
	changes                modelcore.ChangeSet
	diagnostic             *OperationDiagnostic
}
type documentEvaluationResult struct {
	nextJSON                        json.RawMessage
	modelHash                       string
	graph                           *modelcore.DependencyGraph
	manifest                        modelcore.EvaluationManifest
	changes                         modelcore.ChangeSet
	revisionState, evaluationStatus string
	diagnosticModel                 *PartModel
}

func evaluatePartDocument(ctx context.Context, service *Service, input documentEvaluationInput) (result documentEvaluationResult, err error) {
	documentID, revisionID, prepared, promoted, nextJSON, changes, diagnostic := input.documentID, input.revisionID, input.prepared, input.promoted, input.nextJSON, input.changes, input.diagnostic
	modelHash := canonicalModelHash(nextJSON)
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var diagnosticModel *PartModel
	revisionState, evaluationStatus := "READY", "SUCCEEDED"
	err = func() error {
		var model PartModel
		var beforeModel PartModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		if err := json.Unmarshal(prepared.modelJSON, &beforeModel); err != nil {
			return err
		}
		diagnosticModel = &model
		normalizePartModel(&model)
		normalizePartModel(&beforeModel)
		metadataOnly := isPartMetadataCommand(prepared.command)
		if !promoted && !metadataOnly {
			if err := resolveRuntimeParameters(ctx, &model); err != nil {
				return err
			}
			diagnostic.stage("SKETCH_INPUTS")
			finishSolve := perf.Start(ctx, "support-resolve-sketch-solve")
			if err := service.resolveAndSolveSketches(ctx, documentID, prepared.requestID, &model); err != nil {
				finishSolve()
				return err
			}
			finishSolve()
		}
		if !metadataOnly {
			if err := resolveRuntimeParameters(ctx, &model); err != nil {
				return err
			}
		}
		if err := rejectExplicitBrokenSupport(prepared.command, model); err != nil {
			return err
		}
		if err := rejectExplicitUnresolvedExternal(prepared.command, model); err != nil {
			return err
		}
		if !metadataOnly {
			if err := service.resolvePartPublications(ctx, documentID, prepared.requestID, revisionID, &model); err != nil {
				return err
			}
		}
		if err := rejectExplicitBrokenPublication(prepared.command, model); err != nil {
			return err
		}
		if !metadataOnly {
			changes = appendEvaluatedDatumChanges(appendEvaluatedSketchChanges(changes, beforeModel, model), beforeModel, model)
		}
		if metadataOnly {
			if partHasFailedFeature(model) {
				revisionState, evaluationStatus = "FAILED", "FAILED"
			}
			// Display metadata and readable names do not change geometry inputs.
			// Keep the frozen per-Body results without entering the evaluator.
			for i := range model.Bodies {
				for _, previous := range beforeModel.Bodies {
					if previous.ID == model.Bodies[i].ID {
						model.Bodies[i].GeometryKey = previous.GeometryKey
						break
					}
				}
			}
		} else {
			diagnostic.stage("GEOMETRY")
			// An explicit region or modifier edit must resolve all required inputs.
			// Upstream/parameter changes may still create inspectable FAILED
			// revisions; accepting such a revision is not a repair confirmation.
			if !promoted && prepared.command.TypeURI == typeEditFeature {
				var edit editFeaturePayload
				if err := json.Unmarshal(prepared.command.Payload, &edit); err != nil {
					return err
				}
				for _, feature := range model.Features {
					if feature.ID == edit.FeatureID && feature.Type == "HOLE" && !feature.Suppressed {
						if _, err := holeInput(model, feature); err != nil {
							return err
						}
					}
					if feature.ID == edit.FeatureID && feature.Extent == "TO_PLANE" && !feature.Suppressed {
						if _, _, err := service.resolveReferencedPlane(ctx, prepared.requestID+"/validate-termination", model, feature.ID, feature.TerminationPlaneID, feature.TerminationPlane); err != nil {
							return err
						}
					}

					if feature.ID == edit.FeatureID && isSolidGenerator(feature.Type) && feature.ProfileSelection != nil && feature.ProfileSelection.Mode == "REGIONS" && !feature.Suppressed {
						if _, _, err := service.featureProfileRegions(ctx, model, feature, prepared.requestID+"/validate-profile"); err != nil {
							return err
						}
					}
					if feature.ID == edit.FeatureID && isLocalModifier(feature.Type) && !feature.Suppressed {
						if _, err := service.modifierInput(ctx, prepared.requestID+"/validate-input", model, feature); err != nil {
							return featureInputStageError(model, feature.ID, err)
						}
						break
					}
				}
			}
			finishGeometry := perf.Start(ctx, "geometry-evaluate")
			err = service.evaluatePartBodies(ctx, prepared.requestID, &model)
			finishGeometry()
			if err != nil {
				var failed *solidEvaluationFailure
				if !errors.As(err, &failed) || (prepared.command.TypeURI != typeEditFeature && prepared.command.TypeURI != typeSetFeatureSuppression && prepared.command.TypeURI != typeSetParameterLiteral && prepared.command.TypeURI != typeSetParameterExpression && prepared.command.TypeURI != typeEditParameter && prepared.command.TypeURI != typeEditSketch && prepared.command.TypeURI != typeEditDatum) {
					return err
				}
				revisionState, evaluationStatus = "FAILED", "FAILED"
			}
		}
		retainFailedBodyDisplay(&model, beforeModel, prepared.headRevision)
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		graph, manifest, err = buildPartEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		attachEvaluationRuntime(ctx, &manifest)
		if err != nil {
			return err
		}
		return nil
	}()
	return documentEvaluationResult{nextJSON, modelHash, graph, manifest, changes, revisionState, evaluationStatus, diagnosticModel}, err
}
func evaluateProductDocument(ctx context.Context, service *Service, input documentEvaluationInput) (result documentEvaluationResult, err error) {
	documentID, revisionID, prepared, request, candidate, promoted, nextJSON, changes := input.documentID, input.revisionID, input.prepared, input.request, input.candidate, input.promoted, input.nextJSON, input.changes
	modelHash := canonicalModelHash(nextJSON)
	var graph *modelcore.DependencyGraph
	var manifest modelcore.EvaluationManifest
	var diagnosticModel *PartModel
	revisionState, evaluationStatus := "READY", "SUCCEEDED"
	err = func() error {
		var model ProductModel
		if err := json.Unmarshal(nextJSON, &model); err != nil {
			return err
		}
		if prepared.command.TypeURI == typeUpdateReferences {
			for index := range model.ContextBindings {
				model.ContextBindings[index].Accepted.RootProductRevisionID = revisionID
				if len(model.ContextBindings[index].SourceInstancePath.Segments) > 0 {
					model.ContextBindings[index].SourceInstancePath.Segments[0].OwnerVersionID = revisionID
				}
				if len(model.ContextBindings[index].OwningInstancePath.Segments) > 0 {
					model.ContextBindings[index].OwningInstancePath.Segments[0].OwnerVersionID = revisionID
				}
			}
		}
		if promoted && !candidate.definitionOnly && prepared.command.TypeURI != typeSetKinematics && (len(model.Constraints) > 0 || request.SessionID != "") {
			if err = service.promoteAssemblySolveManifest(ctx, documentID, revisionID, prepared.requestID, candidate.assemblyPreviewRequestID, canonicalModelHash(nextJSON)); err != nil {
				return err
			}
		}
		if !promoted && prepared.command.TypeURI != typeOccurrenceVisibility && prepared.command.TypeURI != typeConstraintVisibility && prepared.command.TypeURI != typeSetKinematics && prepared.command.TypeURI != typeSetProductProperties {
			finishSolve := perf.Start(ctx, "assembly-solve")
			drivenInstanceID := ""
			var solveIntent *geometry.AssemblySolveIntent
			if prepared.command.TypeURI == typeMoveInstance {
				var payload moveInstancePayload
				_ = json.Unmarshal(prepared.command.Payload, &payload)
				drivenInstanceID = payload.InstanceID
			} else {
				solveIntent = assemblyConstraintSolveIntent(prepared.command, model)
			}
			if err = service.solveAssembly(ctx, documentID, revisionID, prepared.requestID, drivenInstanceID, solveIntent, &model, ""); err != nil {
				allowFailure := retainsAssemblyDefinition(prepared.command.TypeURI) || prepared.command.TypeURI == typeUpdateReferences || prepared.command.TypeURI == typeReplaceInstance
				if err = acceptAssemblyEvaluationFailure(&model, err, allowFailure); err != nil {
					finishSolve()
					return err
				}
			}
			finishSolve()
			if prepared.command.TypeURI == typeApplyMotionFrame || prepared.command.TypeURI == typeAdoptMechanismPose {
				var p motionApplyPayload
				_ = json.Unmarshal(prepared.command.Payload, &p)
				for _, v := range p.Model.Instances {
					i := slices.IndexFunc(model.Instances, func(n ProductInstance) bool { return n.ID == v.ID })
					if i < 0 || !conflictPoseClose(geometry.AssemblyPose{Translation: model.Instances[i].Translation, Rotation: model.Instances[i].Rotation}, geometry.AssemblyPose{Translation: v.Translation, Rotation: v.Rotation}, 1e-7, 1e-8) {
						return fmt.Errorf("%w: formal assembly evaluation changed selected motion frame", ErrValidation)
					}
				}
			}
		}
		nextJSON, _ = json.Marshal(model)
		modelHash = canonicalModelHash(nextJSON)
		var priorProduct ProductModel
		_ = json.Unmarshal(prepared.modelJSON, &priorProduct)
		changes = appendAssemblyEvaluationChanges(changes, priorProduct, model)
		graph, manifest, err = buildProductEvaluation(model, revisionID, modelHash, changes.ImpactSeeds, prepared.priorManifest)
		if err != nil {
			return err
		}
		return nil
	}()
	return documentEvaluationResult{nextJSON, modelHash, graph, manifest, changes, revisionState, evaluationStatus, diagnosticModel}, err
}

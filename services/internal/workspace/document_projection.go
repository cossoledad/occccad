package workspace

import (
	"context"
	"encoding/json"
	"fmt"

	perf "github.com/occccad/occccad/internal/performance"
	"strings"
)

func projectPartDocument(ctx context.Context, service *Service, summary DocumentSummary, modelJSON []byte) (DocumentView, error) {
	ctx = withAssemblyReadCache(ctx)
	view := DocumentView{Document: summary}
	var err error
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return view, err
	}
	normalizePartModel(&model)
	presentParameters(&model)
	view.Part = &model
	view.BodyAppearances = map[string]ResolvedAppearance{}
	for _, body := range model.Bodies {
		appearance, err := service.bodyAppearanceWithDefaults(ctx, body, model.DefaultAppearance)
		if err != nil {
			return view, err
		}
		if body.Appearance != nil || model.DefaultAppearance != nil {
			view.BodyAppearances[body.ID] = appearance
		}
	}
	projectPartCollections(&view, model)
	view.SketchAnalyses = service.projectExactSketchAnalyses(ctx, model)
	view.ReferenceUpdates = service.projectPartReferenceUpdates(ctx, model)
	view.DatumPlanes = model.DatumPlanes
	view.AxisSystems = model.AxisSystems
	view.DatumAxes = model.DatumAxes
	view.Artifacts, err = service.bodyArtifacts(ctx, model)
	if err != nil {
		return view, err
	}
	finishStructure := perf.Start(ctx, "structure-project")
	structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
		"document:"+summary.ID, summary.Name, InstancePath{RootDocumentID: summary.ID}, map[string]bool{})
	finishStructure()
	if err != nil {
		return view, err
	}
	bindStructureReferenceCurrency(&structure, view.ReferenceUpdates)
	view.StructureTree = &structure
	return view, nil
}
func projectProductDocument(ctx context.Context, service *Service, summary DocumentSummary, modelJSON []byte) (DocumentView, error) {
	ctx = withAssemblyReadCache(ctx)
	view := DocumentView{Document: summary}
	var err error

	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return view, err
	}
	for index := range model.Instances {
		instance := &model.Instances[index]
		if instance.ReferenceMode == "" {
			instance.ReferenceMode = "FOLLOW_HEAD"
		}
		instance.ResolvedVersionID = instance.ReferencedVersionID
		if instance.ReferenceMode == "FOLLOW_HEAD" || instance.ReferenceMode == "FOLLOW_WORKSPACE_WITH_ACCEPT" {
			var head string
			if err := service.database.QueryRow(ctx,
				`SELECT head_version_id::text FROM occccad.documents WHERE id=$1`,
				instance.ReferencedDocumentID).Scan(&head); err != nil {
				return view, err
			}
			instance.HeadChanged = head != instance.ReferencedVersionID
		}
	}
	// Availability of a newer Head does not invalidate evidence for the accepted Revision.
	// Keep currency on instances and UpdatePlan, separate from constraint evaluation.
	view.Product = &model
	if err := validateAssemblyDefinitionFormat(model); err != nil {
		return view, err
	}
	view.FollowedDocumentIDs, view.FollowedProductIDs, err = service.followedProductDocuments(ctx, model)
	if err != nil {
		return view, err
	}
	view.Artifacts = map[string]Artifact{}
	view.ResolvedInstances = []ResolvedInstance{}
	ctx, occurrenceRules := withOccurrenceRules(ctx)
	if err := service.resolveProduct(ctx, summary.VersionID, InstancePose{Rotation: [4]float64{0, 0, 0, 1}}, summary.Name,
		InstancePath{RootDocumentID: summary.ID}, "document:"+summary.ID,
		map[string]bool{}, view.Artifacts, &view.ResolvedInstances, &view.ConstraintDisplayScopes); err != nil {
		return view, err
	}
	items, err := service.expandProductContext(ctx, summary.ID, summary.VersionID)
	if err != nil {
		return view, err
	}
	view.ContextVariants, err = service.acceptedContextVariants(ctx, items)
	if err != nil {
		return view, err
	}
	for _, variant := range view.ContextVariants {
		if variant.Status != "READY" {
			continue
		}
		for _, body := range variant.Bodies {
			if body.GeometryKey == "" || body.Consumed {
				continue
			}
			if _, exists := view.Artifacts[body.GeometryKey]; !exists {
				artifact, err := service.loadArtifact(ctx, body.GeometryKey)
				if err != nil {
					return view, err
				}
				artifact.BodyID = body.ID
				view.Artifacts[body.GeometryKey] = artifact
			}
			for i := range view.ResolvedInstances {
				if view.ResolvedInstances[i].OccurrencePath == variant.OwningInstancePath.Canonical && view.ResolvedInstances[i].BodyID == body.ID {
					view.ResolvedInstances[i].GeometryKey = body.GeometryKey
					resolved := &view.ResolvedInstances[i]
					version := resolved.InstancePath.Segments[len(resolved.InstancePath.Segments)-1].ResolvedVersionID
					appearance, err := service.appearanceAtGeometry(ctx, resolved.DocumentID, version, body.ID, body.GeometryKey)
					if err != nil {
						return view, err
					}
					resolved.Appearance = appearance
					resolved.Appearance, err = service.applyOccurrenceAppearance(ctx, resolved.DocumentID, body, appearance, resolved.OccurrencePath, occurrenceRules[resolved.OccurrencePath])
					if err != nil {
						return view, err
					}
				}
			}
		}
	}
	structure, err := service.buildDocumentStructure(ctx, summary.VersionID,
		"document:"+summary.ID, summary.Name, InstancePath{RootDocumentID: summary.ID}, map[string]bool{})
	if err != nil {
		return view, err
	}
	bindStructureVariants(&structure, view.ContextVariants)
	view.StructureTree = &structure
	projectResolvedOccurrenceNames(view.ResolvedInstances, occurrenceRules)
	return view, nil
}

func projectPartStructure(ctx context.Context, service *Service, root DocumentStructureNode, modelJSON []byte, occurrenceIdentity InstancePath, visiting map[string]bool) (DocumentStructureNode, error) {
	documentID, versionID, path := root.DocumentID, root.VersionID, root.ID
	var model PartModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return DocumentStructureNode{}, err
	}
	normalizePartModel(&model)
	// Capabilities describe domain operations, not authorization or the
	// browser's current edit context. The client enables them only after
	// loading the referenced document and proving Editor access.
	root.Children = partStructureChildren(model, path, documentID, versionID, true)
	rootPath := root.InstancePath
	applyInstancePath(root.Children, rootPath)
	annotateStructure(&root, documentID, "")
	return root, nil
}

func projectProductStructure(ctx context.Context, service *Service, root DocumentStructureNode, modelJSON []byte, occurrenceIdentity InstancePath, visiting map[string]bool) (DocumentStructureNode, error) {
	documentID, versionID, path := root.DocumentID, root.VersionID, root.ID

	root.Capabilities = []string{"CREATE_PART"}
	visiting[documentID] = true
	defer delete(visiting, documentID)
	var model ProductModel
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return DocumentStructureNode{}, err
	}
	root.Children = make([]DocumentStructureNode, 0, len(model.Instances))
	for _, instance := range model.Instances {
		resolvedVersionID := instance.ReferencedVersionID
		mode := strings.ToUpper(instance.ReferenceMode)
		if mode == "" {
			mode = "FOLLOW_HEAD"
		}
		instanceNodePath := path + "/instance:" + instance.ID
		childIdentity := appendInstancePath(occurrenceIdentity, InstancePathSegment{
			OwnerDocumentID: documentID, OwnerVersionID: versionID, InstanceID: instance.ID,
			InstanceName: instance.Name, ReferencedDocumentID: instance.ReferencedDocumentID,
			ResolvedVersionID: resolvedVersionID,
		})
		reference, err := service.buildDocumentStructure(ctx, resolvedVersionID, instanceNodePath+"/reference",
			instance.Name, childIdentity, visiting)
		if err != nil {
			return DocumentStructureNode{}, err
		}
		instanceNode := DocumentStructureNode{
			ID: instanceNodePath, Kind: "INSTANCE", Name: fmt.Sprintf("%s(%s)", reference.ReferenceName, instance.Name),
			ReferenceName: reference.ReferenceName, InstanceName: instance.Name, EntityID: instance.ID,
			DocumentID: reference.DocumentID, DocumentType: reference.DocumentType,
			VersionID: resolvedVersionID, ReferenceMode: mode, InstancePath: &childIdentity,
			Children:        []DocumentStructureNode{reference},
			OwnerDocumentID: documentID, ConnectionStatus: "CONNECTED", CurrencyStatus: "CURRENT",
		}
		instanceNode.Capabilities = []string{"DELETE"}
		if mode == "PINNED" {
			instanceNode.Capabilities = append(instanceNode.Capabilities, "FOLLOW_HEAD")
		} else {
			instanceNode.Capabilities = append(instanceNode.Capabilities, "PIN_VERSION")
		}
		if reference.DocumentType == "PRODUCT" {
			instanceNode.Capabilities = append(instanceNode.Capabilities, "CREATE_PART")
		}
		if mode == "FOLLOW_HEAD" || mode == "FOLLOW_WORKSPACE_WITH_ACCEPT" {
			var head string
			if service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, instance.ReferencedDocumentID).Scan(&head) == nil && head != instance.ReferencedVersionID {
				instanceNode.CurrencyStatus = "UPDATE_AVAILABLE"
				instanceNode.Diagnostic = "NOT_UPDATED: referenced workspace has a newer revision"
				instanceNode.Capabilities = append(instanceNode.Capabilities, "UPDATE_REFERENCES")
			}
		}
		root.Children = append(root.Children, instanceNode)
	}
	if len(model.Publications) > 0 {
		group := DocumentStructureNode{ID: path + "/publications", Kind: "PRODUCT_PUBLICATION_SET", Name: "Publications",
			DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
		for _, publication := range model.Publications {
			copy := publication
			sourceDocumentID := ""
			if segments := publication.Target.InstancePath.Segments; len(segments) > 0 {
				sourceDocumentID = segments[len(segments)-1].ReferencedDocumentID
			}
			diagnostic := ""
			if publication.Resolution.Status != "CONNECTED" {
				diagnostic = strings.TrimSpace(publication.Resolution.DiagnosticCode + ": " + publication.Resolution.Diagnostic)
			}
			group.Children = append(group.Children, DocumentStructureNode{ID: group.ID + "/publication:" + publication.ID,
				Kind: "PRODUCT_PUBLICATION", Name: publication.Name, EntityID: publication.ID, EntityType: publication.Type,
				DocumentID: documentID, VersionID: versionID, Diagnostic: diagnostic,
				ResolutionStatus:  publication.Resolution.Status,
				ConnectionStatus:  publication.Resolution.Status,
				SourceDocumentID:  sourceDocumentID,
				SourceRevisionID:  publication.Resolution.ResolvedVersionID,
				SourceDisplayPath: publication.Target.InstancePath.Display,
				Capabilities:      []string{"EDIT", "DELETE"}, ProductPublication: &copy})
		}
		root.Children = append(root.Children, group)
	}
	if len(model.ContextBindings) > 0 {
		group := DocumentStructureNode{ID: path + "/context-bindings", Kind: "CONTEXT_BINDING_SET", Name: "Context Bindings",
			DocumentID: documentID, VersionID: versionID, Children: []DocumentStructureNode{}}
		for _, binding := range model.ContextBindings {
			copy := binding
			sourceDocumentID := ""
			if segments := binding.SourceInstancePath.Segments; len(segments) > 0 {
				sourceDocumentID = segments[len(segments)-1].ReferencedDocumentID
			}
			diagnostic := ""
			if binding.Resolution.Status != "CONNECTED" {
				diagnostic = strings.TrimSpace(binding.Resolution.DiagnosticCode + ": " + binding.Resolution.Diagnostic)
			}
			group.Children = append(group.Children, DocumentStructureNode{ID: group.ID + "/binding:" + binding.ID,
				Kind: "CONTEXT_BINDING", Name: binding.Name, EntityID: binding.ID, EntityType: binding.Publication.ExpectedType,
				DocumentID: documentID, VersionID: versionID, Diagnostic: diagnostic,
				ResolutionStatus: binding.Resolution.Status, ConnectionStatus: binding.Resolution.Status,
				Capabilities:      []string{"DELETE", "REFRESH"},
				SourceDocumentID:  sourceDocumentID,
				SourceRevisionID:  binding.Accepted.SourceRevisionID,
				SourceDisplayPath: binding.SourceInstancePath.Display, ContextBinding: &copy})
		}
		root.Children = append(root.Children, group)
	}
	if len(model.Constraints) > 0 {
		group := DocumentStructureNode{ID: path + "/assembly-constraints", Kind: "ASSEMBLY_CONSTRAINT_SET", EntityID: "assembly-constraints", Name: "约束", DocumentID: documentID, LocalVisible: boolPointer(false), Capabilities: []string{"SUPPRESS"}, Suppressed: true}
		names := make(map[string]string, len(model.Instances))
		for _, instance := range model.Instances {
			names[instance.ID] = instance.Name
		}
		for _, constraint := range model.Constraints {
			if !constraint.Suppressed {
				group.Suppressed = false
			}
			if visibleOrDefault(constraint.Visible) {
				*group.LocalVisible = true
			}
			name := constraintDisplayName(constraint, func(id string) string { return names[id] }, model.Constraints...)
			status, summary := constraint.EvaluationStatus, constraint.EvaluationSummary
			if constraint.Suppressed {
				name += " [停用]"
			}
			capabilities := assemblyConstraintStructureCapabilities(constraint, status)
			node := DocumentStructureNode{ID: group.ID + "/constraint:" + constraint.ID,
				Kind: "ASSEMBLY_CONSTRAINT", LocalVisible: boolPointer(visibleOrDefault(constraint.Visible)), Suppressed: constraint.Suppressed, Name: name, EntityID: constraint.ID, EntityType: constraint.Kind,
				DocumentID: documentID, EvaluationStatus: string(status),
				Diagnostic: string(status) + ": " + summary, Capabilities: capabilities}
			if isAssemblyGroup(constraint) {
				node.Children = assemblyGroupReferenceNodes(node.ID, documentID, "", constraint, func(id string) string { return names[id] }, model.Constraints)
			}
			group.Children = append(group.Children, node)
		}
		root.Children = append(root.Children, group)
	}
	root.Children = append(root.Children, kinematicsStructure(documentID, versionID, path, model.Kinematics, model.Constraints, model.Instances))
	annotateStructure(&root, documentID, "")
	applyOccurrenceVisibilityProjection(&root, model.VisibilityOverrides)
	projectOccurrenceNames(&root, model, occurrenceIdentity.Canonical)
	return root, nil
}

// Value-dependent collections are projected for both independent and accepted views.
func projectPartCollections(view *DocumentView, model PartModel) {
	view.ReplicationSources = replicationSources(model)
	view.SketchPatternMembers = map[string][]SketchEntity{}
	for _, feature := range model.Features {
		if feature.Sketch == nil || len(feature.Sketch.Patterns) == 0 {
			continue
		}
		members, err := evaluatedSketchPatternEntities(*feature.Sketch)
		if err != nil {
			continue
		}
		patterns := map[string]bool{}
		for _, p := range feature.Sketch.Patterns {
			patterns[p.ID] = true
		}
		for _, member := range members {
			if patterns[member.CreatedByOperationID] {
				view.SketchPatternMembers[feature.ID] = append(view.SketchPatternMembers[feature.ID], member)
			}
		}
	}
}

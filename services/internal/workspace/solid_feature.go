package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

func isLocalModifier(kind string) bool {
	return kind == "FILLET" || kind == "CHAMFER" || kind == "DRAFT" || kind == "SHELL"
}

func isBodyFeature(kind string) bool {
	return kind == "SOLID_PATTERN" || kind == "LOFT" || isLocalModifier(kind) || isSolidGenerator(kind) || kind == "IMPORT_BODY" || kind == "BOOLEAN"
}

func validateSolidStage(f Feature, earlier map[string]Feature) error {
	if f.Type == "LOFT" {
		if len(f.Sections) < 2 || len(f.Sections) > 32 {
			return fmt.Errorf("%w: loft requires 2..32 sections", ErrValidation)
		}
		seen := map[string]bool{}
		profiles := 0
		for i, section := range f.Sections {
			if section.Point != nil {
				if i != 0 && i != len(f.Sections)-1 {
					return fmt.Errorf("%w: LOFT_POINT_MUST_BE_ENDPOINT", ErrValidation)
				}
				if section.MemberSlot != nil || section.Point.SketchID != section.SketchID || (section.Point.AxisEntityID != "" && section.SketchID != "") {
					return fmt.Errorf("%w: invalid loft point reference", ErrValidation)
				}
				if section.Point.AxisEntityID != "" {
					parts := strings.Split(section.Point.AxisEntityID, ":")
					if len(parts) < 2 || (parts[0] != "AXIS_SYSTEM" && parts[0] != "DATUM_AXIS") {
						return fmt.Errorf("%w: invalid loft datum point", ErrValidation)
					}
					continue
				}
			} else {
				profiles++
			}
			sketch, ok := earlier[section.SketchID]
			if !ok || (sketch.Sketch == nil && sketch.Type != "SKETCH_PATTERN") || seen[loftSectionIdentity(section)] || math.IsNaN(section.SeamAngle) || math.IsInf(section.SeamAngle, 0) {
				return fmt.Errorf("%w: invalid loft section", ErrValidation)
			}
			seen[loftSectionIdentity(section)] = true
		}
		if profiles == 0 {
			return fmt.Errorf("%w: LOFT_REQUIRES_PROFILE_SECTION", ErrValidation)
		}
		if f.Operation != "ADD" && f.Operation != "REMOVE" && f.Operation != "INTERSECT" && f.Operation != "NEW_BODY" {
			return fmt.Errorf("%w: invalid loft operation", ErrValidation)
		}
	}
	if isSolidGenerator(f.Type) {
		if f.Extent != "" && f.Extent != "FINITE" && f.Extent != "SYMMETRIC" && f.Extent != "TWO_SIDED" && f.Extent != "THROUGH_ALL" {
			return fmt.Errorf("%w: invalid extrude extent", ErrValidation)
		}
		if f.Extent == "THROUGH_ALL" && (f.Type == "REVOLVE" || f.Operation != "REMOVE") {
			return fmt.Errorf("%w: through all requires linear cut", ErrValidation)
		}
	}
	if f.Type == "DRAFT" {
		if (f.NeutralPlaneID == "") == (f.NeutralPlane == nil) {
			return fmt.Errorf("%w: draft requires one neutral plane", ErrValidation)
		}
		if err := validateReferencedPlane(f.NeutralPlaneID, f.NeutralPlane, earlier); err != nil {
			return err
		}
	}
	if isLocalModifier(f.Type) {
		if len(f.Selections) == 0 || len(f.Selections) > 128 {
			return fmt.Errorf("%w: modifier requires 1..128 selections", ErrValidation)
		}
		for _, pick := range f.Selections {
			expected := modelcore.PersistentTopologyFace
			if f.Type == "FILLET" || f.Type == "CHAMFER" {
				expected = modelcore.PersistentTopologyEdge
			}
			if err := pick.Selection.Validate(); err != nil {
				return fmt.Errorf("%w: %v", ErrValidation, err)
			}
			if pick.SourceFeatureID != "" {
				stage, ok := earlier[pick.SourceFeatureID]
				if !ok || stage.BodyID != f.BodyID || !isBodyFeature(stage.Type) {
					return fmt.Errorf("%w: invalid selection source stage", ErrValidation)
				}
			}
			anchor, exists := earlier[pick.Selection.Anchor.FeatureID]
			if pick.SourceVersionID == "" || pick.Selection.SourceBodyID != f.BodyID || pick.Selection.ExpectedType != expected || !exists || anchor.BodyID != f.BodyID {
				return fmt.Errorf("%w: modifier selection must belong to its upstream Body", ErrValidation)
			}
		}
	}
	if f.Type != "BOOLEAN" {
		return nil
	}
	if f.Operation != "ADD" && f.Operation != "REMOVE" && f.Operation != "INTERSECT" {
		return fmt.Errorf("%w: invalid Boolean operation", ErrValidation)
	}
	if len(f.Tools) == 0 || len(f.Tools) > 32 || (f.Operation == "INTERSECT" && len(f.Tools) != 1) {
		return fmt.Errorf("%w: Boolean needs 1..32 tools; intersection needs exactly one", ErrValidation)
	}
	target := false
	for _, previous := range earlier {
		if previous.BodyID == f.BodyID && isBodyFeature(previous.Type) {
			target = true
		}
	}
	if !target {
		return fmt.Errorf("%w: Boolean target has no upstream solid", ErrValidation)
	}
	seen := map[string]bool{}
	for _, ref := range f.Tools {
		tool, ok := earlier[ref.FeatureID]
		if !ok || tool.BodyID != ref.BodyID || !isBodyFeature(tool.Type) || ref.BodyID == f.BodyID || seen[ref.BodyID] {
			return fmt.Errorf("%w: Boolean tool must be a distinct upstream Body stage", ErrValidation)
		}
		seen[ref.BodyID] = true
	}
	return nil
}

func editSolidDefinition(model PartModel, current *Feature, replacement Feature, sources map[string]modelcore.ValueSource) (json.RawMessage, modelcore.ChangeSet, error) {
	if current == nil || (!isBodyFeature(current.Type) && current.Type != "SKETCH_PATTERN") || current.Type == "IMPORT_BODY" || replacement.Type != current.Type {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: unsupported Feature definition edit", ErrValidation)
	}
	// Identity and history position belong to the existing feature. Target is an explicit input.
	before := *current
	if current.Type != "BOOLEAN" && replacement.BodyID != current.BodyID {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: editing a generator cannot change Body identity", ErrValidation)
	}
	beforeParameters := append([]modelcore.ParameterDefinition(nil), model.Parameters...)
	replacement.ID, replacement.Order, replacement.Name = current.ID, current.Order, current.Name
	replacement.Visible = current.Visible
	*current = replacement
	normalizePartModel(&model)
	for i := range model.Parameters {
		parameter := &model.Parameters[i]
		if parameter.OwnerFeatureID != current.ID {
			continue
		}
		slot := parameter.PropertySlot
		if source, ok := sources[slot]; ok {
			parameter.Source = source
			continue
		}
		var oldValue, newValue float64
		switch slot {
		case "length":
			oldValue, newValue = before.Length, replacement.Length
		case "length2":
			oldValue, newValue = before.Length2, replacement.Length2
		case "angle":
			oldValue, newValue = before.Angle, replacement.Angle
		default:
			found := false
			if replacement.Pattern != nil && before.Pattern != nil {
				for _, p := range patternParameters(&replacement.Pattern.PatternDefinition) {
					if slot == "pattern:"+p.slot {
						newValue = p.value
						found = true
					}
				}
				for _, p := range patternParameters(&before.Pattern.PatternDefinition) {
					if slot == "pattern:"+p.slot {
						oldValue = p.value
					}
				}
			}
			if !found {
				continue
			}
		}
		if oldValue != newValue {
			quantity, err := modelcore.NewQuantity(newValue, parameter.DisplayUnit)
			if err != nil {
				return nil, modelcore.ChangeSet{}, err
			}
			parameter.Source = modelcore.ValueSource{Literal: &quantity}
		}
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: current.ID, SlotID: "entity"}, featureHistoryDefinition(before), featureHistoryDefinition(*current))
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, err := json.Marshal(model)
	changes, seeds := appendParameterLifecycleChanges([]modelcore.ModelChange{change}, []modelcore.DependencyKey{modelcore.DependencyKey("feature:" + current.ID)}, beforeParameters, model.Parameters)
	for _, old := range beforeParameters {
		for _, parameter := range model.Parameters {
			if old.ParameterID == parameter.ParameterID && !reflect.DeepEqual(old.Source, parameter.Source) {
				c, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: parameter.ParameterID, SlotID: "parameter.source"}, old.Source, parameter.Source)
				changes = append(changes, c)
			}
		}
	}
	return next, modelcore.ChangeSet{Changes: changes, ImpactSeeds: seeds}, err
}

func (s *Service) booleanToolInputs(ctx context.Context, requestID string, model PartModel, feature Feature) ([]geometry.BodyToolInput, error) {
	tools := make([]geometry.BodyToolInput, 0, len(feature.Tools))
	for _, ref := range feature.Tools {
		index := -1
		for i, f := range model.Features {
			if f.ID == ref.FeatureID {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("%w: Boolean input stage missing", ErrValidation)
		}
		prefix := model
		prefix.Features = append([]Feature(nil), model.Features[:index+1]...)
		key, err := s.evaluateBodyPrefix(ctx, requestID+"/tool/"+ref.FeatureID, prefix, ref.BodyID)
		if err != nil {
			return nil, err
		}
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
		if err != nil {
			return nil, err
		}
		if manifest == nil || !topologyHistoryComplete(manifest) || manifest.Tips[ref.BodyID] == nil || manifest.Tips[ref.BodyID].FeatureId != ref.FeatureID {
			return nil, fmt.Errorf("%w: Boolean tool requires complete current topology history", ErrValidation)
		}
		brep, err := s.brepArtifactReference(ctx, key)
		if err != nil {
			return nil, err
		}
		naming, err := s.representationObject(ctx, key, "NAMING")
		if err != nil {
			return nil, err
		}
		tools = append(tools, geometry.BodyToolInput{BodyID: ref.BodyID, FeatureID: ref.FeatureID, BRep: brep, Naming: geometry.ArtifactReference{Backend: naming.Backend, ObjectKey: naming.Key, SHA256: naming.SHA256, Size: naming.Size, ContentType: naming.ContentType}})
	}
	return tools, nil
}

// Consumption is model semantics, independent of definition visibility. Tools
// retain their definitions, history, and artifacts for editing and downstream reads.
func bodyConsumed(model PartModel, bodyID string) bool {
	for _, f := range model.Features {
		if strings.EqualFold(f.Type, "BOOLEAN") && !f.Suppressed && !f.KeepTools {
			for _, tool := range f.Tools {
				if tool.BodyID == bodyID {
					return true
				}
			}
		}
	}
	return false
}

func featureParameterExpressions(model PartModel, feature Feature, expressions map[string]string) (map[string]modelcore.ValueSource, error) {
	sources := map[string]modelcore.ValueSource{}
	names := map[string]modelcore.ParameterBinding{}
	for _, p := range model.Parameters {
		names[p.Key] = modelcore.ParameterBinding{ParameterID: p.ParameterID, Dimension: p.Dimension}
	}
	for slot, text := range expressions {
		dimension := modelcore.LengthDimension
		if slot == "angle" {
			dimension = modelcore.AngleDimension
		} else if slot != "length" && slot != "length2" {
			found := false
			if feature.Pattern != nil {
				for _, p := range patternParameters(&feature.Pattern.PatternDefinition) {
					if slot == "pattern:"+p.slot {
						dimension = p.dimension
						found = true
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("%w: unknown feature parameter", ErrValidation)
			}
		}
		qualified, err := resolveQualifiedParameterSource(model, text)
		if err != nil {
			return nil, err
		}
		expression, err := modelcore.CompileExpression(qualified, names, dimension)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrValidation, err)
		}
		sources[slot] = modelcore.ValueSource{Expression: &expression}
	}
	return sources, nil
}

func (s *Service) modifierInput(ctx context.Context, requestID string, model PartModel, feature Feature) (geometry.ProfilePad, error) {
	spec := geometry.ProfilePad{FeatureID: feature.ID, BodyID: feature.BodyID, Generator: feature.Type, Length: feature.Length, RevolveAngle: feature.Angle * math.Pi / 180, Reversed: feature.Reversed}
	index := -1
	for i, f := range model.Features {
		if f.ID == feature.ID {
			index = i
			break
		}
	}
	if index < 0 {
		return spec, fmt.Errorf("%w: modifier stage missing", ErrValidation)
	}
	prefix := model
	prefix.Features = append([]Feature(nil), model.Features[:index]...)
	key, err := s.evaluateBodyPrefix(ctx, requestID+"/input/"+feature.ID, prefix, feature.BodyID)
	if err != nil {
		return spec, err
	}
	for _, pick := range feature.Selections {
		resolution, err := s.resolveFeaturePick(ctx, pick, key)
		if err != nil {
			return spec, err
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			return spec, fmt.Errorf("%w: %s: %s", ErrValidation, resolution.Status, resolution.DiagnosticCode)
		}
		spec.Selections = append(spec.Selections, resolution.Candidates[0].SemanticRef)
	}
	if feature.Type == "DRAFT" {
		spec.NeutralOrigin, spec.NeutralNormal, err = s.resolveReferencedPlane(ctx, requestID, model, feature.ID, feature.NeutralPlaneID, feature.NeutralPlane)
		if err != nil {
			return spec, err
		}
	}

	return spec, nil
}

var featureFailurePattern = regexp.MustCompile(`FEATURE_FAILED\[([^\]]+)\]`)

type solidEvaluationFailure struct{ cause error }

func (f *solidEvaluationFailure) Error() string { return f.cause.Error() }
func (f *solidEvaluationFailure) Unwrap() error { return f.cause }
func markSolidFailure(model *PartModel, featureID, diagnostic string) {
	graph, _, err := buildPartEvaluation(*model, "", "", nil, nil)
	if err != nil {
		return
	}
	affected := map[string]bool{}
	for _, key := range graph.DirtyClosure([]modelcore.DependencyKey{modelcore.DependencyKey("feature:" + featureID)}) {
		affected[strings.TrimPrefix(string(key), "feature:")] = true
	}
	for i := range model.Features {
		feature := &model.Features[i]
		if !affected[feature.ID] {
			continue
		}
		feature.EvaluationStatus = "BLOCKED"
		feature.Diagnostic = "Upstream feature " + featureID + " failed"
		if feature.ID == featureID {
			feature.EvaluationStatus = "FAILED"
			feature.Diagnostic = diagnostic
		}
		if index := bodyIndex(*model, feature.BodyID); index >= 0 {
			model.Bodies[index].GeometryKey = ""
		}
	}
}

func featureHistoryDefinition(feature Feature) Feature {
	feature.EvaluationStatus = ""
	feature.Diagnostic = ""
	return feature
}

func featureInputIDs(feature Feature) []string {
	var ids []string
	for _, input := range featureInputs(feature) {
		if input.EntityKind == "FEATURE" {
			ids = append(ids, input.FeatureID)
		}
	}
	if feature.Sketch != nil {
		for _, p := range feature.Sketch.Patterns {
			for _, id := range patternReferenceFeatures(p.PatternDefinition) {
				if id != feature.ID {
					ids = append(ids, id)
				}
			}
		}
	}
	return uniqueAssociationIDs(ids)
}

func partHasFailedFeature(model PartModel) bool {
	for _, feature := range model.Features {
		if feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED" {
			return true
		}
	}
	return false
}

// loftGeometrySections is shared by definition preparation and every evaluation path.
func (s *Service) loftGeometrySections(ctx context.Context, requestID string, model PartModel, feature Feature, earlier map[string]Feature) ([]geometry.LoftSection, error) {
	result := make([]geometry.LoftSection, 0, len(feature.Sections))
	for _, section := range feature.Sections {
		value := geometry.LoftSection{SketchID: section.SketchID, Reversed: section.Reversed, SeamEntityID: section.SeamEntityID, SeamAngle: section.SeamAngle * math.Pi / 180, CorrespondenceResolved: section.CorrespondenceResolved}
		if section.Point != nil {
			point, err := patternReferencePoint(model, *section.Point)
			if err != nil {
				return nil, fmt.Errorf("%w: LOFT_POINT_UNAVAILABLE: %v", ErrValidation, err)
			}
			ref := section.Point.Reference
			value.PointID = ref.Target + "/" + ref.EntityID + "/" + ref.SubElement + "/" + ref.PointID
			if section.Point.AxisEntityID != "" {
				value.PointID = section.Point.AxisEntityID
				value.SketchID = section.Point.AxisEntityID
			}
			value.Point = point
		} else {
			sketch, err := resolvePatternSketch(model, earlier, section.SketchID, section.MemberSlot)
			if err != nil {
				return nil, err
			}
			regions, err := s.buildExactProfileRegions(ctx, sketch, requestID+"/section/"+sketch.ID)
			if err != nil {
				return nil, err
			}
			if len(regions) != 1 || len(regions[0].Holes) != 0 {
				return nil, fmt.Errorf("%w: loft requires one closed region without holes", ErrValidation)
			}
			origin, u, normal, ok := supportFrame(model, sketch.Sketch.Support)
			if !ok {
				return nil, fmt.Errorf("%w: loft section support unavailable", ErrValidation)
			}
			value.SketchID = sketch.ID
			value.Region = regions[0]
			value.Origin = origin
			value.Normal = normal
			value.UDirection = u
			if section.MemberSlot != nil {
				for _, curve := range regions[0].Outer.Curves {
					if patternMemberID(section.SketchID, *section.MemberSlot, curve.EntityID) == section.SeamEntityID {
						value.SeamEntityID = curve.EntityID
						break
					}
				}
			}
		}
		result = append(result, value)
	}
	return result, nil
}

// A whole point-only sketch is an unambiguous endpoint selection. Bind once to
// its stable entity, before profile validation; never reinterpret an accepted
// profile or replace an existing point reference after upstream edits.
func bindLoftPointSketches(sections []LoftSection, earlier map[string]Feature) ([]LoftSection, error) {
	result := append([]LoftSection(nil), sections...)
	for i := range result {
		section := &result[i]
		if section.Point != nil || section.CorrespondenceResolved || section.SeamEntityID != "" || section.MemberSlot != nil {
			continue
		}
		sketch, ok := earlier[section.SketchID]
		if !ok || sketch.Suppressed || sketch.Sketch == nil || len(sketch.Sketch.ExternalGeometry) > 0 {
			continue
		}
		entities, err := evaluatedSketchPatternEntities(*sketch.Sketch)
		if err != nil {
			return nil, err
		}
		if len(entities) == 0 {
			continue
		}
		onlyPoints := true
		for _, entity := range entities {
			if entity.Kind != "POINT" {
				onlyPoints = false
				break
			}
		}
		if !onlyPoints {
			continue
		}
		if len(entities) != 1 {
			return nil, fmt.Errorf("%w: LOFT_POINT_SELECTION_AMBIGUOUS: select one point", ErrValidation)
		}
		section.Point = &PatternPointReference{SketchID: section.SketchID, Reference: SketchGeometryRef{Target: "ENTITY", EntityID: entities[0].ID, SubElement: "POINT"}}
		section.Reversed = false
		section.SeamAngle = 0
	}
	return result, nil
}

func (s *Service) prepareLoftDefinition(ctx context.Context, requestID string, model PartModel, feature *Feature) error {
	if feature.Type != "LOFT" {
		return nil
	}
	earlier := map[string]Feature{}
	for _, f := range model.Features {
		if f.ID == feature.ID {
			break
		}
		earlier[f.ID] = f
	}
	bound, err := bindLoftPointSketches(feature.Sections, earlier)
	if err != nil {
		return err
	}
	feature.Sections = bound
	if err := validateSolidStage(*feature, earlier); err != nil {
		return err
	}
	sections, err := s.loftGeometrySections(ctx, requestID, model, *feature, earlier)
	if err != nil {
		return err
	}
	resolved, err := s.worker.ResolveLoftCorrespondence(ctx, requestID+"/loft-correspondence", sections)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}
	feature.Sections = append([]LoftSection(nil), feature.Sections...)
	for i, choice := range resolved {
		feature.Sections[i].SeamEntityID = choice.SeamEntityID
		feature.Sections[i].SeamAngle = choice.SeamAngle * 180 / math.Pi
		feature.Sections[i].Reversed = choice.Reversed
		feature.Sections[i].CorrespondenceResolved = true
	}
	return nil
}

func loftSectionIdentity(section LoftSection) string {
	if section.Point != nil {
		raw, _ := json.Marshal(section.Point)
		return "point/" + string(raw)
	}
	if section.MemberSlot == nil {
		return section.SketchID
	}
	return fmt.Sprintf("%s/member/%d", section.SketchID, *section.MemberSlot)
}

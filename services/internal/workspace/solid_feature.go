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
	return kind == "LOFT" || isLocalModifier(kind) || isSolidGenerator(kind) || kind == "IMPORT_BODY" || kind == "BOOLEAN"
}

func validateSolidStage(f Feature, earlier map[string]Feature) error {
	if f.Type == "LOFT" {
		if len(f.Sections) < 2 || len(f.Sections) > 32 {
			return fmt.Errorf("%w: loft requires 2..32 sections", ErrValidation)
		}
		seen := map[string]bool{}
		for _, section := range f.Sections {
			sketch, ok := earlier[section.SketchID]
			if !ok || sketch.Sketch == nil || seen[section.SketchID] || math.IsNaN(section.SeamAngle) || math.IsInf(section.SeamAngle, 0) {
				return fmt.Errorf("%w: invalid loft section", ErrValidation)
			}
			seen[section.SketchID] = true
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
	if current == nil || !isBodyFeature(current.Type) || current.Type == "IMPORT_BODY" || replacement.Type != current.Type {
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
			continue
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
			return nil, fmt.Errorf("%w: unknown feature parameter", ErrValidation)
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
		found := false
		for _, plane := range model.DatumPlanes {
			if plane.ID == feature.NeutralPlaneID {
				spec.NeutralOrigin = plane.Origin
				spec.NeutralNormal = plane.Normal
				found = true
			}
		}
		if !found {
			return spec, fmt.Errorf("%w: neutral plane missing", ErrValidation)
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
	if feature.Profile != "" {
		ids = append(ids, feature.Profile)
	}
	for _, tool := range feature.Tools {
		ids = append(ids, tool.FeatureID)
	}
	for _, section := range feature.Sections {
		ids = append(ids, section.SketchID)
	}
	for _, pick := range feature.Selections {
		ids = append(ids, pick.Selection.Anchor.FeatureID)
		if pick.SourceFeatureID != "" {
			ids = append(ids, pick.SourceFeatureID)
		}
	}
	if axis := strings.Split(feature.AxisEntityID, ":"); len(axis) == 3 && axis[0] == "SKETCH_LINE" {
		ids = append(ids, axis[1])
	}
	return ids
}
func partHasFailedFeature(model PartModel) bool {
	for _, feature := range model.Features {
		if feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED" {
			return true
		}
	}
	return false
}

// Choose an initial closed-point edge once, then persist its sketch entity ID.
// Evaluators never silently choose another seam after an upstream edit.
func (s *Service) prepareLoftDefinition(ctx context.Context, requestID string, model PartModel, feature *Feature) error {
	if feature.Type != "LOFT" {
		return nil
	}
	feature.Sections = append([]LoftSection(nil), feature.Sections...)
	for i := range feature.Sections {
		section := &feature.Sections[i]
		if section.SeamEntityID != "" {
			continue
		}
		var sketch *Feature
		for j := range model.Features {
			if model.Features[j].ID == section.SketchID {
				sketch = &model.Features[j]
				break
			}
		}
		if sketch == nil || sketch.Sketch == nil {
			return fmt.Errorf("%w: loft section missing", ErrValidation)
		}
		regions, err := s.buildExactProfileRegions(ctx, *sketch, requestID+"/loft-seam/"+sketch.ID)
		if err != nil {
			return err
		}
		if len(regions) != 1 || len(regions[0].Holes) != 0 {
			return fmt.Errorf("%w: loft requires one closed region without holes", ErrValidation)
		}
		boundary := map[string]bool{}
		for _, curve := range regions[0].Outer.Curves {
			boundary[curve.EntityID] = true
		}
		// Entity order is the explicit sketch definition's creation order. This only
		// initializes a new seam; it is never used to recover an invalid saved ref.
		for _, entity := range sketch.Sketch.Entities {
			if boundary[entity.ID] {
				section.SeamEntityID = entity.ID
				break
			}
		}
		if section.SeamEntityID == "" {
			return fmt.Errorf("%w: no loft seam edge", ErrValidation)
		}
	}
	return nil
}

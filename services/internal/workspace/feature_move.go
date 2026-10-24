package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/modelcore"
)

const typeMoveFeature = "occccad://part/feature/move"

type moveFeaturePayload struct {
	FeatureID             string `json:"featureId"`
	AnchorFeatureID       string `json:"anchorFeatureId"`
	Placement             string `json:"placement"`
	ExpectedFeatureDigest string `json:"expectedFeatureDigest"`
}

func movableFeature(model PartModel, id string) bool {
	for _, body := range model.Bodies {
		if body.CreatedByFeatureID == id {
			return false
		}
	}
	first := map[string]bool{}
	for _, f := range model.Features {
		if !isBodyFeature(f.Type) {
			continue
		}
		base := !first[f.BodyID]
		first[f.BodyID] = true
		if f.ID == id {
			return !base && (isSolidGenerator(f.Type) || f.Type == "HOLE" || f.Type == "LOFT" || isLocalModifier(f.Type))
		}
	}
	return false
}

func moveBlocked(f Feature, reason string) error {
	return fmt.Errorf("%w: MOVE_FEATURE_BLOCKED[%s] %s: %s", ErrValidation, f.ID, f.Name, reason)
}

// Validate declared inputs against the candidate list, without carrying over
// implicit Body predecessor edges from the old evaluation graph.
func validateMoveInputs(model PartModel) error { return validateFeatureOrderInputs(model, moveBlocked) }

func validateFeatureOrderInputs(model PartModel, blocked func(Feature, string) error) error {
	seen := map[string]bool{}
	earlier := map[string]Feature{}
	for _, f := range model.Features {
		for _, input := range featureInputs(f) {
			if input.EntityKind != "FEATURE" || input.Role == "PATTERN_REFERENCE" && input.FeatureID == f.ID && f.Sketch != nil {
				continue
			}
			if !seen[input.FeatureID] {
				return blocked(f, fmt.Sprintf("%s source %s must precede its consumer", input.Role, input.FeatureID))
			}
		}
		if f.Sketch != nil {
			for _, e := range f.Sketch.ExternalGeometry {
				if e.ContextReferenceID != "" {
					continue
				}
				if d := e.DatumReference; d != nil {
					for _, id := range []string{d.FeatureID, func() string {
						if d.Selection != nil {
							return d.Selection.Anchor.FeatureID
						}
						return ""
					}()} {
						if id != "" && !seen[id] {
							return blocked(f, "external source "+id+" must precede sketch")
						}
					}
				} else if id := e.PersistentSelection.Anchor.FeatureID; id != "" && !seen[id] {
					return blocked(f, "external topology source "+id+" must precede sketch")
				}
			}
		}
		// The regular definition validators cover types, Body/source stage and datum
		// rules. Report the owning consumer as the actual blocker.
		if err := validateSolidStage(f, earlier); err != nil {
			return blocked(f, err.Error())
		}
		seen[f.ID] = true
		earlier[f.ID] = f
	}
	if err := validateAndResolvePartParameters(&model); err != nil {
		return err
	}
	graph, _, err := buildPartEvaluation(model, "", "", nil, nil)
	if err != nil {
		return err
	}
	return validateDatumFeatureOrder(model, graph)
}

func rangeMembers(model PartModel, p *FeaturePattern) []string {
	inside := false
	var ids []string
	for _, f := range model.Features {
		if f.ID == p.StartFeatureID {
			inside = true
		}
		if inside && f.BodyID == p.Source.BodyID && isBodyFeature(f.Type) {
			ids = append(ids, f.ID)
		}
		if f.ID == p.Source.FeatureID {
			break
		}
	}
	return ids
}

func applyMoveFeature(raw, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var m PartModel
	var p moveFeaturePayload
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &p); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	normalizePartModel(&m)
	i := slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == p.FeatureID })
	a := slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == p.AnchorFeatureID })
	if i < 0 || a < 0 || i == a || (p.Placement != "BEFORE" && p.Placement != "AFTER") {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: MOVE_FEATURE_TARGET_INVALID", ErrValidation)
	}
	source := m.Features[i]
	if !movableFeature(m, source.ID) {
		return nil, modelcore.ChangeSet{}, moveBlocked(source, "fixed feature cannot move")
	}
	if source.BodyID != m.Features[a].BodyID {
		return nil, modelcore.ChangeSet{}, moveBlocked(source, "anchor must belong to the same Body")
	}
	digest, err := featureDefinitionDigest(m, p.FeatureID)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if p.ExpectedFeatureDigest == "" || digest != p.ExpectedFeatureDigest {
		return nil, modelcore.ChangeSet{}, featureEditFailure{code: "FEATURE_EDIT_STALE"}
	}
	before := append([]Feature(nil), m.Features...)
	m.Features = append(m.Features[:i:i], m.Features[i+1:]...)
	a = slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == p.AnchorFeatureID })
	if p.Placement == "AFTER" {
		a++
	}
	if a == i {
		return nil, modelcore.ChangeSet{}, moveBlocked(source, "position is unchanged")
	}
	m.Features = slices.Insert(m.Features, a, source)
	for j := range m.Features {
		m.Features[j].Order = j + 1
	}
	// Fixed nodes divide each Body's history. Other Bodies keep their relative
	// order, and their explicit consumers are checked over the full global list.
	oldIndex, newIndex := map[string]int{}, map[string]int{}
	for j, f := range before {
		oldIndex[f.ID] = j
	}
	for j, f := range m.Features {
		newIndex[f.ID] = j
	}
	oldModel := m
	oldModel.Features = before
	for _, f := range before {
		if f.BodyID == source.BodyID && isBodyFeature(f.Type) && !movableFeature(oldModel, f.ID) && (i < oldIndex[f.ID]) != (a < newIndex[f.ID]) {
			return nil, modelcore.ChangeSet{}, moveBlocked(f, "fixed feature cannot be crossed")
		}
	}
	if err := protectFeatureRanges(oldModel, m, moveBlocked); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := validateMoveInputs(m); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	set := modelcore.ChangeSet{}
	for j, f := range m.Features {
		original := before[oldIndex[f.ID]]
		if original.Order == f.Order {
			continue
		}
		c, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: f.ID, SlotID: "entity"}, featureHistoryDefinition(original), featureHistoryDefinition(m.Features[j]))
		if err != nil {
			return nil, set, err
		}
		set.Changes = append(set.Changes, c)
		set.ImpactSeeds = append(set.ImpactSeeds, modelcore.DependencyKey("feature:"+f.ID))
	}
	if err := set.Finalize(); err != nil {
		return nil, set, err
	}
	next, err := json.Marshal(m)
	return next, set, err
}

func rejectOrderReferenceFailures(model PartModel, blocked func(Feature, string) error) error {
	for _, p := range model.Publications {
		if p.Resolution.Status == "BROKEN_PUBLICATION" {
			return blocked(Feature{ID: p.ID, Name: p.Name}, "publication: "+p.Resolution.Diagnostic)
		}
	}
	for _, f := range model.Features {
		if f.Suppressed || f.Sketch == nil {
			continue
		}
		if f.Sketch.Support.Status == "FAILED_SUPPORT" || f.Sketch.Support.Status == "BLOCKED" {
			return blocked(f, f.Sketch.Support.DiagnosticCode+": "+f.Sketch.Support.Diagnostic)
		}
		for _, e := range f.Sketch.ExternalGeometry {
			if e.Status != "CONNECTED" {
				return blocked(f, e.DiagnosticCode+": "+e.Diagnostic)
			}
		}
	}
	return nil
}
func orderEvaluationError(model PartModel, err error, blocked func(Feature, string) error) error {
	matches := featureFailurePattern.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) > 0 {
		for _, f := range model.Features {
			if f.ID == matches[len(matches)-1][1] {
				return fmt.Errorf("%w: %w", blocked(f, "geometry evaluation failed"), err)
			}
		}
	}
	return err
}
func (s *Service) evaluateOrderedBodies(ctx context.Context, documentID, requestID string, m *PartModel, blocked func(Feature, string) error) error {
	if err := s.validateOrderHistoricalStages(ctx, documentID, *m, blocked); err != nil {
		return err
	}
	if err := rejectOrderReferenceFailures(*m, blocked); err != nil {
		return err
	}
	if err := s.evaluatePartBodies(ctx, requestID, m); err != nil {
		return orderEvaluationError(*m, err, blocked)
	}
	return nil
}

// Empty stage is the existing "historical Body tip" contract, not permission
// to ignore that stage during reordering. Read its exact immutable witness;
// never fill or rewrite the saved selection as part of a move.
func (s *Service) validateOrderHistoricalStages(ctx context.Context, documentID string, m PartModel, blocked func(Feature, string) error) error {
	indices := map[string]int{}
	for i, f := range m.Features {
		indices[f.ID] = i
	}
	stages := map[string]string{}
	for i, f := range m.Features {
		picks := append([]FeatureSelection(nil), f.Selections...)
		for _, p := range []*FeatureSelection{f.NeutralPlane, f.TerminationPlane} {
			if p != nil {
				picks = append(picks, *p)
			}
		}
		if f.Pattern != nil && f.Pattern.MirrorPlane != nil {
			picks = append(picks, *f.Pattern.MirrorPlane)
		}
		if f.Sketch != nil && f.Sketch.Support.PersistentSelection != nil {
			p := f.Sketch.Support
			picks = append(picks, FeatureSelection{Selection: *p.PersistentSelection, SourceVersionID: p.SourceVersionID, SourceFeatureID: p.SourceFeatureID})
		}
		if f.Sketch != nil {
			for _, e := range f.Sketch.ExternalGeometry {
				if e.ContextReferenceID != "" {
					continue
				}
				if d := e.DatumReference; d != nil {
					if d.Selection != nil {
						picks = append(picks, FeatureSelection{Selection: *d.Selection, SourceVersionID: d.SourceVersionID, SourceFeatureID: d.FeatureID})
					}
				} else {
					picks = append(picks, FeatureSelection{Selection: e.PersistentSelection, SourceVersionID: e.SourceVersionID})
				}
			}
		}
		for _, p := range picks {
			if p.Selection.SourceDocumentID != documentID {
				return blocked(f, "topology input must belong to the same Part")
			}
			stage := p.SourceFeatureID
			if stage == "" {
				cacheKey := p.SourceVersionID + "/" + p.Selection.SourceBodyID
				stage = stages[cacheKey]
				if stage == "" {
					key, err := s.featureStageAtVersion(ctx, documentID, p.SourceVersionID, "", p.Selection.SourceBodyID)
					if err != nil {
						return err
					}
					manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
					if err != nil {
						return err
					}
					tip := manifest.Tips[p.Selection.SourceBodyID]
					if tip == nil || !topologyHistoryComplete(manifest) {
						return blocked(f, "source stage history is unavailable")
					}
					stage = tip.FeatureId
					stages[cacheKey] = stage
				}
			}
			if index, exists := indices[stage]; !exists || index >= i {
				return blocked(f, "SOURCE_STAGE "+stage+" must precede its consumer")
			}
		}
	}
	return nil
}

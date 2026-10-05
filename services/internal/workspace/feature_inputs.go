package workspace

import "strings"

// Feature inputs are ordered definition references, never ownership edges.
type featureInput struct {
	EntityKind   string
	Role         string
	FeatureID    string
	EntityID     string
	MemberSlot   *int
	SectionOrder int
}

func featureInputs(f Feature) []featureInput {
	var out []featureInput
	add := func(role, id, entity string, slot *int) {
		if id != "" {
			out = append(out, featureInput{Role: role, EntityKind: "FEATURE", FeatureID: id, EntityID: entity, MemberSlot: slot})
		}
	}
	axis := func(id string) {
		parts := strings.Split(id, ":")
		if len(parts) == 3 && parts[0] == "SKETCH_LINE" {
			add("AXIS", parts[1], parts[2], nil)
		} else if len(parts) > 1 {
			out = append(out, featureInput{Role: "AXIS", EntityKind: parts[0], FeatureID: parts[1], EntityID: strings.Join(parts[2:], ":")})
		}
	}
	add("PROFILE", f.Profile, "", f.ProfileMemberSlot)
	for order, s := range f.Sections {
		start := len(out)
		if s.Point != nil {
			add("SECTION_POINT", s.Point.SketchID, s.Point.Reference.EntityID, nil)
			axis(s.Point.AxisEntityID)
		} else {
			add("SECTION", s.SketchID, "", s.MemberSlot)
		}
		for i := start; i < len(out); i++ {
			out[i].SectionOrder = order
		}
	}
	axis(f.AxisEntityID)
	if f.NeutralPlaneID != "" {
		out = append(out, featureInput{Role: "NEUTRAL_PLANE", EntityKind: "PLANE", FeatureID: f.NeutralPlaneID})
	}
	for _, t := range f.Tools {
		add("TOOL", t.FeatureID, "", nil)
	}
	if f.Pattern != nil {
		add("SEED", f.Pattern.Source.FeatureID, "", nil)
		add("RANGE_START", f.Pattern.StartFeatureID, "", nil)
		axis(f.Pattern.AxisEntityID)
		for _, id := range patternReferenceFeatures(f.Pattern.PatternDefinition) {
			add("PATTERN_REFERENCE", id, "", nil)
		}
	}
	if f.Sketch != nil && f.Sketch.Support.DatumPlaneID != "" {
		out = append(out, featureInput{Role: "SUPPORT", EntityKind: "PLANE", FeatureID: f.Sketch.Support.DatumPlaneID})
	}
	if f.Sketch != nil && f.Sketch.Support.PersistentSelection != nil {
		add("SUPPORT", f.Sketch.Support.PersistentSelection.Anchor.FeatureID, "", nil)
	}
	if p := f.NeutralPlane; p != nil {
		add("NEUTRAL_PLANE", p.Selection.Anchor.FeatureID, "", nil)
		add("STAGE", p.SourceFeatureID, "", nil)
	}
	for _, p := range f.Selections {
		add("TOPOLOGY", p.Selection.Anchor.FeatureID, "", nil)
		add("STAGE", p.SourceFeatureID, "", nil)
	}
	return out
}

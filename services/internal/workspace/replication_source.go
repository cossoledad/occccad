package workspace

// ReplicationSource describes intrinsic geometry contracts, not a promise to
// replay an arbitrary feature at another position. It is derived read data.
type ReplicationSource struct {
	SourceKind     string `json:"sourceKind,omitempty"`
	StartFeatureID string `json:"startFeatureId,omitempty"`
	Operation      string `json:"operation,omitempty"`
	Tool           bool   `json:"tool"`
	RangeStart     bool   `json:"rangeStart"`
	RangeModifier  bool   `json:"rangeModifier"`
	BodyStage      bool   `json:"bodyStage"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}

func replicationSource(f Feature) ReplicationSource {
	c := ReplicationSource{BodyStage: isBodyFeature(f.Type)}
	switch f.Type {
	case "SOLID_PATTERN":
		c.Tool = f.Pattern != nil && !f.Pattern.Suppressed && patternHasAdditionalMembers(f.Pattern) && (f.Operation == "ADD" || f.Operation == "REMOVE")
		if !c.Tool {
			c.Diagnostic = "PATTERN_INDEPENDENT_TOOL_UNAVAILABLE"
		}
	case "HOLE":
		kind := holeKind(f)
		c.Tool = (f.Extent == "BLIND" || f.Extent == "THROUGH_ALL") && f.Operation == "REMOVE" && (kind == "SIMPLE" || kind == "COUNTERBORE" || kind == "COUNTERSINK") && validateHoleDimensions(f) == nil
		c.RangeStart = c.Tool && f.Extent == "BLIND"
		if !c.Tool {
			c.Diagnostic = "PATTERN_REEXECUTION_UNSUPPORTED"
		}
	case "PAD", "LINEAR_EXTRUDE", "REVOLVE", "LOFT":
		c.Tool = (f.Extent == "" || f.Extent == "FINITE" || f.Extent == "SYMMETRIC" || f.Extent == "TWO_SIDED") && (f.Operation == "ADD" || f.Operation == "REMOVE" || f.Operation == "NEW_BODY" || f.Operation == "")
		c.RangeStart = c.Tool
		if !c.Tool {
			c.Diagnostic = "PATTERN_REEXECUTION_UNSUPPORTED"
		}
	case "FILLET", "CHAMFER", "DRAFT":
		c.RangeModifier = true
	default:
		c.Diagnostic = "PATTERN_INDEPENDENT_TOOL_UNAVAILABLE"
	}
	return c
}

func replicationSources(model PartModel) map[string]ReplicationSource {
	result := make(map[string]ReplicationSource, len(model.Features))
	for i, f := range model.Features {
		c := replicationSource(f)
		if f.Suppressed || f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
			c = ReplicationSource{Diagnostic: "PATTERN_SOURCE_STAGE_UNAVAILABLE"}
			result[f.ID] = c
			continue
		}
		if c.Tool {
			c.SourceKind = "GENERATOR_TOOL"
			c.Operation = f.Operation
			if c.Operation == "NEW_BODY" || c.Operation == "" {
				c.Operation = "ADD"
			}
		} else if f.Sketch != nil && !f.Suppressed {
			c.SourceKind = "SKETCH_FRAME"
		} else if c.RangeModifier {
			for j := i - 1; j >= 0; j-- {
				previous := model.Features[j]
				if previous.BodyID != f.BodyID || previous.Suppressed || !isBodyFeature(previous.Type) {
					continue
				}
				cap := result[previous.ID]
				if cap.RangeModifier {
					continue
				}
				if cap.RangeStart {
					c.SourceKind = "FEATURE_DELTA"
					c.StartFeatureID = previous.ID
					c.Operation = cap.Operation
				}
				break
			}
		}
		result[f.ID] = c
	}
	return result
}

// Only additional active slots form an independently reusable tool. Keep
// out-of-count skip identities for later restoration, without counting them.
func patternHasAdditionalMembers(p *FeaturePattern) bool {
	if p == nil || p.Count < 2 || p.Count > maxPatternMembers {
		return false
	}
	skipped := make(map[int]bool, len(p.SkippedSlots))
	for _, slot := range p.SkippedSlots {
		skipped[slot] = true
	}
	for slot := 1; slot < int(p.Count); slot++ {
		if !skipped[slot] {
			return true
		}
	}
	return false
}

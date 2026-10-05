package workspace

// ReplicationSource describes intrinsic geometry contracts, not a promise to
// replay an arbitrary feature at another position. It is derived read data.
type ReplicationSource struct {
	Tool          bool   `json:"tool"`
	RangeStart    bool   `json:"rangeStart"`
	RangeModifier bool   `json:"rangeModifier"`
	BodyStage     bool   `json:"bodyStage"`
	Diagnostic    string `json:"diagnostic,omitempty"`
}

func replicationSource(f Feature) ReplicationSource {
	c := ReplicationSource{BodyStage: isBodyFeature(f.Type)}
	switch f.Type {
	case "PAD", "LINEAR_EXTRUDE", "REVOLVE", "LOFT":
		c.Tool = f.Extent != "THROUGH_ALL" && (f.Operation == "ADD" || f.Operation == "REMOVE" || f.Operation == "NEW_BODY" || f.Operation == "")
		c.RangeStart = c.Tool && f.Operation != "REMOVE"
		if !c.Tool {
			c.Diagnostic = "PATTERN_REEXECUTION_UNSUPPORTED"
		}
	case "FILLET", "CHAMFER":
		c.RangeModifier = true
	default:
		c.Diagnostic = "PATTERN_INDEPENDENT_TOOL_UNAVAILABLE"
	}
	return c
}

func replicationSources(model PartModel) map[string]ReplicationSource {
	result := make(map[string]ReplicationSource, len(model.Features))
	for _, f := range model.Features {
		c := replicationSource(f)
		if f.Suppressed || f.EvaluationStatus == "FAILED" || f.EvaluationStatus == "BLOCKED" {
			c = ReplicationSource{Diagnostic: "PATTERN_SOURCE_STAGE_UNAVAILABLE"}
		}
		result[f.ID] = c
	}
	return result
}

package workspace

import (
	"encoding/json"

	"github.com/occccad/occccad/internal/modelcore"
)

// Only the successful production preview return calls this projection. It must
// never derive geometry from operation parameters, meshes, or an old snapshot.
func solvedSketchPreviewCandidates(command modelcore.DomainCommand, model PartModel) []SketchCandidatePreview {
	if command.TypeURI != typeEditSketch {
		return nil
	}
	var payload editSketchPayload
	if json.Unmarshal(command.Payload, &payload) != nil || payload.SketchID == "" {
		return nil
	}
	for _, feature := range model.Features {
		if feature.ID != payload.SketchID || feature.Sketch == nil {
			continue
		}
		// Copy the solved candidate so response consumers cannot mutate another
		// projection's slices or the interaction candidate snapshot.
		raw, err := json.Marshal(SketchCandidatePreview{FeatureID: feature.ID, Entities: feature.Sketch.Entities, Solve: feature.Sketch.Solve})
		if err != nil {
			return nil
		}
		var candidate SketchCandidatePreview
		if json.Unmarshal(raw, &candidate) != nil {
			return nil
		}
		return []SketchCandidatePreview{candidate}
	}
	return nil
}

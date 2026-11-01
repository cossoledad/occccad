package workspace

import (
	"encoding/json"
	"fmt"

	"github.com/occccad/occccad/internal/modelcore"
)

type analysisPlacement struct {
	DocumentID, Occurrence, EntityID, RevisionID string
	Pose                                         InstancePose
}

// Analysis adapters declare the inputs they actually read. This shared
// fingerprint engine ignores provenance-only revision changes and presentation.
func analysisInputStamp(evaluator, policy string, definition any, reads []modelcore.DependencyRead) (string, error) {
	raw, err := json.Marshal(definition)
	if err != nil {
		return "", err
	}
	return (modelcore.DerivedInput{Evaluator: evaluator, Policy: policy, Definition: raw, Reads: reads}).Fingerprint()
}

func measurementInputStamps(kind string, reads []modelcore.DependencyRead, placements []analysisPlacement) (map[string]string, InstancePose, error) {
	base := InstancePose{Rotation: [4]float64{0, 0, 0, 1}}
	if len(placements) > 0 {
		base = placements[0].Pose
	}
	out := map[string]string{}
	for _, facet := range []string{"INTRINSIC", "RELATIVE", "FRAME"} {
		dependencies := append([]modelcore.DependencyRead(nil), reads...)
		if facet != "INTRINSIC" {
			for _, p := range placements {
				pose := p.Pose
				if facet == "RELATIVE" {
					pose = inverseRelativePose(pose, base)
				}
				dependencies = append(dependencies, modelcore.DependencyRead{DocumentID: p.DocumentID, Occurrence: p.Occurrence, EntityID: p.EntityID, Facet: modelcore.Placement, Digest: resolvedDigest(pose), RevisionID: p.RevisionID})
			}
		}
		stamp, err := analysisInputStamp(measurementEvaluatorVersion, measurementPolicy, struct{ Kind, Facet string }{kind, facet}, dependencies)
		if err != nil {
			return nil, base, err
		}
		out[facet] = stamp
	}
	return out, base, nil
}

func measurementOutputFacet(kind, key string) string {
	switch key {
	case "xMm", "yMm", "zMm", "deltaXMm", "deltaYMm", "deltaZMm", "sizeXMm", "sizeYMm", "sizeZMm":
		return "FRAME"
	}
	if kind != "ITEM" {
		return "RELATIVE"
	}
	return "INTRINSIC"
}

func measurementPrimaryFacet(m Measurement) string {
	if m.Kind != "ITEM" {
		return "RELATIVE"
	}
	if len(m.Supports) == 1 && (m.Supports[0].Kind == "VERTEX" || m.Supports[0].Mode == "CENTER") {
		return "FRAME"
	}
	return "INTRINSIC"
}

func measurementQuantity(key string, value float64) (modelcore.Quantity, string, error) {
	unit := "mm"
	switch key {
	case "areaMm2":
		unit = "mm2"
	case "volumeMm3":
		unit = "mm3"
	case "angleRad", "arcAngleRad":
		unit = "rad"
	case "distanceMm", "lengthMm", "perimeterMm", "radiusMm", "diameterMm", "majorRadiusMm", "minorRadiusMm", "xMm", "yMm", "zMm", "deltaXMm", "deltaYMm", "deltaZMm", "sizeXMm", "sizeYMm", "sizeZMm":
	default:
		return modelcore.Quantity{}, "", fmt.Errorf("%w: unknown measurement output %s", ErrValidation, key)
	}
	q, err := modelcore.NewQuantity(value, unit)
	return q, unit, err
}

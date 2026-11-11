package workspace

import "context"

// Reference display belongs to the accepted Part snapshot, including Parts
// without any solid artifact. It is independent of per-Body tessellation.
type ReferenceDisplayScope struct {
	DocumentID        string                    `json:"documentId"`
	VersionID         string                    `json:"versionId"`
	InstancePath      InstancePath              `json:"instancePath"`
	Translation       [3]float64                `json:"translation"`
	Rotation          [4]float64                `json:"rotation"`
	ReferenceGeometry ReferenceGeometry         `json:"referenceGeometry"`
	Results           []ReferenceGeometryResult `json:"results,omitempty"`
}
type ReferenceGeometryResult struct {
	FeatureID   string             `json:"featureId"`
	ResultID    string             `json:"resultId"`
	GeometryKey string             `json:"geometryKey"`
	Domain      string             `json:"domain"`
	Visible     bool               `json:"visible"`
	Appearance  ResolvedAppearance `json:"appearance"`
}
type referenceDisplayScopesKey struct{}

func (s *Service) captureReferenceDisplayScope(ctx context.Context, documentID, revisionID string, path InstancePath, pose InstancePose, model PartModel, artifacts map[string]Artifact, rules []occurrenceAppearanceRule) error {
	if scopes, ok := ctx.Value(referenceDisplayScopesKey{}).(*[]ReferenceDisplayScope); ok {
		scope := ReferenceDisplayScope{DocumentID: documentID, VersionID: revisionID, InstancePath: path, Translation: pose.Translation, Rotation: pose.Rotation, ReferenceGeometry: referenceGeometry(model)}
		for _, feature := range model.Features {
			if feature.Type != "GEOMETRY" || feature.Result == nil || feature.Suppressed || feature.EvaluationStatus == "FAILED" || feature.EvaluationStatus == "BLOCKED" {
				continue
			}
			key := feature.Result.GeometryKey
			if key == "" {
				continue
			}
			if _, ok := artifacts[key]; !ok {
				artifact, err := s.loadArtifact(ctx, key)
				if err != nil {
					return err
				}
				artifacts[key] = artifact
			}
			appearance, err := s.resultAppearance(ctx, feature, model.DefaultAppearance)
			if err != nil {
				return err
			}
			appearance, err = s.applyOccurrenceAppearance(ctx, documentID, PartBody{GeometryKey: key}, appearance, path.Canonical, rules)
			if err != nil {
				return err
			}
			scope.Results = append(scope.Results, ReferenceGeometryResult{FeatureID: feature.ID, ResultID: feature.Result.ID, GeometryKey: key, Domain: feature.Result.Domain, Visible: visibleOrDefault(feature.Visible) && GeometryFeatureVisible(model, feature), Appearance: appearance})
		}
		*scopes = append(*scopes, scope)
	}
	return nil
}

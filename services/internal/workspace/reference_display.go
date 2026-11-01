package workspace

import "context"

// Reference display belongs to the accepted Part snapshot, including Parts
// without any solid artifact. It is independent of per-Body tessellation.
type ReferenceDisplayScope struct {
	DocumentID        string            `json:"documentId"`
	VersionID         string            `json:"versionId"`
	InstancePath      InstancePath      `json:"instancePath"`
	Translation       [3]float64        `json:"translation"`
	Rotation          [4]float64        `json:"rotation"`
	ReferenceGeometry ReferenceGeometry `json:"referenceGeometry"`
}
type referenceDisplayScopesKey struct{}

func captureReferenceDisplayScope(ctx context.Context, documentID, revisionID string, path InstancePath, pose InstancePose, model PartModel) {
	if scopes, ok := ctx.Value(referenceDisplayScopesKey{}).(*[]ReferenceDisplayScope); ok {
		*scopes = append(*scopes, ReferenceDisplayScope{DocumentID: documentID, VersionID: revisionID, InstancePath: path, Translation: pose.Translation, Rotation: pose.Rotation, ReferenceGeometry: referenceGeometry(model)})
	}
}

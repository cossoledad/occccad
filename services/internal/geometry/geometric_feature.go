package geometry

import (
	"context"
	"fmt"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	perf "github.com/occccad/occccad/internal/performance"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// Conversions belong to the transport boundary; domain definitions do not
// persist generated protocol messages or kernel implementation types.
func ProfileCurveProto(c ProfileCurve) *workerv1.ProfileCurve                { return profileCurveProto(c) }
func ArtifactReferenceProto(r ArtifactReference) *workerv1.ArtifactReference { return artifactProto(r) }
func (c *Client) EvaluateGeometryFeature(ctx context.Context, requestID, key string, spec *workerv1.GeometryFeatureSpec, brepOutput, visualOutput string) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	options := PartRuntime(ctx)
	request := &workerv1.EvaluatePartRequest{RequestId: requestID, GeometryKey: key, GeometryFeature: spec, TopologyPolicy: topologyNamingPolicyProto(), BrepOutputKey: brepOutput, GlbOutputKey: visualOutput, ForceCold: options.ForceCold, ExactOnly: options.ExactOnly, RuntimeAffinity: options.Affinity}
	if options.ExactOnly {
		request.GlbOutputKey = ""
	}
	var header metadata.MD
	finishRPC := perf.Start(ctx, "part-rpc")
	result, err := c.worker.EvaluatePart(ctx, request, grpc.Header(&header))
	finishRPC()
	if err != nil {
		return nil, fmt.Errorf("evaluate geometric feature: %w", err)
	}
	c.rememberWorker(ctx, key, header)
	return result, nil
}

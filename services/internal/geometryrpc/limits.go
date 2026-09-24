// Package geometryrpc owns the bounded transport budget shared by geometry
// clients and routers. File limits are separate: files use ArtifactReference.
package geometryrpc

import "google.golang.org/grpc"

// MaxMessageBytes allows evaluation mesh/topology payloads above gRPC's 4 MiB
// default. Keep in sync with kGeometryRpcMaxMessageBytes in the C++ Worker.
// This is a per-message ceiling, not an aggregate memory or model-size budget.
const MaxMessageBytes = 128 << 20

func ClientOptions() grpc.DialOption {
	return grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(MaxMessageBytes), grpc.MaxCallSendMsgSize(MaxMessageBytes))
}
func ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.MaxRecvMsgSize(MaxMessageBytes), grpc.MaxSendMsgSize(MaxMessageBytes)}
}

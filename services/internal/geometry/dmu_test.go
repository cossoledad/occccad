package geometry

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"strings"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/artifact"
	"google.golang.org/grpc"
)

func TestDMUWorkerSharedInstancesPartialInvalidAndCancel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("OCCCCAD_DATA_DIR", root)
	client := openCppWorkerForNamingContract(t)
	local, e := artifact.NewLocalStore(root)
	if e != nil {
		t.Fatal(e)
	}
	r, e := client.EvaluatePartFromArtifact(t.Context(), "dmu/box", "dmu/box", []RectangularPad{{Width: 10, Height: 10, Length: 10, Plane: "XY"}}, ArtifactReference{}, "dmu/box.brep", "dmu/box.glb")
	if e != nil {
		t.Fatal(e)
	}
	bad, e := local.Put(t.Context(), artifact.KindBREP, "brep", strings.NewReader("not a B-Rep"))
	if e != nil {
		t.Fatal(e)
	}
	poses := []AssemblyPose{{Rotation: [4]float64{0, 0, 0, 1}}, {Translation: [3]float64{15, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}
	inputs := []AnalysisGeometry{{ID: "root/a/body1", GeometryID: r.GeometryId, BRep: ArtifactReference{Backend: r.BrepArtifact.Backend, ObjectKey: r.BrepArtifact.ObjectKey, SHA256: r.BrepArtifact.Sha256, Size: int64(r.BrepArtifact.SizeBytes)}, Pose: poses[0]}, {ID: "root/b/body1", GeometryID: r.GeometryId, BRep: ArtifactReference{Backend: r.BrepArtifact.Backend, ObjectKey: r.BrepArtifact.ObjectKey, SHA256: r.BrepArtifact.Sha256, Size: int64(r.BrepArtifact.SizeBytes)}, Pose: poses[1]}, {ID: "root/c/body1", GeometryID: "bad", BRep: ArtifactReference{Backend: "LOCAL", ObjectKey: bad.Key, SHA256: bad.SHA256, Size: bad.Size}, Pose: poses[0]}}
	pairs := []AnalysisPair{{FirstID: inputs[0].ID, SecondID: inputs[1].ID}, {FirstID: inputs[0].ID, SecondID: inputs[2].ID}}
	report, e := client.AnalyzeInterference(t.Context(), "partial", inputs, pairs, 4, 1e-7)
	if e != nil || report.Complete || !report.Pairs[0].Complete || report.Pairs[0].Classification != "SEPARATED" || report.Pairs[1].Complete || report.Pairs[1].Classification != "INCONCLUSIVE" {
		t.Fatal(report, e)
	}
	if report.Pairs[0].FirstID == report.Pairs[0].SecondID {
		t.Fatal("shared geometry collapsed instance identity")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = client.AnalyzeInterference(ctx, "cancel", inputs[:2], pairs[:1], 4, 1e-7); e == nil {
		t.Fatal("cancelled query passed")
	}
}

// A failed OCCT computation may carry non-finite intermediate measures. They
// must neither certify success nor prevent completed pairs from being saved.
type dmuFixtureServer struct {
	workerv1.UnimplementedGeometryWorkerServer
	response *workerv1.AnalyzeInterferenceResponse
}

func (s dmuFixtureServer) AnalyzeInterference(context.Context, *workerv1.AnalyzeInterferenceRequest) (*workerv1.AnalyzeInterferenceResponse, error) {
	return s.response, nil
}
func TestDMUIncompleteNumericIntermediatesRemainPersistable(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := grpc.NewServer()
	defer server.Stop()
	response := &workerv1.AnalyzeInterferenceResponse{KernelBuild: "fixture", Pairs: []*workerv1.InterferencePairResult{
		{FirstId: "a", SecondId: "b", Classification: "SEPARATED", DistanceMm: 5, Complete: true, ClearanceSatisfied: true, FirstWitness: &workerv1.Vec3{}, SecondWitness: &workerv1.Vec3{X: 5}},
		{FirstId: "a", SecondId: "c", Classification: "INCONCLUSIVE", DistanceMm: math.NaN(), CommonVolumeMm3: math.Inf(1), Diagnostic: "DISTANCE_UNFINISHED"},
	}}
	workerv1.RegisterGeometryWorkerServer(server, dmuFixtureServer{response: response})
	go server.Serve(listener)
	client, e := Open(listener.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	r, e := client.AnalyzeInterference(t.Context(), "bad-numeric", nil, []AnalysisPair{{FirstID: "a", SecondID: "b"}, {FirstID: "a", SecondID: "c"}}, 0, 1e-7)
	if e != nil || r.Complete || !r.Pairs[0].Complete || r.Pairs[1].ClearanceSatisfied || r.Pairs[1].Diagnostic != "DISTANCE_UNFINISHED" {
		t.Fatal(r, e)
	}
	if _, e = json.Marshal(r); e != nil {
		t.Fatal("completed pair report lost to invalid intermediate", e)
	}
}

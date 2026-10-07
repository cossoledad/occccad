package geometry

import (
	"bytes"
	"encoding/json"
	"errors"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
)

func TestAssemblyReplayPreservesNumericalInputAndFailure(t *testing.T) {
	budget := uint64(0)
	input := &workerv1.SolveAssemblyRequest{RequestId: "preview/fixture", LengthScale: 1, AngleScale: 1,
		Bodies:        []*workerv1.AssemblyBody{{Id: "a", InitialPose: &workerv1.RigidPose{Translation: &workerv1.Vec3{X: -252.55719832993879}, Rotation: &workerv1.Quaternion{Z: 0.41411770542833365, W: 0.9102233385553086}}}},
		SolverProfile: &workerv1.AssemblySolverProfile{SchemaVersion: 2, MaxPreferenceIterations: &budget}}
	result := &workerv1.SolveAssemblyResponse{Status: "MAX_ITERATIONS", SolverBuild: "fixture-build"}
	data, err := makeAssemblyReplay(input, result, nil)
	if err != nil {
		t.Fatal(err)
	}
	var file AssemblyReplay
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	var restored workerv1.SolveAssemblyRequest
	if err = protojson.Unmarshal(file.Request, &restored); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, &restored) {
		t.Fatal("numerical request changed in round trip")
	}
	if file.Schema != AssemblyReplaySchema || len(file.Result) == 0 {
		t.Fatal("missing replay metadata or failure evidence")
	}
	data, err = makeAssemblyReplay(input, nil, errors.New("deadline"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &file); err != nil || file.TransportError != "deadline" {
		t.Fatalf("transport failure missing: %s", data)
	}
}

func TestAssemblyReplayFreezesEffectiveDefaultsWithoutMutatingRequest(t *testing.T) {
	input := &workerv1.SolveAssemblyRequest{LengthScale: 1, AngleScale: 1}
	result := &workerv1.SolveAssemblyResponse{Status: "CONVERGED", EffectiveSolverProfile: &workerv1.AssemblySolverProfile{SchemaVersion: 2, MaxIterations: 100, LengthTolerance: 1e-7}}
	data, err := makeAssemblyReplay(input, result, nil)
	if err != nil {
		t.Fatal(err)
	}
	var file AssemblyReplay
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	var restored workerv1.SolveAssemblyRequest
	if err = protojson.Unmarshal(file.Request, &restored); err != nil {
		t.Fatal(err)
	}
	if input.SolverProfile != nil || !proto.Equal(restored.SolverProfile, result.EffectiveSolverProfile) {
		t.Fatal("defaults were not frozen independently of the input")
	}
}

func TestAssemblyCompilerCompactsProductionAndReplayIdentifiers(t *testing.T) {
	large := strings.Repeat("persistent-reference-snapshot/", 1000)
	input := &workerv1.SolveAssemblyRequest{RequestId: "replay", Bodies: []*workerv1.AssemblyBody{{Id: "occurrence", InitialPose: &workerv1.RigidPose{Rotation: &workerv1.Quaternion{W: 1}}}}, Geometry: []*workerv1.AssemblyGeometry{{Id: large, BodyId: "occurrence", Kind: "CYLINDER", Radius: 12.5, Origin: &workerv1.Vec3{X: 3.141592653589793}, Direction: &workerv1.Vec3{Z: 1}}}, Constraints: []*workerv1.AssemblyConstraint{{Id: "constraint", First: &workerv1.AssemblyGeometryRef{BodyId: "occurrence", GeometryId: large}, AngleReference: &workerv1.AssemblyGeometryRef{BodyId: "occurrence", GeometryId: large}}}}
	before := proto.Clone(input)
	bodies, values, cs, opts, e := AssemblyInputFromRequest(input)
	if e != nil {
		t.Fatal(e)
	}
	compiled, e := CompileAssemblyRequest(input.RequestId, bodies, values, cs, opts)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := makeAssemblyReplay(compiled, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(raw) > 4096 || bytes.Contains(raw, []byte("persistent-reference-snapshot")) {
		t.Fatal("business snapshot leaked into numerical replay")
	}
	var replay AssemblyReplay
	json.Unmarshal(raw, &replay)
	var restored workerv1.SolveAssemblyRequest
	if e = protojson.Unmarshal(replay.Request, &restored); e != nil {
		t.Fatal(e)
	}
	id := restored.Geometry[0].Id
	if id != restored.Constraints[0].First.GeometryId || id != restored.Constraints[0].AngleReference.GeometryId {
		t.Fatal("replay references disconnected")
	}
	restored.Geometry[0].Id = large
	restored.Constraints[0].First.GeometryId = large
	restored.Constraints[0].AngleReference.GeometryId = large
	if restored.Geometry[0].Radius != input.Geometry[0].Radius || restored.Geometry[0].Origin.X != input.Geometry[0].Origin.X || !proto.Equal(before, input) {
		t.Fatal("numeric input or live stable references changed")
	}
}

package geometry

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestAssemblyCompiledIdentityOwnershipOrderAndIsolation(t *testing.T) {
	large := strings.Repeat("occurrence/persistent-selection/evidence/", 500)
	zero := 0.0
	input := []AssemblyGeometry{{ID: large, BodyID: "b", Kind: "AXIS", ParameterStart: &zero}, {ID: large, BodyID: "a", Kind: "AXIS"}}
	cs := []AssemblyConstraint{{ID: "angle", FirstBodyID: "a", FirstGeometryID: large, SecondBodyID: "b", SecondGeometryID: large, AngleReferenceBodyID: "a", AngleReferenceGeometryID: large}}
	original, _ := json.Marshal([]any{input, cs})
	values, compiled, ids, err := CompileAssemblyInput(input, cs)
	if err != nil {
		t.Fatal(err)
	}
	if values[0].BodyID != "a" || values[0].ID == values[1].ID || len(values[0].ID) > 8 || compiled[0].FirstGeometryID == compiled[0].SecondGeometryID || ids[AssemblyGeometryKey{"a", large}] != compiled[0].AngleReferenceGeometryID {
		t.Fatal("occurrence or third-axis identity lost")
	}
	again, againCS, _, err := CompileAssemblyInput(values, compiled)
	if err != nil || !reflect.DeepEqual(values, again) || !reflect.DeepEqual(compiled, againCS) {
		t.Fatal("compiler is not idempotent", err)
	}
	after, _ := json.Marshal([]any{input, cs})
	if string(original) != string(after) {
		t.Fatal("compiler mutated persistent references")
	}
	bad := append([]AssemblyConstraint(nil), cs...)
	bad[0].FirstBodyID = "missing"
	if _, _, _, err := CompileAssemblyInput(input, bad); err == nil {
		t.Fatal("accepted wrong body endpoint")
	}
	duplicate := append(append([]AssemblyGeometry(nil), input...), input[0])
	if _, _, _, err := CompileAssemblyInput(duplicate, cs); err == nil {
		t.Fatal("accepted duplicate endpoint")
	}
}
func TestAssemblyNumericalAdapterOptionalFieldsRoundTrip(t *testing.T) {
	bodies, values, cs := replayBenchmarkInput()
	values = values[:3]
	bodies = bodies[:3]
	cs = cs[:2]
	zero := 0.0
	pref := uint64(0)
	values[0].ParameterStart = &zero
	values[0].ParameterEnd = &zero
	p := AssemblyPose{Translation: [3]float64{math.SmallestNonzeroFloat64, -0.125, 1.2345678901234567}, Rotation: [4]float64{0, 0, 0, 1}}
	bodies[0].InitialGuess = &p
	cs[0].Kind = "ANGLE"
	cs[0].Value = 5.789123456789
	cs[0].AngleReferenceBodyID = values[2].BodyID
	cs[0].AngleReferenceGeometryID = values[2].ID
	cs[0].AngleBranchState = &AssemblyAngleBranchState{WrappedAngle: 0.9, UnwrappedAngle: 1.2, Winding: math.MaxInt64}
	cs[0].AngleReferenceDirection = &[3]float64{0, 0, 1}
	cs[0].FixedPose = &p
	options := AssemblySolveOptions{SolverProfile: &AssemblySolverProfile{SchemaVersion: 2, MaxPreferenceIterations: &pref}, Debug: &AssemblyDebugOptions{CaptureEvaluations: true, ByteBudget: 4096}, Intent: &AssemblySolveIntent{MovingBodyIDs: []string{bodies[0].ID}, ReferenceBodyIDs: []string{bodies[1].ID}}, DragTarget: &AssemblyDragTarget{BodyID: bodies[0].ID, TargetPose: p, FrameRotation: p.Rotation, TargetSequence: math.MaxUint64, TranslationComponents: [3]bool{true}, HoldRotationComponents: [3]bool{false, true}}}
	request, err := CompileAssemblyRequest("fixture", bodies, values, cs, options)
	if err != nil {
		t.Fatal(err)
	}
	before := proto.Clone(request)
	b, g, c, o, err := AssemblyInputFromRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := CompileAssemblyRequest("fixture", b, g, c, o)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(request, restored) {
		t.Fatal("adapter lost optional/numeric fields", request, restored)
	}
	raw, err := makeAssemblyReplay(request, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var file AssemblyReplay
	_ = json.Unmarshal(raw, &file)
	var wire workerv1.SolveAssemblyRequest
	if err := protojson.Unmarshal(file.Request, &wire); err != nil || !proto.Equal(request, &wire) {
		t.Fatal("file lost numerical precision or optional presence", err)
	}
	*g[0].ParameterStart = 12
	*o.SolverProfile.MaxPreferenceIterations = 20
	b[0].InitialGuess.Translation[0] = 99
	if !proto.Equal(before, request) || *values[0].ParameterStart != 0 || *options.SolverProfile.MaxPreferenceIterations != 0 {
		t.Fatal("mutable alias crossed frozen adapter boundary")
	}
}
func TestAssemblyReplayOutcomeDoesNotOverclaimUnsatisfiability(t *testing.T) {
	for _, status := range []string{"UNSATISFIED", "MAX_ITERATIONS", "NUMERICAL_FAILURE", "INCONSISTENT"} {
		if got := ClassifyAssemblyReplay(&workerv1.SolveAssemblyResponse{Status: status}, nil); got != "NUMERICAL_NONCONVERGENCE" {
			t.Fatal(status, got)
		}
	}
	result := &workerv1.SolveAssemblyResponse{Status: "INCONSISTENT", Diagnostics: []*workerv1.AssemblySolveDiagnostic{{Code: "GROUNDED_CONTRADICTION"}}}
	if ClassifyAssemblyReplay(result, nil) != "PROVEN_CONTRADICTION" {
		t.Fatal("explicit ground certificate discarded")
	}
}

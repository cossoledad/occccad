package workspace

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"os"
	"os/exec"
	"reflect"
	"testing"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func motionWorker(t *testing.T) *geometry.Client {
	t.Helper()
	bin := os.Getenv("OCCCCAD_TEST_GEOMETRY_WORKER")
	if bin == "" {
		t.Fatal("matching-source Worker binary is required")
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	l.Close()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "OCCCCAD_GEOMETRY_WORKER_LISTEN="+addr, "OCCCCAD_DATA_DIR="+t.TempDir())
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	c, e := geometry.Open(addr)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, stop := context.WithTimeout(t.Context(), 100*time.Millisecond)
		_, e = c.Ping(ctx)
		stop()
		if e == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(e)
		}
		time.Sleep(25 * time.Millisecond)
	}
	return c
}
func jointEnd(id string, x, y float64) JointEndpoint {
	return JointEndpoint{InstanceID: id, Frame: InstancePose{Translation: [3]float64{x, y, 0}, Rotation: [4]float64{0, 0, 0, 1}}}
}
func motionFixture(t *testing.T, closed bool, kind string) MotionSnapshot {
	t.Helper()
	ids := []string{"ground", "crank"}
	angles := []float64{0, math.Pi / 3}
	origins := [][3]float64{{}, {}}
	joints := []MechanismJoint{{ID: "g", Kind: "GROUND", First: jointEnd("ground", 0, 0), Direction: 1}, {ID: "drive", Kind: kind, First: jointEnd("ground", 0, 0), Second: ptrEnd(jointEnd("crank", 0, 0)), Direction: 1, Zero: MotionQuantity{Unit: "rad"}}}
	if kind == "PRISMATIC" {
		angles[1] = 0
		joints[1].Zero.Unit = "mm"
	}
	if closed {
		px, py := 20*math.Cos(angles[1]), 20*math.Sin(angles[1])
		dx, dy := 40-px, -py
		dist := math.Hypot(dx, dy)
		a := (1600 - 900 + dist*dist) / (2 * dist)
		h := math.Sqrt(1600 - a*a)
		cx, cy := px+a*dx/dist-h*dy/dist, py+a*dy/dist+h*dx/dist
		ids = append(ids, "coupler", "rocker")
		origins = append(origins, [3]float64{px, py, 0}, [3]float64{40, 0, 0})
		angles = append(angles, math.Atan2(cy-py, cx-px), math.Atan2(cy, cx-40))
		for _, j := range []MechanismJoint{{ID: "j2", First: jointEnd("crank", 20, 0), Second: ptrEnd(jointEnd("coupler", 0, 0))}, {ID: "j3", First: jointEnd("coupler", 40, 0), Second: ptrEnd(jointEnd("rocker", 30, 0))}, {ID: "j4", First: jointEnd("ground", 40, 0), Second: ptrEnd(jointEnd("rocker", 0, 0))}} {
			j.Kind = "REVOLUTE"
			j.Zero.Unit = "rad"
			j.Direction = 1
			joints = append(joints, j)
		}
	}
	m := Mechanism{ID: "m", Name: "mechanism", UnitIDs: ids, Joints: joints}
	s := MotionStudy{ID: "s", Name: "linear", MechanismID: "m", DriverJointID: "drive", Start: MotionQuantity{Value: 20, Unit: "deg"}, End: MotionQuantity{Value: 120, Unit: "deg"}, Frames: 9, DurationSeconds: 2, BudgetMS: 30000, Clearance: MotionQuantity{Unit: "mm"}}
	if kind == "PRISMATIC" {
		s.Start = MotionQuantity{Value: -5, Unit: "mm"}
		s.End = MotionQuantity{Value: 15, Unit: "mm"}
	}
	f := AssemblySolveManifest{}
	for i, id := range ids {
		f.Bodies = append(f.Bodies, geometry.AssemblyBody{ID: id, Pose: geometry.AssemblyPose{Translation: origins[i], Rotation: [4]float64{0, 0, math.Sin(angles[i] / 2), math.Cos(angles[i] / 2)}}})
	}
	eq, e := compileMechanism(f, m, s)
	if e != nil {
		t.Fatal(e)
	}
	snap := MotionSnapshot{Schema: 1, Policy: motionPolicy, Mechanism: m, Study: s, Equations: eq, SolverProfile: defaultAssemblySolverProfile()}
	snap.Digest = motionDigest(snap)
	return snap
}
func ptrEnd(v JointEndpoint) *JointEndpoint { return &v }
func TestMotionHardDriverClosedLoopAndLimits(t *testing.T) {
	service := New(nil, motionWorker(t))
	for _, c := range []struct {
		name, kind string
		closed     bool
	}{{"revolute", "REVOLUTE", false}, {"prismatic", "PRISMATIC", false}, {"fourbar", "REVOLUTE", true}} {
		t.Run(c.name, func(t *testing.T) {
			s := motionFixture(t, c.closed, c.kind)
			before, _ := json.Marshal(s)
			r, e := service.RunMotionStudy(t.Context(), s, nil)
			if e != nil || !r.Completed || len(r.Frames) != s.Study.Frames {
				t.Fatalf("run %s frames %d failure %+v error %v", r.Status, len(r.Frames), r.Failure, e)
			}
			for _, f := range r.Frames {
				if !f.KinematicValid {
					t.Fatal("unqualified frame")
				}
				if _, e = verifyMechanismJoints(s.Mechanism, f.UnitPoses, nil, s.Study.DriverJointID, f.DriverValue); e != nil {
					t.Fatal(e)
				}
			}
			after, _ := json.Marshal(s)
			if string(before) != string(after) {
				t.Fatal("mutated frozen input")
			}
			lo := MotionQuantity{Value: 75, Unit: "deg"}
			if c.kind == "PRISMATIC" {
				lo = MotionQuantity{Value: 7, Unit: "mm"}
			}
			s.Mechanism.Joints[1].Upper = &lo
			s.Digest = motionDigest(s)
			r, e = service.RunMotionStudy(t.Context(), s, nil)
			if e != nil || r.Completed || r.Status != "LIMIT_REACHED" || len(r.Frames) == 0 {
				t.Fatalf("limits: %s frames %d %+v %v", r.Status, len(r.Frames), r.Failure, e)
			}
		})
	}
}
func TestMotionFullTurnsCancelAndUnsupportedDof(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, false, "REVOLUTE")
	s.Study.Start.Value = 60
	s.Study.End.Value = 780
	s.Study.Frames = 13
	s.Digest = motionDigest(s)
	r, e := service.RunMotionStudy(t.Context(), s, nil)
	if e != nil || !r.Completed {
		t.Fatalf("full turn: %s %+v %v", r.Status, r.Failure, e)
	}
	if math.Abs(r.Frames[12].Coordinates["drive"]-780*math.Pi/180) > 1e-7 {
		t.Fatal("lost winding")
	}
	r, e = service.RunMotionStudy(t.Context(), s, func(n int) error {
		if n == 2 {
			return context.Canceled
		}
		return nil
	})
	if e != nil || r.Completed || len(r.Frames) != 2 {
		t.Fatal("partial frames not retained", r, e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, e = service.RunMotionStudy(ctx, s, nil)
	if e != nil || r.Status != "CANCELED" {
		t.Fatal(r, e)
	}
	s.Equations.Constraints = nil
	s.Digest = motionDigest(s)
	r, e = service.RunMotionStudy(t.Context(), s, nil)
	if e != nil || r.Status != "UNSUPPORTED_DOF" {
		t.Fatal(r, e)
	}
}
func TestMotionDefinitionHistoryAndEquationAdmission(t *testing.T) {
	s := motionFixture(t, true, "REVOLUTE")
	model := ProductModel{}
	for _, b := range s.Equations.Bodies {
		model.Instances = append(model.Instances, ProductInstance{ID: b.ID, Translation: b.Pose.Translation, Rotation: b.Pose.Rotation})
	}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{s.Mechanism}, Studies: []MotionStudy{s.Study}}
	raw, _ := json.Marshal(model)
	payload, _ := json.Marshal(k)
	after, changes, e := applyKinematicsDefinition(raw, payload)
	if e != nil {
		t.Fatal(e)
	}
	var next ProductModel
	json.Unmarshal(after, &next)
	if !kinematicsOnlyHistory(model, next, changes) || !reflect.DeepEqual(model.Instances, next.Instances) {
		t.Fatal("definition-only invariant")
	}
	values, e := modelValues("PRODUCT", after, changes)
	if e != nil {
		t.Fatal(e)
	}
	if len(values) != 1 {
		t.Fatal(values)
	}
	active := AssemblyConstraint{ID: "missing", Mode: "DRIVING", EvaluationStatus: modelcore.AssemblyConstraintNotUpdated}
	_, e = compileMechanism(AssemblySolveManifest{Bodies: s.Equations.Bodies, Definitions: []AssemblyConstraint{active}}, s.Mechanism, s.Study)
	if e == nil {
		t.Fatal("silently omitted hard equation")
	}
	k.Studies[0].Start.Unit = "mm"
	if validateKinematics(model, k) == nil {
		t.Fatal("unit mismatch accepted")
	}
}

func TestMotionRigidAndDirectedZero(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, false, "REVOLUTE")
	s.Mechanism.Joints[1].Direction = -1
	s.Mechanism.Joints[1].Zero = MotionQuantity{Value: 45, Unit: "deg"}
	s.Study.Start.Value = -15
	s.Study.End.Value = -375
	body := s.Equations.Bodies[1]
	body.ID = "rigid-child"
	nominal := append(append([]geometry.AssemblyBody(nil), s.Equations.Bodies...), body)
	s.Mechanism.UnitIDs = append(s.Mechanism.UnitIDs, body.ID)
	end := jointEnd(body.ID, 0, 0)
	s.Mechanism.Joints = append(s.Mechanism.Joints, MechanismJoint{ID: "rigid", Name: "rigid member", Kind: "RIGID", First: jointEnd("crank", 0, 0), Second: &end, Direction: 1})
	var e error
	s.Equations, e = compileMechanism(AssemblySolveManifest{Bodies: nominal}, s.Mechanism, s.Study)
	if e != nil {
		t.Fatal(e)
	}
	s.Digest = motionDigest(s)
	r, e := service.RunMotionStudy(t.Context(), s, nil)
	if e != nil || !r.Completed {
		t.Fatalf("rigid/zero %s %+v %v", r.Status, r.Failure, e)
	}
	for _, f := range r.Frames {
		if math.Abs(f.Coordinates["drive"]-f.DriverValue) > 1e-7 {
			t.Fatal("direction/zero lost")
		}
		a, b := f.UnitPoses["crank"], f.UnitPoses[body.ID]
		if !conflictPoseClose(geometry.AssemblyPose(a), geometry.AssemblyPose(b), 1e-7, 1e-8) {
			t.Fatal("rigid relative pose changed")
		}
	}
}

func TestMotionRetainedEquationAndExplicitDriverReplacement(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, false, "REVOLUTE")
	coordinate := s.Equations.Driver
	coordinate.ID = "existing-design-angle"
	coordinate.Value = math.Pi / 3
	frozen := AssemblySolveManifest{Bodies: s.Equations.Bodies, Definitions: []AssemblyConstraint{{ID: coordinate.ID, Mode: "DRIVING"}}}
	for _, g := range s.Equations.Geometry {
		if g.ID == coordinate.FirstGeometryID || g.ID == coordinate.SecondGeometryID || g.ID == coordinate.AngleReferenceGeometryID {
			old := g.ID
			g.ID = "source/" + old
			frozen.Geometry = append(frozen.Geometry, g)
			if old == coordinate.FirstGeometryID {
				coordinate.FirstGeometryID = g.ID
			}
			if old == coordinate.SecondGeometryID {
				coordinate.SecondGeometryID = g.ID
			}
			if old == coordinate.AngleReferenceGeometryID {
				coordinate.AngleReferenceGeometryID = g.ID
			}
		}
	}
	frozen.Constraints = []geometry.AssemblyConstraint{coordinate}
	var e error
	s.Equations, e = compileMechanism(frozen, s.Mechanism, s.Study)
	if e != nil || !reflect.DeepEqual(s.Equations.RetainedConstraintIDs, []string{coordinate.ID}) {
		t.Fatal(s.Equations, e)
	}
	s.Digest = motionDigest(s)
	r, e := service.RunMotionStudy(t.Context(), s, nil)
	if e != nil || r.Completed || r.Status != "UNSUPPORTED_DOF" || len(r.Frames) != 0 {
		t.Fatal("fixed design angle was silently disabled", r, e)
	}
	s.Study.ReplaceDriverConstraintID = coordinate.ID
	s.Equations, e = compileMechanism(frozen, s.Mechanism, s.Study)
	if e != nil || s.Equations.ReplacedConstraintID != coordinate.ID || len(s.Equations.RetainedConstraintIDs) != 0 {
		t.Fatal(s.Equations, e)
	}
	s.Digest = motionDigest(s)
	r, e = service.RunMotionStudy(t.Context(), s, nil)
	if e != nil || !r.Completed {
		t.Fatal("equivalent explicit replacement failed", r.Status, r.Failure, e)
	}
	frozen.Geometry[0].Origin[0] = 1
	if _, e = compileMechanism(frozen, s.Mechanism, s.Study); e == nil {
		t.Fatal("different coordinate accepted as a replacement")
	}
}

func TestMotionBudgetRetainsCompletedFrame(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, false, "REVOLUTE")
	s.Study.BudgetMS = 100
	s.Digest = motionDigest(s)
	r, e := service.RunMotionStudy(t.Context(), s, func(n int) error {
		if n == 1 {
			<-time.After(150 * time.Millisecond)
		}
		return nil
	})
	if e != nil || r.Completed || r.Status != "BUDGET_EXHAUSTED" || len(r.Frames) != 1 || !r.Frames[0].KinematicValid {
		t.Fatal(r.Status, len(r.Frames), r.Failure, e)
	}
}

func TestMotionConcurrentFrozenRunsIndependent(t *testing.T) {
	service := New(nil, motionWorker(t))
	s := motionFixture(t, true, "REVOLUTE")
	s.Study.Frames = 3
	s.Digest = motionDigest(s)
	before, _ := json.Marshal(s)
	type answer struct {
		run MotionRun
		err error
	}
	results := make(chan answer, 2)
	for range 2 {
		go func() { r, e := service.RunMotionStudy(t.Context(), s, nil); results <- answer{r, e} }()
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || !a.run.Completed || !b.run.Completed || !reflect.DeepEqual(a.run.Frames, b.run.Frames) {
		t.Fatal(a.run.Status, a.run.Failure, a.err, b.run.Status, b.run.Failure, b.err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("concurrent jobs mutated frozen input")
	}
}

type motionFailureWorker struct {
	workerv1.UnimplementedGeometryWorkerServer
	code codes.Code
}

func (s motionFailureWorker) SolveAssembly(context.Context, *workerv1.SolveAssemblyRequest) (*workerv1.SolveAssemblyResponse, error) {
	if s.code != codes.OK {
		return nil, status.Error(s.code, "injected Worker failure")
	}
	return &workerv1.SolveAssemblyResponse{SolverBuild: "fixture", Status: "MAX_ITERATIONS", NormalizedResidual: math.Inf(1)}, nil
}
func TestMotionExecutionInputAndNonFiniteFailures(t *testing.T) {
	for _, c := range []struct {
		code codes.Code
		want string
	}{{codes.Unavailable, "EXECUTION_FAILURE"}, {codes.InvalidArgument, "INPUT_ERROR"}, {codes.OK, "KINEMATIC_FAILED"}} {
		t.Run(c.want, func(t *testing.T) {
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			server := grpc.NewServer()
			defer server.Stop()
			workerv1.RegisterGeometryWorkerServer(server, motionFailureWorker{code: c.code})
			go server.Serve(l)
			client, e := geometry.Open(l.Addr().String())
			if e != nil {
				t.Fatal(e)
			}
			defer client.Close()
			r, e := New(nil, client).RunMotionStudy(t.Context(), motionFixture(t, false, "REVOLUTE"), nil)
			if e != nil || r.Status != c.want || r.Completed || len(r.Frames) != 0 || r.SolveCalls != 1 {
				t.Fatal(r.Status, r.Failure, r.SolveCalls, e)
			}
			if _, e = json.Marshal(r); e != nil {
				t.Fatal("failure cannot be saved", e)
			}
		})
	}
	if _, e := motionValue(MotionQuantity{Value: 1e308, Unit: "m"}, false); e == nil {
		t.Fatal("conversion overflow accepted")
	}
}

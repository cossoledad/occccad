package control

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/performance"
	"github.com/occccad/occccad/internal/workspace"
	"google.golang.org/protobuf/proto"
)

// Opt-in, serial, real Router/Worker measurements. Setup is outside sample
// timing. Do not run alongside builds, benchmarks or parallel tests.
func TestAssemblyPerformance(t *testing.T) {
	if os.Getenv("OCCCCAD_RUN_ASSEMBLY_PERFORMANCE") != "1" {
		t.Skip("opt-in performance measurement")
	}
	n := 5
	if raw := os.Getenv("OCCCCAD_PERFORMANCE_SAMPLES"); raw != "" {
		var err error
		n, err = strconv.Atoi(raw)
		if err != nil || n < 2 || n > 100 {
			t.Fatal("samples must be 2..100")
		}
	}
	f := newCompositionControlFixture(t)
	q := [4]float64{0, 0, 0, 1}
	log := func(v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		t.Log("ASSEMBLY_PERFORMANCE " + string(raw))
	}
	for _, scene := range []string{"plane5", "plane15", "plane30", "single", "connected50-200", "independent50", "group-contact", "grounded-failure", "budget"} {
		t.Run(scene, func(t *testing.T) {
			bodies, values, constraints := performanceAssemblyInput(scene)
			opts := geometry.AssemblySolveOptions{DisableConflictProbes: true, SolverProfile: &geometry.AssemblySolverProfile{SchemaVersion: 2}}
			if scene[:min(5, len(scene))] == "plane" {
				opts.Intent = &geometry.AssemblySolveIntent{MovingBodyIDs: []string{bodies[len(bodies)-1].ID}, ReferenceBodyIDs: []string{"b1"}, PreferencePolicy: "MOVE_FIRST_MINIMIZE_REFERENCE"}
			} else {
				opts.AffectedBodyIDs = []string{"b0"}
				opts.DragTarget = &geometry.AssemblyDragTarget{BodyID: "b0", TargetPose: bodies[0].Pose, FrameRotation: q, TranslationComponents: [3]bool{true, true, true}}
			}
			if scene == "budget" {
				limit := uint64(1)
				opts.SolverProfile.MaxPreferenceIterations = &limit
			}
			for sample := 1; sample <= n; sample++ {
				if opts.DragTarget != nil {
					opts.DragTarget.TargetSequence = uint64(sample)
					opts.DragTarget.TargetPose.Translation[0] = .1 * float64(sample)
				}
				ctx, recorder := performance.WithRecorder(t.Context())
				start := time.Now()
				result, err := f.client.SolveAssemblyWithOptions(ctx, fmt.Sprintf("perf-%s-%d", scene, sample), bodies, values, constraints, opts)
				elapsed := time.Since(start)
				if err != nil {
					t.Fatal(err)
				}
				failure := scene == "grounded-failure" || scene == "budget"
				if opts.DragTarget != nil {
					if result.Interaction == nil || result.Interaction.EligibleForCommit == failure {
						t.Fatalf("unexpected qualification: %+v", result.Interaction)
					}
				} else if result.Status != "CONVERGED" {
					t.Fatal(result.Status, result.Diagnostic)
				}
				if !failure {
					assertPerformanceGeometry(t, scene, bodies, result, opts.DragTarget)
				}
				components, rank, dof, iterations := len(result.Components), uint64(0), uint64(0), uint64(0)
				for _, c := range result.Components {
					rank += c.JacobianRank
					dof += c.RelativeDof + c.GaugeDof
					iterations += c.Preference.Iterations
				}
				// Serialization calibration is separate, never added to RPC elapsed.
				request, err := geometry.CompileAssemblyRequest("probe", bodies, values, constraints, opts)
				if err != nil {
					t.Fatal(err)
				}
				marshalStart := time.Now()
				wire, err := proto.Marshal(request)
				marshalMS := float64(time.Since(marshalStart).Nanoseconds()) / 1e6
				if err != nil {
					t.Fatal(err)
				}
				log(map[string]any{"kind": "router", "scene": scene, "sample": sample, "cold": sample == 1, "bodies": len(bodies), "constraints": len(constraints), "components": components, "rank": rank, "dof": dof, "hard_iterations": result.Iterations, "preference_iterations": iterations, "valid": true, "status": result.Status, "total_ms": float64(elapsed.Nanoseconds()) / 1e6, "phases": recorder.SnapshotMilliseconds(), "marshal_probe_ms": marshalMS, "request_bytes": len(wire)})
				if !failure {
					for i := range bodies {
						pose := result.Bodies[i].Pose
						bodies[i].InitialGuess = &pose
					}
				}
			}
		})
	}
	// Real Session work includes two Head/EditContext checks, frozen input,
	// digest, RPC, model update and output JSON. The numerical manifest selects
	// the complete connected component, while the Product has 50 occurrences.
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("session-nested-%v", nested), func(t *testing.T) {
			part, err := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PART", Name: "performance part"})
			if err != nil {
				t.Fatal(err)
			}
			create := func(name string) workspace.DocumentView {
				p, e := f.service.CreateDocument(t.Context(), workspace.CreateDocumentRequest{ActorID: f.actor, Type: "PRODUCT", Name: name})
				if e != nil {
					t.Fatal(e)
				}
				return p
			}
			insert := func(p workspace.DocumentView, child string, count int) workspace.DocumentView {
				for i := 0; i < count; i++ {
					var e error
					p, e = f.service.ApplyCommand(t.Context(), p.Document.ID, workspace.CommandRequest{Type: "INSERT_INSTANCE", ActorID: f.actor, RequestID: fmt.Sprintf("%s-insert-%d", p.Document.ID, i), ReferencedDocumentID: child})
					if e != nil {
						t.Fatal(e)
					}
				}
				return p
			}
			p := insert(create("performance leaf Product"), part.Document.ID, 50)
			var edit *workspace.AssemblyInteractionEditContext
			var root workspace.DocumentView
			if nested {
				parent := insert(create("performance parent"), p.Document.ID, 1)
				root = insert(create("performance root"), parent.Document.ID, 2)
				canonical := root.Product.Instances[0].ID + "/" + parent.Product.Instances[0].ID
				design, e := f.service.GetProductDesignSession(t.Context(), root.Document.ID, canonical)
				if e != nil {
					t.Fatal(e)
				}
				edit = &workspace.AssemblyInteractionEditContext{RootDocumentID: root.Document.ID, RootRevisionID: root.Document.VersionID, ActiveDocumentID: p.Document.ID, ActiveRevisionID: p.Document.VersionID, InstancePath: design.ActiveInstancePath}
			}
			ctx, recorder := performance.WithRecorder(t.Context())
			start := time.Now()
			session, e := f.service.BeginAssemblyInteraction(ctx, p.Document.ID, f.actor, workspace.AssemblyInteractionBegin{BaseRevisionID: p.Document.VersionID, InstanceID: p.Product.Instances[0].ID, FrameRotation: q, EditContext: edit})
			if e != nil {
				t.Fatal(e)
			}
			log(map[string]any{"kind": "session-begin", "nested": nested, "total_ms": float64(time.Since(start).Nanoseconds()) / 1e6, "phases": recorder.SnapshotMilliseconds()})
			defer f.service.CancelAssemblyInteraction(p.Document.ID, f.actor, session.SessionID)
			for sample := 1; sample <= n; sample++ {
				ctx, recorder := performance.WithRecorder(t.Context())
				start := time.Now()
				target := geometry.AssemblyDragTarget{BodyID: session.BodyID, FrameRotation: q, TargetPose: geometry.AssemblyPose{Translation: [3]float64{.1 * float64(sample), 0, 0}, Rotation: q}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: uint64(sample)}
				frame, e := f.service.UpdateAssemblyInteraction(ctx, p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: uint64(sample), Target: target})
				elapsed := time.Since(start)
				if e != nil || frame.Interaction == nil || !frame.Interaction.EligibleForCommit || frame.PreviewID != "" {
					t.Fatal(frame, e)
				}
				for _, pose := range frame.InstancePoses {
					if pose.InstanceID == session.BodyID {
						if math.Abs(pose.Translation[0]-target.TargetPose.Translation[0]) > 1e-7 {
							t.Fatal(pose)
						}
					} else if pose.Translation != [3]float64{} || pose.Rotation != q {
						t.Fatal("unrelated pose changed", pose)
					}
				}
				start = time.Now()
				raw, e := json.Marshal(frame)
				if e != nil {
					t.Fatal(e)
				}
				jsonMS := float64(time.Since(start).Nanoseconds()) / 1e6
				log(map[string]any{"kind": "session-update", "nested": nested, "sample": sample, "cold": sample == 1, "bodies": 50, "components": len(frame.AssemblyComponents), "valid": true, "iterations": frame.Interaction.Iterations, "total_ms": float64(elapsed.Nanoseconds()) / 1e6, "phases": recorder.SnapshotMilliseconds(), "response_json_ms": jsonMS, "response_bytes": len(raw)})
			}
			current, e := f.service.GetDocument(t.Context(), p.Document.ID)
			if e != nil || current.Document.VersionID != p.Document.VersionID {
				t.Fatal("preview changed Head", e)
			}
			if nested {
				_, e = f.service.ApplyCommand(t.Context(), root.Document.ID, workspace.CommandRequest{Type: "RENAME_INSTANCE", ActorID: f.actor, RequestID: "invalidate-" + root.Document.ID, InstanceID: root.Product.Instances[0].ID, Name: "changed occurrence"})
				if e != nil {
					t.Fatal(e)
				}
				target := geometry.AssemblyDragTarget{BodyID: session.BodyID, FrameRotation: q, TargetPose: geometry.AssemblyPose{Rotation: q}, TranslationComponents: [3]bool{true, true, true}, TargetSequence: uint64(n + 1)}
				if _, e = f.service.UpdateAssemblyInteraction(t.Context(), p.Document.ID, f.actor, workspace.AssemblyInteractionUpdate{SessionID: session.SessionID, Sequence: uint64(n + 1), Target: target}); e == nil {
					t.Fatal("changed root accepted")
				}
			}
		})
	}
}

func performanceAssemblyInput(scene string) ([]geometry.AssemblyBody, []geometry.AssemblyGeometry, []geometry.AssemblyConstraint) {
	q := [4]float64{0, 0, 0, 1}
	n := 1
	switch scene {
	case "connected50-200", "independent50", "budget":
		n = 50
	case "group-contact":
		n = 4
	case "grounded-failure":
		n = 2
	case "plane5":
		n = 5
	case "plane15":
		n = 15
	case "plane30":
		n = 30
	}
	bodies := make([]geometry.AssemblyBody, n)
	var values []geometry.AssemblyGeometry
	var constraints []geometry.AssemblyConstraint
	for i := range bodies {
		id := fmt.Sprintf("b%d", i)
		bodies[i] = geometry.AssemblyBody{ID: id, Pose: geometry.AssemblyPose{Translation: [3]float64{10 * float64(i), 0, 0}, Rotation: q}}
		if len(scene) > 5 && scene[:5] == "plane" {
			bodies[i].Pose.Translation = [3]float64{0, 0, float64(i)}
			values = append(values, geometry.AssemblyGeometry{ID: "plane", BodyID: id, Kind: "PLANE", Direction: [3]float64{0, 0, 1}, LengthUnit: "mm"})
			if i == 0 {
				constraints = append(constraints, geometry.AssemblyConstraint{ID: "fix", Kind: "FIX", FirstBodyID: id})
			} else {
				constraints = append(constraints, geometry.AssemblyConstraint{ID: fmt.Sprintf("mate%d", i), Kind: "COINCIDENT", FirstBodyID: id, FirstGeometryID: "plane", SecondBodyID: fmt.Sprintf("b%d", i-1), SecondGeometryID: "plane", DirectionRelation: "SAME"})
			}
		}
		if scene == "connected50-200" || scene == "budget" {
			for _, kind := range []string{"p", "x", "z"} {
				g := geometry.AssemblyGeometry{ID: kind, BodyID: id, Kind: "AXIS", Direction: [3]float64{0, 0, 1}, LengthUnit: "mm"}
				if kind == "p" {
					g.Kind = "POINT"
					g.Origin = [3]float64{-10 * float64(i), 0, 0}
				}
				if kind == "x" {
					g.Direction = [3]float64{1, 0, 0}
				}
				values = append(values, g)
				if i > 0 {
					k := "PARALLEL"
					if kind == "p" {
						k = "COINCIDENT"
					}
					constraints = append(constraints, geometry.AssemblyConstraint{ID: fmt.Sprintf("chain%d/%s", i, kind), Kind: k, FirstBodyID: id, FirstGeometryID: kind, SecondBodyID: fmt.Sprintf("b%d", i-1), SecondGeometryID: kind, DirectionRelation: "SAME"})
				}
			}
		}
		if scene == "grounded-failure" {
			values = append(values, geometry.AssemblyGeometry{ID: "p", BodyID: id, Kind: "POINT", LengthUnit: "mm"})
			constraints = append(constraints, geometry.AssemblyConstraint{ID: "fix/" + id, Kind: "FIX", FirstBodyID: id})
		}
	}
	if scene == "connected50-200" || scene == "budget" {
		for j := 0; j < 53; j++ {
			a := 1 + j%49
			b := (j * 7) % a
			constraints = append(constraints, geometry.AssemblyConstraint{ID: fmt.Sprintf("loop%d", j), Kind: "COINCIDENT", FirstBodyID: fmt.Sprintf("b%d", a), FirstGeometryID: "p", SecondBodyID: fmt.Sprintf("b%d", b), SecondGeometryID: "p", DirectionRelation: "SAME"})
		}
	}
	if scene == "grounded-failure" {
		constraints = append(constraints, geometry.AssemblyConstraint{ID: "impossible", Kind: "COINCIDENT", FirstBodyID: "b0", FirstGeometryID: "p", SecondBodyID: "b1", SecondGeometryID: "p"})
	}
	if scene == "group-contact" {
		for i := 0; i < 3; i++ {
			bodies[i].Pose.Translation = [3]float64{3 * float64(i), 0, 2}
		}
		bodies[3].Pose.Translation = [3]float64{}
		values = []geometry.AssemblyGeometry{{ID: "plane", BodyID: "b3", Kind: "PLANE", Direction: [3]float64{0, 0, 1}, LengthUnit: "mm"}, {ID: "sphere", BodyID: "b0", Kind: "SPHERE", Radius: 2, LengthUnit: "mm"}}
		for i := 0; i < 2; i++ {
			fixed := geometry.AssemblyPose{Translation: [3]float64{-3, 0, 0}, Rotation: q}
			constraints = append(constraints, geometry.AssemblyConstraint{ID: fmt.Sprintf("group%d", i), Kind: "RIGID", FirstBodyID: fmt.Sprintf("b%d", i), SecondBodyID: fmt.Sprintf("b%d", i+1), FixedPose: &fixed})
		}
		constraints = append(constraints, geometry.AssemblyConstraint{ID: "contact", Kind: "CONTACT", FirstBodyID: "b3", FirstGeometryID: "plane", SecondBodyID: "b0", SecondGeometryID: "sphere", ContactKind: "POINT", ContactSide: "EXTERNAL", ContactBranch: 1, DirectionRelation: "SAME"}, geometry.AssemblyConstraint{ID: "ground", Kind: "FIX", FirstBodyID: "b3"})
	}
	return bodies, values, constraints
}

func assertPerformanceGeometry(t *testing.T, scene string, initial []geometry.AssemblyBody, r geometry.AssemblySolve, target *geometry.AssemblyDragTarget) {
	t.Helper()
	poses := map[string]geometry.AssemblyPose{}
	for _, b := range r.Bodies {
		poses[b.ID] = b.Pose
	}
	if len(poses) != len(initial) {
		t.Fatal("missing poses")
	}
	if target != nil {
		p := poses[target.BodyID]
		for k := range p.Translation {
			if math.Abs(p.Translation[k]-target.TargetPose.Translation[k]) > 1e-7 {
				t.Fatal("independent target mismatch", p)
			}
		}
	}
	if scene == "independent50" {
		for _, b := range initial {
			if b.ID != "b0" && poses[b.ID] != b.Pose {
				t.Fatal("unrelated component moved", b.ID)
			}
		}
	}
	if scene == "connected50-200" {
		base := poses["b0"]
		for i := 0; i < 50; i++ {
			p := poses[fmt.Sprintf("b%d", i)]
			q := p.Rotation
			v := [3]float64{-10 * float64(i), 0, 0}
			rotated := [3]float64{(1 - 2*(q[1]*q[1]+q[2]*q[2])) * v[0], 2 * (q[0]*q[1] + q[3]*q[2]) * v[0], 2 * (q[0]*q[2] - q[3]*q[1]) * v[0]}
			for k := range rotated {
				if math.Abs(p.Translation[k]+rotated[k]-base.Translation[k]) > 1e-7 {
					t.Fatal("point chain violated")
				}
			}
			if math.Abs(q[0])+math.Abs(q[1])+math.Abs(q[2]) > 1e-7 {
				t.Fatal("parallel axes violated")
			}
		}
	}
	if scene == "group-contact" {
		if math.Abs(poses["b0"].Translation[2]-2) > 1e-7 {
			t.Fatal("contact violated")
		}
		for i := 1; i < 3; i++ {
			a, b := poses["b0"], poses[fmt.Sprintf("b%d", i)]
			d := math.Sqrt(math.Pow(a.Translation[0]-b.Translation[0], 2) + math.Pow(a.Translation[1]-b.Translation[1], 2) + math.Pow(a.Translation[2]-b.Translation[2], 2))
			if math.Abs(d-3*float64(i)) > 1e-7 {
				t.Fatal("rigid relation violated")
			}
		}
	}
	if len(scene) > 5 && scene[:5] == "plane" {
		for _, p := range poses {
			if math.Abs(p.Translation[2]) > 1e-7 {
				t.Fatal("plane chain violated")
			}
		}
	}
}

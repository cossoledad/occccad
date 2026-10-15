package workspace

import (
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
)

func TestSimpleJointPersistentDefinitionsAndIndependentConversion(t *testing.T) {
	db, err := database.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "simple_joints_test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	store, err := artifact.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewWithArtifacts(db, motionWorker(t), artifact.NewService(db, store))
	actor := access.DefaultUserID
	part, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PART", Name: "joint datums", ActorID: actor})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"PRISMATIC", "CYLINDRICAL", "PLANAR", "RIGID", "SPHERICAL", "PRISMATIC_OPPOSITE", "CYLINDRICAL_OPPOSITE", "PRISMATIC_PLANE_OPPOSITE"} {
		t.Run(name, func(t *testing.T) {
			kind := strings.TrimSuffix(strings.TrimSuffix(name, "_PLANE_OPPOSITE"), "_OPPOSITE")
			view, err := svc.CreateDocument(t.Context(), CreateDocumentRequest{Type: "PRODUCT", Name: kind, ActorID: actor})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(req CommandRequest) DocumentView {
				t.Helper()
				req.ActorID = actor
				req.VersionID = view.Document.VersionID
				req.RequestID = uuid.NewString()
				v, e := svc.ApplyCommand(t.Context(), view.Document.ID, req)
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
			for range 2 {
				view = apply(CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part.Document.ID})
			}
			a, b := view.Product.Instances[0].ID, view.Product.Instances[1].ID
			view = apply(CommandRequest{Type: "MOVE_INSTANCE", InstanceID: b, Translation: [3]float64{2, 4, 15}, Rotation: [4]float64{0, math.Sqrt(.5), 0, math.Sqrt(.5)}})
			endpoint := func(id string) JointEndpoint {
				end := jointEnd(id, 0, 0)
				switch kind {
				case "PRISMATIC", "CYLINDRICAL":
					end.Axis = &AssemblyGeometryRef{InstanceID: id, Kind: "AXIS", GeometryID: "axis-system-default", Axis: "Z"}
					if kind == "PRISMATIC" {
						end.Plane = &AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-yz"}
					}
				case "PLANAR":
					end.Plane = &AssemblyGeometryRef{InstanceID: id, Kind: "PLANE", GeometryID: "datum-xy"}
				case "SPHERICAL":
					end.Point = &AssemblyGeometryRef{InstanceID: id, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-origin"}
				}
				return end
			}
			first, second := endpoint(a), endpoint(b)
			joint := MechanismJoint{ID: "joint", Name: kind + ".1", Kind: kind, First: first, Second: &second, OrientationAngle: MotionQuantity{Unit: "deg"}, AxialDirection: "SAME", AxialOffset: MotionQuantity{Value: 3, Unit: "mm"}}
			if strings.HasSuffix(name, "_OPPOSITE") && !strings.HasSuffix(name, "_PLANE_OPPOSITE") {
				joint.AxisDirection = "OPPOSITE"
			}
			if strings.HasSuffix(name, "_PLANE_OPPOSITE") {
				joint.AxialDirection = "OPPOSITE"
			}
			for _, role := range jointCoordinateRoles(kind) {
				dimension, unit := "LENGTH", "mm"
				if role == "AXIAL_ROTATION" {
					dimension, unit = "ANGLE", "rad"
				}
				joint.Coordinates = append(joint.Coordinates, JointCoordinate{ID: "coordinate/" + role, Role: role, Dimension: dimension, Direction: 1, Zero: MotionQuantity{Unit: unit}, Lower: &MotionQuantity{Value: -100, Unit: unit}, Upper: &MotionQuantity{Value: 100, Unit: unit}})
				if joint.AxisDirection == "OPPOSITE" && dimension == "LENGTH" {
					joint.Coordinates[len(joint.Coordinates)-1].Direction = -1
				}
			}
			m := Mechanism{ID: "mechanism", Name: "机械装置.1", UnitIDs: []string{a, b}, Joints: []MechanismJoint{{ID: "ground", Name: "固定零件.1", Kind: "GROUND", First: jointEnd(a, 0, 0)}, joint}}
			k := KinematicsDefinitions{Mechanisms: []Mechanism{m}}
			for _, c := range joint.Coordinates {
				k.Drivers = append(k.Drivers, MotionDriver{ID: "driver/" + c.Role, Name: c.Role, MechanismID: m.ID, JointID: joint.ID, CoordinateID: c.ID})
			}
			view = apply(CommandRequest{Type: "SAVE_KINEMATICS", Kinematics: &k})
			saved := view.Product.Kinematics.Mechanisms[0]
			bound := saved.Joints[1]
			if joint.AxisDirection == "OPPOSITE" && bound.Constraints[0].DirectionRelation != "OPPOSITE" {
				t.Fatal("coordinate direction replaced the opposite axis relation")
			}
			if joint.AxialDirection == "OPPOSITE" {
				if bound.AxialDirection != "OPPOSITE" || math.Abs(bound.Constraints[1].Value-math.Pi) > 1e-10 {
					t.Fatal("opposite plane branch was not compiled", bound)
				}
			}
			expectedRelations := 1
			if kind == "PRISMATIC" {
				expectedRelations = 2
			}
			if len(bound.Constraints) != expectedRelations {
				t.Fatal("incorrect public relations", bound.Constraints)
			}
			expectedDOF := map[string]uint64{"PRISMATIC": 1, "CYLINDRICAL": 2, "PLANAR": 3, "RIGID": 0, "SPHERICAL": 3}[kind]
			if saved.Evaluation.RelativeDOF != expectedDOF || saved.Evaluation.GaugeDOF != 0 {
				t.Fatal("persistent evaluation", saved.Evaluation)
			}
			if len(view.Product.Constraints) != 0 {
				t.Fatal("mechanism silently added product constraints")
			}
			if len(bound.Coordinates) > 0 {
				altered, err := cloneAssemblyProduct(*view.Product)
				if err != nil {
					t.Fatal(err)
				}
				altered.Kinematics.Mechanisms[0].Joints[1].Coordinates[0].Zero.Value += 1
				_, err = svc.ApplyCommand(t.Context(), view.Document.ID, CommandRequest{Type: "SAVE_KINEMATICS", ActorID: actor, VersionID: view.Document.VersionID, RequestID: uuid.NewString(), Kinematics: &altered.Kinematics})
				if err == nil {
					t.Fatal("client replaced the captured physical zero")
				}
				unchanged, err := svc.GetDocument(t.Context(), view.Document.ID)
				if err != nil || unchanged.Document.VersionID != view.Document.VersionID {
					t.Fatal("rejected coordinate edit changed Head", err)
				}
			}
			if kind == "RIGID" {
				capture := bound.Constraints[0].GroupRelations
				renamed := view.Product.Kinematics
				renamed.Mechanisms[0].Joints[1].Name = "刚性.改名"
				view = apply(CommandRequest{Type: "SAVE_KINEMATICS", Kinematics: &renamed})
				if !reflect.DeepEqual(capture, view.Product.Kinematics.Mechanisms[0].Joints[1].Constraints[0].GroupRelations) {
					t.Fatal("rename recaptured rigid baseline")
				}
				pointA := &AssemblyGeometryRef{InstanceID: a, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-origin"}
				pointB := &AssemblyGeometryRef{InstanceID: b, Kind: "FRAME", GeometryID: "axis-system-default", DerivedRole: "frame-origin"}
				view = apply(CommandRequest{Type: "ADD_ASSEMBLY_CONSTRAINT", ConstraintKind: "COINCIDENT", FirstAssemblyRef: pointA, SecondAssemblyRef: pointB})
				conflict, err := svc.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, MotionApplyRequest{MechanismID: m.ID})
				if err != nil || conflict.Ready || !slices.ContainsFunc(conflict.Items, func(item MotionApplyItem) bool { return item.Action == "CONFLICT" }) {
					t.Fatal("conversion recaptured rigid baseline to conceal a product conflict", conflict, err)
				}
				unchanged, err := svc.GetDocument(t.Context(), view.Document.ID)
				if err != nil || unchanged.Document.VersionID != view.Document.VersionID || !reflect.DeepEqual(capture, unchanged.Product.Kinematics.Mechanisms[0].Joints[1].Constraints[0].GroupRelations) {
					t.Fatal("conflicting conversion changed persistent capture", err)
				}
				view = apply(CommandRequest{Type: "UNDO"})
			}
			if len(k.Drivers) > 0 {
				opened, err := svc.BeginMechanismSession(t.Context(), view.Document.ID, actor, MechanismSessionBegin{OwnerID: "test", Generation: 1, BaseRevisionID: view.Document.VersionID, MechanismID: m.ID})
				if err != nil || !opened.Qualified {
					t.Fatal("persistent command qualification", opened, err)
				}
				goals := map[string]MotionQuantity{}
				for _, d := range view.Product.Kinematics.Drivers {
					_, c, _ := driverCoordinate(saved, d)
					goals[d.ID] = MotionQuantity{Value: 20, Unit: c.unit()}
					if c.angular() {
						goals[d.ID] = MotionQuantity{Value: 45, Unit: "deg"}
					}
				}
				frame, err := svc.UpdateMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionUpdate{Sequence: 1, Values: goals})
				if err != nil || !frame.Accepted {
					t.Fatal(frame, err)
				}
				command, err := svc.FinalizeMechanismSession(t.Context(), view.Document.ID, actor, opened.SessionID, MechanismSessionFinalize{Sequence: 1})
				if err != nil {
					t.Fatal(err)
				}
				view = apply(command)
				for _, coordinate := range bound.Coordinates {
					if coordinate.Dimension == "LENGTH" {
						accepted := view.Product.Kinematics.Mechanisms[0]
						z := rotateByPose(composeInstancePose(saved.Poses[a], bound.First.Frame), [3]float64{0, 0, 1})
						delta := conflictDot(conflictSub(accepted.Poses[b].Translation, saved.Poses[b].Translation), z)
						if math.Abs(delta-20*float64(coordinate.Direction)) > 1e-6 {
							t.Fatal("signed axial target does not match physical translation", delta)
						}
					}
				}
				svc.CancelMechanismSession(view.Document.ID, actor, opened.SessionID)
			}
			req := MotionApplyRequest{MechanismID: m.ID}
			for _, c := range bound.Coordinates {
				req.LockCoordinateIDs = append(req.LockCoordinateIDs, c.ID)
			}
			plan, err := svc.PlanMotionApply(t.Context(), view.Document.ID, actor, view.Document.VersionID, req)
			if err != nil || !plan.Ready {
				t.Fatal("independent conversion", plan.Items, err)
			}
			req.PlanDigest = plan.Digest
			before := view
			view = apply(CommandRequest{Type: "APPLY_MOTION_FRAME", MotionApply: &req})
			if len(view.Product.Constraints) != 1+expectedRelations+len(bound.Coordinates) {
				t.Fatal("conversion relation/lock cardinality", view.Product.Constraints)
			}
			for _, c := range view.Product.Constraints {
				for _, owned := range bound.Constraints {
					if c.ID == owned.ID {
						t.Fatal("conversion reused mechanism constraint identity")
					}
				}
				if c.RelationRole != "" && len(c.RelationRole) > 16 && c.Kind == "DISTANCE" {
					if c.First.CapturedFrame == nil || c.Second.CapturedFrame == nil || c.First.CapturedFrame.ID != c.ID+"/support/a" {
						t.Fatal("length lock lacks independently owned frame")
					}
				}
			}
			for _, instance := range view.Product.Instances {
				want := view.Product.Kinematics.Mechanisms[0].Poses[instance.ID]
				if math.Abs(instance.Translation[2]-want.Translation[2]) > 1e-7 {
					t.Fatal("conversion changed its selected source pose")
				}
			}
			view = apply(CommandRequest{Type: "UNDO"})
			if len(view.Product.Constraints) != 0 || !reflect.DeepEqual(view.Product.Kinematics, before.Product.Kinematics) {
				t.Fatal("conversion undo did not restore the full definition")
			}
			view = apply(CommandRequest{Type: "REDO"})
			if len(view.Product.Constraints) == 0 {
				t.Fatal("conversion redo lost independent relationships")
			}
		})
	}
}

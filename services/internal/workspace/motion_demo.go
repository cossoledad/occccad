package workspace

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
)

// Small persisted design built exclusively with ordinary Domain Commands. The
// boxes deliberately overlap at hinges: a valid motion can have DMU violations.
func (service *Service) CreateFourBarDemo(ctx context.Context, documentID, actor, baseRevision string) (DocumentView, error) {
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return view, err
	}
	if view.Product == nil || len(view.Product.Instances) != 0 || view.Document.VersionID != baseRevision {
		return view, fmt.Errorf("%w: demo requires an empty owning Product at the requested revision", ErrValidation)
	}
	command := func(id string, r CommandRequest) (DocumentView, error) {
		r.RequestID = uuid.NewString()
		r.ActorID = actor
		return service.ApplyCommand(ctx, id, r)
	}
	parts := map[int]string{}
	for _, length := range []int{20, 30, 40} {
		p, e := service.CreateDocument(ctx, CreateDocumentRequest{RequestID: uuid.NewString(), ActorID: actor, Name: fmt.Sprintf("四杆演示 — 连杆 %d mm", length), Type: "PART"})
		if e != nil {
			return view, e
		}
		p, e = command(p.Document.ID, CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
		if e != nil {
			return view, e
		}
		sketch := ""
		for _, f := range p.Part.Features {
			if f.Type == "SKETCH" {
				sketch = f.ID
			}
		}
		points := []SketchPoint2{{X: 0, Y: -1.5}, {X: float64(length), Y: -1.5}, {X: float64(length), Y: 1.5}, {X: 0, Y: 1.5}}
		ops := []SketchOperation{}
		edgeIds := make([]string, 4)
		for i := range 4 {
			a, b := points[i], points[(i+1)%4]
			edgeIds[i] = uuid.NewString()
			entity := SketchEntity{ID: edgeIds[i], Kind: "LINE", Role: "PROFILE", Start: &a, End: &b}
			ops = append(ops, SketchOperation{Type: "ADD_ENTITY", Entity: &entity})
		}

		for i := range 4 {
			c := SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: edgeIds[i], SubElement: "END"}, {Target: "ENTITY", EntityID: edgeIds[(i+1)%4], SubElement: "START"}}}
			ops = append(ops, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c})
		}
		p, e = command(p.Document.ID, CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
		if e != nil {
			return view, e
		}
		p, e = command(p.Document.ID, CommandRequest{Type: "PAD_SKETCH", SketchID: sketch, Length: 2})
		if e != nil {
			return view, e
		}
		parts[length] = p.Document.ID
	}
	refs := []string{parts[40], parts[20], parts[40], parts[30]}
	// Save the original baseline before adding units. Concurrent insertion is
	// rejected by the normal command CAS; final study definition has its own CAS.
	current, e := service.GetDocument(ctx, documentID)
	if e != nil {
		return view, e
	}
	if current.Document.VersionID != baseRevision {
		return view, fmt.Errorf("%w: demo Product changed", ErrValidation)
	}

	ids := []string{}
	known := map[string]bool{}
	for _, ref := range refs {
		view, e = command(documentID, CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: ref})
		if e != nil {
			return view, e
		}
		added := ""
		for _, v := range view.Product.Instances {
			if !known[v.ID] {
				if added != "" {
					return view, fmt.Errorf("%w: demo instance set changed", ErrValidation)
				}
				added = v.ID
			}
		}
		if added == "" || len(view.Product.Instances) != len(ids)+1 {
			return view, fmt.Errorf("%w: demo instance set changed", ErrValidation)
		}
		known[added] = true
		ids = append(ids, added)
	}
	theta := math.Pi / 3
	px, py := 20*math.Cos(theta), 20*math.Sin(theta)
	dx, dy := 40-px, -py
	distance := math.Hypot(dx, dy)
	a := (1600 - 900 + distance*distance) / (2 * distance)
	h := math.Sqrt(1600 - a*a)
	cx, cy := px+a*dx/distance-h*dy/distance, py+a*dy/distance+h*dx/distance
	origins := [][3]float64{{}, {}, {px, py, 0}, {40, 0, 0}}
	angles := []float64{0, theta, math.Atan2(cy-py, cx-px), math.Atan2(cy, cx-40)}
	names := []string{"机架 40", "曲柄 20", "连杆 40（共享定义的另一个实例）", "摇杆 30"}
	for i, id := range ids {
		view, e = command(documentID, CommandRequest{Type: "RENAME_INSTANCE", InstanceID: id, Name: names[i]})
		if e != nil {
			return view, e
		}
		view, e = command(documentID, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Translation: origins[i], Rotation: [4]float64{0, 0, math.Sin(angles[i] / 2), math.Cos(angles[i] / 2)}})
		if e != nil {
			return view, e
		}
	}
	endpoint := func(id string, x float64) JointEndpoint {
		return JointEndpoint{InstanceID: id, Frame: InstancePose{Translation: [3]float64{x, 0, 0}, Rotation: [4]float64{0, 0, 0, 1}}}
	}
	mid := uuid.NewString()
	joints := []MechanismJoint{{ID: uuid.NewString(), Name: "机架接地", Kind: "GROUND", First: endpoint(ids[0], 0), Direction: 1}}
	for i, v := range []struct {
		a, b int
		x, y float64
	}{{0, 1, 0, 0}, {1, 2, 20, 0}, {2, 3, 40, 30}, {0, 3, 40, 0}} {
		b := endpoint(ids[v.b], v.y)
		joints = append(joints, MechanismJoint{ID: uuid.NewString(), Name: fmt.Sprintf("转动关节 %d", i+1), Kind: "REVOLUTE", First: endpoint(ids[v.a], v.x), Second: &b, Zero: MotionQuantity{Unit: "deg"}, Direction: 1})
	}
	k := KinematicsDefinitions{Mechanisms: []Mechanism{{ID: mid, Name: "40/20/40/30 四杆闭环", UnitIDs: ids, Joints: joints}}}
	did := uuid.NewString()
	k.Drivers = []MotionDriver{{ID: did, Name: "曲柄角度驱动", MechanismID: mid, JointID: joints[1].ID}}
	k.Studies = []MotionStudy{{ID: uuid.NewString(), Name: "曲柄 20° → 120°", MechanismID: mid, DriverID: did, Start: MotionQuantity{Value: 20, Unit: "deg"}, End: MotionQuantity{Value: 120, Unit: "deg"}, DurationSeconds: 3, Frames: 21, BudgetMS: 60000, Clearance: MotionQuantity{Unit: "mm"}}}
	k.Analyses = []InterferenceAnalysis{{ID: uuid.NewString(), Name: "四杆采样干涉 / 间隙 0.5 mm", StudyID: k.Studies[0].ID, Clearance: MotionQuantity{Value: .5, Unit: "mm"}}}
	return command(documentID, CommandRequest{Type: "SAVE_KINEMATICS", VersionID: view.Document.VersionID, Kinematics: &k})
}

// A cylinder and a pierced two-blade propeller, made by the same sketch and
// additive Pad commands as ordinary modeling. No mechanism/animation is seeded.
func (service *Service) CreatePropellerDemo(ctx context.Context, doc, actor, base string) (DocumentView, error) {
	view, e := service.GetDocument(ctx, doc)
	if e != nil {
		return view, e
	}
	if view.Product == nil || view.Document.VersionID != base || len(view.Product.Instances) != 0 {
		return view, fmt.Errorf("%w: demo requires an empty Product", ErrValidation)
	}
	command := func(id string, r CommandRequest) (DocumentView, error) {
		r.ActorID = actor
		r.RequestID = uuid.NewString()
		return service.ApplyCommand(ctx, id, r)
	}
	parts := []string{}
	for i, name := range []string{"圆柱轴 Ø8 × 30", "简化螺旋桨（通孔 Ø9）"} {
		p, er := service.CreateDocument(ctx, CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: name, Type: "PART"})
		if er != nil {
			return view, er
		}
		pad := func(entities []SketchEntity, length float64) error {
			var er error
			p, er = command(p.Document.ID, CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			if er != nil {
				return er
			}
			sketch := ""
			for _, f := range p.Part.Features {
				if f.Type == "SKETCH" {
					sketch = f.ID
				}
			}
			ops := []SketchOperation{}
			for k := range entities {
				entities[k].ID = uuid.NewString()
				entities[k].Role = "PROFILE"
				v := entities[k]
				ops = append(ops, SketchOperation{Type: "ADD_ENTITY", Entity: &v})
			}
			if len(entities) == 4 && entities[0].Kind == "LINE" {
				for k := range entities {
					c := SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []SketchGeometryRef{{Target: "ENTITY", EntityID: entities[k].ID, SubElement: "END"}, {Target: "ENTITY", EntityID: entities[(k+1)%4].ID, SubElement: "START"}}}
					ops = append(ops, SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c})
				}
			}
			p, er = command(p.Document.ID, CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
			if er != nil {
				return er
			}
			p, er = command(p.Document.ID, CommandRequest{Type: "PAD_SKETCH", SketchID: sketch, Length: length, Operation: "ADD"})
			return er
		}
		center := SketchPoint2{}
		radius := 4.
		length := 30.
		if i == 1 {
			radius = 12
			length = 4
		}
		circles := []SketchEntity{{Kind: "CIRCLE", Center: &center, Radius: radius}}
		if i == 1 {
			circles = append(circles, SketchEntity{Kind: "CIRCLE", Center: &center, Radius: 4.5})
		}
		if er = pad(circles, length); er != nil {
			return view, er
		}
		if i == 1 {
			for _, bounds := range [][2]float64{{8, 40}, {-40, -8}} {
				points := []SketchPoint2{{X: bounds[0], Y: -3}, {X: bounds[1], Y: -3}, {X: bounds[1], Y: 3}, {X: bounds[0], Y: 3}}
				edges := []SketchEntity{}
				for k := range 4 {
					a, b := points[k], points[(k+1)%4]
					edges = append(edges, SketchEntity{Kind: "LINE", Start: &a, End: &b})
				}
				if er = pad(edges, 4); er != nil {
					return view, er
				}
			}
		}
		parts = append(parts, p.Document.ID)
	}
	latest, e := service.GetDocument(ctx, doc)
	if e != nil {
		return view, e
	}
	if latest.Document.VersionID != base {
		return view, fmt.Errorf("%w: demo Product changed", ErrValidation)
	}
	for i, part := range parts {
		view, e = command(doc, CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part})
		if e != nil {
			return view, e
		}
		id := view.Product.Instances[len(view.Product.Instances)-1].ID
		name := "圆柱轴"
		pose := [3]float64{}
		if i == 1 {
			name = "螺旋桨"
			pose = [3]float64{22, 15, 7}
		}
		view, e = command(doc, CommandRequest{Type: "RENAME_INSTANCE", InstanceID: id, Name: name})
		if e != nil {
			return view, e
		}
		view, e = command(doc, CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Translation: pose, Rotation: [4]float64{0, 0, 0, 1}})
		if e != nil {
			return view, e
		}
	}
	return view, nil
}

package main

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/workspace"
)

// Test-only real geometry, created through ordinary modeling commands.
func buildShaftRotorFixture(ctx context.Context, service *workspace.Service, doc, actor, base string) (workspace.DocumentView, error) {
	view, e := service.GetDocument(ctx, doc)
	if e != nil {
		return view, e
	}
	if view.Product == nil || view.Document.VersionID != base || len(view.Product.Instances) != 0 {
		return view, fmt.Errorf("%w: demo requires an empty Product", workspace.ErrValidation)
	}
	command := func(id string, r workspace.CommandRequest) (workspace.DocumentView, error) {
		r.ActorID = actor
		r.RequestID = uuid.NewString()
		return service.ApplyCommand(ctx, id, r)
	}
	parts := []string{}
	for i, name := range []string{"圆柱轴 Ø8 × 30", "简化螺旋桨（通孔 Ø9）"} {
		p, er := service.CreateDocument(ctx, workspace.CreateDocumentRequest{ActorID: actor, RequestID: uuid.NewString(), Name: name, Type: "PART"})
		if er != nil {
			return view, er
		}
		pad := func(entities []workspace.SketchEntity, length float64) error {
			var er error
			p, er = command(p.Document.ID, workspace.CommandRequest{Type: "CREATE_SKETCH", Plane: "XY"})
			if er != nil {
				return er
			}
			sketch := ""
			for _, f := range p.Part.Features {
				if f.Type == "SKETCH" {
					sketch = f.ID
				}
			}
			ops := []workspace.SketchOperation{}
			for k := range entities {
				entities[k].ID = uuid.NewString()
				entities[k].Role = "PROFILE"
				v := entities[k]
				ops = append(ops, workspace.SketchOperation{Type: "ADD_ENTITY", Entity: &v})
			}
			if len(entities) == 4 && entities[0].Kind == "LINE" {
				for k := range entities {
					c := workspace.SketchConstraint{ID: uuid.NewString(), Kind: "COINCIDENT", References: []workspace.SketchGeometryRef{{Target: "ENTITY", EntityID: entities[k].ID, SubElement: "END"}, {Target: "ENTITY", EntityID: entities[(k+1)%4].ID, SubElement: "START"}}}
					ops = append(ops, workspace.SketchOperation{Type: "ADD_CONSTRAINT", Constraint: &c})
				}
			}
			p, er = command(p.Document.ID, workspace.CommandRequest{Type: "EDIT_SKETCH", SketchID: sketch, Operations: ops})
			if er != nil {
				return er
			}
			p, er = command(p.Document.ID, workspace.CommandRequest{Type: "PAD_SKETCH", SketchID: sketch, Length: length, Operation: "ADD"})
			return er
		}
		center := workspace.SketchPoint2{}
		radius := 4.
		length := 30.
		if i == 1 {
			radius = 12
			length = 4
		}
		circles := []workspace.SketchEntity{{Kind: "CIRCLE", Center: &center, Radius: radius}}
		if i == 1 {
			circles = append(circles, workspace.SketchEntity{Kind: "CIRCLE", Center: &center, Radius: 4.5})
		}
		if er = pad(circles, length); er != nil {
			return view, er
		}
		if i == 1 {
			for _, bounds := range [][2]float64{{8, 40}, {-40, -8}} {
				points := []workspace.SketchPoint2{{X: bounds[0], Y: -3}, {X: bounds[1], Y: -3}, {X: bounds[1], Y: 3}, {X: bounds[0], Y: 3}}
				edges := []workspace.SketchEntity{}
				for k := range 4 {
					a, b := points[k], points[(k+1)%4]
					edges = append(edges, workspace.SketchEntity{Kind: "LINE", Start: &a, End: &b})
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
		return view, fmt.Errorf("%w: demo Product changed", workspace.ErrValidation)
	}
	for i, part := range parts {
		view, e = command(doc, workspace.CommandRequest{Type: "INSERT_INSTANCE", ReferencedDocumentID: part})
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
		view, e = command(doc, workspace.CommandRequest{Type: "RENAME_INSTANCE", InstanceID: id, Name: name})
		if e != nil {
			return view, e
		}
		view, e = command(doc, workspace.CommandRequest{Type: "MOVE_INSTANCE", InstanceID: id, Translation: pose, Rotation: [4]float64{0, 0, 0, 1}})
		if e != nil {
			return view, e
		}
	}
	return view, nil
}

type motionRemoteStore struct{ *artifact.LocalStore }

func (motionRemoteStore) Backend() string { return "TEST" }

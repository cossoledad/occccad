package geometry

import (
	"context"
	"fmt"
	"sync"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type Client struct {
	connection *grpc.ClientConn
	worker     workerv1.GeometryWorkerClient
	workers    sync.Map
}

type RectangularPad struct {
	OriginX, OriginY, Width, Height, Length float64
	Plane                                   string
}

type ProfileCurve struct {
	EntityID, Kind               string
	Reversed                     bool
	Start, End, Center           [2]float64
	Radius, StartAngle, EndAngle float64
	ControlPoints                [][2]float64
	Degree                       uint32
	Closed                       bool
}
type ProfileLoop struct {
	ID     string
	Curves []ProfileCurve
}
type ProfileRegion struct {
	ID    string
	Outer ProfileLoop
	Holes []ProfileLoop
}
type ProfilePad struct {
	Regions                                   []ProfileRegion
	Length, RevolveAngle                      float64
	AxisStart, AxisEnd                        [2]float64
	PlaneOrigin, PlaneNormal, PlaneUDirection [3]float64
	Plane, BodyOperation, Generator           string
	Reversed                                  bool
}

func profilePadsProto(pads []ProfilePad) []*workerv1.ProfilePadSpec {
	result := make([]*workerv1.ProfilePadSpec, 0, len(pads))
	for _, pad := range pads {
		value := &workerv1.ProfilePadSpec{PadLength: pad.Length, Units: "mm", Plane: pad.Plane,
			BodyOperation: pad.BodyOperation, Generator: pad.Generator, RevolveAngle: pad.RevolveAngle,
			AxisStart: &workerv1.Vec2{X: pad.AxisStart[0], Y: pad.AxisStart[1]},
			AxisEnd:   &workerv1.Vec2{X: pad.AxisEnd[0], Y: pad.AxisEnd[1]}, Reversed: pad.Reversed}
		value.PlaneOrigin = &workerv1.Vec3{X: pad.PlaneOrigin[0], Y: pad.PlaneOrigin[1], Z: pad.PlaneOrigin[2]}
		value.PlaneNormal = &workerv1.Vec3{X: pad.PlaneNormal[0], Y: pad.PlaneNormal[1], Z: pad.PlaneNormal[2]}
		value.PlaneUDirection = &workerv1.Vec3{X: pad.PlaneUDirection[0], Y: pad.PlaneUDirection[1], Z: pad.PlaneUDirection[2]}
		loopProto := func(loop ProfileLoop) *workerv1.ProfileLoop {
			output := &workerv1.ProfileLoop{Id: loop.ID}
			for _, curve := range loop.Curves {
				item := &workerv1.ProfileCurve{EntityId: curve.EntityID, Kind: curve.Kind, Reversed: curve.Reversed, Radius: curve.Radius, StartAngle: curve.StartAngle, EndAngle: curve.EndAngle, Degree: curve.Degree, Closed: curve.Closed,
					Start: &workerv1.Vec2{X: curve.Start[0], Y: curve.Start[1]}, End: &workerv1.Vec2{X: curve.End[0], Y: curve.End[1]}, Center: &workerv1.Vec2{X: curve.Center[0], Y: curve.Center[1]}}
				for _, point := range curve.ControlPoints {
					item.ControlPoints = append(item.ControlPoints, &workerv1.Vec2{X: point[0], Y: point[1]})
				}
				output.Curves = append(output.Curves, item)
			}
			return output
		}
		for _, region := range pad.Regions {
			item := &workerv1.ProfileRegion{Id: region.ID, Outer: loopProto(region.Outer)}
			for _, hole := range region.Holes {
				item.Holes = append(item.Holes, loopProto(hole))
			}
			value.Regions = append(value.Regions, item)
		}
		result = append(result, value)
	}
	return result
}

type ArtifactReference struct {
	Backend, ObjectKey, SHA256, ContentType string
	Size                                    int64
}

type ExchangeComponentInfo struct {
	SourceIndex uint32
	Name        string
}

type ExchangeInspection struct {
	DocumentType string
	Components   []ExchangeComponentInfo
}

type ExchangeComponent struct {
	Name        string
	BRep        ArtifactReference
	Translation [3]float64
}

func artifactProto(value ArtifactReference) *workerv1.ArtifactReference {
	return &workerv1.ArtifactReference{Backend: value.Backend, ObjectKey: value.ObjectKey,
		Sha256: value.SHA256, SizeBytes: uint64(value.Size), ContentType: value.ContentType}
}

type SketchPoint struct {
	ID   string
	X, Y float64
	Role string
}
type SketchLine struct {
	ID                         string
	StartX, StartY, EndX, EndY float64
	Role                       string
}
type SketchCircle struct {
	ID, Role                 string
	CenterX, CenterY, Radius float64
}
type SketchArc struct {
	ID, Role                                       string
	CenterX, CenterY, Radius, StartAngle, EndAngle float64
}
type SketchSpline struct {
	ID, Role      string
	ControlPoints [][2]float64
	Degree        uint32
	Closed        bool
}
type SketchReference struct {
	Target, EntityID, SubElement string
	ControlPointIndex            *int
}
type SketchConstraint struct {
	ID, Kind       string
	References     []SketchReference
	FixedX, FixedY float64
	Value          float64
	Unit           string
	Internal       bool
}
type SketchModel struct {
	Points      []SketchPoint
	Lines       []SketchLine
	Circles     []SketchCircle
	Arcs        []SketchArc
	Splines     []SketchSpline
	Constraints []SketchConstraint
}
type SketchSolveStatus string

const (
	SketchSolveFullyConstrained SketchSolveStatus = "FULLY_CONSTRAINED"
	SketchSolveUnderConstrained SketchSolveStatus = "UNDER_CONSTRAINED"
	SketchSolveConflicting      SketchSolveStatus = "CONFLICTING"
	SketchSolveRedundant        SketchSolveStatus = "REDUNDANT"
	SketchSolveInvalid          SketchSolveStatus = "INVALID"
	SketchSolveFailed           SketchSolveStatus = "FAILED"
)

// sketchSolveStatus is the sole adapter from the current worker protocol to
// the platform vocabulary. Solver-specific names and codes must not escape it.
func sketchSolveStatus(status string) SketchSolveStatus {
	switch status {
	case "SOLVED", "FULLY_CONSTRAINED":
		return SketchSolveFullyConstrained
	case "UNDER_CONSTRAINED":
		return SketchSolveUnderConstrained
	case "CONFLICTING":
		return SketchSolveConflicting
	case "REDUNDANT":
		return SketchSolveRedundant
	case "INVALID_MODEL", "INVALID":
		return SketchSolveInvalid
	default:
		return SketchSolveFailed
	}
}

type SketchSolve struct {
	Model                                            SketchModel
	Status                                           SketchSolveStatus
	DegreesOfFreedom                                 int
	Diagnostic                                       string
	ConflictingConstraintIDs, RedundantConstraintIDs []string
}

func (client *Client) SolveSketch(ctx context.Context, requestID string, model SketchModel) (SketchSolve, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	input := &workerv1.SketchModel{SchemaVersion: 1}
	for _, point := range model.Points {
		input.Points = append(input.Points, &workerv1.SketchPoint{Id: point.ID, Point: &workerv1.Vec2{X: point.X, Y: point.Y}, Role: point.Role})
	}
	for _, line := range model.Lines {
		input.Lines = append(input.Lines, &workerv1.SketchLine{Id: line.ID, Start: &workerv1.Vec2{X: line.StartX, Y: line.StartY}, End: &workerv1.Vec2{X: line.EndX, Y: line.EndY}, Role: line.Role})
	}
	for _, circle := range model.Circles {
		input.Circles = append(input.Circles, &workerv1.SketchCircle{Id: circle.ID, Center: &workerv1.Vec2{X: circle.CenterX, Y: circle.CenterY}, Radius: circle.Radius, Role: circle.Role})
	}
	for _, arc := range model.Arcs {
		input.Arcs = append(input.Arcs, &workerv1.SketchArc{Id: arc.ID, Center: &workerv1.Vec2{X: arc.CenterX, Y: arc.CenterY}, Radius: arc.Radius, StartAngle: arc.StartAngle, EndAngle: arc.EndAngle, Role: arc.Role})
	}
	for _, spline := range model.Splines {
		value := &workerv1.SketchSpline{Id: spline.ID, Degree: spline.Degree, Closed: spline.Closed, Role: spline.Role}
		for _, point := range spline.ControlPoints {
			value.ControlPoints = append(value.ControlPoints, &workerv1.Vec2{X: point[0], Y: point[1]})
		}
		input.Splines = append(input.Splines, value)
	}
	for _, constraint := range model.Constraints {
		value := &workerv1.SketchConstraint{Id: constraint.ID, Kind: constraint.Kind, FixedPoint: &workerv1.Vec2{X: constraint.FixedX, Y: constraint.FixedY}, Value: constraint.Value, Unit: constraint.Unit, Internal: constraint.Internal}
		for _, reference := range constraint.References {
			index := uint32(0)
			if reference.ControlPointIndex != nil {
				index = uint32(*reference.ControlPointIndex)
			}
			value.References = append(value.References, &workerv1.SketchGeometryRef{Target: reference.Target, EntityId: reference.EntityID,
				SubElement: reference.SubElement, ControlPointIndex: index})
		}
		input.Constraints = append(input.Constraints, value)
	}
	response, err := client.worker.SolveSketch(ctx, &workerv1.SolveSketchRequest{RequestId: requestID, Sketch: input})
	if err != nil {
		return SketchSolve{}, fmt.Errorf("solve sketch: %w", err)
	}
	result := SketchSolve{Status: sketchSolveStatus(response.GetStatus()), DegreesOfFreedom: int(response.GetDegreesOfFreedom()), Diagnostic: response.GetDiagnostic(), ConflictingConstraintIDs: response.GetConflictingConstraintIds(), RedundantConstraintIDs: response.GetRedundantConstraintIds()}
	for _, point := range response.GetSketch().GetPoints() {
		result.Model.Points = append(result.Model.Points, SketchPoint{ID: point.GetId(), X: point.GetPoint().GetX(), Y: point.GetPoint().GetY(), Role: point.GetRole()})
	}
	for _, line := range response.GetSketch().GetLines() {
		result.Model.Lines = append(result.Model.Lines, SketchLine{ID: line.GetId(), StartX: line.GetStart().GetX(), StartY: line.GetStart().GetY(), EndX: line.GetEnd().GetX(), EndY: line.GetEnd().GetY(), Role: line.GetRole()})
	}
	for _, circle := range response.GetSketch().GetCircles() {
		result.Model.Circles = append(result.Model.Circles, SketchCircle{ID: circle.GetId(), CenterX: circle.GetCenter().GetX(), CenterY: circle.GetCenter().GetY(), Radius: circle.GetRadius(), Role: circle.GetRole()})
	}
	for _, arc := range response.GetSketch().GetArcs() {
		result.Model.Arcs = append(result.Model.Arcs, SketchArc{ID: arc.GetId(), CenterX: arc.GetCenter().GetX(), CenterY: arc.GetCenter().GetY(), Radius: arc.GetRadius(), StartAngle: arc.GetStartAngle(), EndAngle: arc.GetEndAngle(), Role: arc.GetRole()})
	}
	for _, spline := range response.GetSketch().GetSplines() {
		value := SketchSpline{ID: spline.GetId(), Degree: spline.GetDegree(), Closed: spline.GetClosed(), Role: spline.GetRole()}
		for _, point := range spline.GetControlPoints() {
			value.ControlPoints = append(value.ControlPoints, [2]float64{point.GetX(), point.GetY()})
		}
		result.Model.Splines = append(result.Model.Splines, value)
	}
	result.Model.Constraints = model.Constraints
	return result, nil
}

func Open(address string) (*Client, error) {
	connection, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, fmt.Errorf("create geometry worker client: %w", err)
	}
	return &Client{
		connection: connection,
		worker:     workerv1.NewGeometryWorkerClient(connection),
	}, nil
}

func (client *Client) Close() error { return client.connection.Close() }

func (client *Client) Ping(ctx context.Context) (*workerv1.PingResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return client.worker.Ping(ctx, &workerv1.PingRequest{})
}

func (client *Client) WorkerFor(geometryKey string) string {
	if value, ok := client.workers.Load(geometryKey); ok {
		return value.(string)
	}
	return ""
}

func (client *Client) GetTopology(
	ctx context.Context, geometryID string, brep []byte, topologyType string, localID uint64,
) (*workerv1.GetTopologyResponse, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var header metadata.MD
	response, err := client.worker.GetTopology(ctx, &workerv1.GetTopologyRequest{
		GeometryId: geometryID, BrepData: brep, TopologyType: topologyType, LocalId: localID,
	}, grpc.Header(&header))
	if err != nil {
		return nil, "", fmt.Errorf("query B-Rep topology: %w", err)
	}
	workerID := ""
	if values := header.Get("x-occccad-worker-id"); len(values) > 0 {
		workerID = values[0]
	}
	return response, workerID, nil
}

func (client *Client) GetTopologyFromArtifact(
	ctx context.Context, geometryID string, brep ArtifactReference, topologyType string, localID uint64,
) (*workerv1.GetTopologyResponse, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var header metadata.MD
	response, err := client.worker.GetTopology(ctx, &workerv1.GetTopologyRequest{
		GeometryId: geometryID, BrepArtifact: artifactProto(brep), TopologyType: topologyType, LocalId: localID,
	}, grpc.Header(&header))
	if err != nil {
		return nil, "", fmt.Errorf("query B-Rep topology: %w", err)
	}
	workerID := ""
	if values := header.Get("x-occccad-worker-id"); len(values) > 0 {
		workerID = values[0]
	}
	return response, workerID, nil
}

func (client *Client) rememberWorker(ctx context.Context, geometryKey string, header metadata.MD) {
	if values := header.Get("x-occccad-worker-id"); len(values) > 0 && values[0] != "" {
		client.workers.Store(geometryKey, values[0])
		return
	}
	if ping, err := client.worker.Ping(ctx, &workerv1.PingRequest{}); err == nil && ping.GetWorkerId() != "" {
		client.workers.Store(geometryKey, ping.GetWorkerId())
	}
}

func (client *Client) EvaluateRectangularPad(
	ctx context.Context,
	requestID string,
	geometryKey string,
	originX float64,
	originY float64,
	width float64,
	height float64,
	padLength float64,
	plane string,
) (*workerv1.EvaluatePartResponse, error) {
	return client.EvaluatePart(ctx, requestID, geometryKey, []RectangularPad{{
		OriginX: originX, OriginY: originY, Width: width, Height: height,
		Length: padLength, Plane: plane,
	}}, nil)
}

func (client *Client) EvaluatePart(
	ctx context.Context,
	requestID string,
	geometryKey string,
	pads []RectangularPad,
	baseBRep []byte,
) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	protoPads := make([]*workerv1.RectangularPadSpec, 0, len(pads))
	for _, pad := range pads {
		protoPads = append(protoPads, &workerv1.RectangularPadSpec{
			OriginX: pad.OriginX, OriginY: pad.OriginY, Width: pad.Width,
			Height: pad.Height, PadLength: pad.Length, Units: "mm", Plane: pad.Plane,
		})
	}
	var header metadata.MD
	response, err := client.worker.EvaluatePart(ctx, &workerv1.EvaluatePartRequest{
		RequestId: requestID, GeometryKey: geometryKey,
		RectangularPads: protoPads, BaseBrepData: baseBRep,
		LinearDeflection:  0.1,
		AngularDeflection: 0.5,
	}, grpc.Header(&header))
	if err != nil {
		return nil, fmt.Errorf("evaluate Part feature chain: %w", err)
	}
	client.rememberWorker(ctx, geometryKey, header)
	return response, nil
}

func (client *Client) EvaluatePartFromArtifact(
	ctx context.Context, requestID, geometryKey string, pads []RectangularPad,
	baseBRep ArtifactReference, brepOutputKey, glbOutputKey string,
) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	protoPads := make([]*workerv1.RectangularPadSpec, 0, len(pads))
	for _, pad := range pads {
		protoPads = append(protoPads, &workerv1.RectangularPadSpec{
			OriginX: pad.OriginX, OriginY: pad.OriginY, Width: pad.Width,
			Height: pad.Height, PadLength: pad.Length, Units: "mm", Plane: pad.Plane,
		})
	}
	request := &workerv1.EvaluatePartRequest{RequestId: requestID, GeometryKey: geometryKey,
		RectangularPads: protoPads, LinearDeflection: 0.1, AngularDeflection: 0.5,
		BrepOutputKey: brepOutputKey, GlbOutputKey: glbOutputKey}
	if baseBRep.ObjectKey != "" {
		request.BaseBrepArtifact = artifactProto(baseBRep)
	}
	var header metadata.MD
	response, err := client.worker.EvaluatePart(ctx, request, grpc.Header(&header))
	if err != nil {
		return nil, fmt.Errorf("evaluate Part feature chain: %w", err)
	}
	client.rememberWorker(ctx, geometryKey, header)
	return response, nil
}

func (client *Client) EvaluateProfilePart(ctx context.Context, requestID, geometryKey string, pads []ProfilePad, baseBRep []byte) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var header metadata.MD
	response, err := client.worker.EvaluatePart(ctx, &workerv1.EvaluatePartRequest{RequestId: requestID, GeometryKey: geometryKey,
		ProfilePads: profilePadsProto(pads), BaseBrepData: baseBRep, LinearDeflection: 0.1, AngularDeflection: 0.5}, grpc.Header(&header))
	if err != nil {
		return nil, fmt.Errorf("evaluate Part profile feature chain: %w", err)
	}
	client.rememberWorker(ctx, geometryKey, header)
	return response, nil
}

func (client *Client) EvaluateProfilePartFromArtifact(ctx context.Context, requestID, geometryKey string, pads []ProfilePad, baseBRep ArtifactReference, brepOutputKey, glbOutputKey string) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	request := &workerv1.EvaluatePartRequest{RequestId: requestID, GeometryKey: geometryKey,
		ProfilePads: profilePadsProto(pads), LinearDeflection: 0.1, AngularDeflection: 0.5, BrepOutputKey: brepOutputKey, GlbOutputKey: glbOutputKey}
	if baseBRep.ObjectKey != "" {
		request.BaseBrepArtifact = artifactProto(baseBRep)
	}
	var header metadata.MD
	response, err := client.worker.EvaluatePart(ctx, request, grpc.Header(&header))
	if err != nil {
		return nil, fmt.Errorf("evaluate Part profile feature chain: %w", err)
	}
	client.rememberWorker(ctx, geometryKey, header)
	return response, nil
}

func (client *Client) InspectExchange(ctx context.Context, requestID, format string, source ArtifactReference) (ExchangeInspection, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	response, err := client.worker.InspectExchange(ctx, &workerv1.InspectExchangeRequest{
		RequestId: requestID, Format: format, Source: artifactProto(source),
	})
	if err != nil {
		return ExchangeInspection{}, fmt.Errorf("inspect %s exchange: %w", format, err)
	}
	result := ExchangeInspection{DocumentType: response.GetDocumentType()}
	for _, component := range response.GetComponents() {
		result.Components = append(result.Components, ExchangeComponentInfo{
			SourceIndex: component.GetSourceIndex(), Name: component.GetName(),
		})
	}
	return result, nil
}

func (client *Client) ImportExchange(ctx context.Context, requestID, geometryKey, format string,
	source ArtifactReference, sourceIndex uint32, brepOutputKey, glbOutputKey string) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	var header metadata.MD
	response, err := client.worker.ImportExchange(ctx, &workerv1.ImportExchangeRequest{
		RequestId: requestID, GeometryKey: geometryKey, Format: format, Source: artifactProto(source),
		SourceIndex: sourceIndex, BrepOutputKey: brepOutputKey, GlbOutputKey: glbOutputKey,
		LinearDeflection: 0.1, AngularDeflection: 0.5,
	}, grpc.Header(&header))
	if err != nil {
		return nil, fmt.Errorf("import %s exchange: %w", format, err)
	}
	client.rememberWorker(ctx, geometryKey, header)
	return response, nil
}

func (client *Client) ExportExchange(ctx context.Context, requestID, format, outputKey string,
	components []ExchangeComponent) (ArtifactReference, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	request := &workerv1.ExportExchangeRequest{RequestId: requestID, Format: format, OutputKey: outputKey}
	for _, component := range components {
		request.Components = append(request.Components, &workerv1.ExchangeComponent{Name: component.Name,
			Brep: artifactProto(component.BRep), Translation: &workerv1.Vec3{X: component.Translation[0], Y: component.Translation[1], Z: component.Translation[2]}})
	}
	response, err := client.worker.ExportExchange(ctx, request)
	if err != nil {
		return ArtifactReference{}, fmt.Errorf("export %s exchange: %w", format, err)
	}
	result := response.GetResult()
	return ArtifactReference{Backend: result.GetBackend(), ObjectKey: result.GetObjectKey(),
		SHA256: result.GetSha256(), Size: int64(result.GetSizeBytes()), ContentType: result.GetContentType()}, nil
}

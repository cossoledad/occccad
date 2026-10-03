package geometry

import (
	"context"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"time"
)

type SketchCurveIntersection struct {
	Kind                            string
	Point                           [2]float64
	FirstParameter, SecondParameter float64
}
type SketchCurveOverlap struct{ FirstStart, FirstEnd, SecondStart, SecondEnd float64 }
type SketchCurveComputation struct {
	Curves        []ProfileCurve
	Intersections []SketchCurveIntersection
	Overlaps      []SketchCurveOverlap
}

func profileCurveProto(curve ProfileCurve) *workerv1.ProfileCurve {
	item := &workerv1.ProfileCurve{EntityId: curve.EntityID, Kind: curve.Kind, Reversed: curve.Reversed, Radius: curve.Radius, StartAngle: curve.StartAngle, EndAngle: curve.EndAngle, Degree: curve.Degree, Closed: curve.Closed, MajorRadius: curve.MajorRadius, MinorRadius: curve.MinorRadius, Rotation: curve.Rotation, Mode: curve.Mode, Knots: curve.Knots, Multiplicities: curve.Multiplicities, Weights: curve.Weights, Periodic: curve.Periodic, ParameterStart: curve.ParameterStart, ParameterEnd: curve.ParameterEnd, Start: &workerv1.Vec2{X: curve.Start[0], Y: curve.Start[1]}, End: &workerv1.Vec2{X: curve.End[0], Y: curve.End[1]}, Center: &workerv1.Vec2{X: curve.Center[0], Y: curve.Center[1]}}
	for _, p := range curve.ControlPoints {
		item.ControlPoints = append(item.ControlPoints, &workerv1.Vec2{X: p[0], Y: p[1]})
	}
	for _, p := range curve.Poles {
		item.Poles = append(item.Poles, &workerv1.Vec2{X: p[0], Y: p[1]})
	}
	return item
}
func profileCurveFromProto(c *workerv1.ProfileCurve) ProfileCurve {
	curve := ProfileCurve{EntityID: c.GetEntityId(), Kind: c.GetKind(), Reversed: c.GetReversed(), Radius: c.GetRadius(), StartAngle: c.GetStartAngle(), EndAngle: c.GetEndAngle(), Degree: c.GetDegree(), Closed: c.GetClosed(), MajorRadius: c.GetMajorRadius(), MinorRadius: c.GetMinorRadius(), Rotation: c.GetRotation(), Mode: c.GetMode(), Knots: c.GetKnots(), Multiplicities: c.GetMultiplicities(), Weights: c.GetWeights(), Periodic: c.GetPeriodic(), ParameterStart: c.GetParameterStart(), ParameterEnd: c.GetParameterEnd(), Start: [2]float64{c.GetStart().GetX(), c.GetStart().GetY()}, End: [2]float64{c.GetEnd().GetX(), c.GetEnd().GetY()}, Center: [2]float64{c.GetCenter().GetX(), c.GetCenter().GetY()}}
	for _, p := range c.GetControlPoints() {
		curve.ControlPoints = append(curve.ControlPoints, [2]float64{p.GetX(), p.GetY()})
	}
	for _, p := range c.GetPoles() {
		curve.Poles = append(curve.Poles, [2]float64{p.GetX(), p.GetY()})
	}
	return curve
}

func (client *Client) ComputeSketchCurves(ctx context.Context, requestID, operation string, curves []ProfileCurve, start, end *float64) (SketchCurveComputation, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request := &workerv1.ComputeSketchCurvesRequest{RequestId: requestID, Operation: operation, ParameterStart: start, ParameterEnd: end}
	for _, c := range curves {
		request.Curves = append(request.Curves, profileCurveProto(c))
	}
	response, err := client.worker.ComputeSketchCurves(ctx, request)
	if err != nil {
		return SketchCurveComputation{}, fmt.Errorf("exact sketch curve computation: %w", err)
	}
	if response.GetStatus() != "OK" {
		return SketchCurveComputation{}, fmt.Errorf("exact sketch curve computation: %s", response.GetDiagnostic())
	}
	result := SketchCurveComputation{}
	for _, c := range response.GetCurves() {
		result.Curves = append(result.Curves, profileCurveFromProto(c))
	}
	for _, p := range response.GetIntersections() {
		result.Intersections = append(result.Intersections, SketchCurveIntersection{Kind: p.GetKind(), Point: [2]float64{p.GetPoint().GetX(), p.GetPoint().GetY()}, FirstParameter: p.GetFirstParameter(), SecondParameter: p.GetSecondParameter()})
	}
	for _, p := range response.GetOverlaps() {
		result.Overlaps = append(result.Overlaps, SketchCurveOverlap{FirstStart: p.GetFirstStart(), FirstEnd: p.GetFirstEnd(), SecondStart: p.GetSecondStart(), SecondEnd: p.GetSecondEnd()})
	}
	return result, nil
}

func (client *Client) ClassifySketchProfile(ctx context.Context, requestID string, loops []ProfileLoop) ([]ProfileRegion, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request := &workerv1.ComputeSketchCurvesRequest{RequestId: requestID, Operation: "PROFILE"}
	for _, loop := range loops {
		value := &workerv1.ProfileLoop{Id: loop.ID}
		for _, curve := range loop.Curves {
			value.Curves = append(value.Curves, profileCurveProto(curve))
		}
		request.Loops = append(request.Loops, value)
	}
	response, err := client.worker.ComputeSketchCurves(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("exact profile classification: %w", err)
	}
	if response.GetStatus() != "OK" {
		return nil, fmt.Errorf("exact profile classification: %s", response.GetDiagnostic())
	}
	readLoop := func(input *workerv1.ProfileLoop) ProfileLoop {
		output := ProfileLoop{ID: input.GetId()}
		for _, curve := range input.GetCurves() {
			output.Curves = append(output.Curves, profileCurveFromProto(curve))
		}
		return output
	}
	regions := []ProfileRegion{}
	for _, input := range response.GetRegions() {
		region := ProfileRegion{ID: input.GetId(), Outer: readLoop(input.GetOuter())}
		for _, hole := range input.GetHoles() {
			region.Holes = append(region.Holes, readLoop(hole))
		}
		regions = append(regions, region)
	}
	return regions, nil
}

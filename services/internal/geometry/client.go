package geometry

import (
	"context"
	"fmt"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	connection *grpc.ClientConn
	worker     workerv1.GeometryWorkerClient
}

type RectangularPad struct {
	OriginX, OriginY, Width, Height, Length float64
	Plane                                   string
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
	response, err := client.worker.EvaluatePart(ctx, &workerv1.EvaluatePartRequest{
		RequestId: requestID, GeometryKey: geometryKey,
		RectangularPads: protoPads, BaseBrepData: baseBRep,
		LinearDeflection:  0.1,
		AngularDeflection: 0.5,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate Part feature chain: %w", err)
	}
	return response, nil
}

func (client *Client) ImportStep(
	ctx context.Context, requestID, geometryKey, fileName string, data []byte,
) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	response, err := client.worker.ImportStep(ctx, &workerv1.ImportStepRequest{
		RequestId: requestID, GeometryKey: geometryKey, FileName: fileName, StepData: data,
		LinearDeflection: 0.1, AngularDeflection: 0.5,
	})
	if err != nil {
		return nil, fmt.Errorf("import STEP: %w", err)
	}
	return response, nil
}

func (client *Client) ExportStep(
	ctx context.Context, requestID string, brep []byte,
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	response, err := client.worker.ExportStep(ctx, &workerv1.ExportStepRequest{
		RequestId: requestID, BrepData: brep,
	})
	if err != nil {
		return nil, fmt.Errorf("export STEP: %w", err)
	}
	return response.GetStepData(), nil
}

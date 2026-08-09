package geometry

import (
	"context"
	"fmt"
	"time"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	connection *grpc.ClientConn
	worker     workerv1.GeometryWorkerClient
}

func Open(address string) (*Client, error) {
	connection, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
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
	width float64,
	height float64,
	padLength float64,
) (*workerv1.EvaluatePartResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	response, err := client.worker.EvaluatePart(ctx, &workerv1.EvaluatePartRequest{
		RequestId:   requestID,
		GeometryKey: geometryKey,
		RectangularPad: &workerv1.RectangularPadSpec{
			OriginX:   0,
			OriginY:   0,
			Width:     width,
			Height:    height,
			PadLength: padLength,
			Units:     "mm",
		},
		LinearDeflection:  0.1,
		AngularDeflection: 0.5,
	})
	if err != nil {
		return nil, fmt.Errorf("evaluate rectangular pad: %w", err)
	}
	return response, nil
}

package geometry

import (
	"context"
	"encoding/json"
	"fmt"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const AssemblyReplaySchema = "occccad.3dreplay.v1"

// AssemblyReplay contains only the exact numerical RPC input and solve evidence.
// Geometry is already resolved in body-local coordinates; no database or B-Rep is needed to replay.
type AssemblyReplay struct {
	Schema         string          `json:"schema"`
	Units          string          `json:"units"`
	Request        json.RawMessage `json:"request"`
	Result         json.RawMessage `json:"result,omitempty"`
	TransportError string          `json:"transportError,omitempty"`
}

func makeAssemblyReplay(request *workerv1.SolveAssemblyRequest, result *workerv1.SolveAssemblyResponse, solveErr error) ([]byte, error) {
	effective := proto.Clone(request).(*workerv1.SolveAssemblyRequest)
	if result != nil && result.EffectiveSolverProfile != nil {
		effective.SolverProfile = proto.Clone(result.EffectiveSolverProfile).(*workerv1.AssemblySolverProfile)
	}
	input, err := protojson.Marshal(effective)
	if err != nil {
		return nil, err
	}
	replay := AssemblyReplay{Schema: AssemblyReplaySchema, Units: "mm,rad; body-local geometry; quaternion xyzw", Request: input}
	if result != nil {
		compact := proto.Clone(result).(*workerv1.SolveAssemblyResponse)
		compact.EffectiveSolverProfile = nil
		for _, c := range compact.Components {
			c.NullSpaceBasis = nil
			c.SingularValues = nil
			c.Freedoms = nil
		}
		replay.Result, err = protojson.Marshal(compact)
		if err != nil {
			return nil, err
		}
	}
	if solveErr != nil {
		replay.TransportError = solveErr.Error()
	}
	return json.MarshalIndent(replay, "", "  ")
}

// ReplayAssembly uses the same public RPC and validation as an ordinary solve.
func (client *Client) ReplayAssembly(ctx context.Context, data []byte) ([]byte, error) {
	var replay AssemblyReplay
	if err := json.Unmarshal(data, &replay); err != nil {
		return nil, err
	}
	if replay.Schema != AssemblyReplaySchema {
		return nil, fmt.Errorf("unsupported 3dreplay schema %q", replay.Schema)
	}
	var input workerv1.SolveAssemblyRequest
	if err := protojson.Unmarshal(replay.Request, &input); err != nil {
		return nil, err
	}
	result, solveErr := client.worker.SolveAssembly(ctx, &input)
	return makeAssemblyReplay(&input, result, solveErr)
}

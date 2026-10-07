package geometry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Snapshot       json.RawMessage `json:"snapshot,omitempty"`
	AssemblyResult json.RawMessage `json:"assemblyResult,omitempty"`
}

func makeAssemblyReplay(request *workerv1.SolveAssemblyRequest, result *workerv1.SolveAssemblyResponse, solveErr error) ([]byte, error) {
	effective := proto.Clone(request).(*workerv1.SolveAssemblyRequest)
	if result != nil && result.EffectiveSolverProfile != nil {
		effective.SolverProfile = proto.Clone(result.EffectiveSolverProfile).(*workerv1.AssemblySolverProfile)
	}
	// Reference recipes are sometimes encoded in runtime geometry IDs. Replay
	// needs only equality/ownership, never those business snapshots. Compact the
	// private numerical namespace without changing the live request or model.
	ids := map[string]string{}
	for _, g := range effective.Geometry {
		if len(g.Id) > 128 {
			sum := sha256.Sum256([]byte(g.Id))
			ids[g.Id] = "geometry:" + hex.EncodeToString(sum[:])
			g.Id = ids[g.Id]
		}
	}
	for _, c := range effective.Constraints {
		for _, ref := range []*workerv1.AssemblyGeometryRef{c.First, c.Second, c.AngleReference} {
			if ref != nil {
				if id, ok := ids[ref.GeometryId]; ok {
					ref.GeometryId = id
				}
			}
		}
	}
	input, err := protojson.Marshal(effective)
	if err != nil {
		return nil, err
	}
	replay := AssemblyReplay{Schema: AssemblyReplaySchema, Units: "mm,rad; body-local geometry; quaternion xyzw", Request: input}
	if result != nil {
		compact := proto.Clone(result).(*workerv1.SolveAssemblyResponse)
		compact.EffectiveSolverProfile = nil
		compact.EquationResiduals = nil
		compact.ConstraintRanks = nil
		compact.Diagnostics = nil
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
	return json.Marshal(replay)
}

// MakeAssemblyReplay freezes the exact production RPC encoding without a solve.
func MakeAssemblyReplay(requestID string, bodies []AssemblyBody, values []AssemblyGeometry, constraints []AssemblyConstraint, options AssemblySolveOptions) ([]byte, error) {
	return makeAssemblyReplay(assemblySolveRequest(requestID, bodies, values, constraints, options), nil, nil)
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
	output, err := makeAssemblyReplay(&input, result, solveErr)
	if err != nil {
		return nil, err
	}
	var updated AssemblyReplay
	if err = json.Unmarshal(output, &updated); err != nil {
		return nil, err
	}
	updated.Snapshot = replay.Snapshot
	return json.Marshal(updated)
}

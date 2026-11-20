package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	perf "github.com/occccad/occccad/internal/performance"
	"google.golang.org/protobuf/proto"
)

func (s *Service) readNamingBlock(ctx context.Context, object artifactstore.Object, reference *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
	if object.Kind != artifactstore.KindNamingHistory || object.SHA256 != reference.Sha256 || object.Size < 0 || uint64(object.Size) != reference.SizeBytes {
		return nil, namingError("CORRUPT", "TOPOLOGY_HISTORY_METADATA_INVALID", "命名历史块元数据不匹配。")
	}
	load := func() (*decodedNamingBlock, int64, error) {
		reader, err := s.artifacts.OpenReadyObject(ctx, object)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, database.ErrNoRows) {
				return nil, 0, namingError("CORRUPT", "TOPOLOGY_HISTORY_BLOCK_MISSING", "命名历史块缺失。")
			}
			return nil, 0, err
		}
		data, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return nil, 0, err
		}
		if closeErr != nil {
			return nil, 0, closeErr
		}
		block, err := decodeNamingBlock(ctx, data, reference, false)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			return nil, 0, namingError("CORRUPT", "TOPOLOGY_HISTORY_BLOCK_INVALID", err.Error())
		}
		return block, int64(block.retainedBytes), nil
	}
	if geometry.PartRuntime(ctx).ForceCold {
		block, _, err := load()
		return block, err
	}
	key := object.ID + "\x00" + object.SHA256 + "\x00" + modelcore.TopologyNamingPolicyDigest
	return s.namingBlocks.read(ctx, key, namingBlockBudget, load)
}

func (s *Service) namingHistoryLoader(ctx context.Context, parent *string) namingHistoryLoader {
	var objects map[string]artifactstore.Object
	var initialized bool
	return func(ctx context.Context, ref *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
		if s.artifacts == nil {
			return nil, fmt.Errorf("topology artifact store is not configured")
		}
		if !initialized && parent != nil {
			children, err := s.artifacts.NamingHistorySuffix(ctx, *parent, namingPrefix(ctx))
			if err != nil {
				return nil, err
			}
			objects = map[string]artifactstore.Object{}
			for _, object := range children {
				if object.Kind != artifactstore.KindNamingHistory {
					return nil, fmt.Errorf("invalid naming root dependency kind")
				}
				objects[object.SHA256] = object
			}
			initialized = true
		}
		if parent == nil {
			var err error
			objects, err = s.artifacts.ReadyByDigests(ctx, artifactstore.KindNamingHistory, []string{ref.Sha256})
			if err != nil {
				return nil, err
			}
		}
		object, ok := objects[ref.Sha256]
		if !ok {
			return nil, namingError("CORRUPT", "TOPOLOGY_HISTORY_BLOCK_MISSING", "拓扑命名历史块缺失，需要恢复制品或重新求值。")
		}
		return s.readNamingBlock(ctx, object, ref)
	}
}

func (s *Service) adoptNamingHistory(ctx context.Context, m *workerv1.PartEvaluationManifest, geometryID, brep string, displayAssociations bool) (artifactstore.Object, *topologyManifest, error) {
	defer perf.Start(ctx, "naming-adopt")()
	finish := perf.Start(ctx, "naming-stage-root")
	root, err := s.adoptEvaluationObject(ctx, m.TopologyManifestArtifact, artifactstore.KindTopologyManifest)
	finish()
	if err != nil {
		return root, nil, err
	}
	if err = validateTopologyManifestDigests(m.TopologyManifestDigest, m.GetTopologyManifestArtifact().GetSha256(), root.SHA256); err != nil {
		return root, nil, err
	}
	finish = perf.Start(ctx, "naming-root-read")
	reader, err := s.artifacts.OpenReadyObject(ctx, root)
	if err != nil {
		return root, nil, err
	}
	data, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil {
		return root, nil, err
	}
	if closeErr != nil {
		return root, nil, closeErr
	}
	finish()
	finish = perf.Start(ctx, "naming-root-decode")
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != root.SHA256 {
		return root, nil, fmt.Errorf("adopted Naming digest mismatch")
	}
	stored := &workerv1.PartTopologyManifest{}
	if err = proto.Unmarshal(data, stored); err != nil {
		return root, nil, err
	}
	finish()
	finish = perf.Start(ctx, "naming-prefix-check")
	wanted := map[string]*workerv1.NamingHistoryReference{}
	digests := []string{}
	for _, ref := range stored.History {
		if wanted[ref.Sha256] != nil {
			return root, nil, fmt.Errorf("duplicate naming history reference")
		}
		wanted[ref.Sha256] = ref
		digests = append(digests, ref.Sha256)
	}
	checkpoint, continuing := ctx.Value(namingContinuationKey{}).(partCheckpoint)
	prefix := 0
	if continuing && len(checkpoint.naming.History) < len(stored.History) {
		prefix = len(checkpoint.naming.History)
		for i, ref := range checkpoint.naming.History {
			if !proto.Equal(ref, stored.History[i]) {
				prefix = 0
				break
			}
		}
	}
	finish()
	finish = perf.Start(ctx, "naming-adopt-blocks")
	objects, err := s.artifacts.ReadyByDigests(ctx, artifactstore.KindNamingHistory, digests[prefix:])
	if err != nil {
		return root, nil, err
	}
	provided := map[string]bool{}
	for _, ref := range m.NamingHistoryArtifacts {
		expected := wanted[ref.Sha256]
		if expected == nil || provided[ref.Sha256] || ref.SizeBytes != expected.SizeBytes || ref.Backend != "LOCAL" {
			return root, nil, fmt.Errorf("unexpected naming history output")
		}
		provided[ref.Sha256] = true
		object, err := s.adoptEvaluationObject(ctx, ref, artifactstore.KindNamingHistory)
		if err != nil {
			return root, nil, err
		}
		objects[object.SHA256] = object
	}
	finish()
	if stored.GeometryId != geometryID || stored.BrepSha256 != brep {
		return root, nil, fmt.Errorf("feature association snapshot mismatch")
	}
	if prefix > 0 {
		view, err := s.adoptNamingContinuation(ctx, checkpoint, root, stored, objects, displayAssociations)
		if view != nil || err != nil {
			return root, view, err
		}
		older, err := s.artifacts.ReadyByDigests(ctx, artifactstore.KindNamingHistory, digests[:prefix])
		if err != nil {
			return root, nil, err
		}
		for digest, object := range older {
			objects[digest] = object
		}
	}
	finish = perf.Start(ctx, "naming-full-history")
	view, err := unpackVerifiedNamingRoot(ctx, stored, len(data), false, func(ctx context.Context, ref *workerv1.NamingHistoryReference) (*decodedNamingBlock, error) {
		object, ok := objects[ref.Sha256]
		if !ok {
			return nil, namingError("CORRUPT", "TOPOLOGY_HISTORY_BLOCK_MISSING", "求值结果缺少命名历史块。")
		}
		return s.readNamingBlock(ctx, object, ref)
	})
	finish()
	if err != nil {
		return root, nil, err
	}
	if view.GeometryID != geometryID || view.BRepSHA256 != brep {
		return root, nil, fmt.Errorf("feature association snapshot mismatch")
	}
	ordered := make([]artifactstore.Object, 0, len(stored.History))
	for _, ref := range stored.History {
		ordered = append(ordered, objects[ref.Sha256])
	}
	if err = s.artifacts.RecordNamingClosure(ctx, root, ordered); err != nil {
		return root, nil, err
	}
	return root, view, nil
}

func namingViewKey(id, digest string) string {
	return id + "\x00" + digest + "\x00" + modelcore.TopologyNamingPolicyDigest
}
func namingViewCharge(view *topologyManifest, key string) int64 {
	if int64(view.retainedBytes) > namingViewBudget {
		return namingViewBudget + 1
	}
	return int64(view.retainedBytes) + int64(len(key)) + 128
}

func (s *Service) historyArtifactReference(ctx context.Context, parent, digest string) (*workerv1.ArtifactReference, error) {
	objects, err := s.artifacts.NamingHistoryObjects(ctx, parent, digest)
	if err != nil {
		return nil, err
	}
	for _, o := range objects {
		if o.Kind == artifactstore.KindNamingHistory && o.SHA256 == digest {
			return &workerv1.ArtifactReference{Backend: o.Backend, ObjectKey: o.Key, Sha256: o.SHA256, SizeBytes: uint64(o.Size), ContentType: o.ContentType}, nil
		}
	}
	return nil, fmt.Errorf("naming history dependency unavailable: %s", strconv.Quote(digest))
}

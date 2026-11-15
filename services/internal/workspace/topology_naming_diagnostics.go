package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

// NamingAvailability describes artifact capability, not a per-selection result.
type NamingAvailability struct {
	Status         string `json:"status"`
	CanBind        bool   `json:"canBind"`
	DiagnosticCode string `json:"diagnosticCode,omitempty"`
	Diagnostic     string `json:"diagnostic,omitempty"`
}

type topologyNamingError struct{ NamingAvailability }

func (e *topologyNamingError) Error() string { return e.DiagnosticCode + ": " + e.Diagnostic }
func (e *topologyNamingError) Unwrap() error { return ErrValidation }
func namingError(status, code, message string) error {
	return &topologyNamingError{NamingAvailability{Status: status, DiagnosticCode: code, Diagnostic: message}}
}
func namingDiagnostic(err error) (NamingAvailability, bool) {
	var failure *topologyNamingError
	if errors.As(err, &failure) {
		return failure.NamingAvailability, true
	}
	return NamingAvailability{}, false
}

func decodeTopologyManifest(data []byte, digest string) (*topologyManifest, error) {
	return decodeTopologyManifestContext(context.Background(), data, digest)
}

func decodeTopologyManifestContext(ctx context.Context, data []byte, digest string) (*topologyManifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(digest)) {
		return nil, namingError("CORRUPT", "TOPOLOGY_MANIFEST_DIGEST_MISMATCH", "拓扑命名制品摘要不匹配，需要重新生成制品。")
	}
	stored := &workerv1.PartTopologyManifest{}
	if err := proto.Unmarshal(data, stored); err != nil {
		return nil, namingError("CORRUPT", "TOPOLOGY_MANIFEST_INVALID", "拓扑命名制品无法解码，需要重新生成制品。")
	}
	if stored.GetSchemaVersion() != 2 || stored.GetPolicyDigest() != modelcore.TopologyNamingPolicyDigest {
		return nil, namingError("INCOMPATIBLE", "TOPOLOGY_MANIFEST_CONTRACT_MISMATCH", "拓扑命名制品与当前求值合同不匹配，需要重新求值。")
	}
	manifest, err := unpackNaming(stored)
	if err != nil {
		return nil, namingError("CORRUPT", "TOPOLOGY_MANIFEST_INVALID", err.Error())
	}
	if !topologyHistoryComplete(manifest) {
		return nil, namingError("FAILED", "TOPOLOGY_HISTORY_INCOMPLETE", "当前几何未生成完整拓扑命名，暂不支持持久面、边、点引用。")
	}
	manifest.search, err = buildNamingSearchIndex(ctx, manifest)
	if err != nil {
		return nil, err
	}
	manifest.encodedBytes = len(data)
	return manifest, nil
}

func (service *Service) readTopologyManifest(ctx context.Context, inline []byte, objectID, digest *string) (*topologyManifest, string, error) {
	// Inline bytes are validated every time. Object keys include both the
	// concrete immutable object ID and its expected digest, never just a caller's
	// claimed SHA or geometry key. Cold evaluation bypasses shared parsed views.
	if len(inline) > 0 || objectID == nil || digest == nil || strings.TrimSpace(*digest) == "" || geometry.PartRuntime(ctx).ForceCold {
		return service.loadTopologyManifest(ctx, inline, objectID, digest)
	}
	key := *objectID + "\x00" + strings.ToLower(strings.TrimSpace(*digest)) + "\x00" + modelcore.TopologyNamingPolicyDigest
	manifest, err := service.namingViews.read(ctx, key, namingViewBudget, func() (*topologyManifest, int64, error) {
		value, _, err := service.loadTopologyManifest(ctx, inline, objectID, digest)
		if err != nil {
			return nil, 0, err
		}
		charge := namingViewBudget + 1
		if int64(value.encodedBytes) <= namingViewBudget/16 {
			charge = int64(value.encodedBytes)*16 + int64(len(key)) + 128
		}
		return value, charge, nil
	})
	return manifest, strings.TrimSpace(*digest), err
}

func (service *Service) loadTopologyManifest(ctx context.Context, inline []byte, objectID, digest *string) (*topologyManifest, string, error) {
	if len(inline) == 0 && objectID == nil && (digest == nil || strings.TrimSpace(*digest) == "") {
		return nil, "", namingError("UNAVAILABLE", "TOPOLOGY_NAMING_UNAVAILABLE", "当前几何没有拓扑命名；可继续查看和使用基准几何，面上草图、边点投影及拓扑约束需要先建立导入命名。")
	}
	if digest == nil || strings.TrimSpace(*digest) == "" {
		return nil, "", namingError("CORRUPT", "TOPOLOGY_MANIFEST_METADATA_INVALID", "拓扑命名制品存在但缺少摘要，不能安全绑定持久引用。")
	}
	data := inline
	if len(data) == 0 && objectID != nil {
		if service.artifacts == nil {
			return nil, "", fmt.Errorf("topology artifact store is not configured")
		}
		_, reader, err := service.artifacts.Open(ctx, *objectID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, database.ErrNoRows) {
				return nil, "", namingError("CORRUPT", "TOPOLOGY_MANIFEST_OBJECT_MISSING", "拓扑命名制品已登记但对象丢失，需要恢复或重新生成制品。")
			}
			return nil, "", err
		}
		data, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return nil, "", err
		}
		if closeErr != nil {
			return nil, "", closeErr
		}
	}
	if len(data) == 0 {
		return nil, "", namingError("CORRUPT", "TOPOLOGY_MANIFEST_OBJECT_MISSING", "拓扑命名摘要已登记但制品内容缺失，需要恢复或重新生成制品。")
	}
	manifest, err := decodeTopologyManifestContext(ctx, data, *digest)
	return manifest, strings.TrimSpace(*digest), err
}

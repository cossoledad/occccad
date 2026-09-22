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

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
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

func decodeTopologyManifest(data []byte, digest string) (*workerv1.PartTopologyManifest, error) {
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(digest)) {
		return nil, namingError("CORRUPT", "TOPOLOGY_MANIFEST_DIGEST_MISMATCH", "拓扑命名制品摘要不匹配，需要重新生成制品。")
	}
	manifest := &workerv1.PartTopologyManifest{}
	if err := proto.Unmarshal(data, manifest); err != nil {
		return nil, namingError("CORRUPT", "TOPOLOGY_MANIFEST_INVALID", "拓扑命名制品无法解码，需要重新生成制品。")
	}
	if manifest.GetSchemaVersion() != modelcore.TopologyNamingSchemaVersion || manifest.GetPolicyDigest() != modelcore.TopologyNamingPolicyDigest {
		return nil, namingError("INCOMPATIBLE", "TOPOLOGY_MANIFEST_CONTRACT_MISMATCH", "拓扑命名制品与当前求值合同不匹配，需要重新求值。")
	}
	if !topologyHistoryComplete(manifest) {
		return nil, namingError("FAILED", "TOPOLOGY_HISTORY_INCOMPLETE", "当前几何未生成完整拓扑命名，暂不支持持久面、边、点引用。")
	}
	return manifest, nil
}

func (service *Service) readTopologyManifest(ctx context.Context, inline []byte, objectID, digest *string) (*workerv1.PartTopologyManifest, string, error) {
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
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, pgx.ErrNoRows) {
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
	manifest, err := decodeTopologyManifest(data, *digest)
	return manifest, strings.TrimSpace(*digest), err
}

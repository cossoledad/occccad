package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

func TestTopologyManifestDiagnostics(t *testing.T) {
	ready := &workerv1.PartTopologyManifest{SchemaVersion: modelcore.TopologyNamingSchemaVersion, PolicyDigest: modelcore.TopologyNamingPolicyDigest, FeatureResults: []*workerv1.FeatureResult{{FeatureId: "extrude", TopologyHistoryComplete: true}}}
	valid, _ := proto.Marshal(ready)
	incomplete := proto.Clone(ready).(*workerv1.PartTopologyManifest)
	incomplete.FeatureResults[0].TopologyHistoryComplete = false
	failed, _ := proto.Marshal(incomplete)
	incompatible := proto.Clone(ready).(*workerv1.PartTopologyManifest)
	incompatible.PolicyDigest = "other-policy"
	old, _ := proto.Marshal(incompatible)
	digestOf := func(data []byte) *string {
		sum := sha256.Sum256(data)
		value := hex.EncodeToString(sum[:])
		return &value
	}
	empty := ""
	tests := []struct {
		name         string
		data         []byte
		digest       *string
		status, code string
	}{
		{"nullable import", nil, nil, "UNAVAILABLE", "TOPOLOGY_NAMING_UNAVAILABLE"},
		{"legacy empty", nil, &empty, "UNAVAILABLE", "TOPOLOGY_NAMING_UNAVAILABLE"},
		{"missing digest", valid, nil, "CORRUPT", "TOPOLOGY_MANIFEST_METADATA_INVALID"},
		{"missing content", nil, digestOf(valid), "CORRUPT", "TOPOLOGY_MANIFEST_OBJECT_MISSING"},
		{"wrong digest", valid, digestOf([]byte("other")), "CORRUPT", "TOPOLOGY_MANIFEST_DIGEST_MISMATCH"},
		{"bad protobuf", []byte{255}, digestOf([]byte{255}), "CORRUPT", "TOPOLOGY_MANIFEST_INVALID"},
		{"incomplete", failed, digestOf(failed), "FAILED", "TOPOLOGY_HISTORY_INCOMPLETE"},
		{"different policy", old, digestOf(old), "INCOMPATIBLE", "TOPOLOGY_MANIFEST_CONTRACT_MISMATCH"},
		{"native ready", valid, digestOf(valid), "", ""},
	}
	service := &Service{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, _, err := service.readTopologyManifest(t.Context(), tt.data, nil, tt.digest)
			if tt.status == "" {
				if err != nil || !topologyHistoryComplete(manifest) {
					t.Fatalf("ready manifest: %v", err)
				}
				return
			}
			diagnostic, ok := namingDiagnostic(err)
			if !ok || diagnostic.Status != tt.status || diagnostic.DiagnosticCode != tt.code || diagnostic.CanBind || manifest != nil || !errors.Is(err, ErrValidation) {
				t.Fatalf("manifest failure misclassified: %+v %v", diagnostic, err)
			}
		})
	}
	object := "object-id"
	// An unconfigured store is infrastructure failure, not a fabricated missing
	// manifest. The caller must propagate it rather than declare the part ready.
	_, _, err := service.readTopologyManifest(t.Context(), nil, &object, digestOf(valid))
	if _, ok := namingDiagnostic(err); err == nil || ok {
		t.Fatalf("infrastructure failure hidden: %v", err)
	}
}

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

func TestTopologyManifestDiagnostics(t *testing.T) {
	ready := &workerv1.PartTopologyManifest{GeometryId: "geometry", SchemaVersion: 2, PolicyDigest: modelcore.TopologyNamingPolicyDigest, CoordinateSpace: "PART_LOCAL", LengthUnit: "mm", Transitions: []*workerv1.NamingTransition{{FeatureId: "extrude", BodyId: "body-main", Complete: true}}, Bodies: []*workerv1.NamingBody{{BodyId: "body-main", Transitions: []uint32{1}, TipTransition: 1}}}
	valid, _ := proto.Marshal(ready)
	incomplete := proto.Clone(ready).(*workerv1.PartTopologyManifest)
	incomplete.Transitions[0].Complete = false
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

func TestTopologyManifestViewsCannotBypassArtifactQualification(t *testing.T) {
	service := &Service{}
	object, digest := "immutable-object", "expected-sha"
	key := object + "\x00" + digest + "\x00" + modelcore.TopologyNamingPolicyDigest
	view := &topologyManifest{GeometryID: "qualified"}
	_, err := service.namingViews.read(t.Context(), key, namingViewBudget, func() (*topologyManifest, int64, error) {
		return view, 10, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _, err := service.readTopologyManifest(t.Context(), nil, &object, &digest); err != nil || got != view {
		t.Fatal("validated immutable view was not reused", err)
	}
	otherObject, otherDigest := "other-object", "other-sha"
	for _, test := range []struct {
		name           string
		inline         []byte
		object, digest *string
		cold           bool
	}{
		{"different object", nil, &otherObject, &digest, false},
		{"different digest", nil, &object, &otherDigest, false},
		{"inline bytes", []byte("corrupt"), &object, &digest, false},
		{"cold bypass", nil, &object, &digest, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			if test.cold {
				ctx = geometry.WithPartRuntime(ctx, geometry.PartRuntimeOptions{ForceCold: true})
			}
			if got, _, err := service.readTopologyManifest(ctx, test.inline, test.object, test.digest); err == nil || got != nil {
				t.Fatal("cache bypassed immutable object/digest/cold qualification", got, err)
			}
		})
	}
}

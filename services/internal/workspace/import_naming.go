package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
)

const importPolicy = "occccad.import.frozen-brep.v1"
const typeRepairImportNaming = "occccad://part/exchange/repair-naming"

// Source describes the original exchange object, not a display artifact.
type ImportSource struct {
	ObjectID       string `json:"objectId"`
	SHA256         string `json:"sha256"`
	Format         string `json:"format"`
	ComponentIndex uint32 `json:"componentIndex"`
}
type importedIdentity struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	LocalID uint64 `json:"localId"` // valid only for BRepSHA256
}
type importDefinition struct {
	FeatureID   string             `json:"featureId"`
	BodyID      string             `json:"bodyId"`
	GeometryKey string             `json:"geometryKey"`
	BRepSHA256  string             `json:"brepSha256"`
	Policy      string             `json:"policy"`
	OCCTVersion string             `json:"occtVersion"`
	Units       string             `json:"units"`
	Source      *ImportSource      `json:"source,omitempty"`
	Identities  []importedIdentity `json:"identities"`
}

func (service *Service) allocateImportDefinition(ctx context.Context, documentID string, feature Feature, source *ImportSource) (string, error) {
	id := commandEntityID("import-definition", documentID+"/"+feature.ID+"/"+importPolicy)
	var stored []byte
	err := service.database.QueryRow(ctx, `SELECT definition FROM occccad.import_definitions WHERE id=$1`, id).Scan(&stored)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var topology struct{ Faces, Edges, Vertices, Solids int }
	var topologyJSON, inline []byte
	var digest *string
	var occt string
	err = service.database.QueryRow(ctx, `SELECT a.topology_json,a.brep_data,o.sha256,a.occt_version FROM occccad.geometry_artifacts a LEFT JOIN occccad.artifact_objects o ON o.id=a.brep_object_id WHERE a.geometry_key=$1`, feature.GeometryKey).Scan(&topologyJSON, &inline, &digest, &occt)
	if err != nil {
		return "", err
	}
	if err = json.Unmarshal(topologyJSON, &topology); err != nil {
		return "", err
	}
	if topology.Solids != 1 || topology.Faces == 0 {
		return "", fmt.Errorf("%w: IMPORT_NAMING_REQUIRES_VALID_SINGLE_SOLID", ErrValidation)
	}
	if digest == nil {
		if len(inline) == 0 {
			return "", fmt.Errorf("%w: IMPORT_BREP_UNAVAILABLE", ErrValidation)
		}
		sum := sha256.Sum256(inline)
		value := hex.EncodeToString(sum[:])
		digest = &value
	}
	def := importDefinition{FeatureID: feature.ID, BodyID: "body-main", GeometryKey: feature.GeometryKey, BRepSHA256: *digest, Policy: importPolicy, OCCTVersion: occt, Units: "mm", Source: source}
	for _, group := range []struct {
		kind  string
		count int
	}{{"FACE", topology.Faces}, {"EDGE", topology.Edges}, {"VERTEX", topology.Vertices}} {
		for i := 1; i <= group.count; i++ {
			def.Identities = append(def.Identities, importedIdentity{ID: newID("import-topology"), Kind: group.kind, LocalID: uint64(i)})
		}
	}
	stored, err = json.Marshal(def)
	if err != nil {
		return "", err
	}
	var sourceID any
	if source != nil {
		if service.artifacts == nil {
			return "", fmt.Errorf("import source requires ArtifactStore")
		}
		object, err := service.artifacts.Get(ctx, source.ObjectID)
		if err != nil {
			return "", err
		}
		if object.SHA256 != source.SHA256 {
			return "", fmt.Errorf("%w: IMPORT_SOURCE_DIGEST_MISMATCH", ErrValidation)
		}
		sourceID = source.ObjectID
	}
	// Concurrent retries all use the winning immutable allocation; no locator-derived IDs.
	_, err = service.database.Exec(ctx, `INSERT INTO occccad.import_definitions(id,document_id,source_geometry_key,source_object_id,definition) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`, id, documentID, feature.GeometryKey, sourceID, stored)
	return id, err
}

func (service *Service) importSeed(ctx context.Context, feature Feature) (*workerv1.ImportTopologySeed, error) {
	var raw []byte
	if err := service.database.QueryRow(ctx, `SELECT definition FROM occccad.import_definitions WHERE id=$1`, feature.ImportDefinitionID).Scan(&raw); err != nil {
		return nil, err
	}
	var def importDefinition
	if err := json.Unmarshal(raw, &def); err != nil {
		return nil, err
	}
	if def.FeatureID != feature.ID || def.GeometryKey != feature.GeometryKey || def.Policy != importPolicy || def.Units != "mm" {
		return nil, fmt.Errorf("%w: IMPORT_DEFINITION_MISMATCH", ErrValidation)
	}
	seed := &workerv1.ImportTopologySeed{FeatureId: feature.ID, BodyId: def.BodyID, BrepSha256: def.BRepSHA256, OcctVersion: def.OCCTVersion, PolicyId: def.Policy}
	for _, entry := range def.Identities {
		kind := map[string]workerv1.PersistentTopologyType{"FACE": workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE, "EDGE": workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_EDGE, "VERTEX": workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_VERTEX}[entry.Kind]
		if kind == 0 {
			return nil, fmt.Errorf("%w: invalid imported topology type", ErrValidation)
		}
		seed.Identities = append(seed.Identities, &workerv1.ImportedTopologyIdentity{StableId: entry.ID, TopologyType: kind, LocalId: entry.LocalID})
	}
	return seed, nil
}

func applyRepairImportNaming(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var payload createFeaturePayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	for i, before := range model.Features {
		if before.ID != payload.Feature.ID {
			continue
		}
		if before.Type != "IMPORT_BODY" || before.ImportDefinitionID != "" || payload.Feature.ImportDefinitionID == "" {
			return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: import naming already exists or invalid repair", ErrValidation)
		}
		after := before
		after.ImportDefinitionID = payload.Feature.ImportDefinitionID
		model.Features[i] = after
		change, _ := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: before.ID, SlotID: "entity"}, before, after)
		raw, err := json.Marshal(model)
		return raw, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{"feature:" + modelcore.DependencyKey(before.ID)}}, err
	}
	return nil, modelcore.ChangeSet{}, ErrNotFound
}

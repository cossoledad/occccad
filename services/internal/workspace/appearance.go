package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	"github.com/occccad/occccad/internal/modelcore"
	"google.golang.org/protobuf/proto"
)

// SurfaceAppearance is Revision-owned display data. Face assignments use the
// same semantic identities as Naming; exchange local IDs never enter the model.
type SurfaceAppearance struct {
	Color string           `json:"color,omitempty"`
	Faces []FaceAppearance `json:"faces,omitempty"`
}
type FaceAppearance struct {
	Anchor modelcore.SemanticTopologyRef `json:"anchor"`
	Color  string                        `json:"color"`
}

// ResolvedAppearance is an ephemeral projection scoped to GeometryKey.
type ResolvedAppearance struct {
	GeometryKey     string            `json:"geometryKey,omitempty"`
	Color           string            `json:"color,omitempty"`
	Faces           map[uint64]string `json:"faces,omitempty"`
	UnresolvedFaces int               `json:"unresolvedFaces,omitempty"`
}

const typeSetBodyColor = "occccad://part/body/set-color"

var opaqueColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type bodyColorPayload struct {
	BodyID string `json:"bodyId"`
	Color  string `json:"color"` // empty resets the complete Body appearance
}

func applyBodyColor(modelJSON, payloadJSON json.RawMessage) (json.RawMessage, modelcore.ChangeSet, error) {
	var model PartModel
	var input bodyColorPayload
	if err := json.Unmarshal(modelJSON, &model); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if err := json.Unmarshal(payloadJSON, &input); err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	if input.Color != "" && !opaqueColor.MatchString(input.Color) {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: color must be sRGB #rrggbb", ErrValidation)
	}
	index := bodyIndex(model, input.BodyID)
	if index < 0 {
		return nil, modelcore.ChangeSet{}, fmt.Errorf("%w: Body does not exist", ErrValidation)
	}
	before := model.Bodies[index].Appearance
	model.Bodies[index].Appearance = nil
	if input.Color != "" {
		model.Bodies[index].Appearance = &SurfaceAppearance{Color: strings.ToLower(input.Color)}
	}
	change, err := modelcore.NewChange(modelcore.ChangeUpdate, modelcore.PropertyAddress{EntityID: input.BodyID, SlotID: "body.appearance"}, before, model.Bodies[index].Appearance)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	next, err := json.Marshal(model)
	return next, modelcore.ChangeSet{Changes: []modelcore.ModelChange{change}, ImpactSeeds: []modelcore.DependencyKey{modelcore.DependencyKey("appearance:" + input.BodyID)}}, err
}

func (s *Service) importedAppearance(ctx context.Context, feature Feature, source *ImportSource) (*SurfaceAppearance, error) {
	if source == nil || source.Appearance == nil {
		return nil, nil
	}
	input := source.Appearance
	if input.Color != "" && !opaqueColor.MatchString(input.Color) {
		return nil, fmt.Errorf("%w: invalid imported color", ErrValidation)
	}
	out := &SurfaceAppearance{Color: strings.ToLower(input.Color)}
	if len(input.Faces) == 0 {
		if out.Color == "" {
			return nil, nil
		}
		return out, nil
	}
	var objectID string
	if err := s.database.QueryRow(ctx, `SELECT identity_object_id::text FROM occccad.import_definitions WHERE id=$1`, feature.ImportDefinitionID).Scan(&objectID); err != nil {
		return nil, err
	}
	object, reader, err := s.artifacts.Open(ctx, objectID)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if int64(len(data)) != object.Size || hex.EncodeToString(digest[:]) != object.SHA256 {
		return nil, fmt.Errorf("import identity integrity mismatch")
	}
	var seed workerv1.ImportTopologySeed
	if err := proto.Unmarshal(data, &seed); err != nil {
		return nil, err
	}
	if source.AppearanceBRepSHA256 == "" || seed.BrepSha256 != source.AppearanceBRepSHA256 {
		return nil, fmt.Errorf("%w: IMPORT_COLOR_BREP_MISMATCH", ErrValidation)
	}
	identities := map[uint64]string{}
	for _, id := range seed.Identities {
		if id.TopologyType == workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE {
			identities[id.LocalId] = id.StableId
		}
	}
	seen := map[uint64]bool{}
	for _, face := range input.Faces {
		id := identities[face.LocalId]
		if id == "" || seen[face.LocalId] || !opaqueColor.MatchString(face.Color) {
			return nil, fmt.Errorf("%w: invalid imported face color binding", ErrValidation)
		}
		seen[face.LocalId] = true
		out.Faces = append(out.Faces, FaceAppearance{Anchor: modelcore.SemanticTopologyRef{FeatureID: feature.ID, OutputSlot: "import.topology", SourceIDs: []string{id}}, Color: strings.ToLower(face.Color)})
	}
	return out, nil
}

func resolveAppearance(body PartBody, manifest *topologyManifest) ResolvedAppearance {
	out := ResolvedAppearance{GeometryKey: body.GeometryKey}
	if body.Appearance == nil {
		return out
	}
	out.Color = body.Appearance.Color
	if len(body.Appearance.Faces) == 0 {
		return out
	}
	// Propagate through actual lineage. A merge of different colors is left
	// unresolved, never assigned by geometric proximity or traversal position.
	colors := map[string]string{}
	for _, face := range body.Appearance.Faces {
		colors[refKey(face.Anchor)] = face.Color
	}
	if manifest == nil || !topologyHistoryComplete(manifest) {
		out.UnresolvedFaces = len(body.Appearance.Faces)
		return out
	}
	for _, stage := range manifest.GetFeatureResults() {
		if stage.BodyId != body.ID {
			continue
		}
		for _, edge := range stage.GetTopologyHistory().GetLineage() {
			color, conflict := "", false
			for _, source := range edge.Sources {
				candidate := colors[refKey(semanticRef(source))]
				if candidate == "" {
					continue
				}
				if color != "" && color != candidate {
					conflict = true
				}
				color = candidate
			}
			key := refKey(semanticRef(edge.Result))
			if existing := colors[key]; existing != "" && color != "" && existing != color {
				conflict = true
			}
			if conflict {
				colors[key] = "!conflict"
			} else if color != "" {
				colors[key] = color
			}
		}
		for _, ambiguous := range stage.GetTopologyHistory().GetAmbiguous() {
			for _, source := range ambiguous.Sources {
				if colors[refKey(semanticRef(source))] != "" {
					out.UnresolvedFaces++
					break
				}
			}
		}
		for _, deleted := range stage.GetTopologyHistory().GetDeleted() {
			delete(colors, refKey(semanticRef(deleted.Source)))
		}
	}
	out.Faces = map[uint64]string{}
	if tip := manifest.bodyTips()[body.ID]; tip != nil {
		for _, face := range tip.SemanticOutputs {
			if face.TopologyType != workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE {
				continue
			}
			color := colors[refKey(semanticRef(face.SemanticRef))]
			if color == "!conflict" {
				out.UnresolvedFaces++
				continue
			}
			if color != "" {
				out.Faces[face.LocalId] = color
			}
		}
	}
	return out
}
func (s *Service) bodyAppearance(ctx context.Context, body PartBody) (ResolvedAppearance, error) {
	if body.Appearance == nil || len(body.Appearance.Faces) == 0 {
		return resolveAppearance(body, nil), nil
	}
	if body.GeometryKey == "" {
		return resolveAppearance(body, nil), nil
	}
	raw, _ := json.Marshal(body.Appearance)
	key := assemblyReadKey{"appearance", body.ID, body.GeometryKey + "/" + modelcore.ValueDigest(raw)}
	return assemblyRead(ctx, key, func() (ResolvedAppearance, error) {
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, body.GeometryKey)
		if err != nil {
			return ResolvedAppearance{}, err
		}
		return resolveAppearance(body, manifest), nil
	})
}
func (s *Service) appearanceAtGeometry(ctx context.Context, documentID, revisionID, bodyID, geometryKey string) (ResolvedAppearance, error) {
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, documentID, revisionID).Scan(&raw); err != nil {
		return ResolvedAppearance{}, err
	}
	var part PartModel
	if err := json.Unmarshal(raw, &part); err != nil {
		return ResolvedAppearance{}, err
	}
	index := bodyIndex(part, bodyID)
	if index < 0 {
		return ResolvedAppearance{}, fmt.Errorf("%w: appearance Body missing", ErrValidation)
	}
	body := part.Bodies[index]
	body.GeometryKey = geometryKey
	return s.bodyAppearance(ctx, body)
}
func exchangeAppearance(input ResolvedAppearance) *workerv1.ExchangeAppearance {
	out := &workerv1.ExchangeAppearance{Color: input.Color}
	ids := make([]uint64, 0, len(input.Faces))
	for id := range input.Faces {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		out.Faces = append(out.Faces, &workerv1.ExchangeFaceColor{LocalId: id, Color: input.Faces[id]})
	}
	return out
}

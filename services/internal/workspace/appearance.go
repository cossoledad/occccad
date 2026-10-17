package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
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
	GraphicProperties
	Color string           `json:"color,omitempty"`
	Faces []FaceAppearance `json:"faces,omitempty"`
}
type FaceAppearance struct {
	GraphicProperties
	Anchor          modelcore.SemanticTopologyRef `json:"anchor"`
	Color           string                        `json:"color"`
	InheritedFields []string                      `json:"inheritedFields,omitempty"`
}

// ResolvedAppearance is an ephemeral projection scoped to GeometryKey.
type ResolvedAppearance struct {
	Issues []AppearanceIssue `json:"issues,omitempty"`
	GraphicProperties
	Sources         map[string]AppearanceSource            `json:"sources,omitempty"`
	FaceSources     map[uint64]map[string]AppearanceSource `json:"faceSources,omitempty"`
	GeometryKey     string                                 `json:"geometryKey,omitempty"`
	Color           string                                 `json:"color,omitempty"`
	Faces           map[uint64]string                      `json:"faces,omitempty"`
	FaceProperties  map[uint64]GraphicProperties           `json:"faceProperties,omitempty"`
	FaceInherit     map[uint64][]string                    `json:"faceInherit,omitempty"`
	UnresolvedFaces int                                    `json:"unresolvedFaces,omitempty"`
}

type AppearanceIssue struct {
	Code           string `json:"code"`
	OccurrencePath string `json:"occurrencePath,omitempty"`
	BodyID         string `json:"bodyId,omitempty"`
}

const typeSetBodyColor = "occccad://part/body/set-color"

var opaqueColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type bodyColorPayload struct {
	BodyID string `json:"bodyId"`
	Color  string `json:"color"` // empty restores inherited Body color
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
	value := json.RawMessage("null")
	if input.Color != "" {
		value, _ = json.Marshal(strings.ToLower(input.Color))
	}
	after, err := patchAppearance(before, map[string]json.RawMessage{"color": value}, false)
	if err != nil {
		return nil, modelcore.ChangeSet{}, err
	}
	model.Bodies[index].Appearance = after

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
	out := &SurfaceAppearance{GraphicProperties: GraphicProperties{Opacity: input.Opacity}, Color: strings.ToLower(input.Color)}
	if err := validateSurfaceStyle(appearanceStyle(out)); err != nil {
		return nil, err
	}
	if len(input.Faces) == 0 {
		if out.Color == "" && out.Opacity == nil {
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
		if id == "" || seen[face.LocalId] || (face.Color != "" && !opaqueColor.MatchString(face.Color)) || (face.Color == "" && face.Opacity == nil) {
			return nil, fmt.Errorf("%w: invalid imported face color binding", ErrValidation)
		}
		if err := validateSurfaceStyle(SurfaceStyle{GraphicProperties: GraphicProperties{Opacity: face.Opacity}, Color: face.Color}); err != nil {
			return nil, err
		}
		seen[face.LocalId] = true
		out.Faces = append(out.Faces, FaceAppearance{GraphicProperties: GraphicProperties{Opacity: face.Opacity}, Anchor: modelcore.SemanticTopologyRef{FeatureID: feature.ID, OutputSlot: "import.topology", SourceIDs: []string{id}}, Color: strings.ToLower(face.Color)})
	}
	return out, nil
}

func resolveAppearance(body PartBody, manifest *topologyManifest) ResolvedAppearance {
	out := ResolvedAppearance{GeometryKey: body.GeometryKey}
	if body.Appearance == nil {
		return out
	}
	out.Color = body.Appearance.Color
	out.GraphicProperties = body.Appearance.GraphicProperties
	if len(body.Appearance.Faces) == 0 {
		return out
	}
	if manifest == nil || !topologyHistoryComplete(manifest) {
		out.UnresolvedFaces = len(body.Appearance.Faces)
		return out
	}
	type fields = map[string]json.RawMessage
	assignments := map[string]fields{}
	styles := map[string]fields{}
	conflicts := map[string]map[string]bool{}
	for _, face := range body.Appearance.Faces {
		key := refKey(face.Anchor)
		raw, _ := json.Marshal(SurfaceStyle{GraphicProperties: face.GraphicProperties, Color: face.Color})
		var values fields
		_ = json.Unmarshal(raw, &values)
		for _, field := range face.InheritedFields {
			values[field] = json.RawMessage("null")
		}
		assignments[key] = values
		styles[key] = values
	}
	for _, stage := range manifest.GetFeatureResults() {
		if stage.BodyId != body.ID {
			continue
		}
		for _, edge := range stage.GetTopologyHistory().GetLineage() {
			key := refKey(semanticRef(edge.Result))
			merged := fields{}
			failed := map[string]bool{}
			merge := func(source string) {
				for field, value := range styles[source] {
					if previous, ok := merged[field]; ok && string(previous) != string(value) {
						failed[field] = true
					}
					merged[field] = value
				}
				for field := range conflicts[source] {
					failed[field] = true
				}
			}
			for _, source := range edge.Sources {
				merge(refKey(semanticRef(source)))
			}
			if assignments[key] == nil {
				merge(key)
			}
			// An explicit assignment on this stable result overrides inherited values
			// for its fields. Unrelated ambiguous lineage remains unresolved.
			for field, value := range assignments[key] {
				merged[field] = value
				delete(failed, field)
			}
			styles[key] = merged
			conflicts[key] = failed
		}
		for _, ambiguous := range stage.GetTopologyHistory().GetAmbiguous() {
			for _, source := range ambiguous.Sources {
				key := refKey(semanticRef(source))
				if len(styles[key]) > 0 || len(conflicts[key]) > 0 {
					out.UnresolvedFaces++
					break
				}
			}
		}
		for _, deleted := range stage.GetTopologyHistory().GetDeleted() {
			key := refKey(semanticRef(deleted.Source))
			delete(styles, key)
			delete(conflicts, key)
		}
	}
	out.Faces = map[uint64]string{}
	out.FaceProperties = map[uint64]GraphicProperties{}
	out.FaceInherit = map[uint64][]string{}
	if tip := manifest.bodyTips()[body.ID]; tip != nil {
		for _, face := range tip.SemanticOutputs {
			if face.TopologyType != workerv1.PersistentTopologyType_PERSISTENT_TOPOLOGY_TYPE_FACE {
				continue
			}
			key := refKey(semanticRef(face.SemanticRef))
			if len(conflicts[key]) > 0 {
				out.UnresolvedFaces++
				continue
			}
			for field, value := range styles[key] {
				if string(value) == "null" {
					out.FaceInherit[face.LocalId] = append(out.FaceInherit[face.LocalId], field)
				}
			}
			sort.Strings(out.FaceInherit[face.LocalId])
			raw, _ := json.Marshal(styles[key])
			var style SurfaceStyle
			_ = json.Unmarshal(raw, &style)
			if style.Color != "" {
				out.Faces[face.LocalId] = style.Color
			}
			if !reflect.DeepEqual(style.GraphicProperties, GraphicProperties{}) {
				out.FaceProperties[face.LocalId] = style.GraphicProperties
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
	return s.bodyAppearanceWithDefaults(ctx, body, part.DefaultAppearance)
}
func exchangeAppearance(input ResolvedAppearance) *workerv1.ExchangeAppearance {
	out := &workerv1.ExchangeAppearance{Color: input.Color, Opacity: input.Opacity}
	ids := map[uint64]bool{}
	for id := range input.Faces {
		ids[id] = true
	}
	for id, p := range input.FaceProperties {
		if p.Opacity != nil {
			ids[id] = true
		}
	}
	ordered := make([]uint64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, id := range ordered {
		color := input.Faces[id]
		if color == "" {
			color = input.Color
		}
		if color == "" {
			color = "#98a8b8"
		}
		out.Faces = append(out.Faces, &workerv1.ExchangeFaceColor{LocalId: id, Color: color, Opacity: input.FaceProperties[id].Opacity})
	}
	return out
}

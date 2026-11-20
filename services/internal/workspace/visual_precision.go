package workspace

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/visual"
)

// VisualizationPrecision changes only the derived display of a frozen source.
// originalID must already be authorized in the caller's representation scope.
func (s *Service) VisualizationPrecision(ctx context.Context, key, originalID, precision string) (artifactstore.Object, error) {
	linear, angular := 0.15, 0.30
	switch precision {
	case "balanced":
	case "fine":
		linear, angular = 0.05, 0.18
	case "extra-fine":
		linear, angular = 0.02, 0.10
	default:
		return artifactstore.Object{}, fmt.Errorf("%w: invalid display precision", ErrValidation)
	}
	a, err := s.loadArtifact(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return artifactstore.Object{}, ErrNotFound
	}
	if err != nil {
		return artifactstore.Object{}, err
	}
	original := a.Representations["VISUAL"]
	if original.ObjectID != originalID {
		return artifactstore.Object{}, ErrNotFound
	}
	current := a.Visualization.Precision
	if current == "" {
		current = "fine" // frozen visuals created before precision was recorded
	}
	if a.Representations["BREP"].ObjectID == "" || current == precision {
		return s.artifacts.Get(ctx, originalID)
	}
	variant := "sha256:" + modelcore.ValueDigest([]byte("visual-precision-v1\x00"+displayStrategyVersion+"\x00"+original.Digest+"\x00"+precision))
	if object, err := s.representationObject(ctx, variant, "VISUAL"); !errors.Is(err, sql.ErrNoRows) {
		return object, err
	}
	brep, err := s.brepArtifactReference(ctx, key)
	if err != nil {
		return artifactstore.Object{}, err
	}
	_, reader, err := s.artifacts.Open(ctx, originalID)
	if err != nil {
		return artifactstore.Object{}, err
	}
	v, err := frozenVisualization(reader, original.Size, a.GeometryID, a.Representations["NAMING"].Digest)
	reader.Close()
	if err != nil {
		return artifactstore.Object{}, err
	}
	v.Precision = precision
	output := artifactstore.StagingKey(newID("visual-precision"), "mesh.glb")
	result, err := s.worker.Tessellate(ctx, a.GeometryID, brep, linear, angular, output)
	if err != nil {
		return artifactstore.Object{}, err
	}
	ref := result.GetVisualArtifact()
	if ref == nil || ref.ObjectKey != output || ref.SizeBytes > math.MaxInt64 {
		return artifactstore.Object{}, fmt.Errorf("invalid tessellation artifact")
	}
	object, err := s.artifacts.AdoptTransformed(ctx, artifactstore.KindGLB, "model/gltf-binary", ref.ObjectKey, ref.Sha256, int64(ref.SizeBytes), func(data io.Reader, size int64) (io.Reader, error) {
		return glbWithVisualizationReader(data, size, v, map[string]string{"geometryId": a.GeometryID, "namingDigest": a.Representations["NAMING"].Digest})
	})
	if err != nil {
		return artifactstore.Object{}, err
	}
	a.Visualization, a.TriangleCount, a.DisplayVertexCount = v, result.TriangleCount, result.DisplayVertexCount
	if err = s.persistGeometry(ctx, variant, a, map[string]artifactstore.Object{"VISUAL": object}); err != nil {
		return artifactstore.Object{}, err
	}
	return s.representationObject(ctx, variant, "VISUAL")
}

type frozenVisualHeader[V any] struct {
	Extensions struct {
		CAD    struct{ Association visual.Association } `json:"OCCCCAD_cad"`
		Visual V                                        `json:"OCCCCAD_visualization"`
	}
	BufferViews []struct{ Buffer, ByteOffset, ByteLength, ByteStride int }
	Accessors   []struct {
		BufferView, ByteOffset, Count, ComponentType int
		Type                                         string
	}
}

func readFrozenVisualHeader[V any](source io.Reader, size int64, geometryID, namingDigest string) (frozenVisualHeader[V], int, error) {
	var doc frozenVisualHeader[V]
	chunk, err := readVisualizationJSON(source, size)
	if err != nil {
		return doc, 0, err
	}
	if err := json.Unmarshal(chunk, &doc); err != nil {
		return doc, 0, err
	}
	association := doc.Extensions.CAD.Association
	if association.GeometryID != geometryID || association.NamingDigest != namingDigest {
		return doc, 0, fmt.Errorf("visualization source association mismatch")
	}
	return doc, len(chunk), nil
}

// Continuation needs the display index only. This leaves all GLB buffers and
// other visualization projections unread.
func readFrozenAssociations(source io.Reader, size int64, geometryID, namingDigest string) (*FeatureAssociationIndex, int64, error) {
	type associationFields struct {
		Index *FeatureAssociationIndex `json:"featureAssociations"`
		Table *featureAssociationTable `json:"featureAssociationTable"`
	}
	doc, _, err := readFrozenVisualHeader[*associationFields](source, size, geometryID, namingDigest)
	if err != nil {
		return nil, 0, err
	}
	wire := doc.Extensions.Visual
	if wire == nil {
		return nil, 0, fmt.Errorf("missing visualization associations")
	}
	if wire.Table != nil {
		index, err := wire.Table.unpack()
		if err != nil {
			return nil, 0, err
		}
		return index, wire.Table.decodedListBytes(), nil
	}
	if wire.Index == nil {
		return nil, 0, nil
	}
	// Expanded JSON owns the ID strings in each row independently.
	var bytes int64
	chargeIDs := func(ids []string) {
		bytes += int64(cap(ids)) * 16
		for _, id := range ids {
			bytes += int64(len(id))
		}
	}
	chargeIDs(wire.Index.Features)
	for _, e := range wire.Index.Elements {
		chargeIDs(e.Origins)
		chargeIDs(e.Primary)
		chargeIDs(e.Supports)
		chargeIDs(e.Modifiers)
		bytes += int64(cap(e.Members)) * 24
		for _, m := range e.Members {
			bytes += int64(len(m.PatternID))
		}
	}
	return wire.Index, bytes, nil
}

// Read only the metadata and the auxiliary sketch/annotation suffix. The
// original shaded mesh has no role in producing a different precision.
func frozenVisualization(source io.Reader, size int64, geometryID, namingDigest string) (VisualizationManifest, error) {
	var v VisualizationManifest
	doc, chunkSize, err := readFrozenVisualHeader[json.RawMessage](source, size, geometryID, namingDigest)
	if err != nil {
		return v, err
	}
	var fields struct {
		visualizationJSON
		BufferStart int64
		Primitives  []map[string]json.RawMessage
	}
	fields.visualizationManifestJSON = (*visualizationManifestJSON)(&v)
	if err = json.Unmarshal(doc.Extensions.Visual, &fields); err != nil {
		return v, err
	}
	if fields.Table != nil {
		if v.FeatureAssociations, err = fields.Table.unpack(); err != nil {
			return v, err
		}
	}
	if len(fields.Primitives) == 0 {
		return v, nil
	}
	var header [8]byte
	if _, err = io.ReadFull(source, header[:]); err != nil {
		return v, err
	}
	binarySize := int64(binary.LittleEndian.Uint32(header[:4]))
	if binary.LittleEndian.Uint32(header[4:]) != 0x004e4942 || binarySize != size-28-int64(chunkSize) || fields.BufferStart < 0 || fields.BufferStart > binarySize || binarySize-fields.BufferStart > 64<<20 {
		return v, fmt.Errorf("invalid visualization suffix length")
	}
	if seeker, ok := source.(io.Seeker); ok {
		_, err = seeker.Seek(fields.BufferStart, io.SeekCurrent)
	} else {
		_, err = io.CopyN(io.Discard, source, fields.BufferStart)
	}
	if err != nil {
		return v, err
	}
	suffix := make([]byte, binarySize-fields.BufferStart)
	if _, err = io.ReadFull(source, suffix); err != nil {
		return v, err
	}
	accessor := func(raw json.RawMessage, kind string, component, width int) ([]byte, error) {
		if len(raw) == 0 {
			return nil, nil
		}
		var index int
		if err := json.Unmarshal(raw, &index); err != nil || index < 0 || index >= len(doc.Accessors) {
			return nil, fmt.Errorf("invalid visualization accessor")
		}
		a := doc.Accessors[index]
		if a.BufferView < 0 || a.BufferView >= len(doc.BufferViews) || a.Count < 0 || a.ComponentType != component || a.Type != kind {
			return nil, fmt.Errorf("invalid visualization accessor format")
		}
		b := doc.BufferViews[a.BufferView]
		start := int64(b.ByteOffset) + int64(a.ByteOffset) - fields.BufferStart
		if b.Buffer != 0 || b.ByteStride != 0 || a.ByteOffset < 0 || a.ByteOffset > b.ByteLength || start < 0 || start > int64(len(suffix)) || a.Count > (b.ByteLength-a.ByteOffset)/width || int64(a.Count) > (int64(len(suffix))-start)/int64(width) {
			return nil, fmt.Errorf("invalid visualization accessor range")
		}
		return suffix[start : start+int64(a.Count)*int64(width)], nil
	}
	for _, fields := range fields.Primitives {
		positions, err := accessor(fields["positions"], "VEC3", 5126, 12)
		if err != nil {
			return v, err
		}
		indices, err := accessor(fields["indices"], "SCALAR", 5125, 4)
		if err != nil {
			return v, err
		}
		delete(fields, "positions")
		delete(fields, "indices")
		raw, err := json.Marshal(fields)
		if err != nil {
			return v, err
		}
		var p VisualPrimitive
		if err = json.Unmarshal(raw, &p); err != nil {
			return v, err
		}
		for i := 0; i < len(positions); i += 12 {
			var point [3]float64
			for j := range point {
				point[j] = float64(math.Float32frombits(binary.LittleEndian.Uint32(positions[i+j*4:])))
			}
			p.Positions = append(p.Positions, point)
		}
		for i := 0; i < len(indices); i += 4 {
			p.Indices = append(p.Indices, binary.LittleEndian.Uint32(indices[i:]))
		}
		v.Primitives = append(v.Primitives, p)
	}
	return v, nil
}

package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	workerv1 "github.com/occccad/occccad/gen/worker/v1"
	artifactstore "github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

// Ownership is exclusive: BodyID for material history, GeometricalSetID for
// independent geometry (and sketches authored in the geometry workbench).
// Result is a derived immutable snapshot; it never enters definition/history.
type FeatureGeometryResult struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	GeometryKey string `json:"geometryKey"`
}
type GeometricInput struct {
	Kind      string                 `json:"kind"` // RESULT | TOPOLOGY | SKETCH | PLANE | DATUM | POINT
	Datum     *DatumReference        `json:"datum,omitempty"`
	Point     *PatternPointReference `json:"point,omitempty"`
	FeatureID string                 `json:"featureId,omitempty"`
	ResultID  string                 `json:"resultId,omitempty"`
	EntityIDs []string               `json:"entityIds,omitempty"` // fixed sketch members
	Selection *FeatureSelection      `json:"selection,omitempty"`
	PlaneID   string                 `json:"planeId,omitempty"`
	Reversed  bool                   `json:"reversed,omitempty"`
}
type GeometryControlPoint struct {
	Reference *GeometricInput `json:"reference,omitempty"`
	ID        string          `json:"id"`
	Position  [3]float64      `json:"position"`
	PointID   string          `json:"pointId,omitempty"` // existing datum point, evaluated in this candidate
}
type GeometryCurveDefinition struct {
	Support        *GeometricInput        `json:"support,omitempty"`
	FramePlaneID   string                 `json:"framePlaneId,omitempty"`
	Points         []GeometryControlPoint `json:"points"`
	Normal         [3]float64             `json:"normal"`
	XDirection     [3]float64             `json:"xDirection"`
	Radius         float64                `json:"radius,omitempty"`
	StartAngle     float64                `json:"startAngle,omitempty"` // degrees
	EndAngle       float64                `json:"endAngle,omitempty"`
	Degree         uint32                 `json:"degree,omitempty"`
	Knots          []float64              `json:"knots,omitempty"`
	Multiplicities []uint32               `json:"multiplicities,omitempty"`
	Weights        []float64              `json:"weights,omitempty"`
	Closed         bool                   `json:"closed,omitempty"`
}
type GeometryOperationParameters struct {
	Direction [3]float64 `json:"direction"`
	Origin    [3]float64 `json:"origin"`
	Distance  float64    `json:"distance"`
	Angle     float64    `json:"angle"` // degrees
	Start     float64    `json:"start"` // normalized arc length
	End       float64    `json:"end"`
	Side      string     `json:"side,omitempty"`
	Tension   float64    `json:"tension"`
}
type GeometrySurfaceDefinition struct {
	Ruled      bool                      `json:"ruled"`
	FrameMode  string                    `json:"frameMode,omitempty"`
	Tolerance  float64                   `json:"tolerance"` // mm, explicit construction/connection policy
	Spine      *GeometricInput           `json:"spine,omitempty"`
	Guide      *GeometricInput           `json:"guide,omitempty"`
	Sections   []GeometrySectionControl  `json:"sections,omitempty"`
	Boundaries []GeometryBoundaryControl `json:"boundaries,omitempty"`
}
type GeometrySectionControl struct {
	ID           string                  `json:"id"`
	Section      GeometricInput          `json:"section"`
	ClosingPoint *GeometricInput         `json:"closingPoint,omitempty"`
	StationPoint *GeometricInput         `json:"stationPoint,omitempty"`
	Couplings    []GeometryCouplingPoint `json:"couplings,omitempty"`
}
type GeometryCouplingPoint struct {
	ID    string          `json:"id"`
	Point *GeometricInput `json:"point"`
}
type GeometryBoundaryControl struct {
	ID         string          `json:"id"`
	Boundary   GeometricInput  `json:"boundary"`
	Support    *GeometricInput `json:"support,omitempty"`
	Continuity string          `json:"continuity"`
	Tension    float64         `json:"tension"`
}
type GeometricFeatureDefinition struct {
	Surface     *GeometrySurfaceDefinition  `json:"surface,omitempty"`
	KeepPoint   *GeometricInput             `json:"keepPoint,omitempty"`
	Direction   *GeometricInput             `json:"direction,omitempty"`
	Axis        *GeometricInput             `json:"axis,omitempty"`
	MirrorPlane *GeometricInput             `json:"mirrorPlane,omitempty"`
	Kind        string                      `json:"kind"`
	Inputs      []GeometricInput            `json:"inputs,omitempty"`
	Curve       *GeometryCurveDefinition    `json:"curve,omitempty"`
	Parameters  GeometryOperationParameters `json:"parameters"`
}

var geometryKinds = map[string]int{
	"IMPORTED": 0, "LINE": 0, "CIRCLE": 0, "ARC": 0, "SPLINE": 0,
	"POINT_ON_CURVE": 1, "EXTRACT": 1, "BOUNDARY": 1, "COMPOSITE": -1,
	"SURFACE_EXTRUDE": 1, "THICKEN": 1,
	"TRANSLATE": 1, "ROTATE": 1, "MIRROR": 1,
	"SPLIT": 1, "TRIM": 1, "EXTEND": 1, "G1_CONNECT": 2,
	"OFFSET": 2, "PROJECT": 2, "INTERSECTION": 2,
	"SURFACE_SWEEP": 2, "SURFACE_LOFT": -1, "SURFACE_FILL": -1,
	"SURFACE_TRIM": 2, "SURFACE_EXTEND": 2, "SURFACE_JOIN": -1,
	"SURFACE_SEW": -1, "SURFACE_OFFSET": 1, "CLOSE_SURFACE": 1,
	"SURFACE_BOUNDARY": 4, "SURFACE_BLEND": 2,
}

func isEngineeringSurface(kind string) bool {
	return strings.HasPrefix(kind, "SURFACE_") && kind != "SURFACE_EXTRUDE" || kind == "CLOSE_SURFACE"
}
func isGeometryBodyFeature(kind string) bool { return kind == "THICKEN" || kind == "CLOSE_SURFACE" }

func geometryResultReference(f Feature) *modelcore.GeometryResultReference {
	if f.Result == nil {
		return nil
	}
	return &modelcore.GeometryResultReference{FeatureID: f.ID, ResultID: f.Result.ID}
}
func geometryInputFeatureID(i GeometricInput) string {
	if i.Datum != nil {
		return i.Datum.FeatureID
	}
	if i.Point != nil {
		return i.Point.SketchID
	}
	if i.Selection != nil {
		if i.Selection.Selection.SourceResult != nil {
			return i.Selection.Selection.SourceResult.FeatureID
		}
		return i.Selection.SourceFeatureID
	}
	return i.FeatureID
}
func validateGeometricFeature(f Feature, earlier map[string]Feature, model PartModel) error {
	if f.Geometry == nil {
		if f.Type == "GEOMETRY" || isGeometryBodyFeature(f.Type) {
			return fmt.Errorf("%w: GEOMETRY_DEFINITION_REQUIRED", ErrValidation)
		}
		return nil
	}
	d := f.Geometry
	if f.ImportedAppearance != nil && d.Kind != "IMPORTED" {
		return fmt.Errorf("%w: IMPORT_APPEARANCE_REQUIRES_IMPORTED_RESULT", ErrValidation)
	}
	if d.Kind == "IMPORTED" && (f.Type != "GEOMETRY" || f.ImportDefinitionID == "" || f.GeometryKey == "" || len(d.Inputs) != 0 || d.Curve != nil || d.Surface != nil) {
		return fmt.Errorf("%w: IMPORT_DEFINITION_REQUIRED", ErrValidation)
	}
	if isEngineeringSurface(d.Kind) {
		if d.Surface == nil || (math.IsNaN(d.Surface.Tolerance) || math.IsInf(d.Surface.Tolerance, 0)) || d.Surface.Tolerance < 1e-7 || d.Surface.Tolerance > 1e-2 {
			return fmt.Errorf("%w: SURFACE_TOLERANCE_OUT_OF_RANGE", ErrValidation)
		}
		if d.Kind == "SURFACE_LOFT" && (len(d.Inputs) < 2 || len(d.Inputs) > 32) {
			return fmt.Errorf("%w: SURFACE_LOFT_SECTION_COUNT_INVALID", ErrValidation)
		}
		if d.Kind == "SURFACE_SWEEP" && !slices.Contains([]string{"CORRECTED_FRENET", "FRENET", "GUIDE_FRAME", "GUIDE_BORDER"}, d.Surface.FrameMode) {
			return fmt.Errorf("%w: SURFACE_SWEEP_FRAME_REQUIRED", ErrValidation)
		}
		if d.Kind == "SURFACE_TRIM" && d.KeepPoint == nil {
			return fmt.Errorf("%w: SURFACE_TRIM_KEEP_POINT_REQUIRED", ErrValidation)
		}
		if err := validateSurfaceControls(d); err != nil {
			return err
		}
	} else if d.Surface != nil {
		return fmt.Errorf("%w: SURFACE_DEFINITION_UNEXPECTED", ErrValidation)
	}
	if d.KeepPoint != nil && d.Kind != "SURFACE_TRIM" {
		return fmt.Errorf("%w: GEOMETRY_REFERENCE_ROLE_INVALID", ErrValidation)
	}
	if d.Curve != nil && d.Curve.Support != nil && !slices.Contains([]string{"CIRCLE", "ARC"}, d.Kind) {
		return fmt.Errorf("%w: GEOMETRY_REFERENCE_ROLE_INVALID", ErrValidation)
	}
	if d.Direction != nil && !slices.Contains([]string{"SURFACE_EXTRUDE", "TRANSLATE", "PROJECT"}, d.Kind) || d.Axis != nil && d.Kind != "ROTATE" || d.MirrorPlane != nil && d.Kind != "MIRROR" {
		return fmt.Errorf("%w: GEOMETRY_REFERENCE_ROLE_INVALID", ErrValidation)
	}
	for _, reference := range geometryReferences(d) {
		if err := validateGeometryReference(reference, earlier, model); err != nil {
			return err
		}
	}
	count, ok := geometryKinds[d.Kind]
	if !ok || (f.Type != "GEOMETRY" && !isGeometryBodyFeature(f.Type)) || isGeometryBodyFeature(f.Type) != isGeometryBodyFeature(d.Kind) || isGeometryBodyFeature(f.Type) && f.Type != d.Kind {
		return fmt.Errorf("%w: GEOMETRY_KIND_INVALID", ErrValidation)
	}
	if count >= 0 && len(d.Inputs) != count || count < 0 && (len(d.Inputs) == 0 || len(d.Inputs) > 128) {
		return fmt.Errorf("%w: GEOMETRY_INPUT_COUNT_INVALID", ErrValidation)
	}
	if isGeometryBodyFeature(f.Type) && !slices.Contains([]string{"NEW_BODY", "ADD", "REMOVE", "INTERSECT"}, f.Operation) {
		return fmt.Errorf("%w: THICKEN_OPERATION_INVALID", ErrValidation)
	}
	for _, input := range d.Inputs {
		if input.Datum != nil || input.Point != nil {
			return fmt.Errorf("%w: GEOMETRY_INPUT_KIND_INVALID", ErrValidation)
		}
		switch input.Kind {
		case "RESULT":
			source, exists := earlier[input.FeatureID]
			if !exists || source.Type != "GEOMETRY" || input.ResultID != "result" || input.Selection != nil || input.PlaneID != "" || len(input.EntityIDs) != 0 {
				return fmt.Errorf("%w: GEOMETRY_RESULT_INPUT_INVALID", ErrValidation)
			}
		case "SKETCH":
			source, exists := earlier[input.FeatureID]
			if !exists || source.Sketch == nil || len(input.EntityIDs) == 0 || len(input.EntityIDs) > 4096 || input.ResultID != "" || input.Selection != nil || input.PlaneID != "" {
				return fmt.Errorf("%w: GEOMETRY_SKETCH_INPUT_INVALID", ErrValidation)
			}
			seen := map[string]bool{}
			for _, id := range input.EntityIDs {
				if seen[id] || !slices.ContainsFunc(source.Sketch.Entities, func(e SketchEntity) bool { return e.ID == id && e.Role != "CONSTRUCTION" && e.Kind != "POINT" }) {
					return fmt.Errorf("%w: GEOMETRY_SKETCH_MEMBER_MISSING", ErrValidation)
				}
				seen[id] = true
			}
		case "TOPOLOGY":
			if input.Selection == nil || input.Selection.SourceVersionID == "" || input.FeatureID != "" || input.ResultID != "" || input.PlaneID != "" || len(input.EntityIDs) != 0 {
				return fmt.Errorf("%w: GEOMETRY_TOPOLOGY_INPUT_INVALID", ErrValidation)
			}
			if err := input.Selection.Selection.Validate(); err != nil {
				return fmt.Errorf("%w: %v", ErrValidation, err)
			}
			source, exists := earlier[geometryInputFeatureID(input)]
			if !exists || (!isBodyFeature(source.Type) && source.Type != "GEOMETRY") {
				return fmt.Errorf("%w: GEOMETRY_INPUT_MUST_PRECEDE_FEATURE", ErrValidation)
			}
			if r := input.Selection.Selection.SourceResult; r != nil {
				if source.Type != "GEOMETRY" || r.ResultID != "result" || input.Selection.SourceFeatureID != r.FeatureID {
					return fmt.Errorf("%w: GEOMETRY_RESULT_OWNER_INVALID", ErrValidation)
				}
			} else if source.BodyID != input.Selection.Selection.SourceBodyID {
				return fmt.Errorf("%w: GEOMETRY_BODY_OWNER_INVALID", ErrValidation)
			}
		case "PLANE":
			if input.FeatureID != "" || input.ResultID != "" || input.Selection != nil || len(input.EntityIDs) != 0 || !slices.ContainsFunc(model.DatumPlanes, func(p DatumPlane) bool { return p.ID == input.PlaneID }) {
				return fmt.Errorf("%w: GEOMETRY_PLANE_INPUT_INVALID", ErrValidation)
			}
		default:
			return fmt.Errorf("%w: GEOMETRY_INPUT_KIND_INVALID", ErrValidation)
		}
	}
	if count == 0 && d.Kind != "IMPORTED" {
		if d.Curve == nil || len(d.Curve.Points) == 0 || len(d.Curve.Points) > 4096 {
			return fmt.Errorf("%w: GEOMETRY_CURVE_DEFINITION_REQUIRED", ErrValidation)
		}
		if d.Curve.Support != nil && d.Curve.FramePlaneID != "" {
			return fmt.Errorf("%w: CURVE_FRAME_CONFLICT", ErrValidation)
		}
		if d.Curve.FramePlaneID != "" && !slices.ContainsFunc(model.DatumPlanes, func(p DatumPlane) bool { return p.ID == d.Curve.FramePlaneID }) {
			return fmt.Errorf("%w: CURVE_FRAME_MISSING", ErrValidation)
		}
		seen := map[string]bool{}
		for _, p := range d.Curve.Points {
			if p.Reference != nil {
				if p.PointID != "" {
					return fmt.Errorf("%w: CURVE_POINT_REFERENCE_INVALID", ErrValidation)
				}
				if err := validateGeometryReference(*p.Reference, earlier, model); err != nil {
					return err
				}
				if p.Reference.Selection != nil && p.Reference.Selection.Selection.ExpectedType != modelcore.PersistentTopologyVertex {
					return fmt.Errorf("%w: CURVE_POINT_REQUIRES_VERTEX", ErrValidation)
				}
			}
			if p.ID == "" || seen[p.ID] {
				return fmt.Errorf("%w: CURVE_CONTROL_IDENTITY_INVALID", ErrValidation)
			}
			seen[p.ID] = true
			if p.PointID != "" && !slices.ContainsFunc(model.DatumPoints, func(d DatumPoint) bool { return d.ID == p.PointID }) {
				return fmt.Errorf("%w: CURVE_POINT_REFERENCE_MISSING", ErrValidation)
			}
		}
	} else if d.Curve != nil {
		return fmt.Errorf("%w: GEOMETRY_CURVE_DEFINITION_UNEXPECTED", ErrValidation)
	}
	return nil
}

type geometricParameter struct {
	slot, label, unit string
	dimension         modelcore.Dimension
	value             *float64
}

func geometryParameters(d *GeometricFeatureDefinition) []geometricParameter {
	if d == nil {
		return nil
	}
	var out []geometricParameter
	add := func(slot, label, unit string, value *float64) {
		dimension := modelcore.LengthDimension
		if unit == "1" {
			dimension = modelcore.Dimensionless
		} else if unit == "deg" {
			dimension = modelcore.AngleDimension
		}
		out = append(out, geometricParameter{"geometry:" + slot, label, unit, dimension, value})
	}
	vector := func(slot string, values *[3]float64, unit string, labels ...string) {
		label := slot
		if len(labels) > 0 {
			label = labels[0]
		}
		for i, axis := range []string{"x", "y", "z"} {
			add(slot+":"+axis, label+" "+strings.ToUpper(axis), unit, &values[i])
		}
	}
	if d.Surface != nil {
		add("tolerance", "Tolerance", "mm", &d.Surface.Tolerance)
		for i := range d.Surface.Boundaries {
			c := &d.Surface.Boundaries[i]
			if c.Continuity != "G0" {
				add("tension:"+c.ID, "Tension", "1", &c.Tension)
			}
		}
	}
	if c := d.Curve; c != nil {
		for i := range c.Points {
			p := &c.Points[i]
			if p.PointID == "" && p.Reference == nil {
				label := fmt.Sprintf("Point %d", i+1)
				if d.Kind == "LINE" {
					label = "Start"
					if i > 0 {
						label = "End"
					}
				} else if d.Kind == "CIRCLE" || d.Kind == "ARC" {
					label = "Center"
				}
				vector("point:"+p.ID, &p.Position, "mm", label)
			}
		}
		if d.Kind == "SPLINE" && len(c.Knots) > 0 && len(c.Weights) == len(c.Points) {
			for i := range c.Points {
				add("weight:"+c.Points[i].ID, "Weight", "1", &c.Weights[i])
			}
		}

		if d.Kind == "CIRCLE" || d.Kind == "ARC" {
			add("radius", "Radius", "mm", &c.Radius)
			if c.FramePlaneID == "" && c.Support == nil {
				vector("normal", &c.Normal, "1")
				vector("xDirection", &c.XDirection, "1")
			}
		}
		if d.Kind == "ARC" {
			add("startAngle", "Start angle", "deg", &c.StartAngle)
			add("endAngle", "End angle", "deg", &c.EndAngle)
		}
	}
	switch d.Kind {
	case "SURFACE_EXTRUDE", "TRANSLATE", "ROTATE", "MIRROR", "PROJECT":
		if d.Direction == nil && d.Axis == nil && d.MirrorPlane == nil {
			vector("direction", &d.Parameters.Direction, "1")
		}
	}
	switch d.Kind {
	case "SURFACE_EXTRUDE", "THICKEN", "TRANSLATE", "OFFSET", "EXTEND", "SURFACE_EXTEND", "SURFACE_OFFSET":
		add("distance", "Distance", "mm", &d.Parameters.Distance)
	}
	if (d.Kind == "ROTATE" || d.Kind == "MIRROR") && d.Axis == nil && d.MirrorPlane == nil {
		vector("origin", &d.Parameters.Origin, "mm")
	}
	if d.Kind == "ROTATE" {
		add("angle", "Angle", "deg", &d.Parameters.Angle)
	}
	if d.Kind == "POINT_ON_CURVE" || d.Kind == "SPLIT" || d.Kind == "TRIM" {
		add("start", "Position", "1", &d.Parameters.Start)
	}
	if d.Kind == "TRIM" {
		add("end", "End", "1", &d.Parameters.End)
	}
	if d.Kind == "G1_CONNECT" {
		add("tension", "Tangent scale", "1", &d.Parameters.Tension)
	}
	return out
}
func resolveGeometryParameters(f *Feature, values map[string]modelcore.Quantity) error {
	for _, p := range geometryParameters(f.Geometry) {
		q, ok := values["parameter:"+f.ID+":"+p.slot]
		if !ok {
			return fmt.Errorf("%w: GEOMETRY_PARAMETER_MISSING", ErrValidation)
		}
		v, err := quantityInDisplayUnit(q, p.unit)
		if err != nil {
			return err
		}
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%w: GEOMETRY_NONFINITE_PARAMETER", ErrValidation)
		}
		*p.value = v
	}
	return nil
}
func geometryParameterValue(d *GeometricFeatureDefinition, slot string) (float64, bool) {
	for _, p := range geometryParameters(d) {
		if p.slot == slot {
			return *p.value, true
		}
	}
	return 0, false
}

func geometryDomain(e *workerv1.EvaluatePartResponse) string {
	if domain := e.GetTopology().GetGeometryDomain(); domain != "" {
		return domain
	}
	if e.GetTopology().GetSolidCount() > 0 {
		return "SOLID_SET"
	}
	if e.GetTopology().GetFaceCount() > 0 {
		return "SHEET_SET"
	}
	if e.GetTopology().GetEdgeCount() > 0 {
		return "WIRE_SET"
	}
	return "POINT_SET"
}
func geometryVec(v [3]float64) *workerv1.Vec3 { return &workerv1.Vec3{X: v[0], Y: v[1], Z: v[2]} }
func (s *Service) evaluateGeometrySpec(ctx context.Context, requestID string, spec *workerv1.GeometryFeatureSpec, dependencies ...any) (string, error) {
	frozen, _ := json.Marshal(struct {
		Spec              *workerv1.GeometryFeatureSpec
		Policy, Evaluator string
		Exact             bool
		Dependencies      []any
	}{spec, modelcore.TopologyNamingPolicyDigest, evaluatorVersion, geometry.PartRuntime(ctx).ExactOnly, dependencies})
	key := modelcore.ValueDigest(frozen)
	var exists bool
	if err := s.database.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM occccad.geometry_artifacts WHERE geometry_key=$1)`, key).Scan(&exists); err != nil {
		return "", err
	}
	if exists && !geometry.PartRuntime(ctx).ForceCold {
		return key, nil
	}
	if s.artifacts == nil {
		return "", fmt.Errorf("persistent geometry evaluation requires ArtifactStore")
	}
	attempt := requestID + "/" + newID("artifact-attempt")
	evaluation, err := s.worker.EvaluateGeometryFeature(ctx, requestID, key, spec, artifactstore.StagingKey(attempt, "shape.brep"), artifactstore.StagingKey(attempt, "mesh.glb"))
	if err != nil {
		return "", err
	}
	recordWorkerEvaluation(ctx, evaluation)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := s.storeGeometryEvaluation(ctx, key, evaluation); err != nil {
		return "", err
	}
	return key, nil
}
func (s *Service) geometrySource(ctx context.Context, requestID string, model PartModel, input GeometricInput) (*workerv1.GeometryFeatureInput, error) {
	result := &workerv1.GeometryFeatureInput{Reversed: input.Reversed}
	if input.Kind == "PLANE" {
		for _, p := range model.DatumPlanes {
			if p.ID == input.PlaneID {
				result.Plane = &workerv1.GeometryPlane{Id: p.ID, Origin: geometryVec(p.Origin), Normal: geometryVec(p.Normal), UDirection: geometryVec(p.UDirection)}
				return result, nil
			}
		}
		return nil, fmt.Errorf("%w: GEOMETRY_PLANE_INPUT_MISSING", ErrValidation)
	}
	sourceID := geometryInputFeatureID(input)
	index := slices.IndexFunc(model.Features, func(f Feature) bool { return f.ID == sourceID })
	if index < 0 || model.Features[index].Suppressed {
		return nil, fmt.Errorf("%w: GEOMETRY_SOURCE_UNAVAILABLE", ErrValidation)
	}
	source := model.Features[index]
	var key string
	var err error
	sourceResult := &modelcore.GeometryResultReference{FeatureID: source.ID, ResultID: "result"}
	switch input.Kind {
	case "SKETCH":
		chains, e := sketchCurveChains(source, input.EntityIDs)
		if e != nil {
			return nil, e
		}
		origin, u, normal, ok := supportFrame(model, source.Sketch.Support)
		if !ok {
			return nil, fmt.Errorf("%w: GEOMETRY_SKETCH_SUPPORT_UNAVAILABLE", ErrValidation)
		}
		spec := &workerv1.GeometryFeatureSpec{FeatureId: source.ID, ResultId: "wire", Operation: "EXTRACT", SketchWire: &workerv1.GeometrySketchWire{SketchId: source.ID, Origin: geometryVec(origin), Normal: geometryVec(normal), UDirection: geometryVec(u)}}
		for _, chain := range chains {
			for _, c := range chain.Curves {
				spec.SketchWire.Curves = append(spec.SketchWire.Curves, geometry.ProfileCurveProto(c))
			}
		}
		key, err = s.evaluateGeometrySpec(ctx, requestID+"/sketch-wire", spec)
		sourceResult.ResultID = "wire"
	case "RESULT", "TOPOLOGY":
		if source.Type == "GEOMETRY" {
			key, err = s.evaluateGeometricFeature(ctx, requestID+"/source", &model, source.ID)
		} else {
			key, err = s.featureStageInModel(ctx, model, "", source.ID, source.BodyID)
		}
	default:
		return nil, fmt.Errorf("%w: GEOMETRY_INPUT_KIND_INVALID", ErrValidation)
	}
	if err != nil {
		return nil, err
	}
	brep, err := s.loadBrepArtifactReference(ctx, key)
	if err != nil {
		return nil, err
	}
	naming, err := s.geometryRepresentationReference(ctx, key, "NAMING")
	if err != nil {
		return nil, err
	}
	result.Source = &workerv1.BodyToolInput{Brep: geometry.ArtifactReferenceProto(brep), Naming: geometry.ArtifactReferenceProto(naming), FeatureId: source.ID}
	if source.Type == "GEOMETRY" || input.Kind == "SKETCH" {
		result.Source.Result = &workerv1.GeometryResultReference{FeatureId: sourceResult.FeatureID, ResultId: sourceResult.ResultID}
	} else {
		result.Source.BodyId = source.BodyID
	}
	if input.Kind == "TOPOLOGY" {
		pick := *input.Selection
		resolution, e := s.resolveFeaturePick(ctx, pick, key)
		if e != nil {
			return nil, e
		}
		if resolution.Status != modelcore.SelectionResolved || len(resolution.Candidates) != 1 {
			return nil, fmt.Errorf("%w: GEOMETRY_INPUT_%s: %s", ErrValidation, resolution.Status, resolution.DiagnosticCode)
		}
		a := resolution.Candidates[0].SemanticRef
		result.Members = []*workerv1.SemanticTopologyRef{{FeatureId: a.FeatureID, OutputSlot: a.OutputSlot, SourceIds: a.SourceIDs}}
	}
	return result, nil
}
func (s *Service) geometryFeatureSpec(ctx context.Context, requestID string, model PartModel, f Feature) (*compiledGeometryFeature, error) {
	d := f.Geometry
	if d == nil {
		return nil, fmt.Errorf("%w: GEOMETRY_DEFINITION_REQUIRED", ErrValidation)
	}
	spec := &compiledGeometryFeature{GeometryFeatureSpec: &workerv1.GeometryFeatureSpec{FeatureId: f.ID, ResultId: "result", Operation: d.Kind, Parameters: &workerv1.GeometryOperationDefinition{Direction: geometryVec(d.Parameters.Direction), Origin: geometryVec(d.Parameters.Origin), Distance: d.Parameters.Distance, Angle: d.Parameters.Angle * math.Pi / 180, Start: d.Parameters.Start, End: d.Parameters.End, Side: d.Parameters.Side, Tension: d.Parameters.Tension}}}
	if d.Kind == "IMPORTED" {
		seed, err := s.importSeed(ctx, f)
		if err != nil {
			return nil, err
		}
		ref, err := s.brepArtifactReference(ctx, f.GeometryKey)
		if err != nil {
			return nil, err
		}
		spec.ImportSeed = seed
		spec.ImportBrep = &workerv1.ArtifactReference{Backend: ref.Backend, ObjectKey: ref.ObjectKey, Sha256: ref.SHA256, SizeBytes: uint64(ref.Size), ContentType: ref.ContentType}
		return spec, nil
	}
	resolve := func(input GeometricInput) (datumFrame, error) {
		frame, snapshot, err := s.geometryReferenceFrame(ctx, requestID, model, input)
		if err == nil {
			spec.References = append(spec.References, snapshot)
		}
		return frame, err
	}
	if d.Surface != nil {
		spec.Surface = &workerv1.GeometrySurfaceDefinition{Ruled: d.Surface.Ruled, FrameMode: d.Surface.FrameMode, Tolerance: d.Surface.Tolerance}
		if d.KeepPoint != nil {
			frame, err := resolve(*d.KeepPoint)
			if err != nil {
				return nil, err
			}
			if !frame.point {
				return nil, fmt.Errorf("%w: SURFACE_TRIM_KEEP_POINT_REQUIRED", ErrValidation)
			}
			spec.Surface.KeepPoint = geometryVec(frame.origin)
		}
	}
	if err := s.compileSurfaceControls(ctx, requestID, model, d, spec, resolve); err != nil {
		return nil, err
	}

	for _, reference := range []struct {
		role  string
		input *GeometricInput
	}{{"direction", d.Direction}, {"axis", d.Axis}, {"mirror", d.MirrorPlane}} {
		role, input := reference.role, reference.input
		if input == nil {
			continue
		}
		frame, err := resolve(*input)
		if err != nil {
			return nil, err
		}
		if frame.point || role == "axis" && frame.plane || role == "mirror" && !frame.plane {
			return nil, fmt.Errorf("%w: GEOMETRY_REFERENCE_TYPE_MISMATCH", ErrValidation)
		}
		spec.Parameters.Direction = geometryVec(frame.direction)
		if role != "direction" {
			spec.Parameters.Origin = geometryVec(frame.origin)
		}
	}
	if c := d.Curve; c != nil {
		spec.Curve = &workerv1.GeometryCurveDefinition{Normal: geometryVec(c.Normal), XDirection: geometryVec(c.XDirection), Radius: c.Radius, StartAngle: c.StartAngle * math.Pi / 180, EndAngle: c.EndAngle * math.Pi / 180, Degree: c.Degree, Knots: c.Knots, Multiplicities: c.Multiplicities, Weights: c.Weights, Closed: c.Closed}
		var frame *DatumPlane
		if c.Support != nil {
			resolved, err := resolve(*c.Support)
			if err != nil {
				return nil, err
			}
			if !resolved.plane {
				return nil, fmt.Errorf("%w: CURVE_FRAME_REQUIRES_PLANE", ErrValidation)
			}
			frame = &DatumPlane{ID: geometryReferenceSource(*c.Support), Origin: resolved.origin, Normal: resolved.direction, UDirection: resolved.u}
			spec.Curve.Normal = geometryVec(frame.Normal)
			spec.Curve.XDirection = geometryVec(frame.UDirection)
		}
		if c.FramePlaneID != "" {
			for i := range model.DatumPlanes {
				if model.DatumPlanes[i].ID == c.FramePlaneID {
					frame = &model.DatumPlanes[i]
					break
				}
			}
			if frame == nil {
				return nil, fmt.Errorf("%w: CURVE_FRAME_MISSING", ErrValidation)
			}
			spec.Curve.Normal = geometryVec(frame.Normal)
			spec.Curve.XDirection = geometryVec(frame.UDirection)
		}
		for _, p := range c.Points {
			position := p.Position
			var sources []*workerv1.SemanticTopologyRef
			if frame != nil && p.PointID == "" && p.Reference == nil {
				v := geometryCross(frame.Normal, frame.UDirection)
				for j := range 3 {
					position[j] = frame.Origin[j] + frame.UDirection[j]*p.Position[0] + v[j]*p.Position[1] + frame.Normal[j]*p.Position[2]
				}
				sources = append(sources, &workerv1.SemanticTopologyRef{FeatureId: frame.ID, OutputSlot: "datum.plane", SourceIds: []string{frame.ID}})
			}
			if p.Reference != nil {
				var e error
				position, sources, e = s.geometryPointPosition(ctx, requestID, model, *p.Reference, &spec.References)
				if e != nil {
					return nil, e
				}
			}
			if p.PointID != "" {
				i := slices.IndexFunc(model.DatumPoints, func(d DatumPoint) bool { return d.ID == p.PointID })
				if i < 0 {
					return nil, fmt.Errorf("%w: CURVE_POINT_REFERENCE_MISSING", ErrValidation)
				}
				position = model.DatumPoints[i].Position
				sources = []*workerv1.SemanticTopologyRef{{FeatureId: p.PointID, OutputSlot: "datum.point", SourceIds: []string{p.PointID}}}
			}
			if c.Support != nil && (d.Kind == "CIRCLE" || d.Kind == "ARC") {
				delta := 0.0
				for j := range 3 {
					delta += (position[j] - frame.Origin[j]) * frame.Normal[j]
				}
				if math.Abs(delta) > 1e-6 {
					return nil, fmt.Errorf("%w: CURVE_POINT_NOT_ON_SUPPORT", ErrValidation)
				}
			}
			spec.Curve.Points = append(spec.Curve.Points, &workerv1.GeometryControlPoint{Id: p.ID, Position: geometryVec(position), Sources: sources})
		}
	}
	for index, input := range d.Inputs {
		if d.Kind == "OFFSET" && index == 1 && input.Kind != "PLANE" {
			frame, err := resolve(input)
			if err != nil {
				return nil, err
			}
			if !frame.plane {
				return nil, fmt.Errorf("%w: CURVE_FRAME_REQUIRES_PLANE", ErrValidation)
			}
			spec.Inputs = append(spec.Inputs, &workerv1.GeometryFeatureInput{Plane: &workerv1.GeometryPlane{Id: geometryReferenceSource(input), Origin: geometryVec(frame.origin), Normal: geometryVec(frame.direction), UDirection: geometryVec(frame.u)}})
			continue
		}
		resolved, e := s.geometrySource(ctx, requestID+"/"+f.ID, model, input)
		if e != nil {
			return nil, e
		}
		spec.Inputs = append(spec.Inputs, resolved)
	}
	return spec, nil
}
func (s *Service) evaluateGeometricFeature(ctx context.Context, requestID string, model *PartModel, id string) (string, error) {
	index := slices.IndexFunc(model.Features, func(f Feature) bool { return f.ID == id })
	if index < 0 {
		return "", fmt.Errorf("%w: GEOMETRY_SOURCE_UNAVAILABLE", ErrValidation)
	}
	f := model.Features[index]
	if f.Type != "GEOMETRY" || f.Geometry == nil || f.Suppressed {
		return "", fmt.Errorf("%w: GEOMETRY_SOURCE_UNAVAILABLE", ErrValidation)
	}
	// Structural validation enforces upstream inputs; recursion only visits a
	// frozen, strictly shorter prefix and cannot follow a mutable document Head.
	prefix := *model
	prefix.Features = append([]Feature(nil), model.Features[:index]...)
	spec, err := s.geometryFeatureSpec(ctx, requestID, prefix, f)
	if err != nil {
		return "", fmt.Errorf("FEATURE_FAILED[%s]: %w", id, err)
	}
	key, err := s.evaluateGeometrySpec(ctx, requestID, spec.GeometryFeatureSpec, spec.References, f.Geometry)
	if err != nil {
		return "", fmt.Errorf("FEATURE_FAILED[%s]: %w", id, err)
	}
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return "", err
	}
	domain := a.GeometryDomain
	model.Features[index].Result = &FeatureGeometryResult{ID: "result", Domain: domain, GeometryKey: key}
	model.Features[index].EvaluationStatus = ""
	model.Features[index].Diagnostic = ""
	return key, nil
}

func (s *Service) resultManifestForVersion(ctx context.Context, documentID, versionID string, result *modelcore.GeometryResultReference) (*topologyManifest, string, string, error) {
	if result == nil || result.FeatureID == "" || result.ResultID != "result" {
		return nil, "", "", fmt.Errorf("%w: GEOMETRY_RESULT_REFERENCE_INVALID", ErrValidation)
	}
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, documentID, versionID).Scan(&raw); err != nil {
		return nil, "", "", err
	}
	var m PartModel
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, "", "", err
	}
	i := slices.IndexFunc(m.Features, func(f Feature) bool { return f.ID == result.FeatureID })
	if i < 0 || m.Features[i].Suppressed || m.Features[i].Result == nil || m.Features[i].Result.ID != result.ResultID || m.Features[i].Result.GeometryKey == "" {
		return nil, "", "", ErrNotFound
	}
	key := m.Features[i].Result.GeometryKey
	manifest, digest, err := s.topologyManifestForGeometryKey(ctx, key)
	if err == nil && (manifest == nil || manifest.Results[*result] == nil) {
		err = fmt.Errorf("%w: NAMING_RESULT_OWNER_MISMATCH", ErrValidation)
	}
	return manifest, key, digest, err
}
func (s *Service) selectionManifestForVersion(ctx context.Context, documentID, versionID string, selection modelcore.PersistentSelection) (*topologyManifest, string, string, error) {
	if selection.SourceResult != nil {
		return s.resultManifestForVersion(ctx, documentID, versionID, selection.SourceResult)
	}
	return s.topologyManifestForVersion(ctx, documentID, versionID, selection.SourceBodyID)
}

func (s *Service) inferSelectionOwner(ctx context.Context, documentID string, request *BindPersistentSelectionRequest) error {
	if request.BodyID != "" || request.Result != nil {
		return nil
	}
	var raw []byte
	if err := s.database.QueryRow(ctx, `SELECT model_json FROM occccad.document_versions WHERE document_id=$1 AND id=$2`, documentID, request.SourceVersionID).Scan(&raw); err != nil {
		return err
	}
	var m PartModel
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	count := 0
	for _, b := range m.Bodies {
		if b.GeometryKey == request.GeometryKey {
			request.BodyID = b.ID
			count++
		}
	}
	for _, f := range m.Features {
		if f.Result != nil && f.Result.GeometryKey == request.GeometryKey {
			request.Result = geometryResultReference(f)
			count++
		}
	}
	if count != 1 {
		return ErrNotFound
	}
	return nil
}

func qualifyGeometryInputs(documentID string, f Feature) error {
	var inputs []GeometricInput
	if f.Sweep != nil && f.Sweep.Path.Geometry != nil {
		inputs = append(inputs, *f.Sweep.Path.Geometry)
	}
	if f.Geometry == nil {
		for _, i := range inputs {
			if i.Selection != nil && i.Selection.Selection.SourceDocumentID != documentID {
				return fmt.Errorf("%w: cross-document geometry requires an accepted context input", ErrValidation)
			}
		}
		return nil
	}
	inputs = append(inputs, f.Geometry.Inputs...)
	inputs = append(inputs, geometryReferences(f.Geometry)...)
	if c := f.Geometry.Curve; c != nil {
		for _, p := range c.Points {
			if p.Reference != nil {
				inputs = append(inputs, *p.Reference)
			}
		}
	}
	for _, i := range inputs {
		if i.Datum != nil && i.Datum.Selection != nil && i.Datum.Selection.SourceDocumentID != documentID {
			return fmt.Errorf("%w: cross-document geometry requires an accepted context input", ErrValidation)
		}
		if i.Selection != nil && i.Selection.Selection.SourceDocumentID != documentID {
			return fmt.Errorf("%w: cross-document geometry requires an accepted context input", ErrValidation)
		}
	}
	return nil
}
func geometryKindLabel(kind string) string {
	labels := map[string]string{"LINE": "直线", "CIRCLE": "圆", "ARC": "圆弧", "SPLINE": "样条", "POINT_ON_CURVE": "曲线上点", "EXTRACT": "提取", "BOUNDARY": "边界", "COMPOSITE": "复合曲线", "SURFACE_EXTRUDE": "拉伸曲面", "THICKEN": "加厚", "TRANSLATE": "平移", "ROTATE": "旋转", "MIRROR": "镜像", "SPLIT": "分割", "TRIM": "裁剪", "EXTEND": "延伸", "G1_CONNECT": "连接曲线", "OFFSET": "偏移曲线", "PROJECT": "投影", "INTERSECTION": "交线", "SURFACE_SWEEP": "曲面扫掠", "SURFACE_LOFT": "曲面放样", "SURFACE_FILL": "填充曲面", "SURFACE_TRIM": "裁剪曲面", "SURFACE_EXTEND": "延长曲面", "SURFACE_JOIN": "连接曲面", "SURFACE_SEW": "缝合曲面", "SURFACE_OFFSET": "偏移曲面", "CLOSE_SURFACE": "封闭曲面", "SURFACE_BOUNDARY": "边界曲面", "SURFACE_BLEND": "桥接曲面"}
	if label := labels[kind]; label != "" {
		return label
	}
	return "几何特征"
}

func previewGeometryFeatureID(payload json.RawMessage, model PartModel) string {
	var p struct {
		Feature   Feature `json:"feature"`
		FeatureID string  `json:"featureId"`
	}
	_ = json.Unmarshal(payload, &p)
	for _, f := range model.Features {
		if f.Type == "GEOMETRY" && (f.ID == p.Feature.ID || f.ID == p.FeatureID) {
			return f.ID
		}
	}
	return ""
}
func (s *Service) geometricFeatureInput(ctx context.Context, documentID string, request FeatureInputRequest, model PartModel, f Feature) (FeatureInput, error) {
	if request.BodyID != "" {
		return FeatureInput{}, fmt.Errorf("%w: GEOMETRY_RESULT_HAS_NO_BODY", ErrValidation)
	}
	key, err := s.featureStageInModel(ctx, model, request.VersionID, f.ID, "", true)
	if err != nil {
		return FeatureInput{}, err
	}
	a, err := s.loadArtifact(ctx, key)
	if err != nil {
		return FeatureInput{}, err
	}
	a.Result = &modelcore.GeometryResultReference{FeatureID: f.ID, ResultID: "result"}
	a.DisplayStageFeatureID = f.ID
	result := FeatureInput{VersionID: request.VersionID, SourceFeatureID: f.ID, Artifact: a, Picks: []FeatureInputPick{}}
	if request.Kind != "" || request.LocalID != 0 {
		if request.ResultStage || request.GeometryKey != key {
			return FeatureInput{}, fmt.Errorf("%w: GEOMETRY_PICK_SNAPSHOT_MISMATCH", ErrValidation)
		}
		kind := modelcore.PersistentTopologyType(strings.ToUpper(request.Kind))
		manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
		if err != nil {
			return FeatureInput{}, err
		}
		entry, ok := manifest.Locators[fmt.Sprintf("%d:%d", namingTopologyType(kind), request.LocalID)]
		if !ok || entry.Result == nil || *entry.Result != *a.Result {
			return FeatureInput{}, ErrNotFound
		}
		pick := FeatureSelection{SourceVersionID: request.VersionID, SourceFeatureID: f.ID, Selection: modelcore.PersistentSelection{SchemaVersion: modelcore.TopologyNamingSchemaVersion, SourceDocumentID: documentID, SourceResult: a.Result, Anchor: semanticRef(entry.Output.SemanticRef), ExpectedType: kind, Selector: modelcore.SelectionRecipe{Kind: modelcore.SelectionLineageDescendant}, CreationEvidence: selectionEvidence(entry.Output.Evidence)}}
		if err := pick.Selection.Validate(); err != nil {
			return FeatureInput{}, err
		}
		result.Selection = &pick
	}
	return result, nil
}

func geometryCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (s *Service) geometryPointPosition(ctx context.Context, requestID string, model PartModel, input GeometricInput, snapshots ...*[]any) ([3]float64, []*workerv1.SemanticTopologyRef, error) {
	if input.Kind == "POINT" || input.Kind == "DATUM" {
		frame, snapshot, err := s.geometryReferenceFrame(ctx, requestID, model, input)
		if err != nil {
			return [3]float64{}, nil, err
		}
		if !frame.point {
			return [3]float64{}, nil, fmt.Errorf("%w: CURVE_POINT_REQUIRES_VERTEX", ErrValidation)
		}
		for _, out := range snapshots {
			*out = append(*out, snapshot)
		}
		owner := geometryInputFeatureID(input)
		if input.Datum != nil {
			owner = input.Datum.EntityID
		}
		return frame.origin, []*workerv1.SemanticTopologyRef{{FeatureId: owner, OutputSlot: "point", SourceIds: []string{geometryReferenceSource(input)}}}, nil
	}
	source, err := s.geometrySource(ctx, requestID+"/point", model, input)
	if err != nil {
		return [3]float64{}, nil, err
	}
	for _, out := range snapshots {
		*out = append(*out, struct {
			Reference GeometricInput
			Source    *workerv1.GeometryFeatureInput
		}{input, source})
	}
	// The source snapshot already passed owner/stage validation. Locate its
	// single declared point using Naming; display points never supply position.
	var key string
	if input.Kind == "RESULT" {
		for _, f := range model.Features {
			if f.ID == input.FeatureID {
				key, err = s.evaluateGeometricFeature(ctx, requestID+"/point-result", &model, f.ID)
				break
			}
		}
	} else {
		key, err = s.featureStageInModel(ctx, model, "", geometryInputFeatureID(input), input.Selection.Selection.SourceBodyID)
	}
	if err != nil {
		return [3]float64{}, nil, err
	}
	if input.Kind == "RESULT" {
		a, e := s.loadArtifact(ctx, key)
		if e != nil {
			return [3]float64{}, nil, e
		}
		if a.GeometryDomain != "POINT_SET" {
			return [3]float64{}, nil, fmt.Errorf("%w: CURVE_POINT_REQUIRES_POINT_RESULT", ErrValidation)
		}
	}
	manifest, _, err := s.topologyManifestForGeometryKey(ctx, key)
	if err != nil {
		return [3]float64{}, nil, err
	}
	var point *workerv1.SemanticTopologyOutput
	for _, entry := range manifest.Locators {
		if topologyType(entry.Output.TopologyType) != modelcore.PersistentTopologyVertex {
			continue
		}
		if len(source.Members) > 0 && !sameSemanticRef(semanticRef(entry.Output.SemanticRef), semanticRef(source.Members[0])) {
			continue
		}
		if point != nil {
			return [3]float64{}, nil, fmt.Errorf("%w: CURVE_POINT_RESULT_AMBIGUOUS", ErrValidation)
		}
		point = entry.Output
	}
	if point == nil {
		return [3]float64{}, nil, fmt.Errorf("%w: CURVE_POINT_REFERENCE_MISSING", ErrValidation)
	}
	properties, err := s.getTopologyElementPropertiesFromArtifact(ctx, key, "VERTEX", point.LocalId)
	if err != nil {
		return [3]float64{}, nil, err
	}
	if properties.Point == nil {
		return [3]float64{}, nil, fmt.Errorf("%w: CURVE_POINT_COORDINATES_UNAVAILABLE", ErrValidation)
	}
	return *properties.Point, []*workerv1.SemanticTopologyRef{point.SemanticRef}, nil
}

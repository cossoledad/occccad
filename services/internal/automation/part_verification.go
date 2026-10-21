package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"

	"github.com/occccad/occccad/internal/access"
	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

// These small, explicit checks are application evidence. A planner can request
// additional checks; the Run owns mandatory checks and their user goal version.
type PartCheck struct {
	ID                string                      `json:"id"`
	Kind              string                      `json:"kind"`
	BodyID            string                      `json:"bodyId,omitempty"`
	FeatureID         string                      `json:"featureId,omitempty"`
	SketchID          string                      `json:"sketchId,omitempty"`
	ReferenceSketchID string                      `json:"referenceSketchId,omitempty"`
	ParameterID       string                      `json:"parameterId,omitempty"`
	ExpectedType      string                      `json:"expectedType,omitempty"`
	Plane             string                      `json:"plane,omitempty"`
	Width             float64                     `json:"width,omitempty"`
	Height            float64                     `json:"height,omitempty"`
	Length            float64                     `json:"length,omitempty"`
	Radius            float64                     `json:"radius,omitempty"`
	Value             float64                     `json:"value,omitempty"`
	Unit              string                      `json:"unit,omitempty"`
	Tolerance         float64                     `json:"tolerance,omitempty"`
	SelectionSet      string                      `json:"selectionSet,omitempty"`
	ExpectedCount     int                         `json:"expectedCount,omitempty"`
	Selection         *workspace.FeatureSelection `json:"selection,omitempty"`
}

type PartVerificationRequest struct {
	RevisionID       string      `json:"revisionId,omitempty"`
	GoalVersion      uint64      `json:"goalVersion"`
	CreatedEntityIDs []string    `json:"createdEntityIds,omitempty"`
	Checks           []PartCheck `json:"checks"`
}

type PartCheckResult struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`
	Passed   bool           `json:"passed"`
	Status   string         `json:"status"` // PASSED | FAILED | NOT_VERIFIED
	Code     string         `json:"code,omitempty"`
	Message  string         `json:"message,omitempty"`
	Observed map[string]any `json:"observed,omitempty"`
}

type PartVerificationResult struct {
	GoalVersion uint64                  `json:"goalVersion"`
	Snapshot    workspace.ModelSnapshot `json:"snapshot"`
	Passed      bool                    `json:"passed"`
	Checks      []PartCheckResult       `json:"checks"`
}

func (s *Service) VerifyPart(ctx context.Context, actor, document string, input PartVerificationRequest) (PartVerificationResult, error) {
	if err := s.authorize(ctx, actor, document, access.RoleViewer); err != nil {
		return PartVerificationResult{}, err
	}
	if input.GoalVersion == 0 || len(input.Checks) == 0 || len(input.Checks) > 64 || len(input.CreatedEntityIDs) > 4096 || len(marshal(input)) > 96<<10 {
		return PartVerificationResult{}, fault("INVALID_VERIFICATION", "verification requires a goal version and 1..64 bounded checks")
	}
	snapshot, err := s.workspace.ReadModelSnapshot(ctx, document, input.RevisionID)
	if err != nil {
		return PartVerificationResult{}, err
	}
	if snapshot.DocumentType != "PART" {
		return PartVerificationResult{}, fault("DOCUMENT_TYPE_UNSUPPORTED", "Part verification requires a Part snapshot")
	}
	var part workspace.PartModel
	if err = json.Unmarshal(snapshot.Model, &part); err != nil {
		return PartVerificationResult{}, err
	}
	result := PartVerificationResult{GoalVersion: input.GoalVersion, Snapshot: snapshot, Passed: true, Checks: []PartCheckResult{}}
	seen := map[string]bool{}
	created := map[string]bool{}
	for _, id := range input.CreatedEntityIDs {
		if id != "" {
			created[id] = true
		}
	}
	for _, check := range input.Checks {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if check.ID == "" || len(check.ID) > 160 || seen[check.ID] || check.Tolerance < 0 || math.IsNaN(check.Tolerance) || math.IsInf(check.Tolerance, 0) {
			return result, fault("INVALID_VERIFICATION", "check identities must be unique and tolerances finite and nonnegative")
		}
		seen[check.ID] = true
		item := s.verifyPartCheck(ctx, document, snapshot, part, created, check)
		result.Passed = result.Passed && item.Passed
		result.Checks = append(result.Checks, item)
	}
	return result, nil
}

func (s *Service) verifyPartCheck(ctx context.Context, document string, snapshot workspace.ModelSnapshot, part workspace.PartModel, created map[string]bool, check PartCheck) PartCheckResult {
	result := PartCheckResult{ID: check.ID, Kind: check.Kind, Status: "FAILED", Code: "GOAL_CHECK_FAILED", Observed: map[string]any{}}
	fail := func(message string) PartCheckResult { result.Message = message; return result }
	pass := func() PartCheckResult {
		result.Passed = true
		result.Status = "PASSED"
		result.Code = ""
		return result
	}
	notVerified := func(code, message string) PartCheckResult {
		result.Status = "NOT_VERIFIED"
		result.Code = code
		result.Message = message
		return result
	}
	tolerance := check.Tolerance
	if tolerance == 0 {
		tolerance = 1e-6
	}
	lengthCheck := check.Kind == "RECTANGLE_DIMENSIONS" || check.Kind == "EXTRUSION_LENGTH" || check.Kind == "CENTERED_THROUGH_HOLE" || check.Kind == "THROUGH_HOLE" || (check.Kind == "ALL_EDGES_FILLET" || check.Kind == "SELECTED_EDGES_FILLET")
	if lengthCheck && check.Unit != "mm" {
		return notVerified("UNIT_UNSUPPORTED", "dimension checks require explicit mm units")
	}
	features := []workspace.Feature{}
	for _, feature := range part.Features {
		if check.BodyID != "" && feature.BodyID != check.BodyID {
			continue
		}
		id := check.FeatureID
		if id == "" && check.SketchID != "" {
			id = check.SketchID
		}
		if id != "" && feature.ID != id {
			continue
		}
		if id == "" && !created[feature.ID] {
			continue
		}
		features = append(features, feature)
	}
	validFeature := func(feature workspace.Feature) bool {
		if feature.Sketch != nil {
			switch feature.Sketch.Solve.Status {
			case "FAILED", "CONFLICTING", "UNRESOLVED_EXTERNAL":
				return false
			}
		}
		return !feature.Suppressed && feature.Diagnostic == "" && (feature.EvaluationStatus == "" || feature.EvaluationStatus == "VALID" || feature.EvaluationStatus == "SUCCEEDED")
	}
	switch check.Kind {
	case "ENTITY_EXISTS":
		if check.BodyID != "" && check.FeatureID == "" && check.SketchID == "" {
			for _, body := range part.Bodies {
				if body.ID == check.BodyID {
					result.Observed["bodyId"] = body.ID
					return pass()
				}
			}
		}
		for _, feature := range features {
			if check.ExpectedType == "" || feature.Type == check.ExpectedType {
				result.Observed["featureId"] = feature.ID
				return pass()
			}
		}
		return fail("required stable entity was not found in this goal's output scope")
	case "FEATURE_VALID":
		if len(features) != 1 {
			return fail("feature validity requires one unambiguous feature identity")
		}
		result.Observed["featureId"], result.Observed["evaluationStatus"] = features[0].ID, features[0].EvaluationStatus
		if !validFeature(features[0]) {
			return fail("feature is suppressed or has an evaluation diagnostic")
		}
		return pass()
	case "RECTANGLE_DIMENSIONS":
		for _, feature := range features {
			if feature.Sketch == nil || !validFeature(feature) {
				continue
			}
			if check.Plane != "" && feature.Plane != check.Plane && feature.Sketch.Support.Plane != check.Plane {
				continue
			}
			width, height, ok := rectangleDimensions(*feature.Sketch, tolerance)
			if !ok {
				continue
			}
			result.Observed["featureId"], result.Observed["width"], result.Observed["height"] = feature.ID, width, height
			if check.Width > 0 && check.Height > 0 && closeNumber(width, check.Width, tolerance) && closeNumber(height, check.Height, tolerance) {
				return pass()
			}
		}
		return fail("a closed axis-aligned four-line rectangle does not match the required dimensions")
	case "EXTRUSION_LENGTH":
		for _, feature := range features {
			if !PartGoalTarget(part, feature, "EXTRUSION_LENGTH") || !validFeature(feature) {
				continue
			}
			if check.SketchID != "" && feature.Profile != check.SketchID {
				continue
			}
			result.Observed["featureId"], result.Observed["length"], result.Observed["extent"] = feature.ID, feature.Length, feature.Extent
			if check.Length > 0 && closeNumber(feature.Length, check.Length, tolerance) && (feature.Extent == "" || feature.Extent == "FINITE" || feature.Extent == "LENGTH" || feature.Extent == "ONE_SIDED") {
				return pass()
			}
		}
		return fail("extrusion length or extent does not match the required one-sided length")
	case "CENTERED_THROUGH_HOLE", "THROUGH_HOLE":
		for _, feature := range features {
			if !PartGoalTarget(part, feature, check.Kind) || !validFeature(feature) || feature.Extent != "THROUGH_ALL" || check.Radius <= 0 {
				continue
			}
			var sketch, reference *workspace.Feature
			for i := range part.Features {
				f := &part.Features[i]
				if f.ID == feature.Profile {
					sketch = f
				}
				if f.ID == check.ReferenceSketchID {
					reference = f
				}
			}
			if sketch == nil || !validFeature(*sketch) {
				continue
			}
			circle := profileCircle(*sketch.Sketch)
			if circle == nil || !closeNumber(circle.Radius, check.Radius, tolerance) {
				continue
			}
			normal := sketch.Sketch.Support.Normal
			center := sketchPointWorld(sketch.Sketch.Support, *circle.Center)
			if check.Plane != "" {
				planes := map[string][3]float64{"XY": {0, 0, 1}, "YZ": {1, 0, 0}, "XZ": {0, 1, 0}}
				expected, ok := planes[check.Plane]
				if !ok || !parallelVectors(normal, expected, tolerance) {
					continue
				}
			}
			if check.Kind == "CENTERED_THROUGH_HOLE" {
				if reference == nil || reference.Sketch == nil || reference.BodyID != feature.BodyID || !validFeature(*reference) {
					return notVerified("HOLE_CENTER_REFERENCE_REQUIRED", "孔中心需要绑定原始矩形草图，不能用后建草图或显示包围盒代替")
				}
				if _, _, ok := rectangleDimensions(*reference.Sketch, tolerance); !ok {
					return notVerified("HOLE_CENTER_REFERENCE_REQUIRED", "当前中心孔检查需要有效的矩形基体轮廓")
				}
				minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
				for _, e := range reference.Sketch.Entities {
					if e.Suppressed || e.Role != "PROFILE" || e.Kind != "LINE" {
						continue
					}
					for _, p := range []*workspace.SketchPoint2{e.Start, e.End} {
						if p != nil {
							minX = math.Min(minX, p.X)
							minY = math.Min(minY, p.Y)
							maxX = math.Max(maxX, p.X)
							maxY = math.Max(maxY, p.Y)
						}
					}
				}
				referenceCenter := sketchPointWorld(reference.Sketch.Support, workspace.SketchPoint2{X: (minX + maxX) / 2, Y: (minY + maxY) / 2})
				if !parallelVectors(normal, reference.Sketch.Support.Normal, tolerance) || !onAxis(center, referenceCenter, normal, tolerance) {
					return fail("贯穿孔中心与原始矩形面的中心不一致")
				}
			}
			for _, later := range part.Features {
				if later.BodyID == feature.BodyID && later.Order > feature.Order && !later.Suppressed && (PartGoalTarget(part, later, "EXTRUSION_LENGTH") || later.Type == "BOOLEAN" || later.Type == "SHELL" || later.Type == "SOLID_PATTERN") {
					return notVerified("HOLE_DOWNSTREAM_UNVERIFIED", "孔之后存在可能封堵孔口的实体操作，需要进一步检查最终贯穿性")
				}
			}
			// A cut definition alone is insufficient: require its named cylindrical
			// wall to survive in the current authoritative Body topology.
			reader, ok := s.workspace.(topologyReader)
			if !ok {
				return notVerified("TOPOLOGY_UNAVAILABLE", "贯穿孔检查需要权威拓扑")
			}
			topology, err := reader.ReadBodyTopology(ctx, document, snapshot.RevisionID, feature.BodyID, "FACE")
			if err != nil {
				return notVerified("TOPOLOGY_EVIDENCE_UNAVAILABLE", err.Error())
			}
			for _, face := range topology.Items {
				evidence := face.Selection.Selection.CreationEvidence
				if face.Selection.Selection.Anchor.FeatureID == feature.ID && strings.EqualFold(face.GeometryType, "CYLINDER") && evidence.RadiusMM != nil && closeNumber(*evidence.RadiusMM, check.Radius, tolerance) && parallelVectors(evidence.Direction, normal, tolerance) && onAxis(evidence.Origin, center, normal, tolerance) {
					result.Observed["featureId"], result.Observed["sketchId"], result.Observed["diameter"], result.Observed["extent"] = feature.ID, sketch.ID, 2*circle.Radius, feature.Extent
					return pass()
				}
			}
			return fail("当前实体没有该切除特征产生的有效圆柱孔壁")
		}
		return fail("未找到符合直径、位置及贯穿范围的圆孔切除特征")
	case "NO_FEATURE_TYPE":
		if check.ExpectedType == "" {
			return notVerified("CHECK_INPUT_REQUIRED", "forbidden feature type is required")
		}
		for _, feature := range part.Features {
			if feature.Type == check.ExpectedType && !feature.Suppressed && (check.BodyID == "" || feature.BodyID == check.BodyID) && (check.FeatureID == "" || feature.ID == check.FeatureID) {
				result.Observed["featureId"] = feature.ID
				return fail("a forbidden active feature exists in the target Body")
			}
		}
		return pass()
	case "PARAMETER_VALUE":
		if check.ParameterID == "" {
			return notVerified("CHECK_INPUT_REQUIRED", "parameter check requires a stable parameter ID")
		}
		quantity, err := modelcore.NewQuantity(check.Value, check.Unit)
		if err != nil {
			return notVerified("UNIT_UNSUPPORTED", err.Error())
		}
		for _, parameter := range part.Parameters {
			if parameter.ParameterID == check.ParameterID {
				if parameter.EvaluatedValue == nil {
					return fail("parameter has no evaluated value")
				}
				result.Observed["quantity"] = parameter.EvaluatedValue
				if parameter.EvaluatedValue.Dimension == quantity.Dimension && closeNumber(parameter.EvaluatedValue.SIValue, quantity.SIValue, tolerance*1e-3) {
					return pass()
				}
				return fail("evaluated parameter differs from the required quantity")
			}
		}
		return fail("parameter does not exist")
	case "BODY_GEOMETRY_VALID":
		bodyID := check.BodyID
		if bodyID == "" {
			bodyID = part.ActiveBodyID
		}
		for _, feature := range part.Features {
			if feature.BodyID == bodyID && !feature.Suppressed && !validFeature(feature) {
				return fail("an active feature in the target Body has an evaluation failure")
			}
		}
		for _, body := range part.Bodies {
			if body.ID == bodyID {
				if body.GeometryKey == "" || s.db == nil {
					return notVerified("GEOMETRY_EVIDENCE_MISSING", "Body has no authoritative evaluated geometry artifact")
				}
				var volume float64
				var unit, worker, evaluator string
				var breps int
				err := s.db.QueryRow(ctx, `SELECT volume,units,worker_id,evaluator_version,(SELECT count(*) FROM occccad.geometry_representations r JOIN occccad.artifact_objects o ON o.id=r.object_id AND o.state='READY' WHERE r.geometry_key=a.geometry_key AND r.role='BREP') FROM occccad.geometry_artifacts a WHERE geometry_key=$1`, body.GeometryKey).Scan(&volume, &unit, &worker, &evaluator, &breps)
				if errors.Is(err, database.ErrNoRows) {
					return notVerified("GEOMETRY_EVIDENCE_MISSING", "evaluated geometry artifact is unavailable")
				}
				if err != nil {
					return notVerified("GEOMETRY_EVIDENCE_UNAVAILABLE", err.Error())
				}
				result.Observed["bodyId"], result.Observed["geometryKey"], result.Observed["volume"], result.Observed["units"], result.Observed["workerId"], result.Observed["evaluatorVersion"] = bodyID, body.GeometryKey, volume, unit, worker, evaluator
				if !math.IsNaN(volume) && !math.IsInf(volume, 0) && volume > 0 && unit == "mm" && worker != "" && evaluator != "" && breps == 1 {
					return pass()
				}
				return fail("solid evaluation needs a positive finite volume and a ready authoritative B-Rep")
			}
		}
		return fail("target Body does not exist")
	case "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET":
		reader, ok := s.workspace.(topologyReader)
		if !ok {
			return notVerified("TOPOLOGY_UNAVAILABLE", "authoritative topology reader is unavailable")
		}
		for _, feature := range features {
			if feature.Type != "FILLET" || !validFeature(feature) || !closeNumber(feature.Length, check.Radius, tolerance) || check.Radius <= 0 || len(feature.Selections) == 0 {
				continue
			}
			sourceRevision := feature.Selections[0].SourceVersionID
			data, err := reader.ReadBodyTopology(ctx, document, sourceRevision, feature.BodyID, "EDGE")
			if err != nil {
				return notVerified("TOPOLOGY_EVIDENCE_UNAVAILABLE", err.Error())
			}
			result.Observed["featureId"], result.Observed["sourceRevisionId"], result.Observed["sourceEdgeCount"], result.Observed["selectedEdgeCount"], result.Observed["radius"] = feature.ID, sourceRevision, len(data.Items), len(feature.Selections), feature.Length
			required := data.Items
			if check.Kind == "SELECTED_EDGES_FILLET" && check.SelectionSet == "" {
				return notVerified("CHECK_INPUT_REQUIRED", "selected-edge verification requires the complete frozen selectionSet")
			}
			if check.SelectionSet != "" {
				var set topologySet
				raw, decodeErr := base64.RawURLEncoding.DecodeString(check.SelectionSet)
				if decodeErr != nil || json.Unmarshal(raw, &set) != nil || set.Document != document || set.Revision != sourceRevision || set.Body != feature.BodyID || set.Kind != "EDGE" || set.Digest != data.ManifestDigest || set.Count < 1 {
					return fail("selectionSet does not match the exact source manifest")
				}
				selected := map[string]bool{}
				for _, id := range set.IDs {
					if selected[id] {
						return fail("selectionSet has duplicate stable IDs")
					}
					selected[id] = true
				}
				if len(selected) > 0 {
					required = []workspace.BodyTopologyItem{}
					for _, item := range data.Items {
						if selected[item.ID] {
							required = append(required, item)
						}
					}
					if len(required) != len(selected) {
						return fail("selectionSet includes unavailable edges")
					}
				}
				if len(required) != set.Count {
					return fail("selectionSet is incomplete")
				}
				if check.Kind == "ALL_EDGES_FILLET" && len(required) != len(data.Items) {
					return fail("selectionSet is a subset of the required all-edge set")
				}
			}
			if len(feature.Selections) != len(required) || check.ExpectedCount > 0 && check.ExpectedCount != len(required) {
				return fail("fillet does not cover the complete required source edge set")
			}

			wanted := map[string]bool{}
			for _, item := range required {
				wanted[modelcore.ValueDigest(marshal(item.Selection.Selection))] = true
			}
			for _, selection := range feature.Selections {
				id := modelcore.ValueDigest(marshal(selection.Selection))
				if selection.SourceVersionID != sourceRevision || selection.Selection.SourceDocumentID != document || selection.Selection.SourceBodyID != feature.BodyID || selection.Selection.ExpectedType != "EDGE" || !wanted[id] {
					return fail("fillet includes a duplicate, missing or incorrectly scoped persistent edge")
				}
				delete(wanted, id)
			}
			if len(wanted) != 0 {
				return fail("some source edges were omitted")
			}

			return pass()
		}
		return fail("no evaluated fillet matches the required radius in this goal's output scope")
	case "PROTECTED_SELECTION":
		return s.verifyProtectedSelection(ctx, document, snapshot, check, result)
	default:
		return notVerified("CHECK_UNSUPPORTED", "unsupported Part verification kind: "+check.Kind)
	}
}

func closeNumber(actual, expected, tolerance float64) bool {
	return !math.IsNaN(actual) && !math.IsNaN(expected) && !math.IsInf(actual, 0) && !math.IsInf(expected, 0) && math.Abs(actual-expected) <= tolerance
}

func vectorCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func vectorNorm(a [3]float64) float64 { return math.Sqrt(a[0]*a[0] + a[1]*a[1] + a[2]*a[2]) }
func parallelVectors(a, b [3]float64, tolerance float64) bool {
	return closeNumber(vectorNorm(a), 1, tolerance) && closeNumber(vectorNorm(b), 1, tolerance) && closeNumber(vectorNorm(vectorCross(a, b)), 0, tolerance)
}
func onAxis(point, origin, axis [3]float64, tolerance float64) bool {
	return closeNumber(vectorNorm(axis), 1, tolerance) && closeNumber(vectorNorm(vectorCross([3]float64{point[0] - origin[0], point[1] - origin[1], point[2] - origin[2]}, axis)), 0, tolerance)
}
func sketchPointWorld(support workspace.SketchSupport, p workspace.SketchPoint2) [3]float64 {
	y := vectorCross(support.Normal, support.XDirection)
	return [3]float64{support.Origin[0] + p.X*support.XDirection[0] + p.Y*y[0], support.Origin[1] + p.X*support.XDirection[1] + p.Y*y[1], support.Origin[2] + p.X*support.XDirection[2] + p.Y*y[2]}
}

// PartGoalTarget classifies a receipt's explicit output by its domain role.
// ADD pads and REMOVE cuts share a generator; they are different goal targets.
// Incidental sketch updates from reevaluation must never retarget a goal.
func PartGoalTarget(part workspace.PartModel, feature workspace.Feature, kind string) bool {
	switch kind {
	case "RECTANGLE_DIMENSIONS":
		if feature.Sketch == nil {
			return false
		}
		_, _, ok := rectangleDimensions(*feature.Sketch, 1e-6)
		return ok
	case "EXTRUSION_LENGTH":
		return (feature.Type == "PAD" || feature.Type == "LINEAR_EXTRUDE") && (feature.Operation == "" || feature.Operation == "ADD" || feature.Operation == "NEW_BODY")
	case "ALL_EDGES_FILLET", "SELECTED_EDGES_FILLET":
		return feature.Type == "FILLET"
	case "THROUGH_HOLE", "CENTERED_THROUGH_HOLE":
		if (feature.Type != "PAD" && feature.Type != "LINEAR_EXTRUDE") || feature.Operation != "REMOVE" {
			return false
		}
		for _, sketch := range part.Features {
			if sketch.ID == feature.Profile && sketch.Sketch != nil {
				return profileCircle(*sketch.Sketch) != nil
			}
		}
	}
	return false
}

func profileCircle(sketch workspace.SketchFeature) *workspace.SketchEntity {
	var circle *workspace.SketchEntity
	for i := range sketch.Entities {
		e := &sketch.Entities[i]
		if e.Suppressed || strings.EqualFold(e.Role, "CONSTRUCTION") {
			continue
		}
		if !strings.EqualFold(e.Kind, "CIRCLE") || e.Center == nil || circle != nil {
			return nil
		}
		circle = e
	}
	return circle
}

func rectangleDimensions(sketch workspace.SketchFeature, tolerance float64) (float64, float64, bool) {
	points := []workspace.SketchPoint2{}
	edges := [][2]int{}
	for _, entity := range sketch.Entities {
		if entity.Suppressed || strings.ToUpper(entity.Role) == "CONSTRUCTION" {
			continue
		}
		if strings.ToUpper(entity.Kind) != "LINE" || entity.Start == nil || entity.End == nil {
			return 0, 0, false
		}
		pair := [2]int{}
		for i, point := range []workspace.SketchPoint2{*entity.Start, *entity.End} {
			index := -1
			for j, p := range points {
				if closeNumber(p.X, point.X, tolerance) && closeNumber(p.Y, point.Y, tolerance) {
					index = j
					break
				}
			}
			if index < 0 {
				index = len(points)
				points = append(points, point)
			}
			pair[i] = index
		}
		if pair[0] == pair[1] {
			return 0, 0, false
		}
		edges = append(edges, pair)
	}
	if len(edges) != 4 || len(points) != 4 {
		return 0, 0, false
	}
	minX, maxX, minY, maxY := points[0].X, points[0].X, points[0].Y, points[0].Y
	degree := [4]int{}
	seen := map[[2]int]bool{}
	for _, edge := range edges {
		a, b := points[edge[0]], points[edge[1]]
		if !closeNumber(a.X, b.X, tolerance) && !closeNumber(a.Y, b.Y, tolerance) {
			return 0, 0, false
		}
		if edge[0] > edge[1] {
			edge[0], edge[1] = edge[1], edge[0]
		}
		if seen[edge] {
			return 0, 0, false
		}
		seen[edge] = true
		degree[edge[0]]++
		degree[edge[1]]++
	}
	for i, p := range points {
		if degree[i] != 2 {
			return 0, 0, false
		}
		minX = math.Min(minX, p.X)
		maxX = math.Max(maxX, p.X)
		minY = math.Min(minY, p.Y)
		maxY = math.Max(maxY, p.Y)
	}
	if maxX-minX <= tolerance || maxY-minY <= tolerance {
		return 0, 0, false
	}
	for _, p := range points {
		if (!closeNumber(p.X, minX, tolerance) && !closeNumber(p.X, maxX, tolerance)) || (!closeNumber(p.Y, minY, tolerance) && !closeNumber(p.Y, maxY, tolerance)) {
			return 0, 0, false
		}
	}
	return maxX - minX, maxY - minY, true
}

type protectedSelectionReader interface {
	GetResolvedTopologyElementProperties(context.Context, string, workspace.ResolvePersistentSelectionRequest) (workspace.ResolvedTopologyProperties, error)
}

func (s *Service) verifyProtectedSelection(ctx context.Context, document string, snapshot workspace.ModelSnapshot, check PartCheck, result PartCheckResult) PartCheckResult {
	reader, ok := s.workspace.(protectedSelectionReader)
	if !ok || check.Selection == nil {
		result.Status = "NOT_VERIFIED"
		result.Code = "SELECTION_EVIDENCE_UNAVAILABLE"
		result.Message = "protected geometry requires a formal selection and an authoritative resolver"
		return result
	}
	selection := check.Selection
	if selection.SourceVersionID == "" || selection.Selection.SourceDocumentID != document || selection.Selection.Validate() != nil {
		result.Message = "protected selection has an invalid source scope"
		return result
	}
	request := workspace.ResolvePersistentSelectionRequest{Selection: selection.Selection, SourceVersionID: selection.SourceVersionID, TargetVersionID: selection.SourceVersionID, PolicyDigest: modelcore.TopologyNamingPolicyDigest}
	before, err := reader.GetResolvedTopologyElementProperties(ctx, document, request)
	if err != nil {
		result.Status = "NOT_VERIFIED"
		result.Code = "SELECTION_EVIDENCE_UNAVAILABLE"
		result.Message = err.Error()
		return result
	}
	request.TargetVersionID = snapshot.RevisionID
	after, err := reader.GetResolvedTopologyElementProperties(ctx, document, request)
	if err != nil {
		result.Status = "NOT_VERIFIED"
		result.Code = "SELECTION_EVIDENCE_UNAVAILABLE"
		result.Message = err.Error()
		return result
	}
	result.Observed["sourceRevisionId"], result.Observed["targetRevisionId"], result.Observed["resolutionStatus"] = selection.SourceVersionID, snapshot.RevisionID, after.Resolution.Status
	if before.Resolution.Status != modelcore.SelectionResolved || after.Resolution.Status != modelcore.SelectionResolved || before.Properties == nil || after.Properties == nil {
		result.Message = "protected geometry is missing, ambiguous or has a type mismatch"
		return result
	}
	canonical := func(properties *workspace.TopologyElementProperties) map[string]any {
		return map[string]any{"geometryType": properties.GeometryType, "bbox": properties.BBox, "point": properties.Point, "properties": properties.Properties}
	}
	tolerance := check.Tolerance
	if tolerance == 0 {
		tolerance = 1e-6
	}
	var first, second any
	_ = json.Unmarshal(marshal(canonical(before.Properties)), &first)
	_ = json.Unmarshal(marshal(canonical(after.Properties)), &second)
	if !equalGeometryEvidence(first, second, tolerance) {
		result.Message = "protected geometry changed relative to its frozen source"
		return result
	}
	result.Passed = true
	result.Status = "PASSED"
	result.Code = ""
	return result
}

func equalGeometryEvidence(first, second any, tolerance float64) bool {
	switch a := first.(type) {
	case float64:
		b, ok := second.(float64)
		return ok && closeNumber(a, b, tolerance)
	case []any:
		b, ok := second.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i, v := range a {
			if !equalGeometryEvidence(v, b[i], tolerance) {
				return false
			}
		}
		return true
	case map[string]any:
		b, ok := second.(map[string]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for key, v := range a {
			w, exists := b[key]
			if !exists || !equalGeometryEvidence(v, w, tolerance) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(first, second)
	}
}

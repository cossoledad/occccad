package workspace

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/occccad/occccad/internal/geometry"
)

// SketchProfileAnalysis is a read projection of an accepted Sketch revision.
// It never drives solver geometry, creates Coincident relations, or prevents
// an open sketch from being saved. Solid features still use buildProfileRegions.
type SketchProfileAnalysis struct {
	GeometryVerified bool                 `json:"geometryVerified"`
	Status           string               `json:"status"` // EMPTY | OPEN | INVALID | CLOSED
	RegionCount      int                  `json:"regionCount"`
	LoopCount        int                  `json:"loopCount"`
	RegionIDs        []string             `json:"regionIds,omitempty"`
	Issues           []SketchProfileIssue `json:"issues"`
	// Regions come directly from the production Profile Builder, not from a
	// second display-only connectivity implementation. CLOSED is its gate, not
	// a claim that a downstream OCCT Shape operation has already succeeded.
	Regions []geometry.ProfileRegion `json:"-"`
}

type SketchProfileIssue struct {
	Code       string              `json:"code"`
	Message    string              `json:"message"`
	EntityIDs  []string            `json:"entityIds"`
	References []SketchGeometryRef `json:"references,omitempty"`
	Position   *SketchPoint2       `json:"position,omitempty"`
}

func projectSketchAnalyses(model PartModel) map[string]SketchProfileAnalysis {
	result := map[string]SketchProfileAnalysis{}
	for _, feature := range model.Features {
		if feature.Sketch != nil {
			result[feature.ID] = analyzeSketchProfile(feature)
		}
	}
	return result
}

func analyzeSketchProfile(feature Feature) SketchProfileAnalysis {
	analysis := SketchProfileAnalysis{Status: "EMPTY", Issues: []SketchProfileIssue{}}
	if feature.Sketch == nil {
		return analysis
	}
	connectivity := profileConnectivity(feature)
	type endpoint struct {
		entityID string
		ref      SketchGeometryRef
		point    SketchPoint2
	}
	endpoints := map[string][]endpoint{}
	entityIDs := []string{}
	duplicateKeys := map[string]string{}
	orderedEntities := append([]SketchEntity(nil), feature.Sketch.Entities...)
	sort.Slice(orderedEntities, func(i, j int) bool { return orderedEntities[i].ID < orderedEntities[j].ID })
	var profilePosition *SketchPoint2
	for _, entity := range orderedEntities {
		if entity.Suppressed || entity.Role != "PROFILE" || entity.Kind == "POINT" {
			continue
		}
		entityIDs = append(entityIDs, entity.ID)
		if profilePosition == nil {
			profilePosition = profileEntityAnalysisPosition(entity)
		}
		key := profileDuplicateGeometryKey(entity)
		if prior, exists := duplicateKeys[key]; key != "" && exists {
			position := profileEntityAnalysisPosition(entity)
			analysis.Issues = append(analysis.Issues, SketchProfileIssue{Code: "DUPLICATE_EDGE", Message: "两个轮廓元素具有相同几何；请删除重复元素", EntityIDs: []string{prior, entity.ID}, Position: position})
		} else if key != "" {
			duplicateKeys[key] = entity.ID
		}
		start, end, open := entityProfileEndpoints(entity)
		if !open {
			continue
		}
		if math.Hypot(end.X-start.X, end.Y-start.Y) <= profileTolerance && entity.Kind == "LINE" {
			p := start
			analysis.Issues = append(analysis.Issues, SketchProfileIssue{Code: "DEGENERATE_EDGE", Message: "轮廓线段长度低于几何容差", EntityIDs: []string{entity.ID}, Position: &p})
		}
		for _, value := range []struct {
			sub string
			p   SketchPoint2
		}{{"START", start}, {"END", end}} {
			node := connectivity.find(entity.ID + "/" + value.sub)
			endpoints[node] = append(endpoints[node], endpoint{entity.ID, SketchGeometryRef{Target: "ENTITY", EntityID: entity.ID, SubElement: value.sub}, value.p})
		}
	}
	if len(entityIDs) == 0 {
		return analysis
	}
	sort.Strings(entityIDs)
	nodes := make([]string, 0, len(endpoints))
	for node := range endpoints {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		members := endpoints[node]
		refs := []SketchGeometryRef{}
		ids := []string{}
		for _, member := range members {
			refs = append(refs, member.ref)
			ids = append(ids, member.entityID)
		}
		p := members[0].point
		if len(members) != 2 {
			code, message := "OPEN_ENDPOINT", "轮廓端点尚未通过正式重合关系连接"
			if len(members) > 2 {
				code, message = "BRANCH_POINT", "轮廓连接点包含多于两条边"
			}
			analysis.Issues = append(analysis.Issues, SketchProfileIssue{Code: code, Message: message, EntityIDs: ids, References: refs, Position: &p})
		}
		for _, member := range members[1:] {
			if math.Hypot(member.point.X-p.X, member.point.Y-p.Y) > profileTolerance {
				analysis.Issues = append(analysis.Issues, SketchProfileIssue{Code: "UNSOLVED_CONNECTION", Message: "正式重合关系的端点坐标尚未一致", EntityIDs: ids, References: refs, Position: &p})
				break
			}
		}
	}
	if len(analysis.Issues) > 0 {
		analysis.Status = "OPEN"
		for _, issue := range analysis.Issues {
			if issue.Code != "OPEN_ENDPOINT" {
				analysis.Status = "INVALID"
				break
			}
		}
		sortProfileAnalysisIssues(analysis.Issues)
		return analysis
	}
	regions, err := buildProfileRegions(feature)
	if err != nil {
		analysis.Status = "INVALID"
		message := strings.TrimPrefix(err.Error(), ErrValidation.Error()+": ")
		analysis.Issues = append(analysis.Issues, SketchProfileIssue{Code: "INVALID_REGION", Message: message, EntityIDs: entityIDs, Position: profilePosition})
		return analysis
	}
	analysis.Status = "CLOSED"
	analysis.Regions = regions
	analysis.RegionCount = len(regions)
	for _, region := range regions {
		analysis.RegionIDs = append(analysis.RegionIDs, region.ID)
		analysis.LoopCount += 1 + len(region.Holes)
	}
	return analysis
}

func profileEntityAnalysisPosition(entity SketchEntity) *SketchPoint2 {
	if entity.Start != nil {
		p := *entity.Start
		return &p
	}
	if entity.Center != nil {
		p := *entity.Center
		return &p
	}
	if len(entity.ControlPoints) > 0 {
		p := entity.ControlPoints[0]
		return &p
	}
	return nil
}

// Exact duplicate definitions are diagnostics, never connectivity. Lines are
// canonicalized without direction; full periodic curves without their seam.
// Partial overlaps and curve intersections remain the production Builder gate.
func profileDuplicateGeometryKey(entity SketchEntity) string {
	entity.ID = ""
	entity.CreatedByOperationID = ""
	entity.SourceEntityID = ""
	entity.ControlPointIDs = nil
	entity.PoleIDs = nil
	entity.Visible = nil
	entity.Role = ""
	entity.Suppressed = false
	if entity.Kind == "SPLINE" && len(entity.Poles) > 0 {
		entity.ControlPoints = nil
		entity.Mode = ""
	}
	if entity.Kind == "LINE" && entity.Start != nil && entity.End != nil {
		if entity.Start.X > entity.End.X || (entity.Start.X == entity.End.X && entity.Start.Y > entity.End.Y) {
			entity.Start, entity.End = entity.End, entity.Start
		}
	}
	if entity.Kind == "ELLIPSE" {
		entity.Rotation = math.Mod(entity.Rotation, math.Pi)
		if entity.Rotation < 0 {
			entity.Rotation += math.Pi
		}
		entity.StartAngle = 0
		entity.EndAngle = 0
	}
	if entity.Kind == "CIRCLE" {
		entity.StartAngle = 0
		entity.EndAngle = 0
	}
	value, err := json.Marshal(entity)
	if err != nil {
		return ""
	}
	return string(value)
}

func sortProfileAnalysisIssues(issues []SketchProfileIssue) {
	for index := range issues {
		sort.Strings(issues[index].EntityIDs)
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return strings.Join(issues[i].EntityIDs, "/") < strings.Join(issues[j].EntityIDs, "/")
	})
}

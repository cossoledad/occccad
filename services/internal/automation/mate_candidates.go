package automation

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/occccad/occccad/internal/modelcore"
	"github.com/occccad/occccad/internal/workspace"
)

type MateCandidateQuery struct {
	RevisionID             string   `json:"revisionId"`
	FirstInstanceIDs       []string `json:"firstInstanceIds"`
	SecondInstanceIDs      []string `json:"secondInstanceIds"`
	FirstBodyID            string   `json:"firstBodyId,omitempty"`
	SecondBodyID           string   `json:"secondBodyId,omitempty"`
	AxisDirectionRelation  string   `json:"axisDirectionRelation,omitempty"`
	PlaneDirectionRelation string   `json:"planeDirectionRelation,omitempty"`
	OffsetMM               float64  `json:"offsetMM,omitempty"`
	Limit                  int      `json:"limit,omitempty"`
	CandidateID            string   `json:"candidateId,omitempty"`
	IncludeRecipe          bool     `json:"includeRecipe,omitempty"`
}
type MateRecipeStep struct {
	Role        string         `json:"role"`
	OperationID string         `json:"operationId"`
	Version     int            `json:"version"`
	Arguments   map[string]any `json:"arguments"`
}
type MateCandidate struct {
	includeRecipe            bool
	ID                       string                        `json:"id"`
	FirstAxis                workspace.AssemblyGeometryRef `json:"firstAxis"`
	SecondAxis               workspace.AssemblyGeometryRef `json:"secondAxis"`
	FirstPlane               workspace.AssemblyGeometryRef `json:"firstPlane"`
	SecondPlane              workspace.AssemblyGeometryRef `json:"secondPlane"`
	AxisDirectionRelation    string                        `json:"axisDirectionRelation"`
	PlaneDirectionRelation   string                        `json:"planeDirectionRelation"`
	OffsetMM                 float64                       `json:"offsetMM"`
	RadialClearanceMM        float64                       `json:"radialClearanceMM"`
	FirstMeasuredAxis        workspace.AutomationSupport   `json:"firstMeasuredAxis"`
	SecondMeasuredAxis       workspace.AutomationSupport   `json:"secondMeasuredAxis"`
	FirstAxialRangeMM        *[2]float64                   `json:"firstAxialRangeMM,omitempty"`
	SecondAxialRangeMM       *[2]float64                   `json:"secondAxialRangeMM,omitempty"`
	InitialAxisDistanceMM    float64                       `json:"initialAxisDistanceMM"`
	InitialAxisAngleRad      float64                       `json:"initialAxisAngleRad"`
	InitialEndPlaneOffsetMM  float64                       `json:"initialEndPlaneOffsetMM"`
	MovingEndCoordinateMM    float64                       `json:"movingEndCoordinateMM"`
	ReferenceEndCoordinateMM float64                       `json:"referenceEndCoordinateMM"`
	Reasons                  []string                      `json:"reasons"`
	Recipe                   []MateRecipeStep              `json:"-"`
	Verification             AssemblyVerificationInput     `json:"-"`
}

// Canonical formal references occur once in a tool page. After choosing a
// stable candidate ID, a separate small read supplies its original commands;
// repeating every reference in choices, recipe and goal checks wastes context.
func (c MateCandidate) MarshalJSON() ([]byte, error) {
	verification := c.Verification
	verification.FirstAxis, verification.SecondAxis, verification.FirstPlane, verification.SecondPlane = nil, nil, nil, nil
	if c.includeRecipe {
		return json.Marshal(struct {
			ID           string                    `json:"id"`
			Recipe       []MateRecipeStep          `json:"recipe"`
			Verification AssemblyVerificationInput `json:"verification"`
		}{c.ID, c.Recipe, verification})
	}
	type plain MateCandidate
	return json.Marshal(struct {
		plain
		Verification AssemblyVerificationInput `json:"verification"`
	}{plain(c), verification})
}

type MateCandidatePage struct {
	RevisionID        string          `json:"revisionId"`
	Items             []MateCandidate `json:"items"`
	MatchedCount      int             `json:"matchedCount"`
	Complete          bool            `json:"complete"`
	RequiresSelection bool            `json:"requiresSelection"`
	Explanation       string          `json:"explanation"`
}

func mateDot(a, b [3]float64) float64    { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func mateSub(a, b [3]float64) [3]float64 { return [3]float64{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func mateCross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func mateNorm(a [3]float64) float64 { return math.Sqrt(mateDot(a, a)) }

func (s *Service) mateSupports(ctx context.Context, actor, document, revision string, ids []string, body string) ([]workspace.AutomationGeometry, bool, error) {
	items := []workspace.AutomationGeometry{}
	complete := true
	for _, kind := range []string{"PUBLICATION", "DATUM", "FACE"} {
		cursor := ""
		for pageNo := 0; pageNo < 5; pageNo++ {
			page, err := s.QueryOccurrence(ctx, actor, document, OccurrenceQuery{RevisionID: revision, InstanceIDs: ids, BodyID: body, Kind: kind, Cursor: cursor, Limit: 20})
			if err != nil {
				return nil, false, err
			}
			items = append(items, page.Items...)
			if !page.HasMore {
				break
			}
			cursor = page.NextCursor
			if pageNo == 4 {
				complete = false
			}
		}
	}
	return items, complete, nil
}

// Prefer a geometrically equivalent formal Publication/Datum while retaining
// measured radius/material side from the actual cylindrical or planar face.
func preferredMateReference(face workspace.AutomationGeometry, items []workspace.AutomationGeometry) workspace.AssemblyGeometryRef {
	best := face.Reference
	rank := 0
	ambiguous := false
	for _, item := range items {
		priority := 0
		if item.Source == "DATUM" {
			priority = 1
		}
		if item.Source == "PUBLICATION" {
			priority = 2
		}
		if priority < rank || priority == 0 {
			continue
		}
		axial := face.Kind == "CYLINDER" && (item.Kind == "AXIS" || item.Kind == "CYLINDER")
		planar := face.Kind == "PLANE" && item.Kind == "PLANE"
		if !axial && !planar {
			continue
		}
		if mateNorm(mateCross(face.Definition.Direction, item.Definition.Direction)) > 1e-8 {
			continue
		}
		if mateDot(face.Definition.Direction, item.Definition.Direction) < 1-1e-9 {
			continue
		}
		delta := mateSub(face.Definition.Origin, item.Definition.Origin)
		if axial && mateNorm(mateCross(delta, face.Definition.Direction)) > 1e-7 {
			continue
		}
		if planar && (math.Abs(mateDot(delta, face.Definition.Direction)) > 1e-7 || mateDot(face.Definition.Direction, item.Definition.Direction) < 1-1e-9) {
			continue
		}
		if priority == rank {
			ambiguous = true
			continue
		}
		best, rank, ambiguous = item.Reference, priority, false
	}
	if ambiguous {
		return face.Reference
	}
	return best
}

// MateCandidates describes choices; it never inserts an instance, saves a
// constraint or silently selects a same-name object. Recipes use the original
// public Domain Commands and still require normal preview/CAS execution.
func (s *Service) MateCandidates(ctx context.Context, actor, document string, input MateCandidateQuery) (MateCandidatePage, error) {
	out := MateCandidatePage{RevisionID: input.RevisionID, Items: []MateCandidate{}, Complete: true}
	if input.Limit == 0 {
		input.Limit = 2
	}
	if input.Limit < 1 || input.Limit > 8 || input.RevisionID == "" || math.IsNaN(input.OffsetMM) || math.IsInf(input.OffsetMM, 0) {
		return out, fault("INVALID_QUERY", "exact root revision, finite offset and candidate limit 1..8 required")
	}
	if len(input.CandidateID) > 160 || input.IncludeRecipe && input.CandidateID == "" {
		return out, fault("INVALID_QUERY", "read a recipe only after selecting its stable candidateId")
	}
	firstPath, err := occurrencePath(document, input.FirstInstanceIDs, nil)
	if err != nil {
		return out, err
	}
	secondPath, err := occurrencePath(document, input.SecondInstanceIDs, nil)
	if err != nil {
		return out, err
	}
	if firstPath.Segments[0].InstanceID == secondPath.Segments[0].InstanceID {
		return out, fault("SAME_MOTION_UNIT", "nested supports in one rigid Product cannot be mated by moving its parent against itself")
	}
	if input.AxisDirectionRelation == "" {
		input.AxisDirectionRelation = "SAME"
	}
	if input.PlaneDirectionRelation == "" {
		input.PlaneDirectionRelation = "OPPOSITE"
	}
	for _, relation := range []string{input.AxisDirectionRelation, input.PlaneDirectionRelation} {
		if relation != "SAME" && relation != "OPPOSITE" {
			return out, fault("INVALID_QUERY", "direction relations must be SAME or OPPOSITE")
		}
	}
	first, c1, err := s.mateSupports(ctx, actor, document, input.RevisionID, input.FirstInstanceIDs, input.FirstBodyID)
	if err != nil {
		return out, err
	}
	second, c2, err := s.mateSupports(ctx, actor, document, input.RevisionID, input.SecondInstanceIDs, input.SecondBodyID)
	if err != nil {
		return out, err
	}
	out.Complete = c1 && c2
	root, err := s.workspace.ReadModelSnapshot(ctx, document, input.RevisionID)
	if err != nil {
		return out, err
	}
	var product workspace.ProductModel
	if err = json.Unmarshal(root.Model, &product); err != nil {
		return out, err
	}
	referenceFixed := false
	for _, constraint := range product.Constraints {
		if !constraint.Suppressed && constraint.Kind == "FIX" && constraint.First.InstanceID == input.SecondInstanceIDs[0] && constraint.EvaluationStatus == modelcore.AssemblyConstraintVerified {
			referenceFixed = true
		}
	}
	for _, a := range first {
		if a.Kind != "CYLINDER" || a.Source != "TOPOLOGY" {
			continue
		}
		for _, b := range second {
			if b.Kind != "CYLINDER" || b.Source != "TOPOLOGY" || a.Definition.MaterialSide*b.Definition.MaterialSide != -1 {
				continue
			}
			clearance := b.Definition.Radius - a.Definition.Radius
			if a.Definition.MaterialSide < 0 {
				clearance = -clearance
			}
			if clearance < -1e-7 {
				continue
			}
			for _, p := range first {
				if p.Kind != "PLANE" || p.Source != "TOPOLOGY" || math.Abs(mateDot(p.Definition.Direction, a.Definition.Direction)) < 1-1e-9 {
					continue
				}
				for _, q := range second {
					if q.Kind != "PLANE" || q.Source != "TOPOLOGY" || math.Abs(mateDot(q.Definition.Direction, b.Definition.Direction)) < 1-1e-9 {
						continue
					}
					axisSign, planeSign := 1.0, 1.0
					if input.AxisDirectionRelation == "OPPOSITE" {
						axisSign = -1
					}
					if input.PlaneDirectionRelation == "OPPOSITE" {
						planeSign = -1
					}
					if mateDot(p.Definition.Direction, a.Definition.Direction)*mateDot(q.Definition.Direction, b.Definition.Direction)*axisSign*planeSign < 1-1e-8 {
						continue
					}
					fa, sa, fp, sp := preferredMateReference(a, first), preferredMateReference(b, second), preferredMateReference(p, first), preferredMateReference(q, second)
					target := uint64(1)
					offset := input.OffsetMM
					candidate := MateCandidate{FirstAxis: fa, SecondAxis: sa, FirstPlane: fp, SecondPlane: sp, AxisDirectionRelation: input.AxisDirectionRelation, PlaneDirectionRelation: input.PlaneDirectionRelation, OffsetMM: offset, RadialClearanceMM: clearance, InitialAxisDistanceMM: mateNorm(mateCross(mateSub(a.World.Origin, b.World.Origin), b.World.Direction)), InitialAxisAngleRad: math.Acos(math.Min(1, math.Abs(mateDot(a.World.Direction, b.World.Direction)))), InitialEndPlaneOffsetMM: mateDot(mateSub(p.World.Origin, q.World.Origin), q.World.Direction), MovingEndCoordinateMM: mateDot(mateSub(p.Definition.Origin, a.Definition.Origin), a.Definition.Direction), ReferenceEndCoordinateMM: mateDot(mateSub(q.Definition.Origin, b.Definition.Origin), b.Definition.Direction), Reasons: []string{"analytic cylindrical faces have opposite material sides (shaft/hole)", "hole radius accommodates shaft radius; reported radial clearance is not a fit tolerance", "end planes are perpendicular to the measured axes", "formal Publication/Datum preferred where geometrically equivalent", "direction and end choice remain explicit; ranking is not approval"}, Verification: AssemblyVerificationInput{RevisionID: input.RevisionID, MovingInstanceID: input.FirstInstanceIDs[0], ReferenceInstanceID: input.SecondInstanceIDs[0], FirstAxis: &fa, SecondAxis: &sa, FirstPlane: &fp, SecondPlane: &sp, OffsetMM: &offset, DirectionRelation: input.PlaneDirectionRelation, TargetDOF: &target}}
					candidate.ID = modelcore.ValueDigest(marshal(struct {
						Revision    string
						Refs        []workspace.AssemblyGeometryRef
						Axis, Plane string
						Offset      float64
					}{input.RevisionID, []workspace.AssemblyGeometryRef{fa, sa, fp, sp}, input.AxisDirectionRelation, input.PlaneDirectionRelation, offset}))
					if input.CandidateID != "" && input.CandidateID != candidate.ID {
						continue
					}
					candidate.includeRecipe = input.IncludeRecipe
					candidate.InitialEndPlaneOffsetMM = mateDot(mateSub(p.World.Origin, q.World.Origin), p.World.Direction)
					candidate.Verification.AxisDirectionRelation = input.AxisDirectionRelation
					candidate.FirstMeasuredAxis, candidate.SecondMeasuredAxis = a.Definition, b.Definition
					candidate.FirstAxialRangeMM, candidate.SecondAxialRangeMM = a.AxialRangeMM, b.AxialRangeMM
					if !referenceFixed {
						candidate.Recipe = append(candidate.Recipe, MateRecipeStep{Role: "ground-reference", OperationID: "command.add_assembly_constraint", Version: 1, Arguments: map[string]any{"constraintKind": "FIX", "fixMode": "SPACE", "firstAssemblyRef": workspace.AssemblyGeometryRef{InstanceID: input.SecondInstanceIDs[0], Kind: "BODY"}}})
					}
					candidate.Recipe = append(candidate.Recipe, MateRecipeStep{Role: "coaxial", OperationID: "command.add_assembly_constraint", Version: 1, Arguments: map[string]any{"constraintKind": "CONCENTRIC", "firstAssemblyRef": fa, "secondAssemblyRef": sa, "directionRelation": input.AxisDirectionRelation}})
					planeKind := "COINCIDENT"
					if offset != 0 {
						planeKind = "DISTANCE"
					}
					candidate.Recipe = append(candidate.Recipe, MateRecipeStep{Role: "end-location", OperationID: "command.add_assembly_constraint", Version: 1, Arguments: map[string]any{"constraintKind": planeKind, "firstAssemblyRef": fp, "secondAssemblyRef": sp, "directionRelation": input.PlaneDirectionRelation, "value": offset, "distanceRelation": "SELECTED_PLANE_NORMAL_V1"}})
					out.Items = append(out.Items, candidate)
				}
			}
		}
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		if a.RadialClearanceMM != b.RadialClearanceMM {
			return a.RadialClearanceMM < b.RadialClearanceMM
		}
		return a.ID < b.ID
	})
	out.MatchedCount = len(out.Items)
	if input.CandidateID != "" && out.MatchedCount == 0 {
		return out, fault("MATE_CANDIDATE_NOT_FOUND", "candidate no longer belongs to the requested immutable root query")
	}
	if len(out.Items) > input.Limit {
		out.Items = out.Items[:input.Limit]
		out.Complete = false
	}
	for len(out.Items) > 1 && len(marshal(out)) > 28<<10 {
		out.Items = out.Items[:len(out.Items)-1]
		out.Complete = false
	}
	if len(marshal(out)) > 28<<10 {
		return out, fault("MATE_DESCRIPTOR_RESOURCE", "candidate exceeds the tool page budget; query its exact supports individually")
	}
	out.RequiresSelection = out.MatchedCount != 1 || !out.Complete
	out.Explanation = "First occurrence moves; second is reference. Select by geometry/end coordinates. Re-read the same query with candidateId and includeRecipe=true for original Domain Commands. References are formal, never display names or array positions. Offset is signed along firstPlane normal (SELECTED_PLANE_NORMAL_V1)."
	if out.MatchedCount == 0 {
		out.Explanation = "No compatible cylindrical shaft/hole and end-plane pair found in the bounded analytic pages; query more geometry or choose formal supports explicitly."
	}
	return out, nil
}

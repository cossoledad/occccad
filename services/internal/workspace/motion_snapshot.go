package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/occccad/occccad/internal/geometry"
)

const motionPolicy = "grounded-single-coordinate-explicit-relations-v2"

type MotionRunRequest struct {
	BaseRevisionID  string          `json:"baseRevisionId"`
	AnalysisID      string          `json:"analysisId,omitempty"`
	Trial           *MotionQuantity `json:"trial,omitempty"`
	DriverID        string          `json:"driverId,omitempty"` // trial without a saved time study
	StudyID         string          `json:"studyId"`
	RequestID       string          `json:"requestId"`
	CurrentOnly     bool            `json:"currentOnly"`
	Scope           []DMUAddress    `json:"scope,omitempty"`
	Clearance       MotionQuantity  `json:"clearance"`
	IncludeSameUnit bool            `json:"includeSameUnit"`
}
type MotionGeometryUnit struct {
	ID           string                     `json:"id"`
	MotionUnitID string                     `json:"motionUnitId"`
	Address      DMUAddress                 `json:"address"`
	GeometryKey  string                     `json:"geometryKey"`
	GeometryID   string                     `json:"geometryId"`
	BRep         geometry.ArtifactReference `json:"brep"`
	RelativePose InstancePose               `json:"relativePose"`
}
type MotionSnapshot struct {
	Schema        int                            `json:"schema"`
	Policy        string                         `json:"policy"`
	Digest        string                         `json:"digest"`
	DocumentID    string                         `json:"documentId"`
	RevisionID    string                         `json:"revisionId"`
	CurrentOnly   bool                           `json:"currentOnly"`
	View          DocumentView                   `json:"view"`
	Mechanism     Mechanism                      `json:"mechanism"`
	Analysis      *InterferenceAnalysis          `json:"analysis,omitempty"`
	Study         MotionStudy                    `json:"study"`
	Equations     mechanismEquations             `json:"equations"`
	SolverProfile geometry.AssemblySolverProfile `json:"solverProfile"`
	GeometryUnits []MotionGeometryUnit           `json:"geometryUnits"`
	Pairs         []geometry.AnalysisPair        `json:"pairs"`
}

func motionDigest(s MotionSnapshot) string {
	s.Digest = ""
	b, _ := json.Marshal(s)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (service *Service) FreezeMotionStudy(ctx context.Context, documentID string, req MotionRunRequest) (MotionSnapshot, error) {
	view, err := service.GetDocument(ctx, documentID)
	if err != nil {
		return MotionSnapshot{}, err
	}
	if view.Product == nil || req.BaseRevisionID == "" || view.Document.VersionID != req.BaseRevisionID {
		return MotionSnapshot{}, fmt.Errorf("%w: Product revision changed", ErrValidation)
	}
	s := MotionSnapshot{Schema: 1, Policy: motionPolicy, DocumentID: documentID, RevisionID: req.BaseRevisionID, View: view, CurrentOnly: req.CurrentOnly, SolverProfile: defaultAssemblySolverProfile()}
	if (req.DriverID != "" && (req.Trial == nil || req.StudyID != "")) || (req.Trial != nil && (req.CurrentOnly || req.AnalysisID != "")) {
		return s, fmt.Errorf("%w: ambiguous motion trial request", ErrValidation)
	}
	if len(view.Product.Instances) > 64 {
		return s, fmt.Errorf("%w: study supports at most 64 owning Product units", ErrValidation)
	}
	nominal := map[string]InstancePose{}
	for _, v := range view.Product.Instances {
		nominal[v.ID] = InstancePose{Translation: v.Translation, Rotation: v.Rotation}
		s.Equations.Bodies = append(s.Equations.Bodies, geometry.AssemblyBody{ID: v.ID, Pose: geometry.AssemblyPose(nominal[v.ID])})
	}
	if req.AnalysisID != "" {
		i := slices.IndexFunc(view.Product.Kinematics.Analyses, func(a InterferenceAnalysis) bool { return a.ID == req.AnalysisID })
		if i < 0 {
			return s, fmt.Errorf("%w: unknown analysis", ErrValidation)
		}
		a := view.Product.Kinematics.Analyses[i]
		s.Analysis = &a
		req.Clearance, req.Scope, req.IncludeSameUnit = a.Clearance, a.Scope, a.IncludeSameUnit
		if a.StudyID == "" {
			req.CurrentOnly = true
		} else {
			req.StudyID = a.StudyID
		}
		s.CurrentOnly = req.CurrentOnly
	}
	if req.CurrentOnly {
		if len(req.Scope) > 128 {
			return s, fmt.Errorf("%w: DMU scope limit", ErrValidation)
		}
		gap, e := motionValue(req.Clearance, false)
		if e != nil || gap < 0 || gap > 1e6 {
			return s, fmt.Errorf("%w: invalid clearance", ErrValidation)
		}
		s.Study = MotionStudy{ID: "current", Name: "当前姿态 DMU", Frames: 1, BudgetMS: 30000, CheckDMU: true, Clearance: req.Clearance, Scope: req.Scope, IncludeSameUnit: req.IncludeSameUnit}
	} else {
		if req.Trial != nil && req.DriverID != "" {
			i := slices.IndexFunc(view.Product.Kinematics.Drivers, func(d MotionDriver) bool { return d.ID == req.DriverID })
			if i < 0 {
				return s, fmt.Errorf("%w: unknown trial driver", ErrValidation)
			}
			d := view.Product.Kinematics.Drivers[i]
			h := sha256.Sum256([]byte("motion-trial/" + d.ID))
			s.Study = MotionStudy{ID: hex.EncodeToString(h[:16]), Name: d.Name + " 试动", MechanismID: d.MechanismID, DriverID: d.ID, DurationSeconds: 1, Frames: 2, BudgetMS: 30000, Clearance: MotionQuantity{Unit: "mm"}}
		} else {
			i := slices.IndexFunc(view.Product.Kinematics.Studies, func(x MotionStudy) bool { return x.ID == req.StudyID })
			if i < 0 {
				return s, fmt.Errorf("%w: unknown study", ErrValidation)
			}
			s.Study = view.Product.Kinematics.Studies[i]
		}
		if s.Study.DriverID != "" {
			for _, d := range view.Product.Kinematics.Drivers {
				if d.ID == s.Study.DriverID && d.MechanismID == s.Study.MechanismID {
					s.Study.DriverJointID = d.JointID
				}
			}
		}
		if req.Trial != nil {
			s.Study.Start, s.Study.End = *req.Trial, *req.Trial
			s.Study.Frames = 2
			s.Study.CheckDMU = false
		}
		if req.AnalysisID != "" {
			s.Study.CheckDMU = true
			s.Study.Scope = req.Scope
			s.Study.Clearance = req.Clearance
			s.Study.IncludeSameUnit = req.IncludeSameUnit
		}
		for _, m := range view.Product.Kinematics.Mechanisms {
			if m.ID == s.Study.MechanismID {
				s.Mechanism = m
			}
		}
		if s.Mechanism.ID == "" {
			return s, fmt.Errorf("%w: unknown mechanism", ErrValidation)
		}
		selected := KinematicsDefinitions{Mechanisms: []Mechanism{s.Mechanism}, Studies: []MotionStudy{s.Study}}
		for _, d := range view.Product.Kinematics.Drivers {
			if d.ID == s.Study.DriverID {
				selected.Drivers = append(selected.Drivers, d)
			}
		}
		for _, id := range s.Mechanism.UnitIDs {
			if slices.IndexFunc(view.Product.Instances, func(v ProductInstance) bool { return v.ID == id }) < 0 {
				return s, fmt.Errorf("%w: selected motion unit no longer exists", ErrValidation)
			}
		}
		if err = validateKinematics(*view.Product, selected); err != nil {
			return s, err
		}
		if s.Mechanism, err = service.resolveMechanismGeometry(ctx, *view.Product, s.Mechanism, false); err != nil {
			return s, err
		}
		input, e := mechanismProductInput(*view.Product, s.Mechanism)
		if e != nil {
			return s, e
		}
		frozen, e := service.FreezeAssemblyInput(ctx, documentID, req.BaseRevisionID, input)
		if e != nil {
			return s, e
		}
		s.Equations, err = compileMechanism(frozen, s.Mechanism, s.Study)
		if err != nil {
			return s, err
		}
	}
	if s.Analysis != nil {
		s.Study.Name = s.Analysis.Name
	}
	if s.Study.CheckDMU {
		scope := append([]DMUAddress(nil), s.Study.Scope...)
		for i := range scope {
			scope[i].InstancePath.Segments = append([]InstancePathSegment(nil), scope[i].InstancePath.Segments...)
			if len(scope[i].InstancePath.Segments) > 0 && scope[i].InstancePath.Segments[0].OwnerDocumentID == documentID {
				scope[i].InstancePath.Segments[0].OwnerVersionID = req.BaseRevisionID
			}
		}
		found := make([]bool, len(scope))
		coveredUnits := map[string]bool{}
		for _, v := range view.ResolvedInstances {
			selected := len(scope) == 0
			for i, a := range scope {
				if a.BodyID == v.BodyID && validateResolvedInstancePath(a.InstancePath, v.InstancePath) == nil {
					found[i] = true
					selected = true
				}
			}
			if !selected {
				continue
			}
			if len(v.InstancePath.Segments) == 0 || v.BodyID == "" || v.GeometryKey == "" || v.DisplayFallback != nil {
				return s, fmt.Errorf("%w: DMU requires complete exact geometry for %s", ErrValidation, v.ID)
			}
			owner := v.InstancePath.Segments[0].InstanceID
			coveredUnits[owner] = true
			pose, ok := nominal[owner]
			if !ok {
				return s, fmt.Errorf("%w: missing rigid motion unit", ErrValidation)
			}
			object, e := service.representationObject(ctx, v.GeometryKey, "BREP")
			if e != nil {
				return s, fmt.Errorf("DMU exact B-Rep %s: %w", v.ID, e)
			}
			address := DMUAddress{InstancePath: v.InstancePath, BodyID: v.BodyID}
			encoded, _ := json.Marshal(address)
			h := sha256.Sum256(encoded)
			source, ok := view.Artifacts[v.GeometryKey]
			if !ok || source.GeometryID == "" {
				return s, fmt.Errorf("%w: missing geometry identity", ErrValidation)
			}
			s.GeometryUnits = append(s.GeometryUnits, MotionGeometryUnit{ID: hex.EncodeToString(h[:]), MotionUnitID: owner, Address: address, GeometryKey: v.GeometryKey, GeometryID: source.GeometryID, BRep: geometry.ArtifactReference{Backend: object.Backend, ObjectKey: object.Key, SHA256: object.SHA256, Size: object.Size, ContentType: object.ContentType}, RelativePose: inverseRelativePose(InstancePose{Translation: v.Translation, Rotation: v.Rotation}, pose)})
		}
		for _, ok := range found {
			if !ok {
				return s, fmt.Errorf("%w: selected occurrence/Body no longer resolves", ErrValidation)
			}
		}
		if len(scope) == 0 {
			for id := range nominal {
				if !coveredUnits[id] {
					return s, fmt.Errorf("%w: no exact geometry for owning unit %s; select an explicit scope", ErrValidation, id)
				}
			}
		}
		if len(s.GeometryUnits) < 2 || len(s.GeometryUnits) > 128 {
			return s, fmt.Errorf("%w: DMU requires 2..128 exact geometry instances", ErrValidation)
		}
		for i, a := range s.GeometryUnits {
			for _, b := range s.GeometryUnits[i+1:] {
				if a.MotionUnitID != b.MotionUnitID || s.Study.IncludeSameUnit {
					s.Pairs = append(s.Pairs, geometry.AnalysisPair{FirstID: a.ID, SecondID: b.ID})
				}
			}
		}
		if len(s.Pairs) == 0 || len(s.Pairs)*s.Study.Frames > 8192 {
			return s, fmt.Errorf("%w: DMU pair/frame budget", ErrValidation)
		}
	}
	s.Digest = motionDigest(s)
	b, e := json.Marshal(s)
	if e != nil || len(b) > 8<<20 {
		return s, fmt.Errorf("%w: snapshot size limit", ErrValidation)
	}
	// Read Head again: compilation/artifact resolution must belong to one accepted snapshot.
	var head string
	if e = service.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1`, documentID).Scan(&head); e != nil {
		return s, e
	}
	if head != req.BaseRevisionID {
		return s, fmt.Errorf("%w: Product revision changed during freeze", ErrValidation)
	}
	return s, nil
}

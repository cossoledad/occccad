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

const motionPolicy = "revolute-owning-constraints-v3"

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
type MotionSnapshot struct {
	Schema        int                            `json:"schema"`
	Policy        string                         `json:"policy"`
	Digest        string                         `json:"digest"`
	DocumentID    string                         `json:"documentId"`
	RevisionID    string                         `json:"revisionId"`
	View          DocumentView                   `json:"view"`
	Mechanism     Mechanism                      `json:"mechanism"`
	Study         MotionStudy                    `json:"study"`
	Equations     mechanismEquations             `json:"equations"`
	SolverProfile geometry.AssemblySolverProfile `json:"solverProfile"`
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
	s := MotionSnapshot{Schema: 1, Policy: motionPolicy, DocumentID: documentID, RevisionID: req.BaseRevisionID, View: view, SolverProfile: defaultAssemblySolverProfile()}
	if (req.DriverID != "" && (req.Trial == nil || req.StudyID != "")) || (req.Trial != nil && (req.CurrentOnly || req.AnalysisID != "")) {
		return s, fmt.Errorf("%w: ambiguous motion trial request", ErrValidation)
	}
	if len(view.Product.Instances) > 64 {
		return s, fmt.Errorf("%w: study supports at most 64 owning Product units", ErrValidation)
	}
	if req.CurrentOnly || req.AnalysisID != "" || len(req.Scope) > 0 || req.IncludeSameUnit || req.Clearance.Value != 0 || req.Trial != nil || req.DriverID != "" {
		return s, fmt.Errorf("%w: mechanism runs accept only a saved simulation", ErrValidation)
	}
	{
		i := slices.IndexFunc(view.Product.Kinematics.Studies, func(x MotionStudy) bool { return x.ID == req.StudyID })
		if i < 0 {
			return s, fmt.Errorf("%w: unknown simulation", ErrValidation)
		}
		s.Study = view.Product.Kinematics.Studies[i]
		if s.Study.CheckDMU {
			return s, fmt.Errorf("%w: interference is not a simulation dependency", ErrValidation)
		}
		if s.Study.DriverID != "" {
			for _, d := range view.Product.Kinematics.Drivers {
				if d.ID == s.Study.DriverID && d.MechanismID == s.Study.MechanismID {
					s.Study.DriverJointID = d.JointID
				}
			}
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
		poseModel, e := mechanismPoseModel(*view.Product, s.Mechanism)
		if e != nil {
			return s, e
		}
		if s.Mechanism, err = service.resolveMechanismGeometry(ctx, poseModel, s.Mechanism, false); err != nil {
			return s, err
		}
		input, e := mechanismProductInput(*view.Product, s.Mechanism)
		if e != nil {
			return s, e
		}
		appendMechanismConstraints(&input, s.Mechanism)
		frozen, e := service.FreezeAssemblyInput(ctx, documentID, req.BaseRevisionID, input)
		if e != nil {
			return s, e
		}
		s.Equations, err = compileMechanism(frozen, s.Mechanism, s.Study)
		if err != nil {
			return s, err
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

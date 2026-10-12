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

const motionPolicy = "revolute-joint-limits-recorded-path-v2"

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
	// A frozen path cannot keep the same cache identity across solver policy
	// changes, even when all user parameters and geometry remain identical.
	b, _ := json.Marshal(struct {
		Input             MotionSnapshot `json:"input"`
		SolverBuildPolicy string         `json:"solverBuildPolicy"`
	}{s, assemblySolverBuildPolicy})
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
		for _, d := range selected.Drivers {
			if d.Reference == nil {
				return s, fmt.Errorf("%w: command reference must be saved", ErrValidation)
			}
			if len(s.Study.Keyframes) > 0 && (s.Study.BasisDigest != recordingBasis(s.Mechanism, d) || s.Study.GeometryDigest != recordingGeometry(view, s.Mechanism)) {
				return s, fmt.Errorf("%w: SIMULATION_REVALIDATION_REQUIRED", ErrValidation)
			}
		}
		if len(s.Study.Keyframes) > 0 {
			s.Mechanism.Poses = s.Study.InitialPoses
			s.Mechanism.Coordinates = s.Study.InitialCoordinates
		}
		if err = service.compileMotionSnapshot(ctx, &s); err != nil {
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

func (service *Service) compileMotionSnapshot(ctx context.Context, s *MotionSnapshot) error {
	if len(s.Study.Keyframes) > 0 {
		product, err := cloneAssemblyProduct(*s.View.Product)
		if err != nil {
			return err
		}
		for id, pose := range s.Study.EnvironmentPoses {
			if !slices.ContainsFunc(product.Instances, func(i ProductInstance) bool { return i.ID == id }) || slices.Contains(s.Mechanism.UnitIDs, id) || !validMotionPose(pose) {
				return fmt.Errorf("%w: invalid frozen environment", ErrValidation)
			}
		}
		for i, v := range product.Instances {
			if pose, ok := s.Study.EnvironmentPoses[v.ID]; ok {
				if slices.Contains(s.Mechanism.UnitIDs, v.ID) || !validMotionPose(pose) {
					return fmt.Errorf("%w: invalid frozen environment", ErrValidation)
				}
				product.Instances[i].Translation, product.Instances[i].Rotation = pose.Translation, pose.Rotation
			}
		}
		s.View.Product = &product
	}
	poseModel, err := mechanismPoseModel(*s.View.Product, s.Mechanism)
	if err != nil {
		return err
	}
	if s.Mechanism, err = service.resolveMechanismGeometry(ctx, poseModel, s.Mechanism, false); err != nil {
		return err
	}
	input, err := mechanismProductInput(*s.View.Product, s.Mechanism)
	if err != nil {
		return err
	}
	appendMechanismConstraints(&input, s.Mechanism)
	frozen, err := service.FreezeAssemblyInput(ctx, s.DocumentID, s.RevisionID, input)
	if err != nil {
		return err
	}
	s.Equations, err = compileMechanism(frozen, s.Mechanism, s.Study)
	return err
}

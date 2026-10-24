package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/occccad/occccad/internal/database"
	"github.com/occccad/occccad/internal/geometry"
	"github.com/occccad/occccad/internal/modelcore"
)

const mechanismAnalysisPolicy = "mechanism-physical-v1/" + assemblySolverBuildPolicy + "/" + modelcore.TopologyNamingPolicyDigest

type MechanismSupportAnalysis struct {
	Role       string               `json:"role"`
	Reference  *AssemblyGeometryRef `json:"reference,omitempty"`
	Status     string               `json:"status"`
	ExactType  string               `json:"exactType,omitempty"`
	Diagnostic string               `json:"diagnostic,omitempty"`
}
type MechanismEndpointAnalysis struct {
	Side       string                     `json:"side"`
	InstanceID string                     `json:"instanceId"`
	Name       string                     `json:"name"`
	Status     string                     `json:"status"`
	Diagnostic string                     `json:"diagnostic,omitempty"`
	Supports   []MechanismSupportAnalysis `json:"supports"`
}
type MechanismJointAnalysis struct {
	Joint               MechanismJoint              `json:"joint"`
	Endpoints           []MechanismEndpointAnalysis `json:"endpoints"`
	Drivers             []MotionDriver              `json:"drivers"`
	SupportStatus       string                      `json:"supportStatus"`
	StructureStatus     string                      `json:"structureStatus"`
	ConfigurationStatus string                      `json:"configurationStatus"`
	Diagnostic          string                      `json:"diagnostic,omitempty"`
}
type MechanismAnalysis struct {
	DocumentID   string                      `json:"documentId"`
	RevisionID   string                      `json:"revisionId"`
	MechanismID  string                      `json:"mechanismId"`
	Name         string                      `json:"name"`
	InputDigest  string                      `json:"inputDigest"`
	PolicyDigest string                      `json:"policyDigest"`
	MemberIDs    []string                    `json:"memberIds"`
	Poses        map[string]InstancePose     `json:"poses"`
	GroundCount  int                         `json:"groundCount"`
	CommandCount int                         `json:"commandCount"`
	Joints       []MechanismJointAnalysis    `json:"joints"`
	Diagnostics  []string                    `json:"diagnostics"`
	Evidence     AssemblyEngineeringEvidence `json:"evidence"`
}

// Inspect exact saved definitions without activation, admission, capture, driver
// locks, pose adoption, warm-start promotion or any persisted analysis records.
func (s *Service) AnalyzeMechanism(ctx context.Context, doc, rev, id string, numerical bool) (out MechanismAnalysis, err error) {
	ctx, cancel := context.WithTimeout(withAssemblyReadCache(ctx), 10*time.Second)
	defer cancel()
	var rawModel []byte
	var head, kind string
	if err = s.database.QueryRow(ctx, `SELECT d.head_version_id::text,d.document_type,v.model_json FROM occccad.documents d JOIN occccad.document_versions v ON v.id=d.head_version_id WHERE d.id=$1 AND d.deleted_at IS NULL`, doc).Scan(&head, &kind, &rawModel); err != nil {
		if errors.Is(err, database.ErrNoRows) {
			err = ErrNotFound
		}
		return out, err
	}
	if head != rev {
		return out, fmt.Errorf("%w: MECHANISM_ANALYSIS_STALE", ErrValidation)
	}
	if kind != "PRODUCT" {
		return out, fmt.Errorf("%w: owning Product required", ErrValidation)
	}
	var model ProductModel
	if err = json.Unmarshal(rawModel, &model); err != nil {
		return out, err
	}
	var m Mechanism
	for _, v := range model.Kinematics.Mechanisms {
		if v.ID == id {
			m = v
			break
		}
	}
	if m.ID == "" {
		return out, ErrNotFound
	}
	out = MechanismAnalysis{DocumentID: doc, RevisionID: rev, MechanismID: id, Name: m.Name, PolicyDigest: mechanismAnalysisPolicy, MemberIDs: m.UnitIDs, Poses: m.Poses, Joints: []MechanismJointAnalysis{}, Diagnostics: []string{}, Evidence: AssemblyEngineeringEvidence{DocumentID: doc, RevisionID: rev, Source: "READ_ONLY_ANALYSIS", Status: "NOT_COMPUTED", SolverBuildPolicy: assemblySolverBuildPolicy, ScopeBodyIDs: m.UnitIDs, ReferenceSemantics: "STATIC_RELATIVE", CoordinateFrame: "OWNING_PRODUCT", LengthUnit: "mm", Components: []AssemblyEngineeringComponent{}}}
	raw, _ := json.Marshal(struct {
		Document, Revision, Policy string
		Model                      ProductModel
		Mechanism                  Mechanism
	}{doc, rev, mechanismAnalysisPolicy, model, m})
	digest := sha256.Sum256(raw)
	out.InputDigest = hex.EncodeToString(digest[:])
	out.Evidence.InputDigest = out.InputDigest
	for _, d := range model.Kinematics.Drivers {
		if d.MechanismID == id {
			out.CommandCount++
		}
	}
	input, poseErr := mechanismPoseModel(model, m)
	complete := poseErr == nil && len(m.UnitIDs) > 0 && len(m.Poses) == len(m.UnitIDs)
	if !complete {
		out.Diagnostics = append(out.Diagnostics, "MECHANISM_ACCEPTED_POSES_UNAVAILABLE")
	}
	// Environment instances are not members and must not acquire artificial Fixes.
	input.Instances = slices.DeleteFunc(append([]ProductInstance(nil), input.Instances...), func(i ProductInstance) bool { return !slices.Contains(m.UnitIDs, i.ID) })
	for _, member := range m.UnitIDs {
		if !slices.ContainsFunc(input.Instances, func(i ProductInstance) bool { return i.ID == member }) {
			complete = false
			out.Diagnostics = append(out.Diagnostics, "MISSING_MEMBER: "+member)
		}
	}
	if len(m.SupplementalConstraintIDs) > 0 {
		complete = false
		out.Diagnostics = append(out.Diagnostics, "LEGACY_PRODUCT_CONSTRAINT_SCOPE: 不混入 Product 约束")
	}
	posesComplete := complete
	for _, i := range input.Instances {
		out.Evidence.References = append(out.Evidence.References, AssemblyEngineeringReference{InstanceID: i.ID, DocumentID: i.ReferencedDocumentID, RevisionID: i.ReferencedVersionID})
	}
	resolver := newAssemblySupportResolver(ctx, s, &model)
	resolved := m
	resolved.Joints = nil
	for _, joint := range m.Joints {
		row := MechanismJointAnalysis{Joint: joint, SupportStatus: "VALID", StructureStatus: "VALID", ConfigurationStatus: "UNKNOWN", Endpoints: []MechanismEndpointAnalysis{}, Drivers: []MotionDriver{}}
		for _, c := range joint.Constraints {
			if c.Suppressed || c.Mode == "MEASURED" {
				row.StructureStatus = "INVALID"
				row.Diagnostic = "机构物理关系被停用或标为测量，完整关系证据不足"
				complete = false
			}
		}
		if joint.Kind == "GROUND" {
			out.GroundCount++
		}
		for _, d := range model.Kinematics.Drivers {
			if d.MechanismID == id && d.JointID == joint.ID {
				row.Drivers = append(row.Drivers, d)
			}
		}
		ends := []*JointEndpoint{&joint.First}
		if joint.Kind != "GROUND" {
			ends = append(ends, joint.Second)
		}
		for side, end := range ends {
			e := MechanismEndpointAnalysis{Side: []string{"first", "second"}[side], Status: "VALID", Supports: []MechanismSupportAnalysis{}}
			if end == nil {
				e.Status = "MISSING"
				e.Diagnostic = "接合端点缺失"
			} else {
				e.InstanceID = end.InstanceID
				for _, i := range model.Instances {
					if i.ID == end.InstanceID {
						e.Name = i.Name
					}
				}
				if !slices.ContainsFunc(model.Instances, func(i ProductInstance) bool { return i.ID == end.InstanceID }) || !slices.Contains(m.UnitIDs, end.InstanceID) {
					e.Status = "MISSING"
					e.Diagnostic = "机构成员 occurrence 不存在"
				}
				roles := []string{}
				switch joint.Kind {
				case "REVOLUTE", "PRISMATIC":
					roles = []string{"axis", "plane"}
				case "CYLINDRICAL":
					roles = []string{"axis"}
				case "PLANAR":
					roles = []string{"plane"}
				case "SPHERICAL":
					roles = []string{"point"}
				case "GROUND", "RIGID":
				default:
					e.Status = "UNSUPPORTED"
				}
				frameOnly := end.Axis == nil && end.Plane == nil && end.Point == nil
				for _, role := range roles {
					ref := map[string]*AssemblyGeometryRef{"axis": end.Axis, "plane": end.Plane, "point": end.Point}[role]
					a := MechanismSupportAnalysis{Role: role, Reference: ref, Status: "VALID"}
					if ref == nil {
						if frameOnly && validMotionPose(end.Frame) {
							a.Status = "EXPLICIT_FRAME"
						} else {
							a.Status = "MISSING"
							a.Diagnostic = "缺少几何支撑"
						}
					} else if ref.InstanceID != end.InstanceID {
						a.Status = "TYPE_MISMATCH"
						a.Diagnostic = "支撑与端点 occurrence 不一致"
					} else {
						value, resolveErr := resolver.resolve(*ref)
						if resolveErr != nil {
							if !errors.Is(resolveErr, ErrValidation) && !errors.Is(resolveErr, ErrNotFound) {
								return out, resolveErr
							}
							a.Status = "UNAVAILABLE"
							var failure *assemblySupportResolutionError
							if errors.As(resolveErr, &failure) {
								a.Status = failure.Status
							}
							if errors.Is(resolveErr, ErrNotFound) {
								a.Status = "MISSING"
							}
							a.Diagnostic = resolveErr.Error()
						} else {
							a.ExactType = value.Kind
							expected := map[string]string{"axis": "AXIS", "plane": "PLANE", "point": "POINT"}[role]
							if value.Kind != expected {
								a.Status = "TYPE_MISMATCH"
								a.Diagnostic = "需要 " + expected + "，得到 " + value.Kind
							}
						}
					}
					if a.Status != "VALID" && a.Status != "EXPLICIT_FRAME" {
						e.Status = "INVALID"
					}
					e.Supports = append(e.Supports, a)
				}
			}
			if e.Status != "VALID" {
				row.SupportStatus = "INVALID"
			}
			row.Endpoints = append(row.Endpoints, e)
		}
		if row.SupportStatus == "VALID" {
			one := m
			one.Joints = []MechanismJoint{joint}
			evaluated, resolveErr := s.resolveMechanismGeometry(ctx, model, one, false)
			if resolveErr != nil {
				if !errors.Is(resolveErr, ErrValidation) && !errors.Is(resolveErr, ErrNotFound) {
					return out, resolveErr
				}
				row.SupportStatus = "INVALID"
				row.Diagnostic = resolveErr.Error()
			} else {
				resolved.Joints = append(resolved.Joints, evaluated.Joints[0])
				if joint.Kind == "GROUND" {
					if len(joint.Constraints) != 1 || joint.Constraints[0].FixedPose == nil {
						row.ConfigurationStatus = "UNKNOWN"
						row.Diagnostic = "Ground capture unavailable"
						complete = false
					} else if !conflictPoseClose(geometry.AssemblyPose(m.Poses[joint.First.InstanceID]), geometry.AssemblyPose(*joint.Constraints[0].FixedPose), 1e-7, 1e-8) {
						row.ConfigurationStatus = "INVALID"
						row.Diagnostic = "Ground pose differs from saved capture"
					} else {
						row.ConfigurationStatus = "VALID"
					}
				}

				if posesComplete && joint.Kind != "GROUND" {
					if _, closureErr := verifyMechanismJoints(evaluated, m.Poses, m.Coordinates, nil); closureErr != nil {
						row.ConfigurationStatus = "INVALID"
						row.Diagnostic = closureErr.Error()
					} else {
						row.ConfigurationStatus = "VALID"
					}
				}
			}
		}
		if row.SupportStatus != "VALID" {
			complete = false
		}
		out.Joints = append(out.Joints, row)
	}
	if numerical {
		out.Evidence.Status = "UNKNOWN"
		if !complete {
			out.Evidence.DiagnosticCode = "MECHANISM_COMPLETE_INPUT_UNAVAILABLE"
		} else {
			appendMechanismConstraints(&input, resolved)
			capture := &assemblyDiagnosticCapture{invalid: map[string]bool{}}
			frozen, freezeErr := s.FreezeAssemblyInput(context.WithValue(ctx, assemblyDiagnosticCaptureKey{}, capture), doc, rev, input)
			if freezeErr != nil {
				if !errors.Is(freezeErr, ErrValidation) && !errors.Is(freezeErr, ErrNotFound) {
					return out, freezeErr
				}
				out.Evidence.DiagnosticCode = "MECHANISM_EQUATIONS_UNAVAILABLE"
				out.Evidence.Diagnostic = freezeErr.Error()
			} else if len(capture.failures) > 0 {
				out.Evidence.DiagnosticCode = "MECHANISM_EQUATIONS_UNAVAILABLE"
				for _, f := range capture.failures {
					out.Diagnostics = append(out.Diagnostics, f.ConstraintID+": "+f.Error)
				}
			} else {
				eq, compileErr := compileMechanismEquations(frozen, resolved, MotionStudy{}, false)
				if compileErr != nil {
					out.Evidence.DiagnosticCode = "MECHANISM_EQUATIONS_INCOMPLETE"
					out.Evidence.Diagnostic = compileErr.Error()
				} else {
					for _, c := range eq.Constraints {
						out.Evidence.ConstraintIDs = append(out.Evidence.ConstraintIDs, c.ID)
					}
					result, solveErr := s.worker.SolveAssemblyWithOptions(ctx, "mechanism-analysis/"+out.InputDigest, eq.Bodies, eq.Geometry, eq.Constraints, geometry.AssemblySolveOptions{SolverProfile: &frozen.SolverProfile, DisableConflictProbes: true})
					if solveErr != nil {
						return out, solveErr
					}
					engineeringResult(&out.Evidence, engineeringBodies(input), result, frozen.SolverProfile)
				}
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if err = s.database.QueryRow(ctx, `SELECT head_version_id::text FROM occccad.documents WHERE id=$1 AND deleted_at IS NULL`, doc).Scan(&head); err != nil {
		return out, err
	}
	if head != rev {
		return out, fmt.Errorf("%w: MECHANISM_ANALYSIS_STALE", ErrValidation)
	}
	return out, nil
}

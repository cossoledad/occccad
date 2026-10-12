package workspace

import "fmt"

const assemblyCapturedFramePolicy = "body-local-right-handed-frame-mm-v1"

func validateCapturedAssemblyFrame(ref AssemblyGeometryRef) error {
	c := ref.CapturedFrame
	if c == nil {
		return nil
	}
	if c.ID == "" || len(c.ID) > 128 || c.Policy != assemblyCapturedFramePolicy || c.LengthUnit != "mm" || !validMotionPose(c.Frame) || ref.InstanceID == "" || ref.SourceVersionID == "" ||
		(ref.Kind != "POINT" && ref.Kind != "AXIS" && ref.Kind != "PLANE") || ref.PersistentSelection != nil || ref.PublicationRef != nil || ref.CapturedDirection != nil || ref.DerivedRole != "" {
		return fmt.Errorf("%w: invalid constraint-owned captured frame", ErrValidation)
	}
	return nil
}
func capturedFrameReference(model ProductModel, end JointEndpoint, id, kind string, frame InstancePose) AssemblyGeometryRef {
	ref := AssemblyGeometryRef{InstanceID: end.InstanceID, Kind: kind, GeometryID: id, CapturedFrame: &AssemblyCapturedFrame{ID: id, Policy: assemblyCapturedFramePolicy, LengthUnit: "mm", Frame: frame}}
	for _, i := range model.Instances {
		if i.ID == end.InstanceID {
			ref.SourceVersionID = i.ReferencedVersionID
			if ref.SourceVersionID == "" {
				ref.SourceVersionID = i.ResolvedVersionID
			}
		}
	}
	return ref
}

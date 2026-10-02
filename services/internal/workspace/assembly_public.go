package workspace

import (
	"context"
	"fmt"
	"github.com/occccad/occccad/internal/assemblycontract"
	"github.com/occccad/occccad/internal/geometry"
	"math"
	"strings"
)

// Exact resolution and suitability for a new public definition are distinct.
// Keep the parameter-domain policy shared by command compilation and the
// read-only UI inspection; consumers must not invent their own angular cutoff.
func assemblySupportConstraintEligibility(value geometry.AssemblyGeometry, role string) (bool, string, string) {
	switch value.Kind {
	case "BODY", "POINT", "AXIS", "PLANE", "CIRCLE", "CYLINDER", "SPHERE", "CONE", "FRAME":
	default:
		return false, "ASSEMBLY_SUPPORT_CONTRACT_UNAVAILABLE", "exact support type has no public constraint contract"
	}
	if value.Kind == "CIRCLE" && value.ParameterStart != nil && value.ParameterEnd != nil && *value.ParameterEnd-*value.ParameterStart < 2*math.Pi-defaultAssemblySolverProfile().AngleTolerance && role != "underlying-circle" {
		return false, "ASSEMBLY_SUPPORT_REQUIRES_UNDERLYING_CIRCLE", "trimmed Arc requires explicit underlying-circle support"
	}
	return true, "", ""
}

// Compile only new commands against exact, accepted-version descriptors. Reads
// and frozen replay retain their recorded primitives and never reinterpret them.
func (service *Service) validatePublicAssemblySupports(ctx context.Context, model *ProductModel, c *AssemblyConstraint) error {
	if c.DefinitionVersion != 2 {
		return fmt.Errorf("%w: unsupported assembly definition version", ErrValidation)
	}
	if c.Kind == "FIX" || c.Kind == "FIX_TOGETHER" {
		return nil
	}
	if c.Second == nil {
		return fmt.Errorf("%w: binary definition missing second support", ErrValidation)
	}
	r := newAssemblySupportResolver(ctx, service, model)
	a, err := r.resolve(c.First)
	if err != nil {
		return err
	}
	b, err := r.resolve(*c.Second)
	if err != nil {
		return err
	}
	for _, support := range []struct {
		value geometry.AssemblyGeometry
		role  string
	}{{a, c.First.DerivedRole}, {b, c.Second.DerivedRole}} {
		if eligible, _, diagnostic := assemblySupportConstraintEligibility(support.value, support.role); !eligible {
			return fmt.Errorf("%w: %s", ErrValidation, diagnostic)
		}
	}
	pointSurface := (a.Kind == "POINT" && (b.Kind == "CYLINDER" || b.Kind == "SPHERE" || b.Kind == "CONE")) || (b.Kind == "POINT" && (a.Kind == "CYLINDER" || a.Kind == "SPHERE" || a.Kind == "CONE"))
	if c.Family == "Coincidence" && pointSurface && c.Subtype == "" {
		c.Subtype = "point-surface"
	}
	if c.Subtype == "" && (c.Family == "Coincidence" || c.Family == "Offset") {
		for _, capability := range assemblycontract.ForFamily(c.Family) {
			if capability.Family == c.Family && publicCapabilityPair(capability, a.Kind, b.Kind) {
				c.Subtype = capability.Subtype
				break
			}
		}
	}
	if c.Family == "Coincidence" || c.Family == "Offset" {
		valid := false
		for _, capability := range assemblycontract.ForFamily(c.Family) {
			valid = valid || (capability.Family == c.Family && capability.Subtype == c.Subtype && publicCapabilityPair(capability, a.Kind, b.Kind))
		}
		if !valid {
			return fmt.Errorf("%w: subtype does not match exact supports", ErrValidation)
		}
	}
	if c.Family != "Contact" && !publicAssemblyPair(c.Family, c.AngleRelation, a.Kind, b.Kind) {
		return fmt.Errorf("%w: unsupported exact %s combination %s/%s", ErrValidation, c.Family, a.Kind, b.Kind)
	}
	if c.Kind == "CONCENTRIC" {
		axis := func(kind string) bool { return kind == "AXIS" || kind == "CYLINDER" }
		if !axis(a.Kind) || !axis(b.Kind) {
			return fmt.Errorf("%w: coaxial shortcut requires explicit axial supports", ErrValidation)
		}
	}
	if c.Family == "Contact" && !publicContactPair(c.ContactKind, a.Kind, b.Kind) {
		return fmt.Errorf("%w: unsupported exact Contact combination %s/%s/%s", ErrValidation, a.Kind, b.Kind, c.ContactKind)
	}
	if c.Family == "Offset" {
		valid := func(k string) bool { return k == "POINT" || k == "AXIS" || k == "PLANE" }
		if !valid(a.Kind) || !valid(b.Kind) {
			return fmt.Errorf("%w: Offset requires explicit Point/Line/Plane supports; select a derived axis for a curved face", ErrValidation)
		}
	}
	if c.Family == "Angle" {
		valid := func(k string) bool { return k == "AXIS" || k == "PLANE" || k == "CYLINDER" }
		if !valid(a.Kind) || !valid(b.Kind) {
			return fmt.Errorf("%w: Angle requires explicit directional support", ErrValidation)
		}
	}
	return nil
}

func publicAssemblyPair(family, relation, first, second string) bool {
	for _, capability := range assemblycontract.ForFamily(family) {
		if capability.Family != family || len(capability.Roles) != 2 ||
			(family == "Angle" && capability.Subtype != relation) {
			continue
		}
		if publicCapabilityPair(capability, first, second) {
			return true
		}
	}
	return false
}

func publicCapabilityPair(capability assemblycontract.Capability, first, second string) bool {
	if len(capability.Roles) != 2 {
		return false
	}
	accepts := func(role assemblycontract.Role, exact string) bool {
		if role.Descriptor == exact {
			return true
		}
		for _, kind := range role.SupportedDescriptors {
			if kind == exact {
				return true
			}
		}
		return false
	}
	a, b := capability.Roles[0], capability.Roles[1]
	return (accepts(a, first) && accepts(b, second)) || (accepts(a, second) && accepts(b, first))
}

// canonicalAssemblyDefinition is the public-to-numerical compilation boundary.
// Old immutable revisions are not canonicalized on read. Only commands write v2.
func canonicalAssemblyDefinition(c *AssemblyConstraint, family, subtype string) error {
	if family == "" {
		switch c.Kind {
		case "COINCIDENT", "CONCENTRIC":
			family = "Coincidence"
		case "DISTANCE", "OFFSET":
			family = "Offset"
		case "ANGLE", "PARALLEL", "PERPENDICULAR":
			family = "Angle"
		case "FIX":
			family = "Fix"
		case "CONTACT":
			family = "Contact"
		case "FIX_TOGETHER":
			family = "FixTogether"
		case "RIGID":
			return fmt.Errorf("%w: persisted pair Rigid is unsupported; use Fix Together", ErrValidation)
		default:
			return fmt.Errorf("%w: unknown assembly family", ErrValidation)
		}
	}
	switch family {
	case "Coincidence":
		if c.Kind != "COINCIDENT" && c.Kind != "CONCENTRIC" {
			return fmt.Errorf("%w: coincidence primitive mismatch", ErrValidation)
		}
	case "Offset":
		c.Kind = "DISTANCE"
	case "Angle":
		if c.Kind == "PARALLEL" || c.Kind == "PERPENDICULAR" {
			c.AngleRelation = c.Kind
		}
		if subtype != "" {
			c.AngleRelation = strings.ToUpper(subtype)
		}
		c.Kind = "ANGLE"
		if c.AngleRelation == "" {
			c.AngleRelation = "FREE"
		}
		subtype = c.AngleRelation
	case "Fix":
		c.Kind = "FIX"
		if subtype != "" {
			c.FixMode = strings.ToUpper(subtype)
		}
		if c.FixMode == "" {
			c.FixMode = "SPACE"
		}
		subtype = c.FixMode
	case "Contact":
		c.Kind = "CONTACT"
		if subtype != "" {
			c.ContactKind = strings.ToUpper(subtype)
		}
		if c.ContactSide == "" {
			c.ContactSide = "EXTERNAL"
		}
		if c.ContactBranch == 0 {
			c.ContactBranch = 1
		}
		subtype = c.ContactKind
	case "FixTogether":
		c.Kind = "FIX_TOGETHER"
	default:
		return fmt.Errorf("%w: unknown assembly family %q", ErrValidation, family)
	}
	c.DefinitionVersion, c.Family, c.Subtype = 2, family, subtype
	return nil
}

// Geometry combinations come from the single production contract, not picking
// categories, a second matrix, or generated test PASS counts.
func publicContactPair(kind, first, second string) bool {
	for _, c := range assemblycontract.ForFamily("Contact") {
		if c.Family != "Contact" || len(c.Roles) != 2 || !strings.HasSuffix(c.Subtype, "-"+strings.ToLower(kind)) {
			continue
		}
		a, b := c.Roles[0].Descriptor, c.Roles[1].Descriptor
		if (a == first && b == second) || (a == second && b == first) {
			return true
		}
	}
	return false
}

func validateContactDefinition(c AssemblyConstraint) error {
	if c.Kind != "CONTACT" {
		return nil
	}
	if c.ContactKind != "FACE" && c.ContactKind != "LINE" && c.ContactKind != "POINT" && c.ContactKind != "RING" {
		return fmt.Errorf("%w: contact branch kind required", ErrValidation)
	}
	if c.ContactSide != "EXTERNAL" && c.ContactSide != "INTERNAL" {
		return fmt.Errorf("%w: invalid contact material branch", ErrValidation)
	}
	if c.ContactBranch != 1 && c.ContactBranch != -1 {
		return fmt.Errorf("%w: contact branch must be +/-1", ErrValidation)
	}
	return nil
}

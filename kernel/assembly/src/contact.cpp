#include "contact.hpp"

#include <unsupported/Eigen/AutoDiff>
#include <Eigen/Geometry>
#include <algorithm>
#include <cmath>
#include <stdexcept>
#include <utility>

namespace occccad::assembly::analytic_contact {
namespace {
constexpr double pi = 3.1415926535897932384626433832795;
using Derivative = Eigen::Matrix<double, 12, 1>;
using Scalar = Eigen::AutoDiffScalar<Derivative>;
using Vector = Eigen::Matrix<Scalar, 3, 1>;

Scalar scalar(double x) { return Scalar(x, Derivative::Zero()); }
Vector variable(const Eigen::Vector3d& x, int offset) {
    Vector out;
    for (int i = 0; i < 3; ++i) {
        out[i] = scalar(x[i]);
        out[i].derivatives()[offset + i] = 1;
    }
    return out;
}
Vector unit(const Vector& x) {
    using std::sqrt;
    const Scalar length = sqrt(x.dot(x));
    return x / length;
}
std::string validate(const Support& s) {
    if (!s.origin.allFinite() || !s.axis.allFinite()) return "CONTACT_NONFINITE_SUPPORT";
    if (s.material_side != 1 && s.material_side != -1) return "CONTACT_INVALID_MATERIAL_SIDE";
    if (s.kind != SupportKind::Sphere && std::abs(s.axis.norm()-1.0) > 1e-10)
        return "CONTACT_NONUNIT_DIRECTION";
    if ((s.kind == SupportKind::Cylinder || s.kind == SupportKind::Sphere ||
         s.kind == SupportKind::Circle) && (!std::isfinite(s.radius) || s.radius <= 0))
        return "CONTACT_INVALID_RADIUS";
    if (s.kind == SupportKind::Cone &&
        (!std::isfinite(s.half_angle) || s.half_angle <= 0 || s.half_angle >= pi/2 ||
         (s.cone_leaf != 1 && s.cone_leaf != -1))) return "CONTACT_INVALID_CONE";
    return {};
}
bool close_dimension(double a, double b) {
    // Descriptor compatibility gate, not the geometric solve tolerance.
    return std::abs(a-b) <= 1e-12*std::max({1.0, std::abs(a), std::abs(b)});
}
}  // namespace

Evaluation evaluate(const Definition& definition, const Support& first, const Support& second) {
    Evaluation out;
    for (const auto* s : {&first, &second}) {
        out.diagnostic = validate(*s);
        if (!out.diagnostic.empty()) return out;
    }
    if (definition.branch != 1 && definition.branch != -1) {
        out.diagnostic = "CONTACT_INVALID_BRANCH";
        return out;
    }
    // Canonical pair order is a local evaluation detail, never a persistent swap.
    const bool swapped = static_cast<int>(first.kind) > static_cast<int>(second.kind);
    const Support& a = swapped ? second : first;
    const Support& b = swapped ? first : second;
    const int ai = swapped ? 6 : 0, bi = swapped ? 0 : 6;
    const Vector p = variable(a.origin, ai), q = variable(b.origin, bi);
    const Vector u = variable(a.axis, ai+3), v = variable(b.axis, bi+3);
    const Vector delta = q-p;
    std::vector<Scalar> rows;
    auto append = [&](const Scalar& value, std::string name, bool angular = false) {
        rows.push_back(value);
        out.rowKinds.push_back(std::move(name));
        out.angularRows.push_back(angular);
    };
    auto appendVector = [&](const Vector& value, const std::string& name, bool angular = false) {
        for (int i = 0; i < 3; ++i) append(value[i], name+"_"+std::to_string(i), angular);
    };
    auto fail = [&](std::string diagnostic) {
        out.diagnostic = std::move(diagnostic);
        return out;
    };
    const bool external = definition.side == ContactSide::External;
    const bool effectiveExternal = external == (a.material_side*b.material_side > 0);
    const int branch = definition.branch;
    const int material = a.material_side*b.material_side;
    if (a.kind == SupportKind::Plane && b.kind == SupportKind::Plane &&
        definition.kind == ContactKind::Face) {
        const int orientation = external ? -1 : 1;
        appendVector(u-scalar(orientation)*v, "FACE_NORMAL", true);
        append(delta.dot(u), "FACE_OFFSET");
        out.generalRank = 3;
    } else if (a.kind == SupportKind::Plane && b.kind == SupportKind::Cylinder &&
               definition.kind == ContactKind::Line) {
        if (branch != (external ? b.material_side : -b.material_side))
            return fail("CONTACT_PLANE_MATERIAL_BRANCH_MISMATCH");
        append(u.dot(v), "CYLINDER_PLANE_DIRECTION", true);
        append(delta.dot(u)-scalar(branch*b.radius), "CYLINDER_PLANE_TANGENCY");
        out.generalRank = 2;
    } else if (a.kind == SupportKind::Plane && b.kind == SupportKind::Sphere &&
               definition.kind == ContactKind::Point) {
        if (branch != (external ? b.material_side : -b.material_side))
            return fail("CONTACT_PLANE_MATERIAL_BRANCH_MISMATCH");
        append(delta.dot(u)-scalar(branch*b.radius), "SPHERE_PLANE_TANGENCY");
        out.generalRank = 1;
    } else if (a.kind == SupportKind::Cylinder && b.kind == SupportKind::Cylinder) {
        // Axis signs are parameterization, not cylinder material orientation.
        const int orientation = a.axis.dot(b.axis) >= 0 ? 1 : -1;
        appendVector(u-scalar(orientation)*v, "CYLINDER_AXIS_DIRECTION", true);
        const Vector radial = delta-u*delta.dot(u);
        if (definition.kind == ContactKind::Face) {
            if (!close_dimension(a.radius,b.radius)) return fail("CONTACT_FACE_RADIUS_MISMATCH");
            if (material != (external ? -1 : 1)) return fail("CONTACT_MATERIAL_SIDE_MISMATCH");
            appendVector(radial, "CYLINDER_FACE_OFFSET");
            out.generalRank = 4;
        } else if (definition.kind == ContactKind::Line) {
            const double distance = effectiveExternal ? a.radius+b.radius : std::abs(a.radius-b.radius);
            if (distance == 0) return fail("CONTACT_LINE_DEGENERATES_TO_FACE");
            using std::sqrt;
            // At concentric initial poses this norm has no unique derivative.
            // Explicit failure lets the solver establish a branch-specific seed.
            if (radial.dot(radial).value() == 0) return fail("CONTACT_RADIAL_BRANCH_DEGENERATE");
            append(sqrt(radial.dot(radial))-scalar(distance), "CYLINDER_LINE_TANGENCY");
            out.generalRank = 3;
        } else return fail("CONTACT_UNSUPPORTED_BRANCH");
    } else if (a.kind == SupportKind::Sphere && b.kind == SupportKind::Sphere &&
               definition.kind == ContactKind::Face) {
        if (!close_dimension(a.radius,b.radius)) return fail("CONTACT_FACE_RADIUS_MISMATCH");
        if (material != (external ? -1 : 1)) return fail("CONTACT_MATERIAL_SIDE_MISMATCH");
        appendVector(delta, "SPHERE_FACE_CENTER");
        out.generalRank = 3;
    } else if (a.kind == SupportKind::Sphere && b.kind == SupportKind::Cone &&
               definition.kind == ContactKind::Ring) {
        if (material != (external ? -1 : 1)) return fail("CONTACT_MATERIAL_SIDE_MISMATCH");
        const Vector axis = scalar(b.cone_leaf)*v;
        const Vector center = p-q;
        appendVector(center-scalar(a.radius/std::sin(b.half_angle))*axis, "SPHERE_CONE_RING_POSITION");
        out.generalRank = 3;
    } else if (a.kind == SupportKind::Sphere && b.kind == SupportKind::Circle &&
               definition.kind == ContactKind::Ring) {
        if (b.radius > a.radius) return fail("CONTACT_CIRCLE_EXCEEDS_SPHERE");
        const double height = std::sqrt((a.radius-b.radius)*(a.radius+b.radius));
        appendVector(delta-scalar(branch*height)*v, "SPHERE_CIRCLE_RING_POSITION");
        out.generalRank = 3;
    } else if (a.kind == SupportKind::Cone && b.kind == SupportKind::Cone) {
        const Vector aa = scalar(a.cone_leaf)*u, bb = scalar(b.cone_leaf)*v;
        if (definition.kind == ContactKind::Face) {
            if (!close_dimension(a.half_angle,b.half_angle)) return fail("CONTACT_FACE_ANGLE_MISMATCH");
            if (material != (external ? -1 : 1)) return fail("CONTACT_MATERIAL_SIDE_MISMATCH");
            appendVector(aa-bb, "CONE_FACE_AXIS", true);
            appendVector(delta, "CONE_FACE_APEX");
            out.generalRank = 5;
        } else if (definition.kind == ContactKind::Line) {
            const double beta = effectiveExternal ? a.half_angle+b.half_angle : std::abs(a.half_angle-b.half_angle);
            if (beta == 0) return fail("CONTACT_LINE_DEGENERATES_TO_FACE");
            const Scalar cosine = aa.dot(bb);
            const Scalar denominator = scalar(1)-cosine*cosine;
            if (denominator.value() <= 1e-20) return fail("CONTACT_CONE_GENERATOR_DEGENERATE");
            // Common generator is the unique axis-plane solution g.Ai=cos(alpha_i).
            // Normalize off-manifold; at target angle it already has unit length.
            const Scalar x = (scalar(std::cos(a.half_angle))-cosine*scalar(std::cos(b.half_angle)))/denominator;
            const Scalar y = (scalar(std::cos(b.half_angle))-cosine*scalar(std::cos(a.half_angle)))/denominator;
            const Vector g = unit(x*aa+y*bb);
            using std::atan2;
            using std::sqrt;
            const Vector axisCross=aa.cross(bb);
            append(atan2(sqrt(axisCross.dot(axisCross)),cosine)-scalar(beta), "CONE_LINE_AXIS_ANGLE", true);
            appendVector(delta.cross(g), "CONE_LINE_GENERATOR_OFFSET");
            out.generalRank = 3;
        } else return fail("CONTACT_UNSUPPORTED_BRANCH");
    } else if (a.kind == SupportKind::Cone && b.kind == SupportKind::Circle &&
               definition.kind == ContactKind::Ring) {
        const Vector axis = scalar(a.cone_leaf)*u;
        const int orientation = (a.cone_leaf*a.axis).dot(b.axis) >= 0 ? 1 : -1;
        appendVector(axis-scalar(orientation)*v, "CONE_CIRCLE_AXIS", true);
        appendVector(delta-scalar(b.radius/std::tan(a.half_angle))*axis, "CONE_CIRCLE_RING_POSITION");
        out.generalRank = 5;
    } else return fail("CONTACT_UNSUPPORTED_COMBINATION");
    out.residual.resize(static_cast<Eigen::Index>(rows.size()));
    out.derivatives.resize(static_cast<Eigen::Index>(rows.size()),12);
    for (std::size_t i = 0; i < rows.size(); ++i) {
        out.residual[static_cast<Eigen::Index>(i)] = rows[i].value();
        out.derivatives.row(static_cast<Eigen::Index>(i)) = rows[i].derivatives().transpose();
    }
    if (!out.residual.allFinite() || !out.derivatives.allFinite()) return fail("CONTACT_NONFINITE_EVALUATION");
    out.valid = true;
    out.diagnostic.clear();
    return out;
}
}  // namespace occccad::assembly::analytic_contact

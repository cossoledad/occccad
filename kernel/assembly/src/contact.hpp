#pragma once

#include <Eigen/Core>
#include <string>
#include <vector>

namespace occccad::assembly::analytic_contact {

enum class SupportKind { Plane, Cylinder, Sphere, Cone, Circle };
enum class ContactKind { Face, Line, Point, Ring };
enum class ContactSide { External, Internal };

// Frozen owning-Product values: lengths in mm, half_angle in radians.
// Cone axis is oriented; cone_leaf selects the single mathematical nappe.
// Curved-support material_side multiplies the canonical analytic outward normal.
// Plane.axis IS its material outer normal; Plane.material_side is provenance
// only and MUST NOT be applied again. Circle is a support, not a solid surface.
struct Support {
    SupportKind kind{SupportKind::Plane};
    Eigen::Vector3d origin{Eigen::Vector3d::Zero()};
    Eigen::Vector3d axis{Eigen::Vector3d::UnitZ()};
    double radius{1.0};
    double half_angle{0.5};
    int cone_leaf{1};
    int material_side{1};
};

struct Definition {
    ContactKind kind{ContactKind::Face};
    ContactSide side{ContactSide::External};
    int branch{1};
};

struct Evaluation {
    bool valid{};
    std::string diagnostic;
    Eigen::VectorXd residual;
    // Columns: first.origin, first.axis, second.origin, second.axis.
    // These are value derivatives; the caller owns the SE(3) chain rule.
    Eigen::Matrix<double, Eigen::Dynamic, 12> derivatives;
    std::vector<std::string> rowKinds;
    std::vector<bool> angularRows;
    int generalRank{};
};

Evaluation evaluate(const Definition&, const Support& first, const Support& second);

}  // namespace occccad::assembly::analytic_contact

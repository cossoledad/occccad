#pragma once

#include <planegcs/GCS.h>

#include <cmath>

namespace occccad::geometry::sketch {
// Reflection permits a zero source-to-image chord for points on the axis.
// PlaneGCS's ordinary perpendicular primitive normalizes both line lengths,
// which is undefined for that legal self-mapping case. Keep its exact residual
// and analytic gradient, normalizing only the (nondegenerate) reflection axis.
class ReflectionNormalConstraint final : public GCS::ConstraintPerpendicular {
public:
    ReflectionNormalConstraint(GCS::Point& first, GCS::Point& second, GCS::Line& axis)
        : GCS::ConstraintPerpendicular(first, second, axis.p1, axis.p2) {
        rescale();
    }
    void rescale(double coefficient = 1.0) override {
        const double dx = *pvec[4] - *pvec[6], dy = *pvec[5] - *pvec[7];
        scale = coefficient / std::hypot(dx, dy);
    }
};

}  // namespace occccad::geometry::sketch

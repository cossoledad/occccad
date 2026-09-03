#include <occccad/assembly/solver.hpp>

#include <chrono>
#include <cstddef>
#include <iostream>
#include <string>

namespace {

occccad::assembly::Model plane_chain(const std::size_t bodies) {
    using namespace occccad::assembly;
    Model model;
    model.bodies.reserve(bodies);
    model.geometry.reserve(bodies);
    model.constraints.reserve(bodies);
    for (std::size_t index = 0; index < bodies; ++index) {
        const std::string id = "body-" + std::to_string(index);
        model.bodies.push_back({id, {{0.0, 0.0, static_cast<double>(index)}, {}}});
        model.geometry.push_back({"plane", id, PlaneGeometry{}});
        if (index == 0) {
            Constraint fixed;
            fixed.id = "fix";
            fixed.kind = ConstraintKind::Fix;
            fixed.first = {id, {}};
            model.constraints.push_back(fixed);
        } else {
            Constraint coincidence;
            coincidence.id = "coincident-" + std::to_string(index);
            coincidence.kind = ConstraintKind::Coincident;
            coincidence.first = {id, "plane"};
            coincidence.second = GeometryRef{"body-" + std::to_string(index - 1), "plane"};
            coincidence.direction_relation = DirectionRelation::Same;
            model.constraints.push_back(coincidence);
        }
    }
    return model;
}

}  // namespace

int main() {
    using namespace occccad::assembly;
    for (const std::size_t bodies : {5U, 15U, 30U}) {
        const Model model = plane_chain(bodies);
        SolverOptions options;
        options.verify_analytic_jacobians = false;
        constexpr std::size_t samples = 3;
        const auto start = std::chrono::steady_clock::now();
        SolveResult result;
        for (std::size_t sample = 0; sample < samples; ++sample)
            result = Solver{}.solve(model, options);
        const auto elapsed = std::chrono::duration_cast<std::chrono::nanoseconds>(
                                 std::chrono::steady_clock::now() - start)
                                 .count();
        if (result.status != SolveStatus::Converged)
            return 1;
        std::cout << "AssemblyPlaneChain" << bodies << " " << samples << " "
                  << elapsed / static_cast<long long>(samples) << " ns/op\n";
    }
}

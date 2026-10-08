#include <occccad/assembly/solver.hpp>

#include <sys/resource.h>

#include <chrono>
#include <cmath>
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

int main(int argc, char** argv) {
    using namespace occccad::assembly;
    for (const std::size_t bodies : {5U, 15U, 30U}) {
        Model model = plane_chain(bodies);
        if (argc > 2) {
            for (auto& geometry : model.geometry)
                geometry.id = std::string(8192, 'x') + geometry.id;
            for (auto& c : model.constraints)
                if (c.kind != ConstraintKind::Fix) {
                    c.first.geometry_id = std::string(8192, 'x') + c.first.geometry_id;
                    c.second->geometry_id = c.first.geometry_id;
                }
        }
        SolverOptions options;
        options.verify_analytic_jacobians = false;
        options.solve_intent = SolveIntent{{"body-" + std::to_string(bodies-1)}, {"body-1"},
                                          SolvePreferencePolicy::MoveFirstMinimizeReference};
        const std::size_t samples = argc > 1 ? std::stoul(argv[1]) : 3;
        if (samples < 1 || samples > 1000) return 2;
        const auto start = std::chrono::steady_clock::now();
        SolveResult result;
        for (std::size_t sample = 0; sample < samples; ++sample)
            result = Solver{}.solve(model, options);
        const auto elapsed = std::chrono::duration_cast<std::chrono::nanoseconds>(
                                 std::chrono::steady_clock::now() - start)
                                 .count();
        if (result.status != SolveStatus::Converged || result.components.front().preference.status != PreferenceStatus::Converged)
            return 1;
        std::cout << "AssemblyPlaneChain" << bodies << " " << samples << " "
                  << elapsed / static_cast<long long>(samples)
                  << " ns/op iterations=" << result.iterations
                  << " preference_iterations=" << result.components.front().preference.iterations
                  << " residual=" << result.normalized_residual << " reference_objective="
                  << result.components.front().preference.reference_objective
                  << " reference_translation="
                  << result.components.front().preference.bodies[1].translation
                  << " residual_ms=" << result.metrics.residual_ms
                  << " jacobian_ms=" << result.metrics.jacobian_ms
                  << " compile_ms=" << result.metrics.input_compile_ms
                  << " hard_ms=" << result.metrics.hard_feasibility_ms
                  << " preference_ms=" << result.metrics.preference_ms
                  << " retraction_ms=" << result.metrics.feasibility_retraction_ms
                  << " dof_ms=" << result.metrics.dof_analysis_ms
                  << " redundancy_ms=" << result.metrics.redundancy_ms
                  << " factorization_ms=" << result.metrics.factorization_ms
                  << " factorizations=" << result.metrics.factorizations
                  << " kernel_cache_hits=" << result.metrics.physical_kernel_cache_hits
                  << " bfgs_ms=" << result.metrics.bfgs_update_ms
                  << " bfgs_updates=" << result.metrics.bfgs_updates
                  << " pose_ms=" << result.metrics.pose_build_ms
                  << " components=" << result.components.size()
                  << " rank=" << result.components.front().jacobian_rank
                  << " valid=true configuration=optimized"
                  << " evaluations=" << result.metrics.residual_evaluations << "/"
                  << result.metrics.jacobian_evaluations
                  << " hot_string_lookups=" << result.metrics.hot_string_lookups
                  << " hot_string_bytes=" << result.metrics.hot_string_bytes;
        rusage usage{};
        getrusage(RUSAGE_SELF, &usage);
        std::cout << " max_rss_kib=" << usage.ru_maxrss << "\n";
    }
    // Existing rotation-objective regression, repeated over independent bodies:
    // actually exercise BFGS instead of attributing unexecuted updates to chains.
    Model rotated;
    for (int i = 0; i < 30; ++i) {
        Body b;
        b.id = "rotation-" + std::to_string(i);
        b.initial_pose.rotation = {0, 0, std::sin(.3), std::cos(.3)};
        b.initial_guess = Pose{{}, {std::sin(.5), 0, 0, std::cos(.5)}};
        rotated.bodies.push_back(b);
    }
    SolverOptions options;
    options.verify_analytic_jacobians = false;
    const auto samples = argc > 1 ? std::stoul(argv[1]) : 3;
    for (std::size_t sample = 0; sample < samples; ++sample) {
        const auto start = std::chrono::steady_clock::now();
        const auto r = Solver{}.solve(rotated, options);
        for (const auto& c : r.components)
            if (c.preference.status != PreferenceStatus::Converged || c.preference.total_objective > 1e-12) return 1;
        std::cout << "AssemblyWarmRotation30 sample=" << sample + 1 << " ms="
                  << std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start).count()
                  << " components=" << r.components.size() << " bfgs_ms=" << r.metrics.bfgs_update_ms
                  << " bfgs_updates=" << r.metrics.bfgs_updates << " preference_ms=" << r.metrics.preference_ms
                  << " valid=true\n";
    }
}

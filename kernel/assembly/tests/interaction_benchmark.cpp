#include <occccad/assembly/solver.hpp>

#include <Eigen/Geometry>
#include <algorithm>
#include <chrono>
#include <cmath>
#include <iostream>
#include <map>
#include <string>
#include <thread>
using namespace occccad::assembly;
namespace {
Constraint relation(std::string id, ConstraintKind kind, GeometryRef a, GeometryRef b) {
    Constraint c;
    c.id = std::move(id);
    c.kind = kind;
    c.first = std::move(a);
    c.second = std::move(b);
    c.direction_relation = DirectionRelation::Same;
    return c;
}
Model connected() {
    Model m;
    for (int i = 0; i < 50; ++i) {
        const auto id = "b" + std::to_string(i);
        m.bodies.push_back({id, {{10.0 * i, 0, 0}, {}}});
        m.geometry.push_back({"p", id, PointGeometry{{-10.0 * i, 0, 0}}});
        m.geometry.push_back({"x", id, AxisGeometry{{}, {1, 0, 0}}});
        m.geometry.push_back({"z", id, AxisGeometry{}});
        if (i)
            for (const auto& type : {std::string("p"), std::string("x"), std::string("z")})
                m.constraints.push_back(
                    relation("chain/" + std::to_string(i) + "/" + type,
                             type == "p" ? ConstraintKind::Coincident : ConstraintKind::Parallel,
                             {id, type}, {"b" + std::to_string(i - 1), type}));
    }
    // Explicit loop-closing definitions. Some are dependent: record physical rank
    // and all 200 active definitions, rather than pretending 200 independent rows.
    for (int j = 0; j < 53; ++j) {
        int a = 1 + j % 49;
        int b = (j * 7) % a;
        m.constraints.push_back(relation("loop/" + std::to_string(j), ConstraintKind::Coincident,
                                         {"b" + std::to_string(a), "p"},
                                         {"b" + std::to_string(b), "p"}));
    }
    return m;
}
Model free_scene(int n) {
    Model m;
    for (int i = 0; i < n; ++i)
        m.bodies.push_back({"b" + std::to_string(i), {{10.0 * i, 0, 0}, {}}});
    return m;
}
Model group_contact() {
    Model m;
    m.bodies = {
        {"b0", {{0, 0, 2}, {}}}, {"b1", {{3, 0, 2}, {}}}, {"b2", {{6, 0, 2}, {}}}, {"ground", {}}};
    m.geometry = {{"plane", "ground", PlaneGeometry{}}, {"sphere", "b0", SphereGeometry{{}, 2}}};
    m.constraints = {
        relation("group/01", ConstraintKind::Rigid, {"b0", {}}, {"b1", {}}),
        relation("group/12", ConstraintKind::Rigid, {"b1", {}}, {"b2", {}}),
        relation("contact", ConstraintKind::Contact, {"ground", "plane"}, {"b0", "sphere"})};
    m.constraints[0].fixed_pose = Pose{{-3, 0, 0}, {}};
    m.constraints[1].fixed_pose = Pose{{-3, 0, 0}, {}};
    m.constraints.back().contact_kind = ContactKind::Point;
    Constraint f;
    f.id = "ground";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    m.constraints.push_back(f);
    return m;
}
Eigen::Vector3d world_point(const Pose& p, const PointGeometry& g) {
    return Eigen::Quaterniond(p.rotation.w, p.rotation.x, p.rotation.y, p.rotation.z) *
               Eigen::Vector3d(g.position.x, g.position.y, g.position.z) +
           Eigen::Vector3d(p.translation.x, p.translation.y, p.translation.z);
}
bool independently_valid(const std::string& name, const Model&, const SolveResult& r) {
    std::map<std::string, Pose> poses;
    for (const auto& b : r.bodies)
        poses[b.id] = b.pose;
    if (name == "connected50-200") {
        const auto origin = world_point(poses.at("b0"), PointGeometry{});
        for (int i = 0; i < 50; ++i)
            if ((world_point(poses.at("b" + std::to_string(i)), PointGeometry{{-10.0 * i, 0, 0}}) -
                 origin)
                    .norm() > 1e-7)
                return false;
    }
    if (name == "group-contact") {
        if (std::abs(poses.at("b0").translation.z - 2) > 1e-7)
            return false;
        for (int i = 1; i < 3; ++i) {
            const auto& a = poses.at("b0");
            const auto& b = poses.at("b" + std::to_string(i));
            if (std::abs(std::hypot(std::hypot(a.translation.x - b.translation.x,
                                               a.translation.y - b.translation.y),
                                    a.translation.z - b.translation.z) -
                         3 * i) > 1e-7)
                return false;
        }
    }
    return true;
}
}  // namespace
int main(int argc, char** argv) {
    const int samples = argc > 1 ? std::stoi(argv[1]) : 5;
    if (samples < 1 || samples > 1000)
        return 2;
    const std::string filter = argc > 2 ? argv[2] : "";
    const std::vector<std::pair<std::string, Model>> scenes = {{"single", free_scene(1)},
                                                               {"connected50-200", connected()},
                                                               {"independent50", free_scene(50)},
                                                               {"group-contact", group_contact()}};
    bool selected = false;
    for (const auto& scene : scenes) {
        if (!filter.empty() && filter != scene.first)
            continue;
        selected = true;
        auto m = scene.second;
        SolverOptions o;
        o.verify_analytic_jacobians = false;
        o.max_conflict_probes = 0;
        o.affected_body_ids = {"b0"};
        DragTarget d;
        d.body_id = "b0";
        d.target_pose = m.bodies[0].initial_pose;
        o.drag_target = d;
        std::vector<double> elapsed;
        std::size_t solves = 0;
        SolveResult r;
        for (int sample = 0; sample < samples; ++sample) {
            o.drag_target->target_pose.translation.x = 0.1 * (sample + 1);
            o.drag_target->target_sequence = sample + 1;
            auto start = std::chrono::steady_clock::now();
            r = Solver{}.solve(m, o);
            elapsed.push_back(
                std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start)
                    .count());
            ++solves;
            const bool valid = r.interaction && r.interaction->eligible_for_commit &&
                               independently_valid(scene.first, m, r);
            std::cout << "{\"kind\":\"sample\",\"scene\":\"" << scene.first
                      << "\",\"sample\":" << sample + 1 << ",\"kernel_ms\":" << elapsed.back()
                      << ",\"qualified\":" << (valid ? "true" : "false") << "}" << std::endl;
            if (!valid)
                return 1;
            for (auto& body : m.bodies)
                for (const auto& solved : r.bodies)
                    if (body.id == solved.id)
                        body.initial_guess = solved.pose;
        }
        std::sort(elapsed.begin(), elapsed.end());
        std::size_t active = 0, rank = 0, updated = 0;
        for (const auto& c : m.constraints)
            if (c.mode == ConstraintMode::Driving || c.mode == ConstraintMode::Controlled)
                ++active;
        for (const auto& c : r.components) {
            rank += c.jacobian_rank;
            if (c.solved)
                ++updated;
        }
#ifdef NDEBUG
        const char* configuration = "Release";
#else
        const char* configuration = "Debug";
#endif
        std::cout << "{\"kind\":\"summary\",\"scene\":\"" << scene.first
                  << "\",\"configuration\":\"" << configuration
                  << "\",\"hardware_threads\":" << std::thread::hardware_concurrency()
                  << ",\"bodies\":" << m.bodies.size()
                  << ",\"constraints\":" << m.constraints.size()
                  << ",\"active_constraints\":" << active
                  << ",\"components\":" << r.components.size()
                  << ",\"updated_components\":" << updated << ",\"physical_rank\":" << rank
                  << ",\"samples\":" << samples << ",\"solve_calls\":" << solves
                  << ",\"kernel_p50_ms\":" << elapsed[(elapsed.size() - 1) / 2]
                  << ",\"kernel_p95_ms\":"
                  << elapsed[static_cast<std::size_t>(std::ceil(0.95 * elapsed.size())) - 1]
                  << ",\"hard_feasible\":true,\"transport_measured\":false,\"rendering_measured\":"
                     "false}\n";
    }
    return selected ? 0 : 2;
}

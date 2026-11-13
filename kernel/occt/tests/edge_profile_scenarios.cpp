#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_Sewing.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeSphere.hxx>
#include <BRepTools.hxx>
#include <Geom_BezierSurface.hxx>
#include <TColgp_Array2OfPnt.hxx>
#include <TopExp.hxx>
#include <TopoDS.hxx>
#include <gp_Pln.hxx>

#include <internal/edge_profile.hpp>
#include <internal/occt_kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <gtest/gtest.h>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <limits>
#include <map>
#include <sstream>

namespace occccad::kernel {
namespace {
double dot(const Vec3& a, const Vec3& b) {
    return a.x * b.x + a.y * b.y + a.z * b.z;
}
double weight(double c) {
    const auto lo = std::cos(35 * 3.141592653589793 / 180),
               hi = std::cos(5 * 3.141592653589793 / 180);
    const double t = std::clamp((c - lo) / (hi - lo), 0.0, 1.0);
    return 1 - t * t * (3 - 2 * t);
}
TopoDS_Shape sewn(bool varying) {
    const auto plane =
        BRepBuilderAPI_MakeFace(gp_Pln(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1)), -10, 10, -10, 0).Face();
    TopoDS_Face other;
    if (varying) {
        TColgp_Array2OfPnt poles(1, 2, 1, 2);
        poles(1, 1) = gp_Pnt(-10, 0, 0);
        poles(2, 1) = gp_Pnt(10, 0, 0);
        poles(1, 2) = gp_Pnt(-10, 10, 0);
        poles(2, 2) = gp_Pnt(10, 10, 14);
        other = BRepBuilderAPI_MakeFace(new Geom_BezierSurface(poles), 1e-7).Face();
    } else
        other = BRepBuilderAPI_MakeFace(gp_Pln(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1)), -10, 10, 0, 10)
                    .Face();
    BRepBuilderAPI_Sewing sewing(1e-7);
    sewing.Add(plane);
    sewing.Add(other);
    sewing.Perform();
    return sewing.SewedShape();
}
std::vector<std::pair<std::string, TopoDS_Shape>> fixtures() {
    const auto cube = BRepPrimAPI_MakeBox(20, 20, 20).Shape();
    BRepFilletAPI_MakeFillet fillet(cube);
    TopTools_IndexedMapOfShape edges;
    TopExp::MapShapes(cube, TopAbs_EDGE, edges);
    for (int i = 1; i <= edges.Extent(); ++i) {
        fillet.Add(3, TopoDS::Edge(edges(i)));
    }
    fillet.Build();
    if (!fillet.IsDone())
        throw std::runtime_error("fixture fillet failed");
    gp_Trsf pose;
    pose.SetRotation(gp_Ax1(gp_Pnt(0, 0, 0), gp_Dir(1, 2, 3)), .73);
    pose.SetTranslationPart(gp_Vec(12, -8, 4));
    gp_Trsf mirror;
    mirror.SetMirror(gp_Ax2(gp_Pnt(0, 0, 0), gp_Dir(1, 0, 0)));
    return {
        {"cube", cube},
        {"fillet", fillet.Shape()},
        {"cylinder", BRepPrimAPI_MakeCylinder(10, 20).Shape()},
        {"sphere", BRepPrimAPI_MakeSphere(10).Shape()},
        {"coplanar", sewn(false)},
        {"varying", sewn(true)},
        {"open", BRepBuilderAPI_MakeFace(gp_Pln(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1)), -10, 10, -10, 10)
                     .Face()},
        {"rotated", BRepBuilderAPI_Transform(cube, pose, true).Shape()},
        {"mirrored", BRepBuilderAPI_Transform(cube, mirror, true).Shape()},
        {"small", BRepPrimAPI_MakeBox(.02, .02, .02).Shape()},
        {"large", BRepPrimAPI_MakeBox(20000, 20000, 20000).Shape()}};
}
}  // namespace
TEST(EdgeProfile, ExactAdjacencyNormalsAndAcceptanceFixtures) {
    const char* output = std::getenv("OCCCCAD_EDGE_FIXTURES");
    std::ofstream report;
    if (output) {
        std::filesystem::create_directories(output);
        report.open(std::filesystem::path(output) / "geometry.json");
        report << "[";
    }
    bool first = true;
    for (const auto& [name, shape] : fixtures()) {
        SCOPED_TRACE(name);
        std::ostringstream stream;
        BRepTools::Write(shape, stream);
        const auto text = stream.str();
        OcctKernel kernel;
        const auto id = kernel.loadBrepr({text.begin(), text.end()});
        const auto before = kernel.serializeBrepr(id);
        const auto start = std::chrono::steady_clock::now();
        const auto mesh = kernel.tessellate(id);
        const auto elapsed =
            std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start)
                .count();
        ASSERT_FALSE(mesh.edge_profile.empty());
        EXPECT_EQ(kernel.serializeBrepr(id), before);
        std::map<EdgeDisplayKind, size_t> kinds;
        size_t tangent = 0, sharp = 0;
        for (const auto& segment : mesh.edge_profile) {
            ++kinds[segment.kind];
            ASSERT_GT(segment.local_id, 0U);
            for (const auto* p : {&segment.first, &segment.second}) {
                EXPECT_TRUE(std::isfinite(p->point.x + p->point.y + p->point.z));
                if (segment.kind == EdgeDisplayKind::manifold ||
                    segment.kind == EdgeDisplayKind::seam ||
                    segment.kind == EdgeDisplayKind::coplanar) {
                    EXPECT_NEAR(dot(p->normal_a, p->normal_a), 1, 1e-8);
                    EXPECT_NEAR(dot(p->normal_b, p->normal_b), 1, 1e-8);
                    const auto cosine = dot(p->normal_a, p->normal_b);
                    if (cosine > .99999)
                        ++tangent;
                    if (cosine < .1)
                        ++sharp;
                }
            }
        }
        if (name == "cube" || name == "rotated" || name == "mirrored" || name == "small" ||
            name == "large") {
            EXPECT_EQ(kinds[EdgeDisplayKind::manifold], 12U);
            EXPECT_EQ(sharp, 24U);
        }
        if (name == "fillet") {
            EXPECT_GT(tangent, 0U);
            EXPECT_EQ(sharp, 0U);
        }
        if (name == "cylinder" || name == "sphere") {
            EXPECT_GT(kinds[EdgeDisplayKind::seam], 0U);
            for (const auto& s : mesh.edge_profile)
                if (s.kind == EdgeDisplayKind::seam) {
                    EXPECT_EQ(s.quality, 0U);
                    EXPECT_GT(dot(s.first.normal_a, s.first.normal_b), .99999);
                }
        }
        if (name == "coplanar") {
            EXPECT_GT(kinds[EdgeDisplayKind::coplanar], 0U);
        }
        if (name == "open") {
            EXPECT_EQ(kinds.size(), 1U);
            EXPECT_EQ(kinds[EdgeDisplayKind::boundary], mesh.edge_profile.size());
        }
        if (name == "varying") {
            EXPECT_GT(kinds[EdgeDisplayKind::manifold], 2U);
            double maxError = 0;
            size_t checked = 0;
            for (const auto& s : mesh.edge_profile)
                if (s.kind == EdgeDisplayKind::manifold) {
                    EXPECT_EQ(s.quality, 0U);
                    for (int i = 0; i <= 64; ++i) {
                        const auto t = i / 64.0,
                                   x = s.first.point.x * (1 - t) + s.second.point.x * t,
                                   k = (x + 10) / 20 * 1.4;
                        // The GPU evaluates endpoint weights, then interpolates the
                        // varying along a rendered chord (not interpolated normals).
                        const auto actual =
                            weight(dot(s.first.normal_a, s.first.normal_b)) * (1 - t) +
                            weight(dot(s.second.normal_a, s.second.normal_b)) * t;
                        maxError =
                            std::max(maxError, std::abs(actual - weight(1 / std::sqrt(1 + k * k))));
                        ++checked;
                    }
                }
            EXPECT_GT(checked, 100U);
            EXPECT_LT(maxError, .025);
        }
        const auto encode = std::chrono::steady_clock::now();
        const auto glb = make_glb(mesh);
        const double encoding =
            std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - encode)
                .count();
        auto uniform = mesh;
        uniform.edge_profile.clear();
        const auto old = make_glb(uniform);
        if (output) {
            const auto dir = std::filesystem::path(output);
            std::ofstream b(dir / (name + ".brep"), std::ios::binary);
            b.write(text.data(), text.size());
            std::ofstream g(dir / (name + ".glb"), std::ios::binary);
            g.write(reinterpret_cast<const char*>(glb.data()), glb.size());
            if (!first) {
                report << ',';
            }
            first = false;
            report << "{\"name\":\"" << name << "\",\"tessellationMs\":" << elapsed
                   << ",\"normalMs\":" << mesh.timings.edge_profile_ms
                   << ",\"encodingMs\":" << encoding
                   << ",\"normalEvaluations\":" << mesh.timings.edge_normal_evaluations
                   << ",\"segments\":" << mesh.edge_profile.size()
                   << ",\"samplingLimits\":" << mesh.timings.edge_sampling_limits
                   << ",\"triangles\":" << mesh.triangles.size()
                   << ",\"uniformBytes\":" << old.size() << ",\"profileBytes\":" << glb.size()
                   << "}";
        }
    }
    if (output)
        report << "]";
}

TEST(EdgeProfile, MissingEvidenceDegeneracyAndCancellationStayConservative) {
    TessellationResult mesh;
    const auto shape = BRepPrimAPI_MakeBox(1, 1, 1).Shape();
    TopTools_IndexedMapOfShape edges;
    TopExp::MapShapes(shape, TopAbs_EDGE, edges);
    const auto edge = TopoDS::Edge(edges(1));
    const std::vector<Vec3> points{{0, 0, 0}, {0, 0, 0}, {1, 0, 0}};
    detail::append_edge_profile(mesh, 1, edge, points, {});
    ASSERT_EQ(mesh.edge_profile.size(), 1U);
    EXPECT_EQ(mesh.edge_profile.front().kind, EdgeDisplayKind::wire);
    auto use = detail::VisualEdgeUse{};
    use.points = {{0, 0, 0}, {1, 0, 0}};
    use.face_id = 1;
    auto other = use;
    other.face_id = 2;
    mesh.edge_profile.clear();
    detail::append_edge_profile(mesh, 1, edge, use.points, {use, other});
    ASSERT_EQ(mesh.edge_profile.size(), 1U);
    EXPECT_EQ(mesh.edge_profile.front().kind, EdgeDisplayKind::manifold);
    EXPECT_EQ(mesh.edge_profile.front().quality & 1, 1U);
    TopTools_IndexedMapOfShape sphere_faces;
    TopExp::MapShapes(BRepPrimAPI_MakeSphere(1).Shape(), TopAbs_FACE, sphere_faces);
    use.surface = std::make_shared<BRepAdaptor_Surface>(TopoDS::Face(sphere_faces(1)));
    use.uv = {gp_Pnt2d(std::numeric_limits<double>::quiet_NaN(), 0),
              gp_Pnt2d(std::numeric_limits<double>::quiet_NaN(), 0)};
    mesh.edge_profile.clear();
    detail::append_edge_profile(mesh, 1, edge, use.points, {use, other});
    ASSERT_EQ(mesh.edge_profile.size(), 1U);
    EXPECT_EQ(mesh.edge_profile.front().quality & 1, 1U);
    EXPECT_TRUE(std::isfinite(
        dot(mesh.edge_profile.front().first.normal_a, mesh.edge_profile.front().first.normal_a)));
    use.plane_normal = {0, 0, 1};
    other.plane_normal = {0, 0, -1};
    // Supply no plane adaptor: use three neighbors to exercise non-manifold
    // fallback without pretending that antiparallel normals are coplanar.
    mesh.edge_profile.clear();
    detail::append_edge_profile(mesh, 1, edge, use.points, {use, other, use});
    ASSERT_EQ(mesh.edge_profile.size(), 1U);
    EXPECT_EQ(mesh.edge_profile.front().kind, EdgeDisplayKind::non_manifold);
    EXPECT_THROW(detail::append_edge_profile(mesh, 1, edge, use.points, {},
                                             [] { throw std::runtime_error("cancelled"); }),
                 std::runtime_error);
}
}  // namespace occccad::kernel

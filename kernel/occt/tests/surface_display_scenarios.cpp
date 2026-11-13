#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <Poly_Triangulation.hxx>
#include <internal/surface_normals.hpp>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeSphere.hxx>
#include <BRepTools.hxx>
#include <Geom_BezierSurface.hxx>
#include <TColgp_Array2OfPnt.hxx>
#include <TopExp.hxx>
#include <TopoDS.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <gtest/gtest.h>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <tuple>

namespace occccad::kernel {
namespace {
TopoDS_Shape display_patch(bool saddle) {
    TColgp_Array2OfPnt poles(1, 3, 1, 3);
    for (int u = 1; u <= 3; ++u)
        for (int v = 1; v <= 3; ++v) {
            const double z = saddle ? ((u == 2 ? 0 : 6) - (v == 2 ? 0 : 6)) : (u == 2 ? 8 : 0);
            poles(u, v) = gp_Pnt((u - 2) * 30, (v - 1) * 20, z);
        }
    return BRepBuilderAPI_MakeFace(new Geom_BezierSurface(poles), 1e-7).Face();
}
std::vector<std::pair<std::string, TopoDS_Shape>> display_fixtures() {
    const auto cube = BRepPrimAPI_MakeBox(20, 20, 20).Shape();
    BRepFilletAPI_MakeFillet round(cube);
    TopTools_IndexedMapOfShape edges;
    TopExp::MapShapes(cube, TopAbs_EDGE, edges);
    for (int i = 1; i <= edges.Extent(); ++i)
        round.Add(3, TopoDS::Edge(edges(i)));
    round.Build();
    if (!round.IsDone())
        throw std::runtime_error("display fillet fixture failed");
    return {{"cube", cube},
            {"fillet", round.Shape()},
            {"sphere", BRepPrimAPI_MakeSphere(10).Shape()},
            {"cylinder", BRepPrimAPI_MakeCylinder(10, 20).Shape()},
            {"shallow", display_patch(false)},
            {"saddle", display_patch(true)}};
}
void write_display_file(const std::filesystem::path& directory, const std::string& name,
                        const TessellationResult& mesh) {
    const auto data = make_glb(mesh);
    std::ofstream file(directory / (name + ".glb"), std::ios::binary);
    file.write(reinterpret_cast<const char*>(data.data()), data.size());
}
}  // namespace
TEST(SurfaceDisplay, MeshingQualityAndImmutableFixtures) {
    const char* output = std::getenv("OCCCCAD_VISUAL_STUDIO_DIR");
    std::ofstream report;
    if (output) {
        std::filesystem::create_directories(output);
        report.open(std::filesystem::path(output) / "geometry.json");
        report << '[';
    }
    bool first = true;
    for (const auto& [name, shape] : display_fixtures()) {
        SCOPED_TRACE(name);
        std::ostringstream stream;
        BRepTools::Write(shape, stream);
        const auto text = stream.str();
        OcctKernel kernel;
        const auto id = kernel.loadBrepr({text.begin(), text.end()});
        const auto before = kernel.serializeBrepr(id);
        for (const auto& [quality, linear, angular] :
             std::vector<std::tuple<std::string, double, double>>{
                 {"fine", .02, .12}, {"balanced", .05, .18}, {"fast", .08, .24}}) {
            const auto start = std::chrono::steady_clock::now();
            auto mesh = kernel.tessellate(id, linear, angular, false);
            const auto ms =
                std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start)
                    .count();
            ASSERT_FALSE(mesh.triangles.empty());
            ASSERT_EQ(mesh.normals.size(), mesh.vertices.size());
            EXPECT_EQ(kernel.serializeBrepr(id), before);
            double error = 0;
            size_t checked = 0;
            for (size_t i = 0; i < mesh.vertices.size(); ++i) {
                const auto& p = mesh.vertices[i];
                const auto& n = mesh.normals[i];
                EXPECT_TRUE(std::isfinite(n.x + n.y + n.z));
                EXPECT_NEAR(std::hypot(n.x, n.y, n.z), 1, 1e-6);
                Vec3 expected;
                bool known = false;
                if (name == "sphere") {
                    const double length = std::hypot(p.x, p.y, p.z);
                    expected = {p.x / length, p.y / length, p.z / length};
                    known = true;
                }
                if (name == "shallow") {
                    const double slope = -8 * p.x / 900, length = std::hypot(slope, 1);
                    expected = {-slope / length, 0, 1 / length};
                    known = true;
                }
                if (known) {
                    error = std::max(
                        error,
                        std::acos(std::clamp(n.x * expected.x + n.y * expected.y + n.z * expected.z,
                                             -1.0, 1.0)) *
                            180 / 3.141592653589793);
                    ++checked;
                }
            }
            if (name == "sphere" || name == "shallow") {
                EXPECT_EQ(checked, mesh.vertices.size());
                EXPECT_LT(error, .001) << "surface normals must not follow coarse triangle averages";
                EXPECT_EQ(mesh.timings.surface_normal_fallbacks, 0u);
            }
            if (output) {
                const auto dir = std::filesystem::path(output);
                write_display_file(dir, name + "-" + quality, mesh);
                if (quality == "fine") {
                    auto legacy = mesh;
                    compute_mesh_normals(legacy);
                    write_display_file(dir, name + "-legacy", legacy);
                    std::ofstream brep(dir / (name + ".brep"), std::ios::binary);
                    brep.write(text.data(), text.size());
                }
                if (!first)
                    report << ',';
                first = false;
                report << "{\"name\":\"" << name << "\",\"quality\":\"" << quality
                       << "\",\"linearMm\":" << linear << ",\"angularRad\":" << angular
                       << ",\"triangles\":" << mesh.triangles.size()
                       << ",\"vertices\":" << mesh.vertices.size()
                       << ",\"bytes\":" << make_glb(mesh).size() << ",\"tessellationMs\":" << ms
                       << ",\"normalMs\":" << mesh.timings.normals_ms
                       << ",\"normalChecked\":" << checked << ",\"normalMaxErrorDeg\":" << error
                       << '}';
            }
        }
    }
    if (output)
        report << ']';
}
TEST(SurfaceDisplay, LocatedReversedNormalsAndUnavailableUVFallback) {
    auto face=TopoDS::Face(display_patch(false));
    gp_Trsf rotation;rotation.SetRotation(gp_Ax1(gp_Pnt(),gp_Dir(1,0,0)),.7);
    face=TopoDS::Face(face.Located(TopLoc_Location(rotation)).Reversed());
    Handle(Poly_Triangulation) tri=new Poly_Triangulation(3,1,true);
    for(int i=1;i<=3;++i){tri->SetNode(i,gp_Pnt());tri->SetUVNode(i,gp_Pnt2d(.5,.5));}
    std::vector<Vec3> normals;VisualStageTimings timings;
    detail::append_surface_normals(face,tri,normals,timings,{});
    auto expected=gp_Dir(0,0,-1);expected.Transform(rotation);
    ASSERT_EQ(normals.size(),3u);
    for(const auto& n:normals){EXPECT_NEAR(n.x,expected.X(),1e-7);EXPECT_NEAR(n.y,expected.Y(),1e-7);EXPECT_NEAR(n.z,expected.Z(),1e-7);}
    EXPECT_EQ(timings.surface_normal_fallbacks,0u);
    Handle(Poly_Triangulation) no_uv=new Poly_Triangulation(3,1,false);
    normals.clear();timings={};
    detail::append_surface_normals(face,no_uv,normals,timings,{});
    EXPECT_EQ(timings.surface_normal_fallbacks,3u);EXPECT_EQ(timings.surface_normal_evaluations,0u);
    for(const auto& n:normals)EXPECT_EQ(n.x*n.x+n.y*n.y+n.z*n.z,0);
    EXPECT_THROW(detail::append_surface_normals(face,tri,normals,timings,[]{throw std::runtime_error("cancel");}),std::runtime_error);
}
TEST(SurfaceDisplay, ScaledGeometryKeepsOutwardUnitNormals) {
    for(const auto scale:{.001,1000.}){
        gp_Trsf transform;transform.SetScale(gp_Pnt(),scale);
        const auto shape=BRepBuilderAPI_Transform(BRepPrimAPI_MakeSphere(10).Shape(),transform,true).Shape();
        std::ostringstream stream;BRepTools::Write(shape,stream);const auto text=stream.str();
        OcctKernel kernel;const auto id=kernel.loadBrepr({text.begin(),text.end()});const auto mesh=kernel.tessellate(id,.05*scale,.18);
        for(size_t i=0;i<mesh.vertices.size();++i){const auto& p=mesh.vertices[i];const auto& n=mesh.normals[i];
            EXPECT_NEAR(std::hypot(n.x,n.y,n.z),1,1e-6);EXPECT_GT((p.x*n.x+p.y*n.y+p.z*n.z)/std::hypot(p.x,p.y,p.z),.999999)<<"scale="<<scale<<" vertex="<<i;
        }
    }
}
}  // namespace occccad::kernel

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/topology_naming.hpp>
#include <gtest/gtest.h>
#include <BRepPrimAPI_MakeCone.hxx>
#include <BRepPrimAPI_MakeSphere.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepTools.hxx>
#include <gp_Circ.hxx>
#include <gp_Trsf.hxx>
#include <algorithm>
#include <cmath>
#include <sstream>
#include <filesystem>
#include <fstream>
#include <cstdlib>

namespace occccad::kernel {
namespace {
GeometryId import_shape(OcctKernel& kernel, const TopoDS_Shape& shape) {
    std::ostringstream out;
    BRepTools::Write(shape, out);
    const auto bytes = out.str();
    return kernel.loadBrepr({bytes.begin(), bytes.end()});
}
const TopologyProperty& property(const std::vector<TopologyProperty>& properties,
                                 const std::string& name) {
    const auto found = std::find_if(properties.begin(), properties.end(),
                                    [&](const auto& p) { return p.name == name; });
    if (found == properties.end()) throw std::runtime_error("missing exact property: " + name);
    return *found;
}
TEST(AssemblyExactSupport, SphereFreezesMillimetresAndMaterialSideAfterLocatedBrep) {
    OcctKernel kernel;
    gp_Trsf placement;
    placement.SetTranslation(gp_Vec(23, -41, 17));
    const auto shape = BRepBuilderAPI_Transform(BRepPrimAPI_MakeSphere(7.5).Shape(), placement, true).Shape();
    const auto id = import_shape(kernel, shape);
    const auto& faces = kernel.getTopology(id).faces;
    const auto sphere = std::find_if(faces.begin(), faces.end(), [](const auto& face) { return face.surface_type == 3; });
    ASSERT_NE(sphere, faces.end());
    const auto center = property(sphere->properties, "center").vector_value;
    EXPECT_DOUBLE_EQ(center.x, 23); EXPECT_DOUBLE_EQ(center.y, -41); EXPECT_DOUBLE_EQ(center.z, 17);
    EXPECT_DOUBLE_EQ(property(sphere->properties, "radius").number_value, 7.5);
    EXPECT_EQ(property(sphere->properties, "lengthUnit").text_value, "mm");
    EXPECT_EQ(property(sphere->properties, "materialSide").integer_value, 1);
    const auto reversed = import_shape(kernel, shape.Reversed());
    for (const auto& face : kernel.getTopology(reversed).faces) {
        if (face.surface_type == 3) { EXPECT_EQ(property(face.properties, "materialSide").integer_value, -1); }
    }
}
TEST(AssemblyExactSupport, ManipulatorHintsComeFromActualBoundaries) {
    OcctKernel kernel;
    const auto box=kernel.getTopology(import_shape(kernel,BRepPrimAPI_MakeBox(gp_Pnt(13,17,19),8,4,2).Shape()));
    for(const auto& face:box.faces){
        const auto center=property(face.properties,"snapCenter").vector_value;
        const auto direction=property(face.properties,"snapBoundaryDirection").vector_value;
        const auto normal=property(face.properties,"normal").vector_value;
        EXPECT_GE(center.x,13);EXPECT_GE(center.y,17);EXPECT_GE(center.z,19);
        EXPECT_NEAR(direction.x*direction.x+direction.y*direction.y+direction.z*direction.z,1,1e-12);
        EXPECT_NEAR(direction.x*normal.x+direction.y*normal.y+direction.z*normal.z,0,1e-12);
    }
    const auto cylinder=kernel.getTopology(import_shape(kernel,BRepPrimAPI_MakeCylinder(gp_Ax2(gp_Pnt(13,17,19),gp_Dir(0,1,0)),6,12).Shape()));
    const auto wall=std::find_if(cylinder.faces.begin(),cylinder.faces.end(),[](const auto& face){return face.surface_type==1;});
    ASSERT_NE(wall,cylinder.faces.end());
    const auto first=property(wall->properties,"snapEndFirst").vector_value;
    const auto last=property(wall->properties,"snapEndLast").vector_value;
    EXPECT_NEAR(first.x,13,1e-12);EXPECT_NEAR(first.y,17,1e-12);EXPECT_NEAR(first.z,19,1e-12);
    EXPECT_NEAR(last.x,13,1e-12);EXPECT_NEAR(last.y,29,1e-12);EXPECT_NEAR(last.z,19,1e-12);
}
TEST(AssemblyExactSupport, ConeApexAxisLeafAndHalfAngleAreIndependentOfFaceOrigin) {
    OcctKernel kernel;
    const gp_Ax2 frame(gp_Pnt(11, -8, 3), gp_Dir(0, 1, 0), gp_Dir(1, 0, 0));
    const auto id = import_shape(kernel, BRepPrimAPI_MakeCone(frame, 9, 3, 12).Shape());
    const auto& faces = kernel.getTopology(id).faces;
    const auto cone = std::find_if(faces.begin(), faces.end(), [](const auto& face) { return face.surface_type == 2; });
    ASSERT_NE(cone, faces.end());
    const auto apex = property(cone->properties, "apex").vector_value;
    EXPECT_NEAR(apex.x, 11, 1e-10); EXPECT_NEAR(apex.y, 10, 1e-10); EXPECT_NEAR(apex.z, 3, 1e-10);
    const auto axis = property(cone->properties, "axis").vector_value;
    EXPECT_NEAR(axis.x, 0, 1e-12); EXPECT_NEAR(axis.y, 1, 1e-12); EXPECT_NEAR(axis.z, 0, 1e-12);
    const double angle = property(cone->properties, "semiAngle").number_value;
    EXPECT_NEAR(angle, -std::atan(0.5), 1e-12);
    EXPECT_EQ(property(cone->properties, "coneLeaf").integer_value, -1);
    EXPECT_EQ(property(cone->properties, "angleUnit").text_value, "rad");
}
TEST(AssemblyExactSupport, TrimmedArcPreservesUnderlyingCircleAndParameterDomain) {
    OcctKernel kernel;
    const gp_Circ circle(gp_Ax2(gp_Pnt(31, 22, -13), gp_Dir(1, 0, 0), gp_Dir(0, 0, 1)), 12.5);
    const auto arc = import_shape(kernel, BRepBuilderAPI_MakeEdge(circle, 0.3, 1.7).Shape());
    const auto& edges = kernel.getTopology(arc).edges;
    ASSERT_EQ(edges.size(), 1U);
    const auto& p = edges.front().properties;
    EXPECT_DOUBLE_EQ(property(p, "radius").number_value, 12.5);
    EXPECT_NEAR(property(p, "firstParameter").number_value, 0.3, 1e-12);
    EXPECT_NEAR(property(p, "lastParameter").number_value, 1.7, 1e-12);
    EXPECT_FALSE(property(p, "fullCircle").bool_value);
    EXPECT_EQ(property(p, "parameterUnit").text_value, "rad");
    const auto normal = property(p, "normal").vector_value;
    const auto x = property(p, "xDirection").vector_value;
    EXPECT_DOUBLE_EQ(normal.x, 1); EXPECT_DOUBLE_EQ(x.z, 1);
    const auto full = import_shape(kernel, BRepBuilderAPI_MakeEdge(circle).Shape());
    EXPECT_TRUE(property(kernel.getTopology(full).edges.front().properties, "fullCircle").bool_value);
}
TEST(AssemblyExactSupport, ImportedNamingFreezesAnalyticParametersAndStableSourceIdentity) {
    OcctKernel kernel;
    for (const auto& shape : {BRepPrimAPI_MakeSphere(gp_Pnt(10, 20, 30), 6).Shape(),
                             BRepPrimAPI_MakeCone(9, 3, 12).Shape()}) {
        const auto id = import_shape(kernel, shape);
        const auto bytes = kernel.serializeBrepr(id);
        const auto topology = kernel.getTopology(id);
        ImportTopologySeed seed;
        seed.feature_id = "fixture-import";
        seed.body_id = "fixture-cad-body";
        seed.brep_sha256 = make_geometry_id(std::string(bytes.begin(), bytes.end())).substr(7);
        int allocation = 900;
        // Independently allocated domain IDs; local IDs only locate this frozen
        // BREP snapshot. They are never used as semantic IDs or to reconnect.
        for (const auto& face : topology.faces) seed.identities.push_back({"fixture-identity-" + std::to_string(allocation++), PersistentTopologyType::face, face.local_id});
        for (const auto& edge : topology.edges) seed.identities.push_back({"fixture-identity-" + std::to_string(allocation++), PersistentTopologyType::edge, edge.local_id});
        for (const auto& vertex : topology.vertices) seed.identities.push_back({"fixture-identity-" + std::to_string(allocation++), PersistentTopologyType::vertex, vertex.local_id});
        const auto result = kernel.evaluateProfilePadsWithHistory({}, bytes, &seed);
        ASSERT_EQ(result.feature_results.size(), 1U);
        bool analytic = false;
        bool circle = false;
        for (const auto& output : result.feature_results.front().semantic_outputs) {
            const auto& e = output.evidence;
            EXPECT_EQ(output.semantic_ref.feature_id, seed.feature_id);
            ASSERT_EQ(output.semantic_ref.source_ids.size(), 1U);
            EXPECT_NE(output.semantic_ref.source_ids.front().find("fixture-identity-"), std::string::npos);
            if (e.geometry_type == "SPHERE") {
                analytic = true;
                ASSERT_TRUE(e.radius_mm); EXPECT_DOUBLE_EQ(*e.radius_mm, 6);
                ASSERT_TRUE(e.measure_si);
                EXPECT_NEAR(*e.measure_si, 4*std::acos(-1.0)*36*1e-6, 1e-14);
                EXPECT_EQ(e.measure_dimension, "AREA");
                EXPECT_DOUBLE_EQ(e.origin.x, 10); EXPECT_DOUBLE_EQ(e.origin.y, 20); EXPECT_DOUBLE_EQ(e.origin.z, 30);
                ASSERT_TRUE(e.material_side); EXPECT_EQ(*e.material_side, 1);
            }
            if (e.geometry_type == "CONE") {
                analytic = true;
                ASSERT_TRUE(e.half_angle_radians); EXPECT_NEAR(*e.half_angle_radians, std::atan(0.5), 1e-12);
                ASSERT_TRUE(e.cone_leaf); EXPECT_EQ(*e.cone_leaf, -1);
                EXPECT_NEAR(e.origin.z, 18, 1e-12);
            }
            if (e.geometry_type == "CIRCLE") {
                circle = true;
                ASSERT_TRUE(e.radius_mm); EXPECT_GT(*e.radius_mm, 0);
                ASSERT_TRUE(e.x_direction);
                EXPECT_NEAR(e.direction.x*e.x_direction->x+e.direction.y*e.x_direction->y+e.direction.z*e.x_direction->z, 0, 1e-12);
                ASSERT_TRUE(e.parameter_start); ASSERT_TRUE(e.parameter_end);
                // A sphere's pole-to-pole seam is a trimmed semicircle;
                // a cone's end boundary is a complete circle.
                const double expected_range = topology.faces.size() == 1 ? std::acos(-1.0) : 2*std::acos(-1.0);
                EXPECT_NEAR(*e.parameter_end-*e.parameter_start, expected_range, 1e-12);
            }
        }
        EXPECT_TRUE(analytic);
        EXPECT_TRUE(circle);
        auto stale = seed; stale.brep_sha256 = "invalid-frozen-digest";
        EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({}, bytes, &stale), std::invalid_argument);
    }
}
TEST(AssemblyExactSupport, ExportAnalyticRouterFixtures) {
    // Optional deterministic preparation for real Router/Worker integration.
    // With no output directory, still validate every exact solid fixture.
    OcctKernel kernel;
    const std::vector<std::pair<std::string, TopoDS_Shape>> fixtures{
        {"sphere-r6", BRepPrimAPI_MakeSphere(6).Shape()},
        {"sphere-r3", BRepPrimAPI_MakeSphere(3).Shape()},
        {"sphere-r075", BRepPrimAPI_MakeSphere(0.75).Shape()},
        {"cylinder-r6-h12", BRepPrimAPI_MakeCylinder(6,12).Shape()},
        {"cylinder-r3-h12", BRepPrimAPI_MakeCylinder(3,12).Shape()},
        {"cone-r9-r3-h12", BRepPrimAPI_MakeCone(9,3,12).Shape()},
        {"cone-r6-r2-h8", BRepPrimAPI_MakeCone(6,2,8).Shape()},
    };
    const char* directory = std::getenv("OCCCCAD_ASSEMBLY_FIXTURE_DIR");
    if (directory) {
        ASSERT_FALSE(std::string(directory).empty());
        std::filesystem::create_directories(directory);
    }
    for (const auto& [name, shape] : fixtures) {
        const auto id = import_shape(kernel, shape);
        EXPECT_EQ(kernel.getTopology(id).solid_count, 1U);
        EXPECT_GT(kernel.getVolume(id), 0);
        if (!directory) continue;
        for (const auto& [suffix, bytes] : std::vector<std::pair<std::string,std::vector<uint8_t>>>{{".brep",kernel.serializeBrepr(id)},{".step",kernel.serializeStep(id)}}) {
            const auto path=std::filesystem::path(directory)/(name+suffix);
            std::ofstream out(path,std::ios::binary);
            out.write(reinterpret_cast<const char*>(bytes.data()),static_cast<std::streamsize>(bytes.size()));
            ASSERT_TRUE(out.good()) << path;
        }
    }
}
} // namespace
} // namespace occccad::kernel

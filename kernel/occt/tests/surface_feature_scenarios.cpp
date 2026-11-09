#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepGProp.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <GProp_GProps.hxx>
#include <Geom_Surface.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <gp_Pln.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/geometry_id.hpp>

#include <gtest/gtest.h>

#include <algorithm>
#include <cmath>
#include <set>
#include <sstream>

namespace occccad::kernel {
namespace {
GeometryFeatureSpec surface_spec(std::string id, std::string op,
                                 std::vector<GeometryFeatureInput> inputs) {
    GeometryFeatureSpec s;
    s.feature_id = std::move(id);
    s.operation = std::move(op);
    s.inputs = std::move(inputs);
    s.surface.tolerance = 1e-6;
    s.surface.frame_mode = "CORRECTED_FRENET";
    return s;
}
GeometryFeatureSpec source_line(std::string id, Vec3 a = {0, 0, 0}, Vec3 b = {50, 0, 0}) {
    GeometryFeatureSpec s;
    s.feature_id = id;
    s.operation = "LINE";
    s.curve.points = {{"a", a}, {"b", b}};
    return s;
}
GeometryFeatureInput source_input(OcctKernel& k, const ProfileEvaluationResult& result) {
    GeometryFeatureInput input;
    const auto& feature = result.feature_results.back();
    input.source.feature_id = feature.feature_id;
    input.source.body_id = feature.body_id;
    input.source.result_id = feature.result_id;
    input.source.geometry_id = result.geometry_id;
    input.source.brep = k.serializeBrepr(result.geometry_id);
    for (const auto& o : feature.semantic_outputs)
        input.source.topology.push_back(
            {o.semantic_ref.feature_id, o.semantic_ref.output_slot, o.semantic_ref.source_ids,
             o.topology_type == PersistentTopologyType::face   ? TopologyType::FACE
             : o.topology_type == PersistentTopologyType::edge ? TopologyType::EDGE
                                                               : TopologyType::VERTEX,
             o.local_id});
    return input;
}
GeometryFeatureSpec rectangle(std::string id, double z = 0) {
    GeometryFeatureSpec s;
    s.feature_id = id;
    s.operation = "EXTRACT";
    s.sketch_wire.sketch_id = id + "-sketch";
    s.sketch_wire.origin = {0, 0, z};
    s.sketch_wire.normal = {0, 0, 1};
    s.sketch_wire.u_direction = {1, 0, 0};
    const std::vector<Vec2> ps{{0, 0}, {10, 0}, {10, 10}, {0, 10}};
    for (size_t i = 0; i < 4; ++i) {
        ProfileCurveSpec c;
        c.kind = "LINE";
        c.entity_id = "side-" + std::to_string(i);
        c.start = ps[i];
        c.end = ps[(i + 1) % 4];
        s.sketch_wire.curves.push_back(c);
    }
    return s;
}
double surface_area(OcctKernel& k, const GeometryId& id) {
    const auto b = k.serializeBrepr(id);
    std::istringstream in(std::string(b.begin(), b.end()));
    BRep_Builder builder;
    TopoDS_Shape shape;
    BRepTools::Read(shape, in, builder);
    GProp_GProps props;
    BRepGProp::SurfaceProperties(shape, props);
    return props.Mass();
}
std::set<std::string> names(const ProfileEvaluationResult& result) {
    std::set<std::string> out;
    for (const auto& o : result.feature_results.back().semantic_outputs) {
        std::string id = o.semantic_ref.feature_id + "/" + o.semantic_ref.output_slot;
        for (const auto& src : o.semantic_ref.source_ids)
            id += "/" + src;
        EXPECT_TRUE(out.insert(id).second);
    }
    EXPECT_TRUE(result.feature_results.back().topology_history_complete);
    return out;
}
ProfileEvaluationResult sheet(OcctKernel& k) {
    const auto line = k.evaluateGeometryFeature(source_line("line"));
    GeometryFeatureSpec s;
    s.feature_id = "sheet";
    s.operation = "SURFACE_EXTRUDE";
    s.inputs = {source_input(k, line)};
    s.parameters.direction = {0, 0, 1};
    s.parameters.distance = 20;
    return k.evaluateGeometryFeature(s);
}
}  // namespace
TEST(SurfaceFeature, SinglePathSweepOpenProfileAndColdHistory) {
    OcctKernel k;
    const auto p = k.evaluateGeometryFeature(source_line("profile", {0, 0, 0}, {10, 0, 0}));
    const auto rail = k.evaluateGeometryFeature(source_line("rail", {0, 0, 0}, {0, 0, 20}));
    auto s = surface_spec("sweep", "SURFACE_SWEEP", {source_input(k, p), source_input(k, rail)});
    auto r = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(r.geometry_id).solid_count, 0U);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 200, 1e-6);
    const auto identity = names(r);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    s.feature_id = "wrong-plane";
    s.inputs = {source_input(k, rail), source_input(k, rail)};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceFeature, OpenAndClosedLoftHaveExplicitSectionCorrespondence) {
    OcctKernel k;
    const auto a = k.evaluateGeometryFeature(source_line("a", {0, 0, 0}, {10, 0, 0})),
               b = k.evaluateGeometryFeature(source_line("b", {0, 0, 20}, {10, 0, 20}));
    auto s = surface_spec("loft", "SURFACE_LOFT", {source_input(k, a), source_input(k, b)});
    const auto open = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, open.geometry_id), 200, 1e-5);
    names(open);
    const auto c = k.evaluateGeometryFeature(rectangle("c")),
               d = k.evaluateGeometryFeature(rectangle("d", 20));
    s = surface_spec("closed-loft", "SURFACE_LOFT", {source_input(k, c), source_input(k, d)});
    const auto closed = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(closed.geometry_id).solid_count, 0U);
    EXPECT_NEAR(surface_area(k, closed.geometry_id), 800, 1e-4);
    names(closed);
}
TEST(SurfaceFeature, G0FillMeasuresBoundaryErrorAndRejectsOpenLoop) {
    OcctKernel k;
    const auto boundary = k.evaluateGeometryFeature(rectangle("loop"));
    auto s = surface_spec("fill", "SURFACE_FILL", {source_input(k, boundary)});
    const auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 100, 1e-4);
    names(r);
    const auto open = k.evaluateGeometryFeature(source_line("open"));
    s.inputs = {source_input(k, open)};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceFeature, TrimUsesRetainedDesignPointAndRejectsBoundaryAmbiguity) {
    OcctKernel k;
    const auto source = sheet(k);
    GeometryFeatureInput plane;
    plane.plane = GeometryPlane{{25, 0, 0}, {1, 0, 0}, {0, 1, 0}, "cut"};
    auto s = surface_spec("trim", "SURFACE_TRIM", {source_input(k, source), plane});
    s.surface.has_keep_point = true;
    s.surface.keep_point = {5, 0, 5};
    const auto trimmed = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, trimmed.geometry_id), 500, 1e-6);
    names(trimmed);
    s.surface.keep_point = {25, 0, 5};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceFeature, ExtendUsesExactFreeBoundaryAndStableConstructionRoles) {
    OcctKernel k;
    const auto source = sheet(k);
    auto boundary = source_input(k, source);
    for (const auto& o : source.feature_results.back().semantic_outputs)
        if (o.topology_type == PersistentTopologyType::edge &&
            o.semantic_ref.output_slot.find("extrude/end") != std::string::npos) {
            boundary.members.push_back(
                {o.semantic_ref.feature_id, o.semantic_ref.output_slot, o.semantic_ref.source_ids});
            break;
        }
    ASSERT_EQ(boundary.members.size(), 1U);
    auto s = surface_spec("extend", "SURFACE_EXTEND", {source_input(k, source), boundary});
    s.parameters.distance = 10;
    const auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 1500, 1e-4);
    const auto identity = names(r);
    s.parameters.distance = 5;
    EXPECT_EQ(names(k.evaluateGeometryFeature(s)), identity);
}
TEST(SurfaceFeature, ConstantOffsetPreservesFiniteDomainAndHistory) {
    OcctKernel k;
    auto s = surface_spec("offset", "SURFACE_OFFSET", {source_input(k, sheet(k))});
    s.parameters.distance = 3;
    const auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 1000, 1e-5);
    names(r);
    s.parameters.distance = -3;
    names(k.evaluateGeometryFeature(s));
}
TEST(SurfaceFeature, SewCapsAndWallsThenCloseThroughBodyPipeline) {
    OcctKernel k;
    const auto bottom_wire = k.evaluateGeometryFeature(rectangle("bottom-wire")),
               top_wire = k.evaluateGeometryFeature(rectangle("top-wire", 10));
    GeometryFeatureSpec walls;
    walls.feature_id = "walls";
    walls.operation = "SURFACE_EXTRUDE";
    walls.inputs = {source_input(k, bottom_wire)};
    walls.parameters.direction = {0, 0, 1};
    walls.parameters.distance = 10;
    const auto sides = k.evaluateGeometryFeature(walls);
    const auto bottom = k.evaluateGeometryFeature(
                   surface_spec("bottom", "SURFACE_FILL", {source_input(k, bottom_wire)})),
               top = k.evaluateGeometryFeature(
                   surface_spec("top", "SURFACE_FILL", {source_input(k, top_wire)}));
    auto sew =
        surface_spec("sew", "SURFACE_SEW",
                     {source_input(k, sides), source_input(k, bottom), source_input(k, top)});
    const auto shell = k.evaluateGeometryFeature(sew);
    EXPECT_NEAR(surface_area(k, shell.geometry_id), 600, 1e-3);
    names(shell);
    const auto check_normals = [&](const ProfileEvaluationResult& result) {
        const auto topology = k.getTopology(result.geometry_id);
        for (const auto& output : result.feature_results.back().semantic_outputs) {
            if (output.topology_type != PersistentTopologyType::face ||
                output.evidence.geometry_type != "PLANE")
                continue;
            const auto face =
                std::find_if(topology.faces.begin(), topology.faces.end(),
                             [&](const auto& f) { return f.local_id == output.local_id; });
            ASSERT_NE(face, topology.faces.end());
            for (const auto& p : face->properties)
                if (p.name == "normal") {
                    EXPECT_NEAR(output.evidence.direction.x, p.vector_value.x, 1e-9);
                    EXPECT_NEAR(output.evidence.direction.y, p.vector_value.y, 1e-9);
                    EXPECT_NEAR(output.evidence.direction.z, p.vector_value.z, 1e-9);
                }
        }
    };
    check_normals(shell);
    auto reversed_sew = sew;
    reversed_sew.feature_id = "sew-reversed";
    reversed_sew.inputs.front().reversed = true;
    check_normals(k.evaluateGeometryFeature(reversed_sew));
    ProfilePadSpec close;
    close.generator = "CLOSE_SURFACE";
    close.feature_id = "close";
    close.body_id = "body";
    close.body_operation = "NEW_BODY";
    close.geometry_generator = std::make_shared<GeometryFeatureSpec>(
        surface_spec("close", "CLOSE_SURFACE", {source_input(k, shell)}));
    const auto solid = k.evaluateProfilePadsWithHistory({close});
    EXPECT_NEAR(k.getVolume(solid.geometry_id), 1000, 1e-3);
    names(solid);
    sew.operation = "SURFACE_JOIN";
    names(k.evaluateGeometryFeature(sew));
    close.geometry_generator->inputs = {source_input(k, sides)};
    EXPECT_THROW(k.evaluateProfilePadsWithHistory({close}), std::invalid_argument);
}
TEST(SurfaceFeature, CancellationAndToleranceDoNotProducePartialResults) {
    OcctKernel k;
    auto s = surface_spec("offset", "SURFACE_OFFSET", {source_input(k, sheet(k))});
    s.parameters.distance = 2;
    PartRuntimeOptions runtime;
    runtime.cancelled = []() { return true; };
    EXPECT_THROW(k.evaluateGeometryFeature(s, runtime), std::runtime_error);
    s.surface.tolerance = 0;
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceFeature, ThreeSectionsRuledSmoothAndPeriodicSeam) {
    OcctKernel k;
    std::vector<GeometryFeatureInput> sections;
    for (const auto& entry :
         std::vector<std::pair<std::string, double>>{{"first", 0}, {"middle", 10}, {"last", 20}}) {
        auto shape = k.evaluateGeometryFeature(rectangle(entry.first, entry.second));
        sections.push_back(source_input(k, shape));
    }
    auto s = surface_spec("three-sections", "SURFACE_LOFT", sections);
    for (bool ruled : {false, true}) {
        s.surface.ruled = ruled;
        try {
            const auto r = k.evaluateGeometryFeature(s);
            EXPECT_NEAR(surface_area(k, r.geometry_id), 800, 1e-4);
            names(r);
        } catch (const std::exception& e) {
            throw std::runtime_error(std::string(ruled ? "ruled:" : "smooth:") + e.what());
        }
    }
    sections.clear();
    for (const auto& entry :
         std::vector<std::pair<std::string, double>>{{"circle-a", 0}, {"circle-b", 20}}) {
        auto circle = rectangle(entry.first, entry.second);
        circle.sketch_wire.curves.clear();
        ProfileCurveSpec c;
        c.entity_id = "circle";
        c.kind = "CIRCLE";
        c.radius = 5;
        circle.sketch_wire.curves.push_back(c);
        sections.push_back(source_input(k, k.evaluateGeometryFeature(circle)));
    }
    s = surface_spec("periodic-loft", "SURFACE_LOFT", sections);
    const auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 200 * std::acos(-1), 1e-4);
    names(r);
}
TEST(SurfaceFeature, ImportedWireAndPointUseFrozenIdentityAndRejectDigestMismatch) {
    OcctKernel k;
    const auto line = k.evaluateGeometryFeature(source_line("source"));
    auto input = source_input(k, line);
    auto s = surface_spec("import", "IMPORTED", {});
    s.import_brep = input.source.brep;
    ImportTopologySeed seed;
    seed.feature_id = "import";
    seed.result_id = "result";
    seed.brep_sha256 = make_geometry_id(s.import_brep.data(), s.import_brep.size()).substr(7);
    for (const auto& n : input.source.topology)
        seed.identities.push_back({n.output_slot,
                                   n.type == TopologyType::EDGE ? PersistentTopologyType::edge
                                                                : PersistentTopologyType::vertex,
                                   n.local_id});
    s.import_seed = seed;
    const auto r = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(r.geometry_id).geometry_domain, "WIRE_SET");
    const auto identity = names(r);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    s.import_seed->brep_sha256 = "invalid";
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceFeature, OpenImportRejectsUnboundedSupportBeforeMeshingAndNaming) {
    OcctKernel k;
    const auto plane = BRepBuilderAPI_MakeFace(gp_Pln(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1))).Face();
    std::ostringstream stream;
    BRepTools::Write(plane, stream);
    const auto bytes = stream.str();
    const auto id = k.loadBrepr({bytes.begin(), bytes.end()});
    EXPECT_THROW(k.repairImportedGeometry(id), std::invalid_argument);
}

TEST(SurfaceFeature, SingleEdgeSplineContinuityIsSharedBySurfaceAndSolidSweeps) {
    OcctKernel k;
    const auto profile = k.evaluateGeometryFeature(source_line("profile", {0, 0, 0}, {2, 0, 0}));
    GeometryFeatureSpec path;
    path.feature_id = "path";
    path.operation = "SPLINE";
    path.curve.points = {{"a", {0, 0, 0}, {}}, {"b", {0, 0, 10}, {}}, {"c", {0, 10, 10}, {}}};
    path.curve.degree = 1;
    path.curve.knots = {0, .5, 1};
    path.curve.multiplicities = {2, 1, 2};
    path.curve.weights = {1, 1, 1};
    const auto sharp = k.evaluateGeometryFeature(path);
    auto sweep = surface_spec("surface-sweep", "SURFACE_SWEEP",
                              {source_input(k, profile), source_input(k, sharp)});
    try {
        k.evaluateGeometryFeature(sweep);
        FAIL() << "a C0 knot inside one edge was accepted";
    } catch (const std::invalid_argument& e) {
        EXPECT_STREQ(e.what(), "SURFACE_SWEEP_G1_PATH_REQUIRED");
    }
    ProfilePadSpec solid;
    solid.feature_id = "solid-sweep";
    solid.body_id = "body";
    solid.profile_feature_id = "section";
    solid.generator = "SWEEP";
    solid.body_operation = "NEW_BODY";
    solid.plane_normal = {0, 0, 1};
    solid.plane_u_direction = {1, 0, 0};
    solid.sweep_path.exact_path = std::make_shared<GeometryFeatureInput>(source_input(k, sharp));
    ProfileRegionSpec region;
    region.id = "disk";
    region.outer.id = "boundary";
    ProfileCurveSpec circle;
    circle.entity_id = "circle";
    circle.kind = "CIRCLE";
    circle.radius = 1;
    region.outer.curves = {circle};
    solid.regions = {region};
    try {
        k.evaluateProfilePadsWithHistory({solid});
        FAIL() << "solid sweep accepted the same C0 knot";
    } catch (const std::invalid_argument& e) {
        EXPECT_STREQ(e.what(), "FEATURE_FAILED[solid-sweep]: SWEEP_SPATIAL_PATH_REQUIRES_G1");
    }
    // A speed jump is parametrically C0 but geometrically G1: it must remain valid.
    path.curve.points.back().position = {0, 0, 30};
    const auto smooth = k.evaluateGeometryFeature(path);
    sweep.inputs[1] = source_input(k, smooth);
    const auto sheet = k.evaluateGeometryFeature(sweep);
    EXPECT_NEAR(surface_area(k, sheet.geometry_id), 60, 1e-5);
    names(sheet);
    solid.sweep_path.exact_path = std::make_shared<GeometryFeatureInput>(source_input(k, smooth));
    const auto material = k.evaluateProfilePadsWithHistory({solid});
    EXPECT_NEAR(k.getVolume(material.geometry_id), 30 * std::acos(-1), 1e-5);
}

}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(SurfaceShapeControl, PolynomialBlendG2UsesEntirePlaneJetsAndColdNames) {
    OcctKernel k;
    const auto a = k.evaluateGeometryFeature(source_line("blend-a", {0, 0, 0}, {10, 0, 0}));
    const auto b = k.evaluateGeometryFeature(source_line("blend-b", {0, 10, 3}, {10, 10, 3}));
    auto s = surface_spec("blend", "SURFACE_BLEND", {source_input(k, a), source_input(k, b)});
    GeometryFeatureInput pa, pb;
    pa.plane = GeometryPlane{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, "pa"};
    pb.plane = GeometryPlane{{0, 0, 3}, {0, 0, 1}, {1, 0, 0}, "pb"};
    s.surface.boundaries = {{"start", "G2", 0, pa, 1}, {"end", "G2", 1, pb, 1}};
    const auto result = k.evaluateGeometryFeature(s);
    const auto identity = names(result);
    EXPECT_GT(surface_area(k, result.geometry_id), 100);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    s.surface.boundaries[0].support->plane->origin.z = 1;
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(SurfaceShapeControl, OrderedBoundaryPatchAndSupportCornerConflict) {
    OcctKernel k;
    std::vector<GeometryFeatureInput> edges;
    const std::array<Vec3, 4> ps{{{0, 0, 0}, {10, 0, 0}, {10, 10, 0}, {0, 10, 0}}};
    for (size_t i = 0; i < 4; ++i)
        edges.push_back(source_input(k, k.evaluateGeometryFeature(source_line(
                                            "quad-" + std::to_string(i), ps[i], ps[(i + 1) % 4]))));
    auto s = surface_spec("quad", "SURFACE_BOUNDARY", edges);
    const auto out = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, out.geometry_id), 100, 1e-6);
    names(out);
    GeometryFeatureInput plane;
    plane.plane = GeometryPlane{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, "p"};
    s.surface.boundaries = {{"edge-a", "G2", 0, plane, 1}, {"edge-b", "G1", 1, plane, 1}};
    names(k.evaluateGeometryFeature(s));
    s.surface.boundaries[1].support->plane =
        GeometryPlane{{10, 0, 0}, {1, 0, 0}, {0, 1, 0}, "vertical"};
    try {
        k.evaluateGeometryFeature(s);
        FAIL() << "accepted incompatible corner jets";
    } catch (const std::invalid_argument& e) {
        EXPECT_NE(std::string(e.what()).find("SURFACE_SUPPORT_CORNER_JETS_INCOMPATIBLE"),
                  std::string::npos);
    }
    std::swap(s.inputs[0], s.inputs[1]);
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(SurfaceShapeControl, GuidedSweepAndExplicitSpineStationsRetainHistory) {
    OcctKernel k;
    auto a = k.evaluateGeometryFeature(source_line("guided-a", {0, 0, 0}, {10, 0, 0}));
    auto b = k.evaluateGeometryFeature(source_line("guided-b", {0, 0, 20}, {10, 0, 20}));
    auto path = k.evaluateGeometryFeature(source_line("spine", {0, 0, 0}, {0, 0, 20}));
    auto guide = k.evaluateGeometryFeature(source_line("guide", {10, 0, 0}, {10, 0, 20}));
    auto s = surface_spec("guided", "SURFACE_SWEEP", {source_input(k, a), source_input(k, path)});
    s.surface.guide = source_input(k, guide);
    s.surface.frame_mode = "GUIDE_FRAME";
    auto result = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, result.geometry_id), 200, 1e-4);
    names(result);
    s = surface_spec("spine-loft", "SURFACE_LOFT", {source_input(k, a), source_input(k, b)});
    s.surface.spine = source_input(k, path);
    s.surface.sections = {{"a", 0, std::nullopt, Vec3{0, 0, 0}},
                          {"b", 1, std::nullopt, Vec3{0, 0, 20}}};
    auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 200, 1e-4);
    auto identity = names(r);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    s.surface.sections[1].station_point = Vec3{0, 0, 20 - .5 * s.surface.tolerance};
    EXPECT_EQ(names(k.evaluateGeometryFeature(s)), identity);
    s.surface.sections[1].station_point = Vec3{0, 0, 19};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(SurfaceShapeControl, ExplicitInteriorCouplingCreatesDistinctStableSpans) {
    OcctKernel k;
    auto a = k.evaluateGeometryFeature(source_line("coupled-a", {0, 0, 0}, {10, 0, 0})),
         b = k.evaluateGeometryFeature(source_line("coupled-b", {0, 10, 0}, {20, 10, 0}));
    auto s = surface_spec("coupled", "SURFACE_LOFT", {source_input(k, a), source_input(k, b)});
    GeometrySectionControl ca, cb;
    ca.id = "section-a";
    ca.section_index = 0;
    ca.couplings = {{"match", {4, 0, 0}}};
    cb.id = "section-b";
    cb.section_index = 1;
    cb.couplings = {{"match", {12, 10, 0}}};
    s.surface.sections = {ca, cb};
    auto r = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(surface_area(k, r.geometry_id), 150, 1e-4);
    EXPECT_EQ(k.getTopologySummary(r.geometry_id).face_count, 2U);
    auto identity = names(r);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    s.surface.sections[1].couplings[0].position = {21, 10, 0};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(SurfaceShapeControl, CurvedPolynomialSupportJetsAndRationalRestriction) {
    OcctKernel k;
    std::vector<GeometryFeatureInput> edges, supports;
    for (int side = 0; side < 2; ++side) {
        GeometryFeatureSpec profile;
        profile.feature_id = "support-profile-" + std::to_string(side);
        profile.operation = "SPLINE";
        const double y = side ? 20 : 0, z = side ? 3 : 0;
        profile.curve.points = {
            {"p0", {0, y, z}}, {"p1", {3, y, z + 2}}, {"p2", {7, y, z + 2}}, {"p3", {10, y, z}}};
        profile.curve.degree = 3;
        profile.curve.knots = {0, 1};
        profile.curve.multiplicities = {4, 4};
        profile.curve.weights = {1, 1, 1, 1};
        const auto p = k.evaluateGeometryFeature(profile);
        GeometryFeatureSpec extrude;
        extrude.feature_id = "support-" + std::to_string(side);
        extrude.operation = "SURFACE_EXTRUDE";
        extrude.inputs = {source_input(k, p)};
        extrude.parameters.direction = {0, 1, 0};
        extrude.parameters.distance = 5;
        const auto support = k.evaluateGeometryFeature(extrude);
        auto input = source_input(k, support);
        supports.push_back(input);
        const auto& outputs = support.feature_results.back().semantic_outputs;
        auto edge = std::find_if(outputs.begin(), outputs.end(), [&](const auto& o) {
            return o.topology_type == PersistentTopologyType::edge &&
                   o.semantic_ref.output_slot.find(side ? "extrude/start" : "extrude/end") == 0;
        });
        ASSERT_NE(edge, outputs.end());
        input.members = {{edge->semantic_ref.feature_id, edge->semantic_ref.output_slot,
                          edge->semantic_ref.source_ids}};
        edges.push_back(input);
    }
    edges[0].reversed = true;  // Opposite support faces carry opposite boundary orientation.
    auto spec = surface_spec("curved-blend", "SURFACE_BLEND", edges);
    spec.surface.boundaries = {{"a", "G2", 0, supports[0], 1}, {"b", "G2", 1, supports[1], 1}};
    const auto out = k.evaluateGeometryFeature(spec);
    names(out);
    EXPECT_GT(surface_area(k, out.geometry_id), 150);
    // A conic is rational. This bounded jet algorithm must reject it rather than
    // sampling/approximating a different support and claiming curvature continuity.
    GeometryFeatureSpec circle;
    circle.feature_id = "rational";
    circle.operation = "ARC";
    circle.curve.start_angle = 0;
    circle.curve.end_angle = std::acos(-1) / 2;
    circle.curve.points = {{"c", {0, 0, 0}}};
    circle.curve.normal = {0, 0, 1};
    circle.curve.x_direction = {1, 0, 0};
    circle.curve.radius = 5;
    const auto round = k.evaluateGeometryFeature(circle);
    spec.inputs[0] = source_input(k, round);
    spec.surface.boundaries.clear();
    try {
        k.evaluateGeometryFeature(spec);
        FAIL() << "accepted rational patch as polynomial";
    } catch (const std::invalid_argument& e) {
        EXPECT_NE(std::string(e.what()).find("SURFACE_SUPPORT_POLYNOMIAL_DOMAIN_REQUIRED"),
                  std::string::npos);
    }
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(SurfaceShapeControl, BorderGuideIsExactForEntirePolynomialRail) {
    OcctKernel k;
    auto profile = k.evaluateGeometryFeature(source_line("border-profile", {0, 0, 0}, {10, 0, 0})),
         spine = k.evaluateGeometryFeature(source_line("border-spine", {0, 0, 0}, {0, 0, 20}));
    GeometryFeatureSpec guide;
    guide.feature_id = "border-rail";
    guide.operation = "SPLINE";
    guide.curve.points = {
        {"a", {10, 0, 0}}, {"b", {15, 4, 5}}, {"c", {8, 4, 15}}, {"d", {20, 0, 20}}};
    guide.curve.degree = 3;
    guide.curve.knots = {0, 1};
    guide.curve.multiplicities = {4, 4};
    guide.curve.weights = {1, 1, 1, 1};
    const auto rail = k.evaluateGeometryFeature(guide);
    auto s =
        surface_spec("border", "SURFACE_SWEEP", {source_input(k, profile), source_input(k, spine)});
    s.surface.guide = source_input(k, rail);
    s.surface.frame_mode = "GUIDE_BORDER";
    const auto result = k.evaluateGeometryFeature(s);
    const auto identity = names(result);
    OcctKernel cold;
    EXPECT_EQ(names(cold.evaluateGeometryFeature(s)), identity);
    const auto raw = k.serializeBrepr(result.geometry_id);
    BRep_Builder builder;
    TopoDS_Shape shape;
    std::istringstream stream(std::string(raw.begin(), raw.end()));
    BRepTools::Read(shape, stream, builder);
    TopExp_Explorer faces(shape, TopAbs_FACE);
    ASSERT_TRUE(faces.More());
    const auto surface = BRep_Tool::Surface(TopoDS::Face(faces.Current()));
    // An independent evaluation of the Bezier rail agrees at all check points.
    for (int i = 0; i <= 100; ++i) {
        const double u = i / 100., a = 1 - u;
        const Vec3 p{a * a * a * 10 + 3 * a * a * u * 15 + 3 * a * u * u * 8 + u * u * u * 20,
                     3 * a * a * u * 4 + 3 * a * u * u * 4,
                     3 * a * a * u * 5 + 3 * a * u * u * 15 + u * u * u * 20};
        const auto actual = surface->Value(u, 1);
        EXPECT_NEAR(actual.X(), p.x, 1e-10);
        EXPECT_NEAR(actual.Y(), p.y, 1e-10);
        EXPECT_NEAR(actual.Z(), p.z, 1e-10);
    }
    auto top = k.evaluateGeometryFeature(source_line("border-top", {0, 0, 20}, {20, 0, 20}));
    s.operation = "SURFACE_LOFT";
    s.inputs = {source_input(k, profile), source_input(k, top)};
    s.surface.spine = source_input(k, spine);
    s.surface.sections = {{"a", 0, std::nullopt, Vec3{0, 0, 0}, {}},
                          {"b", 1, std::nullopt, Vec3{0, 0, 20}, {}}};
    names(k.evaluateGeometryFeature(s));
    guide.curve.points[1].position.z = -5;
    s.surface.guide = source_input(k, k.evaluateGeometryFeature(guide));
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
}  // namespace occccad::kernel

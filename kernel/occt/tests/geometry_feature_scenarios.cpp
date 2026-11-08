#include <BRepGProp.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <GProp_GProps.hxx>
#include <TopoDS_Shape.hxx>

#include <internal/occt_kernel.hpp>

#include <gtest/gtest.h>

#include <cmath>
#include <limits>
#include <set>
#include <sstream>

namespace occccad::kernel {
namespace {
GeometryFeatureSpec line(const std::string& id, Vec3 a = {0, 0, 0}, Vec3 b = {50, 0, 0}) {
    GeometryFeatureSpec s;
    s.feature_id = id;
    s.operation = "LINE";
    s.curve.points = {{"start", a}, {"end", b}};
    return s;
}
GeometryFeatureInput input(OcctKernel& k, const ProfileEvaluationResult& r) {
    GeometryFeatureInput i;
    const auto& f = r.feature_results.back();
    i.source.feature_id = f.feature_id;
    i.source.body_id = f.body_id;
    i.source.result_id = f.result_id;
    i.source.geometry_id = r.geometry_id;
    i.source.brep = k.serializeBrepr(r.geometry_id);
    for (const auto& o : f.semantic_outputs)
        i.source.topology.push_back(
            {o.semantic_ref.feature_id, o.semantic_ref.output_slot, o.semantic_ref.source_ids,
             o.topology_type == PersistentTopologyType::face   ? TopologyType::FACE
             : o.topology_type == PersistentTopologyType::edge ? TopologyType::EDGE
                                                               : TopologyType::VERTEX,
             o.local_id});
    return i;
}
double area(OcctKernel& k, const GeometryId& id) {
    const auto b = k.serializeBrepr(id);
    std::istringstream s(std::string(b.begin(), b.end()));
    BRep_Builder builder;
    TopoDS_Shape shape;
    BRepTools::Read(shape, s, builder);
    GProp_GProps p;
    BRepGProp::SurfaceProperties(shape, p);
    return p.Mass();
}
std::set<std::string> identities(const ProfileEvaluationResult& r) {
    std::set<std::string> out;
    for (const auto& o : r.feature_results.back().semantic_outputs) {
        std::string s = o.semantic_ref.feature_id + "/" + o.semantic_ref.output_slot;
        for (const auto& id : o.semantic_ref.source_ids)
            s += "/" + id;
        EXPECT_TRUE(out.insert(s).second);
    }
    return out;
}
ProfilePadSpec thickener(const GeometryFeatureInput& source, double thickness = 2) {
    ProfilePadSpec p;
    p.generator = "THICKEN";
    p.feature_id = "thick";
    p.body_id = "body";
    p.body_operation = "NEW_BODY";
    p.geometry_generator = std::make_shared<GeometryFeatureSpec>();
    auto& s = *p.geometry_generator;
    s.feature_id = p.feature_id;
    s.operation = "THICKEN";
    s.inputs = {source};
    s.parameters.distance = thickness;
    return p;
}
}  // namespace
TEST(GeometryFeature, SourceCurvesAndExtrusionsShareVisualMeshingPolicy) {
    const auto distance = [](const Vec3& p, const Vec3& a, const Vec3& b) {
        const Vec3 d{b.x - a.x, b.y - a.y, b.z - a.z};
        const double length = d.x * d.x + d.y * d.y + d.z * d.z;
        const double t =
            length > 0
                ? std::clamp(((p.x - a.x) * d.x + (p.y - a.y) * d.y + (p.z - a.z) * d.z) / length,
                             0.0, 1.0)
                : 0;
        return std::hypot(p.x - a.x - t * d.x, p.y - a.y - t * d.y, p.z - a.z - t * d.z);
    };
    const auto deviation = [&](const std::vector<Vec3>& from, const std::vector<Vec3>& to) {
        double worst = 0;
        for (size_t i = 1; i < from.size(); ++i)
            for (int j = 0; j <= 4; ++j) {
                const double t = j / 4.0;
                const Vec3 p{from[i - 1].x * (1 - t) + from[i].x * t,
                             from[i - 1].y * (1 - t) + from[i].y * t,
                             from[i - 1].z * (1 - t) + from[i].z * t};
                double best = std::numeric_limits<double>::infinity();
                for (size_t k = 1; k < to.size(); ++k)
                    best = std::min(best, distance(p, to[k - 1], to[k]));
                worst = std::max(worst, best);
            }
        return worst;
    };
    for (const bool circle : {false, true}) {
        OcctKernel k;
        GeometryFeatureSpec s;
        s.feature_id = "profile";
        s.operation = circle ? "CIRCLE" : "SPLINE";
        if (circle) {
            s.curve.points = {{"center", {0, 0, 0}}};
            s.curve.radius = 32;
            s.curve.normal = {0, 0, 1};
            s.curve.x_direction = {1, 0, 0};
        } else
            s.curve.points = {
                {"a", {0, 0, 0}}, {"b", {8, 12, 0}}, {"c", {20, -12, 0}}, {"d", {32, 0, 0}}};
        const auto curve = k.evaluateGeometryFeature(s);
        GeometryFeatureSpec extrude;
        extrude.feature_id = "sheet";
        extrude.operation = "SURFACE_EXTRUDE";
        extrude.inputs = {input(k, curve)};
        extrude.parameters.direction = {0, 0, 1};
        extrude.parameters.distance = 80;
        const auto sheet = k.evaluateGeometryFeature(extrude);
        const auto originalCurve = k.serializeBrepr(curve.geometry_id),
                   originalSheet = k.serializeBrepr(sheet.geometry_id);
        const auto coarse = k.tessellate(curve.geometry_id, .1, .5);
        const auto fine = k.tessellate(curve.geometry_id);
        const auto surface = k.tessellate(sheet.geometry_id);
        ASSERT_EQ(fine.edges.size(), 1U);
        ASSERT_EQ(coarse.edges.size(), 1U);
        EXPECT_GT(fine.edges.front().points.size(), coarse.edges.front().points.size());
        const auto boundary =
            std::find_if(surface.edges.begin(), surface.edges.end(), [](const auto& edge) {
                return std::all_of(edge.points.begin(), edge.points.end(),
                                   [](const auto& p) { return std::abs(p.z) < 1e-8; });
            });
        ASSERT_NE(boundary, surface.edges.end());
        const auto error = std::max(deviation(fine.edges.front().points, boundary->points),
                                    deviation(boundary->points, fine.edges.front().points));
        EXPECT_LE(error, visual_linear_deflection_mm);
        EXPECT_EQ(k.serializeBrepr(curve.geometry_id), originalCurve);
        EXPECT_EQ(k.serializeBrepr(sheet.geometry_id), originalSheet);
        std::cout << (circle ? "circle" : "spline") << " source points "
                  << coarse.edges.front().points.size() << " -> "
                  << fine.edges.front().points.size() << ", surface boundary "
                  << boundary->points.size() << ", sampled display gap " << error << " mm\n";
    }
}
TEST(GeometryFeature, SheetExtrudeThickenAndColdNamingHaveIndependentOwners) {
    OcctKernel k;
    const auto curve = k.evaluateGeometryFeature(line("line"));
    GeometryFeatureSpec s;
    s.feature_id = "sheet";
    s.operation = "SURFACE_EXTRUDE";
    s.inputs = {input(k, curve)};
    s.parameters.direction = {0, 0, 1};
    s.parameters.distance = 20;
    const auto sheet = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(sheet.geometry_id).solid_count, 0U);
    EXPECT_NEAR(area(k, sheet.geometry_id), 1000, 1e-8);
    EXPECT_EQ(sheet.feature_results.back().body_id, "");
    EXPECT_EQ(sheet.feature_results.back().result_id, "result");
    EXPECT_TRUE(sheet.feature_results.back().topology_history_complete);
    const auto baseline = identities(sheet);
    OcctKernel cold;
    const auto replay = cold.evaluateGeometryFeature(s);
    EXPECT_EQ(baseline, identities(replay));
    EXPECT_EQ(k.serializeBrepr(sheet.geometry_id), cold.serializeBrepr(replay.geometry_id));
    const auto solid = k.evaluateProfilePadsWithHistory({thickener(input(k, sheet))});
    EXPECT_EQ(k.getTopologySummary(solid.geometry_id).solid_count, 1U);
    EXPECT_NEAR(k.getVolume(solid.geometry_id), 2000, 1e-7);
    EXPECT_TRUE(solid.feature_results.back().topology_history_complete);
    ProfilePadSpec fillet;
    fillet.feature_id = "round";
    fillet.input_feature_id = "thick";
    fillet.body_id = "body";
    fillet.generator = "FILLET";
    fillet.pad_length = .25;
    const auto edge =
        std::find_if(solid.feature_results.back().semantic_outputs.begin(),
                     solid.feature_results.back().semantic_outputs.end(),
                     [](const auto& o) { return o.topology_type == PersistentTopologyType::edge; });
    ASSERT_NE(edge, solid.feature_results.back().semantic_outputs.end());
    fillet.selections = {{edge->semantic_ref.feature_id, edge->semantic_ref.output_slot,
                          edge->semantic_ref.source_ids}};
    const auto rounded = k.evaluateProfilePadsWithHistory({thickener(input(k, sheet)), fillet});
    EXPECT_GT(k.getVolume(rounded.geometry_id), 0);
    EXPECT_LT(k.getVolume(rounded.geometry_id), 2000);
    EXPECT_TRUE(rounded.feature_results.back().topology_history_complete);
    s.parameters.distance = 30;
    const auto changed = k.evaluateGeometryFeature(s);
    EXPECT_EQ(baseline, identities(changed));
    const auto updated = k.evaluateProfilePadsWithHistory({thickener(input(k, changed))});
    EXPECT_NEAR(k.getVolume(updated.geometry_id), 3000, 1e-7);
    GeometryFeatureSpec infinite;
    infinite.feature_id = "unbounded";
    infinite.operation = "EXTRACT";
    GeometryFeatureInput plane;
    plane.plane = GeometryPlane{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, "plane"};
    infinite.inputs = {plane};
    EXPECT_THROW(k.evaluateGeometryFeature(infinite), std::invalid_argument);
}
TEST(GeometryFeature, PeriodicSheetAndRectangleShellThickenWithoutLooseTopology) {
    OcctKernel k;
    GeometryFeatureSpec c;
    c.feature_id = "circle";
    c.operation = "CIRCLE";
    c.curve.points = {{"center", {0, 0, 0}}};
    c.curve.radius = 10;
    c.curve.normal = {0, 0, 1};
    c.curve.x_direction = {1, 0, 0};
    const auto curve = k.evaluateGeometryFeature(c);
    GeometryFeatureSpec e;
    e.feature_id = "tube";
    e.operation = "SURFACE_EXTRUDE";
    e.inputs = {input(k, curve)};
    e.parameters.direction = {0, 0, 1};
    e.parameters.distance = 20;
    const auto sheet = k.evaluateGeometryFeature(e);
    EXPECT_NEAR(area(k, sheet.geometry_id), 400 * std::acos(-1), 1e-6);
    ProfileEvaluationResult solid;
    ASSERT_NO_THROW(solid = k.evaluateProfilePadsWithHistory({thickener(input(k, sheet))}));
    EXPECT_NEAR(k.getVolume(solid.geometry_id), 20 * std::acos(-1) * (100 - 64), 1e-5);
    EXPECT_THROW(k.evaluateProfilePadsWithHistory({thickener(input(k, sheet), 10)}),
                 std::invalid_argument);
    GeometryFeatureSpec boundary;
    boundary.feature_id = "boundary";
    boundary.operation = "BOUNDARY";
    boundary.inputs = {input(k, sheet)};
    const auto b = k.evaluateGeometryFeature(boundary);
    EXPECT_EQ(k.getTopologySummary(b.geometry_id).edge_count, 2U);
    GeometryFeatureSpec rectangle;
    rectangle.feature_id = "rectangle";
    rectangle.operation = "SURFACE_EXTRUDE";
    rectangle.parameters.direction = {0, 0, 1};
    rectangle.parameters.distance = 10;
    rectangle.sketch_wire.sketch_id = "sketch";
    rectangle.sketch_wire.normal = {0, 0, 1};
    rectangle.sketch_wire.u_direction = {1, 0, 0};
    const Vec2 ps[] = {{0, 0}, {30, 0}, {30, 20}, {0, 20}};
    for (size_t i = 0; i < 4; ++i) {
        ProfileCurveSpec edge;
        edge.kind = "LINE";
        edge.entity_id = "side-" + std::to_string(i);
        edge.start = ps[i];
        edge.end = ps[(i + 1) % 4];
        rectangle.sketch_wire.curves.push_back(edge);
    }
    const auto walls = k.evaluateGeometryFeature(rectangle);
    EXPECT_NEAR(area(k, walls.geometry_id), 1000, 1e-6);
    ProfileEvaluationResult box;
    ASSERT_NO_THROW(box = k.evaluateProfilePadsWithHistory({thickener(input(k, walls))}));
    EXPECT_NEAR(k.getVolume(box.geometry_id), 1840, 1e-6);
}
TEST(GeometryFeature, FiniteCurveOperationsRespectDirectionAndSemanticMembers) {
    OcctKernel k;
    const auto source = k.evaluateGeometryFeature(line("source"));
    GeometryFeatureSpec s;
    s.feature_id = "trim";
    s.operation = "TRIM";
    s.inputs = {input(k, source)};
    s.parameters.start = .2;
    s.parameters.end = .6;
    const auto trim = k.evaluateGeometryFeature(s);
    const auto bounds = k.getBoundingBox(trim.geometry_id);
    EXPECT_NEAR(bounds.min.x, 10, 1e-6);
    EXPECT_NEAR(bounds.max.x, 30, 1e-6);
    s.operation = "POINT_ON_CURVE";
    s.feature_id = "point";
    s.parameters.start = .5;
    const auto p = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(k.getBoundingBox(p.geometry_id).min.x, 25, 1e-6);
    s.operation = "SPLIT";
    s.feature_id = "split";
    const auto split = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(split.geometry_id).edge_count, 2U);
    s.operation = "EXTEND";
    s.feature_id = "extend";
    s.parameters.distance = 5;
    s.parameters.side = "START";
    s.inputs.front().reversed = true;
    const auto extended = k.evaluateGeometryFeature(s);
    EXPECT_NEAR(k.getBoundingBox(extended.geometry_id).max.x, 55, 1e-6);
    s.operation = "EXTRACT";
    s.feature_id = "extract";
    s.inputs = {input(k, source)};
    s.inputs[0].members = {FeatureTopologyRef{"missing", "curve", {}}};
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
    s.operation = "TRIM";
    s.inputs = {input(k, source)};
    s.parameters.start = .8;
    s.parameters.end = .2;
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
TEST(GeometryFeature, SpatialSplineTransformAndG1ConnectionKeepFormalSources) {
    OcctKernel k;
    GeometryFeatureSpec s;
    s.feature_id = "spline";
    s.operation = "SPLINE";
    s.curve.points = {{"a", {0, 0, 0}}, {"b", {10, 5, 5}}, {"c", {20, 0, 10}}};
    const auto spline = k.evaluateGeometryFeature(s);
    EXPECT_EQ(k.getTopologySummary(spline.geometry_id).edge_count, 1U);
    GeometryFeatureSpec tr;
    tr.feature_id = "move";
    tr.operation = "TRANSLATE";
    tr.inputs = {input(k, spline)};
    tr.parameters.direction = {1, 0, 0};
    tr.parameters.distance = 5;
    const auto moved = k.evaluateGeometryFeature(tr);
    EXPECT_NEAR(k.getBoundingBox(moved.geometry_id).min.x, 5, 1e-6);
    EXPECT_TRUE(moved.feature_results[0].topology_history_complete);
    const auto first = k.evaluateGeometryFeature(line("a", {0, 0, 0}, {10, 0, 0})),
               last = k.evaluateGeometryFeature(line("b", {20, 10, 5}, {30, 10, 5}));
    GeometryFeatureSpec connect;
    connect.feature_id = "connect";
    connect.operation = "G1_CONNECT";
    connect.inputs = {input(k, first), input(k, last)};
    connect.parameters.tension = 1;
    const auto result = k.evaluateGeometryFeature(connect);
    EXPECT_EQ(k.getTopologySummary(result.geometry_id).edge_count, 1U);
    PartRuntimeOptions cancelled;
    cancelled.cancelled = []() { return true; };
    EXPECT_THROW(k.evaluateGeometryFeature(connect, cancelled), std::runtime_error);
}
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(GeometryFeature, CompositeOffsetProjectionIntersectionRespectFiniteDomains) {
    OcctKernel k;
    const auto a = k.evaluateGeometryFeature(line("first", {0, 0, 0}, {20, 0, 0}));
    const auto b = k.evaluateGeometryFeature(line("second", {20, 0, 0}, {20, 15, 5}));
    GeometryFeatureSpec composite;
    composite.feature_id = "chain";
    composite.operation = "COMPOSITE";
    composite.inputs = {input(k, a), input(k, b)};
    const auto joined = k.evaluateGeometryFeature(composite);
    EXPECT_EQ(k.getTopologySummary(joined.geometry_id).edge_count, 2U);
    EXPECT_TRUE(joined.feature_results.back().topology_history_complete);
    const auto far = k.evaluateGeometryFeature(line("far", {30, 0, 0}, {40, 0, 0}));
    composite.inputs = {input(k, a), input(k, far)};
    EXPECT_THROW(k.evaluateGeometryFeature(composite), std::invalid_argument);
    GeometryFeatureSpec circle;
    circle.feature_id = "circle";
    circle.operation = "CIRCLE";
    circle.curve.points = {{"center", {0, 0, 0}, {}}};
    circle.curve.normal = {0, 0, 1};
    circle.curve.x_direction = {1, 0, 0};
    circle.curve.radius = 10;
    const auto round = k.evaluateGeometryFeature(circle);
    GeometryFeatureInput plane;
    plane.plane = GeometryPlane{{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, "xy"};
    GeometryFeatureSpec offset;
    offset.feature_id = "offset";
    offset.operation = "OFFSET";
    offset.inputs = {input(k, round), plane};
    offset.parameters.distance = 2;
    const auto larger = k.evaluateGeometryFeature(offset);
    EXPECT_NEAR(k.getBoundingBox(larger.geometry_id).max.x, 8, 1e-6);
    offset.inputs = {input(k, a), plane};
    const auto offset_line = k.evaluateGeometryFeature(offset);
    EXPECT_EQ(k.getTopologySummary(offset_line.geometry_id).edge_count, 1U);
    EXPECT_NEAR(k.getBoundingBox(offset_line.geometry_id).min.y, 2, 1e-6);
    offset.parameters.distance = 4;
    const auto moved_offset = k.evaluateGeometryFeature(offset);
    EXPECT_NEAR(k.getBoundingBox(moved_offset.geometry_id).min.y, 4, 1e-6);
    EXPECT_EQ(identities(offset_line), identities(moved_offset));
    GeometryFeatureSpec extrude;
    extrude.feature_id = "xz";
    extrude.operation = "SURFACE_EXTRUDE";
    extrude.inputs = {input(k, a)};
    extrude.parameters.direction = {0, 0, 1};
    extrude.parameters.distance = 20;
    const auto xz = k.evaluateGeometryFeature(extrude);
    const auto above = k.evaluateGeometryFeature(line("above", {-10, 10, 5}, {30, 10, 5}));
    GeometryFeatureSpec project;
    project.feature_id = "projection";
    project.operation = "PROJECT";
    project.inputs = {input(k, above), input(k, xz)};
    project.parameters.direction = {0, -1, 0};
    const auto projected = k.evaluateGeometryFeature(project);
    const auto bounds = k.getBoundingBox(projected.geometry_id);
    EXPECT_NEAR(bounds.min.x, 0, 1e-6);
    EXPECT_NEAR(bounds.max.x, 20, 1e-6);
    EXPECT_NEAR(bounds.min.y, 0, 1e-6);
    const auto alongY = k.evaluateGeometryFeature(line("along-y", {10, -10, 0}, {10, 10, 0}));
    extrude.feature_id = "yz";
    extrude.inputs = {input(k, alongY)};
    const auto yz = k.evaluateGeometryFeature(extrude);
    GeometryFeatureSpec section;
    section.feature_id = "intersection";
    section.operation = "INTERSECTION";
    section.inputs = {input(k, xz), input(k, yz)};
    const auto crossed = k.evaluateGeometryFeature(section);
    const auto cb = k.getBoundingBox(crossed.geometry_id);
    EXPECT_NEAR(cb.min.x, 10, 1e-6);
    EXPECT_NEAR(cb.max.x, 10, 1e-6);
    EXPECT_NEAR(cb.min.y, 0, 1e-6);
    EXPECT_NEAR(cb.min.z, 0, 1e-6);
    EXPECT_NEAR(cb.max.z, 20, 1e-6);
}
TEST(GeometryFeature, RationalSpatialSplinePointAndReflectionsHaveExactResults) {
    OcctKernel k;
    GeometryFeatureSpec s;
    s.feature_id = "rational";
    s.operation = "SPLINE";
    s.curve.points = {{"a", {1, 0, 3}, {}}, {"b", {1, 1, 3}, {}}, {"c", {0, 1, 3}, {}}};
    s.curve.degree = 2;
    s.curve.knots = {0, 1};
    s.curve.multiplicities = {3, 3};
    s.curve.weights = {1, std::sqrt(.5), 1};
    const auto rational = k.evaluateGeometryFeature(s);
    GeometryFeatureSpec point;
    point.feature_id = "at";
    point.operation = "POINT_ON_CURVE";
    point.inputs = {input(k, rational)};
    point.parameters.start = .5;
    const auto at = k.evaluateGeometryFeature(point);
    const auto p = k.getBoundingBox(at.geometry_id);
    EXPECT_NEAR(p.min.x, std::sqrt(.5), 1e-6);
    EXPECT_NEAR(p.min.y, std::sqrt(.5), 1e-6);
    EXPECT_NEAR(p.min.z, 3, 1e-6);
    GeometryFeatureSpec transform;
    transform.feature_id = "mirror";
    transform.operation = "MIRROR";
    transform.inputs = {input(k, at)};
    transform.parameters.direction = {1, 0, 0};
    const auto mirrored = k.evaluateGeometryFeature(transform);
    EXPECT_NEAR(k.getBoundingBox(mirrored.geometry_id).min.x, -std::sqrt(.5), 1e-6);
    transform.operation = "ROTATE";
    transform.feature_id = "rotate";
    transform.parameters.direction = {0, 0, 1};
    transform.parameters.angle = std::acos(-1) / 2;
    const auto rotated = k.evaluateGeometryFeature(transform);
    EXPECT_NEAR(k.getBoundingBox(rotated.geometry_id).min.x, -std::sqrt(.5), 1e-6);
    EXPECT_NEAR(k.getBoundingBox(rotated.geometry_id).min.y, std::sqrt(.5), 1e-6);
    s.curve.weights[1] = -1;
    EXPECT_THROW(k.evaluateGeometryFeature(s), std::invalid_argument);
}
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(GeometryFeature, ExistingSolidSweepConsumesAnAssociativeSpatialSpline) {
    OcctKernel k;
    GeometryFeatureSpec path;
    path.feature_id = "spatial-path";
    path.operation = "SPLINE";
    path.curve.points = {{"a", {0, 0, 0}, {}},
                         {"b", {0, 0, 10}, {}},
                         {"c", {10, 10, 20}, {}},
                         {"d", {10, 20, 30}, {}}};
    path.curve.degree = 3;
    path.curve.knots = {0, 1};
    path.curve.multiplicities = {4, 4};
    const auto curve = k.evaluateGeometryFeature(path);
    const auto source = input(k, curve);
    ProfilePadSpec sweep;
    sweep.feature_id = "sweep";
    sweep.body_id = "body";
    sweep.profile_feature_id = "section";
    sweep.generator = "SWEEP";
    sweep.body_operation = "NEW_BODY";
    sweep.plane_normal = {0, 0, 1};
    sweep.plane_u_direction = {1, 0, 0};
    sweep.sweep_path.exact_path = std::make_shared<GeometryFeatureInput>(source);
    ProfileRegionSpec region;
    region.id = "disk";
    region.outer.id = "boundary";
    ProfileCurveSpec circle;
    circle.entity_id = "circle";
    circle.kind = "CIRCLE";
    circle.radius = 1;
    region.outer.curves = {circle};
    sweep.regions = {region};
    const auto solid = k.evaluateProfilePadsWithHistory({sweep});
    EXPECT_EQ(k.getTopologySummary(solid.geometry_id).solid_count, 1U);
    EXPECT_TRUE(solid.feature_results.back().topology_history_complete);
    EXPECT_GT(k.getVolume(solid.geometry_id), 30 * std::acos(-1));
    const auto before = identities(solid);
    path.curve.points.back().position.z = 40;
    const auto updated = k.evaluateGeometryFeature(path);
    sweep.sweep_path.exact_path = std::make_shared<GeometryFeatureInput>(input(k, updated));
    const auto changed = k.evaluateProfilePadsWithHistory({sweep});
    EXPECT_GT(k.getVolume(changed.geometry_id), k.getVolume(solid.geometry_id));
    EXPECT_EQ(before, identities(changed));
}
}  // namespace occccad::kernel

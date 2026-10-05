#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepGProp.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GC_MakeArcOfCircle.hxx>
#include <GProp_GProps.hxx>
#include <Geom2d_Curve.hxx>
#include <STEPControl_Reader.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>

#include <internal/fillet_boundary.hpp>
#include <internal/occt_kernel.hpp>
#include <occccad/kernel/geometry_id.hpp>

#include <gtest/gtest.h>

#include <chrono>
#include <filesystem>
#include <iostream>
#include <set>
#include <sstream>

namespace occccad::kernel {
namespace {
void describe(const TopoDS_Shape& shape) {
    for (TopExp_Explorer it(shape, TopAbs_FACE); it.More(); it.Next()) {
        const auto face = TopoDS::Face(it.Current());
        BRepAdaptor_Surface s(face);
        GProp_GProps props;
        BRepGProp::SurfaceProperties(face, props);
        const auto c = props.CentreOfMass();
        std::cout << "face type=" << s.GetType() << " area=" << props.Mass() << " center=" << c.X()
                  << "," << c.Y() << "," << c.Z();
        if (s.GetType() == GeomAbs_Cylinder) {
            const auto cy = s.Cylinder();
            const auto p = cy.Location();
            const auto d = cy.Axis().Direction();
            std::cout << " R=" << cy.Radius() << " axis=" << p.X() << "," << p.Y() << "," << p.Z()
                      << " dir=" << d.X() << "," << d.Y() << "," << d.Z();
        }
        if (s.GetType() == GeomAbs_Plane) {
            const auto d = s.Plane().Axis().Direction();
            std::cout << " normal=" << d.X() << "," << d.Y() << "," << d.Z();
        }
        std::cout << "\n";
    }
}
}  // namespace
TEST(FilletBoundary, TargetSupportEvidence) {
    for (const auto& name : {"Solar Panel - Solar Panel.step", "Main Fan - Main Fan.step"}) {
        const auto path = std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) / name;
        STEPControl_Reader reader;
        ASSERT_EQ(reader.ReadFile(path.c_str()), IFSelect_RetDone);
        reader.TransferRoots();
        std::cout << "TARGET " << name << "\n";
        describe(reader.OneShape());
    }
}
TEST(FilletBoundary, OriginalFiniteDomainReproductions) {
    const auto panel = BRepPrimAPI_MakeBox(60, 30, 2.2).Shape();
    BRepFilletAPI_MakeFillet algorithm(panel);
    std::vector<TopoDS_Shape> panelEdges;
    TopTools_IndexedMapOfShape seen;
    for (TopExp_Explorer it(panel, TopAbs_EDGE); it.More(); it.Next()) {
        const auto edge = TopoDS::Edge(it.Current());
        BRepAdaptor_Curve curve(edge);
        if (std::abs(curve.Value(curve.FirstParameter()).Z()) < 1e-8 &&
            std::abs(curve.Value(curve.LastParameter()).Z()) < 1e-8) {
            algorithm.Add(3, edge);
            if (!seen.Contains(edge)) {
                seen.Add(edge);
                panelEdges.push_back(edge);
            }
        }
    }
    auto t = std::chrono::steady_clock::now();
    algorithm.Build();
    std::cout
        << "SOLAR finite support R3 h2.2 done=" << algorithm.IsDone()
        << " faulty_contours=" << algorithm.NbFaultyContours() << " ms="
        << std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - t).count()
        << "\n";
    EXPECT_FALSE(algorithm.IsDone());
    for (int i = 1; i <= algorithm.NbFaultyContours(); ++i)
        std::cout << " contour_status=" << algorithm.StripeStatus(algorithm.FaultyContour(i))
                  << "\n";
    // A section through a 30-degree, 2 mm blade entering a vertical hub wall.
    const double slope = std::tan(M_PI / 6), half = 1 / std::cos(M_PI / 6);
    BRepBuilderAPI_MakePolygon polygon;
    for (const auto& p : {gp_Pnt(0, -10, -10), gp_Pnt(0, 0, -10), gp_Pnt(0, 0, -half),
                          gp_Pnt(0, 60, 60 * slope - half), gp_Pnt(0, 60, 60 * slope + half),
                          gp_Pnt(0, 0, half), gp_Pnt(0, 0, 10), gp_Pnt(0, -10, 10)})
        polygon.Add(p);
    polygon.Close();
    const auto untrimmed =
        BRepPrimAPI_MakePrism(BRepBuilderAPI_MakeFace(polygon.Wire()).Face(), gp_Vec(10, 0, 0))
            .Shape();
    const auto fan =
        BRepAlgoAPI_Common(untrimmed,
                           BRepPrimAPI_MakeBox(gp_Pnt(-1, -20, -10), gp_Pnt(11, 70, 10)).Shape())
            .Shape();
    BRepFilletAPI_MakeFillet root(fan);
    std::vector<TopoDS_Shape> fanEdges;
    seen.Clear();
    for (TopExp_Explorer it(fan, TopAbs_EDGE); it.More(); it.Next()) {
        const auto edge = TopoDS::Edge(it.Current());
        BRepAdaptor_Curve c(edge);
        auto p = c.Value(c.FirstParameter()), q = c.Value(c.LastParameter());
        if (std::abs(p.Y()) < 1e-8 && std::abs(q.Y()) < 1e-8 && std::abs(p.Z() - half) < 1e-8 &&
            std::abs(q.Z() - half) < 1e-8) {
            root.Add(15, edge);
            if (!seen.Contains(edge)) {
                seen.Add(edge);
                fanEdges.push_back(edge);
            }
        }
    }
    t = std::chrono::steady_clock::now();
    root.Build();
    std::cout
        << "FAN finite support R15 t2 done=" << root.IsDone()
        << " faulty_contours=" << root.NbFaultyContours() << " ms="
        << std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - t).count()
        << "\n";
    for (int i = 1; i <= root.NbFaultyContours(); ++i)
        std::cout << " contour_status=" << root.StripeStatus(root.FaultyContour(i)) << "\n";
    EXPECT_FALSE(root.IsDone());
    auto panelResult = detail::rebuild_fillet_boundary(panel, panelEdges, 3);
    ASSERT_TRUE(panelResult.IsDone());
    describe(panelResult.shape);
    EXPECT_THROW(detail::rebuild_fillet_boundary(fan, fanEdges, 15), std::runtime_error);
    gp_Trsf tr;
    tr.SetRotation(gp_Ax1(gp_Pnt(0, 0, 0), gp_Dir(0, 1, 0)), M_PI / 6);
    const auto longBlade =
        BRepPrimAPI_MakePrism(BRepBuilderAPI_MakeFace(polygon.Wire()).Face(), gp_Vec(60, 0, 0))
            .Shape();
    gp_Trsf shift;
    shift.SetTranslation(gp_Vec(-30, 0, 0));
    tr.Multiply(shift);
    const auto rotated = BRepBuilderAPI_Transform(longBlade, tr, true).Shape();
    const auto limited =
        BRepAlgoAPI_Common(
            rotated, BRepPrimAPI_MakeBox(gp_Pnt(-100, -100, -10), gp_Pnt(100, 100, 10)).Shape())
            .Shape();
    fanEdges.clear();
    seen.Clear();
    auto inv = tr.Inverted();
    for (TopExp_Explorer it(limited, TopAbs_EDGE); it.More(); it.Next()) {
        auto e = TopoDS::Edge(it.Current());
        BRepAdaptor_Curve c(e);
        auto p = c.Value(c.FirstParameter()).Transformed(inv),
             q = c.Value(c.LastParameter()).Transformed(inv);
        if (std::abs(p.Y()) < 1e-6 && std::abs(q.Y()) < 1e-6 && std::abs(p.Z() - half) < 1e-6 &&
            std::abs(q.Z() - half) < 1e-6 && !seen.Contains(e)) {
            seen.Add(e);
            fanEdges.push_back(e);
        }
    }
    ASSERT_EQ(fanEdges.size(), 1);
    auto fanResult = detail::rebuild_fillet_boundary(limited, fanEdges, 15);
    ASSERT_TRUE(fanResult.IsDone());
    int retained = 0;
    for (TopExp_Explorer it(fanResult.shape, TopAbs_FACE); it.More(); it.Next()) {
        BRepAdaptor_Surface f(TopoDS::Face(it.Current()));
        if (f.GetType() == GeomAbs_Cylinder) {
            ++retained;
            EXPECT_NEAR(f.Cylinder().Radius(), 15, 1e-7);
            EXPECT_NEAR(
                std::abs(f.Cylinder().Axis().Direction().Dot(gp_Dir(1, 0, 0).Transformed(tr))), 1,
                1e-7);
        }
    }
    ASSERT_EQ(retained, 1);
    // Independent reference: an exact circular cross-section, not a cylinder
    // subtraction or the production support-extension algorithm.
    const gp_Pnt p(0, 0, half), center(0, 15, half + 15 * std::sqrt(3.)), t1(0, 0, center.Z()),
        t2(0, 22.5, center.Z() - 15 * std::cos(M_PI / 6));
    const auto mid = center.Translated(gp_Vec(gp_Dir(gp_Vec(center, p))) * 15);
    BRepBuilderAPI_MakeWire wire;
    wire.Add(BRepBuilderAPI_MakeEdge(p, t1));
    wire.Add(BRepBuilderAPI_MakeEdge(GC_MakeArcOfCircle(t1, mid, t2).Value()));
    wire.Add(BRepBuilderAPI_MakeEdge(t2, p));
    const auto section =
        BRepPrimAPI_MakePrism(BRepBuilderAPI_MakeFace(wire.Wire()).Face(), gp_Vec(60, 0, 0))
            .Shape();
    const auto extra =
        BRepAlgoAPI_Common(
            BRepBuilderAPI_Transform(section, tr, true).Shape(),
            BRepPrimAPI_MakeBox(gp_Pnt(-100, -100, -10), gp_Pnt(100, 100, 10)).Shape())
            .Shape();
    const auto reference = BRepAlgoAPI_Fuse(limited, extra).Shape();
    for (bool reverse : {false, true}) {
        GProp_GProps props;
        BRepGProp::VolumeProperties(BRepAlgoAPI_Cut(reverse ? reference : fanResult.shape,
                                                    reverse ? fanResult.shape : reference)
                                        .Shape(),
                                    props);
        EXPECT_NEAR(props.Mass(), 0, 1e-5);
    }
    Bnd_Box bounds;
    BRepBndLib::AddOptimal(fanResult.shape, bounds, false, false);
    double x0, y0, z0, x1, y1, z1;
    bounds.Get(x0, y0, z0, x1, y1, z1);
    EXPECT_NEAR(z0, -10, 1e-7);
    EXPECT_NEAR(z1, 10, 1e-7);
    EXPECT_TRUE(BRepCheck_Analyzer(fanResult.shape, true, false, true).IsValid());
    OcctKernel kernel;
    std::ostringstream stream;
    BRepTools::Write(limited, stream);
    const auto serialized = stream.str();
    const std::vector<uint8_t> bytes(serialized.begin(), serialized.end());
    ImportTopologySeed seed;
    seed.feature_id = "root";
    seed.body_id = "body";
    seed.brep_sha256 = make_geometry_id(serialized).substr(7);
    const auto id = kernel.loadBrepr(bytes);
    const auto topology = kernel.getTopology(id);
    int allocation = 0;
    for (const auto& f : topology.faces)
        seed.identities.push_back(
            {"fixture-" + std::to_string(++allocation), PersistentTopologyType::face, f.local_id});
    for (const auto& e : topology.edges)
        seed.identities.push_back(
            {"fixture-" + std::to_string(++allocation), PersistentTopologyType::edge, e.local_id});
    for (const auto& v : topology.vertices)
        seed.identities.push_back({"fixture-" + std::to_string(++allocation),
                                   PersistentTopologyType::vertex, v.local_id});
    const auto initial = kernel.evaluateProfilePadsWithHistory({}, bytes, &seed);
    const auto mesh = kernel.tessellate(initial.geometry_id);
    ProfilePadSpec fillet;
    fillet.feature_id = "root-fillet";
    fillet.body_id = "body";
    fillet.input_feature_id = "root";
    fillet.generator = "FILLET";
    for (const auto& output : initial.feature_results.back().semantic_outputs) {
        if (output.topology_type != PersistentTopologyType::edge)
            continue;
        const auto it = std::find_if(mesh.edges.begin(), mesh.edges.end(),
                                     [&](const auto& e) { return e.local_id == output.local_id; });
        if (it != mesh.edges.end() && it->points.size() > 1 &&
            std::all_of(it->points.begin(), it->points.end(), [&](const auto& v) {
                const auto local = gp_Pnt(v.x, v.y, v.z).Transformed(inv);
                return std::abs(local.Y()) < 1e-7 && std::abs(local.Z() - half) < 1e-7;
            })) {
            const auto& r = output.semantic_ref;
            fillet.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
        }
    }
    ASSERT_EQ(fillet.selections.size(), 1);
    std::string accepted;
    for (double radius : {12., 15., 16., 15.}) {
        fillet.pad_length = radius;
        const auto evaluated = kernel.evaluateProfilePadsWithHistory({fillet}, bytes, &seed);
        ASSERT_TRUE(evaluated.feature_results.back().topology_history_complete);
        if (radius == 15) {
            if (accepted.empty())
                accepted = evaluated.feature_results.back().topology_history.evidence_digest;
            else
                EXPECT_EQ(accepted,
                          evaluated.feature_results.back().topology_history.evidence_digest);
        }
        OcctKernel cold;
        const auto reopened = cold.evaluateProfilePadsWithHistory({fillet}, bytes, &seed);
        EXPECT_EQ(evaluated.feature_results.back().topology_history.evidence_digest,
                  reopened.feature_results.back().topology_history.evidence_digest);
    }
    EXPECT_EQ(kernel.serializeBrepr(id), bytes);
    // The surviving hub/blade contact lines are tangent to the radius circle.
    const auto radial1 = gp_Dir(gp_Vec(center, t1)), radial2 = gp_Dir(gp_Vec(center, t2));
    EXPECT_NEAR(std::abs(radial1.Dot(gp_Dir(0, 1, 0))), 1, 1e-7);
    EXPECT_NEAR(std::abs(radial2.Dot(gp_Dir(0, -0.5, std::cos(M_PI / 6)))), 1, 1e-7);
}

TEST(FilletBoundary, PanelFeatureHistoryAndParameterRoundTrip) {
    const auto volume = [](const TopoDS_Shape& shape) {
        GProp_GProps p;
        BRepGProp::VolumeProperties(shape, p);
        return p.Mass();
    };
    const auto read = [](const std::vector<uint8_t>& bytes) {
        TopoDS_Shape s;
        BRep_Builder b;
        std::istringstream in(std::string(bytes.begin(), bytes.end()));
        BRepTools::Read(s, in, b);
        return s;
    };
    for (int pose = 0; pose < 2; ++pose) {
        OcctKernel kernel;
        ProfilePadSpec base;
        base.feature_id = "base";
        base.body_id = "body";
        base.profile_feature_id = "sketch";
        base.pad_length = 2.2;
        ProfileRegionSpec region;
        region.id = "region";
        region.outer.id = "outer";
        const std::vector<Vec2> corners = {{0, 0}, {60, 0}, {60, 30}, {0, 30}};
        for (size_t i = 0; i < 4; ++i) {
            ProfileCurveSpec c;
            c.entity_id = "edge-" + std::to_string(i);
            c.kind = "LINE";
            c.start = corners[i];
            c.end = corners[(i + 1) % 4];
            region.outer.curves.push_back(c);
        }
        base.regions = {region};
        gp_Trsf tr;
        if (pose) {
            tr.SetRotation(gp_Ax1(gp_Pnt(0, 0, 0), gp_Dir(1, 2, 3)), 0.7);
            tr.SetTranslationPart(gp_Vec(7, -11, 5));
        }
        auto o = gp_Pnt(0, 0, 0).Transformed(tr);
        auto u = gp_Dir(1, 0, 0).Transformed(tr), n = gp_Dir(0, 0, 1).Transformed(tr);
        base.plane_origin = {o.X(), o.Y(), o.Z()};
        base.plane_u_direction = {u.X(), u.Y(), u.Z()};
        base.plane_normal = {n.X(), n.Y(), n.Z()};
        const auto initial = kernel.evaluateProfilePadsWithHistory({base});
        const auto bytes = kernel.serializeBrepr(initial.geometry_id);
        const auto topo = kernel.tessellate(initial.geometry_id);
        ProfilePadSpec fillet;
        fillet.feature_id = "fillet";
        fillet.body_id = "body";
        fillet.input_feature_id = "base";
        fillet.generator = "FILLET";
        for (const auto& output : initial.feature_results.back().semantic_outputs) {
            if (output.topology_type != PersistentTopologyType::edge)
                continue;
            const auto it = std::find_if(topo.edges.begin(), topo.edges.end(), [&](const auto& e) {
                return e.local_id == output.local_id;
            });
            if (it != topo.edges.end() && it->points.size() > 1 &&
                std::all_of(it->points.begin(), it->points.end(), [&](const auto& p) {
                    return std::abs(gp_Pnt(p.x, p.y, p.z).Transformed(tr.Inverted()).Z()) < 1e-7;
                })) {
                const auto& r = output.semantic_ref;
                fillet.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
            }
        }
        ASSERT_EQ(fillet.selections.size(), 4);
        for (double radius : {2., 2.2, 2.3, 3., 4., 3.}) {
            SCOPED_TRACE(radius);
            fillet.pad_length = radius;
            const auto result = kernel.evaluateProfilePadsWithHistory({base, fillet});
            const auto shape = read(kernel.serializeBrepr(result.geometry_id));
            ASSERT_TRUE(BRepCheck_Analyzer(shape).IsValid());
            ASSERT_TRUE(BRepAlgoAPI_Check(shape, false, true).IsValid());
            EXPECT_TRUE(result.feature_results.back().topology_history_complete);
            size_t generatedFaces = 0;
            for (const auto& output : result.feature_results.back().semantic_outputs)
                if (output.topology_type == PersistentTopologyType::face &&
                    output.semantic_ref.output_slot.rfind("MODIFIER_GENERATED/", 0) == 0)
                    ++generatedFaces;
            EXPECT_GE(generatedFaces, 4);
            int cylinders = 0;
            for (TopExp_Explorer it(shape, TopAbs_FACE); it.More(); it.Next()) {
                BRepAdaptor_Surface f(TopoDS::Face(it.Current()));
                if (f.GetType() == GeomAbs_Cylinder) {
                    ++cylinders;
                    EXPECT_NEAR(f.Cylinder().Radius(), radius, 1e-7);
                }
            }
            EXPECT_EQ(cylinders, 4);
            // Independent thick-stock reference is used only for verification.
            const auto stock = BRepPrimAPI_MakeBox(60, 30, 12).Shape();
            BRepFilletAPI_MakeFillet ref(stock);
            TopTools_IndexedMapOfShape edges;
            TopExp::MapShapes(stock, TopAbs_EDGE, edges);
            for (int i = 1; i <= edges.Extent(); ++i) {
                BRepAdaptor_Curve c(TopoDS::Edge(edges(i)));
                if (std::abs(c.Value(c.FirstParameter()).Z()) < 1e-8 &&
                    std::abs(c.Value(c.LastParameter()).Z()) < 1e-8)
                    ref.Add(radius, TopoDS::Edge(edges(i)));
            }
            ref.Build();
            ASSERT_TRUE(ref.IsDone());
            const auto expected =
                BRepAlgoAPI_Common(BRepBuilderAPI_Transform(ref.Shape(), tr, true).Shape(),
                                   read(bytes))
                    .Shape();
            EXPECT_NEAR(volume(shape), volume(expected), 1e-5);
            EXPECT_NEAR(volume(BRepAlgoAPI_Cut(shape, expected).Shape()), 0, 1e-5);
            EXPECT_NEAR(volume(BRepAlgoAPI_Cut(expected, shape).Shape()), 0, 1e-5);
            std::reverse(fillet.selections.begin(), fillet.selections.end());
            OcctKernel cold;
            const auto again = cold.evaluateProfilePadsWithHistory({base, fillet});
            EXPECT_NEAR(cold.getVolume(again.geometry_id), volume(shape), 1e-7);
            EXPECT_EQ(kernel.serializeBrepr(initial.geometry_id), bytes);
        }
    }
}
TEST(FilletBoundary, CylindricalSupportBeyondAxialLimit) {
    const auto input = BRepPrimAPI_MakeCylinder(20, 2.2).Shape();
    TopTools_IndexedMapOfShape edges;
    TopExp::MapShapes(input, TopAbs_EDGE, edges);
    std::vector<TopoDS_Shape> selected;
    for (int i = 1; i <= edges.Extent(); ++i) {
        BRepAdaptor_Curve c(TopoDS::Edge(edges(i)));
        if (c.GetType() == GeomAbs_Circle && std::abs(c.Circle().Location().Z()) < 1e-8)
            selected.push_back(edges(i));
    }
    ASSERT_EQ(selected.size(), 1);
    auto result = detail::rebuild_fillet_boundary(input, selected, 3);
    ASSERT_TRUE(result.IsDone());
    int tori = 0;
    for (TopExp_Explorer it(result.shape, TopAbs_FACE); it.More(); it.Next()) {
        BRepAdaptor_Surface f(TopoDS::Face(it.Current()));
        if (f.GetType() == GeomAbs_Torus) {
            ++tori;
            EXPECT_NEAR(f.Torus().MinorRadius(), 3, 1e-7);
        }
    }
    EXPECT_EQ(tori, 1);
    EXPECT_TRUE(BRepCheck_Analyzer(result.shape).IsValid());
}
}  // namespace occccad::kernel
namespace occccad::kernel {
TEST(FilletBoundary, RejectsNonTangentConsumedCornerPatches) {
    for (double cornerRadius : {0.5, 1.0}) {
        const auto box = BRepPrimAPI_MakeBox(60, 30, 2.2).Shape();
        TopTools_IndexedMapOfShape edges;
        TopExp::MapShapes(box, TopAbs_EDGE, edges);
        BRepFilletAPI_MakeFillet sides(box);
        for (int i = 1; i <= edges.Extent(); ++i) {
            BRepAdaptor_Curve c(TopoDS::Edge(edges(i)));
            if (std::abs(c.Value(c.FirstParameter()).Z() - c.Value(c.LastParameter()).Z()) > 2)
                sides.Add(cornerRadius, TopoDS::Edge(edges(i)));
        }
        sides.Build();
        ASSERT_TRUE(sides.IsDone());
        edges.Clear();
        TopExp::MapShapes(sides.Shape(), TopAbs_EDGE, edges);
        std::vector<TopoDS_Shape> selected;
        for (int i = 1; i <= edges.Extent(); ++i) {
            BRepAdaptor_Curve c(TopoDS::Edge(edges(i)));
            if (std::abs(c.Value(c.FirstParameter()).Z()) < 1e-7 &&
                std::abs(c.Value(c.LastParameter()).Z()) < 1e-7)
                selected.push_back(edges(i));
        }
        ASSERT_EQ(selected.size(), 8);
        // OCCT constructs closed patches here, but their boundary normals are
        // not tangent (4-7 degrees). Reject this known unsupported corner rather
        // than accepting IsDone, complete naming, or a visually plausible mesh.
        try {
            detail::rebuild_fillet_boundary(sides.Shape(), selected, 3);
            FAIL() << "accepted a non-tangent corner";
        } catch (const std::runtime_error& e) {
            EXPECT_EQ(std::string(e.what()), "FILLET_BOUNDARY_CORNER_TANGENCY_FAILED");
        }
    }
}
}  // namespace occccad::kernel

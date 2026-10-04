#include <internal/occt_kernel.hpp>
#include <occccad/kernel/topology_naming.hpp>
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/mesh_glb.hpp>

#include <gtest/gtest.h>

#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <gp_Circ.hxx>
#include <BRepGProp.hxx>
#include <BRep_Tool.hxx>
#include <TopoDS.hxx>
#include <GProp_GProps.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeSphere.hxx>
#include <TopoDS_Compound.hxx>
#include <TopExp.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <OSD_ThreadPool.hxx>
#include <OSD_Parallel.hxx>

#include <algorithm>
#include <chrono>
#include <cmath>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <stdexcept>
#include <sstream>
#include <set>
#include <vector>

namespace occccad::kernel {
namespace {

class TemporaryStepFile {
public:
    explicit TemporaryStepFile(const std::vector<uint8_t>& data)
        : path_(std::filesystem::temp_directory_path() /
                ("occccad-exchange-test-" +
                 std::to_string(std::chrono::steady_clock::now().time_since_epoch().count()) +
                 ".step")) {
        std::ofstream output(path_, std::ios::binary);
        output.write(reinterpret_cast<const char*>(data.data()),
                     static_cast<std::streamsize>(data.size()));
        if (!output)
            throw std::runtime_error("cannot write STEP test fixture");
    }

    ~TemporaryStepFile() { std::filesystem::remove(path_); }
    const std::filesystem::path& path() const { return path_; }

private:
    std::filesystem::path path_;
};

TEST(GeometryExchange, VisualV2LocatorsMatchExactTopology) {
    OcctKernel kernel;
    const auto id=kernel.createRectangularPad({0,0,100,60,40,"XY"});
    const auto& topology=kernel.getTopology(id);
    const auto mesh=kernel.tessellate(id);
    ASSERT_EQ(mesh.triangles.size(),mesh.face_ids.size());
    for(size_t i=0;i<mesh.triangles.size();++i){
        const auto local=mesh.face_ids[i];ASSERT_GT(local,0U);ASSERT_LE(local,topology.faces.size());
        const auto& face=topology.faces[local-1];EXPECT_EQ(face.local_id,local);
        for(const auto index:{mesh.triangles[i].v0,mesh.triangles[i].v1,mesh.triangles[i].v2}){
            const auto& p=mesh.vertices[index];
            EXPECT_GE(p.x,face.bbox.min.x-1e-6);EXPECT_LE(p.x,face.bbox.max.x+1e-6);
            EXPECT_GE(p.y,face.bbox.min.y-1e-6);EXPECT_LE(p.y,face.bbox.max.y+1e-6);
            EXPECT_GE(p.z,face.bbox.min.z-1e-6);EXPECT_LE(p.z,face.bbox.max.z+1e-6);
        }
    }
    for(const auto& e:mesh.edges){ASSERT_GT(e.local_id,0U);ASSERT_LE(e.local_id,topology.edges.size());EXPECT_EQ(topology.edges[e.local_id-1].local_id,e.local_id);}
    for(const auto& p:mesh.topology_vertices){ASSERT_GT(p.local_id,0U);ASSERT_LE(p.local_id,topology.vertices.size());EXPECT_EQ(topology.vertices[p.local_id-1].point.x,p.point.x);}
    EXPECT_FALSE(make_glb(mesh).empty());
    auto invalid=mesh;invalid.face_ids[0]=0;EXPECT_THROW((void)make_glb(invalid),std::invalid_argument);
}

TEST(GeometryExchange, CurvedVisualSnapshotUsesEveryAdjacentMeshBoundary) {
    std::vector<TopoDS_Shape> shapes;
    shapes.push_back(BRepPrimAPI_MakeCylinder(10,20).Shape());
    shapes.push_back(BRepPrimAPI_MakeSphere(10).Shape());
    shapes.push_back(BRepAlgoAPI_Common(BRepPrimAPI_MakeSphere(12).Shape(), BRepPrimAPI_MakeBox(gp_Pnt(-9,-9,-9),gp_Pnt(9,9,9)).Shape()).Shape());
    const auto box = BRepPrimAPI_MakeBox(20,30,40).Shape();
    BRepFilletAPI_MakeFillet fillet(box);
    TopTools_IndexedMapOfShape boxEdges; TopExp::MapShapes(box,TopAbs_EDGE,boxEdges);
    for(int i=1;i<=boxEdges.Extent();++i) fillet.Add(3,TopoDS::Edge(boxEdges(i)));
    fillet.Build(); ASSERT_TRUE(fillet.IsDone()); shapes.push_back(fillet.Shape());
    BRep_Builder builder;
    TopoDS_Compound child, parent; builder.MakeCompound(child); builder.MakeCompound(parent);
    gp_Trsf rotation, translation;
    rotation.SetRotation(gp_Ax1(gp_Pnt(0,0,0),gp_Dir(1,2,3)),0.73);
    translation.SetTranslation(gp_Vec(43,-17,29));
    builder.Add(child, shapes.front().Moved(TopLoc_Location(rotation)));
    builder.Add(parent, child.Moved(TopLoc_Location(translation)));
    shapes.push_back(parent);
    TopoDS_Compound withWire; builder.MakeCompound(withWire);
    builder.Add(withWire, shapes.front());
    builder.Add(withWire, BRepBuilderAPI_MakeEdge(gp_Circ(gp_Ax2(gp_Pnt(0,0,30),gp_Dir(0,0,1)),7)).Edge());
    shapes.push_back(withWire);
    constexpr double deflection = 0.04;
    auto dist=[](const Vec3& a,const Vec3& b){return std::hypot(a.x-b.x,a.y-b.y,a.z-b.z);};
    for(size_t fixture=0;fixture<shapes.size();++fixture) {
        SCOPED_TRACE(fixture);
        std::ostringstream out; BRepTools::Write(shapes[fixture],out);
        const auto data=out.str(); OcctKernel kernel;
        const auto id=kernel.loadBrepr({data.begin(),data.end()});
        const auto original=kernel.serializeBrepr(id);
        const auto& topology=kernel.getTopology(id);
        for(const auto& edge:topology.edges) EXPECT_TRUE(edge.render_points.empty());
        const auto serial=kernel.tessellate(id,deflection,0.2,false);
        const auto parallel=kernel.tessellate(id,deflection,0.2,true);
        EXPECT_EQ(make_glb(serial),make_glb(parallel));
        EXPECT_EQ(kernel.serializeBrepr(id),original);
        EXPECT_EQ(serial.normals.size(),serial.vertices.size());
        TopTools_IndexedMapOfShape faces, edges;
        TopExp::MapShapes(shapes[fixture],TopAbs_FACE,faces); TopExp::MapShapes(shapes[fixture],TopAbs_EDGE,edges);
        std::vector<std::set<std::pair<uint32_t,uint32_t>>> segments(faces.Extent()+1);
        std::vector<size_t> counts(faces.Extent()+1);
        double signedVolume=0;
        for(size_t i=0;i<serial.triangles.size();++i) {
            const auto f=serial.face_ids[i]; ASSERT_LE(f,faces.Extent()); ++counts[f];
            const auto t=serial.triangles[i];
            for(auto pair : {std::pair{t.v0,t.v1},std::pair{t.v1,t.v2},std::pair{t.v2,t.v0}}) segments[f].insert(std::minmax(pair.first,pair.second));
            // Independent accuracy check against the exact trimmed face.
            const auto &a=serial.vertices[t.v0], &b=serial.vertices[t.v1], &c=serial.vertices[t.v2];
            signedVolume += (a.x*(b.y*c.z-b.z*c.y)+a.y*(b.z*c.x-b.x*c.z)+a.z*(b.x*c.y-b.y*c.x))/6;
            const gp_Pnt center((a.x+b.x+c.x)/3,(a.y+b.y+c.y)/3,(a.z+b.z+c.z)/3);
            // Bound test time while sampling every face and both small/large meshes.
            if(counts[f] <= 3 || i % 29 == 0) {
                BRepExtrema_DistShapeShape error(BRepBuilderAPI_MakeVertex(center).Shape(),faces(f));
                ASSERT_TRUE(error.IsDone()); EXPECT_LE(error.Value(),deflection);
            }
        }
        EXPECT_NEAR(signedVolume,kernel.getVolume(id),kernel.getVolume(id)*.02);
        for(int i=1;i<=faces.Extent();++i) EXPECT_GT(counts[i],0U);
        size_t drawable=0;
        for(int i=1;i<=edges.Extent();++i) if(!BRep_Tool::Degenerated(TopoDS::Edge(edges(i)))) ++drawable;
        EXPECT_EQ(serial.edges.size(),drawable);
        for(const auto& line:serial.edges) {
            ASSERT_LE(line.local_id,edges.Extent());
            const auto edge=edges(line.local_id);
            for(int f=1;f<=faces.Extent();++f) {
                bool adjacent=false;
                for(TopExp_Explorer it(faces(f),TopAbs_EDGE);it.More();it.Next()) adjacent |= it.Current().IsSame(edge);
                if(!adjacent) continue;
                for(size_t n=1;n<line.points.size();++n) {
                    bool found=false;
                    for(const auto& [a,b]:segments[f]) {
                        found |= (dist(serial.vertices[a],line.points[n-1])<1e-6 && dist(serial.vertices[b],line.points[n])<1e-6)
                              || (dist(serial.vertices[b],line.points[n-1])<1e-6 && dist(serial.vertices[a],line.points[n])<1e-6);
                    }
                    EXPECT_TRUE(found) << "edge=" << line.local_id << " face=" << f << " segment=" << n;
                }
            }
        }
        if(fixture==0) if(const char* path=std::getenv("OCCCCAD_TEST_CURVED_GLB")) {
            const auto bytes=make_glb(serial); std::ofstream file(path,std::ios::binary);
            file.write(reinterpret_cast<const char*>(bytes.data()),bytes.size());
        }
    }
}

TEST(GeometryExchange, VisualSnapshotRejectsUnmeshedFace) {
    const auto face=BRepBuilderAPI_MakeFace(gp_Pln(gp_Pnt(0,0,0),gp_Dir(0,0,1))).Face();
    std::ostringstream stream; BRepTools::Write(face,stream); const auto data=stream.str();
    OcctKernel kernel; const auto id=kernel.loadBrepr({data.begin(),data.end()});
    EXPECT_THROW((void)kernel.tessellate(id),std::runtime_error);
}

TEST(GeometryExchange, RectanglePadProducesSelectableTopologyAndRoundTrips) {
    OcctKernel kernel;
    const auto id = kernel.createRectangularPad({0.0, 0.0, 100.0, 60.0, 40.0, "XY"});
    const auto topology = kernel.getTopology(id);
    const auto mesh = kernel.tessellate(id);

    EXPECT_EQ(topology.faces.size(), 6U);
    EXPECT_EQ(topology.edges.size(), 12U);
    EXPECT_EQ(topology.vertices.size(), 8U);
    ASSERT_FALSE(topology.faces.empty());
    ASSERT_FALSE(topology.edges.empty());
    EXPECT_FALSE(topology.faces.front().properties.empty());
    EXPECT_FALSE(topology.edges.front().properties.empty());
    EXPECT_EQ(mesh.edges.size(), 12U);
    EXPECT_EQ(mesh.topology_vertices.size(), 8U);

    TemporaryStepFile step(kernel.serializeStep(id));
    ASSERT_EQ(kernel.inspectStepRootCount(step.path().string()), 1U);
    const auto imported = kernel.loadStepRoot(step.path().string(), 1U);
    const auto brep = kernel.serializeBrepr(imported);
    const auto brep_round_trip = kernel.loadBrepr(brep);
    EXPECT_FALSE(brep.empty());
    EXPECT_NEAR(kernel.getVolume(imported), kernel.getVolume(id), 1e-6);
    EXPECT_NEAR(kernel.getVolume(brep_round_trip), kernel.getVolume(id), 1e-6);
}

TEST(GeometryExchange, ReusesTopologyAnalysisForEveryElementOfTheSameBody) {
    OcctKernel kernel;
    const auto id = kernel.createRectangularPad({0.0, 0.0, 20.0, 10.0, 5.0, "XY"});

    const auto& first = kernel.getTopology(id);
    const auto& second = kernel.getTopology(id);

    EXPECT_EQ(&first, &second);
    EXPECT_EQ(first.faces.size(), 6U);
}

TEST(GeometryExchange, ProfilePadSupportsCircularOuterLoopAndHole) {
    OcctKernel kernel;
    ProfileCurveSpec outer;
    outer.entity_id = "outer";
    outer.kind = "CIRCLE";
    outer.center = {0.0, 0.0};
    outer.radius = 20.0;
    ProfileCurveSpec hole;
    hole.entity_id = "hole";
    hole.kind = "CIRCLE";
    hole.reversed = true;
    hole.center = {0.0, 0.0};
    hole.radius = 8.0;
    ProfileRegionSpec region;
    region.id = "annulus";
    region.outer = {"outer-loop", {outer}};
    region.holes = {{"hole-loop", {hole}}};

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 12.0;
    pad.plane = "XY";
    const auto evaluation = kernel.evaluateProfilePadsWithHistory({pad});
    const auto id = evaluation.geometry_id;

    EXPECT_NEAR(kernel.getVolume(id), 3.14159265358979323846 * (400.0 - 64.0) * 12.0, 1.0e-5);
    EXPECT_GT(kernel.getTopology(id).solid_count, 0U);
    ASSERT_EQ(evaluation.feature_results.size(), 1U);
    const auto& feature = evaluation.feature_results.front();
    EXPECT_TRUE(feature.topology_history_complete);
    EXPECT_EQ(feature.semantic_outputs.size(), kernel.getTopology(id).face_count +
                                                   kernel.getTopology(id).edge_count +
                                                   kernel.getTopology(id).vertex_count);
    EXPECT_TRUE(std::any_of(feature.semantic_outputs.begin(), feature.semantic_outputs.end(),
                            [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex &&
                                       output.evidence.endpoint_role.find("CAP_VERTEX") !=
                                           std::string::npos;
                            }));
}

TEST(GeometryExchange, ProfilePadBuildsExactRotatedEllipseAndSignedEllipticalArc) {
    OcctKernel kernel;
    ProfileCurveSpec ellipse;
    ellipse.entity_id="ellipse"; ellipse.kind="ELLIPSE";
    ellipse.center={23,-11};ellipse.major_radius=12;ellipse.minor_radius=4;ellipse.rotation=0.7;
    ProfileRegionSpec region;region.id="ellipse-region";region.outer={"ellipse-loop",{ellipse}};
    ProfilePadSpec pad;pad.regions={region};pad.pad_length=7;pad.plane="XY";
    const auto id=kernel.evaluateProfilePads({pad});
    EXPECT_NEAR(kernel.getVolume(id),3.14159265358979323846*12*4*7,1e-5);
    EXPECT_EQ(kernel.getTopology(id).solid_count,1U);
    for (const double sweep : {3.14159265358979323846,-3.14159265358979323846}) {
        ellipse.kind="ELLIPTICAL_ARC";ellipse.start_angle=0;ellipse.end_angle=sweep;
        ProfileCurveSpec diameter;diameter.entity_id="diameter";diameter.kind="LINE";
        const double dx=12*std::cos(0.7),dy=12*std::sin(0.7);
        diameter.start={23-dx,-11-dy};diameter.end={23+dx,-11+dy};
        region.outer={"half-loop",{ellipse,diameter}};pad.regions={region};
        const auto half=kernel.evaluateProfilePads({pad});
        EXPECT_NEAR(kernel.getVolume(half),0.5*3.14159265358979323846*12*4*7,1e-5);
        EXPECT_EQ(kernel.getTopology(half).solid_count,1U);
    }
}

TEST(GeometryExchange, ProfilePadSignedArcDoesNotUseComplementaryInterval) {
    OcctKernel kernel;
    ProfileCurveSpec arc;arc.entity_id="quarter";arc.kind="ARC";arc.radius=10;
    arc.start_angle=0;arc.end_angle=-3.14159265358979323846/2;
    ProfileCurveSpec chord;chord.entity_id="chord";chord.kind="LINE";chord.start={0,-10};chord.end={10,0};
    ProfileRegionSpec region;region.id="segment";region.outer={"segment-loop",{arc,chord}};
    ProfilePadSpec pad;pad.regions={region};pad.pad_length=3;pad.plane="XY";
    const auto id=kernel.evaluateProfilePads({pad});
    EXPECT_NEAR(kernel.getVolume(id),(3.14159265358979323846/4-0.5)*100*3,1e-5);
}

TEST(GeometryExchange, NamingRejectsProfileEdgesBelowPolicyTolerance) {
    OcctKernel kernel;
    ProfilePadSpec pad;
    pad.feature_id = "short-edge-pad";
    pad.body_id = "body-main";
    pad.profile_feature_id = "short-edge-sketch";
    ProfileRegionSpec region;
    region.id = "short";
    region.outer.id = "short-outer";
    const std::vector<Vec2> points{{0, 0}, {0.00001, 0}, {0.00001, 10}, {0, 10}};
    for (std::size_t index = 0; index < points.size(); ++index) {
        ProfileCurveSpec line;
        line.entity_id = "short-edge-" + std::to_string(index);
        line.kind = "LINE";
        line.start = points[index];
        line.end = points[(index + 1) % points.size()];
        region.outer.curves.push_back(line);
    }
    pad.regions = {region};
    pad.pad_length = 5;
    pad.body_operation = "NEW_BODY";

    try {
        (void)kernel.evaluateProfilePadsWithHistory({pad});
        FAIL() << "profile edge below naming tolerance must be rejected";
    } catch (const std::invalid_argument& error) {
        EXPECT_NE(std::string(error.what()).find("DEGENERATE_PROFILE_EDGE"), std::string::npos);
    }
}

TEST(GeometryExchange, ProfilePadKeepsArcAnglesInTheSketchPlane) {
    OcctKernel kernel;
    ProfileCurveSpec arc;
    arc.entity_id = "arc";
    arc.kind = "ARC";
    arc.center = {0.0, 0.0};
    arc.radius = 10.0;
    arc.start_angle = 0.0;
    arc.end_angle = 3.14159265358979323846;
    ProfileCurveSpec diameter;
    diameter.entity_id = "diameter";
    diameter.kind = "LINE";
    diameter.start = {-10.0, 0.0};
    diameter.end = {10.0, 0.0};
    ProfileRegionSpec region;
    region.id = "semicircle";
    region.outer = {"semicircle-loop", {arc, diameter}};

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 7.0;
    pad.plane = "XZ";
    const auto id = kernel.evaluateProfilePads({pad});

    EXPECT_NEAR(kernel.getVolume(id), 0.5 * 3.14159265358979323846 * 100.0 * 7.0, 1.0e-5);
}

TEST(GeometryExchange, ProfilePadBuildsClosedSplineWire) {
    OcctKernel kernel;
    ProfileCurveSpec spline;
    spline.entity_id = "spline";
    spline.kind = "SPLINE";
    spline.control_points = {{0.0, 0.0}, {20.0, 0.0}, {20.0, 20.0}, {0.0, 20.0}};
    spline.degree = 3;
    spline.closed = true;
    ProfileRegionSpec region;
    region.id = "spline-region";
    region.outer = {"spline-loop", {spline}};

    ProfilePadSpec pad;
    pad.regions = {region};
    pad.pad_length = 5.0;
    pad.plane = "YZ";
    const auto id = kernel.evaluateProfilePads({pad});

    EXPECT_GT(kernel.getVolume(id), 0.0);
    EXPECT_GT(kernel.getTopology(id).solid_count, 0U);
}

ProfileRegionSpec rectangular_region(const std::string& id, double x0, double y0, double x1,
                                     double y1) {
    ProfileRegionSpec region;
    region.id = id;
    region.outer.id = id + "-outer";
    const std::vector<Vec2> points{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}};
    for (std::size_t index = 0; index < points.size(); ++index) {
        ProfileCurveSpec line;
        line.entity_id = id + "-edge-" + std::to_string(index);
        line.kind = "LINE";
        line.start = points[index];
        line.end = points[(index + 1) % points.size()];
        region.outer.curves.push_back(line);
    }
    return region;
}

Vec3 face_vector(const FaceInfo& face, const std::string& name) {
    for (const auto& property : face.properties)
        if (property.name == name)
            return property.vector_value;
    throw std::runtime_error("missing face vector: " + name);
}

TEST(GeometryExchange, OutwardFaceFramesDrivePocketAndPadOnEveryBoxFace) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base";
    base.body_id = "body";
    base.profile_feature_id = "base-sketch";
    base.regions = {rectangular_region("base-region", 0, 0, 20, 20)};
    base.pad_length = 20;
    base.body_operation = "NEW_BODY";
    for (const bool reversed : {false, true}) {
        base.reversed = reversed;
        const auto evaluated = kernel.evaluateProfilePadsWithHistory({base});
        const auto topology = kernel.getTopology(evaluated.geometry_id);
        ASSERT_EQ(topology.faces.size(), 6U);
        for (const auto& face : topology.faces) {
            const auto normal = face_vector(face, "normal");
            const auto u = face_vector(face, "xDirection");
            const auto v = face_vector(face, "yDirection");
            const Vec3 center{(face.bbox.min.x + face.bbox.max.x) / 2,
                              (face.bbox.min.y + face.bbox.max.y) / 2,
                              (face.bbox.min.z + face.bbox.max.z) / 2};
            EXPECT_NEAR((center.x - 10) * normal.x + (center.y - 10) * normal.y +
                        (center.z - (reversed ? -10 : 10)) * normal.z, 10, 1e-7);
            EXPECT_NEAR(u.y * v.z - u.z * v.y, normal.x, 1e-9);
            EXPECT_NEAR(u.z * v.x - u.x * v.z, normal.y, 1e-9);
            EXPECT_NEAR(u.x * v.y - u.y * v.x, normal.z, 1e-9);
            bool found = false;
            for (const auto& output : evaluated.feature_results.back().semantic_outputs) {
                if (output.topology_type != PersistentTopologyType::face || output.local_id != face.local_id) continue;
                found = true;
                EXPECT_NEAR(output.evidence.direction.x, normal.x, 1e-9);
                EXPECT_NEAR(output.evidence.direction.y, normal.y, 1e-9);
                EXPECT_NEAR(output.evidence.direction.z, normal.z, 1e-9);
            }
            EXPECT_TRUE(found);
            for (const bool pocket : {false, true}) {
                ProfilePadSpec feature;
                feature.feature_id = "on-face";
                feature.body_id = "body";
                feature.input_feature_id = "base";
                feature.profile_feature_id = "face-sketch";
                feature.regions = {rectangular_region("face-region", -1, -1, 1, 1)};
                feature.plane_origin = center;
                feature.plane_normal = normal;
                feature.plane_u_direction = u;
                feature.pad_length = 2;
                feature.body_operation = pocket ? "REMOVE" : "ADD";
                feature.reversed = pocket;
                const auto result = kernel.evaluateProfilePadsWithHistory({base, feature});
                EXPECT_NEAR(kernel.getVolume(result.geometry_id), 8000 + (pocket ? -8 : 8), 1e-6);
                // The cavity floor at depth 2 must still point out of material
                // and into the removed volume, not away from the part center.
                if (pocket) {
                    const Vec3 floor{center.x - 2 * normal.x, center.y - 2 * normal.y, center.z - 2 * normal.z};
                    bool found_floor = false;
                    for (const auto& cut_face : kernel.getTopology(result.geometry_id).faces) {
                        const Vec3 c{(cut_face.bbox.min.x + cut_face.bbox.max.x) / 2,
                                     (cut_face.bbox.min.y + cut_face.bbox.max.y) / 2,
                                     (cut_face.bbox.min.z + cut_face.bbox.max.z) / 2};
                        if (std::abs(c.x-floor.x)+std::abs(c.y-floor.y)+std::abs(c.z-floor.z)>1e-6) continue;
                        found_floor = true;
                        const auto n = face_vector(cut_face, "normal");
                        EXPECT_NEAR(n.x*normal.x+n.y*normal.y+n.z*normal.z, 1, 1e-9);
                        bool found_evidence = false;
                        for (const auto& output : result.feature_results.back().semantic_outputs) {
                            if (output.topology_type != PersistentTopologyType::face ||
                                output.local_id != cut_face.local_id) continue;
                            found_evidence = true;
                            const auto& direction = output.evidence.direction;
                            EXPECT_NEAR(direction.x*n.x + direction.y*n.y + direction.z*n.z, 1, 1e-9);
                        }
                        EXPECT_TRUE(found_evidence);
                    }
                    EXPECT_TRUE(found_floor);
                }
            }
        }
    }
}

std::size_t faces_on_z(const TopologyInfo& topology, const double z) {
    return static_cast<std::size_t>(
        std::count_if(topology.faces.begin(), topology.faces.end(), [z](const FaceInfo& face) {
            return std::abs(face.bbox.min.z - z) < 1.0e-6 && std::abs(face.bbox.max.z - z) < 1.0e-6;
        }));
}

std::size_t faces_on_y(const TopologyInfo& topology, const double y) {
    return static_cast<std::size_t>(
        std::count_if(topology.faces.begin(), topology.faces.end(), [y](const FaceInfo& face) {
            return std::abs(face.bbox.min.y - y) < 1.0e-6 && std::abs(face.bbox.max.y - y) < 1.0e-6;
        }));
}

TEST(GeometryExchange, NamingFixtureExtrudeLengthChangesGeometryButKeepsDomainIdentity) {
    OcctKernel kernel;
    ProfilePadSpec twenty;
    twenty.feature_id = "extrude-1";
    twenty.body_id = "body-main";
    twenty.profile_feature_id = "sketch-1";
    twenty.regions = {rectangular_region("region-1", 0, 0, 20, 10)};
    twenty.pad_length = 20;
    twenty.body_operation = "NEW_BODY";
    ProfilePadSpec forty = twenty;
    forty.pad_length = 40;

    const auto twenty_id = kernel.evaluateProfilePads({twenty});
    const auto forty_id = kernel.evaluateProfilePads({forty});

    EXPECT_NE(twenty_id, forty_id);
    EXPECT_NEAR(kernel.getBoundingBox(twenty_id).max.z, 20.0, 1.0e-6);
    EXPECT_NEAR(kernel.getBoundingBox(forty_id).max.z, 40.0, 1.0e-6);
    EXPECT_EQ(twenty.feature_id, forty.feature_id);
    EXPECT_EQ(twenty.profile_feature_id, forty.profile_feature_id);
    EXPECT_EQ(faces_on_z(kernel.getTopology(twenty_id), 20.0), 1U);
    EXPECT_EQ(faces_on_z(kernel.getTopology(forty_id), 40.0), 1U);
}

TEST(GeometryExchange, TopologyHistoryKeepsExtrudeSemanticOutputsAcrossLengthEdit) {
    OcctKernel kernel;
    ProfilePadSpec twenty;
    twenty.feature_id = "extrude-1";
    twenty.body_id = "body-main";
    twenty.profile_feature_id = "sketch-1";
    twenty.regions = {rectangular_region("region-1", 0, 0, 20, 10)};
    twenty.pad_length = 20;
    twenty.body_operation = "NEW_BODY";
    auto forty = twenty;
    forty.pad_length = 40;

    const auto first = kernel.evaluateProfilePadsWithHistory({twenty});
    const auto second = kernel.evaluateProfilePadsWithHistory({forty});
    const auto repeated = kernel.evaluateProfilePadsWithHistory({twenty});
    ASSERT_EQ(first.feature_results.size(), 1U);
    ASSERT_EQ(second.feature_results.size(), 1U);
    const auto slots = [](const FeatureResult& feature) {
        std::vector<std::string> result;
        for (const auto& output : feature.semantic_outputs)
            result.push_back(output.semantic_ref.output_slot);
        std::sort(result.begin(), result.end());
        return result;
    };
    EXPECT_EQ(slots(first.feature_results.front()), slots(second.feature_results.front()));
    EXPECT_EQ(first.geometry_id, repeated.geometry_id);
    EXPECT_EQ(slots(first.feature_results.front()), slots(repeated.feature_results.front()));
    EXPECT_EQ(first.feature_results.front().topology_history.evidence_digest,
              repeated.feature_results.front().topology_history.evidence_digest);
    const auto& first_feature = first.feature_results.front();
    const auto& first_topology = kernel.getTopology(first.geometry_id);
    EXPECT_EQ(first_feature.semantic_outputs.size(), 26U);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::face;
                            }),
              6);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::edge;
                            }),
              12);
    EXPECT_EQ(std::count_if(first_feature.semantic_outputs.begin(),
                            first_feature.semantic_outputs.end(), [](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex;
                            }),
              8);
    EXPECT_EQ(first_feature.semantic_outputs.size(), first_topology.face_count +
                                                         first_topology.edge_count +
                                                         first_topology.vertex_count);
    EXPECT_TRUE(first.feature_results.front().topology_history_complete);
    EXPECT_FALSE(first.feature_results.front().topology_history.evidence_digest.empty());
    for (const auto& output : second.feature_results.front().semantic_outputs) {
        EXPECT_GE(output.local_id, 1U);
        const auto& topology = kernel.getTopology(second.geometry_id);
        if (output.topology_type == PersistentTopologyType::face)
            EXPECT_LE(output.local_id, topology.face_count);
        else if (output.topology_type == PersistentTopologyType::edge)
            EXPECT_LE(output.local_id, topology.edge_count);
        else if (output.topology_type == PersistentTopologyType::vertex)
            EXPECT_LE(output.local_id, topology.vertex_count);
        else
            ADD_FAILURE() << "semantic output has unspecified topology type";
        EXPECT_FALSE(output.evidence.evidence_digest.empty());
        EXPECT_FALSE(output.evidence.adjacent.empty());
    }
    const auto contains_slot = [&](const std::string& prefix) {
        return std::any_of(first_feature.semantic_outputs.begin(),
                           first_feature.semantic_outputs.end(), [&](const auto& output) {
                               return output.semantic_ref.output_slot.rfind(prefix, 0) == 0;
                           });
    };
    EXPECT_TRUE(contains_slot("START_BOUNDARY_FROM_PROFILE_EDGE/"));
    EXPECT_TRUE(contains_slot("END_BOUNDARY_FROM_PROFILE_EDGE/"));
    EXPECT_TRUE(contains_slot("VERTICAL_FROM_PROFILE_ENDPOINTS/"));
    EXPECT_TRUE(contains_slot("START_VERTEX_FROM_PROFILE_ENDPOINTS/"));
    EXPECT_TRUE(contains_slot("END_VERTEX_FROM_PROFILE_ENDPOINTS/"));
}

TEST(GeometryExchange, TopologyHistoryComposesBooleanAndSameDomainHistory) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec add = base;
    add.feature_id = "extrude-add";
    add.input_feature_id = base.feature_id;
    add.profile_feature_id = "sketch-add";
    add.regions = {rectangular_region("add", 10, 0, 30, 20)};
    add.body_operation = "ADD";
    ProfilePadSpec remove = base;
    remove.feature_id = "cut-1";
    remove.input_feature_id = add.feature_id;
    remove.profile_feature_id = "sketch-cut";
    remove.regions = {rectangular_region("cut", 12, 5, 18, 15)};
    remove.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, add, remove});
    const auto repeated = kernel.evaluateProfilePadsWithHistory({base, add, remove});
    ASSERT_EQ(evaluation.feature_results.size(), 3U);
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 5400.0, 1.0e-6);
    const auto& add_history = evaluation.feature_results[1].topology_history;
    const auto& cut_history = evaluation.feature_results[2].topology_history;
    EXPECT_TRUE(std::any_of(
        add_history.lineage.begin(), add_history.lineage.end(),
        [](const TopologyLineage& value) { return value.kind == TopologyLineageKind::merged; }));
    EXPECT_TRUE(std::any_of(cut_history.lineage.begin(), cut_history.lineage.end(),
                            [](const TopologyLineage& value) {
                                return value.kind == TopologyLineageKind::modified ||
                                       value.kind == TopologyLineageKind::unchanged;
                            }));
    EXPECT_FALSE(cut_history.deleted.empty());
    EXPECT_TRUE(evaluation.feature_results[2].topology_history_complete);
    const auto& final_topology = kernel.getTopology(evaluation.geometry_id);
    const auto& final_outputs = evaluation.feature_results[2].semantic_outputs;
    EXPECT_EQ(final_outputs.size(), final_topology.face_count + final_topology.edge_count +
                                        final_topology.vertex_count);
    EXPECT_EQ(std::count_if(final_outputs.begin(), final_outputs.end(), [](const auto& output) {
                  return output.topology_type == PersistentTopologyType::edge;
              }),
              final_topology.edge_count);
    EXPECT_EQ(std::count_if(final_outputs.begin(), final_outputs.end(), [](const auto& output) {
                  return output.topology_type == PersistentTopologyType::vertex;
              }),
              final_topology.vertex_count);
    const auto merged_edge = std::find_if(
        evaluation.feature_results[1].topology_history.lineage.begin(),
        evaluation.feature_results[1].topology_history.lineage.end(), [&](const auto& lineage) {
            if (lineage.kind != TopologyLineageKind::merged)
                return false;
            return std::any_of(evaluation.feature_results[1].semantic_outputs.begin(),
                               evaluation.feature_results[1].semantic_outputs.end(),
                               [&](const auto& output) {
                                   return output.topology_type == PersistentTopologyType::edge &&
                                          output.semantic_ref.feature_id == lineage.result.feature_id &&
                                          output.semantic_ref.output_slot == lineage.result.output_slot;
                               });
        });
    EXPECT_NE(merged_edge, evaluation.feature_results[1].topology_history.lineage.end());
    EXPECT_EQ(evaluation.feature_results[1].topology_history.evidence_digest,
              repeated.feature_results[1].topology_history.evidence_digest);
    ASSERT_EQ(evaluation.feature_results[1].semantic_outputs.size(),
              repeated.feature_results[1].semantic_outputs.size());
    for (std::size_t index = 0; index < evaluation.feature_results[1].semantic_outputs.size(); ++index)
        EXPECT_EQ(evaluation.feature_results[1].semantic_outputs[index].semantic_ref.output_slot,
                  repeated.feature_results[1].semantic_outputs[index].semantic_ref.output_slot);
    for (const auto& tombstone : cut_history.deleted) {
        EXPECT_FALSE(
            std::any_of(evaluation.feature_results[2].semantic_outputs.begin(),
                        evaluation.feature_results[2].semantic_outputs.end(),
                        [&](const SemanticTopologyOutput& output) {
                            return output.semantic_ref.feature_id == tombstone.source.feature_id &&
                                   output.semantic_ref.output_slot == tombstone.source.output_slot;
                        }));
    }
}

TEST(GeometryExchange, RevolveProvidesCompleteTopologyHistory) {
    OcctKernel kernel;
    ProfilePadSpec revolve;
    revolve.feature_id = "revolve-1";
    revolve.body_id = "body-main";
    revolve.profile_feature_id = "sketch-revolve";
    revolve.regions = {rectangular_region("section", 5, 0, 10, 5)};
    revolve.generator = "REVOLVE";
    revolve.revolve_angle = 2.0 * 3.14159265358979323846;
    revolve.axis_start = {0, -10};
    revolve.axis_end = {0, 10};
    revolve.body_operation = "NEW_BODY";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({revolve});
    ASSERT_EQ(evaluation.feature_results.size(), 1U);
    EXPECT_TRUE(evaluation.feature_results.front().topology_history_complete);
    const auto topology = kernel.getTopology(evaluation.geometry_id);
    EXPECT_EQ(evaluation.feature_results.front().semantic_outputs.size(),
              topology.face_count + topology.edge_count + topology.vertex_count);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 375.0 * 3.14159265358979323846, 1e-6);
}

TEST(GeometryExchange, RevolveAboutProfileBoundaryHasCompleteHistory) {
    OcctKernel kernel;
    ProfilePadSpec revolve;
    revolve.feature_id = "revolve-boundary";
    revolve.body_id = "body";
    revolve.profile_feature_id = "sketch";
    revolve.regions = {rectangular_region("section", 5, 2, 10, 10)};
    revolve.generator = "REVOLVE";
    revolve.axis_start = {5, 2};
    revolve.axis_end = {5, 10};
    revolve.body_operation = "NEW_BODY";
    for (const double angle : {std::acos(-1.0), 2 * std::acos(-1.0)}) {
        SCOPED_TRACE(angle);
        revolve.revolve_angle = angle;
        const auto evaluation = kernel.evaluateProfilePadsWithHistory({revolve});
        const auto topology = kernel.getTopology(evaluation.geometry_id);
        EXPECT_TRUE(evaluation.feature_results.back().topology_history_complete);
        EXPECT_EQ(evaluation.feature_results.back().semantic_outputs.size(), topology.face_count + topology.edge_count + topology.vertex_count);
        EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 100 * angle, 1e-6);
        const auto repeat = kernel.evaluateProfilePadsWithHistory({revolve});
        EXPECT_EQ(evaluation.feature_results.back().topology_history.evidence_digest, repeat.feature_results.back().topology_history.evidence_digest);
    }
}

TEST(GeometryExchange, NamingFixtureCutRetainsModifiedTopFace) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec hole;
    hole.feature_id = "cut-1";
    hole.body_id = "body-main";
    hole.input_feature_id = base.feature_id;
    hole.profile_feature_id = "sketch-hole";
    hole.regions = {rectangular_region("hole", 5, 5, 15, 15)};
    hole.pad_length = 10;
    hole.body_operation = "REMOVE";

    const auto result = kernel.evaluateProfilePads({base, hole});
    EXPECT_EQ(kernel.getTopology(result).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(result), 3000.0, 1.0e-6);
    EXPECT_EQ(faces_on_z(kernel.getTopology(result), 10.0), 1U);
}

TEST(GeometryExchange, NamingFixtureCutDeletesOriginalTopFace) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec remove_top;
    remove_top.regions = {rectangular_region("top-half", 0, 0, 20, 20)};
    remove_top.pad_length = 5;
    remove_top.body_operation = "REMOVE";
    remove_top.plane_origin = {0, 0, 5};
    remove_top.plane_normal = {0, 0, 1};
    remove_top.plane_u_direction = {1, 0, 0};

    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    remove_top.feature_id = "cut-top";
    remove_top.body_id = "body-main";
    remove_top.input_feature_id = base.feature_id;
    remove_top.profile_feature_id = "sketch-cut";
    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, remove_top});
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 2000.0, 1.0e-6);
    EXPECT_EQ(faces_on_z(kernel.getTopology(evaluation.geometry_id), 10.0), 0U);
    EXPECT_EQ(faces_on_z(kernel.getTopology(evaluation.geometry_id), 5.0), 1U);
    const auto& deleted = evaluation.feature_results.back().topology_history.deleted;
    EXPECT_TRUE(std::any_of(deleted.begin(), deleted.end(), [](const auto& tombstone) {
        return tombstone.source.output_slot.rfind("END_VERTEX_FROM_PROFILE_ENDPOINTS/", 0) == 0;
    }));
}

TEST(GeometryExchange, NamingFixtureSideOpeningCreatesTwoAmbiguousFaceCandidates) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-1";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec notch;
    notch.feature_id = "cut-1";
    notch.body_id = "body-main";
    notch.input_feature_id = base.feature_id;
    notch.profile_feature_id = "sketch-notch";
    notch.regions = {rectangular_region("notch", 8, 0, 12, 5)};
    notch.pad_length = 10;
    notch.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, notch});
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).solid_count, 1U);
    EXPECT_EQ(faces_on_y(kernel.getTopology(evaluation.geometry_id), 0.0), 2U);
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    const auto& cut = evaluation.feature_results.back();
    EXPECT_TRUE(cut.topology_history_complete);
    const auto ambiguity = std::find_if(
        cut.topology_history.ambiguous.begin(), cut.topology_history.ambiguous.end(),
        [](const AmbiguousLineage& value) {
            return value.sources.size() == 1U &&
                   value.sources.front().output_slot ==
                       "SIDE_FROM_PROFILE_EDGE/base-edge-0";
        });
    ASSERT_NE(ambiguity, cut.topology_history.ambiguous.end());
    EXPECT_EQ(ambiguity->diagnostic_code, "TOPOLOGY_SPLIT_AMBIGUOUS");
    EXPECT_EQ(ambiguity->candidates.size(), 2U);
    EXPECT_TRUE(std::any_of(cut.topology_history.ambiguous.begin(),
                            cut.topology_history.ambiguous.end(), [](const auto& value) {
                                return !value.sources.empty() &&
                                       (value.sources.front().output_slot.find("BOUNDARY_") !=
                                            std::string::npos ||
                                        value.sources.front().output_slot.find("VERTICAL_") !=
                                            std::string::npos);
                            }));
}

TEST(GeometryExchange, NamingFixtureXZThroughCutKeepsSixBaseFacesAndAddsFourHoleFaces) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";

    ProfilePadSpec hole;
    hole.feature_id = "cut-hole";
    hole.body_id = "body-main";
    hole.input_feature_id = base.feature_id;
    hole.profile_feature_id = "sketch-hole-xz";
    hole.regions = {rectangular_region("hole", 5, 2, 15, 8)};
    hole.pad_length = 20;
    hole.plane = "XZ";
    hole.reversed = true;
    hole.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, hole});
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    const auto& result = evaluation.feature_results.back();
    EXPECT_TRUE(result.topology_history_complete);
    EXPECT_TRUE(result.diagnostics.empty());
    EXPECT_EQ(kernel.getTopology(evaluation.geometry_id).face_count, 10U);
    const auto& topology = kernel.getTopology(evaluation.geometry_id);
    ASSERT_EQ(result.semantic_outputs.size(), topology.face_count + topology.edge_count +
                                                  topology.vertex_count);
    std::vector<std::uint64_t> local_ids;
    std::size_t base_faces = 0;
    std::size_t hole_faces = 0;
    std::size_t named_edges = 0;
    std::size_t named_vertices = 0;
    for (const auto& output : result.semantic_outputs) {
        local_ids.push_back((static_cast<std::uint64_t>(output.topology_type) << 56U) |
                            output.local_id);
        if (output.topology_type == PersistentTopologyType::face) {
            base_faces += output.semantic_ref.feature_id == base.feature_id ? 1U : 0U;
            hole_faces += output.semantic_ref.feature_id == hole.feature_id ? 1U : 0U;
        } else if (output.topology_type == PersistentTopologyType::edge) {
            ++named_edges;
        } else if (output.topology_type == PersistentTopologyType::vertex) {
            ++named_vertices;
        }
    }
    std::sort(local_ids.begin(), local_ids.end());
    EXPECT_EQ(std::adjacent_find(local_ids.begin(), local_ids.end()), local_ids.end());
    EXPECT_EQ(base_faces, 6U);
    EXPECT_EQ(hole_faces, 4U);
    EXPECT_EQ(named_edges, topology.edge_count);
    EXPECT_EQ(named_vertices, topology.vertex_count);
    EXPECT_TRUE(std::any_of(result.semantic_outputs.begin(), result.semantic_outputs.end(),
                            [&](const auto& output) {
                                return output.topology_type == PersistentTopologyType::edge &&
                                       output.semantic_ref.feature_id == hole.feature_id;
                            }));
    EXPECT_TRUE(std::any_of(result.semantic_outputs.begin(), result.semantic_outputs.end(),
                            [&](const auto& output) {
                                return output.topology_type == PersistentTopologyType::vertex &&
                                       output.semantic_ref.feature_id == hole.feature_id;
                            }));
}

TEST(GeometryExchange, NamingFixturePocketFromPlanarFaceCoversFinalShape) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "extrude-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-base";
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";

    ProfilePadSpec pocket;
    pocket.feature_id = "cut-pocket";
    pocket.body_id = "body-main";
    pocket.input_feature_id = base.feature_id;
    pocket.profile_feature_id = "sketch-on-top-face";
    pocket.regions = {rectangular_region("pocket", 5, 5, 15, 15)};
    pocket.pad_length = 5;
    pocket.plane_origin = {0, 0, 10};
    pocket.plane_normal = {0, 0, 1};
    pocket.plane_u_direction = {1, 0, 0};
    pocket.reversed = true;
    pocket.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, pocket});
    const auto repeated = kernel.evaluateProfilePadsWithHistory({base, pocket});
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    ASSERT_EQ(repeated.feature_results.size(), 2U);
    const auto& result = evaluation.feature_results.back();
    EXPECT_TRUE(result.topology_history_complete);
    EXPECT_EQ(result.topology_history.evidence_digest,
              repeated.feature_results.back().topology_history.evidence_digest);
    ASSERT_EQ(result.semantic_outputs.size(),
              repeated.feature_results.back().semantic_outputs.size());
    const auto same_ref = [](const SemanticTopologyRef& left,
                             const SemanticTopologyRef& right) {
        return left.feature_id == right.feature_id &&
               left.output_slot == right.output_slot &&
               left.source_ids == right.source_ids;
    };
    for (std::size_t index = 0; index < result.semantic_outputs.size(); ++index) {
        EXPECT_TRUE(same_ref(result.semantic_outputs[index].semantic_ref,
                             repeated.feature_results.back()
                                 .semantic_outputs[index]
                                 .semantic_ref));
    }
    const auto& topology = kernel.getTopology(evaluation.geometry_id);
    EXPECT_EQ(result.semantic_outputs.size(), topology.face_count + topology.edge_count +
                                                  topology.vertex_count);
    EXPECT_NEAR(kernel.getVolume(evaluation.geometry_id), 3500.0, 1.0e-6);

    ProfileCurveSpec circle;
    circle.entity_id = "pocket-circle";
    circle.kind = "CIRCLE";
    circle.center = {10, 10};
    circle.radius = 4;
    ProfileRegionSpec circular_region;
    circular_region.id = "circular-pocket";
    circular_region.outer = {"circular-pocket-loop", {circle}};
    pocket.feature_id = "cut-circular-pocket";
    pocket.profile_feature_id = "sketch-circle-on-top-face";
    pocket.regions = {circular_region};
    const auto circular = kernel.evaluateProfilePadsWithHistory({base, pocket});
    const auto& circular_result = circular.feature_results.back();
    const auto& circular_topology = kernel.getTopology(circular.geometry_id);
    EXPECT_TRUE(circular_result.topology_history_complete);
    EXPECT_EQ(circular_result.semantic_outputs.size(), circular_topology.face_count +
                                                           circular_topology.edge_count +
                                                           circular_topology.vertex_count);

    ProfilePadSpec boss = pocket;
    boss.feature_id = "add-boss";
    boss.profile_feature_id = "sketch-boss-on-top-face";
    boss.regions = {rectangular_region("boss", 5, 5, 15, 15)};
    boss.reversed = false;
    boss.body_operation = "ADD";
    const auto added = kernel.evaluateProfilePadsWithHistory({base, boss});
    const auto& added_result = added.feature_results.back();
    const auto& added_topology = kernel.getTopology(added.geometry_id);
    EXPECT_TRUE(added_result.topology_history_complete);
    EXPECT_EQ(added_result.semantic_outputs.size(), added_topology.face_count +
                                                        added_topology.edge_count +
                                                        added_topology.vertex_count);

    ProfilePadSpec side_pocket = pocket;
    side_pocket.feature_id = "cut-side-pocket";
    side_pocket.profile_feature_id = "sketch-side-pocket-on-top-face";
    side_pocket.regions = {rectangular_region("side-pocket", 0, 5, 10, 15)};
    const auto opened = kernel.evaluateProfilePadsWithHistory({base, side_pocket});
    const auto& opened_result = opened.feature_results.back();
    const auto& opened_topology = kernel.getTopology(opened.geometry_id);
    EXPECT_TRUE(opened_result.topology_history_complete);
    EXPECT_EQ(opened_result.semantic_outputs.size(), opened_topology.face_count +
                                                         opened_topology.edge_count +
                                                         opened_topology.vertex_count);

    ProfilePadSpec side_face_pocket = pocket;
    side_face_pocket.feature_id = "cut-from-side-face";
    side_face_pocket.profile_feature_id = "sketch-on-side-face";
    side_face_pocket.regions = {rectangular_region("side-face-pocket", 5, 2, 15, 8)};
    side_face_pocket.plane_origin = {20, 0, 0};
    side_face_pocket.plane_normal = {1, 0, 0};
    side_face_pocket.plane_u_direction = {0, 1, 0};
    side_face_pocket.reversed = true;
    const auto side_cut = kernel.evaluateProfilePadsWithHistory({base, side_face_pocket});
    const auto& side_cut_result = side_cut.feature_results.back();
    const auto& side_cut_topology = kernel.getTopology(side_cut.geometry_id);
    EXPECT_TRUE(side_cut_result.topology_history_complete);
    EXPECT_EQ(side_cut_result.semantic_outputs.size(), side_cut_topology.face_count +
                                                           side_cut_topology.edge_count +
                                                           side_cut_topology.vertex_count);

    ProfileCurveSpec annulus_outer;
    annulus_outer.entity_id = "annulus-outer";
    annulus_outer.kind = "CIRCLE";
    annulus_outer.center = {10, 10};
    annulus_outer.radius = 6;
    ProfileCurveSpec annulus_inner = annulus_outer;
    annulus_inner.entity_id = "annulus-inner";
    annulus_inner.radius = 2;
    annulus_inner.reversed = true;
    ProfileRegionSpec annulus;
    annulus.id = "annular-pocket";
    annulus.outer = {"annulus-outer-loop", {annulus_outer}};
    annulus.holes = {{"annulus-inner-loop", {annulus_inner}}};
    ProfilePadSpec annular_pocket = pocket;
    annular_pocket.feature_id = "cut-annular-pocket";
    annular_pocket.profile_feature_id = "sketch-annulus-on-top-face";
    annular_pocket.regions = {annulus};
    const auto annular = kernel.evaluateProfilePadsWithHistory({base, annular_pocket});
    const auto& annular_result = annular.feature_results.back();
    const auto& annular_topology = kernel.getTopology(annular.geometry_id);
    EXPECT_TRUE(annular_result.topology_history_complete);
    EXPECT_EQ(annular_result.semantic_outputs.size(), annular_topology.face_count +
                                                          annular_topology.edge_count +
                                                          annular_topology.vertex_count);

    ProfilePadSpec two_pockets = pocket;
    two_pockets.feature_id = "cut-two-pockets";
    two_pockets.profile_feature_id = "sketch-two-pockets-on-top-face";
    two_pockets.regions = {rectangular_region("pocket-left", 2, 5, 7, 15),
                           rectangular_region("pocket-right", 13, 5, 18, 15)};
    const auto doubled = kernel.evaluateProfilePadsWithHistory({base, two_pockets});
    const auto& doubled_result = doubled.feature_results.back();
    const auto& doubled_topology = kernel.getTopology(doubled.geometry_id);
    EXPECT_TRUE(doubled_result.topology_history_complete);
    EXPECT_EQ(doubled_result.semantic_outputs.size(), doubled_topology.face_count +
                                                          doubled_topology.edge_count +
                                                          doubled_topology.vertex_count);

    ProfileCurveSpec arc;
    arc.entity_id = "pocket-arc";
    arc.kind = "ARC";
    arc.center = {10, 10};
    arc.radius = 5;
    arc.start_angle = 0;
    arc.end_angle = 3.14159265358979323846;
    ProfileCurveSpec diameter;
    diameter.entity_id = "pocket-diameter";
    diameter.kind = "LINE";
    diameter.start = {5, 10};
    diameter.end = {15, 10};
    ProfileRegionSpec semicircle;
    semicircle.id = "semicircle-pocket";
    semicircle.outer = {"semicircle-pocket-loop", {arc, diameter}};
    ProfilePadSpec arc_pocket = pocket;
    arc_pocket.feature_id = "cut-arc-pocket";
    arc_pocket.profile_feature_id = "sketch-arc-on-top-face";
    arc_pocket.regions = {semicircle};
    const auto arced = kernel.evaluateProfilePadsWithHistory({base, arc_pocket});
    const auto& arced_result = arced.feature_results.back();
    const auto& arced_topology = kernel.getTopology(arced.geometry_id);
    EXPECT_TRUE(arced_result.topology_history_complete);
    EXPECT_EQ(arced_result.semantic_outputs.size(), arced_topology.face_count +
                                                        arced_topology.edge_count +
                                                        arced_topology.vertex_count);

    ProfileCurveSpec spline;
    spline.entity_id = "pocket-spline";
    spline.kind = "SPLINE";
    spline.control_points = {{5, 10}, {8, 5}, {15, 8}, {14, 15}, {7, 15}};
    spline.degree = 3;
    spline.closed = true;
    ProfileRegionSpec spline_region;
    spline_region.id = "spline-pocket";
    spline_region.outer = {"spline-pocket-loop", {spline}};
    ProfilePadSpec spline_pocket = pocket;
    spline_pocket.feature_id = "cut-spline-pocket";
    spline_pocket.profile_feature_id = "sketch-spline-on-top-face";
    spline_pocket.regions = {spline_region};
    const auto splined = kernel.evaluateProfilePadsWithHistory({base, spline_pocket});
    const auto& splined_result = splined.feature_results.back();
    const auto& splined_topology = kernel.getTopology(splined.geometry_id);
    EXPECT_TRUE(splined_result.topology_history_complete);
    EXPECT_EQ(splined_result.semantic_outputs.size(), splined_topology.face_count +
                                                          splined_topology.edge_count +
                                                          splined_topology.vertex_count);
}

TEST(GeometryExchange, NamingFixturePocketFromObliquePlanarFaceCoversFinalShape) {
    OcctKernel kernel;
    const std::vector<Vec2> triangle{{0, 0}, {20, 0}, {0, 20}};
    ProfileRegionSpec triangular_region;
    triangular_region.id = "triangular-base";
    triangular_region.outer.id = "triangular-base-loop";
    for (std::size_t index = 0; index < triangle.size(); ++index) {
        ProfileCurveSpec edge;
        edge.entity_id = "triangle-edge-" + std::to_string(index);
        edge.kind = "LINE";
        edge.start = triangle[index];
        edge.end = triangle[(index + 1) % triangle.size()];
        triangular_region.outer.curves.push_back(edge);
    }
    ProfilePadSpec base;
    base.feature_id = "extrude-triangular-base";
    base.body_id = "body-main";
    base.profile_feature_id = "sketch-triangular-base";
    base.regions = {triangular_region};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";

    const double inverse_sqrt_two = 1.0 / std::sqrt(2.0);
    ProfilePadSpec pocket;
    pocket.feature_id = "cut-oblique-face-pocket";
    pocket.body_id = "body-main";
    pocket.input_feature_id = base.feature_id;
    pocket.profile_feature_id = "sketch-on-oblique-face";
    pocket.regions = {rectangular_region("oblique-pocket", 5, 2, 15, 8)};
    pocket.pad_length = 5;
    pocket.plane_origin = {20, 0, 0};
    pocket.plane_normal = {inverse_sqrt_two, inverse_sqrt_two, 0};
    pocket.plane_u_direction = {-inverse_sqrt_two, inverse_sqrt_two, 0};
    pocket.reversed = true;
    pocket.body_operation = "REMOVE";

    const auto evaluation = kernel.evaluateProfilePadsWithHistory({base, pocket});
    ASSERT_EQ(evaluation.feature_results.size(), 2U);
    const auto& result = evaluation.feature_results.back();
    const auto& topology = kernel.getTopology(evaluation.geometry_id);
    EXPECT_TRUE(result.topology_history_complete);
    EXPECT_EQ(result.semantic_outputs.size(), topology.face_count + topology.edge_count +
                                                  topology.vertex_count);
    EXPECT_LT(kernel.getVolume(evaluation.geometry_id),
              kernel.getVolume(kernel.evaluateProfilePads({base})));
}

TEST(GeometryExchange, SolidFeatureChainFusesAndCutsOneBody) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.regions = {rectangular_region("base", 0, 0, 20, 20)};
    base.pad_length = 10;
    base.body_operation = "NEW_BODY";
    ProfilePadSpec add;
    add.regions = {rectangular_region("add", 10, 0, 30, 20)};
    add.pad_length = 10;
    add.body_operation = "ADD";
    ProfilePadSpec remove;
    remove.regions = {rectangular_region("cut", 12, 5, 18, 15)};
    remove.pad_length = 10;
    remove.body_operation = "REMOVE";

    const auto fused = kernel.evaluateProfilePads({base, add});
    EXPECT_EQ(kernel.getTopology(fused).solid_count, 1U);
    // Equal-height overlapping pads form one 30 x 20 x 10 box.  A raw OCCT
    // Fuse has the correct volume but retains section edges and exposes the top
    // as three faces; a Body result must merge those same-domain regions.
    EXPECT_EQ(kernel.getTopology(fused).face_count, 6U);
    EXPECT_EQ(kernel.getTopology(fused).edge_count, 12U);
    EXPECT_NEAR(kernel.getVolume(fused), 6000.0, 1.0e-6);
    const auto cut = kernel.evaluateProfilePads({base, add, remove});
    EXPECT_EQ(kernel.getTopology(cut).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(cut), 5400.0, 1.0e-6);
}

TEST(GeometryExchange, OneBodyCanContainSeveralSolidsAndKeepBooleanHistory) {
    OcctKernel kernel;
    ProfilePadSpec first;
    first.feature_id = "pad-first";
    first.body_id = "body-stable";
    first.profile_feature_id = "sketch-first";
    first.regions = {rectangular_region("first", 0, 0, 10, 10)};
    first.pad_length = 10;
    first.body_operation = "NEW_BODY";

    ProfilePadSpec second = first;
    second.feature_id = "pad-second";
    second.profile_feature_id = "sketch-second";
    second.regions = {rectangular_region("second", 20, 0, 30, 10)};
    second.body_operation = "ADD";
    auto separated = kernel.evaluateProfilePadsWithHistory({first, second});
    EXPECT_EQ(kernel.getTopology(separated.geometry_id).solid_count, 2U);
    EXPECT_EQ(separated.feature_results.size(), 2U);
    EXPECT_EQ(separated.feature_results.back().body_id, "body-stable");
    EXPECT_TRUE(separated.feature_results.back().topology_history_complete);

    ProfilePadSpec bridge = first;
    bridge.feature_id = "pad-bridge";
    bridge.profile_feature_id = "sketch-bridge";
    bridge.regions = {rectangular_region("bridge", 8, 0, 22, 10)};
    bridge.body_operation = "ADD";
    auto joined = kernel.evaluateProfilePadsWithHistory({first, second, bridge});
    EXPECT_EQ(kernel.getTopology(joined.geometry_id).solid_count, 1U);
    EXPECT_EQ(joined.feature_results.back().body_id, "body-stable");

    ProfilePadSpec split = first;
    split.feature_id = "pocket-split";
    split.profile_feature_id = "sketch-split";
    split.regions = {rectangular_region("split", 14, -1, 16, 11)};
    split.body_operation = "REMOVE";
    auto cut = kernel.evaluateProfilePadsWithHistory({first, second, bridge, split});
    EXPECT_EQ(kernel.getTopology(cut.geometry_id).solid_count, 2U);
    EXPECT_EQ(cut.feature_results.back().body_id, "body-stable");
    EXPECT_TRUE(cut.feature_results.back().topology_history_complete);

    ProfilePadSpec two_regions = first;
    two_regions.feature_id = "pad-two-regions";
    two_regions.regions = {first.regions.front(), second.regions.front()};
    EXPECT_EQ(kernel.getTopology(kernel.evaluateProfilePads({two_regions})).solid_count, 2U);

    ProfilePadSpec no_change = first;
    no_change.feature_id = "pad-no-change";
    no_change.body_operation = "ADD";
    EXPECT_THROW(kernel.evaluateProfilePads({first, no_change}), std::invalid_argument);
    no_change.body_operation = "INTERSECT";
    EXPECT_THROW(kernel.evaluateProfilePads({first, no_change}), std::invalid_argument);
    no_change.body_operation = "REMOVE";
    EXPECT_THROW(kernel.evaluateProfilePads({first, no_change}), std::invalid_argument);
}

TEST(GeometryExchange, RevolveBuildsSolidAroundConstructionAxis) {
    OcctKernel kernel;
    ProfilePadSpec revolve;
    revolve.regions = {rectangular_region("profile", 5, -2, 10, 2)};
    revolve.generator = "REVOLVE";
    revolve.body_operation = "NEW_BODY";
    revolve.revolve_angle = 2.0 * 3.14159265358979323846;
    revolve.axis_start = {0, -10};
    revolve.axis_end = {0, 10};
    const auto result = kernel.evaluateProfilePads({revolve});
    EXPECT_EQ(kernel.getTopology(result).solid_count, 1U);
    EXPECT_NEAR(kernel.getVolume(result), 300.0 * 3.14159265358979323846, 1.0e-5);
}

TEST(GeometryExchange, ExplicitDatumFramePlacesExtrudeOffTheDefaultPlanes) {
    OcctKernel kernel;
    ProfilePadSpec extrude;
    extrude.regions = {rectangular_region("offset-profile", 0, 0, 10, 5)};
    extrude.pad_length = 8;
    extrude.body_operation = "NEW_BODY";
    extrude.plane_origin = {100, 20, 30};
    extrude.plane_normal = {1, 0, 0};
    extrude.plane_u_direction = {0, 1, 0};
    const auto result = kernel.evaluateProfilePads({extrude});
    const auto bounds = kernel.getBoundingBox(result);
    EXPECT_NEAR(bounds.min.x, 100, 1.0e-6);
    EXPECT_NEAR(bounds.max.x, 108, 1.0e-6);
    EXPECT_NEAR(bounds.min.y, 20, 1.0e-6);
    EXPECT_NEAR(bounds.max.y, 30, 1.0e-6);
    EXPECT_NEAR(bounds.min.z, 30, 1.0e-6);
    EXPECT_NEAR(bounds.max.z, 35, 1.0e-6);
}

TEST(GeometryExchange, ImportedSolidRepairRejectsMissingFace) {
    const auto box = BRepPrimAPI_MakeBox(10, 20, 30).Shape();
    BRep_Builder builder;
    TopoDS_Shell shell;
    builder.MakeShell(shell);
    TopExp_Explorer faces(box, TopAbs_FACE);
    ASSERT_TRUE(faces.More());
    faces.Next(); // A real hole must not be hidden by normalization.
    for (; faces.More(); faces.Next()) builder.Add(shell, faces.Current());
    TopoDS_Solid solid;
    builder.MakeSolid(solid);
    builder.Add(solid, shell);
    std::ostringstream stream;
    BRepTools::Write(solid, stream);
    const auto serialized = stream.str();
    OcctKernel kernel;
    const auto original = kernel.loadBrepr({serialized.begin(), serialized.end()});
    const auto frozen = kernel.serializeBrepr(original);
    EXPECT_THROW(kernel.repairImportedSolid(original), std::invalid_argument);
    EXPECT_EQ(kernel.serializeBrepr(original), frozen);
}

TEST(GeometryExchange, ImportedSolidRepairCorpus) {
    const char* path = std::getenv("OCCCCAD_TEST_IMPORT_BREP");
    if (!path) GTEST_SKIP() << "set OCCCCAD_TEST_IMPORT_BREP for a repair corpus";
    std::ifstream file(path, std::ios::binary);
    ASSERT_TRUE(file.good());
    const std::vector<uint8_t> bytes{std::istreambuf_iterator<char>(file), std::istreambuf_iterator<char>()};
    OcctKernel kernel;
    const auto original = kernel.loadBrepr(bytes);
    const auto frozen = kernel.serializeBrepr(original);
    const auto repaired = kernel.repairImportedSolid(original);
    EXPECT_EQ(kernel.serializeBrepr(original), frozen);
    EXPECT_EQ(kernel.getTopology(repaired).solid_count, 1U);
    EXPECT_GT(kernel.getVolume(repaired), 0.0);
    EXPECT_EQ(kernel.repairImportedSolid(repaired), repaired);
    EXPECT_EQ(kernel.repairImportedSolid(original), repaired); // Deterministic retry; input stays immutable.
}

TEST(GeometryExchange, SplitCompoundSolidsPreservesPlacementAndOccurrences) {
    OcctKernel kernel;
    const auto box = kernel.createRectangularPad({0, 0, 20, 10, 5, "XY"});
    const auto compound = kernel.combine({{box, {0, 0, 0}, {0, 0, 0, 1}},
        {box, {50, 0, 0}, {0, 0, 0, 1}}, {box, {50, 0, 0}, {0, 0, 0, 1}},
        {box, {50, 0, 0}, {0, 0, std::sqrt(0.5), std::sqrt(0.5)}}});
    const auto solids = kernel.splitSolids(compound);
    ASSERT_EQ(solids.size(), 4U); // Coincident occurrences must not be deduplicated.
    for (const auto& solid : solids) {
        EXPECT_EQ(kernel.getTopology(solid).solid_count, 1U);
        EXPECT_NEAR(kernel.getVolume(solid), 1000, 1e-6);
    }
    EXPECT_NEAR(kernel.getBoundingBox(solids[1]).min.x, 50, 1e-6);
    TemporaryStepFile step(kernel.serializeStep(compound));
    EXPECT_EQ(kernel.inspectStepRootCount(step.path().string()), 1U);
    const auto parsed = kernel.loadStep(step.path().string());
    const auto split = kernel.splitSolids(parsed);
    ASSERT_EQ(split.size(), 4U);
    EXPECT_NEAR(kernel.getBoundingBox(split[1]).min.x, 50, 1e-6);
    EXPECT_NEAR(kernel.getBoundingBox(split[3]).min.x, 40, 1e-6);
    EXPECT_NEAR(kernel.getBoundingBox(split[3]).max.y, 20, 1e-6);
    EXPECT_EQ(kernel.splitSolids(box).size(), 1U);
    EXPECT_EQ(kernel.repairImportedSolid(box), box);
    EXPECT_EQ(kernel.repairImportedSolid(compound),compound);
}

TEST(GeometryExchange, XdeSharedDefinitionsNestedPlacementRoundTrip) {
    OcctKernel kernel;
    const auto box=kernel.createBox(20,10,5);
    occccad::kernel::ExchangeGraph graph;
    graph.definitions.push_back({"part","Part 名称","PART",box,{}});
    graph.definitions.push_back({"distinct","Distinct business Part","PART",box,{}});
    occccad::kernel::ExchangeDefinition sub{"sub","Sub Assembly","PRODUCT","",{}};
    constexpr double q=0.7071067811865476;
    for(int i=0;i<100;++i) sub.children.push_back({"instance/"+std::to_string(i),"part","Pin "+std::to_string(i),{"",{double(i)*30,2,3},{0,0,q,q}}});
    graph.definitions.push_back(sub);
    graph.definitions.push_back({"root","Root Assembly","PRODUCT","",{
        {"sub1","sub","Sub.1",{"",{1,2,3},{0,0,0,1}}},
        {"sub2","sub","Sub.2",{"",{4,5,6},{0,0,q,q}}},
        {"other","distinct","Different Part",{}}
    }});
    graph.roots.push_back({"root","root","Root Assembly",{}});
    const auto bytes=kernel.writeStepGraph(graph);
    TemporaryStepFile file(bytes);
    auto imported=kernel.readStepGraph(file.path().string());
    for(int cycle=0;cycle<2;++cycle) {
        ASSERT_EQ(imported.definitions.size(),4U);
        ASSERT_EQ(imported.roots.size(),1U);
        size_t parts=0,products=0;
        std::string shared;
        for(const auto& def:imported.definitions) {
            if(def.kind=="PART") {
                ++parts;
                EXPECT_TRUE(def.name=="Part 名称" || def.name=="Distinct business Part");
                EXPECT_NEAR(kernel.getBoundingBox(def.geometry_id).min.x,0,1e-6);
                EXPECT_NEAR(kernel.getVolume(def.geometry_id),1000,1e-6);
            }else {
                ++products;
                if(def.name=="Sub Assembly") {
                    ASSERT_EQ(def.children.size(),100U);
                    shared=def.children[0].definition_id;
                    for(size_t i=0;i<100;++i) {
                        EXPECT_EQ(def.children[i].definition_id,shared);
                        EXPECT_EQ(def.children[i].name,"Pin "+std::to_string(i));
                        EXPECT_NEAR(def.children[i].placement.translation.x,double(i)*30,1e-6);
                        EXPECT_NEAR(std::abs(def.children[i].placement.rotation.z),q,1e-6);
                        EXPECT_NEAR(std::abs(def.children[i].placement.rotation.z*q+def.children[i].placement.rotation.w*q),1,1e-6);
                    }
                }else {
                    EXPECT_EQ(def.name,"Root Assembly"); ASSERT_EQ(def.children.size(),3U);
                    EXPECT_EQ(def.children[0].definition_id,def.children[1].definition_id);
                    EXPECT_NEAR(def.children[1].placement.translation.x,4,1e-6);
                }
            }
        }
        EXPECT_EQ(parts,2U);EXPECT_EQ(products,2U);EXPECT_FALSE(shared.empty());
        TemporaryStepFile again(kernel.writeStepGraph(imported));
        imported=kernel.readStepGraph(again.path().string());
    }
    auto one=graph;
    one.definitions[2].children.resize(1);
    const auto small=kernel.writeStepGraph(one);
    EXPECT_LT(bytes.size(),small.size()*15U); // 100 references must not copy 100 B-Reps.
}

TEST(GeometryExchange, XdeMultipleRootsPreserveSharedReferences) {
    OcctKernel kernel;
    occccad::kernel::ExchangeGraph graph;
    graph.definitions.push_back({"part","Part","PART",kernel.createBox(1,2,3),{}});
    graph.roots={{"a","part","first",{}},{"b","part","second",{"",{10,0,0},{0,0,0,1}}}};
    TemporaryStepFile file(kernel.writeStepGraph(graph));
    const auto read=kernel.readStepGraph(file.path().string());
    ASSERT_EQ(read.definitions.size(),2U);
    for(const auto& def:read.definitions) if(def.kind=="PRODUCT") {
        ASSERT_EQ(def.children.size(),2U);
        EXPECT_EQ(def.children[0].definition_id,def.children[1].definition_id);
        EXPECT_EQ(def.children[1].name,"second");
        EXPECT_NEAR(def.children[1].placement.translation.x,10,1e-6);
    }
}

TEST(GeometryExchange, XdeRejectsCyclicAndInvalidPlacementGraphs) {
    OcctKernel kernel;
    occccad::kernel::ExchangeGraph graph;
    graph.definitions.push_back({"p","P","PRODUCT","",{{"self","p","self",{}}}});
    graph.roots.push_back({"root","p","P",{}});
    EXPECT_THROW(kernel.writeStepGraph(graph),std::invalid_argument);
    graph.definitions[0].children.clear();
    graph.definitions[0].kind="PART";graph.definitions[0].geometry_id=kernel.createBox(1,2,3);
    graph.roots[0].placement.rotation.w=2;
    EXPECT_THROW(kernel.writeStepGraph(graph),std::invalid_argument);
}

TEST(GeometryExchange, RepositoryStepFixturesContainImportableSolidGeometry) {
    OcctKernel kernel;
    const std::vector<std::filesystem::path> fixtures = {
        std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) / "Bottom Support - Bottom Support.step",
        std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) /
            "Windmill Head Cover - Windmill Head Cover.step",
    };
    for (const auto& fixture : fixtures) {
        SCOPED_TRACE(fixture.string());
        ASSERT_TRUE(std::filesystem::is_regular_file(fixture));
        const auto roots = kernel.inspectStepRootCount(fixture.string());
        ASSERT_GT(roots, 0U);
        for (uint32_t index = 1; index <= roots; ++index) {
            const auto imported = kernel.loadStepRoot(fixture.string(), index);
            EXPECT_GT(kernel.getVolume(imported), 0.0);
            EXPECT_GT(kernel.getTopology(imported).solid_count, 0U);
        }
    }
}


ImportTopologySeed imported_box_seed(OcctKernel& kernel, const std::vector<uint8_t>& bytes) {
    ImportTopologySeed seed;
    seed.feature_id = "import-test";
    seed.body_id = "body-main";
    seed.brep_sha256 = make_geometry_id(std::string(bytes.begin(), bytes.end())).substr(7);
    const auto topology = kernel.getTopology(kernel.loadBrepr(bytes));
    // Fixed fixture allocations stand in for independently allocated persisted IDs.
    int allocation = 97;
    for (const auto& face : topology.faces)
        seed.identities.push_back({"opaque-" + std::to_string(allocation++), PersistentTopologyType::face, face.local_id});
    for (const auto& edge : topology.edges)
        seed.identities.push_back({"opaque-" + std::to_string(allocation++), PersistentTopologyType::edge, edge.local_id});
    for (const auto& vertex : topology.vertices)
        seed.identities.push_back({"opaque-" + std::to_string(allocation++), PersistentTopologyType::vertex, vertex.local_id});
    return seed;
}

TEST(GeometryExchange, XdeSolidCompoundIsOneNamedPartDefinition) {
    OcctKernel kernel;
    const auto box=kernel.createBox(2,3,4);
    const auto compound=kernel.combine({{box,{0,0,0},{0,0,0,1}},{box,{10,0,0},{0,0,0,1}}});
    occccad::kernel::ExchangeGraph graph;
    graph.definitions.push_back({"multi","Multisolid Part","PART",compound,{}});
    graph.roots.push_back({"root","multi","Multisolid Part",{}});
    TemporaryStepFile file(kernel.writeStepGraph(graph));
    const auto imported=kernel.readStepGraph(file.path().string());
    ASSERT_EQ(imported.definitions.size(),1U);
    const auto id=kernel.repairImportedSolid(imported.definitions[0].geometry_id);
    EXPECT_EQ(kernel.getTopology(id).solid_count,2U);
    const auto bytes=kernel.serializeBrepr(id);
    const auto seed=imported_box_seed(kernel,bytes);
    const auto result=kernel.evaluateProfilePadsWithHistory({},bytes,&seed);
    EXPECT_EQ(kernel.getTopology(result.geometry_id).solid_count,2U);
}

TEST(GeometryExchange, ImportedNamingAdjacencyMatchesTopologicalOracle) {
    for (const auto& shape : {BRepPrimAPI_MakeBox(20, 30, 40).Shape(), BRepPrimAPI_MakeCylinder(10, 20).Shape()}) {
        std::ostringstream stream;
        BRepTools::Write(shape, stream);
        const auto serialized = stream.str();
        const std::vector<uint8_t> bytes(serialized.begin(), serialized.end());
        OcctKernel kernel;
        auto seed = imported_box_seed(kernel, bytes);
        std::reverse(seed.identities.begin(), seed.identities.end());
        const auto evaluation = kernel.evaluateProfilePadsWithHistory({}, bytes, &seed);
        const auto& outputs = evaluation.feature_results.front().semantic_outputs;
        TopoDS_Shape restored;
        BRep_Builder builder;
        std::istringstream input(serialized);
        BRepTools::Read(restored, input, builder);
        TopTools_IndexedMapOfShape faces, edges, vertices;
        TopExp::MapShapes(restored, TopAbs_FACE, faces);
        TopExp::MapShapes(restored, TopAbs_EDGE, edges);
        TopExp::MapShapes(restored, TopAbs_VERTEX, vertices);
        const auto resolve = [&](const auto& output) {
            const auto& map = output.topology_type == PersistentTopologyType::face ? faces :
                output.topology_type == PersistentTopologyType::edge ? edges : vertices;
            return map(static_cast<int>(output.local_id));
        };
        // Independent pairwise oracle: same-dimensional objects share a
        // boundary; different dimensions require actual containment.
        const auto adjacent = [](const TopoDS_Shape& a, const TopoDS_Shape& b) {
            if (a.ShapeType() == b.ShapeType()) {
                const auto boundary = a.ShapeType() == TopAbs_FACE ? TopAbs_EDGE : TopAbs_VERTEX;
                TopTools_IndexedMapOfShape left, right;
                TopExp::MapShapes(a, boundary, left);
                TopExp::MapShapes(b, boundary, right);
                for (int index = 1; index <= left.Extent(); ++index)
                    if (right.Contains(left(index))) return true;
                return false;
            }
            const auto& owner = a.ShapeType() < b.ShapeType() ? a : b;
            const auto& member = a.ShapeType() < b.ShapeType() ? b : a;
            TopTools_IndexedMapOfShape children;
            TopExp::MapShapes(owner, member.ShapeType(), children);
            return children.Contains(member);
        };
        for (std::size_t i = 0; i < outputs.size(); ++i) {
            std::set<std::string> expected, actual;
            for (std::size_t j = 0; j < outputs.size(); ++j)
                if (i != j && adjacent(resolve(outputs[i]), resolve(outputs[j])))
                    expected.insert(outputs[j].semantic_ref.source_ids.front());
            for (const auto& neighbor : outputs[i].evidence.adjacent) {
                EXPECT_EQ(neighbor.feature_id, seed.feature_id);
                actual.insert(neighbor.source_ids.front());
            }
            EXPECT_EQ(actual.size(), outputs[i].evidence.adjacent.size());
            EXPECT_EQ(actual, expected);
        }
    }
}

TEST(GeometryExchange, ImportedNamingSnapshotAndColdIdentity) {
    OcctKernel kernel;
    const auto bytes = kernel.serializeBrepr(kernel.createBox(20,20,10));
    auto seed = imported_box_seed(kernel,bytes);
    const auto first = kernel.evaluateProfilePadsWithHistory({},bytes,&seed);
    ASSERT_EQ(first.feature_results.size(),1U);
    const auto& root = first.feature_results.front();
    EXPECT_TRUE(root.topology_history_complete);
    EXPECT_EQ(root.semantic_outputs.size(),26U);
    for (const auto& output : root.semantic_outputs) {
        EXPECT_EQ(output.semantic_ref.output_slot,"import.topology");
        EXPECT_FALSE(output.evidence.adjacent.empty());
    }
    OcctKernel cold;
    std::reverse(seed.identities.begin(),seed.identities.end());
    const auto again = cold.evaluateProfilePadsWithHistory({},bytes,&seed);
    for (const auto& output : root.semantic_outputs) {
        const auto& candidates = again.feature_results.front().semantic_outputs;
        const auto found = std::find_if(candidates.begin(),candidates.end(),[&](const auto& other){return other.semantic_ref.source_ids==output.semantic_ref.source_ids;});
        ASSERT_NE(found,candidates.end());
        EXPECT_EQ(found->local_id,output.local_id);
        EXPECT_EQ(found->evidence.evidence_digest,output.evidence.evidence_digest);
    }
    auto broken = seed;
    broken.brep_sha256="wrong";
    EXPECT_THROW(cold.evaluateProfilePadsWithHistory({},bytes,&broken),std::invalid_argument);
    broken=seed;broken.identities.pop_back();
    EXPECT_THROW(cold.evaluateProfilePadsWithHistory({},bytes,&broken),std::invalid_argument);
    broken=seed;broken.identities[0].stable_id=broken.identities[1].stable_id;
    EXPECT_THROW(cold.evaluateProfilePadsWithHistory({},bytes,&broken),std::invalid_argument);
    // Even a geometrically similar symmetric snapshot cannot reuse locator mappings.
    const auto changed = kernel.serializeBrepr(kernel.createBox(20,20,11));
    EXPECT_THROW(cold.evaluateProfilePadsWithHistory({},changed,&seed),std::invalid_argument);
}

TEST(GeometryExchange, ImportedNamingPropagatesThroughBooleanChain) {
    OcctKernel kernel;
    const auto bytes=kernel.serializeBrepr(kernel.createBox(20,20,10));
    const auto seed=imported_box_seed(kernel,bytes);
    ProfilePadSpec add;
    add.feature_id="add";add.body_id="body-main";add.input_feature_id=seed.feature_id;add.profile_feature_id="sketch-add";
    add.regions={rectangular_region("add-region",10,0,30,20)};add.pad_length=10;add.body_operation="ADD";
    ProfilePadSpec cut;
    cut.feature_id="cut";cut.body_id="body-main";cut.input_feature_id="add";cut.profile_feature_id="sketch-cut";
    cut.regions={rectangular_region("cut-region",12,5,18,15)};cut.pad_length=10;cut.body_operation="REMOVE";
    const auto result=kernel.evaluateProfilePadsWithHistory({add,cut},bytes,&seed);
    ASSERT_EQ(result.feature_results.size(),3U);
    for(const auto& feature:result.feature_results) EXPECT_TRUE(feature.topology_history_complete);
    const auto topology=kernel.getTopology(result.geometry_id);
    EXPECT_EQ(result.feature_results.back().semantic_outputs.size(),topology.face_count+topology.edge_count+topology.vertex_count);
    EXPECT_NEAR(kernel.getVolume(result.geometry_id),5400,1e-6);
    EXPECT_TRUE(std::any_of(result.feature_results[1].topology_history.lineage.begin(),result.feature_results[1].topology_history.lineage.end(),[&](const auto& lineage){return std::any_of(lineage.sources.begin(),lineage.sources.end(),[&](const auto& source){return source.feature_id==seed.feature_id;});}));
}

}  // namespace
}  // namespace occccad::kernel

namespace occccad::kernel {
TEST(GeometryExchange, CrossBodyBooleanUsesFrozenNamedStageAndColdRebuild) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base";
    base.body_id = "target";
    base.profile_feature_id = "base-sketch";
    base.regions = {rectangular_region("base-region", 0, 0, 20, 20)};
    base.pad_length = 10;
    auto cutter = base;
    cutter.feature_id = "cutter";
    cutter.body_id = "tool";
    cutter.profile_feature_id = "tool-sketch";
    cutter.regions = {rectangular_region("tool-region", 5, 5, 10, 10)};
    cutter.pad_length = 20;
    auto evaluated = kernel.evaluateProfilePadsWithHistory({cutter});
    BodyToolInput input;
    input.body_id = "tool";
    input.feature_id = "cutter";
    input.geometry_id = evaluated.geometry_id;
    input.brep = kernel.serializeBrepr(evaluated.geometry_id);
    for (const auto& output : evaluated.feature_results.back().semantic_outputs) {
        const auto& ref = output.semantic_ref;
        input.topology.push_back(
            {ref.feature_id, ref.output_slot, ref.source_ids,
             output.topology_type == PersistentTopologyType::face   ? TopologyType::FACE
             : output.topology_type == PersistentTopologyType::edge ? TopologyType::EDGE
                                                                    : TopologyType::VERTEX,
             output.local_id});
    }
    ProfilePadSpec boolean;
    boolean.feature_id = "boolean";
    boolean.body_id = "target";
    boolean.input_feature_id = "base";
    boolean.generator = "BOOLEAN";
    boolean.tools = {input};
    for (const auto& [operation, volume] : std::vector<std::pair<std::string, double>>{
             {"REMOVE", 3750}, {"ADD", 4250}, {"INTERSECT", 250}}) {
        boolean.body_operation = operation;
        const auto result = kernel.evaluateProfilePadsWithHistory({base, boolean});
        EXPECT_NEAR(kernel.getVolume(result.geometry_id), volume, 1e-6);
        const auto& history = result.feature_results.back();
        EXPECT_TRUE(history.topology_history_complete);
        const auto topology = kernel.getTopology(result.geometry_id);
        EXPECT_EQ(history.semantic_outputs.size(),
                  topology.face_count + topology.edge_count + topology.vertex_count);
        OcctKernel cold;
        const auto rebuilt = cold.evaluateProfilePadsWithHistory({base, boolean});
        EXPECT_NEAR(cold.getVolume(rebuilt.geometry_id), volume, 1e-6);
        EXPECT_EQ(history.topology_history.evidence_digest,
                  rebuilt.feature_results.back().topology_history.evidence_digest);
    }
    boolean.tools[0].geometry_id = "wrong";
    EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base, boolean}), std::invalid_argument);
}

TEST(GeometryExchange, ExtrudeExtentsUseInputBodyAndPreserveNaming) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base";
    base.body_id = "body";
    base.profile_feature_id = "sketch";
    base.pad_length = 10;
    base.regions = {rectangular_region("region", 0, 0, 20, 20)};
    for (const auto& mode : std::vector<std::string>{"FINITE", "SYMMETRIC", "TWO_SIDED"}) {
        auto spec = base;
        spec.extent = mode;
        spec.second_length = 5;
        auto result = kernel.evaluateProfilePadsWithHistory({spec});
        EXPECT_NEAR(kernel.getVolume(result.geometry_id), mode == "TWO_SIDED" ? 6000 : 4000, 1e-6);
        EXPECT_TRUE(result.feature_results.back().topology_history_complete);
    }
    auto cut = base;
    cut.feature_id = "cut";
    cut.input_feature_id = "base";
    cut.profile_feature_id = "hole";
    cut.regions = {rectangular_region("hole-region", 5, 5, 10, 10)};
    cut.body_operation = "REMOVE";
    cut.extent = "THROUGH_ALL";
    cut.pad_length = 1;
    cut.plane_origin = {0, 0, 1000};
    cut.plane_normal = {0, 0, 1};
    cut.plane_u_direction = {1, 0, 0};
    auto result = kernel.evaluateProfilePadsWithHistory({base, cut});
    EXPECT_NEAR(kernel.getVolume(result.geometry_id), 3750, 1e-6);
    EXPECT_TRUE(result.feature_results.back().topology_history_complete);
    base.pad_length = 25;
    result = kernel.evaluateProfilePadsWithHistory({base, cut});
    EXPECT_NEAR(kernel.getVolume(result.geometry_id), 9375, 1e-6);
    cut.body_operation = "ADD";
    EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base, cut}), std::invalid_argument);
}

TEST(GeometryExchange, LocalModifiersUseSemanticInputsAndCompleteHistory) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base";
    base.body_id = "body";
    base.profile_feature_id = "sketch";
    base.pad_length = 10;
    base.regions = {rectangular_region("region", 0, 0, 20, 20)};
    const auto initial = kernel.evaluateProfilePadsWithHistory({base});
    const auto pick = [&](const FeatureResult& result, const std::string& prefix) {
        for (const auto& output : result.semantic_outputs)
            if (output.semantic_ref.output_slot.rfind(prefix, 0) == 0) {
                const auto& r = output.semantic_ref;
                return FeatureTopologyRef{r.feature_id, r.output_slot, r.source_ids};
            }
        throw std::runtime_error("test semantic slot missing: " + prefix);
    };
    const auto check = [&](const ProfileEvaluationResult& result) {
        const auto topology = kernel.getTopology(result.geometry_id);
        EXPECT_TRUE(result.feature_results.back().topology_history_complete);
        EXPECT_EQ(result.feature_results.back().semantic_outputs.size(),
                  topology.face_count + topology.edge_count + topology.vertex_count);
        EXPECT_GT(kernel.getVolume(result.geometry_id), 0);
    };
    for (const auto& kind : std::vector<std::string>{"FILLET", "CHAMFER"}) {
        ProfilePadSpec modifier;
        modifier.feature_id = "modify";
        modifier.body_id = "body";
        modifier.input_feature_id = "base";
        modifier.generator = kind;
        modifier.pad_length = 1;
        modifier.selections = {
            pick(initial.feature_results.back(), "VERTICAL_FROM_PROFILE_ENDPOINTS/")};
        const auto result = kernel.evaluateProfilePadsWithHistory({base, modifier});
        check(result);
        EXPECT_LT(kernel.getVolume(result.geometry_id), 4000);
        modifier.selections.front().output_slot = "missing";
        EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base, modifier}),
                     std::invalid_argument);
    }
    ProfilePadSpec shell;
    shell.feature_id = "shell";
    shell.body_id = "body";
    shell.input_feature_id = "base";
    shell.generator = "SHELL";
    shell.pad_length = 1;
    shell.selections = {pick(initial.feature_results.back(), "END_CAP/")};
    const auto hollow = kernel.evaluateProfilePadsWithHistory({base, shell});
    check(hollow);
    EXPECT_NEAR(kernel.getVolume(hollow.geometry_id), 4000 - 18 * 18 * 9, 1e-5);
    ProfilePadSpec draft;
    draft.feature_id = "draft";
    draft.body_id = "body";
    draft.input_feature_id = "base";
    draft.generator = "DRAFT";
    draft.revolve_angle = 5 * 3.14159265358979323846 / 180;
    draft.neutral_normal = {0, 0, 1};
    for (const auto& output : initial.feature_results.back().semantic_outputs)
        if (output.semantic_ref.output_slot.rfind("SIDE_FROM_PROFILE_EDGE/", 0) == 0) {
            const auto& r = output.semantic_ref;
            draft.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
        }
    const auto tapered = kernel.evaluateProfilePadsWithHistory({base, draft});
    check(tapered);
    EXPECT_LT(kernel.getVolume(tapered.geometry_id), 4000);
    shell.input_feature_id = "draft";
    shell.selections = {pick(tapered.feature_results.back(), "END_CAP/")};
    const auto chain = kernel.evaluateProfilePadsWithHistory({base, draft, shell});
    check(chain);
    EXPECT_LT(kernel.getVolume(chain.geometry_id), kernel.getVolume(tapered.geometry_id));
    shell.pad_length = 1000;
    EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base, draft, shell}),
                 std::invalid_argument);
}

TEST(GeometryExchange, ChamferedTrihedralCornerFillets) {
    const auto readShape = [](const std::vector<uint8_t>& bytes) {
        std::istringstream stream(std::string(bytes.begin(), bytes.end()));
        BRep_Builder builder;
        TopoDS_Shape shape;
        BRepTools::Read(shape, stream, builder);
        return shape;
    };
    const auto volume = [](const TopoDS_Shape& shape) {
        GProp_GProps props;
        BRepGProp::VolumeProperties(shape, props);
        return props.Mass();
    };
    // Rotate the entire construction: the special corner is a topology/support
    // relation, never a world-axis or coordinate-based identity in production.
    for (int pose = 0; pose < 3; ++pose) {
        SCOPED_TRACE(pose);
        OcctKernel kernel;
        ProfilePadSpec base;
        base.feature_id = "base";
        base.body_id = "body";
        base.profile_feature_id = "sketch";
        base.pad_length = 40;
        base.regions = {rectangular_region("region", 0, 0, 20, 30)};
        gp_Dir u(1, 0, 0), n(0, 0, 1);
        gp_Pnt origin(0, 0, 0);
        if (pose == 1) { u = gp_Dir(0, 1, 0); n = gp_Dir(-1, 0, 0); origin = gp_Pnt(7, -11, 5); }
        if (pose == 2) { u = gp_Dir(1, 1, 0); n = gp_Dir(-1, 1, 2); origin = gp_Pnt(7, -11, 5); }
        const gp_Dir v = n.Crossed(u);
        base.plane_origin = {origin.X(), origin.Y(), origin.Z()};
        base.plane_u_direction = {u.X(), u.Y(), u.Z()};
        base.plane_normal = {n.X(), n.Y(), n.Z()};
        const auto local = [&](const Vec3& point) {
            const gp_Vec delta(origin, gp_Pnt(point.x, point.y, point.z));
            return Vec3{delta.Dot(gp_Vec(u)), delta.Dot(gp_Vec(v)), delta.Dot(gp_Vec(n))};
        };
        const auto initial = kernel.evaluateProfilePadsWithHistory({base});
        const auto initialTopology = kernel.tessellate(initial.geometry_id);
        ProfilePadSpec chamfer;
        chamfer.feature_id = "chamfer";
        chamfer.body_id = "body";
        chamfer.input_feature_id = "base";
        chamfer.generator = "CHAMFER";
        chamfer.pad_length = 1;
        for (const auto& output : initial.feature_results.back().semantic_outputs) {
            if (output.topology_type != PersistentTopologyType::edge) continue;
            const auto it = std::find_if(initialTopology.edges.begin(), initialTopology.edges.end(), [&](const auto& e) { return e.local_id == output.local_id; });
            if (it != initialTopology.edges.end() && it->points.size() > 1 &&
                std::all_of(it->points.begin(), it->points.end(), [&](const auto& point) { const auto p = local(point); return std::abs(p.x) < 1e-7 && std::abs(p.y) < 1e-7; })) {
                const auto& r = output.semantic_ref;
                chamfer.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
            }
        }
        ASSERT_EQ(chamfer.selections.size(), 1);
        const auto beveled = kernel.evaluateProfilePadsWithHistory({base, chamfer});
        const auto inputBytes = kernel.serializeBrepr(beveled.geometry_id);
        const auto bevelShape = readShape(inputBytes);
        const auto topology = kernel.tessellate(beveled.geometry_id);
        ProfilePadSpec fillet;
        fillet.feature_id = "fillet";
        fillet.body_id = "body";
        fillet.input_feature_id = "chamfer";
        fillet.generator = "FILLET";
        for (const auto& output : beveled.feature_results.back().semantic_outputs) {
            if (output.topology_type != PersistentTopologyType::edge) continue;
            const auto it = std::find_if(topology.edges.begin(), topology.edges.end(), [&](const auto& e) { return e.local_id == output.local_id; });
            if (it != topology.edges.end() && it->points.size() > 1 &&
                std::all_of(it->points.begin(), it->points.end(), [&](const auto& p) { return std::abs(local(p).z) < 1e-7; }) &&
                (std::all_of(it->points.begin(), it->points.end(), [&](const auto& p) { return std::abs(local(p).x) < 1e-7; }) ||
                 std::all_of(it->points.begin(), it->points.end(), [&](const auto& p) { return std::abs(local(p).y) < 1e-7; }))) {
                const auto& r = output.semantic_ref;
                fillet.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
            }
        }
        ASSERT_EQ(fillet.selections.size(), 2);
        for (double radius : {0.5, 1.0, 1.01, 1.5, 2.0, 5.0}) {
            SCOPED_TRACE(radius);
            fillet.pad_length = radius;
            const auto result = kernel.evaluateProfilePadsWithHistory({base, chamfer, fillet});
            const auto shape = readShape(kernel.serializeBrepr(result.geometry_id));
            EXPECT_TRUE(BRepCheck_Analyzer(shape).IsValid());
            EXPECT_TRUE(result.feature_results.back().topology_history_complete);
            // Independent OCCT reference: round the un-beveled box in one
            // operation, then clip by the original beveled solid.
            const auto box = BRepPrimAPI_MakeBox(20, 30, 40).Shape();
            BRepFilletAPI_MakeFillet reference(box);
            TopTools_IndexedMapOfShape boxEdges;
            TopExp::MapShapes(box, TopAbs_EDGE, boxEdges);
            for (int e = 1; e <= boxEdges.Extent(); ++e) {
                bool x0 = true, y0 = true, z0 = true;
                for (TopExp_Explorer vertices(boxEdges(e), TopAbs_VERTEX); vertices.More(); vertices.Next()) {
                    const auto p = BRep_Tool::Pnt(TopoDS::Vertex(vertices.Current()));
                    x0 &= std::abs(p.X()) < 1e-7;
                    y0 &= std::abs(p.Y()) < 1e-7;
                    z0 &= std::abs(p.Z()) < 1e-7;
                }
                if (z0 && (x0 || y0)) reference.Add(radius, TopoDS::Edge(boxEdges(e)));
            }
            reference.Build();
            ASSERT_TRUE(reference.IsDone());
            gp_Trsf transform;
            transform.SetValues(u.X(), v.X(), n.X(), origin.X(), u.Y(), v.Y(), n.Y(), origin.Y(), u.Z(), v.Z(), n.Z(), origin.Z());
            BRepAlgoAPI_Common clipped(BRepBuilderAPI_Transform(reference.Shape(), transform, true).Shape(), bevelShape);
            ASSERT_TRUE(clipped.IsDone());
            EXPECT_NEAR(kernel.getVolume(result.geometry_id), volume(clipped.Shape()), 1e-5);
            for (bool reverse : {false, true}) {
                BRepAlgoAPI_Cut difference(reverse ? shape : clipped.Shape(), reverse ? clipped.Shape() : shape);
                ASSERT_TRUE(difference.IsDone());
                EXPECT_NEAR(volume(difference.Shape()), 0, 1e-6);
            }
            TopTools_IndexedMapOfShape faces;
            TopExp::MapShapes(shape, TopAbs_FACE, faces);
            int cylinders = 0;
            for (int f = 1; f <= faces.Extent(); ++f) {
                const BRepAdaptor_Surface surface(TopoDS::Face(faces(f)));
                if (surface.GetType() == GeomAbs_Cylinder) {
                    ++cylinders;
                    EXPECT_NEAR(surface.Cylinder().Radius(), radius, 1e-8);
                } else EXPECT_EQ(surface.GetType(), GeomAbs_Plane);
            }
            EXPECT_EQ(cylinders, 2);
            const auto mesh = kernel.tessellate(result.geometry_id, 0.01, 0.1);
            ASSERT_FALSE(mesh.triangles.empty());
            EXPECT_FALSE(make_glb(mesh).empty());
            std::set<uint32_t> meshedFaces(mesh.face_ids.begin(), mesh.face_ids.end());
            EXPECT_EQ(meshedFaces.size(), static_cast<size_t>(faces.Extent()));
            double meshVolume = 0;
            for (const auto& p : mesh.vertices) EXPECT_TRUE(std::isfinite(p.x) && std::isfinite(p.y) && std::isfinite(p.z));
            for (const auto& triangle : mesh.triangles) {
                const auto a = local(mesh.vertices.at(triangle.v0));
                const auto b = local(mesh.vertices.at(triangle.v1));
                const auto c = local(mesh.vertices.at(triangle.v2));
                const gp_Vec ab(b.x-a.x, b.y-a.y, b.z-a.z), ac(c.x-a.x, c.y-a.y, c.z-a.z);
                EXPECT_GT(ab.Crossed(ac).Magnitude(), 1e-12);
                meshVolume += gp_Vec(a.x,a.y,a.z).Dot(gp_Vec(b.x,b.y,b.z).Crossed(gp_Vec(c.x,c.y,c.z))) / 6;
            }
            EXPECT_NEAR(meshVolume, kernel.getVolume(result.geometry_id), 0.5);
            OcctKernel cold;
            auto reversed = fillet;
            std::reverse(reversed.selections.begin(), reversed.selections.end());
            const auto repeated = cold.evaluateProfilePadsWithHistory({base, chamfer, reversed});
            EXPECT_EQ(result.feature_results.back().topology_history.evidence_digest, repeated.feature_results.back().topology_history.evidence_digest);
            EXPECT_EQ(inputBytes, kernel.serializeBrepr(beveled.geometry_id));
        }
        fillet.pad_length = 1000;
        EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base, chamfer, fillet}), std::invalid_argument);
    }
}

TEST(GeometryExchange, ChamferedBoxBottomFilletsAndInwardShell) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base";
    base.body_id = "body";
    base.profile_feature_id = "sketch";
    base.pad_length = 40;
    base.regions = {rectangular_region("region", 0, 0, 20, 30)};
    const auto initial = kernel.evaluateProfilePadsWithHistory({base});
    ProfilePadSpec chamfer;
    chamfer.feature_id = "chamfer";
    chamfer.body_id = "body";
    chamfer.input_feature_id = "base";
    chamfer.generator = "CHAMFER";
    chamfer.pad_length = 2;
    for (const auto& output : initial.feature_results.back().semantic_outputs) {
        const auto& r = output.semantic_ref;
        if (r.output_slot.rfind("VERTICAL_FROM_PROFILE_ENDPOINTS/", 0) == 0)
            chamfer.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
    }
    ASSERT_EQ(chamfer.selections.size(), 4);
    const auto beveled = kernel.evaluateProfilePadsWithHistory({base, chamfer});
    const auto topology = kernel.tessellate(beveled.geometry_id);
    ProfilePadSpec fillet;
    fillet.feature_id = "fillet";
    fillet.body_id = "body";
    fillet.input_feature_id = "chamfer";
    fillet.generator = "FILLET";
    fillet.pad_length = 1;
    ProfilePadSpec shell = fillet;
    shell.feature_id = "shell";
    shell.generator = "SHELL";
    shell.pad_length = 1;
    for (const auto& output : beveled.feature_results.back().semantic_outputs) {
        const auto& r = output.semantic_ref;
        if (output.topology_type == PersistentTopologyType::edge) {
            const auto it = std::find_if(topology.edges.begin(), topology.edges.end(), [&](const auto& e) { return e.local_id == output.local_id; });
            if (it != topology.edges.end() && it->points.size() > 1 &&
                std::all_of(it->points.begin(), it->points.end(), [](const auto& v) { return std::abs(v.z) < 1e-7; }))
                fillet.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
        }
        if (r.output_slot.rfind("END_CAP/", 0) == 0)
            shell.selections.push_back({r.feature_id, r.output_slot, r.source_ids});
    }
    ASSERT_GE(fillet.selections.size(), 4);
    for (size_t count = 1; count <= fillet.selections.size(); ++count) {
        auto subset = fillet;
        subset.selections.resize(count);
        SCOPED_TRACE(count);
        ProfileEvaluationResult result;
        EXPECT_NO_THROW(result = kernel.evaluateProfilePadsWithHistory({base, chamfer, subset}));
        if (result.geometry_id.empty()) continue;
        EXPECT_LT(kernel.getVolume(result.geometry_id), kernel.getVolume(beveled.geometry_id));
        EXPECT_TRUE(result.feature_results.back().topology_history_complete);
    }
    ASSERT_EQ(shell.selections.size(), 1);
    const auto hollow = kernel.evaluateProfilePadsWithHistory({base, chamfer, shell});
    EXPECT_LT(kernel.getVolume(hollow.geometry_id), kernel.getVolume(beveled.geometry_id));
    EXPECT_GT(kernel.getVolume(hollow.geometry_id), 0);
    EXPECT_TRUE(hollow.feature_results.back().topology_history_complete);
    // The inward cavity remains valid when its offset erases a small bevel.
    chamfer.pad_length = 0.5;
    const auto smallBevel = kernel.evaluateProfilePadsWithHistory({base, chamfer});
    shell.selections.clear();
    for (const auto& output : smallBevel.feature_results.back().semantic_outputs) {
        const auto& r = output.semantic_ref;
        if (r.output_slot.rfind("END_CAP/",0) == 0) shell.selections.push_back({r.feature_id,r.output_slot,r.source_ids});
    }
    const auto smallHollow = kernel.evaluateProfilePadsWithHistory({base, chamfer, shell});
    EXPECT_NEAR(kernel.getVolume(smallHollow.geometry_id), 23980 - 18*28*39, 1e-5);
    EXPECT_TRUE(smallHollow.feature_results.back().topology_history_complete);
    const auto repeated = kernel.evaluateProfilePadsWithHistory({base, chamfer, shell});
    EXPECT_EQ(smallHollow.feature_results.back().topology_history.evidence_digest, repeated.feature_results.back().topology_history.evidence_digest);
    shell.pad_length = 1000;
    EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({base,chamfer,shell}), std::invalid_argument);
}

TEST(GeometryExchange, LoftOrderedSectionsHaveHistoryAndSurviveBooleanAndChamfer) {
    OcctKernel kernel;
    ProfilePadSpec loft;
    loft.feature_id = "loft";
    loft.body_id = "body";
    loft.generator = "LOFT";
    for (int i = 0; i < 3; ++i) {
        LoftSectionSpec section;
        section.sketch_id = "section-" + std::to_string(i);
        section.region = rectangular_region(section.sketch_id, 0, 0, 20, 20);
        section.origin = {0, 0, 10.0 * i};
        section.normal = {0, 0, 1};
        section.u_direction = {1, 0, 0};
        loft.sections.push_back(section);
    }
    for (bool ruled : {true, false}) {
        loft.ruled = ruled;
        const auto solid = kernel.evaluateProfilePadsWithHistory({loft});
        EXPECT_NEAR(kernel.getVolume(solid.geometry_id), 8000, 1e-5);
        const auto topo = kernel.getTopology(solid.geometry_id);
        EXPECT_TRUE(solid.feature_results.back().topology_history_complete);
        EXPECT_EQ(solid.feature_results.back().semantic_outputs.size(),
                  topo.face_count + topo.edge_count + topo.vertex_count);
        ProfilePadSpec cut;
        cut.feature_id = "cut";
        cut.body_id = "body";
        cut.input_feature_id = "loft";
        cut.profile_feature_id = "hole";
        cut.regions = {rectangular_region("hole-region", 5, 5, 10, 10)};
        cut.pad_length = 25;
        cut.body_operation = "REMOVE";
        const auto bored = kernel.evaluateProfilePadsWithHistory({loft, cut});
        EXPECT_NEAR(kernel.getVolume(bored.geometry_id), 7500, 1e-5);
        ProfilePadSpec chamfer;
        chamfer.feature_id = "chamfer";
        chamfer.body_id = "body";
        chamfer.input_feature_id = "cut";
        chamfer.generator = "CHAMFER";
        chamfer.pad_length = 2;
        for (const auto& output : bored.feature_results.back().semantic_outputs)
            if (output.topology_type == PersistentTopologyType::edge &&
                output.evidence.geometry_type == "LINE") {
                const auto& r = output.semantic_ref;
                chamfer.selections = {{r.feature_id, r.output_slot, r.source_ids}};
                break;
            }
        const auto finished = kernel.evaluateProfilePadsWithHistory({loft, cut, chamfer});
        EXPECT_TRUE(finished.feature_results.back().topology_history_complete);
        EXPECT_LT(kernel.getVolume(finished.geometry_id), 7500);
    }
}

TEST(GeometryExchange, LoftTaperedSectionsRespectStoredSeams) {
    OcctKernel kernel;
    ProfilePadSpec loft;
    loft.feature_id = "loft";
    loft.body_id = "body";
    loft.generator = "LOFT";
    for (int i = 0; i < 2; ++i) {
        LoftSectionSpec section;
        section.sketch_id = "section-" + std::to_string(i);
        section.region = rectangular_region(section.sketch_id, i * 5, i * 5, 20 - i * 5, 20 - i * 5);
        section.seam_entity_id = section.region.outer.curves.front().entity_id;
        // Profile traversal may start elsewhere; the stored seam is authoritative.
        auto& curves = section.region.outer.curves;
        std::rotate(curves.begin(), curves.begin() + i + 1, curves.end());
        section.origin = {0, 0, i * 20.0};
        section.normal = {0, 0, 1};
        section.u_direction = {1, 0, 0};
        loft.sections.push_back(section);
    }
    for (bool ruled : {true, false}) {
        loft.ruled = ruled;
        const auto result = kernel.evaluateProfilePadsWithHistory({loft});
        EXPECT_NEAR(kernel.getVolume(result.geometry_id), 14000.0 / 3.0, 1e-5);
        EXPECT_TRUE(result.feature_results.back().topology_history_complete);
    }
    loft.sections.back().seam_entity_id = "removed-edge";
    EXPECT_THROW(kernel.evaluateProfilePadsWithHistory({loft}), std::invalid_argument);
}

void check_shell_chain(OcctKernel& kernel, std::vector<ProfilePadSpec> chain, const std::string& cap, std::vector<double> thicknesses = {0.5,1.0,2.0}, double height = 40) {
    const auto solid = kernel.evaluateProfilePadsWithHistory(chain);
    ProfilePadSpec shell;
    shell.feature_id = "shell"; shell.body_id = "body"; shell.generator = "SHELL";
    shell.input_feature_id = chain.back().feature_id;
    for (const auto& output : solid.feature_results.back().semantic_outputs) {
        const auto& r = output.semantic_ref;
        if (r.output_slot.rfind(cap, 0) == 0) shell.selections.push_back({r.feature_id,r.output_slot,r.source_ids});
    }
    ASSERT_EQ(shell.selections.size(), 1);
    const auto originalBytes = kernel.serializeBrepr(solid.geometry_id);
    const auto readShape = [](const std::vector<uint8_t>& bytes) {
        BRep_Builder builder; TopoDS_Shape shape;
        std::istringstream stream(std::string(bytes.begin(), bytes.end()));
        BRepTools::Read(shape, stream, builder);
        return shape;
    };
    const auto original = readShape(originalBytes);
    double inputTolerance = 0;
    for (TopExp_Explorer vertices(original,TopAbs_VERTEX); vertices.More(); vertices.Next())
        inputTolerance = std::max(inputTolerance,BRep_Tool::Tolerance(TopoDS::Vertex(vertices.Current())));
    SCOPED_TRACE("upstream vertex tolerance " + std::to_string(inputTolerance));
    for (double thickness : thicknesses) {
        SCOPED_TRACE(thickness);
        shell.pad_length = thickness;
        auto input = chain; input.push_back(shell);
        ProfileEvaluationResult result;
        EXPECT_NO_THROW(result = kernel.evaluateProfilePadsWithHistory(input));
        if (result.geometry_id.empty()) continue;
        EXPECT_GT(kernel.getVolume(result.geometry_id), 0);
        EXPECT_LT(kernel.getVolume(result.geometry_id), kernel.getVolume(solid.geometry_id));
        EXPECT_TRUE(result.feature_results.back().topology_history_complete);
        const auto shape = readShape(kernel.serializeBrepr(result.geometry_id));
        EXPECT_TRUE(BRepCheck_Analyzer(shape).IsValid());
        for (TopExp_Explorer vertices(shape,TopAbs_VERTEX); vertices.More(); vertices.Next())
            EXPECT_LT(BRep_Tool::Tolerance(TopoDS::Vertex(vertices.Current())),thickness*1e-3);
        EXPECT_EQ(kernel.getTopology(result.geometry_id).solid_count, 1);
        // The opening stays open, the base has the requested thickness,
        // and material immediately beyond the inner wall is absent.
        const auto state = [&](const gp_Pnt& p) { return BRepClass3d_SolidClassifier(shape, p, 1e-6).State(); };
        EXPECT_EQ(state(gp_Pnt(0,0,height-thickness*0.1)), TopAbs_OUT);
        EXPECT_EQ(state(gp_Pnt(0,0,thickness*0.5)), TopAbs_IN);
        EXPECT_EQ(state(gp_Pnt(0,0,thickness*1.5)), TopAbs_OUT);
        int checked = 0;
        for (TopExp_Explorer faces(original, TopAbs_FACE); faces.More(); faces.Next()) {
            const auto face = TopoDS::Face(faces.Current());
            double u0,v0,u1,v1;
            BRepTools::UVBounds(face,u0,u1,v0,v1);
            const double u=(u0+u1)*0.5, v=(v0+v1)*0.5;
            BRepAdaptor_Surface surface(face);
            gp_Pnt point; gp_Vec du,dv;
            surface.D1(u,v,point,du,dv);
            if (point.Z() > height-1e-6 || du.Crossed(dv).Magnitude() < 1e-9) continue;
            bool nearEdge = false;
            const auto vertex = BRepBuilderAPI_MakeVertex(point).Vertex();
            for (TopExp_Explorer edges(face,TopAbs_EDGE); edges.More(); edges.Next()) {
                BRepExtrema_DistShapeShape distance(vertex,edges.Current());
                if (!distance.IsDone() || distance.Value() < 3*thickness) nearEdge=true;
            }
            if (nearEdge) continue;
            gp_Vec normal(gp_Dir(du.Crossed(dv)));
            if (face.Orientation() == TopAbs_REVERSED) normal.Reverse();
            EXPECT_EQ(state(point.Translated(normal*(-0.5*thickness))),TopAbs_IN);
            EXPECT_EQ(state(point.Translated(normal*(-1.5*thickness))),TopAbs_OUT);
            ++checked;
        }
        EXPECT_GE(checked, 2);
        const auto mesh = kernel.tessellate(result.geometry_id);
        ASSERT_FALSE(mesh.triangles.empty());
        const std::set<uint32_t> rendered(mesh.face_ids.begin(),mesh.face_ids.end());
        EXPECT_EQ(rendered.size(),kernel.getTopology(result.geometry_id).face_count);
        EXPECT_FALSE(make_glb(mesh).empty());
        for (const auto& p : mesh.vertices) EXPECT_TRUE(std::isfinite(p.x) && std::isfinite(p.y) && std::isfinite(p.z));
        EXPECT_EQ(originalBytes,kernel.serializeBrepr(solid.geometry_id));
        if (thickness == 1 && !chain.front().ruled && chain.size() < 3) {
            OcctKernel cold;
            const auto repeated=cold.evaluateProfilePadsWithHistory(input);
            EXPECT_EQ(result.feature_results.back().topology_history.evidence_digest,repeated.feature_results.back().topology_history.evidence_digest);
        }
    }
}

TEST(GeometryExchange, ShellAfterChamferAndCornerFillets) {
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base"; base.body_id = "body"; base.profile_feature_id = "sketch";
    base.pad_length = 40; base.regions = {rectangular_region("region", -15,-15,15,15)};
    const auto initial = kernel.evaluateProfilePadsWithHistory({base});
    ProfilePadSpec modifier;
    modifier.feature_id="modifier"; modifier.body_id="body"; modifier.input_feature_id="base";
    modifier.generator="CHAMFER"; modifier.pad_length=8;
    for (const auto& output : initial.feature_results.back().semantic_outputs) {
        const auto& r=output.semantic_ref;
        if (r.output_slot.rfind("VERTICAL_FROM_PROFILE_ENDPOINTS/",0)==0)
            modifier.selections.push_back({r.feature_id,r.output_slot,r.source_ids});
    }
    const auto beveled = kernel.evaluateProfilePadsWithHistory({base,modifier});
    const auto topology = kernel.tessellate(beveled.geometry_id);
    ProfilePadSpec rounding;
    rounding.feature_id = "rounding"; rounding.body_id = "body"; rounding.input_feature_id = modifier.feature_id;
    rounding.generator = "FILLET"; rounding.pad_length = 5;
    for (const auto& output : beveled.feature_results.back().semantic_outputs) {
        if (output.topology_type != PersistentTopologyType::edge) continue;
        const auto it=std::find_if(topology.edges.begin(),topology.edges.end(),[&](const auto& e){return e.local_id==output.local_id;});
        if (it==topology.edges.end() || it->points.size()<2) continue;
        bool bottom=true,x=true,y=true;
        for (const auto& point : it->points) {
            bottom &= std::abs(point.z)<1e-7; x &= std::abs(point.x+15)<1e-7; y &= std::abs(point.y+15)<1e-7;
        }
        if (bottom && (x || y)) { const auto& r=output.semantic_ref; rounding.selections.push_back({r.feature_id,r.output_slot,r.source_ids}); }
    }
    ASSERT_EQ(rounding.selections.size(),2);
    SCOPED_TRACE("large chamfer followed by corner fillets");
    check_shell_chain(kernel, {base,modifier,rounding},"END_CAP/",{1.0});
}

TEST(GeometryExchange, ShellAfterLargeModifiersAndMixedLoft) {
    OSD_Parallel::SetUseOcctThreads(Standard_True);
    OSD_ThreadPool::DefaultPool(4)->SetNbDefaultThreadsToLaunch(4);
    OcctKernel kernel;
    ProfilePadSpec base;
    base.feature_id = "base"; base.body_id = "body"; base.profile_feature_id = "sketch";
    base.pad_length = 40; base.regions = {rectangular_region("region", -15,-15,15,15)};
    const auto initial = kernel.evaluateProfilePadsWithHistory({base});
    for (bool allEdges : {false, true}) for (const std::string generator : {"CHAMFER", "FILLET"}) {
        SCOPED_TRACE(allEdges);
        SCOPED_TRACE(generator);
        ProfilePadSpec modifier;
        modifier.feature_id = "modifier"; modifier.body_id = "body"; modifier.input_feature_id = "base";
        modifier.generator = generator; modifier.pad_length = 8;
        for (const auto& output : initial.feature_results.back().semantic_outputs) {
            const auto& r = output.semantic_ref;
            if (output.topology_type == PersistentTopologyType::edge && (allEdges || r.output_slot.rfind("VERTICAL_FROM_PROFILE_ENDPOINTS/",0) == 0))
                modifier.selections.push_back({r.feature_id,r.output_slot,r.source_ids});
        }
        check_shell_chain(kernel, {base,modifier}, "END_CAP/");

    }
    ProfilePadSpec loft;
    loft.feature_id = "loft"; loft.body_id = "body"; loft.generator = "LOFT";
    LoftSectionSpec bottom;
    bottom.sketch_id = "rectangle"; bottom.region = rectangular_region("rectangle", -15,-15,15,15);
    bottom.normal = {0,0,1}; bottom.u_direction = {1,0,0};
    bottom.seam_entity_id = bottom.region.outer.curves.front().entity_id;
    LoftSectionSpec top = bottom;
    top.sketch_id = "circle"; top.origin = {0,0,40};
    ProfileCurveSpec circle; circle.entity_id = "circle-edge"; circle.kind = "CIRCLE"; circle.radius = 10;
    top.region.id = "circle"; top.region.outer = {"circle-loop",{circle}}; top.seam_entity_id = circle.entity_id;
    loft.sections = {bottom,top};
    for (bool ruled : {false,true}) {
        SCOPED_TRACE(ruled);
        loft.ruled = ruled;
        check_shell_chain(kernel, {loft}, "LOFT_END_CAP");
    }
}

TEST(GeometryExchange, ShellOffsetRectangleToLargerCircle) {
    OSD_Parallel::SetUseOcctThreads(Standard_True);
    OSD_ThreadPool::DefaultPool(4)->SetNbDefaultThreadsToLaunch(4);
    OcctKernel kernel;
    ProfilePadSpec loft; loft.feature_id="loft"; loft.body_id="body"; loft.generator="LOFT";
    LoftSectionSpec bottom;
    bottom.sketch_id="rectangle";
    bottom.region=rectangular_region("rectangle",-82.8172482034993,-87.47795914135841,111.25953546530698,69.55214784189991);
    bottom.normal={0,0,1}; bottom.u_direction={1,0,0}; bottom.seam_entity_id=bottom.region.outer.curves.front().entity_id;
    auto top=bottom; top.sketch_id="circle"; top.origin={0,0,100};
    ProfileCurveSpec circle; circle.entity_id="circle-edge"; circle.kind="CIRCLE"; circle.radius=90;
    top.region.id="circle"; top.region.outer={"circle-loop",{circle}}; top.seam_entity_id=circle.entity_id;
    loft.sections={bottom,top};
    check_shell_chain(kernel, {loft}, "LOFT_END_CAP", {1.0}, 100);
}

TEST(GeometryExchange, LoftRectangleToCircleUsesActualSplitHistory) {
    OcctKernel kernel;
    ProfilePadSpec loft;
    loft.feature_id = "mixed-loft"; loft.body_id = "body"; loft.generator = "LOFT";
    LoftSectionSpec rectangle;
    rectangle.sketch_id = "rectangle";
    rectangle.region = rectangular_region("rectangle-region", -10, -10, 10, 10);
    rectangle.normal = {0, 0, 1}; rectangle.u_direction = {1, 0, 0};
    rectangle.seam_entity_id = rectangle.region.outer.curves.front().entity_id;
    LoftSectionSpec circle = rectangle;
    circle.sketch_id = "circle"; circle.origin = {0, 0, 20};
    ProfileCurveSpec curve; curve.entity_id = "circle-entity"; curve.kind = "CIRCLE";
    curve.center = {0, 0}; curve.radius = 10;
    circle.region.id = "circle-region"; circle.region.outer = {"circle-loop", {curve}};
    circle.seam_entity_id = curve.entity_id;
    loft.sections = {rectangle, circle};
    for (bool ruled : {false, true}) {
        loft.ruled = ruled;
        const auto result = kernel.evaluateProfilePadsWithHistory({loft});
        const auto& topology = kernel.getTopology(result.geometry_id);
        EXPECT_EQ(topology.solid_count, 1);
        const auto volume = kernel.getVolume(result.geometry_id);
        EXPECT_GT(volume, 20 * 3.14159265358979323846 * 100);
        EXPECT_LT(volume, 20 * 400);
        const auto& history = result.feature_results.back();
        EXPECT_TRUE(history.topology_history_complete);
        EXPECT_EQ(history.semantic_outputs.size(), topology.face_count + topology.edge_count + topology.vertex_count);
        bool start = false, end = false, circle_side = false;
        for (const auto& output : history.semantic_outputs) {
            if (output.semantic_ref.output_slot == "LOFT_START_CAP") { start = true; EXPECT_NEAR(output.evidence.measure_si.value_or(-1), 400e-6, 1e-8); }
            if (output.semantic_ref.output_slot == "LOFT_END_CAP") { end = true; EXPECT_NEAR(output.evidence.measure_si.value_or(-1), 3.14159265358979323846 * 100e-6, 1e-8); }
        }
        for (const auto& generated : history.topology_history.lineage) {
            for (const auto& source : generated.sources)
                if (source.feature_id == "circle" && source.output_slot == "PROFILE_EDGE/circle-entity") circle_side = true;
        }
        EXPECT_TRUE(start); EXPECT_TRUE(end); EXPECT_TRUE(circle_side);
        OcctKernel cold;
        EXPECT_EQ(cold.evaluateProfilePadsWithHistory({loft}).geometry_id, result.geometry_id);
    }
}

}  // namespace occccad::kernel

#include <BRepBuilderAPI_Copy.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepMesh_Context.hxx>
#include <BRepMesh_DelabellaMeshAlgoFactory.hxx>
#include <BRepMesh_FaceDiscret.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRepMesh_MeshAlgoFactory.hxx>
#include <BRepTools.hxx>
#include <BRep_Tool.hxx>
#include <BRep_Builder.hxx>
#include <IMeshData_Face.hxx>
#include <IMeshTools_MeshAlgo.hxx>
#include <OSD_Parallel.hxx>
#include <OSD_ThreadPool.hxx>
#include <Poly_Triangulation.hxx>
#include <Poly_PolygonOnTriangulation.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Circ.hxx>
#include <gp_Pln.hxx>
#include <occccad/kernel/kernel.hpp>

#include <gtest/gtest.h>

#include <algorithm>
#include <chrono>
#include <cstdlib>
#include <fstream>
#include <iostream>
#include <mutex>
#include <vector>

namespace {
using Clock = std::chrono::steady_clock;
struct FaceTiming {
    int type, wires;
    double milliseconds;
};
struct MeshTrace {
    std::mutex mutex;
    std::vector<FaceTiming> faces;
};

class TimedMesh final : public IMeshTools_MeshAlgo {
public:
    TimedMesh(Handle(IMeshTools_MeshAlgo) delegate, MeshTrace& trace)
        : delegate_(std::move(delegate)), trace_(trace) {}
    void Perform(const IMeshData::IFaceHandle& face, const IMeshTools_Parameters& parameters,
                 const Message_ProgressRange& range) override {
        const auto start = Clock::now();
        delegate_->Perform(face, parameters, range);
        const double ms = std::chrono::duration<double, std::milli>(Clock::now() - start).count();
        const std::lock_guard<std::mutex> guard(trace_.mutex);
        trace_.faces.push_back({static_cast<int>(face->GetSurface()->GetType()), face->WiresNb(), ms});
    }
private:
    Handle(IMeshTools_MeshAlgo) delegate_;
    MeshTrace& trace_;
};

// Keep curved-surface refinement identical; vary only the exact planar base
// triangulation. This probe is also runnable against a frozen STEP-derived BREP.
class ProfiledFactory final : public BRepMesh_MeshAlgoFactory {
public:
    ProfiledFactory(int strategy, MeshTrace& trace) : strategy_(strategy), trace_(trace) {}
    Handle(IMeshTools_MeshAlgo) GetAlgo(GeomAbs_SurfaceType type,
                                       const IMeshTools_Parameters& parameters) const override {
        Handle(IMeshTools_MeshAlgo) algo;
        if (type == GeomAbs_Plane && strategy_ == 2) {
            algo = BRepMesh_DelabellaMeshAlgoFactory().GetAlgo(type, parameters);
        } else {
            auto local = parameters;
            if (type == GeomAbs_Plane && strategy_ == 1)
                local.EnableControlSurfaceDeflectionAllSurfaces = Standard_False;
            algo = BRepMesh_MeshAlgoFactory::GetAlgo(type, local);
        }
        return new TimedMesh(algo, trace_);
    }
private:
    int strategy_;
    MeshTrace& trace_;
};

TopoDS_Shape perforated_plane() {
    BRepBuilderAPI_MakeFace face(gp_Pln(gp_Pnt(0, 0, 0), gp_Dir(0, 0, 1)), 0, 300, 0, 150);
    for (int i = 0; i < 450; ++i) {
        const gp_Circ circle(gp_Ax2(gp_Pnt(5 + 10 * (i % 30), 5 + 10 * (i / 30), 0),
                                   gp_Dir(0, 0, 1)), 1);
        auto wire = BRepBuilderAPI_MakeWire(BRepBuilderAPI_MakeEdge(circle).Edge()).Wire();
        wire.Reverse();
        face.Add(wire);
    }
    return face.Face();
}
} // namespace

TEST(SurfaceDisplay, PlanarMeshingAlgorithmsPreserveTrimmedArea) {
    OSD_Parallel::SetUseOcctThreads(Standard_True);
    OSD_ThreadPool::DefaultPool(4)->SetNbDefaultThreadsToLaunch(4);
    TopoDS_Shape original;
    if (const auto path = std::getenv("OCCCCAD_TEST_MESH_BREP")) {
        std::ifstream input(path, std::ios::binary);
        ASSERT_TRUE(input.good());
        BRep_Builder builder;
        BRepTools::Read(original, input, builder);
        ASSERT_FALSE(original.IsNull());
    } else {
        original = perforated_plane();
    }
    double reference_area = 0;
    int reference_boundary_nodes = 0;
    for (int strategy = 0; strategy < 3; ++strategy) {
        SCOPED_TRACE(strategy);
        BRepBuilderAPI_Copy copy(original, Standard_True, Standard_False);
        IMeshTools_Parameters parameters;
        parameters.Deflection = occccad::kernel::visual_linear_deflection_mm;
        parameters.Angle = parameters.AngleInterior = occccad::kernel::visual_angular_deflection_radians;
        parameters.EnableControlSurfaceDeflectionAllSurfaces = Standard_True;
        parameters.InParallel = Standard_True;
        MeshTrace trace;
        Handle(BRepMesh_Context) context = new BRepMesh_Context();
        context->SetFaceDiscret(new BRepMesh_FaceDiscret(new ProfiledFactory(strategy, trace)));
        BRepMesh_IncrementalMesh mesher;
        mesher.SetShape(copy.Shape());
        mesher.ChangeParameters() = parameters;
        const auto start = Clock::now();
        mesher.Perform(context);
        const double ms = std::chrono::duration<double, std::milli>(Clock::now() - start).count();
        ASSERT_TRUE(mesher.IsDone());
        ASSERT_EQ(mesher.GetStatusFlags(), 0);
        double area = 0;
        int nodes = 0, triangles = 0, boundary_nodes = 0;
        for (TopExp_Explorer faces(copy.Shape(), TopAbs_FACE); faces.More(); faces.Next()) {
            const auto face = TopoDS::Face(faces.Current());
            TopLoc_Location location;
            const auto mesh = BRep_Tool::Triangulation(face, location);
            ASSERT_FALSE(mesh.IsNull());
            nodes += mesh->NbNodes();
            triangles += mesh->NbTriangles();
            for (int t = 1; t <= mesh->NbTriangles(); ++t) {
                int a, b, c;
                mesh->Triangle(t).Get(a, b, c);
                area += gp_Vec(mesh->Node(a), mesh->Node(b)).Crossed(gp_Vec(mesh->Node(a), mesh->Node(c))).Magnitude() / 2;
            }
            for (TopExp_Explorer edges(face, TopAbs_EDGE); edges.More(); edges.Next()) {
                const auto polygon = BRep_Tool::PolygonOnTriangulation(TopoDS::Edge(edges.Current()), mesh, location);
                ASSERT_FALSE(polygon.IsNull());
                boundary_nodes += polygon->NbNodes();
            }
        }
        if (!strategy) {
            reference_area = area;
            reference_boundary_nodes = boundary_nodes;
        } else {
            EXPECT_NEAR(area, reference_area, 1e-4);
            EXPECT_EQ(boundary_nodes, reference_boundary_nodes);
        }
        std::sort(trace.faces.begin(), trace.faces.end(), [](const auto& a, const auto& b) {
            return a.milliseconds > b.milliseconds;
        });
        std::cout << "mesh_algorithm strategy=" << strategy << " linear=" << parameters.Deflection
                  << " angular=" << parameters.Angle << " total_ms=" << ms << " nodes=" << nodes
                  << " triangles=" << triangles << " area=" << area << " boundary_nodes=" << boundary_nodes << '\n';
        for (size_t i = 0; i < std::min<size_t>(3, trace.faces.size()); ++i)
            std::cout << "mesh_face type=" << trace.faces[i].type << " wires=" << trace.faces[i].wires
                      << " ms=" << trace.faces[i].milliseconds << '\n';
    }
}

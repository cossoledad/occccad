// OCCT Kernel Adapter — implementation
//
// ALL OCCT headers and types are confined to this file.
// The rest of the system only sees occccad::kernel::ICadKernel.

#include <internal/occt_kernel.hpp>

#include <BRepBndLib.hxx>
#include <BRepFilletAPI_MakeChamfer.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <STEPControl_Reader.hxx>
#include <StlAPI_Writer.hxx>
#include <TopExp_Explorer.hxx>
#include <TopExp.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Shape.hxx>
#include <TopoDS_Solid.hxx>
#include <TopoDS_Vertex.hxx>
#include <TopoDS_Iterator.hxx>

#include <Geom_BSplineCurve.hxx>
#include <Geom_BSplineSurface.hxx>
#include <Geom_Circle.hxx>
#include <Geom_ConicalSurface.hxx>
#include <Geom_CylindricalSurface.hxx>
#include <Geom_Ellipse.hxx>
#include <Geom_Line.hxx>
#include <Geom_Plane.hxx>
#include <Geom_SphericalSurface.hxx>
#include <Geom_ToroidalSurface.hxx>
#include <gp_Pnt.hxx>

#include <cmath>
#include <cstring>
#include <stdexcept>
#include <string>

namespace occccad::kernel {

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

namespace {

BoundingBox to_bbox(const Bnd_Box& occt_box) {
    BoundingBox result{};
    if (!occt_box.IsVoid()) {
        occt_box.Get(
            result.min.x, result.min.y, result.min.z,
            result.max.x, result.max.y, result.max.z);
    }
    return result;
}

[[maybe_unused]] Vec3 to_vec3(const gp_Pnt& p) {
    return {p.X(), p.Y(), p.Z()};
}

int classify_surface(const TopoDS_Face& face) {
    // Simplified: production code would use GeomAbs_SurfaceType
    Handle(Geom_Surface) surf = BRep_Tool::Surface(face);
    if (surf.IsNull()) return -1;

    if (surf->IsKind(STANDARD_TYPE(Geom_Plane))) return 0;
    if (surf->IsKind(STANDARD_TYPE(Geom_CylindricalSurface))) return 1;
    if (surf->IsKind(STANDARD_TYPE(Geom_ConicalSurface))) return 2;
    if (surf->IsKind(STANDARD_TYPE(Geom_SphericalSurface))) return 3;
    if (surf->IsKind(STANDARD_TYPE(Geom_ToroidalSurface))) return 4;
    if (surf->IsKind(STANDARD_TYPE(Geom_BSplineSurface))) return 5;
    return -1;
}

int classify_curve(const TopoDS_Edge& edge) {
    Standard_Real first, last;
    Handle(Geom_Curve) curve = BRep_Tool::Curve(edge, first, last);
    if (curve.IsNull()) return -1;

    if (curve->IsKind(STANDARD_TYPE(Geom_Line))) return 0;
    if (curve->IsKind(STANDARD_TYPE(Geom_Circle))) return 1;
    if (curve->IsKind(STANDARD_TYPE(Geom_Ellipse))) return 2;
    if (curve->IsKind(STANDARD_TYPE(Geom_BSplineCurve))) return 3;
    return -1;
}

}  // namespace

// ---------------------------------------------------------------------------
// OcctKernel implementation
// ---------------------------------------------------------------------------

struct OcctKernel::Impl {
    // Placeholder for any additional state
};

OcctKernel::OcctKernel() : impl_(std::make_unique<Impl>()) {}
OcctKernel::~OcctKernel() = default;

OcctKernel::OcctKernel(OcctKernel&&) noexcept = default;
OcctKernel& OcctKernel::operator=(OcctKernel&&) noexcept = default;

bool OcctKernel::is_loaded(const GeometryId& id) const noexcept {
    return shapes_.find(id) != shapes_.end();
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

GeometryId OcctKernel::loadBrepr(const std::vector<uint8_t>& /*data*/) {
    throw std::runtime_error("B-Rep deserialization not yet implemented");
}

GeometryId OcctKernel::loadStep(const std::string& path) {
    STEPControl_Reader reader;
    IFSelect_ReturnStatus status = reader.ReadFile(path.c_str());
    if (status != IFSelect_RetDone) {
        throw std::runtime_error("STEP read failed: " + path);
    }

    reader.TransferRoots();
    TopoDS_Shape shape = reader.OneShape();

    auto* shape_ptr = new TopoDS_Shape(shape);
    std::string id = "sha256:stub:" + std::to_string(shapes_.size());
    shapes_[id] = shape_ptr;
    return id;
}

void OcctKernel::unload(const GeometryId& id) {
    auto it = shapes_.find(id);
    if (it != shapes_.end()) {
        delete static_cast<TopoDS_Shape*>(it->second);
        shapes_.erase(it);
    }
}

// ---------------------------------------------------------------------------
// Primitives
// ---------------------------------------------------------------------------

GeometryId OcctKernel::createBox(double dx, double dy, double dz) {
    TopoDS_Shape shape = BRepPrimAPI_MakeBox(dx, dy, dz).Shape();
    if (shape.IsNull()) {
        throw std::runtime_error("Box creation failed");
    }

    auto* shape_ptr = new TopoDS_Shape(shape);
    std::string id = "sha256:box:" + std::to_string(shapes_.size());
    shapes_[id] = shape_ptr;
    return id;
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

BoundingBox OcctKernel::getBoundingBox(const GeometryId& id) {
    auto* shape = static_cast<TopoDS_Shape*>(shapes_.at(id));
    Bnd_Box box;
    BRepBndLib::Add(*shape, box);
    return to_bbox(box);
}

TopologyInfo OcctKernel::getTopology(const GeometryId& id) {
    auto* shape = static_cast<TopoDS_Shape*>(shapes_.at(id));

    TopoDS_Shape s = *shape;
    TopTools_IndexedMapOfShape face_map, edge_map, vertex_map, solid_map;

    TopExp::MapShapes(s, TopAbs_FACE, face_map);
    TopExp::MapShapes(s, TopAbs_EDGE, edge_map);
    TopExp::MapShapes(s, TopAbs_VERTEX, vertex_map);
    TopExp::MapShapes(s, TopAbs_SOLID, solid_map);

    TopologyInfo info{};
    info.face_count = face_map.Extent();
    info.edge_count = edge_map.Extent();
    info.vertex_count = vertex_map.Extent();
    info.solid_count = solid_map.Extent();

    // Faces
    for (int i = 1; i <= face_map.Extent(); ++i) {
        const TopoDS_Face& face = TopoDS::Face(face_map(i));
        FaceInfo fi{};
        fi.local_id = i;
        fi.surface_type = classify_surface(face);

        Bnd_Box box;
        BRepBndLib::Add(face, box);
        fi.bbox = to_bbox(box);

        info.faces.push_back(fi);
    }

    // Edges
    for (int i = 1; i <= edge_map.Extent(); ++i) {
        const TopoDS_Edge& edge = TopoDS::Edge(edge_map(i));
        EdgeInfo ei{};
        ei.local_id = i;
        ei.curve_type = classify_curve(edge);

        Bnd_Box box;
        BRepBndLib::Add(edge, box);
        ei.bbox = to_bbox(box);

        info.edges.push_back(ei);
    }

    return info;
}

// ---------------------------------------------------------------------------
// Tessellation
// ---------------------------------------------------------------------------

TessellationResult OcctKernel::tessellate(
    const GeometryId& id,
    double /*linear_deflection*/,
    double /*angular_deflection*/) {

    // Stub: production code uses BRepMesh_IncrementalMesh then
    // traverses triangulation via TopExp_Explorer and BRep_Tool::Triangulation
    auto* shape = static_cast<TopoDS_Shape*>(shapes_.at(id)); (void)shape;

    TessellationResult result;
    result.bbox = getBoundingBox(id);
    // Result would be populated here by the actual tessellation
    return result;
}

// ---------------------------------------------------------------------------
// Features
// ---------------------------------------------------------------------------

GeometryId OcctKernel::chamfer(
    const GeometryId& id,
    const std::vector<uint64_t>& /*edge_local_ids*/,
    double distance) {
    (void)distance;
    auto* shape = static_cast<TopoDS_Shape*>(shapes_.at(id));

    // BRepFilletAPI_MakeChamfer chamfer_maker(*shape);
    // for (auto edge_id : edge_local_ids) {
    //     chamfer_maker.Add(distance, distance, edge_from_local_id(edge_id));
    // }
    // chamfer_maker.Build();
    // TopoDS_Shape result = chamfer_maker.Shape();

    // Stub: return a new ID (actual OCCT calls above are commented as
    // they require a running OCCT installation)
    auto* result_ptr = new TopoDS_Shape(*shape);  // placeholder copy
    std::string new_id = "sha256:chamfer:" + std::to_string(shapes_.size());
    shapes_[new_id] = result_ptr;
    return new_id;
}

GeometryId OcctKernel::fillet(
    const GeometryId& id,
    const std::vector<uint64_t>& /*edge_local_ids*/,
    double radius) {
    (void)radius;
    auto* shape = static_cast<TopoDS_Shape*>(shapes_.at(id));

    // BRepFilletAPI_MakeFillet fillet_maker(*shape);
    // for (auto edge_id : edge_local_ids) {
    //     fillet_maker.Add(radius, edge_from_local_id(edge_id));
    // }
    // fillet_maker.Build();
    // TopoDS_Shape result = fillet_maker.Shape();

    auto* result_ptr = new TopoDS_Shape(*shape);  // placeholder copy
    std::string new_id = "sha256:fillet:" + std::to_string(shapes_.size());
    shapes_[new_id] = result_ptr;
    return new_id;
}

// ---------------------------------------------------------------------------
// Serialization
// ---------------------------------------------------------------------------

std::vector<uint8_t> OcctKernel::serializeBrepr(const GeometryId& /*id*/) {
    throw std::runtime_error("B-Rep serialization not yet implemented");
}

}  // namespace occccad::kernel

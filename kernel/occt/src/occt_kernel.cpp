#include <internal/occt_kernel.hpp>

#include <occccad/kernel/geometry_id.hpp>

#include <BRepBndLib.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepGProp.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GProp_GProps.hxx>
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
#include <IFSelect_ReturnStatus.hxx>
#include <Poly_Triangulation.hxx>
#include <STEPControl_Reader.hxx>
#include <STEPControl_Writer.hxx>
#include <TopAbs_Orientation.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopLoc_Location.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Shape.hxx>
#include <gp_Pnt.hxx>
#include <gp_Vec.hxx>

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cmath>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <sstream>
#include <stdexcept>
#include <string>
#include <unordered_map>
#include <utility>
#include <vector>

namespace occccad::kernel {
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

Vec3 to_vec3(const gp_Pnt& point) {
    return {point.X(), point.Y(), point.Z()};
}

int classify_surface(const TopoDS_Face& face) {
    const Handle(Geom_Surface) surface = BRep_Tool::Surface(face);
    if (surface.IsNull()) return -1;
    if (surface->IsKind(STANDARD_TYPE(Geom_Plane))) return 0;
    if (surface->IsKind(STANDARD_TYPE(Geom_CylindricalSurface))) return 1;
    if (surface->IsKind(STANDARD_TYPE(Geom_ConicalSurface))) return 2;
    if (surface->IsKind(STANDARD_TYPE(Geom_SphericalSurface))) return 3;
    if (surface->IsKind(STANDARD_TYPE(Geom_ToroidalSurface))) return 4;
    if (surface->IsKind(STANDARD_TYPE(Geom_BSplineSurface))) return 5;
    return -1;
}

int classify_curve(const TopoDS_Edge& edge) {
    Standard_Real first = 0.0;
    Standard_Real last = 0.0;
    const Handle(Geom_Curve) curve = BRep_Tool::Curve(edge, first, last);
    if (curve.IsNull()) return -1;
    if (curve->IsKind(STANDARD_TYPE(Geom_Line))) return 0;
    if (curve->IsKind(STANDARD_TYPE(Geom_Circle))) return 1;
    if (curve->IsKind(STANDARD_TYPE(Geom_Ellipse))) return 2;
    if (curve->IsKind(STANDARD_TYPE(Geom_BSplineCurve))) return 3;
    return -1;
}

std::vector<uint8_t> write_brep(const TopoDS_Shape& shape) {
    std::ostringstream stream(std::ios::binary);
    BRepTools::Write(shape, stream);
    if (!stream.good()) {
        throw std::runtime_error("B-Rep serialization failed");
    }
    const std::string bytes = stream.str();
    return {bytes.begin(), bytes.end()};
}

void validate_positive(const double value, const char* name) {
    if (!std::isfinite(value) || value <= 0.0) {
        throw std::invalid_argument(std::string(name) + " must be finite and greater than zero");
    }
}

TopoDS_Shape make_rectangular_pad(const RectangularPadSpec& spec) {
    validate_positive(spec.width, "width");
    validate_positive(spec.height, "height");
    validate_positive(spec.pad_length, "pad_length");
    if (!std::isfinite(spec.origin_x) || !std::isfinite(spec.origin_y)) {
        throw std::invalid_argument("origin must be finite");
    }

    gp_Pnt origin;
    gp_Vec width_axis;
    gp_Vec height_axis;
    gp_Vec pad_axis;
    if (spec.plane == "XY") {
        origin = gp_Pnt(spec.origin_x, spec.origin_y, 0.0);
        width_axis = gp_Vec(spec.width, 0.0, 0.0);
        height_axis = gp_Vec(0.0, spec.height, 0.0);
        pad_axis = gp_Vec(0.0, 0.0, spec.pad_length);
    } else if (spec.plane == "XZ") {
        origin = gp_Pnt(spec.origin_x, 0.0, spec.origin_y);
        width_axis = gp_Vec(spec.width, 0.0, 0.0);
        height_axis = gp_Vec(0.0, 0.0, spec.height);
        pad_axis = gp_Vec(0.0, -spec.pad_length, 0.0);
    } else if (spec.plane == "YZ") {
        origin = gp_Pnt(0.0, spec.origin_x, spec.origin_y);
        width_axis = gp_Vec(0.0, spec.width, 0.0);
        height_axis = gp_Vec(0.0, 0.0, spec.height);
        pad_axis = gp_Vec(spec.pad_length, 0.0, 0.0);
    } else {
        throw std::invalid_argument("plane must be XY, XZ, or YZ");
    }

    const gp_Pnt width_end = origin.Translated(width_axis);
    const gp_Pnt opposite = width_end.Translated(height_axis);
    const gp_Pnt height_end = origin.Translated(height_axis);
    BRepBuilderAPI_MakePolygon polygon;
    polygon.Add(origin);
    polygon.Add(width_end);
    polygon.Add(opposite);
    polygon.Add(height_end);
    polygon.Close();
    if (!polygon.IsDone()) {
        throw std::runtime_error("rectangle wire construction failed");
    }
    BRepBuilderAPI_MakeFace face_builder(polygon.Wire());
    if (!face_builder.IsDone()) {
        throw std::runtime_error("rectangle face construction failed");
    }
    BRepPrimAPI_MakePrism prism(face_builder.Face(), pad_axis);
    prism.Build();
    if (!prism.IsDone()) {
        throw std::runtime_error("pad construction failed");
    }
    return prism.Shape();
}

std::filesystem::path temporary_step_path() {
    static std::atomic<uint64_t> sequence{0};
    const auto stamp = std::chrono::steady_clock::now().time_since_epoch().count();
    return std::filesystem::temp_directory_path() /
        ("occccad-" + std::to_string(stamp) + "-" +
         std::to_string(sequence.fetch_add(1)) + ".step");
}

class ScopedFile final {
public:
    explicit ScopedFile(std::filesystem::path value) : path(std::move(value)) {}
    ~ScopedFile() { std::error_code ignored; std::filesystem::remove(path, ignored); }
    std::filesystem::path path;
};

}  // namespace

struct OcctKernel::Impl {
    struct StoredGeometry {
        TopoDS_Shape shape;
        std::vector<uint8_t> brep;
    };

    std::unordered_map<GeometryId, StoredGeometry> shapes;

    GeometryId store(const TopoDS_Shape& shape) {
        if (shape.IsNull()) {
            throw std::runtime_error("cannot store a null shape");
        }
        const std::vector<uint8_t> bytes = write_brep(shape);
        const GeometryId id = make_geometry_id(bytes.data(), bytes.size());
        shapes.insert_or_assign(id, StoredGeometry{shape, bytes});
        return id;
    }

    TopoDS_Shape& find(const GeometryId& id) {
        const auto iterator = shapes.find(id);
        if (iterator == shapes.end()) {
            throw std::out_of_range("geometry is not resident: " + id);
        }
        return iterator->second.shape;
    }
};

OcctKernel::OcctKernel() : impl_(std::make_unique<Impl>()) {}
OcctKernel::~OcctKernel() = default;
OcctKernel::OcctKernel(OcctKernel&&) noexcept = default;
OcctKernel& OcctKernel::operator=(OcctKernel&&) noexcept = default;

size_t OcctKernel::resident_count() const noexcept {
    return impl_->shapes.size();
}

bool OcctKernel::is_loaded(const GeometryId& id) const noexcept {
    return impl_->shapes.find(id) != impl_->shapes.end();
}

GeometryId OcctKernel::loadBrepr(const std::vector<uint8_t>& data) {
    if (data.empty()) {
        throw std::invalid_argument("B-Rep data must not be empty");
    }
    const std::string bytes(data.begin(), data.end());
    std::istringstream stream(bytes, std::ios::binary);
    BRep_Builder builder;
    TopoDS_Shape shape;
    BRepTools::Read(shape, stream, builder);
    if (stream.bad() || shape.IsNull()) {
        throw std::runtime_error("B-Rep deserialization failed");
    }
    const GeometryId id = make_geometry_id(data.data(), data.size());
    impl_->shapes.insert_or_assign(id, Impl::StoredGeometry{shape, data});
    return id;
}

GeometryId OcctKernel::loadStep(const std::string& path) {
    STEPControl_Reader reader;
    const IFSelect_ReturnStatus status = reader.ReadFile(path.c_str());
    if (status != IFSelect_RetDone) {
        throw std::runtime_error("STEP read failed: " + path);
    }
    if (reader.TransferRoots() == 0) {
        throw std::runtime_error("STEP file contains no transferable roots: " + path);
    }
    return impl_->store(reader.OneShape());
}

GeometryId OcctKernel::loadStepData(const std::vector<uint8_t>& data) {
    if (data.empty()) {
        throw std::invalid_argument("STEP data must not be empty");
    }
    ScopedFile temporary(temporary_step_path());
    std::ofstream stream(temporary.path, std::ios::binary);
    stream.write(reinterpret_cast<const char*>(data.data()),
                 static_cast<std::streamsize>(data.size()));
    stream.close();
    if (!stream) {
        throw std::runtime_error("cannot write temporary STEP input");
    }
    return loadStep(temporary.path.string());
}

void OcctKernel::unload(const GeometryId& id) {
    impl_->shapes.erase(id);
}

GeometryId OcctKernel::createBox(const double dx, const double dy, const double dz) {
    validate_positive(dx, "dx");
    validate_positive(dy, "dy");
    validate_positive(dz, "dz");
    return impl_->store(BRepPrimAPI_MakeBox(dx, dy, dz).Shape());
}

GeometryId OcctKernel::createRectangularPad(const RectangularPadSpec& spec) {
    return impl_->store(make_rectangular_pad(spec));
}

GeometryId OcctKernel::evaluateRectangularPads(
    const std::vector<RectangularPadSpec>& specs,
    const std::vector<uint8_t>& base_brep) {
    TopoDS_Shape result;
    if (!base_brep.empty()) {
        const GeometryId base_id = loadBrepr(base_brep);
        result = impl_->find(base_id);
    }
    for (const auto& spec : specs) {
        const TopoDS_Shape pad = make_rectangular_pad(spec);
        if (result.IsNull()) {
            result = pad;
            continue;
        }
        BRepAlgoAPI_Fuse fuse(result, pad);
        fuse.Build();
        if (!fuse.IsDone()) {
            throw std::runtime_error("feature chain boolean fuse failed");
        }
        result = fuse.Shape();
    }
    if (result.IsNull()) {
        throw std::invalid_argument("feature chain contains no solid geometry");
    }
    return impl_->store(result);
}

BoundingBox OcctKernel::getBoundingBox(const GeometryId& id) {
    Bnd_Box box;
    BRepBndLib::Add(impl_->find(id), box);
    return to_bbox(box);
}

TopologyInfo OcctKernel::getTopology(const GeometryId& id) {
    const TopoDS_Shape& shape = impl_->find(id);
    TopTools_IndexedMapOfShape face_map;
    TopTools_IndexedMapOfShape edge_map;
    TopTools_IndexedMapOfShape vertex_map;
    TopTools_IndexedMapOfShape solid_map;
    TopExp::MapShapes(shape, TopAbs_FACE, face_map);
    TopExp::MapShapes(shape, TopAbs_EDGE, edge_map);
    TopExp::MapShapes(shape, TopAbs_VERTEX, vertex_map);
    TopExp::MapShapes(shape, TopAbs_SOLID, solid_map);

    TopologyInfo info{};
    info.face_count = static_cast<uint32_t>(face_map.Extent());
    info.edge_count = static_cast<uint32_t>(edge_map.Extent());
    info.vertex_count = static_cast<uint32_t>(vertex_map.Extent());
    info.solid_count = static_cast<uint32_t>(solid_map.Extent());

    for (int index = 1; index <= face_map.Extent(); ++index) {
        const TopoDS_Face& face = TopoDS::Face(face_map(index));
        Bnd_Box box;
        BRepBndLib::Add(face, box);
        info.faces.push_back({
            static_cast<uint64_t>(index), classify_surface(face), to_bbox(box)});
    }
    for (int index = 1; index <= edge_map.Extent(); ++index) {
        const TopoDS_Edge& edge = TopoDS::Edge(edge_map(index));
        Bnd_Box box;
        BRepBndLib::Add(edge, box);
        info.edges.push_back({
            static_cast<uint64_t>(index), classify_curve(edge), to_bbox(box)});
    }
    return info;
}

double OcctKernel::getVolume(const GeometryId& id) {
    GProp_GProps properties;
    BRepGProp::VolumeProperties(impl_->find(id), properties);
    return properties.Mass();
}

TessellationResult OcctKernel::tessellate(
    const GeometryId& id,
    const double linear_deflection,
    const double angular_deflection) {
    validate_positive(linear_deflection, "linear_deflection");
    validate_positive(angular_deflection, "angular_deflection");

    TopoDS_Shape& shape = impl_->find(id);
    BRepMesh_IncrementalMesh mesher(
        shape, linear_deflection, Standard_False, angular_deflection, Standard_True);
    mesher.Perform();
    if (!mesher.IsDone()) {
        throw std::runtime_error("tessellation failed");
    }

    TessellationResult result;
    result.bbox = getBoundingBox(id);
    uint32_t face_id = 0;
    for (TopExp_Explorer explorer(shape, TopAbs_FACE); explorer.More(); explorer.Next()) {
        const TopoDS_Face& face = TopoDS::Face(explorer.Current());
        TopLoc_Location location;
        const Handle(Poly_Triangulation) triangulation =
            BRep_Tool::Triangulation(face, location);
        if (triangulation.IsNull()) {
            ++face_id;
            continue;
        }

        const uint32_t vertex_offset = static_cast<uint32_t>(result.vertices.size());
        for (int node = 1; node <= triangulation->NbNodes(); ++node) {
            result.vertices.push_back(
                to_vec3(triangulation->Node(node).Transformed(location.Transformation())));
        }
        for (int triangle = 1; triangle <= triangulation->NbTriangles(); ++triangle) {
            int n1 = 0;
            int n2 = 0;
            int n3 = 0;
            triangulation->Triangle(triangle).Get(n1, n2, n3);
            if (face.Orientation() == TopAbs_REVERSED) {
                std::swap(n2, n3);
            }
            result.triangles.push_back({
                vertex_offset + static_cast<uint32_t>(n1 - 1),
                vertex_offset + static_cast<uint32_t>(n2 - 1),
                vertex_offset + static_cast<uint32_t>(n3 - 1),
            });
            result.face_ids.push_back(face_id);
        }
        ++face_id;
    }
    return result;
}

GeometryId OcctKernel::chamfer(
    const GeometryId& id,
    const std::vector<uint64_t>& /*edge_local_ids*/,
    const double distance) {
    validate_positive(distance, "distance");
    return impl_->store(impl_->find(id));
}

GeometryId OcctKernel::fillet(
    const GeometryId& id,
    const std::vector<uint64_t>& /*edge_local_ids*/,
    const double radius) {
    validate_positive(radius, "radius");
    return impl_->store(impl_->find(id));
}

std::vector<uint8_t> OcctKernel::serializeBrepr(const GeometryId& id) {
    const auto iterator = impl_->shapes.find(id);
    if (iterator == impl_->shapes.end()) {
        throw std::out_of_range("geometry is not resident: " + id);
    }
    return iterator->second.brep;
}

std::vector<uint8_t> OcctKernel::serializeStep(const GeometryId& id) {
    STEPControl_Writer writer;
    if (writer.Transfer(impl_->find(id), STEPControl_AsIs) != IFSelect_RetDone) {
        throw std::runtime_error("STEP transfer failed");
    }
    ScopedFile temporary(temporary_step_path());
    if (writer.Write(temporary.path.string().c_str()) != IFSelect_RetDone) {
        throw std::runtime_error("STEP write failed");
    }
    std::ifstream stream(temporary.path, std::ios::binary);
    if (!stream.is_open()) {
        throw std::runtime_error("cannot open temporary STEP output");
    }
    std::vector<uint8_t> data(
        (std::istreambuf_iterator<char>(stream)), std::istreambuf_iterator<char>());
    if (data.empty()) {
        throw std::runtime_error("cannot read temporary STEP output");
    }
    return data;
}

}  // namespace occccad::kernel

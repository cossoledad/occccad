#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepCheck_Wire.hxx>
#include <BRepCheck_Result.hxx>
#include <BRepCheck_ListIteratorOfListOfStatus.hxx>
#include <BRepGProp.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepPrimAPI_MakeRevol.hxx>
#include <BRepTools.hxx>
#include <BRepTools_History.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GCPnts_AbscissaPoint.hxx>
#include <GProp_GProps.hxx>
#include <GeomAPI_Interpolate.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BSplineSurface.hxx>
#include <Geom_BezierCurve.hxx>
#include <Geom_BezierSurface.hxx>
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
#include <ShapeUpgrade_UnifySameDomain.hxx>
#include <ShapeFix_Shape.hxx>
#include <ShapeFix_Face.hxx>
#include <ShapeBuild_ReShape.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <TColStd_Array1OfInteger.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TColgp_Array1OfPnt.hxx>
#include <TColgp_HArray1OfPnt.hxx>
#include <TopAbs_Orientation.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopLoc_Location.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_MapOfShape.hxx>
#include <TopTools_ListIteratorOfListOfShape.hxx>
#include <TopTools_ListOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Shape.hxx>
#include <TopoDS_Vertex.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Ax1.hxx>
#include <gp_Ax2.hxx>
#include <gp_Dir.hxx>
#include <gp_Pnt.hxx>
#include <gp_Trsf.hxx>
#include <gp_Quaternion.hxx>
#include <gp_Vec.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/topology_naming.hpp>

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
#include <unordered_set>
#include <utility>
#include <vector>

namespace occccad::kernel {
namespace {

BoundingBox to_bbox(const Bnd_Box& occt_box) {
    BoundingBox result{};
    if (!occt_box.IsVoid()) {
        occt_box.Get(result.min.x, result.min.y, result.min.z, result.max.x, result.max.y,
                     result.max.z);
    }
    return result;
}

Vec3 to_vec3(const gp_Pnt& point) {
    return {point.X(), point.Y(), point.Z()};
}

int classify_surface(const TopoDS_Face& face) {
    switch (BRepAdaptor_Surface(face, Standard_True).GetType()) {
        case GeomAbs_Plane:
            return 0;
        case GeomAbs_Cylinder:
            return 1;
        case GeomAbs_Cone:
            return 2;
        case GeomAbs_Sphere:
            return 3;
        case GeomAbs_Torus:
            return 4;
        case GeomAbs_BSplineSurface:
            return 5;
        case GeomAbs_BezierSurface:
            return 6;
        case GeomAbs_SurfaceOfExtrusion:
            return 7;
        case GeomAbs_SurfaceOfRevolution:
            return 8;
        case GeomAbs_OffsetSurface:
            return 9;
        default:
            return -1;
    }
}

int classify_curve(const TopoDS_Edge& edge) {
    switch (BRepAdaptor_Curve(edge).GetType()) {
        case GeomAbs_Line:
            return 0;
        case GeomAbs_Circle:
            return 1;
        case GeomAbs_Ellipse:
            return 2;
        case GeomAbs_BSplineCurve:
            return 3;
        case GeomAbs_Hyperbola:
            return 4;
        case GeomAbs_Parabola:
            return 5;
        case GeomAbs_BezierCurve:
            return 6;
        case GeomAbs_OffsetCurve:
            return 7;
        default:
            return -1;
    }
}

TopologyProperty number_property(std::string name, const double value) {
    TopologyProperty result;
    result.name = std::move(name);
    result.kind = TopologyProperty::Kind::NUMBER;
    result.number_value = value;
    return result;
}

TopologyProperty integer_property(std::string name, const int64_t value) {
    TopologyProperty result;
    result.name = std::move(name);
    result.kind = TopologyProperty::Kind::INTEGER;
    result.integer_value = value;
    return result;
}

TopologyProperty boolean_property(std::string name, const bool value) {
    TopologyProperty result;
    result.name = std::move(name);
    result.kind = TopologyProperty::Kind::BOOLEAN;
    result.bool_value = value;
    return result;
}

TopologyProperty vector_property(std::string name, const gp_XYZ& value) {
    TopologyProperty result;
    result.name = std::move(name);
    result.kind = TopologyProperty::Kind::VECTOR;
    result.vector_value = {value.X(), value.Y(), value.Z()};
    return result;
}

// A surface parameter normal ignores the owning face's orientation. Solid
// boundary normals must include it (including walls facing into a cavity).
gp_Dir oriented_plane_normal(const TopoDS_Face& face, const gp_Pln& plane) {
    auto normal = plane.Axis().Direction();
    if (face.Orientation() == TopAbs_REVERSED)
        normal.Reverse();
    return normal;
}

void append_surface_properties(const TopoDS_Face& face, FaceInfo& output) {
    BRepAdaptor_Surface surface(face, Standard_True);
    output.properties.push_back(number_property("uFirst", surface.FirstUParameter()));
    output.properties.push_back(number_property("uLast", surface.LastUParameter()));
    output.properties.push_back(number_property("vFirst", surface.FirstVParameter()));
    output.properties.push_back(number_property("vLast", surface.LastVParameter()));
    output.properties.push_back(number_property("tolerance", BRep_Tool::Tolerance(face)));
    output.properties.push_back(
        integer_property("orientation", static_cast<int>(face.Orientation())));
    output.properties.push_back(boolean_property("uPeriodic", surface.IsUPeriodic()));
    output.properties.push_back(boolean_property("vPeriodic", surface.IsVPeriodic()));
    GProp_GProps area;
    BRepGProp::SurfaceProperties(face, area);
    output.properties.push_back(number_property("area", area.Mass()));
    switch (surface.GetType()) {
        case GeomAbs_Plane: {
            const auto value = surface.Plane();
            output.properties.push_back(vector_property("origin", value.Location().XYZ()));
            const auto normal = oriented_plane_normal(face, value);
            output.properties.push_back(vector_property("normal", normal.XYZ()));
            output.properties.push_back(
                vector_property("xDirection", value.XAxis().Direction().XYZ()));
            output.properties.push_back(
                vector_property("yDirection", normal.Crossed(value.XAxis().Direction()).XYZ()));
            break;
        }
        case GeomAbs_Cylinder: {
            const auto value = surface.Cylinder();
            output.properties.push_back(vector_property("origin", value.Location().XYZ()));
            output.properties.push_back(vector_property("axis", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("radius", value.Radius()));
            break;
        }
        case GeomAbs_Cone: {
            const auto value = surface.Cone();
            output.properties.push_back(vector_property("origin", value.Location().XYZ()));
            output.properties.push_back(vector_property("axis", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("referenceRadius", value.RefRadius()));
            output.properties.push_back(number_property("semiAngle", value.SemiAngle()));
            break;
        }
        case GeomAbs_Sphere: {
            const auto value = surface.Sphere();
            output.properties.push_back(vector_property("center", value.Location().XYZ()));
            output.properties.push_back(number_property("radius", value.Radius()));
            break;
        }
        case GeomAbs_Torus: {
            const auto value = surface.Torus();
            output.properties.push_back(vector_property("center", value.Location().XYZ()));
            output.properties.push_back(vector_property("axis", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("majorRadius", value.MajorRadius()));
            output.properties.push_back(number_property("minorRadius", value.MinorRadius()));
            break;
        }
        case GeomAbs_BSplineSurface: {
            const auto value = surface.BSpline();
            output.properties.push_back(integer_property("uDegree", value->UDegree()));
            output.properties.push_back(integer_property("vDegree", value->VDegree()));
            output.properties.push_back(integer_property("uPoles", value->NbUPoles()));
            output.properties.push_back(integer_property("vPoles", value->NbVPoles()));
            output.properties.push_back(integer_property("uKnots", value->NbUKnots()));
            output.properties.push_back(integer_property("vKnots", value->NbVKnots()));
            output.properties.push_back(boolean_property("uRational", value->IsURational()));
            output.properties.push_back(boolean_property("vRational", value->IsVRational()));
            break;
        }
        case GeomAbs_BezierSurface: {
            const auto value = surface.Bezier();
            output.properties.push_back(integer_property("uDegree", value->UDegree()));
            output.properties.push_back(integer_property("vDegree", value->VDegree()));
            output.properties.push_back(integer_property("uPoles", value->NbUPoles()));
            output.properties.push_back(integer_property("vPoles", value->NbVPoles()));
            output.properties.push_back(boolean_property("uRational", value->IsURational()));
            output.properties.push_back(boolean_property("vRational", value->IsVRational()));
            break;
        }
        case GeomAbs_SurfaceOfExtrusion:
            output.properties.push_back(vector_property("direction", surface.Direction().XYZ()));
            break;
        case GeomAbs_SurfaceOfRevolution:
            output.properties.push_back(
                vector_property("axisOrigin", surface.AxeOfRevolution().Location().XYZ()));
            output.properties.push_back(
                vector_property("axisDirection", surface.AxeOfRevolution().Direction().XYZ()));
            break;
        default:
            break;
    }
}

void append_curve_properties(const TopoDS_Edge& edge, EdgeInfo& output) {
    BRepAdaptor_Curve curve(edge);
    const double first = curve.FirstParameter();
    const double last = curve.LastParameter();
    output.properties.push_back(number_property("firstParameter", first));
    output.properties.push_back(number_property("lastParameter", last));
    output.properties.push_back(number_property("tolerance", BRep_Tool::Tolerance(edge)));
    if (std::isfinite(first) && std::isfinite(last)) {
        output.properties.push_back(
            number_property("length", GCPnts_AbscissaPoint::Length(curve, first, last)));
    }
    output.properties.push_back(boolean_property("closed", curve.IsClosed()));
    output.properties.push_back(boolean_property("periodic", curve.IsPeriodic()));
    switch (curve.GetType()) {
        case GeomAbs_Line: {
            const auto value = curve.Line();
            output.properties.push_back(vector_property("origin", value.Location().XYZ()));
            output.properties.push_back(vector_property("direction", value.Direction().XYZ()));
            break;
        }
        case GeomAbs_Circle: {
            const auto value = curve.Circle();
            output.properties.push_back(vector_property("center", value.Location().XYZ()));
            output.properties.push_back(vector_property("normal", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("radius", value.Radius()));
            break;
        }
        case GeomAbs_Ellipse: {
            const auto value = curve.Ellipse();
            output.properties.push_back(vector_property("center", value.Location().XYZ()));
            output.properties.push_back(vector_property("normal", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("majorRadius", value.MajorRadius()));
            output.properties.push_back(number_property("minorRadius", value.MinorRadius()));
            break;
        }
        case GeomAbs_BSplineCurve: {
            const auto value = curve.BSpline();
            output.properties.push_back(integer_property("degree", value->Degree()));
            output.properties.push_back(integer_property("poles", value->NbPoles()));
            output.properties.push_back(integer_property("knots", value->NbKnots()));
            output.properties.push_back(boolean_property("rational", value->IsRational()));
            break;
        }
        case GeomAbs_Hyperbola: {
            const auto value = curve.Hyperbola();
            output.properties.push_back(vector_property("center", value.Location().XYZ()));
            output.properties.push_back(vector_property("normal", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("majorRadius", value.MajorRadius()));
            output.properties.push_back(number_property("minorRadius", value.MinorRadius()));
            break;
        }
        case GeomAbs_Parabola: {
            const auto value = curve.Parabola();
            output.properties.push_back(vector_property("location", value.Location().XYZ()));
            output.properties.push_back(vector_property("axis", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("focal", value.Focal()));
            break;
        }
        case GeomAbs_BezierCurve: {
            const auto value = curve.Bezier();
            output.properties.push_back(integer_property("degree", value->Degree()));
            output.properties.push_back(integer_property("poles", value->NbPoles()));
            output.properties.push_back(boolean_property("rational", value->IsRational()));
            break;
        }
        default:
            break;
    }
}

std::vector<Vec3> sample_edge(const TopoDS_Edge& edge) {
    BRepAdaptor_Curve curve(edge);
    const double first = curve.FirstParameter();
    const double last = curve.LastParameter();
    if (!std::isfinite(first) || !std::isfinite(last))
        return {};
    int samples = 24;
    if (curve.GetType() == GeomAbs_Line)
        samples = 2;
    if (curve.GetType() == GeomAbs_Circle || curve.GetType() == GeomAbs_Ellipse)
        samples = 49;
    std::vector<Vec3> result;
    result.reserve(static_cast<size_t>(samples));
    for (int sample = 0; sample < samples; ++sample) {
        const double ratio = samples == 1 ? 0.0 : static_cast<double>(sample) / (samples - 1);
        result.push_back(to_vec3(curve.Value(first + (last - first) * ratio)));
    }
    return result;
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

struct ProfileFrame {
    gp_Pnt origin;
    gp_Dir normal;
    gp_Dir u_direction;
    gp_Dir v_direction;
};

ProfileFrame profile_frame(const ProfilePadSpec& spec) {
    const gp_Vec explicit_normal(spec.plane_normal.x, spec.plane_normal.y, spec.plane_normal.z);
    if (explicit_normal.Magnitude() > 1.0e-9) {
        const gp_Vec explicit_u(spec.plane_u_direction.x, spec.plane_u_direction.y,
                                spec.plane_u_direction.z);
        if (explicit_u.Magnitude() <= 1.0e-9)
            throw std::invalid_argument("explicit sketch plane requires a U direction");
        const gp_Dir normal(explicit_normal);
        const gp_Dir u(explicit_u);
        if (std::abs(normal.Dot(u)) > 1.0e-8)
            throw std::invalid_argument("sketch plane U direction must be perpendicular to normal");
        return {{spec.plane_origin.x, spec.plane_origin.y, spec.plane_origin.z},
                normal,
                u,
                gp_Dir(normal.Crossed(u))};
    }
    if (spec.plane == "XY")
        return {{0, 0, 0}, {0, 0, 1}, {1, 0, 0}, {0, 1, 0}};
    if (spec.plane == "XZ")
        return {{0, 0, 0}, {0, -1, 0}, {1, 0, 0}, {0, 0, 1}};
    if (spec.plane == "YZ")
        return {{0, 0, 0}, {1, 0, 0}, {0, 1, 0}, {0, 0, 1}};
    throw std::invalid_argument("sketch support requires an explicit frame or XY/XZ/YZ datum");
}

gp_Pnt profile_point(const ProfileFrame& frame, const Vec2& point) {
    return frame.origin.Translated(gp_Vec(frame.u_direction).Multiplied(point.x) +
                                   gp_Vec(frame.v_direction).Multiplied(point.y));
}

gp_Ax2 profile_axes(const ProfileFrame& frame, const Vec2& center) {
    return {profile_point(frame, center), frame.normal, frame.u_direction};
}

TopoDS_Edge make_profile_edge(const ProfileCurveSpec& curve, const ProfileFrame& frame) {
    TopoDS_Edge edge;
    if (curve.kind == "LINE") {
        if (std::hypot(curve.end.x - curve.start.x, curve.end.y - curve.start.y) <=
            topology_linear_tolerance_meters * 1000.0)
            throw std::invalid_argument("DEGENERATE_PROFILE_EDGE: line is below naming tolerance");
        edge = BRepBuilderAPI_MakeEdge(profile_point(frame, curve.start),
                                       profile_point(frame, curve.end));
    } else if (curve.kind == "CIRCLE") {
        validate_positive(curve.radius, "profile circle radius");
        Handle(Geom_Circle) circle =
            new Geom_Circle(profile_axes(frame, curve.center), curve.radius);
        edge = BRepBuilderAPI_MakeEdge(circle);
    } else if (curve.kind == "ARC") {
        validate_positive(curve.radius, "profile arc radius");
        double end = curve.end_angle;
        while (end <= curve.start_angle)
            end += 2.0 * 3.14159265358979323846;
        if (curve.radius * (end - curve.start_angle) <=
            topology_linear_tolerance_meters * 1000.0)
            throw std::invalid_argument("DEGENERATE_PROFILE_EDGE: arc is below naming tolerance");
        Handle(Geom_Circle) circle =
            new Geom_Circle(profile_axes(frame, curve.center), curve.radius);
        edge = BRepBuilderAPI_MakeEdge(circle, curve.start_angle, end);
    } else if (curve.kind == "SPLINE") {
        if (curve.control_points.size() < 3U)
            throw std::invalid_argument("profile spline requires at least three control points");
        const int point_count = static_cast<int>(curve.control_points.size());
        Handle(TColgp_HArray1OfPnt) points = new TColgp_HArray1OfPnt(1, point_count);
        for (std::size_t index = 0; index < curve.control_points.size(); ++index)
            points->SetValue(static_cast<int>(index) + 1,
                             profile_point(frame, curve.control_points[index]));
        GeomAPI_Interpolate interpolation(points, curve.closed, 1.0e-7);
        interpolation.Perform();
        if (!interpolation.IsDone())
            throw std::runtime_error("profile interpolation spline construction failed");
        Handle(Geom_BSplineCurve) spline = interpolation.Curve();
        edge = BRepBuilderAPI_MakeEdge(spline);
    } else {
        throw std::invalid_argument("unsupported profile curve kind: " + curve.kind);
    }
    if (edge.IsNull())
        throw std::runtime_error("profile edge construction failed");
    return curve.reversed ? TopoDS::Edge(edge.Reversed()) : edge;
}

Vec2 profile_curve_endpoint(const ProfileCurveSpec& curve, const bool start) {
    if (curve.kind == "LINE")
        return start ? curve.start : curve.end;
    if (curve.kind == "ARC") {
        const double angle = start ? curve.start_angle : curve.end_angle;
        return {curve.center.x + curve.radius * std::cos(angle),
                curve.center.y + curve.radius * std::sin(angle)};
    }
    if (curve.kind == "SPLINE" && !curve.control_points.empty())
        return start ? curve.control_points.front() : curve.control_points.back();
    if (curve.kind == "CIRCLE")
        return {curve.center.x + curve.radius, curve.center.y};
    return start ? curve.start : curve.end;
}

struct NamedShape {
    SemanticTopologyRef ref;
    TopoDS_Shape shape;
};

struct ToolBuild {
    TopoDS_Shape shape;
    std::vector<NamedShape> named;
    std::vector<TopologyLineage> generated;
    std::string feature_id;
    bool topology_history_complete{true};
    std::vector<std::string> diagnostics;
};

SemanticTopologyRef profile_source(const ProfilePadSpec& spec, const std::string& slot,
                                   const std::string& source_id) {
    return {spec.profile_feature_id, slot + "/" + source_id, {source_id}};
}

void append_generated(ToolBuild& build, const SemanticTopologyRef& source,
                      const SemanticTopologyRef& result, const TopoDS_Shape& shape) {
    if (shape.IsNull())
        return;
    build.named.push_back({result, shape});
    build.generated.push_back({{source}, result, TopologyLineageKind::generated, {}});
}

template <typename Algorithm>
std::vector<NamedShape> map_named_shapes(const std::vector<NamedShape>& sources,
                                         Algorithm& algorithm, const TopoDS_Shape& result) {
    TopTools_IndexedMapOfShape result_faces;
    TopTools_IndexedMapOfShape result_edges;
    TopTools_IndexedMapOfShape result_vertices;
    TopExp::MapShapes(result, TopAbs_FACE, result_faces);
    TopExp::MapShapes(result, TopAbs_EDGE, result_edges);
    TopExp::MapShapes(result, TopAbs_VERTEX, result_vertices);
    const auto contains = [&](const TopoDS_Shape& shape) {
        switch (shape.ShapeType()) {
            case TopAbs_FACE:
                return result_faces.Contains(shape);
            case TopAbs_EDGE:
                return result_edges.Contains(shape);
            case TopAbs_VERTEX:
                return result_vertices.Contains(shape);
            default:
                return false;
        }
    };
    std::vector<NamedShape> mapped;
    for (const auto& source : sources) {
        TopTools_ListOfShape candidates = algorithm.Modified(source.shape);
        const auto generated = algorithm.Generated(source.shape);
        for (TopTools_ListIteratorOfListOfShape it(generated); it.More(); it.Next())
            candidates.Append(it.Value());
        if (candidates.IsEmpty() && !algorithm.IsDeleted(source.shape) && contains(source.shape)) {
            candidates.Append(source.shape);
        }
        for (TopTools_ListIteratorOfListOfShape it(candidates); it.More(); it.Next()) {
            if (it.Value().ShapeType() == source.shape.ShapeType() && contains(it.Value()))
                mapped.push_back({source.ref, it.Value()});
        }
    }
    return mapped;
}

std::vector<NamedShape> map_named_history(const std::vector<NamedShape>& sources,
                                          const Handle(BRepTools_History) & history,
                                          const TopoDS_Shape& result) {
    TopTools_IndexedMapOfShape result_faces;
    TopTools_IndexedMapOfShape result_edges;
    TopTools_IndexedMapOfShape result_vertices;
    TopExp::MapShapes(result, TopAbs_FACE, result_faces);
    TopExp::MapShapes(result, TopAbs_EDGE, result_edges);
    TopExp::MapShapes(result, TopAbs_VERTEX, result_vertices);
    const auto contains = [&](const TopoDS_Shape& shape) {
        switch (shape.ShapeType()) {
            case TopAbs_FACE:
                return result_faces.Contains(shape);
            case TopAbs_EDGE:
                return result_edges.Contains(shape);
            case TopAbs_VERTEX:
                return result_vertices.Contains(shape);
            default:
                return false;
        }
    };
    std::vector<NamedShape> mapped;
    for (const auto& source : sources) {
        TopTools_ListOfShape candidates = history->Modified(source.shape);
        const auto& generated = history->Generated(source.shape);
        for (TopTools_ListIteratorOfListOfShape it(generated); it.More(); it.Next())
            candidates.Append(it.Value());
        if (candidates.IsEmpty() && !history->IsRemoved(source.shape) && contains(source.shape))
            candidates.Append(source.shape);
        for (TopTools_ListIteratorOfListOfShape it(candidates); it.More(); it.Next())
            if (it.Value().ShapeType() == source.shape.ShapeType() && contains(it.Value()))
                mapped.push_back({source.ref, it.Value()});
    }
    return mapped;
}

ToolBuild make_profile_tool(const ProfilePadSpec& spec) {
    if (spec.regions.empty())
        throw std::invalid_argument("solid feature requires at least one profile region");
    const std::string generator = spec.generator.empty() ? "LINEAR_EXTRUDE" : spec.generator;
    if (generator == "LINEAR_EXTRUDE") {
        validate_positive(spec.pad_length, "extrude length");
    } else if (generator == "REVOLVE") {
        validate_positive(spec.revolve_angle, "revolve angle");
        if (spec.revolve_angle > 2.0 * 3.14159265358979323846 + 1.0e-12)
            throw std::invalid_argument("revolve angle must not exceed 2*pi");
        if (std::hypot(spec.axis_end.x - spec.axis_start.x, spec.axis_end.y - spec.axis_start.y) <=
            1.0e-9)
            throw std::invalid_argument("revolve axis is degenerate");
    } else {
        throw std::invalid_argument("unsupported solid generator: " + generator);
    }
    const ProfileFrame frame = profile_frame(spec);
    ToolBuild result;
    result.feature_id = spec.feature_id;
    for (const auto& region : spec.regions) {
        std::vector<std::pair<const ProfileCurveSpec*, TopoDS_Edge>> profile_edges;
        const auto build_wire = [&](const ProfileLoopSpec& loop) {
            BRepBuilderAPI_MakeWire wire;
            for (const auto& curve : loop.curves) {
                auto edge = make_profile_edge(curve, frame);
                wire.Add(edge);
                profile_edges.emplace_back(&curve, edge);
            }
            if (!wire.IsDone())
                throw std::runtime_error("profile wire construction failed: " + loop.id);
            return wire.Wire();
        };
        BRepBuilderAPI_MakeFace face_builder(build_wire(region.outer));
        for (const auto& hole : region.holes)
            face_builder.Add(build_wire(hole));
        face_builder.Build();
        if (!face_builder.IsDone() || !BRepCheck_Analyzer(face_builder.Face()).IsValid())
            throw std::runtime_error("profile face is invalid: " + region.id);
        std::vector<TopoDS_Edge> face_edges;
        for (TopExp_Explorer edge(face_builder.Face(), TopAbs_EDGE); edge.More(); edge.Next())
            face_edges.push_back(TopoDS::Edge(edge.Current()));
        if (face_edges.size() != profile_edges.size())
            throw std::runtime_error("profile face boundary identity mismatch: " + region.id);
        std::vector<TopoDS_Edge> named_face_edges;
        for (const auto& [curve, source_edge] : profile_edges) {
            (void)curve;
            Bnd_Box source_box;
            BRepBndLib::Add(source_edge, source_box);
            const auto source_bounds = to_bbox(source_box);
            GProp_GProps source_properties;
            BRepGProp::LinearProperties(source_edge, source_properties);
            std::vector<TopoDS_Edge> matches;
            for (const auto& candidate : face_edges) {
                Bnd_Box candidate_box;
                BRepBndLib::Add(candidate, candidate_box);
                const auto bounds = to_bbox(candidate_box);
                GProp_GProps properties;
                BRepGProp::LinearProperties(candidate, properties);
                const auto close = [](const double left, const double right) {
                    return std::abs(left - right) <= 1.0e-7;
                };
                const auto& a = source_properties.CentreOfMass();
                const auto& b = properties.CentreOfMass();
                if (classify_curve(candidate) == classify_curve(source_edge) &&
                    close(source_properties.Mass(), properties.Mass()) && close(a.X(), b.X()) &&
                    close(a.Y(), b.Y()) && close(a.Z(), b.Z()) &&
                    close(source_bounds.min.x, bounds.min.x) &&
                    close(source_bounds.min.y, bounds.min.y) &&
                    close(source_bounds.min.z, bounds.min.z) &&
                    close(source_bounds.max.x, bounds.max.x) &&
                    close(source_bounds.max.y, bounds.max.y) &&
                    close(source_bounds.max.z, bounds.max.z))
                    matches.push_back(candidate);
            }
            if (matches.size() != 1U)
                throw std::runtime_error("profile edge identity was not preserved: " + region.id);
            named_face_edges.push_back(matches.front());
        }
        struct NamedProfileVertex {
            TopoDS_Vertex shape;
            std::vector<std::string> endpoint_ids;
        };
        std::vector<NamedProfileVertex> profile_vertices;
        const auto add_profile_vertex = [&](const TopoDS_Vertex& vertex,
                                            const std::string& endpoint_id) {
            auto existing = std::find_if(profile_vertices.begin(), profile_vertices.end(),
                                         [&](const auto& value) {
                                             return value.shape.IsSame(vertex);
                                         });
            if (existing == profile_vertices.end()) {
                profile_vertices.push_back({vertex, {endpoint_id}});
            } else {
                existing->endpoint_ids.push_back(endpoint_id);
            }
        };
        for (std::size_t edge_index = 0; edge_index < profile_edges.size(); ++edge_index) {
            TopoDS_Vertex first;
            TopoDS_Vertex last;
            TopExp::Vertices(named_face_edges[edge_index], first, last);
            if (first.IsNull() || last.IsNull())
                continue;
            const auto* curve = profile_edges[edge_index].first;
            const auto expected_start = profile_point(frame, profile_curve_endpoint(*curve, true));
            const auto first_point = BRep_Tool::Pnt(first);
            const auto last_point = BRep_Tool::Pnt(last);
            const bool first_is_start = first_point.Distance(expected_start) <=
                                        last_point.Distance(expected_start);
            add_profile_vertex(first_is_start ? first : last, curve->entity_id + "/START");
            add_profile_vertex(first_is_start ? last : first, curve->entity_id + "/END");
        }
        for (auto& vertex : profile_vertices) {
            std::sort(vertex.endpoint_ids.begin(), vertex.endpoint_ids.end());
            vertex.endpoint_ids.erase(
                std::unique(vertex.endpoint_ids.begin(), vertex.endpoint_ids.end()),
                vertex.endpoint_ids.end());
        }
        TopoDS_Shape generated;
        if (generator == "LINEAR_EXTRUDE") {
            gp_Vec direction(frame.normal);
            direction.Multiply(spec.reversed ? -spec.pad_length : spec.pad_length);
            BRepPrimAPI_MakePrism prism(face_builder.Face(), direction);
            prism.Build();
            if (!prism.IsDone())
                throw std::runtime_error("profile prism failed: " + region.id);
            generated = prism.Shape();
            const auto start_ref =
                SemanticTopologyRef{spec.feature_id, "START_CAP/" + region.id, {region.id}};
            const auto end_ref =
                SemanticTopologyRef{spec.feature_id, "END_CAP/" + region.id, {region.id}};
            append_generated(result, profile_source(spec, "PROFILE_REGION", region.id), start_ref,
                             prism.FirstShape());
            append_generated(result, profile_source(spec, "PROFILE_REGION", region.id), end_ref,
                             prism.LastShape());
            for (std::size_t edge_index = 0; edge_index < profile_edges.size(); ++edge_index) {
                const auto* curve = profile_edges[edge_index].first;
                const auto side_ref =
                    SemanticTopologyRef{spec.feature_id,
                                        "SIDE_FROM_PROFILE_EDGE/" + curve->entity_id,
                                        {region.id, curve->entity_id}};
                const auto source_ref = profile_source(spec, "PROFILE_EDGE", curve->entity_id);
                const auto sides = prism.Generated(named_face_edges[edge_index]);
                for (TopTools_ListIteratorOfListOfShape it(sides); it.More(); it.Next()) {
                    append_generated(result, source_ref, side_ref, it.Value());
                }
                append_generated(
                    result, source_ref,
                    {spec.feature_id, "START_BOUNDARY_FROM_PROFILE_EDGE/" + curve->entity_id,
                     {region.id, curve->entity_id}},
                    prism.FirstShape(named_face_edges[edge_index]));
                append_generated(
                    result, source_ref,
                    {spec.feature_id, "END_BOUNDARY_FROM_PROFILE_EDGE/" + curve->entity_id,
                     {region.id, curve->entity_id}},
                    prism.LastShape(named_face_edges[edge_index]));
            }
            for (const auto& vertex : profile_vertices) {
                std::ostringstream identity;
                for (const auto& endpoint : vertex.endpoint_ids)
                    identity << endpoint << '\0';
                const auto suffix = make_geometry_id(identity.str()).substr(7, 16);
                const SemanticTopologyRef source_ref{
                    spec.profile_feature_id, "PROFILE_VERTEX/" + suffix, vertex.endpoint_ids};
                append_generated(
                    result, source_ref,
                    {spec.feature_id, "START_VERTEX_FROM_PROFILE_ENDPOINTS/" + suffix,
                     vertex.endpoint_ids},
                    prism.FirstShape(vertex.shape));
                append_generated(
                    result, source_ref,
                    {spec.feature_id, "END_VERTEX_FROM_PROFILE_ENDPOINTS/" + suffix,
                     vertex.endpoint_ids},
                    prism.LastShape(vertex.shape));
                const auto vertical = prism.Generated(vertex.shape);
                for (TopTools_ListIteratorOfListOfShape it(vertical); it.More(); it.Next())
                    append_generated(
                        result, source_ref,
                        {spec.feature_id, "VERTICAL_FROM_PROFILE_ENDPOINTS/" + suffix,
                         vertex.endpoint_ids},
                        it.Value());
            }
        } else {
            const gp_Pnt start = profile_point(frame, spec.axis_start);
            const gp_Pnt end = profile_point(frame, spec.axis_end);
            gp_Dir direction(gp_Vec(start, end));
            if (spec.reversed)
                direction.Reverse();
            BRepPrimAPI_MakeRevol revolve(face_builder.Face(), gp_Ax1(start, direction),
                                          spec.revolve_angle, Standard_True);
            revolve.Build();
            if (!revolve.IsDone())
                throw std::runtime_error("profile revolve failed: " + region.id);
            generated = revolve.Shape();
            result.topology_history_complete = false;
            result.diagnostics.push_back("TOPOLOGY_HISTORY_UNSUPPORTED_GENERATOR:REVOLVE");
        }
        if (generated.IsNull() || !BRepCheck_Analyzer(generated).IsValid())
            throw std::runtime_error("generated solid tool is invalid: " + region.id);
        if (result.shape.IsNull())
            result.shape = generated;
        else {
            BRepAlgoAPI_Fuse fuse(result.shape, generated);
            fuse.Build();
            if (!fuse.IsDone())
                throw std::runtime_error("profile tool region fuse failed");
            result.named = map_named_shapes(result.named, fuse, fuse.Shape());
            result.shape = fuse.Shape();
        }
    }
    return result;
}

double shape_volume(const TopoDS_Shape& shape) {
    GProp_GProps properties;
    BRepGProp::VolumeProperties(shape, properties);
    return properties.Mass();
}

int solid_count(const TopoDS_Shape& shape) {
    TopTools_IndexedMapOfShape solids;
    TopExp::MapShapes(shape, TopAbs_SOLID, solids);
    return solids.Extent();
}

struct BodyOperationResult {
    TopoDS_Shape shape;
    std::vector<NamedShape> named;
    std::vector<TopologyLineage> derived;
    std::vector<std::string> diagnostics;
};

bool same_ref(const SemanticTopologyRef& left, const SemanticTopologyRef& right) {
    return left.feature_id == right.feature_id && left.output_slot == right.output_slot &&
           left.source_ids == right.source_ids;
}

std::string ref_key(const SemanticTopologyRef& ref) {
    std::string key = ref.feature_id + "\n" + ref.output_slot;
    for (const auto& source : ref.source_ids)
        key += "\n" + source;
    return key;
}

void sort_refs(std::vector<SemanticTopologyRef>& refs) {
    std::sort(refs.begin(), refs.end(), [](const auto& left, const auto& right) {
        return ref_key(left) < ref_key(right);
    });
}

SelectionEvidence face_evidence(const TopoDS_Face& face) {
    SelectionEvidence evidence;
    evidence.geometry_type = std::to_string(classify_surface(face));
    GProp_GProps properties;
    BRepGProp::SurfaceProperties(face, properties);
    evidence.measure_si = properties.Mass() * 1.0e-6;
    evidence.measure_dimension = "AREA";
    evidence.centroid = to_vec3(properties.CentreOfMass());
    BRepAdaptor_Surface surface(face);
    if (surface.GetType() == GeomAbs_Plane) {
        evidence.geometry_type = "PLANE";
        evidence.origin = to_vec3(surface.Plane().Location());
        const auto direction = oriented_plane_normal(face, surface.Plane());
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
    } else if (surface.GetType() == GeomAbs_Cylinder) {
        evidence.geometry_type = "CYLINDER";
        evidence.origin = to_vec3(surface.Cylinder().Location());
        const auto direction = surface.Cylinder().Axis().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
    }
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << *evidence.measure_si << '|' << evidence.centroid.x
              << ',' << evidence.centroid.y << ',' << evidence.centroid.z << '|'
              << evidence.origin.x << ',' << evidence.origin.y << ',' << evidence.origin.z << '|'
              << evidence.direction.x << ',' << evidence.direction.y << ',' << evidence.direction.z;
    evidence.evidence_digest = make_geometry_id(canonical.str());
    return evidence;
}

std::string curve_geometry_type(const TopoDS_Edge& edge) {
    switch (BRepAdaptor_Curve(edge).GetType()) {
        case GeomAbs_Line:
            return "LINE";
        case GeomAbs_Circle:
            return "CIRCLE";
        case GeomAbs_Ellipse:
            return "ELLIPSE";
        case GeomAbs_BSplineCurve:
            return "BSPLINE";
        case GeomAbs_BezierCurve:
            return "BEZIER";
        case GeomAbs_Hyperbola:
            return "HYPERBOLA";
        case GeomAbs_Parabola:
            return "PARABOLA";
        case GeomAbs_OffsetCurve:
            return "OFFSET";
        default:
            return "OTHER_CURVE";
    }
}

SelectionEvidence edge_evidence(const TopoDS_Edge& edge) {
    SelectionEvidence evidence;
    evidence.geometry_type = curve_geometry_type(edge);
    GProp_GProps properties;
    BRepGProp::LinearProperties(edge, properties);
    evidence.measure_si = properties.Mass() * 1.0e-3;
    evidence.measure_dimension = "LENGTH";
    evidence.centroid = to_vec3(properties.CentreOfMass());
    BRepAdaptor_Curve curve(edge);
    if (std::isfinite(curve.FirstParameter()))
        evidence.parameter_start = curve.FirstParameter();
    if (std::isfinite(curve.LastParameter()))
        evidence.parameter_end = curve.LastParameter();
    if (curve.GetType() == GeomAbs_Line) {
        evidence.origin = to_vec3(curve.Line().Location());
        const auto direction = curve.Line().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
    } else if (curve.GetType() == GeomAbs_Circle) {
        evidence.origin = to_vec3(curve.Circle().Location());
        const auto direction = curve.Circle().Axis().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
    }
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << *evidence.measure_si << '|'
              << evidence.centroid.x << ',' << evidence.centroid.y << ','
              << evidence.centroid.z << '|' << evidence.origin.x << ',' << evidence.origin.y
              << ',' << evidence.origin.z << '|' << evidence.direction.x << ','
              << evidence.direction.y << ',' << evidence.direction.z << '|'
              << curve.FirstParameter() << ',' << curve.LastParameter();
    evidence.evidence_digest = make_geometry_id(canonical.str());
    return evidence;
}

SelectionEvidence vertex_evidence(const TopoDS_Vertex& vertex) {
    SelectionEvidence evidence;
    evidence.geometry_type = "POINT";
    evidence.measure_dimension = "NONE";
    evidence.centroid = to_vec3(BRep_Tool::Pnt(vertex));
    evidence.origin = evidence.centroid;
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << evidence.centroid.x << ','
              << evidence.centroid.y << ',' << evidence.centroid.z;
    evidence.evidence_digest = make_geometry_id(canonical.str());
    return evidence;
}

PersistentTopologyType persistent_topology_type(const TopoDS_Shape& shape) {
    switch (shape.ShapeType()) {
        case TopAbs_FACE:
            return PersistentTopologyType::face;
        case TopAbs_EDGE:
            return PersistentTopologyType::edge;
        case TopAbs_VERTEX:
            return PersistentTopologyType::vertex;
        default:
            return PersistentTopologyType::unspecified;
    }
}

SelectionEvidence topology_evidence(const TopoDS_Shape& shape) {
    switch (shape.ShapeType()) {
        case TopAbs_FACE:
            return face_evidence(TopoDS::Face(shape));
        case TopAbs_EDGE:
            return edge_evidence(TopoDS::Edge(shape));
        case TopAbs_VERTEX:
            return vertex_evidence(TopoDS::Vertex(shape));
        default:
            throw std::runtime_error("TOPOLOGY_HISTORY_UNSUPPORTED_SHAPE_TYPE");
    }
}

void append_semantic_role(SelectionEvidence& evidence, const SemanticTopologyRef& ref,
                          const PersistentTopologyType type) {
    if (type == PersistentTopologyType::edge) {
        if (ref.output_slot.rfind("START_BOUNDARY_FROM_PROFILE_EDGE/", 0) == 0)
            evidence.endpoint_role = "START_CAP_BOUNDARY";
        else if (ref.output_slot.rfind("END_BOUNDARY_FROM_PROFILE_EDGE/", 0) == 0)
            evidence.endpoint_role = "END_CAP_BOUNDARY";
        else if (ref.output_slot.rfind("VERTICAL_FROM_PROFILE_ENDPOINTS/", 0) == 0)
            evidence.endpoint_role = "VERTICAL_GENERATED_EDGE";
        else
            evidence.endpoint_role = "DERIVED_EDGE";
    } else if (type == PersistentTopologyType::vertex) {
        if (ref.output_slot.rfind("START_VERTEX_FROM_PROFILE_ENDPOINTS/", 0) == 0)
            evidence.endpoint_role = "START_CAP_VERTEX";
        else if (ref.output_slot.rfind("END_VERTEX_FROM_PROFILE_ENDPOINTS/", 0) == 0)
            evidence.endpoint_role = "END_CAP_VERTEX";
        else
            evidence.endpoint_role = "DERIVED_VERTEX";
    }
    if (!evidence.endpoint_role.empty())
        evidence.evidence_digest =
            make_geometry_id(evidence.evidence_digest + "|role=" + evidence.endpoint_role);
}

bool topology_adjacent(const TopoDS_Shape& left, const TopoDS_Shape& right) {
    if (left.ShapeType() == TopAbs_FACE && right.ShapeType() == TopAbs_FACE) {
        TopTools_IndexedMapOfShape edges;
        TopExp::MapShapes(left, TopAbs_EDGE, edges);
        for (TopExp_Explorer edge(right, TopAbs_EDGE); edge.More(); edge.Next())
            if (edges.Contains(edge.Current()))
                return true;
        return false;
    }
    if (left.ShapeType() == TopAbs_EDGE && right.ShapeType() == TopAbs_EDGE) {
        TopTools_IndexedMapOfShape vertices;
        TopExp::MapShapes(left, TopAbs_VERTEX, vertices);
        for (TopExp_Explorer vertex(right, TopAbs_VERTEX); vertex.More(); vertex.Next())
            if (vertices.Contains(vertex.Current()))
                return true;
        return false;
    }
    const TopoDS_Shape* owner = &left;
    const TopoDS_Shape* member = &right;
    if (static_cast<int>(owner->ShapeType()) > static_cast<int>(member->ShapeType()))
        std::swap(owner, member);
    TopTools_IndexedMapOfShape members;
    TopExp::MapShapes(*owner, member->ShapeType(), members);
    return members.Contains(*member);
}

// Root imports cover every unique Face/Edge/Vertex once. Index containment
// and shared boundaries instead of rebuilding subshape maps for every pair.
std::vector<std::vector<std::size_t>> imported_adjacency(const std::vector<NamedShape>& named) {
    TopTools_IndexedMapOfShape index;
    for (const auto& item : named) index.Add(item.shape);
    std::vector<std::vector<std::size_t>> adjacent(named.size()), faceOwners(named.size()), edgeOwners(named.size());
    const auto connect = [&](std::size_t a, std::size_t b) {
        if (a == b) return;
        adjacent[a].push_back(b);
        adjacent[b].push_back(a);
    };
    for (std::size_t owner = 0; owner < named.size(); ++owner) {
        const auto type = named[owner].shape.ShapeType();
        for (const auto memberType : {TopAbs_EDGE, TopAbs_VERTEX}) {
            if (type >= memberType) continue;
            TopTools_IndexedMapOfShape members;
            TopExp::MapShapes(named[owner].shape, memberType, members);
            for (int item = 1; item <= members.Extent(); ++item) {
                const int located = index.FindIndex(members(item));
                if (located == 0) continue;
                const auto member = static_cast<std::size_t>(located - 1);
                connect(owner, member);
                if (type == TopAbs_FACE && memberType == TopAbs_EDGE) faceOwners[member].push_back(owner);
                if (type == TopAbs_EDGE && memberType == TopAbs_VERTEX) edgeOwners[member].push_back(owner);
            }
        }
    }
    for (const auto* owners : {&faceOwners, &edgeOwners}) {
        for (const auto& shared : *owners)
            for (std::size_t a = 0; a < shared.size(); ++a)
                for (std::size_t b = a + 1; b < shared.size(); ++b) connect(shared[a], shared[b]);
    }
    for (auto& neighbors : adjacent) {
        std::sort(neighbors.begin(), neighbors.end());
        neighbors.erase(std::unique(neighbors.begin(), neighbors.end()), neighbors.end());
    }
    return adjacent;
}

bool topology_contains(const TopoDS_Shape& owner, const TopoDS_Shape& member) {
    if (owner.ShapeType() == member.ShapeType())
        return owner.IsSame(member);
    TopTools_IndexedMapOfShape members;
    TopExp::MapShapes(owner, member.ShapeType(), members);
    return members.Contains(member);
}

std::string derived_topology_name(const TopAbs_ShapeEnum type) {
    switch (type) {
        case TopAbs_FACE:
            return "FACE";
        case TopAbs_EDGE:
            return "EDGE";
        case TopAbs_VERTEX:
            return "VERTEX";
        default:
            throw std::runtime_error("TOPOLOGY_HISTORY_UNSUPPORTED_DERIVED_TYPE");
    }
}

std::vector<SemanticTopologyRef> adjacent_naming_sources(
    const TopoDS_Shape& target, const std::vector<NamedShape>& named) {
    std::vector<SemanticTopologyRef> sources;
    const auto collect = [&](const auto& predicate) {
        for (const auto& candidate : named)
            if (predicate(candidate.shape) &&
                std::none_of(sources.begin(), sources.end(), [&](const auto& source) {
                    return same_ref(source, candidate.ref);
                }))
                sources.push_back(candidate.ref);
    };
    // Prefer the immediate topological boundary/owner.  This gives a derived
    // edge its incident semantic faces and a vertex its incident semantic
    // edges, rather than coupling identity to every nearby sub-shape.
    if (target.ShapeType() == TopAbs_FACE) {
        collect([&](const TopoDS_Shape& candidate) {
            return candidate.ShapeType() == TopAbs_EDGE && topology_contains(target, candidate);
        });
    } else if (target.ShapeType() == TopAbs_EDGE) {
        collect([&](const TopoDS_Shape& candidate) {
            return candidate.ShapeType() == TopAbs_FACE && topology_contains(candidate, target);
        });
    } else if (target.ShapeType() == TopAbs_VERTEX) {
        collect([&](const TopoDS_Shape& candidate) {
            return candidate.ShapeType() == TopAbs_EDGE && topology_contains(candidate, target);
        });
    }
    if (sources.empty())
        collect([&](const TopoDS_Shape& candidate) {
            return topology_adjacent(target, candidate);
        });
    sort_refs(sources);
    return sources;
}

std::vector<TopologyLineage> complete_boolean_topology_naming(
    const TopoDS_Shape& result, std::vector<NamedShape>& named,
    const std::string& feature_id) {
    std::vector<TopologyLineage> derived;
    for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX}) {
        TopTools_IndexedMapOfShape final_shapes;
        TopExp::MapShapes(result, type, final_shapes);
        struct Pending {
            TopoDS_Shape shape;
            std::vector<SemanticTopologyRef> sources;
            SelectionEvidence evidence;
        };
        std::unordered_map<std::string, std::vector<Pending>> groups;
        for (int index = 1; index <= final_shapes.Extent(); ++index) {
            const auto& shape = final_shapes.FindKey(index);
            const bool covered = std::any_of(named.begin(), named.end(), [&](const auto& value) {
                return value.shape.ShapeType() == type && value.shape.IsSame(shape);
            });
            if (covered)
                continue;
            auto sources = adjacent_naming_sources(shape, named);
            if (sources.empty())
                throw std::runtime_error("TOPOLOGY_HISTORY_DERIVED_SOURCE_MISSING:" +
                                         derived_topology_name(type));
            std::ostringstream signature;
            signature << derived_topology_name(type);
            for (const auto& source : sources)
                signature << '\0' << ref_key(source);
            groups[signature.str()].push_back({shape, std::move(sources),
                                               topology_evidence(shape)});
        }
        for (auto& [signature, pending] : groups) {
            if (feature_id.empty())
                throw std::runtime_error("TOPOLOGY_HISTORY_DERIVED_FEATURE_ID_MISSING");
            std::sort(pending.begin(), pending.end(), [](const auto& left, const auto& right) {
                return left.evidence.evidence_digest < right.evidence.evidence_digest;
            });
            for (std::size_t index = 1; index < pending.size(); ++index)
                if (pending[index - 1].evidence.evidence_digest ==
                    pending[index].evidence.evidence_digest)
                    throw std::runtime_error("TOPOLOGY_HISTORY_DERIVED_AMBIGUOUS:" +
                                             derived_topology_name(type));
            const auto signature_digest = make_geometry_id(signature).substr(7, 16);
            for (std::size_t index = 0; index < pending.size(); ++index) {
                std::vector<std::string> source_ids;
                for (const auto& source : pending[index].sources) {
                    source_ids.push_back(source.feature_id + "/" + source.output_slot);
                    source_ids.insert(source_ids.end(), source.source_ids.begin(),
                                      source.source_ids.end());
                }
                std::sort(source_ids.begin(), source_ids.end());
                source_ids.erase(std::unique(source_ids.begin(), source_ids.end()),
                                 source_ids.end());
                SemanticTopologyRef output{
                    feature_id,
                    "DERIVED_" + derived_topology_name(type) + "_FROM_ADJACENCY/" +
                        signature_digest + "/" + std::to_string(index + 1),
                    std::move(source_ids)};
                named.push_back({output, pending[index].shape});
                derived.push_back({pending[index].sources, output,
                                   TopologyLineageKind::generated, pending[index].evidence});
            }
        }
    }
    return derived;
}

std::string topology_policy_digest() {
    return std::string(topology_naming_policy_digest);
}

std::string topology_history_digest(const TopologyHistory& history) {
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << "schema=" << history.schema_version << "|feature=" << history.feature_id
              << "|input=" << history.input_geometry_id << "|result="
              << history.result_geometry_id << "|policy=" << history.policy_digest;
    auto lineage = history.lineage;
    std::sort(lineage.begin(), lineage.end(), [](const auto& left, const auto& right) {
        return ref_key(left.result) < ref_key(right.result);
    });
    for (auto& item : lineage) {
        sort_refs(item.sources);
        canonical << "|lineage=" << static_cast<int>(item.kind) << ':' << ref_key(item.result)
                  << ':' << item.evidence.evidence_digest;
        for (const auto& source : item.sources)
            canonical << ":source=" << ref_key(source);
        auto adjacent = item.evidence.adjacent;
        sort_refs(adjacent);
        for (const auto& ref : adjacent)
            canonical << ":adjacent=" << ref_key(ref);
    }
    auto deleted = history.deleted;
    std::sort(deleted.begin(), deleted.end(), [](const auto& left, const auto& right) {
        return ref_key(left.source) < ref_key(right.source);
    });
    for (const auto& item : deleted)
        canonical << "|deleted=" << ref_key(item.source) << ':' << item.reason << ':'
                  << item.evidence.evidence_digest;
    auto ambiguous = history.ambiguous;
    std::sort(ambiguous.begin(), ambiguous.end(), [](const auto& left, const auto& right) {
        return (left.sources.empty() ? std::string{} : ref_key(left.sources.front())) <
               (right.sources.empty() ? std::string{} : ref_key(right.sources.front()));
    });
    for (auto& item : ambiguous) {
        sort_refs(item.sources);
        sort_refs(item.candidates);
        canonical << "|ambiguous=" << item.diagnostic_code;
        for (const auto& source : item.sources)
            canonical << ":source=" << ref_key(source);
        for (const auto& candidate : item.candidates)
            canonical << ":candidate=" << ref_key(candidate);
    }
    return make_geometry_id(canonical.str());
}

BodyOperationResult apply_body_operation(const TopoDS_Shape& input,
                                         const std::vector<NamedShape>& input_named,
                                         const ToolBuild& tool,
                                         const std::string& requested_operation) {
    const std::string operation = requested_operation.empty() ? "ADD" : requested_operation;
    if (input.IsNull()) {
        if (operation == "REMOVE" || operation == "INTERSECT")
            throw std::invalid_argument(operation + " requires an input body");
        return {tool.shape, tool.named, {}, {}};  // Legacy first ADD is equivalent to NEW_BODY.
    }
    if (operation == "NEW_BODY")
        throw std::invalid_argument(
            "NEW_BODY requires an empty target body in the current single-body model");
    TopoDS_Shape result;
    std::vector<NamedShape> mapped;
    std::vector<NamedShape> sources = input_named;
    sources.insert(sources.end(), tool.named.begin(), tool.named.end());
    if (operation == "ADD") {
        BRepAlgoAPI_Fuse algorithm(input, tool.shape);
        algorithm.Build();
        if (!algorithm.IsDone())
            throw std::runtime_error("body fuse failed");
        result = algorithm.Shape();
        mapped = map_named_shapes(sources, algorithm, result);
    } else if (operation == "REMOVE") {
        BRepAlgoAPI_Cut algorithm(input, tool.shape);
        algorithm.Build();
        if (!algorithm.IsDone())
            throw std::runtime_error("body cut failed");
        result = algorithm.Shape();
        mapped = map_named_shapes(sources, algorithm, result);
        if (shape_volume(input) - shape_volume(result) <= 1.0e-9)
            throw std::invalid_argument(
                "NO_MATERIAL_CHANGE: cut does not intersect the target body");
    } else if (operation == "INTERSECT") {
        BRepAlgoAPI_Common algorithm(input, tool.shape);
        algorithm.Build();
        if (!algorithm.IsDone())
            throw std::runtime_error("body common failed");
        result = algorithm.Shape();
        mapped = map_named_shapes(sources, algorithm, result);
    } else {
        throw std::invalid_argument("unsupported body operation: " + operation);
    }
    // OCCT boolean builders deliberately preserve section edges.  That history is
    // useful while mapping generated topology, but it is not the canonical shape
    // of a Part Design Body: coplanar/cotangent pieces left by overlapping pads
    // would otherwise be exposed as several selectable faces.  Normalize the
    // completed operation before validation and content hashing.  This keeps the
    // B-Rep deterministic and makes a continuous planar skin one logical face.
    ShapeUpgrade_UnifySameDomain unifier(result, Standard_True, Standard_True, Standard_False);
    unifier.Build();
    mapped = map_named_history(mapped, unifier.History(), unifier.Shape());
    result = unifier.Shape();
    if (result.IsNull() || shape_volume(result) <= 1.0e-9)
        throw std::invalid_argument("EMPTY_RESULT: solid operation produced no material");
    if (!BRepCheck_Analyzer(result).IsValid())
        throw std::runtime_error("solid operation produced invalid B-Rep");
    if (solid_count(result) != 1)
        throw std::invalid_argument("DISJOINT_RESULT: standard Body requires exactly one solid");
    auto derived = complete_boolean_topology_naming(result, mapped, tool.feature_id);
    std::vector<std::string> diagnostics;
    if (!derived.empty())
        diagnostics.push_back("TOPOLOGY_HISTORY_DERIVED_CLOSURE:" +
                              std::to_string(derived.size()));
    return {result, mapped, std::move(derived), std::move(diagnostics)};
}

// Some STEP writers split a cylindrical thread band into neighboring faces
// whose individual wires wind once around the periodic surface. The 3D wires
// close, but their UV boundaries do not. Join only two such invalid faces with
// matching parameter frames and exactly one shared edge. No new material,
// boundary curves or approximate replacement surfaces are introduced.
TopoDS_Shape repair_split_cylindrical_faces(const TopoDS_Shape& shape) {
    TopTools_MapOfShape candidates, consumed;
    BRepCheck_Analyzer analysis(shape);
    for (TopExp_Explorer faces(shape, TopAbs_FACE); faces.More(); faces.Next()) {
        const auto face = TopoDS::Face(faces.Current());
        if (analysis.IsValid(face) || BRepAdaptor_Surface(face).GetType() != GeomAbs_Cylinder) continue;
        TopExp_Explorer wires(face, TopAbs_WIRE);
        if (!wires.More()) continue;
        const auto wire = TopoDS::Wire(wires.Current());
        wires.Next();
        if (wires.More()) continue;
        BRepCheck_Wire check(wire);
        if (check.Closed() == BRepCheck_NoError && check.Closed2d(face) == BRepCheck_NotClosed)
            candidates.Add(face);
    }
    Handle(ShapeBuild_ReShape) replacements = new ShapeBuild_ReShape;
    TopTools_IndexedDataMapOfShapeListOfShape adjacency;
    TopExp::MapShapesAndAncestors(shape, TopAbs_EDGE, TopAbs_FACE, adjacency);
    for (int index = 1; index <= adjacency.Extent(); ++index) {
        const auto& neighbors = adjacency(index);
        if (neighbors.Extent() != 2) continue;
        auto first = TopoDS::Face(neighbors.First()), second = TopoDS::Face(neighbors.Last());
        if (first.IsSame(second) || !candidates.Contains(first) || !candidates.Contains(second) ||
            consumed.Contains(first) || consumed.Contains(second) || first.Orientation() != second.Orientation()) continue;
        const auto a = BRepAdaptor_Surface(first).Cylinder(), b = BRepAdaptor_Surface(second).Cylinder();
        // Numerical equality of the full frame is needed to reuse UV curves;
        // merely coaxial or approximately coincident cylinders are insufficient.
        if (a.Location().Distance(b.Location()) > 1e-9 || std::abs(a.Radius() - b.Radius()) > 1e-9 ||
            (a.Axis().Direction().XYZ() - b.Axis().Direction().XYZ()).Modulus() > 1e-12 ||
            (a.Position().XDirection().XYZ() - b.Position().XDirection().XYZ()).Modulus() > 1e-12 ||
            (a.Position().YDirection().XYZ() - b.Position().YDirection().XYZ()).Modulus() > 1e-12) continue;
        TopTools_IndexedMapOfShape firstEdges, secondEdges;
        TopExp::MapShapes(first, TopAbs_EDGE, firstEdges);
        TopExp::MapShapes(second, TopAbs_EDGE, secondEdges);
        int shared = 0;
        for (int edge = 1; edge <= firstEdges.Extent(); ++edge)
            if (secondEdges.Contains(firstEdges(edge))) ++shared;
        if (shared != 1) continue;
        const auto orientation = first.Orientation();
        first.Orientation(TopAbs_FORWARD);
        second.Orientation(TopAbs_FORWARD);
        TopoDS_Face merged = TopoDS::Face(first.EmptyCopied());
        TopoDS_Wire boundary;
        BRep_Builder builder;
        builder.MakeWire(boundary);
        for (const auto& face : {first, second}) {
            for (TopExp_Explorer edges(face, TopAbs_EDGE); edges.More(); edges.Next()) {
                const auto edge = TopoDS::Edge(edges.Current());
                if (edge.IsSame(adjacency.FindKey(index))) continue;
                double start, end;
                const auto curve = BRep_Tool::CurveOnSurface(edge, face, start, end);
                if (curve.IsNull()) throw std::invalid_argument("IMPORT_SOLID_REPAIR_MISSING_PCURVE");
                builder.UpdateEdge(edge, curve, merged, BRep_Tool::Tolerance(edge));
                builder.Range(edge, merged, start, end);
                builder.Add(boundary, edge);
            }
        }
        builder.Add(merged, boundary);
        ShapeFix_Face fix(merged);
        fix.SetContext(replacements);
        fix.SetPrecision(1e-6);
        fix.SetMinTolerance(1e-7);
        fix.SetMaxTolerance(1e-3);
        fix.Perform();
        auto result = fix.Result();
        if (result.ShapeType() != TopAbs_FACE || !BRepCheck_Analyzer(result).IsValid())
            throw std::invalid_argument("IMPORT_SOLID_REPAIR_PERIODIC_FACE_FAILED");
        first.Orientation(orientation);
        second.Orientation(orientation);
        result.Orientation(orientation);
        replacements->Replace(first, result);
        replacements->Remove(second);
        consumed.Add(first);
        consumed.Add(second);
    }
    return replacements->Apply(shape);
}

std::filesystem::path temporary_step_path() {
    static std::atomic<uint64_t> sequence{0};
    const auto stamp = std::chrono::steady_clock::now().time_since_epoch().count();
    return std::filesystem::temp_directory_path() /
           ("occccad-" + std::to_string(stamp) + "-" + std::to_string(sequence.fetch_add(1)) +
            ".step");
}

class ScopedFile final {
public:
    explicit ScopedFile(std::filesystem::path value) : path(std::move(value)) {}
    ~ScopedFile() {
        std::error_code ignored;
        std::filesystem::remove(path, ignored);
    }
    std::filesystem::path path;
};

}  // namespace

struct OcctKernel::Impl {
    struct StoredGeometry {
        TopoDS_Shape shape;
        std::vector<uint8_t> brep;
    };

    std::unordered_map<GeometryId, StoredGeometry> shapes;
    std::unordered_map<GeometryId, TopologyInfo> topologies;

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

OcctKernel::OcctKernel() : impl_(std::make_unique<Impl>()) {
}
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

uint32_t OcctKernel::inspectStepRootCount(const std::string& path) {
    STEPControl_Reader reader;
    if (reader.ReadFile(path.c_str()) != IFSelect_RetDone) {
        throw std::runtime_error("STEP read failed: " + path);
    }
    const int count = reader.NbRootsForTransfer();
    if (count <= 0) {
        throw std::runtime_error("STEP file contains no transferable roots: " + path);
    }
    return static_cast<uint32_t>(count);
}

GeometryId OcctKernel::loadStepRoot(const std::string& path, const uint32_t root_index) {
    STEPControl_Reader reader;
    if (reader.ReadFile(path.c_str()) != IFSelect_RetDone) {
        throw std::runtime_error("STEP read failed: " + path);
    }
    const int count = reader.NbRootsForTransfer();
    if (root_index == 0U || root_index > static_cast<uint32_t>(count)) {
        throw std::invalid_argument("STEP root index is out of range");
    }
    if (!reader.TransferRoot(static_cast<int>(root_index))) {
        throw std::runtime_error("STEP root transfer failed");
    }
    const TopoDS_Shape shape = reader.Shape(1);
    if (shape.IsNull()) {
        throw std::runtime_error("STEP root produced an empty shape");
    }
    return impl_->store(shape);
}

std::vector<GeometryId> OcctKernel::splitSolids(const GeometryId& id) {
    // Copy the handle before store() grows the unordered map.
    const TopoDS_Shape shape = impl_->find(id);
    for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX}) {
        if (TopExp_Explorer(shape, type, TopAbs_SOLID).More())
            throw std::invalid_argument("IMPORT_UNSUPPORTED_NON_SOLID_GEOMETRY");
    }
    std::vector<GeometryId> result;
    for (TopExp_Explorer explorer(shape, TopAbs_SOLID); explorer.More(); explorer.Next()) {
        const auto solid = explorer.Current();
        result.push_back(impl_->store(solid));
    }
    if (result.empty())
        throw std::invalid_argument("IMPORT_REQUIRES_SOLID_GEOMETRY");
    return result;
}

GeometryId OcctKernel::repairImportedSolid(const GeometryId& id) {
    const TopoDS_Shape original = impl_->find(id);
    TopTools_IndexedMapOfShape solids;
    TopExp::MapShapes(original, TopAbs_SOLID, solids);
    if (solids.Extent() < 1)
        throw std::invalid_argument("IMPORT_REQUIRES_SOLID_DEFINITION");
    for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX})
        if (TopExp_Explorer(original, type, TopAbs_SOLID).More())
            throw std::invalid_argument("IMPORT_UNSUPPORTED_NON_SOLID_GEOMETRY");
    const auto original_solid_count=solids.Extent();
    if (BRepCheck_Analyzer(original).IsValid())
        return id;
    // Heal before allocating persistent naming, on a copy of immutable input.
    // Internal length units are mm: precision 1e-6, maximum repair tolerance 1e-3.
    ShapeFix_Shape fix(BRepBuilderAPI_Copy(original, true, false).Shape());
    fix.SetPrecision(1e-6);
    fix.SetMinTolerance(1e-7);
    fix.SetMaxTolerance(1e-3);
    fix.Perform();
    TopoDS_Shape repaired = fix.Shape();
    if (!BRepCheck_Analyzer(repaired).IsValid()) {
        repaired = repair_split_cylindrical_faces(repaired);
    }
    solids.Clear();
    TopExp::MapShapes(repaired, TopAbs_SOLID, solids);
    BRepCheck_Analyzer analysis(repaired);
    if (solids.Extent() != original_solid_count || !analysis.IsValid()) {
        std::string diagnostic;
        TopTools_IndexedMapOfShape shapes;
        TopExp::MapShapes(repaired, shapes);
        for (int index = 1; index <= shapes.Extent(); ++index) {
            const auto result = analysis.Result(shapes(index));
            if (result.IsNull()) continue;
            for (result->InitContextIterator(); result->MoreShapeInContext(); result->NextShapeInContext()) {
                for (BRepCheck_ListIteratorOfListOfStatus status(result->StatusOnShape()); status.More(); status.Next()) {
                    if (status.Value() != BRepCheck_NoError && diagnostic.size() < 512)
                        diagnostic += " shape=" + std::to_string(index) + " type=" + std::to_string(shapes(index).ShapeType()) + " contextIndex=" + std::to_string(shapes.FindIndex(result->ContextualShape())) + " status=" + std::to_string(status.Value());
                }
            }
        }
        throw std::invalid_argument("IMPORT_SOLID_REPAIR_FAILED:" + diagnostic);
    }
    for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX}) {
        if (TopExp_Explorer(repaired, type, TopAbs_SOLID).More())
            throw std::invalid_argument("IMPORT_SOLID_REPAIR_LEFT_LOOSE_TOPOLOGY");
    }
    return impl_->store(repaired);
}

GeometryId OcctKernel::combine(const std::vector<PlacedGeometry>& components) {
    if (components.empty()) {
        throw std::invalid_argument("exchange export requires at least one component");
    }
    BRep_Builder builder;
    TopoDS_Compound compound;
    builder.MakeCompound(compound);
    for (const auto& component : components) {
        gp_Trsf transform;
        transform.SetRotation(gp_Quaternion(component.rotation.x, component.rotation.y,
                                            component.rotation.z, component.rotation.w));
        transform.SetTranslationPart(
            gp_Vec(component.translation.x, component.translation.y, component.translation.z));
        builder.Add(compound, impl_->find(component.geometry_id).Moved(TopLoc_Location(transform)));
    }
    return impl_->store(compound);
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
    impl_->topologies.erase(id);
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

GeometryId OcctKernel::evaluateRectangularPads(const std::vector<RectangularPadSpec>& specs,
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

ProfileEvaluationResult OcctKernel::evaluateProfilePadsWithHistory(
    const std::vector<ProfilePadSpec>& specs, const std::vector<uint8_t>& base_brep,
    const ImportTopologySeed* import_seed) {
    TopoDS_Shape result;
    GeometryId result_id;
    std::vector<NamedShape> live_named;
    if (!base_brep.empty()) {
        const GeometryId base_id = loadBrepr(base_brep);
        result = impl_->find(base_id);
        result_id = base_id;
    }
    ProfileEvaluationResult evaluation;
    bool topology_history_complete = base_brep.empty();
    if (import_seed) {
        const auto& seed = *import_seed;
        const std::string bytes(base_brep.begin(), base_brep.end());
        if (base_brep.empty() || seed.feature_id.empty() || seed.body_id.empty() ||
            make_geometry_id(bytes) != "sha256:" + seed.brep_sha256)
            throw std::invalid_argument("IMPORT_SEED_SNAPSHOT_MISMATCH");
        TopTools_IndexedMapOfShape solids, faces, edges, vertices;
        TopExp::MapShapes(result, TopAbs_SOLID, solids);
        TopExp::MapShapes(result, TopAbs_FACE, faces);
        TopExp::MapShapes(result, TopAbs_EDGE, edges);
        TopExp::MapShapes(result, TopAbs_VERTEX, vertices);
        if (solids.Extent() < 1 || !BRepCheck_Analyzer(result).IsValid())
            throw std::invalid_argument("IMPORT_NAMING_REQUIRES_VALID_SOLID_DEFINITION");
        for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX})
            if (TopExp_Explorer(result, type, TopAbs_SOLID).More())
                throw std::invalid_argument("IMPORT_NAMING_REQUIRES_VALID_SOLID_DEFINITION");
        if (seed.identities.size() != static_cast<std::size_t>(faces.Extent() + edges.Extent() + vertices.Extent()))
            throw std::invalid_argument("IMPORT_SEED_COVERAGE_MISMATCH");
        FeatureResult root;
        root.feature_id = seed.feature_id;
        root.body_id = seed.body_id;
        root.result_geometry_id = result_id;
        root.topology_history_complete = true;
        root.topology_history.feature_id = seed.feature_id;
        root.topology_history.result_geometry_id = result_id;
        root.topology_history.policy_digest = topology_policy_digest();
        std::unordered_set<std::string> ids, locators;
        ids.reserve(seed.identities.size());
        locators.reserve(seed.identities.size());
        for (const auto& entry : seed.identities) {
            const auto* map = entry.topology_type == PersistentTopologyType::face ? &faces :
                              entry.topology_type == PersistentTopologyType::edge ? &edges :
                              entry.topology_type == PersistentTopologyType::vertex ? &vertices : nullptr;
            const auto locator = std::to_string(static_cast<int>(entry.topology_type)) + "/" + std::to_string(entry.local_id);
            if (!map || entry.local_id == 0 || entry.local_id > static_cast<std::uint64_t>(map->Extent()) ||
                entry.stable_id.empty() || !ids.insert(entry.stable_id).second ||
                !locators.insert(locator).second)
                throw std::invalid_argument("IMPORT_SEED_INVALID_IDENTITY_MAP");
            const auto shape = (*map)(static_cast<int>(entry.local_id));
            SemanticTopologyRef ref{seed.feature_id, "import.topology", {entry.stable_id}};
            auto evidence = topology_evidence(shape);
            append_semantic_role(evidence, ref, entry.topology_type);
            root.semantic_outputs.push_back({ref, entry.topology_type, entry.local_id, evidence});
            live_named.push_back({ref, shape});
        }
        const auto adjacency = imported_adjacency(live_named);
        for (std::size_t i = 0; i < live_named.size(); ++i) {
            auto& output = root.semantic_outputs[i];
            for (const auto neighbor : adjacency[i])
                output.evidence.adjacent.push_back(live_named[neighbor].ref);
            sort_refs(output.evidence.adjacent);
            root.topology_history.lineage.push_back({{}, output.semantic_ref, TopologyLineageKind::generated, output.evidence});
        }
        root.topology_history.evidence_digest = topology_history_digest(root.topology_history);
        evaluation.feature_results.push_back(std::move(root));
        topology_history_complete = true;
    }

    for (const auto& spec : specs) {
        const auto input_shape = result;
        const auto input_id = result_id;
        const auto input_named = live_named;
        const auto tool = make_profile_tool(spec);
        topology_history_complete = topology_history_complete && tool.topology_history_complete;
        auto operation = apply_body_operation(result, live_named, tool, spec.body_operation);
        result = operation.shape;
        result_id = impl_->store(result);

        TopTools_IndexedMapOfShape final_faces;
        TopTools_IndexedMapOfShape final_edges;
        TopTools_IndexedMapOfShape final_vertices;
        TopExp::MapShapes(result, TopAbs_FACE, final_faces);
        TopExp::MapShapes(result, TopAbs_EDGE, final_edges);
        TopExp::MapShapes(result, TopAbs_VERTEX, final_vertices);
        struct OutputGroup {
            TopoDS_Shape shape;
            std::vector<SemanticTopologyRef> sources;
        };
        std::vector<OutputGroup> groups;
        for (const auto& mapped : operation.named) {
            auto group = std::find_if(groups.begin(), groups.end(), [&](const OutputGroup& value) {
                return value.shape.IsSame(mapped.shape);
            });
            if (group == groups.end()) {
                groups.push_back({mapped.shape, {mapped.ref}});
            } else if (std::none_of(group->sources.begin(), group->sources.end(),
                                    [&](const auto& ref) { return same_ref(ref, mapped.ref); })) {
                group->sources.push_back(mapped.ref);
            }
        }
        std::vector<std::pair<SemanticTopologyOutput, TopoDS_Shape>> outputs;
        std::unordered_map<std::string, std::size_t> source_counts;
        for (auto& group : groups) {
            // OCCT history uses IsSame identity, which ignores orientation.
            // Generated/tool faces can have the opposite orientation from the
            // face occurrence in the final solid (notably caps and cut walls).
            if (group.shape.ShapeType() == TopAbs_FACE) {
                const int index = final_faces.FindIndex(group.shape);
                if (index <= 0)
                    throw std::runtime_error("TOPOLOGY_HISTORY_DANGLING_RESULT");
                group.shape = final_faces(index);
            }
            sort_refs(group.sources);
        }
        std::sort(groups.begin(), groups.end(), [](const auto& left, const auto& right) {
            const auto left_key = left.sources.empty() ? std::string{} : ref_key(left.sources.front());
            const auto right_key = right.sources.empty() ? std::string{} : ref_key(right.sources.front());
            if (left_key != right_key)
                return left_key < right_key;
            return topology_evidence(left.shape).evidence_digest <
                   topology_evidence(right.shape).evidence_digest;
        });
        for (const auto& group : groups)
            for (const auto& source : group.sources)
                ++source_counts[ref_key(source)];
        std::unordered_map<std::string, std::size_t> split_indices;
        FeatureResult feature;
        feature.feature_id = spec.feature_id;
        feature.body_id = spec.body_id;
        feature.input_feature_id = spec.input_feature_id;
        feature.profile_feature_id = spec.profile_feature_id;
        feature.result_geometry_id = result_id;
        feature.topology_history_complete = topology_history_complete;
        feature.diagnostics = tool.diagnostics;
        feature.diagnostics.insert(feature.diagnostics.end(), operation.diagnostics.begin(),
                                   operation.diagnostics.end());
        if (!base_brep.empty() && input_named.empty()) {
            feature.topology_history_complete = false;
            topology_history_complete = false;
            feature.diagnostics.push_back("TOPOLOGY_HISTORY_UNNAMED_BASE");
        } else if (!feature.topology_history_complete && tool.topology_history_complete) {
            feature.diagnostics.push_back("TOPOLOGY_HISTORY_INCOMPLETE_INPUT");
        }
        feature.topology_history.feature_id = spec.feature_id;
        feature.topology_history.input_geometry_id = input_id;
        feature.topology_history.result_geometry_id = result_id;
        feature.topology_history.policy_digest = topology_policy_digest();
        for (const auto& group : groups) {
            SemanticTopologyRef output_ref;
            TopologyLineageKind kind = TopologyLineageKind::modified;
            if (group.sources.size() > 1U) {
                std::ostringstream merged;
                for (const auto& source : group.sources)
                    merged << ref_key(source) << '\0';
                const auto merged_digest = make_geometry_id(merged.str());
                std::vector<std::string> source_ids;
                for (const auto& source : group.sources) {
                    source_ids.push_back(source.feature_id + "/" + source.output_slot);
                    source_ids.insert(source_ids.end(), source.source_ids.begin(),
                                      source.source_ids.end());
                }
                std::sort(source_ids.begin(), source_ids.end());
                source_ids.erase(std::unique(source_ids.begin(), source_ids.end()),
                                 source_ids.end());
                output_ref = {spec.feature_id, "MERGED_FROM/" + merged_digest.substr(7), source_ids};
                kind = TopologyLineageKind::merged;
            } else {
                const auto& source = group.sources.front();
                if (source_counts[ref_key(source)] > 1U) {
                    output_ref = {spec.feature_id,
                                  "SPLIT_FROM/" + source.feature_id + "/" + source.output_slot +
                                      "/" + std::to_string(++split_indices[ref_key(source)]),
                                  source.source_ids};
                    kind = TopologyLineageKind::split;
                } else {
                    output_ref = source;
                    const auto original = std::find_if(
                        input_named.begin(), input_named.end(),
                        [&](const NamedShape& value) { return same_ref(value.ref, source); });
                    if (original != input_named.end() && original->shape.IsSame(group.shape))
                        kind = TopologyLineageKind::unchanged;
                    else if (source.feature_id == spec.feature_id)
                        kind = TopologyLineageKind::generated;
                }
            }
            int local_id = 0;
            switch (group.shape.ShapeType()) {
                case TopAbs_FACE:
                    local_id = final_faces.FindIndex(group.shape);
                    break;
                case TopAbs_EDGE:
                    local_id = final_edges.FindIndex(group.shape);
                    break;
                case TopAbs_VERTEX:
                    local_id = final_vertices.FindIndex(group.shape);
                    break;
                default:
                    break;
            }
            if (local_id <= 0)
                throw std::runtime_error("TOPOLOGY_HISTORY_DANGLING_RESULT");
            auto evidence = topology_evidence(group.shape);
            const auto output_type = persistent_topology_type(group.shape);
            append_semantic_role(evidence, output_ref, output_type);
            outputs.push_back({{output_ref, output_type,
                                static_cast<std::uint64_t>(local_id), evidence},
                               group.shape});
            std::vector<SemanticTopologyRef> lineage_sources = group.sources;
            if (kind == TopologyLineageKind::generated && group.sources.size() == 1U) {
                const auto generated =
                    std::find_if(tool.generated.begin(), tool.generated.end(),
                                 [&](const TopologyLineage& value) {
                                     return same_ref(value.result, group.sources.front());
                                 });
                const auto derived =
                    std::find_if(operation.derived.begin(), operation.derived.end(),
                                 [&](const TopologyLineage& value) {
                                     return same_ref(value.result, group.sources.front());
                                 });
                if (generated != tool.generated.end())
                    lineage_sources = generated->sources;
                else if (derived != operation.derived.end())
                    lineage_sources = derived->sources;
            }
            feature.topology_history.lineage.push_back(
                {lineage_sources, output_ref, kind, evidence});
            if (kind == TopologyLineageKind::split) {
                auto ambiguity = std::find_if(
                    feature.topology_history.ambiguous.begin(),
                    feature.topology_history.ambiguous.end(), [&](const AmbiguousLineage& value) {
                        return value.sources.size() == 1U &&
                               same_ref(value.sources.front(), group.sources.front());
                    });
                if (ambiguity == feature.topology_history.ambiguous.end()) {
                    feature.topology_history.ambiguous.push_back(
                        {{group.sources.front()}, {output_ref}, "TOPOLOGY_SPLIT_AMBIGUOUS"});
                } else {
                    ambiguity->candidates.push_back(output_ref);
                }
            }
        }
        for (std::size_t left = 0; left < outputs.size(); ++left) {
            for (std::size_t right = 0; right < outputs.size(); ++right) {
                if (left == right)
                    continue;
                if (topology_adjacent(outputs[left].second, outputs[right].second))
                    outputs[left].first.evidence.adjacent.push_back(
                        outputs[right].first.semantic_ref);
            }
            sort_refs(outputs[left].first.evidence.adjacent);
            outputs[left].first.evidence.adjacent.erase(
                std::unique(outputs[left].first.evidence.adjacent.begin(),
                            outputs[left].first.evidence.adjacent.end(), same_ref),
                outputs[left].first.evidence.adjacent.end());
            auto lineage = std::find_if(feature.topology_history.lineage.begin(),
                                        feature.topology_history.lineage.end(), [&](const auto& value) {
                                            return same_ref(value.result, outputs[left].first.semantic_ref);
                                        });
            if (lineage != feature.topology_history.lineage.end())
                lineage->evidence = outputs[left].first.evidence;
        }
        std::vector<NamedShape> all_sources = input_named;
        all_sources.insert(all_sources.end(), tool.named.begin(), tool.named.end());
        for (const auto& source : all_sources) {
            const bool alive = std::any_of(
                operation.named.begin(), operation.named.end(),
                [&](const NamedShape& value) { return same_ref(value.ref, source.ref); });
            if (!alive) {
                auto evidence = topology_evidence(source.shape);
                append_semantic_role(evidence, source.ref,
                                     persistent_topology_type(source.shape));
                feature.topology_history.deleted.push_back(
                    {source.ref, "OCCT_IS_DELETED_OR_OUTSIDE_RESULT", evidence});
            }
        }
        for (const auto& tombstone : feature.topology_history.deleted) {
            if (std::any_of(outputs.begin(), outputs.end(), [&](const auto& output) {
                    return same_ref(output.first.semantic_ref, tombstone.source);
                }))
                throw std::runtime_error("TOPOLOGY_HISTORY_LIVE_TOMBSTONE_CONFLICT");
        }
        if (feature.topology_history_complete) {
            const auto final_count = static_cast<std::size_t>(
                final_faces.Extent() + final_edges.Extent() + final_vertices.Extent());
            if (outputs.size() != final_count) {
                const auto output_count = [&](const PersistentTopologyType type) {
                    return std::count_if(outputs.begin(), outputs.end(), [&](const auto& output) {
                        return output.first.topology_type == type;
                    });
                };
                std::ostringstream diagnostic;
                diagnostic << "TOPOLOGY_HISTORY_INCOMPLETE_FINAL_SHAPE:faces="
                           << output_count(PersistentTopologyType::face) << '/'
                           << final_faces.Extent() << ",edges="
                           << output_count(PersistentTopologyType::edge) << '/'
                           << final_edges.Extent() << ",vertices="
                           << output_count(PersistentTopologyType::vertex) << '/'
                           << final_vertices.Extent();
                throw std::runtime_error(diagnostic.str());
            }
            std::vector<std::string> local_ids;
            std::vector<std::string> semantic_refs;
            for (const auto& output : outputs) {
                if (output.first.local_id == 0U ||
                    output.first.topology_type == PersistentTopologyType::unspecified)
                    throw std::runtime_error("TOPOLOGY_HISTORY_INVALID_LOCAL_ID");
                local_ids.push_back(std::to_string(static_cast<int>(output.first.topology_type)) +
                                    "/" + std::to_string(output.first.local_id));
                semantic_refs.push_back(ref_key(output.first.semantic_ref));
                const auto matches = std::count_if(
                    feature.topology_history.lineage.begin(), feature.topology_history.lineage.end(),
                    [&](const auto& value) { return same_ref(value.result, output.first.semantic_ref); });
                if (matches != 1)
                    throw std::runtime_error("TOPOLOGY_HISTORY_LINEAGE_OUTPUT_MISMATCH");
            }
            std::sort(local_ids.begin(), local_ids.end());
            if (std::adjacent_find(local_ids.begin(), local_ids.end()) != local_ids.end())
                throw std::runtime_error("TOPOLOGY_HISTORY_DUPLICATE_LOCAL_ID");
            std::sort(semantic_refs.begin(), semantic_refs.end());
            if (std::adjacent_find(semantic_refs.begin(), semantic_refs.end()) != semantic_refs.end())
                throw std::runtime_error("TOPOLOGY_HISTORY_DUPLICATE_SEMANTIC_REF");
        }
        feature.topology_history.evidence_digest =
            topology_history_digest(feature.topology_history);
        live_named.clear();
        for (auto& output : outputs) {
            feature.semantic_outputs.push_back(std::move(output.first));
            live_named.push_back({feature.semantic_outputs.back().semantic_ref, output.second});
        }
        evaluation.feature_results.push_back(std::move(feature));
    }
    if (result.IsNull())
        throw std::invalid_argument("feature chain contains no solid geometry");
    if (!BRepCheck_Analyzer(result).IsValid())
        throw std::runtime_error("feature chain produced invalid B-Rep");
    evaluation.geometry_id = result_id.empty() ? impl_->store(result) : result_id;
    return evaluation;
}

GeometryId OcctKernel::evaluateProfilePads(const std::vector<ProfilePadSpec>& specs,
                                           const std::vector<uint8_t>& base_brep) {
    return evaluateProfilePadsWithHistory(specs, base_brep).geometry_id;
}

BoundingBox OcctKernel::getBoundingBox(const GeometryId& id) {
    Bnd_Box box;
    BRepBndLib::Add(impl_->find(id), box);
    return to_bbox(box);
}

const TopologyInfo& OcctKernel::getTopology(const GeometryId& id) {
    const auto cached = impl_->topologies.find(id);
    if (cached != impl_->topologies.end()) {
        return cached->second;
    }
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
        FaceInfo face_info{};
        face_info.local_id = static_cast<uint64_t>(index);
        face_info.surface_type = classify_surface(face);
        face_info.bbox = to_bbox(box);
        append_surface_properties(face, face_info);
        info.faces.push_back(std::move(face_info));
    }
    for (int index = 1; index <= edge_map.Extent(); ++index) {
        const TopoDS_Edge& edge = TopoDS::Edge(edge_map(index));
        Bnd_Box box;
        BRepBndLib::Add(edge, box);
        EdgeInfo edge_info{};
        edge_info.local_id = static_cast<uint64_t>(index);
        edge_info.curve_type = classify_curve(edge);
        edge_info.bbox = to_bbox(box);
        append_curve_properties(edge, edge_info);
        edge_info.render_points = sample_edge(edge);
        info.edges.push_back(std::move(edge_info));
    }
    for (int index = 1; index <= vertex_map.Extent(); ++index) {
        const TopoDS_Vertex& vertex = TopoDS::Vertex(vertex_map(index));
        VertexInfo vertex_info{};
        vertex_info.local_id = static_cast<uint64_t>(index);
        vertex_info.point = to_vec3(BRep_Tool::Pnt(vertex));
        vertex_info.properties.push_back(
            number_property("tolerance", BRep_Tool::Tolerance(vertex)));
        info.vertices.push_back(std::move(vertex_info));
    }
    return impl_->topologies.emplace(id, std::move(info)).first->second;
}

double OcctKernel::getVolume(const GeometryId& id) {
    GProp_GProps properties;
    BRepGProp::VolumeProperties(impl_->find(id), properties);
    return properties.Mass();
}

TessellationResult OcctKernel::tessellate(const GeometryId& id, const double linear_deflection,
                                          const double angular_deflection) {
    validate_positive(linear_deflection, "linear_deflection");
    validate_positive(angular_deflection, "angular_deflection");

    TopoDS_Shape& shape = impl_->find(id);
    BRepMesh_IncrementalMesh mesher(shape, linear_deflection, Standard_False, angular_deflection,
                                    Standard_True);
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
        const Handle(Poly_Triangulation) triangulation = BRep_Tool::Triangulation(face, location);
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
    TopTools_IndexedMapOfShape edge_map;
    TopTools_IndexedMapOfShape vertex_map;
    TopExp::MapShapes(shape, TopAbs_EDGE, edge_map);
    TopExp::MapShapes(shape, TopAbs_VERTEX, vertex_map);
    for (int index = 1; index <= edge_map.Extent(); ++index) {
        const TopoDS_Edge& edge = TopoDS::Edge(edge_map(index));
        EdgePolyline polyline{static_cast<uint64_t>(index), sample_edge(edge)};
        if (polyline.points.empty())
            continue;
        result.edges.push_back(std::move(polyline));
    }
    for (int index = 1; index <= vertex_map.Extent(); ++index) {
        const TopoDS_Vertex& vertex = TopoDS::Vertex(vertex_map(index));
        result.topology_vertices.push_back(
            {static_cast<uint64_t>(index), to_vec3(BRep_Tool::Pnt(vertex))});
    }
    return result;
}

GeometryId OcctKernel::chamfer(const GeometryId& id,
                               const std::vector<uint64_t>& /*edge_local_ids*/,
                               const double distance) {
    validate_positive(distance, "distance");
    return impl_->store(impl_->find(id));
}

GeometryId OcctKernel::fillet(const GeometryId& id, const std::vector<uint64_t>& /*edge_local_ids*/,
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
    ExchangeGraph graph;
    graph.definitions.push_back({"part","Part","PART",id,{}});
    graph.roots.push_back({"root","part","Part",{}});
    return writeStepGraph(graph);
}

}  // namespace occccad::kernel

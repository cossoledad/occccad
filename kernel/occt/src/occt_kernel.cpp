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
#include <BRepGProp.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepPrimAPI_MakeRevol.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GCPnts_AbscissaPoint.hxx>
#include <GProp_GProps.hxx>
#include <Geom_BSplineCurve.hxx>
#include <GeomAPI_Interpolate.hxx>
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
#include <STEPControl_Writer.hxx>
#include <TColgp_Array1OfPnt.hxx>
#include <TColgp_HArray1OfPnt.hxx>
#include <TColStd_Array1OfInteger.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TopAbs_Orientation.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopLoc_Location.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
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
#include <gp_Vec.hxx>

#include <internal/occt_kernel.hpp>
#include <occccad/kernel/geometry_id.hpp>

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
            output.properties.push_back(vector_property("normal", value.Axis().Direction().XYZ()));
            output.properties.push_back(
                vector_property("xDirection", value.XAxis().Direction().XYZ()));
            output.properties.push_back(
                vector_property("yDirection", value.YAxis().Direction().XYZ()));
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
        return {{spec.plane_origin.x, spec.plane_origin.y, spec.plane_origin.z}, normal, u,
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
        edge = BRepBuilderAPI_MakeEdge(profile_point(frame, curve.start),
                                       profile_point(frame, curve.end));
    } else if (curve.kind == "CIRCLE") {
        validate_positive(curve.radius, "profile circle radius");
        Handle(Geom_Circle) circle = new Geom_Circle(profile_axes(frame, curve.center), curve.radius);
        edge = BRepBuilderAPI_MakeEdge(circle);
    } else if (curve.kind == "ARC") {
        validate_positive(curve.radius, "profile arc radius");
        double end = curve.end_angle;
        while (end <= curve.start_angle)
            end += 2.0 * 3.14159265358979323846;
        Handle(Geom_Circle) circle = new Geom_Circle(profile_axes(frame, curve.center), curve.radius);
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

TopoDS_Wire make_profile_wire(const ProfileLoopSpec& loop, const ProfileFrame& frame) {
    if (loop.curves.empty())
        throw std::invalid_argument("profile loop has no curves");
    BRepBuilderAPI_MakeWire builder;
    for (const auto& curve : loop.curves)
        builder.Add(make_profile_edge(curve, frame));
    if (!builder.IsDone())
        throw std::runtime_error("profile wire construction failed: " + loop.id);
    return builder.Wire();
}

TopoDS_Shape make_profile_tool(const ProfilePadSpec& spec) {
    if (spec.regions.empty())
        throw std::invalid_argument("solid feature requires at least one profile region");
    const std::string generator = spec.generator.empty() ? "LINEAR_EXTRUDE" : spec.generator;
    if (generator == "LINEAR_EXTRUDE") {
        validate_positive(spec.pad_length, "extrude length");
    } else if (generator == "REVOLVE") {
        validate_positive(spec.revolve_angle, "revolve angle");
        if (spec.revolve_angle > 2.0 * 3.14159265358979323846 + 1.0e-12)
            throw std::invalid_argument("revolve angle must not exceed 2*pi");
        if (std::hypot(spec.axis_end.x - spec.axis_start.x,
                       spec.axis_end.y - spec.axis_start.y) <= 1.0e-9)
            throw std::invalid_argument("revolve axis is degenerate");
    } else {
        throw std::invalid_argument("unsupported solid generator: " + generator);
    }
    const ProfileFrame frame = profile_frame(spec);
    TopoDS_Shape result;
    for (const auto& region : spec.regions) {
        BRepBuilderAPI_MakeFace face_builder(
            make_profile_wire(region.outer, frame));
        for (const auto& hole : region.holes)
            face_builder.Add(make_profile_wire(hole, frame));
        face_builder.Build();
        if (!face_builder.IsDone() || !BRepCheck_Analyzer(face_builder.Face()).IsValid())
            throw std::runtime_error("profile face is invalid: " + region.id);
        TopoDS_Shape generated;
        if (generator == "LINEAR_EXTRUDE") {
            gp_Vec direction(frame.normal);
            direction.Multiply(spec.reversed ? -spec.pad_length : spec.pad_length);
            BRepPrimAPI_MakePrism prism(face_builder.Face(), direction);
            prism.Build();
            if (!prism.IsDone())
                throw std::runtime_error("profile prism failed: " + region.id);
            generated = prism.Shape();
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
        }
        if (generated.IsNull() || !BRepCheck_Analyzer(generated).IsValid())
            throw std::runtime_error("generated solid tool is invalid: " + region.id);
        if (result.IsNull())
            result = generated;
        else {
            BRepAlgoAPI_Fuse fuse(result, generated);
            fuse.Build();
            if (!fuse.IsDone())
                throw std::runtime_error("profile tool region fuse failed");
            result = fuse.Shape();
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

TopoDS_Shape apply_body_operation(const TopoDS_Shape& input, const TopoDS_Shape& tool,
                                  const std::string& requested_operation) {
    const std::string operation = requested_operation.empty() ? "ADD" : requested_operation;
    if (input.IsNull()) {
        if (operation == "REMOVE" || operation == "INTERSECT")
            throw std::invalid_argument(operation + " requires an input body");
        return tool;  // Legacy first ADD is equivalent to NEW_BODY.
    }
    if (operation == "NEW_BODY")
        throw std::invalid_argument("NEW_BODY requires an empty target body in the current single-body model");
    TopoDS_Shape result;
    if (operation == "ADD") {
        BRepAlgoAPI_Fuse algorithm(input, tool);
        algorithm.Build();
        if (!algorithm.IsDone()) throw std::runtime_error("body fuse failed");
        result = algorithm.Shape();
    } else if (operation == "REMOVE") {
        BRepAlgoAPI_Cut algorithm(input, tool);
        algorithm.Build();
        if (!algorithm.IsDone()) throw std::runtime_error("body cut failed");
        result = algorithm.Shape();
        if (shape_volume(input) - shape_volume(result) <= 1.0e-9)
            throw std::invalid_argument("NO_MATERIAL_CHANGE: cut does not intersect the target body");
    } else if (operation == "INTERSECT") {
        BRepAlgoAPI_Common algorithm(input, tool);
        algorithm.Build();
        if (!algorithm.IsDone()) throw std::runtime_error("body common failed");
        result = algorithm.Shape();
    } else {
        throw std::invalid_argument("unsupported body operation: " + operation);
    }
    if (result.IsNull() || shape_volume(result) <= 1.0e-9)
        throw std::invalid_argument("EMPTY_RESULT: solid operation produced no material");
    if (!BRepCheck_Analyzer(result).IsValid())
        throw std::runtime_error("solid operation produced invalid B-Rep");
    if (solid_count(result) != 1)
        throw std::invalid_argument("DISJOINT_RESULT: standard Body requires exactly one solid");
    return result;
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

GeometryId OcctKernel::combine(const std::vector<PlacedGeometry>& components) {
    if (components.empty()) {
        throw std::invalid_argument("exchange export requires at least one component");
    }
    BRep_Builder builder;
    TopoDS_Compound compound;
    builder.MakeCompound(compound);
    for (const auto& component : components) {
        gp_Trsf transform;
        transform.SetTranslation(
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

GeometryId OcctKernel::evaluateProfilePads(const std::vector<ProfilePadSpec>& specs,
                                           const std::vector<uint8_t>& base_brep) {
    TopoDS_Shape result;
    if (!base_brep.empty()) {
        const GeometryId base_id = loadBrepr(base_brep);
        result = impl_->find(base_id);
    }
    for (const auto& spec : specs) {
        const TopoDS_Shape tool = make_profile_tool(spec);
        result = apply_body_operation(result, tool, spec.body_operation);
    }
    if (result.IsNull())
        throw std::invalid_argument("feature chain contains no solid geometry");
    if (!BRepCheck_Analyzer(result).IsValid())
        throw std::runtime_error("feature chain produced invalid B-Rep");
    return impl_->store(result);
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
    return serializeStepComponents({{id, {0.0, 0.0, 0.0}}});
}

std::vector<uint8_t> OcctKernel::serializeStepComponents(
    const std::vector<PlacedGeometry>& components) {
    if (components.empty()) {
        throw std::invalid_argument("STEP export requires at least one component");
    }
    STEPControl_Writer writer;
    for (const auto& component : components) {
        gp_Trsf transform;
        transform.SetTranslation(
            gp_Vec(component.translation.x, component.translation.y, component.translation.z));
        const auto shape = impl_->find(component.geometry_id).Moved(TopLoc_Location(transform));
        if (writer.Transfer(shape, STEPControl_AsIs) != IFSelect_RetDone) {
            throw std::runtime_error("STEP component transfer failed");
        }
    }
    ScopedFile temporary(temporary_step_path());
    if (writer.Write(temporary.path.string().c_str()) != IFSelect_RetDone) {
        throw std::runtime_error("STEP write failed");
    }
    std::ifstream stream(temporary.path, std::ios::binary);
    if (!stream.is_open()) {
        throw std::runtime_error("cannot open temporary STEP output");
    }
    std::vector<uint8_t> data((std::istreambuf_iterator<char>(stream)),
                              std::istreambuf_iterator<char>());
    if (data.empty()) {
        throw std::runtime_error("cannot read temporary STEP output");
    }
    return data;
}

}  // namespace occccad::kernel

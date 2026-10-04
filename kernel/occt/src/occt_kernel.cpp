#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepCheck_ListIteratorOfListOfStatus.hxx>
#include <BRepCheck_Result.hxx>
#include <BRepCheck_Wire.hxx>
#include <BRepFilletAPI_MakeChamfer.hxx>
#include <BRepFilletAPI_MakeFillet.hxx>
#include <BRepGProp.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRepOffsetAPI_DraftAngle.hxx>
#include <BRepOffsetAPI_MakeThickSolid.hxx>
#include <BRepOffsetAPI_ThruSections.hxx>
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
#include <Precision.hxx>
#include <STEPControl_Reader.hxx>
#include <ShapeBuild_ReShape.hxx>
#include <ShapeFix_Face.hxx>
#include <ShapeFix_Shape.hxx>
#include <ShapeUpgrade_UnifySameDomain.hxx>
#include <TColStd_Array1OfInteger.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TColgp_Array1OfPnt.hxx>
#include <TColgp_HArray1OfPnt.hxx>
#include <TopAbs_Orientation.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopLoc_Location.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopTools_ListIteratorOfListOfShape.hxx>
#include <TopTools_ListOfShape.hxx>
#include <TopTools_MapOfShape.hxx>
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
#include <gp_Quaternion.hxx>
#include <gp_Trsf.hxx>
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

TopologyProperty text_property(std::string name, std::string value) {
    TopologyProperty result;
    result.name = std::move(name);
    result.kind = TopologyProperty::Kind::TEXT;
    result.text_value = std::move(value);
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
    output.properties.push_back(text_property("lengthUnit", "mm"));
    output.properties.push_back(text_property("angleUnit", "rad"));
    // +1: outward analytic normal; -1: the material lies on the other side
    // (e.g. a cavity wall). This is not a Same/Opposite constraint intent.
    output.properties.push_back(integer_property("materialSide", face.Orientation() == TopAbs_REVERSED ? -1 : 1));
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
    output.properties.push_back(vector_property("snapCenter", area.CentreOfMass().XYZ()));
    // Read-only interaction hints from actual topological boundaries, never
    // triangulation/PCA. Bounded output: one direction and two axial centers.
    double longest = 0;
    gp_XYZ boundary_direction;
    std::vector<gp_Pnt> circular_centers;
    for (TopExp_Explorer edges(face, TopAbs_EDGE); edges.More(); edges.Next()) {
        BRepAdaptor_Curve edge(TopoDS::Edge(edges.Current()));
        if (edge.GetType() == GeomAbs_Line && std::isfinite(edge.FirstParameter()) && std::isfinite(edge.LastParameter())) {
            const double length = std::abs(edge.LastParameter() - edge.FirstParameter());
            auto direction = edge.Line().Direction().XYZ();
            for (int i = 1; i <= 3; ++i) if (std::abs(direction.Coord(i)) > 1e-12) {
                if (direction.Coord(i) < 0) direction *= -1;
                break;
            }
            const bool tie = std::abs(length - longest) <= 1e-9;
            if (length > longest + 1e-9 || (tie && std::array{direction.X(),direction.Y(),direction.Z()} > std::array{boundary_direction.X(),boundary_direction.Y(),boundary_direction.Z()})) {
                longest = length; boundary_direction = direction;
            }
        } else if (surface.GetType() == GeomAbs_Cylinder && edge.GetType() == GeomAbs_Circle) {
            const auto circle = edge.Circle();
            if (circle.Axis().Direction().IsParallel(surface.Cylinder().Axis().Direction(), 1e-9))
                circular_centers.push_back(circle.Location());
        }
    }
    if (surface.GetType() == GeomAbs_Plane && longest > 1e-9)
        output.properties.push_back(vector_property("snapBoundaryDirection", boundary_direction));
    if (!circular_centers.empty()) {
        const auto axis = surface.Cylinder().Axis();
        std::sort(circular_centers.begin(), circular_centers.end(), [&](const auto& a,const auto& b){
            return gp_Vec(axis.Location(),a).Dot(gp_Vec(axis.Direction())) < gp_Vec(axis.Location(),b).Dot(gp_Vec(axis.Direction()));
        });
        output.properties.push_back(vector_property("snapEndFirst", circular_centers.front().XYZ()));
        output.properties.push_back(vector_property("snapEndLast", circular_centers.back().XYZ()));
    }
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
            output.properties.push_back(vector_property("apex", value.Apex().XYZ()));
            output.properties.push_back(vector_property("origin", value.Location().XYZ()));
            output.properties.push_back(vector_property("axis", value.Axis().Direction().XYZ()));
            output.properties.push_back(number_property("referenceRadius", value.RefRadius()));
            output.properties.push_back(number_property("semiAngle", value.SemiAngle()));
            output.properties.push_back(integer_property("coneLeaf", value.SemiAngle() >= 0 ? 1 : -1));
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
    output.properties.push_back(text_property("lengthUnit", "mm"));
    output.properties.push_back(text_property("parameterUnit", curve.GetType() == GeomAbs_Line ? "mm" : curve.GetType() == GeomAbs_Circle ? "rad" : "native"));
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
            output.properties.push_back(vector_property("xDirection", value.XAxis().Direction().XYZ()));
            output.properties.push_back(number_property("radius", value.Radius()));
            output.properties.push_back(boolean_property("fullCircle", std::abs(last-first-2.0*std::acos(-1.0)) <= Precision::PConfusion()));
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
        const double sweep=curve.end_angle-curve.start_angle;
        if (!std::isfinite(sweep) || std::abs(sweep)>=2*3.14159265358979323846 || curve.radius*std::abs(sweep)<=topology_linear_tolerance_meters*1000.0)
            throw std::invalid_argument("DEGENERATE_PROFILE_EDGE: invalid arc interval or below naming tolerance");
        Handle(Geom_Circle) circle = new Geom_Circle(profile_axes(frame,curve.center),curve.radius);
        edge=BRepBuilderAPI_MakeEdge(circle,std::min(curve.start_angle,curve.end_angle),std::max(curve.start_angle,curve.end_angle));
        if (sweep<0) edge=TopoDS::Edge(edge.Reversed());
    } else if (curve.kind == "ELLIPSE" || curve.kind == "ELLIPTICAL_ARC") {
        validate_positive(curve.minor_radius, "profile ellipse minor radius");
        if (!std::isfinite(curve.major_radius) || curve.major_radius <= curve.minor_radius || !std::isfinite(curve.rotation))
            throw std::invalid_argument("profile ellipse requires major > minor > 0 and finite rotation");
        gp_Ax2 axes = profile_axes(frame, curve.center);
        axes.Rotate(gp_Ax1(axes.Location(),axes.Direction()),curve.rotation);
        Handle(Geom_Ellipse) ellipse = new Geom_Ellipse(axes,curve.major_radius,curve.minor_radius);
        if (curve.kind == "ELLIPSE") edge = BRepBuilderAPI_MakeEdge(ellipse);
        else {
            if (!std::isfinite(curve.start_angle) || !std::isfinite(curve.end_angle) || curve.start_angle==curve.end_angle || std::abs(curve.end_angle-curve.start_angle)>=2*3.14159265358979323846)
                throw std::invalid_argument("invalid elliptical arc parameter interval");
            edge = BRepBuilderAPI_MakeEdge(ellipse,std::min(curve.start_angle,curve.end_angle),std::max(curve.start_angle,curve.end_angle));
            if (curve.end_angle<curve.start_angle) edge=TopoDS::Edge(edge.Reversed());
        }
    } else if (curve.kind == "SPLINE") {
        // Use exact canonical poles and knots when supplied. Legacy FIT input
        // is normalized once here; already canonical curves are never re-fitted.
        auto request=curve;
        if (!request.poles.empty()) request.mode="CONTROL";
        const auto canonical = canonicalize_sketch_curve(request);
        TColgp_Array1OfPnt poles(1,static_cast<int>(canonical.poles.size()));
        TColStd_Array1OfReal weights(1,poles.Length()),knots(1,static_cast<int>(canonical.knots.size()));
        TColStd_Array1OfInteger multiplicities(1,knots.Length());
        if(canonical.weights.size()!=canonical.poles.size() || canonical.multiplicities.size()!=canonical.knots.size())
            throw std::invalid_argument("invalid canonical profile spline arrays");
        for(int i=1;i<=poles.Length();++i) {
            poles.SetValue(i,profile_point(frame,canonical.poles[static_cast<std::size_t>(i-1)]));
            weights.SetValue(i,canonical.weights[static_cast<std::size_t>(i-1)]);
        }
        for(int i=1;i<=knots.Length();++i) {
            knots.SetValue(i,canonical.knots[static_cast<std::size_t>(i-1)]);
            multiplicities.SetValue(i,static_cast<int>(canonical.multiplicities[static_cast<std::size_t>(i-1)]));
        }
        Handle(Geom_BSplineCurve) spline = new Geom_BSplineCurve(poles,weights,knots,multiplicities,
                                                               static_cast<int>(canonical.degree),canonical.periodic);
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
    if (curve.kind == "ELLIPSE" || curve.kind == "ELLIPTICAL_ARC") {
        const double t = curve.kind=="ELLIPSE"?0:(start?curve.start_angle:curve.end_angle);
        const double x=curve.major_radius*std::cos(t),y=curve.minor_radius*std::sin(t);
        return {curve.center.x+x*std::cos(curve.rotation)-y*std::sin(curve.rotation),curve.center.y+x*std::sin(curve.rotation)+y*std::cos(curve.rotation)};
    }
    if (curve.kind == "SPLINE") {
        const auto canonical=curve.poles.empty()?canonicalize_sketch_curve(curve):curve;
        return evaluate_sketch_curve(canonical,start?canonical.parameter_start:canonical.parameter_end).point;
    }
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
        if (!algorithm.IsDeleted(source.shape) && contains(source.shape)) {
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
        // Keep the declared sketch frame as the face parameter orientation;
        // automatic plane inference can flip it for a reversed closed curve and
        // thereby turn the formally oriented hole into a second outer wire.
        BRepBuilderAPI_MakeFace face_builder(
            gp_Pln(gp_Ax3(frame.origin, frame.normal, frame.u_direction)),
            build_wire(region.outer), true);
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
        const auto name_sweep = [&](auto& prism) {
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
        };
        TopoDS_Shape generated;
        if (generator == "LINEAR_EXTRUDE") {
            gp_Vec direction(frame.normal);
            direction.Multiply(spec.reversed ? -spec.pad_length : spec.pad_length);
            BRepPrimAPI_MakePrism prism(face_builder.Face(), direction);
            prism.Build();
            if (!prism.IsDone())
                throw std::runtime_error("profile prism failed: " + region.id);
            generated = prism.Shape();
            name_sweep(prism);
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
            name_sweep(revolve);
            // A full turn has one seam rather than two independent caps/boundaries.
            // Keep only actual final topology; shared seam identities are merged later.
            TopTools_IndexedMapOfShape members;
            TopExp::MapShapes(generated, members);
            result.named.erase(std::remove_if(result.named.begin(), result.named.end(),
                                              [&](const auto& named) {
                                                  return named.ref.source_ids.size() &&
                                                         named.ref.source_ids.front() ==
                                                             region.id &&
                                                         !members.Contains(named.shape);
                                              }),
                               result.named.end());
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

void validate_body_solid_set(const TopoDS_Shape& shape) {
    if (shape.IsNull() || solid_count(shape) == 0 || shape_volume(shape) <= 1.0e-9)
        throw std::invalid_argument("EMPTY_RESULT: solid operation produced no material");
    if (!BRepCheck_Analyzer(shape).IsValid())
        throw std::runtime_error("INVALID_RESULT: solid operation produced invalid B-Rep");
    // A compound of several solids is legal. Loose faces, edges and vertices are
    // not Body material, even when the compound also contains valid solids.
    for (const auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX})
        if (TopExp_Explorer(shape, type, TopAbs_SOLID).More())
            throw std::invalid_argument("INVALID_RESULT: body contains topology outside solids");
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
        evidence.radius_mm = surface.Cylinder().Radius();
    } else if (surface.GetType() == GeomAbs_Sphere) {
        evidence.geometry_type = "SPHERE";
        evidence.origin = to_vec3(surface.Sphere().Location());
        evidence.radius_mm = surface.Sphere().Radius();
    } else if (surface.GetType() == GeomAbs_Cone) {
        const auto cone = surface.Cone();
        evidence.geometry_type = "CONE";
        evidence.origin = to_vec3(cone.Apex());
        const auto direction = cone.Axis().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
        evidence.half_angle_radians = std::abs(cone.SemiAngle());
        evidence.cone_leaf = cone.SemiAngle() >= 0 ? 1 : -1;
    }
    if (surface.GetType() == GeomAbs_Plane || surface.GetType() == GeomAbs_Cylinder ||
        surface.GetType() == GeomAbs_Sphere || surface.GetType() == GeomAbs_Cone)
        evidence.material_side = face.Orientation() == TopAbs_REVERSED ? -1 : 1;
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << *evidence.measure_si << '|' << evidence.centroid.x
              << ',' << evidence.centroid.y << ',' << evidence.centroid.z << '|'
              << evidence.origin.x << ',' << evidence.origin.y << ',' << evidence.origin.z << '|'
              << evidence.direction.x << ',' << evidence.direction.y << ',' << evidence.direction.z;
    if (evidence.radius_mm) canonical << "|radius_mm=" << *evidence.radius_mm;
    if (evidence.half_angle_radians) canonical << "|half_angle_rad=" << *evidence.half_angle_radians;
    if (evidence.cone_leaf) canonical << "|cone_leaf=" << *evidence.cone_leaf;
    if (evidence.material_side) canonical << "|material_side=" << *evidence.material_side;
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
        evidence.radius_mm = curve.Circle().Radius();
        const auto x = curve.Circle().XAxis().Direction();
        evidence.x_direction = Vec3{x.X(), x.Y(), x.Z()};
    }
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << *evidence.measure_si << '|'
              << evidence.centroid.x << ',' << evidence.centroid.y << ','
              << evidence.centroid.z << '|' << evidence.origin.x << ',' << evidence.origin.y
              << ',' << evidence.origin.z << '|' << evidence.direction.x << ','
              << evidence.direction.y << ',' << evidence.direction.z << '|'
              << curve.FirstParameter() << ',' << curve.LastParameter();
    if (evidence.radius_mm) canonical << "|radius_mm=" << *evidence.radius_mm;
    if (evidence.x_direction) canonical << "|x_direction=" << evidence.x_direction->x << ',' << evidence.x_direction->y << ',' << evidence.x_direction->z;
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

ToolBuild make_loft_tool(const ProfilePadSpec& spec) {
    if (spec.sections.size() < 2 || spec.sections.size() > 32)
        throw std::invalid_argument("INVALID_LOFT_SECTION_COUNT");
    BRepOffsetAPI_ThruSections algorithm(Standard_True, spec.ruled, 1e-7);
    // CompatibleWires splits unlike sections and records the original-edge to
    // generated-face history. Keep explicit seam/direction for matching wires.
    const auto count = spec.sections.front().region.outer.curves.size();
    const bool unlike = std::any_of(spec.sections.begin(), spec.sections.end(),
        [&](const auto& section) { return section.region.outer.curves.size() != count; });
    if (unlike) {
        for (const auto& section : spec.sections) {
            const auto& curves = section.region.outer.curves;
            if (curves.size() != count && !(curves.size() == 1 && curves.front().kind == "CIRCLE") &&
                !(count == 1 && spec.sections.front().region.outer.curves.front().kind == "CIRCLE"))
                throw std::invalid_argument("UNSUPPORTED_UNLIKE_LOFT_SECTIONS");
        }
    }
    algorithm.CheckCompatibility(unlike ? Standard_True : Standard_False);
    algorithm.SetMutableInput(Standard_False);
    std::vector<std::pair<SemanticTopologyRef, TopoDS_Edge>> edges;
    for (const auto& section : spec.sections) {
        if (!section.region.holes.empty() || section.region.outer.curves.empty())
            throw std::invalid_argument("LOFT_REQUIRES_SINGLE_CLOSED_SECTION");
        auto curves = section.region.outer.curves;
        if (section.reversed) {
            std::reverse(curves.begin(), curves.end());
            for (auto& curve : curves)
                curve.reversed = !curve.reversed;
        }
        if (!section.seam_entity_id.empty()) {
            auto seam = std::find_if(curves.begin(), curves.end(), [&](const auto& curve) {
                return curve.entity_id == section.seam_entity_id;
            });
            if (seam == curves.end())
                throw std::invalid_argument("SECTION_SEAM_MISSING");
            std::rotate(curves.begin(), seam, curves.end());
        }
        ProfilePadSpec plane;
        plane.plane_origin = section.origin;
        plane.plane_normal = section.normal;
        plane.plane_u_direction = section.u_direction;
        const auto frame = profile_frame(plane);
        BRepBuilderAPI_MakeWire wire;
        for (const auto& curve : curves) {
            auto edge = make_profile_edge(curve, frame);
            if (curve.kind == "CIRCLE" && section.seam_angle != 0) {
                Handle(Geom_Circle) circle =
                    new Geom_Circle(profile_axes(frame, curve.center), curve.radius);
                edge = BRepBuilderAPI_MakeEdge(circle, section.seam_angle,
                                               section.seam_angle + 2 * 3.14159265358979323846);
                if (curve.reversed)
                    edge.Reverse();
            }
            wire.Add(edge);
            if (!wire.IsDone())
                throw std::invalid_argument("INVALID_LOFT_WIRE");
            edges.push_back({{section.sketch_id,
                              "PROFILE_EDGE/" + curve.entity_id,
                              {section.region.id, curve.entity_id}},
                             wire.Edge()});
        }
        if (!wire.Wire().Closed())
            throw std::invalid_argument("OPEN_LOFT_SECTION");
        algorithm.AddWire(wire.Wire());
    }
    algorithm.Build();
    if (!algorithm.IsDone())
        throw std::runtime_error("LOFT_ALGORITHM_FAILED");
    ToolBuild tool;
    tool.feature_id = spec.feature_id;
    tool.shape = algorithm.Shape();
    validate_body_solid_set(tool.shape);
    const auto& first = spec.sections.front();
    const auto& last = spec.sections.back();
    append_generated(
        tool, {first.sketch_id, "PROFILE_REGION/" + first.region.id, {first.region.id}},
        {spec.feature_id, "LOFT_START_CAP", {first.sketch_id}}, algorithm.FirstShape());
    append_generated(tool, {last.sketch_id, "PROFILE_REGION/" + last.region.id, {last.region.id}},
                     {spec.feature_id, "LOFT_END_CAP", {last.sketch_id}}, algorithm.LastShape());
    for (const auto& [source, edge] : edges) {
        const auto generated = algorithm.Generated(edge);
        for (TopTools_ListIteratorOfListOfShape it(generated); it.More(); it.Next()) {
            if (it.Value().ShapeType() != TopAbs_FACE)
                continue;
            const auto key = make_geometry_id(ref_key(source)).substr(7);
            append_generated(
                tool, source,
                {spec.feature_id, "LOFT_SIDE/" + key, {source.feature_id, source.output_slot}},
                it.Value());
        }
    }
    auto closure = complete_boolean_topology_naming(tool.shape, tool.named, spec.feature_id);
    tool.generated.insert(tool.generated.end(), closure.begin(), closure.end());
    return tool;
}

BodyOperationResult apply_body_operation(const TopoDS_Shape& input,
                                         const std::vector<NamedShape>& input_named,
                                         const ToolBuild& tool,
                                         const std::string& requested_operation) {
    const std::string operation = requested_operation.empty() ? "ADD" : requested_operation;
    if (input.IsNull()) {
        if (operation == "REMOVE" || operation == "INTERSECT")
            throw std::invalid_argument(operation + " requires an input body");
        validate_body_solid_set(tool.shape);
        return {tool.shape, tool.named, {}, {}};
    }
    if (operation == "NEW_BODY")
        throw std::invalid_argument(
            "NEW_BODY requires an empty target body");
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
        if (shape_volume(result) - shape_volume(input) <= 1.0e-9)
            throw std::invalid_argument("NO_MATERIAL_CHANGE: add does not change the target body");
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
        if (!result.IsNull() && shape_volume(input) - shape_volume(result) <= 1.0e-9)
            throw std::invalid_argument(
                "NO_MATERIAL_CHANGE: intersection does not change the target body");
    } else {
        throw std::invalid_argument("unsupported body operation: " + operation);
    }
    if (result.IsNull() || solid_count(result) == 0)
        throw std::invalid_argument("EMPTY_RESULT: solid operation produced no material");
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
    validate_body_solid_set(result);
    auto derived = complete_boolean_topology_naming(result, mapped, tool.feature_id);
    std::vector<std::string> diagnostics;
    if (!derived.empty())
        diagnostics.push_back("TOPOLOGY_HISTORY_DERIVED_CLOSURE:" +
                              std::to_string(derived.size()));
    return {result, mapped, std::move(derived), std::move(diagnostics)};
}

// Local modifiers use the exact upstream shape and semantic references. No
// saved local topology index, geometry search, or nearest-element recovery.
BodyOperationResult apply_local_modifier(const TopoDS_Shape& input,
                                         const std::vector<NamedShape>& named,
                                         const ProfilePadSpec& spec) {
    if (input.IsNull() || spec.selections.empty() || spec.selections.size() > 128)
        throw std::invalid_argument("INVALID_MODIFIER_INPUT");
    std::vector<TopoDS_Shape> selected;
    for (const auto& ref : spec.selections) {
        SemanticTopologyRef semantic{ref.feature_id, ref.output_slot, ref.source_ids};
        std::vector<TopoDS_Shape> matches;
        for (const auto& item : named)
            if (same_ref(item.ref, semantic))
                matches.push_back(item.shape);
        if (matches.size() != 1)
            throw std::invalid_argument("MODIFIER_SELECTION_MISSING_OR_AMBIGUOUS");
        const auto expected =
            (spec.generator == "FILLET" || spec.generator == "CHAMFER") ? TopAbs_EDGE : TopAbs_FACE;
        if (matches.front().ShapeType() != expected)
            throw std::invalid_argument("MODIFIER_SELECTION_TYPE_MISMATCH");
        if (std::none_of(selected.begin(), selected.end(),
                         [&](const auto& shape) { return shape.IsSame(matches.front()); }))
            selected.push_back(matches.front());
    }
    const auto finish = [&](auto& algorithm) -> BodyOperationResult {
        if (!algorithm.IsDone())
            throw std::runtime_error("MODIFIER_ALGORITHM_FAILED:" + spec.generator);
        const auto result = algorithm.Shape();
        validate_body_solid_set(result);
        auto mapped = map_named_shapes(named, algorithm, result);
        std::vector<TopologyLineage> derived;
        TopTools_IndexedMapOfShape members;
        TopExp::MapShapes(result, members);
        for (const auto& source : named) {
            const auto generated = algorithm.Generated(source.shape);
            for (TopTools_ListIteratorOfListOfShape it(generated); it.More(); it.Next()) {
                const auto& shape = it.Value();
                if (shape.ShapeType() == source.shape.ShapeType() || !members.Contains(shape) ||
                    (shape.ShapeType() != TopAbs_FACE && shape.ShapeType() != TopAbs_EDGE &&
                     shape.ShapeType() != TopAbs_VERTEX))
                    continue;
                const auto key = make_geometry_id(ref_key(source.ref)).substr(7);
                SemanticTopologyRef ref{
                    spec.feature_id,
                    "MODIFIER_GENERATED/" + key + "/" +
                        std::to_string(static_cast<int>(persistent_topology_type(shape))),
                    {source.ref.feature_id, source.ref.output_slot}};
                mapped.push_back({ref, shape});
                derived.push_back({{source.ref}, ref, TopologyLineageKind::generated, {}});
            }
        }
        std::vector<TopologyLineage> closure;
        try {
            closure = complete_boolean_topology_naming(result, mapped, spec.feature_id);
        } catch (const std::exception& error) {
            throw std::runtime_error(spec.generator + ":" + error.what());
        }
        derived.insert(derived.end(), closure.begin(), closure.end());
        return {result, std::move(mapped), std::move(derived), {}};
    };
    if (spec.generator == "FILLET") {
        validate_positive(spec.pad_length, "fillet radius");
        BRepFilletAPI_MakeFillet algorithm(input);
        for (const auto& shape : selected) {
            algorithm.Add(spec.pad_length, TopoDS::Edge(shape));
        }
        algorithm.Build();
        return finish(algorithm);
    }
    if (spec.generator == "CHAMFER") {
        validate_positive(spec.pad_length, "chamfer distance");
        BRepFilletAPI_MakeChamfer algorithm(input);
        for (const auto& shape : selected) {
            algorithm.Add(spec.pad_length, TopoDS::Edge(shape));
        }
        algorithm.Build();
        return finish(algorithm);
    }
    if (spec.generator == "DRAFT") {
        validate_positive(spec.revolve_angle, "draft angle");
        if (spec.revolve_angle >= 1.5707963267948966)
            throw std::invalid_argument("INVALID_DRAFT_ANGLE");
        gp_Dir normal(spec.neutral_normal.x, spec.neutral_normal.y, spec.neutral_normal.z);
        gp_Pln neutral(gp_Pnt(spec.neutral_origin.x, spec.neutral_origin.y, spec.neutral_origin.z),
                       normal);
        BRepOffsetAPI_DraftAngle algorithm(input);
        for (const auto& shape : selected) {
            algorithm.Add(TopoDS::Face(shape), normal,
                          spec.reversed ? -spec.revolve_angle : spec.revolve_angle, neutral);
            if (!algorithm.AddDone())
                throw std::invalid_argument("DRAFT_FACE_FAILED");
        }
        algorithm.Build();
        return finish(algorithm);
    }
    if (spec.generator == "SHELL") {
        validate_positive(spec.pad_length, "shell thickness");
        TopTools_ListOfShape removed;
        for (const auto& shape : selected)
            removed.Append(shape);
        BRepOffsetAPI_MakeThickSolid algorithm;
        algorithm.MakeThickSolidByJoin(
            input, removed, spec.reversed ? spec.pad_length : -spec.pad_length, 1e-7,
            BRepOffset_Skin, Standard_False, Standard_False, GeomAbs_Intersection);
        auto result = finish(algorithm);
        // OCCT may report a valid offset after an inward wall has crossed the
        // opposite boundary. Such a solid is not an inward shell of the input.
        if (!spec.reversed) {
            const double tolerance = std::max(1.0e-7, shape_volume(input) * 1.0e-9);
            if (shape_volume(result.shape) >= shape_volume(input) - tolerance)
                throw std::invalid_argument("SHELL_THICKNESS_EXCEEDS_INTERIOR");
            BRepAlgoAPI_Cut outside(result.shape, input);
            outside.Build();
            if (!outside.IsDone() || shape_volume(outside.Shape()) > tolerance)
                throw std::invalid_argument("SHELL_OFFSET_OUTSIDE_INPUT");
        }
        return result;
    }
    throw std::invalid_argument("UNSUPPORTED_LOCAL_MODIFIER");
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
        try {
            const auto input_shape = result;
            const auto input_id = result_id;
            const auto input_named = live_named;
            ToolBuild tool;
            const bool modifier = spec.generator == "FILLET" || spec.generator == "CHAMFER" ||
                                  spec.generator == "DRAFT" || spec.generator == "SHELL";
            if (modifier) {
                tool.feature_id = spec.feature_id;
            } else if (spec.generator == "BOOLEAN") {
                if (result.IsNull() || spec.tools.empty() || spec.tools.size() > 32 ||
                    (spec.body_operation == "INTERSECT" && spec.tools.size() != 1))
                    throw std::invalid_argument("INVALID_BOOLEAN_INPUT");
                tool.feature_id = spec.feature_id;
                for (const auto& source : spec.tools) {
                    if (source.body_id == spec.body_id ||
                        source.geometry_id !=
                            make_geometry_id(std::string(source.brep.begin(), source.brep.end())))
                        throw std::invalid_argument("BOOLEAN_TOOL_SNAPSHOT_MISMATCH");
                    const auto shape = impl_->find(loadBrepr(source.brep));
                    validate_body_solid_set(shape);
                    TopTools_IndexedMapOfShape faces, edges, vertices;
                    TopExp::MapShapes(shape, TopAbs_FACE, faces);
                    TopExp::MapShapes(shape, TopAbs_EDGE, edges);
                    TopExp::MapShapes(shape, TopAbs_VERTEX, vertices);
                    if (source.topology.size() !=
                        static_cast<std::size_t>(faces.Extent() + edges.Extent() +
                                                 vertices.Extent()))
                        throw std::invalid_argument("BOOLEAN_TOOL_NAMING_INCOMPLETE");
                    std::unordered_set<std::string> locators, refs;
                    for (const auto& output : source.topology) {
                        const auto& map = output.type == TopologyType::FACE   ? faces
                                          : output.type == TopologyType::EDGE ? edges
                                                                              : vertices;
                        SemanticTopologyRef upstream{output.feature_id, output.output_slot,
                                                     output.source_ids};
                        if (output.local_id == 0 ||
                            output.local_id > static_cast<unsigned>(map.Extent()) ||
                            !locators
                                 .insert(std::to_string(static_cast<int>(output.type)) + "/" +
                                         std::to_string(output.local_id))
                                 .second ||
                            !refs.insert(ref_key(upstream)).second)
                            throw std::invalid_argument("BOOLEAN_TOOL_NAMING_INVALID");
                        const auto suffix =
                            make_geometry_id(source.body_id + "/" + ref_key(upstream)).substr(7);
                        append_generated(tool, upstream,
                                         {spec.feature_id,
                                          "TOOL_TOPOLOGY/" + suffix,
                                          {source.body_id, source.feature_id}},
                                         map(static_cast<int>(output.local_id)));
                    }
                    if (tool.shape.IsNull())
                        tool.shape = shape;
                    else {
                        BRepAlgoAPI_Fuse combine(tool.shape, shape);
                        combine.Build();
                        if (!combine.IsDone())
                            throw std::runtime_error("BOOLEAN_TOOL_UNION_FAILED");
                        tool.named = map_named_shapes(tool.named, combine, combine.Shape());
                        tool.shape = combine.Shape();
                    }
                }
            } else if (spec.generator == "LOFT") {
                tool = make_loft_tool(spec);
            } else {
                auto generator_spec = spec;
                if (spec.generator == "LINEAR_EXTRUDE" || spec.generator.empty()) {
                    auto frame = profile_frame(spec);
                    gp_Vec axis(frame.normal);
                    if (spec.reversed)
                        axis.Reverse();
                    double start = 0, length = spec.pad_length;
                    if (spec.extent == "SYMMETRIC")
                        start = -length / 2;
                    else if (spec.extent == "TWO_SIDED") {
                        validate_positive(spec.second_length, "second length");
                        start = -spec.second_length;
                        length += spec.second_length;
                    } else if (spec.extent == "THROUGH_ALL") {
                        if (spec.body_operation != "REMOVE" || result.IsNull())
                            throw std::invalid_argument("THROUGH_ALL_REQUIRES_CUT_TARGET");
                        Bnd_Box box;
                        BRepBndLib::Add(result, box);
                        const auto bounds = to_bbox(box);
                        double low = std::numeric_limits<double>::infinity(), high = -low;
                        for (double x : {bounds.min.x, bounds.max.x})
                            for (double y : {bounds.min.y, bounds.max.y})
                                for (double z : {bounds.min.z, bounds.max.z}) {
                                    double projection =
                                        gp_Vec(frame.origin, gp_Pnt(x, y, z)).Dot(axis);
                                    low = std::min(low, projection);
                                    high = std::max(high, projection);
                                }
                        const double margin = std::max(1e-5, (high - low) * 1e-6);
                        start = low - margin;
                        length = high - low + 2 * margin;
                    } else if (!spec.extent.empty() && spec.extent != "FINITE")
                        throw std::invalid_argument("INVALID_EXTRUDE_EXTENT");
                    auto origin = frame.origin.Translated(axis.Multiplied(start));
                    generator_spec.plane_origin = {origin.X(), origin.Y(), origin.Z()};
                    generator_spec.plane_normal = {frame.normal.X(), frame.normal.Y(),
                                                   frame.normal.Z()};
                    generator_spec.plane_u_direction = {
                        frame.u_direction.X(), frame.u_direction.Y(), frame.u_direction.Z()};
                    generator_spec.pad_length = length;
                }
                tool = make_profile_tool(generator_spec);
            }
            topology_history_complete = topology_history_complete && tool.topology_history_complete;
            auto operation =
                modifier ? apply_local_modifier(result, live_named, spec)
                         : apply_body_operation(result, live_named, tool, spec.body_operation);
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
                auto group = std::find_if(
                    groups.begin(), groups.end(),
                    [&](const OutputGroup& value) { return value.shape.IsSame(mapped.shape); });
                if (group == groups.end()) {
                    groups.push_back({mapped.shape, {mapped.ref}});
                } else if (std::none_of(
                               group->sources.begin(), group->sources.end(),
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
                const auto left_key =
                    left.sources.empty() ? std::string{} : ref_key(left.sources.front());
                const auto right_key =
                    right.sources.empty() ? std::string{} : ref_key(right.sources.front());
                if (left_key != right_key)
                    return left_key < right_key;
                return topology_evidence(left.shape).evidence_digest <
                       topology_evidence(right.shape).evidence_digest;
            });
            for (const auto& group : groups)
                for (const auto& source : group.sources)
                    ++source_counts[ref_key(source)];
            std::unordered_map<std::string, std::size_t> merged_counts, merged_indices;
            std::unordered_set<std::string> merged_evidence;
            for (const auto& group : groups)
                if (group.sources.size() > 1) {
                    std::string key;
                    for (const auto& source : group.sources)
                        key += ref_key(source) + std::string(1, '\0');
                    ++merged_counts[key];
                    if (!merged_evidence
                             .insert(key + "/" + topology_evidence(group.shape).evidence_digest)
                             .second)
                        throw std::runtime_error("TOPOLOGY_HISTORY_MERGE_SPLIT_AMBIGUOUS");
                }
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
                    auto slot = "MERGED_FROM/" + merged_digest.substr(7);
                    if (merged_counts[merged.str()] > 1)
                        slot += "/SPLIT/" + std::to_string(++merged_indices[merged.str()]);
                    output_ref = {spec.feature_id, slot, source_ids};
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
                outputs.push_back(
                    {{output_ref, output_type, static_cast<std::uint64_t>(local_id), evidence},
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
                if (spec.generator == "LOFT") {
                    // Unlike sections split one circle into several side faces.
                    // Preserve every actual section-edge source, including when
                    // several generated references name the same resulting face.
                    std::vector<SemanticTopologyRef> section_sources;
                    for (const auto& source : group.sources) {
                        bool found = false;
                        for (const auto& generated : tool.generated) {
                            if (!same_ref(generated.result, source)) continue;
                            section_sources.insert(section_sources.end(), generated.sources.begin(), generated.sources.end());
                            found = true;
                        }
                        if (!found) section_sources.push_back(source);
                    }
                    sort_refs(section_sources);
                    lineage_sources = section_sources;
                }
                feature.topology_history.lineage.push_back(
                    {lineage_sources, output_ref, kind, evidence});
                if (kind == TopologyLineageKind::split) {
                    auto ambiguity = std::find_if(feature.topology_history.ambiguous.begin(),
                                                  feature.topology_history.ambiguous.end(),
                                                  [&](const AmbiguousLineage& value) {
                                                      return value.sources.size() == 1U &&
                                                             same_ref(value.sources.front(),
                                                                      group.sources.front());
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
                auto lineage = std::find_if(
                    feature.topology_history.lineage.begin(),
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
                        return std::count_if(
                            outputs.begin(), outputs.end(),
                            [&](const auto& output) { return output.first.topology_type == type; });
                    };
                    std::ostringstream diagnostic;
                    diagnostic << "TOPOLOGY_HISTORY_INCOMPLETE_FINAL_SHAPE:faces="
                               << output_count(PersistentTopologyType::face) << '/'
                               << final_faces.Extent()
                               << ",edges=" << output_count(PersistentTopologyType::edge) << '/'
                               << final_edges.Extent()
                               << ",vertices=" << output_count(PersistentTopologyType::vertex)
                               << '/' << final_vertices.Extent();
                    throw std::runtime_error(diagnostic.str());
                }
                std::vector<std::string> local_ids;
                std::vector<std::string> semantic_refs;
                for (const auto& output : outputs) {
                    if (output.first.local_id == 0U ||
                        output.first.topology_type == PersistentTopologyType::unspecified)
                        throw std::runtime_error("TOPOLOGY_HISTORY_INVALID_LOCAL_ID");
                    local_ids.push_back(
                        std::to_string(static_cast<int>(output.first.topology_type)) + "/" +
                        std::to_string(output.first.local_id));
                    semantic_refs.push_back(ref_key(output.first.semantic_ref));
                    const auto matches = std::count_if(
                        feature.topology_history.lineage.begin(),
                        feature.topology_history.lineage.end(), [&](const auto& value) {
                            return same_ref(value.result, output.first.semantic_ref);
                        });
                    if (matches != 1)
                        throw std::runtime_error("TOPOLOGY_HISTORY_LINEAGE_OUTPUT_MISMATCH");
                }
                std::sort(local_ids.begin(), local_ids.end());
                if (std::adjacent_find(local_ids.begin(), local_ids.end()) != local_ids.end())
                    throw std::runtime_error("TOPOLOGY_HISTORY_DUPLICATE_LOCAL_ID");
                std::sort(semantic_refs.begin(), semantic_refs.end());
                if (std::adjacent_find(semantic_refs.begin(), semantic_refs.end()) !=
                    semantic_refs.end())
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
        } catch (const Standard_Failure& error) {
            throw std::invalid_argument("FEATURE_FAILED[" + spec.feature_id +
                                        "]: " + error.GetMessageString());
        } catch (const std::invalid_argument& error) {
            throw std::invalid_argument("FEATURE_FAILED[" + spec.feature_id + "]: " + error.what());
        } catch (const std::exception& error) {
            throw std::runtime_error("FEATURE_FAILED[" + spec.feature_id + "]: " + error.what());
        }
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
    TopTools_IndexedMapOfShape face_map;
    TopExp::MapShapes(shape, TopAbs_FACE, face_map);
    for (int face_id = 1; face_id <= face_map.Extent(); ++face_id) {
        const TopoDS_Face& face = TopoDS::Face(face_map(face_id));
        TopLoc_Location location;
        const Handle(Poly_Triangulation) triangulation = BRep_Tool::Triangulation(face, location);
        if (triangulation.IsNull()) {
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

#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BOPAlgo_CellsBuilder.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <BRepBuilderAPI_Transform.hxx>
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
#include <IMeshData_Status.hxx>
#include <BRepOffsetAPI_DraftAngle.hxx>
#include <BRepOffsetAPI_MakeThickSolid.hxx>
#include <BRepOffsetAPI_ThruSections.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeHalfSpace.hxx>
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
#include <Poly_PolygonOnTriangulation.hxx>
#include <GCPnts_TangentialDeflection.hxx>
#include <occccad/kernel/mesh_glb.hpp>
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
#include <TopoDS_Shell.hxx>
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
#include <internal/fillet_boundary.hpp>
#include <occccad/kernel/geometry_id.hpp>
#include <occccad/kernel/topology_naming.hpp>

#include <algorithm>
#include <atomic>
#include <chrono>
#include <cmath>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <iostream>
#include <optional>
#include <sstream>
#include <stdexcept>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <set>
#include <map>
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

// A positive-weight spline lies in its pole convex hull. Collinear, ordered
// poles certify a straight, non-backtracking segment over the entire trimmed
// interval; display tessellation or a few sampled points cannot certify this.
std::optional<gp_Lin> analytic_line(const TopoDS_Edge& edge) {
    BRepAdaptor_Curve curve(edge);
    if (curve.GetType() == GeomAbs_Line)
        return curve.Line();
    if (curve.GetType() != GeomAbs_BSplineCurve ||
        !std::isfinite(curve.FirstParameter()) || !std::isfinite(curve.LastParameter()))
        return std::nullopt;
    auto spline = Handle(Geom_BSplineCurve)::DownCast(curve.BSpline()->Copy());
    if (spline->IsPeriodic())
        return std::nullopt;
    spline->Segment(curve.FirstParameter(), curve.LastParameter());
    const auto start = spline->StartPoint(), end = spline->EndPoint();
    const double length = start.Distance(end);
    const double tolerance = Precision::Confusion();
    if (length <= tolerance)
        return std::nullopt;
    const gp_Lin line(start, gp_Dir(gp_Vec(start, end)));
    double previous = -tolerance;
    for (int i = 1; i <= spline->NbPoles(); ++i) {
        const auto pole = spline->Pole(i);
        const double parameter = gp_Vec(start, pole).Dot(gp_Vec(line.Direction()));
        if (spline->Weight(i) <= 0 || line.Distance(pole) > tolerance ||
            parameter < previous - tolerance || parameter < -tolerance ||
            parameter > length + tolerance)
            return std::nullopt;
        previous = parameter;
    }
    return line;
}

int classify_curve(const TopoDS_Edge& edge) {
    if (analytic_line(edge))
        return 0;
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
    if (const auto line = analytic_line(edge); line && curve.GetType() != GeomAbs_Line) {
        output.properties.push_back(text_property("underlyingCurveType", "BSPLINE_CURVE"));
        output.properties.push_back(vector_property("origin", line->Location().XYZ()));
        output.properties.push_back(vector_property("direction", line->Direction().XYZ()));
    }
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
        // A composed history may remove an intermediate tool occurrence while
        // the exact source TShape survives in another Boolean argument. Final
        // membership is authoritative; never rename that survivor via closure.
        if (contains(source.shape)) {
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
        if (contains(source.shape))
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
            const auto first_named = result.named.size();
            name_sweep(revolve);
            // Full turns merge seams; points/edges on the rotation axis can also
            // disappear. OCCT returns helper shapes for these source vertices.
            // Retain only actual generated-region members, including vertex refs
            // whose source IDs are endpoint IDs rather than the region ID.
            TopTools_IndexedMapOfShape members;
            TopExp::MapShapes(generated, members);
            result.named.erase(std::remove_if(result.named.begin() + first_named, result.named.end(),
                                              [&](const auto& named) {
                                                  return !members.Contains(named.shape);
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

ToolBuild make_pattern_tool(const ToolBuild& source, const ProfilePadSpec& spec) {
    if (spec.pattern_placements.empty() || spec.pattern_placements.size() > 256)
        throw std::invalid_argument("PATTERN_MEMBER_BUDGET");
    if (source.named.size() > 100000 / spec.pattern_placements.size())
        throw std::invalid_argument("PATTERN_TOPOLOGY_BUDGET");
    if (!spec.pattern_result_mode.empty() && spec.pattern_result_mode!="COMBINE" && spec.pattern_result_mode!="INDEPENDENT")
        throw std::invalid_argument("PATTERN_RESULT_MODE_INVALID");
    if (spec.pattern_result_mode=="INDEPENDENT" && spec.body_operation!="ADD")
        throw std::invalid_argument("PATTERN_INDEPENDENT_REQUIRES_ADDITIVE_SOURCE");
    ToolBuild output;
    output.feature_id = spec.feature_id;
    output.topology_history_complete = source.topology_history_complete;
    BRep_Builder builder;
    TopoDS_Compound compound;
    builder.MakeCompound(compound);
    std::set<std::uint32_t> slots;
    bool seed = false;
    for (const auto& member : spec.pattern_placements) {
        if (member.slot >= 256 || !slots.insert(member.slot).second)
            throw std::invalid_argument("PATTERN_SLOT_INVALID");
        const auto& m = member.matrix;
        for (const auto value : m)
            if (!std::isfinite(value)) throw std::invalid_argument("PATTERN_TRANSFORM_NON_FINITE");
        for (int a = 0; a < 3; ++a)
            for (int b = 0; b < 3; ++b) {
                double dot = 0;
                for (int c = 0; c < 3; ++c) dot += m[a*4+c]*m[b*4+c];
                if (std::abs(dot - (a == b ? 1.0 : 0.0)) > 1e-10)
                    throw std::invalid_argument("PATTERN_TRANSFORM_NOT_RIGID");
            }
        const double det = m[0]*(m[5]*m[10]-m[6]*m[9])-m[1]*(m[4]*m[10]-m[6]*m[8])+m[2]*(m[4]*m[9]-m[5]*m[8]);
        if (std::abs(std::abs(det)-1.0)>1e-10) throw std::invalid_argument("PATTERN_TRANSFORM_NOT_ISOMETRIC");
        if (member.slot == 0) {
            for (int i=0;i<12;++i)
                if (std::abs(m[i] - (i==0 || i==5 || i==10 ? 1.0 : 0.0))>1e-10)
                    throw std::invalid_argument("PATTERN_SEED_TRANSFORM_UNSUPPORTED");
            seed = true;
            continue; // The accepted upstream stage already contains the seed.
        }
        gp_Trsf transform;
        transform.SetValues(m[0],m[1],m[2],m[3],m[4],m[5],m[6],m[7],m[8],m[9],m[10],m[11]);
        BRepBuilderAPI_Transform algorithm(source.shape, transform, Standard_True);
        if (!algorithm.IsDone()) throw std::runtime_error("PATTERN_TRANSFORM_FAILED");
        builder.Add(compound, algorithm.Shape());
        for (const auto& named : source.named) {
            const auto shape = algorithm.ModifiedShape(named.shape);
            if (shape.IsNull()) throw std::runtime_error("PATTERN_TRANSFORM_HISTORY_MISSING");
            SemanticTopologyRef ref{spec.feature_id,
                "MEMBER/"+std::to_string(member.slot)+"/"+make_geometry_id(ref_key(named.ref)),
                named.ref.source_ids};
            append_generated(output,named.ref,ref,shape);
        }
    }
    if (!seed) throw std::invalid_argument("PATTERN_SEED_SLOT_REQUIRED");
    if (slots.size()>1) output.shape=compound;
    return output;
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
    if (analytic_line(edge))
        return "LINE";
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
    if (const auto line = analytic_line(edge)) {
        evidence.origin = to_vec3(line->Location());
        const auto direction = line->Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
        // Projection consumes line distances, never native spline parameters.
        evidence.parameter_start = gp_Vec(line->Location(), curve.Value(curve.FirstParameter()))
                                       .Dot(gp_Vec(direction));
        evidence.parameter_end = gp_Vec(line->Location(), curve.Value(curve.LastParameter()))
                                     .Dot(gp_Vec(direction));
    } else if (curve.GetType() == GeomAbs_Circle) {
        evidence.origin = to_vec3(curve.Circle().Location());
        const auto direction = curve.Circle().Axis().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
        evidence.radius_mm = curve.Circle().Radius();
        const auto x = curve.Circle().XAxis().Direction();
        evidence.x_direction = Vec3{x.X(), x.Y(), x.Z()};
    } else if (curve.GetType() == GeomAbs_Ellipse) {
        const auto ellipse = curve.Ellipse();
        evidence.origin = to_vec3(ellipse.Location());
        const auto direction = ellipse.Axis().Direction();
        evidence.direction = {direction.X(), direction.Y(), direction.Z()};
        evidence.radius_mm = ellipse.MajorRadius();
        evidence.minor_radius_mm = ellipse.MinorRadius();
        const auto x = ellipse.XAxis().Direction();
        evidence.x_direction = Vec3{x.X(), x.Y(), x.Z()};
    }
    std::ostringstream canonical;
    canonical.precision(17);
    canonical << evidence.geometry_type << '|' << *evidence.measure_si << '|'
              << evidence.centroid.x << ',' << evidence.centroid.y << ','
              << evidence.centroid.z << '|' << evidence.origin.x << ',' << evidence.origin.y
              << ',' << evidence.origin.z << '|' << evidence.direction.x << ','
              << evidence.direction.y << ',' << evidence.direction.z << '|'
              << evidence.parameter_start.value_or(curve.FirstParameter()) << ','
              << evidence.parameter_end.value_or(curve.LastParameter());
    if (evidence.radius_mm) canonical << "|radius_mm=" << *evidence.radius_mm;
    if (evidence.minor_radius_mm) canonical << "|minor_radius_mm=" << *evidence.minor_radius_mm;
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

// Correspondence operates on copies. Candidate order is domain-ID order, never
// OCCT traversal order. Saved/manual choices are singleton candidate sets.
std::vector<ProfileCurveSpec> loft_curves(const LoftSectionSpec& section) {
    auto curves = section.region.outer.curves;
    if (section.reversed) {
        std::reverse(curves.begin(), curves.end());
        for (auto& curve : curves)
            curve.reversed = !curve.reversed;
    }
    if (!section.seam_entity_id.empty()) {
        const auto seam = std::find_if(curves.begin(), curves.end(), [&](const auto& c) {
            return c.entity_id == section.seam_entity_id;
        });
        if (seam == curves.end())
            throw std::invalid_argument("LOFT_CORRESPONDENCE_LOST:SECTION_SEAM_MISSING");
        std::rotate(curves.begin(), seam, curves.end());
    }
    return curves;
}
ProfileFrame loft_frame(const LoftSectionSpec& section) {
    ProfilePadSpec plane;
    plane.plane_origin = section.origin;
    plane.plane_normal = section.normal;
    plane.plane_u_direction = section.u_direction;
    return profile_frame(plane);
}
bool loft_circle(const LoftSectionSpec& section) {
    const auto& c = section.region.outer.curves;
    return c.size() == 1 && c.front().kind == "CIRCLE";
}
std::size_t loft_boundary_count(const std::vector<LoftSectionSpec>& sections) {
    if (sections.size() < 2 || sections.size() > 32)
        throw std::invalid_argument("INVALID_LOFT_SECTION_COUNT");
    std::size_t count = 0, profiles = 0;
    for (std::size_t i = 0; i < sections.size(); ++i) {
        const auto& s = sections[i];
        if (!s.point_id.empty()) {
            if (i != 0 && i + 1 != sections.size())
                throw std::invalid_argument("LOFT_POINT_MUST_BE_ENDPOINT");
            if (!std::isfinite(s.point.x) || !std::isfinite(s.point.y) || !std::isfinite(s.point.z))
                throw std::invalid_argument("INVALID_LOFT_POINT");
            continue;
        }
        if (!std::isfinite(s.seam_angle))
            throw std::invalid_argument("INVALID_LOFT_SEAM_ANGLE");
        ++profiles;
        const auto n = s.region.outer.curves.size();
        if (!n || !s.region.holes.empty())
            throw std::invalid_argument("LOFT_REQUIRES_SINGLE_CLOSED_SECTION");
        if (n > 64)
            throw std::invalid_argument("LOFT_CORRESPONDENCE_RESOURCE_LIMIT");
        if (!loft_circle(s)) {
            if (count && count != n)
                throw std::invalid_argument("UNSUPPORTED_UNLIKE_LOFT_SECTIONS");
            count = n;
        }
    }
    if (!profiles)
        throw std::invalid_argument("LOFT_REQUIRES_PROFILE_SECTION");
    return count ? count : 1;
}
// Circles are partitioned analytically at the accepted phase. These arcs share
// their real source edge and a stable boundary anchor from the polygon section.
std::vector<TopoDS_Edge> loft_edges(const LoftSectionSpec& section, std::size_t count) {
    const auto frame = loft_frame(section);
    const auto curves = loft_curves(section);
    std::vector<TopoDS_Edge> result;
    if (loft_circle(section)) {
        const auto& c = curves.front();
        Handle(Geom_Circle) circle = new Geom_Circle(profile_axes(frame, c.center), c.radius);
        const double direction = c.reversed ? -1 : 1;
        for (std::size_t i = 0; i < count; ++i) {
            const double a = section.seam_angle + direction * 2 * M_PI * i / count;
            const double b = section.seam_angle + direction * 2 * M_PI * (i + 1) / count;
            TopoDS_Edge edge = BRepBuilderAPI_MakeEdge(circle, std::min(a, b), std::max(a, b));
            if (direction < 0)
                edge.Reverse();
            result.push_back(edge);
        }
    } else {
        for (const auto& c : curves)
            result.push_back(make_profile_edge(c, frame));
    }
    return result;
}
gp_Pnt loft_section_center(const LoftSectionSpec& section) {
    BRepBuilderAPI_MakeWire wire;
    for (const auto& edge : loft_edges(section, loft_circle(section) ? 1 : section.region.outer.curves.size()))
        wire.Add(edge);
    if (!wire.IsDone())
        throw std::invalid_argument("INVALID_LOFT_WIRE");
    GProp_GProps properties;
    BRepGProp::LinearProperties(wire.Wire(), properties);
    return properties.CentreOfMass();
}
gp_Quaternion loft_transport(const LoftSectionSpec& a, const LoftSectionSpec& b) {
    auto na = loft_frame(a).normal, nb = loft_frame(b).normal;
    // Support normals are unoriented. Orient them consistently along the actual
    // section-to-section displacement, rather than choosing an arbitrary quarter
    // turn for perpendicular supports. Sketch origins need not be profile centers.
    const gp_Vec travel(loft_section_center(a), loft_section_center(b));
    const double pa = travel.Dot(gp_Vec(na)), pb = travel.Dot(gp_Vec(nb));
    const double tolerance = std::max(Precision::Confusion(), travel.Magnitude() * 1e-8);
    if (std::abs(pa) > tolerance && std::abs(pb) > tolerance) {
        if (pa * pb < 0)
            nb.Reverse();
    } else if (na.Dot(nb) < -1e-10) {
        nb.Reverse();
    }
    return gp_Quaternion(na.XYZ(), nb.XYZ());
}
struct LoftCandidate {
    LoftSectionSpec section;
    std::vector<gp_Vec> samples;
};
LoftCandidate loft_candidate(LoftSectionSpec section, std::size_t count) {
    // A manual direction can arrive before a seam pick. Freeze its actual first
    // source edge as well, so reopening never depends on profile traversal order.
    if (section.seam_entity_id.empty())
        section.seam_entity_id = loft_curves(section).front().entity_id;
    LoftCandidate candidate{std::move(section), {}};
    const auto edges = loft_edges(candidate.section, count);
    gp_XYZ center(0, 0, 0);
    std::vector<gp_Pnt> samples;
    for (const auto& edge : edges) {
        BRepAdaptor_Curve curve(edge);
        for (int k = 0; k < 4; ++k) {
            double t = k / 4.0;
            if (edge.Orientation() == TopAbs_REVERSED)
                t = 1 - t;
            const auto p =
                curve.Value(curve.FirstParameter() * (1 - t) + curve.LastParameter() * t);
            samples.push_back(p);
            center += p.XYZ();
        }
    }
    center /= static_cast<double>(samples.size());
    double scale = 0;
    for (const auto& p : samples)
        scale += (p.XYZ() - center).SquareModulus();
    scale = std::sqrt(scale / samples.size());
    if (scale <= Precision::Confusion())
        throw std::invalid_argument("DEGENERATE_LOFT_SECTION");
    for (const auto& p : samples)
        candidate.samples.emplace_back((p.XYZ() - center) / scale);
    return candidate;
}
std::vector<LoftSectionSpec> resolve_loft_sections(const std::vector<LoftSectionSpec>& input) {
    const auto count = loft_boundary_count(input);
    std::vector<LoftSectionSpec> result = input;
    std::vector<std::size_t> indices;
    std::vector<std::vector<LoftCandidate>> candidates;
    for (std::size_t i = 0; i < input.size(); ++i) {
        auto section = input[i];
        if (!section.point_id.empty())
            continue;
        loft_curves(section);  // Validate every saved edge, including single-edge circles.
        indices.push_back(i);
        std::vector<LoftCandidate> choices;
        if (section.correspondence_resolved ||
            (!section.seam_entity_id.empty() && !loft_circle(section)) || section.reversed ||
            section.seam_angle != 0) {
            choices.push_back(loft_candidate(section, count));
        } else if (loft_circle(section)) {
            // Include exact projected polygon vertices, not only an angular grid.
            std::vector<double> phases{0};
            const auto frame = loft_frame(section);
            for (const auto& other : input) {
                if (!other.point_id.empty())
                    continue;
                const auto rotation = loft_transport(other, section);
                if (loft_circle(other)) {
                    const auto f = loft_frame(other);
                    const auto v = rotation * (gp_Vec(f.u_direction) * std::cos(other.seam_angle) +
                                               gp_Vec(f.v_direction) * std::sin(other.seam_angle));
                    phases.push_back(std::atan2(v.Dot(gp_Vec(frame.v_direction)),
                                                v.Dot(gp_Vec(frame.u_direction))));
                    continue;
                }
                const auto sample = loft_candidate(other, count);
                for (std::size_t k = 0; k < sample.samples.size(); k += 4) {
                    const auto v = rotation * sample.samples[k].XYZ();
                    phases.push_back(
                        std::atan2(v.Dot(frame.v_direction.XYZ()), v.Dot(frame.u_direction.XYZ())));
                }
            }
            for (int k = 1; k < 16; ++k)
                phases.push_back(2 * M_PI * k / 16);
            std::sort(phases.begin(), phases.end());
            phases.erase(std::unique(phases.begin(), phases.end(),
                                     [](double a, double b) { return std::abs(a - b) < 1e-10; }),
                         phases.end());
            if (phases.size() * count * 8 > 65536)
                throw std::invalid_argument("LOFT_CORRESPONDENCE_RESOURCE_LIMIT");
            for (bool reverse : {false, true})
                for (double phase : phases) {
                    section.reversed = reverse;
                    section.seam_angle = phase;
                    section.seam_entity_id = section.region.outer.curves.front().entity_id;
                    choices.push_back(loft_candidate(section, count));
                }
        } else {
            std::vector<std::string> ids;
            for (const auto& c : section.region.outer.curves)
                ids.push_back(c.entity_id);
            std::sort(ids.begin(), ids.end());
            for (bool reverse : {false, true})
                for (const auto& id : ids) {
                    section.reversed = reverse;
                    section.seam_entity_id = id;
                    choices.push_back(loft_candidate(section, count));
                }
        }
        candidates.push_back(std::move(choices));
    }
    // Global shortest path across all section candidate sets. Rotation transports
    // the comparison vectors only; the actual section frames never change.
    std::vector<std::vector<double>> cost(candidates.size());
    std::vector<std::vector<std::size_t>> parent(candidates.size());
    std::size_t comparisonBudget = 0;
    for (std::size_t i = 1; i < candidates.size(); ++i)
        comparisonBudget += candidates[i].size() * candidates[i - 1].size() * count * 4;
    if (comparisonBudget > 100000000)
        throw std::invalid_argument("LOFT_CORRESPONDENCE_RESOURCE_LIMIT");
    for (std::size_t i = 0; i < candidates.size(); ++i) {
        cost[i].assign(candidates[i].size(), std::numeric_limits<double>::infinity());
        parent[i].resize(candidates[i].size());
        if (!i) {
            std::fill(cost[i].begin(), cost[i].end(), 0);
            continue;
        }
        const auto rotation = loft_transport(input[indices[i - 1]], input[indices[i]]);
        for (std::size_t b = 0; b < candidates[i].size(); ++b)
            for (std::size_t a = 0; a < candidates[i - 1].size(); ++a) {
                double transition = 0;
                const auto& av = candidates[i - 1][a].samples;
                const auto& bv = candidates[i][b].samples;
                for (std::size_t k = 0; k < av.size(); ++k)
                    transition += (rotation * av[k].XYZ() - bv[k].XYZ()).SquareMagnitude();
                const double value = cost[i - 1][a] + transition / av.size();
                if (value + 1e-12 < cost[i][b]) {
                    cost[i][b] = value;
                    parent[i][b] = a;
                }
            }
    }
    auto selected = static_cast<std::size_t>(
        std::min_element(cost.back().begin(), cost.back().end()) - cost.back().begin());
    for (std::size_t i = candidates.size(); i-- > 0;) {
        result[indices[i]] = candidates[i][selected].section;
        result[indices[i]].correspondence_resolved = true;
        selected = parent[i][selected];
    }
    return result;
}

ToolBuild make_loft_tool(const ProfilePadSpec& spec) {
    const auto sections = resolve_loft_sections(spec.sections);
    const auto count = loft_boundary_count(sections);
    BRepOffsetAPI_ThruSections algorithm(Standard_True, spec.ruled, 1e-7);
    algorithm.CheckCompatibility(Standard_False);
    // All wires/vertices below are newly constructed and privately owned. OCCT
    // may share their topology, but cannot mutate an upstream BREP or sketch.
    algorithm.SetMutableInput(Standard_True);
    std::vector<std::string> partitions;
    for (const auto& s : sections)
        if (s.point_id.empty() && !loft_circle(s)) {
            for (const auto& c : loft_curves(s))
                partitions.push_back(s.sketch_id + "/" + c.entity_id);
            break;
        }
    struct Boundary {
        SemanticTopologyRef source;
        TopoDS_Edge edge;
        std::string partition;
    };
    std::vector<Boundary> edges;
    std::vector<TopoDS_Vertex> tips(sections.size());
    for (std::size_t i = 0; i < sections.size(); ++i) {
        const auto& section = sections[i];
        if (!section.point_id.empty()) {
            tips[i] = BRepBuilderAPI_MakeVertex(
                gp_Pnt(section.point.x, section.point.y, section.point.z));
            algorithm.AddVertex(tips[i]);
            continue;
        }
        const auto curves = loft_curves(section);
        const auto boundary = loft_edges(section, count);
        BRepBuilderAPI_MakeWire wire;
        for (std::size_t j = 0; j < boundary.size(); ++j) {
            wire.Add(boundary[j]);
            if (!wire.IsDone())
                throw std::invalid_argument("INVALID_LOFT_WIRE");
            const auto& curve = curves[loft_circle(section) ? 0 : j];
            edges.push_back({{section.sketch_id,
                              "PROFILE_EDGE/" + curve.entity_id,
                              {section.region.id, curve.entity_id}},
                             wire.Edge(),
                             loft_circle(section) && count > 1 ? partitions[j] : ""});
        }
        if (!wire.Wire().Closed())
            throw std::invalid_argument("OPEN_LOFT_SECTION");
        algorithm.AddWire(wire.Wire());
    }
    algorithm.Build();
    if (!algorithm.IsDone())
        throw std::runtime_error(
            "LOFT_ALGORITHM_FAILED:CHECK_CORRESPONDENCE_OR_ADD_INTERMEDIATE_SECTION");
    ToolBuild tool;
    tool.feature_id = spec.feature_id;
    tool.shape = algorithm.Shape();
    try {
        validate_body_solid_set(tool.shape);
    } catch (const std::exception& e) {
        throw std::runtime_error(std::string("LOFT_INVALID_SOLID:") + e.what() +
                                 ":ADD_INTERMEDIATE_SECTION");
    }
    BRepAlgoAPI_Check check(tool.shape, Standard_False, Standard_True);
    if (!check.IsValid())
        throw std::runtime_error(
            "LOFT_SELF_INTERFERENCE:CHECK_CORRESPONDENCE_OR_ADD_INTERMEDIATE_SECTION");
    for (const auto i : {std::size_t(0), sections.size() - 1}) {
        const auto& s = sections[i];
        const std::string end = i == 0 ? "START" : "END";
        if (!s.point_id.empty()) {
            append_generated(
                tool, {s.sketch_id, "PROFILE_POINT/" + s.point_id, {s.point_id}},
                {spec.feature_id, "LOFT_" + end + "_VERTEX", {s.sketch_id, s.point_id}}, tips[i]);
        } else {
            append_generated(tool, {s.sketch_id, "PROFILE_REGION/" + s.region.id, {s.region.id}},
                             {spec.feature_id, "LOFT_" + end + "_CAP", {s.sketch_id}},
                             i == 0 ? algorithm.FirstShape() : algorithm.LastShape());
        }
    }
    for (const auto& boundary : edges) {
        const auto generated = algorithm.Generated(boundary.edge);
        for (TopTools_ListIteratorOfListOfShape it(generated); it.More(); it.Next()) {
            if (it.Value().ShapeType() != TopAbs_FACE)
                continue;
            const auto& source = boundary.source;
            const auto key = make_geometry_id(ref_key(source) + "/" + boundary.partition).substr(7);
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
                                         const std::string& requested_operation, bool diagnose_loft = false) {
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
    const auto failed = [&](const char* diagnostic) {
        if (diagnose_loft) {
            BRepAlgoAPI_Check target(input, Standard_False, Standard_True);
            if (!target.IsValid()) throw std::runtime_error("LOFT_BOOLEAN_INVALID_TARGET");
            throw std::runtime_error("LOFT_BOOLEAN_ALGORITHM_FAILED:" + operation + ":" + diagnostic);
        }
        throw std::runtime_error(diagnostic);
    };
    if (operation == "ADD") {
        BRepAlgoAPI_Fuse algorithm;
        algorithm.SetNonDestructive(Standard_True);
        TopTools_ListOfShape arguments,tools;arguments.Append(input);tools.Append(tool.shape);
        algorithm.SetArguments(arguments);algorithm.SetTools(tools);algorithm.Build();
        if (!algorithm.IsDone())
            failed("body fuse failed");
        result = algorithm.Shape();
        mapped = map_named_shapes(sources, algorithm, result);
        if (shape_volume(result) - shape_volume(input) <= 1.0e-9)
            throw std::invalid_argument("NO_MATERIAL_CHANGE: add does not change the target body");
    } else if (operation == "REMOVE") {
        BRepAlgoAPI_Cut algorithm;
        algorithm.SetNonDestructive(Standard_True);
        TopTools_ListOfShape arguments,tools;arguments.Append(input);tools.Append(tool.shape);
        algorithm.SetArguments(arguments);algorithm.SetTools(tools);algorithm.Build();
        if (!algorithm.IsDone())
            failed("body cut failed");
        result = algorithm.Shape();
        mapped = map_named_shapes(sources, algorithm, result);
        if (shape_volume(input) - shape_volume(result) <= 1.0e-9)
            throw std::invalid_argument(
                "NO_MATERIAL_CHANGE: cut does not intersect the target body");
    } else if (operation == "INTERSECT") {
        BRepAlgoAPI_Common algorithm;
        algorithm.SetNonDestructive(Standard_True);
        TopTools_ListOfShape arguments,tools;arguments.Append(input);tools.Append(tool.shape);
        algorithm.SetArguments(arguments);algorithm.SetTools(tools);algorithm.Build();
        if (!algorithm.IsDone())
            failed("body common failed");
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
    unifier.SetSafeInputMode(Standard_True);
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

// Expose a composed OCCT history through the same modifier naming gate.
struct ComposedModifier {
    TopoDS_Shape shape;
    Handle(BRepTools_History) history;
    bool IsDone() const { return !shape.IsNull(); }
    const TopoDS_Shape& Shape() const { return shape; }
    const TopTools_ListOfShape& Modified(const TopoDS_Shape& source) { return history->Modified(source); }
    const TopTools_ListOfShape& Generated(const TopoDS_Shape& source) { return history->Generated(source); }
    bool IsDeleted(const TopoDS_Shape& source) { return history->IsRemoved(source); }
};

// Two orthogonal edge rounds separated by a planar bevel share one support
// face. Construct both constant-radius cylinders from the ORIGINAL supports,
// then trim their corner wedges against the body. This is the same mitered
// intersection as rounding the sharp corner and clipping it by the bevel; a
// small bevel can disappear without moving either cylinder or changing radius.
// Return an empty result outside this deliberately narrow analytic case.
ComposedModifier planar_bevel_corner_fillets(const TopoDS_Shape& input,
                                             const std::vector<TopoDS_Shape>& selected,
                                             double radius) {
    if (selected.size() != 2 || solid_count(input) != 1) return {};
    TopTools_IndexedDataMapOfShapeListOfShape edgeFaces;
    TopExp::MapShapesAndAncestors(input, TopAbs_EDGE, TopAbs_FACE, edgeFaces);
    TopoDS_Shape shared;
    for (TopTools_ListIteratorOfListOfShape a(edgeFaces.FindFromKey(selected[0])); a.More(); a.Next())
        for (TopTools_ListIteratorOfListOfShape b(edgeFaces.FindFromKey(selected[1])); b.More(); b.Next())
            if (a.Value().IsSame(b.Value())) shared = a.Value();
    if (shared.IsNull()) return {};
    TopTools_IndexedMapOfShape endpoints[2];
    for (int i = 0; i < 2; ++i) TopExp::MapShapes(selected[i], TopAbs_VERTEX, endpoints[i]);
    for (int i = 1; i <= endpoints[0].Extent(); ++i)
        if (endpoints[1].Contains(endpoints[0](i))) return {};
    bool bridge = false;
    for (TopExp_Explorer edges(shared, TopAbs_EDGE); edges.More(); edges.Next()) {
        bool connects[2] = {false, false};
        for (TopExp_Explorer vertices(edges.Current(), TopAbs_VERTEX); vertices.More(); vertices.Next())
            for (int i = 0; i < 2; ++i) connects[i] |= endpoints[i].Contains(vertices.Current());
        if (connects[0] && connects[1]) bridge = true;
    }
    if (!bridge) return {};
    BRepAdaptor_Curve curves[2] = {BRepAdaptor_Curve(TopoDS::Edge(selected[0])),
                                 BRepAdaptor_Curve(TopoDS::Edge(selected[1]))};
    if (curves[0].GetType() != GeomAbs_Line || curves[1].GetType() != GeomAbs_Line ||
        std::abs(curves[0].Line().Direction().Dot(curves[1].Line().Direction())) > 1e-8) return {};
    TopTools_IndexedMapOfShape faces, vertices;
    TopExp::MapShapes(input, TopAbs_FACE, faces);
    TopExp::MapShapes(input, TopAbs_VERTEX, vertices);
    std::vector<gp_Dir> outward;
    for (int i = 1; i <= faces.Extent(); ++i) {
        BRepAdaptor_Surface surface(TopoDS::Face(faces(i)));
        if (surface.GetType() != GeomAbs_Plane) return {};
        const auto plane = surface.Plane();
        auto normal = oriented_plane_normal(TopoDS::Face(faces(i)), plane);
        double low = 0, high = 0;
        for (int v = 1; v <= vertices.Extent(); ++v) {
            const double distance = gp_Vec(plane.Location(), BRep_Tool::Pnt(TopoDS::Vertex(vertices(v)))).Dot(gp_Vec(normal));
            low = std::min(low, distance); high = std::max(high, distance);
        }
        if (low < -1e-7 && high > 1e-7) return {};
        if (high > 1e-7) normal.Reverse();
        outward.push_back(normal);
    }
    struct Support { gp_Pnt point; gp_Vec axis, first, second; double low, high; };
    std::vector<Support> supports;
    for (int i = 0; i < 2; ++i) {
        const auto& adjacent = edgeFaces.FindFromKey(selected[i]);
        if (adjacent.Extent() != 2) return {};
        const gp_Vec first(outward[faces.FindIndex(adjacent.First())-1]);
        const gp_Vec second(outward[faces.FindIndex(adjacent.Last())-1]);
        if (std::abs(first.Dot(second)) > 1e-8) return {};
        const auto point = curves[i].Line().Location();
        const gp_Vec axis(curves[i].Line().Direction());
        double low = 0, high = 0, firstDepth = 0, secondDepth = 0;
        for (int v = 1; v <= vertices.Extent(); ++v) {
            const gp_Vec delta(point, BRep_Tool::Pnt(TopoDS::Vertex(vertices(v))));
            low = std::min(low, delta.Dot(axis)); high = std::max(high, delta.Dot(axis));
            firstDepth = std::max(firstDepth, -delta.Dot(first));
            secondDepth = std::max(secondDepth, -delta.Dot(second));
        }
        if (radius >= std::min(firstDepth, secondDepth) - Precision::Confusion())
            throw std::invalid_argument("FILLET_RADIUS_EXCEEDS_SUPPORT");
        supports.push_back({point, axis, first, second, low-radius, high+radius});
    }
    ComposedModifier result{input, new BRepTools_History()};
    std::vector<TopoDS_Shape> cutters;
    for (int i = 0; i < 2; ++i) {
        const auto& support = supports[i];
        const auto corner = support.point.Translated(support.axis * support.low);
        const auto center = corner.Translated((support.first + support.second) * -radius);
        const auto travel = support.axis * (support.high-support.low);
        BRepBuilderAPI_MakePolygon polygon;
        polygon.Add(corner);
        polygon.Add(corner.Translated(support.first * -radius));
        polygon.Add(center);
        polygon.Add(corner.Translated(support.second * -radius));
        polygon.Close();
        const auto prism = BRepPrimAPI_MakePrism(BRepBuilderAPI_MakeFace(polygon.Wire()).Face(), travel).Shape();
        // Keep the periodic surface seam opposite the retained quarter arc.
        // A world-default X direction would split oblique rounds into extra
        // faces and expose an artificial edge in the viewport.
        const gp_Dir seam((support.first + support.second) * -1);
        BRepPrimAPI_MakeCylinder cylinder(gp_Ax2(center, gp_Dir(support.axis), seam), radius, travel.Magnitude());
        // Primitive construction is exact source evidence, followed by the
        // actual Boolean history; no nearest-face/edge recovery is involved.
        result.history->AddGenerated(selected[i], cylinder.Face());
        TopTools_ListOfShape objects, tools;
        objects.Append(prism); tools.Append(cylinder.Shape());
        BRepAlgoAPI_Cut wedge;
        wedge.SetArguments(objects); wedge.SetTools(tools);
        wedge.SetNonDestructive(Standard_True);
        wedge.Build();
        if (!wedge.IsDone()) throw std::runtime_error("FILLET_CORNER_TOOL_FAILED");
        TopTools_ListOfShape arguments;
        arguments.Append(prism); arguments.Append(cylinder.Shape());
        result.history->Merge(arguments, wedge);
        cutters.push_back(wedge.Shape());
    }
    for (const auto& cutter : cutters) {
        TopTools_ListOfShape objects, tools;
        objects.Append(result.shape); tools.Append(cutter);
        BRepAlgoAPI_Cut cut;
        cut.SetArguments(objects); cut.SetTools(tools);
        cut.SetNonDestructive(Standard_True);
        cut.Build();
        if (!cut.IsDone()) throw std::runtime_error("FILLET_CORNER_TRIM_FAILED");
        TopTools_ListOfShape arguments;
        arguments.Append(result.shape); arguments.Append(cutter);
        result.history->Merge(arguments, cut);
        result.shape = cut.Shape();
    }
    validate_body_solid_set(result.shape);
    return result;
}

// Offsetting must not enlarge the upstream uncertainty to hide C0 gaps.
// Boolean/fillet inputs can already carry larger tolerances than the usual
// 1e-5 mm floor, so preserve that measured baseline rather than rejecting it.
double shell_input_tolerance(const TopoDS_Shape& shape) {
    double tolerance = 0;
    for (TopExp_Explorer vertices(shape, TopAbs_VERTEX); vertices.More(); vertices.Next())
        tolerance = std::max(tolerance, BRep_Tool::Tolerance(TopoDS::Vertex(vertices.Current())));
    for (TopExp_Explorer edges(shape, TopAbs_EDGE); edges.More(); edges.Next())
        tolerance = std::max(tolerance, BRep_Tool::Tolerance(TopoDS::Edge(edges.Current())));
    return tolerance;
}

void validate_shell_offset(const TopoDS_Shape& shape, double toleranceLimit) {
    validate_body_solid_set(shape);
    if (shell_input_tolerance(shape) > toleranceLimit)
        throw std::runtime_error("SHELL_OFFSET_TOLERANCE_EXCEEDED");
}

template <typename Algorithm>
Handle(BRepTools_History) shell_offset_history(const TopoDS_Shape& input, Algorithm& algorithm) {
    TopTools_ListOfShape arguments; arguments.Append(input);
    Handle(BRepTools_History) history = new BRepTools_History(arguments, algorithm);
    TopTools_IndexedMapOfShape sources, members;
    TopExp::MapShapes(input, sources); TopExp::MapShapes(algorithm.Shape(), members);
    for (int i = 1; i <= sources.Extent(); ++i) {
        const auto& source = sources(i);
        if (source.ShapeType() != TopAbs_FACE && source.ShapeType() != TopAbs_EDGE && source.ShapeType() != TopAbs_VERTEX) continue;
        // A thickening keeps the outer source while also returning its inner
        // image as Modified. Both real images must survive history composition.
        if (members.Contains(source) && !history->Modified(source).IsEmpty() && !history->Modified(source).Contains(source))
            history->AddModified(source, source);
    }
    return history;
}

ComposedModifier simple_open_shell(const TopoDS_Shape& input,
                                  const std::vector<TopoDS_Shape>& removed, double thickness, double toleranceLimit) {
    TopoDS_Shell open;
    BRep_Builder builder;
    builder.MakeShell(open);
    for (TopExp_Explorer faces(input, TopAbs_FACE); faces.More(); faces.Next()) {
        if (std::none_of(removed.begin(), removed.end(), [&](const auto& r) { return r.IsSame(faces.Current()); }))
            builder.Add(open, faces.Current());
    }
    BRepOffsetAPI_MakeThickSolid algorithm;
    algorithm.MakeThickSolidBySimple(open, -thickness);
    if (!algorithm.IsDone()) throw std::runtime_error("SHELL_SIMPLE_OFFSET_FAILED");
    validate_shell_offset(algorithm.Shape(), toleranceLimit);
    return {algorithm.Shape(), shell_offset_history(open, algorithm)};
}

// Mixed sharp/tangent connectivity (e.g. rectangle-to-circle lofts) is rejected
// by OCCT's whole-shell join algorithm. Offset each retained face independently,
// partition those walls together with the original body once, then assemble
// only wall cells inside the body. This trims real offset surfaces; it never closes
// gaps by increasing sewing tolerances or substitutes scaled profile sections.
ComposedModifier inward_face_shell(const TopoDS_Shape& input,
                                  const std::vector<TopoDS_Shape>& removed, double thickness, double toleranceLimit) {
    const auto started = std::chrono::steady_clock::now();
    auto stage = started;
    const auto measured = [&](const char* name) {
        const auto now = std::chrono::steady_clock::now();
        std::clog << "shell stage=" << name << " duration_ms=" << std::chrono::duration<double,std::milli>(now-stage).count() << std::endl;
        stage = now;
    };
    Handle(BRepTools_History) history = new BRepTools_History();
    TopTools_ListOfShape walls;
    for (TopExp_Explorer faces(input, TopAbs_FACE); faces.More(); faces.Next()) {
        const auto face = TopoDS::Face(faces.Current());
        if (std::any_of(removed.begin(), removed.end(), [&](const auto& r) { return r.IsSame(face); })) continue;
        // Keep each branch independent, including shared boundary vertices.
        BRepOffsetAPI_MakeThickSolid wall;
        BRepBuilderAPI_Copy faceCopy(face, Standard_True, Standard_False);
        wall.MakeThickSolidBySimple(faceCopy.Shape(), -thickness);
        if (!wall.IsDone()) throw std::runtime_error("SHELL_FACE_OFFSET_FAILED");
        validate_shell_offset(wall.Shape(), toleranceLimit);
        TopTools_ListOfShape faceSource; faceSource.Append(face);
        Handle(BRepTools_History) branch = new BRepTools_History(faceSource, faceCopy);
        branch->Merge(shell_offset_history(faceCopy.Shape(), wall));
        TopTools_IndexedMapOfShape sources;
        TopExp::MapShapes(face, sources);
        for (int i = 1; i <= sources.Extent(); ++i) {
            const auto& item = sources(i);
            if (item.ShapeType() != TopAbs_FACE && item.ShapeType() != TopAbs_EDGE && item.ShapeType() != TopAbs_VERTEX) continue;
            // These are parallel branches, not sequential modifications of
            // a shared edge. Accumulate all real images before Boolean merging.
            for (TopTools_ListIteratorOfListOfShape images(branch->Modified(item)); images.More(); images.Next())
                if (!history->Modified(item).Contains(images.Value())) history->AddModified(item, images.Value());
            for (TopTools_ListIteratorOfListOfShape images(branch->Generated(item)); images.More(); images.Next())
                if (!history->Generated(item).Contains(images.Value())) history->AddGenerated(item, images.Value());
        }
        walls.Append(wall.Shape());
    }
    if (walls.IsEmpty()) throw std::invalid_argument("SHELL_NO_RETAINED_FACES");
    measured("wall_offsets");
    // One intersection data set for input and all walls. The previous union
    // followed by Common recomputed expensive offset/BSpline intersections.
    TopTools_ListOfShape arguments = walls;
    arguments.Append(input);
    BOPAlgo_CellsBuilder cells;
    cells.SetArguments(arguments);
    cells.SetNonDestructive(Standard_True);
    cells.SetUseOBB(Standard_True);
    cells.SetRunParallel(Standard_True);
    cells.Perform();
    if (cells.HasErrors()) throw std::runtime_error("SHELL_WALL_PARTITION_FAILED");
    measured("wall_partition");
    const TopTools_ListOfShape avoid;
    for (TopTools_ListIteratorOfListOfShape it(walls); it.More(); it.Next()) {
        TopTools_ListOfShape take; take.Append(input); take.Append(it.Value());
        cells.AddToResult(take, avoid, 1, Standard_False);
    }
    cells.RemoveInternalBoundaries();
    if (cells.HasErrors() || cells.HasWarnings()) throw std::runtime_error("SHELL_WALL_ASSEMBLY_FAILED");
    history->Merge(arguments, cells);
    ShapeUpgrade_UnifySameDomain simplify(cells.Shape(), Standard_True, Standard_True, Standard_False);
    simplify.Build();
    history->Merge(simplify.History());
    measured("wall_assembly");
    return {simplify.Shape(), history};
}

// Exact inward cavity for convex, all-planar solids. Small bevel faces may
// disappear during offset; half-space intersection handles that event without
// adopting OCCT's unchanged-input result. The analytic face offsets and every
// Boolean stage retain their real generated/modified/deleted history.
ComposedModifier convex_planar_shell(const TopoDS_Shape& input,
                                    const std::vector<TopoDS_Shape>& removed,
                                    double thickness) {
    if (solid_count(input) != 1) throw std::invalid_argument("SHELL_PLANAR_FALLBACK_UNSUPPORTED");
    Bnd_Box bounds;
    BRepBndLib::Add(input, bounds);
    Standard_Real xmin, ymin, zmin, xmax, ymax, zmax;
    bounds.Get(xmin, ymin, zmin, xmax, ymax, zmax);
    const double extent = std::max({xmax-xmin, ymax-ymin, zmax-zmin, thickness}) * 2;
    TopoDS_Shape cavity = BRepPrimAPI_MakeBox(gp_Pnt(xmin-extent,ymin-extent,zmin-extent), gp_Pnt(xmax+extent,ymax+extent,zmax+extent)).Shape();
    Handle(BRepTools_History) history = new BRepTools_History();
    for (TopExp_Explorer faces(input, TopAbs_FACE); faces.More(); faces.Next()) {
        const auto face = TopoDS::Face(faces.Current());
        BRepAdaptor_Surface surface(face);
        if (surface.GetType() != GeomAbs_Plane) throw std::invalid_argument("SHELL_PLANAR_FALLBACK_UNSUPPORTED");
        auto plane = surface.Plane();
        auto normal = oriented_plane_normal(face, plane);
        const auto point = plane.Location();
        double minDistance = 0, maxDistance = 0;
        for (TopExp_Explorer vertices(input, TopAbs_VERTEX); vertices.More(); vertices.Next()) {
            const auto distance = gp_Vec(point, BRep_Tool::Pnt(TopoDS::Vertex(vertices.Current()))).Dot(gp_Vec(normal));
            minDistance = std::min(minDistance, distance);
            maxDistance = std::max(maxDistance, distance);
        }
        if (minDistance < -1e-6 && maxDistance > 1e-6) throw std::invalid_argument("SHELL_PLANAR_FALLBACK_NONCONVEX");
        if (maxDistance > 1e-6) normal.Reverse();
        if (std::any_of(removed.begin(), removed.end(), [&](const auto& r) { return r.IsSame(face); })) continue;
        plane.SetLocation(point.Translated(gp_Vec(normal) * -thickness));
        const auto offsetFace = BRepBuilderAPI_MakeFace(plane).Face();
        const auto halfspace = BRepPrimAPI_MakeHalfSpace(offsetFace, plane.Location().Translated(gp_Vec(normal) * -extent)).Solid();
        history->AddGenerated(face, offsetFace);
        BRepAlgoAPI_Common clip(cavity, halfspace);
        clip.Build();
        if (!clip.IsDone() || clip.Shape().IsNull() || solid_count(clip.Shape()) == 0)
            throw std::invalid_argument("SHELL_THICKNESS_EXCEEDS_INTERIOR");
        TopTools_ListOfShape arguments;
        arguments.Append(cavity); arguments.Append(halfspace);
        history->Merge(arguments, clip);
        cavity = clip.Shape();
    }
    BRepAlgoAPI_Cut cut(input, cavity);
    cut.Build();
    if (!cut.IsDone()) throw std::runtime_error("MODIFIER_ALGORITHM_FAILED:SHELL:PLANAR");
    TopTools_ListOfShape arguments;
    arguments.Append(input); arguments.Append(cavity);
    history->Merge(arguments, cut);
    return {cut.Shape(), history};
}

// Local modifiers use the exact upstream shape and semantic references. No
// saved local topology index, geometry search, or nearest-element recovery.
BodyOperationResult apply_local_modifier(const TopoDS_Shape& upstream,
                                         const std::vector<NamedShape>& upstream_named,
                                         const ProfilePadSpec& spec) {
    // Local OCCT builders can adjust their inputs. Immutable stage artifacts
    // must never be changed by preview, failure, or a later modifier.
    BRepBuilderAPI_Copy copy(upstream, Standard_True, Standard_False);
    const auto input = copy.Shape();
    const auto named = map_named_shapes(upstream_named, copy, input);
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
        // A selection is a set; construction and history must not depend on
        // the order in which the user picked its members.
        const auto semantic_key = [&](const TopoDS_Shape& shape) {
            const auto source = std::find_if(named.begin(), named.end(), [&](const auto& item) { return item.shape.IsSame(shape); });
            if (source == named.end()) throw std::runtime_error("FILLET_HISTORY_SOURCE_MISSING");
            return ref_key(source->ref);
        };
        std::sort(selected.begin(), selected.end(), [&](const auto& a, const auto& b) { return semantic_key(a) < semantic_key(b); });
        try {
            auto corner = planar_bevel_corner_fillets(input, selected, spec.pad_length);
            if (corner.IsDone()) return finish(corner);
        } catch (const std::invalid_argument& e) {
            if (std::string(e.what()) != "FILLET_RADIUS_EXCEEDS_SUPPORT") throw;
        }
        BRepBuilderAPI_Copy attempt(input, Standard_True, Standard_False);
        BRepFilletAPI_MakeFillet algorithm(attempt.Shape());
        for (const auto& shape : selected)
            algorithm.Add(spec.pad_length, TopoDS::Edge(attempt.ModifiedShape(shape)));
        const auto started = std::chrono::steady_clock::now();
        algorithm.Build();
        std::clog << "fillet stage=generic done=" << algorithm.IsDone() << " ms="
                  << std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() -
                                                               started)
                         .count()
                  << '\n';
        if (algorithm.IsDone()) {
            ComposedModifier composed{algorithm.Shape(), new BRepTools_History()};
            TopTools_ListOfShape original;
            original.Append(input);
            composed.history->Merge(original, attempt);
            TopTools_ListOfShape copied;
            copied.Append(attempt.Shape());
            composed.history->Merge(copied, algorithm);
            // OCCT history composition drops Generated relations when the
            // intermediate edge is also deleted. Preserve the algorithm's
            // actual generation relation across the isolation copy explicitly.
            for (const auto& source : named) {
                const auto copiedSource = attempt.ModifiedShape(source.shape);
                for (TopTools_ListIteratorOfListOfShape it(algorithm.Generated(copiedSource)); it.More(); it.Next()) {
                    if (!composed.history->Generated(source.shape).Contains(it.Value()))
                        composed.history->AddGenerated(source.shape, it.Value());
                }
            }
            return finish(composed);
        }
        for (int i = 1; i <= algorithm.NbFaultyContours(); ++i)
            std::clog << "fillet stage=generic contour_status="
                      << algorithm.StripeStatus(algorithm.FaultyContour(i)) << '\n';
        auto rebuilt = detail::rebuild_fillet_boundary(input, selected, spec.pad_length);
        if (rebuilt.IsDone()) {
            auto output = finish(rebuilt);
            output.diagnostics = rebuilt.diagnostics;
            return output;
        }
        // Simultaneous corner patches can fail on partially selected chamfered
        // contours. Build the exact requested fillets in deterministic semantic
        // order, transporting each edge only through real OCCT history.
        std::vector<std::pair<std::string, TopoDS_Shape>> ordered;
        for (const auto& shape : selected) {
            const auto source = std::find_if(named.begin(), named.end(), [&](const auto& item) { return item.shape.IsSame(shape); });
            if (source == named.end()) throw std::runtime_error("FILLET_HISTORY_SOURCE_MISSING");
            ordered.emplace_back(ref_key(source->ref), shape);
        }
        std::sort(ordered.begin(), ordered.end(), [](const auto& a, const auto& b) { return a.first < b.first; });
        const auto build_sequential = [&](bool reverse) {
            ComposedModifier sequential{input, new BRepTools_History()};
            std::vector<bool> covered(ordered.size(), false);
            for (size_t position = 0; position < ordered.size(); ++position) {
                const size_t index = reverse ? ordered.size()-1-position : position;
                if (covered[index]) continue;
                const auto& entry = ordered[index];
                TopTools_IndexedMapOfShape members;
                TopExp::MapShapes(sequential.shape, TopAbs_EDGE, members);
                const auto current_edges = [&](const TopoDS_Shape& original) {
                    TopTools_ListOfShape current = sequential.history->Modified(original);
                    if (members.Contains(original)) current.Append(original);
                    return current;
                };
                const auto current = current_edges(entry.second);
                TopTools_MapOfShape unique;
                BRepFilletAPI_MakeFillet step(sequential.shape);
                for (TopTools_ListIteratorOfListOfShape it(current); it.More(); it.Next()) {
                    if (it.Value().ShapeType() == TopAbs_EDGE && members.Contains(it.Value()) && unique.Add(it.Value()))
                        step.Add(spec.pad_length, TopoDS::Edge(it.Value()));
                }
                if (unique.IsEmpty()) {
                    // An adjacent same-radius blend can consume a short
                    // requested edge completely. OCCT deletion is evidence of
                    // that event; there is no surviving edge to round again.
                    if (sequential.history->IsRemoved(entry.second)) continue;
                    throw std::runtime_error("FILLET_HISTORY_EDGE_MISSING");
                }
                // A previous contour can propagate over several requested
                // edges once its neighbors become tangent. Do not fillet an
                // already treated contour for a second time.
                for (size_t candidate = 0; candidate < ordered.size(); ++candidate) {
                    bool found = false, all = true;
                    const auto descendants = current_edges(ordered[candidate].second);
                    for (TopTools_ListIteratorOfListOfShape it(descendants); it.More(); it.Next()) {
                        if (it.Value().ShapeType() != TopAbs_EDGE || !members.Contains(it.Value())) continue;
                        found = true;
                        if (step.Contour(TopoDS::Edge(it.Value())) == 0) all = false;
                    }
                    if (found && all) covered[candidate] = true;
                }
                step.Build();
                if (!step.IsDone()) throw std::runtime_error("MODIFIER_ALGORITHM_FAILED:FILLET:SEQUENTIAL");
                validate_body_solid_set(step.Shape());
                TopTools_ListOfShape arguments;
                arguments.Append(sequential.shape);
                sequential.history->Merge(arguments, step);
                sequential.shape = step.Shape();
            }
            return sequential;
        };
        ComposedModifier sequential;
        try { sequential = build_sequential(false); }
        catch (const std::exception&) { sequential = build_sequential(true); }
        auto result = finish(sequential);
        result.diagnostics.push_back("FILLET_SEQUENTIAL_HISTORY");
        return result;
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
        const double tolerance = std::max(1.0e-7, shape_volume(input) * 1.0e-9);
        const auto inward_valid = [&](const TopoDS_Shape& shape) {
            if (shape.IsNull() || shape_volume(shape) >= shape_volume(input) - tolerance) return false;
            BRepAlgoAPI_Cut outside(shape, input);
            outside.Build();
            return outside.IsDone() && shape_volume(outside.Shape()) <= tolerance;
        };
        if (!spec.reversed) {
            try {
                auto planar = convex_planar_shell(input, selected, spec.pad_length);
                auto result = finish(planar);
                if (!inward_valid(result.shape)) throw std::invalid_argument("SHELL_THICKNESS_EXCEEDS_INTERIOR");
                result.diagnostics.push_back("SHELL_CONVEX_PLANAR_OFFSET_HISTORY");
                return result;
            } catch (const std::invalid_argument& error) {
                const std::string code = error.what();
                if (code != "SHELL_PLANAR_FALLBACK_UNSUPPORTED" && code != "SHELL_PLANAR_FALLBACK_NONCONVEX") throw;
            }
        }
        const double toleranceLimit = std::max(1e-5, shell_input_tolerance(input) * 1.01);
        std::string failure;
        const auto attempt = [&](auto build, bool clippedToInput = false) -> std::optional<BodyOperationResult> {
            try {
                BRepBuilderAPI_Copy attemptCopy(input, Standard_True, Standard_False);
                std::vector<TopoDS_Shape> closing;
                for (const auto& face : selected) closing.push_back(attemptCopy.ModifiedShape(face));
                auto candidate = build(attemptCopy.Shape(), closing);
                validate_shell_offset(candidate.shape, toleranceLimit);
                if (solid_count(candidate.shape) != solid_count(input)) throw std::runtime_error("SHELL_DISCONNECTED_WALLS");
                if (!spec.reversed && (shape_volume(candidate.shape) >= shape_volume(input) - tolerance ||
                    (!clippedToInput && !inward_valid(candidate.shape)))) throw std::runtime_error("SHELL_INVALID_INTERIOR");
                TopTools_ListOfShape source; source.Append(input);
                Handle(BRepTools_History) history = new BRepTools_History(source, attemptCopy);
                history->Merge(candidate.history);
                candidate.history = history;
                return finish(candidate);
            } catch (const Standard_Failure& error) {
                failure = error.GetMessageString() ? error.GetMessageString() : "SHELL_OFFSET_FAILED";
            } catch (const std::exception& error) {
                failure += std::string("[") + error.what() + "]";
            }
            return std::nullopt;
        };
        for (auto join : {GeomAbs_Intersection, GeomAbs_Arc}) {
            auto result = attempt([&](const TopoDS_Shape& shape, const std::vector<TopoDS_Shape>& closing) {
                TopTools_ListOfShape faces; for (const auto& face : closing) faces.Append(face);
                BRepOffsetAPI_MakeThickSolid algorithm;
                algorithm.MakeThickSolidByJoin(shape, faces, spec.reversed ? spec.pad_length : -spec.pad_length,
                    1e-7, BRepOffset_Skin, Standard_False, Standard_False, join);
                if (!algorithm.IsDone()) throw std::runtime_error("SHELL_OFFSET_FAILED:" + std::to_string(static_cast<int>(algorithm.MakeOffset().Error())));
                return ComposedModifier{algorithm.Shape(), shell_offset_history(shape, algorithm)};
            });
            if (result) return *result;
        }
        if (!spec.reversed) {
            auto result = attempt([&](const auto& shape, const auto& closing) { return simple_open_shell(shape, closing, spec.pad_length, toleranceLimit); });
            if (result) return *result;
            result = attempt([&](const auto& shape, const auto& closing) { return inward_face_shell(shape, closing, spec.pad_length, toleranceLimit); }, true);
            if (result) return *result;
        }
        throw std::runtime_error("MODIFIER_ALGORITHM_FAILED:SHELL:" + failure);
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

std::vector<LoftSectionSpec> resolve_loft_correspondence(
    const std::vector<LoftSectionSpec>& sections) {
    return resolve_loft_sections(sections);
}
std::vector<std::vector<Vec3>> loft_connection_points(
    const std::vector<LoftSectionSpec>& sections) {
    const auto count = loft_boundary_count(sections);
    std::vector<std::vector<Vec3>> result;
    for (const auto& section : sections) {
        std::vector<Vec3> points;
        if (!section.point_id.empty())
            points.assign(count, section.point);
        else
            for (const auto& edge : loft_edges(section, count)) {
                points.push_back(to_vec3(BRep_Tool::Pnt(TopExp::FirstVertex(edge, Standard_True))));
            }
        result.push_back(std::move(points));
    }
    return result;
}

struct OcctKernel::Impl {
    struct StoredGeometry {
        TopoDS_Shape shape;
        std::vector<uint8_t> brep;
        uint64_t used = 0;
    };

    // Checkpoints own immutable shapes and true history. OCCT operations remain
    // serialized by the Worker; eviction only drops cache ownership, never locals.
    struct Stage {
        TopoDS_Shape shape;
        GeometryId geometry_id;
        std::vector<NamedShape> named;
        ProfileEvaluationResult evaluation;
        bool history_complete = false;
        std::map<std::string, ToolBuild> tools, bodies;
        std::map<std::string, TopoDS_Shape> bases;
        size_t bytes = 0;
        mutable uint64_t used = 0;
    };
    std::unordered_map<std::string, std::shared_ptr<const Stage>> stages;
    size_t stage_bytes = 0, stage_budget = 128 * 1024 * 1024, stage_evictions = 0;
    uint64_t clock = 0;
    void trim_stages(size_t incoming = 0) {
        while (!stages.empty() && stage_bytes + incoming > stage_budget) {
            auto oldest = std::min_element(stages.begin(), stages.end(), [](const auto& a,const auto& b){return a.second->used < b.second->used;});
            stage_bytes -= oldest->second->bytes; stages.erase(oldest); ++stage_evictions;
        }
    }
    std::unordered_map<GeometryId, StoredGeometry> shapes;
    std::unordered_map<GeometryId, TopologyInfo> topologies;
    size_t shape_bytes=0,shape_budget=128*1024*1024;
    void trim_shapes(const GeometryId& active) {
        // Active handles are also owned by local execution / stage checkpoints.
        // The current working shape may exceed admission budget; retain only it.
        while(shapes.size()>1 && shape_bytes>shape_budget) {
            auto oldest=shapes.end();
            for(auto it=shapes.begin();it!=shapes.end();++it)
                if(it->first!=active&&(oldest==shapes.end()||it->second.used<oldest->second.used))oldest=it;
            if(oldest==shapes.end())break;
            shape_bytes-=oldest->second.brep.capacity()*16;topologies.erase(oldest->first);shapes.erase(oldest);
        }
    }
    void adopt(const GeometryId& id,const TopoDS_Shape& shape,const std::vector<uint8_t>& bytes) {
        const auto previous=shapes.find(id);
        if(previous!=shapes.end()){previous->second.used=++clock;return;}
        shapes.emplace(id,StoredGeometry{shape,bytes,++clock});shape_bytes+=shapes.at(id).brep.capacity()*16;
        trim_shapes(id);
    }

    GeometryId store(const TopoDS_Shape& shape) {
        if (shape.IsNull()) {
            throw std::runtime_error("cannot store a null shape");
        }
        const std::vector<uint8_t> bytes = write_brep(shape);
        const GeometryId id = make_geometry_id(bytes.data(), bytes.size());
        adopt(id,shape,bytes);
        return id;
    }

    TopoDS_Shape& find(const GeometryId& id) {
        const auto iterator = shapes.find(id);
        if (iterator == shapes.end()) {
            throw std::out_of_range("geometry is not resident: " + id);
        }
        iterator->second.used=++clock;
        return iterator->second.shape;
    }
};

OcctKernel::OcctKernel() : impl_(std::make_unique<Impl>()) {
}
OcctKernel::~OcctKernel() = default;
OcctKernel::OcctKernel(OcctKernel&&) noexcept = default;
OcctKernel& OcctKernel::operator=(OcctKernel&&) noexcept = default;

void OcctKernel::clear_runtime_cache() {impl_->stages.clear();impl_->stage_bytes=0;}
void OcctKernel::set_runtime_cache_budget(size_t bytes) {impl_->stage_budget=bytes;impl_->trim_stages();}
size_t OcctKernel::runtime_cache_bytes() const noexcept {return impl_->stage_bytes;}

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
    impl_->adopt(id,shape,data);
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
    const auto found=impl_->shapes.find(id);
    if(found!=impl_->shapes.end())impl_->shape_bytes-=found->second.brep.capacity()*16;
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
    const ImportTopologySeed* import_seed, const PartRuntimeOptions& runtime) {
    PartRuntimeStats local_stats;
    auto& stats = runtime.stats ? *runtime.stats : local_stats;
    stats = {};
    const auto evictions_before = impl_->stage_evictions;
    const auto check_cancel = [&](){if(runtime.cancelled && runtime.cancelled())throw std::runtime_error("EVALUATION_CANCELLED");};
    check_cancel();
    if(runtime.force_cold) {
        // No borrowed OCCT handles survive across a coarse request. The caller's
        // frozen BREP inputs restore exact data; resident Shapes cannot mask defects.
        impl_->shapes.clear();impl_->topologies.clear();impl_->shape_bytes=0;
    }
    std::shared_ptr<const Impl::Stage> restored;
    size_t resume = 0;
    if(!runtime.force_cold && runtime.stage_keys.size()==specs.size()) {
        for(size_t i=specs.size();i>0;--i) {
            const auto found=impl_->stages.find(runtime.stage_keys[i-1]);
            if(found!=impl_->stages.end()){restored=found->second;restored->used=++impl_->clock;resume=i;break;}
        }
    }
    TopoDS_Shape result;
    GeometryId result_id;
    std::vector<NamedShape> live_named;
    if (!restored && !base_brep.empty()) {
        const GeometryId base_id = loadBrepr(base_brep);
        result = impl_->find(base_id);
        result_id = base_id;
    }
    ProfileEvaluationResult evaluation;
    bool topology_history_complete = base_brep.empty();
    if (import_seed && !restored) {
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

    std::map<std::string, ToolBuild> pattern_tools, pattern_bodies;
    std::map<std::string, TopoDS_Shape> pattern_range_bases;
    std::set<std::string> range_starts;
    std::set<std::string> required_pattern_tools, required_pattern_bodies;
    for (const auto& spec : specs) {
        if (spec.generator != "SOLID_PATTERN") continue;
        (spec.pattern_source_kind == "GENERATOR_TOOL" ? required_pattern_tools : required_pattern_bodies).insert(spec.pattern_source_feature_id);
        if (spec.pattern_source_kind == "FEATURE_DELTA") range_starts.insert(spec.pattern_start_feature_id);
    }
    if (import_seed && !result.IsNull() && !restored) {
        ToolBuild stage; stage.shape=result;stage.named=live_named;
        stage.topology_history_complete=topology_history_complete;
        pattern_bodies.emplace(import_seed->feature_id,std::move(stage));
    }
    if(restored) {
        result=restored->shape;result_id=restored->geometry_id;live_named=restored->named;
        evaluation=restored->evaluation;topology_history_complete=restored->history_complete;
        pattern_tools=restored->tools;pattern_bodies=restored->bodies;pattern_range_bases=restored->bases;
        for(size_t i=0;i<resume;++i) {
            stats.reused_feature_ids.push_back(specs[i].feature_id);
            const auto found=std::find_if(evaluation.feature_results.begin(),evaluation.feature_results.end(),[&](const auto& f){return f.feature_id==specs[i].feature_id;});
            if(found!=evaluation.feature_results.end())stats.stages.push_back({specs[i].feature_id,runtime.stage_keys[i],found->result_geometry_id,true});
        }
        stats.stages_reused=resume;
        // Re-adopt the saved BREP handle if the repository was explicitly unloaded.
        if(!is_loaded(result_id))impl_->store(result);
    }
    for (size_t spec_index=resume;spec_index<specs.size();++spec_index) {
        const auto& spec=specs[spec_index];
        check_cancel();
        const auto exact_started=std::chrono::steady_clock::now();
        try {
            const auto input_shape = result;
            const auto input_id = result_id;
            const auto input_named = live_named;
            if (range_starts.count(spec.feature_id)) {
                if ((spec.generator != "LINEAR_EXTRUDE" && spec.generator != "REVOLVE" && spec.generator != "LOFT") ||
                    (spec.body_operation != "ADD" && spec.body_operation != "NEW_BODY") || spec.extent == "THROUGH_ALL")
                    throw std::invalid_argument("PATTERN_RANGE_INVALID");
                pattern_range_bases.emplace(spec.feature_id, result);
            }
            if ((spec.generator=="LINEAR_EXTRUDE" || spec.generator=="REVOLVE" || spec.generator=="LOFT") &&
                (spec.body_operation=="ADD" || spec.body_operation=="NEW_BODY") && spec.extent!="THROUGH_ALL")
                pattern_range_bases.insert_or_assign(spec.feature_id,result);
            ToolBuild tool;
            const bool modifier = spec.generator == "FILLET" || spec.generator == "CHAMFER" ||
                                  spec.generator == "DRAFT" || spec.generator == "SHELL";
            if(!modifier && spec.generator!="SOLID_PATTERN" && spec.generator!="BOOLEAN")++stats.generator_calls;
            if (modifier) {
                tool.feature_id = spec.feature_id;
            } else if (spec.generator == "SOLID_PATTERN") {
                const auto& stages = spec.pattern_source_kind == "GENERATOR_TOOL" ? pattern_tools : pattern_bodies;
                if (spec.pattern_source_kind != "GENERATOR_TOOL" && spec.pattern_source_kind != "BODY_STAGE" && spec.pattern_source_kind != "FEATURE_DELTA")
                    throw std::invalid_argument("PATTERN_SOURCE_KIND_INVALID");
                const auto found = stages.find(spec.pattern_source_feature_id);
                if (found == stages.end()) throw std::invalid_argument("PATTERN_SOURCE_STAGE_UNAVAILABLE");
                if (spec.pattern_source_kind == "FEATURE_DELTA") {
                    const auto base = pattern_range_bases.find(spec.pattern_start_feature_id);
                    if (base == pattern_range_bases.end() || spec.body_operation != "ADD")
                        throw std::invalid_argument("PATTERN_RANGE_INVALID");
                    bool inside = false, ended = false;
                    for (const auto& member : specs) {
                        if (member.feature_id == spec.pattern_start_feature_id) inside = true;
                        if (inside && member.feature_id != spec.pattern_start_feature_id &&
                            member.generator != "FILLET" && member.generator != "CHAMFER")
                            throw std::invalid_argument("PATTERN_RANGE_REEXECUTION_UNSUPPORTED");
                        if (inside && member.feature_id == spec.pattern_source_feature_id) { ended = true; break; }
                    }
                    if (!ended) throw std::invalid_argument("PATTERN_RANGE_INVALID");
                    auto delta = found->second;
                    if (!base->second.IsNull()) {
                        // Additive ranges must retain all baseline material. This is
                        // an exact stage difference, not re-execution or Body copying.
                        BRepAlgoAPI_Cut removed;
                        removed.SetNonDestructive(Standard_True);
                        TopTools_ListOfShape baseline, endpoint; baseline.Append(base->second); endpoint.Append(delta.shape);
                        removed.SetArguments(baseline);
                        removed.SetTools(endpoint);
                        removed.Build();
                        if (!removed.IsDone() || removed.HasErrors())
                            throw std::runtime_error("PATTERN_RANGE_BASELINE_CHECK_FAILED");
                        if (shape_volume(removed.Shape()) > std::max(1e-7, shape_volume(base->second)*1e-10))
                            throw std::invalid_argument("PATTERN_RANGE_REMOVES_BASELINE");
                        BRepAlgoAPI_Cut cut;
                        cut.SetNonDestructive(Standard_True);
                        cut.SetArguments(endpoint);
                        cut.SetTools(baseline);
                        cut.Build();
                        if (!cut.IsDone() || cut.HasErrors()) throw std::runtime_error("PATTERN_RANGE_EXTRACTION_FAILED");
                        validate_body_solid_set(cut.Shape());
                        delta.named = map_named_shapes(delta.named, cut, cut.Shape());
                        delta.shape = cut.Shape();
                        delta.generated = complete_boolean_topology_naming(delta.shape, delta.named, spec.pattern_start_feature_id);
                    }
                    tool = make_pattern_tool(delta, spec);
                } else tool = make_pattern_tool(found->second,spec);
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
                        BRepAlgoAPI_Fuse combine;
                        combine.SetNonDestructive(Standard_True);
                        TopTools_ListOfShape arguments,tools;arguments.Append(tool.shape);tools.Append(shape);
                        combine.SetArguments(arguments);combine.SetTools(tools);combine.Build();
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
            if (required_pattern_tools.count(spec.feature_id) &&
                (modifier || spec.generator=="SOLID_PATTERN" || spec.extent=="THROUGH_ALL" || tool.shape.IsNull()))
                throw std::invalid_argument("PATTERN_INDEPENDENT_TOOL_UNAVAILABLE");
            if (!modifier && spec.generator!="SOLID_PATTERN" && spec.generator!="BOOLEAN" && spec.extent!="THROUGH_ALL" && !tool.shape.IsNull()) {
                // Retain the real independently constructed tool before Body
                // application, never the already-cut stage or the whole Body.
                if (modifier || spec.generator == "SOLID_PATTERN" || spec.extent == "THROUGH_ALL" || tool.shape.IsNull())
                    throw std::invalid_argument("PATTERN_INDEPENDENT_TOOL_UNAVAILABLE");
                validate_body_solid_set(tool.shape);
                auto closure = complete_boolean_topology_naming(tool.shape, tool.named, spec.feature_id);
                tool.generated.insert(tool.generated.end(), closure.begin(), closure.end());
                pattern_tools.emplace(spec.feature_id, tool);
            }
            auto independent = [&]() {
                BRep_Builder builder;TopoDS_Compound compound;builder.MakeCompound(compound);
                builder.Add(compound,result);builder.Add(compound,tool.shape);
                validate_body_solid_set(compound);
                auto named=live_named;named.insert(named.end(),tool.named.begin(),tool.named.end());
                return BodyOperationResult{compound,std::move(named),{}, {}};
            };
            if(modifier)++stats.modifier_calls;
            else ++stats.body_operation_calls;
            auto operation = spec.generator == "SOLID_PATTERN" && tool.shape.IsNull()
                ? BodyOperationResult{result,live_named,{}, {}}
                : spec.generator=="SOLID_PATTERN" && spec.pattern_result_mode=="INDEPENDENT" ? independent()
                : modifier ? apply_local_modifier(result, live_named, spec)
                           : apply_body_operation(result, live_named, tool, spec.body_operation, spec.generator == "LOFT");
            result = operation.shape;
            result_id = impl_->store(result);

            stats.exact_ms+=std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-exact_started).count();
            const auto naming_started=std::chrono::steady_clock::now();
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
            {
                ToolBuild stage;
                stage.shape=result; stage.named=live_named; stage.feature_id=spec.feature_id;
                stage.topology_history_complete=topology_history_complete;
                pattern_bodies.emplace(spec.feature_id,std::move(stage));
            }
            evaluation.feature_results.push_back(std::move(feature));
            stats.naming_ms+=std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-naming_started).count();
            ++stats.stages_executed;stats.executed_feature_ids.push_back(spec.feature_id);
            stats.stages.push_back({spec.feature_id,runtime.stage_keys.size()==specs.size()?runtime.stage_keys[spec_index]:"",result_id,false});
            check_cancel();
            if(!runtime.force_cold && runtime.stage_keys.size()==specs.size()) {
                auto stage=std::make_shared<Impl::Stage>();
                stage->shape=result;stage->geometry_id=result_id;stage->named=live_named;
                stage->evaluation=evaluation;stage->history_complete=topology_history_complete;
                stage->tools=pattern_tools;stage->bodies=pattern_bodies;stage->bases=pattern_range_bases;
                // Conservative retained-shape allowance plus actual serialized history.
                // Maps share OCCT handles; budget intentionally overcounts shared ancestors.
                stage->bytes=sizeof(Impl::Stage)+impl_->shapes.at(result_id).brep.capacity()*16;
                for(const auto& f:stage->evaluation.feature_results)
                    stage->bytes+=f.topology_history.lineage.size()*sizeof(TopologyLineage)+f.semantic_outputs.size()*sizeof(SemanticTopologyOutput);
                for(const auto& entry:stage->bodies) {
                    stage->bytes+=entry.second.named.size()*sizeof(NamedShape);
                }
                stage->bytes*=1+stage->tools.size()+stage->bodies.size();
                stage->used=++impl_->clock;
                if(stage->bytes<=impl_->stage_budget) {
                    impl_->trim_stages(stage->bytes);
                    const auto key=runtime.stage_keys[spec_index];
                    const auto old=impl_->stages.find(key);
                    if(old!=impl_->stages.end())impl_->stage_bytes-=old->second->bytes;
                    impl_->stage_bytes+=stage->bytes;impl_->stages.insert_or_assign(key,std::move(stage));
                }
            }
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
    stats.stage_cache_bytes=impl_->stage_bytes;stats.stage_evictions=impl_->stage_evictions-evictions_before;
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

const TopoDS_Shape& OcctKernel::analysis_shape(const GeometryId& id) const {
    return impl_->find(id);
}

TopologyInfo OcctKernel::getTopologySummary(const GeometryId& id) {
    const auto& shape=impl_->find(id);TopologyInfo out;
    const auto count=[&](TopAbs_ShapeEnum type){TopTools_IndexedMapOfShape map;TopExp::MapShapes(shape,type,map);return static_cast<uint32_t>(map.Extent());};
    out.face_count=count(TopAbs_FACE);out.edge_count=count(TopAbs_EDGE);
    out.vertex_count=count(TopAbs_VERTEX);out.solid_count=count(TopAbs_SOLID);return out;
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
        // Display polylines belong exclusively to the visual snapshot.
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

TessellationResult OcctKernel::tessellate(const GeometryId& geometry_id, const double linear_deflection,
                                          const double angular_deflection, const bool parallel) {
    validate_positive(linear_deflection, "linear_deflection");
    validate_positive(angular_deflection, "angular_deflection");
    try {
        const auto start = std::chrono::steady_clock::now();
        auto elapsed = [](auto from) { return std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now()-from).count(); };
        // A TopoDS value copy aliases the same TShapes. Copy geometry as well, omit
        // old triangulations: meshing must never mutate resident exact geometry.
        const auto& source = impl_->find(geometry_id);
        BRepBuilderAPI_Copy copy(source, Standard_True, Standard_False);
        const auto shape = copy.Shape();
        IMeshTools_Parameters parameters;
        parameters.Deflection = linear_deflection;
        parameters.Angle = angular_deflection;
        parameters.AngleInterior = angular_deflection;
        parameters.InParallel = parallel;
        parameters.EnableControlSurfaceDeflectionAllSurfaces = Standard_True;
        BRepMesh_IncrementalMesh mesher(shape, parameters);
        constexpr int invalid_mesh = IMeshData_OpenWire | IMeshData_SelfIntersectingWire | IMeshData_Failure
            | IMeshData_UnorientedWire | IMeshData_TooFewPoints | IMeshData_UserBreak;
        if (!mesher.IsDone() || (mesher.GetStatusFlags() & invalid_mesh))
            throw std::runtime_error("VISUAL_MESH_FAILED:status="+std::to_string(mesher.GetStatusFlags()));
        TessellationResult result;
        result.timings.meshing_ms = elapsed(start);
        result.bbox = getBoundingBox(geometry_id);
        TopTools_IndexedMapOfShape face_map, edge_map, vertex_map;
        // IDs are taken from the exact source snapshot, never from copy traversal.
        TopExp::MapShapes(source, TopAbs_FACE, face_map);
        TopExp::MapShapes(source, TopAbs_EDGE, edge_map);
        TopExp::MapShapes(source, TopAbs_VERTEX, vertex_map);
        std::vector<std::vector<std::vector<Vec3>>> boundaries(edge_map.Extent());
        const auto extraction = std::chrono::steady_clock::now();
        for (int face_id = 1; face_id <= face_map.Extent(); ++face_id) {
            const auto face = TopoDS::Face(copy.ModifiedShape(face_map(face_id)).Oriented(face_map(face_id).Orientation()));
            TopLoc_Location location;
            const auto triangulation = BRep_Tool::Triangulation(face, location);
            if (triangulation.IsNull() || triangulation->NbTriangles() == 0)
                throw std::runtime_error("VISUAL_MISSING_FACE:" + std::to_string(face_id));
            const uint32_t base = static_cast<uint32_t>(result.vertices.size());
            for (int n = 1; n <= triangulation->NbNodes(); ++n)
                result.vertices.push_back(to_vec3(triangulation->Node(n).Transformed(location.Transformation())));
            for (int t = 1; t <= triangulation->NbTriangles(); ++t) {
                int a, b, c;
                triangulation->Triangle(t).Get(a,b,c);
                if (face.Orientation() == TopAbs_REVERSED) std::swap(b,c);
                result.triangles.push_back({base+uint32_t(a-1),base+uint32_t(b-1),base+uint32_t(c-1)});
                result.face_ids.push_back(face_id);
            }
            const auto edge_start = std::chrono::steady_clock::now();
            // Traverse every oriented EdgeUse, including both sides of a periodic
            // seam. Looking up only one polygon per topological edge loses seams.
            for (TopExp_Explorer uses(face_map(face_id).Oriented(TopAbs_FORWARD), TopAbs_EDGE); uses.More(); uses.Next()) {
                const auto original = TopoDS::Edge(uses.Current());
                const auto edge = TopoDS::Edge(copy.ModifiedShape(original).Oriented(original.Orientation()));
                const auto polygon = BRep_Tool::PolygonOnTriangulation(edge, triangulation, location);
                const auto edge_id = edge_map.FindIndex(original);
                if (polygon.IsNull() || polygon->NbNodes() < 2)
                    throw std::runtime_error("VISUAL_MISSING_BOUNDARY:face="+std::to_string(face_id)+":edge="+std::to_string(edge_id));
                std::vector<Vec3> points;
                for (int i = 1; i <= polygon->NbNodes(); ++i) {
                    const auto node = polygon->Node(i);
                    if (node < 1 || node > triangulation->NbNodes()) throw std::runtime_error("VISUAL_BOUNDARY_INDEX");
                    points.push_back(result.vertices[base+node-1]);
                }
                boundaries[edge_id-1].push_back(std::move(points));
            }
            result.timings.edges_ms += elapsed(edge_start);
        }
        result.timings.faces_ms = elapsed(extraction) - result.timings.edges_ms;
        const auto edge_validation = std::chrono::steady_clock::now();
        auto distance = [](const Vec3& a, const Vec3& b) { return std::hypot(a.x-b.x, a.y-b.y, a.z-b.z); };
        for (int id = 1; id <= edge_map.Extent(); ++id) {
            const auto edge = TopoDS::Edge(edge_map(id));
            const auto& uses = boundaries[id-1];
            if (uses.empty()) {
                // Only truly unattached curves have an independent discretization.
                if (BRep_Tool::Degenerated(edge)) continue;
                BRepAdaptor_Curve curve(edge);
                if (!std::isfinite(curve.FirstParameter()) || !std::isfinite(curve.LastParameter()))
                    throw std::runtime_error("VISUAL_UNBOUNDED_CURVE");
                GCPnts_TangentialDeflection sample(curve, angular_deflection, linear_deflection);
                EdgePolyline line{static_cast<uint64_t>(id), {}};
                for (int i = 1; i <= sample.NbPoints(); ++i) line.points.push_back(to_vec3(sample.Value(i)));
                if (line.points.size()<2) throw std::runtime_error("VISUAL_EMPTY_CURVE");
                result.edges.push_back(std::move(line));
                continue;
            }
            const auto& canonical = uses.front();
            // Compare every neighbor using the exact model's own uncertainty, not
            // display deflection. No coordinate welding or independent resampling.
            double tolerance = std::max(Precision::Confusion(), BRep_Tool::Tolerance(edge));
            for (TopExp_Explorer v(edge, TopAbs_VERTEX); v.More(); v.Next())
                tolerance = std::max(tolerance, BRep_Tool::Tolerance(TopoDS::Vertex(v.Current())));
            for (const auto& use : uses) {
                bool forward = use.size() == canonical.size(), reverse = forward;
                for (size_t i = 0; i < use.size() && (forward || reverse); ++i) {
                    forward = forward && distance(use[i],canonical[i]) <= tolerance;
                    reverse = reverse && distance(use[i],canonical[canonical.size()-1-i]) <= tolerance;
                }
                if (!forward && !reverse) throw std::runtime_error("VISUAL_ADJACENT_BOUNDARY_MISMATCH:edge="+std::to_string(id));
            }
            if (BRep_Tool::Degenerated(edge)) {
                for (const auto& p : canonical) if (distance(p,canonical.front()) > tolerance)
                    throw std::runtime_error("VISUAL_INVALID_DEGENERATE_EDGE");
                continue; // poles have topology vertices, no drawable line
            }
            result.edges.push_back({static_cast<uint64_t>(id), canonical});
        }
        for (int id = 1; id <= vertex_map.Extent(); ++id)
            result.topology_vertices.push_back({static_cast<uint64_t>(id), to_vec3(BRep_Tool::Pnt(TopoDS::Vertex(vertex_map(id))))});
        result.timings.edges_ms += elapsed(edge_validation);
        const auto normals = std::chrono::steady_clock::now();
        compute_mesh_normals(result);
        result.timings.normals_ms = elapsed(normals);
        return result;
    } catch (const Standard_Failure& error) {
        throw std::runtime_error(std::string("VISUAL_OCCT_FAILURE:") + (error.GetMessageString() ? error.GetMessageString() : "unknown"));
    }
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

#include <BRepAdaptor_Curve.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepAlgoAPI_Section.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeSolid.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepBuilderAPI_Sewing.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepGProp.hxx>
#include <BRepLib.hxx>
#include <BRepOffsetAPI_MakeOffset.hxx>
#include <BRepOffsetAPI_MakeThickSolid.hxx>
#include <BRepOffset_MakeOffset.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepPrimAPI_MakeRevol.hxx>
#include <BRepProj_Projection.hxx>
#include <BRepTools.hxx>
#include <BRepTools_WireExplorer.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GCPnts_AbscissaPoint.hxx>
#include <GC_MakeArcOfCircle.hxx>
#include <GProp_GProps.hxx>
#include <Geom2d_BSplineCurve.hxx>
#include <Geom2d_Curve.hxx>
#include <GeomAPI.hxx>
#include <GeomAPI_Interpolate.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BezierCurve.hxx>
#include <Geom_Circle.hxx>
#include <Geom_Ellipse.hxx>
#include <Geom_Line.hxx>
#include <Geom_Plane.hxx>
#include <Geom_TrimmedCurve.hxx>
#include <Standard_Failure.hxx>
#include <TColStd_Array1OfInteger.hxx>
#include <TColStd_Array1OfReal.hxx>
#include <TColgp_Array1OfPnt.hxx>
#include <TColgp_HArray1OfPnt.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopTools_ListIteratorOfListOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Iterator.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <TopoDS_Vertex.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Ax2.hxx>
#include <gp_Pln.hxx>

#include <internal/geometry_feature_support.hpp>
#include <occccad/kernel/geometry_id.hpp>

#include <algorithm>
#include <cmath>
#include <map>
#include <set>
#include <sstream>
#include <stdexcept>

namespace occccad::kernel::detail {
namespace geometry_features {
constexpr double pi = 3.14159265358979323846;
gp_Pnt point(const Vec3& v) {
    if (!std::isfinite(v.x) || !std::isfinite(v.y) || !std::isfinite(v.z))
        throw std::invalid_argument("GEOMETRY_NONFINITE_COORDINATE");
    return {v.x, v.y, v.z};
}
gp_Vec vector(const Vec3& v) {
    const auto p = point(v);
    return {p.X(), p.Y(), p.Z()};
}
gp_Dir direction(const Vec3& v) {
    const auto x = vector(v);
    if (x.Magnitude() <= tolerance)
        throw std::invalid_argument("GEOMETRY_DIRECTION_REQUIRED");
    return gp_Dir(x);
}
std::string key(const SemanticTopologyRef& r) {
    std::ostringstream s;
    s << r.feature_id << '\0' << r.output_slot;
    for (const auto& id : r.source_ids)
        s << '\0' << id;
    return s.str();
}
SemanticTopologyRef ref(const FeatureTopologyRef& r) {
    return {r.feature_id, r.output_slot, r.source_ids};
}
std::string signature(std::vector<SemanticTopologyRef> refs) {
    std::vector<std::string> keys;
    for (const auto& r : refs)
        keys.push_back(key(r));
    std::sort(keys.begin(), keys.end());
    std::ostringstream s;
    for (const auto& k : keys)
        s << k << '\0';
    return make_geometry_id(s.str()).substr(7);
}
SemanticTopologyRef derived(const std::string& id, const std::string& role,
                            const std::vector<SemanticTopologyRef>& sources) {
    std::vector<std::string> ids;
    for (const auto& r : sources)
        ids.push_back(r.feature_id + "/" + r.output_slot);
    std::sort(ids.begin(), ids.end());
    return {id, role + "/" + signature(sources), ids};
}
bool contains(const TopoDS_Shape& parent, const TopoDS_Shape& child) {
    if (parent.IsSame(child))
        return true;
    for (TopExp_Explorer e(parent, child.ShapeType()); e.More(); e.Next())
        if (e.Current().IsSame(child))
            return true;
    return false;
}
void add(GeometryFeatureBuild& b, const SemanticTopologyRef& r, const TopoDS_Shape& shape,
         std::vector<SemanticTopologyRef> sources, std::vector<SemanticTopologyRef> roles) {
    if (shape.IsNull() || !contains(b.shape, shape))
        return;
    if (shape.ShapeType() != TopAbs_FACE && shape.ShapeType() != TopAbs_EDGE &&
        shape.ShapeType() != TopAbs_VERTEX)
        return;
    const auto unique = [](auto& refs) {
        std::sort(refs.begin(), refs.end(),
                  [](const auto& a, const auto& c) { return key(a) < key(c); });
        refs.erase(std::unique(refs.begin(), refs.end(),
                               [](const auto& a, const auto& c) { return key(a) == key(c); }),
                   refs.end());
    };
    if (roles.empty())
        roles.push_back(r);
    for (auto& n : b.named)
        if (n.shape.IsSame(shape)) {
            if (key(n.ref) != key(r)) {
                if (n.roles.empty())
                    n.roles.push_back(n.ref);
                n.roles.insert(n.roles.end(), roles.begin(), roles.end());
                unique(n.roles);
                n.sources.insert(n.sources.end(), sources.begin(), sources.end());
                unique(n.sources);
                n.ref = derived(r.feature_id, "merged", n.roles);
            }
            return;
        }
    b.named.push_back({r, shape, std::move(sources), std::move(roles)});
}
TopoDS_Shape compound(const std::vector<TopoDS_Shape>& shapes) {
    if (shapes.size() == 1)
        return shapes.front();
    BRep_Builder builder;
    TopoDS_Compound c;
    builder.MakeCompound(c);
    for (const auto& s : shapes)
        builder.Add(c, s);
    return c;
}
std::vector<TopoDS_Shape> members(const TopoDS_Shape& shape, TopAbs_ShapeEnum type) {
    TopTools_IndexedMapOfShape m;
    TopExp::MapShapes(shape, type, m);
    std::vector<TopoDS_Shape> out;
    for (int i = 1; i <= m.Extent(); ++i)
        out.push_back(m(i));
    return out;
}
void endpoints(GeometryFeatureBuild& b, const std::string& id, const TopoDS_Edge& e,
               const std::string& role, const std::vector<SemanticTopologyRef>& sources) {
    TopoDS_Vertex a, z;
    TopExp::Vertices(e, a, z, Standard_True);
    add(b, derived(id, role + "/start", sources), a, sources);
    if (!z.IsSame(a))
        add(b, derived(id, role + "/end", sources), z, sources);
}
void curve_result(GeometryFeatureBuild& b, const std::string& id, const TopoDS_Edge& e,
                  const std::vector<SemanticTopologyRef>& sources,
                  const std::string& role = "curve") {
    b.shape = e;
    add(b, derived(id, role, sources), e, sources);
    endpoints(b, id, e, role, sources);
}
TopoDS_Wire wire(const TopoDS_Shape& s) {
    BRepBuilderAPI_MakeWire w;
    const auto es = members(s, TopAbs_EDGE);
    if (es.empty())
        throw std::invalid_argument("CURVE_INPUT_REQUIRED");
    for (const auto& e : es)
        w.Add(TopoDS::Edge(e));
    if (!w.IsDone())
        throw std::invalid_argument("CURVE_CHAIN_DISCONNECTED_OR_BRANCHING");
    const auto result = w.Wire();
    int count = 0;
    for (BRepTools_WireExplorer e(result); e.More(); e.Next())
        ++count;
    if (static_cast<size_t>(count) != es.size())
        throw std::invalid_argument("CURVE_CHAIN_DISCONNECTED_OR_BRANCHING");
    return result;
}
const GeometryNamedShape& named(const GeometryFeatureBuild& b, const TopoDS_Shape& s) {
    const auto it = std::find_if(b.named.begin(), b.named.end(),
                                 [&](const auto& n) { return n.shape.IsSame(s); });
    if (it == b.named.end())
        throw std::invalid_argument("GEOMETRY_INPUT_NAMING_INCOMPLETE");
    return *it;
}
TopoDS_Edge single_edge(const GeometryFeatureBuild& b) {
    const auto e = members(b.shape, TopAbs_EDGE);
    if (e.size() != 1)
        throw std::invalid_argument("SINGLE_CURVE_REQUIRED");
    return TopoDS::Edge(e.front());
}
bool internally_g1(const TopoDS_Edge& edge, double linear_tol, double angular_tol,
                   const std::function<bool()>& cancelled) {
    BRepAdaptor_Curve curve(edge);
    const int count = curve.NbIntervals(GeomAbs_C1);
    if (count > 4096)
        throw std::invalid_argument("GEOMETRY_INPUT_BUDGET");
    if (count <= 1)
        return true;
    TColStd_Array1OfReal spans(1, count + 1);
    curve.Intervals(spans, GeomAbs_C1);
    for (int i = 2; i <= count; ++i) {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
        const double u = spans(i);
        const auto left = curve.Trim(spans(i - 1), u, linear_tol),
                   right = curve.Trim(u, spans(i + 1), linear_tol);
        gp_Pnt a, b;
        gp_Vec ta, tb;
        left->D1(u, a, ta);
        right->D1(u, b, tb);
        if (a.Distance(b) > linear_tol || ta.Magnitude() <= 1e-10 || tb.Magnitude() <= 1e-10)
            return false;
        ta.Normalize();
        tb.Normalize();
        if (ta.Dot(tb) <= 0 || ta.Crossed(tb).Magnitude() > angular_tol)
            return false;
    }
    return true;
}
double fraction_parameter(const TopoDS_Edge& e, double fraction) {
    if (!std::isfinite(fraction) || fraction < 0 || fraction > 1)
        throw std::invalid_argument("CURVE_FRACTION_OUT_OF_RANGE");
    BRepAdaptor_Curve c(e);
    const auto start = e.Orientation() == TopAbs_REVERSED ? c.LastParameter() : c.FirstParameter();
    const auto end = e.Orientation() == TopAbs_REVERSED ? c.FirstParameter() : c.LastParameter();
    if (fraction == 0)
        return start;
    if (fraction == 1)
        return end;
    const auto length =
        GCPnts_AbscissaPoint::Length(c, c.FirstParameter(), c.LastParameter(), tolerance);
    GCPnts_AbscissaPoint at(tolerance, c, (end > start ? 1 : -1) * length * fraction, start);
    if (!at.IsDone())
        throw std::invalid_argument("CURVE_ARC_LENGTH_FAILED");
    return at.Parameter();
}
gp_Pnt curve_point(const TopoDS_Edge& e, double fraction) {
    BRepAdaptor_Curve c(e);
    return c.Value(fraction_parameter(e, fraction));
}
TopoDS_Edge sketch_edge(const ProfileCurveSpec& source, const GeometrySketchWire& frame) {
    const auto c = canonicalize_sketch_curve(source);
    const auto o = point(frame.origin);
    const gp_Ax2 axis(o, direction(frame.normal), direction(frame.u_direction));
    const auto p = [&](const Vec2& v) {
        return o.Translated(gp_Vec(axis.XDirection()) * v.x + gp_Vec(axis.YDirection()) * v.y);
    };
    TopoDS_Edge edge;
    if (c.kind == "LINE")
        edge = BRepBuilderAPI_MakeEdge(p(c.start), p(c.end)).Edge();
    else if (c.kind == "CIRCLE" || c.kind == "ARC") {
        Handle(Geom_Circle) curve =
            new Geom_Circle(gp_Ax2(p(c.center), axis.Direction(), axis.XDirection()), c.radius);
        edge = c.kind == "CIRCLE"
                   ? BRepBuilderAPI_MakeEdge(curve).Edge()
                   : BRepBuilderAPI_MakeEdge(curve, c.start_angle, c.end_angle).Edge();
    } else if (c.kind == "ELLIPSE" || c.kind == "ELLIPTICAL_ARC") {
        gp_Dir x = axis.XDirection();
        x.Rotate(gp_Ax1(o, axis.Direction()), c.rotation);
        Handle(Geom_Ellipse) curve = new Geom_Ellipse(gp_Ax2(p(c.center), axis.Direction(), x),
                                                      c.major_radius, c.minor_radius);
        edge = c.kind == "ELLIPSE"
                   ? BRepBuilderAPI_MakeEdge(curve).Edge()
                   : BRepBuilderAPI_MakeEdge(curve, c.start_angle, c.end_angle).Edge();
    } else if (c.kind == "SPLINE") {
        TColgp_Array1OfPnt poles(1, static_cast<int>(c.poles.size()));
        TColStd_Array1OfReal weights(1, poles.Length()), knots(1, static_cast<int>(c.knots.size()));
        TColStd_Array1OfInteger mult(1, knots.Length());
        for (int i = 1; i <= poles.Length(); ++i) {
            poles(i) = p(c.poles[static_cast<size_t>(i - 1)]);
            weights(i) = c.weights.empty() ? 1 : c.weights[static_cast<size_t>(i - 1)];
        }
        for (int i = 1; i <= knots.Length(); ++i) {
            knots(i) = c.knots[static_cast<size_t>(i - 1)];
            mult(i) = static_cast<int>(c.multiplicities[static_cast<size_t>(i - 1)]);
        }
        Handle(Geom_BSplineCurve) curve = new Geom_BSplineCurve(
            poles, weights, knots, mult, static_cast<int>(c.degree), c.periodic);
        edge = BRepBuilderAPI_MakeEdge(curve, c.parameter_start, c.parameter_end).Edge();
    } else
        throw std::invalid_argument("SKETCH_WIRE_CURVE_UNSUPPORTED");
    if (c.reversed)
        edge.Reverse();
    return edge;
}
void validate_result(const GeometryFeatureBuild& b, const std::string& op) {
    if (b.shape.IsNull() || !BRepCheck_Analyzer(b.shape).IsValid())
        throw std::invalid_argument("GEOMETRY_RESULT_INVALID");
    Bnd_Box bounds;
    BRepBndLib::Add(b.shape, bounds);
    if (bounds.IsVoid() || bounds.IsWhole() || bounds.IsOpenXmin() || bounds.IsOpenXmax() ||
        bounds.IsOpenYmin() || bounds.IsOpenYmax() || bounds.IsOpenZmin() || bounds.IsOpenZmax())
        throw std::invalid_argument("GEOMETRY_RESULT_UNBOUNDED");
    Standard_Real x0, y0, z0, x1, y1, z1;
    bounds.Get(x0, y0, z0, x1, y1, z1);
    for (const auto value : {x0, y0, z0, x1, y1, z1})
        if (!std::isfinite(value))
            throw std::invalid_argument("GEOMETRY_RESULT_UNBOUNDED");
    const auto solids = members(b.shape, TopAbs_SOLID), faces = members(b.shape, TopAbs_FACE),
               edges = members(b.shape, TopAbs_EDGE), vertices = members(b.shape, TopAbs_VERTEX);
    if (op == "THICKEN" || op == "CLOSE_SURFACE") {
        BRepAlgoAPI_Check check(b.shape, Standard_True, Standard_True);
        if (!check.IsValid()) {
            std::string statuses;
            std::set<int> unique;
            for (const auto& failure : check.Result())
                unique.insert(failure.GetCheckStatus());
            for (const auto status : unique)
                statuses += "/" + std::to_string(status);
            throw std::invalid_argument("THICKEN_SELF_INTERSECTION_OR_INVALID_SOLID" + statuses);
        }
        GProp_GProps p;
        BRepGProp::VolumeProperties(b.shape, p);
        if (solids.empty() || !std::isfinite(p.Mass()) || p.Mass() <= 1e-9)
            throw std::invalid_argument("THICKEN_REQUIRES_VALID_SOLID");
        for (auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX})
            if (TopExp_Explorer(b.shape, type, TopAbs_SOLID).More())
                throw std::invalid_argument("THICKEN_LOOSE_TOPOLOGY");
    } else if (op != "IMPORTED" && !solids.empty())
        throw std::invalid_argument("GEOMETRY_RESULT_DOMAIN_MISMATCH");
    if (faces.empty() && edges.empty() && vertices.empty())
        throw std::invalid_argument("GEOMETRY_RESULT_EMPTY");
    if (op != "THICKEN" && op != "CLOSE_SURFACE" && (!faces.empty() || !edges.empty())) {
        GProp_GProps measure;
        if (!faces.empty())
            BRepGProp::SurfaceProperties(b.shape, measure);
        else
            BRepGProp::LinearProperties(b.shape, measure);
        if (!std::isfinite(measure.Mass()) || measure.Mass() <= tolerance * tolerance)
            throw std::invalid_argument("GEOMETRY_RESULT_DEGENERATE");
    }
    if (faces.size() + edges.size() + vertices.size() > 100000)
        throw std::invalid_argument("GEOMETRY_TOPOLOGY_BUDGET");
}
}  // namespace geometry_features
using namespace geometry_features;

GeometryFeatureBuild read_geometry_input(const GeometryFeatureInput& input) {
    GeometryFeatureBuild out;
    if (input.plane) {
        const auto& p = *input.plane;
        if (p.id.empty())
            throw std::invalid_argument("GEOMETRY_PLANE_ID_REQUIRED");
        out.shape = BRepBuilderAPI_MakeFace(gp_Pln(gp_Ax3(point(p.origin), direction(p.normal),
                                                          direction(p.u_direction))))
                        .Face();
        add(out, {p.id, "datum.plane", {p.id}}, out.shape);
        return out;
    }
    if (input.source.brep.empty() || input.source.feature_id.empty())
        throw std::invalid_argument("GEOMETRY_INPUT_SNAPSHOT_REQUIRED");
    BRep_Builder builder;
    std::istringstream stream(std::string(input.source.brep.begin(), input.source.brep.end()));
    BRepTools::Read(out.shape, stream, builder);
    if (out.shape.IsNull() || stream.bad())
        throw std::invalid_argument("GEOMETRY_INPUT_BREP_INVALID");
    std::map<TopAbs_ShapeEnum, TopTools_IndexedMapOfShape> maps;
    for (auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX})
        TopExp::MapShapes(out.shape, type, maps[type]);
    std::set<std::string> identities, locators;
    for (const auto& n : input.source.topology) {
        const auto type = n.type == TopologyType::FACE     ? TopAbs_FACE
                          : n.type == TopologyType::EDGE   ? TopAbs_EDGE
                          : n.type == TopologyType::VERTEX ? TopAbs_VERTEX
                                                           : TopAbs_SHAPE;
        const auto r = ref({n.feature_id, n.output_slot, n.source_ids});
        if (type == TopAbs_SHAPE || n.local_id == 0 ||
            n.local_id > static_cast<uint64_t>(maps[type].Extent()) ||
            !identities.insert(key(r)).second ||
            !locators.insert(std::to_string(type) + "/" + std::to_string(n.local_id)).second)
            throw std::invalid_argument("GEOMETRY_INPUT_NAMING_INVALID");
        out.named.push_back({r, maps[type](static_cast<int>(n.local_id)), {r}});
    }
    size_t expected = 0;
    for (const auto& m : maps)
        expected += static_cast<size_t>(m.second.Extent());
    if (out.named.size() != expected)
        throw std::invalid_argument("GEOMETRY_INPUT_NAMING_INCOMPLETE");
    if (!input.members.empty()) {
        std::vector<TopoDS_Shape> chosen;
        std::set<std::string> unique;
        for (const auto& member : input.members) {
            const auto k = key(ref(member));
            if (!unique.insert(k).second)
                throw std::invalid_argument("GEOMETRY_INPUT_DUPLICATE_MEMBER");
            const auto it = std::find_if(out.named.begin(), out.named.end(),
                                         [&](const auto& n) { return key(n.ref) == k; });
            if (it == out.named.end())
                throw std::invalid_argument("GEOMETRY_INPUT_MEMBER_MISSING");
            chosen.push_back(it->shape);
        }
        out.shape = compound(chosen);
        out.named.erase(
            std::remove_if(out.named.begin(), out.named.end(),
                           [&](const auto& n) { return !contains(out.shape, n.shape); }),
            out.named.end());
    }
    if (input.reversed)
        out.shape.Reverse();
    return out;
}

void complete_geometry_naming(GeometryFeatureBuild& b, const std::string& id) {
    for (auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX}) {
        std::map<std::string,
                 std::vector<std::pair<TopoDS_Shape, std::vector<SemanticTopologyRef>>>>
            groups;
        for (const auto& s : members(b.shape, type)) {
            if (std::any_of(b.named.begin(), b.named.end(),
                            [&](const auto& n) { return n.shape.IsSame(s); }))
                continue;
            std::vector<SemanticTopologyRef> sources;
            for (const auto& n : b.named)
                if ((type == TopAbs_FACE && n.shape.ShapeType() == TopAbs_EDGE &&
                     contains(s, n.shape)) ||
                    (type == TopAbs_EDGE && n.shape.ShapeType() == TopAbs_FACE &&
                     contains(n.shape, s)) ||
                    (type == TopAbs_VERTEX && n.shape.ShapeType() == TopAbs_EDGE &&
                     contains(n.shape, s)))
                    sources.push_back(n.ref);
            if (sources.empty())
                throw std::invalid_argument("GEOMETRY_HISTORY_INCOMPLETE");
            groups[signature(sources)].push_back({s, sources});
        }
        for (const auto& group : groups) {
            if (group.second.size() != 1)
                throw std::invalid_argument(
                    "GEOMETRY_HISTORY_BRANCH_AMBIGUOUS:" + std::to_string(type) + "/" +
                    std::to_string(group.second.size()));
            const auto& p = group.second.front();
            add(b, derived(id, "boundary/" + std::to_string(type), p.second), p.first, p.second);
        }
    }
    std::set<std::string> ids;
    for (const auto& n : b.named)
        if (!ids.insert(key(n.ref)).second)
            throw std::invalid_argument("GEOMETRY_HISTORY_DUPLICATE_IDENTITY:" + n.ref.output_slot);
}

GeometryFeatureBuild build_geometry_feature(const GeometryFeatureSpec& s,
                                            const std::function<bool()>& cancelled) try {
    if (s.feature_id.empty() || (s.result_id != "result" && s.result_id != "wire") ||
        s.inputs.size() > 128 || s.curve.points.size() > 4096)
        throw std::invalid_argument("GEOMETRY_DEFINITION_INVALID");
    const auto check = [&]() {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
    };
    check();
    if (s.operation == "IMPORTED") {
        if (!s.import_seed || s.import_seed->feature_id != s.feature_id ||
            s.import_seed->result_id != s.result_id || !s.import_seed->body_id.empty() ||
            !s.inputs.empty() ||
            make_geometry_id(s.import_brep.data(), s.import_brep.size()) !=
                "sha256:" + s.import_seed->brep_sha256)
            throw std::invalid_argument("IMPORT_SEED_SNAPSHOT_MISMATCH");
        GeometryFeatureInput input;
        input.source.feature_id = s.feature_id;
        input.source.brep = s.import_brep;
        for (const auto& n : s.import_seed->identities) {
            if (n.stable_id.empty())
                throw std::invalid_argument("IMPORT_SEED_INVALID_IDENTITY_MAP");
            const auto type = n.topology_type == PersistentTopologyType::face   ? TopologyType::FACE
                              : n.topology_type == PersistentTopologyType::edge ? TopologyType::EDGE
                              : n.topology_type == PersistentTopologyType::vertex
                                  ? TopologyType::VERTEX
                                  : TopologyType::COMPOUND;
            input.source.topology.push_back(
                {s.feature_id, "import.topology", {n.stable_id}, type, n.local_id});
        }
        auto out = read_geometry_input(input);
        validate_result(out, s.operation);
        complete_geometry_naming(out, s.feature_id);
        check();
        return out;
    }
    const auto& p = s.parameters;
    const double construction_tolerance = s.construction_tolerance;
    if (!std::isfinite(construction_tolerance) || construction_tolerance < 1e-7 ||
        construction_tolerance > 1e-2)
        throw std::invalid_argument("GEOMETRY_TOLERANCE_OUT_OF_RANGE");
    for (double v : {p.distance, p.angle, p.start, p.end, p.tension})
        if (!std::isfinite(v))
            throw std::invalid_argument("GEOMETRY_NONFINITE_PARAMETER");
    std::vector<GeometryFeatureBuild> inputs;
    for (const auto& input : s.inputs) {
        check();
        inputs.push_back(read_geometry_input(input));
    }
    if (!s.sketch_wire.curves.empty()) {
        GeometryFeatureBuild b;
        std::vector<TopoDS_Shape> es;
        for (const auto& c : s.sketch_wire.curves) {
            if (c.entity_id.empty())
                throw std::invalid_argument("SKETCH_CURVE_ID_REQUIRED");
            es.push_back(sketch_edge(c, s.sketch_wire));
        }
        b.shape = compound(es);
        for (size_t i = 0; i < es.size(); ++i) {
            const auto& c = s.sketch_wire.curves[i];
            const SemanticTopologyRef r{
                s.sketch_wire.sketch_id, "SKETCH_EDGE/" + c.entity_id, {c.entity_id}};
            add(b, r, es[i]);
            endpoints(b, s.sketch_wire.sketch_id, TopoDS::Edge(es[i]),
                      "SKETCH_VERTEX/" + c.entity_id, {r});
        }
        inputs.insert(inputs.begin(), std::move(b));
    }
    GeometryFeatureBuild out;
    const auto require = [&](size_t n) {
        if (inputs.size() != n)
            throw std::invalid_argument("GEOMETRY_INPUT_COUNT_INVALID");
    };
    if (is_surface_feature(s.operation)) {
        out = build_surface_feature(s, inputs, cancelled);
    } else if (s.operation == "LINE" || s.operation == "CIRCLE" || s.operation == "ARC" ||
               s.operation == "SPLINE") {
        require(0);
        std::set<std::string> ids;
        std::vector<SemanticTopologyRef> sources;
        for (const auto& v : s.curve.points) {
            if (v.id.empty() || !ids.insert(v.id).second)
                throw std::invalid_argument("CURVE_CONTROL_IDENTITY_INVALID");
            point(v.position);
            sources.push_back({s.feature_id, "control/" + v.id, {v.id}});
            for (const auto& source : v.sources)
                sources.push_back(ref(source));
        }
        TopoDS_Edge edge;
        if (s.operation == "LINE") {
            if (s.curve.points.size() != 2)
                throw std::invalid_argument("LINE_REQUIRES_TWO_POINTS");
            if (point(s.curve.points[0].position).Distance(point(s.curve.points[1].position)) <=
                tolerance)
                throw std::invalid_argument("LINE_POINTS_COINCIDENT");
            edge = BRepBuilderAPI_MakeEdge(point(s.curve.points[0].position),
                                           point(s.curve.points[1].position))
                       .Edge();
        } else if (s.operation == "CIRCLE" || s.operation == "ARC") {
            if (s.curve.points.size() != 1 || !std::isfinite(s.curve.radius) ||
                s.curve.radius <= tolerance)
                throw std::invalid_argument("CIRCLE_DEFINITION_INVALID");
            Handle(Geom_Circle) c =
                new Geom_Circle(gp_Ax2(point(s.curve.points[0].position), direction(s.curve.normal),
                                       direction(s.curve.x_direction)),
                                s.curve.radius);
            if (s.operation == "CIRCLE")
                edge = BRepBuilderAPI_MakeEdge(c).Edge();
            else {
                const double angle = s.curve.end_angle - s.curve.start_angle;
                if (!std::isfinite(angle) || angle <= 1e-9 || angle > 2 * pi + 1e-9)
                    throw std::invalid_argument("ARC_RANGE_INVALID");
                edge = BRepBuilderAPI_MakeEdge(c, s.curve.start_angle, s.curve.end_angle).Edge();
            }
        } else {
            if (s.curve.points.size() < 2)
                throw std::invalid_argument("SPLINE_POINTS_REQUIRED");
            if (s.curve.knots.empty()) {
                Handle(TColgp_HArray1OfPnt) points =
                    new TColgp_HArray1OfPnt(1, static_cast<int>(s.curve.points.size()));
                for (int i = 1; i <= points->Length(); ++i)
                    points->SetValue(i, point(s.curve.points[static_cast<size_t>(i - 1)].position));
                GeomAPI_Interpolate fit(points, s.curve.closed, construction_tolerance);
                fit.Perform();
                if (!fit.IsDone())
                    throw std::invalid_argument("SPLINE_INTERPOLATION_FAILED");
                edge = BRepBuilderAPI_MakeEdge(fit.Curve()).Edge();
            } else {
                if (s.curve.degree < 1 || s.curve.degree > 25 ||
                    s.curve.knots.size() != s.curve.multiplicities.size() ||
                    s.curve.knots.size() > 4096 ||
                    (!s.curve.weights.empty() && s.curve.weights.size() != s.curve.points.size()))
                    throw std::invalid_argument("SPLINE_DEFINITION_INVALID");
                TColgp_Array1OfPnt poles(1, static_cast<int>(s.curve.points.size()));
                TColStd_Array1OfReal weights(1, poles.Length()),
                    knots(1, static_cast<int>(s.curve.knots.size()));
                TColStd_Array1OfInteger mult(1, knots.Length());
                for (int i = 1; i <= poles.Length(); ++i) {
                    poles(i) = point(s.curve.points[static_cast<size_t>(i - 1)].position);
                    weights(i) =
                        s.curve.weights.empty() ? 1 : s.curve.weights[static_cast<size_t>(i - 1)];
                    if (!std::isfinite(weights(i)) || weights(i) <= 0)
                        throw std::invalid_argument("SPLINE_WEIGHT_INVALID");
                }
                for (int i = 1; i <= knots.Length(); ++i) {
                    knots(i) = s.curve.knots[static_cast<size_t>(i - 1)];
                    mult(i) = static_cast<int>(s.curve.multiplicities[static_cast<size_t>(i - 1)]);
                }
                Handle(Geom_BSplineCurve) c = new Geom_BSplineCurve(
                    poles, weights, knots, mult, static_cast<int>(s.curve.degree), s.curve.closed);
                edge = BRepBuilderAPI_MakeEdge(c).Edge();
            }
        }
        curve_result(out, s.feature_id, edge, sources);
    } else if (s.operation == "POINT_ON_CURVE") {
        require(1);
        const auto e = single_edge(inputs[0]);
        out.shape = BRepBuilderAPI_MakeVertex(curve_point(e, p.start)).Vertex();
        add(out, derived(s.feature_id, "point", {named(inputs[0], e).ref}), out.shape,
            {named(inputs[0], e).ref});
    } else if (s.operation == "EXTRACT") {
        require(1);
        out.shape = inputs[0].shape;
        for (const auto& n : inputs[0].named)
            add(out, derived(s.feature_id, "extract", {n.ref}), n.shape, {n.ref});
    } else if (s.operation == "BOUNDARY") {
        require(1);
        const auto fs = members(inputs[0].shape, TopAbs_FACE);
        if (fs.empty())
            throw std::invalid_argument("BOUNDARY_REQUIRES_SURFACE");
        std::vector<TopoDS_Shape> edges;
        for (const auto& e : members(inputs[0].shape, TopAbs_EDGE)) {
            int count = 0;
            for (const auto& f : fs)
                if (contains(f, e)) {
                    if (BRep_Tool::IsClosed(TopoDS::Edge(e), TopoDS::Face(f)))
                        count += 2;
                    else
                        ++count;
                }
            if (count == 1 && !BRep_Tool::Degenerated(TopoDS::Edge(e)))
                edges.push_back(e);
        }
        if (edges.empty())
            throw std::invalid_argument("BOUNDARY_EMPTY");
        out.shape = compound(edges);
        for (const auto& n : inputs[0].named)
            if (contains(out.shape, n.shape))
                add(out, derived(s.feature_id, "boundary", {n.ref}), n.shape, {n.ref});
    } else if (s.operation == "COMPOSITE") {
        if (inputs.empty())
            throw std::invalid_argument("COMPOSITE_INPUT_REQUIRED");
        BRepBuilderAPI_MakeWire builder;
        struct Contribution {
            TopoDS_Edge original, result;
            const GeometryFeatureBuild* source;
        };
        std::vector<Contribution> contributions;
        for (const auto& source : inputs) {
            if (!members(source.shape, TopAbs_FACE).empty())
                throw std::invalid_argument("COMPOSITE_REQUIRES_CURVES");
            for (const auto& edge : members(source.shape, TopAbs_EDGE)) {
                builder.Add(TopoDS::Edge(edge));
                if (!builder.IsDone())
                    throw std::invalid_argument("CURVE_CHAIN_DISCONNECTED_OR_BRANCHING");
                contributions.push_back({TopoDS::Edge(edge), builder.Edge(), &source});
            }
        }
        if (contributions.empty() || !builder.IsDone())
            throw std::invalid_argument("COMPOSITE_INPUT_REQUIRED");
        out.shape = builder.Wire();
        size_t traversed = 0;
        for (BRepTools_WireExplorer it(builder.Wire()); it.More(); it.Next())
            ++traversed;
        if (traversed != contributions.size())
            throw std::invalid_argument("CURVE_CHAIN_DISCONNECTED_OR_BRANCHING");
        for (const auto& c : contributions) {
            const auto& edge_ref = named(*c.source, c.original).ref;
            add(out, derived(s.feature_id, "composite/edge", {edge_ref}), c.result, {edge_ref});
            TopoDS_Vertex oa, oz, ra, rz;
            TopExp::Vertices(c.original, oa, oz, Standard_False);
            TopExp::Vertices(c.result, ra, rz, Standard_False);
            for (const auto& pair :
                 std::vector<std::pair<TopoDS_Vertex, TopoDS_Vertex>>{{oa, ra}, {oz, rz}}) {
                const auto& source = named(*c.source, pair.first).ref;
                add(out, derived(s.feature_id, "composite/vertex", {source}), pair.second,
                    {source});
            }
        }
    } else if (s.operation == "SURFACE_REVOLVE") {
        require(1);
        if (!members(inputs[0].shape, TopAbs_FACE).empty() ||
            members(inputs[0].shape, TopAbs_EDGE).empty())
            throw std::invalid_argument("SURFACE_REVOLVE_REQUIRES_CURVES");
        const double full = 2 * std::acos(-1.0);
        if (std::abs(p.angle) <= 1e-10 || std::abs(p.angle) > full + 1e-10)
            throw std::invalid_argument("SURFACE_REVOLVE_ANGLE_INVALID");
        BRepPrimAPI_MakeRevol op(inputs[0].shape, gp_Ax1(point(p.origin), direction(p.direction)),
                                 p.angle, Standard_True);
        op.Build();
        check();
        if (!op.IsDone() || op.Shape().IsNull())
            throw std::invalid_argument("SURFACE_REVOLVE_FAILED");
        out.shape = op.Shape();
        history(out, inputs[0], op, s.feature_id, "revolve");
        if (std::abs(p.angle) < full - 1e-10)
            for (const auto& n : inputs[0].named) {
                add(out, derived(s.feature_id, "revolve/start", {n.ref}), op.FirstShape(n.shape),
                    {n.ref});
                add(out, derived(s.feature_id, "revolve/end", {n.ref}), op.LastShape(n.shape),
                    {n.ref});
            }
    } else if (s.operation == "SURFACE_EXTRUDE") {
        require(1);
        if (!members(inputs[0].shape, TopAbs_FACE).empty())
            throw std::invalid_argument("SURFACE_EXTRUDE_REQUIRES_CURVES");
        if (std::abs(p.distance) <= tolerance)
            throw std::invalid_argument("EXTRUSION_LENGTH_REQUIRED");
        const gp_Vec delta = gp_Vec(direction(p.direction)) * p.distance;
        std::vector<TopoDS_Shape> shapes;
        for (const auto& e : members(inputs[0].shape, TopAbs_EDGE)) {
            check();
            BRepPrimAPI_MakePrism prism(e, delta, Standard_True);
            prism.Build();
            if (!prism.IsDone())
                throw std::invalid_argument("SURFACE_EXTRUSION_FAILED");
            GeometryFeatureBuild part;
            part.shape = prism.Shape();
            const auto r = named(inputs[0], e).ref;
            add(part, derived(s.feature_id, "surface", {r}), part.shape, {r});
            GeometryFeatureBuild edge_source;
            edge_source.shape = e;
            for (const auto& n : inputs[0].named)
                if (contains(e, n.shape))
                    edge_source.named.push_back(n);
            history(part, edge_source, prism, s.feature_id, "extrude");
            for (const auto& n : edge_source.named) {
                const auto a = prism.FirstShape(n.shape), z = prism.LastShape(n.shape);
                add(part, derived(s.feature_id, "extrude/start", {n.ref}), a, {n.ref});
                add(part, derived(s.feature_id, "extrude/end", {n.ref}), z, {n.ref});
            }
            shapes.push_back(part.shape);
            out.named.insert(out.named.end(), part.named.begin(), part.named.end());
        }
        out.shape = compound(shapes);
        if (shapes.size() > 1) {
            BRepBuilderAPI_Sewing sew(construction_tolerance, Standard_True, Standard_True,
                                      Standard_True, Standard_False);
            for (const auto& shape : shapes)
                sew.Add(shape);
            sew.Perform();
            GeometryFeatureBuild sewn;
            sewn.shape = sew.SewedShape();
            for (const auto& n : out.named) {
                auto mapped =
                    sew.IsModifiedSubShape(n.shape) ? sew.ModifiedSubShape(n.shape) : n.shape;
                add(sewn, n.ref, mapped, n.sources, n.roles);
            }
            out = std::move(sewn);
        }
    } else if (s.operation == "THICKEN") {
        require(1);
        if (!members(inputs[0].shape, TopAbs_SOLID).empty() ||
            members(inputs[0].shape, TopAbs_FACE).empty() || std::abs(p.distance) <= tolerance)
            throw std::invalid_argument("THICKEN_REQUIRES_SURFACE_AND_THICKNESS");
        BRepOffsetAPI_MakeThickSolid thick;
        BRepOffset_MakeOffset joined;
        const bool multi_face = members(inputs[0].shape, TopAbs_FACE).size() > 1;
        if (multi_face) {
            // G0 face junctions need real intersecting offset walls. The
            // simple algorithm does not compute intersections at corners.
            joined.Initialize(inputs[0].shape, p.distance, tolerance, BRepOffset_Skin,
                              Standard_False, Standard_False, GeomAbs_Intersection, Standard_True);
            joined.MakeOffsetShape();
            if (!joined.IsDone())
                throw std::invalid_argument("THICKEN_FAILED");
            out.shape = joined.Shape();
        } else {
            thick.MakeThickSolidBySimple(inputs[0].shape, p.distance);
            if (!thick.IsDone())
                throw std::invalid_argument("THICKEN_FAILED");
            out.shape = thick.Shape();
        }
        auto solids = members(out.shape, TopAbs_SOLID);
        if (solids.empty())
            for (const auto& shell : members(out.shape, TopAbs_SHELL)) {
                if (!BRep_Tool::IsClosed(shell))
                    throw std::invalid_argument("THICKEN_OPEN_SHELL");
                BRepBuilderAPI_MakeSolid make(TopoDS::Shell(shell));
                if (!make.IsDone())
                    throw std::invalid_argument("THICKEN_SOLID_FAILED");
                solids.push_back(make.Solid());
            }
        std::vector<TopoDS_Shape> oriented;
        for (const auto& shape : solids) {
            auto solid = TopoDS::Solid(shape);
            if (!BRepLib::OrientClosedSolid(solid))
                throw std::invalid_argument("THICKEN_ORIENTATION_FAILED");
            // Keep a forward Solid with the effective shell orientations. A
            // reversed root has positive mass, but OCCT modifiers rebuild the
            // root and can otherwise expose its inside-out shell orientation.
            BRep_Builder builder;
            TopoDS_Solid canonical;
            builder.MakeSolid(canonical);
            for (TopoDS_Iterator child(solid); child.More(); child.Next())
                builder.Add(canonical, child.Value());
            oriented.push_back(canonical);
        }
        out.shape = compound(oriented);
        if (multi_face)
            history(out, inputs[0], joined, s.feature_id, "thicken");
        else
            history(out, inputs[0], thick, s.feature_id, "thicken");
    } else if (s.operation == "TRANSLATE" || s.operation == "ROTATE" || s.operation == "MIRROR") {
        require(1);
        gp_Trsf tr;
        if (s.operation == "TRANSLATE")
            tr.SetTranslation(gp_Vec(direction(p.direction)) * p.distance);
        else if (s.operation == "ROTATE")
            tr.SetRotation(gp_Ax1(point(p.origin), direction(p.direction)), p.angle);
        else
            tr.SetMirror(gp_Ax2(point(p.origin), direction(p.direction)));
        BRepBuilderAPI_Transform move(inputs[0].shape, tr, Standard_True);
        if (!move.IsDone())
            throw std::invalid_argument("GEOMETRY_TRANSFORM_FAILED");
        out.shape = move.Shape();
        history(out, inputs[0], move, s.feature_id, "transform");
    } else if (s.operation == "SPLIT" || s.operation == "TRIM" || s.operation == "EXTEND") {
        require(1);
        const auto edge = single_edge(inputs[0]);
        const auto r = named(inputs[0], edge).ref;
        Standard_Real first, last;
        const auto c = BRep_Tool::Curve(edge, first, last);
        if (c.IsNull())
            throw std::invalid_argument("CURVE_GEOMETRY_UNAVAILABLE");
        if (s.operation == "SPLIT") {
            if (p.start <= 0 || p.start >= 1)
                throw std::invalid_argument("SPLIT_INTERIOR_POINT_REQUIRED");
            const auto at = fraction_parameter(edge, p.start);
            const auto start = fraction_parameter(edge, 0), end = fraction_parameter(edge, 1);
            std::vector<TopoDS_Shape> shapes;
            for (const auto& range : std::vector<std::pair<std::string, std::pair<double, double>>>{
                     {"before", {start, at}}, {"after", {at, end}}}) {
                auto e =
                    BRepBuilderAPI_MakeEdge(c, std::min(range.second.first, range.second.second),
                                            std::max(range.second.first, range.second.second))
                        .Edge();
                if (start > end)
                    e.Reverse();
                shapes.push_back(e);
                GeometryFeatureBuild part;
                curve_result(part, s.feature_id, e, {r}, "split/" + range.first);
                out.named.insert(out.named.end(), part.named.begin(), part.named.end());
            }
            out.shape = compound(shapes);
        } else if (s.operation == "TRIM") {
            if (p.start >= p.end)
                throw std::invalid_argument("TRIM_INTERVAL_INVALID");
            const auto a = fraction_parameter(edge, p.start), z = fraction_parameter(edge, p.end);
            auto e = BRepBuilderAPI_MakeEdge(c, std::min(a, z), std::max(a, z)).Edge();
            if (a > z)
                e.Reverse();
            curve_result(out, s.feature_id, e, {r}, "trim");
        } else {
            if (p.distance <= tolerance ||
                (p.side != "START" && p.side != "END" && p.side != "BOTH"))
                throw std::invalid_argument("EXTEND_DEFINITION_INVALID");
            BRepAdaptor_Curve a(edge);
            double delta;
            if (a.GetType() == GeomAbs_Line)
                delta = p.distance;
            else if (a.GetType() == GeomAbs_Circle)
                delta = p.distance / a.Circle().Radius();
            else
                throw std::invalid_argument("EXTEND_SUPPORTS_LINE_AND_CIRCLE");
            const bool reverse = edge.Orientation() == TopAbs_REVERSED;
            const bool before =
                p.side == "BOTH" || (p.side == "START" && !reverse) || (p.side == "END" && reverse);
            const bool after =
                p.side == "BOTH" || (p.side == "END" && !reverse) || (p.side == "START" && reverse);
            if (before)
                first -= delta;
            if (after)
                last += delta;
            if (a.GetType() == GeomAbs_Circle && last - first > 2 * pi + tolerance)
                throw std::invalid_argument("EXTEND_CIRCLE_RANGE_INVALID");
            auto e = BRepBuilderAPI_MakeEdge(c, first, last).Edge();
            if (reverse)
                e.Reverse();
            curve_result(out, s.feature_id, e, {r}, "extend");
        }
    } else if (s.operation == "G1_CONNECT") {
        require(2);
        const auto first = single_edge(inputs[0]), last = single_edge(inputs[1]);
        const auto a = curve_point(first, 1), z = curve_point(last, 0);
        const auto span = a.Distance(z);
        if (span <= tolerance || p.tension <= 0 || p.tension > 10)
            throw std::invalid_argument("CONNECT_DEFINITION_INVALID");
        BRepAdaptor_Curve ca(first), cz(last);
        gp_Pnt ignored;
        gp_Vec ta, tz;
        ca.D1(fraction_parameter(first, 1), ignored, ta);
        cz.D1(fraction_parameter(last, 0), ignored, tz);
        if (first.Orientation() == TopAbs_REVERSED)
            ta.Reverse();
        if (last.Orientation() == TopAbs_REVERSED)
            tz.Reverse();
        if (ta.Magnitude() <= tolerance || tz.Magnitude() <= tolerance)
            throw std::invalid_argument("CONNECT_TANGENT_UNAVAILABLE");
        ta.Normalize();
        tz.Normalize();
        TColgp_Array1OfPnt poles(1, 4);
        poles(1) = a;
        poles(2) = a.Translated(ta * (span * p.tension / 3));
        poles(3) = z.Translated(tz * (-span * p.tension / 3));
        poles(4) = z;
        Handle(Geom_BezierCurve) c = new Geom_BezierCurve(poles);
        curve_result(out, s.feature_id, BRepBuilderAPI_MakeEdge(c).Edge(),
                     {named(inputs[0], first).ref, named(inputs[1], last).ref}, "g1.connect");
    } else if (s.operation == "OFFSET") {
        require(2);
        if (!s.inputs.back().plane)
            throw std::invalid_argument("OFFSET_PLANE_REQUIRED");
        if (std::abs(p.distance) <= tolerance)
            throw std::invalid_argument("OFFSET_DISTANCE_REQUIRED");
        const auto w = wire(inputs[0].shape);
        const auto plane = TopoDS::Face(inputs[1].shape);
        // Exact curve-in-plane check: build pcurves and reject edges which do
        // not lie on the declared plane; no camera- or sample-derived normal.
        const auto& frame = *s.inputs.back().plane;
        const gp_Pln support(
            gp_Ax3(point(frame.origin), direction(frame.normal), direction(frame.u_direction)));
        const auto on_plane = [&](const gp_Pnt& q) {
            if (support.Distance(q) > tolerance)
                throw std::invalid_argument("OFFSET_CURVE_NOT_PLANAR");
        };
        for (const auto& e : members(w, TopAbs_EDGE)) {
            BRepAdaptor_Curve c(TopoDS::Edge(e));
            if (c.GetType() == GeomAbs_Line) {
                on_plane(c.Value(c.FirstParameter()));
                if (std::abs(gp_Vec(c.Line().Direction()).Dot(gp_Vec(support.Axis().Direction()))) >
                    1e-9)
                    throw std::invalid_argument("OFFSET_CURVE_NOT_PLANAR");
            } else if (c.GetType() == GeomAbs_Circle) {
                on_plane(c.Circle().Location());
                if (gp_Vec(c.Circle().Axis().Direction())
                        .Crossed(gp_Vec(support.Axis().Direction()))
                        .Magnitude() > 1e-9)
                    throw std::invalid_argument("OFFSET_CURVE_NOT_PLANAR");
            } else if (c.GetType() == GeomAbs_Ellipse) {
                on_plane(c.Ellipse().Location());
                if (gp_Vec(c.Ellipse().Axis().Direction())
                        .Crossed(gp_Vec(support.Axis().Direction()))
                        .Magnitude() > 1e-9)
                    throw std::invalid_argument("OFFSET_CURVE_NOT_PLANAR");
            } else if (c.GetType() == GeomAbs_BSplineCurve) {
                const auto curve = c.BSpline();
                for (int i = 1; i <= curve->NbPoles(); ++i)
                    on_plane(curve->Pole(i));
            } else if (c.GetType() == GeomAbs_BezierCurve) {
                const auto curve = c.Bezier();
                for (int i = 1; i <= curve->NbPoles(); ++i)
                    on_plane(curve->Pole(i));
            } else
                throw std::invalid_argument("OFFSET_CURVE_FAMILY_UNSUPPORTED");
        }
        BRepOffsetAPI_MakeOffset offset(plane, GeomAbs_Intersection, Standard_True);
        offset.AddWire(w);
        offset.Perform(p.distance);
        if (!offset.IsDone())
            throw std::invalid_argument("CURVE_OFFSET_FAILED");
        out.shape = offset.Shape();
        history(out, inputs[0], offset, s.feature_id, "offset");
        // OCCT offset history can omit endpoint images. Directed endpoint
        // roles of each actually named output edge provide exact topology
        // identities; shared vertices merge all incident roles deterministically.
        GeometryFeatureBuild boundary;
        boundary.shape = out.shape;
        for (const auto& edge : members(out.shape, TopAbs_EDGE))
            endpoints(boundary, s.feature_id, TopoDS::Edge(edge), "offset", {named(out, edge).ref});
        for (const auto& vertex : boundary.named)
            if (std::none_of(out.named.begin(), out.named.end(),
                             [&](const auto& n) { return n.shape.IsSame(vertex.shape); }))
                add(out, vertex.ref, vertex.shape, vertex.sources, vertex.roles);
    } else if (s.operation == "PROJECT") {
        require(2);
        if (members(inputs[1].shape, TopAbs_FACE).empty())
            throw std::invalid_argument("PROJECTION_SUPPORT_REQUIRED");
        std::vector<TopoDS_Shape> shapes;
        for (const auto& e : members(inputs[0].shape, TopAbs_EDGE)) {
            const auto r = named(inputs[0], e).ref;
            for (const auto& f : members(inputs[1].shape, TopAbs_FACE)) {
                BRepProj_Projection project(e, f, direction(p.direction));
                if (!project.IsDone())
                    continue;
                const auto es = members(project.Shape(), TopAbs_EDGE);
                if (es.size() > 1)
                    throw std::invalid_argument("PROJECTION_BRANCH_AMBIGUOUS");
                if (es.empty())
                    continue;
                GeometryFeatureBuild part;
                curve_result(part, s.feature_id, TopoDS::Edge(es.front()),
                             {r, named(inputs[1], f).ref}, "projection");
                shapes.push_back(part.shape);
                out.named.insert(out.named.end(), part.named.begin(), part.named.end());
            }
        }
        if (shapes.empty())
            throw std::invalid_argument("PROJECTION_EMPTY");
        out.shape = compound(shapes);
    } else if (s.operation == "INTERSECTION") {
        require(2);
        BRepAlgoAPI_Section section(inputs[0].shape, inputs[1].shape, Standard_False);
        section.ComputePCurveOn1(Standard_True);
        section.ComputePCurveOn2(Standard_True);
        section.Approximation(Standard_True);
        section.Build();
        if (!section.IsDone())
            throw std::invalid_argument("INTERSECTION_FAILED");
        out.shape = section.Shape();
        std::set<std::string> branch;
        for (const auto& e : members(out.shape, TopAbs_EDGE)) {
            TopoDS_Shape first, last;
            std::vector<SemanticTopologyRef> sources;
            if (section.HasAncestorFaceOn1(e, first))
                sources.push_back(named(inputs[0], first).ref);
            if (section.HasAncestorFaceOn2(e, last))
                sources.push_back(named(inputs[1], last).ref);
            if (sources.size() != 2)
                throw std::invalid_argument("INTERSECTION_ANCESTOR_UNAVAILABLE");
            const auto k = signature(sources);
            if (!branch.insert(k).second)
                throw std::invalid_argument("INTERSECTION_BRANCH_AMBIGUOUS");
            add(out, derived(s.feature_id, "intersection", sources), e, sources);
            endpoints(out, s.feature_id, TopoDS::Edge(e), "intersection", sources);
        }
        if (members(out.shape, TopAbs_EDGE).empty())
            throw std::invalid_argument("INTERSECTION_EMPTY");
    } else
        throw std::invalid_argument("GEOMETRY_OPERATION_UNSUPPORTED");
    check();
    validate_result(out, s.operation);
    complete_geometry_naming(out, s.feature_id);
    check();
    return out;
} catch (const Standard_Failure& error) {
    throw std::invalid_argument(std::string("GEOMETRY_CONSTRUCTION_FAILED: ") +
                                error.GetMessageString());
}
}  // namespace occccad::kernel::detail

#include <BRepAdaptor_Curve.hxx>
#include <BRepAlgoAPI_Check.hxx>
#include <BRepAlgoAPI_Splitter.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeSolid.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BRepBuilderAPI_MakeWire.hxx>
#include <BRepBuilderAPI_Sewing.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepGProp.hxx>
#include <BRepLib.hxx>
#include <BRepOffsetAPI_MakeFilling.hxx>
#include <BRepOffsetAPI_MakeOffsetShape.hxx>
#include <BRepOffsetAPI_MakePipe.hxx>
#include <BRepOffsetAPI_ThruSections.hxx>
#include <BRepTools.hxx>
#include <BRepTools_WireExplorer.hxx>
#include <BRep_Tool.hxx>
#include <GProp_GProps.hxx>
#include <Geom2d_Line.hxx>
#include <GeomAbs_JoinType.hxx>
#include <GeomFill_Trihedron.hxx>
#include <GeomLib.hxx>
#include <Geom_BSplineCurve.hxx>
#include <Geom_BezierCurve.hxx>
#include <Geom_RectangularTrimmedSurface.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedDataMapOfShapeListOfShape.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <TopoDS_Vertex.hxx>
#include <gp_Circ.hxx>
#include <gp_Elips.hxx>
#include <gp_Lin.hxx>

#include <internal/geometry_feature_support.hpp>

#include <cmath>
#include <limits>
#include <queue>
#include <set>

namespace occccad::kernel::detail {
using namespace geometry_features;
namespace {
void require_sheet(const GeometryFeatureBuild& in) {
    if (members(in.shape, TopAbs_FACE).empty() || !members(in.shape, TopAbs_SOLID).empty())
        throw std::invalid_argument("SURFACE_INPUT_REQUIRED");
    if (TopExp_Explorer(in.shape, TopAbs_EDGE, TopAbs_FACE).More() ||
        TopExp_Explorer(in.shape, TopAbs_VERTEX, TopAbs_FACE).More())
        throw std::invalid_argument("SURFACE_LOOSE_TOPOLOGY");
}
void require_curve(const GeometryFeatureBuild& in) {
    if (!members(in.shape, TopAbs_FACE).empty() || members(in.shape, TopAbs_EDGE).empty())
        throw std::invalid_argument("CURVE_INPUT_REQUIRED");
}
void manifold(const TopoDS_Shape& shape, bool closed = false) {
    TopTools_IndexedDataMapOfShapeListOfShape uses;
    TopExp::MapShapesAndAncestors(shape, TopAbs_EDGE, TopAbs_FACE, uses);
    for (int i = 1; i <= uses.Extent(); ++i) {
        std::vector<TopoDS_Shape> faces;
        for (TopTools_ListIteratorOfListOfShape it(uses(i)); it.More(); it.Next())
            if (std::none_of(faces.begin(), faces.end(),
                             [&](const auto& f) { return f.IsSame(it.Value()); }))
                faces.push_back(it.Value());
        const bool seam = faces.size() == 1 && BRep_Tool::IsClosed(TopoDS::Edge(uses.FindKey(i)),
                                                                   TopoDS::Face(faces.front()));
        if (faces.size() > 2)
            throw std::invalid_argument("SURFACE_NON_MANIFOLD");
        if (closed && faces.size() != 2 && !seam &&
            !BRep_Tool::Degenerated(TopoDS::Edge(uses.FindKey(i))))
            throw std::invalid_argument("CLOSE_SURFACE_FREE_BOUNDARY");
    }
}
// Compose all real histories before assigning a role. Several source edges can
// generate the same face, while a ruled section can generate two different
// spans. Their complete source sets distinguish those spans without indices.
template <class Algorithm>
void joint_history(GeometryFeatureBuild& out, const std::vector<GeometryFeatureBuild>& inputs,
                   Algorithm& op, const std::string& id, const std::string& role) {
    struct Candidate {
        TopoDS_Shape shape;
        std::vector<SemanticTopologyRef> sources;
    };
    std::vector<Candidate> candidates;
    const auto collect = [&](const TopoDS_Shape& shape, const SemanticTopologyRef& source) {
        if (shape.IsNull() || !contains(out.shape, shape) ||
            (shape.ShapeType() != TopAbs_FACE && shape.ShapeType() != TopAbs_EDGE &&
             shape.ShapeType() != TopAbs_VERTEX))
            return;
        auto it = std::find_if(candidates.begin(), candidates.end(),
                               [&](const auto& c) { return c.shape.IsSame(shape); });
        if (it == candidates.end())
            candidates.push_back({shape, {source}});
        else if (std::none_of(it->sources.begin(), it->sources.end(),
                              [&](const auto& r) { return key(r) == key(source); }))
            it->sources.push_back(source);
    };
    for (const auto& input : inputs)
        for (const auto& n : input.named) {
            collect(n.shape, n.ref);
            for (const auto& list : {op.Modified(n.shape), op.Generated(n.shape)})
                for (TopTools_ListIteratorOfListOfShape it(list); it.More(); it.Next())
                    collect(it.Value(), n.ref);
        }
    std::map<std::string, std::vector<const Candidate*>> groups;
    for (const auto& c : candidates)
        groups[std::to_string(c.shape.ShapeType()) + "/" + signature(c.sources)].push_back(&c);
    for (const auto& [signature, group] : groups) {
        // Unqualified branches can still have a unique role from already named
        // boundaries. Let the shared adjacency closure name those shapes.
        if (group.size() != 1)
            continue;
        const auto& c = *group.front();
        add(out, derived(id, role + "/history/" + std::to_string(c.shape.ShapeType()), c.sources),
            c.shape, c.sources);
    }
}
void named_boundary(GeometryFeatureBuild& out, const TopoDS_Shape& boundary, const std::string& id,
                    const std::string& role) {
    for (const auto& edge : members(boundary, TopAbs_EDGE)) {
        std::vector<SemanticTopologyRef> sources;
        for (const auto& n : out.named)
            if (n.shape.ShapeType() == TopAbs_FACE && contains(n.shape, edge))
                sources.push_back(n.ref);
        if (sources.empty())
            throw std::invalid_argument("SURFACE_BOUNDARY_HISTORY_INCOMPLETE");
        add(out, derived(id, role, sources), edge, sources);
    }
}
GeometryFeatureBuild sewn(const std::vector<GeometryFeatureBuild>& inputs,
                          const GeometryFeatureSpec& s, const std::function<void()>& check,
                          bool join) {
    BRepBuilderAPI_Sewing sew(s.surface.tolerance, Standard_True, Standard_True, Standard_True,
                              Standard_False);
    std::vector<TopoDS_Shape> shapes;
    for (const auto& in : inputs) {
        check();
        require_sheet(in);
        shapes.push_back(in.shape);
        sew.Add(in.shape);
    }
    sew.Perform();
    check();
    if (sew.NbMultipleEdges() > 0)
        throw std::invalid_argument("SURFACE_NON_MANIFOLD");
    if (sew.SewedShape().IsNull() || (members(sew.SewedShape(), TopAbs_SHELL).size() != 1 &&
                                      members(sew.SewedShape(), TopAbs_FACE).size() != 1))
        throw std::invalid_argument("SURFACE_JOIN_DISCONNECTED");
    manifold(sew.SewedShape());
    if (!BRepAlgoAPI_Check(sew.SewedShape(), Standard_False, Standard_True).IsValid())
        throw std::invalid_argument("SURFACE_SELF_INTERSECTION_OR_INVALID_RESULT");
    GeometryFeatureBuild out;
    out.shape = join ? compound(shapes) : sew.SewedShape();
    for (const auto& in : inputs)
        for (const auto& n : in.named) {
            const auto mapped = join                              ? n.shape
                                : sew.IsModifiedSubShape(n.shape) ? sew.ModifiedSubShape(n.shape)
                                                                  : n.shape;
            add(out, derived(s.feature_id, join ? "join/member" : "sew/member", {n.ref}), mapped,
                {n.ref});
        }
    return out;
}
// MakeWire can replace vertices while connecting independently constructed edges.
// Capture each builder-produced edge and its oriented endpoint roles immediately;
// never match the copies by coordinates or traversal position.
GeometryFeatureBuild connected_curve(const GeometryFeatureBuild& input) {
    require_curve(input);
    BRepBuilderAPI_MakeWire builder;
    GeometryFeatureBuild out;
    for (const auto& shape : members(input.shape, TopAbs_EDGE)) {
        const auto edge = TopoDS::Edge(shape);
        builder.Add(edge);
        if (!builder.IsDone())
            throw std::invalid_argument("CURVE_CHAIN_DISCONNECTED_OR_BRANCHING");
        const auto copy = builder.Edge();
        out.shape = builder.Wire();
        const auto source = named(input, edge).ref;
        add(out, source, copy, {source});
        TopoDS_Vertex a, b, x, y;
        TopExp::Vertices(edge, a, b, Standard_True);
        TopExp::Vertices(copy, x, y, Standard_True);
        add(out, named(input, a).ref, x, {named(input, a).ref});
        add(out, named(input, b).ref, y, {named(input, b).ref});
    }
    out.shape = wire(builder.Wire());
    return out;
}
gp_Vec edge_tangent(const TopoDS_Edge& edge, bool at_start) {
    BRepAdaptor_Curve curve(edge);
    const bool reversed = edge.Orientation() == TopAbs_REVERSED;
    const double p = (at_start != reversed) ? curve.FirstParameter() : curve.LastParameter();
    gp_Pnt q;
    gp_Vec t;
    curve.D1(p, q, t);
    if (reversed)
        t.Reverse();
    if (t.Magnitude() <= tolerance)
        throw std::invalid_argument("SURFACE_SWEEP_SINGULAR_PATH");
    return t.Normalized();
}
void profile_in_plane(const GeometryFeatureBuild& profile, const gp_Pnt& origin,
                      const gp_Vec& normal, double tol) {
    const auto on = [&](const gp_Pnt& p) {
        if (std::abs(gp_Vec(origin, p).Dot(normal)) > tol)
            throw std::invalid_argument("SURFACE_SWEEP_PROFILE_NOT_NORMAL_AT_START");
    };
    for (const auto& edge : members(profile.shape, TopAbs_EDGE)) {
        BRepAdaptor_Curve c(TopoDS::Edge(edge));
        if (c.GetType() == GeomAbs_Line) {
            on(c.Value(c.FirstParameter()));
            on(c.Value(c.LastParameter()));
            if (std::abs(gp_Vec(c.Line().Direction()).Dot(normal)) > 1e-9)
                throw std::invalid_argument("SURFACE_SWEEP_PROFILE_NOT_NORMAL_AT_START");
        } else if (c.GetType() == GeomAbs_Circle) {
            on(c.Circle().Location());
            if (gp_Vec(c.Circle().Axis().Direction()).Crossed(normal).Magnitude() > 1e-9)
                throw std::invalid_argument("SURFACE_SWEEP_PROFILE_NOT_NORMAL_AT_START");
        } else if (c.GetType() == GeomAbs_Ellipse) {
            on(c.Ellipse().Location());
            if (gp_Vec(c.Ellipse().Axis().Direction()).Crossed(normal).Magnitude() > 1e-9)
                throw std::invalid_argument("SURFACE_SWEEP_PROFILE_NOT_NORMAL_AT_START");
        } else if (c.GetType() == GeomAbs_BSplineCurve) {
            const auto curve = c.BSpline();
            for (int i = 1; i <= curve->NbPoles(); ++i)
                on(curve->Pole(i));
        } else if (c.GetType() == GeomAbs_BezierCurve) {
            const auto curve = c.Bezier();
            for (int i = 1; i <= curve->NbPoles(); ++i)
                on(curve->Pole(i));
        } else
            throw std::invalid_argument("SURFACE_SWEEP_PROFILE_PLANE_UNVERIFIED");
    }
}
GeometryFeatureBuild sweep(const GeometryFeatureSpec& s,
                           const std::vector<GeometryFeatureBuild>& inputs,
                           const std::function<void()>& check) {
    const auto profile_count = members(inputs[0].shape, TopAbs_EDGE).size(),
               path_count = members(inputs[1].shape, TopAbs_EDGE).size();
    if (profile_count > 256 || path_count > 512 || profile_count * path_count > 8192)
        throw std::invalid_argument("SURFACE_SWEEP_PATH_BUDGET");
    const auto profile_input = connected_curve(inputs[0]), path_input = connected_curve(inputs[1]);
    const auto profile = wire(profile_input.shape), path = wire(path_input.shape);
    std::vector<TopoDS_Edge> edges;
    for (BRepTools_WireExplorer it(path); it.More(); it.Next())
        edges.push_back(it.Current());
    if (edges.empty())
        throw std::invalid_argument("SURFACE_SWEEP_PATH_BUDGET");
    for (const auto& edge : edges) {
        check();
        if (!internally_g1(edge, tolerance, topology_angular_tolerance_radians))
            throw std::invalid_argument("SURFACE_SWEEP_G1_PATH_REQUIRED");
    }
    for (size_t i = 1; i < edges.size(); ++i)
        if (edge_tangent(edges[i - 1], false).Dot(edge_tangent(edges[i], true)) <= 0 ||
            edge_tangent(edges[i - 1], false).Crossed(edge_tangent(edges[i], true)).Magnitude() >
                topology_angular_tolerance_radians)
            throw std::invalid_argument("SURFACE_SWEEP_G1_PATH_REQUIRED");
    if (path.Closed() &&
        (edge_tangent(edges.back(), false).Dot(edge_tangent(edges.front(), true)) <= 0 ||
         edge_tangent(edges.back(), false).Crossed(edge_tangent(edges.front(), true)).Magnitude() >
             topology_angular_tolerance_radians))
        throw std::invalid_argument("SURFACE_SWEEP_G1_PATH_REQUIRED");
    BRepAdaptor_Curve first(edges.front());
    const auto start =
        first.Value(edges.front().Orientation() == TopAbs_REVERSED ? first.LastParameter()
                                                                   : first.FirstParameter());
    profile_in_plane(inputs[0], start, edge_tangent(edges.front(), true), s.surface.tolerance);
    const auto mode =
        s.surface.frame_mode == "FRENET" ? GeomFill_IsFrenet : GeomFill_IsCorrectedFrenet;
    if (s.surface.frame_mode != "FRENET" && s.surface.frame_mode != "CORRECTED_FRENET")
        throw std::invalid_argument("SURFACE_SWEEP_FRAME_REQUIRED");
    BRepOffsetAPI_MakePipe pipe(path, profile, mode, Standard_False);
    check();
    if (!pipe.IsDone())
        throw std::invalid_argument("SURFACE_SWEEP_FAILED");
    if (pipe.ErrorOnSurface() > s.surface.tolerance)
        throw std::invalid_argument("SURFACE_APPROXIMATION_TOLERANCE_EXCEEDED");
    GeometryFeatureBuild out;
    out.shape = pipe.Shape();
    for (const auto& rail : edges) {
        check();
        const auto rail_ref = named(path_input, rail).ref;
        for (const auto& n : profile_input.named)
            if (n.shape.ShapeType() == TopAbs_EDGE || n.shape.ShapeType() == TopAbs_VERTEX) {
                const auto generated = pipe.Generated(rail, n.shape);
                if (!generated.IsNull())
                    add(out,
                        derived(s.feature_id,
                                "sweep/product/" + std::to_string(generated.ShapeType()),
                                {rail_ref, n.ref}),
                        generated, {rail_ref, n.ref});
            }
    }
    named_boundary(out, pipe.FirstShape(), s.feature_id, "sweep/start");
    named_boundary(out, pipe.LastShape(), s.feature_id, "sweep/end");
    return out;
}
std::map<int, TopoDS_Edge> rectangle_sides(const TopoDS_Face& face);
GeometryFeatureBuild loft(const GeometryFeatureSpec& s,
                          const std::vector<GeometryFeatureBuild>& inputs,
                          const std::function<void()>& check) {
    BRepOffsetAPI_ThruSections op(Standard_False, s.surface.ruled, s.surface.tolerance);
    op.CheckCompatibility(Standard_False);
    op.SetMutableInput(Standard_True);
    std::vector<GeometryFeatureBuild> sections;
    for (const auto& input : inputs)
        sections.push_back(connected_curve(input));
    size_t edge_count = 0;
    bool closed = false;
    for (size_t i = 0; i < inputs.size(); ++i) {
        check();
        const auto section = wire(sections[i].shape);
        const auto count = members(section, TopAbs_EDGE).size();
        if (i == 0) {
            edge_count = count;
            closed = section.Closed();
        } else if (count != edge_count || section.Closed() != closed)
            throw std::invalid_argument("SURFACE_LOFT_SECTION_CORRESPONDENCE_REQUIRED");
        if (count > 256)
            throw std::invalid_argument("SURFACE_LOFT_SECTION_BUDGET");
        op.AddWire(section);
    }
    op.Build();
    check();
    if (!op.IsDone())
        throw std::invalid_argument("SURFACE_LOFT_FAILED");
    GeometryFeatureBuild out;
    out.shape = op.Shape();
    joint_history(out, sections, op, s.feature_id, "loft");
    // Smooth ThruSections creates new V-isoparametric section boundaries but
    // exposes First/LastShape only for solids. Its generated faces retain the
    // profile correspondence; min/max V are explicit construction end roles.
    if (!s.surface.ruled && sections.size() > 2) {
        const auto faces = out.named;
        for (const auto& n : faces)
            if (n.shape.ShapeType() == TopAbs_FACE) {
                const auto sides = rectangle_sides(TopoDS::Face(n.shape));
                add(out, derived(s.feature_id, "loft/start", {n.ref}), sides.at(2), {n.ref});
                add(out, derived(s.feature_id, "loft/end", {n.ref}), sides.at(3), {n.ref});
            }
    }
    named_boundary(out, op.FirstShape(), s.feature_id, "loft/start");
    named_boundary(out, op.LastShape(), s.feature_id, "loft/end");
    return out;
}
GeometryFeatureBuild fill(const GeometryFeatureSpec& s,
                          const std::vector<GeometryFeatureBuild>& inputs,
                          const std::function<void()>& check) {
    std::vector<TopoDS_Shape> edges;
    std::vector<SemanticTopologyRef> sources;
    for (const auto& input : inputs) {
        require_curve(input);
        for (const auto& e : members(input.shape, TopAbs_EDGE)) {
            check();
            edges.push_back(e);
            sources.push_back(named(input, e).ref);
        }
    }
    if (edges.size() > 256)
        throw std::invalid_argument("SURFACE_FILL_BOUNDARY_BUDGET");
    const auto boundary = wire(compound(edges));
    if (!boundary.Closed())
        throw std::invalid_argument("SURFACE_FILL_CLOSED_BOUNDARY_REQUIRED");
    BRepOffsetAPI_MakeFilling op(3, 15, 3, Standard_False, 1e-8, s.surface.tolerance, 1e-3, 1e-3, 8,
                                 16);
    for (const auto& edge : edges) {
        check();
        op.Add(TopoDS::Edge(edge), GeomAbs_C0, Standard_True);
    }
    op.Build();
    check();
    if (!op.IsDone())
        throw std::invalid_argument("SURFACE_FILL_FAILED");
    if (!std::isfinite(op.G0Error()) || op.G0Error() > s.surface.tolerance)
        throw std::invalid_argument("SURFACE_FILL_G0_TOLERANCE_EXCEEDED");
    GeometryFeatureBuild out;
    out.shape = op.Shape();
    add(out, derived(s.feature_id, "fill/surface", sources), out.shape, sources);
    joint_history(out, inputs, op, s.feature_id, "fill");
    return out;
}
GeometryFeatureBuild trim(const GeometryFeatureSpec& s,
                          const std::vector<GeometryFeatureBuild>& inputs,
                          const std::function<void()>& check) {
    require_sheet(inputs[0]);
    require_sheet(inputs[1]);
    if (!s.surface.has_keep_point)
        throw std::invalid_argument("SURFACE_TRIM_KEEP_POINT_REQUIRED");
    BRepAlgoAPI_Splitter op;
    TopTools_ListOfShape args, tools;
    args.Append(inputs[0].shape);
    tools.Append(inputs[1].shape);
    op.SetArguments(args);
    op.SetTools(tools);
    op.SetNonDestructive(Standard_True);
    op.SetRunParallel(Standard_False);
    op.SetFuzzyValue(0);
    op.Build();
    check();
    if (op.HasErrors() || op.Shape().IsNull())
        throw std::invalid_argument("SURFACE_TRIM_FAILED");
    const auto faces = members(op.Shape(), TopAbs_FACE);
    if (faces.empty())
        throw std::invalid_argument("SURFACE_TRIM_EMPTY");
    if (op.SectionEdges().IsEmpty())
        throw std::invalid_argument("SURFACE_TRIM_NO_INTERSECTION");
    std::vector<TopoDS_Shape> cuts;
    for (TopTools_ListIteratorOfListOfShape it(op.SectionEdges()); it.More(); it.Next())
        cuts.push_back(it.Value());
    const auto keep = BRepBuilderAPI_MakeVertex(point(s.surface.keep_point)).Vertex();
    std::vector<size_t> seeds;
    for (size_t i = 0; i < faces.size(); ++i) {
        check();
        BRepExtrema_DistShapeShape distance(keep, faces[i]);
        if (!distance.IsDone())
            throw std::invalid_argument("SURFACE_TRIM_POINT_CLASSIFICATION_FAILED");
        if (distance.Value() <= s.surface.tolerance)
            seeds.push_back(i);
    }
    if (seeds.size() != 1)
        throw std::invalid_argument(seeds.empty() ? "SURFACE_TRIM_KEEP_POINT_NOT_ON_SOURCE"
                                                  : "SURFACE_TRIM_KEEP_POINT_ON_BOUNDARY");
    TopTools_IndexedDataMapOfShapeListOfShape adjacency;
    TopExp::MapShapesAndAncestors(op.Shape(), TopAbs_EDGE, TopAbs_FACE, adjacency);
    std::set<size_t> selected{seeds[0]};
    std::queue<size_t> pending;
    pending.push(seeds[0]);
    while (!pending.empty()) {
        check();
        const auto index = pending.front();
        pending.pop();
        for (const auto& edge : members(faces[index], TopAbs_EDGE)) {
            if (std::any_of(cuts.begin(), cuts.end(),
                            [&](const auto& c) { return c.IsSame(edge); }) ||
                !adjacency.Contains(edge))
                continue;
            for (TopTools_ListIteratorOfListOfShape it(adjacency.FindFromKey(edge)); it.More();
                 it.Next())
                for (size_t i = 0; i < faces.size(); ++i)
                    if (faces[i].IsSame(it.Value()) && selected.insert(i).second)
                        pending.push(i);
        }
    }
    std::vector<TopoDS_Shape> retained;
    for (auto i : selected)
        retained.push_back(faces[i]);
    GeometryFeatureBuild out;
    out.shape = compound(retained);
    joint_history(out, inputs, op, s.feature_id, "trim");
    return out;
}
// A rectangle side is a construction role in the source surface's parameter
// chart, not a persistent position or the order OCCT happened to traverse it.
std::map<int, TopoDS_Edge> rectangle_sides(const TopoDS_Face& face) {
    double u0, u1, v0, v1;
    BRepTools::UVBounds(face, u0, u1, v0, v1);
    if (!std::isfinite(u0) || !std::isfinite(u1) || !std::isfinite(v0) || !std::isfinite(v1) ||
        members(face, TopAbs_WIRE).size() != 1)
        throw std::invalid_argument("SURFACE_EXTEND_RECTANGULAR_DOMAIN_REQUIRED");
    std::map<int, TopoDS_Edge> sides;
    for (const auto& shape : members(face, TopAbs_EDGE)) {
        const auto edge = TopoDS::Edge(shape);
        double first, last;
        auto curve = BRep_Tool::CurveOnSurface(edge, face, first, last);
        if (curve.IsNull())
            throw std::invalid_argument("SURFACE_EXTEND_BOUNDARY_PCURVE_REQUIRED");
        const auto line = Handle(Geom2d_Line)::DownCast(curve);
        if (line.IsNull())
            throw std::invalid_argument("SURFACE_EXTEND_ISOPARAMETRIC_BOUNDARY_REQUIRED");
        const auto origin = line->Location();
        const auto dir = line->Direction();
        const double eps = 1e-8 * std::max({1., std::abs(u1 - u0), std::abs(v1 - v0)});
        int side = -1;
        if (std::abs(dir.X()) < 1e-10) {
            if (std::abs(origin.X() - u0) <= eps)
                side = 0;
            else if (std::abs(origin.X() - u1) <= eps)
                side = 1;
        } else if (std::abs(dir.Y()) < 1e-10) {
            if (std::abs(origin.Y() - v0) <= eps)
                side = 2;
            else if (std::abs(origin.Y() - v1) <= eps)
                side = 3;
        }
        if (side < 0 || sides.count(side))
            throw std::invalid_argument("SURFACE_EXTEND_RECTANGULAR_DOMAIN_REQUIRED");
        sides.emplace(side, edge);
    }
    // Periodic charts have the same seam edge twice; the omitted alias is exact
    // topology, and that seam is never a free boundary available to extend.
    if (sides.size() == 3) {
        if (!sides.count(0) && sides.count(1))
            sides[0] = sides[1];
        else if (!sides.count(1) && sides.count(0))
            sides[1] = sides[0];
        else if (!sides.count(2) && sides.count(3))
            sides[2] = sides[3];
        else if (!sides.count(3) && sides.count(2))
            sides[3] = sides[2];
    }
    if (sides.size() != 4)
        throw std::invalid_argument("SURFACE_EXTEND_RECTANGULAR_DOMAIN_REQUIRED");
    return sides;
}
GeometryFeatureBuild extend(const GeometryFeatureSpec& s,
                            const std::vector<GeometryFeatureBuild>& inputs,
                            const std::function<void()>& check) {
    require_sheet(inputs[0]);
    const auto fs = members(inputs[0].shape, TopAbs_FACE);
    if (fs.size() != 1 || s.parameters.distance <= s.surface.tolerance)
        throw std::invalid_argument("SURFACE_EXTEND_SINGLE_FACE_AND_LENGTH_REQUIRED");
    const auto face = TopoDS::Face(fs[0]);
    const auto selected_ref = named(inputs[1], single_edge(inputs[1])).ref;
    const auto boundary =
        std::find_if(inputs[0].named.begin(), inputs[0].named.end(), [&](const auto& n) {
            return n.shape.ShapeType() == TopAbs_EDGE && key(n.ref) == key(selected_ref);
        });
    if (boundary == inputs[0].named.end())
        throw std::invalid_argument("SURFACE_EXTEND_FREE_BOUNDARY_REQUIRED");
    const auto edge = TopoDS::Edge(boundary->shape);
    if (!contains(face, edge) || BRep_Tool::IsClosed(edge, face))
        throw std::invalid_argument("SURFACE_EXTEND_FREE_BOUNDARY_REQUIRED");
    const auto original = rectangle_sides(face);
    int selected = -1;
    for (const auto& [side, e] : original)
        if (e.IsSame(edge))
            selected = side;
    if (selected < 0)
        throw std::invalid_argument("SURFACE_EXTEND_FREE_BOUNDARY_REQUIRED");
    double u0, u1, v0, v1;
    BRepTools::UVBounds(face, u0, u1, v0, v1);
    Handle(Geom_BoundedSurface) surface =
        new Geom_RectangularTrimmedSurface(BRep_Tool::Surface(face), u0, u1, v0, v1);
    GeomLib::ExtendSurfByLength(surface, s.parameters.distance, 1, selected < 2, selected % 2 == 1);
    check();
    BRepBuilderAPI_MakeFace make(surface, s.surface.tolerance);
    if (!make.IsDone())
        throw std::invalid_argument("SURFACE_EXTEND_FAILED");
    auto result = make.Face();
    if (face.Orientation() == TopAbs_REVERSED)
        result.Reverse();
    const auto generated = rectangle_sides(result);
    GeometryFeatureBuild out;
    out.shape = result;
    const auto r = named(inputs[0], face).ref;
    const auto boundary_ref = named(inputs[0], edge).ref;
    add(out, derived(s.feature_id, "extend/surface", {r, boundary_ref}), result, {r, boundary_ref});
    for (const auto& [side, e] : generated) {
        const auto& source = named(inputs[0], original.at(side)).ref;
        add(out,
            derived(s.feature_id, side == selected ? "extend/moved-boundary" : "extend/boundary",
                    {source, boundary_ref}),
            e, {source, boundary_ref});
    }
    return out;
}
GeometryFeatureBuild close(const GeometryFeatureSpec& s,
                           const std::vector<GeometryFeatureBuild>& inputs) {
    require_sheet(inputs[0]);
    manifold(inputs[0].shape, true);
    const auto shells = members(inputs[0].shape, TopAbs_SHELL);
    if (shells.size() != 1 || !BRep_Tool::IsClosed(shells.front()))
        throw std::invalid_argument("CLOSE_SURFACE_CLOSED_SHELL_REQUIRED");
    BRepBuilderAPI_MakeSolid make(TopoDS::Shell(shells.front()));
    if (!make.IsDone())
        throw std::invalid_argument("CLOSE_SURFACE_FAILED");
    auto solid = make.Solid();
    if (!BRepLib::OrientClosedSolid(solid))
        throw std::invalid_argument("CLOSE_SURFACE_ORIENTATION_FAILED");
    GeometryFeatureBuild out;
    out.shape = solid;
    for (const auto& n : inputs[0].named)
        add(out, derived(s.feature_id, "close/retained", {n.ref}), n.shape, {n.ref});
    return out;
}
}  // namespace
bool is_surface_feature(const std::string& op) {
    return op == "SURFACE_SWEEP" || op == "SURFACE_LOFT" || op == "SURFACE_FILL" ||
           op == "SURFACE_TRIM" || op == "SURFACE_EXTEND" || op == "SURFACE_JOIN" ||
           op == "SURFACE_SEW" || op == "SURFACE_OFFSET" || op == "CLOSE_SURFACE";
}
GeometryFeatureBuild build_surface_feature(const GeometryFeatureSpec& s,
                                           const std::vector<GeometryFeatureBuild>& inputs,
                                           const std::function<bool()>& cancelled) {
    const auto check = [&]() {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
    };
    check();
    size_t input_topology = 0;
    for (const auto& input : inputs) {
        check();
        input_topology += input.named.size();
        if (input_topology > 32768)
            throw std::invalid_argument("SURFACE_INPUT_BUDGET");
    }
    const bool multi = s.operation == "SURFACE_LOFT" || s.operation == "SURFACE_FILL" ||
                       s.operation == "SURFACE_JOIN" || s.operation == "SURFACE_SEW";
    const size_t count = (s.operation == "SURFACE_SWEEP" || s.operation == "SURFACE_TRIM" ||
                          s.operation == "SURFACE_EXTEND")
                             ? 2
                             : 1;
    if ((!multi && inputs.size() != count) || (multi && (inputs.empty() || inputs.size() > 128)) ||
        (s.operation == "SURFACE_LOFT" && (inputs.size() < 2 || inputs.size() > 32)))
        throw std::invalid_argument("GEOMETRY_INPUT_COUNT_INVALID");
    if (!std::isfinite(s.surface.tolerance) || s.surface.tolerance < 1e-7 ||
        s.surface.tolerance > 1e-2)
        throw std::invalid_argument("SURFACE_TOLERANCE_OUT_OF_RANGE");
    GeometryFeatureBuild out;
    if (s.operation == "SURFACE_SWEEP")
        out = sweep(s, inputs, check);
    else if (s.operation == "SURFACE_LOFT")
        out = loft(s, inputs, check);
    else if (s.operation == "SURFACE_FILL")
        out = fill(s, inputs, check);
    else if (s.operation == "SURFACE_TRIM")
        out = trim(s, inputs, check);
    else if (s.operation == "SURFACE_EXTEND")
        out = extend(s, inputs, check);
    else if (s.operation == "SURFACE_JOIN" || s.operation == "SURFACE_SEW")
        out = sewn(inputs, s, check, s.operation == "SURFACE_JOIN");
    else if (s.operation == "CLOSE_SURFACE")
        out = close(s, inputs);
    else {
        require_sheet(inputs[0]);
        if (std::abs(s.parameters.distance) <= s.surface.tolerance)
            throw std::invalid_argument("SURFACE_OFFSET_DISTANCE_REQUIRED");
        BRepOffsetAPI_MakeOffsetShape op;
        op.PerformByJoin(inputs[0].shape, s.parameters.distance, s.surface.tolerance,
                         BRepOffset_Skin, Standard_False, Standard_False, GeomAbs_Intersection,
                         Standard_False);
        check();
        if (!op.IsDone())
            throw std::invalid_argument("SURFACE_OFFSET_FAILED");
        out.shape = op.Shape();
        joint_history(out, inputs, op, s.feature_id, "offset");
    }
    check();
    if (out.shape.IsNull() ||
        (s.operation != "SURFACE_JOIN" &&
         !BRepAlgoAPI_Check(out.shape, Standard_False, Standard_True).IsValid()))
        throw std::invalid_argument("SURFACE_SELF_INTERSECTION_OR_INVALID_RESULT:" + s.operation);
    manifold(out.shape);
    return out;
}
}  // namespace occccad::kernel::detail

#ifndef OCCCCAD_GEOMETRY_FEATURE_SUPPORT_HPP
#define OCCCCAD_GEOMETRY_FEATURE_SUPPORT_HPP
#include <TopTools_ListIteratorOfListOfShape.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Wire.hxx>
#include <TopoDS_Face.hxx>
#include <gp_Dir.hxx>
#include <gp_Pnt.hxx>

#include <internal/geometry_feature.hpp>

#include <algorithm>
#include <map>
#include <stdexcept>

namespace occccad::kernel::detail::geometry_features {
inline constexpr double tolerance = 1e-7;
gp_Pnt point(const Vec3&);
gp_Dir direction(const Vec3&);
std::string key(const SemanticTopologyRef&);
std::string signature(std::vector<SemanticTopologyRef>);
SemanticTopologyRef derived(const std::string&, const std::string&,
                            const std::vector<SemanticTopologyRef>&);
bool contains(const TopoDS_Shape&, const TopoDS_Shape&);
void add(GeometryFeatureBuild&, const SemanticTopologyRef&, const TopoDS_Shape&,
         std::vector<SemanticTopologyRef> sources = {},
         std::vector<SemanticTopologyRef> roles = {});
TopoDS_Shape compound(const std::vector<TopoDS_Shape>&);
std::vector<TopoDS_Shape> members(const TopoDS_Shape&, TopAbs_ShapeEnum);
TopoDS_Wire wire(const TopoDS_Shape&);
const GeometryNamedShape& named(const GeometryFeatureBuild&, const TopoDS_Shape&);
TopoDS_Edge single_edge(const GeometryFeatureBuild&);
// Exact one-sided derivatives on continuity intervals, shared by surface/solid sweeps.
bool internally_g1(const TopoDS_Edge&, double linear_tolerance, double angular_tolerance,
                   const std::function<bool()>& cancelled = {});

// Actual OCCT history and retained topology only. Repeated unqualified branches
// are rejected rather than assigned identities from traversal or coordinates.
template <class Builder>
void history(GeometryFeatureBuild& b, const GeometryFeatureBuild& source, Builder& operation,
             const std::string& id, const std::string& role) {
    for (const auto& n : source.named) {
        if (contains(b.shape, n.shape))
            add(b, derived(id, role + "/retained", {n.ref}), n.shape, {n.ref});
        for (const auto& entry : std::vector<std::pair<std::string, TopTools_ListOfShape>>{
                 {"modified", operation.Modified(n.shape)},
                 {"generated", operation.Generated(n.shape)}}) {
            std::vector<TopoDS_Shape> same;
            for (TopTools_ListIteratorOfListOfShape it(entry.second); it.More(); it.Next())
                if (contains(b.shape, it.Value()) &&
                    (it.Value().ShapeType() == TopAbs_FACE ||
                     it.Value().ShapeType() == TopAbs_EDGE ||
                     it.Value().ShapeType() == TopAbs_VERTEX) &&
                    std::none_of(same.begin(), same.end(),
                                 [&](const auto& v) { return v.IsSame(it.Value()); }))
                    same.push_back(it.Value());
            std::map<TopAbs_ShapeEnum, size_t> counts;
            for (const auto& v : same)
                ++counts[v.ShapeType()];
            for (const auto& v : same) {
                if (counts[v.ShapeType()] > 1)
                    throw std::invalid_argument("GEOMETRY_HISTORY_BRANCH_AMBIGUOUS:" + role + "/" +
                                                entry.first + "/" + std::to_string(v.ShapeType()));
                add(b,
                    derived(id, role + "/" + entry.first + "/" + std::to_string(v.ShapeType()),
                            {n.ref}),
                    v, {n.ref});
            }
        }
    }
}
}  // namespace occccad::kernel::detail::geometry_features
namespace occccad::kernel::detail {
std::map<int, TopoDS_Edge> surface_rectangle_sides(const TopoDS_Face&,const std::string& prefix="SURFACE_EXTEND");
GeometryFeatureBuild build_border_guide_surface(const GeometryFeatureSpec&,const std::vector<GeometryFeatureBuild>&,const std::function<bool()>&);
GeometryFeatureBuild build_shape_control_surface(const GeometryFeatureSpec&, const std::vector<GeometryFeatureBuild>&, const std::function<bool()>&);
bool is_surface_feature(const std::string& operation);
GeometryFeatureBuild build_surface_feature(const GeometryFeatureSpec&,
                                           const std::vector<GeometryFeatureBuild>&,
                                           const std::function<bool()>& cancelled);
}  // namespace occccad::kernel::detail
#endif

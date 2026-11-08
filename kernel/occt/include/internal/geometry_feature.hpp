#ifndef OCCCCAD_OCCT_GEOMETRY_FEATURE_HPP
#define OCCCCAD_OCCT_GEOMETRY_FEATURE_HPP
#include <TopoDS_Shape.hxx>

#include <occccad/kernel/geometry_feature.hpp>
#include <occccad/kernel/topology_naming.hpp>

#include <functional>

namespace occccad::kernel::detail {
struct GeometryNamedShape {
    SemanticTopologyRef ref;
    TopoDS_Shape shape;
    std::vector<SemanticTopologyRef> sources;
    std::vector<SemanticTopologyRef> roles;
};
struct GeometryFeatureBuild {
    TopoDS_Shape shape;
    std::vector<GeometryNamedShape> named;
};
GeometryFeatureBuild build_geometry_feature(const GeometryFeatureSpec& spec,
                                            const std::function<bool()>& cancelled = {});
GeometryFeatureBuild read_geometry_input(const GeometryFeatureInput& input);
// Complete only topologically unique construction roles; never number branches
// by traversal order, coordinates or distance to old geometry.
void complete_geometry_naming(GeometryFeatureBuild& result, const std::string& feature_id);
}  // namespace occccad::kernel::detail
#endif

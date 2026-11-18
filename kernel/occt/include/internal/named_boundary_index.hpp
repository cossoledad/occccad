#pragma once

#include <TopExp.hxx>
#include <TopoDS_Shape.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopTools_ShapeMapHasher.hxx>

#include <unordered_map>
#include <vector>

namespace occccad::kernel::detail {
// Shape identity includes location. Indices address the caller's immutable
// name order for the current completion pass only.
class NamedBoundaryIndex {
    using Index = std::unordered_map<TopoDS_Shape, std::vector<std::size_t>,
                                     TopTools_ShapeMapHasher, TopTools_ShapeMapHasher>;
    Index named_, owners_;

public:
    template<class Names>
    explicit NamedBoundaryIndex(const Names& names, TopAbs_ShapeEnum target) {
        for (std::size_t i = 0; i < names.size(); ++i) {
            const auto& shape = names[i].shape;
            named_[shape].push_back(i);
            if ((target == TopAbs_EDGE && shape.ShapeType() == TopAbs_FACE) ||
                (target == TopAbs_VERTEX && shape.ShapeType() == TopAbs_EDGE)) {
                TopTools_IndexedMapOfShape children;
                TopExp::MapShapes(shape, target, children);
                for (int j = 1; j <= children.Extent(); ++j) owners_[children(j)].push_back(i);
            }
        }
    }

    bool contains(const TopoDS_Shape& shape) const { return named_.find(shape) != named_.end(); }

    template<class Visit>
    void visit(const TopoDS_Shape& shape, const Visit& visit) const {
        if (shape.ShapeType() == TopAbs_FACE) {
            TopTools_IndexedMapOfShape boundary;
            TopExp::MapShapes(shape, TopAbs_EDGE, boundary);
            for (int j = 1; j <= boundary.Extent(); ++j) {
                if (const auto found = named_.find(boundary(j)); found != named_.end())
                    for (const auto i : found->second) visit(i);
            }
        } else if (const auto found = owners_.find(shape); found != owners_.end()) {
            for (const auto i : found->second) visit(i);
        }
    }
};
}  // namespace occccad::kernel::detail

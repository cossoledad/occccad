#include "plane_partition.hpp"
#include "solid_material.hpp"

#include <BOPAlgo_Splitter.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <TopExp.hxx>
#include <TopoDS.hxx>

namespace occccad::kernel::detail {

PlanePartition partition_solid(const TopoDS_Shape& input, const gp_Pln& plane) {
    TopTools_IndexedMapOfShape solids;
    TopExp::MapShapes(input, TopAbs_SOLID, solids);
    if (solids.Extent() != 1) throw std::invalid_argument("SPLIT_REQUIRES_SINGLE_SOLID");
    BOPAlgo_Splitter splitter;
    splitter.SetNonDestructive(true);
    splitter.AddArgument(input);
    splitter.AddTool(BRepBuilderAPI_MakeFace(plane).Face());
    splitter.Perform();
    if (splitter.HasErrors() || splitter.HasWarnings())
        throw std::runtime_error("SPLIT_PARTITION_FAILED");
    PlanePartition result;
    for (TopExp_Explorer it(splitter.Shape(), TopAbs_SOLID); it.More(); it.Next()) {
        auto solid = TopoDS::Solid(it.Current());
        if (!BRepCheck_Analyzer(solid).IsValid())
            throw std::runtime_error("SPLIT_INVALID_RESULT");
        const bool classified = visit_solid_material(solid, [&](const gp_Pnt& witness) {
            const auto height = gp_Vec(plane.Location(), witness).Dot(gp_Vec(plane.Axis().Direction()));
            if (std::abs(height) <= Precision::Confusion()) return false;
            auto& side = height > 0 ? result.positive : result.negative;
            if (!side.IsNull()) throw std::invalid_argument("SPLIT_MULTIPLE_CELLS_ON_SIDE");
            side = solid;
            return true;
        });
        if (!classified) throw std::runtime_error("SPLIT_UNCLASSIFIED_CELL");
    }
    if (result.positive.IsNull() || result.negative.IsNull())
        throw std::invalid_argument("SPLIT_PLANE_MISSES_MATERIAL");
    result.history = splitter.History();
    return result;
}

} // namespace occccad::kernel::detail

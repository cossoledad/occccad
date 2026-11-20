#include "body_validation.hpp"

#include <BRepBndLib.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepCheck_Shell.hxx>
#include <BRepCheck_Solid.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRep_Builder.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Iterator.hxx>

#include <stdexcept>
#include <unordered_set>

#include "planar_boundary.hpp"

namespace occccad::kernel::detail {
namespace {

bool full_check(const TopoDS_Shape& shape, bool& parallel) {
    TopExp_Explorer faces(shape, TopAbs_FACE);
    for (int i = 0; i < 128 && faces.More(); ++i)
        faces.Next();
    parallel = faces.More();
    return BRepCheck_Analyzer(shape, Standard_True, parallel).IsValid();
}

bool clean(const BRepCheck_ListOfStatus& statuses) {
    return statuses.Extent() == 1 && statuses.First() == BRepCheck_NoError;
}

TopoDS_Shape single_solid(const TopoDS_Shape& shape) {
    TopExp_Explorer solids(shape, TopAbs_SOLID);
    if (!solids.More())
        return {};
    const auto first = solids.Current();
    solids.Next();
    return solids.More() ? TopoDS_Shape{} : first;
}

}  // namespace

bool valid_body_boundary(const TopoDS_Shape& result, const TopoDS_Shape& predecessor,
                         const std::function<bool()>& cancelled, bool& parallel) {
    parallel = false;
    if (predecessor.IsNull())
        return full_check(result, parallel);
    const auto poll = [&] {
        if (cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
    };
    // A Body can contain internal free edges, wires or vertices inside a solid.
    // Face checks do not cover those contexts. Delegate such shapes to OCCT.
    if (TopExp_Explorer(result, TopAbs_SHELL, TopAbs_SOLID).More())
        return full_check(result, parallel);
    for (const auto kind : {TopAbs_WIRE, TopAbs_EDGE, TopAbs_VERTEX}) {
        if (TopExp_Explorer(result, kind, TopAbs_FACE).More())
            return full_check(result, parallel);
    }
    std::unordered_set<TopoDS_Shape> inherited;
    std::vector<TopoDS_Face> trimmed_predecessors;
    for (TopExp_Explorer old(predecessor, TopAbs_FACE); old.More(); old.Next()) {
        inherited.insert(old.Current());
        if (old.Current().NbChildren() >= 2 * planar_batch_size)
            trimmed_predecessors.push_back(TopoDS::Face(old.Current()));
    }
    BRep_Builder builder;
    TopoDS_Compound changed;
    builder.MakeCompound(changed);
    int inherited_count = 0, changed_count = 0;
    bool partitioned = false;
    for (TopExp_Explorer face(result, TopAbs_FACE); face.More(); face.Next()) {
        if (inherited.count(face.Current()))
            ++inherited_count;
        else {
            if (append_planar_face_checks(TopoDS::Face(face.Current()), changed, cancelled, trimmed_predecessors))
                partitioned = true;
            else
                builder.Add(changed, face.Current());
            ++changed_count;
        }
    }
    if (!partitioned && (inherited_count == 0 || changed_count > inherited_count))
        return full_check(result, parallel);
    poll();
    if (changed_count && !full_check(changed, parallel))
        return partitioned && full_check(result, parallel);

    // OCCT 7.9 BRepCheck_Analyzer performs intrinsic checks on each shell
    // and solid, and contextual shell checks in each containing solid. These
    // cross-face checks must run even if every face survived the operation.
    for (TopExp_Explorer solids(result, TopAbs_SOLID); solids.More(); solids.Next()) {
        poll();
        const auto solid = TopoDS::Solid(solids.Current());
        try {
            BRepCheck_Solid solid_check(solid);
            if (!clean(solid_check.Status()))
                return false;
            for (TopoDS_Iterator shell(solid); shell.More(); shell.Next()) {
                if (shell.Value().ShapeType() != TopAbs_SHELL)
                    return full_check(result, parallel);
                for (TopoDS_Iterator member(shell.Value()); member.More(); member.Next())
                    if (member.Value().ShapeType() != TopAbs_FACE)
                        return full_check(result, parallel);
                BRepCheck_Shell boundary(TopoDS::Shell(shell.Value()));
                if (!clean(boundary.Status()))
                    return false;
                boundary.InContext(solid);
                if (!clean(boundary.StatusOnShape(solid)))
                    return false;
            }
        } catch (const Standard_Failure&) {
            return full_check(result, parallel);
        }
    }
    return true;
}

bool proves_material_change(const TopoDS_Shape& input, const TopoDS_Shape& tool,
                            const TopoDS_Shape& result, bool addition,
                            const std::function<bool()>& cancelled) {
    const auto before = single_solid(input), region = single_solid(tool),
               after = single_solid(result);
    if (before.IsNull() || region.IsNull() || after.IsNull())
        return false;
    try {
        Bnd_Box box;
        BRepBndLib::Add(region, box, Standard_False);
        if (box.IsVoid() || box.IsOpen())
            return false;
        const gp_XYZ low = box.CornerMin().XYZ(), high = box.CornerMax().XYZ();
        const gp_XYZ center = (low + high) * 0.5;
        BRepClass3d_SolidClassifier tool_classifier(region), input_classifier, result_classifier;
        bool operands_loaded = false;
        for (int i = 0; i < 7; ++i) {
            if (cancelled && cancelled())
                throw std::runtime_error("EVALUATION_CANCELLED");
            gp_Pnt point(center);
            if (i) {
                const int axis = (i + 1) / 2;
                point.SetCoord(axis, center.Coord(axis) + (i % 2 ? 0.25 : -0.25) *
                                                              (high.Coord(axis) - low.Coord(axis)));
            }
            constexpr double min_margin = 4;
            const double tolerance = min_margin * Precision::Confusion();
            tool_classifier.Perform(point, tolerance);
            if (tool_classifier.State() != TopAbs_IN)
                continue;
            if (!operands_loaded) {
                input_classifier.Load(before);
                result_classifier.Load(after);
                operands_loaded = true;
            }
            input_classifier.Perform(point, tolerance);
            if (input_classifier.State() != (addition ? TopAbs_OUT : TopAbs_IN))
                continue;
            result_classifier.Perform(point, tolerance);
            if (result_classifier.State() == (addition ? TopAbs_IN : TopAbs_OUT))
                return true;
        }
    } catch (const Standard_Failure&) {
        // This optional classifier could not produce a witness on these inputs.
    }
    return false;
}

}  // namespace occccad::kernel::detail

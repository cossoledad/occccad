#include "body_validation.hpp"

#include <BRepBndLib.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepCheck_Shell.hxx>
#include <BRepCheck_Solid.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopoDS_Iterator.hxx>

#include <algorithm>
#include <cmath>
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

// For an already certified finite solid, a tolerance tube reaching infinity
// without touching any face proves exterior membership. Try all axis rays in
// one traversal. Blocked rays convey no membership information.
bool has_clear_exterior_ray(const TopoDS_Shape& solid, const gp_Pnt& point, double tolerance,
                            const std::function<bool()>& cancelled) {
    if (TopExp_Explorer(solid, TopAbs_EDGE, TopAbs_FACE).More() ||
        TopExp_Explorer(solid, TopAbs_VERTEX, TopAbs_FACE).More())
        return false;
    bool clear[3][2] = {{true, true}, {true, true}, {true, true}};
    const double p[] = {point.X(), point.Y(), point.Z()};
    size_t visited = 0;
    for (TopExp_Explorer it(solid, TopAbs_FACE); it.More(); it.Next()) {
        if (visited++ % 64 == 0 && cancelled && cancelled())
            throw std::runtime_error("EVALUATION_CANCELLED");
        Bnd_Box box;
        const auto face = TopoDS::Face(it.Current());
        TopLoc_Location location;
        if (BRep_Tool::Surface(face, location).IsNull())
            return false;
        BRepBndLib::Add(face, box, Standard_False);
        if (box.IsVoid() || box.IsOpen())
            return false;
        const double subshape_tolerance = std::max(BRep_Tool::MaxTolerance(face, TopAbs_EDGE),
                                                   BRep_Tool::MaxTolerance(face, TopAbs_VERTEX));
        box.SetGap(box.GetGap() + tolerance + subshape_tolerance);
        double lo[3], hi[3];
        box.Get(lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]);
        for (int axis = 0; axis < 3; ++axis)
            if (!std::isfinite(lo[axis]) || !std::isfinite(hi[axis]))
                return false;
        bool any = false;
        for (int axis = 0; axis < 3; ++axis) {
            const int u = (axis + 1) % 3, v = (axis + 2) % 3;
            if (lo[u] <= p[u] && p[u] <= hi[u] && lo[v] <= p[v] && p[v] <= hi[v]) {
                if (lo[axis] <= p[axis]) clear[axis][0] = false;
                if (hi[axis] >= p[axis]) clear[axis][1] = false;
            }
            any |= clear[axis][0] || clear[axis][1];
        }
        if (!any)
            return false;
    }
    return visited != 0;
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

bool reuse_boolean_orientation(const TopoDS_Shape& validated_input, const TopoDS_Shape& tool) {
    const auto input_solid = single_solid(validated_input), tool_solid = single_solid(tool);
    if (input_solid.IsNull() || tool_solid.IsNull())
        return false;
    TopExp_Explorer faces(input_solid, TopAbs_FACE);
    for (int i = 0; i < 128 && faces.More(); ++i)
        faces.Next();
    if (!faces.More())
        return false;
    try {
        BRepClass3d_SolidClassifier classifier(tool_solid);
        classifier.PerformInfinitePoint(Precision::Confusion());
        return classifier.State() == TopAbs_OUT;
    } catch (const Standard_Failure&) {
        return false;
    }
}

bool proves_material_change(const TopoDS_Shape& input, const TopoDS_Shape& tool,
                            const TopoDS_Shape& result, bool addition,
                            const std::function<bool()>& cancelled) {
    const auto before = single_solid(input), after = single_solid(result);
    TopExp_Explorer regions(tool, TopAbs_SOLID);
    if (before.IsNull() || !regions.More() || after.IsNull())
        return false;
    try {
        Bnd_Box box;
        // One interior point of one member suffices if the actual Body changes
        // membership there. An unsuccessful probe leaves the whole partition to OCCT.
        const auto region = regions.Current();
        BRepBndLib::Add(region, box, Standard_False);
        if (box.IsVoid() || box.IsOpen())
            return false;
        const gp_XYZ low = box.CornerMin().XYZ(), high = box.CornerMax().XYZ();
        const gp_XYZ center = (low + high) * 0.5;
        BRepClass3d_SolidClassifier tool_classifier(region), input_classifier, result_classifier;
        bool before_loaded = false, after_loaded = false;
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
            if (!(addition && i == 0 && has_clear_exterior_ray(before, point, tolerance, cancelled))) {
                if (!before_loaded) {
                    input_classifier.Load(before);
                    before_loaded = true;
                }
                input_classifier.Perform(point, tolerance);
                if (input_classifier.State() != (addition ? TopAbs_OUT : TopAbs_IN))
                    continue;
            }
            if (!after_loaded) {
                result_classifier.Load(after);
                after_loaded = true;
            }
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

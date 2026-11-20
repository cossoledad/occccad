#include "planar_boundary.hpp"

#include <BRepTopAdaptor_FClass2d.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <BndLib_Add2dCurve.hxx>
#include <Bnd_Box2d.hxx>
#include <Geom2dAdaptor_Curve.hxx>
#include <Geom_Plane.hxx>
#include <Precision.hxx>
#include <TopTools_DataMapOfShapeInteger.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Iterator.hxx>
#include <TopoDS_Wire.hxx>

#include <algorithm>
#include <array>
#include <cmath>
#include <stdexcept>
#include <unordered_map>
#include <vector>

namespace occccad::kernel::detail {
namespace {
struct Boundary {
    TopoDS_Wire wire;
    std::array<double, 2> low, high;
};

// A Boolean may rebuild a wire container while carrying its exact edges.
class ValidatedLoops {
public:
    ValidatedLoops(const TopoDS_Face& forward, const std::function<bool()>& cancelled) {
        std::size_t count = 0;
        for (TopoDS_Iterator wire(forward); wire.More(); wire.Next()) {
            if (count++ % 64 == 0 && cancelled && cancelled())
                throw std::runtime_error("EVALUATION_CANCELLED");
            if (wire.Value().ShapeType() != TopAbs_WIRE) continue;
            for (TopoDS_Iterator edge(wire.Value()); edge.More(); edge.Next())
                owners_.emplace(edge.Value(), TopoDS::Wire(wire.Value()));
        }
    }
    ValidatedLoops() = default;
    bool contains(const TopoDS_Wire& wire) const {
        TopoDS_Iterator current(wire);
        if (!current.More()) return false;
        const auto found = owners_.find(current.Value());
        if (found == owners_.end()) return false;
        const auto& old = found->second;
        if (old.IsEqual(wire)) return true;
        if (old.NbChildren() != wire.NbChildren() || old.Location() != wire.Location() ||
            old.Orientation() != wire.Orientation() || old.Closed() != wire.Closed() ||
            old.Orientable() != wire.Orientable() || old.Infinite() != wire.Infinite() ||
            old.Convex() != wire.Convex())
            return false;
        TopoDS_Iterator previous(old);
        while (previous.More() && !previous.Value().IsEqual(current.Value())) previous.Next();
        if (!previous.More()) return false;
        // Only the start of a closed loop may rotate. Each oriented edge and
        // its placement must be identical, including repeated occurrences.
        for (; current.More(); current.Next(), previous.Next()) {
            if (!previous.More()) {
                if (!wire.Closed()) return false;
                previous.Initialize(old);
            }
            if (!previous.Value().IsEqual(current.Value())) return false;
        }
        return true;
    }
private:
    std::unordered_map<TopoDS_Shape, TopoDS_Wire> owners_;
};

ValidatedLoops certified_wires(const TopoDS_Face& forward, const TopoDS_Wire& outer,
                               const std::vector<TopoDS_Face>& validated,
                               const std::function<bool()>& cancelled) {
    TopLoc_Location location;
    const auto& surface = BRep_Tool::Surface(forward, location);
    for (const auto& face : validated) {
        TopLoc_Location old_location;
        if (BRep_Tool::Surface(face, old_location) != surface || location != old_location ||
            BRep_Tool::Tolerance(face) != BRep_Tool::Tolerance(forward) ||
            BRep_Tool::NaturalRestriction(face) ||
            (face.Orientation() != TopAbs_FORWARD && face.Orientation() != TopAbs_REVERSED))
            continue;
        ValidatedLoops loops(TopoDS::Face(face.Oriented(TopAbs_FORWARD)), cancelled);
        if (loops.contains(outer))
            return loops;
    }
    return {};
}

bool wire_box(Boundary& loop, const TopoDS_Face& face, double gap, int owner,
              TopTools_DataMapOfShapeInteger& owners) {
    Bnd_Box2d box;
    for (TopoDS_Iterator edge(loop.wire); edge.More(); edge.Next()) {
        if (edge.Value().ShapeType() != TopAbs_EDGE || owners.IsBound(edge.Value()))
            return false;
        owners.Bind(edge.Value(), owner);
        const auto e = TopoDS::Edge(edge.Value());
        if (BRep_Tool::Degenerated(e) || BRep_Tool::IsClosed(e, face))
            return false;
        for (TopoDS_Iterator vertex(e); vertex.More(); vertex.Next()) {
            const auto* previous = owners.Seek(vertex.Value());
            if (previous && *previous != owner)
                return false;
            if (!previous)
                owners.Bind(vertex.Value(), owner);
        }
        double first, last;
        const auto curve = BRep_Tool::CurveOnSurface(e, face, first, last);
        if (curve.IsNull() || !std::isfinite(first) || !std::isfinite(last) || first >= last)
            return false;
        const Geom2dAdaptor_Curve adaptor(curve);
        if (adaptor.GetType() != GeomAbs_Line && adaptor.GetType() != GeomAbs_Circle)
            return false;
        Bnd_Box2d edge_box;
        BndLib_Add2dCurve::Add(adaptor, first, last, gap, edge_box);
        if (edge_box.IsVoid())
            return false;
        box.Add(edge_box);
    }
    if (box.IsVoid() || box.IsOpenXmin() || box.IsOpenXmax() || box.IsOpenYmin() ||
        box.IsOpenYmax())
        return false;
    box.Get(loop.low[0], loop.low[1], loop.high[0], loop.high[1]);
    for (int i = 0; i < 2; ++i)
        if (!std::isfinite(loop.low[i]) || !std::isfinite(loop.high[i]) ||
            loop.high[i] <= loop.low[i])
            return false;
    return true;
}

// Cut only in empty strips between tolerance-expanded UV envelopes. Within
// any one strip OCCT still performs every intersection and containment check.
std::vector<std::size_t> separate(std::vector<Boundary>& holes, int axis) {
    std::sort(holes.begin(), holes.end(),
              [axis](const auto& a, const auto& b) { return a.low[axis] < b.low[axis]; });
    std::vector<std::size_t> ends;
    double high = holes.front().high[axis];
    std::size_t start = 0;
    for (std::size_t i = 1; i < holes.size(); ++i) {
        if (i - start >= planar_batch_size && high < holes[i].low[axis]) {
            ends.push_back(i);
            start = i;
        }
        high = std::max(high, holes[i].high[axis]);
    }
    ends.push_back(holes.size());
    return ends;
}
}  // namespace

bool append_planar_face_checks(const TopoDS_Face& face, TopoDS_Compound& checks,
                               const std::function<bool()>& cancelled,
                               const std::vector<TopoDS_Face>& validated_faces) {
    if (face.NbChildren() < static_cast<int>(2 * planar_batch_size) || BRep_Tool::NaturalRestriction(face))
        return false;
    TopLoc_Location location;
    if (Handle(Geom_Plane)::DownCast(BRep_Tool::Surface(face, location)).IsNull() ||
        location.Transformation().ScaleFactor() != 1.0 ||
        (face.Orientation() != TopAbs_FORWARD && face.Orientation() != TopAbs_REVERSED))
        return false;
    const auto forward = TopoDS::Face(face.Oriented(TopAbs_FORWARD));
    try {
        std::vector<Boundary> wires;
        TopTools_DataMapOfShapeInteger owners;
        const double gap = 4 * std::max({Precision::Confusion(), BRep_Tool::Tolerance(face),
                                         BRep_Tool::MaxTolerance(face, TopAbs_EDGE),
                                         BRep_Tool::MaxTolerance(face, TopAbs_VERTEX)});
        if (!std::isfinite(gap))
            return false;
        for (TopoDS_Iterator it(forward); it.More(); it.Next()) {
            if (wires.size() % 64 == 0 && cancelled && cancelled())
                throw std::runtime_error("EVALUATION_CANCELLED");
            if (it.Value().ShapeType() != TopAbs_WIRE ||
                (it.Value().Orientation() != TopAbs_FORWARD &&
                 it.Value().Orientation() != TopAbs_REVERSED))
                return false;
            Boundary loop{TopoDS::Wire(it.Value()), {}, {}};
            if (!wire_box(loop, forward, gap, static_cast<int>(wires.size()), owners))
                return false;
            wires.push_back(loop);
        }
        // A strictly enclosing envelope identifies the only possible outside
        // wire. Every batch includes this actual wire, including concavities.
        const auto outside =
            std::min_element(wires.begin(), wires.end(),
                             [](const auto& a, const auto& b) { return a.low[0] < b.low[0]; });
        const auto outer = *outside;
        wires.erase(outside);
        for (const auto& w : wires)
            for (int i = 0; i < 2; ++i)
                if (w.low[i] <= outer.low[i] || w.high[i] >= outer.high[i])
                    return false;
        const int axis = outer.high[1] - outer.low[1] > outer.high[0] - outer.low[0] ? 1 : 0;
        auto ends = separate(wires, axis);
        if (ends.size() == 1)
            ends = separate(wires, 1 - axis);
        if (ends.size() == 1)
            return false;

        BRep_Builder builder;
        const auto make_outer = [&] {
            auto batch = TopoDS::Face(forward.EmptyCopied());
            builder.Add(batch, outer.wire);
            return batch;
        };
        if (BRepTopAdaptor_FClass2d(make_outer(), Precision::PConfusion()).PerformInfinitePoint() !=
            TopAbs_OUT)
            return false;
        TopoDS_Compound batches;
        builder.MakeCompound(batches);
        const auto inherited = certified_wires(forward, outer.wire, validated_faces, cancelled);
        std::size_t start = 0;
        for (const auto end : ends) {
            if (!validated_faces.empty() && std::all_of(wires.begin() + start, wires.begin() + end,
                    [&](const Boundary& loop) { return inherited.contains(loop.wire); })) {
                start = end;
                continue;
            }
            auto batch = make_outer();
            for (; start < end; ++start)
                builder.Add(batch, wires[start].wire);
            builder.Add(batches, batch.Oriented(face.Orientation()));
        }
        if (!batches.NbChildren())
            builder.Add(batches, make_outer().Oriented(face.Orientation()));
        builder.Add(checks, batches);
        return true;
    } catch (const Standard_Failure&) {
        return false;
    }
}
}  // namespace occccad::kernel::detail

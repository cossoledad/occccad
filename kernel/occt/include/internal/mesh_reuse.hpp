#pragma once

#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <BRepTools.hxx>
#include <Poly_Triangulation.hxx>
#include <Poly_PolygonOnTriangulation.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopTools_ShapeMapHasher.hxx>
#include <algorithm>
#include <iterator>
#include <memory>
#include <sstream>
#include <unordered_map>
#include <unordered_set>
#include <vector>

namespace occccad::kernel::detail {

// Exact, process-local identity only. Never serialized as a topology reference.
struct OrientedShapeHash {
    size_t operator()(const TopoDS_Shape& s) const noexcept {
        return TopTools_ShapeMapHasher{}(s) ^ (static_cast<size_t>(s.Orientation()) << 1);
    }
};
struct OrientedShapeEqual {
    bool operator()(const TopoDS_Shape& a, const TopoDS_Shape& b) const noexcept { return a.IsEqual(b); }
};

/** Immutable face packets are copied into an isolated meshing shape. Shared
 * source TShapes establish reuse; the normal boundary validator remains the
 * authority on compatibility with newly meshed adjacent faces. */
class MeshReuse {
    struct Edge {
        TopoDS_Edge source;
        Handle(Poly_PolygonOnTriangulation) forward, reverse;
    };
    struct Packet {
        TopoDS_Face source;
        Handle(Poly_Triangulation) triangulation;
        TopLoc_Location location;
        std::vector<Edge> edges;
        size_t bytes = 0;
    };
    using Frame = std::unordered_map<TopoDS_Shape,std::shared_ptr<const Packet>,OrientedShapeHash,OrientedShapeEqual>;
    std::vector<Frame> frames_;
    double linear_ = 0, angular_ = 0;
    size_t budget_ = 128 * 1024 * 1024;

    std::shared_ptr<const Packet> find(const TopoDS_Shape& face) const {
        for (const auto& frame : frames_) { const auto it = frame.find(face); if (it != frame.end()) return it->second; }
        return {};
    }
    size_t bytes() const {
        BRep_Builder builder; TopoDS_Compound geometry; builder.MakeCompound(geometry);
        std::unordered_set<const Packet*> unique; size_t result = 0;
        for (const auto& frame : frames_) for (const auto& entry : frame) if (unique.insert(entry.second.get()).second) {
            builder.Add(geometry,entry.second->source); result += entry.second->bytes + sizeof(Packet);
        }
        // Serialize the union: shared surfaces/curves are charged once, instead
        // of charging a full copy of the imported model for every result.
        std::ostringstream stream;
        BRepTools::Write(geometry,stream,Standard_False,Standard_False,TopTools_FormatVersion_CURRENT);
        return result + static_cast<size_t>(stream.tellp()) * 16;
    }
public:
    void clear() { frames_.clear(); }
    void policy(double linear, double angular) {
        if (linear != linear_ || angular != angular_) { clear(); linear_ = linear; angular_ = angular; }
    }
    size_t seed(const TopoDS_Shape& source, const BRepBuilderAPI_Copy& copy,
                std::vector<Handle(Poly_Triangulation)>& seeded) const {
        BRep_Builder builder; size_t count = 0;
        for (TopExp_Explorer faces(source,TopAbs_FACE); faces.More(); faces.Next()) {
            const auto packet = find(faces.Current()); if (!packet) continue;
            const auto face = TopoDS::Face(copy.ModifiedShape(packet->source));
            const auto triangulation = packet->triangulation->Copy();
            builder.UpdateFace(face,triangulation);
            for (const auto& edge : packet->edges) {
                const auto target = TopoDS::Edge(copy.ModifiedShape(edge.source));
                if (!edge.forward.IsNull() && !edge.reverse.IsNull())
                    builder.UpdateEdge(target,edge.forward->Copy(),edge.reverse->Copy(),triangulation,packet->location);
                else builder.UpdateEdge(target,(!edge.forward.IsNull()?edge.forward:edge.reverse)->Copy(),triangulation,packet->location);
            }
            seeded.push_back(triangulation); ++count;
        }
        return count;
    }
    void admit(const TopoDS_Shape& source, const BRepBuilderAPI_Copy& copy) {
        Frame frame;
        for (TopExp_Explorer faces(source,TopAbs_FACE); faces.More(); faces.Next()) {
            const auto original = TopoDS::Face(faces.Current());
            if (frame.find(original) != frame.end()) continue;
            const auto previous = find(original);
            if (previous) { frame.emplace(original,previous); continue; }
            auto packet = std::make_shared<Packet>(); packet->source = original;
            const auto face = TopoDS::Face(copy.ModifiedShape(original));
            const auto triangulation = BRep_Tool::Triangulation(face,packet->location);
            if (triangulation.IsNull()) continue;
            packet->triangulation = triangulation->Copy();
            packet->bytes = triangulation->NbNodes() * (sizeof(gp_Pnt)+sizeof(gp_Pnt2d)+sizeof(gp_Vec3f)) + triangulation->NbTriangles()*sizeof(Poly_Triangle);
            for (TopExp_Explorer uses(original.Oriented(TopAbs_FORWARD),TopAbs_EDGE); uses.More(); uses.Next()) {
                const auto originalEdge = TopoDS::Edge(uses.Current());
                const auto edge = TopoDS::Edge(copy.ModifiedShape(originalEdge).Oriented(originalEdge.Orientation()));
                const auto polygon = BRep_Tool::PolygonOnTriangulation(edge,triangulation,packet->location);
                if (polygon.IsNull()) continue;
                auto existing = std::find_if(packet->edges.begin(),packet->edges.end(),[&](const auto& e){return e.source.IsSame(originalEdge);});
                if (existing == packet->edges.end()) {packet->edges.push_back({originalEdge,{},{}}); existing = std::prev(packet->edges.end());}
                auto& target = originalEdge.Orientation() == TopAbs_REVERSED ? existing->reverse : existing->forward;
                target = polygon->Copy(); packet->bytes += polygon->NbNodes() * (sizeof(int)+sizeof(double));
            }
            frame.emplace(original,std::move(packet));
        }
        frames_.insert(frames_.begin(),std::move(frame));
        while (!frames_.empty() && (frames_.size() > 2 || bytes() > budget_)) frames_.pop_back();
    }
};
} // namespace occccad::kernel::detail

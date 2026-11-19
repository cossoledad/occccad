#pragma once

#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <Poly_Triangulation.hxx>
#include <Poly_PolygonOnTriangulation.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <TopoDS.hxx>
#include <algorithm>
#include <array>
#include <memory>
#include <exception>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include <internal/edge_profile.hpp>

namespace occccad::kernel::detail {

/** Immutable packets contain no BREP handles. A frame is addressed by the SHA
 * of the complete frozen BREP and locators within that exact snapshot, just as
 * mesh.faceIds are. Locators are never persistent naming or cross-Revision
 * identities. Only verified unchanged-copy correspondence can bridge frames. */
class MeshReuse {
    struct Edge {Handle(Poly_PolygonOnTriangulation) forward,reverse;};
    struct Payload {
        Handle(Poly_Triangulation) triangulation;
        std::vector<Edge> edges;
        size_t bytes=0;
    };
    struct Packet {
        std::shared_ptr<const Payload> payload;
        std::vector<int> edges;
        TopAbs_Orientation orientation;
    };
    struct EdgeUseValues {
        TopAbs_Orientation orientation;
        bool reversed;
        std::vector<gp_Pnt2d> uv;
        std::vector<double> parameters;
    };
    struct EdgeProfileValues {
        std::vector<Vec3> boundary;
        std::vector<EdgeUseValues> uses;
        std::vector<EdgeDisplaySegment> segments;
        size_t bytes = 0;
    };
    struct EdgeProfilePacket {
        std::shared_ptr<const EdgeProfileValues> values;
        std::array<uint32_t, 2> faces{};
    };
public:
    // The producer owns these records until all output boundaries are checked.
    struct EdgeProfiles {
        std::unordered_map<int, EdgeProfilePacket> packets;
        size_t new_bytes = 0;
    };
private:
    struct Frame {
        std::string geometry;
        std::unordered_map<int,std::shared_ptr<const Packet>> packets;
        EdgeProfiles profiles;
        uint64_t used=0;
    };
    std::vector<Frame> frames_;
    Frame pending_;
    std::string input_;
    std::vector<std::pair<int,int>> faces_,edges_;
    uint64_t clock_=0;
    double linear_=0,angular_=0;
    size_t budget_=128*1024*1024;
    Frame* find_frame(const std::string& geometry) {
        for(auto& frame:frames_)if(frame.geometry==geometry){frame.used=++clock_;return &frame;}
        return nullptr;
    }
    std::shared_ptr<const Packet> find(const std::string& geometry,int face) {
        if(pending_.geometry==geometry){const auto at=pending_.packets.find(face);if(at!=pending_.packets.end())return at->second;}
        if(auto* frame=find_frame(geometry)){const auto at=frame->packets.find(face);if(at!=frame->packets.end())return at->second;}
        return {};
    }
    const EdgeProfilePacket* find_profile(const std::string& geometry, int edge) {
        if (pending_.geometry == geometry) {
            const auto found = pending_.profiles.packets.find(edge);
            if (found != pending_.profiles.packets.end()) return &found->second;
        }
        if (const auto* frame = find_frame(geometry)) {
            const auto found = frame->profiles.packets.find(edge);
            if (found != frame->profiles.packets.end()) return &found->second;
        }
        return nullptr;
    }
    size_t bytes() const {
        std::unordered_set<const Payload*> unique;
        std::unordered_set<const EdgeProfileValues*> profiles;
        size_t result=(faces_.capacity()+edges_.capacity())*sizeof(std::pair<int,int>);
        const auto charge=[&](const Frame& frame){
            for(const auto& entry:frame.packets){
                result+=sizeof(entry)+sizeof(Packet)+entry.second->edges.capacity()*sizeof(int)+3*sizeof(void*);
                const auto& payload=entry.second->payload;
                if(unique.insert(payload.get()).second)result+=sizeof(Payload)+payload->bytes+payload->edges.capacity()*sizeof(Edge);
            }
            for (const auto& entry : frame.profiles.packets) {
                result += sizeof(entry) + 3 * sizeof(void*);
                if (profiles.insert(entry.second.values.get()).second)
                    result += entry.second.values->bytes;
            }
        };
        for(const auto& frame:frames_) charge(frame);
        charge(pending_);
        return result;
    }
public:
    using Correspondence=std::vector<std::pair<TopoDS_Shape,TopoDS_Shape>>;
    void discard_pending(){pending_=Frame{};input_.clear();faces_.clear();edges_.clear();}
    class FailureGuard {
        MeshReuse& owner_;int depth_=std::uncaught_exceptions();
    public:
        explicit FailureGuard(MeshReuse& owner):owner_(owner){}
        FailureGuard(const FailureGuard&)=delete;
        ~FailureGuard(){if(std::uncaught_exceptions()>depth_)owner_.discard_pending();}
    };
    FailureGuard on_failure(){return FailureGuard(*this);}

    void clear(){frames_.clear();pending_=Frame{};input_.clear();faces_={};edges_={};}
    bool has(const std::string& geometry, double linear, double angular) const {
        return linear == linear_ && angular == angular_ && std::any_of(frames_.begin(), frames_.end(), [&](const auto& frame) {
            return frame.geometry == geometry && !frame.packets.empty();
        });
    }
    void policy(double linear,double angular){
        if(linear!=linear_||angular!=angular_){frames_.clear();pending_.packets.clear();pending_.profiles={};linear_=linear;angular_=angular;}
    }
    void inherit(const Correspondence& pairs,const std::string& input,const TopoDS_Shape& before,
                 const std::string& output,const TopoDS_Shape& after){
        TopTools_IndexedMapOfShape oldFaces,newFaces,oldEdges,newEdges;
        TopExp::MapShapes(before,TopAbs_FACE,oldFaces);TopExp::MapShapes(after,TopAbs_FACE,newFaces);
        TopExp::MapShapes(before,TopAbs_EDGE,oldEdges);TopExp::MapShapes(after,TopAbs_EDGE,newEdges);
        input_=input;faces_.clear();edges_.clear();
        std::unordered_map<int,int> edges;
        for(const auto& pair:pairs){
            if(pair.first.Location()!=pair.second.Location())continue;
            if(pair.first.ShapeType()==TopAbs_EDGE){const int a=oldEdges.FindIndex(pair.first),b=newEdges.FindIndex(pair.second);if(a&&b){edges.emplace(a,b);edges_.emplace_back(a,b);}}
            if(pair.first.ShapeType()==TopAbs_FACE&&pair.first.Orientation()==pair.second.Orientation()){
                const int a=oldFaces.FindIndex(pair.first),b=newFaces.FindIndex(pair.second);if(a&&b)faces_.emplace_back(a,b);
            }
        }
        Frame next;next.geometry=output;
        for(const auto& face:faces_){
            const auto previous=find(input,face.first);if(!previous)continue;
            auto packet=std::make_shared<Packet>();packet->payload=previous->payload;packet->orientation=previous->orientation;bool complete=true;
            for(const auto edge:previous->edges){const auto at=edges.find(edge);if(at==edges.end()){complete=false;break;}packet->edges.push_back(at->second);}
            if(complete)next.packets.emplace(face.second,std::move(packet));
        }
        // Mesh eligibility alone does not prove identical support derivatives
        // or edge tolerances. Reuse profiles only for the same native subshapes.
        std::unordered_map<int, int> face_ids;
        for (const auto& face : faces_)
            if (oldFaces(face.first).IsSame(newFaces(face.second)))
                face_ids.emplace(face);
        for (const auto& edge : edges_) {
            const auto* source = find_profile(input, edge.first);
            if (!source || !oldEdges(edge.first).IsSame(newEdges(edge.second))) continue;
            auto packet = *source;
            bool complete = true;
            for (auto& face : packet.faces) {
                if (!face) continue;
                const auto match = face_ids.find(face);
                if (match == face_ids.end()) { complete = false; break; }
                face = match->second;
            }
            if (complete) next.profiles.packets.emplace(edge.second, std::move(packet));
        }
        pending_=std::move(next);if(bytes()>budget_)pending_=Frame{};
    }
    bool append_profile(const std::string& geometry, int edge, const std::vector<Vec3>& boundary,
                        const std::vector<VisualEdgeUse>& uses, TessellationResult& mesh, EdgeProfiles& accepted) {
        const auto* packet = find_profile(geometry, edge);
        if (!packet) return false;
        const auto& values = *packet->values;
        if (values.uses.size() != uses.size() || values.boundary.size() != boundary.size() ||
            !std::equal(boundary.begin(), boundary.end(), values.boundary.begin(), [](const auto& a, const auto& b) {
                return a.x == b.x && a.y == b.y && a.z == b.z;
            })) return false;
        for (size_t i = 0; i < uses.size(); ++i) {
            const auto& use = uses[i];
            const auto& old = values.uses[i];
            if (packet->faces[i] != use.face_id || old.orientation != use.edge.Orientation() ||
                old.reversed != use.reversed || old.parameters != use.parameters || old.uv.size() != use.uv.size() ||
                !std::equal(old.uv.begin(), old.uv.end(), use.uv.begin(), [](const auto& a, const auto& b) {
                    return a.X() == b.X() && a.Y() == b.Y();
                })) return false;
        }
        for (auto segment : values.segments) {
            segment.local_id = edge;
            segment.face_a = packet->faces[0];
            segment.face_b = packet->faces[1];
            if (segment.quality & 2) ++mesh.timings.edge_sampling_limits;
            mesh.edge_profile.push_back(segment);
        }
        accepted.packets.emplace(edge, *packet);
        return true;
    }
    void remember_profile(int edge, const std::vector<Vec3>& boundary, const std::vector<VisualEdgeUse>& uses,
                          const TessellationResult& mesh, size_t first, EdgeProfiles& accepted) {
        if (uses.size() > 2 || first == mesh.edge_profile.size()) return;
        size_t bytes = sizeof(EdgeProfileValues) + boundary.size() * sizeof(Vec3) +
                       (mesh.edge_profile.size() - first) * sizeof(EdgeDisplaySegment);
        for (const auto& use : uses)
            bytes += sizeof(EdgeUseValues) + use.uv.size() * sizeof(gp_Pnt2d) + use.parameters.size() * sizeof(double);
        if (bytes > 32 * 1024 * 1024 - accepted.new_bytes) return;
        auto values = std::make_shared<EdgeProfileValues>();
        values->boundary = boundary;
        values->segments.assign(mesh.edge_profile.begin() + first, mesh.edge_profile.end());
        values->bytes = bytes;
        EdgeProfilePacket packet{values};
        values->uses.reserve(uses.size());
        for (size_t i = 0; i < uses.size(); ++i) {
            const auto& use = uses[i];
            packet.faces[i] = use.face_id;
            values->uses.push_back({use.edge.Orientation(), use.reversed, use.uv, use.parameters});
        }
        accepted.new_bytes += bytes;
        accepted.packets.emplace(edge, std::move(packet));
    }
    size_t seed(const std::string& geometry,const TopoDS_Shape& source,const BRepBuilderAPI_Copy& copy,
                std::vector<Handle(Poly_Triangulation)>& seeded){
        TopTools_IndexedMapOfShape faces,edges;TopExp::MapShapes(source,TopAbs_FACE,faces);TopExp::MapShapes(source,TopAbs_EDGE,edges);
        BRep_Builder builder;size_t count=0;
        for(int id=1;id<=faces.Extent();++id){
            const auto original=TopoDS::Face(faces(id));const auto packet=find(geometry,id);if(!packet||packet->orientation!=original.Orientation())continue;
            TopTools_IndexedMapOfShape boundary;TopExp::MapShapes(original,TopAbs_EDGE,boundary);
            if(std::any_of(packet->edges.begin(),packet->edges.end(),[&](int e){return e<1||e>edges.Extent()||!boundary.Contains(edges(e));}))continue;
            const auto& payload=*packet->payload;const auto face=TopoDS::Face(copy.ModifiedShape(original));
            const auto triangulation=payload.triangulation->Copy();builder.UpdateFace(face,triangulation);
            // The exact placement can survive a BREP reload with a different
            // TopLoc datum stack. Bind polygons to the copied face's location.
            const auto& location = face.Location();
            for(size_t i=0;i<packet->edges.size();++i){
                const auto target=TopoDS::Edge(copy.ModifiedShape(edges(packet->edges[i])));const auto& edge=payload.edges[i];
                if(!edge.forward.IsNull()&&!edge.reverse.IsNull())builder.UpdateEdge(target,edge.forward->Copy(),edge.reverse->Copy(),triangulation,location);
                else builder.UpdateEdge(target,(!edge.forward.IsNull()?edge.forward:edge.reverse)->Copy(),triangulation,location);
            }
            seeded.push_back(triangulation);++count;
        }
        return count;
    }
    template<class CorrespondingShape>
    size_t admit(const std::string& geometry,const TopoDS_Shape& source,const CorrespondingShape& corresponding,
                 EdgeProfiles profiles = {}){
        Frame frame;frame.geometry=geometry;frame.profiles=std::move(profiles);
        TopTools_IndexedMapOfShape faces,edges;TopExp::MapShapes(source,TopAbs_FACE,faces);TopExp::MapShapes(source,TopAbs_EDGE,edges);
        for(int id=1;id<=faces.Extent();++id){
            const auto original=TopoDS::Face(faces(id));const auto previous=find(geometry,id);
            if(previous){frame.packets.emplace(id,previous);continue;}
            auto payload=std::make_shared<Payload>();auto packet=std::make_shared<Packet>();packet->payload=payload;packet->orientation=original.Orientation();
            const auto face=TopoDS::Face(corresponding(original));TopLoc_Location location;
            const auto triangulation=BRep_Tool::Triangulation(face,location);if(triangulation.IsNull())continue;
            payload->triangulation=triangulation->Copy();
            payload->bytes=triangulation->NbNodes()*(sizeof(gp_Pnt)+sizeof(gp_Pnt2d)+sizeof(gp_Vec3f))+triangulation->NbTriangles()*sizeof(Poly_Triangle);
            for(TopExp_Explorer uses(original.Oriented(TopAbs_FORWARD),TopAbs_EDGE);uses.More();uses.Next()){
                const auto originalEdge=TopoDS::Edge(uses.Current());const int edgeId=edges.FindIndex(originalEdge);
                const auto edge=TopoDS::Edge(corresponding(originalEdge).Oriented(originalEdge.Orientation()));
                const auto polygon=BRep_Tool::PolygonOnTriangulation(edge,triangulation,location);if(polygon.IsNull())continue;
                auto at=std::find(packet->edges.begin(),packet->edges.end(),edgeId);
                if(at==packet->edges.end()){packet->edges.push_back(edgeId);payload->edges.push_back({});at=std::prev(packet->edges.end());}
                auto& pair=payload->edges[std::distance(packet->edges.begin(),at)];auto& target=originalEdge.Orientation()==TopAbs_REVERSED?pair.reverse:pair.forward;
                target=polygon->Copy();payload->bytes+=polygon->NbNodes()*(sizeof(int)+sizeof(double));
            }
            frame.packets.emplace(id,std::move(packet));
        }
        if(!input_.empty()&&pending_.geometry==geometry){
            Frame input;input.geometry=input_;std::unordered_map<int,int> reverse;
            for(const auto& edge:edges_)reverse.emplace(edge.second,edge.first);
            for(const auto& face:faces_){
                const auto at=frame.packets.find(face.second);if(at==frame.packets.end())continue;
                auto packet=std::make_shared<Packet>();packet->payload=at->second->payload;packet->orientation=at->second->orientation;bool complete=true;
                for(const auto edge:at->second->edges){const auto key=reverse.find(edge);if(key==reverse.end()){complete=false;break;}packet->edges.push_back(key->second);}
                if(complete)input.packets.emplace(face.first,std::move(packet));
            }
            if(!input.packets.empty()){
                if(auto* previous=find_frame(input_)){for(auto& entry:input.packets)previous->packets.emplace(std::move(entry));}
                else{input.used=++clock_;frames_.push_back(std::move(input));}
            }
        }
        const auto count = frame.packets.size();
        frame.used=++clock_;
        frames_.erase(std::remove_if(frames_.begin(),frames_.end(),[&](const auto& f){return f.geometry==geometry;}),frames_.end());
        frames_.push_back(std::move(frame));pending_=Frame{};input_.clear();faces_.clear();edges_.clear();
        // LRU keeps the repeatedly used input and latest result; count shared
        // payloads once. This budget owns mesh data, never the original BREP graph.
        while(!frames_.empty()&&(frames_.size()>2||bytes()>budget_))frames_.erase(std::min_element(frames_.begin(),frames_.end(),[](const auto& a,const auto& b){return a.used<b.used;}));
        return has(geometry, linear_, angular_) ? count : 0;
    }
};
} // namespace occccad::kernel::detail

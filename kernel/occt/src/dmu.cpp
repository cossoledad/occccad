#include <internal/occt_kernel.hpp>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <gp_Pln.hxx>
#include <array>
#include <limits>
#include <map>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Section.hxx>
#include <BRepAdaptor_Curve.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepBuilderAPI_Copy.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepGProp.hxx>
#include <BRepMesh_IncrementalMesh.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GCPnts_QuasiUniformDeflection.hxx>
#include <GProp_GProps.hxx>
#include <Message_ProgressIndicator.hxx>
#include <Poly_Triangulation.hxx>
#include <Standard_Failure.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <gp_Quaternion.hxx>
#include <gp_Trsf.hxx>
#include <algorithm>
#include <cmath>
#include <sstream>

namespace occccad::kernel {
namespace {
class AnalysisProgress : public Message_ProgressIndicator {
public:
    explicit AnalysisProgress(std::function<bool()> check) : check_(std::move(check)) {}
    Standard_Boolean UserBreak() override { return check_ && check_(); }
    void Show(const Message_ProgressScope&, const Standard_Boolean) override {}
private:
    std::function<bool()> check_;
};
TopoDS_Shape placed(const TopoDS_Shape& shape, const PlacedGeometry& pose) {
    const auto& q=pose.rotation;
    const double norm=std::sqrt(q.x*q.x+q.y*q.y+q.z*q.z+q.w*q.w);
    if (!std::isfinite(norm) || std::abs(norm-1)>1e-8 ||
        !std::isfinite(pose.translation.x) || !std::isfinite(pose.translation.y) || !std::isfinite(pose.translation.z))
        throw std::invalid_argument("invalid analysis placement");
    gp_Trsf transform;
    transform.SetRotation(gp_Quaternion(q.x,q.y,q.z,q.w));
    transform.SetTranslationPart(gp_Vec(pose.translation.x,pose.translation.y,pose.translation.z));
    return BRepBuilderAPI_Transform(shape,transform,Standard_True).Shape();
}
double volume(const TopoDS_Shape& shape) {
    GProp_GProps props; BRepGProp::VolumeProperties(shape,props);
    return std::abs(props.Mass());
}
bool valid_solid(const TopoDS_Shape& shape) {
    if (shape.IsNull() || !BRepCheck_Analyzer(shape).IsValid()) return false;
    bool solid=false;
    for (TopExp_Explorer it(shape,TopAbs_SOLID);it.More();it.Next()) solid=true;
    for (const auto kind : {TopAbs_FACE,TopAbs_EDGE,TopAbs_VERTEX})
        if (TopExp_Explorer(shape,kind,TopAbs_SOLID).More()) return false;
    const double v=volume(shape);
    return solid && std::isfinite(v) && v>0;
}
Vec3 point(const gp_Pnt& p) { return {p.X(),p.Y(),p.Z()}; }
std::string placement_key(const PlacedGeometry& p) {
    std::ostringstream s;s<<p.geometry_id<<'/'<<std::hexfloat<<p.translation.x<<'/'<<p.translation.y<<'/'<<p.translation.z
        <<'/'<<p.rotation.x<<'/'<<p.rotation.y<<'/'<<p.rotation.z<<'/'<<p.rotation.w;return s.str();
}
// Test the actual residual material, not equality of the operands' volumes.
bool empty_difference(const TopoDS_Shape& a,const TopoDS_Shape& b,double eps,
                      const Handle(AnalysisProgress)& progress) {
    BRepAlgoAPI_Cut cut;TopTools_ListOfShape args,tools;args.Append(a);tools.Append(b);
    cut.SetArguments(args);cut.SetTools(tools);cut.SetNonDestructive(true);cut.SetRunParallel(false);cut.Build(progress->Start());
    if(!cut.IsDone()||cut.HasErrors()||(!cut.Shape().IsNull()&&!BRepCheck_Analyzer(cut.Shape()).IsValid()))
        throw std::runtime_error("CONTAINMENT_UNFINISHED");
    const double residual=volume(cut.Shape());
    if(!std::isfinite(residual))throw std::runtime_error("INVALID_RESIDUAL_VOLUME");
    return residual<=eps;
}
void graphics(const TopoDS_Shape& a,const TopoDS_Shape& b,const TopoDS_Shape& common,
              bool has_volume,InterferenceResult& out,const Handle(AnalysisProgress)& progress,
              const std::function<bool()>& cancelled) {
    out.detail_status="UNAVAILABLE";
    try {
        Bnd_Box box;BRepBndLib::Add(a,box);BRepBndLib::Add(b,box);
        const double deflection=std::max(1e-4,std::sqrt(box.SquareExtent())/1000.0);
        BRepAlgoAPI_Section section;section.Init1(a);section.Init2(b);section.SetRunParallel(false);
        section.Approximation(true);section.Build(progress->Start());
        if(!section.IsDone()||section.HasErrors())return;
        std::vector<Vec3> lines,triangles;
        for(TopExp_Explorer it(section.Shape(),TopAbs_EDGE);it.More();it.Next()) {
            if(cancelled&&cancelled())return;
            BRepAdaptor_Curve curve(TopoDS::Edge(it.Current()));
            GCPnts_QuasiUniformDeflection sample(curve,deflection);
            if(!sample.IsDone())return;
            if(lines.size()+2*sample.NbPoints()>32768){out.detail_status="LIMIT";return;}
            for(int i=2;i<=sample.NbPoints();++i){lines.push_back(point(sample.Value(i-1)));lines.push_back(point(sample.Value(i)));}
        }
        if(has_volume) {
            // Display meshing owns its geometry, never the cached exact inputs.
            auto shape=BRepBuilderAPI_Copy(common,true,false).Shape();
            BRepMesh_IncrementalMesh mesh(shape,deflection,false,0.35,false);
            mesh.Perform(progress->Start());if(!mesh.IsDone())return;
            for(TopExp_Explorer it(shape,TopAbs_FACE);it.More();it.Next()) {
                if(cancelled&&cancelled())return;
                const auto face=TopoDS::Face(it.Current());TopLoc_Location loc;
                auto poly=BRep_Tool::Triangulation(face,loc);if(poly.IsNull())return;
                if(triangles.size()+3*poly->NbTriangles()>90000){out.detail_status="LIMIT";return;}
                for(int i=1;i<=poly->NbTriangles();++i){int x,y,z;poly->Triangle(i).Get(x,y,z);
                    if(face.Orientation()==TopAbs_REVERSED)std::swap(y,z);
                    for(int n:{x,y,z})triangles.push_back(point(poly->Node(n).Transformed(loc.Transformation())));
                }
            }
        }
        if(cancelled&&cancelled())return;
        out.section_points=std::move(lines);out.overlap_triangles=std::move(triangles);out.detail_status="COMPLETE";
    } catch(const Standard_Failure&) {} catch(const std::exception&) {}
}
}
struct OcctKernel::InterferenceCache {
    struct Prepared {TopoDS_Shape shape;Bnd_Box box;double volume=0;bool valid=false;};
    std::unordered_map<std::string,std::shared_ptr<Prepared>> prepared;
    std::unordered_map<std::string,InterferenceResult> pairs;
};
InterferenceResult OcctKernel::analyze_interference(const PlacedGeometry& first,
    const PlacedGeometry& second,double clearance,double tolerance,const std::function<bool()>& cancelled,
    bool threshold_only,bool details) {
    InterferenceResult out;
    try {
        if (!std::isfinite(clearance) || clearance<0 || !std::isfinite(tolerance) || tolerance<=0)
            throw std::invalid_argument("invalid clearance/contact tolerance");
        if (cancelled && cancelled()) {out.diagnostic="CANCELLED";return out;}
        if(!interference_cache_)interference_cache_=std::make_shared<InterferenceCache>();
        auto& cache=*interference_cache_;
        auto prepare=[&](const PlacedGeometry& pose){
            const auto key=placement_key(pose);auto found=cache.prepared.find(key);
            if(found!=cache.prepared.end())return found->second;
            if(cache.prepared.size()>=128)cache.prepared.clear();
            auto value=std::make_shared<InterferenceCache::Prepared>();
            value->shape=placed(analysis_shape(pose.geometry_id),pose);value->valid=valid_solid(value->shape);
            if(value->valid){BRepBndLib::Add(value->shape,value->box,Standard_False);value->volume=volume(value->shape);}
            cache.prepared.emplace(key,value);return value;
        };
        const auto pa=prepare(first),pb=prepare(second);
        if(!pa->valid||!pb->valid){out.diagnostic="INVALID_SOLID";return out;}
        const auto& a=pa->shape;const auto& b=pb->shape;
        const double lower=pa->box.Distance(pb->box);
        if(threshold_only&&!details&&lower>std::max(clearance,tolerance)){
            out.classification="SEPARATED";out.complete=true;out.clearance_satisfied=true;return out;
        }
        std::ostringstream key;key<<placement_key(first)<<'|'<<placement_key(second)<<'|'<<std::hexfloat<<tolerance;
        if(!details){auto found=cache.pairs.find(key.str());if(found!=cache.pairs.end()){
            out=found->second;out.clearance_satisfied=out.classification=="SEPARATED"||out.classification=="CONTACT" ? out.distance>=clearance:false;return out;
        }}
        Handle(AnalysisProgress) progress=new AnalysisProgress(cancelled);
        BRepExtrema_DistShapeShape distance;
        distance.LoadS1(a);distance.LoadS2(b);distance.SetMultiThread(Standard_False);
        distance.Perform(progress->Start());
        if (!distance.IsDone() || !distance.NbSolution() || (cancelled && cancelled())) {
            out.diagnostic="DISTANCE_UNFINISHED";return out;
        }
        out.distance=distance.Value();out.distance_exact=true;
        if (!std::isfinite(out.distance) || out.distance<0) {out.diagnostic="INVALID_DISTANCE";return out;}
        out.first_witness=point(distance.PointOnShape1(1));out.second_witness=point(distance.PointOnShape2(1));
        out.witness_kind=out.distance<=tolerance?"CONTACT":"DISTANCE";
        TopoDS_Shape common_shape;
        if (lower<=tolerance) {
            BRepAlgoAPI_Common common;TopTools_ListOfShape args,tools;args.Append(a);tools.Append(b);
            common.SetArguments(args);common.SetTools(tools);common.SetNonDestructive(true);
            common.SetRunParallel(false);common.Build(progress->Start());
            if (!common.IsDone() || common.HasErrors() || (cancelled && cancelled())) {
                out.diagnostic="COMMON_UNFINISHED";return out;
            }
            common_shape=common.Shape();
            if (!common_shape.IsNull() && !BRepCheck_Analyzer(common_shape).IsValid()) {
                out.diagnostic="INVALID_COMMON";return out;
            }
            out.common_tested=true;out.common_volume=volume(common_shape);
            if(!std::isfinite(out.common_volume)){out.diagnostic="INVALID_COMMON_VOLUME";return out;}
            // A positive solid intersection is never erased by a relative-volume
            // threshold. Tolerance applies only to the residual containment proof.
            bool solid=false;for(TopExp_Explorer it(common_shape,TopAbs_SOLID);it.More();it.Next())solid=true;
            if (solid&&out.common_volume>0) {
                const double eps=std::pow(tolerance,3);
                bool ai=false,bi=false;
                if(std::abs(out.common_volume-pa->volume)<=std::max(eps,pa->volume*1e-12))ai=empty_difference(a,b,eps,progress);
                if(std::abs(out.common_volume-pb->volume)<=std::max(eps,pb->volume*1e-12))bi=empty_difference(b,a,eps,progress);
                out.classification=ai||bi?"CONTAINMENT":"PENETRATION";
                out.containment=ai&&bi?"COINCIDENT":ai?"FIRST_INSIDE_SECOND":bi?"SECOND_INSIDE_FIRST":"";
                out.distance=0;out.witness_kind="NONE";
            }
        }
        if(out.classification=="INCONCLUSIVE"){
            out.classification=out.distance<=tolerance?"CONTACT":"SEPARATED";
            out.clearance_satisfied=out.distance>=clearance;
        }
        if(cancelled&&cancelled()){out=InterferenceResult{};out.diagnostic="CANCELLED";return out;}
        out.complete=true;
        if(details){graphics(a,b,common_shape,out.common_volume>0,out,progress,cancelled);if(cancelled&&cancelled()){out=InterferenceResult{};out.diagnostic="CANCELLED";}}
        else {if(cache.pairs.size()>=512)cache.pairs.clear();cache.pairs[key.str()]=out;}
    } catch(const Standard_Failure& e) {out=InterferenceResult{};out.diagnostic=e.GetMessageString();}
      catch(const std::exception& e) {out=InterferenceResult{};out.diagnostic=e.what();}
    return out;
}
}

namespace occccad::kernel {
SpatialGeometryResult OcctKernel::analyze_spatial(const PlacedGeometry& input,
    const std::vector<PlacedGeometry>& target_inputs,const SpatialGeometryOptions& o,const std::function<bool()>& cancelled) {
    SpatialGeometryResult out;
    try {
        if(!std::isfinite(o.accuracy)||o.accuracy<0.01||o.accuracy>1000)throw std::invalid_argument("INVALID_SPATIAL_ACCURACY");
        if(cancelled&&cancelled())throw std::runtime_error("CANCELLED");
        auto source=placed(analysis_shape(input.geometry_id),input);
        if(!valid_solid(source))throw std::invalid_argument("INVALID_SOLID");
        Handle(AnalysisProgress) progress=new AnalysisProgress(cancelled);
        auto mesh_triangles=[&](const TopoDS_Shape& shape){
            if(shape.IsNull())return;
            auto copy=BRepBuilderAPI_Copy(shape,true,false).Shape();
            BRepMesh_IncrementalMesh mesh(copy,o.accuracy/8,false,0.25,false);mesh.Perform(progress->Start());
            if(!mesh.IsDone())throw std::runtime_error("SPATIAL_MESH_UNFINISHED");
            for(TopExp_Explorer it(copy,TopAbs_FACE);it.More();it.Next()){
                const auto face=TopoDS::Face(it.Current());TopLoc_Location loc;auto poly=BRep_Tool::Triangulation(face,loc);
                if(poly.IsNull())throw std::runtime_error("SPATIAL_MISSING_FACE");
                if(out.triangles.size()+3*poly->NbTriangles()>90000)throw std::runtime_error("SPATIAL_GRAPHICS_LIMIT");
                for(int i=1;i<=poly->NbTriangles();++i){int a,b,c;poly->Triangle(i).Get(a,b,c);if(face.Orientation()==TopAbs_REVERSED)std::swap(b,c);for(int n:{a,b,c})out.triangles.push_back(point(poly->Node(n).Transformed(loc.Transformation())));}
            }
        };
        if(o.kind=="SECTION"){
            if((o.section_type!="PLANE"&&o.section_type!="SLICE"&&o.section_type!="BOX") ||
               !std::isfinite(o.size.x)||!std::isfinite(o.size.y)||!std::isfinite(o.size.z)||o.size.x<=0||o.size.y<=0||o.size.z<=0)
                throw std::invalid_argument("INVALID_SECTION_DEFINITION");
            const auto& q=o.frame.rotation;gp_Trsf frame;frame.SetRotation(gp_Quaternion(q.x,q.y,q.z,q.w));frame.SetTranslationPart(gp_Vec(o.frame.translation.x,o.frame.translation.y,o.frame.translation.z));
            // Validate the frame through the same placement contract, then use
            // exact local geometry. Plane dimensions only size its representation.
            PlacedGeometry inverse;inverse.rotation={-q.x,-q.y,-q.z,q.w};
            gp_Trsf inv=frame.Inverted();
            inverse.translation={inv.TranslationPart().X(),inv.TranslationPart().Y(),inv.TranslationPart().Z()};
            auto local=placed(source,inverse);
            struct Plane {gp_Pnt point;gp_Dir normal;};std::vector<Plane> planes;
            if(o.section_type=="BOX")planes={{{o.size.x/2,0,0},{1,0,0}},{{-o.size.x/2,0,0},{1,0,0}},{{0,o.size.y/2,0},{0,1,0}},{{0,-o.size.y/2,0},{0,1,0}},{{0,0,o.size.z/2},{0,0,1}},{{0,0,-o.size.z/2},{0,0,1}}};
            else if(o.section_type=="SLICE")planes={{{0,0,o.size.z/2},{0,0,1}},{{0,0,-o.size.z/2},{0,0,1}}};
            else planes={{{0,0,0},{0,0,1}}};
            Bnd_Box bounds;BRepBndLib::Add(local,bounds,false);double x0,y0,z0,x1,y1,z1;bounds.Get(x0,y0,z0,x1,y1,z1);
            const double extent=std::max({std::abs(x0),std::abs(y0),std::abs(z0),std::abs(x1),std::abs(y1),std::abs(z1),o.size.x,o.size.y,o.size.z,1.0})*4;
            TopoDS_Shape region=local;
            if(o.section_type=="BOX"){
                auto box=BRepPrimAPI_MakeBox(gp_Pnt(-o.size.x/2,-o.size.y/2,-o.size.z/2),o.size.x,o.size.y,o.size.z).Shape();
                BRepAlgoAPI_Common common;TopTools_ListOfShape a,b;a.Append(local);b.Append(box);common.SetArguments(a);common.SetTools(b);common.SetNonDestructive(true);common.SetRunParallel(false);common.Build(progress->Start());
                if(!common.IsDone()||common.HasErrors())throw std::runtime_error("SECTION_BOX_UNFINISHED");region=common.Shape();
            }
            double area=0;gp_XYZ center(0,0,0);
            for(const auto& p:planes){
                if(cancelled&&cancelled())throw std::runtime_error("CANCELLED");
                auto face=BRepBuilderAPI_MakeFace(gp_Pln(p.point,p.normal),-extent,extent,-extent,extent).Shape();
                BRepAlgoAPI_Common cross;TopTools_ListOfShape a,b;a.Append(region);b.Append(face);cross.SetArguments(a);cross.SetTools(b);cross.SetNonDestructive(true);cross.SetRunParallel(false);cross.Build(progress->Start());
                if(!cross.IsDone()||cross.HasErrors()||(!cross.Shape().IsNull()&&!BRepCheck_Analyzer(cross.Shape()).IsValid()))throw std::runtime_error("SECTION_UNFINISHED");
                const auto section=cross.Shape();GProp_GProps props;BRepGProp::SurfaceProperties(section,props);
                const double value=std::abs(props.Mass());if(!std::isfinite(value))throw std::runtime_error("INVALID_SECTION_AREA");
                if(value>0){area+=value;center+=props.CentreOfMass().XYZ()*value;}
                for(TopExp_Explorer edge(section,TopAbs_EDGE);edge.More();edge.Next()){
                    BRepAdaptor_Curve curve(TopoDS::Edge(edge.Current()));GCPnts_QuasiUniformDeflection sample(curve,o.accuracy/8);
                    if(!sample.IsDone())throw std::runtime_error("SECTION_CURVE_UNFINISHED");
                    if(out.segments.size()+2*sample.NbPoints()>32768)throw std::runtime_error("SPATIAL_GRAPHICS_LIMIT");
                    for(int n=2;n<=sample.NbPoints();++n){out.segments.push_back(point(sample.Value(n-1).Transformed(frame)));out.segments.push_back(point(sample.Value(n).Transformed(frame)));}
                }
                mesh_triangles(BRepBuilderAPI_Transform(section,frame,true).Shape());
            }
            out.area=area;if(area>0)out.centroid=point(gp_Pnt(center/area).Transformed(frame));
        } else if(o.kind=="BAND") {
            if(target_inputs.empty()||target_inputs.size()>128||!std::isfinite(o.minimum)||!std::isfinite(o.maximum)||o.minimum<0||o.maximum<=o.minimum)throw std::invalid_argument("INVALID_DISTANCE_BAND");
            std::vector<TopoDS_Shape> targets;std::vector<Bnd_Box> boxes;
            for(const auto& input:target_inputs){auto shape=placed(analysis_shape(input.geometry_id),input);if(!valid_solid(shape))throw std::invalid_argument("INVALID_BAND_TARGET");Bnd_Box b;BRepBndLib::Add(shape,b,false);targets.push_back(shape);boxes.push_back(b);}
            size_t queries=0;std::map<std::array<double,3>,double> distances;
            auto distance=[&](const gp_Pnt& p){
                const std::array<double,3> key{p.X(),p.Y(),p.Z()};const auto found=distances.find(key);if(found!=distances.end())return found->second;
                if(++queries>10000)throw std::runtime_error("SPATIAL_QUERY_LIMIT");
                if(cancelled&&cancelled())throw std::runtime_error("CANCELLED");
                double best=std::numeric_limits<double>::infinity();Bnd_Box pb;pb.Add(p);const auto vertex=BRepBuilderAPI_MakeVertex(p).Shape();
                for(size_t i=0;i<targets.size();++i){if(boxes[i].Distance(pb)>best)continue;BRepExtrema_DistShapeShape d;d.LoadS1(vertex);d.LoadS2(targets[i]);d.SetMultiThread(false);d.Perform(progress->Start());if(!d.IsDone()||!d.NbSolution()||!std::isfinite(d.Value()))throw std::runtime_error("BAND_DISTANCE_UNFINISHED");best=std::min(best,d.Value());}
                distances.emplace(key,best);return best;
            };
            auto copy=BRepBuilderAPI_Copy(source,true,false).Shape();BRepMesh_IncrementalMesh mesh(copy,o.accuracy/8,false,0.25,false);mesh.Perform(progress->Start());if(!mesh.IsDone())throw std::runtime_error("SPATIAL_MESH_UNFINISHED");
            for(TopExp_Explorer faceIt(copy,TopAbs_FACE);faceIt.More();faceIt.Next()){
                const auto face=TopoDS::Face(faceIt.Current());BRepAdaptor_Surface surface(face);TopLoc_Location loc;auto poly=BRep_Tool::Triangulation(face,loc);if(poly.IsNull()||!poly->HasUVNodes())throw std::runtime_error("BAND_UV_UNAVAILABLE");
                std::function<void(gp_Pnt2d,gp_Pnt2d,gp_Pnt2d,int)> visit;
                visit=[&](gp_Pnt2d a,gp_Pnt2d b,gp_Pnt2d c,int depth){
                    if(cancelled&&cancelled())throw std::runtime_error("CANCELLED");
                    const auto pa=surface.Value(a.X(),a.Y()),pb=surface.Value(b.X(),b.Y()),pc=surface.Value(c.X(),c.Y());
                    const gp_Pnt2d uv((a.X()+b.X()+c.X())/3,(a.Y()+b.Y()+c.Y())/3);const auto middle=surface.Value(uv.X(),uv.Y());
                    const double d=distance(middle),radius=std::max({middle.Distance(pa),middle.Distance(pb),middle.Distance(pc)})+o.accuracy/8;
                    const double lower=std::max(0.0,d-radius),upper=d+radius;
                    if(lower>o.maximum)return;
                    uint32_t tag=2;
                    if(upper<o.minimum)tag=0;else if(lower>=o.minimum&&upper<=o.maximum)tag=1;
                    const double edge=std::max({pa.Distance(pb),pb.Distance(pc),pc.Distance(pa)});
                    if(tag==2&&edge>o.accuracy){if(depth>=12)throw std::runtime_error("SPATIAL_REFINEMENT_LIMIT");auto ab=gp_Pnt2d((a.X()+b.X())/2,(a.Y()+b.Y())/2),bc=gp_Pnt2d((b.X()+c.X())/2,(b.Y()+c.Y())/2),ca=gp_Pnt2d((c.X()+a.X())/2,(c.Y()+a.Y())/2);visit(a,ab,ca,depth+1);visit(ab,b,bc,depth+1);visit(ca,bc,c,depth+1);visit(ab,bc,ca,depth+1);return;}
                    if(out.triangles.size()+3>60000)throw std::runtime_error("SPATIAL_GRAPHICS_LIMIT");
                    out.triangles.push_back(point(pa));out.triangles.push_back(point(pb));out.triangles.push_back(point(pc));out.regions.push_back(tag);
                };
                for(int n=1;n<=poly->NbTriangles();++n){int a,b,c;poly->Triangle(n).Get(a,b,c);visit(poly->UVNode(a),poly->UVNode(b),poly->UVNode(c),0);}
            }
        } else throw std::invalid_argument("UNKNOWN_SPATIAL_ANALYSIS");
        if(cancelled&&cancelled())throw std::runtime_error("CANCELLED");out.complete=true;
    } catch(const Standard_Failure& e){out=SpatialGeometryResult{};out.diagnostic=e.GetMessageString()?e.GetMessageString():"SPATIAL_COMPUTATION_FAILED";}
      catch(const std::exception& e){out=SpatialGeometryResult{};out.diagnostic=e.what();}
    return out;
}
}

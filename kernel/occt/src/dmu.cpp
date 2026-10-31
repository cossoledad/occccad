#include <internal/occt_kernel.hpp>
#include <BRepAlgoAPI_Common.hxx>
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

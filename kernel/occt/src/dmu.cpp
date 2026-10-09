#include <internal/occt_kernel.hpp>
#include <BRepAlgoAPI_Common.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepGProp.hxx>
#include <Bnd_Box.hxx>
#include <GProp_GProps.hxx>
#include <Message_ProgressIndicator.hxx>
#include <Standard_Failure.hxx>
#include <TopExp_Explorer.hxx>
#include <gp_Quaternion.hxx>
#include <gp_Trsf.hxx>
#include <algorithm>
#include <cmath>

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
    // Isolate OCCT algorithms from shared source geometry and its tolerances.
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
    // Reject open/surface-only shapes. They cannot certify solid interference.
    for (const auto kind : {TopAbs_FACE,TopAbs_EDGE,TopAbs_VERTEX})
        if (TopExp_Explorer(shape,kind,TopAbs_SOLID).More()) return false;
    const double v=volume(shape);
    return solid && std::isfinite(v) && v>0;
}
Vec3 point(const gp_Pnt& p) { return {p.X(),p.Y(),p.Z()}; }
}
InterferenceResult OcctKernel::analyze_interference(const PlacedGeometry& first,
    const PlacedGeometry& second,double clearance,double tolerance,const std::function<bool()>& cancelled) {
    InterferenceResult out;
    try {
        if (!std::isfinite(clearance) || clearance<0 || !std::isfinite(tolerance) || tolerance<=0)
            throw std::invalid_argument("invalid clearance/contact tolerance");
        if (cancelled && cancelled()) {out.diagnostic="CANCELLED";return out;}
        const auto a=placed(analysis_shape(first.geometry_id),first);
        const auto b=placed(analysis_shape(second.geometry_id),second);
        if (!valid_solid(a) || !valid_solid(b)) {out.diagnostic="INVALID_SOLID";return out;}
        Handle(AnalysisProgress) progress=new AnalysisProgress(cancelled);
        BRepExtrema_DistShapeShape distance;
        distance.LoadS1(a);distance.LoadS2(b);distance.SetMultiThread(Standard_False);
        distance.Perform(progress->Start());
        if (!distance.IsDone() || !distance.NbSolution() || (cancelled && cancelled())) {
            out.diagnostic="DISTANCE_UNFINISHED";return out;
        }
        out.distance=distance.Value();
        if (!std::isfinite(out.distance) || out.distance<0) {out.diagnostic="INVALID_DISTANCE";return out;}
        out.first_witness=point(distance.PointOnShape1(1));out.second_witness=point(distance.PointOnShape2(1));
        Bnd_Box ba,bb;BRepBndLib::Add(a,ba);BRepBndLib::Add(b,bb);
        ba.Enlarge(std::max(clearance,tolerance));bb.Enlarge(std::max(clearance,tolerance));
        // Conservative boxes prune Boolean work, not the exact minimum-distance query.
        if (!ba.IsOut(bb)) {
            BRepAlgoAPI_Common common;
            TopTools_ListOfShape args,tools;args.Append(a);tools.Append(b);
            common.SetArguments(args);common.SetTools(tools);common.SetNonDestructive(Standard_True);
            common.SetRunParallel(Standard_False);common.Build(progress->Start());
            if (!common.IsDone() || common.HasErrors() || (cancelled && cancelled())) {
                out.diagnostic="COMMON_UNFINISHED";return out;
            }
            if (!common.Shape().IsNull() && !BRepCheck_Analyzer(common.Shape()).IsValid()) {
                out.diagnostic="INVALID_COMMON";return out;
            }
            out.common_tested=true;out.common_volume=volume(common.Shape());
            if(!std::isfinite(out.common_volume)){out.diagnostic="INVALID_COMMON_VOLUME";return out;}
            const double va=volume(a),vb=volume(b);
            const double volume_tolerance=std::max(std::pow(tolerance,3),std::min(va,vb)*1e-12);
            if (out.common_volume>volume_tolerance) {
                out.classification=std::abs(out.common_volume-std::min(va,vb))<=volume_tolerance ? "CONTAINMENT" : "PENETRATION";
                out.distance=0;out.complete=true;return out;
            }
        }
        out.classification=out.distance<=tolerance ? "CONTACT" : "SEPARATED";
        out.clearance_satisfied=out.distance>=clearance;
        out.complete=true;
    } catch(const Standard_Failure& e) {out.diagnostic=e.GetMessageString();}
      catch(const std::exception& e) {out.diagnostic=e.what();}
    return out;
}
}

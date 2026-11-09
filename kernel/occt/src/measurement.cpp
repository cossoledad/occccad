#include <internal/mass_properties.hpp>
#include <internal/curvature.hpp>
#include <internal/occt_kernel.hpp>
#include <BRepAdaptor_Curve.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepBndLib.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepBuilderAPI_MakeVertex.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepExtrema_DistShapeShape.hxx>
#include <BRepGProp.hxx>
#include <BRep_Builder.hxx>
#include <BRep_Tool.hxx>
#include <Bnd_Box.hxx>
#include <GProp_GProps.hxx>
#include <Message_ProgressIndicator.hxx>
#include <Standard_Failure.hxx>
#include <TopExp.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Compound.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <gp_Quaternion.hxx>
#include <gp_Trsf.hxx>
#include <algorithm>
#include <cmath>
#include <optional>

namespace occccad::kernel {
namespace {
constexpr double eps=1e-7;
Vec3 vec(const gp_Pnt& p){return {p.X(),p.Y(),p.Z()};}
gp_Pnt pnt(const Vec3& p){return {p.x,p.y,p.z};}
Vec3 vec(const gp_Dir& p){return {p.X(),p.Y(),p.Z()};}
class MeasureProgress : public Message_ProgressIndicator {
public:
    explicit MeasureProgress(std::function<bool()> f):check_(std::move(f)){}
    Standard_Boolean UserBreak() override {return check_&&check_();}
    void Show(const Message_ProgressScope&,const Standard_Boolean) override {}
private: std::function<bool()> check_;
};
struct Support {
    TopoDS_Shape shape;
    std::string type{"BODY"};
    std::optional<gp_Pnt> center;
    std::optional<gp_Dir> direction;
    bool plane=false;
    std::optional<double> radius,minor_radius,arc_angle;
};
TopoDS_Shape select_shape(const TopoDS_Shape& root,const MeasurementElement& e){
    if(e.kind=="BODY"&&e.local_id==0)return root;
    const auto t=e.kind=="FACE"?TopAbs_FACE:e.kind=="EDGE"?TopAbs_EDGE:e.kind=="VERTEX"?TopAbs_VERTEX:TopAbs_SHAPE;
    if(t==TopAbs_SHAPE)throw std::invalid_argument("MEASUREMENT_SUPPORT_INVALID");
    TopTools_IndexedMapOfShape map;TopExp::MapShapes(root,t,map);
    if(e.local_id==0||e.local_id>static_cast<uint64_t>(map.Extent()))throw std::invalid_argument("MEASUREMENT_SUPPORT_INVALID");
    return map(static_cast<int>(e.local_id));
}
Support prepare(const TopoDS_Shape& root,const MeasurementElement& e){
    const auto& q=e.geometry.rotation;const auto& t=e.geometry.translation;
    const double n=std::hypot(std::hypot(q.x,q.y),std::hypot(q.z,q.w));
    if(!std::isfinite(n)||std::abs(n-1)>1e-8||!std::isfinite(t.x)||!std::isfinite(t.y)||!std::isfinite(t.z))
        throw std::invalid_argument("MEASUREMENT_PLACEMENT_INVALID");
    gp_Trsf tr;tr.SetRotation(gp_Quaternion(q.x,q.y,q.z,q.w));tr.SetTranslationPart(gp_Vec(t.x,t.y,t.z));
    Support s;s.shape=BRepBuilderAPI_Transform(select_shape(root,e),tr,true).Shape();
    if(e.kind=="VERTEX"){s.type="POINT";s.center=BRep_Tool::Pnt(TopoDS::Vertex(s.shape));}
    if(e.kind=="EDGE"){
        BRepAdaptor_Curve c(TopoDS::Edge(s.shape));
        switch(c.GetType()){
        case GeomAbs_Line:s.type="LINE";s.direction=c.Line().Direction();break;
        case GeomAbs_Circle:s.type="CIRCLE";s.radius=c.Circle().Radius();s.center=c.Circle().Location();s.direction=c.Circle().Axis().Direction();s.arc_angle=std::abs(c.LastParameter()-c.FirstParameter());break;
        case GeomAbs_Ellipse:s.type="ELLIPSE";s.radius=c.Ellipse().MajorRadius();s.minor_radius=c.Ellipse().MinorRadius();s.center=c.Ellipse().Location();s.direction=c.Ellipse().Axis().Direction();break;
        default:s.type="CURVE";
        }
    }
    if(e.kind=="FACE"){
        BRepAdaptor_Surface f(TopoDS::Face(s.shape));
        switch(f.GetType()){
        case GeomAbs_Plane:s.type="PLANE";s.direction=f.Plane().Axis().Direction();s.plane=true;break;
        case GeomAbs_Cylinder:s.type="CYLINDER";s.radius=f.Cylinder().Radius();s.direction=f.Cylinder().Axis().Direction();s.center=f.Cylinder().Location();break;
        case GeomAbs_Sphere:s.type="SPHERE";s.radius=f.Sphere().Radius();s.center=f.Sphere().Location();break;
        case GeomAbs_Cone:s.type="CONE";s.direction=f.Cone().Axis().Direction();s.center=f.Cone().Apex();break;
        case GeomAbs_Torus:s.type="TORUS";s.radius=f.Torus().MajorRadius();s.minor_radius=f.Torus().MinorRadius();s.direction=f.Torus().Axis().Direction();s.center=f.Torus().Location();break;
        default:s.type="SURFACE";
        }
    }
    if(e.mode=="CENTER"){
        if(!s.center||(s.type!="POINT"&&s.type!="CIRCLE"&&s.type!="ELLIPSE"&&s.type!="SPHERE"))
            throw std::invalid_argument("MEASUREMENT_CENTER_UNSUPPORTED");
        s.shape=BRepBuilderAPI_MakeVertex(*s.center).Shape();s.type="POINT";s.direction.reset();s.plane=false;s.radius.reset();s.minor_radius.reset();s.arc_angle.reset();
    }else if(e.mode=="AXIS"){
        if(!s.center||!s.direction||s.type=="PLANE"||s.type=="LINE")throw std::invalid_argument("MEASUREMENT_AXIS_UNSUPPORTED");
        s.shape=BRepBuilderAPI_MakeEdge(gp_Lin(*s.center,*s.direction)).Shape();s.type="AXIS";s.radius.reset();s.minor_radius.reset();s.arc_angle.reset();
    }else if(e.mode!="GEOMETRY")throw std::invalid_argument("MEASUREMENT_MODE_INVALID");
    return s;
}
TopoDS_Shape compound(const std::vector<Support>& supports){
    if(supports.size()==1)return supports.front().shape;
    BRep_Builder builder;TopoDS_Compound out;builder.MakeCompound(out);
    for(const auto& s:supports)builder.Add(out,s.shape);
    return out;
}
void add_point(MeasurementResult& r,const std::string& key,const gp_Pnt& p){r.anchors[key]=vec(p);}
void item(const std::vector<Support>& ss,MeasurementResult& r,const std::function<bool()>& cancelled){
    const auto& s=ss.front();r.geometry_type=ss.size()==1?s.type:"BODY_SET";
    if(ss.size()==1&&s.type=="POINT"){
        const auto p=BRep_Tool::Pnt(TopoDS::Vertex(s.shape));add_point(r,"position",p);
        r.values["xMm"]=p.X();r.values["yMm"]=p.Y();r.values["zMm"]=p.Z();return;
    }
    if(s.type=="AXIS")throw std::invalid_argument("MEASUREMENT_ITEM_AXIS_UNBOUNDED");
    const auto shape=compound(ss);GProp_GProps props;
    if(ss.size()==1&&s.shape.ShapeType()==TopAbs_EDGE){
        BRepGProp::LinearProperties(shape,props);r.values["lengthMm"]=props.Mass();
        BRepAdaptor_Curve c(TopoDS::Edge(shape));add_point(r,"start",c.Value(c.FirstParameter()));add_point(r,"end",c.Value(c.LastParameter()));
        if(s.arc_angle)r.values["arcAngleRad"]=*s.arc_angle;
    }else{
        BRepGProp::SurfaceProperties(shape,props,1e-10);r.values["areaMm2"]=props.Mass();
        if(ss.size()==1&&s.shape.ShapeType()==TopAbs_FACE){
            // Periodic parameter seams are not physical face boundaries.
            // LinearProperties(face) would count both uses of a cylinder seam.
            const auto face=TopoDS::Face(shape);TopTools_IndexedMapOfShape edges;TopExp::MapShapes(face,TopAbs_EDGE,edges);
            double perimeter=0;
            for(int i=1;i<=edges.Extent();++i){
                if(cancelled&&cancelled())throw std::runtime_error("MEASUREMENT_CANCELLED");
                const auto edge=TopoDS::Edge(edges(i));
                if(BRep_Tool::Degenerated(edge)||BRep_Tool::IsClosed(edge,face))continue;
                GProp_GProps boundary;BRepGProp::LinearProperties(edge,boundary);perimeter+=boundary.Mass();
            }
            r.values["perimeterMm"]=perimeter;
        }else{
            detail::volume_properties(shape,props,true);r.values["volumeMm3"]=std::abs(props.Mass());
        }
    }
    if(std::abs(props.Mass())>1e-20)add_point(r,"centroid",props.CentreOfMass());
    if(s.radius){
        if(s.type=="ELLIPSE"||s.type=="TORUS")r.values["majorRadiusMm"]=*s.radius;
        else {r.values["radiusMm"]=*s.radius;r.values["diameterMm"]=2 * *s.radius;}
    }
    if(s.minor_radius)r.values["minorRadiusMm"]=*s.minor_radius;
    if(s.center)add_point(r,s.type=="CYLINDER"?"axisOrigin":s.type=="CONE"?"apex":"center",*s.center);
    if(s.direction)r.anchors["direction"]=vec(*s.direction);
    Bnd_Box box;BRepBndLib::AddOptimal(shape,box,false,false);
    if(!box.IsVoid()&&!box.IsOpen()){
        double x,y,z,a,b,c;box.Get(x,y,z,a,b,c);r.anchors["bboxMin"]={x,y,z};r.anchors["bboxMax"]={a,b,c};
        r.values["sizeXMm"]=a-x;r.values["sizeYMm"]=b-y;r.values["sizeZMm"]=c-z;
    }
}
}
MeasurementResult OcctKernel::measure_geometry(const std::string& kind,const std::vector<std::vector<MeasurementElement>>& operands,const std::function<bool()>& cancelled,uint32_t sample_count){
    MeasurementResult r;
    try{
        const size_t required=(kind=="ITEM"||kind=="CURVATURE")?1:kind=="BETWEEN"?2:kind=="ANGLE_POINTS"?3:0;
        if(!required||operands.size()!=required)throw std::invalid_argument("MEASUREMENT_INPUT_INVALID");
        size_t count=0;std::vector<std::vector<Support>> ss;
        for(const auto& operand:operands){
            if(operand.empty()||operand.size()>128||count+operand.size()>128)throw std::invalid_argument("MEASUREMENT_SCOPE_LIMIT");
            ss.emplace_back();for(const auto& e:operand){
                if(cancelled&&cancelled())throw std::runtime_error("MEASUREMENT_CANCELLED");
                ss.back().push_back(prepare(analysis_shape(e.geometry.geometry_id),e));++count;
            }
        }
        if(kind=="CURVATURE"){std::vector<TopoDS_Shape> shapes;for(const auto& support:ss[0])shapes.push_back(support.shape);return detail::analyze_curvature(shapes,sample_count,cancelled);}
        if(kind=="ITEM")item(ss[0],r,cancelled);
        else if(kind=="BETWEEN"){
            gp_Pnt a,b;
            if(ss[0].size()==1&&ss[1].size()==1&&(ss[0][0].type=="AXIS"||ss[1][0].type=="AXIS")){
                const auto& x=ss[0][0];const auto& y=ss[1][0];
                if(x.type=="AXIS"&&y.type=="AXIS"){
                    const gp_Vec u(*x.direction),v(*y.direction),w(*y.center,*x.center);
                    const double c=u.Dot(v),d=u.Dot(w),e=v.Dot(w),denom=1-c*c;
                    const double t=denom>1e-14?(c*e-d)/denom:0;
                    const double z=denom>1e-14?(e-c*d)/denom:e;
                    a=x.center->Translated(t*u);b=y.center->Translated(z*v);
                }else{
                    const bool first=x.type=="AXIS";const auto& axis=first?x:y;const auto& other=first?y:x;
                    if(other.type!="POINT")throw std::invalid_argument("MEASUREMENT_AXIS_COMBINATION_UNSUPPORTED");
                    const auto point=BRep_Tool::Pnt(TopoDS::Vertex(other.shape));
                    const auto projection=axis.center->Translated(gp_Vec(*axis.direction)*gp_Vec(*axis.center,point).Dot(gp_Vec(*axis.direction)));
                    a=first?projection:point;b=first?point:projection;
                }
                r.values["distanceMm"]=a.Distance(b);
            }else{
                Handle(MeasureProgress) progress=new MeasureProgress(cancelled);
                BRepExtrema_DistShapeShape distance;distance.LoadS1(compound(ss[0]));distance.LoadS2(compound(ss[1]));distance.SetDeflection(eps);distance.SetMultiThread(false);
                distance.Perform(progress->Start());
                if(!distance.IsDone()||distance.NbSolution()<1)throw std::runtime_error("MEASUREMENT_DISTANCE_UNFINISHED");
                r.values["distanceMm"]=distance.Value();a=distance.PointOnShape1(1);b=distance.PointOnShape2(1);
            }
            add_point(r,"first",a);add_point(r,"second",b);
            r.values["deltaXMm"]=b.X()-a.X();r.values["deltaYMm"]=b.Y()-a.Y();r.values["deltaZMm"]=b.Z()-a.Z();
            if(ss[0].size()==1&&ss[1].size()==1&&ss[0][0].direction&&ss[1][0].direction){
                const auto& x=ss[0][0];const auto& y=ss[1][0];
                // Circle normals do not imply an edge tangent. Only lines,
                // explicit axes and planes have an unambiguous angle here.
                auto eligible=[](const Support& s){return s.plane||s.type=="LINE"||s.type=="AXIS";};
                if(eligible(x)&&eligible(y)){
                    const double dot=std::clamp(std::abs(x.direction->Dot(*y.direction)),0.0,1.0);
                    const double angle=x.plane!=y.plane?std::asin(dot):std::acos(dot);r.values["angleRad"]=angle;
                    r.anchors["firstDirection"]=vec(*x.direction);r.anchors["secondDirection"]=vec(*y.direction);
                }
            }
        }else{
            for(const auto& op:ss)if(op.size()!=1||op.front().type!="POINT")throw std::invalid_argument("MEASUREMENT_REQUIRES_THREE_POINTS");
            const auto a=BRep_Tool::Pnt(TopoDS::Vertex(ss[0][0].shape)),o=BRep_Tool::Pnt(TopoDS::Vertex(ss[1][0].shape)),b=BRep_Tool::Pnt(TopoDS::Vertex(ss[2][0].shape));
            const gp_Vec u(o,a),v(o,b);if(u.Magnitude()<=eps||v.Magnitude()<=eps)throw std::invalid_argument("MEASUREMENT_DEGENERATE_ANGLE");
            r.values["angleRad"]=u.Angle(v);add_point(r,"first",a);add_point(r,"vertex",o);add_point(r,"second",b);
        }
        if(cancelled&&cancelled())throw std::runtime_error("MEASUREMENT_CANCELLED");
        for(const auto& [key,v]:r.values)if(!std::isfinite(v))throw std::runtime_error("MEASUREMENT_NONFINITE_RESULT");
        for(const auto& [key,p]:r.anchors)if(!std::isfinite(p.x)||!std::isfinite(p.y)||!std::isfinite(p.z))throw std::runtime_error("MEASUREMENT_NONFINITE_RESULT");
        r.complete=true;
    }catch(const Standard_Failure&){r=MeasurementResult{};r.diagnostic="MEASUREMENT_KERNEL_FAILURE";}
    catch(const std::exception& e){r=MeasurementResult{};r.diagnostic=e.what();}
    return r;
}
}

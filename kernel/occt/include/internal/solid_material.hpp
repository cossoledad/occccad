#pragma once

#include <BRepBndLib.hxx>
#include <BRepAdaptor_Surface.hxx>
#include <BRepClass_FaceClassifier.hxx>
#include <BRepTools.hxx>
#include <BRepClass3d_SolidClassifier.hxx>
#include <BRepClass3d_SolidExplorer.hxx>
#include <Bnd_Box.hxx>
#include <Precision.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Face.hxx>
#include <TopoDS_Solid.hxx>
#include <algorithm>
#include <cmath>
#include <functional>
#include <stdexcept>

namespace occccad::kernel::detail {

// Interior UV samples are exact points on the trimmed face, not a mesh or
// centroid estimate. Try the face interior before the boundary-biased fallback.
template<class Visit> bool visit_face_interior(const TopoDS_Face& face, Visit visit) {
    BRepAdaptor_Surface surface(face);
    Standard_Real u0,u1,v0,v1;BRepTools::UVBounds(face,u0,u1,v0,v1);
    const auto point_at=[&](double u,double v) {
        if(BRepClass_FaceClassifier(face,gp_Pnt2d(u,v),Precision::Confusion()).State()!=TopAbs_IN)return false;
        gp_Pnt point;gp_Vec du,dv;surface.D1(u,v,point,du,dv);
        auto normal=du.Crossed(dv);if(normal.SquareMagnitude()<=1e-24)return false;
        normal.Normalize();return visit(point,normal);
    };
    if(std::isfinite(u0)&&std::isfinite(u1)&&std::isfinite(v0)&&std::isfinite(v1))
        for(double u:{.5,.25,.75})for(double v:{.5,.25,.75})
            if(point_at(u0+(u1-u0)*u,v0+(v1-v0)*v))return true;
    gp_Pnt point;Standard_Real u,v;
    if(BRepClass3d_SolidExplorer::FindAPointInTheFace(face,point,u,v))return point_at(u,v);
    return false;
}

// A material predicate, not a mass-property query. A strictly interior witness
// establishes a three-dimensional region; face/edge contact does not. An
// unresolved classifier is an error, never an assertion that material is absent.
template<class Accept> bool visit_solid_material(const TopoDS_Shape& shape, Accept accept,
                               const std::function<bool()>& cancelled = {},
                               double tolerance = Precision::Confusion()) {
    if (shape.IsNull()) return false;
    const auto check = [&] {
        if (cancelled && cancelled()) throw std::runtime_error("EVALUATION_CANCELLED");
    };
    for (TopExp_Explorer solids(shape, TopAbs_SOLID); solids.More(); solids.Next()) {
        check();
        const auto solid = TopoDS::Solid(solids.Current());
        if (!TopExp_Explorer(solid, TopAbs_FACE).More()) continue;
        BRepClass3d_SolidClassifier classifier(solid);
        classifier.PerformInfinitePoint(tolerance);
        if (classifier.State() != TopAbs_OUT)
            throw std::runtime_error("INVALID_RESULT: unbounded or incorrectly oriented solid");
        Bnd_Box box; BRepBndLib::Add(solid, box, Standard_False);
        if (box.IsVoid() || box.IsOpen())
            throw std::runtime_error("INVALID_RESULT: unbounded solid");
        Standard_Real x0,y0,z0,x1,y1,z1;box.Get(x0,y0,z0,x1,y1,z1);
        const double scale = std::hypot(x1-x0,y1-y0,z1-z0);
        const double minimum = tolerance*4;
        bool witnessed=false;
        int inspected = 0;
        for (TopExp_Explorer faces(solid,TopAbs_FACE);faces.More() && inspected<32;faces.Next(),++inspected) {
            check();
            if(visit_face_interior(TopoDS::Face(faces.Current()),[&](const gp_Pnt& point,const gp_Vec& normal) {
                double step=std::max(minimum,scale*1e-4);
                for (;;) {
                    for(const double sign:{-1.0,1.0}) {
                        check();classifier.Perform(point.Translated(normal*(sign*step)),tolerance);
                        if(classifier.State()==TopAbs_IN){witnessed=true;if(accept(point.Translated(normal*(sign*step))))return true;}
                    }
                    if(step<=minimum)break;
                    step=std::max(minimum,step*.1);
                }
                return false;
            }))return true;
        }
        if(!witnessed)throw std::runtime_error("MATERIAL_CLASSIFICATION_UNRESOLVED");
    }
    return false;
}

inline bool has_solid_material(const TopoDS_Shape& shape,
                               const std::function<bool()>& cancelled = {},
                               double tolerance = Precision::Confusion()) {
    return visit_solid_material(shape,[](const auto&){return true;},cancelled,tolerance);
}

} // namespace occccad::kernel::detail

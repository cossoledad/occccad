#include <BRep_Tool.hxx>
#include <GeomLib.hxx>
#include <Precision.hxx>
#include <Standard_Failure.hxx>
#include <TopLoc_Location.hxx>

#include <internal/surface_normals.hpp>

#include <cmath>

namespace occccad::kernel::detail {
void append_surface_normals(const TopoDS_Face& face,
                            const Handle(Poly_Triangulation) & triangulation,
                            std::vector<Vec3>& normals, VisualStageTimings& timings,
                            const std::function<void()>& checkpoint) {
    TopLoc_Location location;
    const auto surface = BRep_Tool::Surface(face, location);
    const auto offset=normals.size();
    std::vector<bool> estimated;
    for (int i = 1; i <= triangulation->NbNodes(); ++i) {
        if (checkpoint && (i % 256 == 1))
            checkpoint();
        Vec3 normal;
        if (!surface.IsNull() && triangulation->HasUVNodes()) {
            ++timings.surface_normal_evaluations;
            try {
                gp_Dir n;
                // OCCT's D2 estimate handles singular poles; it does not average
                // across CAD faces or use the incident triangles for this result.
                const auto status=GeomLib::NormEstim(surface, triangulation->UVNode(i), Precision::Confusion(), n);
                if (status <= 1) {
                    if(status==1){if(estimated.empty())estimated.resize(triangulation->NbNodes());estimated[i-1]=true;}
                    n.Transform(location.Transformation());
                    if (face.Orientation() == TopAbs_REVERSED)
                        n.Reverse();
                    if (std::isfinite(n.X() + n.Y() + n.Z()))
                        normal = {n.X(), n.Y(), n.Z()};
                }
            } catch (const Standard_Failure&) {
            }
        }
        normals.push_back(normal);
    }
    if(!estimated.empty()){
        // NormEstim's D2 limit can have ambiguous sign at tiny singular poles.
        // Resolve only that sign against oriented incident mesh triangles; the
        // direction still comes from the surface, not a triangle average.
        std::vector<gp_Vec> reference(triangulation->NbNodes());
        TopLoc_Location mesh_location;BRep_Tool::Triangulation(face,mesh_location);
        for(int i=1;i<=triangulation->NbTriangles();++i){
            if(checkpoint&&i%256==1)checkpoint();
            int a,b,c;triangulation->Triangle(i).Get(a,b,c);
            if(!estimated[a-1]&&!estimated[b-1]&&!estimated[c-1])continue;
            const auto cross=gp_Vec(triangulation->Node(a),triangulation->Node(b)).Crossed(gp_Vec(triangulation->Node(a),triangulation->Node(c)));
            for(const auto node:{a,b,c})if(estimated[node-1])reference[node-1]+=cross;
        }
        for(size_t i=0;i<estimated.size();++i)if(estimated[i]){
            auto& n=normals[offset+i];auto r=reference[i];r.Transform(mesh_location.Transformation());
            if(face.Orientation()==TopAbs_REVERSED)r.Reverse();
            if(r.SquareMagnitude()>0){if(n.x*r.X()+n.y*r.Y()+n.z*r.Z()<0){n.x=-n.x;n.y=-n.y;n.z=-n.z;}}
            else n={};
        }
    }
    for(size_t i=offset;i<normals.size();++i){const auto& n=normals[i];
        if(n.x*n.x+n.y*n.y+n.z*n.z<.5)++timings.surface_normal_fallbacks;
    }
}
}  // namespace occccad::kernel::detail

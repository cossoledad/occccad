#pragma once
#include <BRepAdaptor_Surface.hxx>
#include <Geom2d_Curve.hxx>
#include <Poly_PolygonOnTriangulation.hxx>
#include <Poly_Triangulation.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Face.hxx>
#include <gp_Pnt2d.hxx>

#include <occccad/kernel/kernel.hpp>

#include <memory>

namespace occccad::kernel::detail {
struct VisualEdgeUse {
    uint32_t face_id = 0;
    TopoDS_Face face;
    TopoDS_Edge edge;
    std::shared_ptr<BRepAdaptor_Surface> surface;
    Handle(Geom2d_Curve) pcurve;
    std::vector<Vec3> points;
    std::vector<gp_Pnt2d> uv;
    std::vector<double> parameters;
    Vec3 plane_normal;
    bool reversed = false;
};
VisualEdgeUse visual_edge_use(uint32_t face_id, const TopoDS_Face& face, const TopoDS_Edge& edge,
                              const Handle(Poly_Triangulation) & triangulation,
                              const Handle(Poly_PolygonOnTriangulation) & polygon,
                              const std::vector<Vec3>& vertices, uint32_t base);
void append_edge_profile(TessellationResult& mesh, uint32_t edge_id, const TopoDS_Edge& edge,
                         const std::vector<Vec3>& canonical, const std::vector<VisualEdgeUse>& uses,
                         const std::function<void()>& checkpoint = {});
}  // namespace occccad::kernel::detail

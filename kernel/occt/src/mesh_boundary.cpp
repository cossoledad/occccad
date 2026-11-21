#include <BRepBuilderAPI_Copy.hxx>
#include <BRepMesh_Context.hxx>
#include <BRepMesh_NURBSRangeSplitter.hxx>
#include <BRepMesh_DelaunayBaseMeshAlgo.hxx>
#include <BRepMesh_DelaunayDeflectionControlMeshAlgo.hxx>
#include <BRepMesh_DelabellaMeshAlgoFactory.hxx>
#include <BRepMesh_FaceDiscret.hxx>
#include <BRepMesh_MeshAlgoFactory.hxx>
#include <BRepMesh_ModelBuilder.hxx>
#include <BRepMesh_UndefinedRangeSplitter.hxx>
#include <IMeshData_Face.hxx>
#include <Precision.hxx>

#include <internal/mesh_boundary.hpp>

namespace occccad::kernel {
namespace {

template <class Splitter>
class BoundaryPreservingMesh final : public BRepMesh_DelaunayDeflectionControlMeshAlgo<Splitter, BRepMesh_DelaunayBaseMeshAlgo> {
    using Base = BRepMesh_DelaunayDeflectionControlMeshAlgo<Splitter, BRepMesh_DelaunayBaseMeshAlgo>;
protected:
    Standard_Integer registerNode(const gp_Pnt& point, const gp_Pnt2d& uv,
                                  BRepMesh_DegreeOfFreedom mobility,
                                  Standard_Boolean force) override {
        const auto index = Base::registerNode(point, uv, mobility, force);
        const auto& node = this->getStructure()->GetNode(index);
        const auto& existing = this->getNodesMap()->Value(node.Location3d());
        // A shared source boundary already supplies the same 3D sample to every
        // face. UV proximity must not replace it with a different source sample.
        if (mobility == BRepMesh_Frontier && !force &&
            existing.Distance(point) > Precision::Confusion())
            return Base::registerNode(point, uv, mobility, Standard_True);
        return index;
    }
};

class DisplayMeshFactory final : public BRepMesh_MeshAlgoFactory {
public:
    explicit DisplayMeshFactory(bool preserve_boundary) : preserve_boundary_(preserve_boundary) {}
    Handle(IMeshTools_MeshAlgo)
        GetAlgo(GeomAbs_SurfaceType type, const IMeshTools_Parameters& parameters) const override {
        // A planar triangle lies exactly on its support. Keep the same edge
        // samples/internal vertices, while avoiding costly incremental point
        // insertion on heavily trimmed planes. Do not switch curved surfaces:
        // Delabella's generic factory has different refinement policies there.
        if (type == GeomAbs_Plane)
            return BRepMesh_DelabellaMeshAlgoFactory().GetAlgo(type, parameters);
        if (preserve_boundary_ && (type == GeomAbs_OffsetSurface || type == GeomAbs_OtherSurface))
            return new BoundaryPreservingMesh<BRepMesh_UndefinedRangeSplitter>();
        if (preserve_boundary_ && (type == GeomAbs_BSplineSurface || type == GeomAbs_BezierSurface))
            return new BoundaryPreservingMesh<BRepMesh_NURBSRangeSplitter>();
        return BRepMesh_MeshAlgoFactory::GetAlgo(type, parameters);
    }
private:
    bool preserve_boundary_;
};

}  // namespace

Handle(IMeshTools_Context) display_mesh_context(bool preserve_boundary) {
    Handle(BRepMesh_Context) context = new BRepMesh_Context();
    context->SetFaceDiscret(new BRepMesh_FaceDiscret(new DisplayMeshFactory(preserve_boundary)));
    return context;
}

bool has_ordered_surface_boundaries(const TopoDS_Shape& shape) {
    // Use the consumer's wire analysis before accepting an offset candidate.
    // A valid tolerant solid can still contain an ambiguous tiny edge loop.
    BRepBuilderAPI_Copy copy(shape, Standard_True, Standard_False);
    BRepMesh_ModelBuilder builder;
    IMeshTools_Parameters parameters;
    const auto model = builder.Perform(copy.Shape(), parameters);
    if (model.IsNull() || model->FacesNb() == 0)
        return false;
    for (int i = 0; i < model->FacesNb(); ++i) {
        const auto& face = model->GetFace(i);
        if (face->IsSet(IMeshData_UnorientedWire) || face->IsSet(IMeshData_Failure))
            return false;
    }
    return true;
}

}  // namespace occccad::kernel

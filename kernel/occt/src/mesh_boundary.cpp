#include <BRepBuilderAPI_Copy.hxx>
#include <BRepMesh_Context.hxx>
#include <BRepMesh_DelaunayBaseMeshAlgo.hxx>
#include <BRepMesh_DelaunayDeflectionControlMeshAlgo.hxx>
#include <BRepMesh_FaceDiscret.hxx>
#include <BRepMesh_MeshAlgoFactory.hxx>
#include <BRepMesh_ModelBuilder.hxx>
#include <BRepMesh_UndefinedRangeSplitter.hxx>
#include <IMeshData_Face.hxx>
#include <Precision.hxx>

#include <internal/mesh_boundary.hpp>

namespace occccad::kernel {
namespace {

using GenericMesh = BRepMesh_DelaunayDeflectionControlMeshAlgo<BRepMesh_UndefinedRangeSplitter,
                                                               BRepMesh_DelaunayBaseMeshAlgo>;

class BoundaryPreservingMesh final : public GenericMesh {
protected:
    Standard_Integer registerNode(const gp_Pnt& point, const gp_Pnt2d& uv,
                                  BRepMesh_DegreeOfFreedom mobility,
                                  Standard_Boolean force) override {
        const auto index = GenericMesh::registerNode(point, uv, mobility, force);
        const auto& node = getStructure()->GetNode(index);
        const auto& existing = getNodesMap()->Value(node.Location3d());
        // A shared source boundary already supplies the same 3D sample to every
        // face. UV proximity must not replace it with a different source sample.
        if (mobility == BRepMesh_Frontier && !force &&
            existing.Distance(point) > Precision::Confusion())
            return GenericMesh::registerNode(point, uv, mobility, Standard_True);
        return index;
    }
};

class BoundaryMeshFactory final : public BRepMesh_MeshAlgoFactory {
public:
    Handle(IMeshTools_MeshAlgo)
        GetAlgo(GeomAbs_SurfaceType type, const IMeshTools_Parameters& parameters) const override {
        if (type == GeomAbs_OffsetSurface || type == GeomAbs_OtherSurface)
            return new BoundaryPreservingMesh();
        return BRepMesh_MeshAlgoFactory::GetAlgo(type, parameters);
    }
};

}  // namespace

Handle(IMeshTools_Context) boundary_preserving_mesh_context() {
    Handle(BRepMesh_Context) context = new BRepMesh_Context();
    context->SetFaceDiscret(new BRepMesh_FaceDiscret(new BoundaryMeshFactory()));
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

#include <internal/occt_kernel.hpp>
#include <internal/mass_properties.hpp>
#include <BRepAlgoAPI_Cut.hxx>
#include <BRepBuilderAPI_NurbsConvert.hxx>
#include <BRepBuilderAPI_Transform.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <ShapeUpgrade_ShapeDivideContinuity.hxx>
#include <gtest/gtest.h>
#include <chrono>
#include <cstdlib>
#include <sstream>

namespace occccad::kernel {
namespace {
std::string brep(const TopoDS_Shape& shape) {
    std::ostringstream output;
    BRepTools::Write(shape,output);
    return output.str();
}
}
TEST(GeometryExchange, ExactVolumeSpansPreserveRationalGeometryAndCentroid) {
    const auto pi=std::acos(-1.0);
    auto cylinder=BRepBuilderAPI_NurbsConvert(BRepPrimAPI_MakeCylinder(3,11).Shape(),true).Shape();
    gp_Trsf placement;
    placement.SetTranslation(gp_Vec(123456,-234567,345678));
    const auto shape=BRepBuilderAPI_Transform(cylinder,placement,true).Shape();
    const auto before=brep(shape);
    GProp_GProps properties;
    detail::volume_properties(shape,properties,true);
    EXPECT_NEAR(properties.Mass(),pi*9*11,1e-7);
    EXPECT_NEAR(properties.CentreOfMass().X(),123456,1e-7);
    EXPECT_NEAR(properties.CentreOfMass().Y(),-234567,1e-7);
    EXPECT_NEAR(properties.CentreOfMass().Z(),345678+5.5,1e-7);
    EXPECT_EQ(before,brep(shape));
}
TEST(GeometryExchange, ExactVolumeSpansPreserveVoidsAndScale) {
    const auto pi=std::acos(-1.0);
    const auto box=BRepPrimAPI_MakeBox(20,20,10).Shape();
    const auto bore=BRepBuilderAPI_NurbsConvert(BRepPrimAPI_MakeCylinder(gp_Ax2(gp_Pnt(10,10,-1),gp_Dir(0,0,1)),3,12).Shape(),true).Shape();
    const auto shape=BRepAlgoAPI_Cut(box,bore).Shape();
    for (const auto scale:{0.001,1.,1000.}) {
        gp_Trsf transform;
        transform.SetScale(gp_Pnt(0,0,0),scale);
        GProp_GProps properties;
        detail::volume_properties(BRepBuilderAPI_Transform(shape,transform,true).Shape(),properties,true);
        const auto expected=(4000-pi*90)*scale*scale*scale;
        EXPECT_NEAR(properties.Mass(),expected,std::abs(expected)*1e-9);
        const auto centroidTolerance=std::max(scale*1e-8,1e-9);
        EXPECT_NEAR(properties.CentreOfMass().X(),10*scale,centroidTolerance);
        EXPECT_NEAR(properties.CentreOfMass().Y(),10*scale,centroidTolerance);
        EXPECT_NEAR(properties.CentreOfMass().Z(),5*scale,centroidTolerance);
    }
}
TEST(GeometryExchange, ExactVolumeSpansHandleTrimmedRationalGeometry) {
    const auto verify=[](const TopoDS_Shape& shape, double analytic=0) {
        const auto before=brep(shape);
        GProp_GProps actual,reference;
        const auto start=std::chrono::steady_clock::now();
        detail::volume_properties(shape,actual);
        const auto elapsed=std::chrono::duration<double,std::milli>(std::chrono::steady_clock::now()-start).count();
        ShapeUpgrade_ShapeDivideContinuity split(shape);
        split.SetSurfaceCriterion(GeomAbs_CN);
        split.Perform();
        BRepGProp::VolumeProperties(split.Result(),reference,1e-11);
        if (analytic!=0) EXPECT_NEAR(actual.Mass(),analytic,std::abs(analytic)*1e-10);
        else EXPECT_NEAR(actual.Mass(),reference.Mass(),std::abs(reference.Mass())*1e-9);
        EXPECT_EQ(before,brep(shape));
        RecordProperty("volume_mm3",actual.Mass());
        RecordProperty("exact_volume_ms",elapsed);
    };
    const auto path=std::getenv("OCCCCAD_TEST_EXCHANGE_STEP");
    if (!path || !*path) {
        const auto cylinder=BRepBuilderAPI_NurbsConvert(BRepPrimAPI_MakeCylinder(10,10).Shape(),true).Shape();
        const auto cutter=BRepPrimAPI_MakeBox(gp_Pnt(0,-20,-1),20,40,12).Shape();
        verify(BRepAlgoAPI_Cut(cylinder,cutter).Shape(),500*std::acos(-1.0));
        return;
    }
    OcctKernel kernel(true);
    const auto graph=kernel.readStepGraph(path);
    for(const auto& def:graph.definitions) {
        if(def.geometry_id.empty()||kernel.getTopologySummary(def.geometry_id).geometry_domain!="SOLID_SET")continue;
        const auto bytes=kernel.serializeBrepr(def.geometry_id);
        std::istringstream stream(std::string(bytes.begin(),bytes.end()));
        TopoDS_Shape shape;
        BRep_Builder builder;
        BRepTools::Read(shape,stream,builder);
        verify(shape);
        EXPECT_EQ(bytes,kernel.serializeBrepr(def.geometry_id));
    }
}
}

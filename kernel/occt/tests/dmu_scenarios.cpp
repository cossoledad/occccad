#include <internal/occt_kernel.hpp>
#include <gtest/gtest.h>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepTools.hxx>
#include <gp_Pln.hxx>
#include <sstream>
namespace occccad::kernel {
namespace {
GeometryId dmu_shape(OcctKernel& k,const TopoDS_Shape& shape){std::ostringstream o;BRepTools::Write(shape,o);auto s=o.str();return k.loadBrepr({s.begin(),s.end()});}
PlacedGeometry at(GeometryId id,double x=0,double y=0,double z=0){PlacedGeometry p;p.geometry_id=id;p.rotation={0,0,0,1};p.translation={x,y,z};return p;}
TEST(DMU,ExactSolidClassificationGapAndSourceImmutability){
 OcctKernel k;auto a=dmu_shape(k,BRepPrimAPI_MakeBox(10,10,10).Shape());auto tiny=dmu_shape(k,BRepPrimAPI_MakeBox(2,2,2).Shape());auto before=k.serializeBrepr(a);
 auto r=k.analyze_interference(at(a),at(a,15),0,1e-7);ASSERT_TRUE(r.complete);EXPECT_EQ(r.classification,"SEPARATED");EXPECT_NEAR(r.distance,5,1e-8);EXPECT_TRUE(r.clearance_satisfied);EXPECT_FALSE(r.common_tested);
 r=k.analyze_interference(at(a),at(a,15),6,1e-7);EXPECT_TRUE(r.complete);EXPECT_FALSE(r.clearance_satisfied);
 r=k.analyze_interference(at(a),at(a,10),0,1e-7);EXPECT_TRUE(r.complete);EXPECT_EQ(r.classification,"CONTACT");EXPECT_TRUE(r.clearance_satisfied);
 r=k.analyze_interference(at(a),at(a,5),0,1e-7);EXPECT_TRUE(r.complete);EXPECT_EQ(r.classification,"PENETRATION");EXPECT_NEAR(r.common_volume,500,1e-6);EXPECT_FALSE(r.clearance_satisfied);
 r=k.analyze_interference(at(a),at(tiny,2,2,2),0,1e-7);EXPECT_TRUE(r.complete);EXPECT_EQ(r.classification,"CONTAINMENT");EXPECT_NEAR(r.common_volume,8,1e-6);
 r=k.analyze_interference(at(a),at(a),0,1e-7);EXPECT_EQ(r.classification,"CONTAINMENT");EXPECT_TRUE(r.complete);
 EXPECT_EQ(before,k.serializeBrepr(a));
}
TEST(DMU,InvalidAndCancelledAreInconclusive){OcctKernel k;auto a=dmu_shape(k,BRepPrimAPI_MakeBox(10,10,10).Shape());auto face=dmu_shape(k,BRepBuilderAPI_MakeFace(gp_Pln(),0,10,0,10).Face());
 auto r=k.analyze_interference(at(a),at(face),0,1e-7);EXPECT_FALSE(r.complete);EXPECT_EQ(r.classification,"INCONCLUSIVE");EXPECT_FALSE(r.clearance_satisfied);
 r=k.analyze_interference(at(a),at(a),0,1e-7,[]{return true;});EXPECT_FALSE(r.complete);EXPECT_EQ(r.diagnostic,"CANCELLED");
}
}
}

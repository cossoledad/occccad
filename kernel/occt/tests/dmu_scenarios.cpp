#include <internal/occt_kernel.hpp>
#include <gtest/gtest.h>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepAlgoAPI_Cut.hxx>
#include <cmath>
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
namespace occccad::kernel {
TEST(DMU,ThresholdProofDetailedOverlapAndContainmentWitnesses){
 OcctKernel k;auto box=dmu_shape(k,BRepPrimAPI_MakeBox(10,10,10).Shape());
 auto r=k.analyze_interference(at(box),at(box,15),2,1e-7,{},true);
 ASSERT_TRUE(r.complete);EXPECT_TRUE(r.clearance_satisfied);EXPECT_FALSE(r.distance_exact);EXPECT_EQ(r.witness_kind,"NONE");
 r=k.analyze_interference(at(box),at(box,15),6,1e-7,{},true);
 EXPECT_TRUE(r.distance_exact);EXPECT_NEAR(r.distance,5,1e-9);EXPECT_FALSE(r.clearance_satisfied);EXPECT_EQ(r.witness_kind,"DISTANCE");
 // A new threshold reuses geometric evidence, never the former pass/fail flag.
 r=k.analyze_interference(at(box),at(box,15),4,1e-7);EXPECT_TRUE(r.clearance_satisfied);EXPECT_TRUE(r.distance_exact);
 r=k.analyze_interference(at(box),at(box,8),0,1e-7,{},false,true);
 ASSERT_TRUE(r.complete);EXPECT_EQ(r.classification,"PENETRATION");EXPECT_EQ(r.witness_kind,"NONE");
 ASSERT_EQ(r.detail_status,"COMPLETE");EXPECT_FALSE(r.section_points.empty());EXPECT_FALSE(r.overlap_triangles.empty());
 EXPECT_EQ(r.section_points.size()%2,0);EXPECT_EQ(r.overlap_triangles.size()%3,0);EXPECT_NEAR(r.common_volume,200,1e-7);
 auto tiny=dmu_shape(k,BRepPrimAPI_MakeBox(2,2,2).Shape());
 r=k.analyze_interference(at(box),at(tiny,2,2,2),0,1e-7,{},false,true);
 ASSERT_TRUE(r.complete);EXPECT_EQ(r.containment,"SECOND_INSIDE_FIRST");EXPECT_EQ(r.witness_kind,"NONE");EXPECT_FALSE(r.overlap_triangles.empty());
 r=k.analyze_interference(at(box),at(box),0,1e-7);EXPECT_EQ(r.containment,"COINCIDENT");
 // A genuine tiny solid overlap may not be removed by relative volume filtering.
 r=k.analyze_interference(at(box),at(box,9.999999),0,1e-7);ASSERT_TRUE(r.complete);EXPECT_EQ(r.classification,"PENETRATION");EXPECT_GT(r.common_volume,0);
}
}
namespace occccad::kernel {
TEST(DMU,SpatialSectionUsesExactMaterialAndFrame){
 OcctKernel k;auto box=dmu_shape(k,BRepPrimAPI_MakeBox(10,10,10).Shape());auto before=k.serializeBrepr(box);
 SpatialGeometryOptions o;o.kind="SECTION";o.section_type="PLANE";o.frame=at(box,0,0,5);o.accuracy=1;
 auto r=k.analyze_spatial(at(box),{},o);ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.area,100,1e-7);EXPECT_NEAR(r.centroid.z,5,1e-7);EXPECT_FALSE(r.segments.empty());EXPECT_FALSE(r.triangles.empty());
 auto drill=BRepPrimAPI_MakeCylinder(gp_Ax2(gp_Pnt(5,5,0),gp_Dir(0,0,1)),2,10).Shape();auto hollow=dmu_shape(k,BRepAlgoAPI_Cut(BRepPrimAPI_MakeBox(10,10,10).Shape(),drill).Shape());
 auto holed=k.analyze_spatial(at(hollow),{},o);ASSERT_TRUE(holed.complete)<<holed.diagnostic;EXPECT_NEAR(holed.area,100-4*std::acos(-1),1e-6);EXPECT_FALSE(holed.triangles.empty());
 o.frame=at(box,5,5,5);o.frame.rotation={0,std::sin(std::acos(-1)/8),0,std::cos(std::acos(-1)/8)};auto tilted=k.analyze_spatial(at(box),{},o);ASSERT_TRUE(tilted.complete)<<tilted.diagnostic;EXPECT_NEAR(tilted.area,100*std::sqrt(2),1e-6);o.frame=at(box,0,0,5);
 o.section_type="SLICE";o.size={10,10,2};r=k.analyze_spatial(at(box),{},o);ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.area,200,1e-7);
 o.section_type="BOX";o.size={4,4,4};o.frame=at(box,5,5,5);r=k.analyze_spatial(at(box),{},o);ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.area,96,1e-6);EXPECT_EQ(before,k.serializeBrepr(box));
 r=k.analyze_spatial(at(box),{},o,[]{return true;});EXPECT_FALSE(r.complete);EXPECT_TRUE(r.triangles.empty());
}
TEST(DMU,SpatialDistanceBandHasConservativeBoundaryAndExactSamples){
 OcctKernel k;auto box=dmu_shape(k,BRepPrimAPI_MakeBox(10,10,10).Shape());SpatialGeometryOptions o;o.kind="BAND";o.minimum=2;o.maximum=7;o.accuracy=2;
 auto r=k.analyze_spatial(at(box),{at(box,15)},o);ASSERT_TRUE(r.complete)<<r.diagnostic;ASSERT_FALSE(r.triangles.empty());EXPECT_EQ(r.regions.size()*3,r.triangles.size());EXPECT_TRUE(std::find(r.regions.begin(),r.regions.end(),1)!=r.regions.end());
 o.maximum=3;r=k.analyze_spatial(at(box),{at(box,15)},o);ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_TRUE(r.triangles.empty());
}
}

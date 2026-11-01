#include <internal/occt_kernel.hpp>
#include <gtest/gtest.h>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepBuilderAPI_MakeEdge.hxx>
#include <BRepTools.hxx>
#include <gp_Elips.hxx>
#include <sstream>
#include <cmath>
using namespace occccad::kernel;
namespace {
MeasurementElement body(const std::string& id,double x=0){return {{id,{x,0,0},{0,0,0,1}},"BODY",0,"GEOMETRY"};}
}
TEST(Measurement, ExactItemAndOccurrenceDistance){
    OcctKernel k;const auto id=k.createBox(10,20,30);
    auto r=k.measure_geometry("ITEM",{{body(id)}});ASSERT_TRUE(r.complete)<<r.diagnostic;
    EXPECT_NEAR(r.values.at("volumeMm3"),6000,1e-8);EXPECT_NEAR(r.values.at("areaMm2"),2200,1e-8);
    EXPECT_NEAR(r.anchors.at("centroid").x,5,1e-8);
    r=k.measure_geometry("BETWEEN",{{body(id)},{body(id,50)}});ASSERT_TRUE(r.complete)<<r.diagnostic;
    EXPECT_NEAR(r.values.at("distanceMm"),40,1e-7);EXPECT_NEAR(r.values.at("deltaXMm"),40,1e-7);
    EXPECT_NEAR(r.anchors.at("first").x,10,1e-7);EXPECT_NEAR(r.anchors.at("second").x,50,1e-7);
    r=k.measure_geometry("BETWEEN",{{body(id)},{body(id,5)}});ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.values.at("distanceMm"),0,1e-7);
}
TEST(Measurement, ExactTopologyAndThreePoints){
    OcctKernel k;const auto id=k.createBox(10,20,30);const auto& topology=k.getTopology(id);
    auto e=body(id);e.kind="EDGE";e.local_id=1;
    auto r=k.measure_geometry("ITEM",{{e}});ASSERT_TRUE(r.complete)<<r.diagnostic;
    EXPECT_GT(r.values.at("lengthMm"),0);EXPECT_EQ(r.geometry_type,"LINE");
    std::vector<MeasurementElement> points;std::vector<Vec3> positions;
    for(const auto& v:topology.vertices)if((v.point.x==0&&v.point.y==0&&v.point.z==0)||(v.point.x==10&&v.point.y==0&&v.point.z==0)||(v.point.x==0&&v.point.y==20&&v.point.z==0)){
        auto p=body(id);p.kind="VERTEX";p.local_id=v.local_id;points.push_back(p);positions.push_back(v.point);
    }
    ASSERT_EQ(points.size(),3u);size_t origin=0;for(size_t i=0;i<3;++i)if(positions[i].x==0&&positions[i].y==0)origin=i;
    r=k.measure_geometry("ANGLE_POINTS",{{points[(origin+1)%3]},{points[origin]},{points[(origin+2)%3]}});ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.values.at("angleRad"),std::acos(-1)/2,1e-8);
    r=k.measure_geometry("ANGLE_POINTS",{{points[0]},{points[0]},{points[1]}});EXPECT_FALSE(r.complete);EXPECT_EQ(r.diagnostic,"MEASUREMENT_DEGENERATE_ANGLE");
    e.local_id=99999;r=k.measure_geometry("ITEM",{{e}});EXPECT_FALSE(r.complete);EXPECT_TRUE(r.values.empty());
}
TEST(Measurement, CentersAxesAndCancellation){
    OcctKernel k;std::ostringstream stream;BRepTools::Write(BRepPrimAPI_MakeCylinder(5,20).Shape(),stream);
    const auto raw=stream.str();const auto id=k.loadBrepr(std::vector<uint8_t>(raw.begin(),raw.end()));
    const auto& topology=k.getTopology(id);uint64_t circle=0,cylinder=0;
    for(const auto& e:topology.edges)if(e.curve_type==1)circle=e.local_id;
    for(const auto& f:topology.faces)if(f.surface_type==1)cylinder=f.local_id;
    ASSERT_NE(circle,0u);ASSERT_NE(cylinder,0u);
    auto face=body(id);face.kind="FACE";face.local_id=cylinder;
    auto properties=k.measure_geometry("ITEM",{{face}});ASSERT_TRUE(properties.complete)<<properties.diagnostic;
    EXPECT_NEAR(properties.values.at("areaMm2"),200*std::acos(-1),1e-6);
    EXPECT_NEAR(properties.values.at("perimeterMm"),20*std::acos(-1),1e-6);
    EXPECT_EQ(properties.anchors.count("center"),0u);EXPECT_EQ(properties.anchors.count("axisOrigin"),1u);
    auto c=body(id);c.kind="EDGE";c.local_id=circle;c.mode="CENTER";
    auto d=c;d.geometry.translation.x=30;
    auto r=k.measure_geometry("BETWEEN",{{c},{d}});ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.values.at("distanceMm"),30,1e-7);
    c.kind="FACE";c.local_id=cylinder;c.mode="AXIS";d=c;d.geometry.translation.x=30;
    r=k.measure_geometry("BETWEEN",{{c},{d}});ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.values.at("distanceMm"),30,1e-7);EXPECT_NEAR(r.values.at("angleRad"),0,1e-7);
    d.geometry.translation.y=10;d.geometry.rotation={0,std::sin(std::acos(-1)/8),0,std::cos(std::acos(-1)/8)};
    r=k.measure_geometry("BETWEEN",{{c},{d}});ASSERT_TRUE(r.complete)<<r.diagnostic;EXPECT_NEAR(r.values.at("distanceMm"),10,1e-7);EXPECT_NEAR(r.values.at("angleRad"),std::acos(-1)/4,1e-7);
    r=k.measure_geometry("ITEM",{{body(id)}},[]{return true;});EXPECT_FALSE(r.complete);EXPECT_TRUE(r.anchors.empty());
    r=k.measure_geometry("ITEM",{std::vector<MeasurementElement>(129,body(id))});EXPECT_FALSE(r.complete);EXPECT_EQ(r.diagnostic,"MEASUREMENT_SCOPE_LIMIT");
}
TEST(Measurement, EllipseRadiiAreNotCircleRadius){
    OcctKernel k;std::ostringstream stream;
    BRepTools::Write(BRepBuilderAPI_MakeEdge(gp_Elips(gp_Ax2(gp_Pnt(0,0,0),gp_Dir(0,0,1)),8,3)).Shape(),stream);
    const auto raw=stream.str();const auto id=k.loadBrepr(std::vector<uint8_t>(raw.begin(),raw.end()));
    auto edge=body(id);edge.kind="EDGE";edge.local_id=1;
    const auto r=k.measure_geometry("ITEM",{{edge}});ASSERT_TRUE(r.complete)<<r.diagnostic;
    EXPECT_DOUBLE_EQ(r.values.at("majorRadiusMm"),8);EXPECT_DOUBLE_EQ(r.values.at("minorRadiusMm"),3);EXPECT_EQ(r.values.count("radiusMm"),0u);EXPECT_EQ(r.values.count("diameterMm"),0u);
}

#include "../src/contact.hpp"

#include <Eigen/Geometry>
#include <Eigen/SVD>
#include <gtest/gtest.h>
#include <cmath>
#include <vector>

namespace occccad::assembly::analytic_contact {
namespace {
constexpr double pi = 3.1415926535897932384626433832795;
Support support(SupportKind kind, Eigen::Vector3d origin = Eigen::Vector3d::Zero(),
                Eigen::Vector3d axis = Eigen::Vector3d::UnitZ(), double radius = 1,
                double angle = pi/6) {
    return {kind, origin, axis, radius, angle, 1, 1};
}
Eigen::Matrix3d skew(const Eigen::Vector3d& v) {
    Eigen::Matrix3d m;
    m << 0,-v.z(),v.y(), v.z(),0,-v.x(), -v.y(),v.x(),0;
    return m;
}
Eigen::Matrix<double,12,6> tangent(const Support& moving) {
    Eigen::Matrix<double,12,6> chain = Eigen::Matrix<double,12,6>::Zero();
    chain.block<3,3>(6,0).setIdentity();
    chain.block<3,3>(6,3) = -skew(moving.origin);
    chain.block<3,3>(9,3) = -skew(moving.axis);
    return chain;
}
struct Case {
    Definition definition;
    Support first, second;
    int rank;
};
std::vector<Case> cases(double scale=1) {
    const Eigen::Vector3d z = Eigen::Vector3d::UnitZ();
    const Eigen::Vector3d zero = Eigen::Vector3d::Zero();
    const Eigen::Vector3d g(.5,0,std::sqrt(3.)/2);
    return {
        {{ContactKind::Face},support(SupportKind::Plane),support(SupportKind::Plane,zero,-z),3},
        {{ContactKind::Line},support(SupportKind::Plane),support(SupportKind::Cylinder,{0,0,2*scale},Eigen::Vector3d::UnitX(),2*scale),2},
        {{ContactKind::Point},support(SupportKind::Plane),support(SupportKind::Sphere,{0,0,2*scale},z,2*scale),1},
        {{ContactKind::Line},support(SupportKind::Cylinder,zero,z,2*scale),support(SupportKind::Cylinder,{5*scale,0,0},z,3*scale),3},
        {{ContactKind::Line,ContactSide::Internal},support(SupportKind::Cylinder,zero,z,2*scale),support(SupportKind::Cylinder,{scale,0,0},z,3*scale),3},
        {{ContactKind::Face,ContactSide::Internal},support(SupportKind::Cylinder,zero,z,2*scale),support(SupportKind::Cylinder,{0,0,8*scale},-z,2*scale),4},
        {{ContactKind::Face,ContactSide::Internal},support(SupportKind::Sphere,zero,z,2*scale),support(SupportKind::Sphere,zero,z,2*scale),3},
        {{ContactKind::Ring,ContactSide::Internal},support(SupportKind::Sphere,{0,0,4*scale},z,2*scale),support(SupportKind::Cone),3},
        {{ContactKind::Ring},support(SupportKind::Sphere,zero,z,5*scale),support(SupportKind::Circle,{0,0,4*scale},z,3*scale),3},
        {{ContactKind::Ring,ContactSide::External,-1},support(SupportKind::Sphere,zero,z,5*scale),support(SupportKind::Circle,{0,0,-4*scale},z,3*scale),3},
        {{ContactKind::Ring},support(SupportKind::Sphere,zero,z,5*scale),support(SupportKind::Circle,zero,z,5*scale),3},
        {{ContactKind::Line},support(SupportKind::Cone),support(SupportKind::Cone,10*scale*g,{std::sin(5*pi/18),0,std::cos(5*pi/18)},1,pi/9),3},
        {{ContactKind::Line,ContactSide::Internal},support(SupportKind::Cone),support(SupportKind::Cone,10*scale*g,{std::sin(pi/18),0,std::cos(pi/18)},1,pi/9),3},
        {{ContactKind::Face,ContactSide::Internal},support(SupportKind::Cone),support(SupportKind::Cone),5},
        {{ContactKind::Ring},support(SupportKind::Cone),support(SupportKind::Circle,{0,0,3*std::sqrt(3.)*scale},z,3*scale),5}
    };
}

TEST(AssemblyContactAnalytic, EveryBranchHasIndependentSE3RankAndAnalyticDerivative) {
    for (double scale : {.001,1.,1000.}) {
        for (const auto& c : cases(scale)) {
            SCOPED_TRACE(static_cast<int>(c.first.kind));
            SCOPED_TRACE(static_cast<int>(c.second.kind));
            SCOPED_TRACE(scale);
            const auto e = evaluate(c.definition,c.first,c.second);
            ASSERT_TRUE(e.valid) << e.diagnostic;
            EXPECT_LT(e.residual.norm(),1e-10*std::max(1.,scale));
            EXPECT_EQ(e.generalRank,c.rank);
            const Eigen::MatrixXd jacobian = e.derivatives*tangent(c.second);
            // Dimensionless rank: the input translation scale and length-row
            // scale change together. Raw mm/rad conditioning is not rank loss.
            Eigen::MatrixXd rankJacobian=jacobian;
            rankJacobian.leftCols(3)*=scale;
            for (Eigen::Index row=0;row<rankJacobian.rows();++row)
                if (!e.angularRows[static_cast<std::size_t>(row)]) rankJacobian.row(row)/=scale;
            Eigen::JacobiSVD<Eigen::MatrixXd> svd(rankJacobian);
            svd.setThreshold(1e-9);
            EXPECT_EQ(svd.rank(),c.rank) << jacobian;
            const double h = 1e-6;
            for (int column=0; column<6; ++column) {
                auto plus=c.second, minus=c.second;
                if (column<3) { plus.origin[column]+=h; minus.origin[column]-=h; }
                else {
                    const auto axis=Eigen::Vector3d::Unit(column-3);
                    const Eigen::Matrix3d rp=Eigen::AngleAxisd(h,axis).toRotationMatrix();
                    const Eigen::Matrix3d rm=Eigen::AngleAxisd(-h,axis).toRotationMatrix();
                    plus.origin=rp*c.second.origin; plus.axis=rp*c.second.axis;
                    minus.origin=rm*c.second.origin; minus.axis=rm*c.second.axis;
                }
                const auto ep=evaluate(c.definition,c.first,plus), em=evaluate(c.definition,c.first,minus);
                ASSERT_TRUE(ep.valid) << ep.diagnostic;
                ASSERT_TRUE(em.valid) << em.diagnostic;
                const Eigen::VectorXd observed=(ep.residual-em.residual)/(2*h);
                EXPECT_LT((observed-jacobian.col(column)).norm(),
                          2e-6*std::max(1.,jacobian.col(column).norm()));
            }
        }
    }
}

TEST(AssemblyContactAnalytic, SwappingSelectionAndRigidFrameKeepsTheContactSet) {
    const Eigen::Matrix3d rotation=Eigen::AngleAxisd(.73,Eigen::Vector3d(1,2,3).normalized()).toRotationMatrix();
    const Eigen::Vector3d translation(83,-24,71);
    for (const auto& c : cases()) {
        auto a=c.first,b=c.second;
        a.origin=rotation*a.origin+translation; a.axis=rotation*a.axis;
        b.origin=rotation*b.origin+translation; b.axis=rotation*b.axis;
        const auto ab=evaluate(c.definition,a,b), ba=evaluate(c.definition,b,a);
        ASSERT_TRUE(ab.valid) << ab.diagnostic;
        ASSERT_TRUE(ba.valid) << ba.diagnostic;
        EXPECT_LT(ab.residual.norm(),1e-10);
        EXPECT_LT(ba.residual.norm(),1e-10);
        EXPECT_EQ(ab.generalRank,ba.generalRank);
    }
}

// These assertions sample the actual support equations, not evaluate()'s
// residual. They also distinguish ring support from generic zero distance.
TEST(AssemblyContactAnalytic, AnnularRelationsSatisfyIndependentSurfaceEquations) {
    for (int i=0;i<24;++i) {
        const double t=2*pi*i/24;
        const Eigen::Vector3d coneSpherePoint(std::sqrt(3.)*std::cos(t),std::sqrt(3.)*std::sin(t),3);
        EXPECT_NEAR((coneSpherePoint-Eigen::Vector3d(0,0,4)).norm(),2,1e-12);
        EXPECT_NEAR(coneSpherePoint.head<2>().norm(),coneSpherePoint.z()*std::tan(pi/6),1e-12);
        const Eigen::Vector3d sphereCirclePoint(3*std::cos(t),3*std::sin(t),4);
        EXPECT_NEAR(sphereCirclePoint.norm(),5,1e-12);
        const Eigen::Vector3d coneCirclePoint(3*std::cos(t),3*std::sin(t),3*std::sqrt(3.));
        EXPECT_NEAR(coneCirclePoint.head<2>().norm(),coneCirclePoint.z()*std::tan(pi/6),1e-12);
    }
}

TEST(AssemblyContactAnalytic, ConeLineSamplesHaveCommonGeneratorsAndTangentPlanes) {
    for (bool external : {true,false}) {
        const double beta=external ? 5*pi/18 : pi/18;
        const Eigen::Vector3d a(0,0,1),b(std::sin(beta),0,std::cos(beta));
        const Eigen::Vector3d g(.5,0,std::sqrt(3.)/2), apex=10*g;
        for (double parameter : {11.,20.,35.}) {
            const Eigen::Vector3d point=parameter*g, q=point-apex;
            EXPECT_NEAR(point.dot(a)/point.norm(),std::cos(pi/6),1e-12);
            EXPECT_NEAR(q.dot(b)/q.norm(),std::cos(pi/9),1e-12);
            const Eigen::Vector3d n1=(std::cos(pi/6)*g-a)/std::sin(pi/6);
            const Eigen::Vector3d n2=(std::cos(pi/9)*g-b)/std::sin(pi/9);
            EXPECT_NEAR(n1.dot(n2),external ? -1 : 1,1e-12);
            EXPECT_NEAR(n1.dot(g),0,1e-12);
            EXPECT_NEAR(n2.dot(g),0,1e-12);
        }
    }
}

TEST(AssemblyContactAnalytic, MaterialReversalAndConeLeafAreExplicit) {
    auto a=support(SupportKind::Plane), b=support(SupportKind::Plane,Eigen::Vector3d::Zero(),-Eigen::Vector3d::UnitZ());
    EXPECT_TRUE(evaluate({},a,b).valid);
    EXPECT_EQ(evaluate({},a,b).residual.norm(),0);
    b.material_side=-1;
    // Plane.axis is already material-oriented, provenance must not reapply it.
    EXPECT_EQ(evaluate({},a,b).residual.norm(),0);
    b.axis=-b.axis;
    EXPECT_GT(evaluate({},a,b).residual.norm(),1);
    auto cone=support(SupportKind::Cone); cone.cone_leaf=-1;
    auto sphere=support(SupportKind::Sphere,{0,0,-4},Eigen::Vector3d::UnitZ(),2);
    EXPECT_LT(evaluate({ContactKind::Ring,ContactSide::Internal},sphere,cone).residual.norm(),1e-12);
    sphere.origin.z()=4;
    EXPECT_GT(evaluate({ContactKind::Ring,ContactSide::Internal},sphere,cone).residual.norm(),1);
}

TEST(AssemblyContactAnalytic, InvalidAndDegenerateInputsHaveStableSpecificDiagnostics) {
    auto cylinder=support(SupportKind::Cylinder), other=cylinder;
    EXPECT_EQ(evaluate({ContactKind::Line,ContactSide::Internal},cylinder,other).diagnostic,"CONTACT_LINE_DEGENERATES_TO_FACE");
    other.radius=2;
    EXPECT_EQ(evaluate({ContactKind::Face},cylinder,other).diagnostic,"CONTACT_FACE_RADIUS_MISMATCH");
    EXPECT_EQ(evaluate({ContactKind::Line},cylinder,other).diagnostic,"CONTACT_RADIAL_BRANCH_DEGENERATE");
    auto sphere=support(SupportKind::Sphere), circle=support(SupportKind::Circle,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2);
    EXPECT_EQ(evaluate({ContactKind::Ring},sphere,circle).diagnostic,"CONTACT_CIRCLE_EXCEEDS_SPHERE");
    auto cone=support(SupportKind::Cone), otherCone=cone;
    EXPECT_EQ(evaluate({ContactKind::Line,ContactSide::Internal},cone,otherCone).diagnostic,"CONTACT_LINE_DEGENERATES_TO_FACE");
    EXPECT_EQ(evaluate({ContactKind::Line},cone,otherCone).diagnostic,"CONTACT_CONE_GENERATOR_DEGENERATE");
    otherCone.half_angle=pi/5;
    EXPECT_EQ(evaluate({ContactKind::Face},cone,otherCone).diagnostic,"CONTACT_FACE_ANGLE_MISMATCH");
    cone.half_angle=0;
    EXPECT_EQ(evaluate({ContactKind::Ring},cone,circle).diagnostic,"CONTACT_INVALID_CONE");
    circle.radius=0;
    EXPECT_EQ(evaluate({ContactKind::Ring},sphere,circle).diagnostic,"CONTACT_INVALID_RADIUS");
    EXPECT_EQ(evaluate({ContactKind::Line},sphere,sphere).diagnostic,"CONTACT_UNSUPPORTED_COMBINATION");
}

TEST(AssemblyContactAnalytic, EverySideAndMaterialCombinationHasIndependentNormalEvidence) {
    for (bool external : {true,false}) for (int ma : {-1,1}) for (int mb : {-1,1}) {
        const ContactSide side=external ? ContactSide::External : ContactSide::Internal;
        const double desired=external ? -1 : 1;
        for (SupportKind kind : {SupportKind::Sphere,SupportKind::Cylinder}) {
            auto plane=support(SupportKind::Plane);
            plane.material_side=ma; // provenance only: outward axis is already z.
            const int heightSign=external ? mb : -mb;
            auto curved=support(kind,{0,0,2.*heightSign},
                                kind==SupportKind::Cylinder ? Eigen::Vector3d::UnitX() : Eigen::Vector3d::UnitZ(),2);
            curved.material_side=mb;
            const ContactKind contact=kind==SupportKind::Sphere ? ContactKind::Point : ContactKind::Line;
            const auto e=evaluate({contact,side,heightSign},plane,curved);
            ASSERT_TRUE(e.valid) << e.diagnostic;
            EXPECT_LT(e.residual.norm(),1e-12);
            const Eigen::Vector3d nPlane=plane.axis;
            const Eigen::Vector3d nCurved=mb*(Eigen::Vector3d::Zero()-curved.origin).normalized();
            EXPECT_NEAR(nPlane.dot(nCurved),desired,1e-12);
            EXPECT_EQ(evaluate({contact,side,-heightSign},plane,curved).diagnostic,
                      "CONTACT_PLANE_MATERIAL_BRANCH_MISMATCH");
        }
        const bool effectiveExternal=external==(ma*mb>0);
        auto a=support(SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2);
        auto b=support(SupportKind::Cylinder,{effectiveExternal ? 5. : 1.,0,0},Eigen::Vector3d::UnitZ(),3);
        a.material_side=ma; b.material_side=mb;
        const auto cylinders=evaluate({ContactKind::Line,side},a,b);
        ASSERT_TRUE(cylinders.valid) << cylinders.diagnostic;
        EXPECT_LT(cylinders.residual.norm(),1e-12);
        const Eigen::Vector3d contactPoint(effectiveExternal ? 2. : -2.,0,0);
        const Eigen::Vector3d na=ma*(contactPoint-a.origin).normalized();
        const Eigen::Vector3d nb=mb*(contactPoint-b.origin).normalized();
        EXPECT_NEAR(na.dot(nb),desired,1e-12);

        const double beta=effectiveExternal ? 5*pi/18 : pi/18;
        auto ca=support(SupportKind::Cone);
        auto cb=support(SupportKind::Cone,{5,0,5*std::sqrt(3.)},
                        {std::sin(beta),0,std::cos(beta)},1,pi/9);
        ca.material_side=ma; cb.material_side=mb;
        const auto cones=evaluate({ContactKind::Line,side},ca,cb);
        ASSERT_TRUE(cones.valid) << cones.diagnostic;
        EXPECT_LT(cones.residual.norm(),1e-12);
        const Eigen::Vector3d generator(.5,0,std::sqrt(3.)/2);
        const Eigen::Vector3d nca=ma*(std::cos(pi/6)*generator-ca.axis)/std::sin(pi/6);
        const Eigen::Vector3d ncb=mb*(std::cos(pi/9)*generator-cb.axis)/std::sin(pi/9);
        EXPECT_NEAR(nca.dot(ncb),desired,1e-12);

        for (SupportKind kind : {SupportKind::Cylinder,SupportKind::Sphere,SupportKind::Cone}) {
            auto faceA=support(kind),faceB=faceA;
            faceA.material_side=ma; faceB.material_side=mb;
            const auto face=evaluate({ContactKind::Face,side},faceA,faceB);
            EXPECT_EQ(face.valid,ma*mb==desired);
            if (!face.valid) EXPECT_EQ(face.diagnostic,"CONTACT_MATERIAL_SIDE_MISMATCH");
        }
        auto sphere=support(SupportKind::Sphere,{0,0,4},Eigen::Vector3d::UnitZ(),2);
        auto cone=support(SupportKind::Cone);
        sphere.material_side=ma; cone.material_side=mb;
        const auto ring=evaluate({ContactKind::Ring,side},sphere,cone);
        EXPECT_EQ(ring.valid,ma*mb==desired);
        if (ring.valid) {
            const Eigen::Vector3d point(std::sqrt(3.),0,3);
            const Eigen::Vector3d ns=ma*(point-sphere.origin).normalized();
            const Eigen::Vector3d nc=mb*Eigen::Vector3d(std::cos(pi/6),0,-std::sin(pi/6));
            EXPECT_NEAR(ns.dot(nc),desired,1e-12);
        } else EXPECT_EQ(ring.diagnostic,"CONTACT_MATERIAL_SIDE_MISMATCH");
        // Whole-circle incidence is not tangent material contact.
        auto circle=support(SupportKind::Circle,{0,0,4},Eigen::Vector3d::UnitZ(),3);
        auto sphereForCircle=support(SupportKind::Sphere,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),5);
        circle.material_side=ma; sphereForCircle.material_side=mb;
        const auto supportRing=evaluate({ContactKind::Ring,side},sphereForCircle,circle);
        ASSERT_TRUE(supportRing.valid) << supportRing.diagnostic;
        EXPECT_LT(supportRing.residual.norm(),1e-12);
    }
}
}  // namespace
}  // namespace occccad::assembly::analytic_contact

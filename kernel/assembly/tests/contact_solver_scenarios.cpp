#include <occccad/assembly/solver.hpp>
#include "../src/contact.hpp"

#include <Eigen/Geometry>
#include <gtest/gtest.h>
#include <algorithm>
#include <cmath>

namespace occccad::assembly {
namespace {
namespace ac=analytic_contact;
constexpr double pi=3.1415926535897932384626433832795;
Eigen::Vector3d vector(Vec3 v) { return {v.x,v.y,v.z}; }
Vec3 value(const Eigen::Vector3d& v) { return {v.x(),v.y(),v.z()}; }
Eigen::Quaterniond rotation(Quaternion q) { return {q.w,q.x,q.y,q.z}; }
Geometry geometry(const ac::Support& s) {
    switch(s.kind) {
    case ac::SupportKind::Plane: return PlaneGeometry{value(s.origin),value(s.axis)};
    case ac::SupportKind::Cylinder: return CylinderGeometry{value(s.origin),value(s.axis),s.radius,s.material_side};
    case ac::SupportKind::Sphere: return SphereGeometry{value(s.origin),s.radius,s.material_side};
    case ac::SupportKind::Cone: return ConeGeometry{value(s.origin),value(s.axis),s.half_angle,s.cone_leaf,s.material_side};
    case ac::SupportKind::Circle: return CircleGeometry{value(s.origin),value(s.axis),s.radius,{1,0,0}};
    }
    return PointGeometry{};
}
ac::Support support(ac::SupportKind kind, Eigen::Vector3d origin=Eigen::Vector3d::Zero(),
                    Eigen::Vector3d axis=Eigen::Vector3d::UnitZ(),double radius=1,double angle=pi/6,int material=1) {
    return {kind,origin,axis,radius,angle,1,material};
}
ac::Support placed(ac::Support s, const Pose& p) {
    s.origin=rotation(p.rotation)*s.origin+vector(p.translation);
    s.axis=rotation(p.rotation)*s.axis;
    return s;
}
void independent_geometry(const ac::Definition& d,ac::Support a,ac::Support b,
                          double lengthTolerance,double angleTolerance) {
    if (static_cast<int>(a.kind)>static_cast<int>(b.kind)) std::swap(a,b);
    const auto delta=(b.origin-a.origin).eval();
    const bool external=d.side==ac::ContactSide::External;
    const bool effectiveExternal=external==(a.material_side*b.material_side>0);
    const double sampleTolerance=lengthTolerance+angleTolerance*std::max({a.radius,b.radius,delta.norm()});
    if (a.kind==ac::SupportKind::Plane && b.kind==ac::SupportKind::Plane) {
        EXPECT_LE((a.axis-(external ? -b.axis : b.axis)).norm(),angleTolerance);
        EXPECT_LE(std::abs(delta.dot(a.axis)),lengthTolerance);
    } else if (a.kind==ac::SupportKind::Plane && b.kind==ac::SupportKind::Cylinder) {
        EXPECT_LE(std::abs(a.axis.dot(b.axis)),angleTolerance);
        EXPECT_LE(std::abs(delta.dot(a.axis)-d.branch*b.radius),lengthTolerance);
        const Eigen::Vector3d point=b.origin-d.branch*b.radius*a.axis;
        EXPECT_LE(std::abs((point-a.origin).dot(a.axis)),lengthTolerance);
        EXPECT_LE(std::abs(((point-b.origin).cross(b.axis)).norm()-b.radius),sampleTolerance);
        const Eigen::Vector3d curvedNormal=b.material_side*(point-b.origin).normalized();
        EXPECT_NEAR(a.axis.dot(curvedNormal),external ? -1. : 1.,angleTolerance);
    } else if (a.kind==ac::SupportKind::Plane && b.kind==ac::SupportKind::Sphere) {
        const Eigen::Vector3d point=b.origin-d.branch*b.radius*a.axis;
        EXPECT_LE(std::abs((point-a.origin).dot(a.axis)),lengthTolerance);
        EXPECT_LE(std::abs((point-b.origin).norm()-b.radius),sampleTolerance);
        EXPECT_NEAR(a.axis.dot(b.material_side*(point-b.origin).normalized()),external ? -1. : 1.,angleTolerance);
    } else if (a.kind==ac::SupportKind::Cylinder && b.kind==ac::SupportKind::Cylinder) {
        EXPECT_LE(a.axis.cross(b.axis).norm(),angleTolerance);
        const double distance=delta.cross(a.axis).norm();
        const double target=d.kind==ac::ContactKind::Face ? 0 :
                            effectiveExternal ? a.radius+b.radius : std::abs(a.radius-b.radius);
        EXPECT_LE(std::abs(distance-target),lengthTolerance);
    } else if (a.kind==ac::SupportKind::Sphere && b.kind==ac::SupportKind::Sphere) {
        EXPECT_LE(delta.norm(),lengthTolerance);
        EXPECT_EQ(a.radius,b.radius);
    } else if (a.kind==ac::SupportKind::Sphere && b.kind==ac::SupportKind::Cone) {
        const Eigen::Vector3d axis=b.cone_leaf*b.axis;
        const Eigen::Vector3d radial=axis.unitOrthogonal();
        const Eigen::Vector3d point=b.origin+(a.radius/std::sin(b.half_angle))*
            (std::cos(b.half_angle)*std::cos(b.half_angle)*axis+
             std::sin(b.half_angle)*std::cos(b.half_angle)*radial);
        EXPECT_LE(std::abs((point-a.origin).norm()-a.radius),sampleTolerance);
        const auto q=(point-b.origin).eval();
        EXPECT_LE(std::abs(q.cross(axis).norm()-q.dot(axis)*std::tan(b.half_angle)),sampleTolerance);
        EXPECT_LE((a.origin-b.origin).cross(axis).norm(),lengthTolerance);
        const Eigen::Vector3d sphereNormal=a.material_side*(point-a.origin).normalized();
        const Eigen::Vector3d coneNormal=b.material_side*(std::cos(b.half_angle)*radial-std::sin(b.half_angle)*axis);
        EXPECT_NEAR(sphereNormal.dot(coneNormal),external ? -1. : 1.,angleTolerance);
    } else if (a.kind==ac::SupportKind::Sphere && b.kind==ac::SupportKind::Circle) {
        const Eigen::Vector3d x=b.axis.unitOrthogonal(),y=b.axis.cross(x);
        for (double theta : {0.,.7,2.1,4.5}) {
            const Eigen::Vector3d point=b.origin+b.radius*(std::cos(theta)*x+std::sin(theta)*y);
            EXPECT_LE(std::abs((point-a.origin).norm()-a.radius),sampleTolerance);
        }
        const double height=d.branch*std::sqrt((a.radius-b.radius)*(a.radius+b.radius));
        EXPECT_LE(std::abs(delta.dot(b.axis)-height),lengthTolerance);
    } else if (a.kind==ac::SupportKind::Cone && b.kind==ac::SupportKind::Cone) {
        const Eigen::Vector3d aa=a.cone_leaf*a.axis,bb=b.cone_leaf*b.axis;
        if (d.kind==ac::ContactKind::Face) {
            EXPECT_LE(delta.norm(),lengthTolerance);
            EXPECT_LE((aa-bb).norm(),angleTolerance);
        } else {
            const double beta=effectiveExternal ? a.half_angle+b.half_angle : std::abs(a.half_angle-b.half_angle);
            EXPECT_LE(std::abs(std::acos(std::clamp(aa.dot(bb),-1.,1.))-beta),angleTolerance);
            // A point well beyond both apices must lie on both selected leaves.
            const double dot=aa.dot(bb),denominator=1-dot*dot;
            const Eigen::Vector3d generator=(((std::cos(a.half_angle)-dot*std::cos(b.half_angle))*aa+
                (std::cos(b.half_angle)-dot*std::cos(a.half_angle))*bb)/denominator).normalized();
            EXPECT_LE(delta.cross(generator).norm(),lengthTolerance);
            const Eigen::Vector3d point=a.origin+(delta.norm()+20)*generator;
            for (const auto* cone : {&a,&b}) {
                const Eigen::Vector3d q=point-cone->origin,axis=cone->cone_leaf*cone->axis;
                EXPECT_GT(q.dot(axis),0);
                EXPECT_LE(std::abs(q.cross(axis).norm()-q.dot(axis)*std::tan(cone->half_angle)),sampleTolerance);
            }
            auto materialNormal=[&](const ac::Support& cone) {
                const Eigen::Vector3d axis=cone.cone_leaf*cone.axis,q=point-cone.origin;
                const Eigen::Vector3d radial=(q-q.dot(axis)*axis).normalized();
                return (cone.material_side*(std::cos(cone.half_angle)*radial-std::sin(cone.half_angle)*axis)).eval();
            };
            EXPECT_NEAR(materialNormal(a).dot(materialNormal(b)),external ? -1. : 1.,angleTolerance);
        }
    } else if (a.kind==ac::SupportKind::Cone && b.kind==ac::SupportKind::Circle) {
        const Eigen::Vector3d axis=a.cone_leaf*a.axis,x=b.axis.unitOrthogonal(),y=b.axis.cross(x);
        for (double theta : {0.,.7,2.1,4.5}) {
            const Eigen::Vector3d q=b.origin+b.radius*(std::cos(theta)*x+std::sin(theta)*y)-a.origin;
            EXPECT_GT(q.dot(axis),0);
            EXPECT_LE(std::abs(q.cross(axis).norm()-q.dot(axis)*std::tan(a.half_angle)),sampleTolerance);
        }
        EXPECT_LE(b.axis.cross(axis).norm(),angleTolerance);
    }
}
void run(ac::Definition definition,ac::Support first,ac::Support second,int rank,bool swapped=false,
         std::optional<Pose> initial=std::nullopt) {
    if(swapped) std::swap(first,second);
    Model model;
    model.bodies={{"reference",{}},{"moving",{{.4,.2,.7},{0,std::sin(.025),0,std::cos(.025)}}}};
    if(initial) model.bodies[1].initial_pose=*initial;
    model.geometry={{"support","reference",geometry(first)},{"support","moving",geometry(second)}};
    Constraint fix; fix.id="ground"; fix.kind=ConstraintKind::Fix; fix.first={"reference",{}};
    Constraint contact; contact.id="contact"; contact.kind=ConstraintKind::Contact;
    contact.first={"reference","support"}; contact.second=GeometryRef{"moving","support"};
    contact.contact_kind=static_cast<ContactKind>(definition.kind);
    contact.contact_side=static_cast<ContactSide>(definition.side);
    contact.contact_branch=definition.branch;
    model.constraints={fix,contact};
    SolverOptions options;
    options.solve_intent=SolveIntent{{"moving"},{"reference"},SolvePreferencePolicy::MoveFirstMinimizeReference};
    const auto result=Solver{}.solve(model,options);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged)
        << "reference_opt=" << result.components[0].preference.reference_optimality
        << "total_opt=" << result.components[0].preference.total_optimality
        << "energy=" << result.components[0].preference.total_objective
        << "iterations=" << result.components[0].preference.iterations;
    EXPECT_EQ(result.components[0].jacobian_rank,static_cast<std::size_t>(rank));
    EXPECT_EQ(result.components[0].relative_dof,static_cast<std::size_t>(6-rank));
    const auto moving=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& body){return body.id=="moving";});
    ASSERT_NE(moving,result.bodies.end());
    EXPECT_GT((vector(moving->pose.translation)-vector(model.bodies[1].initial_pose.translation)).norm()+
        Eigen::AngleAxisd(rotation(moving->pose.rotation)*rotation(model.bodies[1].initial_pose.rotation).inverse()).angle(),1e-5);
    independent_geometry(definition,first,placed(second,moving->pose),options.length_tolerance,options.angle_tolerance);
}

TEST(AssemblyContact, PlanePlaneFaceMovesAndHasRankThree) { run({ac::ContactKind::Face},support(ac::SupportKind::Plane),support(ac::SupportKind::Plane,Eigen::Vector3d::Zero(),-Eigen::Vector3d::UnitZ()),3); }
TEST(AssemblyContact, PlaneCylinderLineMovesAndHasRankTwo) { run({ac::ContactKind::Line},support(ac::SupportKind::Plane),support(ac::SupportKind::Cylinder,{0,0,2},Eigen::Vector3d::UnitX(),2),2); }
TEST(AssemblyContact, PlaneSpherePointMovesAndHasRankOne) { run({ac::ContactKind::Point},support(ac::SupportKind::Plane),support(ac::SupportKind::Sphere,{0,0,2},Eigen::Vector3d::UnitZ(),2),1); }
TEST(AssemblyContact, CylinderCylinderExternalLineMovesAndHasRankThree) { run({ac::ContactKind::Line},support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2),support(ac::SupportKind::Cylinder,{5,0,0},Eigen::Vector3d::UnitZ(),3),3); }
TEST(AssemblyContact, CylinderCylinderInternalLineMovesAndHasRankThree) { run({ac::ContactKind::Line,ac::ContactSide::Internal},support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2),support(ac::SupportKind::Cylinder,{1,0,0},Eigen::Vector3d::UnitZ(),3),3); }
TEST(AssemblyContact, CylinderCylinderFaceMovesAndHasRankFour) { run({ac::ContactKind::Face,ac::ContactSide::Internal},support(ac::SupportKind::Cylinder),support(ac::SupportKind::Cylinder,{0,0,8}),4); }
TEST(AssemblyContact, SphereSphereFaceMovesAndHasRankThree) { run({ac::ContactKind::Face,ac::ContactSide::Internal},support(ac::SupportKind::Sphere),support(ac::SupportKind::Sphere),3); }
TEST(AssemblyContact, SphereConeRingMovesAndHasRankThree) { run({ac::ContactKind::Ring,ac::ContactSide::Internal},support(ac::SupportKind::Sphere,{0,0,4},Eigen::Vector3d::UnitZ(),2),support(ac::SupportKind::Cone),3); }
TEST(AssemblyContact, SphereCircleRingMovesAndHasRankThree) { run({ac::ContactKind::Ring},support(ac::SupportKind::Sphere,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),5),support(ac::SupportKind::Circle,{0,0,4},Eigen::Vector3d::UnitZ(),3),3); }
TEST(AssemblyContact, SphereCircleEquatorKeepsRankThree) { run({ac::ContactKind::Ring},support(ac::SupportKind::Sphere,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),5),support(ac::SupportKind::Circle,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),5),3); }
TEST(AssemblyContact, ConeConeExternalLineMovesAndHasRankThree) { run({ac::ContactKind::Line},support(ac::SupportKind::Cone),support(ac::SupportKind::Cone,{5,0,5*std::sqrt(3.)},{std::sin(5*pi/18),0,std::cos(5*pi/18)},1,pi/9),3); }
TEST(AssemblyContact, ConeConeInternalLineMovesAndHasRankThree) { run({ac::ContactKind::Line,ac::ContactSide::Internal},support(ac::SupportKind::Cone),support(ac::SupportKind::Cone,{5,0,5*std::sqrt(3.)},{std::sin(pi/18),0,std::cos(pi/18)},1,pi/9),3); }
TEST(AssemblyContact, ConeConeFaceMovesAndHasRankFive) { run({ac::ContactKind::Face,ac::ContactSide::Internal},support(ac::SupportKind::Cone),support(ac::SupportKind::Cone),5); }
TEST(AssemblyContact, ConeCircleRingMovesAndHasRankFive) { run({ac::ContactKind::Ring},support(ac::SupportKind::Cone),support(ac::SupportKind::Circle,{0,0,3*std::sqrt(3.)},Eigen::Vector3d::UnitZ(),3),5); }
TEST(AssemblyContact, SwappedSpherePlaneKeepsTheSameContactSet) { run({ac::ContactKind::Point},support(ac::SupportKind::Plane),support(ac::SupportKind::Sphere,{0,0,2},Eigen::Vector3d::UnitZ(),2),1,true); }
TEST(AssemblyContact, ConcentricCylinderInitialPoseIsSeededNotRejected) { run({ac::ContactKind::Line},support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2),support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),3),3,false,Pose{}); }
TEST(AssemblyContact, ParallelConeInitialPoseIsSeededNotRejected) { run({ac::ContactKind::Line},support(ac::SupportKind::Cone),support(ac::SupportKind::Cone,{5,0,5*std::sqrt(3.)},Eigen::Vector3d::UnitZ(),1,pi/9),3,false,Pose{}); }

TEST(AssemblyContact, PlaneSphereAllMaterialSidesAndSelectionOrders) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int material : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(material);
            SCOPED_TRACE(swapped);
            const int branch=side==ac::ContactSide::External ? material : -material;
            run({ac::ContactKind::Point,side,branch},support(ac::SupportKind::Plane),
                support(ac::SupportKind::Sphere,{0,0,2.*branch},Eigen::Vector3d::UnitZ(),2,pi/6,material),1,swapped);
        }
}
TEST(AssemblyContact, PlaneCylinderAllMaterialSidesAndSelectionOrders) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int material : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(material);
            SCOPED_TRACE(swapped);
            const int branch=side==ac::ContactSide::External ? material : -material;
            run({ac::ContactKind::Line,side,branch},support(ac::SupportKind::Plane),
                support(ac::SupportKind::Cylinder,{0,0,2.*branch},Eigen::Vector3d::UnitX(),2,pi/6,material),2,swapped);
        }
}
TEST(AssemblyContact, CylinderLineAllMaterialSidesAndSelectionOrders) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int first : {-1,1}) for(int second : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(first);
            SCOPED_TRACE(second);
            SCOPED_TRACE(swapped);
            const bool ext=(side==ac::ContactSide::External)==(first*second>0);
            run({ac::ContactKind::Line,side},support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2,pi/6,first),
                support(ac::SupportKind::Cylinder,{ext ? 5. : 1.,0,0},Eigen::Vector3d::UnitZ(),3,pi/6,second),3,swapped);
        }
}
TEST(AssemblyContact, FixedImpossibleContactCannotAdoptCandidateGeometry) {
    Model model;
    model.bodies={{"plane",{}},{"sphere",{{0,0,7},{}}}};
    model.geometry={{"plane","plane",PlaneGeometry{{},{0,0,1}}},{"sphere","sphere",SphereGeometry{{},2,1}}};
    Constraint a; a.id="fix-plane"; a.kind=ConstraintKind::Fix; a.first={"plane",{}};
    Constraint b=a; b.id="fix-sphere"; b.first={"sphere",{}};
    Constraint c; c.id="impossible-contact"; c.kind=ConstraintKind::Contact;
    c.first={"plane","plane"}; c.second=GeometryRef{"sphere","sphere"}; c.contact_kind=ContactKind::Point;
    model.constraints={a,b,c};
    const auto result=Solver{}.solve(model);
    EXPECT_EQ(result.status,SolveStatus::Inconsistent);
    const auto sphere=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& body){return body.id=="sphere";});
    ASSERT_NE(sphere,result.bodies.end()); EXPECT_DOUBLE_EQ(sphere->pose.translation.z,7.);
    EXPECT_GT(std::abs(sphere->pose.translation.z-2.),1.);
    EXPECT_NE(std::find(result.conflicting_constraint_ids.begin(),result.conflicting_constraint_ids.end(),c.id),result.conflicting_constraint_ids.end());
}

TEST(AssemblyContact, RouterCylinderCapParallelNormalInitialPoseRequiresQuarterTurn) {
    run({ac::ContactKind::Line},support(ac::SupportKind::Plane,{0,0,12},Eigen::Vector3d::UnitZ()),
        support(ac::SupportKind::Cylinder,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),6),2,false,
        Pose{{19,-7,24},{}});
}

TEST(AssemblyContact, ConeLineMaterialSidesSelectedLeavesAndSelectionExchange) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int ma : {-1,1}) for(int mb : {-1,1}) for(int leaf : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(static_cast<int>(side));
            SCOPED_TRACE(ma);
            SCOPED_TRACE(mb);
            SCOPED_TRACE(leaf);
            SCOPED_TRACE(swapped);
            const bool external=(side==ac::ContactSide::External)==(ma*mb>0);
            const double beta=external ? 5*pi/18 : pi/18;
            auto first=support(ac::SupportKind::Cone,Eigen::Vector3d::Zero(),leaf*Eigen::Vector3d::UnitZ(),1,pi/6,ma);
            auto second=support(ac::SupportKind::Cone,{5,0,5*std::sqrt(3.)},leaf*Eigen::Vector3d(std::sin(beta),0,std::cos(beta)),1,pi/9,mb);
            first.cone_leaf=leaf; second.cone_leaf=leaf;
            run({ac::ContactKind::Line,side},first,second,3,swapped);
        }
}
TEST(AssemblyContact, SphereConeRingMaterialSidesSelectedLeavesAndSelectionExchange) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int material : {-1,1}) for(int leaf : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(static_cast<int>(side));
            SCOPED_TRACE(material);
            SCOPED_TRACE(leaf);
            SCOPED_TRACE(swapped);
            const int coneMaterial=side==ac::ContactSide::External ? -material : material;
            auto cone=support(ac::SupportKind::Cone,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),1,pi/6,coneMaterial);
            cone.cone_leaf=leaf;
            run({ac::ContactKind::Ring,side},support(ac::SupportKind::Sphere,{0,0,4.*leaf},Eigen::Vector3d::UnitZ(),2,pi/6,material),cone,3,swapped);
        }
}
TEST(AssemblyContact, SphereCircleTwoBranchesNormalReversalAndSelectionExchange) {
    for(int branch : {-1,1}) for(int normal : {-1,1}) for(bool swapped : {false,true}) {
        SCOPED_TRACE(branch);
        SCOPED_TRACE(normal);
        SCOPED_TRACE(swapped);
        run({ac::ContactKind::Ring,ac::ContactSide::External,branch},
            support(ac::SupportKind::Sphere,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),5),
            support(ac::SupportKind::Circle,{0,0,4.*branch*normal},normal*Eigen::Vector3d::UnitZ(),3),3,swapped);
    }
}
TEST(AssemblyContact, ConeCircleSelectedLeafMaterialDoesNotInventTangencyAndSelectionExchange) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int material : {-1,1}) for(int leaf : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(static_cast<int>(side));
            SCOPED_TRACE(material);
            SCOPED_TRACE(leaf);
            SCOPED_TRACE(swapped);
            auto cone=support(ac::SupportKind::Cone,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),1,pi/6,material);
            cone.cone_leaf=leaf;
            run({ac::ContactKind::Ring,side},cone,
                support(ac::SupportKind::Circle,{0,0,leaf*3*std::sqrt(3.)},-leaf*Eigen::Vector3d::UnitZ(),3),5,swapped);
        }
}
TEST(AssemblyContact, PlaneSphereCoupledAngleAndCenterOffsetIsRedundantWithoutExtraRank) {
    Model model;
    model.bodies={{"reference",{}},{"moving",{{5,-3,7},{0,std::sin(.2),0,std::cos(.2)}}}};
    model.geometry={{"plane","reference",PlaneGeometry{{},{0,0,1}}},
        {"sphere","moving",SphereGeometry{{},2,1}},
        {"axis","moving",AxisGeometry{{},{0,0,1}}},
        {"center","moving",PointGeometry{}}};
    Constraint fix; fix.id="ground"; fix.kind=ConstraintKind::Fix; fix.first={"reference",{}};
    Constraint contact; contact.id="contact"; contact.kind=ConstraintKind::Contact;
    contact.first={"reference","plane"}; contact.second=GeometryRef{"moving","sphere"}; contact.contact_kind=ContactKind::Point;
    Constraint angle; angle.id="angle"; angle.kind=ConstraintKind::Parallel;
    angle.first={"moving","axis"}; angle.second=GeometryRef{"reference","plane"}; angle.direction_relation=DirectionRelation::Same;
    Constraint offset; offset.id="redundant-center-offset"; offset.kind=ConstraintKind::Distance;
    offset.first={"moving","center"}; offset.second=GeometryRef{"reference","plane"};
    offset.distance_relation=DistanceRelation::SelectedPlaneNormal; offset.value=2.;
    model.constraints={fix,contact,angle,offset};
    const auto result=Solver{}.solve(model);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].jacobian_rank,3);
    EXPECT_EQ(result.components[0].relative_dof,3);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged);
    const auto moving=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& body){return body.id=="moving";});
    ASSERT_NE(moving,result.bodies.end());
    EXPECT_NEAR(moving->pose.translation.z,2.,1e-7);
    EXPECT_LT((rotation(moving->pose.rotation)*Eigen::Vector3d::UnitZ()-Eigen::Vector3d::UnitZ()).norm(),1e-8);
    EXPECT_GT((vector(moving->pose.translation)-vector(model.bodies[1].initial_pose.translation)).norm(),1.);
    EXPECT_FALSE(result.redundant_constraint_ids.empty());
}

TEST(AssemblyContact, PlaneFaceBothMaterialNormalRelationsAndSelectionExchange) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal}) for(bool swapped : {false,true}) {
        SCOPED_TRACE(static_cast<int>(side));
        SCOPED_TRACE(swapped);
        Eigen::Vector3d normal=Eigen::Vector3d::UnitZ();
        if(side==ac::ContactSide::External) normal=-normal;
        run({ac::ContactKind::Face,side},support(ac::SupportKind::Plane),
            support(ac::SupportKind::Plane,Eigen::Vector3d::Zero(),normal),3,swapped);
    }
}
void face_materials(ac::SupportKind kind,int rank) {
    for(auto side : {ac::ContactSide::External,ac::ContactSide::Internal})
        for(int material : {-1,1}) for(bool swapped : {false,true}) {
            SCOPED_TRACE(static_cast<int>(side));
            SCOPED_TRACE(material);
            SCOPED_TRACE(swapped);
            const int other=side==ac::ContactSide::External ? -material : material;
            run({ac::ContactKind::Face,side},support(kind,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2,pi/6,material),
                support(kind,Eigen::Vector3d::Zero(),Eigen::Vector3d::UnitZ(),2,pi/6,other),rank,swapped);
        }
}
TEST(AssemblyContact, CylinderFaceBothMaterialSidesAndSelectionExchange) { face_materials(ac::SupportKind::Cylinder,4); }
TEST(AssemblyContact, SphereFaceBothMaterialSidesAndSelectionExchange) { face_materials(ac::SupportKind::Sphere,3); }
TEST(AssemblyContact, ConeFaceBothMaterialSidesAndSelectionExchange) { face_materials(ac::SupportKind::Cone,5); }

TEST(AssemblyContact, ExternalPlaneFaceAntipodeTurnsWholeRigidGroupAndPreservesInternalRelations) {
    Model model;
    model.bodies={{"a",{}},{"b",{{0,0,8},{}}},{"reference",{}}};
    for(const auto& body:model.bodies) {
        model.geometry.push_back({"cap",body.id,PlaneGeometry{{0,0,12},{0,0,1}}});
        model.geometry.push_back({"axis",body.id,AxisGeometry{{},{0,0,1}}});
    }
    Constraint fix; fix.id="fix-reference"; fix.kind=ConstraintKind::Fix; fix.first={"reference",{}};
    Constraint rigid; rigid.id="group-link"; rigid.kind=ConstraintKind::Rigid;
    // Native Rigid stores first pose in second coordinates (T_b^-1 T_a).
    rigid.first={"a",{}}; rigid.second=GeometryRef{"b",{}}; rigid.fixed_pose=Pose{{0,0,-8},{}};
    Constraint offset; offset.id="inner-offset"; offset.kind=ConstraintKind::Distance;
    offset.first={"b","cap"}; offset.second=GeometryRef{"a","cap"};
    offset.distance_relation=DistanceRelation::SelectedPlaneNormal; offset.direction_relation=DirectionRelation::Same; offset.value=8;
    Constraint coincide; coincide.id="inner-coaxial"; coincide.kind=ConstraintKind::Concentric;
    coincide.first={"a","axis"}; coincide.second=GeometryRef{"b","axis"}; coincide.direction_relation=DirectionRelation::Same;
    Constraint angle=coincide; angle.id="inner-angle"; angle.kind=ConstraintKind::Angle; angle.value=0;
    Constraint contact; contact.id="outer-contact"; contact.kind=ConstraintKind::Contact;
    contact.first={"a","cap"}; contact.second=GeometryRef{"reference","cap"};
    contact.contact_kind=ContactKind::Face; contact.contact_side=ContactSide::External;
    model.constraints={fix,rigid,offset,coincide,angle,contact};
    SolverOptions options; options.solve_intent=SolveIntent{{"a"},{"reference"},SolvePreferencePolicy::MoveFirstMinimizeReference};
    const auto result=Solver{}.solve(model,options);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1); EXPECT_EQ(result.components[0].jacobian_rank,3);
    EXPECT_EQ(result.components[0].relative_dof,3);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged);
    const auto pose=[&](const char* id) {
        const auto found=std::find_if(result.bodies.begin(),result.bodies.end(),[&](const auto& body){return body.id==id;});
        EXPECT_NE(found,result.bodies.end()); return found==result.bodies.end() ? Pose{} : found->pose;
    };
    const auto a=pose("a"),b=pose("b"),reference=pose("reference");
    const Eigen::Vector3d normal=rotation(a.rotation)*Eigen::Vector3d::UnitZ();
    const Eigen::Vector3d capA=vector(a.translation)+rotation(a.rotation)*Eigen::Vector3d(0,0,12);
    const Eigen::Vector3d capB=vector(b.translation)+rotation(b.rotation)*Eigen::Vector3d(0,0,12);
    EXPECT_LT((normal+Eigen::Vector3d::UnitZ()).norm(),1e-8);
    EXPECT_NEAR(capA.z(),12.,1e-7);
    EXPECT_NEAR((capB-capA).dot(normal),8.,1e-7);
    EXPECT_LT((rotation(b.rotation)*Eigen::Vector3d::UnitZ()-normal).norm(),1e-8);
    EXPECT_LT((rotation(a.rotation).inverse()*(vector(b.translation)-vector(a.translation))-Eigen::Vector3d(0,0,8)).norm(),1e-7);
    EXPECT_LT(vector(reference.translation).norm(),1e-12);
    EXPECT_GT(Eigen::AngleAxisd(rotation(a.rotation)).angle(),3.);
}

void point_on(Geometry surface,ConstraintKind kind,int rank,Vec3 initial={3,2,4}) {
    Model model;
    model.bodies={{"reference",{}},{"moving",{initial,{}}}};
    model.geometry={{"surface","reference",surface},{"point","moving",PointGeometry{}}};
    Constraint ground; ground.id="ground"; ground.kind=ConstraintKind::Fix; ground.first={"reference",{}};
    Constraint incidence; incidence.id="incidence"; incidence.kind=kind;
    incidence.first={"moving","point"}; incidence.second=GeometryRef{"reference","surface"};
    model.constraints={ground,incidence};
    const auto result=Solver{}.solve(model);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged);
    EXPECT_EQ(result.components[0].jacobian_rank,static_cast<std::size_t>(rank));
    const auto body=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& b){return b.id=="moving";});
    ASSERT_NE(body,result.bodies.end());
    const Eigen::Vector3d p=vector(body->pose.translation);
    EXPECT_GT((p-vector(initial)).norm(),1e-5);
    if(std::holds_alternative<CircleGeometry>(surface)) {
        EXPECT_LE(std::abs(p.z()),1e-7); EXPECT_LE(std::abs(p.head<2>().norm()-2),1e-7);
    } else if(std::holds_alternative<SphereGeometry>(surface)) EXPECT_LE(std::abs(p.norm()-2),1e-7);
    else if(std::holds_alternative<ConeGeometry>(surface)) {
        EXPECT_GT(p.z(),0); EXPECT_LE(std::abs(p.head<2>().norm()-p.z()*std::tan(pi/6)),1e-7);
    } else EXPECT_LE(std::abs(p.head<2>().norm()-2),1e-7);
}
TEST(AssemblyExactIncidence, PointCircleUsesExactUnderlyingCircleAndRankTwo) { point_on(CircleGeometry{{},{0,0,1},2,{1,0,0}},ConstraintKind::Coincident,2); }
TEST(AssemblyExactIncidence, PointSphereUsesSurfaceNotCenterAndRankOne) { point_on(SphereGeometry{{},2,1},ConstraintKind::Coincident,1); }
TEST(AssemblyExactIncidence, PointConeUsesSelectedLeafAndRankOne) { point_on(ConeGeometry{{},{0,0,1},pi/6,1,1},ConstraintKind::Coincident,1); }
TEST(AssemblyExactIncidence, ExplicitPointCylinderSurfaceDoesNotBecomeAxisIncidence) { point_on(CylinderGeometry{{},{0,0,1},2,1},ConstraintKind::SurfaceIncidence,1); }
TEST(AssemblyExactIncidence, CircleCenterInitialPoseUsesStableRadialSeed) { point_on(CircleGeometry{{},{0,0,1},2,{1,0,0}},ConstraintKind::Coincident,2,{}); }
TEST(AssemblyExactIncidence, SphereCenterInitialPoseUsesStableRadialSeed) { point_on(SphereGeometry{{},2,1},ConstraintKind::Coincident,1,{}); }
TEST(AssemblyExactIncidence, CylinderAxisInitialPoseUsesStableRadialSeed) { point_on(CylinderGeometry{{},{0,0,1},2,1},ConstraintKind::SurfaceIncidence,1,{}); }

TEST(AssemblyExactIncidence, FrameFrameControlsFullRelativePoseIncludingAntipodalInitialRotation) {
    Model model;
    model.bodies={{"reference",{}},{"moving",{{4,-2,7},{0,1,0,0}}}};
    const Pose a{{10,20,-5},{0,0,std::sin(.2),std::cos(.2)}},b{{1,-3,2},{std::sin(.1),0,0,std::cos(.1)}};
    model.geometry={{"frame","reference",FrameGeometry{a}},{"frame","moving",FrameGeometry{b}}};
    Constraint ground; ground.id="ground"; ground.kind=ConstraintKind::Fix; ground.first={"reference",{}};
    Constraint match; match.id="frame-match"; match.kind=ConstraintKind::Coincident;
    match.first={"moving","frame"}; match.second=GeometryRef{"reference","frame"};
    model.constraints={ground,match};
    const auto result=Solver{}.solve(model);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged);
    EXPECT_EQ(result.components[0].jacobian_rank,6);
    EXPECT_EQ(result.components[0].relative_dof,0);
    const auto body=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& x){return x.id=="moving";});
    ASSERT_NE(body,result.bodies.end());
    const Eigen::Vector3d finalOrigin=rotation(body->pose.rotation)*vector(b.translation)+vector(body->pose.translation);
    const Eigen::Quaterniond finalRotation=rotation(body->pose.rotation)*rotation(b.rotation);
    EXPECT_LE((finalOrigin-vector(a.translation)).norm(),1e-7);
    EXPECT_LE(Eigen::AngleAxisd(rotation(a.rotation).inverse()*finalRotation).angle(),1e-8);
}

TEST(AssemblyExactIncidence, ConcentricAcceptsDifferentRadiiAndConeCircleAxes) {
    Model model;
    model.bodies={{"reference",{}},{"moving",{{3,2,4},{0,std::sin(.2),0,std::cos(.2)}}}};
    model.geometry={{"axis","reference",ConeGeometry{{},{0,0,1},pi/6,1,1}},
                    {"axis","moving",CircleGeometry{{},{0,0,1},7,{1,0,0}}}};
    Constraint ground; ground.id="ground"; ground.kind=ConstraintKind::Fix; ground.first={"reference",{}};
    Constraint concentric; concentric.id="axes"; concentric.kind=ConstraintKind::Concentric;
    concentric.first={"moving","axis"}; concentric.second=GeometryRef{"reference","axis"};
    model.constraints={ground,concentric};
    const auto result=Solver{}.solve(model);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].jacobian_rank,4);
    EXPECT_EQ(result.components[0].relative_dof,2);
    const auto body=std::find_if(result.bodies.begin(),result.bodies.end(),[](const auto& b){return b.id=="moving";});
    ASSERT_NE(body,result.bodies.end());
    EXPECT_LE(vector(body->pose.translation).head<2>().norm(),1e-7);
    EXPECT_LE((rotation(body->pose.rotation)*Eigen::Vector3d::UnitZ()).cross(Eigen::Vector3d::UnitZ()).norm(),1e-8);
}
}  // namespace
}  // namespace occccad::assembly

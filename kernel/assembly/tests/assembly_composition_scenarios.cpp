#include <occccad/assembly/solver.hpp>

#include <Eigen/Geometry>
#include <Eigen/QR>
#include <gtest/gtest.h>
#include <algorithm>
#include <cmath>

namespace occccad::assembly {
namespace {
constexpr double pi=3.1415926535897932384626433832795;
enum class Shape { Point,Axis,Plane,Cylinder };
enum class Relation { Free,Directed,Parallel,Perpendicular };
Eigen::Vector3d vec(Vec3 v) { return {v.x,v.y,v.z}; }
Vec3 val(const Eigen::Vector3d& v) { return {v.x(),v.y(),v.z()}; }
Eigen::Quaterniond quat(Quaternion q) { return {q.w,q.x,q.y,q.z}; }
Quaternion val(const Eigen::Quaterniond& q) { return {q.x(),q.y(),q.z(),q.w()}; }
Pose result_pose(const SolveResult& result,const char* id) {
    const auto found=std::find_if(result.bodies.begin(),result.bodies.end(),[&](const auto& b){return b.id==id;});
    EXPECT_NE(found,result.bodies.end());
    return found==result.bodies.end() ? Pose{} : found->pose;
}
Geometry geometry(Shape type,Eigen::Vector3d origin,Eigen::Vector3d direction,double radius) {
    switch(type) {
    case Shape::Point: return PointGeometry{val(origin)};
    case Shape::Axis: return AxisGeometry{val(origin),val(direction)};
    case Shape::Plane: return PlaneGeometry{val(origin),val(direction)};
    case Shape::Cylinder: return CylinderGeometry{val(origin),val(direction),radius,1};
    }
    return PointGeometry{};
}
Constraint ground(const char* id="reference") {
    Constraint c; c.id=std::string("fix-")+id; c.kind=ConstraintKind::Fix; c.first={id,{}}; return c;
}
struct Fixture {
    Model model;
    SolverOptions options;
    Eigen::Vector3d firstOrigin,secondOrigin,firstDirection,secondDirection;
};
Fixture fixture(Shape first,Shape second,double scale,Eigen::Vector3d a=Eigen::Vector3d::UnitZ(),
                Eigen::Vector3d b=Eigen::Vector3d::UnitZ()) {
    Fixture f;
    const Eigen::Quaterniond frame(Eigen::AngleAxisd(.57,Eigen::Vector3d(1,2,-1).normalized()));
    const Eigen::Quaterniond tilt(Eigen::AngleAxisd(.19,Eigen::Vector3d::UnitY()));
    const Eigen::Vector3d translation=scale*Eigen::Vector3d(7,-11,5);
    f.model.bodies={{"reference",{val(translation),val(frame)}},
                    {"moving",{val(translation+frame*(scale*Eigen::Vector3d(4,-2,3))),val(frame*tilt)}}};
    f.firstOrigin=scale*Eigen::Vector3d(1.2,-.8,.4);
    f.secondOrigin=scale*Eigen::Vector3d(-.7,.3,-1.1);
    f.firstDirection=a.normalized(); f.secondDirection=b.normalized();
    f.model.geometry={{"support","moving",geometry(first,f.firstOrigin,f.firstDirection,2*scale)},
                      {"support","reference",geometry(second,f.secondOrigin,f.secondDirection,3*scale)}};
    f.model.constraints={ground()};
    f.options.length_scale=scale;
    f.options.motion_length_scale=scale;
    f.options.solve_intent=SolveIntent{{"moving"},{"reference"},SolvePreferencePolicy::MoveFirstMinimizeReference};
    return f;
}
Constraint binary(ConstraintKind kind,bool swap) {
    Constraint c; c.id="relation"; c.kind=kind;
    c.first=swap ? GeometryRef{"reference","support"} : GeometryRef{"moving","support"};
    c.second=swap ? GeometryRef{"moving","support"} : GeometryRef{"reference","support"};
    return c;
}
Eigen::Vector3d world_origin(const Pose& p,const Eigen::Vector3d& origin) { return quat(p.rotation)*origin+vec(p.translation); }
Eigen::Vector3d world_direction(const Pose& p,const Eigen::Vector3d& direction) { return quat(p.rotation)*direction; }
Eigen::Matrix<double,6,6> subspace_projector(const ComponentDof& component) {
    Eigen::MatrixXd basis(6,static_cast<Eigen::Index>(component.null_space_basis.size()));
    for(Eigen::Index i=0;i<basis.cols();++i) {
        if(component.null_space_basis[static_cast<std::size_t>(i)].size()!=6) {
            ADD_FAILURE()<<"unexpected component tangent size";
            return Eigen::Matrix<double,6,6>::Zero();
        }
        basis.col(i)=Eigen::Map<const Eigen::Matrix<double,6,1>>(component.null_space_basis[static_cast<std::size_t>(i)].data());
    }
    if(!basis.cols()) return Eigen::Matrix<double,6,6>::Zero();
    // Component output de-normalizes the rank-equilibrated columns then
    // normalizes each separately. They span the physical kernel but need not
    // be mutually orthogonal. Reorthogonalize before comparing projectors.
    Eigen::ColPivHouseholderQR<Eigen::MatrixXd> qr(basis);
    EXPECT_EQ(qr.rank(),basis.cols());
    const Eigen::MatrixXd q=qr.householderQ()*Eigen::MatrixXd::Identity(6,basis.cols());
    return q*q.transpose();
}
void status_and_rank(const SolveResult& result,int rank) {
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic
        << "; iterations=" << result.iterations << "; normalized residual=" << result.normalized_residual;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged)
        << result.components[0].preference.total_optimality;
    EXPECT_EQ(result.components[0].jacobian_rank,static_cast<std::size_t>(rank));
    EXPECT_EQ(result.components[0].relative_dof,static_cast<std::size_t>(6-rank));
    // Compare subspace projector, never SVD basis ordering or signs.
    const auto projector=subspace_projector(result.components[0]);
    EXPECT_NEAR(projector.trace(),6-rank,1e-8);
    EXPECT_LT((projector*projector-projector).norm(),1e-8);
}
double line_distance(Eigen::Vector3d a,Eigen::Vector3d u,Eigen::Vector3d b,Eigen::Vector3d v) {
    const Eigen::Vector3d normal=u.cross(v),delta=a-b;
    return normal.norm()>1e-12 ? std::abs(delta.dot(normal))/normal.norm() : delta.cross(v).norm();
}
void coincidence(Shape first,Shape second,int rank,ConstraintKind kind=ConstraintKind::Coincident) {
    for(double scale : {.1,10.}) for(bool swap : {false,true}) {
        SCOPED_TRACE(scale);
        SCOPED_TRACE(swap);
        const bool linePlane=first==Shape::Axis && second==Shape::Plane;
        auto f=fixture(first,second,scale,linePlane ? Eigen::Vector3d::UnitX() : Eigen::Vector3d::UnitZ());
        auto c=binary(kind,swap); c.direction_relation=DirectionRelation::Same;
        f.model.constraints.push_back(c);
        const auto result=Solver{}.solve(f.model,f.options);
        status_and_rank(result,rank);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference");
        const auto a=world_origin(moving,f.firstOrigin),b=world_origin(reference,f.secondOrigin);
        const auto u=world_direction(moving,f.firstDirection),v=world_direction(reference,f.secondDirection);
        if(first==Shape::Point && second==Shape::Point) EXPECT_LE((a-b).norm(),f.options.length_tolerance);
        else if(first==Shape::Point && second==Shape::Axis) EXPECT_LE((a-b).cross(v).norm(),f.options.length_tolerance);
        else if(first==Shape::Point && second==Shape::Plane) EXPECT_LE(std::abs((a-b).dot(v)),f.options.length_tolerance);
        else if(linePlane) {
            EXPECT_LE(std::abs(u.dot(v)),f.options.angle_tolerance);
            EXPECT_LE(std::abs((a-b).dot(v)),f.options.length_tolerance);
        } else if(first==Shape::Plane) {
            EXPECT_LE((u-v).norm(),f.options.angle_tolerance);
            EXPECT_LE(std::abs((a-b).dot(v)),f.options.length_tolerance);
        } else {
            EXPECT_LE((u-v).norm(),f.options.angle_tolerance);
            EXPECT_LE((a-b).cross(v).norm(),f.options.length_tolerance);
        }
        EXPECT_GT((vec(moving.translation)-vec(f.model.bodies[1].initial_pose.translation)).norm()+
            Eigen::AngleAxisd(quat(moving.rotation)*quat(f.model.bodies[1].initial_pose.rotation).inverse()).angle(),1e-6);
    }
}
void offset(Shape first,Shape second,int rank) {
    const bool plane=first==Shape::Plane || second==Shape::Plane;
    for(double scale : {.1,10.}) for(bool swap : {false,true}) {
        SCOPED_TRACE(scale);
        SCOPED_TRACE(swap);
        const Eigen::Vector3d a=(first==Shape::Axis && second==Shape::Plane) ? Eigen::Vector3d::UnitX() : Eigen::Vector3d::UnitZ();
        const Eigen::Vector3d b=(first==Shape::Axis && second==Shape::Axis) ? Eigen::Vector3d::UnitX() : Eigen::Vector3d::UnitZ();
        auto f=fixture(first,second,scale,a,b);
        auto c=binary(ConstraintKind::Distance,swap);
        const double userValue=(plane ? -1.7 : 1.7)*scale;
        c.value=swap && plane ? -userValue : userValue;
        if(plane) c.distance_relation=DistanceRelation::SelectedPlaneNormal;
        c.direction_relation=DirectionRelation::Same;
        f.model.constraints.push_back(c);
        const auto result=Solver{}.solve(f.model,f.options);
        status_and_rank(result,rank);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference");
        const auto p=world_origin(moving,f.firstOrigin),q=world_origin(reference,f.secondOrigin);
        const auto u=world_direction(moving,f.firstDirection),v=world_direction(reference,f.secondDirection);
        double observed=0;
        if(first==Shape::Point && second==Shape::Point) observed=(p-q).norm();
        else if(first==Shape::Point && second==Shape::Axis) observed=(p-q).cross(v).norm();
        else if(first==Shape::Axis && second==Shape::Axis) observed=line_distance(p,u,q,v);
        else {
            observed=(p-q).dot(first==Shape::Plane ? u : v);
            if(first==Shape::Axis) EXPECT_LE(std::abs(u.dot(v)),f.options.angle_tolerance);
            if(first==Shape::Plane) EXPECT_LE((u-v).norm(),f.options.angle_tolerance);
        }
        EXPECT_NEAR(observed,userValue,f.options.length_tolerance);
        auto measured=f.model;
        measured.bodies[1].initial_pose=moving;
        measured.constraints[1].mode=ConstraintMode::Measured;
        const auto measurement=Solver{}.solve(measured,f.options);
        ASSERT_EQ(measurement.status,SolveStatus::Converged) << measurement.diagnostic;
        const auto unchanged=result_pose(measurement,"moving");
        EXPECT_LT((vec(unchanged.translation)-vec(moving.translation)).norm(),1e-12);
        EXPECT_LT(Eigen::AngleAxisd(quat(unchanged.rotation)*quat(moving.rotation).inverse()).angle(),1e-12);
        const auto free=std::find_if(measurement.components.begin(),measurement.components.end(),[](const auto& component){return std::find(component.body_ids.begin(),component.body_ids.end(),"moving")!=component.body_ids.end();});
        ASSERT_NE(free,measurement.components.end());
        EXPECT_EQ(free->jacobian_rank,0);
        // A measured relation does not connect solver components. An isolated
        // moving occurrence has six global gauge directions, not six relative
        // directions with respect to the separately grounded component.
        EXPECT_EQ(free->relative_dof+free->gauge_dof,6);
    }
}
void angle(Relation relation,Shape first,Shape second) {
    for(double scale : {.1,10.}) for(int branch : {1,-1}) {
        SCOPED_TRACE(scale);
        SCOPED_TRACE(branch);
        const Eigen::Vector3d a(.7,0,std::sqrt(.51)),b(.2,.7,std::sqrt(.47));
        auto f=fixture(first,second,scale,a,b);
        auto c=binary(relation==Relation::Parallel ? ConstraintKind::Parallel : ConstraintKind::Angle,false);
        c.direction_relation=relation==Relation::Parallel ? (branch>0 ? DirectionRelation::Same : DirectionRelation::Opposite) : DirectionRelation::Same;
        const Eigen::Vector3d initialA=quat(f.model.bodies[1].initial_pose.rotation)*a;
        const Eigen::Vector3d initialB=quat(f.model.bodies[0].initial_pose.rotation)*b;
        const Eigen::Vector3d selector=quat(f.model.bodies[0].initial_pose.rotation).inverse()*initialA.cross(initialB).normalized();
        if(relation==Relation::Directed) {
            const auto frame=quat(f.model.bodies[0].initial_pose.rotation);
            const Eigen::Quaterniond axisRotation=frame*Eigen::Quaterniond(Eigen::AngleAxisd(.13,Eigen::Vector3d::UnitX()));
            f.model.bodies.push_back({"axis-source",{{31,-8,17},val(axisRotation)}});
            f.model.geometry.push_back({"axis","axis-source",AxisGeometry{{1,3,-2},{0,0,1}}});
            f.model.constraints.push_back(ground("axis-source"));
            c.angle_reference_geometry=GeometryRef{"axis-source","axis"};
            c.reverse_angle_reference=branch<0;
            c.value=branch>0 ? 1.0 : 2*pi-1.0;
        } else if(relation!=Relation::Parallel) {
            c.spatial_angle_branch_direction=val(selector);
            c.value=relation==Relation::Perpendicular ? (branch>0 ? pi/2 : 3*pi/2) : (branch>0 ? .9 : 2*pi-.9);
        }
        f.model.constraints.push_back(c);
        const auto result=Solver{}.solve(f.model,f.options);
        status_and_rank(result,relation==Relation::Parallel ? 2 : 1);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference");
        const auto u=world_direction(moving,a),v=world_direction(reference,b);
        Eigen::Vector3d blocked;
        if(relation==Relation::Parallel) {
            EXPECT_LE((u-(branch>0 ? v : -v)).norm(),f.options.angle_tolerance);
        } else if(relation==Relation::Directed) {
            Eigen::Vector3d k=world_direction(result_pose(result,"axis-source"),Eigen::Vector3d::UnitZ());
            if(c.reverse_angle_reference) k=-k;
            const Eigen::Vector3d up=u-u.dot(k)*k,vp=v-v.dot(k)*k;
            ASSERT_GT(up.norm(),1e-5); ASSERT_GT(vp.norm(),1e-5);
            double observed=std::atan2(k.dot(up.cross(vp)),up.dot(vp));
            if(observed<0) observed+=2*pi;
            EXPECT_NEAR(observed,c.value,f.options.angle_tolerance);
            blocked=(k-u.dot(k)*u).normalized();
        } else {
            const double separation=std::atan2(u.cross(v).norm(),u.dot(v));
            EXPECT_NEAR(separation,relation==Relation::Perpendicular ? pi/2 : .9,f.options.angle_tolerance);
            const auto worldSelector=world_direction(reference,selector);
            EXPECT_GT(branch*worldSelector.dot(u.cross(v)),0);
            blocked=u.cross(v).normalized();
        }
        Eigen::Matrix<double,6,6> expected=Eigen::Matrix<double,6,6>::Zero();
        expected.topLeftCorner<3,3>().setIdentity();
        if(relation==Relation::Parallel) expected.bottomRightCorner<3,3>()=u*u.transpose();
        else expected.bottomRightCorner<3,3>()=Eigen::Matrix3d::Identity()-blocked*blocked.transpose();
        const auto observed=subspace_projector(result.components[0]);
        EXPECT_LT((observed-expected).norm(),1e-8);
        if(relation==Relation::Parallel || relation==Relation::Perpendicular) continue;
        auto measured=f.model; measured.bodies[1].initial_pose=moving; measured.constraints.back().mode=ConstraintMode::Measured;
        const auto output=Solver{}.solve(measured,f.options);
        ASSERT_EQ(output.status,SolveStatus::Converged) << output.diagnostic;
        const auto unchanged=result_pose(output,"moving");
        EXPECT_LT((vec(unchanged.translation)-vec(moving.translation)).norm(),1e-12);
        EXPECT_LT(Eigen::AngleAxisd(quat(unchanged.rotation)*quat(moving.rotation).inverse()).angle(),1e-12);
    }
}

TEST(AssemblyCapabilityMatrix,CoincidencePointPoint) { coincidence(Shape::Point,Shape::Point,3); }
TEST(AssemblyCapabilityMatrix,CoincidencePointAxis) { coincidence(Shape::Point,Shape::Axis,2); }
TEST(AssemblyCapabilityMatrix,CoincidencePointPlane) { coincidence(Shape::Point,Shape::Plane,1); }
TEST(AssemblyCapabilityMatrix,CoincidenceAxisAxis) { coincidence(Shape::Axis,Shape::Axis,4); }
TEST(AssemblyCapabilityMatrix,CoincidenceAxisPlane) { coincidence(Shape::Axis,Shape::Plane,2); }
TEST(AssemblyCapabilityMatrix,CoincidencePlanePlane) { coincidence(Shape::Plane,Shape::Plane,3); }
TEST(AssemblyCapabilityMatrix,CoincidenceCylinderAxis) { coincidence(Shape::Cylinder,Shape::Axis,4,ConstraintKind::Concentric); }
TEST(AssemblyCapabilityMatrix,CoincidenceCylinderCylinderAxis) { coincidence(Shape::Cylinder,Shape::Cylinder,4,ConstraintKind::Concentric); }
TEST(AssemblyCapabilityMatrix,OffsetPointPoint) { offset(Shape::Point,Shape::Point,1); }
TEST(AssemblyCapabilityMatrix,OffsetPointAxis) { offset(Shape::Point,Shape::Axis,1); }
TEST(AssemblyCapabilityMatrix,OffsetPointPlane) { offset(Shape::Point,Shape::Plane,1); }
TEST(AssemblyCapabilityMatrix,OffsetAxisAxis) { offset(Shape::Axis,Shape::Axis,1); }
TEST(AssemblyCapabilityMatrix,OffsetAxisPlane) { offset(Shape::Axis,Shape::Plane,2); }
TEST(AssemblyCapabilityMatrix,OffsetPlanePlane) { offset(Shape::Plane,Shape::Plane,3); }

void axis_plane_from_normal(ConstraintKind kind) {
    for(double scale : {.1,10.}) for(bool swap : {false,true}) {
        SCOPED_TRACE(scale);
        SCOPED_TRACE(swap);
        const Eigen::Quaterniond q(Eigen::AngleAxisd(.34,Eigen::Vector3d::UnitY()));
        Model model;
        model.bodies={{"reference",{val(scale*Eigen::Vector3d(9,2,-4)),val(q)}},
                      {"moving",{val(scale*Eigen::Vector3d(31,-7,13)),val(q)}}};
        const Eigen::Vector3d axisOrigin=scale*Eigen::Vector3d(2,-3,5),planeOrigin=scale*Eigen::Vector3d(1,2,-1);
        model.geometry={{"support","moving",AxisGeometry{val(axisOrigin),{0,0,1}}},
                        {"support","reference",PlaneGeometry{val(planeOrigin),{0,0,1}}}};
        auto c=binary(kind,swap);
        c.direction_relation=DirectionRelation::Unoriented;
        c.distance_relation=DistanceRelation::SelectedPlaneNormal;
        c.value=kind==ConstraintKind::Distance ? (swap ? -11. : 11.)*scale : 0.;
        model.constraints={ground(),c};
        SolverOptions options; options.length_scale=scale; options.motion_length_scale=scale;
        options.solve_intent=SolveIntent{{"moving"},{"reference"},SolvePreferencePolicy::MoveFirstMinimizeReference};
        if(kind==ConstraintKind::Distance) {
            auto measured=model; measured.constraints[1].mode=ConstraintMode::Measured;
            const auto result=Solver{}.solve(measured,options);
            ASSERT_EQ(result.status,SolveStatus::Converged);
            EXPECT_LT((vec(result_pose(result,"moving").translation)-vec(model.bodies[1].initial_pose.translation)).norm(),1e-12);
            EXPECT_LT(Eigen::AngleAxisd(quat(result_pose(result,"moving").rotation)*q.inverse()).angle(),1e-12);
        }
        const auto result=Solver{}.solve(model,options);
        status_and_rank(result,2);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference");
        const Eigen::Vector3d a=world_origin(moving,axisOrigin),p=world_origin(reference,planeOrigin);
        const Eigen::Vector3d u=world_direction(moving,Eigen::Vector3d::UnitZ()),n=q*Eigen::Vector3d::UnitZ();
        EXPECT_LE(std::abs(u.dot(n)),options.angle_tolerance);
        EXPECT_NEAR(n.dot(a-p),kind==ConstraintKind::Distance ? 11.*scale : 0.,options.length_tolerance);
        // The selected axis must turn by 90 degrees. The unconstrained clocking
        // preference can additionally turn the body, so its full SO(3) motion
        // is not required to equal the minimum support-direction turn.
        EXPECT_GE(Eigen::AngleAxisd(quat(moving.rotation)*q.inverse()).angle(),pi/2-1e-7);
        EXPECT_LT((vec(reference.translation)-vec(model.bodies[0].initial_pose.translation)).norm(),1e-12);
        EXPECT_LT(Eigen::AngleAxisd(quat(reference.rotation)*q.inverse()).angle(),1e-12);
    }
}
TEST(AssemblyCapabilityMatrix,OffsetAxisPlaneMeasuredRestoreFromNormalAlignmentScaledRotatedAndSwapped) { axis_plane_from_normal(ConstraintKind::Distance); }
TEST(AssemblyCapabilityMatrix,CoincidenceAxisPlaneEscapesNormalAlignmentScaledRotatedAndSwapped) { axis_plane_from_normal(ConstraintKind::Coincident); }

TEST(AssemblyCapabilityMatrix,AngleFreeAxisAxis) { angle(Relation::Free,Shape::Axis,Shape::Axis); }
TEST(AssemblyCapabilityMatrix,AngleDirectedAxisAxis) { angle(Relation::Directed,Shape::Axis,Shape::Axis); }
TEST(AssemblyCapabilityMatrix,AngleParallelAxisAxis) { angle(Relation::Parallel,Shape::Axis,Shape::Axis); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularAxisAxis) { angle(Relation::Perpendicular,Shape::Axis,Shape::Axis); }
TEST(AssemblyCapabilityMatrix,AngleFreeAxisPlane) { angle(Relation::Free,Shape::Axis,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleDirectedAxisPlane) { angle(Relation::Directed,Shape::Axis,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleParallelAxisPlane) { angle(Relation::Parallel,Shape::Axis,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularAxisPlane) { angle(Relation::Perpendicular,Shape::Axis,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleFreeAxisCylinder) { angle(Relation::Free,Shape::Axis,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleDirectedAxisCylinder) { angle(Relation::Directed,Shape::Axis,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleParallelAxisCylinder) { angle(Relation::Parallel,Shape::Axis,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularAxisCylinder) { angle(Relation::Perpendicular,Shape::Axis,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleFreePlanePlane) { angle(Relation::Free,Shape::Plane,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleDirectedPlanePlane) { angle(Relation::Directed,Shape::Plane,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleParallelPlanePlane) { angle(Relation::Parallel,Shape::Plane,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularPlanePlane) { angle(Relation::Perpendicular,Shape::Plane,Shape::Plane); }
TEST(AssemblyCapabilityMatrix,AngleFreePlaneCylinder) { angle(Relation::Free,Shape::Plane,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleDirectedPlaneCylinder) { angle(Relation::Directed,Shape::Plane,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleParallelPlaneCylinder) { angle(Relation::Parallel,Shape::Plane,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularPlaneCylinder) { angle(Relation::Perpendicular,Shape::Plane,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleFreeCylinderCylinder) { angle(Relation::Free,Shape::Cylinder,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleDirectedCylinderCylinder) { angle(Relation::Directed,Shape::Cylinder,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AngleParallelCylinderCylinder) { angle(Relation::Parallel,Shape::Cylinder,Shape::Cylinder); }
TEST(AssemblyCapabilityMatrix,AnglePerpendicularCylinderCylinder) { angle(Relation::Perpendicular,Shape::Cylinder,Shape::Cylinder); }

void captured_fix(Pose target) {
    Model model; model.bodies={{"moving",{{-30,20,10},{0,std::sin(.3),0,std::cos(.3)}}}};
    auto c=ground("moving"); c.fixed_pose=target; model.constraints={c};
    const auto result=Solver{}.solve(model);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    const auto pose=result_pose(result,"moving");
    EXPECT_LT((vec(pose.translation)-vec(target.translation)).norm(),1e-12);
    EXPECT_LT(Eigen::AngleAxisd(quat(pose.rotation)*quat(target.rotation).inverse()).angle(),1e-12);
    ASSERT_EQ(result.components.size(),1); EXPECT_EQ(result.components[0].relative_dof,0);
}
TEST(AssemblyCapabilityMatrix,FixSpaceCapturedPose) { captured_fix({{12,-23,8},{std::sin(.17),0,0,std::cos(.17)}}); }
TEST(AssemblyCapabilityMatrix,FixRelativeExplicitCapturedUpdate) {
    // Domain owns the explicit-confirm transition. Native independently proves
    // the newly captured baseline is consumed, not substituted by nominal.
    captured_fix({{17,-11,3},{0,0,std::sin(.24),std::cos(.24)}});
    captured_fix({{19,-7,5},{0,0,std::sin(.36),std::cos(.36)}});
}

TEST(AssemblyCapabilityMatrix,DirectedAxisSourceSurvivesSelectionExchangeAndAxisReversal) {
    for(bool swap : {false,true}) for(bool reverse : {false,true}) {
        SCOPED_TRACE(swap);
        SCOPED_TRACE(reverse);
        auto f=fixture(Shape::Axis,Shape::Plane,1,Eigen::Vector3d(.7,0,std::sqrt(.51)),Eigen::Vector3d(.2,.7,std::sqrt(.47)));
        const auto frame=quat(f.model.bodies[0].initial_pose.rotation);
        f.model.bodies.push_back({"axis-source",{{31,-8,17},val(frame*Eigen::Quaterniond(Eigen::AngleAxisd(.13,Eigen::Vector3d::UnitX())))}});
        f.model.geometry.push_back({"axis","axis-source",AxisGeometry{{2,-4,8},{0,0,1}}});
        f.model.constraints.push_back(ground("axis-source"));
        auto c=binary(ConstraintKind::Angle,swap);
        c.angle_reference_geometry=GeometryRef{"axis-source","axis"}; c.reverse_angle_reference=reverse;
        c.value=swap!=reverse ? 2*pi-1 : 1;
        f.model.constraints.push_back(c);
        const auto result=Solver{}.solve(f.model,f.options);
        status_and_rank(result,1);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference"),source=result_pose(result,"axis-source");
        const auto u=world_direction(moving,f.firstDirection),v=world_direction(reference,f.secondDirection),k=world_direction(source,Eigen::Vector3d::UnitZ());
        const Eigen::Vector3d up=u-u.dot(k)*k,vp=v-v.dot(k)*k;
        double observed=std::atan2(k.dot(up.cross(vp)),up.dot(vp)); if(observed<0) observed+=2*pi;
        EXPECT_NEAR(observed,1,f.options.angle_tolerance);
        EXPECT_LT((vec(source.translation)-vec(f.model.bodies[2].initial_pose.translation)).norm(),1e-12);
        EXPECT_LT(Eigen::AngleAxisd(quat(source.rotation)*quat(f.model.bodies[2].initial_pose.rotation).inverse()).angle(),1e-12);
    }
}

TEST(AssemblyCapabilityMatrix,DirectedUnfixedThirdAxisParticipatesInVariablesAndJacobian) {
    auto f=fixture(Shape::Axis,Shape::Plane,1,Eigen::Vector3d(.7,0,std::sqrt(.51)),Eigen::Vector3d(.2,.7,std::sqrt(.47)));
    const auto frame=quat(f.model.bodies[0].initial_pose.rotation);
    f.model.bodies.push_back({"axis-source",{{31,-8,17},val(frame*Eigen::Quaterniond(Eigen::AngleAxisd(.13,Eigen::Vector3d::UnitX())))}});
    f.model.geometry.push_back({"axis","axis-source",AxisGeometry{{2,-4,8},{0,0,1}}});
    auto c=binary(ConstraintKind::Angle,false); c.angle_reference_geometry=GeometryRef{"axis-source","axis"}; c.value=.83;
    f.model.constraints.push_back(c);
    const auto result=Solver{}.solve(f.model,f.options);
    ASSERT_EQ(result.status,SolveStatus::Converged) << result.diagnostic;
    ASSERT_EQ(result.components.size(),1);
    EXPECT_EQ(result.components[0].tangent_variable_count,12);
    EXPECT_EQ(result.components[0].jacobian_rank,1);
    EXPECT_EQ(result.components[0].relative_dof,11);
    EXPECT_EQ(result.components[0].preference.status,PreferenceStatus::Converged);
    const auto u=world_direction(result_pose(result,"moving"),f.firstDirection),v=world_direction(result_pose(result,"reference"),f.secondDirection);
    const auto k=world_direction(result_pose(result,"axis-source"),Eigen::Vector3d::UnitZ());
    const Eigen::Vector3d up=u-u.dot(k)*k,vp=v-v.dot(k)*k;
    double observed=std::atan2(k.dot(up.cross(vp)),up.dot(vp)); if(observed<0) observed+=2*pi;
    EXPECT_NEAR(observed,.83,f.options.angle_tolerance);
}

TEST(AssemblyCapabilityMatrix,FreeAngleZeroPiAndFullTurnHaveAlignmentRankNotScalarRank) {
    for(double target : {0.,pi,2*pi}) {
        auto f=fixture(Shape::Axis,Shape::Axis,1);
        auto c=binary(ConstraintKind::Angle,false); c.value=target; c.direction_relation=DirectionRelation::Same;
        f.model.constraints.push_back(c);
        const auto result=Solver{}.solve(f.model,f.options);
        status_and_rank(result,2);
        ASSERT_EQ(result.status,SolveStatus::Converged);
        const auto a=world_direction(result_pose(result,"moving"),f.firstDirection),b=world_direction(result_pose(result,"reference"),f.secondDirection);
        EXPECT_LE((a-(target==pi ? -b : b)).norm(),f.options.angle_tolerance);
    }
}

TEST(AssemblyCapabilityMatrix,DuplicateAngleIsRedundantNotAnExtraRemovedDof) {
    auto f=fixture(Shape::Axis,Shape::Plane,1,Eigen::Vector3d(.7,0,std::sqrt(.51)),Eigen::Vector3d(.2,.7,std::sqrt(.47)));
    auto c=binary(ConstraintKind::Angle,false); c.value=.9;
    f.model.constraints.push_back(c); c.id="duplicate"; f.model.constraints.push_back(c);
    const auto result=Solver{}.solve(f.model,f.options);
    status_and_rank(result,1);
    ASSERT_EQ(result.status,SolveStatus::Converged);
    EXPECT_EQ(result.redundant_constraint_ids.size(),1);
    const auto a=world_direction(result_pose(result,"moving"),f.firstDirection),b=world_direction(result_pose(result,"reference"),f.secondDirection);
    EXPECT_NEAR(std::atan2(a.cross(b).norm(),a.dot(b)),.9,f.options.angle_tolerance);
}

TEST(AssemblyCapabilityMatrix,FixedInfeasiblePointRelationDoesNotFakeGeometrySuccess) {
    auto f=fixture(Shape::Point,Shape::Point,1);
    f.model.constraints.push_back(ground("moving"));
    f.model.constraints.push_back(binary(ConstraintKind::Coincident,false));
    const auto result=Solver{}.solve(f.model,f.options);
    EXPECT_NE(result.status,SolveStatus::Converged);
    const auto moving=result_pose(result,"moving"),reference=result_pose(result,"reference");
    EXPECT_GT((world_origin(moving,f.firstOrigin)-world_origin(reference,f.secondOrigin)).norm(),1);
    EXPECT_LT((vec(moving.translation)-vec(f.model.bodies[1].initial_pose.translation)).norm(),1e-12);
}
}  // namespace
}  // namespace occccad::assembly

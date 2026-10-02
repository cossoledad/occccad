#include <occccad/assembly/solver.hpp>

#include <gtest/gtest.h>

#include <cmath>
#include <fstream>
#include <cstdlib>
#include <chrono>
using namespace occccad::assembly;
namespace {
Pose body_pose(const SolveResult& r, const std::string& id) {
    for (const auto& b : r.bodies)
        if (b.id == id)
            return b.pose;
    throw std::runtime_error("missing body");
}
SolverOptions drag(const std::string& body, Vec3 target) {
    SolverOptions o;
    DragTarget d;
    d.body_id = body;
    d.target_pose.translation = target;
    d.target_sequence = 7;
    o.drag_target = d;
    o.affected_body_ids = {body};
    return o;
}
void qualified(const SolveResult& r) {
    ASSERT_EQ(r.status, SolveStatus::Converged) << r.diagnostic;
    ASSERT_TRUE(r.interaction);
    EXPECT_TRUE(r.interaction->hard_feasible);
    EXPECT_TRUE(r.interaction->eligible_for_commit) << r.interaction->target_optimality;
}
TEST(AssemblyInteraction, EccentricTranslationPreservesIntent) {
    for(double x:{10.,100.,500.})for(double originY:{0.,20.,-100.})for(double scale:{.001,1.,1000.}) {
    SCOPED_TRACE(::testing::Message()<<"x="<<x<<" originY="<<originY<<" scale="<<scale);
    Model m;m.bodies={{"a",{{0,originY,0},{}}}};
    auto options=drag("a",{x,originY,0});
    options.motion_length_scale=scale;
    options.drag_target->local_grab_point={0,50-originY,0};
    options.drag_target->translation_components={true,false,false};
    options.drag_target->hold_translation_components={false,true,true};
    options.drag_target->hold_rotation_components={true,true,true};
    const auto r=Solver{}.solve(m,options);
    if(r.interaction&&!r.interaction->eligible_for_commit)std::cout<<"INTENT_FAILURE stage="<<r.interaction->termination_stage<<" reason="<<r.interaction->termination_reason<<" hold="<<r.interaction->hold_optimality<<" total="<<r.components[0].preference.total_optimality<<" ref="<<r.components[0].preference.reference_optimality<<" preference="<<static_cast<int>(r.components[0].preference.status)<<"\n";
    qualified(r);
    const auto pose=body_pose(r,"a");
    EXPECT_NEAR(pose.translation.x,x,1e-7);
    EXPECT_NEAR(pose.translation.y,originY,1e-7);
    EXPECT_NEAR(pose.rotation.x,0,1e-8);EXPECT_NEAR(pose.rotation.y,0,1e-8);EXPECT_NEAR(pose.rotation.z,0,1e-8);
    }
}
TEST(AssemblyInteraction, LegacyLeverArmTerminationEvidence) {
    for(double arm:{50.,500.})for(double x:{10.,100.,500.}) {
        Model m;m.bodies={{"a",{}}};auto options=drag("a",{x,0,0});
        options.drag_target->local_grab_point={0,arm,0};options.drag_target->translation_components={true,false,false};
        const auto r=Solver{}.solve(m,options);ASSERT_TRUE(r.interaction);
        EXPECT_TRUE(r.interaction->hard_feasible);EXPECT_FALSE(r.interaction->termination_reason.empty());
        std::cout<<"LEGACY_LEVER arm="<<arm<<" x="<<x<<" stage="<<r.interaction->termination_stage<<" reason="<<r.interaction->termination_reason<<" qualified="<<r.interaction->eligible_for_commit<<" target="<<r.interaction->target_error<<" iterations="<<r.interaction->iterations<<"\n";
    }
}
// Cumulative trace: nominal is fixed; only accepted checkpoint is a guess.
// Coarse/dense sampling, reverse and pauses exercise production objectives.
TEST(AssemblyInteraction, CumulativeEccentricTranslationTrace) {
    std::ofstream trace;if(const char* path=std::getenv("OCCCCAD_DRAG_TRACE_OUTPUT"))trace.open(path);
    for(bool plane:{false,true})for(double arm:{1.,50.,500.})for(int density:{1,4}) {
        Model m;Pose baseline{{17,-23,11},{-std::sin(.3)*std::sin(.2),std::cos(.3)*std::sin(.2),std::sin(.3)*std::cos(.2),std::cos(.3)*std::cos(.2)}};
        m.bodies={{"a",baseline},{"unrelated",{{900,800,700},{}}}};
        if(plane){m.bodies.push_back({"ground",baseline});m.geometry={{"p","a",PlaneGeometry{}},{"p","ground",PlaneGeometry{}}};
            Constraint f;f.id="ground";f.kind=ConstraintKind::Fix;f.first={"ground",{}};
            Constraint c;c.id="plane";c.kind=ConstraintKind::Coincident;c.first={"a","p"};c.second=GeometryRef{"ground","p"};c.direction_relation=DirectionRelation::Same;m.constraints={f,c};}
        auto options=drag("a",baseline.translation);options.drag_target->local_grab_point={0,arm,3};
        options.drag_target->frame_rotation=baseline.rotation;options.drag_target->target_pose.rotation=baseline.rotation;
        options.drag_target->translation_components={true,false,false};options.drag_target->hold_translation_components={false,true,true};options.drag_target->hold_rotation_components={true,true,true};
        for(int sample=0;sample<=60*density;++sample) {
            const double u=sample<=30*density?double(sample)/density:60.-double(sample)/density;
            options.drag_target->target_pose.translation={17+u*std::cos(.6)*std::cos(.4),-23+u*std::sin(.6)*std::cos(.4),11-u*std::sin(.4)};options.drag_target->target_sequence=sample+1;
            const auto started=std::chrono::steady_clock::now();auto r=Solver{}.solve(m,options);qualified(r);const auto p=body_pose(r,"a");
            if(trace&&r.interaction)trace<<"{\"fixture\":\"eccentric-translation-v1\",\"plane\":"<<plane<<",\"arm\":"<<arm<<",\"density\":"<<density<<",\"sequence\":"<<sample+1<<",\"cumulative\":"<<u<<",\"accepted\":["<<p.translation.x<<","<<p.translation.y<<","<<p.translation.z<<"],\"rotation\":["<<p.rotation.x<<","<<p.rotation.y<<","<<p.rotation.z<<","<<p.rotation.w<<"],\"hardError\":"<<r.interaction->hard_error<<",\"targetError\":"<<r.interaction->target_error<<",\"stage\":\""<<r.interaction->termination_stage<<"\",\"reason\":\""<<r.interaction->termination_reason<<"\",\"iterations\":"<<r.interaction->iterations<<",\"restorations\":"<<r.interaction->restorations<<",\"elapsedUs\":"<<std::chrono::duration_cast<std::chrono::microseconds>(std::chrono::steady_clock::now()-started).count()<<"}\n";
            EXPECT_NEAR(p.translation.x,options.drag_target->target_pose.translation.x,1e-7);EXPECT_NEAR(p.translation.y,options.drag_target->target_pose.translation.y,1e-7);EXPECT_NEAR(p.translation.z,options.drag_target->target_pose.translation.z,1e-7);
            EXPECT_NEAR(p.rotation.x,baseline.rotation.x,1e-8);EXPECT_NEAR(p.rotation.y,baseline.rotation.y,1e-8);EXPECT_NEAR(p.rotation.z,baseline.rotation.z,1e-8);EXPECT_NEAR(p.rotation.w,baseline.rotation.w,1e-8);
            EXPECT_EQ(body_pose(r,"unrelated").translation.x,900);EXPECT_EQ(m.bodies[0].initial_pose.translation.x,17);
            EXPECT_EQ(r.interaction->termination_reason,"LOCAL_OPTIMUM");EXPECT_LT(r.interaction->hard_error,1e-6);
            m.bodies[0].initial_guess=p;
        }
    }
}
TEST(AssemblyInteraction, TranslationIntentPermitsNecessaryRevoluteMotion) {
    Model m;Pose initial{{},{0,0,std::sin(.2),std::cos(.2)}};
    m.bodies={{"a",initial},{"ground",{}}};m.geometry={{"point","a",PointGeometry{}},{"point","ground",PointGeometry{}},{"axis","a",AxisGeometry{}},{"axis","ground",AxisGeometry{}}};
    Constraint f;f.id="fix";f.kind=ConstraintKind::Fix;f.first={"ground",{}};
    Constraint point;point.id="ball";point.kind=ConstraintKind::Coincident;point.first={"a","point"};point.second=GeometryRef{"ground","point"};
    Constraint axis;axis.id="axis";axis.kind=ConstraintKind::Concentric;axis.first={"a","axis"};axis.second=GeometryRef{"ground","axis"};m.constraints={f,point,axis};
    auto options=drag("a",{});options.drag_target->local_grab_point={50,0,0};options.drag_target->target_pose.rotation=initial.rotation;
    options.drag_target->translation_components={true,false,false};options.drag_target->hold_translation_components={false,true,true};options.drag_target->hold_rotation_components={true,true,true};
    for(int i=1;i<=40;++i){const double u=-4.*std::sin(i*3.141592653589793/40);options.drag_target->target_pose.translation={u,0,0};options.drag_target->target_sequence=i;
        auto r=Solver{}.solve(m,options);qualified(r);auto p=body_pose(r,"a");
        const double grabX=50*(1-2*(p.rotation.y*p.rotation.y+p.rotation.z*p.rotation.z));EXPECT_NEAR(grabX,50*std::cos(.4)+u,1e-6);EXPECT_NEAR(p.translation.x,0,1e-7);EXPECT_NEAR(p.translation.y,0,1e-7);EXPECT_NEAR(p.translation.z,0,1e-7);
        EXPECT_EQ(r.components[0].relative_dof,1);m.bodies[0].initial_guess=p;
    }
}
TEST(AssemblyInteraction, EccentricRotationTraceAndBudgetRecovery) {
    for(bool group:{false,true}) {
    Model m;m.bodies={{"a",{}}};const double originX=group?10.:0.;
    if(group){m.bodies.push_back({"b",{{10,0,0},{}}});Constraint rigid;rigid.id="captured-group";rigid.kind=ConstraintKind::Rigid;rigid.first={"a",{}};rigid.second=GeometryRef{"b",{}};rigid.fixed_pose=Pose{{-10,0,0},{}};m.constraints={rigid};}
    auto options=drag(group?"b":"a",{originX,0,0});
    options.drag_target->local_grab_point={0,50,0};options.drag_target->rotation_components={false,false,true};options.drag_target->hold_rotation_components={true,true,false};
    for(int i=0;i<=100;++i){double angle=i*.07;
        options.drag_target->target_sequence=i+1;options.drag_target->target_pose.rotation={0,0,std::sin(angle/2),std::cos(angle/2)};
        options.drag_target->target_pose.translation={originX+50*std::sin(angle),50*(1-std::cos(angle)),0};
        if(i==50){auto limited=options;limited.max_preference_iterations=0;auto budget=Solver{}.solve(m,limited);ASSERT_TRUE(budget.interaction);EXPECT_EQ(budget.interaction->status,InteractionStatus::Budget);EXPECT_FALSE(budget.interaction->eligible_for_commit);EXPECT_EQ(budget.interaction->termination_reason,"ITERATION_LIMIT");}
        auto r=Solver{}.solve(m,options);qualified(r);auto p=body_pose(r,group?"b":"a");
        EXPECT_NEAR(p.translation.x,originX+50*std::sin(angle),1e-7);EXPECT_NEAR(p.translation.y,50*(1-std::cos(angle)),1e-7);
        EXPECT_NEAR(std::abs(p.rotation.z*std::sin(angle/2)+p.rotation.w*std::cos(angle/2)),1,1e-8);
        // Independent transformed grab remains at the requested pivot.
        EXPECT_NEAR(p.translation.x-100*p.rotation.z*p.rotation.w,originX,1e-7);EXPECT_NEAR(p.translation.y+50*(1-2*p.rotation.z*p.rotation.z),50,1e-7);
        if(group){const auto root=body_pose(r,"a");EXPECT_NEAR(root.translation.x,p.translation.x-10*(1-2*p.rotation.z*p.rotation.z),1e-7);EXPECT_NEAR(root.translation.y,p.translation.y-20*p.rotation.z*p.rotation.w,1e-7);EXPECT_NEAR(std::abs(root.rotation.z*p.rotation.z+root.rotation.w*p.rotation.w),1,1e-8);EXPECT_EQ(m.bodies[1].initial_pose.translation.x,10);}
        for(auto& body:m.bodies)body.initial_guess=body_pose(r,body.id);
    }
    }
}
TEST(AssemblyInteraction, FreeBodyAndUnrelatedComponent) {
    Model m;
    m.bodies = {{"a", {}}, {"other", {{33, 0, 0}, {}}}};
    auto r = Solver{}.solve(m, drag("a", {8, 4, 3}));
    qualified(r);
    EXPECT_NEAR(body_pose(r, "a").translation.x, 8, 1e-7);
    EXPECT_NEAR(body_pose(r, "a").translation.y, 4, 1e-7);
    EXPECT_EQ(body_pose(r, "other").translation.x, 33);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Reached);
}
TEST(AssemblyInteraction, PhysicalFixReturnsRestrictedFeasibleNotConflict) {
    Model m;
    m.bodies = {{"a", {{2, 3, 4}, {}}}};
    Constraint c;
    c.id = "fixed";
    c.kind = ConstraintKind::Fix;
    c.first = {"a", {}};
    m.constraints = {c};
    auto r = Solver{}.solve(m, drag("a", {8, 4, 3}));
    qualified(r);
    EXPECT_EQ(body_pose(r, "a").translation.x, 2);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
    EXPECT_GT(r.interaction->target_error, 1);
    EXPECT_EQ(r.components[0].jacobian_rank, 0);
}
TEST(AssemblyInteraction, PlaneManifoldKeepsHardConstraint) {
    Model m;
    m.bodies = {{"a", {}}, {"ground", {}}};
    m.geometry = {{"p", "a", PlaneGeometry{}}, {"p", "ground", PlaneGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "plane";
    c.kind = ConstraintKind::Coincident;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.direction_relation = DirectionRelation::Same;
    m.constraints = {f, c};
    auto r = Solver{}.solve(m, drag("a", {5, 7, 12}));
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.x, 5, 1e-7);
    EXPECT_NEAR(p.translation.y, 7, 1e-7);
    EXPECT_NEAR(p.translation.z, 0, 1e-7);
    EXPECT_NEAR(p.rotation.x, 0, 1e-8);
    EXPECT_NEAR(p.rotation.y, 0, 1e-8);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
    EXPECT_EQ(r.components[0].relative_dof, 3);
}
TEST(AssemblyInteraction, DiagonalTargetRetainsXYAndBlocksZ) {
    Model m;
    m.bodies={{"moving",{}},{"ground",{}}};
    m.geometry={{"plane","moving",PlaneGeometry{}},{"plane","ground",PlaneGeometry{}}};
    Constraint fix;fix.id="space";fix.kind=ConstraintKind::Fix;fix.first={"ground",{}};
    Constraint plane;plane.id="plane";plane.kind=ConstraintKind::Coincident;plane.first={"moving","plane"};plane.second=GeometryRef{"ground","plane"};plane.direction_relation=DirectionRelation::Same;
    m.constraints={fix,plane};
    const auto result=Solver{}.solve(m,drag("moving",{1,1,1}));
    qualified(result);const auto pose=body_pose(result,"moving");
    EXPECT_NEAR(pose.translation.x,1,1e-7);EXPECT_NEAR(pose.translation.y,1,1e-7);EXPECT_NEAR(pose.translation.z,0,1e-7);
    EXPECT_NEAR(pose.rotation.x,0,1e-8);EXPECT_NEAR(pose.rotation.y,0,1e-8);
    EXPECT_EQ(result.interaction->status,InteractionStatus::Constrained);
    EXPECT_EQ(result.components[0].relative_dof,3);EXPECT_EQ(result.components[0].gauge_dof,0);
}
TEST(AssemblyInteraction, UngroundedRigidGroupGaugeNotAnchored) {
    Model m;
    m.bodies = {{"a", {}}, {"b", {{3, 0, 0}, {}}}};
    Constraint c;
    c.id = "rigid";
    c.kind = ConstraintKind::Rigid;
    c.first = {"a", {}};
    c.second = GeometryRef{"b", {}};
    c.fixed_pose = Pose{{-3, 0, 0}, {}};
    m.constraints = {c};
    auto o = drag("a", {5, 6, 0});
    o.solve_intent = SolveIntent{{"a"}, {"b"}, SolvePreferencePolicy::MoveFirstMinimizeReference};
    auto r = Solver{}.solve(m, o);
    qualified(r);
    auto a = body_pose(r, "a");
    auto b = body_pose(r, "b");
    EXPECT_NEAR(a.translation.x, 5, 1e-7);
    EXPECT_NEAR(a.translation.y, 6, 1e-7);
    double dx = a.translation.x - b.translation.x, dy = a.translation.y - b.translation.y,
           dz = a.translation.z - b.translation.z;
    EXPECT_NEAR(std::sqrt(dx * dx + dy * dy + dz * dz), 3, 1e-7);
    EXPECT_EQ(r.components[0].gauge_dof, 6);
}
TEST(AssemblyInteraction, AxisMaskDoesNotFixUncontrolledComponents) {
    Model m;
    m.bodies = {{"a", {{1, 2, 3}, {}}}};
    auto o = drag("a", {9, 99, 99});
    o.drag_target->translation_components = {true, false, false};
    auto r = Solver{}.solve(m, o);
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.x, 9, 1e-7);
    EXPECT_NEAR(p.translation.y, 2, 1e-7);
    EXPECT_NEAR(p.translation.z, 3, 1e-7);
}
TEST(AssemblyInteraction, CancelledAndBudgetFramesCannotCommit) {
    Model m;
    m.bodies = {{"a", {}}};
    auto o = drag("a", {9, 0, 0});
    o.should_cancel = [] { return true; };
    auto r = Solver{}.solve(m, o);
    ASSERT_TRUE(r.interaction);
    EXPECT_FALSE(r.interaction->eligible_for_commit);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Cancelled);
    o.should_cancel = {};
    o.max_preference_iterations = 0;
    r = Solver{}.solve(m, o);
    ASSERT_TRUE(r.interaction);
    EXPECT_FALSE(r.interaction->eligible_for_commit);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Budget);
}
TEST(AssemblyInteraction, RotatingGrabPointLongArmAndQuaternionSign) {
    Model m;
    m.bodies = {{"a", {}}};
    auto o = drag("a", {20, 30, 0});
    o.drag_target->local_grab_point = {100, 0, 0};
    o.drag_target->target_pose.rotation = {0, 0, std::sin(0.35), std::cos(0.35)};
    o.drag_target->rotation_components = {false, false, true};
    auto r = Solver{}.solve(m, o);
    qualified(r);
    auto p = body_pose(r, "a");
    const double angle = 2 * std::atan2(p.rotation.z, p.rotation.w);
    EXPECT_NEAR(angle, 0.7, 1e-7);
    EXPECT_NEAR(p.translation.x + 100 * std::cos(angle), 20 + 100 * std::cos(0.7), 1e-7);
    EXPECT_NEAR(p.translation.y + 100 * std::sin(angle), 30 + 100 * std::sin(0.7), 1e-7);
    o.drag_target->target_pose.rotation = {0, 0, -std::sin(0.35), -std::cos(0.35)};
    m.bodies[0].initial_guess = p;
    auto same = Solver{}.solve(m, o);
    qualified(same);
    EXPECT_NEAR(body_pose(same, "a").translation.x, p.translation.x, 1e-7);
}
TEST(AssemblyInteraction, SphericalFiniteMotionRetractionAndFrozenNominal) {
    Model m;
    m.bodies = {{"a", {{10, 0, 0}, {}}}, {"ground", {}}};
    m.geometry = {{"p", "a", PointGeometry{}}, {"p", "ground", PointGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "radius";
    c.kind = ConstraintKind::Distance;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.value = 10;
    m.constraints = {f, c};
    for (double a : {0.2, 0.6, 1.2, 1.6, 1.2, 0.6, 0.2}) {
        auto o = drag("a", {13 * std::cos(a), 13 * std::sin(a), 0});
        auto r = Solver{}.solve(m, o);
        qualified(r);
        auto p = body_pose(r, "a");
        EXPECT_NEAR(
            std::sqrt(p.translation.x * p.translation.x + p.translation.y * p.translation.y +
                      p.translation.z * p.translation.z),
            10, 1e-7);
        EXPECT_NEAR(p.translation.x, 10 * std::cos(a), 1e-6);
        EXPECT_NEAR(p.translation.y, 10 * std::sin(a), 1e-6);
        EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
        m.bodies[0].initial_guess = p;
        EXPECT_EQ(m.bodies[0].initial_pose.translation.x, 10);
    }
}
TEST(AssemblyInteraction, ReferenceFrameAxisAndScaleAreExplicit) {
    Model m;
    m.bodies = {{"a", {{1, 2, 3}, {}}}};
    auto o = drag("a", {99, 9, 99});
    o.drag_target->translation_components = {true, false, false};
    o.drag_target->frame_rotation = {0, 0, std::sin(0.25 * 3.141592653589793),
                                     std::cos(0.25 * 3.141592653589793)};
    o.motion_length_scale = 1000;
    auto r = Solver{}.solve(m, o);
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.x, 1, 1e-6);
    EXPECT_NEAR(p.translation.y, 9, 1e-6);
    EXPECT_NEAR(p.translation.z, 3, 1e-6);
}
TEST(AssemblyInteraction, StationaryFarPoleIsNotCertifiedAsNearestFeasible) {
    Model m;
    m.bodies = {{"a", {{10, 0, 0}, {}}}, {"ground", {}}};
    m.geometry = {{"p", "a", PointGeometry{}}, {"p", "ground", PointGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "radius";
    c.kind = ConstraintKind::Distance;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.value = 10;
    m.constraints = {f, c};
    auto r = Solver{}.solve(m, drag("a", {-13, 0, 0}));
    qualified(r);
    EXPECT_NEAR(body_pose(r, "a").translation.x, -10, 1e-6);
    EXPECT_NEAR(r.interaction->target_error, 3, 1e-6);
}
TEST(AssemblyInteraction, CylindricalAndPrismaticFiniteMotion) {
    for (bool prismatic : {false, true}) {
        Model m;
        m.bodies = {{"a", {}}, {"ground", {}}};
        m.geometry = {{"z", "a", AxisGeometry{}},
                      {"z", "ground", AxisGeometry{}},
                      {"x", "a", AxisGeometry{{}, {1, 0, 0}}},
                      {"x", "ground", AxisGeometry{{}, {1, 0, 0}}}};
        Constraint f;
        f.id = "fix";
        f.kind = ConstraintKind::Fix;
        f.first = {"ground", {}};
        Constraint c;
        c.id = "cylinder";
        c.kind = ConstraintKind::Concentric;
        c.first = {"a", "z"};
        c.second = GeometryRef{"ground", "z"};
        c.direction_relation = DirectionRelation::Same;
        m.constraints = {f, c};
        if (prismatic) {
            Constraint align;
            align.id = "angle";
            align.kind = ConstraintKind::Parallel;
            align.first = {"a", "x"};
            align.second = GeometryRef{"ground", "x"};
            align.direction_relation = DirectionRelation::Same;
            m.constraints.push_back(align);
        }
        for (double t : {0.2, 0.7, 1.4, 0.7, 0.2}) {
            auto o = drag("a", {7, 8, 20 * t});
            o.drag_target->target_pose.rotation = {0, 0, std::sin(t / 2), std::cos(t / 2)};
            o.drag_target->rotation_components = {false, false, true};
            auto r = Solver{}.solve(m, o);
            qualified(r);
            auto p = body_pose(r, "a");
            EXPECT_NEAR(p.translation.x, 0, 1e-7);
            EXPECT_NEAR(p.translation.y, 0, 1e-7);
            EXPECT_NEAR(p.translation.z, 20 * t, 1e-7);
            EXPECT_NEAR(p.rotation.x, 0, 1e-8);
            EXPECT_NEAR(p.rotation.y, 0, 1e-8);
            EXPECT_NEAR(2 * std::atan2(p.rotation.z, p.rotation.w), prismatic ? 0 : t, 1e-7);
            EXPECT_EQ(r.components[0].relative_dof, prismatic ? 1 : 2);
            m.bodies[0].initial_guess = p;
        }
    }
}
TEST(AssemblyInteraction, PlaneSphereContactStaysOnSelectedSide) {
    Model m;
    m.bodies = {{"a", {{0, 0, 2}, {}}}, {"ground", {}}};
    m.geometry = {{"s", "a", SphereGeometry{{}, 2}}, {"p", "ground", PlaneGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "contact";
    c.kind = ConstraintKind::Contact;
    c.first = {"ground", "p"};
    c.second = GeometryRef{"a", "s"};
    c.contact_kind = ContactKind::Point;
    c.contact_branch = 1;
    m.constraints = {f, c};
    auto r = Solver{}.solve(m, drag("a", {5, 6, -4}));
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.z, 2, 1e-7);
    EXPECT_NEAR(p.translation.x, 5, 1e-7);
    EXPECT_NEAR(p.translation.y, 6, 1e-7);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
}
TEST(AssemblyInteraction, SignedOffsetAndMeasuredSuppressedDoNotChangeDrivingSet) {
    Model m;
    m.bodies = {{"a", {{0, 0, -3}, {}}}, {"ground", {}}};
    m.geometry = {{"p", "a", PlaneGeometry{}}, {"p", "ground", PlaneGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "offset";
    c.kind = ConstraintKind::Distance;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.value = -3;
    c.distance_relation = DistanceRelation::SelectedPlaneNormal;
    c.direction_relation = DirectionRelation::Same;
    m.constraints = {f, c};
    c.id = "measured";
    c.mode = ConstraintMode::Measured;
    c.value = 900;
    m.constraints.push_back(c);
    c.id = "suppressed";
    c.mode = ConstraintMode::Suppressed;
    m.constraints.push_back(c);
    auto r = Solver{}.solve(m, drag("a", {5, 6, 9}));
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.z, -3, 1e-7);
    EXPECT_NEAR(p.translation.x, 5, 1e-7);
    EXPECT_NEAR(p.translation.y, 6, 1e-7);
    EXPECT_EQ(r.components[0].relative_dof, 3);
}
TEST(AssemblyInteraction, RevoluteWrapAndReverseRemainContinuous) {
    Model m;
    m.bodies = {{"a", {}}, {"ground", {}}};
    m.geometry = {{"p", "a", PointGeometry{}},
                  {"p", "ground", PointGeometry{}},
                  {"z", "a", AxisGeometry{}},
                  {"z", "ground", AxisGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "pivot";
    c.kind = ConstraintKind::Coincident;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    Constraint axis;
    axis.id = "axis";
    axis.kind = ConstraintKind::Parallel;
    axis.first = {"a", "z"};
    axis.second = GeometryRef{"ground", "z"};
    axis.direction_relation = DirectionRelation::Same;
    m.constraints = {f, c, axis};
    for (double angle : {-0.05, -0.01, 0.01, 0.05, 0.01, -0.01}) {
        auto o = drag("a", {});
        o.drag_target->target_pose.rotation = {0, 0, std::sin(angle / 2), std::cos(angle / 2)};
        o.drag_target->translation_components = {false, false, false};
        o.drag_target->rotation_components = {false, false, true};
        auto r = Solver{}.solve(m, o);
        qualified(r);
        auto p = body_pose(r, "a");
        EXPECT_NEAR(p.translation.x, 0, 1e-7);
        EXPECT_NEAR(p.translation.y, 0, 1e-7);
        EXPECT_NEAR(p.translation.z, 0, 1e-7);
        EXPECT_NEAR(2 * std::atan2(p.rotation.z, p.rotation.w), angle, 1e-7);
        EXPECT_EQ(r.components[0].relative_dof, 1);
        m.bodies[0].initial_guess = p;
    }
}
TEST(AssemblyInteraction, PoseOnlyFramesRetainPhysicalRankButDoNotRepeatDefinitionAudit) {
    Model m;
    m.bodies = {{"a", {}}, {"ground", {}}};
    m.geometry = {{"p", "a", PlaneGeometry{}}, {"p", "ground", PlaneGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "plane";
    c.kind = ConstraintKind::Coincident;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.direction_relation = DirectionRelation::Same;
    m.constraints = {f, c};
    auto frame = Solver{}.solve(m, drag("a", {3, 4, 0}));
    qualified(frame);
    EXPECT_EQ(frame.components[0].jacobian_rank, 3);
    EXPECT_EQ(frame.components[0].relative_dof, 3);
    EXPECT_TRUE(frame.constraint_ranks.empty());
    ASSERT_FALSE(frame.equation_residuals.empty());
    bool tagged = false;
    for (const auto& d : frame.diagnostics)
        if (d.code == "INTERACTION_CONSTRAINT_RANK_AUDIT_FROZEN")
            tagged = true;
    EXPECT_TRUE(tagged);
    auto audit = Solver{}.solve(m);
    EXPECT_FALSE(audit.constraint_ranks.empty());
    EXPECT_EQ(audit.components[0].jacobian_rank, 3);
}
TEST(AssemblyInteraction, FrameRelationIsCompletePoseNotImplicitPoint) {
    Model m;
    Pose captured{{2, 3, 4}, {0, 0, std::sin(0.15), std::cos(0.15)}};
    m.bodies = {{"a", captured}, {"ground", {}}};
    m.geometry = {{"f", "a", FrameGeometry{}}, {"f", "ground", FrameGeometry{captured}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "frame";
    c.kind = ConstraintKind::Coincident;
    c.first = {"a", "f"};
    c.second = GeometryRef{"ground", "f"};
    m.constraints = {f, c};
    auto r = Solver{}.solve(m, drag("a", {9, 8, 7}));
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.x, 2, 1e-7);
    EXPECT_NEAR(p.translation.y, 3, 1e-7);
    EXPECT_NEAR(p.translation.z, 4, 1e-7);
    EXPECT_NEAR(2 * std::atan2(p.rotation.z, p.rotation.w), 0.3, 1e-8);
    EXPECT_EQ(r.components[0].relative_dof, 0);
    EXPECT_EQ(r.components[0].jacobian_rank, 6);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
}
TEST(AssemblyInteraction, DirectedAngleUsesCompleteThirdReferenceOccurrence) {
    constexpr double pi = 3.14159265358979323846;
    Model m;
    m.bodies = {{"a", {{0, 0, 0}, {0, 0, std::sin(-pi / 8), std::cos(-pi / 8)}}},
                {"ground", {}},
                {"axis-source", {{-20, 13, 5}, {0, 0, std::sin(0.37), std::cos(0.37)}}}};
    m.geometry = {{"x", "a", AxisGeometry{{}, {1, 0, 0}}},
                  {"x", "ground", AxisGeometry{{}, {1, 0, 0}}},
                  {"axis", "axis-source", AxisGeometry{{2, 3, 4}, {0, 0, 1}}}};
    Constraint f;
    f.id = "fix-ground";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint axis_fix = f;
    axis_fix.id = "fix-axis";
    axis_fix.first = {"axis-source", {}};
    Constraint c;
    c.id = "directed";
    c.kind = ConstraintKind::Angle;
    c.first = {"a", "x"};
    c.second = GeometryRef{"ground", "x"};
    c.angle_reference_geometry = GeometryRef{"axis-source", "axis"};
    c.value = pi / 4;
    m.constraints = {f, axis_fix, c};
    auto r = Solver{}.solve(m, drag("a", {4, 7, 9}));
    qualified(r);
    auto p = body_pose(r, "a");
    EXPECT_NEAR(p.translation.x, 4, 1e-7);
    EXPECT_NEAR(p.translation.y, 7, 1e-7);
    EXPECT_NEAR(p.translation.z, 9, 1e-7);
    const auto q = p.rotation;
    const double ux = 1 - 2 * (q.y * q.y + q.z * q.z), uy = 2 * (q.x * q.y + q.w * q.z);
    EXPECT_NEAR(std::atan2(-uy, ux), pi / 4, 1e-8);
    const auto source = body_pose(r, "axis-source");
    EXPECT_EQ(source.translation.x, -20);
    EXPECT_EQ(source.translation.y, 13);
    EXPECT_EQ(source.translation.z, 5);
    ASSERT_EQ(r.components.size(), 1);
    EXPECT_EQ(r.components[0].body_ids.size(), 3);
    EXPECT_EQ(r.components[0].jacobian_rank, 1);
}
TEST(AssemblyInteraction, UndefinedAlignmentKeepsFrozenContinuousBranch) {
    constexpr double pi = 3.14159265358979323846;
    for (bool opposite : {false, true}) {
        Model m;
        m.bodies = {{"a", {{}, {0, 0, opposite ? 1.0 : 0.0, opposite ? 0.0 : 1.0}}},
                    {"ground", {}}};
        m.geometry = {{"x", "a", AxisGeometry{{}, {1, 0, 0}}},
                      {"x", "ground", AxisGeometry{{}, {1, 0, 0}}}};
        Constraint f;
        f.id = "fix";
        f.kind = ConstraintKind::Fix;
        f.first = {"ground", {}};
        Constraint c;
        c.id = "alignment";
        c.kind = ConstraintKind::Parallel;
        c.first = {"a", "x"};
        c.second = GeometryRef{"ground", "x"};
        m.constraints = {f, c};
        auto o = drag("a", {});
        o.drag_target->translation_components = {false, false, false};
        o.drag_target->rotation_components = {true, true, true};
        double angle = opposite ? 0.1 : pi - 0.1;
        o.drag_target->target_pose.rotation = {0, 0, std::sin(angle / 2), std::cos(angle / 2)};
        auto r = Solver{}.solve(m, o);
        qualified(r);
        const auto q = body_pose(r, "a").rotation;
        EXPECT_NEAR(1 - 2 * (q.y * q.y + q.z * q.z), opposite ? -1 : 1, 1e-8);
        EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
        ASSERT_EQ(r.alignment_branches.size(), 1);
        EXPECT_EQ(r.alignment_branches[0].constraint_id, "alignment");
        EXPECT_EQ(r.alignment_branches[0].direction_relation,
                  opposite ? DirectionRelation::Opposite : DirectionRelation::Same);
        EXPECT_EQ(m.constraints[1].direction_relation, DirectionRelation::Unoriented);
    }
}
TEST(AssemblyInteraction, UngroundedInstantaneousWorldFreedomIncludesGlobalGauge) {
    Model m;
    m.bodies = {{"a", {}}};
    auto r = Solver{}.solve(m, drag("a", {3, 4, 5}));
    qualified(r);
    ASSERT_EQ(r.components.size(), 1);
    EXPECT_EQ(r.components[0].gauge_dof, 6);
    EXPECT_EQ(r.components[0].relative_dof, 0);
    ASSERT_EQ(r.components[0].freedoms.size(), 1);
    const auto& f = r.components[0].freedoms[0];
    EXPECT_EQ(f.kind, FreedomKind::Free);
    EXPECT_TRUE(f.relative_to_body_id.empty());
    EXPECT_EQ(f.allowed_basis.size(), 6);
    EXPECT_TRUE(f.blocked_basis.empty());
    auto static_result = Solver{}.solve(m);
    ASSERT_EQ(static_result.components[0].freedoms.size(), 1);
    EXPECT_EQ(static_result.components[0].freedoms[0].kind, FreedomKind::Fixed);
    EXPECT_EQ(static_result.components[0].freedoms[0].relative_to_body_id, "a");
}
TEST(AssemblyInteraction, UnsignedPlaneOffsetsFreezeBothSideAndSelectionOrder) {
    for (int shape : {0, 1, 2})
        for (bool swapped : {false, true})
            for (double side : {-1.0, 1.0}) {
                Model m;
                m.bodies = {{"a", {{0, 0, 3 * side}, {}}}, {"ground", {}}};
                Geometry moving = PointGeometry{};
                if (shape == 1)
                    moving = AxisGeometry{{}, {1, 0, 0}};
                if (shape == 2)
                    moving = PlaneGeometry{};
                m.geometry = {{"support", "a", moving}, {"plane", "ground", PlaneGeometry{}}};
                Constraint f;
                f.id = "fix";
                f.kind = ConstraintKind::Fix;
                f.first = {"ground", {}};
                Constraint c;
                c.id = "legacy-offset";
                c.kind = ConstraintKind::Distance;
                c.value = 3;
                c.first = swapped ? GeometryRef{"ground", "plane"} : GeometryRef{"a", "support"};
                c.second = swapped ? GeometryRef{"a", "support"} : GeometryRef{"ground", "plane"};
                m.constraints = {f, c};
                auto r = Solver{}.solve(m, drag("a", {5, 6, -9 * side}));
                qualified(r);
                auto p = body_pose(r, "a");
                EXPECT_NEAR(p.translation.z, 3 * side, 1e-7);
                EXPECT_NEAR(p.translation.x, 5, 1e-7);
                EXPECT_NEAR(p.translation.y, 6, 1e-7);
                EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
                ASSERT_EQ(r.distance_branches.size(), 1);
                EXPECT_EQ(r.distance_branches[0].constraint_id, "legacy-offset");
                EXPECT_NE(r.distance_branches[0].distance_relation, DistanceRelation::Unsigned);
                EXPECT_EQ(m.constraints[1].distance_relation, DistanceRelation::Unsigned);
                if (shape == 1) {
                    const auto q = p.rotation;
                    EXPECT_NEAR(2 * (q.x * q.z - q.w * q.y), 0, 1e-8);
                }  // infinite line remains parallel to plane
                if (shape == 2) {
                    const auto q = p.rotation;
                    EXPECT_NEAR(1 - 2 * (q.x * q.x + q.y * q.y), 1, 1e-8);
                }  // selected alignment branch
            }
}
TEST(AssemblyInteraction, FlatSphereCenterTargetRetainsScalarArgminNominalFreedom) {
    Model m;
    m.bodies = {{"a", {{10, 0, 0}, {}}, Pose{{0, 10, 0}, {}}}, {"ground", {}}};
    m.geometry = {{"p", "a", PointGeometry{}}, {"p", "ground", PointGeometry{}}};
    Constraint f;
    f.id = "fix";
    f.kind = ConstraintKind::Fix;
    f.first = {"ground", {}};
    Constraint c;
    c.id = "radius";
    c.kind = ConstraintKind::Distance;
    c.first = {"a", "p"};
    c.second = GeometryRef{"ground", "p"};
    c.value = 10;
    m.constraints = {f, c};
    auto r = Solver{}.solve(m, drag("a", {}));
    qualified(r);
    auto p = body_pose(r, "a");
    // Every point of this sphere is equally closest to its center; later
    // nominal priority must not freeze the warm-start residual vector.
    EXPECT_NEAR(p.translation.x, 10, 1e-6);
    EXPECT_NEAR(p.translation.y, 0, 1e-6);
    EXPECT_NEAR(p.translation.z, 0, 1e-6);
    EXPECT_NEAR(std::sqrt(p.translation.x * p.translation.x + p.translation.y * p.translation.y +
                          p.translation.z * p.translation.z),
                10, 1e-7);
    EXPECT_EQ(r.interaction->status, InteractionStatus::Constrained);
    EXPECT_NEAR(r.interaction->target_error, 10, 1e-7);
    EXPECT_EQ(r.components[0].preference.status, PreferenceStatus::Converged);
    EXPECT_LT(r.components[0].preference.total_objective, 1e-12);
    EXPECT_EQ(m.bodies[0].initial_pose.translation.x, 10);
    EXPECT_EQ(m.bodies[0].initial_guess->translation.y, 10);
}
}  // namespace

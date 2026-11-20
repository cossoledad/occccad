#include <BRepAlgoAPI_Cut.hxx>
#include <BRepAlgoAPI_Fuse.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRep_Builder.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <gp_Pln.hxx>

#include <internal/body_validation.hpp>
#include <internal/planar_boundary.hpp>

#include <gtest/gtest.h>

namespace {
using namespace occccad::kernel::detail;

TopoDS_Wire rectangle(double x, double y, double dx, double dy) {
    return BRepBuilderAPI_MakePolygon(gp_Pnt(x, y, 0), gp_Pnt(x + dx, y, 0),
                                      gp_Pnt(x + dx, y + dy, 0), gp_Pnt(x, y + dy, 0), true)
        .Wire();
}

enum class HoleCase { separate, crossing, nested, reversed, repeated, touching, outside };

TopoDS_Face perforated(HoleCase mode) {
    auto outer = rectangle(0, 0, 20, 220);
    if (mode == HoleCase::outside) {
        // A notch leaves one hole inside the envelope but outside the face.
        BRepBuilderAPI_MakePolygon polygon;
        for (const gp_Pnt& p :
             {gp_Pnt(0, 0, 0), gp_Pnt(20, 0, 0), gp_Pnt(20, 220, 0), gp_Pnt(0, 220, 0),
              gp_Pnt(0, 103, 0), gp_Pnt(9, 103, 0), gp_Pnt(9, 100, 0), gp_Pnt(0, 100, 0)})
            polygon.Add(p);
        polygon.Close();
        outer = polygon.Wire();
    }
    BRepBuilderAPI_MakeFace face(gp_Pln(gp::XOY()), outer, true);
    for (int i = 0; i < 70; ++i) {
        auto hole = rectangle(5, 2 + 3 * i, 2, 1);
        if (i != 33 || mode != HoleCase::reversed)
            hole.Reverse();
        face.Add(hole);
        if (i == 33) {
            if (mode == HoleCase::repeated)
                face.Add(hole);
            if (mode == HoleCase::crossing || mode == HoleCase::nested ||
                mode == HoleCase::touching) {
                auto extra = mode == HoleCase::nested
                                 ? rectangle(5.5, 101.25, .5, .5)
                                 : rectangle(mode == HoleCase::touching ? 7 : 6, 101.5, 2, 1);
                extra.Reverse();
                face.Add(extra);
            }
        }
    }
    return face.Face();
}

TEST(BodyValidation, SeparatedPlanarChecksMatchWholeFace) {
    for (auto mode : {HoleCase::separate, HoleCase::crossing, HoleCase::nested, HoleCase::reversed,
                      HoleCase::repeated, HoleCase::touching, HoleCase::outside}) {
        auto face = perforated(mode);
        gp_Trsf translation;
        translation.SetTranslation(gp_Vec(23, -17, 4));
        face.Move(TopLoc_Location(translation));
        face.Reverse();
        TopoDS_Compound checks;
        BRep_Builder{}.MakeCompound(checks);
        const bool split = append_planar_face_checks(face, checks, {});
        EXPECT_EQ(split, mode != HoleCase::repeated);
        if (!split)
            BRep_Builder{}.Add(checks, face);
        const bool oracle = BRepCheck_Analyzer(face).IsValid();
        EXPECT_EQ(oracle, mode == HoleCase::separate);
        EXPECT_EQ(BRepCheck_Analyzer(checks).IsValid(), oracle);
    }
}

TEST(BodyValidation, RebuiltPlanarLoopsRequireExactOrientedEdgesAndSeparatedChanges) {
    const auto old = perforated(HoleCase::separate);
    ASSERT_TRUE(BRepCheck_Analyzer(old).IsValid());
    const BRep_Builder builder;
    const auto rebuild = [&](bool rotate) {
        auto face = TopoDS::Face(old.EmptyCopied());
        for (TopoDS_Iterator it(old); it.More(); it.Next()) {
            const auto loop = TopoDS::Wire(it.Value());
            auto copy = TopoDS::Wire(loop.EmptyCopied());
            copy.Closed(loop.Closed());
            TopoDS_Iterator edge(loop);
            const auto first = edge.Value();
            if (rotate) edge.Next();
            for (; edge.More(); edge.Next()) builder.Add(copy, edge.Value());
            if (rotate) builder.Add(copy, first);
            builder.Add(face, copy);
        }
        return face;
    };
    const auto checks_for = [&](const TopoDS_Face& face, const std::vector<TopoDS_Face>& previous) {
        TopoDS_Compound checks;
        builder.MakeCompound(checks);
        EXPECT_TRUE(append_planar_face_checks(face, checks, {}, previous));
        return checks;
    };
    auto face = rebuild(true);
    auto complete = checks_for(face, {});
    auto inherited = checks_for(face, {old});
    // With every loop proven, only the new face with its outer wire is checked.
    EXPECT_GT(TopoDS_Iterator(complete).Value().NbChildren(), 1);
    const auto batches = TopoDS_Iterator(inherited).Value();
    ASSERT_EQ(batches.NbChildren(), 1);
    EXPECT_EQ(TopoDS_Iterator(batches).Value().NbChildren(), 1);
    EXPECT_TRUE(BRepCheck_Analyzer(inherited).IsValid());
    EXPECT_TRUE(BRepCheck_Analyzer(face).IsValid());

    int polls = 0;
    TopoDS_Compound interrupted;
    builder.MakeCompound(interrupted);
    EXPECT_THROW(append_planar_face_checks(face, interrupted, [&] { return ++polls == 3; }, {old}),
                 std::runtime_error);
    EXPECT_EQ(interrupted.NbChildren(), 0);

    for (const double x : {6., 11.}) {
        auto changed = rebuild(false);
        builder.Add(changed, rectangle(x, 101.5, 2, 1).Reversed());
        auto checks = checks_for(changed, {old});
        const bool oracle = BRepCheck_Analyzer(changed).IsValid();
        EXPECT_EQ(oracle, x == 11.);
        EXPECT_EQ(BRepCheck_Analyzer(checks).IsValid(), oracle);
    }
    auto reversed = TopoDS::Face(old.EmptyCopied());
    int index = 0;
    for (TopoDS_Iterator it(old); it.More(); it.Next(), ++index)
        builder.Add(reversed, index == 35 ? it.Value().Reversed() : it.Value());
    EXPECT_FALSE(BRepCheck_Analyzer(checks_for(reversed, {old})).IsValid());
    EXPECT_FALSE(BRepCheck_Analyzer(reversed).IsValid());

    auto notched = TopoDS::Face(old.EmptyCopied());
    const auto outside = perforated(HoleCase::outside);
    builder.Add(notched, TopoDS_Iterator(outside).Value());
    TopoDS_Iterator holes(old);
    for (holes.Next(); holes.More(); holes.Next()) builder.Add(notched, holes.Value());
    auto notched_checks = checks_for(notched, {old});
    EXPECT_FALSE(BRepCheck_Analyzer(notched_checks).IsValid());
    EXPECT_FALSE(BRepCheck_Analyzer(notched).IsValid());

    // All edges remain the same, but this distinct plane owns another
    // pcurve context and cannot inherit these checks.
    BRepBuilderAPI_MakeFace other(gp_Pln(gp::XOY()), TopoDS::Wire(TopoDS_Iterator(old).Value()), true);
    TopoDS_Iterator loop(old);
    for (loop.Next(); loop.More(); loop.Next()) other.Add(TopoDS::Wire(loop.Value()));
    auto separate = checks_for(other.Face(), {old});
    EXPECT_EQ(TopoDS_Iterator(separate).Value().NbChildren(),
              TopoDS_Iterator(complete).Value().NbChildren());
}

TEST(BodyValidation, CancelledBoundaryAndWitnessProbesStop) {
    const auto face = perforated(HoleCase::separate);
    TopoDS_Compound checks;
    BRep_Builder{}.MakeCompound(checks);
    EXPECT_THROW(append_planar_face_checks(face, checks, [] { return true; }), std::runtime_error);
    EXPECT_EQ(checks.NbChildren(), 0);
    const auto solid = BRepPrimAPI_MakeBox(10, 10, 10).Solid();
    EXPECT_THROW(proves_material_change(solid, solid, solid, true, [] { return true; }),
                 std::runtime_error);
}

TEST(BodyValidation, InheritedFacesStillCheckWholeShellAndSolid) {
    const auto box = BRepPrimAPI_MakeBox(5, 6, 7).Solid();
    bool parallel = false;
    ASSERT_TRUE(valid_body_boundary(box, box, {}, parallel));
    BRep_Builder builder;
    for (bool reverse : {false, true}) {
        TopoDS_Shell shell;
        builder.MakeShell(shell);
        TopExp_Explorer faces(box, TopAbs_FACE);
        if (reverse)
            builder.Add(shell, faces.Current().Reversed());
        for (faces.Next(); faces.More(); faces.Next())
            builder.Add(shell, faces.Current());
        TopoDS_Solid solid;
        builder.MakeSolid(solid);
        builder.Add(solid, shell);
        ASSERT_FALSE(BRepCheck_Analyzer(solid).IsValid());
        EXPECT_FALSE(valid_body_boundary(solid, box, {}, parallel));
    }
}

TEST(BodyValidation, MaterialWitnessRequiresActualResultMembership) {
    const auto box = BRepPrimAPI_MakeBox(10, 10, 10).Shape();
    const auto tool = BRepPrimAPI_MakeBox(gp_Pnt(8, 3, 3), 4, 4, 4).Shape();
    const auto touch = BRepPrimAPI_MakeBox(gp_Pnt(10, 3, 3), 4, 4, 4).Shape();
    const auto inside = BRepPrimAPI_MakeBox(gp_Pnt(2, 2, 2), 2, 2, 2).Shape();
    const auto added = BRepAlgoAPI_Fuse(box, tool).Shape();
    const auto cut = BRepAlgoAPI_Cut(box, tool).Shape();
    EXPECT_TRUE(proves_material_change(box, tool, added, true, {}));
    EXPECT_TRUE(proves_material_change(box, tool, cut, false, {}));
    EXPECT_FALSE(proves_material_change(box, tool, box, true, {}));
    EXPECT_FALSE(proves_material_change(box, inside, box, true, {}));
    EXPECT_FALSE(proves_material_change(box, touch, box, false, {}));
}
}  // namespace

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

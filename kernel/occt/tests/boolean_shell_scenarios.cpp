#include <BOPAlgo_Alerts.hxx>
#include <BRepAlgoAPI_BooleanOperation.hxx>
#include <BRepBuilderAPI_MakeFace.hxx>
#include <BRepBuilderAPI_MakePolygon.hxx>
#include <BRepPrimAPI_MakePrism.hxx>
#include <BRepCheck_Analyzer.hxx>
#include <BRepBndLib.hxx>
#include <Bnd_Box.hxx>
#include <STEPControl_Reader.hxx>
#include <BRepGProp.hxx>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepPrimAPI_MakeCylinder.hxx>
#include <BRepPrimAPI_MakeSphere.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <GProp_GProps.hxx>
#include <Message_ProgressIndicator.hxx>
#include <TopExp.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS_Compound.hxx>
#include <gtest/gtest.h>
#include <occccad/kernel/geometry_id.hpp>

#include <algorithm>
#include <chrono>
#include <cstdlib>
#include <cmath>
#include <iostream>
#include <optional>
#include <filesystem>
#include <sstream>

namespace {
using Clock = std::chrono::steady_clock;

TopoDS_Compound blocks(int count) {
    BRep_Builder builder;
    TopoDS_Compound result;
    builder.MakeCompound(result);
    for (int i = 0; i < count; ++i)
        builder.Add(result, BRepPrimAPI_MakeBox(gp_Pnt(3*i, 0, 0), 2, 2, 2).Shape());
    return result;
}

TopoDS_Wire rectangle(double x, double y, double dx, double dy) {
    return BRepBuilderAPI_MakePolygon(gp_Pnt(x,y,0), gp_Pnt(x+dx,y,0), gp_Pnt(x+dx,y+dy,0),
                                      gp_Pnt(x,y+dy,0), true).Wire();
}

void configure(BRepAlgoAPI_BooleanOperation& op, const TopoDS_Shape& stock,
               const TopoDS_Shape& tool, BOPAlgo_Operation kind) {
    TopTools_ListOfShape args, tools;
    args.Append(stock);
    tools.Append(tool);
    op.SetArguments(args);
    op.SetTools(tools);
    op.SetOperation(kind);
    op.SetNonDestructive(true);
    op.SetToFillHistory(true);
}

void check(const char* name, const TopoDS_Shape& stock, const TopoDS_Shape& tool,
           BOPAlgo_Operation kind, std::optional<double> expected_volume) {
    SCOPED_TRACE(name);
    BRepAlgoAPI_BooleanOperation op;
    configure(op, stock, tool, kind);
    op.Build();
    ASSERT_TRUE(op.IsDone() && !op.HasErrors() && !op.Shape().IsNull());
    ASSERT_TRUE(BRepCheck_Analyzer(op.Shape()).IsValid());
    GProp_GProps props;
    BRepGProp::VolumeProperties(op.Shape(), props);
    if (expected_volume) {
        EXPECT_NEAR(props.Mass(), *expected_volume, 1e-6);
    } else {
        GProp_GProps original;
        BRepGProp::VolumeProperties(stock, original);
        EXPECT_GT(props.Mass(), 0);
        EXPECT_LT(props.Mass(), original.Mass());
    }
    TopTools_IndexedMapOfShape members;
    TopExp::MapShapes(op.Shape(), members);
    std::ostringstream history;
    for (const auto& input : {stock, tool}) {
        for (auto type : {TopAbs_FACE, TopAbs_EDGE, TopAbs_VERTEX}) {
            TopTools_IndexedMapOfShape sources;
            TopExp::MapShapes(input, type, sources);
            for (int id = 1; id <= sources.Extent(); ++id) {
                const auto& shape = sources(id);
                history << op.IsDeleted(shape) << ':';
                for (const auto& image : op.Modified(shape)) {
                    ASSERT_TRUE(members.Contains(image));
                    history << members.FindIndex(image) << ',';
                }
                history << '/';
                for (const auto& image : op.Generated(shape)) {
                    ASSERT_TRUE(members.Contains(image));
                    history << members.FindIndex(image) << ',';
                }
                history << ';';
            }
        }
    }
    if (std::getenv("OCCCCAD_TEST_BOOLEAN_REPORT")) {
        int max_face_edges = 0;
        for (const auto& shape : {stock, tool, op.Shape()}) {
            for (TopExp_Explorer it(shape, TopAbs_FACE); it.More(); it.Next()) {
                int uses = 0;
                for (TopExp_Explorer edge(it.Current(), TopAbs_EDGE); edge.More(); edge.Next()) ++uses;
                max_face_edges = std::max(max_face_edges, uses);
            }
        }
        std::ostringstream brep;
        BRepTools::Write(op.Shape(), brep);
        std::cout << "boolean_oracle " << name << ' ' << occccad::kernel::make_geometry_id(brep.str())
                  << ' ' << occccad::kernel::make_geometry_id(history.str()) << " max_face_edges=" << max_face_edges << '\n';
    }
}

TEST(BooleanShell, SmallFacesPeriodicTangencyAndDegeneratePoles) {
    const auto solid = blocks(40);
    const auto strip = BRepPrimAPI_MakeBox(gp_Pnt(-1, 1, 0), 120, 2, 2).Shape();
    check("multi-common", solid, strip, BOPAlgo_COMMON, 160);
    check("multi-cut", solid, strip, BOPAlgo_CUT, 160);
    check("multi-fuse", solid, strip, BOPAlgo_FUSE, 640);
    const double pi = std::acos(-1.);
    const auto cylinder = BRepPrimAPI_MakeCylinder(3, 8).Shape();
    check("cylinder-seam", cylinder, BRepPrimAPI_MakeBox(gp_Pnt(-5,-5,4),10,10,6).Shape(), BOPAlgo_CUT, 36*pi);
    check("tangent-cut", cylinder, BRepPrimAPI_MakeBox(gp_Pnt(3,-2,0),4,4,8).Shape(), BOPAlgo_CUT, 72*pi);
    BRepBuilderAPI_MakeFace plate(rectangle(0,0,40,32));
    for (int i = 0; i < 96; ++i) {
        auto hole = rectangle(1 + 3*(i%12), 1 + 3*(i/12), 1, 1);
        hole.Reverse();
        plate.Add(hole);
    }
    check("perforated-cut", BRepPrimAPI_MakePrism(plate.Face(), gp_Vec(0,0,2)).Shape(),
          BRepPrimAPI_MakeBox(gp_Pnt(37,1,0),1,1,2).Shape(), BOPAlgo_CUT, (40*32-97)*2);
    check("sphere-pole", BRepPrimAPI_MakeSphere(3).Shape(),
          BRepPrimAPI_MakeBox(gp_Pnt(0,-4,-4),4,8,8).Shape(), BOPAlgo_COMMON, 18*pi);
}

TEST(BooleanShell, ImportedSupportHalfCut) {
    const auto path = std::filesystem::path(OCCCCAD_MODEL_FIXTURE_DIR) / "Bottom Support - Bottom Support.step";
    STEPControl_Reader reader;
    ASSERT_EQ(reader.ReadFile(path.c_str()), IFSelect_RetDone);
    ASSERT_GT(reader.TransferRoots(), 0);
    const auto stock = reader.OneShape();
    Bnd_Box bounds;
    BRepBndLib::AddOptimal(stock, bounds, false, false);
    const auto lo = bounds.CornerMin(), hi = bounds.CornerMax();
    const auto tool = BRepPrimAPI_MakeBox(gp_Pnt((lo.X()+hi.X())/2, lo.Y()-1, lo.Z()-1),
                                          gp_Pnt(hi.X()+1, hi.Y()+1, hi.Z()+1)).Shape();
    check("imported-support", stock, tool, BOPAlgo_CUT, std::nullopt);
}

class StopAfterPolls final : public Message_ProgressIndicator {
public:
    Clock::time_point requested;
    int polls = 0;
    Standard_Boolean UserBreak() override {
        if (++polls == 64) requested = Clock::now();
        return polls >= 64;
    }
    void Show(const Message_ProgressScope&, Standard_Boolean) override {}
};

TEST(BooleanShell, CancellationDuringBuildRejectsThePartialResult) {
    BRepAlgoAPI_BooleanOperation op;
    configure(op, blocks(64), BRepPrimAPI_MakeBox(gp_Pnt(-1,1,0), 192,2,2).Shape(), BOPAlgo_CUT);
    Handle(StopAfterPolls) stop = new StopAfterPolls;
    op.Build(stop->Start());
    ASSERT_GE(stop->polls, 64);
    EXPECT_TRUE(op.HasError(STANDARD_TYPE(BOPAlgo_AlertUserBreak)) || !op.IsDone());
    if (std::getenv("OCCCCAD_TEST_BOOLEAN_REPORT"))
        std::cout << "cancel_return_ms " << std::chrono::duration<double,std::milli>(Clock::now()-stop->requested).count() << '\n';
}
}

// Compare the accelerated lookup with the explorer used by the unmodified kernel.
#include <BOPTools_FaceEdgeLookup.hxx>
#include <BRep_Builder.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Edge.hxx>
#include <TopoDS_Wire.hxx>
#include <gp_Trsf.hxx>
#include <gp_Vec.hxx>

#include <iostream>
#include <vector>

int main()
{
  BRep_Builder builder;
  BOPTools_FaceEdgeLookup lookup;
  int comparisons = 0;
  for (const int count : {4, 32, 33, 96})
  {
    TopoDS_Face face;
    TopoDS_Wire wire;
    builder.MakeFace(face);
    builder.MakeWire(wire);
    std::vector<TopoDS_Edge> edges(count);
    for (int i = 0; i < count; ++i)
    {
      builder.MakeEdge(edges[i]);
      const auto orientation = i % 3 == 0 ? TopAbs_INTERNAL : TopAbs_FORWARD;
      builder.Add(wire, edges[i].Oriented(orientation));
    }
    // A seam repeats the same located edge with a different orientation.
    builder.Add(wire, edges.back().Reversed());
    builder.Add(face, wire);
    gp_Trsf shift;
    shift.SetTranslation(gp_Vec(10, 0, 0));
    TopoDS_Edge absent;
    builder.MakeEdge(absent);
    for (const auto& loc : {TopLoc_Location(), TopLoc_Location(shift)})
    {
      for (const auto sense : {TopAbs_FORWARD, TopAbs_REVERSED})
      {
        const auto current = TopoDS::Face(face.Located(loc).Oriented(sense));
        std::vector<TopoDS_Shape> queries = {edges.front().Located(loc), absent, edges.back().Located(loc)};
        for (const auto& edge : edges)
        {
          queries.push_back(edge.Located(loc).Reversed());
          queries.push_back(edge);
        }
        for (const auto& query : queries)
        {
          TopoDS_Shape expected;
          for (TopExp_Explorer it(current, TopAbs_EDGE); it.More(); it.Next())
          {
            if (it.Current().IsSame(query))
            {
              expected = it.Current();
              break;
            }
          }
          const auto actual = lookup.Find(query, current);
          ++comparisons;
          if (actual.IsNull() != expected.IsNull() || (!actual.IsNull() && !actual.IsEqual(expected)))
          {
            std::cerr << "lookup mismatch at degree " << count << ", comparison " << comparisons << '\n';
            return 1;
          }
        }
      }
    }
  }
  std::cout << comparisons << " explorer comparisons passed\n";
}

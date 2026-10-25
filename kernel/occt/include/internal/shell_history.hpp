#pragma once

#include <BRepTools_History.hxx>
#include <TopExp.hxx>
#include <TopTools_IndexedMapOfShape.hxx>

namespace occccad::kernel::detail {

inline Handle(BRepTools_History)
    shell_result_history(const TopoDS_Shape& input, const TopoDS_Shape& result,
                         const Handle(BRepTools_History) & history) {
    Handle(BRepTools_History) resolved = new BRepTools_History();
    TopTools_IndexedMapOfShape sources, members;
    TopExp::MapShapes(input, sources);
    TopExp::MapShapes(result, members);
    for (int i = 1; i <= sources.Extent(); ++i) {
        const auto& source = sources(i);
        if (!BRepTools_History::IsSupportedType(source))
            continue;
        TopTools_IndexedMapOfShape modified;
        for (TopTools_ListIteratorOfListOfShape it(history->Modified(source)); it.More(); it.Next())
            if (members.Contains(it.Value()))
                modified.Add(it.Value());
        if (members.Contains(source))
            modified.Add(source);
        for (int j = 1; j <= modified.Extent(); ++j)
            resolved->AddModified(source, modified(j));
        for (TopTools_ListIteratorOfListOfShape it(history->Generated(source)); it.More();
             it.Next())
            if (members.Contains(it.Value()) && !modified.Contains(it.Value()))
                resolved->AddGenerated(source, it.Value());
        // OCCT Merge can mark a source removed when its generated images
        // disappear, even though a modified image still survives. Do not let
        // that contradictory removal erase a real continuation across a copy.
        if (resolved->Modified(source).IsEmpty() && resolved->Generated(source).IsEmpty())
            resolved->Remove(source);
    }
    return resolved;
}

}  // namespace occccad::kernel::detail

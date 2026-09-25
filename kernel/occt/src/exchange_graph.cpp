#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <STEPCAFControl_Reader.hxx>
#include <STEPCAFControl_Writer.hxx>
#include <STEPConstruct_ExternRefs.hxx>
#include <TCollection_AsciiString.hxx>
#include <TDF_LabelSequence.hxx>
#include <TDF_Tool.hxx>
#include <TDataStd_Name.hxx>
#include <TDocStd_Document.hxx>
#include <TopoDS_Compound.hxx>
#include <XCAFDoc_DocumentTool.hxx>
#include <XCAFDoc_ShapeTool.hxx>
#include <gp_Quaternion.hxx>

#include "internal/occt_kernel.hpp"

#include <cmath>
#include <functional>
#include <map>
#include <set>
#include <sstream>

namespace occccad::kernel {
namespace {
std::string label_id(const TDF_Label& label) {
    TCollection_AsciiString entry;
    TDF_Tool::Entry(label, entry);
    return entry.ToCString();
}
std::string label_name(const TDF_Label& label, const std::string& fallback) {
    Handle(TDataStd_Name) name;
    if (!label.FindAttribute(TDataStd_Name::GetID(), name))
        return fallback;
    const auto& value = name->Get();
    std::string utf8(static_cast<size_t>(value.LengthOfCString()) + 1U, '\0');
    Standard_PCharacter buffer = utf8.data();
    const auto size = value.ToUTF8CString(buffer);
    utf8.resize(static_cast<size_t>(size));
    return utf8.empty() ? fallback : utf8;
}
PlacedGeometry pose(const TopLoc_Location& location) {
    const auto& tr = location.Transformation();
    const auto t = tr.TranslationPart();
    const auto q = tr.GetRotation();
    return {"", {t.X(), t.Y(), t.Z()}, {q.X(), q.Y(), q.Z(), q.W()}};
}
TopLoc_Location location(const PlacedGeometry& p) {
    const double n = p.rotation.x * p.rotation.x + p.rotation.y * p.rotation.y +
                     p.rotation.z * p.rotation.z + p.rotation.w * p.rotation.w;
    if (!std::isfinite(n) || std::abs(n - 1.0) > 1e-6 || !std::isfinite(p.translation.x) ||
        !std::isfinite(p.translation.y) || !std::isfinite(p.translation.z))
        throw std::invalid_argument("EXCHANGE_INVALID_PLACEMENT");
    gp_Trsf tr;
    tr.SetRotation(gp_Quaternion(p.rotation.x, p.rotation.y, p.rotation.z, p.rotation.w));
    tr.SetTranslationPart(gp_Vec(p.translation.x, p.translation.y, p.translation.z));
    return TopLoc_Location(tr);
}
void name(const TDF_Label& label, const std::string& value) {
    TDataStd_Name::Set(label, TCollection_ExtendedString(value.c_str(), true));
}
}  // namespace
ExchangeGraph OcctKernel::readStepGraph(const std::string& path) {
    Handle(TDocStd_Document) doc = new TDocStd_Document("BinXCAF");
    XCAFDoc_DocumentTool::SetLengthUnit(doc, 1.0, UnitsMethods_LengthUnit_Millimeter);
    STEPCAFControl_Reader reader;
    reader.SetNameMode(true);
    if (reader.ReadFile(path.c_str()) != IFSelect_RetDone)
        throw std::invalid_argument("STEP_XDE_READ_FAILED");
    STEPConstruct_ExternRefs refs(reader.Reader().WS());
    refs.LoadExternRefs();
    if (refs.NbExternRefs() > 0)
        throw std::invalid_argument("STEP_EXTERNAL_FILES_UNSUPPORTED");
    if (!reader.Transfer(doc))
        throw std::invalid_argument("STEP_XDE_TRANSFER_FAILED");
    const auto tool = XCAFDoc_DocumentTool::ShapeTool(doc->Main());
    ExchangeGraph graph;
    std::map<std::string, int> state;
    std::function<std::string(TDF_Label, unsigned)> visit;
    visit = [&](TDF_Label label, unsigned depth) -> std::string {
        if (depth > 128 || graph.definitions.size() > 100000)
            throw std::invalid_argument("EXCHANGE_GRAPH_LIMIT");
        TDF_Label referred;
        if (tool->GetReferredShape(label, referred))
            label = referred;
        const auto id = label_id(label);
        if (state[id] == 1)
            throw std::invalid_argument("EXCHANGE_REFERENCE_CYCLE");
        if (state[id] == 2)
            return id;
        state[id] = 1;
        ExchangeDefinition def;
        def.id = id;
        def.name = label_name(label, "Part");
        if (tool->IsAssembly(label)) {
            def.kind = "PRODUCT";
            TDF_LabelSequence children;
            tool->GetComponents(label, children, false);
            for (int i = 1; i <= children.Length(); ++i) {
                const auto child = children.Value(i);
                const auto target = visit(child, depth + 1);
                def.children.push_back({label_id(child), target,
                                        label_name(child, "Instance " + std::to_string(i)),
                                        pose(tool->GetLocation(child))});
            }
        } else {
            def.kind = "PART";
            auto shape = tool->GetShape(label);
            if (shape.IsNull())
                throw std::invalid_argument("EXCHANGE_EMPTY_DEFINITION");
            // Reference locations live on occurrences, never in definition geometry.
            shape.Location(TopLoc_Location());
            std::ostringstream stream;
            BRepTools::Write(shape, stream, false, false, TopTools_FormatVersion_CURRENT);
            const auto bytes = stream.str();
            def.geometry_id = loadBrepr({bytes.begin(), bytes.end()});
        }
        graph.definitions.push_back(std::move(def));
        state[id] = 2;
        return id;
    };
    TDF_LabelSequence roots;
    tool->GetFreeShapes(roots);
    for (int i = 1; i <= roots.Length(); ++i) {
        const auto label = roots.Value(i);
        const auto id = visit(label, 0);
        graph.roots.push_back({"root/" + label_id(label), id, label_name(label, "Root"),
                               pose(tool->GetLocation(label))});
    }
    if (graph.roots.empty())
        throw std::invalid_argument("EXCHANGE_EMPTY_GRAPH");
    return graph;
}
std::vector<uint8_t> OcctKernel::writeStepGraph(const ExchangeGraph& graph) {
    if (graph.definitions.empty() || graph.roots.empty())
        throw std::invalid_argument("EXCHANGE_EMPTY_GRAPH");
    Handle(TDocStd_Document) doc = new TDocStd_Document("BinXCAF");
    XCAFDoc_DocumentTool::SetLengthUnit(doc, 1.0, UnitsMethods_LengthUnit_Millimeter);
    const auto tool = XCAFDoc_DocumentTool::ShapeTool(doc->Main());
    std::map<std::string, const ExchangeDefinition*> definitions;
    std::map<std::string, TDF_Label> labels;
    std::set<std::string> visiting;
    for (const auto& def : graph.definitions)
        if (def.id.empty() || !definitions.emplace(def.id, &def).second)
            throw std::invalid_argument("EXCHANGE_DUPLICATE_DEFINITION");
    std::function<TDF_Label(const std::string&, unsigned)> build;
    build = [&](const std::string& id, unsigned depth) -> TDF_Label {
        if (depth > 128 || visiting.count(id))
            throw std::invalid_argument("EXCHANGE_REFERENCE_CYCLE");
        if (labels.count(id))
            return labels.at(id);
        if (!definitions.count(id))
            throw std::invalid_argument("EXCHANGE_MISSING_DEFINITION");
        visiting.insert(id);
        const auto& def = *definitions.at(id);
        const auto label = tool->NewShape();  // Never AddShape's geometry-based deduplication.
        if (def.kind == "PART") {
            auto bytes = serializeBrepr(def.geometry_id);
            std::istringstream stream(std::string(bytes.begin(), bytes.end()));
            TopoDS_Shape shape;
            BRep_Builder builder;
            BRepTools::Read(shape, stream, builder);
            if (shape.IsNull() || !def.children.empty())
                throw std::invalid_argument("EXCHANGE_INVALID_PART");
            tool->SetShape(label, shape);
        } else if (def.kind == "PRODUCT") {
            TopoDS_Compound compound;
            BRep_Builder builder;
            builder.MakeCompound(compound);
            tool->SetShape(label, compound);
            std::set<std::string> ids;
            for (const auto& child : def.children) {
                if (child.id.empty() || !ids.insert(child.id).second)
                    throw std::invalid_argument("EXCHANGE_DUPLICATE_OCCURRENCE");
                auto instance = tool->AddComponent(label, build(child.definition_id, depth + 1),
                                                   location(child.placement));
                name(instance, child.name);
            }
        } else
            throw std::invalid_argument("EXCHANGE_INVALID_KIND");
        name(label, def.name);
        labels[id] = label;
        visiting.erase(id);
        return label;
    };
    TDF_LabelSequence roots;
    TDF_Label container;
    if (graph.roots.size() > 1) {
        container = tool->NewShape();
        TopoDS_Compound compound;
        BRep_Builder builder;
        builder.MakeCompound(compound);
        tool->SetShape(container, compound);
        name(container, "Assembly");
    }
    for (const auto& root : graph.roots) {
        auto label = build(root.definition_id, 0);
        const auto rootLocation = location(root.placement);
        if (!container.IsNull()) {
            name(tool->AddComponent(container, label, rootLocation), root.name);
            continue;
        }
        const auto& p = root.placement;
        if (p.translation.x != 0 || p.translation.y != 0 || p.translation.z != 0 ||
            p.rotation.x != 0 || p.rotation.y != 0 || p.rotation.z != 0) {
            auto wrapper = tool->NewShape();
            TopoDS_Compound compound;
            BRep_Builder builder;
            builder.MakeCompound(compound);
            tool->SetShape(wrapper, compound);
            name(tool->AddComponent(wrapper, label, rootLocation), root.name);
            name(wrapper, root.name);
            label = wrapper;
        }
        roots.Append(label);
    }
    if (!container.IsNull())
        roots.Append(container);
    tool->UpdateAssemblies();
    STEPCAFControl_Writer writer;
    writer.SetNameMode(true);
    if (!writer.Transfer(roots, STEPControl_AsIs))
        throw std::runtime_error("STEP_XDE_EXPORT_TRANSFER_FAILED");
    std::ostringstream stream;
    if (writer.WriteStream(stream) != IFSelect_RetDone)
        throw std::runtime_error("STEP_XDE_EXPORT_WRITE_FAILED");
    const auto bytes = stream.str();
    return {bytes.begin(), bytes.end()};
}
}  // namespace occccad::kernel

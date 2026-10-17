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
#include <XCAFDoc_ColorTool.hxx>
#include <Quantity_Color.hxx>
#include <TopoDS_Iterator.hxx>
#include <TopExp.hxx>
#include <TopTools_IndexedMapOfShape.hxx>
#include <gp_Quaternion.hxx>

#include "internal/occt_kernel.hpp"

#include <cmath>
#include <functional>
#include <map>
#include <set>
#include <sstream>
#include <iomanip>

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
std::string color_hex(const Quantity_Color& color) {
    double r, g, b;
    color.Values(r, g, b, Quantity_TOC_sRGB);
    std::ostringstream out;
    out << '#' << std::hex << std::setfill('0');
    for (auto channel : {r, g, b}) out << std::setw(2) << int(std::lround(channel * 255));
    return out.str();
}
Quantity_Color color_value(const std::string& value) {
    if (value.size() != 7 || value[0] != '#' || value.find_first_not_of("0123456789abcdefABCDEF", 1) != std::string::npos)
        throw std::invalid_argument("EXCHANGE_INVALID_COLOR");
    const auto rgb = std::stoul(value.substr(1), nullptr, 16);
    return Quantity_Color(double((rgb >> 16) & 255) / 255, double((rgb >> 8) & 255) / 255,
                          double(rgb & 255) / 255, Quantity_TOC_sRGB);
}
ExchangeAppearance read_colors(const TopoDS_Shape& shape, const TDF_Label& label,
                              const Handle(XCAFDoc_ColorTool)& colors) {
    ExchangeAppearance out;
    Quantity_Color color;
    if (colors->GetColor(label, XCAFDoc_ColorSurf, color) || colors->GetColor(label, XCAFDoc_ColorGen, color))
        out.color = color_hex(color);
    TopTools_IndexedMapOfShape faces;
    TopExp::MapShapes(shape, TopAbs_FACE, faces);
    std::map<uint64_t, std::string> values;
    std::function<void(const TopoDS_Shape&, std::string, unsigned)> visit;
    visit = [&](const TopoDS_Shape& current, std::string inherited, unsigned depth) {
        if (depth > 128) throw std::invalid_argument("EXCHANGE_SHAPE_DEPTH_LIMIT");
        Quantity_Color own;
        if (colors->GetColor(current, XCAFDoc_ColorSurf, own) || colors->GetColor(current, XCAFDoc_ColorGen, own))
            inherited = color_hex(own);
        if (current.ShapeType() == TopAbs_FACE) {
            const auto id = static_cast<uint64_t>(faces.FindIndex(current));
            if (!inherited.empty() && inherited != out.color) values[id] = inherited;
            return;
        }
        for (TopoDS_Iterator it(current); it.More(); it.Next()) visit(it.Value(), inherited, depth + 1);
    };
    visit(shape, out.color, 0);
    for (const auto& [id, value] : values) out.faces.push_back({id, value});
    return out;
}
std::string label_color(const TDF_Label& label, const Handle(XCAFDoc_ColorTool)& colors) {
    Quantity_Color color;
    return colors->GetColor(label, XCAFDoc_ColorSurf, color) || colors->GetColor(label, XCAFDoc_ColorGen, color)
        ? color_hex(color) : "";
}
void write_colors(const TopoDS_Shape& shape, const TDF_Label& label, const ExchangeAppearance& appearance,
                  const Handle(XCAFDoc_ShapeTool)& shapes, const Handle(XCAFDoc_ColorTool)& colors,
                  const TDF_Label& owner = {}) {
    if (!appearance.color.empty()) colors->SetColor(label, color_value(appearance.color), XCAFDoc_ColorSurf);
    TopTools_IndexedMapOfShape faces;
    TopExp::MapShapes(shape, TopAbs_FACE, faces);
    std::set<uint64_t> seen;
    for (const auto& face : appearance.faces) {
        if (face.local_id == 0 || face.local_id > static_cast<uint64_t>(faces.Extent()) || !seen.insert(face.local_id).second)
            throw std::invalid_argument("EXCHANGE_INVALID_COLOR_LOCATOR");
        const auto target = shapes->AddSubShape(owner.IsNull() ? label : owner, faces(static_cast<int>(face.local_id)));
        if (target.IsNull()) throw std::invalid_argument("EXCHANGE_COLOR_SUBSHAPE_FAILED");
        colors->SetColor(target, color_value(face.color), XCAFDoc_ColorSurf);
    }
}
}  // namespace
ExchangeGraph OcctKernel::readStepGraph(const std::string& path) {
    Handle(TDocStd_Document) doc = new TDocStd_Document("BinXCAF");
    XCAFDoc_DocumentTool::SetLengthUnit(doc, 1.0, UnitsMethods_LengthUnit_Millimeter);
    STEPCAFControl_Reader reader;
    reader.SetNameMode(true);
    reader.SetColorMode(true);
    if (reader.ReadFile(path.c_str()) != IFSelect_RetDone)
        throw std::invalid_argument("STEP_XDE_READ_FAILED");
    STEPConstruct_ExternRefs refs(reader.Reader().WS());
    refs.LoadExternRefs();
    if (refs.NbExternRefs() > 0)
        throw std::invalid_argument("STEP_EXTERNAL_FILES_UNSUPPORTED");
    if (!reader.Transfer(doc))
        throw std::invalid_argument("STEP_XDE_TRANSFER_FAILED");
    const auto tool = XCAFDoc_DocumentTool::ShapeTool(doc->Main());
    const auto colors = XCAFDoc_DocumentTool::ColorTool(doc->Main());
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
                                        pose(tool->GetLocation(child)), label_color(child, colors)});
            }
        } else {
            def.kind = "PART";
            auto shape = tool->GetShape(label);
            if (shape.IsNull())
                throw std::invalid_argument("EXCHANGE_EMPTY_DEFINITION");
            def.appearance = read_colors(shape, label, colors);
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
    const auto colors = XCAFDoc_DocumentTool::ColorTool(doc->Main());
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
            write_colors(shape, label, def.appearance, tool, colors);
            // Use actual subshape relationships in the combined export shape.
            // Body/face locators never become persistent identity.
            TopoDS_Iterator children(shape);
            for (const auto& body : def.bodies) {
                if (!children.More()) throw std::invalid_argument("EXCHANGE_BODY_SHAPE_MISMATCH");
                const auto subshape = children.Value();
                const auto sublabel = tool->AddSubShape(label, subshape);
                if (sublabel.IsNull()) throw std::invalid_argument("EXCHANGE_BODY_SUBSHAPE_FAILED");
                write_colors(subshape, sublabel, body.appearance, tool, colors, label);
                children.Next();
            }
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
                if (!child.color.empty()) colors->SetColor(instance, color_value(child.color), XCAFDoc_ColorSurf);
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
            const auto component = tool->AddComponent(container, label, rootLocation);
            name(component, root.name);
            if (!root.color.empty()) colors->SetColor(component, color_value(root.color), XCAFDoc_ColorSurf);
            continue;
        }
        const auto& p = root.placement;
        if (p.translation.x != 0 || p.translation.y != 0 || p.translation.z != 0 ||
            p.rotation.x != 0 || p.rotation.y != 0 || p.rotation.z != 0 || !root.color.empty()) {
            auto wrapper = tool->NewShape();
            TopoDS_Compound compound;
            BRep_Builder builder;
            builder.MakeCompound(compound);
            tool->SetShape(wrapper, compound);
            const auto component = tool->AddComponent(wrapper, label, rootLocation);
            name(component, root.name);
            if (!root.color.empty()) colors->SetColor(component, color_value(root.color), XCAFDoc_ColorSurf);
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
    writer.SetColorMode(true);
    if (!writer.Transfer(roots, STEPControl_AsIs))
        throw std::runtime_error("STEP_XDE_EXPORT_TRANSFER_FAILED");
    std::ostringstream stream;
    if (writer.WriteStream(stream) != IFSelect_RetDone)
        throw std::runtime_error("STEP_XDE_EXPORT_WRITE_FAILED");
    const auto bytes = stream.str();
    return {bytes.begin(), bytes.end()};
}
}  // namespace occccad::kernel

#include <gtest/gtest.h>
#include <BRepPrimAPI_MakeBox.hxx>
#include <BRepTools.hxx>
#include <BRep_Builder.hxx>
#include <TopExp_Explorer.hxx>
#include <TopoDS.hxx>
#include <TopoDS_Shell.hxx>
#include <TopoDS_Solid.hxx>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>
#include <unistd.h>

namespace {
std::string trim(std::string value) {
    const auto first = value.find_first_not_of(" \t\r\n");
    if (first == std::string::npos) return {};
    return value.substr(first, value.find_last_not_of(" \t\r\n") - first + 1);
}
std::string env(const char* key, const std::string& fallback) {
    const char* value = std::getenv(key);
    return value && *value ? value : fallback;
}
void set(const char* key, const std::string& value) {
    if (setenv(key, value.c_str(), 1) != 0) throw std::runtime_error("cannot set test environment");
}
std::string resolve(const std::string& value) {
    const std::filesystem::path path(value);
    return (path.is_absolute() ? path : std::filesystem::path(OCCCCAD_TEST_SOURCE_DIR) / path).lexically_normal().string();
}
void load_environment() {
    const auto file = env("OCCCCAD_ENV_FILE", std::string(OCCCCAD_TEST_SOURCE_DIR) + "/.env");
    std::ifstream input(file);
    if (!input && std::getenv("OCCCCAD_ENV_FILE")) throw std::runtime_error("explicit test env file not found");
    for (std::string line; std::getline(input, line);) {
        line = trim(line);
        if (line.empty() || line.front() == '#') continue;
        if (line.rfind("export ", 0) == 0) line = trim(line.substr(7));
        const auto split = line.find('=');
        if (split == std::string::npos) continue;
        const auto key = trim(line.substr(0, split));
        auto value = trim(line.substr(split + 1));
        if (value.size() >= 2 && value.front() == value.back() && (value.front() == '\'' || value.front() == '"'))
            value = value.substr(1, value.size() - 2);
        if (!key.empty() && !std::getenv(key.c_str())) set(key.c_str(), value);
    }
    const auto root = resolve(env("OCCCCAD_TEST_RESOURCES_DIR", "build/test-resources"));
    set("OCCCCAD_TEST_RESOURCES_DIR", root);
    set("OCCCCAD_ASSEMBLY_FIXTURE_DIR", resolve(env("OCCCCAD_ASSEMBLY_FIXTURE_DIR", root + "/analytic-fixtures")));
    set("OCCCCAD_TEST_IMPORT_BREP", resolve(env("OCCCCAD_TEST_IMPORT_BREP", root + "/import-repair.brep")));
    set("OCCCCAD_TEST_CURVED_GLB", resolve(env("OCCCCAD_TEST_CURVED_GLB", root + "/curved.glb")));
    std::filesystem::create_directories(root);
}
void prepare_repair_sample() {
    const auto path = std::filesystem::path(env("OCCCCAD_TEST_IMPORT_BREP", ""));
    if (std::filesystem::exists(path)) return;
    std::filesystem::create_directories(path.parent_path());
    // A closed box with one reversed face exercises orientation healing.
    const auto box = BRepPrimAPI_MakeBox(10, 20, 30).Shape();
    BRep_Builder builder;
    TopoDS_Shell shell;
    builder.MakeShell(shell);
    bool first = true;
    for (TopExp_Explorer faces(box, TopAbs_FACE); faces.More(); faces.Next()) {
        auto face = faces.Current();
        if (first) { face.Reverse(); first = false; }
        builder.Add(shell, face);
    }
    TopoDS_Solid solid;
    builder.MakeSolid(solid);
    builder.Add(solid, shell);
    const auto staged = path.string() + "." + std::to_string(getpid()) + ".tmp";
    std::ofstream output(staged, std::ios::binary);
    BRepTools::Write(solid, output);
    if (!output.good()) throw std::runtime_error("cannot write imported repair sample");
    output.close();
    std::filesystem::rename(staged, path);
}
}

int main(int argc, char** argv) {
    try {
        load_environment();
        testing::InitGoogleTest(&argc, argv);
        if (!testing::GTEST_FLAG(list_tests)) prepare_repair_sample();
        return RUN_ALL_TESTS();
    } catch (const std::exception& error) {
        std::cerr << "test resources: " << error.what() << '\n';
        return 1;
    }
}

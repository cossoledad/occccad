// Internal OCCT Kernel Adapter — implementation class
//
// This header is NOT part of the public API.
// It is only used within kernel/occt and workers/geometry.

#ifndef INTERNAL_OCCT_KERNEL_HPP
#define INTERNAL_OCCT_KERNEL_HPP

#include <occccad/kernel/kernel.hpp>
#include <occccad/kernel/dmu.hpp>
#include <occccad/kernel/topology_naming.hpp>

#include <memory>
#include <functional>
#include <string>
#include <unordered_map>
#include <vector>

// Forward declarations (OCCT types isolated to .cpp)
class TopoDS_Shape;

namespace occccad::kernel {

struct PartStageExecution {std::string feature_id,input_digest,geometry_id;bool reused=false;};
struct PartRuntimeStats {
    size_t stages_executed = 0, stages_reused = 0, generator_calls = 0,
           modifier_calls = 0, body_operation_calls = 0, stage_cache_bytes = 0, stage_evictions = 0;
    double exact_ms = 0, naming_ms = 0;
    std::vector<std::string> executed_feature_ids, reused_feature_ids;
    std::vector<PartStageExecution> stages;
};
struct PartRuntimeOptions {
    // Keys are canonical frozen prefix digests, including naming context/policy.
    std::vector<std::string> stage_keys;
    bool force_cold = false;
    std::function<bool()> cancelled;
    PartRuntimeStats* stats = nullptr;
};

/// OCCT-backed implementation of ICadKernel.
///
/// Manages a set of loaded geometries, each keyed by GeometryId.
/// All OCCT types are confined to the implementation file.
class OcctKernel : public ICadKernel {
public:
    OcctKernel();
    ~OcctKernel() override;

    // Non-copyable, movable
    OcctKernel(const OcctKernel&) = delete;
    OcctKernel& operator=(const OcctKernel&) = delete;
    OcctKernel(OcctKernel&&) noexcept;
    OcctKernel& operator=(OcctKernel&&) noexcept;

    // ICadKernel
    GeometryId loadBrepr(const std::vector<uint8_t>& data) override;
    GeometryId loadStep(const std::string& path) override;
    ExchangeGraph readStepGraph(const std::string& path);
    std::vector<uint8_t> writeStepGraph(const ExchangeGraph& graph);
    uint32_t inspectStepRootCount(const std::string& path) override;
    GeometryId loadStepRoot(const std::string& path, uint32_t root_index) override;
    // Preserve each located Solid occurrence; component ordinal is not a persistent topology ID.
    std::vector<GeometryId> splitSolids(const GeometryId& id) override;
    GeometryId repairImportedSolid(const GeometryId& id) override;
    GeometryId combine(const std::vector<PlacedGeometry>& components) override;
    void unload(const GeometryId& id) override;

    GeometryId createBox(double dx, double dy, double dz) override;
    GeometryId createRectangularPad(const RectangularPadSpec& spec) override;
    GeometryId evaluateRectangularPads(const std::vector<RectangularPadSpec>& specs,
                                       const std::vector<uint8_t>& base_brep = {}) override;
    GeometryId evaluateProfilePads(const std::vector<ProfilePadSpec>& specs,
                                   const std::vector<uint8_t>& base_brep = {}) override;
    ProfileEvaluationResult evaluateProfilePadsWithHistory(
        const std::vector<ProfilePadSpec>& specs, const std::vector<uint8_t>& base_brep = {},
        const ImportTopologySeed* import_seed = nullptr,
        const PartRuntimeOptions& runtime = {});

    TopologyInfo getTopologySummary(const GeometryId& id);
    BoundingBox getBoundingBox(const GeometryId& id) override;
    const TopologyInfo& getTopology(const GeometryId& id) override;
    double getVolume(const GeometryId& id) override;

    TessellationResult tessellate(const GeometryId& id, double linear_deflection = 0.1,
                                  double angular_deflection = 0.5, bool parallel = true) override;

    GeometryId chamfer(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                       double distance) override;

    GeometryId fillet(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                      double radius) override;

    std::vector<uint8_t> serializeBrepr(const GeometryId& id) override;
    GeometryId loadStepData(const std::vector<uint8_t>& data) override;
    std::vector<uint8_t> serializeStep(const GeometryId& id) override;

    // Additional accessors
    void clear_runtime_cache();
    void set_runtime_cache_budget(size_t bytes);
    size_t runtime_cache_bytes() const noexcept;
    size_t resident_count() const noexcept;
    bool is_loaded(const GeometryId& id) const noexcept;
    InterferenceResult analyze_interference(const PlacedGeometry& first, const PlacedGeometry& second,
        double clearance, double tolerance, const std::function<bool()>& cancelled = {});

private:
    const TopoDS_Shape& analysis_shape(const GeometryId& id) const;
    struct Impl;
    std::unique_ptr<Impl> impl_;
};

}  // namespace occccad::kernel

#endif  // INTERNAL_OCCT_KERNEL_HPP

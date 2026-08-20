// Internal OCCT Kernel Adapter — implementation class
//
// This header is NOT part of the public API.
// It is only used within kernel/occt and workers/geometry.

#ifndef INTERNAL_OCCT_KERNEL_HPP
#define INTERNAL_OCCT_KERNEL_HPP

#include <occccad/kernel/kernel.hpp>

#include <memory>
#include <string>
#include <unordered_map>
#include <vector>

// Forward declarations (OCCT types isolated to .cpp)
class TopoDS_Shape;

namespace occccad::kernel {

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
    uint32_t inspectStepRootCount(const std::string& path) override;
    GeometryId loadStepRoot(const std::string& path, uint32_t root_index) override;
    GeometryId combine(const std::vector<PlacedGeometry>& components) override;
    void unload(const GeometryId& id) override;

    GeometryId createBox(double dx, double dy, double dz) override;
    GeometryId createRectangularPad(const RectangularPadSpec& spec) override;
    GeometryId evaluateRectangularPads(const std::vector<RectangularPadSpec>& specs,
                                       const std::vector<uint8_t>& base_brep = {}) override;
    GeometryId evaluateProfilePads(const std::vector<ProfilePadSpec>& specs,
                                   const std::vector<uint8_t>& base_brep = {}) override;

    BoundingBox getBoundingBox(const GeometryId& id) override;
    const TopologyInfo& getTopology(const GeometryId& id) override;
    double getVolume(const GeometryId& id) override;

    TessellationResult tessellate(const GeometryId& id, double linear_deflection = 0.1,
                                  double angular_deflection = 0.5) override;

    GeometryId chamfer(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                       double distance) override;

    GeometryId fillet(const GeometryId& id, const std::vector<uint64_t>& edge_local_ids,
                      double radius) override;

    std::vector<uint8_t> serializeBrepr(const GeometryId& id) override;
    GeometryId loadStepData(const std::vector<uint8_t>& data) override;
    std::vector<uint8_t> serializeStep(const GeometryId& id) override;
    std::vector<uint8_t> serializeStepComponents(
        const std::vector<PlacedGeometry>& components) override;

    // Additional accessors
    size_t resident_count() const noexcept;
    bool is_loaded(const GeometryId& id) const noexcept;

private:
    struct Impl;
    std::unique_ptr<Impl> impl_;
};

}  // namespace occccad::kernel

#endif  // INTERNAL_OCCT_KERNEL_HPP

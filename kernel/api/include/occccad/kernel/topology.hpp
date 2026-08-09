// occccad Kernel — Topology helpers

#ifndef OCCCCAD_KERNEL_TOPOLOGY_HPP
#define OCCCCAD_KERNEL_TOPOLOGY_HPP

#include <cstdint>
#include <string>

namespace occccad::kernel {

/// Opaque topology index — maps local topology IDs to B-Rep sub-shapes
/// within a given GeometryId. This is intentionally a minimal stub;
/// the full topology index lives inside the OCCT adapter.
class TopologyIndex {
public:
    virtual ~TopologyIndex() = default;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_TOPOLOGY_HPP

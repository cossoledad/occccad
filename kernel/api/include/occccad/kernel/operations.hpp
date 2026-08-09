// occccad Kernel — Operations helpers

#ifndef OCCCCAD_KERNEL_OPERATIONS_HPP
#define OCCCCAD_KERNEL_OPERATIONS_HPP

#include <cstdint>
#include <string>
#include <vector>

namespace occccad::kernel {

/// Request to create a chamfer feature.
struct ChamferRequest {
    std::string geometry_id;
    std::vector<uint64_t> edge_local_ids;
    double distance = 0.0;
};

/// Request to create a fillet feature.
struct FilletRequest {
    std::string geometry_id;
    std::vector<uint64_t> edge_local_ids;
    double radius = 0.0;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_KERNEL_OPERATIONS_HPP

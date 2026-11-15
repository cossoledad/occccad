#ifndef OCCCCAD_TOPOLOGY_NAMING_HPP
#define OCCCCAD_TOPOLOGY_NAMING_HPP

#include <occccad/kernel/topology_naming_values.hpp>
#include <occccad/kernel/feature_history.hpp>

namespace occccad::kernel {

// Locators are valid only for the frozen BREP digest; IDs are allocated by the domain.
struct ImportedTopologyIdentity {
    std::string stable_id;
    PersistentTopologyType topology_type{};
    std::uint64_t local_id{};
};
struct ImportTopologySeed {
    std::string result_id;
    std::string feature_id;
    std::string body_id;
    std::string brep_sha256;
    std::vector<ImportedTopologyIdentity> identities;
};

struct ProfileEvaluationResult {
    GeometryId geometry_id;
    FeatureResultSequence feature_results;
};

}  // namespace occccad::kernel

#endif  // OCCCCAD_TOPOLOGY_NAMING_HPP

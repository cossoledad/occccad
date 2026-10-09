#pragma once
#include <occccad/kernel/kernel.hpp>

namespace occccad::kernel {
// Exact solid-only analysis, units mm and mm^3. Inconclusive never passes.
struct InterferenceResult {
    std::string classification{"INCONCLUSIVE"}, diagnostic;
    double distance{}, common_volume{};
    Vec3 first_witness, second_witness;
    bool complete{}, clearance_satisfied{}, common_tested{};
};
}

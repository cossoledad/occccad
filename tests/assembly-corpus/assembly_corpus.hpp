#pragma once

#include <occccad/assembly/solver.hpp>

#include <string>
#include <vector>

namespace occccad::assembly::corpus {

enum class MotionKind { TranslateX, TranslateY, TranslateZ, RotateX, RotateY, RotateZ };

struct FreedomCase {
    std::string name;
    Model model;
    std::string observed_body_id;
    std::size_t expected_relative_dof{};
    std::size_t expected_gauge_dof{};
    std::vector<MotionKind> allowed_motions;
    std::vector<MotionKind> blocked_motions;
};

struct ClassificationCase {
    std::string name;
    Model model;
    SolveStatus baseline_status{SolveStatus::InvalidModel};
    SolveClassification expected_classification{SolveClassification::InvalidModel};
};

std::vector<FreedomCase> freedom_cases();
std::vector<ClassificationCase> classification_cases();

}  // namespace occccad::assembly::corpus

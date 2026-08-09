from conan import ConanFile
from conan.tools.cmake import cmake_layout


class OccccadDependencies(ConanFile):
    """
    occccad root conanfile — C++ dependency graph.

    Package versions are resolved automatically by Conan from the full
    dependency graph. No version ranges are manually specified unless a
    conflict forces intervention.

    Reproducible builds are achieved via lockfiles:
        build-support/conan/locks/*.lock
    """

    settings = "os", "arch", "compiler", "build_type"

    requires = (
        # CAD Kernel (conancenter)
        "opencascade/7.9.1",
        # Logging — uncomment when needed
        # NOTE: may conflict with opencascade over fmt version;
        # if so add "fmt/12.2.0" with override=True
        # "spdlog/*",
        # RPC (uncomment when needed)
        # "protobuf/*",
        # "grpc/*",
        # Math (uncomment when needed)
        # "eigen/*",
        # "ceres-solver/*",
    )

    test_requires = (
        "gtest/[>=1.14 <3]",
        # "benchmark/[>=1.9 <3]",
    )

    generators = "CMakeDeps", "CMakeToolchain"

    def layout(self):
        cmake_layout(self)

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
        # Coarse-grained Geometry Worker RPC
        "grpc/1.71.0",
        # PlaneGCS numerical and graph dependencies. Keep these direct: the
        # sketch solver must not depend on an incidental OCCT dependency edge.
        "eigen/3.4.0",
        "boost/1.86.0",
        # Additional math backends (uncomment when needed)
        # "ceres-solver/*",
    )

    test_requires = (
        "gtest/[>=1.14 <3]",
        # "benchmark/[>=1.9 <3]",
    )

    default_options = {
        # PlaneGCS uses Boost.Graph and Boost.Math headers only. Avoid building
        # the complete Boost binary library set for the geometry worker.
        "boost/*:header_only": True,
    }

    generators = "CMakeDeps", "CMakeToolchain"

    def layout(self):
        cmake_layout(self)

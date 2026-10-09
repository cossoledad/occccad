from conan import ConanFile
from conan.tools.cmake import cmake_layout


class OccccadDependencies(ConanFile):
    settings = "os", "arch", "compiler", "build_type"

    requires = (
        "opencascade/7.9.1",
        "spdlog/1.15.3",
        "grpc/1.71.0",
        # PlaneGCS dependencies must be direct, not inherited through OCCT.
        "eigen/3.4.0",
        "boost/1.86.0",
    )

    test_requires = (
        "gtest/[>=1.14 <3]",
    )

    default_options = {
        # PlaneGCS only uses Boost headers.
        "boost/*:header_only": True,
    }

    generators = "CMakeDeps", "CMakeToolchain"

    def layout(self):
        cmake_layout(self)

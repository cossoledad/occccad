from pathlib import Path

from conan import ConanFile
from conan.tools.cmake import CMakeDeps, CMakeToolchain, cmake_layout


class OccccadDependencies(ConanFile):
    settings = "os", "arch", "compiler", "build_type"

    requires = (
        "opencascade/7.9.1",
        "spdlog/1.15.3",
        "grpc/1.71.0",
        "eigen/3.4.0",
        "boost/1.86.0",
    )
    test_requires = ("gtest/[>=1.14 <3]",)
    # Generators execute on the build machine, using its Release context even
    # when host libraries and the application are Debug/sanitized.
    tool_requires = ("protobuf/5.27.0", "grpc/1.71.0")
    default_options = {"boost/*:header_only": True}

    def layout(self):
        cmake_layout(self)

    def generate(self):
        CMakeDeps(self).generate()
        toolchain = CMakeToolchain(self)
        toolchain.user_presets_path = False
        for package, executable, variable in (
            ("protobuf", "protoc", "OCCCCAD_PROTOC_EXECUTABLE"),
            ("grpc", "grpc_cpp_plugin", "OCCCCAD_GRPC_CPP_PLUGIN_EXECUTABLE"),
        ):
            binary = Path(self.dependencies.build[package].cpp_info.bindirs[0]) / executable
            toolchain.variables[variable] = str(binary)
        toolchain.generate()

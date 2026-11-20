find_package(OpenCASCADE REQUIRED)

add_library(occccad_occt_kernel STATIC
    src/occt_kernel.cpp
    src/body_validation.cpp
    src/planar_boundary.cpp
    src/mass_properties.cpp
    src/fillet_boundary.cpp
    src/mesh_boundary.cpp
    src/edge_profile.cpp
    src/surface_normals.cpp
    src/shell_junction.cpp
    src/shell_corner.cpp
    src/exchange_graph.cpp
    src/sketch_curve.cpp
    src/geometry_feature.cpp
    src/surface_feature.cpp
    src/surface_shape_control.cpp
    src/dmu.cpp
    src/measurement.cpp
    src/curvature.cpp
)

target_include_directories(
    occccad_occt_kernel
    PUBLIC
        include
    PRIVATE
        ${CMAKE_CURRENT_SOURCE_DIR}/include/internal
)

target_link_libraries(
    occccad_occt_kernel
    PUBLIC
        occccad_kernel_api
        opencascade::opencascade
)

target_compile_features(
    occccad_occt_kernel
    PUBLIC
        cxx_std_17
)

get_filename_component(conan_generators "${CMAKE_TOOLCHAIN_FILE}" DIRECTORY)
include("${conan_generators}/occccad-occt.cmake")
target_compile_definitions(occccad_occt_kernel PUBLIC OCCCCAD_OCCT_RECIPE_REVISION="${OCCCCAD_OCCT_RECIPE_REVISION}")
occccad_set_warnings(occccad_occt_kernel)

if(OCCCCAD_BUILD_TESTS)
    add_subdirectory(tests)
endif()

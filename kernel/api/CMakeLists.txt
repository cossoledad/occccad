# Kernel API — public types and OCCT-independent utilities

add_library(occccad_kernel_api STATIC
    src/geometry_id.cpp
    src/mesh_glb.cpp
)

target_include_directories(
    occccad_kernel_api
    PUBLIC
        include
)

target_compile_features(
    occccad_kernel_api
    PUBLIC
        cxx_std_17
)

occccad_set_warnings(occccad_kernel_api PRIVATE)

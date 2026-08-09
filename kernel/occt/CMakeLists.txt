# OCCT Kernel Adapter — links against OpenCASCADE from conancenter

find_package(OpenCASCADE REQUIRED)

add_library(occccad_occt_kernel STATIC
    src/occt_kernel.cpp
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

occccad_set_warnings(occccad_occt_kernel PRIVATE)

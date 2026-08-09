# Kernel API — pure interface, no OCCT dependency

add_library(occccad_kernel_api INTERFACE)

target_include_directories(
    occccad_kernel_api
    INTERFACE
        include
)

target_compile_features(
    occccad_kernel_api
    INTERFACE
        cxx_std_17
)

occccad_set_warnings(occccad_kernel_api INTERFACE)

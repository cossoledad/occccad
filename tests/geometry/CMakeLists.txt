# Geometry smoke tests

add_executable(occccad_smoke_test
    smoke_test.cpp
)

target_link_libraries(
    occccad_smoke_test
    PRIVATE
        occccad_occt_kernel
        occccad_kernel_api
)

occccad_set_warnings(occccad_smoke_test PRIVATE)

add_test(
    NAME occccad_smoke_test
    COMMAND occccad_smoke_test
)

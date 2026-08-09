# Geometry Worker — gRPC server hosting OCCT kernel

add_executable(occccad_geometry_worker
    src/main.cpp
)

target_link_libraries(
    occccad_geometry_worker
    PRIVATE
        occccad_occt_kernel
        occccad_kernel_api
)

occccad_set_warnings(occccad_geometry_worker PRIVATE)
occccad_apply_sanitizers(occccad_geometry_worker)

target_compile_features(
    occccad_geometry_worker
    PRIVATE
        cxx_std_17
)

# Future: link gRPC, Proto, spdlog
# target_link_libraries(occccad_geometry_worker PRIVATE grpc::grpc++ protobuf::libprotobuf spdlog::spdlog)

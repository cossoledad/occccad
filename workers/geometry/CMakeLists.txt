# Geometry Worker — coarse-grained gRPC server hosting the OCCT kernel

find_package(Protobuf REQUIRED CONFIG)
find_package(gRPC REQUIRED CONFIG)

set(OCCCCAD_WORKER_PROTO
    "${PROJECT_SOURCE_DIR}/proto/occccad/worker/v1/geometry_worker.proto"
)
set(OCCCCAD_PROTO_GENERATED_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")
set(OCCCCAD_PROTO_SOURCE
    "${OCCCCAD_PROTO_GENERATED_DIR}/occccad/worker/v1/geometry_worker.pb.cc"
)
set(OCCCCAD_PROTO_HEADER
    "${OCCCCAD_PROTO_GENERATED_DIR}/occccad/worker/v1/geometry_worker.pb.h"
)
set(OCCCCAD_GRPC_SOURCE
    "${OCCCCAD_PROTO_GENERATED_DIR}/occccad/worker/v1/geometry_worker.grpc.pb.cc"
)
set(OCCCCAD_GRPC_HEADER
    "${OCCCCAD_PROTO_GENERATED_DIR}/occccad/worker/v1/geometry_worker.grpc.pb.h"
)

add_custom_command(
    OUTPUT
        "${OCCCCAD_PROTO_SOURCE}"
        "${OCCCCAD_PROTO_HEADER}"
        "${OCCCCAD_GRPC_SOURCE}"
        "${OCCCCAD_GRPC_HEADER}"
    COMMAND ${CMAKE_COMMAND} -E make_directory "${OCCCCAD_PROTO_GENERATED_DIR}"
    COMMAND
        $<TARGET_FILE:protobuf::protoc>
        --proto_path=${PROJECT_SOURCE_DIR}/proto
        --cpp_out=${OCCCCAD_PROTO_GENERATED_DIR}
        --grpc_out=${OCCCCAD_PROTO_GENERATED_DIR}
        --plugin=protoc-gen-grpc=$<TARGET_FILE:gRPC::grpc_cpp_plugin>
        "${OCCCCAD_WORKER_PROTO}"
    DEPENDS "${OCCCCAD_WORKER_PROTO}"
    VERBATIM
)

add_library(occccad_worker_proto STATIC
    "${OCCCCAD_PROTO_SOURCE}"
    "${OCCCCAD_GRPC_SOURCE}"
)
target_include_directories(occccad_worker_proto PUBLIC "${OCCCCAD_PROTO_GENERATED_DIR}")
target_link_libraries(occccad_worker_proto PUBLIC protobuf::libprotobuf gRPC::grpc++)
target_compile_features(occccad_worker_proto PUBLIC cxx_std_17)

add_executable(occccad_geometry_worker
    src/main.cpp
)

target_link_libraries(
    occccad_geometry_worker
    PRIVATE
        occccad_occt_kernel
        occccad_kernel_api
        occccad_worker_proto
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

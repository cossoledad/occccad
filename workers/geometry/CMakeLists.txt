find_package(Protobuf REQUIRED CONFIG)
find_package(gRPC REQUIRED CONFIG)
find_package(spdlog REQUIRED CONFIG)

add_subdirectory(sketch)

# Every project Proto is generated in one native rule. Imported project files,
# generator executables and generated headers remain in Ninja's dependency graph.
foreach(tool IN ITEMS OCCCCAD_PROTOC_EXECUTABLE OCCCCAD_GRPC_CPP_PLUGIN_EXECUTABLE)
    if(NOT EXISTS "${${tool}}")
        message(FATAL_ERROR "Missing build-context generator ${tool}; run mise run cpp:deps")
    endif()
endforeach()
file(GLOB_RECURSE OCCCCAD_PROTO_INPUTS CONFIGURE_DEPENDS "${PROJECT_SOURCE_DIR}/proto/*.proto")
set(OCCCCAD_PROTO_GENERATED_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")
set(OCCCCAD_PROTO_SOURCES)
set(OCCCCAD_PROTO_OUTPUTS)
foreach(proto IN LISTS OCCCCAD_PROTO_INPUTS)
    file(RELATIVE_PATH relative "${PROJECT_SOURCE_DIR}/proto" "${proto}")
    string(REGEX REPLACE "\\.proto$" "" stem "${relative}")
    list(APPEND OCCCCAD_PROTO_SOURCES
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.pb.cc"
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.grpc.pb.cc")
    list(APPEND OCCCCAD_PROTO_OUTPUTS
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.pb.cc"
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.pb.h"
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.grpc.pb.cc"
        "${OCCCCAD_PROTO_GENERATED_DIR}/${stem}.grpc.pb.h")
endforeach()
add_custom_command(
    OUTPUT ${OCCCCAD_PROTO_OUTPUTS}
    COMMAND ${CMAKE_COMMAND} -E make_directory "${OCCCCAD_PROTO_GENERATED_DIR}"
    COMMAND
        "${OCCCCAD_PROTOC_EXECUTABLE}"
        --proto_path=${PROJECT_SOURCE_DIR}/proto
        --cpp_out=${OCCCCAD_PROTO_GENERATED_DIR}
        --grpc_out=${OCCCCAD_PROTO_GENERATED_DIR}
        --plugin=protoc-gen-grpc=${OCCCCAD_GRPC_CPP_PLUGIN_EXECUTABLE}
        ${OCCCCAD_PROTO_INPUTS}
    DEPENDS ${OCCCCAD_PROTO_INPUTS}
        "${OCCCCAD_PROTOC_EXECUTABLE}"
        "${OCCCCAD_GRPC_CPP_PLUGIN_EXECUTABLE}"
    VERBATIM
)
add_library(occccad_worker_proto STATIC ${OCCCCAD_PROTO_SOURCES})
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
        occccad_assembly_solver
        occccad_worker_proto
        occccad_sketch_solver
        spdlog::spdlog
)

occccad_set_warnings(occccad_geometry_worker)
occccad_apply_sanitizers(occccad_geometry_worker)

target_compile_features(
    occccad_geometry_worker
    PRIVATE
        cxx_std_17
)

if(OCCCCAD_BUILD_TESTS)
    find_package(GTest REQUIRED CONFIG)
    include(GoogleTest)
    add_executable(occccad_naming_artifact_tests tests/naming_artifact_test.cpp)
    target_link_libraries(occccad_naming_artifact_tests PRIVATE occccad_worker_proto GTest::gtest_main)
    target_compile_features(occccad_naming_artifact_tests PRIVATE cxx_std_17)
    gtest_discover_tests(occccad_naming_artifact_tests TEST_PREFIX "worker/")
endif()

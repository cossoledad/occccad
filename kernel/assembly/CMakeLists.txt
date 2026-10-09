find_package(Eigen3 REQUIRED)

add_library(occccad_assembly_solver STATIC
    src/solver.cpp
    src/contact.cpp
)

target_include_directories(occccad_assembly_solver PUBLIC include)
target_link_libraries(occccad_assembly_solver PUBLIC Eigen3::Eigen)
target_compile_features(occccad_assembly_solver PUBLIC cxx_std_17)
occccad_set_warnings(occccad_assembly_solver)

# A numerical replay records the actual implementation, independently of the
# stable solve-policy name. Source edits trigger reconfiguration automatically.
string(TOUPPER "${CMAKE_BUILD_TYPE}" _assembly_configuration)
set(_assembly_identity "${CMAKE_CXX_COMPILER_ID}:${CMAKE_CXX_COMPILER_VERSION}:${CMAKE_BUILD_TYPE}:${CMAKE_SYSTEM_PROCESSOR}:${Eigen3_VERSION}:${CMAKE_CXX_FLAGS}:${CMAKE_CXX_FLAGS_${_assembly_configuration}}")
foreach(_source src/solver.cpp src/contact.cpp src/contact.hpp include/occccad/assembly/solver.hpp)
    set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS "${CMAKE_CURRENT_SOURCE_DIR}/${_source}")
    file(SHA256 "${CMAKE_CURRENT_SOURCE_DIR}/${_source}" _source_digest)
    string(APPEND _assembly_identity ":${_source_digest}")
endforeach()
string(SHA256 _assembly_digest "${_assembly_identity}")
target_compile_definitions(occccad_assembly_solver PRIVATE OCCCCAD_ASSEMBLY_IMPLEMENTATION_ID="${_assembly_digest}")

if(OCCCCAD_BUILD_TESTS)
    add_subdirectory(tests)
endif()

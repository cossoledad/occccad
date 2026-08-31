find_package(Eigen3 REQUIRED)

add_library(occccad_assembly_solver STATIC
    src/solver.cpp
)

target_include_directories(occccad_assembly_solver PUBLIC include)
target_link_libraries(occccad_assembly_solver PUBLIC Eigen3::Eigen)
target_compile_features(occccad_assembly_solver PUBLIC cxx_std_17)
occccad_set_warnings(occccad_assembly_solver PRIVATE)

if(OCCCCAD_BUILD_TESTS)
    add_subdirectory(tests)
endif()

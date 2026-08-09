cmake_minimum_required(VERSION 3.30)

project(
    occccad
    VERSION 0.0.6
    DESCRIPTION "Cloud-native distributed CAD platform"
    LANGUAGES CXX
)

# ---- C++ Standard -----------------------------------------------------------
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# ---- Build Options ----------------------------------------------------------
option(OCCCCAD_BUILD_TESTS "Build tests" ON)
option(OCCCCAD_ENABLE_ASAN "Enable AddressSanitizer" OFF)
option(OCCCCAD_ENABLE_UBSAN "Enable UndefinedBehaviorSanitizer" OFF)
option(OCCCCAD_ENABLE_TSAN "Enable ThreadSanitizer" OFF)
option(OCCCCAD_ENABLE_LTO "Enable Link-Time Optimization" OFF)
option(OCCCCAD_ENABLE_CCACHE "Use ccache for compilation" ON)

# ---- CMake Modules ----------------------------------------------------------
list(APPEND CMAKE_MODULE_PATH "${PROJECT_SOURCE_DIR}/cmake")

include(OccccadWarnings)
include(OccccadSanitizers)

# ---- ccache -----------------------------------------------------------------
if(OCCCCAD_ENABLE_CCACHE)
    find_program(CCACHE_PROGRAM ccache)
    if(CCACHE_PROGRAM)
        set(CMAKE_CXX_COMPILER_LAUNCHER "${CCACHE_PROGRAM}")
        set(CMAKE_C_COMPILER_LAUNCHER "${CCACHE_PROGRAM}")
        message(STATUS "occccad: ccache enabled (${CCACHE_PROGRAM})")
    else()
        message(STATUS "occccad: ccache not found, skipping")
    endif()
endif()

# ---- Export compile_commands.json for clangd / IDE --------------------------
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# ---- Subdirectories ---------------------------------------------------------
add_subdirectory(kernel)
add_subdirectory(workers)

cmake_minimum_required(VERSION 3.30)

# Multi-config generators select the configuration at build time.
get_property(_occccad_multi_config GLOBAL PROPERTY GENERATOR_IS_MULTI_CONFIG)
if(NOT CMAKE_BUILD_TYPE AND NOT _occccad_multi_config)
    set(CMAKE_BUILD_TYPE Release CACHE STRING "Build configuration" FORCE)
endif()

project(
    occccad
    VERSION 0.0.6
    DESCRIPTION "Cloud-native distributed CAD platform"
    LANGUAGES CXX
)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

option(OCCCCAD_BUILD_TESTS "Build tests" ON)
option(OCCCCAD_ENABLE_ASAN "Enable AddressSanitizer" OFF)
option(OCCCCAD_ENABLE_UBSAN "Enable UndefinedBehaviorSanitizer" OFF)
option(OCCCCAD_ENABLE_TSAN "Enable ThreadSanitizer" OFF)
option(OCCCCAD_ENABLE_CCACHE "Use ccache for compilation" ON)

if(OCCCCAD_BUILD_TESTS)
    enable_testing()
endif()

list(APPEND CMAKE_MODULE_PATH "${PROJECT_SOURCE_DIR}/cmake")

include(OccccadWarnings)
include(OccccadSanitizers)

if(OCCCCAD_ENABLE_CCACHE)
    find_program(CCACHE_PROGRAM ccache)
    if(CCACHE_PROGRAM)
        set(CMAKE_CXX_COMPILER_LAUNCHER "${CCACHE_PROGRAM}")
        message(STATUS "occccad: ccache enabled (${CCACHE_PROGRAM})")
    else()
        message(STATUS "occccad: ccache not found, skipping")
    endif()
endif()

set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

add_subdirectory(kernel)
add_subdirectory(workers)

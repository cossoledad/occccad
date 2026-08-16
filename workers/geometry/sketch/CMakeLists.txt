# PlaneGCS is an internal FreeCAD component, not a separately versioned library.
# Fetch only the audited source manifest from the official immutable FreeCAD
# commit. Per-file hashes make upstream drift or partial downloads fail closed.
# FreeCAD 1.0.2 is deliberately selected because it supports this repository's
# C++17 baseline without carrying a local source patch.
set(OCCCCAD_PLANEGCS_COMMIT 256fc7eff3379911ab5daf88e10182c509aa8052)
set(OCCCCAD_PLANEGCS_SOURCE_ROOT "${CMAKE_CURRENT_BINARY_DIR}/upstream")
set(OCCCCAD_PLANEGCS_MANIFEST
    "src/Mod/Sketcher/App/planegcs/Constraints.cpp|3983f6c62f25382d3e23b130a3a3afe9566081345ff6b704bb2298b571a192be"
    "src/Mod/Sketcher/App/planegcs/Constraints.h|345d684ec5d7e6c728d2e9db061dad9f5f66cbea56725ed5987da40a82a27dd0"
    "src/Mod/Sketcher/App/planegcs/GCS.cpp|b7609b7493a8e46a3795fc86dd1174885ceada92e59b358095a4f01e8bb2f0ce"
    "src/Mod/Sketcher/App/planegcs/GCS.h|5f8bc0193d8ac13c7e24a71b8c81304e92414e140c4b06a3e36bfb11eec70a5b"
    "src/Mod/Sketcher/App/planegcs/Geo.cpp|eb56eaf4ee6911c0d0082c1e43e9155dcd11cf2c6443cfc7d3703cf2fbd8e330"
    "src/Mod/Sketcher/App/planegcs/Geo.h|a9c17170d80207d0b4df0f3ec521841a4590cf0548dfd822c4e85e558d76b093"
    "src/Mod/Sketcher/App/planegcs/SubSystem.cpp|3d400e9faa1b3262ec6ed8ff4ade4a8eddb0149644dc32f10335fa5ef6400725"
    "src/Mod/Sketcher/App/planegcs/SubSystem.h|9a601de5fc441675551cf717a21da184060eb2f95bd76a026430f053401ae758"
    "src/Mod/Sketcher/App/planegcs/Util.h|ff6df589332423ecb4b29e7b2f8d9f2d779f6520861b4155bb8e5a9b2f7a4eeb"
    "src/Mod/Sketcher/App/planegcs/qp_eq.cpp|2d6c544c80b08a99d6e6b26d515c36f561399cc5714ac41ce34d852ae0ed3d82"
    "src/Mod/Sketcher/App/planegcs/qp_eq.h|f77fcd9cf7e35feba3a9e990e946c8e76961943338f1245adc188f9876ab653e"
    "src/Mod/Sketcher/SketcherGlobal.h|5fcc762c367caef005963aa950bcfb65452faf2b465c3030998c8e4f259fb6a9"
    "src/boost_graph_adjacency_list.hpp|8fa45c6d14db631965bfcad11996be166ca825ed8b8aae3e60c3972ad253bdb3"
    "LICENSE|b185c4b00f01c738376e7c4d392225c079423924e089ed9626c7298c9f2bb1a3"
)

foreach(OCCCCAD_PLANEGCS_ENTRY IN LISTS OCCCCAD_PLANEGCS_MANIFEST)
    string(REPLACE "|" ";" OCCCCAD_PLANEGCS_FIELDS "${OCCCCAD_PLANEGCS_ENTRY}")
    list(GET OCCCCAD_PLANEGCS_FIELDS 0 OCCCCAD_PLANEGCS_PATH)
    list(GET OCCCCAD_PLANEGCS_FIELDS 1 OCCCCAD_PLANEGCS_SHA256)
    set(OCCCCAD_PLANEGCS_DESTINATION
        "${OCCCCAD_PLANEGCS_SOURCE_ROOT}/${OCCCCAD_PLANEGCS_PATH}"
    )
    get_filename_component(OCCCCAD_PLANEGCS_DESTINATION_DIR
        "${OCCCCAD_PLANEGCS_DESTINATION}" DIRECTORY
    )
    file(MAKE_DIRECTORY "${OCCCCAD_PLANEGCS_DESTINATION_DIR}")
    file(DOWNLOAD
        "https://raw.githubusercontent.com/FreeCAD/FreeCAD/${OCCCCAD_PLANEGCS_COMMIT}/${OCCCCAD_PLANEGCS_PATH}"
        "${OCCCCAD_PLANEGCS_DESTINATION}"
        EXPECTED_HASH "SHA256=${OCCCCAD_PLANEGCS_SHA256}"
        TLS_VERIFY ON
        STATUS OCCCCAD_PLANEGCS_DOWNLOAD_STATUS
    )
    list(GET OCCCCAD_PLANEGCS_DOWNLOAD_STATUS 0 OCCCCAD_PLANEGCS_DOWNLOAD_CODE)
    if(NOT OCCCCAD_PLANEGCS_DOWNLOAD_CODE EQUAL 0)
        list(GET OCCCCAD_PLANEGCS_DOWNLOAD_STATUS 1 OCCCCAD_PLANEGCS_DOWNLOAD_MESSAGE)
        message(FATAL_ERROR
            "PlaneGCS source download failed for ${OCCCCAD_PLANEGCS_PATH}: "
            "${OCCCCAD_PLANEGCS_DOWNLOAD_MESSAGE}"
        )
    endif()
endforeach()

find_package(Eigen3 REQUIRED CONFIG)
find_package(Boost REQUIRED CONFIG)

set(OCCCCAD_PLANEGCS_DIR
    "${OCCCCAD_PLANEGCS_SOURCE_ROOT}/src/Mod/Sketcher/App/planegcs"
)

add_library(occccad_planegcs SHARED
    "${OCCCCAD_PLANEGCS_DIR}/Constraints.cpp"
    "${OCCCCAD_PLANEGCS_DIR}/GCS.cpp"
    "${OCCCCAD_PLANEGCS_DIR}/Geo.cpp"
    "${OCCCCAD_PLANEGCS_DIR}/SubSystem.cpp"
    "${OCCCCAD_PLANEGCS_DIR}/qp_eq.cpp"
)
target_compile_definitions(occccad_planegcs PRIVATE Sketcher_EXPORTS)
target_compile_features(occccad_planegcs PUBLIC cxx_std_17)
target_include_directories(occccad_planegcs
    SYSTEM PUBLIC
        "${OCCCCAD_PLANEGCS_SOURCE_ROOT}/src/Mod/Sketcher/App"
        "${CMAKE_CURRENT_SOURCE_DIR}/compat"
    PRIVATE
        "${OCCCCAD_PLANEGCS_SOURCE_ROOT}/src"
)
target_link_libraries(occccad_planegcs PUBLIC Eigen3::Eigen Boost::boost)
add_custom_command(TARGET occccad_planegcs POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different
        "${OCCCCAD_PLANEGCS_SOURCE_ROOT}/LICENSE"
        "$<TARGET_FILE_DIR:occccad_planegcs>/LICENSE.FreeCAD-PlaneGCS"
    VERBATIM
)

add_library(occccad_sketch_solver STATIC
    src/plane_gcs_sketch_solver.cpp
)
target_include_directories(occccad_sketch_solver
    PUBLIC "${CMAKE_CURRENT_SOURCE_DIR}/include"
)
target_link_libraries(occccad_sketch_solver PRIVATE occccad_planegcs)
target_compile_features(occccad_sketch_solver PUBLIC cxx_std_17)
occccad_set_warnings(occccad_sketch_solver PRIVATE)
occccad_apply_sanitizers(occccad_sketch_solver)

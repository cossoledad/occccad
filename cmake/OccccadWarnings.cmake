# occccadWarnings.cmake
# Unified compiler warning flags for the project.

function(occccad_set_warnings target scope)
    if(NOT ${scope} MATCHES "^(PUBLIC|PRIVATE|INTERFACE)$")
        message(FATAL_ERROR "scope must be PUBLIC, PRIVATE, or INTERFACE")
    endif()

    if(CMAKE_CXX_COMPILER_ID MATCHES "GNU|Clang")
        target_compile_options(${target} ${scope}
            -Wall
            -Wextra
            -Wpedantic
            -Wshadow
            -Wnon-virtual-dtor
            -Wold-style-cast
            -Wcast-align
            -Woverloaded-virtual
            -Wformat=2
        )
    elseif(MSVC)
        target_compile_options(${target} ${scope}
            /W4
            /permissive-
        )
    endif()
endfunction()

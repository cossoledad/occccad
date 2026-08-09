# occccadSanitizers.cmake
# Controlled sanitizer support for occccad.

function(occccad_apply_sanitizers target)
    set(sanitizer_flags "")

    if(OCCCCAD_ENABLE_ASAN)
        list(APPEND sanitizer_flags "-fsanitize=address")
    endif()

    if(OCCCCAD_ENABLE_UBSAN)
        list(APPEND sanitizer_flags "-fsanitize=undefined")
    endif()

    if(OCCCCAD_ENABLE_TSAN)
        list(APPEND sanitizer_flags "-fsanitize=thread")
    endif()

    if(sanitizer_flags)
        target_compile_options(${target} PRIVATE ${sanitizer_flags} -fno-omit-frame-pointer)
        target_link_options(${target} PRIVATE ${sanitizer_flags})
        message(STATUS "occccad: Sanitizers enabled for ${target}: ${sanitizer_flags}")
    endif()
endfunction()

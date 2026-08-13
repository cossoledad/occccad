#pragma once

// PlaneGCS uses FreeCAD's component export macros but does not otherwise need
// FCGlobal. This narrow compatibility header keeps the upstream source intact.
#if defined(_WIN32)
#define FREECAD_DECL_EXPORT __declspec(dllexport)
#define FREECAD_DECL_IMPORT __declspec(dllimport)
#else
#define FREECAD_DECL_EXPORT __attribute__((visibility("default")))
#define FREECAD_DECL_IMPORT __attribute__((visibility("default")))
#endif

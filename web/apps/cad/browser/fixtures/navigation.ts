// Resolve Three.js through Vite's current dependency graph, just like the app.
// Browser acceptance must not hard-code a prebundle cache directory.
export * as T from "three";
export { NavigationHUD } from "../../src/cad/navigation/hud/navigation-hud";
export { navigationCursor } from "../../src/cad/navigation/navigation-cursor";
export { SPHERE_CURSOR_SPAN } from "../../src/cad/navigation/hud/navigation-hud-geometry";
export { CATIA_PIVOT_SPAN } from "../../src/cad/navigation/navigation-visuals";

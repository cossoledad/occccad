/** One ordering contract for persistent annotations and transient sketch feedback. */
export const SKETCH_FEEDBACK_ORDER = {
  construction: 20, constructionEndpoint: 21, leader: 25, geometry: 30, endpoint: 35, glyph: 61, label: 62,
  preview: 70, snap: 80, hover: 90, selected: 100, trim: 110,
} as const;

/** Same role preference for model picking and rendered overlay picking.
 * Distance still wins outside the small overlap tolerance. Dash gaps never
 * turn a construction entity into a different selection identity. */
export function sketchGeometryPickPriority(role: string | undefined): number {
  return role === "CONSTRUCTION" ? 0 : 10;
}

/** One ordering contract for persistent annotations and transient sketch feedback. */
export const SKETCH_FEEDBACK_ORDER = {
  leader: 60, glyph: 61, label: 62,
  preview: 70, hover: 90, selected: 100, trim: 110,
} as const;

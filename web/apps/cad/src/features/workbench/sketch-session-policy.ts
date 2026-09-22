import type { DocumentView } from "../../types";

export type NewSketchSession = { documentId: string; sketchId: string; edited: boolean };

// Only sketches created in this session may be abandoned automatically.
// Existing empty sketches and externally populated sketches must survive.
export function canDiscardNewSketch(session: NewSketchSession, view?: DocumentView): boolean {
  if (session.edited || view?.document.id !== session.documentId) return false;
  if (view.part?.features.some((feature) => feature.profile === session.sketchId)) return false;
  const sketch = view.part?.features.find((feature) => feature.id === session.sketchId)?.sketch;
  return Boolean(sketch && !sketch.entities.length && !sketch.constraints.length && !sketch.externalGeometry?.length);
}

export function defaultSolidReversed(generator: string, operation: string): boolean {
  return generator === "LINEAR_EXTRUDE" && operation === "REMOVE";
}

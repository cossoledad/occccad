import type { DocumentView, Feature } from "../../types";
import { modifierTypes, type ModifierType } from "./solid-feature-model";
import type {ContextEditScope} from '../../api/context-edit-scope';

export type HistoricalCreationType = ModifierType | "SKETCH" | "EXTRUDE_ADD" | "EXTRUDE_REMOVE";
export type FeatureInsertionContext = { afterFeatureId: string; bodyId: string; versionId: string; label: string; intentId?:string; context?:ContextEditScope; occurrencePath?:string };
export function historicalCreationTypes(sketch: boolean, independentPart: boolean): HistoricalCreationType[] {
  return sketch ? independentPart ? ["EXTRUDE_ADD", "EXTRUDE_REMOVE"] : [] : [...modifierTypes, ...(independentPart ? ["SKETCH" as const] : [])];
}

export function createdSketchFeature(before: DocumentView, result: DocumentView): Feature | undefined {
  if (before.document.id !== result.document.id) return;
  const known = new Set(before.part?.features.map(f => f.id));
  const created = result.part?.features.filter(f => f.type === "SKETCH" && f.sketch && !known.has(f.id));
  return created?.length === 1 ? created[0] : undefined;
}

export { historicalPartDisplay } from "../../viewport/feature-input-display";

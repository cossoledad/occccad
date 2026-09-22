import type { DocumentView, NamingAvailability, Selection } from "../../types";

export function topologyNamingIssue(geometryKey: string | undefined, ...views: Array<DocumentView | undefined>): NamingAvailability | undefined {
  if (!geometryKey) return;
  for (const view of views) {
    const artifact = view?.artifact?.geometryKey === geometryKey ? view.artifact : view?.artifacts?.[geometryKey];
    if (artifact?.naming) return artifact.naming.canBind ? undefined : artifact.naming;
  }
}

export function selectionNamingIssue(selection: Selection, ...views: Array<DocumentView | undefined>): NamingAvailability | undefined {
  if (!selection || !["face", "edge", "vertex"].includes(selection.kind)) return;
  return topologyNamingIssue(selection.geometryKey, ...views);
}

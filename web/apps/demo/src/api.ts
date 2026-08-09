import type { DocumentSummary, DocumentView, HistoryEntry, Vec2, Vec3 } from "./types";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  const body = await response.json().catch(() => ({})) as { error?: string };
  if (!response.ok) throw new Error(body.error ?? `HTTP ${response.status}`);
  return body as T;
}

const requestId = (): string => crypto.randomUUID();

export const api = {
  health: () => request<{ status: string; occtVersion: string }>("/api/health"),
  listDocuments: async (): Promise<DocumentSummary[]> =>
    (await request<{ documents: DocumentSummary[] }>("/api/documents")).documents,
  getDocument: (id: string) => request<DocumentView>(`/api/documents/${id}`),
  getHistory: async (id: string): Promise<HistoryEntry[]> =>
    (await request<{ workspace: string; history: HistoryEntry[] }>(`/api/documents/${id}/history`)).history,
  createVersion: async (id: string, name: string, description = ""): Promise<HistoryEntry[]> =>
    (await request<{ history: HistoryEntry[] }>(`/api/documents/${id}/versions`, {
      method: "POST", body: JSON.stringify({ requestId: requestId(), name, description }),
    })).history,
  createDocument: (type: "PART" | "PRODUCT", name: string) =>
    request<DocumentView>("/api/documents", {
      method: "POST", body: JSON.stringify({ requestId: requestId(), type, name }),
    }),
  command: (documentId: string, command: Record<string, unknown>) =>
    request<DocumentView>(`/api/documents/${documentId}/commands`, {
      method: "POST", body: JSON.stringify({ requestId: requestId(), ...command }),
    }),
  createSketch: (documentId: string, plane: string, origin: Vec2, width: number, height: number) =>
    api.command(documentId, { type: "CREATE_RECTANGLE_SKETCH", plane, origin, width, height }),
  pad: (documentId: string, sketchId: string, length: number) =>
    api.command(documentId, { type: "PAD_SKETCH", sketchId, length }),
  insert: (documentId: string, referencedDocumentId: string, name: string) =>
    api.command(documentId, {
      type: "INSERT_INSTANCE", referencedDocumentId, name, translation: [0, 0, 0],
    }),
  move: (documentId: string, instanceId: string, translation: Vec3) =>
    api.command(documentId, { type: "MOVE_INSTANCE", instanceId, translation }),
  setReferenceMode: (documentId: string, instanceId: string, referenceMode: "FOLLOW_HEAD" | "PINNED") =>
    api.command(documentId, { type: "SET_REFERENCE_MODE", instanceId, referenceMode }),
  undo: (documentId: string) => api.command(documentId, { type: "UNDO" }),
  redo: (documentId: string) => api.command(documentId, { type: "REDO" }),
  restore: (documentId: string, versionId: string) =>
    api.command(documentId, { type: "RESTORE", versionId }),
  importStep: async (documentId: string, file: File): Promise<DocumentView> => {
    const body = new FormData();
    body.set("requestId", requestId());
    body.set("file", file);
    const response = await fetch(`/api/documents/${documentId}/import-step`, { method: "POST", body });
    const value = await response.json().catch(() => ({})) as DocumentView & { error?: string };
    if (!response.ok) throw new Error(value.error ?? `HTTP ${response.status}`);
    return value;
  },
  exportStep: async (documentId: string): Promise<void> => {
    const response = await fetch(`/api/documents/${documentId}/export-step`, {
      headers: { "X-Request-ID": requestId() },
    });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
    const blob = await response.blob();
    const disposition = response.headers.get("content-disposition") ?? "";
    const match = disposition.match(/filename="?([^";]+)"?/i);
    const link = document.createElement("a");
    link.href = URL.createObjectURL(blob);
    link.download = match?.[1] ?? "occccad-part.step";
    link.click();
    URL.revokeObjectURL(link.href);
  },
};

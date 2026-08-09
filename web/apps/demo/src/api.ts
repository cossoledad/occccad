import type { DocumentSummary, DocumentView, Vec2, Vec3 } from "./types";

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
  undo: (documentId: string) => api.command(documentId, { type: "UNDO" }),
  redo: (documentId: string) => api.command(documentId, { type: "REDO" }),
};

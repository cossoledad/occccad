import type { AuditEvent, DocumentPage, DocumentScope, DocumentView, FolderSummary, HistoryEntry, ShareGrant, Team, User, Vec2, Vec3 } from "./types";

const principalKey = "occccad.principal";
let principalId = localStorage.getItem(principalKey) ?? "00000000-0000-7000-8000-000000000001";
const authenticatedHeaders = (): Record<string, string> => ({ "X-OCCCCAD-User-ID": principalId });

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...authenticatedHeaders(), ...init?.headers },
  });
  const body = await response.json().catch(() => ({})) as { error?: string };
  if (!response.ok) throw new Error(body.error ?? `HTTP ${response.status}`);
  return body as T;
}

const requestId = (): string => crypto.randomUUID();

export const api = {
  setPrincipal: (id: string): void => { principalId = id; localStorage.setItem(principalKey, id); },
  principalId: (): string => principalId,
  session: () => request<{ user: User; authenticationMode: string }>("/api/session"),
  listUsers: async (): Promise<User[]> => (await request<{ users: User[] }>("/api/users")).users,
  listTeams: async (): Promise<Team[]> => (await request<{ teams: Team[] }>("/api/teams")).teams,
  listShares: async (type: "documents" | "folders", id: string): Promise<ShareGrant[]> =>
    (await request<{ grants: ShareGrant[] }>(`/api/${type}/${id}/shares`)).grants,
  share: (type: "documents" | "folders", id: string, subjectType: "USER" | "TEAM", subjectId: string, role: "VIEWER" | "EDITOR") =>
    request<ShareGrant>(`/api/${type}/${id}/shares`, { method: "POST", body: JSON.stringify({ subjectType, subjectId, role }) }),
  unshare: async (type: "documents" | "folders", id: string, grantId: string): Promise<void> => {
    const response = await fetch(`/api/${type}/${id}/shares/${grantId}`, { method: "DELETE", headers: authenticatedHeaders() });
    if (!response.ok) throw new Error((await response.json().catch(() => ({})) as { error?: string }).error ?? `HTTP ${response.status}`);
  },
  listAudit: async (documentId: string): Promise<AuditEvent[]> =>
    (await request<{ events: AuditEvent[] }>(`/api/audit?documentId=${encodeURIComponent(documentId)}`)).events,
  health: () => request<{ status: string; occtVersion: string }>("/api/health"),
  listDocuments: async (options: {
    scope?: DocumentScope; query?: string; type?: string; folderId?: string;
    recent?: boolean; shared?: boolean; allFolders?: boolean; sort?: "updated" | "name" | "created" | "recent"; limit?: number; offset?: number;
  } = {}): Promise<DocumentPage> => {
    const parameters = new URLSearchParams({ scope: options.scope ?? "active" });
    const { query = "", type = "", folderId = "" } = options;
    if (query) parameters.set("q", query);
    if (type) parameters.set("type", type);
    if (folderId) parameters.set("folderId", folderId);
    if (options.recent) parameters.set("recent", "true");
    if (options.shared) parameters.set("shared", "true");
    if (options.allFolders) parameters.set("allFolders", "true");
    if (options.sort) parameters.set("sort", options.sort);
    parameters.set("limit", String(options.limit ?? 50));
    parameters.set("offset", String(options.offset ?? 0));
    return request<DocumentPage>(`/api/documents?${parameters}`);
  },
  listFolders: async (parentId = "", shared = false): Promise<FolderSummary[]> => {
    const parameters = new URLSearchParams();
    if (parentId) parameters.set("parentId", parentId);
    if (shared) parameters.set("shared", "true");
    return (await request<{ folders: FolderSummary[] }>(`/api/folders?${parameters}`)).folders;
  },
  folderBreadcrumbs: async (id: string): Promise<FolderSummary[]> =>
    (await request<{ folders: FolderSummary[] }>(`/api/folders/${id}/breadcrumbs`)).folders,
  createFolder: (name: string, description: string, parentId?: string) =>
    request<FolderSummary>("/api/folders", {
      method: "POST", body: JSON.stringify({ name, description, parentId: parentId || null }),
    }),
  updateFolder: (id: string, name: string, description: string) =>
    request<FolderSummary>(`/api/folders/${id}`, {
      method: "PATCH", body: JSON.stringify({ name, description }),
    }),
  deleteFolder: async (id: string): Promise<void> => {
    const response = await fetch(`/api/folders/${id}`, { method: "DELETE", headers: authenticatedHeaders() });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
  },
  getDocument: (id: string) => request<DocumentView>(`/api/documents/${id}`),
  getHistory: async (id: string): Promise<HistoryEntry[]> =>
    (await request<{ workspace: string; history: HistoryEntry[] }>(`/api/documents/${id}/history`)).history,
  createVersion: async (id: string, name: string, description = ""): Promise<HistoryEntry[]> =>
    (await request<{ history: HistoryEntry[] }>(`/api/documents/${id}/versions`, {
      method: "POST", body: JSON.stringify({ requestId: requestId(), name, description }),
    })).history,
  createDocument: (type: "PART" | "PRODUCT", name: string, description = "", folderId?: string) =>
    request<DocumentView>("/api/documents", {
      method: "POST", body: JSON.stringify({ requestId: requestId(), type, name, description, folderId: folderId || null }),
    }),
  updateDocument: (id: string, name: string, description: string) =>
    request<DocumentView>(`/api/documents/${id}`, {
      method: "PATCH", body: JSON.stringify({ requestId: requestId(), name, description }),
    }),
  deleteDocument: async (id: string): Promise<void> => {
    const response = await fetch(`/api/documents/${id}`, {
      method: "DELETE", headers: { ...authenticatedHeaders(), "X-Request-ID": requestId() },
    });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
  },
  restoreDocument: (id: string) => request<DocumentView>(`/api/documents/${id}/restore`, {
    method: "POST", headers: { "X-Request-ID": requestId() },
  }),
  moveDocument: (id: string, folderId?: string) => request<DocumentView>(`/api/documents/${id}/move`, {
    method: "POST", headers: { "X-Request-ID": requestId() },
    body: JSON.stringify({ folderId: folderId || null }),
  }),
  copyDocument: (id: string, name: string, folderId?: string) => request<DocumentView>(`/api/documents/${id}/copy`, {
    method: "POST", body: JSON.stringify({ requestId: requestId(), name, folderId: folderId ?? null }),
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
    const response = await fetch(`/api/documents/${documentId}/import-step`, { method: "POST", headers: authenticatedHeaders(), body });
    const value = await response.json().catch(() => ({})) as DocumentView & { error?: string };
    if (!response.ok) throw new Error(value.error ?? `HTTP ${response.status}`);
    return value;
  },
  exportStep: async (documentId: string): Promise<void> => {
    const response = await fetch(`/api/documents/${documentId}/export-step`, {
      headers: { ...authenticatedHeaders(), "X-Request-ID": requestId() },
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

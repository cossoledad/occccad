import type { AuditEvent, DocumentPage, DocumentScope, DocumentView, FolderSummary, HistoryEntry, Job, ShareGrant, Team, User, Vec2, Vec3 } from "./types";

const cookie = (name: string): string => decodeURIComponent(document.cookie.split("; ")
  .find((item) => item.startsWith(`${name}=`))?.split("=").slice(1).join("=") ?? "");
const mutationHeaders = (method = "GET"): Record<string, string> =>
  method === "GET" || method === "HEAD" ? {} : { "X-CSRF-Token": cookie("occccad_csrf") };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...mutationHeaders(init?.method), ...init?.headers },
  });
  const body = await response.json().catch(() => ({})) as { error?: string };
  if (!response.ok) throw new Error(body.error ?? `HTTP ${response.status}`);
  return body as T;
}

const requestId = (): string => crypto.randomUUID();

export const api = {
  session: () => request<{ user: User; authenticationMode: string }>("/api/session"),
  login: (email: string, password: string) => request<{ user: User }>("/api/auth/login", {
    method: "POST", body: JSON.stringify({ email, password }),
  }),
  register: (email: string, displayName: string, password: string) => request<{ user: User; message: string }>("/api/auth/register", {
    method: "POST", body: JSON.stringify({ email, displayName, password }),
  }),
  logout: () => request<void>("/api/auth/logout", { method: "POST" }),
  changePassword: (currentPassword: string, newPassword: string) => request<{ message: string }>("/api/auth/change-password", {
    method: "POST", body: JSON.stringify({ currentPassword, newPassword }),
  }),
  adminUsers: async (query = "", status = ""): Promise<User[]> => {
    const parameters = new URLSearchParams(); if (query) parameters.set("q", query); if (status) parameters.set("status", status);
    return (await request<{ users: User[] }>(`/api/admin/users?${parameters}`)).users;
  },
  adminStats: () => request<{ users: number; pending: number; activeSessions: number; documents: number }>("/api/admin/stats"),
  adminCreateUser: (input: { email: string; displayName: string; password: string; platformRole: string; status: string }) =>
    request<User>("/api/admin/users", { method: "POST", body: JSON.stringify(input) }),
  adminUpdateUser: (id: string, input: { displayName: string; platformRole: string; status: string }) =>
    request<User>(`/api/admin/users/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  adminDisableUser: async (id: string): Promise<void> => { await request<void>(`/api/admin/users/${id}`, { method: "DELETE" }); },
  adminResetPassword: (id: string, password: string) => request<{ message: string }>(`/api/admin/users/${id}/reset-password`, {
    method: "POST", body: JSON.stringify({ password }),
  }),
  listUsers: async (): Promise<User[]> => (await request<{ users: User[] }>("/api/users")).users,
  listTeams: async (): Promise<Team[]> => (await request<{ teams: Team[] }>("/api/teams")).teams,
  listShares: async (type: "documents" | "folders", id: string): Promise<ShareGrant[]> =>
    (await request<{ grants: ShareGrant[] }>(`/api/${type}/${id}/shares`)).grants,
  share: (type: "documents" | "folders", id: string, subjectType: "USER" | "TEAM", subjectId: string, role: "VIEWER" | "EDITOR") =>
    request<ShareGrant>(`/api/${type}/${id}/shares`, { method: "POST", body: JSON.stringify({ subjectType, subjectId, role }) }),
  unshare: async (type: "documents" | "folders", id: string, grantId: string): Promise<void> => {
    const response = await fetch(`/api/${type}/${id}/shares/${grantId}`, { method: "DELETE", credentials: "same-origin", headers: mutationHeaders("DELETE") });
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
    const response = await fetch(`/api/folders/${id}`, { method: "DELETE", credentials: "same-origin", headers: mutationHeaders("DELETE") });
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
      method: "DELETE", credentials: "same-origin", headers: { ...mutationHeaders("DELETE"), "X-Request-ID": requestId() },
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
  importStep: async (documentId: string, file: File): Promise<Job> => {
    const body = new FormData();
    body.set("requestId", requestId());
    body.set("file", file);
    const response = await fetch(`/api/documents/${documentId}/import-step`, {
      method: "POST", credentials: "same-origin", headers: mutationHeaders("POST"), body,
    });
    const value = await response.json().catch(() => ({})) as Job & { error?: string };
    if (!response.ok) throw new Error(value.error ?? `HTTP ${response.status}`);
    return value;
  },
  startExportStep: (documentId: string): Promise<Job> => request<Job>(`/api/documents/${documentId}/export-step`, {
    method: "POST", headers: { "X-Request-ID": requestId() },
  }),
  getJob: (id: string): Promise<Job> => request<Job>(`/api/jobs/${id}`),
  downloadJob: async (id: string): Promise<void> => {
    const response = await fetch(`/api/jobs/${id}/download`, {
      credentials: "same-origin", headers: { "X-Request-ID": requestId() },
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

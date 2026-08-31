import type { AuditEvent, CommandPreview, DocumentPage, DocumentProperties, DocumentScope, DocumentSummary, DocumentView, FolderSummary, HistoryEntry, Job, ShareGrant, SketchOperation, Team, ToolbarCatalog, TopologyElementProperties, User, Vec3 } from "./types";
import { realtime } from "./api/realtime-client";
import { randomUUID } from "./utils/random-uuid";
import { clientPerformanceSnapshot, recordClientPerformance } from "./utils/performance";

const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? "").replace(/\/$/, "");
export const apiURL = (path: string): string => `${apiBaseURL}${path}`;

const cookie = (name: string): string => decodeURIComponent(document.cookie.split("; ")
  .find((item) => item.startsWith(`${name}=`))?.split("=").slice(1).join("=") ?? "");
const mutationHeaders = (method = "GET"): Record<string, string> =>
  method === "GET" || method === "HEAD" ? {} : { "X-CSRF-Token": cookie("occccad_csrf") };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const started = performance.now();
  const response = await fetch(apiURL(path), {
    ...init,
    credentials: "include",
    headers: { "Content-Type": "application/json", ...mutationHeaders(init?.method), ...init?.headers },
  });
	recordClientPerformance({ name: `${init?.method ?? "GET"} ${path.split("?")[0]}`, durationMs: performance.now() - started,
		status: response.status, serverTiming: response.headers.get("Server-Timing") ?? "", at: new Date().toISOString() });
  const body = await response.json().catch(() => ({})) as { error?: string };
  if (!response.ok) throw new Error(body.error ?? `HTTP ${response.status}`);
  return body as T;
}

const requestId = (): string => randomUUID();

async function downloadDiagnosticBundle(documentId: string, command: Record<string, unknown>, errorMessage: string): Promise<void> {
	const response = await fetch(apiURL(`/api/documents/${documentId}/diagnostic-bundles`), {
		method: "POST", credentials: "include",
		headers: { "Content-Type": "application/json", ...mutationHeaders("POST"), "X-Request-ID": requestId() },
		body: JSON.stringify({
			failedCommand: command, error: errorMessage,
			client: { generatedAt: new Date().toISOString(), page: window.location.href,
				userAgent: navigator.userAgent, language: navigator.language,
				performance: clientPerformanceSnapshot() },
		}),
	});
	if (!response.ok) throw new Error(`diagnostic export failed with HTTP ${response.status}`);
	const blob = await response.blob();
	const disposition = response.headers.get("Content-Disposition") ?? "";
	const filename = disposition.match(/filename="([^"]+)"/)?.[1] ?? `occccad-diagnostic-${documentId}.json`;
	const link = document.createElement("a");
	link.href = URL.createObjectURL(blob); link.download = filename;
	document.body.appendChild(link); link.click(); link.remove();
	window.setTimeout(() => URL.revokeObjectURL(link.href), 0);
}

async function executeDocumentCommand(documentId: string, command: Record<string, unknown>): Promise<DocumentView> {
	const commandWithID = { requestId: requestId(), ...command };
	try {
		return await realtime.executeCommand(documentId, commandWithID);
	} catch (cause) {
		const error = cause instanceof Error ? cause : new Error(String(cause));
		if (error.message.includes("sketch solve")) void downloadDiagnosticBundle(documentId, commandWithID, error.message).catch(() => undefined);
		throw error;
	}
}

export const restApi = {
  session: () => request<{ user: User; authenticationMode: string }>("/api/session"),
	toolbarCatalog: () => request<ToolbarCatalog>("/api/ui/toolbars"),
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
    const response = await fetch(apiURL(`/api/${type}/${id}/shares/${grantId}`), { method: "DELETE", credentials: "include", headers: mutationHeaders("DELETE") });
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
  listOpenDocuments: async (): Promise<DocumentSummary[]> =>
    (await request<{ documents: DocumentSummary[] }>("/api/open-documents")).documents,
  closeOpenDocument: async (id: string): Promise<void> => {
    const response = await fetch(apiURL(`/api/open-documents/${id}`), {
      method: "DELETE", credentials: "include", headers: mutationHeaders("DELETE"),
    });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
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
    const response = await fetch(apiURL(`/api/folders/${id}`), { method: "DELETE", credentials: "include", headers: mutationHeaders("DELETE") });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
  },
  getDocument: (id: string) => request<DocumentView>(`/api/documents/${id}`),
  getDocumentProperties: (id: string) => request<DocumentProperties>(`/api/documents/${id}/properties`),
  getTopologyProperties: (id: string, geometryKey: string, kind: "FACE" | "EDGE" | "VERTEX", localId: number) => {
    const query = new URLSearchParams({ geometryKey, kind, localId: String(localId) });
    return request<TopologyElementProperties>(`/api/documents/${id}/topology-properties?${query}`);
  },
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
    const response = await fetch(apiURL(`/api/documents/${id}`), {
      method: "DELETE", credentials: "include", headers: { ...mutationHeaders("DELETE"), "X-Request-ID": requestId() },
    });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
  },
  restoreDocument: (id: string) => request<DocumentView>(`/api/documents/${id}/restore`, {
    method: "POST", headers: { "X-Request-ID": requestId() },
  }),
  purgeDocument: async (id: string): Promise<void> => {
    const response = await fetch(apiURL(`/api/documents/${id}/trash`), {
      method: "DELETE", credentials: "include", headers: mutationHeaders("DELETE"),
    });
    if (!response.ok) {
      const value = await response.json().catch(() => ({})) as { error?: string };
      throw new Error(value.error ?? `HTTP ${response.status}`);
    }
  },
  moveDocument: (id: string, folderId?: string) => request<DocumentView>(`/api/documents/${id}/move`, {
    method: "POST", headers: { "X-Request-ID": requestId() },
    body: JSON.stringify({ folderId: folderId || null }),
  }),
  copyDocument: (id: string, name: string, folderId?: string) => request<DocumentView>(`/api/documents/${id}/copy`, {
    method: "POST", body: JSON.stringify({ requestId: requestId(), name, folderId: folderId ?? null }),
  }),
	command: executeDocumentCommand,
	downloadDiagnosticBundle: (documentId: string) => downloadDiagnosticBundle(documentId,
		{ type: "MANUAL_DIAGNOSTIC_EXPORT", requestId: requestId() }, "manual diagnostic export"),
  previewCommand: (documentId: string, command: Record<string, unknown>, signal?: AbortSignal) =>
    request<CommandPreview>(`/api/documents/${documentId}/command-previews`, {
      method: "POST", signal, body: JSON.stringify({ requestId: requestId(), ...command }),
    }),
  createSketch: (documentId: string, plane: string, datumPlaneId?: string) =>
    restApi.command(documentId, { type: "CREATE_SKETCH", plane, datumPlaneId }),
  editSketch: (documentId: string, sketchId: string, operations: SketchOperation[]) =>
    restApi.command(documentId, { type: "EDIT_SKETCH", sketchId, operations }),
  deleteNode: (documentId: string, targetKind: string, targetId: string, ownerEntityId?: string) =>
    restApi.command(documentId, { type: "DELETE_NODE", targetKind, targetId, ownerEntityId }),
  deleteNodes: (documentId: string, targets: Array<{ targetKind: string; targetId: string; ownerEntityId?: string }>) =>
    restApi.command(documentId, { type: "DELETE_NODES", targets }),
  pad: (documentId: string, sketchId: string, length: number, intentRequestId?: string) =>
    restApi.command(documentId, { type: "PAD_SKETCH", sketchId, length,
      ...(intentRequestId ? { requestId: intentRequestId } : {}) }),
  createSolidFeature: (documentId: string, input: { sketchId: string; generator: "LINEAR_EXTRUDE" | "REVOLVE";
    operation: "NEW_BODY" | "ADD" | "REMOVE" | "INTERSECT"; length?: number; angle?: number;
    axisEntityId?: string; reversed?: boolean }, intentRequestId?: string) =>
    restApi.command(documentId, { type: "CREATE_SOLID_FEATURE", ...input,
      ...(intentRequestId ? { requestId: intentRequestId } : {}) }),
  createDatumPlane: (documentId: string, input: { name: string; origin: Vec3; normal: Vec3; uDirection: Vec3 }) =>
    restApi.command(documentId, { type: "CREATE_DATUM_PLANE", ...input }),
  createDatumAxis: (documentId: string, input: { name: string; origin: Vec3; direction: Vec3 }) =>
    restApi.command(documentId, { type: "CREATE_DATUM_AXIS", ...input }),
  insert: (documentId: string, referencedDocumentId: string) =>
    restApi.command(documentId, {
      type: "INSERT_INSTANCE", referencedDocumentId, translation: [0, 0, 0],
    }),
  move: (documentId: string, instanceId: string, translation: Vec3) =>
    restApi.command(documentId, { type: "MOVE_INSTANCE", instanceId, translation }),
  setReferenceMode: (documentId: string, instanceId: string, referenceMode: "FOLLOW_HEAD" | "PINNED") =>
    restApi.command(documentId, { type: "SET_REFERENCE_MODE", instanceId, referenceMode }),
  undo: (documentId: string) => restApi.command(documentId, { type: "UNDO" }),
  redo: (documentId: string) => restApi.command(documentId, { type: "REDO" }),
  restore: (documentId: string, versionId: string) =>
    restApi.command(documentId, { type: "RESTORE", versionId }),
  importDocument: async (file: File, folderId = ""): Promise<Job> => {
    const extension = file.name.split(".").pop()?.toUpperCase();
    const format = extension === "STEP" || extension === "STP" ? "STEP"
      : extension === "BREP" || extension === "BRP" ? "BREP" : "";
    if (!format) throw new Error("仅支持 STEP、STP、BREP 或 BRP 文件");
    const parameters = new URLSearchParams({ format, fileName: file.name });
    if (folderId) parameters.set("folderId", folderId);
    const response = await fetch(apiURL(`/api/exchange/imports?${parameters}`), {
      method: "POST", credentials: "include", body: file,
      headers: { ...mutationHeaders("POST"), "X-Request-ID": requestId(), "Content-Type": file.type || "application/octet-stream" },
    });
    const value = await response.json().catch(() => ({})) as Job & { error?: string };
    if (!response.ok) throw new Error(value.error ?? `HTTP ${response.status}`);
    return value;
  },
  startExport: (documentId: string, format: "STEP" | "BREP"): Promise<Job> => request<Job>("/api/exchange/exports", {
    method: "POST", headers: { "X-Request-ID": requestId() }, body: JSON.stringify({ documentId, format }),
  }),
  getJob: (id: string): Promise<Job> => request<Job>(`/api/jobs/${id}`),
  listJobs: async (): Promise<Job[]> => (await request<{ jobs: Job[] }>("/api/jobs")).jobs,
  cancelJob: (id: string): Promise<Job> => request<Job>(`/api/jobs/${id}/cancel`, { method: "POST" }),
  retryJob: (id: string): Promise<Job> => request<Job>(`/api/jobs/${id}/retry`, { method: "POST" }),
  downloadJob: async (id: string): Promise<void> => {
    const link = document.createElement("a");
    link.href = apiURL(`/api/jobs/${id}/download`);
    link.download = "";
    document.body.appendChild(link);
    link.click();
    link.remove();
  },
};

export type CadApi = typeof restApi;

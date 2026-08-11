export const queryKeys = {
  session: ["session"] as const,
  health: ["health"] as const,
  documents: (filters: unknown = {}) => ["documents", filters] as const,
  openDocuments: ["open-documents"] as const,
  document: (id: string) => ["document", id] as const,
  documentProperties: (id: string) => ["document-properties", id] as const,
  topologyProperties: (id: string, key: string, kind: string, localId: number) => ["topology-properties", id, key, kind, localId] as const,
  history: (id: string) => ["history", id] as const,
  folders: (parent = "") => ["folders", parent] as const,
  users: ["users"] as const,
  adminUsers: ["admin-users"] as const,
  adminStats: ["admin-stats"] as const,
};

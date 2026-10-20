CREATE TABLE automation_tasks (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (actor_id, document_id, request_id)
);
CREATE INDEX automation_tasks_owner_idx ON automation_tasks(actor_id, document_id, created_at DESC);
CREATE INDEX automation_tasks_workspace_idx ON automation_tasks(workspace_id, lease_until);

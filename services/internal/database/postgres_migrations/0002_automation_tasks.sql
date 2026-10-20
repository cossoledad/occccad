CREATE TABLE occccad.automation_tasks (
    id uuid PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES occccad.workspaces(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    payload jsonb NOT NULL,
    lease_token text NOT NULL DEFAULT '',
    lease_until bigint NOT NULL DEFAULT 0,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    UNIQUE (actor_id, document_id, request_id)
);
CREATE INDEX automation_tasks_owner_idx ON occccad.automation_tasks(actor_id, document_id, created_at DESC);
CREATE INDEX automation_tasks_workspace_idx ON occccad.automation_tasks(workspace_id, lease_until);

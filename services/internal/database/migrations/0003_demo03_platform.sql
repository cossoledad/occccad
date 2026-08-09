ALTER TABLE occccad.documents
    ADD COLUMN IF NOT EXISTS workspace_name text NOT NULL DEFAULT 'Main';

ALTER TABLE occccad.document_versions
    ADD COLUMN IF NOT EXISTS version_name text,
    ADD COLUMN IF NOT EXISTS version_description text;

ALTER TABLE occccad.commands
    ADD COLUMN IF NOT EXISTS trace_id text,
    ADD COLUMN IF NOT EXISTS span_id text;

CREATE UNIQUE INDEX IF NOT EXISTS document_version_name_idx
    ON occccad.document_versions(document_id, version_name)
    WHERE version_name IS NOT NULL;

CREATE INDEX IF NOT EXISTS commands_trace_id_idx
    ON occccad.commands(trace_id)
    WHERE trace_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS occccad.document_changes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    version_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    command_id uuid REFERENCES occccad.commands(id) ON DELETE SET NULL,
    change_type text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS document_changes_document_idx
    ON occccad.document_changes(document_id, id DESC);

INSERT INTO occccad.document_changes (
    document_id,
    version_id,
    command_id,
    change_type,
    created_at
)
SELECT
    v.document_id,
    v.id,
    v.created_by_command_id,
    COALESCE(c.command_type, 'CREATE_DOCUMENT'),
    v.created_at
FROM occccad.document_versions v
LEFT JOIN occccad.commands c ON c.id = v.created_by_command_id
WHERE NOT EXISTS (
    SELECT 1
    FROM occccad.document_changes dc
    WHERE dc.version_id = v.id
);

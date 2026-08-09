ALTER TABLE occccad.documents
    ADD COLUMN IF NOT EXISTS history_cursor integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS history_tip integer NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS occccad.document_history (
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    version_id uuid NOT NULL REFERENCES occccad.document_versions(id),
    command_id uuid REFERENCES occccad.commands(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, position),
    UNIQUE (document_id, version_id)
);

INSERT INTO occccad.document_history(document_id, position, version_id, command_id)
SELECT d.id, 0, d.head_version_id, dv.created_by_command_id
FROM occccad.documents d
JOIN occccad.document_versions dv ON dv.id = d.head_version_id
ON CONFLICT (document_id, position) DO NOTHING;

CREATE INDEX IF NOT EXISTS document_history_version_idx
    ON occccad.document_history(version_id);

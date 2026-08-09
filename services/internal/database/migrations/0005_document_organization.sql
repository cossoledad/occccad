CREATE TABLE IF NOT EXISTS occccad.folders (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    parent_id uuid REFERENCES occccad.folders(id) ON DELETE RESTRICT,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE UNIQUE INDEX IF NOT EXISTS folders_parent_name_idx
    ON occccad.folders(parent_id, lower(name)) NULLS NOT DISTINCT;

ALTER TABLE occccad.documents
    ADD COLUMN IF NOT EXISTS folder_id uuid REFERENCES occccad.folders(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS last_opened_at timestamptz,
    ADD COLUMN IF NOT EXISTS copied_from_document_id uuid REFERENCES occccad.documents(id) ON DELETE SET NULL;

ALTER TABLE occccad.documents
    DROP CONSTRAINT IF EXISTS documents_document_type_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS documents_folder_type_name_idx
    ON occccad.documents(folder_id, document_type, lower(name)) NULLS NOT DISTINCT
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS documents_folder_updated_idx
    ON occccad.documents(folder_id, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS documents_recent_idx
    ON occccad.documents(last_opened_at DESC)
    WHERE deleted_at IS NULL AND last_opened_at IS NOT NULL;

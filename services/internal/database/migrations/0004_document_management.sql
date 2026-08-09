ALTER TABLE occccad.documents
    ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

CREATE INDEX IF NOT EXISTS documents_active_updated_idx
    ON occccad.documents(updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS documents_deleted_updated_idx
    ON occccad.documents(deleted_at DESC)
    WHERE deleted_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS documents_name_search_idx
    ON occccad.documents(lower(name));

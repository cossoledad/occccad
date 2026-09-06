-- Immutable numerical evidence, including failed and preview solves. Not model history.
CREATE TABLE occccad.assembly_replays (
    id text PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    request_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    replay jsonb NOT NULL
);
CREATE INDEX assembly_replays_document_request ON occccad.assembly_replays(document_id, request_id, created_at DESC, id);
CREATE INDEX assembly_replays_document_recent ON occccad.assembly_replays(document_id, created_at DESC, id);

CREATE TABLE assistant_credentials (
    actor_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ciphertext TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE assistant_turns (
    id TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('THINKING','COMPLETED','FAILED','CANCELLED','INTERRUPTED')),
    payload TEXT NOT NULL CHECK (json_valid(payload)),
    lease_until INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(actor_id,document_id,request_id)
);
CREATE INDEX assistant_turns_owner_idx ON assistant_turns(actor_id,document_id,created_at DESC,id DESC);
CREATE UNIQUE INDEX assistant_turns_active_idx ON assistant_turns(actor_id,document_id) WHERE status='THINKING';

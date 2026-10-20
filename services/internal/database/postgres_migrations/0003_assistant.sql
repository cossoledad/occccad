CREATE TABLE occccad.assistant_credentials (
    actor_id uuid PRIMARY KEY REFERENCES occccad.users(id) ON DELETE CASCADE,
    ciphertext text NOT NULL,
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);
CREATE TABLE occccad.assistant_turns (
    id uuid PRIMARY KEY,
    actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    status text NOT NULL CHECK (status IN ('THINKING','COMPLETED','FAILED','CANCELLED','INTERRUPTED')),
    payload jsonb NOT NULL,
    lease_until bigint NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    UNIQUE(actor_id,document_id,request_id)
);
CREATE INDEX assistant_turns_owner_idx ON occccad.assistant_turns(actor_id,document_id,created_at DESC,id DESC);
CREATE UNIQUE INDEX assistant_turns_active_idx ON occccad.assistant_turns(actor_id,document_id) WHERE status='THINKING';

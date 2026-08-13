-- C0-C4 model core for a new installation. There is deliberately no legacy
-- command/history backfill: reset development databases when this schema changes.
ALTER TABLE occccad.document_versions
    ADD COLUMN model_hash text NOT NULL,
    ADD COLUMN dependency_snapshot_digest text,
    ADD COLUMN evaluation_manifest jsonb;

CREATE TABLE occccad.workspaces (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    name text NOT NULL,
    head_revision_id uuid NOT NULL REFERENCES occccad.document_versions(id),
    head_sequence bigint NOT NULL CHECK (head_sequence >= 0),
    base_revision_id uuid NOT NULL REFERENCES occccad.document_versions(id),
    policy jsonb NOT NULL DEFAULT '{"evaluation":"IMMEDIATE_ALLOW_FEATURE_FAILURE"}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_id,name)
);

CREATE TABLE occccad.domain_transactions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES occccad.workspaces(id) ON DELETE CASCADE,
    sequence bigint NOT NULL CHECK (sequence > 0),
    actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('DOMAIN','REVERT','REAPPLY','RESTORE','CREATE')),
    status text NOT NULL CHECK (status IN ('RECEIVED','PREPARED','EVALUATING','COMMITTED','REJECTED','CONFLICT','FAILED','CANCELLED')),
    base_revision_id uuid REFERENCES occccad.document_versions(id),
    result_revision_id uuid REFERENCES occccad.document_versions(id),
    root_transaction_id uuid REFERENCES occccad.domain_transactions(id),
    reverts_transaction_id uuid REFERENCES occccad.domain_transactions(id),
    reapplies_transaction_id uuid REFERENCES occccad.domain_transactions(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    committed_at timestamptz,
    UNIQUE (workspace_id,sequence),
    UNIQUE (workspace_id,request_id),
    CHECK (
        (kind IN ('DOMAIN','RESTORE','CREATE') AND root_transaction_id IS NULL
            AND reverts_transaction_id IS NULL AND reapplies_transaction_id IS NULL)
        OR (kind='REVERT' AND root_transaction_id IS NOT NULL
            AND reverts_transaction_id=root_transaction_id AND reapplies_transaction_id IS NULL)
        OR (kind='REAPPLY' AND root_transaction_id IS NOT NULL
            AND reverts_transaction_id IS NULL AND reapplies_transaction_id IS NOT NULL)
    )
);

CREATE TABLE occccad.transaction_commands (
    transaction_id uuid NOT NULL REFERENCES occccad.domain_transactions(id) ON DELETE CASCADE,
    ordinal smallint NOT NULL CHECK (ordinal >= 0),
    command_id text NOT NULL,
    type_uri text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    payload jsonb NOT NULL,
    payload_digest text NOT NULL,
    PRIMARY KEY (transaction_id,ordinal),
    UNIQUE (transaction_id,command_id)
);

CREATE TABLE occccad.change_sets (
    transaction_id uuid PRIMARY KEY REFERENCES occccad.domain_transactions(id) ON DELETE CASCADE,
    canonical_blob jsonb NOT NULL,
    canonical_digest text NOT NULL,
    read_set jsonb NOT NULL DEFAULT '[]'::jsonb,
    write_set jsonb NOT NULL DEFAULT '[]'::jsonb,
    impact_seeds jsonb NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE occccad.revision_parents (
    revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    parent_revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE RESTRICT,
    ordinal smallint NOT NULL DEFAULT 0,
    PRIMARY KEY (revision_id,ordinal),
    UNIQUE (revision_id,parent_revision_id)
);

CREATE TABLE occccad.evaluation_runs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    capability text NOT NULL,
    evaluator_digest text NOT NULL,
    input_digest text NOT NULL,
    manifest jsonb NOT NULL,
    manifest_digest text NOT NULL,
    status text NOT NULL CHECK (status IN ('SUCCEEDED','PARTIAL','FAILED')),
    authoritative boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (revision_id,capability,evaluator_digest,input_digest)
);

CREATE UNIQUE INDEX evaluation_runs_authoritative_idx
    ON occccad.evaluation_runs(revision_id,capability) WHERE authoritative;

CREATE TABLE occccad.dependency_edges (
    revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    source_key text NOT NULL,
    target_key text NOT NULL,
    edge_kind text NOT NULL CHECK (edge_kind IN (
        'READ_VALUE','READ_GEOMETRY','READ_TOPOLOGY','READ_STRUCTURE',
        'READ_CONFIGURATION','READ_MATERIAL','READ_MEASUREMENT')),
    PRIMARY KEY (revision_id,source_key,target_key,edge_kind)
);

CREATE TABLE occccad.outbox_events (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);

CREATE INDEX domain_transactions_workspace_idx
    ON occccad.domain_transactions(workspace_id,sequence DESC);
CREATE INDEX domain_transactions_root_idx
    ON occccad.domain_transactions(root_transaction_id,sequence DESC)
    WHERE root_transaction_id IS NOT NULL;
CREATE INDEX dependency_edges_source_idx
    ON occccad.dependency_edges(revision_id,source_key);
CREATE INDEX outbox_events_unpublished_idx
    ON occccad.outbox_events(created_at,id) WHERE published_at IS NULL;

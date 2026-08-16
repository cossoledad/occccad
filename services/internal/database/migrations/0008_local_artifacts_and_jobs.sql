CREATE TABLE IF NOT EXISTS occccad.artifact_objects (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    kind text NOT NULL CHECK (kind IN ('BREP','GLB','EXCHANGE_SOURCE','EXCHANGE_EXPORT','THUMBNAIL')),
    sha256 char(64) NOT NULL,
    storage_backend text NOT NULL DEFAULT 'LOCAL' CHECK (storage_backend IN ('LOCAL','S3')),
    object_key text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    state text NOT NULL DEFAULT 'READY' CHECK (state IN ('STAGING','READY','QUARANTINED','DELETING')),
    created_at timestamptz NOT NULL DEFAULT now(),
    verified_at timestamptz,
    UNIQUE(kind,sha256),
    UNIQUE(storage_backend,object_key)
);

ALTER TABLE occccad.geometry_artifacts
    ADD COLUMN IF NOT EXISTS brep_object_id uuid REFERENCES occccad.artifact_objects(id),
    ADD COLUMN IF NOT EXISTS glb_object_id uuid REFERENCES occccad.artifact_objects(id),
    ADD COLUMN IF NOT EXISTS storage_state text NOT NULL DEFAULT 'DATABASE'
        CHECK (storage_state IN ('DATABASE','DUAL','OBJECT'));

ALTER TABLE occccad.geometry_artifacts
    ALTER COLUMN brep_data DROP NOT NULL,
    ALTER COLUMN glb_data DROP NOT NULL,
    ADD CONSTRAINT geometry_artifacts_storage_payload_check CHECK (
        (storage_state <> 'DATABASE' OR (brep_data IS NOT NULL AND glb_data IS NOT NULL))
        AND (storage_state <> 'OBJECT' OR (brep_object_id IS NOT NULL AND glb_object_id IS NOT NULL))
    );

CREATE TABLE IF NOT EXISTS occccad.jobs (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    job_type text NOT NULL CHECK (job_type IN ('EXCHANGE_IMPORT','EXCHANGE_EXPORT','THUMBNAIL_RENDER','ARTIFACT_BACKFILL')),
    state text NOT NULL DEFAULT 'QUEUED'
        CHECK (state IN ('QUEUED','RUNNING','RETRY_WAIT','SUCCEEDED','FAILED','CANCELED')),
    document_id uuid REFERENCES occccad.documents(id) ON DELETE CASCADE,
    version_id uuid REFERENCES occccad.document_versions(id) ON DELETE SET NULL,
    requested_by_user_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    input_object_id uuid REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL,
    result_object_id uuid REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key text NOT NULL,
    priority smallint NOT NULL DEFAULT 0,
    attempt_count integer NOT NULL DEFAULT 0,
    max_attempts integer NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 20),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,
    heartbeat_at timestamptz,
    progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    cancel_requested_at timestamptz,
    error_code text,
    error_message text,
    trace_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    UNIQUE(job_type,idempotency_key)
);

CREATE INDEX IF NOT EXISTS jobs_claim_idx
    ON occccad.jobs(state,available_at,priority DESC,created_at);
CREATE INDEX IF NOT EXISTS jobs_lease_idx
    ON occccad.jobs(lease_expires_at) WHERE state='RUNNING';
CREATE INDEX IF NOT EXISTS jobs_document_idx
    ON occccad.jobs(document_id,created_at DESC);

CREATE TABLE IF NOT EXISTS occccad.job_attempts (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    job_id uuid NOT NULL REFERENCES occccad.jobs(id) ON DELETE CASCADE,
    attempt integer NOT NULL,
    worker_id text NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    result text CHECK (result IN ('SUCCEEDED','FAILED','LEASE_EXPIRED','CANCELED')),
    error_code text,
    error_message text,
    UNIQUE(job_id,attempt)
);

CREATE TABLE IF NOT EXISTS occccad.document_previews (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    version_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    preview_identity char(64) NOT NULL,
    renderer_version text NOT NULL,
    object_id uuid REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL,
    state text NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','READY','STALE','FAILED')),
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(document_id,preview_identity)
);

CREATE INDEX IF NOT EXISTS document_previews_current_idx
    ON occccad.document_previews(document_id,updated_at DESC);

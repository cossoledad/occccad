CREATE SCHEMA IF NOT EXISTS occccad;

CREATE TABLE IF NOT EXISTS occccad.schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS occccad.geometry_artifacts (
    geometry_key text PRIMARY KEY,
    geometry_id text NOT NULL,
    evaluator_version text NOT NULL,
    occt_version text NOT NULL,
    units text NOT NULL CHECK (units = 'mm'),
    brep_data bytea NOT NULL,
    glb_data bytea NOT NULL,
    mesh_json jsonb NOT NULL,
    bbox_json jsonb NOT NULL,
    topology_json jsonb NOT NULL,
    volume double precision NOT NULL CHECK (volume > 0),
    evaluation_count integer NOT NULL DEFAULT 1 CHECK (evaluation_count > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS occccad.documents (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    document_type text NOT NULL CHECK (document_type IN ('PART', 'PRODUCT')),
    name text NOT NULL,
    head_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_type, name)
);

CREATE TABLE IF NOT EXISTS occccad.commands (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    request_id text NOT NULL UNIQUE,
    command_type text NOT NULL,
    document_id uuid REFERENCES occccad.documents(id) ON DELETE CASCADE,
    payload jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE TABLE IF NOT EXISTS occccad.document_versions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    parent_version_id uuid REFERENCES occccad.document_versions(id),
    sequence integer NOT NULL CHECK (sequence > 0),
    model_json jsonb NOT NULL,
    geometry_key text REFERENCES occccad.geometry_artifacts(geometry_key),
    state text NOT NULL CHECK (state IN ('PENDING', 'EVALUATING', 'READY', 'FAILED')),
    created_by_command_id uuid REFERENCES occccad.commands(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (document_id, sequence)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'documents_head_version_fk'
    ) THEN
        ALTER TABLE occccad.documents
            ADD CONSTRAINT documents_head_version_fk
            FOREIGN KEY (head_version_id)
            REFERENCES occccad.document_versions(id);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS occccad.product_instances (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    product_version_id uuid NOT NULL
        REFERENCES occccad.document_versions(id) ON DELETE CASCADE,
    instance_key text NOT NULL,
    display_name text NOT NULL,
    referenced_document_id uuid NOT NULL
        REFERENCES occccad.documents(id),
    referenced_version_id uuid NOT NULL
        REFERENCES occccad.document_versions(id),
    translation_x double precision NOT NULL DEFAULT 0,
    translation_y double precision NOT NULL DEFAULT 0,
    translation_z double precision NOT NULL DEFAULT 0,
    UNIQUE (product_version_id, instance_key),
    CHECK (referenced_version_id <> product_version_id)
);

CREATE INDEX IF NOT EXISTS document_versions_document_idx
    ON occccad.document_versions(document_id, sequence DESC);
CREATE INDEX IF NOT EXISTS product_instances_version_idx
    ON occccad.product_instances(product_version_id);

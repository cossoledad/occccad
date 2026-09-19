CREATE TABLE occccad.product_context_variants (
    variant_key text PRIMARY KEY,
    base_document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    base_revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE RESTRICT,
    binding_digest text NOT NULL,
    evaluator_version text NOT NULL,
    evaluation_manifest_digest text NOT NULL,
    geometry_key text NOT NULL,
    publications jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE occccad.product_solve_manifests (
    digest text PRIMARY KEY,
    schema_version integer NOT NULL CHECK (schema_version > 0),
    root_product_document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    -- Commit-time manifests are built before the candidate Revision is inserted;
    -- the immutable revision UUID is nevertheless part of the digest/provenance.
    root_product_revision_id uuid NOT NULL,
    manifest jsonb NOT NULL,
    solver_build_policy text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE occccad.product_solve_results (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    manifest_digest text NOT NULL REFERENCES occccad.product_solve_manifests(digest) ON DELETE RESTRICT,
    request_id text NOT NULL UNIQUE,
    result jsonb,
    result_digest text,
    status text NOT NULL,
    diagnostic text,
    solver_build text,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX product_solve_results_manifest_idx
    ON occccad.product_solve_results(manifest_digest,created_at DESC);

CREATE TABLE occccad.product_releases (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    root_product_document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
    root_product_revision_id uuid NOT NULL REFERENCES occccad.document_versions(id) ON DELETE RESTRICT,
    name text NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    manifest_digest text NOT NULL,
    manifest jsonb NOT NULL,
    gate_status text NOT NULL CHECK (gate_status IN ('PASSED','FAILED')),
    created_by uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (root_product_document_id,name),
    UNIQUE (root_product_document_id,request_id)
);

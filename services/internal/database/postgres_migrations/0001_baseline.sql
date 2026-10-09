-- PostgreSQL current schema baseline. Keep paired with sqlite_migrations/0001_baseline.sql.

-- Domain tables, identity sequences, comments, constraints and indexes.

CREATE TABLE occccad.access_audit_events (
    id bigint NOT NULL,
    actor_user_id uuid,
    action text NOT NULL,
    resource_type text,
    resource_id uuid,
    request_id text,
    trace_id text,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE occccad.access_audit_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME occccad.access_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE occccad.account_audit_events (
    id bigint NOT NULL,
    actor_user_id uuid,
    target_user_id uuid,
    action text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE occccad.account_audit_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME occccad.account_audit_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE occccad.artifact_objects (
    id uuid DEFAULT uuidv7() NOT NULL,
    kind text NOT NULL,
    sha256 character(64) NOT NULL,
    storage_backend text DEFAULT 'LOCAL'::text NOT NULL,
    object_key text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    state text DEFAULT 'READY'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    verified_at timestamp with time zone,
    CONSTRAINT artifact_objects_kind_check CHECK ((length(kind) > 0)),
    CONSTRAINT artifact_objects_size_bytes_check CHECK ((size_bytes >= 0)),
    CONSTRAINT artifact_objects_state_check CHECK ((state = ANY (ARRAY['STAGING'::text, 'READY'::text, 'QUARANTINED'::text, 'DELETING'::text]))),
    CONSTRAINT artifact_objects_storage_backend_check CHECK ((length(storage_backend) > 0))
);

CREATE TABLE occccad.change_sets (
    transaction_id uuid NOT NULL,
    canonical_blob jsonb NOT NULL,
    canonical_digest text NOT NULL,
    read_set jsonb DEFAULT '[]'::jsonb NOT NULL,
    write_set jsonb DEFAULT '[]'::jsonb NOT NULL,
    impact_seeds jsonb DEFAULT '[]'::jsonb NOT NULL
);

CREATE TABLE occccad.commands (
    id uuid DEFAULT uuidv7() NOT NULL,
    request_id text NOT NULL,
    command_type text NOT NULL,
    document_id uuid,
    payload jsonb NOT NULL,
    status text NOT NULL,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    trace_id text,
    span_id text,
    CONSTRAINT commands_status_check CHECK ((status = ANY (ARRAY['PENDING'::text, 'SUCCEEDED'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.dependency_edges (
    revision_id uuid NOT NULL,
    source_key text NOT NULL,
    target_key text NOT NULL,
    edge_kind text NOT NULL,
    CONSTRAINT dependency_edges_edge_kind_check CHECK ((edge_kind = ANY (ARRAY['READ_VALUE'::text, 'READ_GEOMETRY'::text, 'READ_TOPOLOGY'::text, 'READ_STRUCTURE'::text, 'READ_CONFIGURATION'::text, 'READ_MATERIAL'::text, 'READ_MEASUREMENT'::text])))
);

CREATE TABLE occccad.document_changes (
    id bigint NOT NULL,
    document_id uuid NOT NULL,
    version_id uuid NOT NULL,
    command_id uuid,
    change_type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE occccad.document_changes ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME occccad.document_changes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);

CREATE TABLE occccad.document_history (
    document_id uuid NOT NULL,
    "position" integer NOT NULL,
    version_id uuid NOT NULL,
    command_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT document_history_position_check CHECK (("position" >= 0))
);

CREATE TABLE occccad.document_previews (
    id uuid DEFAULT uuidv7() NOT NULL,
    document_id uuid NOT NULL,
    version_id uuid NOT NULL,
    preview_identity character(64) NOT NULL,
    renderer_version text NOT NULL,
    object_id uuid,
    state text DEFAULT 'PENDING'::text NOT NULL,
    error_message text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT document_previews_state_check CHECK ((state = ANY (ARRAY['PENDING'::text, 'READY'::text, 'STALE'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.document_versions (
    id uuid DEFAULT uuidv7() NOT NULL,
    document_id uuid NOT NULL,
    parent_version_id uuid,
    sequence integer NOT NULL,
    model_json jsonb NOT NULL,
    state text NOT NULL,
    created_by_command_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    version_name text,
    version_description text,
    model_hash text NOT NULL,
    dependency_snapshot_digest text,
    evaluation_manifest jsonb,
    CONSTRAINT document_versions_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT document_versions_state_check CHECK ((state = ANY (ARRAY['PENDING'::text, 'EVALUATING'::text, 'READY'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.documents (
    id uuid DEFAULT uuidv7() NOT NULL,
    document_type text NOT NULL,
    name text NOT NULL,
    head_version_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    workspace_name text DEFAULT 'Main'::text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    deleted_at timestamp with time zone,
    folder_id uuid,
    last_opened_at timestamp with time zone,
    copied_from_document_id uuid,
    owner_user_id uuid NOT NULL,
    CONSTRAINT documents_document_type_check CHECK ((document_type = ANY (ARRAY['PART'::text, 'PRODUCT'::text])))
);

CREATE TABLE occccad.domain_transactions (
    id uuid DEFAULT uuidv7() NOT NULL,
    workspace_id uuid NOT NULL,
    sequence bigint NOT NULL,
    actor_id uuid NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    kind text NOT NULL,
    status text NOT NULL,
    base_revision_id uuid,
    result_revision_id uuid,
    root_transaction_id uuid,
    reverts_transaction_id uuid,
    reapplies_transaction_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    committed_at timestamp with time zone,
    product_design_transaction_id uuid,
    CONSTRAINT domain_transactions_check CHECK ((((kind = ANY (ARRAY['DOMAIN'::text, 'RESTORE'::text, 'CREATE'::text])) AND (root_transaction_id IS NULL) AND (reverts_transaction_id IS NULL) AND (reapplies_transaction_id IS NULL)) OR ((kind = 'REVERT'::text) AND (root_transaction_id IS NOT NULL) AND (reverts_transaction_id = root_transaction_id) AND (reapplies_transaction_id IS NULL)) OR ((kind = 'REAPPLY'::text) AND (root_transaction_id IS NOT NULL) AND (reverts_transaction_id IS NULL) AND (reapplies_transaction_id IS NOT NULL)))),
    CONSTRAINT domain_transactions_kind_check CHECK ((kind = ANY (ARRAY['DOMAIN'::text, 'REVERT'::text, 'REAPPLY'::text, 'RESTORE'::text, 'CREATE'::text]))),
    CONSTRAINT domain_transactions_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT domain_transactions_status_check CHECK ((status = ANY (ARRAY['RECEIVED'::text, 'PREPARED'::text, 'EVALUATING'::text, 'COMMITTED'::text, 'REJECTED'::text, 'CONFLICT'::text, 'FAILED'::text, 'CANCELLED'::text])))
);

CREATE TABLE occccad.evaluation_runs (
    id uuid DEFAULT uuidv7() NOT NULL,
    revision_id uuid NOT NULL,
    capability text NOT NULL,
    evaluator_digest text NOT NULL,
    input_digest text NOT NULL,
    manifest jsonb NOT NULL,
    manifest_digest text NOT NULL,
    status text NOT NULL,
    authoritative boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT evaluation_runs_status_check CHECK ((status = ANY (ARRAY['SUCCEEDED'::text, 'PARTIAL'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.folders (
    id uuid DEFAULT uuidv7() NOT NULL,
    parent_id uuid,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    owner_user_id uuid NOT NULL,
    deleted_at timestamp with time zone,
    trashed_by_folder_id uuid,
    CONSTRAINT folders_check CHECK (((parent_id IS NULL) OR (parent_id <> id))),
    CONSTRAINT folders_name_check CHECK (((length(btrim(name)) >= 1) AND (length(btrim(name)) <= 120)))
);

CREATE TABLE occccad.geometry_artifacts (
    geometry_key text NOT NULL,
    geometry_id text NOT NULL,
    evaluator_version text NOT NULL,
    occt_version text NOT NULL,
    units text NOT NULL,
    triangle_count bigint DEFAULT 0 NOT NULL,
    display_vertex_count bigint DEFAULT 0 NOT NULL,
    bbox_json jsonb NOT NULL,
    topology_json jsonb NOT NULL,
    volume double precision NOT NULL,
    evaluation_count integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    reference_geometry_json jsonb DEFAULT '{"primitives": [], "schemaVersion": 1, "referenceGeometry": {"axisSystems": [], "datumPlanes": []}}'::jsonb NOT NULL,
    worker_id text DEFAULT 'metadata-service'::text NOT NULL,
    CONSTRAINT geometry_artifacts_evaluation_count_check CHECK ((evaluation_count > 0)),
    CONSTRAINT geometry_artifacts_units_check CHECK ((units = 'mm'::text)),
    CONSTRAINT geometry_artifacts_volume_check CHECK ((volume >= (0)::double precision)),
    CONSTRAINT reference_geometry_has_no_display_payload CHECK ((((reference_geometry_json -> 'primitives'::text) IS NULL) OR ((reference_geometry_json -> 'primitives'::text) = ANY (ARRAY['[]'::jsonb, 'null'::jsonb]))))
);

COMMENT ON COLUMN occccad.geometry_artifacts.reference_geometry_json IS 'Bounded business reference geometry only. Display primitives are authoritative in the VISUAL GLB artifact.';

COMMENT ON COLUMN occccad.geometry_artifacts.worker_id IS 'Geometry worker that produced the solid, or metadata-service for visualization-only artifacts.';

CREATE TABLE occccad.geometry_representations (
    geometry_key text NOT NULL,
    role text NOT NULL,
    schema_version integer NOT NULL,
    object_id uuid NOT NULL,
    CONSTRAINT geometry_representations_schema_version_check CHECK ((schema_version > 0))
);

CREATE TABLE occccad.import_definitions (
    id text NOT NULL,
    document_id uuid NOT NULL,
    source_geometry_key text NOT NULL,
    source_object_id uuid,
    definition jsonb NOT NULL,
    identity_object_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT import_definitions_definition_check CHECK ((NOT (definition ? 'identities'::text)))
);

CREATE TABLE occccad.job_attempts (
    id uuid DEFAULT uuidv7() NOT NULL,
    job_id uuid NOT NULL,
    attempt integer NOT NULL,
    worker_id text NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone,
    result text,
    error_code text,
    error_message text,
    CONSTRAINT job_attempts_result_check CHECK ((result = ANY (ARRAY['SUCCEEDED'::text, 'FAILED'::text, 'LEASE_EXPIRED'::text, 'CANCELED'::text])))
);

CREATE TABLE occccad.jobs (
    id uuid DEFAULT uuidv7() NOT NULL,
    job_type text NOT NULL,
    state text DEFAULT 'QUEUED'::text NOT NULL,
    document_id uuid,
    version_id uuid,
    requested_by_user_id uuid NOT NULL,
    input_object_id uuid,
    result_object_id uuid,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    idempotency_key text NOT NULL,
    priority smallint DEFAULT 0 NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 3 NOT NULL,
    available_at timestamp with time zone DEFAULT now() NOT NULL,
    lease_owner text,
    lease_expires_at timestamp with time zone,
    heartbeat_at timestamp with time zone,
    progress smallint DEFAULT 0 NOT NULL,
    cancel_requested_at timestamp with time zone,
    error_code text,
    error_message text,
    trace_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    user_visible boolean DEFAULT false NOT NULL,
    CONSTRAINT jobs_job_type_check CHECK ((job_type = ANY (ARRAY['EXCHANGE_IMPORT'::text, 'EXCHANGE_EXPORT'::text, 'THUMBNAIL_RENDER'::text, 'ARTIFACT_BACKFILL'::text, 'MOTION_STUDY'::text]))),
    CONSTRAINT jobs_max_attempts_check CHECK (((max_attempts >= 1) AND (max_attempts <= 20))),
    CONSTRAINT jobs_progress_check CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT jobs_state_check CHECK ((state = ANY (ARRAY['QUEUED'::text, 'RUNNING'::text, 'RETRY_WAIT'::text, 'SUCCEEDED'::text, 'FAILED'::text, 'CANCELED'::text])))
);

CREATE TABLE occccad.outbox_events (
    id uuid DEFAULT uuidv7() NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL,
    schema_version integer NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    published_at timestamp with time zone,
    CONSTRAINT outbox_events_schema_version_check CHECK ((schema_version > 0))
);

CREATE TABLE occccad.product_context_variants (
    variant_key text NOT NULL,
    base_document_id uuid NOT NULL,
    base_revision_id uuid NOT NULL,
    binding_digest text NOT NULL,
    evaluator_version text NOT NULL,
    evaluation_manifest_digest text NOT NULL,
    bodies jsonb NOT NULL,
    publications jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE occccad.product_design_transactions (
    id uuid DEFAULT uuidv7() NOT NULL,
    root_product_document_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    status text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    committed_at timestamp with time zone,
    CONSTRAINT product_design_transactions_status_check CHECK ((status = ANY (ARRAY['COMMITTED'::text, 'REJECTED'::text, 'CONFLICT'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.product_instances (
    id uuid DEFAULT uuidv7() NOT NULL,
    product_version_id uuid NOT NULL,
    instance_key text NOT NULL,
    display_name text NOT NULL,
    referenced_document_id uuid NOT NULL,
    referenced_version_id uuid NOT NULL,
    translation_x double precision DEFAULT 0 NOT NULL,
    translation_y double precision DEFAULT 0 NOT NULL,
    translation_z double precision DEFAULT 0 NOT NULL,
    CONSTRAINT product_instances_check CHECK ((referenced_version_id <> product_version_id))
);

CREATE TABLE occccad.product_releases (
    id uuid DEFAULT uuidv7() NOT NULL,
    root_product_document_id uuid NOT NULL,
    root_product_revision_id uuid NOT NULL,
    name text NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    manifest_digest text NOT NULL,
    manifest jsonb NOT NULL,
    gate_status text NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT product_releases_gate_status_check CHECK ((gate_status = ANY (ARRAY['PASSED'::text, 'FAILED'::text])))
);

CREATE TABLE occccad.product_solve_manifests (
    digest text NOT NULL,
    schema_version integer NOT NULL,
    root_product_document_id uuid NOT NULL,
    root_product_revision_id uuid NOT NULL,
    manifest jsonb NOT NULL,
    solver_build_policy text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT product_solve_manifests_schema_version_check CHECK ((schema_version > 0))
);

CREATE TABLE occccad.product_solve_results (
    id uuid DEFAULT uuidv7() NOT NULL,
    manifest_digest text NOT NULL,
    request_id text NOT NULL,
    result jsonb,
    result_digest text,
    status text NOT NULL,
    diagnostic text,
    solver_build text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    completed_at timestamp with time zone
);

CREATE TABLE occccad.resource_grants (
    id uuid DEFAULT uuidv7() NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    user_id uuid,
    team_id uuid,
    role text NOT NULL,
    granted_by_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT resource_grants_check CHECK (((((user_id IS NOT NULL))::integer + ((team_id IS NOT NULL))::integer) = 1)),
    CONSTRAINT resource_grants_resource_type_check CHECK ((resource_type = ANY (ARRAY['DOCUMENT'::text, 'FOLDER'::text]))),
    CONSTRAINT resource_grants_role_check CHECK ((role = ANY (ARRAY['VIEWER'::text, 'EDITOR'::text])))
);

CREATE TABLE occccad.revision_parents (
    revision_id uuid NOT NULL,
    parent_revision_id uuid NOT NULL,
    ordinal smallint DEFAULT 0 NOT NULL
);

CREATE TABLE occccad.team_members (
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text DEFAULT 'MEMBER'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT team_members_role_check CHECK ((role = ANY (ARRAY['MEMBER'::text, 'ADMIN'::text])))
);

CREATE TABLE occccad.teams (
    id uuid DEFAULT uuidv7() NOT NULL,
    name text NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    owner_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT teams_name_check CHECK (((length(btrim(name)) >= 1) AND (length(btrim(name)) <= 120)))
);

CREATE TABLE occccad.transaction_commands (
    transaction_id uuid NOT NULL,
    ordinal smallint NOT NULL,
    command_id text NOT NULL,
    type_uri text NOT NULL,
    schema_version integer NOT NULL,
    payload jsonb NOT NULL,
    payload_digest text NOT NULL,
    CONSTRAINT transaction_commands_ordinal_check CHECK ((ordinal >= 0)),
    CONSTRAINT transaction_commands_schema_version_check CHECK ((schema_version > 0))
);

CREATE TABLE occccad.user_sessions (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    token_hash character(64) NOT NULL,
    csrf_hash character(64) NOT NULL,
    user_agent text DEFAULT ''::text NOT NULL,
    remote_address text DEFAULT ''::text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    last_seen_at timestamp with time zone DEFAULT now() NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE occccad.users (
    id uuid DEFAULT uuidv7() NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'ACTIVE'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    platform_role text DEFAULT 'MEMBER'::text NOT NULL,
    password_hash text,
    must_change_password boolean DEFAULT false NOT NULL,
    approved_at timestamp with time zone,
    approved_by_user_id uuid,
    failed_login_count integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    CONSTRAINT users_display_name_check CHECK (((length(btrim(display_name)) >= 1) AND (length(btrim(display_name)) <= 120))),
    CONSTRAINT users_platform_role_check CHECK ((platform_role = ANY (ARRAY['ADMIN'::text, 'MEMBER'::text]))),
    CONSTRAINT users_status_check CHECK ((status = ANY (ARRAY['PENDING'::text, 'ACTIVE'::text, 'DISABLED'::text])))
);

CREATE TABLE occccad.workspaces (
    id uuid DEFAULT uuidv7() NOT NULL,
    document_id uuid NOT NULL,
    name text NOT NULL,
    head_revision_id uuid NOT NULL,
    head_sequence bigint NOT NULL,
    base_revision_id uuid NOT NULL,
    policy jsonb DEFAULT '{"evaluation": "IMMEDIATE_ALLOW_FEATURE_FAILURE"}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT workspaces_head_sequence_check CHECK ((head_sequence >= 0))
);

ALTER TABLE ONLY occccad.access_audit_events
    ADD CONSTRAINT access_audit_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.account_audit_events
    ADD CONSTRAINT account_audit_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.artifact_objects
    ADD CONSTRAINT artifact_objects_kind_sha256_key UNIQUE (kind, sha256);

ALTER TABLE ONLY occccad.artifact_objects
    ADD CONSTRAINT artifact_objects_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.change_sets
    ADD CONSTRAINT change_sets_pkey PRIMARY KEY (transaction_id);

ALTER TABLE ONLY occccad.commands
    ADD CONSTRAINT commands_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.commands
    ADD CONSTRAINT commands_request_id_key UNIQUE (request_id);

ALTER TABLE ONLY occccad.dependency_edges
    ADD CONSTRAINT dependency_edges_pkey PRIMARY KEY (revision_id, source_key, target_key, edge_kind);

ALTER TABLE ONLY occccad.document_changes
    ADD CONSTRAINT document_changes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.document_history
    ADD CONSTRAINT document_history_document_id_version_id_key UNIQUE (document_id, version_id);

ALTER TABLE ONLY occccad.document_history
    ADD CONSTRAINT document_history_pkey PRIMARY KEY (document_id, "position");

ALTER TABLE ONLY occccad.document_previews
    ADD CONSTRAINT document_previews_document_id_preview_identity_key UNIQUE (document_id, preview_identity);

ALTER TABLE ONLY occccad.document_previews
    ADD CONSTRAINT document_previews_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.document_versions
    ADD CONSTRAINT document_versions_document_id_sequence_key UNIQUE (document_id, sequence);

ALTER TABLE ONLY occccad.document_versions
    ADD CONSTRAINT document_versions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.documents
    ADD CONSTRAINT documents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_workspace_id_request_id_key UNIQUE (workspace_id, request_id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_workspace_id_sequence_key UNIQUE (workspace_id, sequence);

ALTER TABLE ONLY occccad.evaluation_runs
    ADD CONSTRAINT evaluation_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.evaluation_runs
    ADD CONSTRAINT evaluation_runs_revision_id_capability_evaluator_digest_inp_key UNIQUE (revision_id, capability, evaluator_digest, input_digest);

ALTER TABLE ONLY occccad.folders
    ADD CONSTRAINT folders_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.geometry_artifacts
    ADD CONSTRAINT geometry_artifacts_pkey PRIMARY KEY (geometry_key);

ALTER TABLE ONLY occccad.geometry_representations
    ADD CONSTRAINT geometry_representations_pkey PRIMARY KEY (geometry_key, role);

ALTER TABLE ONLY occccad.import_definitions
    ADD CONSTRAINT import_definitions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.job_attempts
    ADD CONSTRAINT job_attempts_job_id_attempt_key UNIQUE (job_id, attempt);

ALTER TABLE ONLY occccad.job_attempts
    ADD CONSTRAINT job_attempts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_job_type_idempotency_key_key UNIQUE (job_type, idempotency_key);

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.product_context_variants
    ADD CONSTRAINT product_context_variants_pkey PRIMARY KEY (variant_key);

ALTER TABLE ONLY occccad.product_design_transactions
    ADD CONSTRAINT product_design_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.product_design_transactions
    ADD CONSTRAINT product_design_transactions_root_product_document_id_reques_key UNIQUE (root_product_document_id, request_id);

ALTER TABLE ONLY occccad.product_instances
    ADD CONSTRAINT product_instances_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.product_instances
    ADD CONSTRAINT product_instances_product_version_id_instance_key_key UNIQUE (product_version_id, instance_key);

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_root_product_document_id_name_key UNIQUE (root_product_document_id, name);

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_root_product_document_id_request_id_key UNIQUE (root_product_document_id, request_id);

ALTER TABLE ONLY occccad.product_solve_manifests
    ADD CONSTRAINT product_solve_manifests_pkey PRIMARY KEY (digest);

ALTER TABLE ONLY occccad.product_solve_results
    ADD CONSTRAINT product_solve_results_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.product_solve_results
    ADD CONSTRAINT product_solve_results_request_id_key UNIQUE (request_id);

ALTER TABLE ONLY occccad.resource_grants
    ADD CONSTRAINT resource_grants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.revision_parents
    ADD CONSTRAINT revision_parents_pkey PRIMARY KEY (revision_id, ordinal);

ALTER TABLE ONLY occccad.revision_parents
    ADD CONSTRAINT revision_parents_revision_id_parent_revision_id_key UNIQUE (revision_id, parent_revision_id);

ALTER TABLE ONLY occccad.team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (team_id, user_id);

ALTER TABLE ONLY occccad.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.transaction_commands
    ADD CONSTRAINT transaction_commands_pkey PRIMARY KEY (transaction_id, ordinal);

ALTER TABLE ONLY occccad.transaction_commands
    ADD CONSTRAINT transaction_commands_transaction_id_command_id_key UNIQUE (transaction_id, command_id);

ALTER TABLE ONLY occccad.user_sessions
    ADD CONSTRAINT user_sessions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.user_sessions
    ADD CONSTRAINT user_sessions_token_hash_key UNIQUE (token_hash);

ALTER TABLE ONLY occccad.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY occccad.workspaces
    ADD CONSTRAINT workspaces_document_id_name_key UNIQUE (document_id, name);

ALTER TABLE ONLY occccad.workspaces
    ADD CONSTRAINT workspaces_pkey PRIMARY KEY (id);

CREATE INDEX access_audit_actor_idx ON occccad.access_audit_events USING btree (actor_user_id, id DESC);

CREATE INDEX access_audit_resource_idx ON occccad.access_audit_events USING btree (resource_type, resource_id, id DESC);

CREATE INDEX account_audit_target_idx ON occccad.account_audit_events USING btree (target_user_id, id DESC);

CREATE INDEX commands_trace_id_idx ON occccad.commands USING btree (trace_id) WHERE (trace_id IS NOT NULL);

CREATE INDEX dependency_edges_source_idx ON occccad.dependency_edges USING btree (revision_id, source_key);

CREATE INDEX document_changes_document_idx ON occccad.document_changes USING btree (document_id, id DESC);

CREATE INDEX document_history_version_idx ON occccad.document_history USING btree (version_id);

CREATE INDEX document_previews_current_idx ON occccad.document_previews USING btree (document_id, updated_at DESC);

CREATE UNIQUE INDEX document_version_name_idx ON occccad.document_versions USING btree (document_id, version_name) WHERE (version_name IS NOT NULL);

CREATE INDEX document_versions_document_idx ON occccad.document_versions USING btree (document_id, sequence DESC);

CREATE INDEX documents_active_updated_idx ON occccad.documents USING btree (updated_at DESC) WHERE (deleted_at IS NULL);

CREATE INDEX documents_deleted_updated_idx ON occccad.documents USING btree (deleted_at DESC) WHERE (deleted_at IS NOT NULL);

CREATE INDEX documents_folder_type_idx ON occccad.documents USING btree (folder_id, document_type) WHERE (deleted_at IS NULL);

CREATE INDEX documents_folder_updated_idx ON occccad.documents USING btree (folder_id, updated_at DESC) WHERE (deleted_at IS NULL);

CREATE INDEX documents_name_search_idx ON occccad.documents USING btree (lower(name));

CREATE INDEX documents_recent_idx ON occccad.documents USING btree (last_opened_at DESC) WHERE ((deleted_at IS NULL) AND (last_opened_at IS NOT NULL));

CREATE INDEX domain_transactions_product_design_idx ON occccad.domain_transactions USING btree (product_design_transaction_id) WHERE (product_design_transaction_id IS NOT NULL);

CREATE INDEX domain_transactions_root_idx ON occccad.domain_transactions USING btree (root_transaction_id, sequence DESC) WHERE (root_transaction_id IS NOT NULL);

CREATE INDEX domain_transactions_workspace_idx ON occccad.domain_transactions USING btree (workspace_id, sequence DESC);

CREATE UNIQUE INDEX evaluation_runs_authoritative_idx ON occccad.evaluation_runs USING btree (revision_id, capability) WHERE authoritative;

CREATE UNIQUE INDEX folders_parent_name_idx ON occccad.folders USING btree (parent_id, lower(name)) NULLS NOT DISTINCT WHERE (deleted_at IS NULL);

CREATE INDEX folders_trash_roots_idx ON occccad.folders USING btree (deleted_at DESC) WHERE ((deleted_at IS NOT NULL) AND (trashed_by_folder_id = id));

CREATE INDEX jobs_claim_idx ON occccad.jobs USING btree (state, available_at, priority DESC, created_at);

CREATE INDEX jobs_document_idx ON occccad.jobs USING btree (document_id, created_at DESC);

CREATE INDEX jobs_lease_idx ON occccad.jobs USING btree (lease_expires_at) WHERE (state = 'RUNNING'::text);

CREATE INDEX jobs_user_visible_idx ON occccad.jobs USING btree (requested_by_user_id, created_at DESC) WHERE user_visible;

CREATE INDEX outbox_events_unpublished_idx ON occccad.outbox_events USING btree (created_at, id) WHERE (published_at IS NULL);

CREATE INDEX product_instances_version_idx ON occccad.product_instances USING btree (product_version_id);

CREATE INDEX product_solve_results_manifest_idx ON occccad.product_solve_results USING btree (manifest_digest, created_at DESC);

CREATE INDEX resource_grants_resource_idx ON occccad.resource_grants USING btree (resource_type, resource_id);

CREATE UNIQUE INDEX resource_grants_team_idx ON occccad.resource_grants USING btree (resource_type, resource_id, team_id) WHERE (team_id IS NOT NULL);

CREATE UNIQUE INDEX resource_grants_user_idx ON occccad.resource_grants USING btree (resource_type, resource_id, user_id) WHERE (user_id IS NOT NULL);

CREATE INDEX user_sessions_active_idx ON occccad.user_sessions USING btree (token_hash, expires_at) WHERE (revoked_at IS NULL);

CREATE INDEX user_sessions_user_idx ON occccad.user_sessions USING btree (user_id, created_at DESC);

CREATE UNIQUE INDEX users_email_idx ON occccad.users USING btree (lower(email));

ALTER TABLE ONLY occccad.access_audit_events
    ADD CONSTRAINT access_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES occccad.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.account_audit_events
    ADD CONSTRAINT account_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES occccad.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.account_audit_events
    ADD CONSTRAINT account_audit_events_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES occccad.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.change_sets
    ADD CONSTRAINT change_sets_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES occccad.domain_transactions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.commands
    ADD CONSTRAINT commands_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.dependency_edges
    ADD CONSTRAINT dependency_edges_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_changes
    ADD CONSTRAINT document_changes_command_id_fkey FOREIGN KEY (command_id) REFERENCES occccad.commands(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.document_changes
    ADD CONSTRAINT document_changes_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_changes
    ADD CONSTRAINT document_changes_version_id_fkey FOREIGN KEY (version_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_history
    ADD CONSTRAINT document_history_command_id_fkey FOREIGN KEY (command_id) REFERENCES occccad.commands(id);

ALTER TABLE ONLY occccad.document_history
    ADD CONSTRAINT document_history_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_history
    ADD CONSTRAINT document_history_version_id_fkey FOREIGN KEY (version_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.document_previews
    ADD CONSTRAINT document_previews_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_previews
    ADD CONSTRAINT document_previews_object_id_fkey FOREIGN KEY (object_id) REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.document_previews
    ADD CONSTRAINT document_previews_version_id_fkey FOREIGN KEY (version_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_versions
    ADD CONSTRAINT document_versions_created_by_command_id_fkey FOREIGN KEY (created_by_command_id) REFERENCES occccad.commands(id);

ALTER TABLE ONLY occccad.document_versions
    ADD CONSTRAINT document_versions_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.document_versions
    ADD CONSTRAINT document_versions_parent_version_id_fkey FOREIGN KEY (parent_version_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.documents
    ADD CONSTRAINT documents_copied_from_document_id_fkey FOREIGN KEY (copied_from_document_id) REFERENCES occccad.documents(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.documents
    ADD CONSTRAINT documents_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES occccad.folders(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.documents
    ADD CONSTRAINT documents_head_version_fk FOREIGN KEY (head_version_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.documents
    ADD CONSTRAINT documents_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_product_design_transaction_id_fkey FOREIGN KEY (product_design_transaction_id) REFERENCES occccad.product_design_transactions(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_reapplies_transaction_id_fkey FOREIGN KEY (reapplies_transaction_id) REFERENCES occccad.domain_transactions(id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_result_revision_id_fkey FOREIGN KEY (result_revision_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_reverts_transaction_id_fkey FOREIGN KEY (reverts_transaction_id) REFERENCES occccad.domain_transactions(id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_root_transaction_id_fkey FOREIGN KEY (root_transaction_id) REFERENCES occccad.domain_transactions(id);

ALTER TABLE ONLY occccad.domain_transactions
    ADD CONSTRAINT domain_transactions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES occccad.workspaces(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.evaluation_runs
    ADD CONSTRAINT evaluation_runs_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.folders
    ADD CONSTRAINT folders_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.folders
    ADD CONSTRAINT folders_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES occccad.folders(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.geometry_representations
    ADD CONSTRAINT geometry_representations_geometry_key_fkey FOREIGN KEY (geometry_key) REFERENCES occccad.geometry_artifacts(geometry_key) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.geometry_representations
    ADD CONSTRAINT geometry_representations_object_id_fkey FOREIGN KEY (object_id) REFERENCES occccad.artifact_objects(id);

ALTER TABLE ONLY occccad.import_definitions
    ADD CONSTRAINT import_definitions_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id);

ALTER TABLE ONLY occccad.import_definitions
    ADD CONSTRAINT import_definitions_identity_object_id_fkey FOREIGN KEY (identity_object_id) REFERENCES occccad.artifact_objects(id);

ALTER TABLE ONLY occccad.import_definitions
    ADD CONSTRAINT import_definitions_source_geometry_key_fkey FOREIGN KEY (source_geometry_key) REFERENCES occccad.geometry_artifacts(geometry_key);

ALTER TABLE ONLY occccad.import_definitions
    ADD CONSTRAINT import_definitions_source_object_id_fkey FOREIGN KEY (source_object_id) REFERENCES occccad.artifact_objects(id);

ALTER TABLE ONLY occccad.job_attempts
    ADD CONSTRAINT job_attempts_job_id_fkey FOREIGN KEY (job_id) REFERENCES occccad.jobs(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_input_object_id_fkey FOREIGN KEY (input_object_id) REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_requested_by_user_id_fkey FOREIGN KEY (requested_by_user_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_result_object_id_fkey FOREIGN KEY (result_object_id) REFERENCES occccad.artifact_objects(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.jobs
    ADD CONSTRAINT jobs_version_id_fkey FOREIGN KEY (version_id) REFERENCES occccad.document_versions(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.product_context_variants
    ADD CONSTRAINT product_context_variants_base_document_id_fkey FOREIGN KEY (base_document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.product_context_variants
    ADD CONSTRAINT product_context_variants_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES occccad.document_versions(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.product_design_transactions
    ADD CONSTRAINT product_design_transactions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.product_design_transactions
    ADD CONSTRAINT product_design_transactions_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.product_instances
    ADD CONSTRAINT product_instances_product_version_id_fkey FOREIGN KEY (product_version_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.product_instances
    ADD CONSTRAINT product_instances_referenced_document_id_fkey FOREIGN KEY (referenced_document_id) REFERENCES occccad.documents(id);

ALTER TABLE ONLY occccad.product_instances
    ADD CONSTRAINT product_instances_referenced_version_id_fkey FOREIGN KEY (referenced_version_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_created_by_fkey FOREIGN KEY (created_by) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.product_releases
    ADD CONSTRAINT product_releases_root_product_revision_id_fkey FOREIGN KEY (root_product_revision_id) REFERENCES occccad.document_versions(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.product_solve_manifests
    ADD CONSTRAINT product_solve_manifests_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.product_solve_results
    ADD CONSTRAINT product_solve_results_manifest_digest_fkey FOREIGN KEY (manifest_digest) REFERENCES occccad.product_solve_manifests(digest) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.resource_grants
    ADD CONSTRAINT resource_grants_granted_by_user_id_fkey FOREIGN KEY (granted_by_user_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.resource_grants
    ADD CONSTRAINT resource_grants_team_id_fkey FOREIGN KEY (team_id) REFERENCES occccad.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.resource_grants
    ADD CONSTRAINT resource_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES occccad.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.revision_parents
    ADD CONSTRAINT revision_parents_parent_revision_id_fkey FOREIGN KEY (parent_revision_id) REFERENCES occccad.document_versions(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.revision_parents
    ADD CONSTRAINT revision_parents_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES occccad.document_versions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.team_members
    ADD CONSTRAINT team_members_team_id_fkey FOREIGN KEY (team_id) REFERENCES occccad.teams(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.team_members
    ADD CONSTRAINT team_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES occccad.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.teams
    ADD CONSTRAINT teams_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES occccad.users(id) ON DELETE RESTRICT;

ALTER TABLE ONLY occccad.transaction_commands
    ADD CONSTRAINT transaction_commands_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES occccad.domain_transactions(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.user_sessions
    ADD CONSTRAINT user_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES occccad.users(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.users
    ADD CONSTRAINT users_approved_by_user_id_fkey FOREIGN KEY (approved_by_user_id) REFERENCES occccad.users(id) ON DELETE SET NULL;

ALTER TABLE ONLY occccad.workspaces
    ADD CONSTRAINT workspaces_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES occccad.document_versions(id);

ALTER TABLE ONLY occccad.workspaces
    ADD CONSTRAINT workspaces_document_id_fkey FOREIGN KEY (document_id) REFERENCES occccad.documents(id) ON DELETE CASCADE;

ALTER TABLE ONLY occccad.workspaces
    ADD CONSTRAINT workspaces_head_revision_id_fkey FOREIGN KEY (head_revision_id) REFERENCES occccad.document_versions(id);

-- ACL functions use the domain relations above.

CREATE FUNCTION occccad.role_level(role_name text) RETURNS integer
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
    SELECT CASE role_name WHEN 'OWNER' THEN 30 WHEN 'EDITOR' THEN 20 WHEN 'VIEWER' THEN 10 ELSE 0 END
$$;

CREATE FUNCTION occccad.role_name(role_level integer) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
    SELECT CASE WHEN role_level >= 30 THEN 'OWNER' WHEN role_level >= 20 THEN 'EDITOR'
                WHEN role_level >= 10 THEN 'VIEWER' ELSE 'NONE' END
$$;

CREATE FUNCTION occccad.effective_folder_role(target_folder uuid, principal_user uuid) RETURNS integer
    LANGUAGE sql STABLE
    AS $$
    WITH RECURSIVE ancestors AS (
        SELECT id,parent_id,owner_user_id FROM occccad.folders WHERE id=target_folder
        UNION ALL
        SELECT f.id,f.parent_id,f.owner_user_id FROM occccad.folders f
        JOIN ancestors a ON f.id=a.parent_id
    ), candidates AS (
        SELECT CASE WHEN a.owner_user_id=principal_user THEN 30 ELSE 0 END AS level FROM ancestors a
        UNION ALL
        SELECT occccad.role_level(g.role) FROM occccad.resource_grants g
        JOIN ancestors a ON g.resource_type='FOLDER' AND g.resource_id=a.id
        WHERE g.user_id=principal_user OR EXISTS (
            SELECT 1 FROM occccad.team_members tm
            WHERE tm.team_id=g.team_id AND tm.user_id=principal_user)
    ) SELECT COALESCE(max(level),0) FROM candidates
$$;

CREATE FUNCTION occccad.effective_document_role(target_document uuid, principal_user uuid) RETURNS integer
    LANGUAGE sql STABLE
    AS $$
    WITH document_data AS (
        SELECT owner_user_id,folder_id FROM occccad.documents WHERE id=target_document
    ), candidates AS (
        SELECT CASE WHEN owner_user_id=principal_user THEN 30 ELSE 0 END AS level FROM document_data
        UNION ALL
        SELECT occccad.effective_folder_role(folder_id,principal_user)
        FROM document_data WHERE folder_id IS NOT NULL
        UNION ALL
        SELECT occccad.role_level(g.role) FROM occccad.resource_grants g
        WHERE g.resource_type='DOCUMENT' AND g.resource_id=target_document
          AND (g.user_id=principal_user OR EXISTS (
              SELECT 1 FROM occccad.team_members tm
              WHERE tm.team_id=g.team_id AND tm.user_id=principal_user))
    ) SELECT COALESCE(max(level),0) FROM candidates
$$;

-- Bootstrap administrator; credentials are initialized by the authentication service.

INSERT INTO occccad.users(id,email,display_name,status,platform_role,approved_at)
VALUES ('00000000-0000-7000-8000-000000000001','admin@occccad.local','Administrator','ACTIVE','ADMIN',now());

-- SQLite Local Mode current schema baseline. Keep paired with postgres_migrations/0001_baseline.sql.
-- UUID, JSON and boolean checks preserve the PostgreSQL domain constraints.

CREATE TABLE access_audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id TEXT,
    action text NOT NULL,
    resource_type text,
    resource_id TEXT,
    request_id text,
    trace_id text,
    metadata TEXT DEFAULT '{}' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT access_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CHECK (actor_user_id IS NULL OR (length(actor_user_id)=36 AND substr(actor_user_id,9,1)='-' AND substr(actor_user_id,14,1)='-' AND substr(actor_user_id,19,1)='-' AND substr(actor_user_id,24,1)='-' AND length(replace(actor_user_id,'-',''))=32 AND replace(actor_user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (resource_id IS NULL OR (length(resource_id)=36 AND substr(resource_id,9,1)='-' AND substr(resource_id,14,1)='-' AND substr(resource_id,19,1)='-' AND substr(resource_id,24,1)='-' AND length(replace(resource_id,'-',''))=32 AND replace(resource_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (metadata IS NULL OR json_valid(metadata))
);

CREATE TABLE account_audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id TEXT,
    target_user_id TEXT,
    action text NOT NULL,
    metadata TEXT DEFAULT '{}' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT account_audit_events_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT account_audit_events_target_user_id_fkey FOREIGN KEY (target_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CHECK (actor_user_id IS NULL OR (length(actor_user_id)=36 AND substr(actor_user_id,9,1)='-' AND substr(actor_user_id,14,1)='-' AND substr(actor_user_id,19,1)='-' AND substr(actor_user_id,24,1)='-' AND length(replace(actor_user_id,'-',''))=32 AND replace(actor_user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (target_user_id IS NULL OR (length(target_user_id)=36 AND substr(target_user_id,9,1)='-' AND substr(target_user_id,14,1)='-' AND substr(target_user_id,19,1)='-' AND substr(target_user_id,24,1)='-' AND length(replace(target_user_id,'-',''))=32 AND replace(target_user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (metadata IS NULL OR json_valid(metadata))
);

CREATE TABLE artifact_objects (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    kind text NOT NULL,
    sha256 TEXT NOT NULL,
    storage_backend text DEFAULT 'LOCAL' NOT NULL,
    object_key text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    state text DEFAULT 'READY' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    verified_at TEXT,
    CONSTRAINT artifact_objects_kind_check CHECK ((length(kind) > 0)),
    CONSTRAINT artifact_objects_size_bytes_check CHECK ((size_bytes >= 0)),
    CONSTRAINT artifact_objects_state_check CHECK ((state IN ('STAGING', 'READY', 'QUARANTINED', 'DELETING'))),
    CONSTRAINT artifact_objects_storage_backend_check CHECK ((length(storage_backend) > 0)),
    CONSTRAINT artifact_objects_kind_sha256_key UNIQUE (kind, sha256),
    CONSTRAINT artifact_objects_pkey PRIMARY KEY (id),
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE change_sets (
    transaction_id TEXT NOT NULL,
    canonical_blob TEXT NOT NULL,
    canonical_digest text NOT NULL,
    read_set TEXT DEFAULT '[]' NOT NULL,
    write_set TEXT DEFAULT '[]' NOT NULL,
    impact_seeds TEXT DEFAULT '[]' NOT NULL,
    CONSTRAINT change_sets_pkey PRIMARY KEY (transaction_id),
    CONSTRAINT change_sets_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES domain_transactions(id) ON DELETE CASCADE,
    CHECK (transaction_id IS NULL OR (length(transaction_id)=36 AND substr(transaction_id,9,1)='-' AND substr(transaction_id,14,1)='-' AND substr(transaction_id,19,1)='-' AND substr(transaction_id,24,1)='-' AND length(replace(transaction_id,'-',''))=32 AND replace(transaction_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (canonical_blob IS NULL OR json_valid(canonical_blob)),
    CHECK (read_set IS NULL OR json_valid(read_set)),
    CHECK (write_set IS NULL OR json_valid(write_set)),
    CHECK (impact_seeds IS NULL OR json_valid(impact_seeds))
);

CREATE TABLE commands (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    request_id text NOT NULL,
    command_type text NOT NULL,
    document_id TEXT,
    payload TEXT NOT NULL,
    status text NOT NULL,
    error_message text,
    created_at TEXT DEFAULT (now()) NOT NULL,
    completed_at TEXT,
    trace_id text,
    span_id text,
    CONSTRAINT commands_status_check CHECK ((status IN ('PENDING', 'SUCCEEDED', 'FAILED'))),
    CONSTRAINT commands_pkey PRIMARY KEY (id),
    CONSTRAINT commands_request_id_key UNIQUE (request_id),
    CONSTRAINT commands_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (payload IS NULL OR json_valid(payload))
);

CREATE TABLE dependency_edges (
    revision_id TEXT NOT NULL,
    source_key text NOT NULL,
    target_key text NOT NULL,
    edge_kind text NOT NULL,
    CONSTRAINT dependency_edges_edge_kind_check CHECK ((edge_kind IN ('READ_VALUE', 'READ_GEOMETRY', 'READ_TOPOLOGY', 'READ_STRUCTURE', 'READ_CONFIGURATION', 'READ_MATERIAL', 'READ_MEASUREMENT'))),
    CONSTRAINT dependency_edges_pkey PRIMARY KEY (revision_id, source_key, target_key, edge_kind),
    CONSTRAINT dependency_edges_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CHECK (revision_id IS NULL OR (length(revision_id)=36 AND substr(revision_id,9,1)='-' AND substr(revision_id,14,1)='-' AND substr(revision_id,19,1)='-' AND substr(revision_id,24,1)='-' AND length(replace(revision_id,'-',''))=32 AND replace(revision_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE document_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    document_id TEXT NOT NULL,
    version_id TEXT NOT NULL,
    command_id TEXT,
    change_type text NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT document_changes_command_id_fkey FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE SET NULL,
    CONSTRAINT document_changes_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT document_changes_version_id_fkey FOREIGN KEY (version_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (version_id IS NULL OR (length(version_id)=36 AND substr(version_id,9,1)='-' AND substr(version_id,14,1)='-' AND substr(version_id,19,1)='-' AND substr(version_id,24,1)='-' AND length(replace(version_id,'-',''))=32 AND replace(version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (command_id IS NULL OR (length(command_id)=36 AND substr(command_id,9,1)='-' AND substr(command_id,14,1)='-' AND substr(command_id,19,1)='-' AND substr(command_id,24,1)='-' AND length(replace(command_id,'-',''))=32 AND replace(command_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE document_history (
    document_id TEXT NOT NULL,
    "position" integer NOT NULL,
    version_id TEXT NOT NULL,
    command_id TEXT,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT document_history_position_check CHECK (("position" >= 0)),
    CONSTRAINT document_history_document_id_version_id_key UNIQUE (document_id, version_id),
    CONSTRAINT document_history_pkey PRIMARY KEY (document_id, "position"),
    CONSTRAINT document_history_command_id_fkey FOREIGN KEY (command_id) REFERENCES commands(id),
    CONSTRAINT document_history_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT document_history_version_id_fkey FOREIGN KEY (version_id) REFERENCES document_versions(id),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (version_id IS NULL OR (length(version_id)=36 AND substr(version_id,9,1)='-' AND substr(version_id,14,1)='-' AND substr(version_id,19,1)='-' AND substr(version_id,24,1)='-' AND length(replace(version_id,'-',''))=32 AND replace(version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (command_id IS NULL OR (length(command_id)=36 AND substr(command_id,9,1)='-' AND substr(command_id,14,1)='-' AND substr(command_id,19,1)='-' AND substr(command_id,24,1)='-' AND length(replace(command_id,'-',''))=32 AND replace(command_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE document_previews (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    document_id TEXT NOT NULL,
    version_id TEXT NOT NULL,
    preview_identity TEXT NOT NULL,
    renderer_version text NOT NULL,
    object_id TEXT,
    state text DEFAULT 'PENDING' NOT NULL,
    error_message text,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT document_previews_state_check CHECK ((state IN ('PENDING', 'READY', 'STALE', 'FAILED'))),
    CONSTRAINT document_previews_document_id_preview_identity_key UNIQUE (document_id, preview_identity),
    CONSTRAINT document_previews_pkey PRIMARY KEY (id),
    CONSTRAINT document_previews_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT document_previews_object_id_fkey FOREIGN KEY (object_id) REFERENCES artifact_objects(id) ON DELETE SET NULL,
    CONSTRAINT document_previews_version_id_fkey FOREIGN KEY (version_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (version_id IS NULL OR (length(version_id)=36 AND substr(version_id,9,1)='-' AND substr(version_id,14,1)='-' AND substr(version_id,19,1)='-' AND substr(version_id,24,1)='-' AND length(replace(version_id,'-',''))=32 AND replace(version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (object_id IS NULL OR (length(object_id)=36 AND substr(object_id,9,1)='-' AND substr(object_id,14,1)='-' AND substr(object_id,19,1)='-' AND substr(object_id,24,1)='-' AND length(replace(object_id,'-',''))=32 AND replace(object_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE document_versions (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    document_id TEXT NOT NULL,
    parent_version_id TEXT,
    sequence integer NOT NULL,
    model_json TEXT NOT NULL,
    state text NOT NULL,
    created_by_command_id TEXT,
    created_at TEXT DEFAULT (now()) NOT NULL,
    version_name text,
    version_description text,
    model_hash text NOT NULL,
    dependency_snapshot_digest text,
    evaluation_manifest TEXT,
    CONSTRAINT document_versions_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT document_versions_state_check CHECK ((state IN ('PENDING', 'EVALUATING', 'READY', 'FAILED'))),
    CONSTRAINT document_versions_document_id_sequence_key UNIQUE (document_id, sequence),
    CONSTRAINT document_versions_pkey PRIMARY KEY (id),
    CONSTRAINT document_versions_created_by_command_id_fkey FOREIGN KEY (created_by_command_id) REFERENCES commands(id),
    CONSTRAINT document_versions_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT document_versions_parent_version_id_fkey FOREIGN KEY (parent_version_id) REFERENCES document_versions(id),
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (parent_version_id IS NULL OR (length(parent_version_id)=36 AND substr(parent_version_id,9,1)='-' AND substr(parent_version_id,14,1)='-' AND substr(parent_version_id,19,1)='-' AND substr(parent_version_id,24,1)='-' AND length(replace(parent_version_id,'-',''))=32 AND replace(parent_version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (model_json IS NULL OR json_valid(model_json)),
    CHECK (created_by_command_id IS NULL OR (length(created_by_command_id)=36 AND substr(created_by_command_id,9,1)='-' AND substr(created_by_command_id,14,1)='-' AND substr(created_by_command_id,19,1)='-' AND substr(created_by_command_id,24,1)='-' AND length(replace(created_by_command_id,'-',''))=32 AND replace(created_by_command_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (evaluation_manifest IS NULL OR json_valid(evaluation_manifest))
);

CREATE TABLE documents (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    document_type text NOT NULL,
    name text NOT NULL,
    head_version_id TEXT,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    workspace_name text DEFAULT 'Main' NOT NULL,
    description text DEFAULT '' NOT NULL,
    deleted_at TEXT,
    folder_id TEXT,
    last_opened_at TEXT,
    copied_from_document_id TEXT,
    owner_user_id TEXT NOT NULL,
    CONSTRAINT documents_document_type_check CHECK ((document_type IN ('PART', 'PRODUCT'))),
    CONSTRAINT documents_pkey PRIMARY KEY (id),
    CONSTRAINT documents_copied_from_document_id_fkey FOREIGN KEY (copied_from_document_id) REFERENCES documents(id) ON DELETE SET NULL,
    CONSTRAINT documents_folder_id_fkey FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE SET NULL,
    CONSTRAINT documents_head_version_fk FOREIGN KEY (head_version_id) REFERENCES document_versions(id),
    CONSTRAINT documents_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (head_version_id IS NULL OR (length(head_version_id)=36 AND substr(head_version_id,9,1)='-' AND substr(head_version_id,14,1)='-' AND substr(head_version_id,19,1)='-' AND substr(head_version_id,24,1)='-' AND length(replace(head_version_id,'-',''))=32 AND replace(head_version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (folder_id IS NULL OR (length(folder_id)=36 AND substr(folder_id,9,1)='-' AND substr(folder_id,14,1)='-' AND substr(folder_id,19,1)='-' AND substr(folder_id,24,1)='-' AND length(replace(folder_id,'-',''))=32 AND replace(folder_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (copied_from_document_id IS NULL OR (length(copied_from_document_id)=36 AND substr(copied_from_document_id,9,1)='-' AND substr(copied_from_document_id,14,1)='-' AND substr(copied_from_document_id,19,1)='-' AND substr(copied_from_document_id,24,1)='-' AND length(replace(copied_from_document_id,'-',''))=32 AND replace(copied_from_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (owner_user_id IS NULL OR (length(owner_user_id)=36 AND substr(owner_user_id,9,1)='-' AND substr(owner_user_id,14,1)='-' AND substr(owner_user_id,19,1)='-' AND substr(owner_user_id,24,1)='-' AND length(replace(owner_user_id,'-',''))=32 AND replace(owner_user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE domain_transactions (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    workspace_id TEXT NOT NULL,
    sequence bigint NOT NULL,
    actor_id TEXT NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    kind text NOT NULL,
    status text NOT NULL,
    base_revision_id TEXT,
    result_revision_id TEXT,
    root_transaction_id TEXT,
    reverts_transaction_id TEXT,
    reapplies_transaction_id TEXT,
    created_at TEXT DEFAULT (now()) NOT NULL,
    committed_at TEXT,
    product_design_transaction_id TEXT,
    CONSTRAINT domain_transactions_check CHECK ((((kind IN ('DOMAIN', 'RESTORE', 'CREATE')) AND (root_transaction_id IS NULL) AND (reverts_transaction_id IS NULL) AND (reapplies_transaction_id IS NULL)) OR ((kind = 'REVERT') AND (root_transaction_id IS NOT NULL) AND (reverts_transaction_id = root_transaction_id) AND (reapplies_transaction_id IS NULL)) OR ((kind = 'REAPPLY') AND (root_transaction_id IS NOT NULL) AND (reverts_transaction_id IS NULL) AND (reapplies_transaction_id IS NOT NULL)))),
    CONSTRAINT domain_transactions_kind_check CHECK ((kind IN ('DOMAIN', 'REVERT', 'REAPPLY', 'RESTORE', 'CREATE'))),
    CONSTRAINT domain_transactions_sequence_check CHECK ((sequence > 0)),
    CONSTRAINT domain_transactions_status_check CHECK ((status IN ('RECEIVED', 'PREPARED', 'EVALUATING', 'COMMITTED', 'REJECTED', 'CONFLICT', 'FAILED', 'CANCELLED'))),
    CONSTRAINT domain_transactions_pkey PRIMARY KEY (id),
    CONSTRAINT domain_transactions_workspace_id_request_id_key UNIQUE (workspace_id, request_id),
    CONSTRAINT domain_transactions_workspace_id_sequence_key UNIQUE (workspace_id, sequence),
    CONSTRAINT domain_transactions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT domain_transactions_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES document_versions(id),
    CONSTRAINT domain_transactions_product_design_transaction_id_fkey FOREIGN KEY (product_design_transaction_id) REFERENCES product_design_transactions(id) ON DELETE RESTRICT,
    CONSTRAINT domain_transactions_reapplies_transaction_id_fkey FOREIGN KEY (reapplies_transaction_id) REFERENCES domain_transactions(id),
    CONSTRAINT domain_transactions_result_revision_id_fkey FOREIGN KEY (result_revision_id) REFERENCES document_versions(id),
    CONSTRAINT domain_transactions_reverts_transaction_id_fkey FOREIGN KEY (reverts_transaction_id) REFERENCES domain_transactions(id),
    CONSTRAINT domain_transactions_root_transaction_id_fkey FOREIGN KEY (root_transaction_id) REFERENCES domain_transactions(id),
    CONSTRAINT domain_transactions_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (workspace_id IS NULL OR (length(workspace_id)=36 AND substr(workspace_id,9,1)='-' AND substr(workspace_id,14,1)='-' AND substr(workspace_id,19,1)='-' AND substr(workspace_id,24,1)='-' AND length(replace(workspace_id,'-',''))=32 AND replace(workspace_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (actor_id IS NULL OR (length(actor_id)=36 AND substr(actor_id,9,1)='-' AND substr(actor_id,14,1)='-' AND substr(actor_id,19,1)='-' AND substr(actor_id,24,1)='-' AND length(replace(actor_id,'-',''))=32 AND replace(actor_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (base_revision_id IS NULL OR (length(base_revision_id)=36 AND substr(base_revision_id,9,1)='-' AND substr(base_revision_id,14,1)='-' AND substr(base_revision_id,19,1)='-' AND substr(base_revision_id,24,1)='-' AND length(replace(base_revision_id,'-',''))=32 AND replace(base_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (result_revision_id IS NULL OR (length(result_revision_id)=36 AND substr(result_revision_id,9,1)='-' AND substr(result_revision_id,14,1)='-' AND substr(result_revision_id,19,1)='-' AND substr(result_revision_id,24,1)='-' AND length(replace(result_revision_id,'-',''))=32 AND replace(result_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (root_transaction_id IS NULL OR (length(root_transaction_id)=36 AND substr(root_transaction_id,9,1)='-' AND substr(root_transaction_id,14,1)='-' AND substr(root_transaction_id,19,1)='-' AND substr(root_transaction_id,24,1)='-' AND length(replace(root_transaction_id,'-',''))=32 AND replace(root_transaction_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (reverts_transaction_id IS NULL OR (length(reverts_transaction_id)=36 AND substr(reverts_transaction_id,9,1)='-' AND substr(reverts_transaction_id,14,1)='-' AND substr(reverts_transaction_id,19,1)='-' AND substr(reverts_transaction_id,24,1)='-' AND length(replace(reverts_transaction_id,'-',''))=32 AND replace(reverts_transaction_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (reapplies_transaction_id IS NULL OR (length(reapplies_transaction_id)=36 AND substr(reapplies_transaction_id,9,1)='-' AND substr(reapplies_transaction_id,14,1)='-' AND substr(reapplies_transaction_id,19,1)='-' AND substr(reapplies_transaction_id,24,1)='-' AND length(replace(reapplies_transaction_id,'-',''))=32 AND replace(reapplies_transaction_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (product_design_transaction_id IS NULL OR (length(product_design_transaction_id)=36 AND substr(product_design_transaction_id,9,1)='-' AND substr(product_design_transaction_id,14,1)='-' AND substr(product_design_transaction_id,19,1)='-' AND substr(product_design_transaction_id,24,1)='-' AND length(replace(product_design_transaction_id,'-',''))=32 AND replace(product_design_transaction_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE evaluation_runs (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    revision_id TEXT NOT NULL,
    capability text NOT NULL,
    evaluator_digest text NOT NULL,
    input_digest text NOT NULL,
    manifest TEXT NOT NULL,
    manifest_digest text NOT NULL,
    status text NOT NULL,
    authoritative boolean DEFAULT false NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT evaluation_runs_status_check CHECK ((status IN ('SUCCEEDED', 'PARTIAL', 'FAILED'))),
    CONSTRAINT evaluation_runs_pkey PRIMARY KEY (id),
    CONSTRAINT evaluation_runs_revision_id_capability_evaluator_digest_inp_key UNIQUE (revision_id, capability, evaluator_digest, input_digest),
    CONSTRAINT evaluation_runs_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (revision_id IS NULL OR (length(revision_id)=36 AND substr(revision_id,9,1)='-' AND substr(revision_id,14,1)='-' AND substr(revision_id,19,1)='-' AND substr(revision_id,24,1)='-' AND length(replace(revision_id,'-',''))=32 AND replace(revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (manifest IS NULL OR json_valid(manifest)),
    CHECK (authoritative IS NULL OR authoritative IN (0,1))
);

CREATE TABLE folders (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    parent_id TEXT,
    name text NOT NULL,
    description text DEFAULT '' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    owner_user_id TEXT NOT NULL,
    deleted_at TEXT,
    trashed_by_folder_id TEXT,
    CONSTRAINT folders_check CHECK (((parent_id IS NULL) OR (parent_id <> id))),
    CONSTRAINT folders_name_check CHECK (((length(trim(name)) >= 1) AND (length(trim(name)) <= 120))),
    CONSTRAINT folders_pkey PRIMARY KEY (id),
    CONSTRAINT folders_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT folders_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE RESTRICT,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (parent_id IS NULL OR (length(parent_id)=36 AND substr(parent_id,9,1)='-' AND substr(parent_id,14,1)='-' AND substr(parent_id,19,1)='-' AND substr(parent_id,24,1)='-' AND length(replace(parent_id,'-',''))=32 AND replace(parent_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (owner_user_id IS NULL OR (length(owner_user_id)=36 AND substr(owner_user_id,9,1)='-' AND substr(owner_user_id,14,1)='-' AND substr(owner_user_id,19,1)='-' AND substr(owner_user_id,24,1)='-' AND length(replace(owner_user_id,'-',''))=32 AND replace(owner_user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (trashed_by_folder_id IS NULL OR (length(trashed_by_folder_id)=36 AND substr(trashed_by_folder_id,9,1)='-' AND substr(trashed_by_folder_id,14,1)='-' AND substr(trashed_by_folder_id,19,1)='-' AND substr(trashed_by_folder_id,24,1)='-' AND length(replace(trashed_by_folder_id,'-',''))=32 AND replace(trashed_by_folder_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE geometry_artifacts (
    geometry_key text NOT NULL,
    geometry_id text NOT NULL,
    evaluator_version text NOT NULL,
    occt_version text NOT NULL,
    units text NOT NULL,
    triangle_count bigint DEFAULT 0 NOT NULL,
    display_vertex_count bigint DEFAULT 0 NOT NULL,
    bbox_json TEXT NOT NULL,
    topology_json TEXT NOT NULL,
    volume REAL NOT NULL,
    evaluation_count integer DEFAULT 1 NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    reference_geometry_json TEXT DEFAULT '{"primitives": [], "schemaVersion": 1, "referenceGeometry": {"axisSystems": [], "datumPlanes": []}}' NOT NULL,
    worker_id text DEFAULT 'metadata-service' NOT NULL,
    CONSTRAINT geometry_artifacts_evaluation_count_check CHECK ((evaluation_count > 0)),
    CONSTRAINT geometry_artifacts_units_check CHECK ((units = 'mm')),
    CONSTRAINT geometry_artifacts_volume_check CHECK ((volume >= (0))),
    CONSTRAINT reference_geometry_has_no_display_payload CHECK ((((reference_geometry_json -> 'primitives') IS NULL) OR ((reference_geometry_json -> 'primitives') IN ('[]', 'null')))),
    CONSTRAINT geometry_artifacts_pkey PRIMARY KEY (geometry_key),
    CHECK (bbox_json IS NULL OR json_valid(bbox_json)),
    CHECK (topology_json IS NULL OR json_valid(topology_json)),
    CHECK (reference_geometry_json IS NULL OR json_valid(reference_geometry_json))
);

CREATE TABLE geometry_representations (
    geometry_key text NOT NULL,
    role text NOT NULL,
    schema_version integer NOT NULL,
    object_id TEXT NOT NULL,
    CONSTRAINT geometry_representations_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT geometry_representations_pkey PRIMARY KEY (geometry_key, role),
    CONSTRAINT geometry_representations_geometry_key_fkey FOREIGN KEY (geometry_key) REFERENCES geometry_artifacts(geometry_key) ON DELETE CASCADE,
    CONSTRAINT geometry_representations_object_id_fkey FOREIGN KEY (object_id) REFERENCES artifact_objects(id),
    CHECK (object_id IS NULL OR (length(object_id)=36 AND substr(object_id,9,1)='-' AND substr(object_id,14,1)='-' AND substr(object_id,19,1)='-' AND substr(object_id,24,1)='-' AND length(replace(object_id,'-',''))=32 AND replace(object_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE import_definitions (
    id text NOT NULL,
    document_id TEXT NOT NULL,
    source_geometry_key text NOT NULL,
    source_object_id TEXT,
    definition TEXT NOT NULL,
    identity_object_id TEXT NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT import_definitions_definition_check CHECK ((json_type(definition, '$.identities') IS NULL)),
    CONSTRAINT import_definitions_pkey PRIMARY KEY (id),
    CONSTRAINT import_definitions_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id),
    CONSTRAINT import_definitions_identity_object_id_fkey FOREIGN KEY (identity_object_id) REFERENCES artifact_objects(id),
    CONSTRAINT import_definitions_source_geometry_key_fkey FOREIGN KEY (source_geometry_key) REFERENCES geometry_artifacts(geometry_key),
    CONSTRAINT import_definitions_source_object_id_fkey FOREIGN KEY (source_object_id) REFERENCES artifact_objects(id),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (source_object_id IS NULL OR (length(source_object_id)=36 AND substr(source_object_id,9,1)='-' AND substr(source_object_id,14,1)='-' AND substr(source_object_id,19,1)='-' AND substr(source_object_id,24,1)='-' AND length(replace(source_object_id,'-',''))=32 AND replace(source_object_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (definition IS NULL OR json_valid(definition)),
    CHECK (identity_object_id IS NULL OR (length(identity_object_id)=36 AND substr(identity_object_id,9,1)='-' AND substr(identity_object_id,14,1)='-' AND substr(identity_object_id,19,1)='-' AND substr(identity_object_id,24,1)='-' AND length(replace(identity_object_id,'-',''))=32 AND replace(identity_object_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE job_attempts (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    job_id TEXT NOT NULL,
    attempt integer NOT NULL,
    worker_id text NOT NULL,
    started_at TEXT DEFAULT (now()) NOT NULL,
    completed_at TEXT,
    result text,
    error_code text,
    error_message text,
    CONSTRAINT job_attempts_result_check CHECK ((result IN ('SUCCEEDED', 'FAILED', 'LEASE_EXPIRED', 'CANCELED'))),
    CONSTRAINT job_attempts_job_id_attempt_key UNIQUE (job_id, attempt),
    CONSTRAINT job_attempts_pkey PRIMARY KEY (id),
    CONSTRAINT job_attempts_job_id_fkey FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (job_id IS NULL OR (length(job_id)=36 AND substr(job_id,9,1)='-' AND substr(job_id,14,1)='-' AND substr(job_id,19,1)='-' AND substr(job_id,24,1)='-' AND length(replace(job_id,'-',''))=32 AND replace(job_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE jobs (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    job_type text NOT NULL,
    state text DEFAULT 'QUEUED' NOT NULL,
    document_id TEXT,
    version_id TEXT,
    requested_by_user_id TEXT NOT NULL,
    input_object_id TEXT,
    result_object_id TEXT,
    payload TEXT DEFAULT '{}' NOT NULL,
    idempotency_key text NOT NULL,
    priority smallint DEFAULT 0 NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    max_attempts integer DEFAULT 3 NOT NULL,
    available_at TEXT DEFAULT (now()) NOT NULL,
    lease_owner text,
    lease_expires_at TEXT,
    heartbeat_at TEXT,
    progress smallint DEFAULT 0 NOT NULL,
    cancel_requested_at TEXT,
    error_code text,
    error_message text,
    trace_id text,
    created_at TEXT DEFAULT (now()) NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    user_visible boolean DEFAULT false NOT NULL,
    CONSTRAINT jobs_job_type_check CHECK ((job_type IN ('EXCHANGE_IMPORT', 'EXCHANGE_EXPORT', 'THUMBNAIL_RENDER', 'ARTIFACT_BACKFILL'))),
    CONSTRAINT jobs_max_attempts_check CHECK (((max_attempts >= 1) AND (max_attempts <= 20))),
    CONSTRAINT jobs_progress_check CHECK (((progress >= 0) AND (progress <= 100))),
    CONSTRAINT jobs_state_check CHECK ((state IN ('QUEUED', 'RUNNING', 'RETRY_WAIT', 'SUCCEEDED', 'FAILED', 'CANCELED'))),
    CONSTRAINT jobs_job_type_idempotency_key_key UNIQUE (job_type, idempotency_key),
    CONSTRAINT jobs_pkey PRIMARY KEY (id),
    CONSTRAINT jobs_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT jobs_input_object_id_fkey FOREIGN KEY (input_object_id) REFERENCES artifact_objects(id) ON DELETE SET NULL,
    CONSTRAINT jobs_requested_by_user_id_fkey FOREIGN KEY (requested_by_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT jobs_result_object_id_fkey FOREIGN KEY (result_object_id) REFERENCES artifact_objects(id) ON DELETE SET NULL,
    CONSTRAINT jobs_version_id_fkey FOREIGN KEY (version_id) REFERENCES document_versions(id) ON DELETE SET NULL,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (version_id IS NULL OR (length(version_id)=36 AND substr(version_id,9,1)='-' AND substr(version_id,14,1)='-' AND substr(version_id,19,1)='-' AND substr(version_id,24,1)='-' AND length(replace(version_id,'-',''))=32 AND replace(version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (requested_by_user_id IS NULL OR (length(requested_by_user_id)=36 AND substr(requested_by_user_id,9,1)='-' AND substr(requested_by_user_id,14,1)='-' AND substr(requested_by_user_id,19,1)='-' AND substr(requested_by_user_id,24,1)='-' AND length(replace(requested_by_user_id,'-',''))=32 AND replace(requested_by_user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (input_object_id IS NULL OR (length(input_object_id)=36 AND substr(input_object_id,9,1)='-' AND substr(input_object_id,14,1)='-' AND substr(input_object_id,19,1)='-' AND substr(input_object_id,24,1)='-' AND length(replace(input_object_id,'-',''))=32 AND replace(input_object_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (result_object_id IS NULL OR (length(result_object_id)=36 AND substr(result_object_id,9,1)='-' AND substr(result_object_id,14,1)='-' AND substr(result_object_id,19,1)='-' AND substr(result_object_id,24,1)='-' AND length(replace(result_object_id,'-',''))=32 AND replace(result_object_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (payload IS NULL OR json_valid(payload)),
    CHECK (user_visible IS NULL OR user_visible IN (0,1))
);

CREATE TABLE outbox_events (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type text NOT NULL,
    schema_version integer NOT NULL,
    payload TEXT NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    published_at TEXT,
    CONSTRAINT outbox_events_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT outbox_events_pkey PRIMARY KEY (id),
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (aggregate_id IS NULL OR (length(aggregate_id)=36 AND substr(aggregate_id,9,1)='-' AND substr(aggregate_id,14,1)='-' AND substr(aggregate_id,19,1)='-' AND substr(aggregate_id,24,1)='-' AND length(replace(aggregate_id,'-',''))=32 AND replace(aggregate_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (payload IS NULL OR json_valid(payload))
);

CREATE TABLE product_context_variants (
    variant_key text NOT NULL,
    base_document_id TEXT NOT NULL,
    base_revision_id TEXT NOT NULL,
    binding_digest text NOT NULL,
    evaluator_version text NOT NULL,
    evaluation_manifest_digest text NOT NULL,
    bodies TEXT NOT NULL,
    publications TEXT DEFAULT '[]' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT product_context_variants_pkey PRIMARY KEY (variant_key),
    CONSTRAINT product_context_variants_base_document_id_fkey FOREIGN KEY (base_document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT product_context_variants_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES document_versions(id) ON DELETE RESTRICT,
    CHECK (base_document_id IS NULL OR (length(base_document_id)=36 AND substr(base_document_id,9,1)='-' AND substr(base_document_id,14,1)='-' AND substr(base_document_id,19,1)='-' AND substr(base_document_id,24,1)='-' AND length(replace(base_document_id,'-',''))=32 AND replace(base_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (base_revision_id IS NULL OR (length(base_revision_id)=36 AND substr(base_revision_id,9,1)='-' AND substr(base_revision_id,14,1)='-' AND substr(base_revision_id,19,1)='-' AND substr(base_revision_id,24,1)='-' AND length(replace(base_revision_id,'-',''))=32 AND replace(base_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (bodies IS NULL OR json_valid(bodies)),
    CHECK (publications IS NULL OR json_valid(publications))
);

CREATE TABLE product_design_transactions (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    root_product_document_id TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    status text NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    committed_at TEXT,
    CONSTRAINT product_design_transactions_status_check CHECK ((status IN ('COMMITTED', 'REJECTED', 'CONFLICT', 'FAILED'))),
    CONSTRAINT product_design_transactions_pkey PRIMARY KEY (id),
    CONSTRAINT product_design_transactions_root_product_document_id_reques_key UNIQUE (root_product_document_id, request_id),
    CONSTRAINT product_design_transactions_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT product_design_transactions_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (root_product_document_id IS NULL OR (length(root_product_document_id)=36 AND substr(root_product_document_id,9,1)='-' AND substr(root_product_document_id,14,1)='-' AND substr(root_product_document_id,19,1)='-' AND substr(root_product_document_id,24,1)='-' AND length(replace(root_product_document_id,'-',''))=32 AND replace(root_product_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (actor_id IS NULL OR (length(actor_id)=36 AND substr(actor_id,9,1)='-' AND substr(actor_id,14,1)='-' AND substr(actor_id,19,1)='-' AND substr(actor_id,24,1)='-' AND length(replace(actor_id,'-',''))=32 AND replace(actor_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE product_instances (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    product_version_id TEXT NOT NULL,
    instance_key text NOT NULL,
    display_name text NOT NULL,
    referenced_document_id TEXT NOT NULL,
    referenced_version_id TEXT NOT NULL,
    translation_x REAL DEFAULT 0 NOT NULL,
    translation_y REAL DEFAULT 0 NOT NULL,
    translation_z REAL DEFAULT 0 NOT NULL,
    CONSTRAINT product_instances_check CHECK ((referenced_version_id <> product_version_id)),
    CONSTRAINT product_instances_pkey PRIMARY KEY (id),
    CONSTRAINT product_instances_product_version_id_instance_key_key UNIQUE (product_version_id, instance_key),
    CONSTRAINT product_instances_product_version_id_fkey FOREIGN KEY (product_version_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CONSTRAINT product_instances_referenced_document_id_fkey FOREIGN KEY (referenced_document_id) REFERENCES documents(id),
    CONSTRAINT product_instances_referenced_version_id_fkey FOREIGN KEY (referenced_version_id) REFERENCES document_versions(id),
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (product_version_id IS NULL OR (length(product_version_id)=36 AND substr(product_version_id,9,1)='-' AND substr(product_version_id,14,1)='-' AND substr(product_version_id,19,1)='-' AND substr(product_version_id,24,1)='-' AND length(replace(product_version_id,'-',''))=32 AND replace(product_version_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (referenced_document_id IS NULL OR (length(referenced_document_id)=36 AND substr(referenced_document_id,9,1)='-' AND substr(referenced_document_id,14,1)='-' AND substr(referenced_document_id,19,1)='-' AND substr(referenced_document_id,24,1)='-' AND length(replace(referenced_document_id,'-',''))=32 AND replace(referenced_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (referenced_version_id IS NULL OR (length(referenced_version_id)=36 AND substr(referenced_version_id,9,1)='-' AND substr(referenced_version_id,14,1)='-' AND substr(referenced_version_id,19,1)='-' AND substr(referenced_version_id,24,1)='-' AND length(replace(referenced_version_id,'-',''))=32 AND replace(referenced_version_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE product_releases (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    root_product_document_id TEXT NOT NULL,
    root_product_revision_id TEXT NOT NULL,
    name text NOT NULL,
    request_id text NOT NULL,
    request_digest text NOT NULL,
    manifest_digest text NOT NULL,
    manifest TEXT NOT NULL,
    gate_status text NOT NULL,
    created_by TEXT NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT product_releases_gate_status_check CHECK ((gate_status IN ('PASSED', 'FAILED'))),
    CONSTRAINT product_releases_pkey PRIMARY KEY (id),
    CONSTRAINT product_releases_root_product_document_id_name_key UNIQUE (root_product_document_id, name),
    CONSTRAINT product_releases_root_product_document_id_request_id_key UNIQUE (root_product_document_id, request_id),
    CONSTRAINT product_releases_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT product_releases_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT product_releases_root_product_revision_id_fkey FOREIGN KEY (root_product_revision_id) REFERENCES document_versions(id) ON DELETE RESTRICT,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (root_product_document_id IS NULL OR (length(root_product_document_id)=36 AND substr(root_product_document_id,9,1)='-' AND substr(root_product_document_id,14,1)='-' AND substr(root_product_document_id,19,1)='-' AND substr(root_product_document_id,24,1)='-' AND length(replace(root_product_document_id,'-',''))=32 AND replace(root_product_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (root_product_revision_id IS NULL OR (length(root_product_revision_id)=36 AND substr(root_product_revision_id,9,1)='-' AND substr(root_product_revision_id,14,1)='-' AND substr(root_product_revision_id,19,1)='-' AND substr(root_product_revision_id,24,1)='-' AND length(replace(root_product_revision_id,'-',''))=32 AND replace(root_product_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (manifest IS NULL OR json_valid(manifest)),
    CHECK (created_by IS NULL OR (length(created_by)=36 AND substr(created_by,9,1)='-' AND substr(created_by,14,1)='-' AND substr(created_by,19,1)='-' AND substr(created_by,24,1)='-' AND length(replace(created_by,'-',''))=32 AND replace(created_by,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE product_solve_manifests (
    digest text NOT NULL,
    schema_version integer NOT NULL,
    root_product_document_id TEXT NOT NULL,
    root_product_revision_id TEXT NOT NULL,
    manifest TEXT NOT NULL,
    solver_build_policy text NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT product_solve_manifests_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT product_solve_manifests_pkey PRIMARY KEY (digest),
    CONSTRAINT product_solve_manifests_root_product_document_id_fkey FOREIGN KEY (root_product_document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CHECK (root_product_document_id IS NULL OR (length(root_product_document_id)=36 AND substr(root_product_document_id,9,1)='-' AND substr(root_product_document_id,14,1)='-' AND substr(root_product_document_id,19,1)='-' AND substr(root_product_document_id,24,1)='-' AND length(replace(root_product_document_id,'-',''))=32 AND replace(root_product_document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (root_product_revision_id IS NULL OR (length(root_product_revision_id)=36 AND substr(root_product_revision_id,9,1)='-' AND substr(root_product_revision_id,14,1)='-' AND substr(root_product_revision_id,19,1)='-' AND substr(root_product_revision_id,24,1)='-' AND length(replace(root_product_revision_id,'-',''))=32 AND replace(root_product_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (manifest IS NULL OR json_valid(manifest))
);

CREATE TABLE product_solve_results (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    manifest_digest text NOT NULL,
    request_id text NOT NULL,
    result TEXT,
    result_digest text,
    status text NOT NULL,
    diagnostic text,
    solver_build text,
    created_at TEXT DEFAULT (now()) NOT NULL,
    completed_at TEXT,
    CONSTRAINT product_solve_results_pkey PRIMARY KEY (id),
    CONSTRAINT product_solve_results_request_id_key UNIQUE (request_id),
    CONSTRAINT product_solve_results_manifest_digest_fkey FOREIGN KEY (manifest_digest) REFERENCES product_solve_manifests(digest) ON DELETE RESTRICT,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (result IS NULL OR json_valid(result))
);

CREATE TABLE resource_grants (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    resource_type text NOT NULL,
    resource_id TEXT NOT NULL,
    user_id TEXT,
    team_id TEXT,
    role text NOT NULL,
    granted_by_user_id TEXT NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT resource_grants_check CHECK (((((user_id IS NOT NULL)) + ((team_id IS NOT NULL))) = 1)),
    CONSTRAINT resource_grants_resource_type_check CHECK ((resource_type IN ('DOCUMENT', 'FOLDER'))),
    CONSTRAINT resource_grants_role_check CHECK ((role IN ('VIEWER', 'EDITOR'))),
    CONSTRAINT resource_grants_pkey PRIMARY KEY (id),
    CONSTRAINT resource_grants_granted_by_user_id_fkey FOREIGN KEY (granted_by_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT resource_grants_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    CONSTRAINT resource_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (resource_id IS NULL OR (length(resource_id)=36 AND substr(resource_id,9,1)='-' AND substr(resource_id,14,1)='-' AND substr(resource_id,19,1)='-' AND substr(resource_id,24,1)='-' AND length(replace(resource_id,'-',''))=32 AND replace(resource_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (user_id IS NULL OR (length(user_id)=36 AND substr(user_id,9,1)='-' AND substr(user_id,14,1)='-' AND substr(user_id,19,1)='-' AND substr(user_id,24,1)='-' AND length(replace(user_id,'-',''))=32 AND replace(user_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (team_id IS NULL OR (length(team_id)=36 AND substr(team_id,9,1)='-' AND substr(team_id,14,1)='-' AND substr(team_id,19,1)='-' AND substr(team_id,24,1)='-' AND length(replace(team_id,'-',''))=32 AND replace(team_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (granted_by_user_id IS NULL OR (length(granted_by_user_id)=36 AND substr(granted_by_user_id,9,1)='-' AND substr(granted_by_user_id,14,1)='-' AND substr(granted_by_user_id,19,1)='-' AND substr(granted_by_user_id,24,1)='-' AND length(replace(granted_by_user_id,'-',''))=32 AND replace(granted_by_user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE revision_parents (
    revision_id TEXT NOT NULL,
    parent_revision_id TEXT NOT NULL,
    ordinal smallint DEFAULT 0 NOT NULL,
    CONSTRAINT revision_parents_pkey PRIMARY KEY (revision_id, ordinal),
    CONSTRAINT revision_parents_revision_id_parent_revision_id_key UNIQUE (revision_id, parent_revision_id),
    CONSTRAINT revision_parents_parent_revision_id_fkey FOREIGN KEY (parent_revision_id) REFERENCES document_versions(id) ON DELETE RESTRICT,
    CONSTRAINT revision_parents_revision_id_fkey FOREIGN KEY (revision_id) REFERENCES document_versions(id) ON DELETE CASCADE,
    CHECK (revision_id IS NULL OR (length(revision_id)=36 AND substr(revision_id,9,1)='-' AND substr(revision_id,14,1)='-' AND substr(revision_id,19,1)='-' AND substr(revision_id,24,1)='-' AND length(replace(revision_id,'-',''))=32 AND replace(revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (parent_revision_id IS NULL OR (length(parent_revision_id)=36 AND substr(parent_revision_id,9,1)='-' AND substr(parent_revision_id,14,1)='-' AND substr(parent_revision_id,19,1)='-' AND substr(parent_revision_id,24,1)='-' AND length(replace(parent_revision_id,'-',''))=32 AND replace(parent_revision_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE team_members (
    team_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    role text DEFAULT 'MEMBER' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT team_members_role_check CHECK ((role IN ('MEMBER', 'ADMIN'))),
    CONSTRAINT team_members_pkey PRIMARY KEY (team_id, user_id),
    CONSTRAINT team_members_team_id_fkey FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE,
    CONSTRAINT team_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (team_id IS NULL OR (length(team_id)=36 AND substr(team_id,9,1)='-' AND substr(team_id,14,1)='-' AND substr(team_id,19,1)='-' AND substr(team_id,24,1)='-' AND length(replace(team_id,'-',''))=32 AND replace(team_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (user_id IS NULL OR (length(user_id)=36 AND substr(user_id,9,1)='-' AND substr(user_id,14,1)='-' AND substr(user_id,19,1)='-' AND substr(user_id,24,1)='-' AND length(replace(user_id,'-',''))=32 AND replace(user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE teams (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    name text NOT NULL,
    description text DEFAULT '' NOT NULL,
    owner_user_id TEXT NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT teams_name_check CHECK (((length(trim(name)) >= 1) AND (length(trim(name)) <= 120))),
    CONSTRAINT teams_pkey PRIMARY KEY (id),
    CONSTRAINT teams_owner_user_id_fkey FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (owner_user_id IS NULL OR (length(owner_user_id)=36 AND substr(owner_user_id,9,1)='-' AND substr(owner_user_id,14,1)='-' AND substr(owner_user_id,19,1)='-' AND substr(owner_user_id,24,1)='-' AND length(replace(owner_user_id,'-',''))=32 AND replace(owner_user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE transaction_commands (
    transaction_id TEXT NOT NULL,
    ordinal smallint NOT NULL,
    command_id text NOT NULL,
    type_uri text NOT NULL,
    schema_version integer NOT NULL,
    payload TEXT NOT NULL,
    payload_digest text NOT NULL,
    CONSTRAINT transaction_commands_ordinal_check CHECK ((ordinal >= 0)),
    CONSTRAINT transaction_commands_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT transaction_commands_pkey PRIMARY KEY (transaction_id, ordinal),
    CONSTRAINT transaction_commands_transaction_id_command_id_key UNIQUE (transaction_id, command_id),
    CONSTRAINT transaction_commands_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES domain_transactions(id) ON DELETE CASCADE,
    CHECK (transaction_id IS NULL OR (length(transaction_id)=36 AND substr(transaction_id,9,1)='-' AND substr(transaction_id,14,1)='-' AND substr(transaction_id,19,1)='-' AND substr(transaction_id,24,1)='-' AND length(replace(transaction_id,'-',''))=32 AND replace(transaction_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (payload IS NULL OR json_valid(payload))
);

CREATE TABLE user_sessions (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    user_id TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    csrf_hash TEXT NOT NULL,
    user_agent text DEFAULT '' NOT NULL,
    remote_address text DEFAULT '' NOT NULL,
    expires_at TEXT NOT NULL,
    last_seen_at TEXT DEFAULT (now()) NOT NULL,
    revoked_at TEXT,
    created_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT user_sessions_pkey PRIMARY KEY (id),
    CONSTRAINT user_sessions_token_hash_key UNIQUE (token_hash),
    CONSTRAINT user_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (user_id IS NULL OR (length(user_id)=36 AND substr(user_id,9,1)='-' AND substr(user_id,14,1)='-' AND substr(user_id,19,1)='-' AND substr(user_id,24,1)='-' AND length(replace(user_id,'-',''))=32 AND replace(user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE users (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL,
    status text DEFAULT 'ACTIVE' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    platform_role text DEFAULT 'MEMBER' NOT NULL,
    password_hash text,
    must_change_password boolean DEFAULT false NOT NULL,
    approved_at TEXT,
    approved_by_user_id TEXT,
    failed_login_count integer DEFAULT 0 NOT NULL,
    locked_until TEXT,
    CONSTRAINT users_display_name_check CHECK (((length(trim(display_name)) >= 1) AND (length(trim(display_name)) <= 120))),
    CONSTRAINT users_platform_role_check CHECK ((platform_role IN ('ADMIN', 'MEMBER'))),
    CONSTRAINT users_status_check CHECK ((status IN ('PENDING', 'ACTIVE', 'DISABLED'))),
    CONSTRAINT users_pkey PRIMARY KEY (id),
    CONSTRAINT users_approved_by_user_id_fkey FOREIGN KEY (approved_by_user_id) REFERENCES users(id) ON DELETE SET NULL,
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (must_change_password IS NULL OR must_change_password IN (0,1)),
    CHECK (approved_by_user_id IS NULL OR (length(approved_by_user_id)=36 AND substr(approved_by_user_id,9,1)='-' AND substr(approved_by_user_id,14,1)='-' AND substr(approved_by_user_id,19,1)='-' AND substr(approved_by_user_id,24,1)='-' AND length(replace(approved_by_user_id,'-',''))=32 AND replace(approved_by_user_id,'-','') NOT GLOB '*[^0-9a-f]*'))
);

CREATE TABLE workspaces (
    id TEXT DEFAULT (uuidv7()) NOT NULL,
    document_id TEXT NOT NULL,
    name text NOT NULL,
    head_revision_id TEXT NOT NULL,
    head_sequence bigint NOT NULL,
    base_revision_id TEXT NOT NULL,
    policy TEXT DEFAULT '{"evaluation": "IMMEDIATE_ALLOW_FEATURE_FAILURE"}' NOT NULL,
    created_at TEXT DEFAULT (now()) NOT NULL,
    updated_at TEXT DEFAULT (now()) NOT NULL,
    CONSTRAINT workspaces_head_sequence_check CHECK ((head_sequence >= 0)),
    CONSTRAINT workspaces_document_id_name_key UNIQUE (document_id, name),
    CONSTRAINT workspaces_pkey PRIMARY KEY (id),
    CONSTRAINT workspaces_base_revision_id_fkey FOREIGN KEY (base_revision_id) REFERENCES document_versions(id),
    CONSTRAINT workspaces_document_id_fkey FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    CONSTRAINT workspaces_head_revision_id_fkey FOREIGN KEY (head_revision_id) REFERENCES document_versions(id),
    CHECK (id IS NULL OR (length(id)=36 AND substr(id,9,1)='-' AND substr(id,14,1)='-' AND substr(id,19,1)='-' AND substr(id,24,1)='-' AND length(replace(id,'-',''))=32 AND replace(id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (document_id IS NULL OR (length(document_id)=36 AND substr(document_id,9,1)='-' AND substr(document_id,14,1)='-' AND substr(document_id,19,1)='-' AND substr(document_id,24,1)='-' AND length(replace(document_id,'-',''))=32 AND replace(document_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (head_revision_id IS NULL OR (length(head_revision_id)=36 AND substr(head_revision_id,9,1)='-' AND substr(head_revision_id,14,1)='-' AND substr(head_revision_id,19,1)='-' AND substr(head_revision_id,24,1)='-' AND length(replace(head_revision_id,'-',''))=32 AND replace(head_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (base_revision_id IS NULL OR (length(base_revision_id)=36 AND substr(base_revision_id,9,1)='-' AND substr(base_revision_id,14,1)='-' AND substr(base_revision_id,19,1)='-' AND substr(base_revision_id,24,1)='-' AND length(replace(base_revision_id,'-',''))=32 AND replace(base_revision_id,'-','') NOT GLOB '*[^0-9a-f]*')),
    CHECK (policy IS NULL OR json_valid(policy))
);

CREATE INDEX access_audit_actor_idx ON access_audit_events (actor_user_id, id DESC);

CREATE INDEX access_audit_resource_idx ON access_audit_events (resource_type, resource_id, id DESC);

CREATE INDEX account_audit_target_idx ON account_audit_events (target_user_id, id DESC);

CREATE INDEX commands_trace_id_idx ON commands (trace_id) WHERE (trace_id IS NOT NULL);

CREATE INDEX dependency_edges_source_idx ON dependency_edges (revision_id, source_key);

CREATE INDEX document_changes_document_idx ON document_changes (document_id, id DESC);

CREATE INDEX document_history_version_idx ON document_history (version_id);

CREATE INDEX document_previews_current_idx ON document_previews (document_id, updated_at DESC);

CREATE UNIQUE INDEX document_version_name_idx ON document_versions (document_id, version_name) WHERE (version_name IS NOT NULL);

CREATE INDEX document_versions_document_idx ON document_versions (document_id, sequence DESC);

CREATE INDEX documents_active_updated_idx ON documents (updated_at DESC) WHERE (deleted_at IS NULL);

CREATE INDEX documents_deleted_updated_idx ON documents (deleted_at DESC) WHERE (deleted_at IS NOT NULL);

CREATE INDEX documents_folder_type_idx ON documents (folder_id, document_type) WHERE (deleted_at IS NULL);

CREATE INDEX documents_folder_updated_idx ON documents (folder_id, updated_at DESC) WHERE (deleted_at IS NULL);

CREATE INDEX documents_name_search_idx ON documents (lower(name));

CREATE INDEX documents_recent_idx ON documents (last_opened_at DESC) WHERE ((deleted_at IS NULL) AND (last_opened_at IS NOT NULL));

CREATE INDEX domain_transactions_product_design_idx ON domain_transactions (product_design_transaction_id) WHERE (product_design_transaction_id IS NOT NULL);

CREATE INDEX domain_transactions_root_idx ON domain_transactions (root_transaction_id, sequence DESC) WHERE (root_transaction_id IS NOT NULL);

CREATE INDEX domain_transactions_workspace_idx ON domain_transactions (workspace_id, sequence DESC);

CREATE UNIQUE INDEX evaluation_runs_authoritative_idx ON evaluation_runs (revision_id, capability) WHERE authoritative;

CREATE UNIQUE INDEX folders_parent_name_idx ON folders (COALESCE(parent_id,''), lower(name)) WHERE (deleted_at IS NULL);

CREATE INDEX folders_trash_roots_idx ON folders (deleted_at DESC) WHERE ((deleted_at IS NOT NULL) AND (trashed_by_folder_id = id));

CREATE INDEX jobs_claim_idx ON jobs (state, available_at, priority DESC, created_at);

CREATE INDEX jobs_document_idx ON jobs (document_id, created_at DESC);

CREATE INDEX jobs_lease_idx ON jobs (lease_expires_at) WHERE (state = 'RUNNING');

CREATE INDEX jobs_user_visible_idx ON jobs (requested_by_user_id, created_at DESC) WHERE user_visible;

CREATE INDEX outbox_events_unpublished_idx ON outbox_events (created_at, id) WHERE (published_at IS NULL);

CREATE INDEX product_instances_version_idx ON product_instances (product_version_id);

CREATE INDEX product_solve_results_manifest_idx ON product_solve_results (manifest_digest, created_at DESC);

CREATE INDEX resource_grants_resource_idx ON resource_grants (resource_type, resource_id);

CREATE UNIQUE INDEX resource_grants_team_idx ON resource_grants (resource_type, resource_id, team_id) WHERE (team_id IS NOT NULL);

CREATE UNIQUE INDEX resource_grants_user_idx ON resource_grants (resource_type, resource_id, user_id) WHERE (user_id IS NOT NULL);

CREATE INDEX user_sessions_active_idx ON user_sessions (token_hash, expires_at) WHERE (revoked_at IS NULL);

CREATE INDEX user_sessions_user_idx ON user_sessions (user_id, created_at DESC);

CREATE UNIQUE INDEX users_email_idx ON users (lower(email));

INSERT INTO users(id,email,display_name,status,platform_role,approved_at) VALUES('00000000-0000-7000-8000-000000000001','admin@occccad.local','Administrator','ACTIVE','ADMIN',now());

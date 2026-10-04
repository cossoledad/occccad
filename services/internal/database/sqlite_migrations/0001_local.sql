-- SQLite Local Mode baseline, equivalent to PostgreSQL migrations 0001-0038.
-- Keep constraints, foreign keys and catalog changes aligned across providers.

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

CREATE TABLE ui_toolbar_items (
    toolbar_id text NOT NULL,
    command_id text NOT NULL,
    name text NOT NULL,
    help_text text NOT NULL,
    icon_key text NOT NULL,
    group_key text DEFAULT 'primary' NOT NULL,
    sort_order integer NOT NULL,
    repeatable boolean DEFAULT false NOT NULL,
    CONSTRAINT ui_toolbar_items_pkey PRIMARY KEY (toolbar_id, command_id),
    CONSTRAINT ui_toolbar_items_toolbar_id_sort_order_key UNIQUE (toolbar_id, sort_order),
    CONSTRAINT ui_toolbar_items_toolbar_id_fkey FOREIGN KEY (toolbar_id) REFERENCES ui_toolbars(id) ON DELETE CASCADE,
    CHECK (repeatable IS NULL OR repeatable IN (0,1))
);

CREATE TABLE ui_toolbars (
    id text NOT NULL,
    name text NOT NULL,
    workbench text NOT NULL,
    "position" text NOT NULL,
    orientation text DEFAULT 'horizontal' NOT NULL,
    style_key text DEFAULT 'standard' NOT NULL,
    sort_order integer NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    CONSTRAINT ui_toolbars_orientation_check CHECK ((orientation IN ('horizontal', 'vertical'))),
    CONSTRAINT ui_toolbars_position_check CHECK (("position" IN ('top-left', 'top-center', 'top-right', 'bottom-left', 'bottom-center', 'bottom-right'))),
    CONSTRAINT ui_toolbars_style_key_check CHECK ((style_key IN ('standard', 'part', 'sketch', 'assembly', 'debug'))),
    CONSTRAINT ui_toolbars_workbench_check CHECK ((workbench IN ('ALL', 'PART_DESIGN', 'SKETCHER', 'ASSEMBLY_DESIGN'))),
    CONSTRAINT ui_toolbars_pkey PRIMARY KEY (id),
    CHECK (enabled IS NULL OR enabled IN (0,1))
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

INSERT INTO ui_toolbars(id,name,workbench,position,style_key,sort_order) VALUES
('part-design','Part Design','PART_DESIGN','top-left','part',10),
('sketch-geometry','草图几何','SKETCHER','top-left','sketch',20),
('sketch-geometric-constraints','几何约束','SKETCHER','top-left','sketch',30),
('sketch-dimensional-constraints','尺寸约束','SKETCHER','top-left','sketch',40),
('sketch-aggregates','草图常用图形','SKETCHER','top-left','sketch',50),
('assembly-design','Assembly Design','ASSEMBLY_DESIGN','top-left','assembly',60),
('common-edit','编辑','ALL','top-center','standard',70),
('view','视图','ALL','top-right','standard',80),
('debug','Debug','ALL','bottom-right','debug',90);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable) VALUES
('part-design','tool.select','选择','选择视图区或结构树中的对象。','select',10,false),
('part-design','capture.settings','捕捉','设置三维选择过滤和草图吸附类型。','capture',15,false),
('part-design','sketch.start','草图','选择基准面创建草图，或选择已有草图进入编辑。','sketch',20,false),
('part-design','part.pad','拉伸','沿草图平面法向拉伸所选闭合草图。','pad',30,false),
('part-design','part.pocket','切除','沿草图平面法向生成工具体，并从当前 Body 移除材料。','pocket',40,false),
('part-design','part.revolve','旋转','绕草图中的构造线旋转闭合区域并应用到当前 Body。','revolve',50,false),
('part-design','part.datum-plane','基准面','以所选平面为参考创建偏置基准面。','datum-plane',60,false),
('part-design','part.datum-axis','基准轴','以显式原点和方向创建基准轴。','datum-axis',70,false),
('sketch-geometry','tool.select','选择','选择当前活动草图中的几何和约束。','select',10,false),
('sketch-geometry','capture.settings','捕捉','设置三维选择过滤和草图吸附类型。','capture',15,false),
('sketch-geometry','sketch.project','投影','将已有实体的边或顶点关联投影为外部几何。','project',18,true),
('sketch-geometry','sketch.point','点','在活动草图中创建点；双击按钮进入连续创建。','point',20,true),
('sketch-geometry','sketch.line','直线','用起点和终点创建直线；双击按钮连续创建。','line',30,true),
('sketch-geometry','sketch.arc','圆弧','依次指定圆心、起点和终点创建圆弧。','arc',40,true),
('sketch-geometry','sketch.polyline','多段线','连续创建相接线段，双击或 Enter 完成。','polyline',50,true),
('sketch-geometry','sketch.spline','过点曲线','创建经过采集点的插值曲线，双击或 Enter 完成。','spline',60,true),
('sketch-geometry','sketch.finish','退出草图','结束当前草图编辑并返回 Part Design。','finish',70,false),
('sketch-geometric-constraints','sketch.constraint.coincident','重合','使两个点引用重合。','coincident',10,true),
('sketch-geometric-constraints','sketch.constraint.parallel','平行','使两条直线或直线与基准轴平行。','parallel',20,true),
('sketch-geometric-constraints','sketch.constraint.fixed','固定','固定所选几何的当前参数。','fixed',30,true),
('sketch-geometric-constraints','sketch.constraint.horizontal','水平','使直线平行于草图 U 轴。','horizontal',40,true),
('sketch-geometric-constraints','sketch.constraint.vertical','垂直','使直线平行于草图 V 轴。','vertical',50,true),
('sketch-geometric-constraints','sketch.constraint.perpendicular','垂直相交','使两条直线方向互相垂直。','perpendicular',60,true),
('sketch-geometric-constraints','sketch.constraint.tangent','相切','使支持的直线、圆或圆弧相切。','tangent',70,true),
('sketch-geometric-constraints','sketch.constraint.equal','相等','使两条线等长，或使圆和圆弧等半径。','equal',80,true),
('sketch-geometric-constraints','sketch.constraint.concentric','同心','使两个圆形元素共享圆心。','concentric',90,true),
('sketch-geometric-constraints','sketch.constraint.point_on_object','点在对象上','约束一个点位于直线、圆或圆弧上。','point-on-object',100,true),
('sketch-geometric-constraints','sketch.constraint.midpoint','中点','约束一个点位于直线中点。','midpoint',110,true),
('sketch-geometric-constraints','sketch.constraint.symmetry','对称','使用直线/基准轴做轴对称，或使用点/原点做中心对称。','symmetry',120,true),
('sketch-dimensional-constraints','sketch.dimension.linear','线性尺寸','创建线长、点点距离或点线距离尺寸。','distance',10,true),
('sketch-dimensional-constraints','sketch.constraint.radius','半径','设置圆或圆弧的半径尺寸。','radius',20,true),
('sketch-dimensional-constraints','sketch.constraint.angle','角度','设置两条直线或直线与基准轴的夹角。','angle',30,true),
('sketch-aggregates','sketch.rectangle','矩形','用两个对角点创建带内部连接和方向约束的矩形。','rectangle',10,true),
('sketch-aggregates','sketch.polygon','正六边形','用中心和顶点创建正六边形。','polygon',20,true),
('sketch-aggregates','sketch.circle','圆','用圆心和圆周点创建圆。','circle',30,true),
('assembly-design','tool.select','选择','选择装配实例或结构节点。','select',10,false),
('assembly-design','capture.settings','捕捉','设置三维选择过滤和草图吸附类型。','capture',15,false),
('assembly-design','product.insert','插入','向 Product 插入 Part 或 Product 实例。','insert',20,false),
('assembly-design','product.reference.toggle','引用模式','在跟随 Head 和固定版本之间切换实例引用。','reference',30,false),
('assembly-design','assembly.move','移动组件','选择装配实例并使用三维操纵器移动。','move',35,false),
('assembly-design','assembly.fix','固定','固定所选装配实例的当前刚体位置。','fixed',40,false),
('assembly-design','assembly.rigid','固连','保持两个装配实例之间的当前相对刚体位姿。','link',45,false),
('assembly-design','assembly.coincident','重合','使两个基准点、轴或平面重合。','coincident',50,false),
('assembly-design','assembly.concentric','同心','使所选两条基准轴同轴。','concentric',60,false),
('assembly-design','assembly.angle','角度','约束两个基准轴或平面的夹角。','angle',70,false),
('assembly-design','assembly.distance','距离','约束两个基准元素的距离。','distance',80,false),
('common-edit','edit.undo','撤销','补偿最近一个可撤销的领域事务。','undo',10,false),
('common-edit','edit.redo','重做','重新应用最近一个已撤销的领域事务。','redo',20,false),
('common-edit','history.version','创建版本','为当前 Revision 创建命名版本。','version',30,false),
('common-edit','document.share','共享','管理当前文档的访问授权。','share',40,false),
('view','navigation.profile.toggle','导航模式','切换默认与 CATIA 导航方式。','navigation',10,false),
('view','view.fit','适合窗口','调整相机以显示全部模型。','fit',20,false),
('view','view.iso','等轴测','切换到等轴测标准视图。','isometric',30,false),
('debug','debug.download','下载诊断包','导出当前文档、草图、Workspace、事务和求值诊断数据。','debug',10,false);

INSERT INTO ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'sketch-geometry','sketch.project','投影',
    '将已有实体的边或顶点关联投影为外部几何。','project',18,true
)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;

INSERT INTO ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'part-design','part.parameters','参数',
    '集中查看、编辑并复用当前 Part 的参数。','parameters',25,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;

INSERT INTO ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'part-design','part.publications','发布',
    '创建和管理可跨文档引用的稳定几何与参数契约。','parameters',27,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;

INSERT INTO ui_toolbar_items(
    toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable
) VALUES (
    'assembly-design','product.publications','发布',
    '转发子 Part Publication，形成稳定 Product 装配接口。','parameters',25,false
)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
    name=EXCLUDED.name,
    help_text=EXCLUDED.help_text,
    icon_key=EXCLUDED.icon_key,
    sort_order=EXCLUDED.sort_order,
    repeatable=EXCLUDED.repeatable;

DELETE FROM ui_toolbar_items;

DELETE FROM ui_toolbars;

INSERT INTO ui_toolbars(id,name,workbench,position,style_key,sort_order) VALUES
('selection','选择','ALL','top-left','standard',0),
('part-sketch','草图入口','PART_DESIGN','top-left','part',10),
('part-features','实体特征','PART_DESIGN','top-left','part',20),
('part-knowledge','参数与接口','PART_DESIGN','top-left','part',30),
('part-datums','基准','PART_DESIGN','top-left','part',40),
('sketch-lifecycle','草图会话','SKETCHER','top-left','sketch',10),
('sketch-projection','外部几何','SKETCHER','top-left','sketch',20),
('sketch-primitives','草图基本元素','SKETCHER','top-left','sketch',30),
('sketch-profiles','草图轮廓','SKETCHER','top-left','sketch',40),
('sketch-geometric-constraints','几何约束','SKETCHER','top-left','sketch',50),
('sketch-dimensional-constraints','尺寸约束','SKETCHER','top-left','sketch',60),
('product-structure','产品结构','ASSEMBLY_DESIGN','top-left','assembly',10),
('product-interface','产品接口','ASSEMBLY_DESIGN','top-left','assembly',20),
('assembly-positioning','组件定位','ASSEMBLY_DESIGN','top-left','assembly',30),
('assembly-constraints','装配约束','ASSEMBLY_DESIGN','top-left','assembly',40),
('history','历史','ALL','top-center','standard',70),
('collaboration','协作','ALL','top-center','standard',80),
('view-navigation','视图','ALL','top-right','standard',90),
('debug','诊断','ALL','bottom-right','debug',100);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable) VALUES
('selection','tool.select','选择','选择视图区或结构树中的对象。','select','primary',10,false),
('part-sketch','sketch.start','草图','选择基准面创建草图，或选择已有草图进入编辑。','sketch','primary',10,false),
('part-features','part.pad','拉伸','沿草图平面法向拉伸所选闭合草图。','pad','primary',10,false),
('part-features','part.pocket','切除','沿草图平面法向生成工具体，并从当前 Body 移除材料。','pocket','primary',20,false),
('part-features','part.revolve','旋转','绕草图中的构造线旋转闭合区域并应用到当前 Body。','revolve','primary',30,false),
('part-knowledge','part.parameters','参数','集中查看、编辑并复用当前 Part 的参数。','parameters','primary',10,false),
('part-knowledge','part.publications','发布','创建和管理可跨文档引用的稳定几何与参数契约。','publication','primary',20,false),
('part-datums','part.datum-plane','基准面','以所选平面为参考创建偏置基准面。','datum-plane','primary',10,false),
('part-datums','part.datum-axis','基准轴','以显式原点和方向创建基准轴。','datum-axis','primary',20,false),
('sketch-lifecycle','sketch.finish','退出草图','结束当前草图编辑并返回 Part Design。','finish','primary',10,false),
('sketch-projection','sketch.project','投影','将已有实体的边或顶点关联投影为外部几何。','project','primary',10,true),
('sketch-primitives','sketch.point','点','在活动草图中创建点；双击按钮进入连续创建。','point','primary',10,true),
('sketch-primitives','sketch.line','直线','用起点和终点创建直线；双击按钮连续创建。','line','primary',20,true),
('sketch-primitives','sketch.arc','圆弧','依次指定圆心、起点和终点创建圆弧。','arc','primary',30,true),
('sketch-primitives','sketch.polyline','多段线','连续创建相接线段，双击或 Enter 完成。','polyline','primary',40,true),
('sketch-primitives','sketch.spline','过点曲线','创建经过采集点的插值曲线，双击或 Enter 完成。','spline','primary',50,true),
('sketch-profiles','sketch.rectangle','矩形','用两个对角点创建带内部连接和方向约束的矩形。','rectangle','primary',10,true),
('sketch-profiles','sketch.polygon','正六边形','用中心和顶点创建正六边形。','polygon','primary',20,true),
('sketch-profiles','sketch.circle','圆','用圆心和圆周点创建圆。','circle','primary',30,true),
('sketch-profiles','sketch.slot','长圆孔','用中心线和半径创建长圆孔轮廓。','slot','primary',40,true),
('sketch-geometric-constraints','sketch.constraint.coincident','重合','使两个点引用重合。','coincident','primary',10,true),
('sketch-geometric-constraints','sketch.constraint.parallel','平行','使两条直线或直线与基准轴平行。','parallel','primary',20,true),
('sketch-geometric-constraints','sketch.constraint.fixed','固定','固定所选几何的当前参数。','fixed','primary',30,true),
('sketch-geometric-constraints','sketch.constraint.horizontal','水平','使直线平行于草图 U 轴。','horizontal','primary',40,true),
('sketch-geometric-constraints','sketch.constraint.vertical','垂直','使直线平行于草图 V 轴。','vertical','primary',50,true),
('sketch-geometric-constraints','sketch.constraint.perpendicular','垂直相交','使两条直线方向互相垂直。','perpendicular','primary',60,true),
('sketch-geometric-constraints','sketch.constraint.tangent','相切','使支持的直线、圆或圆弧相切。','tangent','primary',70,true),
('sketch-geometric-constraints','sketch.constraint.equal','相等','使两条线等长，或使圆和圆弧等半径。','equal','primary',80,true),
('sketch-geometric-constraints','sketch.constraint.concentric','同心','使两个圆形元素共享圆心。','concentric','primary',90,true),
('sketch-geometric-constraints','sketch.constraint.point_on_object','点在对象上','约束一个点位于直线、圆或圆弧上。','point-on-object','primary',100,true),
('sketch-geometric-constraints','sketch.constraint.midpoint','中点','约束一个点位于直线中点。','midpoint','primary',110,true),
('sketch-geometric-constraints','sketch.constraint.symmetry','对称','使用直线/基准轴做轴对称，或使用点/原点做中心对称。','symmetry','primary',120,true),
('sketch-dimensional-constraints','sketch.dimension.linear','线性尺寸','创建线长、点点距离或点线距离尺寸。','distance','primary',10,true),
('sketch-dimensional-constraints','sketch.constraint.radius','半径','设置圆或圆弧的半径尺寸。','radius','primary',20,true),
('sketch-dimensional-constraints','sketch.constraint.angle','角度','设置两条直线或直线与基准轴的夹角。','angle','primary',30,true),
('product-structure','product.insert','插入','向 Product 插入 Part 或 Product 实例。','insert','primary',10,false),
('product-interface','product.publications','发布','转发子 Part Publication，形成稳定 Product 装配接口。','publication','primary',10,false),
('product-interface','product.release','产品版本','打开不可变产品版本中心。','release','primary',20,false),
('assembly-positioning','assembly.move','移动组件','选择装配实例并使用三维操纵器移动。','move','primary',10,false),
('assembly-constraints','assembly.fix','固定','固定所选装配实例的当前刚体位置。','fixed','primary',10,false),
('assembly-constraints','assembly.rigid','固连','保持两个装配实例之间的当前相对刚体位姿。','link','primary',20,false),
('assembly-constraints','assembly.coincident','重合','使两个基准点、轴或平面重合。','coincident','primary',30,false),
('assembly-constraints','assembly.concentric','同心','使所选两条基准轴同轴。','concentric','primary',40,false),
('assembly-constraints','assembly.angle','角度','约束两个基准轴或平面的夹角。','angle','primary',50,false),
('assembly-constraints','assembly.distance','距离','约束两个基准元素的距离。','distance','primary',60,false),
('history','edit.undo','撤销','补偿最近一个可撤销的领域事务。','undo','primary',10,false),
('history','edit.redo','重做','重新应用最近一个已撤销的领域事务。','redo','primary',20,false),
('history','history.version','创建版本','为当前 Revision 创建命名版本。','version','primary',30,false),
('collaboration','document.share','共享','管理当前文档的访问授权。','share','primary',10,false),
('view-navigation','view.fit','适合窗口','调整相机以显示全部模型。','fit','primary',10,false),
('view-navigation','view.top','顶视图','切换到顶视图。','top-view','primary',20,false),
('view-navigation','view.front','前视图','切换到前视图。','front-view','primary',30,false),
('view-navigation','view.right','右视图','切换到右视图。','right-view','primary',40,false),
('view-navigation','view.iso','等轴测','切换到等轴测标准视图。','isometric','primary',50,false),
('debug','debug.download','下载诊断包','导出当前文档、草图、Workspace、事务和求值诊断数据。','debug','primary',10,false);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('sketch-lifecycle','sketch.normal','正对草图平面','恢复活动草图平面的正视方向，保留当前缩放和关注区域。','top-view','primary',20,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('view-navigation','view.normal','法线视图','选择基准面或实体平面后正对该面，保留显示比例。','top-view','primary',60,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES
('assembly-constraints','assembly.parallel','平行','约束两方向平行，可选择同向或反向。','parallel','primary',51,false),
('assembly-constraints','assembly.perpendicular','垂直','约束两方向垂直，保留正向90°或反向270°意图。','perpendicular','primary',52,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES ('product-structure','product.pattern','多实例化','沿坐标轴重复所选组件并实时预览。','pattern','primary',20,false)
ON CONFLICT (toolbar_id,command_id) DO NOTHING;

DELETE FROM ui_toolbar_items
WHERE toolbar_id = 'sketch-profiles' AND command_id = 'sketch.slot';

INSERT INTO ui_toolbars(id,name,workbench,position,style_key,sort_order) VALUES
('sketch-edit','草图编辑','SKETCHER','top-left','sketch',45)
ON CONFLICT (id) DO UPDATE SET
name=EXCLUDED.name, workbench=EXCLUDED.workbench, position=EXCLUDED.position,
style_key=EXCLUDED.style_key, sort_order=EXCLUDED.sort_order;

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable) VALUES
('sketch-primitives','sketch.point','点','在活动草图中连续创建点；C 切换辅助几何，Esc 或右键结束。','point','primary',10,true),
('sketch-primitives','sketch.line','直线','连续创建两点直线；长度/角度数值输入，Tab 切换，Enter 确认。','line','primary',20,true),
('sketch-primitives','sketch.polyline','多段线','连续创建相接直线和相切圆弧；T 切换，双击或 Enter 完成。','polyline','primary',40,true),
('sketch-primitives','sketch.spline','拟合点样条','采集拟合点并创建必须经过这些点的插值样条；双击或 Enter 完成。','spline','primary',50,true),
('sketch-primitives','sketch.spline.control','控制点样条','创建控制点 B-Spline；不要求经过控制点，闭合缝不保证切向连续。','spline','more',60,true),
('sketch-profiles','sketch.circle.three_point','三点圆','通过三个不共线点创建圆。','circle','more',40,true),
('sketch-profiles','sketch.arc.three_point','三点圆弧','通过起点、中间点和终点创建有向圆弧。','arc','more',50,true),
('sketch-profiles','sketch.rectangle.center','中心矩形','从中心和角点创建带正式中心关系的矩形。','rectangle','more',60,true),
('sketch-profiles','sketch.rectangle.oriented','定向矩形','从底边和第三点创建带正式方向约束的矩形。','rectangle','more',70,true),
('sketch-profiles','sketch.ellipse','椭圆','指定中心、长半轴与短半轴创建真实椭圆。','circle','more',80,true),
('sketch-profiles','sketch.elliptical_arc','椭圆弧','指定椭圆与起止参数范围创建有向椭圆弧。','arc','more',90,true),
('sketch-edit','sketch.edit.delete','删除几何','删除选定本地几何；约束影响由原子领域操作验证。','delete','primary',10,false),
('sketch-edit','sketch.edit.copy','复制几何','创建副本；默认仅几何（连接与尺寸未复制），I 切换内部约束复制，公式依赖拒绝。','copy','primary',20,false),
('sketch-edit','sketch.edit.move','移动几何','保留约束；选基点与目标点或输入 X/Y 位移，冲突明确拒绝。','move','primary',30,false),
('sketch-edit','sketch.edit.rotate','旋转几何','选中心、基准点和目标点，或输入角度；Shift+C 切换带复制。','rotate','primary',40,false),
('sketch-edit','sketch.edit.scale','统一缩放','选中心及两参考点，或输入正比例；固定、公式和尺寸冲突不静默删除。','scale','primary',50,false),
('sketch-edit','sketch.edit.mirror','镜像','默认关联镜像；M 切换独立副本，选直线或 X/Y 轴；独立模式默认不复制连接/尺寸，I 切换。','mirror','primary',60,false),
('sketch-edit','sketch.edit.spline_insert','样条插入点/结点','FIT 插入拟合点；CONTROL 精确插入结点，保持曲线形状。','point','more',83,false),
('sketch-edit','sketch.edit.spline_delete','样条删除点/结点','FIT 删除明确拟合点；CONTROL 只允许精确移除结点，不任意删 pole。','delete','more',84,false),
('sketch-edit','sketch.edit.spline_close','样条开/闭合','明确切换样条开闭；闭合缝不保证高阶连续。','spline','more',85,false),
('sketch-edit','sketch.edit.spline_control','转控制点样条','精确转换已求解 FIT canonical 曲线；U 明确释放受影响点引用。','spline','more',86,false),
('sketch-edit','sketch.edit.extend','延伸','明确要延伸的端点、只读参考边界和目标交点分支；权威内核计算。','line','primary',61,false),
('sketch-edit','sketch.edit.offset','二维偏移','线段、圆、圆弧及连续链独立偏移；有符号距离，M 斜接/圆角，退化和自交拒绝。','reference','primary',62,false),
('sketch-edit','sketch.edit.complement','圆弧补弧','改为另一补弧；U 显式解除整体/端点关系，不按最近端点重连。','arc','more',81,false),
('sketch-edit','sketch.edit.close','闭合曲线','圆弧/椭圆弧转完整周期曲线；原端点关系需显式解除。','circle','more',82,false),
('sketch-edit','sketch.edit.fillet','草图圆角','选择两条线/圆弧裁切端部与分支；半径圆角，K 保留或修剪，B 原子多角点批次。','arc','primary',63,false),
('sketch-edit','sketch.edit.chamfer','草图倒角','两条线段；M 切换等长/双长度/长度加角度，K 保留或修剪，B 原子批次。','line','primary',64,false),
('sketch-edit','sketch.edit.trim','修剪范围','输入曲线真实参数域的保留范围；U 显式查看整体关系释放影响。','reference','primary',65,false),
('sketch-edit','sketch.edit.quick_trim','快速修剪','选择本地/外部只读边界及命中段；Q 切换删除、保留或打断，权威内核计算交点。','reference','primary',66,false),
('sketch-edit','sketch.edit.split','分割几何','指定真实参数域内的位置；稳定端点和约束引用由控制面映射。','reference','more',70,false),
('sketch-edit','sketch.edit.construction','标准/辅助转换','原子切换选定几何的轮廓角色；辅助几何不参与实体轮廓。','reference','more',80,false),
('sketch-geometric-constraints','sketch.constraint.collinear','共线','使两条直线支撑共线；可使用内置草图轴。','line','more',15,true),
('sketch-dimensional-constraints','sketch.constraint.horizontal_distance','水平距离','两点沿草图 U 轴的有向投影距离；按第二点减第一点，允许负值与零。','distance','more',15,true),
('sketch-dimensional-constraints','sketch.constraint.vertical_distance','垂直距离','两点沿草图 V 轴的有向投影距离；按第二点减第一点，允许负值与零。','distance','more',16,true),
('sketch-dimensional-constraints','sketch.constraint.major_radius','长半轴','标注椭圆或椭圆弧的长半轴尺寸。','radius','more',70,true),
('sketch-dimensional-constraints','sketch.constraint.minor_radius','短半轴','标注椭圆或椭圆弧的短半轴尺寸。','radius','more',80,true)
ON CONFLICT (toolbar_id,command_id) DO UPDATE SET
name=EXCLUDED.name, help_text=EXCLUDED.help_text, icon_key=EXCLUDED.icon_key,
group_key=EXCLUDED.group_key, sort_order=EXCLUDED.sort_order,
repeatable=EXCLUDED.repeatable;

DELETE FROM ui_toolbar_items WHERE command_id IN ('sketch.edit.scale','sketch.edit.quick_trim');

UPDATE ui_toolbar_items SET group_key='primary' WHERE group_key='more';

UPDATE ui_toolbar_items SET name='修剪',help_text='点击删除命中区间；没有边界时删除整条曲线。' WHERE command_id='sketch.edit.trim';

UPDATE ui_toolbar_items SET name='分割',help_text='点击内部位置打断；闭合曲线选两个位置。保留形状并连接新端点。' WHERE command_id='sketch.edit.split';

UPDATE ui_toolbar_items SET help_text='选择曲线，移动或拖动到目标位置；捕获边界后使用精确交点及连接关系。' WHERE command_id='sketch.edit.extend';

UPDATE ui_toolbar_items SET help_text='选择两条适用曲线，直接输入半径并预览；Enter 原子提交。' WHERE command_id='sketch.edit.fillet';

UPDATE ui_toolbar_items SET help_text='选择两条直线，直接输入距离并预览；Enter 原子提交。' WHERE command_id='sketch.edit.chamfer';

UPDATE ui_toolbar_items SET help_text='直接点选切换标准/构造状态；合法预选整体原子转换。' WHERE command_id='sketch.edit.construction';

DELETE FROM ui_toolbar_items WHERE command_id='sketch.edit.rotate';

UPDATE ui_toolbar_items SET name=CASE command_id
 WHEN 'sketch.edit.delete' THEN '删除'
 WHEN 'sketch.edit.copy' THEN '复制'
 WHEN 'sketch.edit.move' THEN '移动'
 WHEN 'sketch.edit.offset' THEN '偏移'
 WHEN 'sketch.edit.complement' THEN '补弧'
 WHEN 'sketch.edit.fillet' THEN '圆角'
 WHEN 'sketch.edit.chamfer' THEN '倒角'
 WHEN 'sketch.edit.construction' THEN '构造线'
 ELSE name END WHERE toolbar_id='sketch-edit';

UPDATE ui_toolbar_items SET help_text='使用手柄移动或旋转；实时求解并保留现有约束。' WHERE command_id='sketch.edit.move';

DELETE FROM ui_toolbar_items WHERE command_id='sketch.normal';

UPDATE ui_toolbar_items SET name='内接多边形',help_text='中心与构造圆半径；多边形边与圆相切，边数 3–50。' WHERE command_id='sketch.polygon';

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,group_key,sort_order,repeatable)
VALUES('sketch-profiles','sketch.polygon.circumscribed','外接多边形','中心与构造圆半径；多边形顶点位于圆上，边数 3–50。','polygon','primary',31,true)
ON CONFLICT(toolbar_id,command_id) DO UPDATE SET name=EXCLUDED.name,help_text=EXCLUDED.help_text;

UPDATE ui_toolbar_items SET group_key='variants:'||CASE
 WHEN command_id IN('sketch.spline','sketch.spline.control') THEN 'spline'
 WHEN command_id IN('sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented') THEN 'rectangle'
 WHEN command_id IN('sketch.circle','sketch.circle.three_point') THEN 'circle'
 WHEN command_id IN('sketch.arc','sketch.arc.three_point','sketch.elliptical_arc') THEN 'arc'
 WHEN command_id IN('sketch.ellipse') THEN 'ellipse'
 WHEN command_id IN('sketch.polygon','sketch.polygon.circumscribed') THEN 'polygon'
 WHEN command_id LIKE 'sketch.edit.spline_%' THEN 'spline_control'
 WHEN command_id IN('sketch.dimension.linear','sketch.constraint.horizontal_distance','sketch.constraint.vertical_distance') THEN 'linear'
 WHEN command_id IN('sketch.constraint.radius','sketch.constraint.diameter','sketch.constraint.major_radius','sketch.constraint.minor_radius') THEN 'axis'
 ELSE 'angle' END
 WHERE command_id IN('sketch.spline','sketch.spline.control','sketch.rectangle','sketch.rectangle.center','sketch.rectangle.oriented','sketch.circle','sketch.circle.three_point','sketch.arc','sketch.arc.three_point','sketch.ellipse','sketch.elliptical_arc','sketch.polygon','sketch.polygon.circumscribed','sketch.edit.spline_insert','sketch.edit.spline_delete','sketch.edit.spline_close','sketch.edit.spline_control','sketch.dimension.linear','sketch.constraint.horizontal_distance','sketch.constraint.vertical_distance','sketch.constraint.radius','sketch.constraint.diameter','sketch.constraint.major_radius','sketch.constraint.minor_radius','sketch.constraint.angle');

UPDATE ui_toolbar_items SET toolbar_id='sketch-profiles',sort_order=37 WHERE command_id='sketch.arc';

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable)
VALUES ('part-features','part.boolean','布尔','组合当前 Part 中明确的 Body 输出阶段。','pocket',55,false);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable) VALUES
('part-features','part.fillet','圆角','对持久引用的边创建常量半径圆角。','pad',60,false),
('part-features','part.chamfer','倒角','对持久引用的边创建等距倒角。','pad',70,false),
('part-features','part.draft','拔模','以中性平面创建恒角拔模。','pad',80,false),
('part-features','part.shell','抽壳','移除所选面并创建恒定厚度壳体。','pocket',90,false);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable)
VALUES ('part-features','part.loft','放样','按有序闭合截面创建实体放样。','pad',100,false);

INSERT INTO ui_toolbar_items(toolbar_id,command_id,name,help_text,icon_key,sort_order,repeatable)
VALUES
('part-features','part.pattern.linear','线性阵列','按固定步距或总跨度关联重复上游几何。','pad',110,false),
('part-features','part.pattern.circular','圆周阵列','围绕轴按角度或整圆关联重复上游几何。','pad',120,false),
('sketch-edit','part.pattern.linear','线性阵列','关联重复选定草图几何。','pad',67,false),
('sketch-edit','part.pattern.circular','圆周阵列','关联重复选定草图几何。','pad',68,false);

ALTER TABLE occccad.dependency_edges RENAME TO dependency_edge_rows;

ALTER TABLE occccad.document_versions
    ADD COLUMN dependency_edge_digest text NOT NULL DEFAULT '',
    ADD COLUMN dependency_edge_owner uuid REFERENCES occccad.document_versions(id) DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX document_versions_dependency_edge_digest_idx
    ON occccad.document_versions(document_id, dependency_edge_digest, sequence)
    WHERE dependency_edge_digest <> '' AND dependency_edge_owner IS NULL;

CREATE VIEW occccad.dependency_edges AS
    SELECT version.id AS revision_id, edge.source_key, edge.target_key, edge.edge_kind
    FROM occccad.document_versions version
    JOIN occccad.dependency_edge_rows edge ON edge.revision_id = COALESCE(version.dependency_edge_owner, version.id);

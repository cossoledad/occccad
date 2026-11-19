ALTER TABLE dependency_edges RENAME TO dependency_edge_rows;

ALTER TABLE document_versions ADD COLUMN dependency_edge_digest TEXT NOT NULL DEFAULT '';
ALTER TABLE document_versions ADD COLUMN dependency_edge_owner TEXT REFERENCES document_versions(id) DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX document_versions_dependency_edge_digest_idx
    ON document_versions(document_id, dependency_edge_digest, sequence)
    WHERE dependency_edge_digest <> '' AND dependency_edge_owner IS NULL;

CREATE VIEW dependency_edges AS
    SELECT version.id AS revision_id, edge.source_key, edge.target_key, edge.edge_kind
    FROM document_versions version
    JOIN dependency_edge_rows edge ON edge.revision_id = COALESCE(version.dependency_edge_owner, version.id);

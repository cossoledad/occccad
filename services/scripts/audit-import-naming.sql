-- Read-only audit. Counts metadata, not B-Rep or object-content integrity.
-- Current heads and historical revisions are intentionally reported separately.
WITH imported AS (
  SELECT d.id, v.id AS revision_id, d.head_version_id = v.id AS current_head,
         v.geometry_key, a.geometry_key AS artifact_key, o.sha256 AS digest,
         r.object_id AS object_id
  FROM occccad.documents d
  JOIN occccad.document_versions v ON v.document_id=d.id
  LEFT JOIN occccad.geometry_artifacts a ON a.geometry_key=v.geometry_key
  LEFT JOIN occccad.geometry_representations r ON r.geometry_key=a.geometry_key AND r.role='NAMING'
  LEFT JOIN occccad.artifact_objects o ON o.id=r.object_id
  WHERE d.document_type='PART' AND d.deleted_at IS NULL AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(COALESCE(v.model_json->'features','[]'::jsonb)) f
    WHERE upper(f->>'type')='IMPORT_BODY'
  )
)
SELECT CASE WHEN current_head THEN 'HEAD' ELSE 'HISTORY' END AS snapshot,
       CASE WHEN geometry_key IS NULL THEN 'NO_GEOMETRY'
            WHEN artifact_key IS NULL THEN 'MISSING_GEOMETRY_ARTIFACT'
            WHEN NULLIF(trim(digest),'') IS NULL AND object_id IS NULL THEN 'NAMING_ABSENT'
            WHEN NULLIF(trim(digest),'') IS NULL THEN 'MISSING_DIGEST'
            WHEN object_id IS NULL THEN 'MISSING_CONTENT'
            ELSE 'PRESENT_REQUIRES_VALIDATION' END AS naming_metadata,
       count(*) AS revisions, count(DISTINCT id) AS documents
FROM imported GROUP BY 1,2 ORDER BY 1,2;

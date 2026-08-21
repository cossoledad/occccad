-- Re-render existing heads with the svg-v3 standard-view renderer. Older
-- previews remain as immutable artifacts but are no longer served by the API.
UPDATE occccad.document_previews
SET state='STALE', updated_at=now()
WHERE state='READY' AND renderer_version <> 'svg-v3';

WITH previews AS (
    SELECT d.id AS document_id,
           d.head_version_id AS version_id,
           d.owner_user_id AS requested_by_user_id,
           md5(d.id::text || ':' || d.head_version_id::text || ':svg-v3') ||
           md5('svg-v3:' || d.id::text || ':' || d.head_version_id::text) AS identity
    FROM occccad.documents d
    WHERE d.deleted_at IS NULL
      AND d.head_version_id IS NOT NULL
)
INSERT INTO occccad.jobs(
    job_type,document_id,version_id,requested_by_user_id,payload,idempotency_key)
SELECT 'THUMBNAIL_RENDER',document_id,version_id,requested_by_user_id,
       jsonb_build_object('previewIdentity',identity,'rendererVersion','svg-v3'),identity
FROM previews
ON CONFLICT(job_type,idempotency_key) DO NOTHING;

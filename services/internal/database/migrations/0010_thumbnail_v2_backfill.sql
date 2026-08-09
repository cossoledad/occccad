-- Replace the placeholder cube previews with geometry-derived svg-v2 previews.
-- New commands enqueue previews in the API; this one-time backfill covers all
-- documents that already existed before the renderer upgrade.
WITH previews AS (
    SELECT d.id AS document_id,
           d.head_version_id AS version_id,
           d.owner_user_id AS requested_by_user_id,
           md5(d.id::text || ':' || d.head_version_id::text || ':preview-v2-backfill') ||
           md5('preview-v2-backfill:' || d.id::text || ':' || d.head_version_id::text) AS identity
    FROM occccad.documents d
    WHERE d.deleted_at IS NULL
      AND d.head_version_id IS NOT NULL
)
INSERT INTO occccad.jobs(
    job_type,document_id,version_id,requested_by_user_id,payload,idempotency_key)
SELECT 'THUMBNAIL_RENDER',document_id,version_id,requested_by_user_id,
       jsonb_build_object('previewIdentity',identity,'rendererVersion','svg-v2'),identity
FROM previews
ON CONFLICT(job_type,idempotency_key) DO NOTHING;

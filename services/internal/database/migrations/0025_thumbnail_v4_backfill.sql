-- Display artifacts are rebuilt without touching models, revisions or B-Rep.
UPDATE occccad.document_previews SET state='STALE', updated_at=now()
WHERE state='READY' AND renderer_version <> 'png-v4';

WITH previews AS (
    SELECT d.id AS document_id, d.head_version_id AS version_id,
           d.owner_user_id AS requested_by_user_id,
           md5(d.id::text || ':' || d.head_version_id::text || ':png-v4') ||
           md5('png-v4:' || d.id::text || ':' || d.head_version_id::text) AS identity
    FROM occccad.documents d
    WHERE d.deleted_at IS NULL AND d.head_version_id IS NOT NULL
)
INSERT INTO occccad.jobs(job_type,document_id,version_id,requested_by_user_id,payload,idempotency_key)
SELECT 'THUMBNAIL_RENDER',document_id,version_id,requested_by_user_id,
       jsonb_build_object('previewIdentity',identity,'rendererVersion','png-v4'),identity
FROM previews
ON CONFLICT(job_type,idempotency_key) DO NOTHING;

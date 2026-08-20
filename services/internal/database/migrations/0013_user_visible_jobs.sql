ALTER TABLE occccad.jobs
    ADD COLUMN IF NOT EXISTS user_visible boolean NOT NULL DEFAULT false;

UPDATE occccad.jobs
SET user_visible = true
WHERE job_type IN ('EXCHANGE_IMPORT','EXCHANGE_EXPORT') AND NOT user_visible;

CREATE INDEX IF NOT EXISTS jobs_user_visible_idx
    ON occccad.jobs(requested_by_user_id,created_at DESC)
    WHERE user_visible;

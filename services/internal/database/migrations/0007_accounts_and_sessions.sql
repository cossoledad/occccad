ALTER TABLE occccad.users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE occccad.users
    ADD COLUMN IF NOT EXISTS platform_role text NOT NULL DEFAULT 'MEMBER',
    ADD COLUMN IF NOT EXISTS password_hash text,
    ADD COLUMN IF NOT EXISTS must_change_password boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS approved_at timestamptz,
    ADD COLUMN IF NOT EXISTS approved_by_user_id uuid REFERENCES occccad.users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS failed_login_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS locked_until timestamptz,
    ADD CONSTRAINT users_status_check CHECK (status IN ('PENDING','ACTIVE','DISABLED')),
    ADD CONSTRAINT users_platform_role_check CHECK (platform_role IN ('ADMIN','MEMBER'));

UPDATE occccad.documents
SET owner_user_id='00000000-0000-7000-8000-000000000001'
WHERE owner_user_id IN (
    '00000000-0000-7000-8000-000000000002',
    '00000000-0000-7000-8000-000000000003'
);

UPDATE occccad.folders
SET owner_user_id='00000000-0000-7000-8000-000000000001'
WHERE owner_user_id IN (
    '00000000-0000-7000-8000-000000000002',
    '00000000-0000-7000-8000-000000000003'
);

UPDATE occccad.teams
SET owner_user_id='00000000-0000-7000-8000-000000000001'
WHERE owner_user_id IN (
    '00000000-0000-7000-8000-000000000002',
    '00000000-0000-7000-8000-000000000003'
);

UPDATE occccad.resource_grants
SET granted_by_user_id='00000000-0000-7000-8000-000000000001'
WHERE granted_by_user_id IN (
    '00000000-0000-7000-8000-000000000002',
    '00000000-0000-7000-8000-000000000003'
);

DELETE FROM occccad.teams WHERE id='00000000-0000-7000-8000-000000000101';
DELETE FROM occccad.users WHERE id IN (
    '00000000-0000-7000-8000-000000000002',
    '00000000-0000-7000-8000-000000000003'
);

UPDATE occccad.users SET
    email='admin@occccad.local',
    display_name='Administrator',
    status='ACTIVE',
    platform_role='ADMIN',
    approved_at=COALESCE(approved_at,now()),
    updated_at=now()
WHERE id='00000000-0000-7000-8000-000000000001';

CREATE TABLE IF NOT EXISTS occccad.user_sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
    token_hash char(64) NOT NULL UNIQUE,
    csrf_hash char(64) NOT NULL,
    user_agent text NOT NULL DEFAULT '',
    remote_address text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_sessions_active_idx
    ON occccad.user_sessions(token_hash,expires_at) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS user_sessions_user_idx
    ON occccad.user_sessions(user_id,created_at DESC);

CREATE TABLE IF NOT EXISTS occccad.account_audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES occccad.users(id) ON DELETE SET NULL,
    target_user_id uuid REFERENCES occccad.users(id) ON DELETE SET NULL,
    action text NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS account_audit_target_idx
    ON occccad.account_audit_events(target_user_id,id DESC);

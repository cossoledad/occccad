CREATE TABLE IF NOT EXISTS occccad.users (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    email text NOT NULL,
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 120),
    status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'DISABLED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_idx ON occccad.users(lower(email));

CREATE TABLE IF NOT EXISTS occccad.teams (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '',
    owner_user_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS occccad.team_members (
    team_id uuid NOT NULL REFERENCES occccad.teams(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
    role text NOT NULL DEFAULT 'MEMBER' CHECK (role IN ('MEMBER', 'ADMIN')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

INSERT INTO occccad.users(id,email,display_name) VALUES
    ('00000000-0000-7000-8000-000000000001','owner@occccad.local','Ganjb Owner'),
    ('00000000-0000-7000-8000-000000000002','editor@occccad.local','Demo Editor'),
    ('00000000-0000-7000-8000-000000000003','viewer@occccad.local','Demo Viewer')
ON CONFLICT (id) DO NOTHING;

INSERT INTO occccad.teams(id,name,description,owner_user_id)
VALUES ('00000000-0000-7000-8000-000000000101','Design Team','v0.0.6 local collaboration team',
        '00000000-0000-7000-8000-000000000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO occccad.team_members(team_id,user_id,role) VALUES
    ('00000000-0000-7000-8000-000000000101','00000000-0000-7000-8000-000000000001','ADMIN'),
    ('00000000-0000-7000-8000-000000000101','00000000-0000-7000-8000-000000000002','MEMBER')
ON CONFLICT (team_id,user_id) DO NOTHING;

ALTER TABLE occccad.documents ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES occccad.users(id) ON DELETE RESTRICT;
ALTER TABLE occccad.folders ADD COLUMN IF NOT EXISTS owner_user_id uuid REFERENCES occccad.users(id) ON DELETE RESTRICT;

UPDATE occccad.documents SET owner_user_id='00000000-0000-7000-8000-000000000001' WHERE owner_user_id IS NULL;
UPDATE occccad.folders SET owner_user_id='00000000-0000-7000-8000-000000000001' WHERE owner_user_id IS NULL;

ALTER TABLE occccad.documents ALTER COLUMN owner_user_id SET NOT NULL;
ALTER TABLE occccad.folders ALTER COLUMN owner_user_id SET NOT NULL;

CREATE TABLE IF NOT EXISTS occccad.resource_grants (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    resource_type text NOT NULL CHECK (resource_type IN ('DOCUMENT', 'FOLDER')),
    resource_id uuid NOT NULL,
    user_id uuid REFERENCES occccad.users(id) ON DELETE CASCADE,
    team_id uuid REFERENCES occccad.teams(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('VIEWER', 'EDITOR')),
    granted_by_user_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((user_id IS NOT NULL)::integer + (team_id IS NOT NULL)::integer = 1)
);

CREATE UNIQUE INDEX IF NOT EXISTS resource_grants_user_idx
    ON occccad.resource_grants(resource_type,resource_id,user_id) WHERE user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS resource_grants_team_idx
    ON occccad.resource_grants(resource_type,resource_id,team_id) WHERE team_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS resource_grants_resource_idx
    ON occccad.resource_grants(resource_type,resource_id);

CREATE TABLE IF NOT EXISTS occccad.access_audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id uuid REFERENCES occccad.users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text,
    resource_id uuid,
    request_id text,
    trace_id text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS access_audit_resource_idx
    ON occccad.access_audit_events(resource_type,resource_id,id DESC);
CREATE INDEX IF NOT EXISTS access_audit_actor_idx
    ON occccad.access_audit_events(actor_user_id,id DESC);

CREATE OR REPLACE FUNCTION occccad.role_level(role_name text) RETURNS integer
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE role_name WHEN 'OWNER' THEN 30 WHEN 'EDITOR' THEN 20 WHEN 'VIEWER' THEN 10 ELSE 0 END
$$;

CREATE OR REPLACE FUNCTION occccad.role_name(role_level integer) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN role_level >= 30 THEN 'OWNER' WHEN role_level >= 20 THEN 'EDITOR'
                WHEN role_level >= 10 THEN 'VIEWER' ELSE 'NONE' END
$$;

CREATE OR REPLACE FUNCTION occccad.effective_folder_role(target_folder uuid, principal_user uuid)
RETURNS integer LANGUAGE sql STABLE AS $$
    WITH RECURSIVE ancestors AS (
        SELECT id,parent_id,owner_user_id FROM occccad.folders WHERE id=target_folder
        UNION ALL
        SELECT f.id,f.parent_id,f.owner_user_id FROM occccad.folders f
        JOIN ancestors a ON f.id=a.parent_id
    ), candidates AS (
        SELECT CASE WHEN a.owner_user_id=principal_user THEN 30 ELSE 0 END AS level FROM ancestors a
        UNION ALL
        SELECT occccad.role_level(g.role) FROM occccad.resource_grants g
        JOIN ancestors a ON g.resource_type='FOLDER' AND g.resource_id=a.id
        WHERE g.user_id=principal_user OR EXISTS (
            SELECT 1 FROM occccad.team_members tm
            WHERE tm.team_id=g.team_id AND tm.user_id=principal_user)
    ) SELECT COALESCE(max(level),0) FROM candidates
$$;

CREATE OR REPLACE FUNCTION occccad.effective_document_role(target_document uuid, principal_user uuid)
RETURNS integer LANGUAGE sql STABLE AS $$
    WITH document_data AS (
        SELECT owner_user_id,folder_id FROM occccad.documents WHERE id=target_document
    ), candidates AS (
        SELECT CASE WHEN owner_user_id=principal_user THEN 30 ELSE 0 END AS level FROM document_data
        UNION ALL
        SELECT occccad.effective_folder_role(folder_id,principal_user)
        FROM document_data WHERE folder_id IS NOT NULL
        UNION ALL
        SELECT occccad.role_level(g.role) FROM occccad.resource_grants g
        WHERE g.resource_type='DOCUMENT' AND g.resource_id=target_document
          AND (g.user_id=principal_user OR EXISTS (
              SELECT 1 FROM occccad.team_members tm
              WHERE tm.team_id=g.team_id AND tm.user_id=principal_user))
    ) SELECT COALESCE(max(level),0) FROM candidates
$$;

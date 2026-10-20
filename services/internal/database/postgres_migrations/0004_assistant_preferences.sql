ALTER TABLE occccad.assistant_credentials ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE occccad.assistant_credentials ADD COLUMN thinking TEXT NOT NULL DEFAULT 'off' CHECK (thinking IN ('off','low','high','max'));

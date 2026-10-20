ALTER TABLE occccad.assistant_credentials ADD COLUMN budget TEXT NOT NULL DEFAULT 'medium' CHECK (budget IN ('low','medium','high'));

ALTER TABLE occccad.geometry_artifacts
    ADD COLUMN IF NOT EXISTS reference_geometry_json jsonb NOT NULL DEFAULT '{"datumPlanes":[],"axisSystems":[]}'::jsonb,
    ADD COLUMN IF NOT EXISTS worker_id text NOT NULL DEFAULT 'metadata-service';

ALTER TABLE occccad.geometry_artifacts
    DROP CONSTRAINT IF EXISTS geometry_artifacts_volume_check;

ALTER TABLE occccad.geometry_artifacts
    ADD CONSTRAINT geometry_artifacts_volume_check CHECK (volume >= 0);

COMMENT ON COLUMN occccad.geometry_artifacts.reference_geometry_json IS
    'Display-layer reference geometry mirrored into OCCCCAD_reference_geometry in GLB.';
COMMENT ON COLUMN occccad.geometry_artifacts.worker_id IS
    'Geometry worker that produced the solid, or metadata-service for reference-only artifacts.';

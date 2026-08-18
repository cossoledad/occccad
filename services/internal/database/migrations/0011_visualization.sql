ALTER TABLE occccad.geometry_artifacts
    ADD COLUMN IF NOT EXISTS visualization_json jsonb NOT NULL DEFAULT '{"schemaVersion":1,"referenceGeometry":{"datumPlanes":[],"axisSystems":[]},"primitives":[]}'::jsonb,
    ADD COLUMN IF NOT EXISTS worker_id text NOT NULL DEFAULT 'metadata-service';

ALTER TABLE occccad.geometry_artifacts
    DROP CONSTRAINT IF EXISTS geometry_artifacts_volume_check;

ALTER TABLE occccad.geometry_artifacts
    ADD CONSTRAINT geometry_artifacts_volume_check CHECK (volume >= 0);

COMMENT ON COLUMN occccad.geometry_artifacts.visualization_json IS
    'Versioned reference and selectable non-solid geometry mirrored into OCCCCAD_visualization in GLB.';
COMMENT ON COLUMN occccad.geometry_artifacts.worker_id IS
    'Geometry worker that produced the solid, or metadata-service for visualization-only artifacts.';

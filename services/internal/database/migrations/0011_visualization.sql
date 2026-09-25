ALTER TABLE occccad.geometry_artifacts
    ADD COLUMN IF NOT EXISTS reference_geometry_json jsonb NOT NULL DEFAULT '{"schemaVersion":1,"referenceGeometry":{"datumPlanes":[],"axisSystems":[]},"primitives":[]}'::jsonb,
    ADD COLUMN IF NOT EXISTS worker_id text NOT NULL DEFAULT 'metadata-service';

ALTER TABLE occccad.geometry_artifacts
    DROP CONSTRAINT IF EXISTS geometry_artifacts_volume_check;

ALTER TABLE occccad.geometry_artifacts
    ADD CONSTRAINT geometry_artifacts_volume_check CHECK (volume >= 0);

COMMENT ON COLUMN occccad.geometry_artifacts.reference_geometry_json IS
    'Bounded business reference geometry only. Display primitives are authoritative in the VISUAL GLB artifact.';
COMMENT ON COLUMN occccad.geometry_artifacts.worker_id IS
    'Geometry worker that produced the solid, or metadata-service for visualization-only artifacts.';

ALTER TABLE occccad.geometry_artifacts ADD CONSTRAINT reference_geometry_has_no_display_payload CHECK (
 reference_geometry_json->'primitives' IS NULL OR reference_geometry_json->'primitives' IN ('[]'::jsonb,'null'::jsonb)
);

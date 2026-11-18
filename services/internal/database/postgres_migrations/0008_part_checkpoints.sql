CREATE TABLE occccad.part_stage_checkpoints (
    body_id text NOT NULL,
    feature_id text NOT NULL,
    input_digest text NOT NULL,
    geometry_key text NOT NULL REFERENCES occccad.geometry_artifacts(geometry_key) ON DELETE CASCADE,
    PRIMARY KEY (body_id, feature_id, input_digest)
);

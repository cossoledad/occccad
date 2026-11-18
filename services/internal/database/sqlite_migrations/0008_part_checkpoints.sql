CREATE TABLE part_stage_checkpoints (
    body_id TEXT NOT NULL,
    feature_id TEXT NOT NULL,
    input_digest TEXT NOT NULL,
    geometry_key TEXT NOT NULL REFERENCES geometry_artifacts(geometry_key) ON DELETE CASCADE,
    PRIMARY KEY (body_id, feature_id, input_digest)
);

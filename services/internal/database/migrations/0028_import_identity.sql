-- Immutable import replay inputs. Allocations survive retries and are not caches.
CREATE TABLE occccad.import_definitions (
    id text PRIMARY KEY,
    document_id uuid NOT NULL REFERENCES occccad.documents(id),
    source_geometry_key text NOT NULL REFERENCES occccad.geometry_artifacts(geometry_key),
    source_object_id uuid REFERENCES occccad.artifact_objects(id),
    definition jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Provider names belong to Store implementations, not a closed database enum.
ALTER TABLE occccad.artifact_objects DROP CONSTRAINT artifact_objects_storage_backend_check;
ALTER TABLE occccad.artifact_objects ADD CONSTRAINT artifact_objects_storage_backend_check CHECK (length(storage_backend) > 0);

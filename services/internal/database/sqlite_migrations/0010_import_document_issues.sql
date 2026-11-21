CREATE TABLE import_document_issues (
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    definition_id text NOT NULL,
    code text NOT NULL,
    message text NOT NULL,
    PRIMARY KEY (document_id, job_id, definition_id, code)
);
CREATE INDEX import_definitions_recovery_idx ON import_definitions(document_id, source_object_id);

-- Resolve completed imports through the original idempotent create commands.
INSERT INTO import_document_issues(document_id,job_id,definition_id,code,message)
SELECT DISTINCT COALESCE(c.document_id,j.document_id),j.id,COALESCE(w.value->>'definitionId',''),
       w.value->>'code',w.value->>'message'
FROM jobs j
JOIN json_each(j.payload, '$.exchange.warnings') AS w
LEFT JOIN commands c ON c.request_id=(j.payload->>'requestId') || '/definition/' || (w.value->>'definitionId') || '/document'
    AND c.command_type='CREATE_DOCUMENT' AND c.status='SUCCEEDED'
WHERE j.job_type='EXCHANGE_IMPORT' AND j.state='SUCCEEDED' AND j.document_id IS NOT NULL
  AND w.value->>'code' IS NOT NULL AND w.value->>'message' IS NOT NULL
ON CONFLICT DO NOTHING;

UPDATE jobs SET max_attempts=2 WHERE job_type='EXCHANGE_IMPORT' AND state IN ('QUEUED','RETRY_WAIT') AND attempt_count<2 AND max_attempts=3;

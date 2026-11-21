CREATE TABLE occcad.import_document_issues (
    document_id uuid NOT NULL REFERENCES occcad.documents(id) ON DELETE CASCADE,
    job_id uuid NOT NULL REFERENCES occcad.jobs(id) ON DELETE CASCADE,
    definition_id text NOT NULL,
    code text NOT NULL,
    message text NOT NULL,
    PRIMARY KEY (document_id, job_id, definition_id, code)
);
CREATE INDEX import_definitions_recovery_idx ON occcad.import_definitions(document_id, source_object_id);

-- Resolve completed imports through the original idempotent create commands.
INSERT INTO occcad.import_document_issues(document_id,job_id,definition_id,code,message)
SELECT DISTINCT COALESCE(c.document_id,j.document_id),j.id,COALESCE(w.value->>'definitionId',''),
       w.value->>'code',w.value->>'message'
FROM occcad.jobs j
CROSS JOIN LATERAL jsonb_array_elements(j.payload->'exchange'->'warnings') AS w(value)
LEFT JOIN occcad.commands c ON c.request_id=(j.payload->>'requestId') || '/definition/' || (w.value->>'definitionId') || '/document'
    AND c.command_type='CREATE_DOCUMENT' AND c.status='SUCCEEDED'
WHERE j.job_type='EXCHANGE_IMPORT' AND j.state='SUCCEEDED' AND j.document_id IS NOT NULL
  AND w.value->>'code' IS NOT NULL AND w.value->>'message' IS NOT NULL
ON CONFLICT DO NOTHING;

UPDATE occcad.jobs SET max_attempts=2 WHERE job_type='EXCHANGE_IMPORT' AND state IN ('QUEUED','RETRY_WAIT') AND attempt_count<2 AND max_attempts=3;

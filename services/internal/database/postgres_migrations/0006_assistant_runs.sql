-- Old low/medium/high never becomes an implicit cumulative Run budget.
ALTER TABLE occccad.assistant_credentials DROP COLUMN budget;
ALTER TABLE occccad.assistant_credentials ADD COLUMN token_budget bigint NULL CHECK (token_budget IS NULL OR token_budget>0);
ALTER TABLE occccad.assistant_credentials ADD COLUMN settings_version bigint NOT NULL DEFAULT 1;
CREATE TABLE occccad.assistant_runs (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
 document_id uuid NOT NULL REFERENCES occccad.documents(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL, request_digest TEXT NOT NULL, access_mode TEXT NOT NULL,
 version bigint NOT NULL, epoch bigint NOT NULL, status TEXT NOT NULL,
 payload jsonb NOT NULL, lease_token TEXT NOT NULL DEFAULT '', lease_until bigint NOT NULL DEFAULT 0,
 next_wake bigint NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(actor_id,document_id,request_id)
);
CREATE INDEX assistant_runs_scheduler_idx ON occccad.assistant_runs(status,next_wake,lease_until);
CREATE INDEX assistant_runs_owner_idx ON occccad.assistant_runs(actor_id,document_id,created_at,id);
CREATE UNIQUE INDEX assistant_runs_writer_idx ON occccad.assistant_runs(document_id) WHERE access_mode='WRITE' AND status NOT IN ('COMPLETED','STOPPED');
CREATE TABLE occccad.assistant_inputs (
 id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES occccad.assistant_runs(id) ON DELETE CASCADE,
 sequence bigint NOT NULL, request_id TEXT NOT NULL, request_digest TEXT NOT NULL,
 kind TEXT NOT NULL, status TEXT NOT NULL, payload jsonb NOT NULL,
 lease_token TEXT NOT NULL DEFAULT '', lease_until bigint NOT NULL DEFAULT 0,
 pending_call_id TEXT NOT NULL DEFAULT '', UNIQUE(run_id,sequence), UNIQUE(run_id,request_id)
);
CREATE INDEX assistant_inputs_queue_idx ON occccad.assistant_inputs(kind,status,lease_until);
CREATE TABLE occccad.assistant_run_events (
 run_id uuid NOT NULL REFERENCES occccad.assistant_runs(id) ON DELETE CASCADE,
 sequence bigint NOT NULL, epoch bigint NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(run_id,sequence)
);
CREATE TABLE occccad.assistant_stages (
 run_id uuid NOT NULL REFERENCES occccad.assistant_runs(id) ON DELETE CASCADE,
 sequence bigint NOT NULL, epoch bigint NOT NULL, goal_version bigint NOT NULL,
 task_id uuid NOT NULL REFERENCES occccad.automation_tasks(id), status TEXT NOT NULL,
 PRIMARY KEY(run_id,sequence), UNIQUE(run_id,task_id)
);
CREATE TABLE occccad.assistant_model_calls (
 id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES occccad.assistant_runs(id) ON DELETE CASCADE,
 input_id TEXT NOT NULL DEFAULT '', logical_id uuid NOT NULL, attempt bigint NOT NULL,
 epoch bigint NOT NULL, status TEXT NOT NULL, payload jsonb NOT NULL,
 started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', UNIQUE(logical_id,attempt)
);
CREATE INDEX assistant_calls_run_idx ON occccad.assistant_model_calls(run_id,started_at,id);
CREATE TABLE occccad.assistant_tool_results (
 call_id uuid NOT NULL REFERENCES occccad.assistant_model_calls(id) ON DELETE CASCADE,
 tool_id TEXT NOT NULL, ordinal INTEGER NOT NULL, name TEXT NOT NULL,
 arguments jsonb NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(call_id,tool_id)
);
CREATE TABLE occccad.assistant_contracts (
 run_id uuid NOT NULL REFERENCES occccad.assistant_runs(id) ON DELETE CASCADE,
 operation_id TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(run_id,operation_id)
);
CREATE TABLE occccad.assistant_settings_events (
 actor_id uuid NOT NULL REFERENCES occccad.users(id) ON DELETE CASCADE,
 version bigint NOT NULL, payload jsonb NOT NULL, PRIMARY KEY(actor_id,version)
);

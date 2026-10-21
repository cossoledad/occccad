-- Old low/medium/high never becomes an implicit cumulative Run budget.
ALTER TABLE assistant_credentials DROP COLUMN budget;
ALTER TABLE assistant_credentials ADD COLUMN token_budget INTEGER NULL CHECK (token_budget IS NULL OR token_budget>0);
ALTER TABLE assistant_credentials ADD COLUMN settings_version INTEGER NOT NULL DEFAULT 1;
CREATE TABLE assistant_runs (
 id TEXT PRIMARY KEY, actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL, request_digest TEXT NOT NULL, access_mode TEXT NOT NULL,
 version INTEGER NOT NULL, epoch INTEGER NOT NULL, status TEXT NOT NULL,
 payload TEXT NOT NULL, lease_token TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0,
 next_wake INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 UNIQUE(actor_id,document_id,request_id)
);
CREATE INDEX assistant_runs_scheduler_idx ON assistant_runs(status,next_wake,lease_until);
CREATE INDEX assistant_runs_owner_idx ON assistant_runs(actor_id,document_id,created_at,id);
CREATE UNIQUE INDEX assistant_runs_writer_idx ON assistant_runs(document_id) WHERE access_mode='WRITE' AND status NOT IN ('COMPLETED','STOPPED');
CREATE TABLE assistant_inputs (
 id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
 sequence INTEGER NOT NULL, request_id TEXT NOT NULL, request_digest TEXT NOT NULL,
 kind TEXT NOT NULL, status TEXT NOT NULL, payload TEXT NOT NULL,
 lease_token TEXT NOT NULL DEFAULT '', lease_until INTEGER NOT NULL DEFAULT 0,
 pending_call_id TEXT NOT NULL DEFAULT '', UNIQUE(run_id,sequence), UNIQUE(run_id,request_id)
);
CREATE INDEX assistant_inputs_queue_idx ON assistant_inputs(kind,status,lease_until);
CREATE TABLE assistant_run_events (
 run_id TEXT NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
 sequence INTEGER NOT NULL, epoch INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(run_id,sequence)
);
CREATE TABLE assistant_stages (
 run_id TEXT NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
 sequence INTEGER NOT NULL, epoch INTEGER NOT NULL, goal_version INTEGER NOT NULL,
 task_id TEXT NOT NULL REFERENCES automation_tasks(id), status TEXT NOT NULL,
 PRIMARY KEY(run_id,sequence), UNIQUE(run_id,task_id)
);
CREATE TABLE assistant_model_calls (
 id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
 input_id TEXT NOT NULL DEFAULT '', logical_id TEXT NOT NULL, attempt INTEGER NOT NULL,
 epoch INTEGER NOT NULL, status TEXT NOT NULL, payload TEXT NOT NULL,
 started_at TEXT NOT NULL, finished_at TEXT NOT NULL DEFAULT '', UNIQUE(logical_id,attempt)
);
CREATE INDEX assistant_calls_run_idx ON assistant_model_calls(run_id,started_at,id);
CREATE TABLE assistant_tool_results (
 call_id TEXT NOT NULL REFERENCES assistant_model_calls(id) ON DELETE CASCADE,
 tool_id TEXT NOT NULL, ordinal INTEGER NOT NULL, name TEXT NOT NULL,
 arguments TEXT NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(call_id,tool_id)
);
CREATE TABLE assistant_contracts (
 run_id TEXT NOT NULL REFERENCES assistant_runs(id) ON DELETE CASCADE,
 operation_id TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(run_id,operation_id)
);
CREATE TABLE assistant_settings_events (
 actor_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 version INTEGER NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(actor_id,version)
);

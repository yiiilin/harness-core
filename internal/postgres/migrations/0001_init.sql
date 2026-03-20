-- 0001_init.sql
-- Initial Postgres schema for harness-core durable state.

CREATE TABLE IF NOT EXISTS sessions (
  session_id TEXT PRIMARY KEY,
  task_id TEXT,
  parent_session_id TEXT,
  title TEXT NOT NULL,
  goal TEXT,
  phase TEXT NOT NULL,
  current_step_id TEXT,
  summary TEXT,
  retry_count INTEGER NOT NULL DEFAULT 0,
  execution_state TEXT NOT NULL DEFAULT 'idle',
  in_flight_step_id TEXT,
  pending_approval_id TEXT,
  lease_id TEXT,
  lease_claimed_at BIGINT,
  lease_expires_at BIGINT,
  last_heartbeat_at BIGINT,
  interrupted_at BIGINT,
  metadata_json TEXT,
  version BIGINT NOT NULL DEFAULT 1,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
  task_id TEXT PRIMARY KEY,
  task_type TEXT NOT NULL,
  goal TEXT NOT NULL,
  status TEXT NOT NULL,
  session_id TEXT,
  constraints_json TEXT,
  metadata_json TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS plans (
  plan_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  status TEXT NOT NULL,
  change_reason TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_plans_session_revision
  ON plans(session_id, revision);

CREATE TABLE IF NOT EXISTS plan_steps (
  plan_id TEXT NOT NULL,
  step_index INTEGER NOT NULL DEFAULT 0,
  step_id TEXT NOT NULL,
  title TEXT NOT NULL,
  action_json TEXT NOT NULL,
  verify_json TEXT NOT NULL,
  on_fail_json TEXT,
  status TEXT NOT NULL,
  attempt INTEGER NOT NULL DEFAULT 0,
  reason TEXT,
  metadata_json TEXT,
  started_at BIGINT,
  finished_at BIGINT,
  PRIMARY KEY(plan_id, step_id),
  FOREIGN KEY(plan_id) REFERENCES plans(plan_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS audit_events (
  event_id TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  session_id TEXT,
  task_id TEXT,
  step_id TEXT,
  attempt_id TEXT,
  action_id TEXT,
  trace_id TEXT,
  causation_id TEXT,
  payload_json TEXT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_events_session_created
  ON audit_events(session_id, created_at);

CREATE TABLE IF NOT EXISTS approvals (
  approval_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  task_id TEXT,
  step_id TEXT,
  tool_name TEXT,
  reason TEXT,
  matched_rule TEXT,
  status TEXT NOT NULL,
  reply TEXT,
  step_json TEXT NOT NULL,
  metadata_json TEXT,
  requested_at BIGINT NOT NULL,
  responded_at BIGINT,
  consumed_at BIGINT,
  version BIGINT NOT NULL DEFAULT 1,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approvals_session_requested
  ON approvals(session_id, requested_at);

CREATE TABLE IF NOT EXISTS capability_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  session_id TEXT,
  task_id TEXT,
  step_id TEXT,
  tool_name TEXT NOT NULL,
  version TEXT,
  capability_type TEXT,
  risk_level TEXT,
  metadata_json TEXT,
  resolved_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_capability_snapshots_session_resolved
  ON capability_snapshots(session_id, resolved_at);

CREATE TABLE IF NOT EXISTS context_summaries (
  summary_id TEXT PRIMARY KEY,
  session_id TEXT,
  task_id TEXT,
  strategy TEXT,
  summary_json TEXT,
  metadata_json TEXT,
  original_bytes INTEGER NOT NULL DEFAULT 0,
  compacted_bytes INTEGER NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_context_summaries_session_created
  ON context_summaries(session_id, created_at);

CREATE TABLE IF NOT EXISTS attempts (
  attempt_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  task_id TEXT,
  step_id TEXT,
  approval_id TEXT,
  trace_id TEXT,
  status TEXT NOT NULL,
  step_json TEXT NOT NULL,
  metadata_json TEXT,
  started_at BIGINT NOT NULL,
  finished_at BIGINT
);

CREATE TABLE IF NOT EXISTS action_records (
  action_id TEXT PRIMARY KEY,
  attempt_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  task_id TEXT,
  step_id TEXT,
  tool_name TEXT,
  trace_id TEXT,
  causation_id TEXT,
  status TEXT NOT NULL,
  result_json TEXT NOT NULL,
  metadata_json TEXT,
  started_at BIGINT NOT NULL,
  finished_at BIGINT
);

CREATE TABLE IF NOT EXISTS verification_records (
  verification_id TEXT PRIMARY KEY,
  attempt_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  task_id TEXT,
  step_id TEXT,
  action_id TEXT,
  trace_id TEXT,
  causation_id TEXT,
  status TEXT NOT NULL,
  spec_json TEXT NOT NULL,
  result_json TEXT NOT NULL,
  metadata_json TEXT,
  started_at BIGINT NOT NULL,
  finished_at BIGINT
);

CREATE TABLE IF NOT EXISTS artifacts (
  artifact_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  task_id TEXT,
  step_id TEXT,
  attempt_id TEXT,
  action_id TEXT,
  verification_id TEXT,
  trace_id TEXT,
  name TEXT,
  kind TEXT,
  payload_json TEXT,
  metadata_json TEXT,
  created_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_handles (
  handle_id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  task_id TEXT,
  attempt_id TEXT,
  trace_id TEXT,
  kind TEXT,
  value TEXT,
  metadata_json TEXT,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);

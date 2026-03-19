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
  metadata_json TEXT,
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
  step_id TEXT,
  payload_json TEXT,
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_events_session_created
  ON audit_events(session_id, created_at);

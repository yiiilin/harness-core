ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS runtime_started_at BIGINT;


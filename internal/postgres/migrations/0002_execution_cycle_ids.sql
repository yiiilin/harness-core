ALTER TABLE attempts
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

ALTER TABLE action_records
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

ALTER TABLE verification_records
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

ALTER TABLE artifacts
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

ALTER TABLE runtime_handles
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

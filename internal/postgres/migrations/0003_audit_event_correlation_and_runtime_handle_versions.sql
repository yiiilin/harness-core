CREATE SEQUENCE IF NOT EXISTS audit_events_sequence_seq;

ALTER TABLE audit_events
  ADD COLUMN IF NOT EXISTS sequence BIGINT;

ALTER TABLE audit_events
  ALTER COLUMN sequence SET DEFAULT nextval('audit_events_sequence_seq');

UPDATE audit_events
SET sequence = nextval('audit_events_sequence_seq')
WHERE sequence IS NULL;

ALTER TABLE audit_events
  ALTER COLUMN sequence SET NOT NULL;

ALTER TABLE audit_events
  ADD COLUMN IF NOT EXISTS approval_id TEXT,
  ADD COLUMN IF NOT EXISTS verification_id TEXT,
  ADD COLUMN IF NOT EXISTS cycle_id TEXT;

ALTER TABLE runtime_handles
  ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

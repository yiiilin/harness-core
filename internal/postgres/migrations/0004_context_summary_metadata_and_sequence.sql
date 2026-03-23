CREATE SEQUENCE IF NOT EXISTS context_summaries_sequence_seq;

ALTER TABLE context_summaries
  ADD COLUMN IF NOT EXISTS sequence BIGINT,
  ADD COLUMN IF NOT EXISTS trigger TEXT,
  ADD COLUMN IF NOT EXISTS supersedes_summary_id TEXT;

ALTER TABLE context_summaries
  ALTER COLUMN sequence SET DEFAULT nextval('context_summaries_sequence_seq');

UPDATE context_summaries
SET sequence = nextval('context_summaries_sequence_seq')
WHERE sequence IS NULL;

ALTER TABLE context_summaries
  ALTER COLUMN sequence SET NOT NULL;

DROP INDEX IF EXISTS idx_context_summaries_session_created;

CREATE INDEX IF NOT EXISTS idx_context_summaries_session_created
  ON context_summaries(session_id, created_at, sequence);

CREATE TABLE IF NOT EXISTS records (
  kind TEXT NOT NULL,
  id TEXT NOT NULL,
  workspace_id TEXT NOT NULL DEFAULT '',
  canvas_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT '',
  feedback_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL,
  PRIMARY KEY (kind, id)
);

CREATE INDEX IF NOT EXISTS records_kind_created_idx ON records(kind, created_at, id);
CREATE INDEX IF NOT EXISTS records_kind_canvas_idx ON records(kind, canvas_id, created_at, id);
CREATE INDEX IF NOT EXISTS records_kind_workspace_idx ON records(kind, workspace_id, created_at, id);
CREATE INDEX IF NOT EXISTS records_kind_feedback_idx ON records(kind, feedback_id, created_at, id);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'owner',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  retention_count INTEGER NOT NULL DEFAULT 14,
  retention_days INTEGER NOT NULL DEFAULT 30,
  quota_bytes INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  prefix TEXT NOT NULL,
  key_hash TEXT NOT NULL UNIQUE,
  scopes TEXT NOT NULL,
  last_used_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backups (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  filename TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'archive',
  source TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  tags TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL,
  checksum_sha256 TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_backups_project_created ON backups(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backups_checksum ON backups(checksum_sha256);

CREATE TABLE IF NOT EXISTS webhooks (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  secret TEXT NOT NULL,
  events TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  interval_hours INTEGER NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  actor TEXT NOT NULL,
  action TEXT NOT NULL,
  target TEXT NOT NULL,
  meta TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_logs(created_at DESC);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS destinations (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  account_id TEXT NOT NULL,
  access_key_id TEXT NOT NULL,
  secret_access_key TEXT NOT NULL,
  bucket TEXT NOT NULL,
  jurisdiction TEXT NOT NULL DEFAULT '',
  region TEXT NOT NULL DEFAULT '',
  endpoint TEXT NOT NULL DEFAULT '',
  allow_over_free INTEGER NOT NULL DEFAULT 0,
  key_prefix TEXT NOT NULL DEFAULT 'harbor',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS replicas (
  id TEXT PRIMARY KEY,
  backup_id TEXT NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
  destination_id TEXT NOT NULL REFERENCES destinations(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (backup_id, destination_id)
);

CREATE INDEX IF NOT EXISTS idx_replicas_object ON replicas(destination_id, object_key);

CREATE TABLE IF NOT EXISTS remote_deletes (
  destination_id TEXT NOT NULL REFERENCES destinations(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (destination_id, object_key)
);

CREATE TABLE IF NOT EXISTS setup_guard (
  id INTEGER PRIMARY KEY CHECK (id = 1)
);

INSERT OR IGNORE INTO setup_guard (id) VALUES (1);

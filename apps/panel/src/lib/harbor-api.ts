export type SessionUser = {
  id: string;
  email: string;
  name: string;
  role: string;
  created_at: string;
  updated_at: string;
};

export type Project = {
  id: string;
  name: string;
  slug: string;
  description: string;
  status: "active" | "paused";
  retention_count: number;
  retention_days: number;
  quota_bytes: number;
  used_bytes: number;
  backup_count: number;
  destination_id: string;
  destination_name: string;
  created_at: string;
  updated_at: string;
};

export type ApiKey = {
  id: string;
  project_id: string;
  name: string;
  prefix: string;
  scopes: string;
  last_used_at: string | null;
  created_at: string;
  token?: string;
};

export type Backup = {
  id: string;
  project_id: string;
  project_name?: string;
  name: string;
  filename: string;
  kind: string;
  source: string;
  note: string;
  tags: string;
  size_bytes: number;
  checksum_sha256: string;
  content_type: string;
  created_at: string;
  copies?: BackupCopy[];
};

export type BackupCopy = {
  destination_id: string;
  name: string;
  provider: string;
  status: "synced" | "failed" | "skipped" | string;
  error?: string;
};

export type StorageProvider = {
  id: string;
  name: string;
  group: string;
  summary: string;
  steps: string[];
  free_bytes: number;
  free_note: string;
  account_id: boolean;
  jurisdiction: boolean;
  region: boolean;
  region_placeholder: string;
  endpoint: boolean;
  endpoint_placeholder: string;
  allow_http: boolean;
};

export type StorageProviderList = {
  items: StorageProvider[];
};

export type Destination = {
  id: string;
  name: string;
  provider: string;
  provider_name: string;
  account_id: string;
  access_key_id: string;
  bucket: string;
  jurisdiction: string;
  region: string;
  endpoint: string;
  allow_over_free: boolean;
  key_prefix: string;
  used_bytes: number;
  account_used_bytes: number;
  free_storage_bytes: number;
  created_at: string;
  updated_at: string;
};

export type DestinationList = {
  items: Destination[];
};

export type ReplicateResult = {
  copied: number;
  failed: number;
  skipped: number;
  already: number;
};

export type Webhook = {
  id: string;
  project_id: string;
  url: string;
  secret: string;
  events: string;
  created_at: string;
};

export type Schedule = {
  id: string;
  project_id: string;
  name: string;
  interval_hours: number;
  created_at: string;
  last_backup_at?: string;
  overdue: boolean;
};

export type AuditEvent = {
  id: string;
  actor: string;
  action: string;
  target: string;
  meta: string;
  created_at: string;
};

export type Stats = {
  projects: number;
  backups: number;
  storage_bytes: number;
  backups_last_24h: number;
  overdue_schedules: number;
  recent_backups: Backup[];
  recent_activity: AuditEvent[];
};

export type Settings = {
  instance_name: string;
  version: string;
  data_dir: string;
  max_upload_bytes: number;
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers, credentials: "include" });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: { error?: string } = {};
  if (text) {
    try {
      data = JSON.parse(text) as { error?: string };
    } catch {
      data = { error: text.slice(0, 200) };
    }
  }
  if (!res.ok) {
    throw new ApiError(res.status, data.error || res.statusText);
  }
  return data as T;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exp = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** exp;
  return `${value >= 10 || exp === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[exp]}`;
}

export function formatWhen(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export async function downloadBackup(id: string, filename: string) {
  const res = await fetch(`/api/v1/backups/${id}/download`, { credentials: "include" });
  if (!res.ok) {
    const data = (await res.json().catch(() => ({}))) as { error?: string };
    throw new ApiError(res.status, data.error || "Download failed");
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

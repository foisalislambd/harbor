import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { dangerButtonClass, fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import {
  api,
  formatBytes,
  formatWhen,
  type ApiKey,
  type Backup,
  type Project,
  type Schedule,
  type Webhook,
} from "@/lib/harbor-api";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

export default function ProjectDetailPage() {
  const { projectId = "" } = useParams();
  const navigate = useNavigate();
  const [project, setProject] = useState<Project | null>(null);
  const [keys, setKeys] = useState<ApiKey[]>([]);
  const [hooks, setHooks] = useState<Webhook[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [backups, setBackups] = useState<Backup[]>([]);
  const [freshToken, setFreshToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [keyName, setKeyName] = useState("production");
  const [hookUrl, setHookUrl] = useState("");
  const [scheduleName, setScheduleName] = useState("nightly");
  const [intervalHours, setIntervalHours] = useState(24);

  async function load() {
    const [projectRes, keyRes, hookRes, scheduleRes, backupRes] = await Promise.all([
      api<Project>(`/api/v1/projects/${projectId}`),
      api<{ items: ApiKey[] }>(`/api/v1/projects/${projectId}/keys`),
      api<{ items: Webhook[] }>(`/api/v1/projects/${projectId}/webhooks`),
      api<{ items: Schedule[] }>(`/api/v1/projects/${projectId}/schedules`),
      api<{ items: Backup[] }>(`/api/v1/backups?project_id=${projectId}&limit=8`),
    ]);
    setProject(projectRes);
    setKeys(keyRes.items);
    setHooks(hookRes.items);
    setSchedules(scheduleRes.items);
    setBackups(backupRes.items);
  }

  useEffect(() => {
    load().catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load project"));
  }, [projectId]);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    if (!project) return;
    setError(null);
    try {
      const updated = await api<Project>(`/api/v1/projects/${project.id}`, {
        method: "PATCH",
        body: JSON.stringify({
          name: project.name,
          description: project.description,
          status: project.status,
          retention_count: Number(project.retention_count),
          retention_days: Number(project.retention_days),
          quota_bytes: Number(project.quota_bytes),
        }),
      });
      setProject(updated);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save");
    }
  }

  async function createKey(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      const key = await api<ApiKey>(`/api/v1/projects/${projectId}/keys`, {
        method: "POST",
        body: JSON.stringify({ name: keyName }),
      });
      setFreshToken(key.token ?? null);
      setKeyName("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create key");
    }
  }

  if (error && !project) return <p className="text-sm text-error-500">{error}</p>;
  if (!project) return <AdminLoading message="Loading project…" />;

  const quotaLabel = project.quota_bytes > 0 ? `${formatBytes(project.used_bytes)} / ${formatBytes(project.quota_bytes)}` : `${formatBytes(project.used_bytes)} stored`;

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader
        title={project.name}
        actions={
          <button
            type="button"
            className={dangerButtonClass}
            onClick={() => {
              if (!confirm(`Delete ${project.name} and every backup in it?`)) return;
              void api(`/api/v1/projects/${project.id}`, { method: "DELETE" })
                .then(() => navigate("/projects"))
                .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not delete project"));
            }}
          >
            Delete project
          </button>
        }
      />
      {error && <p className="text-sm text-error-500">{error}</p>}
      <p className="text-sm text-gray-500">Slug {project.slug} · {quotaLabel} · {project.backup_count} backups</p>
      {project.quota_bytes > 0 && project.used_bytes > project.quota_bytes && (
        <p className="text-sm text-orange-700 dark:text-orange-300">
          This project is over its quota. New uploads are rejected until you raise the quota or delete backups.
        </p>
      )}

      <form onSubmit={save} className="admin-card">
        <div className="admin-card-body grid gap-4 md:grid-cols-2">
          <label className="block">
            <span className={labelClass}>Name</span>
            <input className={fieldClass} value={project.name} onChange={(e) => setProject({ ...project, name: e.target.value })} />
          </label>
          <label className="block">
            <span className={labelClass}>Status</span>
            <select className={fieldClass} value={project.status} onChange={(e) => setProject({ ...project, status: e.target.value as Project["status"] })}>
              <option value="active">Active</option>
              <option value="paused">Paused</option>
            </select>
          </label>
          <label className="block md:col-span-2">
            <span className={labelClass}>Description</span>
            <input className={fieldClass} value={project.description} onChange={(e) => setProject({ ...project, description: e.target.value })} />
          </label>
          <label className="block">
            <span className={labelClass}>Keep last N</span>
            <input className={fieldClass} type="number" min={0} value={project.retention_count} onChange={(e) => setProject({ ...project, retention_count: Number(e.target.value) })} />
          </label>
          <label className="block">
            <span className={labelClass}>Keep days</span>
            <input className={fieldClass} type="number" min={0} value={project.retention_days} onChange={(e) => setProject({ ...project, retention_days: Number(e.target.value) })} />
          </label>
          <label className="block">
            <span className={labelClass}>Quota GB</span>
            <input
              className={fieldClass}
              type="number"
              min={0}
              step="0.1"
              value={Math.round((project.quota_bytes / 1024 ** 3) * 1000) / 1000}
              onChange={(e) => setProject({ ...project, quota_bytes: Math.round(Number(e.target.value) * 1024 ** 3) })}
            />
          </label>
          <div className="flex items-end">
            <button className={primaryButtonClass} type="submit">Save project</button>
          </div>
        </div>
      </form>

      <section className="admin-card">
        <div className="admin-card-body space-y-4">
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">API keys</h2>
          <p className="text-sm text-gray-500">Send this key as Authorization: Bearer from the application. The full key is shown once.</p>
          {freshToken && (
            <div className="rounded-xl bg-brand-50 p-3 text-sm break-all text-brand-800 dark:bg-brand-500/10 dark:text-brand-200">
              {freshToken}
            </div>
          )}
          <form onSubmit={createKey} className="flex flex-col gap-2 sm:flex-row">
            <input className={fieldClass} placeholder="Key name" required value={keyName} onChange={(e) => setKeyName(e.target.value)} />
            <button className={primaryButtonClass} type="submit">Create key</button>
          </form>
          <ul className="divide-y divide-gray-100 dark:divide-gray-800">
            {keys.map((key) => (
              <li key={key.id} className="flex items-center justify-between gap-3 py-3 text-sm">
                <div>
                  <p className="font-medium text-gray-900 dark:text-white">{key.name}</p>
                  <p className="text-xs text-gray-500">{key.prefix}… · {key.scopes} · last used {formatWhen(key.last_used_at)}</p>
                </div>
                <button
                  type="button"
                  className={dangerButtonClass}
                  onClick={() => {
                    void api(`/api/v1/projects/${projectId}/keys/${key.id}`, { method: "DELETE" }).then(load).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not revoke key"));
                  }}
                >
                  Revoke
                </button>
              </li>
            ))}
          </ul>
        </div>
      </section>

      <div className="grid gap-4 lg:grid-cols-2">
        <section className="admin-card">
          <div className="admin-card-body space-y-4">
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Expected schedule</h2>
            <p className="text-sm text-gray-500">Harbor marks a schedule overdue when no backup arrives within the interval.</p>
            <form
              className="flex flex-col gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                void api(`/api/v1/projects/${projectId}/schedules`, {
                  method: "POST",
                  body: JSON.stringify({ name: scheduleName, interval_hours: Number(intervalHours) }),
                }).then(load).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not add schedule"));
              }}
            >
              <input className={fieldClass} value={scheduleName} onChange={(e) => setScheduleName(e.target.value)} required />
              <input className={fieldClass} type="number" min={1} value={intervalHours} onChange={(e) => setIntervalHours(Number(e.target.value))} />
              <button className={quietButtonClass} type="submit">Add schedule</button>
            </form>
            <ul className="space-y-2 text-sm">
              {schedules.map((schedule) => (
                <li key={schedule.id} className="flex items-center justify-between gap-2">
                  <span>
                    {schedule.name} · every {schedule.interval_hours}h · {schedule.overdue ? "overdue" : "on time"}
                  </span>
                  <button type="button" className={dangerButtonClass} onClick={() => void api(`/api/v1/projects/${projectId}/schedules/${schedule.id}`, { method: "DELETE" }).then(load).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove schedule"))}>
                    Remove
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </section>
        <section className="admin-card">
          <div className="admin-card-body space-y-4">
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Webhooks</h2>
            <p className="text-sm text-gray-500">Harbor POSTs JSON and signs the body with an X-Harbor-Signature header.</p>
            <form
              className="flex flex-col gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                void api(`/api/v1/projects/${projectId}/webhooks`, {
                  method: "POST",
                  body: JSON.stringify({ url: hookUrl }),
                }).then(() => {
                  setHookUrl("");
                  return load();
                }).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not add webhook"));
              }}
            >
              <input className={fieldClass} placeholder="https://example.com/hooks/harbor" value={hookUrl} onChange={(e) => setHookUrl(e.target.value)} required />
              <button className={quietButtonClass} type="submit">Add webhook</button>
            </form>
            <ul className="space-y-3 text-sm">
              {hooks.map((hook) => (
                <li key={hook.id} className="space-y-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate">{hook.url}</span>
                    <button type="button" className={dangerButtonClass} onClick={() => void api(`/api/v1/projects/${projectId}/webhooks/${hook.id}`, { method: "DELETE" }).then(load).catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not remove webhook"))}>
                      Remove
                    </button>
                  </div>
                  <p className="text-xs break-all text-gray-500">secret {hook.secret}</p>
                </li>
              ))}
            </ul>
          </div>
        </section>
      </div>

      <section className="admin-card">
        <div className="admin-card-body">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="text-base font-semibold text-gray-900 dark:text-white">Latest backups</h2>
            <Link to={`/backups?project_id=${project.id}`} className="text-sm font-medium text-brand-600">Open in backups</Link>
          </div>
          <ul className="divide-y divide-gray-100 text-sm dark:divide-gray-800">
            {backups.map((backup) => (
              <li key={backup.id} className="flex justify-between gap-3 py-2">
                <span className="truncate">{backup.name}</span>
                <span className="shrink-0 text-gray-500">{formatBytes(backup.size_bytes)} · {formatWhen(backup.created_at)}</span>
              </li>
            ))}
            {backups.length === 0 && <li className="py-2 text-gray-500">Nothing stored for this project yet.</li>}
          </ul>
        </div>
      </section>
    </div>
  );
}

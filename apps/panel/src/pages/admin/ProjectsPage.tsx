import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminAddButton } from "@/components/admin/ui/admin-add-button";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Project } from "@/lib/harbor-api";
import { FolderKanban } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";

const emptyForm = {
  name: "",
  description: "",
  retention_count: 14,
  retention_days: 30,
  quota_gb: 0,
};

export default function ProjectsPage() {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function load() {
    const res = await api<{ items: Project[] }>("/api/v1/projects");
    setProjects(res.items);
  }

  useEffect(() => {
    load().catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load projects"));
  }, []);

  async function createProject(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await api("/api/v1/projects", {
        method: "POST",
        body: JSON.stringify({
          name: form.name,
          description: form.description,
          retention_count: Number(form.retention_count),
          retention_days: Number(form.retention_days),
          quota_bytes: Math.round(Number(form.quota_gb) * 1024 * 1024 * 1024),
        }),
      });
      setOpen(false);
      setForm(emptyForm);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create project");
    } finally {
      setSaving(false);
    }
  }

  if (!projects && !error) return <AdminLoading message="Loading projects…" />;

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader title="Projects" actions={<AdminAddButton label="New project" onClick={() => setOpen(true)} />} />
      <p className="text-sm text-gray-500 dark:text-gray-400">
        Each project is one application. Give it an API key, then that app can push archives, database dumps, or files into this vault.
      </p>
      {error && <p className="text-sm text-error-500">{error}</p>}
      {projects && projects.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState
            icon={FolderKanban}
            title="No projects yet"
            description="Add the first application you want to back up. You can keep as many as you need."
            action={<AdminAddButton label="New project" onClick={() => setOpen(true)} />}
          />
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {projects?.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`} className="admin-card transition hover:border-brand-300">
              <div className="admin-card-body">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h2 className="truncate text-base font-semibold text-gray-900 dark:text-white">{project.name}</h2>
                    <p className="mt-1 line-clamp-2 text-sm text-gray-500">{project.description || "No description"}</p>
                  </div>
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                      project.status === "active"
                        ? "bg-success-50 text-success-600 dark:bg-success-500/15 dark:text-success-500"
                        : "bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300"
                    }`}
                  >
                    {project.status}
                  </span>
                </div>
                <dl className="mt-4 grid grid-cols-2 gap-2 text-xs text-gray-500">
                  <div>
                    <dt>Backups</dt>
                    <dd className="text-sm font-medium text-gray-900 dark:text-white">{project.backup_count}</dd>
                  </div>
                  <div>
                    <dt>Stored</dt>
                    <dd className="text-sm font-medium text-gray-900 dark:text-white">{formatBytes(project.used_bytes)}</dd>
                  </div>
                </dl>
              </div>
            </Link>
          ))}
        </div>
      )}

      {open && (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-gray-950/40 p-4 sm:items-center" onClick={() => setOpen(false)}>
          <form
            onClick={(e) => e.stopPropagation()}
            onSubmit={createProject}
            className="admin-card w-full max-w-lg"
          >
            <div className="admin-card-body space-y-4">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white">New project</h2>
              <label className="block">
                <span className={labelClass}>Name</span>
                <input className={fieldClass} required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </label>
              <label className="block">
                <span className={labelClass}>Description</span>
                <input className={fieldClass} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
              </label>
              <div className="grid grid-cols-3 gap-3">
                <label className="block">
                  <span className={labelClass}>Keep last</span>
                  <input className={fieldClass} type="number" min={0} value={form.retention_count} onChange={(e) => setForm({ ...form, retention_count: Number(e.target.value) })} />
                </label>
                <label className="block">
                  <span className={labelClass}>Days</span>
                  <input className={fieldClass} type="number" min={0} value={form.retention_days} onChange={(e) => setForm({ ...form, retention_days: Number(e.target.value) })} />
                </label>
                <label className="block">
                  <span className={labelClass}>Quota GB</span>
                  <input className={fieldClass} type="number" min={0} value={form.quota_gb} onChange={(e) => setForm({ ...form, quota_gb: Number(e.target.value) })} />
                </label>
              </div>
              <p className="text-xs text-gray-500">Zero means unlimited. Retention removes older snapshots after a new backup arrives.</p>
              <div className="flex justify-end gap-2">
                <button type="button" className={quietButtonClass} onClick={() => setOpen(false)}>Cancel</button>
                <button type="submit" className={primaryButtonClass} disabled={saving}>{saving ? "Saving…" : "Create"}</button>
              </div>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}

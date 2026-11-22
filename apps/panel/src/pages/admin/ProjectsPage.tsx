import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminAddButton } from "@/components/admin/ui/admin-add-button";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, formatBytes, type Project } from "@/lib/harbor-api";
import { Archive, FolderKanban, HardDrive, X } from "lucide-react";
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
    let cancelled = false;
    api<{ items: Project[] }>("/api/v1/projects")
      .then((res) => {
        if (!cancelled) setProjects(res.items);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Could not load projects");
      });
    return () => {
      cancelled = true;
    };
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
      <AdminPageHeader
        title="Projects"
        actions={<AdminAddButton label="New project" onClick={() => setOpen(true)} />}
      />
      {error && <p className="text-sm text-error-500">{error}</p>}
      {projects && projects.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState
            icon={FolderKanban}
            title="No projects yet"
            action={<AdminAddButton label="New project" onClick={() => setOpen(true)} />}
          />
        </div>
      ) : (
        <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          {projects?.map((project) => (
            <Link key={project.id} to={`/projects/${project.id}`} className="admin-card transition hover:border-brand-300 hover:shadow-sm">
              <div className="admin-card-body">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="admin-icon-well h-10 w-10 rounded-xl">
                      <FolderKanban className="h-[18px] w-[18px]" aria-hidden />
                    </span>
                    <div className="min-w-0">
                      <h2 className="truncate text-base font-semibold text-gray-900 dark:text-white">{project.name}</h2>
                      <p className="mt-0.5 line-clamp-2 text-sm text-gray-500">{project.description || "No description"}</p>
                    </div>
                  </div>
                  <span
                    className={`shrink-0 rounded-full px-2 py-0.5 text-xs font-medium capitalize ${
                      project.status === "active"
                        ? "bg-success-50 text-success-600 dark:bg-success-500/15 dark:text-success-500"
                        : "bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300"
                    }`}
                  >
                    {project.status}
                  </span>
                </div>
                <dl className="mt-4 grid grid-cols-2 gap-2 border-t border-gray-100 pt-3 text-xs text-gray-500 dark:border-gray-800">
                  <div className="flex items-center gap-2">
                    <Archive className="h-3.5 w-3.5 text-gray-400" aria-hidden />
                    <div>
                      <dt>Backups</dt>
                      <dd className="text-sm font-medium text-gray-900 dark:text-white">{project.backup_count}</dd>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <HardDrive className="h-3.5 w-3.5 text-gray-400" aria-hidden />
                    <div>
                      <dt>Stored</dt>
                      <dd className="text-sm font-medium text-gray-900 dark:text-white">{formatBytes(project.used_bytes)}</dd>
                    </div>
                  </div>
                </dl>
                <p className="mt-3 text-xs text-gray-500">{project.destination_name ? `Also copies to ${project.destination_name}` : "This machine only"}</p>
              </div>
            </Link>
          ))}
        </div>
      )}

      {open && (
        <div className="fixed inset-0 z-50 flex items-end justify-center bg-gray-950/50 p-4 sm:items-center" onClick={() => setOpen(false)}>
          <form
            onClick={(e) => e.stopPropagation()}
            onSubmit={createProject}
            className="admin-dialog w-full max-w-md overflow-hidden rounded-xl border border-gray-200 shadow-xl dark:border-[#3a4860]"
          >
            <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-[#3a4860]">
              <h2 className="text-base font-semibold text-gray-900 dark:text-white">New project</h2>
              <button type="button" onClick={() => setOpen(false)} className="flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:hover:bg-white/10" aria-label="Close">
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="space-y-4 px-5 py-5">
              <label className="block">
                <span className={labelClass}>Name</span>
                <input className={fieldClass} required placeholder="Photos" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              </label>
              <label className="block">
                <span className={labelClass}>Description</span>
                <input className={fieldClass} placeholder="Optional" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
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
            </div>
            <div className="flex justify-end gap-2 border-t border-gray-200 px-5 py-4 dark:border-[#3a4860]">
              <button type="button" className={quietButtonClass} onClick={() => setOpen(false)}>Cancel</button>
              <button type="submit" className={primaryButtonClass} disabled={saving}>{saving ? "Saving…" : "Create"}</button>
            </div>
          </form>
        </div>
      )}
    </div>
  );
}

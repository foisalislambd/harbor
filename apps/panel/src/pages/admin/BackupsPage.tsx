import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { dangerButtonClass, fieldClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, downloadBackup, formatBytes, formatWhen, type Backup, type Project } from "@/lib/harbor-api";
import { Archive } from "lucide-react";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

export default function BackupsPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const projectId = params.get("project_id") ?? "";
  const [projects, setProjects] = useState<Project[]>([]);
  const [items, setItems] = useState<Backup[] | null>(null);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [uploadProject, setUploadProject] = useState(projectId);
  const [file, setFile] = useState<File | null>(null);
  const [kind, setKind] = useState("archive");
  const [uploading, setUploading] = useState(false);

  async function load(nextQ = q, nextProject = projectId) {
    const query = new URLSearchParams();
    if (nextQ) query.set("q", nextQ);
    if (nextProject) query.set("project_id", nextProject);
    query.set("limit", "50");
    const res = await api<{ items: Backup[]; total: number }>(`/api/v1/backups?${query.toString()}`);
    setItems(res.items);
    setTotal(res.total);
  }

  useEffect(() => {
    api<{ items: Project[] }>("/api/v1/projects")
      .then((res) => setProjects(res.items))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load projects"));
  }, []);

  useEffect(() => {
    setUploadProject(projectId);
    load().catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load backups"));
  }, [q, projectId]);

  async function upload(e: React.FormEvent) {
    e.preventDefault();
    if (!file || !uploadProject) return;
    setUploading(true);
    setError(null);
    try {
      const body = new FormData();
      body.set("file", file);
      body.set("name", file.name);
      body.set("kind", kind);
      body.set("source", "panel");
      await api(`/api/v1/projects/${uploadProject}/backups`, { method: "POST", body });
      setFile(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader title="Backups" />
      {error && <p className="text-sm text-error-500">{error}</p>}

      <form onSubmit={upload} className="admin-card">
        <div className="admin-card-body grid gap-3 md:grid-cols-4">
          <select className={fieldClass} value={uploadProject} onChange={(e) => setUploadProject(e.target.value)} required>
            <option value="">Project</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>{project.name}</option>
            ))}
          </select>
          <select className={fieldClass} value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="archive">Archive</option>
            <option value="database">Database</option>
            <option value="files">Files</option>
            <option value="other">Other</option>
          </select>
          <input className={fieldClass} type="file" onChange={(e) => setFile(e.target.files?.[0] ?? null)} required />
          <button className={primaryButtonClass} disabled={uploading} type="submit">{uploading ? "Uploading…" : "Upload"}</button>
        </div>
      </form>

      <div className="flex flex-col gap-3 sm:flex-row">
        <input
          className={fieldClass}
          placeholder="Search name, source, checksum"
          defaultValue={q}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              const value = (e.target as HTMLInputElement).value;
              const next = new URLSearchParams(params);
              if (value) next.set("q", value);
              else next.delete("q");
              setParams(next);
            }
          }}
        />
        <select
          className={fieldClass}
          value={projectId}
          onChange={(e) => {
            const next = new URLSearchParams(params);
            if (e.target.value) next.set("project_id", e.target.value);
            else next.delete("project_id");
            setParams(next);
          }}
        >
          <option value="">All projects</option>
          {projects.map((project) => (
            <option key={project.id} value={project.id}>{project.name}</option>
          ))}
        </select>
      </div>

      {!items ? (
        <AdminLoading message="Loading backups…" />
      ) : items.length === 0 ? (
        <div className="admin-card">
          <AdminEmptyState icon={Archive} title="No backups match" description="Upload a file here, or push one with the project API key." />
        </div>
      ) : (
        <div className="admin-card overflow-x-auto">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead className="text-xs tracking-wide text-gray-500 uppercase">
              <tr>
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Project</th>
                <th className="px-4 py-3 font-medium">Kind</th>
                <th className="px-4 py-3 font-medium">Size</th>
                <th className="px-4 py-3 font-medium">When</th>
                <th className="px-4 py-3 font-medium" />
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-100 dark:divide-gray-800">
              {items.map((backup) => (
                <tr key={backup.id}>
                  <td className="px-4 py-3">
                    <p className="font-medium text-gray-900 dark:text-white">{backup.name}</p>
                    <p className="text-xs text-gray-500">{backup.checksum_sha256.slice(0, 12)}…</p>
                  </td>
                  <td className="px-4 py-3">{backup.project_name}</td>
                  <td className="px-4 py-3">{backup.kind}</td>
                  <td className="px-4 py-3">{formatBytes(backup.size_bytes)}</td>
                  <td className="px-4 py-3">{formatWhen(backup.created_at)}</td>
                  <td className="px-4 py-3 text-right">
                    <button type="button" className={quietButtonClass} onClick={() => void downloadBackup(backup.id, backup.filename)}>
                      Download
                    </button>
                    <button
                      type="button"
                      className={dangerButtonClass}
                      onClick={() => {
                        if (!confirm(`Delete ${backup.name}?`)) return;
                        void api(`/api/v1/backups/${backup.id}`, { method: "DELETE" }).then(() => load());
                      }}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="px-4 py-3 text-xs text-gray-500">{total} total</p>
        </div>
      )}
    </div>
  );
}

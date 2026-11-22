import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";
import { AdminEmptyState } from "@/components/admin/ui/admin-empty-state";
import { AdminLoading } from "@/components/admin/ui/admin-loading";
import { AdminPagination } from "@/components/admin/ui/admin-pagination";
import { dangerButtonClass, fieldClass, labelClass, primaryButtonClass, quietButtonClass } from "@/lib/fields";
import { api, downloadBackup, formatBytes, formatWhen, type Backup, type Project } from "@/lib/harbor-api";
import { ADMIN_PAGE_SIZE } from "@/lib/page-slice";
import { Archive, Download, FileUp, Search, Trash2, Upload } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";

const pageSize = ADMIN_PAGE_SIZE;

function copyLabel(backup: Backup) {
  if (!backup.copies?.length) return "This machine";
  const extra = backup.copies.map((copy) => {
    if (copy.status === "synced") return copy.name;
    if (copy.status === "skipped") return `${copy.name} (free limit)`;
    return `${copy.name} (failed)`;
  });
  return `This machine + ${extra.join(", ")}`;
}

export default function BackupsPage() {
  const [params, setParams] = useSearchParams();
  const q = params.get("q") ?? "";
  const projectId = params.get("project_id") ?? "";
  const page = Math.max(1, Number.parseInt(params.get("page") || "1", 10) || 1);
  const [draftQuery, setDraftQuery] = useState(q);
  const [seenQuery, setSeenQuery] = useState(q);
  if (q !== seenQuery) {
    setSeenQuery(q);
    setDraftQuery(q);
  }
  const [projects, setProjects] = useState<Project[]>([]);
  const [items, setItems] = useState<Backup[] | null>(null);
  const [total, setTotal] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [uploadProject, setUploadProject] = useState(projectId);
  const [seenProject, setSeenProject] = useState(projectId);
  if (projectId !== seenProject) {
    setSeenProject(projectId);
    setUploadProject(projectId);
  }
  const [file, setFile] = useState<File | null>(null);
  const [fileKey, setFileKey] = useState(0);
  const [kind, setKind] = useState("archive");
  const [uploading, setUploading] = useState(false);
  const requestId = useRef(0);

  async function load(nextQ = q, nextProject = projectId, nextPage = page) {
    const query = new URLSearchParams();
    if (nextQ) query.set("q", nextQ);
    if (nextProject) query.set("project_id", nextProject);
    query.set("limit", String(pageSize));
    query.set("offset", String((nextPage - 1) * pageSize));
    const id = ++requestId.current;
    const res = await api<{ items: Backup[]; total: number }>(`/api/v1/backups?${query.toString()}`);
    if (id !== requestId.current) return;
    setItems(res.items);
    setTotal(res.total);
  }

  useEffect(() => {
    api<{ items: Project[] }>("/api/v1/projects")
      .then((res) => setProjects(res.items))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load projects"));
  }, []);

  useEffect(() => {
    const query = new URLSearchParams();
    if (q) query.set("q", q);
    if (projectId) query.set("project_id", projectId);
    query.set("limit", String(pageSize));
    query.set("offset", String((page - 1) * pageSize));
    const id = ++requestId.current;
    api<{ items: Backup[]; total: number }>(`/api/v1/backups?${query.toString()}`)
      .then((res) => {
        if (id !== requestId.current) return;
        setItems(res.items);
        setTotal(res.total);
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not load backups"));
  }, [q, projectId, page]);

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
      setFileKey((value) => value + 1);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader
        title="Backups"
      />
      {error && <p className="text-sm text-error-500">{error}</p>}

      <form onSubmit={upload} className="admin-card">
        <div className="admin-card-body flex flex-col gap-3 md:flex-row md:items-end">
          <label className="block min-w-0 md:w-56">
            <span className={labelClass}>Project</span>
            <select className={fieldClass} value={uploadProject} onChange={(e) => setUploadProject(e.target.value)} required>
              <option value="">Select a project</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>{project.name}</option>
              ))}
            </select>
          </label>
          <label className="block md:w-40">
            <span className={labelClass}>Kind</span>
            <select className={fieldClass} value={kind} onChange={(e) => setKind(e.target.value)}>
              <option value="archive">Archive</option>
              <option value="database">Database</option>
              <option value="files">Files</option>
              <option value="other">Other</option>
            </select>
          </label>
          <label className="block min-w-0 flex-1">
            <span className={labelClass}>File</span>
            <span className={`${fieldClass} relative flex cursor-pointer items-center gap-2 focus-within:border-brand-400 focus-within:ring-2 focus-within:ring-brand-500/20`}>
              <FileUp className="h-4 w-4 shrink-0 text-gray-400" aria-hidden />
              <input
                key={fileKey}
                className="absolute inset-0 cursor-pointer opacity-0"
                type="file"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                required
              />
              <span className={`block truncate ${file ? "text-gray-900 dark:text-white" : "text-gray-400"}`}>
                {file ? file.name : "Choose a file"}
              </span>
            </span>
          </label>
          <button className={`${primaryButtonClass} w-full shrink-0 md:w-auto`} disabled={uploading} type="submit">
            <Upload className="h-4 w-4" aria-hidden />
            {uploading ? "Uploading…" : "Upload"}
          </button>
        </div>
      </form>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
        <label className="relative block min-w-0 flex-1">
          <Search className="pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2 text-gray-400" aria-hidden />
          <input
            className={`${fieldClass} pl-9`}
            placeholder="Search name, source, checksum"
            value={draftQuery}
            onChange={(e) => setDraftQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                const next = new URLSearchParams(params);
                const value = draftQuery.trim();
                if (value) next.set("q", value);
                else next.delete("q");
                next.delete("page");
                setParams(next);
              }
            }}
            aria-label="Search backups"
          />
        </label>
        <select
          className={`${fieldClass} sm:w-64 sm:shrink-0`}
          value={projectId}
          onChange={(e) => {
            const next = new URLSearchParams(params);
            if (e.target.value) next.set("project_id", e.target.value);
            else next.delete("project_id");
            next.delete("page");
            setParams(next);
          }}
          aria-label="Filter by project"
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
          <AdminEmptyState icon={Archive} title="No backups match" />
        </div>
      ) : (
        <div className="admin-card overflow-x-auto">
          <table className="admin-table w-full min-w-[720px] text-left text-sm">
            <thead>
              <tr>
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Project</th>
                <th className="px-4 py-3 font-medium">Kind</th>
                <th className="px-4 py-3 font-medium">Size</th>
                <th className="px-4 py-3 font-medium">When</th>
                <th className="px-4 py-3 font-medium">Where</th>
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
                  <td className="px-4 py-3">
                    <span className="inline-flex rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium capitalize text-gray-700 dark:bg-white/10 dark:text-gray-200">
                      {backup.kind}
                    </span>
                  </td>
                  <td className="px-4 py-3">{formatBytes(backup.size_bytes)}</td>
                  <td className="px-4 py-3">{formatWhen(backup.created_at)}</td>
                  <td className="px-4 py-3">
                    <p>{copyLabel(backup)}</p>
                    {backup.copies?.filter((copy) => copy.error).map((copy) => (
                      <p key={copy.destination_id} className="mt-1 text-xs text-error-500">{copy.error}</p>
                    ))}
                    {backup.copies?.some((copy) => copy.status !== "synced") && (
                      <button
                        type="button"
                        className={quietButtonClass}
                        onClick={() => {
                          setError(null);
                          void api(`/api/v1/backups/${backup.id}/replicate`, { method: "POST" })
                            .then(() => load())
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not copy backup"));
                        }}
                      >
                        Copy again
                      </button>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex justify-end gap-1">
                      <button
                        type="button"
                        className={quietButtonClass}
                        onClick={() => {
                          void downloadBackup(backup.id, backup.filename).catch((err: unknown) =>
                            setError(err instanceof Error ? err.message : "Download failed"),
                          );
                        }}
                      >
                        <Download className="h-4 w-4" aria-hidden />
                        Download
                      </button>
                      <button
                        type="button"
                        className={dangerButtonClass}
                        onClick={() => {
                          if (!confirm(`Delete ${backup.name}?`)) return;
                          setError(null);
                          void api(`/api/v1/backups/${backup.id}`, { method: "DELETE" })
                            .then(() => load())
                            .catch((err: unknown) => setError(err instanceof Error ? err.message : "Could not delete backup"));
                        }}
                      >
                        <Trash2 className="h-4 w-4" aria-hidden />
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <AdminPagination
            page={page}
            pageCount={Math.max(1, Math.ceil(total / pageSize))}
            total={total}
            pageSize={pageSize}
            onPageChange={(nextPage) => {
              const next = new URLSearchParams(params);
              if (nextPage <= 1) next.delete("page");
              else next.set("page", String(nextPage));
              setParams(next);
            }}
          />
        </div>
      )}
    </div>
  );
}

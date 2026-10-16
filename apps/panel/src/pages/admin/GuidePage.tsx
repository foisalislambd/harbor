import { AdminPageHeader } from "@/components/admin/layout/admin-page-header";

const curl = `curl -X POST http://127.0.0.1:8787/api/v1/push \\
  -H "Authorization: Bearer hb_your_project_key" \\
  -F "file=@backup.tar.gz" \\
  -F "name=nightly" \\
  -F "kind=archive" \\
  -F "source=my-app"`;

const cli = `harbor push -file backup.tar.gz -token hb_your_project_key -name nightly -kind archive -source my-app`;

export default function GuidePage() {
  return (
    <div className="admin-content space-y-6">
      <AdminPageHeader title="API guide" />
      <div className="admin-card">
        <div className="admin-card-body space-y-4 text-sm leading-relaxed text-gray-600 dark:text-gray-300">
          <p>
            Create a project, then create an API key. Any application that can send an HTTP request can store a backup here. The key only sees that one project.
          </p>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Push a file</h2>
          <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{curl}</pre>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Harbor CLI</h2>
          <pre className="overflow-x-auto rounded-xl bg-gray-950 p-4 text-xs text-gray-100">{cli}</pre>
          <h2 className="text-base font-semibold text-gray-900 dark:text-white">Other calls with the same key</h2>
          <ul className="list-disc space-y-1 pl-5">
            <li>GET /api/v1/backups</li>
            <li>GET /api/v1/backups/:id</li>
            <li>GET /api/v1/backups/:id/download</li>
            <li>DELETE /api/v1/backups/:id</li>
          </ul>
          <p>Kinds are archive, database, files, or other. Identical file contents are stored once and can be referenced by more than one snapshot.</p>
        </div>
      </div>
    </div>
  );
}

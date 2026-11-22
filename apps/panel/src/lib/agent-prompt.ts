// Text a person can paste into any coding agent so it adds Harbor backups
// to the application open in that chat. The panel fills origin from the
// address in the browser, or from HARBOR_PUBLIC_URL. The key is never included.
export function agentPrompt(origin: string, projectName?: string) {
  const base = origin.replace(/\/$/, "");
  const project = projectName?.trim() || "the Harbor project whose API key I paste";
  return `Add Harbor backups to the application I have open in this chat. Do not only explain the API. Change that application so it can build a backup and send it to this Harbor.

Harbor project: ${project}
Base URL: ${base}
Project API key: if I have not pasted a key that starts with hb_, ask me for it before any call except health. Send it as Authorization: Bearer <the key>.
Never repeat the key, never write it into a repository, and never send it to a host other than the base URL. Read it from an environment variable named HARBOR_TOKEN.

The key belongs to one project. It cannot create projects, keys, users, destinations, or settings. A new key can read, write, and delete backups. If a call returns 403, tell me which scope is missing and stop.

What to add in the application:
- Look at this codebase and decide what should be backed up: a database dump, uploaded files, or an archive of the data directory. Skip .env files, secrets, and the Harbor API key unless I name that file.
- Add a command the project already knows how to run (an npm script, a Make target, or a small script beside the app). The command builds that backup file, then POSTs it to the push URL below.
- source should be this application's name. kind is archive, database, files, or other.
- Document the command in this application's README: HARBOR_TOKEN, the base URL, and how to run a backup. Do not write the key itself.
- Run one backup only after I have given you the key. Then tell me the backup id, name, size_bytes, and checksum_sha256.

Use only the HTTP API below. Do not invent paths, and do not use the panel session cookie.

1. Confirm the server. No key is required.
GET ${base}/api/v1/health
200 JSON has ok, version, needs_setup, and public_url.
If needs_setup is true, stop. An owner still has to be created in the panel at ${base}/.

2. Store a file.
POST ${base}/api/v1/push
Content-Type: multipart/form-data
  file    required. The file bytes. An empty file is rejected.
  name    optional, up to 200 characters. Defaults to the filename.
  kind    optional: archive, database, files, or other. Default archive.
  source  optional, up to 200 characters. The app that made the file.
  note    optional, up to 2000 characters.
  tags    optional, up to 500 characters.
201 JSON is the backup: id, project_id, project_name, name, filename, kind, source, note, tags, size_bytes, checksum_sha256, content_type, created_at.
409 means the project is paused. 413 means the file is over the upload limit or the project quota.

Example:
curl -X POST ${base}/api/v1/push \\
  -H "Authorization: Bearer hb_your_project_key" \\
  -F "file=@backup.tar.gz" \\
  -F "name=nightly" \\
  -F "kind=archive" \\
  -F "source=my-app"

3. List this project's backups.
GET ${base}/api/v1/backups
Optional query: q, limit (default 50, max 200), offset.
200 JSON: {"items":[...],"total":number}

4. Read one backup.
GET ${base}/api/v1/backups/{id}

5. Download the file.
GET ${base}/api/v1/backups/{id}/download
The body is the file. Content-Disposition carries the filename. The X-Checksum-Sha256 header is the sha256 hex. Compare it with checksum_sha256 before trusting the file.

6. Delete a backup only when I ask.
DELETE ${base}/api/v1/backups/{id}
204 with an empty body. The key needs backup:delete.

Errors are JSON {"error":"message"}.
401 means the key is missing or wrong.
403 means the key is missing backup:read, backup:write, or backup:delete.
404 means the backup is gone, or it belongs to another project.`;
}

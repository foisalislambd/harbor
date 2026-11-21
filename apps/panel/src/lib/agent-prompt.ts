// Text a person can paste into any coding agent so it talks to this Harbor.
// The panel fills origin from the address in the browser, or from HARBOR_PUBLIC_URL.
export function agentPrompt(origin: string) {
  const base = origin.replace(/\/$/, "");
  return `Connect to this Harbor backup vault. Use only the HTTP API below. Do not invent paths, and do not use the panel session cookie.

Base URL: ${base}
Project API key: if I have not pasted a key that starts with hb_, ask me for it before any call except health. Send it as Authorization: Bearer <the key>.
Never repeat the key, never write it into a repository, and never send it to a host other than the base URL.

The key belongs to one project. It cannot create projects, keys, users, destinations, or settings. A new key can read, write, and delete backups. If a call returns 403, tell me which scope is missing and stop.

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
404 means the backup is gone, or it belongs to another project.

After a push, tell me the id, name, size_bytes, and checksum_sha256. Do not upload .env files, keys, or passwords unless I name that file.`;
}

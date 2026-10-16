# Harbor

Open-source, self-hosted backup vault. One Go process and a SQLite database store archives, database dumps, and files from as many applications as you want. It runs on a PC, a VPS, or in Docker.

The panel lives at `/`. The API lives at `/api/v1`.

## Install

[VERSION.md](VERSION.md) is the release version. A push to `main` publishes that version when it is higher than every existing GitHub release. The same number, or a lower one, does nothing.

Replace `OWNER` with the GitHub user or organization, and `REPO` with the repository name. The container is `ghcr.io/OWNER/harbor`.

### Docker

```bash
docker pull ghcr.io/OWNER/harbor:0.1.0
docker run -d --name harbor -p 8787:8787 -v harbor-data:/data ghcr.io/OWNER/harbor:0.1.0
```

Open http://127.0.0.1:8787. The first visit creates the owner account. Data stays in the `harbor-data` volume.

### Compose from the published image

```yaml
services:
  harbor:
    image: ghcr.io/OWNER/harbor:0.1.0
    ports:
      - "8787:8787"
    volumes:
      - harbor-data:/data
volumes:
  harbor-data:
```

```bash
docker compose up -d
```

### Release binary

Linux (amd64):

```bash
curl -fsSL -o harbor "https://github.com/OWNER/REPO/releases/download/v0.1.0/harbor-linux-amd64"
curl -fsSL -o SHA256SUMS "https://github.com/OWNER/REPO/releases/download/v0.1.0/SHA256SUMS"
grep harbor-linux-amd64 SHA256SUMS | sha256sum -c -
chmod +x harbor
./harbor serve -addr :8787 -data data
```

Windows (PowerShell):

```powershell
Invoke-WebRequest "https://github.com/OWNER/REPO/releases/download/v0.1.0/harbor-windows-amd64.exe" -OutFile harbor.exe
.\harbor.exe serve -addr :8787 -data data
```

The Docker image serves the panel and the API on port 8787. The release binary serves the API and the `harbor push` command. Other archives on the release: `harbor-linux-arm64`, `harbor-darwin-amd64`, `harbor-darwin-arm64`.

## Layout

```
apps/server   Go API, blob store, and `harbor` CLI
apps/panel    React panel (ZenPanel shell, routes at /)
```

SQLite metadata and content-addressed files live in the data directory (`data/` by default).

## Run it locally

Terminal 1:

```bash
cd apps/server
go run ./cmd/harbor serve -addr :8787 -data data
```

Terminal 2:

```bash
cd apps/panel
npm install
npm run dev
```

Open http://127.0.0.1:5173. The dev server proxies `/api` to port 8787. The first visit creates the owner account.

## Docker

```bash
docker compose up --build
```

The panel and API are both served on http://127.0.0.1:8787.

## Send a backup from any app

Create a project in the panel and copy its API key. That key can only see that project.

```bash
curl -X POST http://127.0.0.1:8787/api/v1/push \
  -H "Authorization: Bearer hb_your_project_key" \
  -F "file=@backup.tar.gz" \
  -F "name=nightly" \
  -F "kind=archive" \
  -F "source=my-app"
```

Or use the same binary:

```bash
harbor push -file backup.tar.gz -token hb_your_project_key -name nightly -kind archive -source my-app
```

`kind` is `archive`, `database`, `files`, or `other`.

With the same key:

- `GET /api/v1/backups`
- `GET /api/v1/backups/{id}`
- `GET /api/v1/backups/{id}/download`
- `DELETE /api/v1/backups/{id}`

Panel users sign in with a session cookie and manage every project from `/`.

## What it keeps track of

- Projects, with pause, retention (last N and/or N days), and a byte quota. `0` means unlimited. If a create request omits retention, Harbor keeps the last 14 snapshots for 30 days.
- API keys with `backup:read`, `backup:write`, and `backup:delete`. The secret is shown once.
- Content-addressed blobs, so identical uploads share one file.
- Schedules that turn overdue when a backup does not arrive in time.
- Webhooks for `backup.created` and `backup.deleted`, signed with `X-Harbor-Signature`.
- An audit log, extra panel users, and a background retention pass every 15 minutes.

## Environment

| Variable | Default | Purpose |
| --- | --- | --- |
| `HARBOR_ADDR` | `:8787` | Listen address |
| `HARBOR_DATA_DIR` | `data` | SQLite file and blobs |
| `HARBOR_PANEL_DIR` | empty | Built panel to serve at `/` |
| `HARBOR_MAX_UPLOAD_BYTES` | 8 GiB | Upload limit |
| `HARBOR_SESSION_HOURS` | 336 | Panel session length |
| `HARBOR_COOKIE_SECURE` | empty | Set to `1` behind HTTPS |
| `HARBOR_CORS_ORIGIN` | empty | Exact origin if the panel is on another host |
| `HARBOR_TOKEN` | empty | Default token for `harbor push` |

## Project files

- [LICENSE](LICENSE) — MIT
- [CONTRIBUTING.md](CONTRIBUTING.md) — build, test, and pull requests
- [SECURITY.md](SECURITY.md) — how to report a vulnerability
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- [CHANGELOG.md](CHANGELOG.md)
- [VERSION.md](VERSION.md) — number published to GitHub releases and GHCR

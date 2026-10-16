# Harbor

Open-source, self-hosted backup vault. One Go process and a SQLite database store archives, database dumps, and files from as many applications as you want. It runs on a PC, a VPS, or in Docker.

The panel lives at `/`. The API lives at `/api/v1`.

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

- Projects, with pause, retention (last N and/or N days), and a byte quota. `0` means unlimited.
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

# Contributing to Harbor

Harbor is a self-hosted backup vault. Changes should keep one Go process, one SQLite database, and a panel at `/`.

## Setup

You need Go 1.26 or newer and Node.js 20 or newer.

```bash
cd apps/server
go test ./...

cd ../panel
npm ci
npx tsc -b --pretty false
```

Run the API and the panel in two terminals:

```bash
cd apps/server
go run ./cmd/harbor serve -addr 127.0.0.1:8787 -data data

cd apps/panel
npm run dev
```

The panel is at http://127.0.0.1:5173 and proxies `/api` to port 8787. The first account created in the panel is the owner.

## What to change

- Server code lives in `apps/server`. Format it with `gofmt`.
- Panel code lives in `apps/panel/src`. Routes stay at `/`, not `/admin`.
- Do not commit `data/`, `*.db`, `node_modules`, `dist`, or built binaries.
- A new API behavior needs a test in `apps/server`. A panel type change must pass `tsc`.

## Releases

The version line in `VERSION.md` is `major.minor.patch`. Set the same value on `Version` in `apps/server/internal/config/config.go`, and add a matching section to `CHANGELOG.md`. Pushing that higher number to `main` builds binaries, pushes `ghcr.io/foisalislambd/harbor:<version>` and `foisalislambd/harbor:<version>` (and `:latest` on both), and opens a GitHub release. An equal or lower number is skipped. Docker Hub login uses the `DOCKERHUB_TOKEN` repository secret.

## Pull requests

Describe the behavior you changed and how you checked it. Keep the change focused on one thing. The CI workflow runs `go test ./...` and the panel typecheck.

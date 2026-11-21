# Changelog

All notable changes to Harbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- The API guide uses the address the panel is open on. Set `HARBOR_PUBLIC_URL` when that address should be a fixed origin instead. Health reports it as `public_url`.

- Panel pages use the full width beside the sidebar, with the same padding on narrow and wide screens.

## [0.2.0] - 2026-11-20

### Security

- Destination secrets and webhook signing secrets are encrypted at rest with AES-256-GCM. The key is `secret.key` in the data directory, or `HARBOR_SECRET_KEY` (64 hex characters or base64 of 32 bytes) when that file does not exist yet. An existing key file is kept even if the environment variable is also set. Plaintext rows from an older install are sealed the next time Harbor opens the database.
- Webhook signing secrets are returned once, when the webhook is created. Listing webhooks no longer includes them.
- Cookie-authenticated changes, including the first setup and sign-in, require the header `X-Harbor-Request: 1`. Requests that send only a Bearer API key do not.
- Only the owner can add, test, or remove destinations, or rename the instance. Operators can still manage projects and backups.
- Webhook URLs and custom storage endpoints cannot point at a link-local or cloud metadata address. Ordinary private addresses, including MinIO on the same machine or LAN, still work. Webhook requests do not use an HTTP proxy.
- Backup downloads are sent as `application/octet-stream`.
- The audit log keeps the newest 10,000 rows. Sign-in lockouts keep a bounded number of addresses. A `?` in the data directory path cannot change SQLite pragmas.
- A project quota check no longer wraps past the size of a 64-bit integer.
- CI pins GitHub Actions by commit and gives the test jobs read-only contents permission. `govulncheck` and `npm audit` run in CI. The Go job and the image build use Go 1.26.11, which includes the standard-library fixes through 1.26.6.
- A new data directory no longer signs in as `admin@harbor.local` / `admin@123456`. The first visit creates the owner. An existing install that still has that password logs a warning at startup.
- Sign-in and password changes lock for 15 minutes after 8 failed attempts from the same address or email.
- Changing a password revokes every other session.
- JSON request bodies are limited to 1 MiB. Responses send `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, and a Content-Security-Policy. API responses use `Cache-Control: no-store`.
- Webhook deliveries do not follow redirects, so a signed body is not sent to a second host.
- The data directory, SQLite file, and desktop log are owner-readable only. The container runs as the `harbor` user and has a health check.
- `X-Forwarded-For` is ignored unless `HARBOR_TRUST_PROXY=1`. Invalid numeric settings fall back to the defaults, and `HARBOR_CORS_ORIGIN` must be an origin with no path.
- The panel ships Geist and Outfit itself, so the UI does not call Google Fonts.

## [0.1.2] - 2026-10-28

### Added

- Object storage destinations. Backups stay on this machine, and a project can also keep a second copy. Cloudflare R2 and Backblaze B2 stay inside the 10 GB free allowance unless that is turned off. Other S3-compatible services can be added from the same page.

## [0.1.1] - 2026-10-16

### Added

- Desktop mode in the release binaries. `harbor` opens the same panel in a window on Windows, Linux, and macOS.
- The release image is also published to Docker Hub as `foisalislambd/harbor`.

## [0.1.0] - 2026-10-16

### Added

- One Go server with SQLite metadata and content-addressed backup files.
- Projects, API keys, retention, quotas, schedules, webhooks, and an audit log.
- A panel at `/` for the vault, plus `harbor push` and a Docker image.

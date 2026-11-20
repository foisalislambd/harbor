# Changelog

All notable changes to Harbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

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

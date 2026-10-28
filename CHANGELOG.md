# Changelog

All notable changes to Harbor are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

# Security

Harbor stores other applications' backups. Treat the data directory, API keys, and session cookies as secrets.

## Supported use

Harbor 0.1.x is the current release. Run it on a machine you control. Put it behind HTTPS before it is reachable from the public internet, and set `HARBOR_COOKIE_SECURE=1`. Set `HARBOR_TRUST_PROXY=1` only when that proxy is the only path to Harbor.

The first visit creates the owner account. Builds before this change created `admin@harbor.local` with the password `admin@123456`. If the server log warns that this password is still in use, change it in Settings before anyone else can reach the panel.

Destination credentials and webhook signing secrets are stored in the SQLite file. The data directory is mode `0700`. Treat a copy of that directory as a full backup of those secrets.

## Reporting a vulnerability

Open a private security advisory on this repository. Do not file a public issue for an unfixed vulnerability.

Include the Harbor version, how to reproduce the problem, and what an attacker would gain. Please allow time for a fix before publishing details.

## Out of scope

- A Harbor process bound to a public address without TLS.
- Webhooks that point at addresses you did not intend. Self-hosted installs may call internal URLs on purpose.
- Lost API keys. A key is shown once; revoke it in the project and create another.

# Security

Harbor stores other applications' backups. Treat the data directory, API keys, and session cookies as secrets.

## Supported use

Harbor 0.1.x is the current release. Run it on a machine you control. Put it behind HTTPS before it is reachable from the public internet, and set `HARBOR_COOKIE_SECURE=1`.

## Reporting a vulnerability

Open a private security advisory on this repository. Do not file a public issue for an unfixed vulnerability.

Include the Harbor version, how to reproduce the problem, and what an attacker would gain. Please allow time for a fix before publishing details.

## Out of scope

- A Harbor process bound to a public address without TLS.
- Webhooks that point at addresses you did not intend. Self-hosted installs may call internal URLs on purpose.
- Lost API keys. A key is shown once; revoke it in the project and create another.

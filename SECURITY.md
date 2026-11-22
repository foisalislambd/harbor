# Security

Harbor stores other applications' backups. Treat the data directory, API keys, and session cookies as secrets.

## Supported use

Harbor 0.1.x is the current release. Run it on a machine you control. Put it behind HTTPS before it is reachable from the public internet, and set `HARBOR_COOKIE_SECURE=1`. Set `HARBOR_TRUST_PROXY=1` only when that proxy is the only path to Harbor.

The first visit creates the owner account. Builds before this change created `admin@harbor.local` with the password `admin@123456`. If the server log warns that this password is still in use, change it in Settings before anyone else can reach the panel.

Destination credentials and webhook signing secrets are encrypted in SQLite. Harbor writes a 32-byte `secret.key` next to `harbor.db` the first time it starts, unless `HARBOR_SECRET_KEY` is already set. Back up `secret.key` with the database. If you set `HARBOR_SECRET_KEY`, keep that value outside the data volume and do not also leave a different `secret.key` in the volume: the file wins when both exist. Losing the key makes stored destination and webhook secrets unreadable.

The panel sends `X-Harbor-Request: 1` on API calls. A browser page on another site cannot use a Harbor session cookie for a state-changing request without that header. API keys used as `Authorization: Bearer` are unchanged.

Only the owner account can add, test, or remove storage destinations or change the instance name. An operator can still create projects, push backups, and turn replication on for a destination the owner already added.

Webhook URLs and custom storage endpoints are refused when they point at a link-local address or a well-known cloud metadata hostname. Addresses on your own network, such as `127.0.0.1` or `192.168.0.10`, remain allowed. Harbor does not follow webhook redirects and does not send webhook deliveries through `HTTP_PROXY`.

The data directory is mode `0700`. Treat a copy of that directory as a full backup of the vault, including `secret.key` when the key is stored there.

## Reporting a vulnerability

Open a private security advisory on this repository. Do not file a public issue for an unfixed vulnerability.

Include the Harbor version, how to reproduce the problem, and what an attacker would gain. Please allow time for a fix before publishing details.

## Out of scope

- A Harbor process bound to a public address without TLS.
- Webhooks that point at ordinary private addresses you did not intend. Harbor blocks link-local and cloud metadata addresses. It does not block the rest of your network, because a self-hosted bucket is a normal destination.
- Lost API keys. A key is shown once; revoke it in the project and create another.

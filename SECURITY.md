# Security notes

- Never commit `.env`, real database credentials, session secrets, or user data.
- Set `COOKIE_SECURE=true` behind HTTPS.
- Judge requires access to Docker daemon via the Docker socket. That socket grants effective host-level control to the judge service. Keep the judge private and, for production, place it on a dedicated isolated VM/host with no unrelated secrets or services.
- User code runs in disposable containers with networking disabled, CPU/memory/process limits, read-only root filesystem, reduced capabilities and a time limit. Container isolation is a defense layer, not a guarantee against every kernel/container escape.
- Add edge rate limits, audit logging, dependency scanning, backups, and incident monitoring before public launch.

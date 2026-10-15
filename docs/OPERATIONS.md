# Operations

## Production bootstrap

1. Provision Postgres **18** separately from the web service. Keep its owner connection string only in an operator environment. Require TLS for remote database access. Create a login `islands_app` with `NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS`; set a unique password through the provider's secret facility or interactive `psql \password islands_app`. Do not reuse Compose credentials. The web login must not be a member of the migration/owner role.
2. In that operator environment, run `MIGRATION_DATABASE_URL=... bin/migrate`. This runs application migrations, explicit River schema migrations, and grants. Run migrations once before deploying the web process. Never run them automatically on web startup. This starter uses forward migrations; restore a backup for destructive schema recovery.
3. Still in the operator environment, run `bin/admin -email owner@example.org -name 'First Administrator'`. It requires MIGRATION_DATABASE_URL and refuses if an enabled administrator already exists. It grants only access administration. After sign-in, use People to admit workflow users and explicitly combine roles when needed.
4. Create a Google OAuth web client. Set the authorized redirect URI to `https://YOUR_APP/auth/callback` exactly. Restrict Workspace domains through GOOGLE_WORKSPACE_DOMAINS. Configure the consent screen and testing/production access in your Google project. An email suffix is not the authorization rule; the verified `hd` claim and local admission are both required.
5. Configure APP_ORIGIN, restricted DATABASE_URL, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, GOOGLE_WORKSPACE_DOMAINS, PORT, and ENABLE_DESTRUCTIVE_DEMO=false in the web service. Do not set MIGRATION_DATABASE_URL there. The Render blueprint intentionally does not create or automatically attach an owner URL.
6. Deploy the reviewed image behind HTTPS. Render terminates HTTPS before forwarding to the Go listener. Secure cookies still travel to the browser over HTTPS. Verify login/callback, logout, access revocation, cross-origin mutation rejection, RLS, health, and worker behavior on the actual host before release. No real Workspace or Render account was used during local development.

The image contains `/server`, `/migrate`, and `/admin`. It contains no preview executable, Node runtime, shell, or production secrets. Override the Docker entrypoint explicitly for operator commands, and keep their credentials out of the web service environment.

## Local secrets

The `.env.schema` has types and non-sensitive defaults; `.env` is ignored. From `web/`, run `npm exec -- varlock run --path .. -- ../bin/server` after building. Varlock is a local helper, not a production dependency. Never copy backend variables into Vite's environment. For quick development without real Google credentials, use `make preview` instead.

## Runtime grants and logs

- `app.users`: read access for trusted identity/role resolution; only provider subject may be updated directly. The narrow security-definer function handles access changes.
- `app.sessions` and `app.login_attempts`: token/state digest lookup and lifecycle CRUD. Domain RLS must not block sign-in.
- `app.requests`, `app.exports`, `app.audit_events`: forced RLS plus column grants. No runtime DELETE on domain records, no audit UPDATE/DELETE, no ownership changes.
- `river.*`: library-owned queue maintenance grants. Job arguments contain identifiers only. Raw internal River logs are suppressed because errors can contain query arguments. HTTP logs contain route pattern, status, duration, and opaque request ID only.

The health endpoint pings Postgres with a one-second deadline. It does not prove Google sign-in or every worker's health. Inspect failed jobs separately with the operator query below. The UI maps discarded/cancelled River jobs to Failed for an authorized export owner.

```sql
SELECT id, state, attempt, max_attempts, kind, created_at, finalized_at
FROM river.river_job WHERE state IN ('discarded','cancelled')
ORDER BY id DESC LIMIT 50;
```

Do not export raw job errors or auth logs into public issues. Jobs retry up to three times, with two workers and a 30-second timeout. Each export is an idempotent, row-locked transaction. SIGTERM stops HTTP intake and waits up to 20 seconds for shutdown, then bounds cancellation to five more seconds. The platform termination grace must exceed that interval.

## Retention and recovery

The following is an operator procedure using separately held migration credentials. Schedule it daily in a trusted operator environment; the runtime role intentionally cannot delete audit records. Adjust the **365-day audit retention** for each deployment's requirements before introducing real data.

```sql
BEGIN;
DELETE FROM app.login_attempts WHERE expires_at < now();
DELETE FROM app.sessions WHERE expires_at < now() OR touched_at < now() - interval '30 minutes';
UPDATE app.exports SET content = NULL WHERE expires_at < now() AND content IS NOT NULL;
DELETE FROM app.audit_events WHERE created_at < now() - interval '365 days';
DELETE FROM app.exports WHERE expires_at < now() - interval '365 days';
COMMIT;
```

River cleans finalized job history according to its configured retention (24 hours for completed/cancelled and seven days for discarded jobs). Monitor failures before that cleanup. Pending exports older than their expiry are displayed as Expired. Requests are retained until a downstream app defines a deletion policy.

Take provider backups and periodic `pg_dump -Fc` snapshots using operator credentials. Store them privately with access and expiry policies. Restore into a **new, empty database**, provision the runtime role, run `pg_restore`, verify migration versions and row counts, and verify runtime-role isolation before switching the web URL. Never test a restore over the active production database. The acceptance test exercises a separate synthetic restore and restart; it is not evidence about a provider's backup service.

## Proxy and capacity

The listener trusts neither X-Forwarded-For nor X-Real-IP. Login limits are 10/minute and export creation 5/minute per observed peer, per process, with bounded storage. A shared proxy may make these limits shared across users. Verify your host's proxy topology before replacing this with a trusted proxy-aware identity. An edge rate limiter can provide additional protection without changing application actor checks.

The default pool is eight connections. A web instance and its two River workers share it. River also opens one dedicated LISTEN connection, so budget nine total runtime connections per instance. Postgres runs separately from the targeted 512 MiB application container. Run the documented capacity test again after changing query patterns, row limits, worker concurrency, hardware architecture, or adding integrations; local measurements do not establish hosted performance.

## Upgrades and known build messages

`make generate` is offline once Go tool dependencies are cached; copied components do not fetch a live registry. Commit generated Go/TypeScript/manifest output. CI checks tracked and untracked drift. Rebuild web/dist and the binary after any UI change.

Vite reports htmx's bundled eval implementation even though allowEval is disabled and CSP rejects eval. Browser acceptance checks for actual CSP errors. The dashboard chunk is approximately 586 kB minified (175 kB gzip) including React, TanStack Table, and Recharts; it loads only on Overview. Splitting it further would not reduce that page's total download.

Socket Firewall checks the clean npm CI install. It is an external CI tool, pinned separately and not shipped with the app. The seven-day npm minimum age and ignore-scripts policy remain enabled. Tailwind CLI is pinned to 4.3.0 with watcher 2.6.0 to avoid the upstream watcher/braces advisory; npm audit must remain clear. The standalone gosec v2.29.0 importer cannot read Go 1.27 export data; CI runs the same gosec analyzer through pinned golangci-lint v2.14.0, whose tools importer supports Go 1.27.

## Credential rotation

Rotate the restricted database login through the provider, update DATABASE_URL in the web service, and restart the process to replace pooled connections. Verify the new login still has only the documented grants. Rotate the OAuth client secret in Google, update GOOGLE_CLIENT_SECRET, restart, and exercise login again. Rotate operator credentials separately; they never belong in the web environment. Existing app sessions do not depend on the OAuth client secret. To end all sessions after an incident, delete app.sessions through the operator connection, then investigate and rotate the affected credentials. Test rotation in a synthetic staging environment before changing a live deployment.

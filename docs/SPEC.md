# go-islands-starter — Spec

Reusable, generic Go + React-islands starter for small internal applications. Each deployed app serves **one organization**. Organization-specific integrations and business rules belong in downstream applications.

Optimized for AI-assisted development, sensitive data, low operating cost, and a small, reviewable dependency surface. This document specifies intended behavior and acceptance requirements; it does not claim that they are already implemented or verified.

**Repository:** `robertguss/go-islands-starter` — intended for public OSS use.

**Updated:** 2026-10-15.

## Goals

- One Go application binary serves server-rendered pages, embedded assets, and rich React islands; a bounded River worker runs in the same process by default.
- Use Go + Postgres for ordinary application behavior. Use React where it improves charts, dashboards, and complex tables.
- Give agents a predictable layout, typed queries, explicit security boundaries, and executable acceptance checks.
- Provide an attractive, accessible shared layout with consistent light/dark themes.
- Target Render's 0.5 CPU / 512 MB application plan, with managed Postgres budgeted separately. Verify capacity through measurement.
- Prove the patterns with one complete generic workflow that a downstream app can replace.

## Non-goals and deferred scope

- Multi-tenancy, organization switching, and tenant administration.
- Institution-specific integrations, data, branding, and migration tooling.
- Full SPA / Next.js, client-side application routing, or a plugin framework.
- SAML, additional identity providers, generic email/upload/object-storage frameworks, and SSE/WebSockets in v1.
- A separate custom outbox, configurable permissions engine, or feature-flag service.
- Rust, SQLite support, OpenTelemetry, and Viper/Koanf in v1.
- Alpine by default; introduce another browser runtime only for a demonstrated need.

## Stack

### Server

| Piece | Choice |
|--------|--------|
| HTTP | Go stdlib `net/http` |
| DB / queries | Postgres, pgx, sqlc |
| Migrations | goose for application schema; explicit River schema migration step |
| Jobs | River, with transactional enqueueing and a retry-safe example |
| Identity | Google Workspace OIDC, with configurable allowed Workspace domain(s) for one organization |
| Sessions | Postgres-backed opaque sessions with expiration and revocation |
| Authorization | Application action permissions plus RLS on protected domain records; both deny by default |
| Config | Plain environment variables + typed Go `Config`, validated at startup |
| Local secrets | Varlock; commit `.env.schema` with non-sensitive defaults only; production uses Render environment variables |
| Logging | `slog`, JSON in production, with a restricted set of safe fields |
| Deploy | Non-root distroless container on Render, described by `render.yaml` |

### Frontend

| Piece | Choice |
|--------|--------|
| Default pages | templ + **shadcn-templ 2.0 beta** + Tailwind + htmx |
| Component workflow | CLI-copied, committed component sources; only components used by the example |
| Rich UI | React islands built with Vite and TypeScript under `web/` |
| Island UI | shadcn-style React components, TanStack Table, Recharts; install only the required components |
| Assets | Build-time shadcn-templ component bundle, Tailwind CSS, and Vite bundles; embedded with `go:embed` |
| Mount API | Small allowlisted Go registry + templ helper `Island(name, props)` |
| Shared types | tygo over deliberate frontend-facing Go structs |
| Design | Shared Tailwind tokens, typography, and light/dark behavior across templ and React |

### shadcn-templ beta adoption

The choice of **shadcn-templ 2.0 beta**, replacing templUI v1, is intentional. Use its supported CLI workflow; the upstream direct-import workflow is experimental and may be removed. See the [installation guide](https://shadcn-templ.com/docs/installation) and [import-workflow status](https://shadcn-templ.com/docs/import-workflow), checked 2026-10-15.

- Select and record an exact beta release or commit at implementation. Pin the CLI and the component source revision; do not use floating `latest`, beta channels, or live registries during routine builds.
- Commit the copied templates, their component scripts, styles, required helpers, and upstream license notices. Record their provenance so updates can be reviewed.
- Build the component JavaScript with the pinned `shadcn-templ bundle` tool, using the configured asset destination under `web/dist/`. Render the generated script helper once in the shared layout. The [upstream build](https://shadcn-templ.com/docs/changelog) uses a Go-based bundler.
- Keep npm manifests and installs only in `web/`. Copied component JavaScript alongside Go templates in `components/` is the explicit exception to the browser-source directory rule; it receives the same review as code in `web/`.
- Beta updates require a reviewed diff and the browser acceptance checks below. Do not regenerate over local customizations without reviewing the changes.

## Project layout

```text
cmd/server/           # application entry point
internal/<feature>/   # handlers + domain behavior per feature
internal/db/          # sqlc generated code (do not hand-edit)
internal/server/      # router, middleware, island registry
db/migrations/        # goose application migrations
db/queries/           # application SQL sources for sqlc
templates/            # shared layouts and page templates
components/           # selected shadcn-templ sources, helpers, script manifest
components.json       # pinned component workflow and asset path configuration
web/                  # Vite React islands, shared styles, npm manifests
web/dist/             # built CSS, component JS, and island assets
web/assets.go         # embeds dist/ for the Go server
AGENTS.md
render.yaml
Dockerfile
docker-compose.yml    # local and integration-test Postgres
.env.schema           # Varlock schema; no secret values
```

Avoid a `pkg/` dumping ground and empty `handlers/`, `services/`, or `repos/` layers. Keep ordinary feature logic independent of HTTP where that makes policy tests straightforward. Add abstractions only when a real caller needs them.

Use Docker Compose Postgres locally and for integration tests, with the same supported major version and role/grant setup as production. Document simple commands for setup, development, generation, tests, and a production build. Local examples and automated tests use synthetic data and require no production credentials.

## Authentication and authorization

- Validate the OIDC signature, issuer, audience, expiry, state, and nonce. Use a standard OIDC implementation; do not implement token verification manually.
- Link a local user ID to the verified provider issuer/subject. Check the verified Google `hd` claim against configured domains; an email suffix or login hint is insufficient. Domain membership authenticates organizational identity, not application permission. See [Google's OIDC guidance](https://developers.google.com/identity/openid-connect/openid-connect).
- Require explicit application admission and server-managed role assignments. Bootstrap the first access administrator through a documented operator procedure; never grant administrator rights to the first login.
- Use random opaque session tokens, store their digests, and set production cookies to `Secure`, `HttpOnly`, and an explicit SameSite policy compatible with OIDC. Rotate sessions at login, enforce idle/absolute expiry, and revoke on logout. Disabled users and removed permissions must take effect on the next protected operation.
- Validate return URLs. Keep any synthetic authentication fixture confined to tests or an isolated local setup and unavailable in production.
- Authorize actions and state transitions in the application, including export creation/download, review decisions, role changes, and external effects. Apply the same checks to HTML handlers, htmx fragments, and JSON endpoints.

The example uses fixed roles rather than a configurable policy engine:

| Role | Example permissions |
|------|---------------------|
| Requester | Create a request, view own requests, export permitted records |
| Reviewer | View submitted requests and approve/reject another user's pending request |
| Access administrator | Admit/disable users and assign roles; no implicit access to workflow data |

Roles may be combined explicitly. Reject self-approval and repeated or conflicting decisions; concurrent decisions must result in one valid transition. User-editable profile fields cannot modify roles or record ownership.

### RLS and transaction contract

RLS protects domain records independently of application action checks. It does not replace action authorization, input validation, or safe query construction.

1. Resolve the actor from a verified session or explicitly permitted job context.
2. Begin an explicit pgx transaction and establish actor context with parameterized transaction-local settings, such as `set_config(..., ..., true)`.
3. Execute protected queries only through sqlc bound to that same transaction. Do not set context on the pool or issue a protected query on a different pooled connection.
4. Commit or roll back before returning success. Always arrange rollback cleanup, including error and cancellation paths.

Keep transactions around database work, not external calls, solver execution, or streaming responses. `SET LOCAL` lasts only for the transaction; see [PostgreSQL transaction-local settings](https://www.postgresql.org/docs/current/sql-set.html).

- Runtime roles must not own protected tables, have superuser/`BYPASSRLS` privileges, alter policies, or assume migration roles. Provision migration credentials separately and exclude them from the running app.
- Enable RLS on protected domain tables and test the effective production grants. Use `USING` and `WITH CHECK` policies to cover reads, inserts, updates, deletes, and attempted ownership changes. Missing or invalid actor context must fail closed.
- Document separately the narrowly granted access needed for identity/session lookup and River's queue tables. A domain-row policy must not accidentally disable authentication or queue maintenance.
- Jobs establish their own scoped context. User-requested work rechecks current permissions; scheduled maintenance uses a bounded service identity. Neither receives blanket RLS bypass.
- Treat actor settings as assertions made by trusted application code. They cannot prevent impersonation by a fully compromised server holding its database credentials.

PostgreSQL owners and `BYPASSRLS` roles can bypass normal policies; acceptance tests must connect as runtime roles, not just the migration account. See [PostgreSQL RLS behavior](https://www.postgresql.org/docs/current/ddl-rowsecurity.html).

## Generic example and background work

Build one request-and-approval workflow with synthetic records:

1. Sign in as an admitted requester; create and list requests using templ/htmx, with search/filter, form validation, a dialog, and feedback.
2. Sign in as a reviewer; approve or reject an eligible request through an authorized state transition.
3. Record the successful state change and its audit event in the same transaction.
4. Request a bounded CSV export: commit its domain record, audit event, and River job in one transaction.
5. Show job status and an authorized download; show a React dashboard with KPI cards, a chart, and a sortable/filterable/paginated table over the same permitted data.

Use a shared sidebar and theme. Include empty, loading, validation-error, forbidden, and failed-job states. All names and data in seeds, screenshots, fixtures, and public demos are synthetic.

### Jobs, audit, and exports

- Use River transactional enqueueing as the initial delivery mechanism; do not add a second generic outbox. A rolled-back operation must leave neither its domain change nor its job. See [River transactional enqueueing](https://riverqueue.com/docs/transactional-enqueueing).
- Demonstrate retry-safe execution with stable operation IDs and database constraints. Set bounded worker concurrency, execution timeouts, and retry limits. Make exhausted retries visible to an authorized operator; shutdown must stop intake and bound in-flight work.
- Keep job arguments to identifiers and necessary non-sensitive metadata. Reauthorize user work when it runs. Document that future external effects need provider idempotency or explicit reconciliation for uncertain outcomes; job uniqueness alone is insufficient.
- Audit events contain actor/service identity, action, target ID, timestamp, and a correlation ID, with only necessary metadata. Runtime roles cannot update/delete the audit history. Audit visibility is authorized, and retention cleanup uses a separately scoped maintenance procedure. This is application-level append-only history, not a claim of tamper-proof storage.
- Keep the example export small: store its result in Postgres with explicit row/byte limits and an expiry. Recheck access to the exported records at download; deny stale permissions and expired artifacts. Downloads are private and never served as public static assets. Object storage belongs to a downstream app when size requires it.
- Use a tested CSV encoding policy for spreadsheet formula injection. Define UTC timestamps, explicit display time zones, and lossless wire formats for large IDs and monetary values when present.
- Destructive downstream integrations must be default-off in typed configuration, checked server-side, and separately authorized. The starter demonstrates the gate with a synthetic operation; it contains no real external destructive tool.

## Browser ownership and shared types

- htmx owns server-rendered regions; React exclusively owns each island subtree. Do not swap or modify inside a mounted island from htmx or component scripts.
- Mount islands on initial load and after fragment insertion; unmount before removal, cancel pending work, and prevent duplicate roots/listeners. Test repeated replacements and navigation. See [React root cleanup](https://react.dev/reference/react-dom/client/createRoot#root-unmount).
- Load island code only where needed. Keep navigation server-owned; no React router or shared global client-state framework in v1.
- Serialize props safely as JSON and expose only authorized fields. Use an allowlisted registry, feature-specific prop structs, and a documented error state for unknown islands or failed loads.
- Generate TypeScript from frontend-facing structs, not sqlc/database models. Define explicit tygo mappings for nullability, dates, identifiers, and any custom types; reject unexplained generated `any`. Validate untrusted inputs in Go. Type generation is not runtime validation.
- Use one shared Tailwind theme with source scanning for both Go templates/components and React. Build CSS, the shadcn-templ bundle/manifest, and Vite assets before embedding. A clean checkout must produce the complete production artifact without a development server or runtime Node installation.

## HTTP security and privacy defaults

- Use CSRF protection for browser mutations, including forms, htmx, and React requests. Prefer the supported, patched Go `http.CrossOriginProtection` implementation and test allowed OIDC callbacks separately. GET/HEAD must not perform business mutations.
- Set server and outbound-client timeouts, body-size limits, validation/error behavior, and targeted rate limits for login and expensive operations. Document trusted proxy handling before using forwarded addresses in security decisions.
- Ship security headers and an enforced production CSP tested with both component systems. Disable htmx eval-dependent features and response-script execution; use the shared script bundle and lifecycle hooks. Avoid blanket script `unsafe-inline`/`unsafe-eval`; document any narrowly required style or nonce policy.
- Disable htmx history snapshots on sensitive pages and use appropriate private/no-store HTTP caching. Do not persist sensitive props, tokens, or records in localStorage. See [htmx security controls](https://htmx.org/docs/#security).
- Logs contain approved fields such as route, status, duration, opaque correlation IDs, and safe error codes. Do not log request/response bodies, cookies, authorization headers, OIDC codes, secrets, or sensitive record values. Keep raw upstream errors out of user responses and logs.
- Commit no secrets or production-derived fixtures. Varlock is a local aid; Go independently validates production configuration. Frontend builds receive no backend secrets. Apply documented retention to sessions, job history, exports, and audit records.
- Document the supported data-handling boundaries. The starter does not by itself establish FERPA or other regulatory compliance for a downstream deployment.

## Quality, supply chain, and agent rules

- GitHub Actions runs formatting, `golangci-lint`, `gosec`, tests with `-race`, `govulncheck`, TypeScript checking, browser checks, and the production container build.
- Commit `go.sum` and the npm lockfile. Pin Go, Node/npm, generators, Tailwind tooling, and component tools. Pin container images by digest and Actions by full commit SHA; update through reviewed changes.
- npm installs exist only under `web/`: exact direct-dependency pins, `npm ci --ignore-scripts`, an enforced minimum package age using a pinned compatible npm version, and Socket checks. Define narrow, reviewed exceptions for urgent security fixes; do not silently disable the policy.
- Require review for dependency additions, copied upstream code, migrations, role/policy changes, and CI/security-rule changes. Preserve upstream notices. Untrusted pull-request builds receive no production credentials or privileged deployment token.
- Application SQL lives in sqlc sources. Allow documented infrastructure exceptions for migrations, transaction setup, and library-owned queries. Handlers must not acquire a raw pool to bypass protected query boundaries.
- Regenerate sqlc, templ, tygo, and the component manifest in CI and fail on unexpected drift, including missing/untracked outputs. Choose and document which generated assets are committed versus built.
- Encode these boundaries and verified development commands in `AGENTS.md`. Pre-commit hooks are a convenience; required CI checks are authoritative. Tests and documentation must not imply that agent instructions alone guarantee safe behavior.

## Acceptance criteria

Use real Postgres with production-equivalent grants, deterministic synthetic fixtures, Go policy tests, and focused browser tests. Fixtures clean up after failures and keep production credentials out of test runs.

| Boundary | Required proof before the initial release |
|----------|------------------------------------------|
| Authentication | OIDC rejection cases, unadmitted users, disabled users, session expiry/revocation, and safe return URLs |
| Permissions | Direct HTML/JSON/htmx requests cannot bypass role checks; no self-escalation or self-approval; concurrent reviews produce one valid transition |
| RLS | Requester A cannot read/change requester B's protected records; verify explicit reviewer grants separately; test all write types, missing context, and ownership changes using runtime credentials |
| Pool reuse | Force connection reuse after commit, rollback, error, and cancellation; prove actor context cannot leak to another request/job |
| Jobs | Rollback leaves no job; duplicate attempts preserve one intended result; revoked access, timeout, exhausted retries, and shutdown are handled |
| Audit / export | State, audit, and enqueue are atomic; runtime cannot rewrite audit history; denied/expired downloads and CSV injection cases are covered |
| Browser | Production CSP, CSRF rejection, prop escaping, repeated island/component insertion/removal, and expired sessions work in the browser |
| Accessibility | Inspect rendered light/dark and narrow/wide layouts; verify keyboard navigation, focus return/trapping, labels, validation feedback, and chart/table alternatives |
| Generation / build | Clean checkout generates and builds reproducibly with installation scripts disabled; embedded assets work without Node or dev servers at runtime |
| Operations | Apply migrations from empty and prior schemas, stop during active work, restart without losing queued work, and restore a synthetic database backup |
| Capacity | Measure overlapping requests and jobs under the target resources; retain workload size, latency/error results, peak memory, pool usage, and queue progress |

The implementation plan must specify numeric workload and acceptance thresholds before the capacity run. The 512 MB target is not a blanket guarantee for downstream apps. Downstream migrations add their own golden/parity tests; organizational fixtures do not enter this public starter.

## Deployment and maintenance

- Default to one web process running HTTP and bounded River workers, plus managed Postgres. Document how a downstream app can separate workers or add a batch command when measurements require it; do not build a deployment matrix in v1.
- Run application and River migrations in an explicit serialized deployment step with scoped credentials. Web replicas do not automatically migrate at startup. Document compatibility across rolling deployments and recovery for failed migrations.
- Provide health/readiness endpoints, graceful SIGTERM handling, bounded pool/concurrency settings, and restart behavior. Persist nothing essential solely on the container filesystem. A distroless image must include the certificates/time-zone support needed by the app.
- Use `slog` and Render metrics initially. Document how operators find failed jobs, queue backlog, stale processing, and application failures without adopting OTel.
- Document backup/restore, secret rotation, retention/cleanup, and supported runtime/database versions. Include a release checklist, an OSS license, a private vulnerability-reporting route, and a changelog with downstream security-update instructions. License selection remains a release decision.
- Budget application compute, managed Postgres, storage, bandwidth, and optional services separately. Record dated deployment measurements and pricing when selecting a production plan; a language choice alone does not justify downsizing.

## Decisions and next steps

Decisions confirmed on 2026-10-15: generic OSS scope; one organization per app; no institution-specific integrations; retain the Go/Postgres/River/islands stack; adopt shadcn-templ 2.0 beta via committed component sources; remove Alpine from the default; defer SAML, OTel, and generic integration frameworks.

1. [ ] Write the implementation plan and acceptance fixtures; select exact tool/beta pins and define workload thresholds.
2. [ ] Implement identity, sessions, permissions, and transaction-bound RLS; verify the corresponding negative tests.
3. [ ] Implement the generic workflow, audit history, transactional jobs, and bounded export; verify retries and concurrent decisions.
4. [ ] Implement shadcn-templ pages, React islands, shared styling, and the asset build; verify CSP, lifecycle, accessibility, and generated output.
5. [ ] Verify the production container, migrations, shutdown/recovery, backup restoration, and measured resource use.
6. [ ] Complete OSS documentation and license selection, then prepare an initial release. Validate the starter in one downstream app before broadening its scope.

# Implementation and acceptance plan

The authority is SPEC.md. Work is local until runtime evidence supports a release; no production accounts or data are needed. The owner selected MIT on 2026-10-15.

1. Scaffold and pin tools. Verify a clean dependency install and generated outputs.
2. Implement Postgres roles, RLS, sessions, and OIDC. Verify denial paths using the restricted runtime account and a forced one-connection pool.
3. Implement requests, decisions, audit, and transactional River exports. Verify rollback, conflicting reviews, retry safety, revocation, and CSV handling.
4. Build the templ/shadcn-templ interface and React dashboard. Verify actual browser interactions, CSP, accessibility, and repeated island replacement.
5. Build the production container and CI; verify restart, backup/restore, and capacity. Document any checks that cannot run locally separately from passing checks.

## Test environment

Use a dedicated Compose project and synthetic Postgres database. Application tests use a non-owner role without BYPASSRLS. Setup/migrations use separate credentials. No test connects to a configured production database. Register cleanup before creating resources or waiting.

The local preview is a separate executable with explicit synthetic login routes, bound to loopback. The production executable contains no demo-login route. OIDC verification uses a local signed-token fixture in tests; actual Workspace sign-in remains an account-backed acceptance check.

## Sample limits

- Request title: 120 characters; description: 2,000 characters.
- Export: at most 200 records and 1 MiB; expires after one hour.
- Session: 30-minute idle expiry and eight-hour absolute expiry.
- Runtime pool: at most eight connections, plus one dedicated River LISTEN connection; two export workers; 30-second job timeout; three attempts.
- Production HTTP body limit: 16 KiB; login/export rate limits are per process, suitable for the default single instance.

## Capacity acceptance

Run the production image limited to 0.5 CPU / 512 MiB, with a separate Postgres container. Seed 1,000 synthetic requests. For 60 seconds run ten concurrent authenticated readers while enqueueing twenty bounded exports. Require no unexpected HTTP failures, p95 reads under one second, no OOM/restart, peak container memory below 450 MiB, no more than eight pooled connections plus one River LISTEN connection, and exports complete within two minutes. These are starter acceptance thresholds, not a guarantee for a downstream workload. Record measurements, image/tool versions, and failures.

## Evidence status

Local implementation and synthetic acceptance completed on 2026-10-15. No production accounts or records were used.

- `make build`: generated SQL/types/templates/component manifest, TypeScript checking, frontend assets, and standalone binaries passed.
- `make test`: uncached `go test -race -count=1 ./...` passed against disposable Postgres 18 databases. Includes signed OIDC rejection cases, state replay, session expiry/revocation, bootstrap restrictions, forced runtime-role RLS, single-connection reuse after commit/rollback/cancellation, competing reviews, immutable ownership/audit grants, CSRF, request limits, administration, and safe island props.
- River tests exercised the actual worker, atomic enqueue rollback, duplicate execution, revoked/expired downloads, three-attempt exhaustion with redacted errors, and interruption during blocked work followed by successful restart with one completion audit event. The HTTP test verifies discarded jobs appear as Failed.
- Three Playwright scenarios passed: requester/reviewer/export workflow; eight repeated htmx island replacements with sorting/filtering/pagination; and administration/forbidden states. Dialog Escape/focus restoration, axe scans, strict CSP console checks, light/dark themes, 390px mobile overflow, and theme-only localStorage passed. Screenshots were visually inspected; the README uses synthetic data.
- Formatting and golangci-lint passed, including the acceptance harness. The bundled gosec analyzer reports zero issues; govulncheck reports no reachable vulnerabilities; npm audit reports zero vulnerabilities.
- Socket Firewall v1.15.4, downloaded from its official release and SHA-256 verified, successfully wrapped a clean-cache `npm ci --ignore-scripts`. The seven-day age policy remained active. Varlock's schema/run command was checked with synthetic environment values.
- A second generator run produced the same source contents and file set. CI additionally checks Git tracked/untracked drift once these files are committed.
- Migrations passed from empty databases and from the earlier local schema. Database restore, production-image restart, persisted sessions, and graceful shutdown passed. Test fixtures were removed after both failed and successful runs.
- The clean Docker build generates DTOs before frontend compilation. Its three binaries were byte-identical to the tested image's binaries after verifying the build order. Runtime user is `65532:65532`; image size is 36,450,829 bytes. No Node or preview executable is included.

### Corrections found during verification

The dependency audit identified a vulnerable Tailwind watcher dependency; the compatible pinned CLI plus watcher update cleared it without disabling the package-age rule. Browser tests caught htmx's default inline indicator stylesheet and a contrast issue during dialog animation. Indicator injection is disabled, the field hint has stronger contrast, and accessibility checks wait for the opening animation to finish. The production CSP remains enforced without inline/eval scripts.

The capacity harness initially counted River's dedicated listener as part of the pool and reused a temporary host port after Docker restart. It now budgets the separately observed listener, rediscovers the port, and disables Go test caching for external-state acceptance. No application performance threshold was relaxed.

### Remaining release checks

Real Google Workspace sign-in, Render deployment/proxy behavior, provider-managed backups, GitHub-hosted Actions, and downstream-app adoption have not been exercised. The local signed-provider fixture is not a substitute for those account-backed checks. Follow RELEASE.md before publishing a release. shadcn-templ remains an intentionally pinned beta.

### Final capacity measurement

Uncached production-image acceptance, 2026-10-15, Docker Linux/arm64 on local Apple Silicon. Image: `sha256:9db6710a03e74a6a00ac8fdf49b76dff4b0feb4dd62b82f927b4ca023b9488d0`. The server binary SHA-256 is `79530aa7a36eb7a9714d7bd846818ac051099d3244409ba8e0f6e859dd182aab`.

The app was limited to **0.5 CPU / 512 MiB**, with separate Postgres, a read-only filesystem, dropped Linux capabilities, and no new privileges. The workload used 1,000 synthetic requests, ten continuous authenticated readers for 60 seconds, and twenty exports enqueued after all readers had started. Each page/export was capped at 200 records.

| Measurement | Result | Threshold |
|---|---:|---:|
| Successful reads | 41,678 | Workload completes |
| Unexpected read failures | 0 | 0 |
| Read p95 | 27.06 ms | <1,000 ms |
| Peak sampled container memory | 19.90 MiB | <450 MiB |
| Configured pool / observed listener / total connections | 8 / 1 / 9 | 8 + 1 |
| Completed exports | 20/20 within the run | 20/20 within 2 minutes |
| OOM / abnormal exit | None | None |
| Restart and authenticated session persistence | Passed | Passed |
| Restored requests / exports / audit events / sessions | 1,000 / 20 / 40 / 1 | Exact match |

These are local synthetic measurements, not a prediction of Render or downstream application performance. Memory is the maximum sampled by Docker stats, not an instrumented instantaneous high-water mark.

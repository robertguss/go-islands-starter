# Go Islands Starter

A small, single-organization starter for internal tools: Go pages by default, React where a chart or rich table helps. MIT licensed. No organization-specific integrations.

The included workflow covers requests, reviewer decisions, private CSV exports, audit history, and access administration. It uses Postgres RLS, Google Workspace OIDC, opaque database sessions, transactional River jobs, templ, **shadcn-templ 2.0.0-beta.15**, htmx, Tailwind, and a React/TanStack Table/Recharts dashboard.

![Synthetic workspace overview](docs/images/overview.png)

## Try it locally

Install Go **1.27.2**, Node **24.19.0**, npm **11.17.0**, and Docker Compose. Then:

```sh
make setup
make preview
```

Open http://127.0.0.1:8080/preview and choose a synthetic requester, reviewer, or access administrator. The preview creates its own `islands_preview` database and is bound to loopback. It has no production credentials. Restart `make preview` after changes; assets are embedded at compile time.

The production executable has no synthetic sign-in route. `cmd/preview` is excluded from the production image. The local credentials in docker-compose.yml are disposable examples, not production defaults.

## Develop and verify

```sh
make generate       # sqlc, tygo, component bundle, templ
make build          # type-check, build assets, embed into Go binaries
make test           # race tests with disposable real Postgres databases
npm exec --prefix web -- playwright install chromium
make browser        # browser workflow, keyboard, CSP and axe checks
make check          # formatting and golangci-lint
make security       # gosec, govulncheck, npm audit
make container      # production image, including migration/admin commands
```

Go tests skip database cases unless `ISLANDS_TEST_DATABASE_URL` is set. `make test` sets it to the dedicated loopback Compose database and is the required test command. Each database fixture is created and dropped automatically, including on failures.

```sh
ISLANDS_TEST_DATABASE_URL='postgres://postgres:local-migrations-only@127.0.0.1:55432/islands?sslmode=disable' \
ISLANDS_TEST_IMAGE=go-islands-starter:local \
go test -count=1 -tags=acceptance -v ./internal/acceptance
```

This additional check exercises a production image at 0.5 CPU/512 MiB, graceful shutdown, and database restore. It requires the local image and Compose Postgres. Results and remaining acceptance work live in [docs/IMPLEMENTATION.md](docs/IMPLEMENTATION.md).

## Use real Workspace sign-in

Follow [docs/OPERATIONS.md](docs/OPERATIONS.md) to create separate runtime/migration credentials, migrate the database, bootstrap the first administrator, configure Google's callback, and enable HTTPS. Put local secrets in ignored `.env`; use the pinned Varlock CLI with `cd web && npm exec -- varlock run --path .. -- ../bin/server`. Go validates configuration independently. Production reads environment variables directly.

Use `make build`, then run `bin/server` with the required environment. The app rejects migration credentials in its environment and privileged database roles at startup. The Render blueprint deliberately requires a manually supplied **restricted runtime URL**; it never wires the database owner's URL into the web service.

## Where things live

- `internal/requests`, `internal/exports`, `internal/auth`: application behavior and policy checks.
- `internal/server`: routes, HTTP limits, security headers, and shutdown.
- `db/queries`, `db/migrations`: typed SQL and explicit database boundaries.
- `templates`, `components`: server UI and copied shadcn-templ sources.
- `web/src`: the lazy-loaded React dashboard and island lifecycle.
- `internal/dashboard/props.go`: the deliberately small frontend contract; tygo generates TypeScript.

Replace the example workflow in a downstream app. Keep the security boundaries and acceptance checks. See [SPEC](docs/SPEC.md), [security boundaries](SECURITY.md), and [copied-source notices](THIRD_PARTY_NOTICES.md).

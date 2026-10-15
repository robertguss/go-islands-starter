# Working on this starter

Read docs/SPEC.md and docs/IMPLEMENTATION.md first. Keep this generic: one organization per app, synthetic fixtures, no institutional integrations.

- Start with codebase-memory-mcp/CodeScent when available. Fall back to ast-grep for syntax discovery and rg for strings/configuration.
- Use `make setup`, `make build`, `make test`, `make browser`, `make check`, `make security`, and `make container`. Tests need the dedicated Compose database. See README.md before running acceptance tests.
- Protected queries live in db/queries and use sqlc through Store.WithActor. Never query domain tables from an HTTP handler using a raw pool. The actor comes from a verified session or a permitted job argument.
- Runtime DB credentials must be non-owner, non-superuser, NOINHERIT, and NOBYPASSRLS. Migration credentials never enter the server environment. Role changes go through the narrowly scoped app.manage_user function.
- Recheck permissions in background jobs and downloads. Domain writes, audit, and River inserts share one transaction. No external effects are included; the synthetic destructive gate is off by default.
- Keep npm and browser source under web/, except committed shadcn-templ component scripts under components/. Do not install npm packages at the repository root.
- Pin exact dependencies, Go tools, container digests, and action SHAs. Preserve npm ignore-scripts and the seven-day release-age policy. Review dependency, policy, migration, copied component, and CI changes. An urgent exception needs a named package/version, advisory, rationale, and removal date in docs/IMPLEMENTATION.md.
- Commit generated SQL, templ Go files, TypeScript DTOs, and the component manifest. Build web/dist; never commit it. Run `make generate` and inspect generated changes. Never hand-edit generated output.
- htmx owns server fragments; React owns island children. Unmount before replacement. No eval, response script execution, private browser history snapshots, or sensitive localStorage entries. Theme is the only persisted browser setting.
- Log only route patterns, status, duration, correlation IDs, and fixed safe error codes. No request bodies, tokens, emails, OIDC codes, raw SQL errors, or database URLs.
- Follow the existing small feature packages. Avoid generic service/repository wrappers, plugins, speculative integrations, or extra frameworks.
- Verify with real runtime-role Postgres tests and browser acceptance, not just compilation. Keep account-backed Workspace/Render checks explicitly pending until actually performed. Never claim compliance certification.

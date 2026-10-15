# Dependency decisions

Pinned on 2026-10-15. Every direct Go dependency is recorded in go.mod/go.sum; every direct browser/build package uses an exact version in web/package.json and the full tree is locked in web/package-lock.json.

- Go 1.27.2 and Node 24.19.0/npm 11.17.0 are the supported toolchains.
- shadcn-templ v2.0.0-beta.15 is intentional. See THIRD_PARTY_NOTICES.md for source provenance and update boundaries.
- React 19.3.0, TanStack Table 8.21.3, and Recharts 3.10.1 implement the one overview island. htmx 2.0.11 handles server fragments.
- Vite 8.1.6, TypeScript 7.0.2, and Tailwind 4.3.0 build assets. Tailwind's newer CLI pinned the vulnerable watcher; 4.3.0 allows watcher 2.6.0, removing the braces advisory. No release-age exception was required.
- Playwright 1.64.0 and axe 4.13.0 provide browser acceptance. Varlock 1.21.1 is a local secret/schema helper.
- golangci-lint 2.14.0 runs lint and its bundled gosec 2.29.0 analyzer. govulncheck 1.9.0 checks reachable Go vulnerabilities. Scanner invocations are versioned in Makefile rather than mixed into runtime imports.

npm's [min-release-age setting](https://docs.npmjs.com/cli/v11/using-npm/config/#min-release-age) uses **days**; web/.npmrc enforces seven. All installs disable lifecycle scripts. CI uses a fresh cache with the [Socket Firewall action](https://github.com/SocketDev/action), pinned to a commit and firewall v1.15.4. It needs no production credentials. No auto-update or auto-merge workflow is included.

Dependency, generator, copied-source, action, and image changes should be reviewed as code. Pin a new exact version/digest, inspect the lockfile diff, regenerate, and run build, tests, browser acceptance, and security checks. For an urgent security fix inside the age window, record the exact package/version, advisory, rationale, narrow exclusion, owner review, and removal date; never disable the policy globally.

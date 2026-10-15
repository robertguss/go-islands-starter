# Security boundaries

This starter is intended for a trusted application server, a restricted database login, admitted Workspace users, and one organization per deployed app. It does not establish regulatory compliance for a downstream application.

Report vulnerabilities privately through this repository's GitHub security advisory feature. Do not put credentials, private records, or reproduction databases in public issues.

## What is enforced

- Google OIDC verifies signatures, issuer, audience, expiry, state, nonce, PKCE, verified email, and the Workspace `hd` claim. Domain membership alone grants no app access.
- Sessions are random opaque tokens with SHA-256 digests in Postgres, secure HttpOnly host cookies, 30-minute idle expiry, and eight-hour absolute expiry. Admission changes revoke sessions. Logout is POST-only.
- Application permissions and forced Postgres RLS protect requests, exports, and audit events. Actor context is transaction-local and protected queries use that same transaction. The runtime cannot own protected tables or assume their owner's role.
- An access administrator can admit people and assign roles through one constrained SQL function. They cannot change their own access or see workflow data without a separately assigned role. The function has a fixed search path, no dynamic SQL, an explicit permission check, and no PUBLIC execute grant.
- State changes, audit events, and River enqueueing are atomic. Jobs reauthorize before producing results, retries are idempotent, and downloads reauthorize the original row set.
- Private HTML, fragments, and exports are no-store. htmx history caching, eval features, and response scripts are disabled. React props are escaped JSON in an attribute. localStorage contains only theme preference.
- Cross-origin browser mutations are rejected by Go's CrossOriginProtection. Production requires HTTPS and secure cookies. Proxy forwarding headers are not trusted for rate-limit identities.
- Script CSP permits self-hosted scripts only. Style attributes are allowed for the copied dialog and Recharts geometry; stylesheet elements remain restricted to self. No inline/eval scripts are allowed.

## Explicit trust and operational limits

The app's actor setting is a trusted server assertion. RLS cannot stop a compromised server that holds runtime credentials from asserting another admitted actor. Identity/session tables and River queue tables have narrowly documented infrastructure grants; they are not protected by domain RLS. The database owner can alter policy and audit history.

Audit is application-level append-only, not tamper-proof. Export data expires after one hour, but an operator must run retention cleanup to erase expired bytes from storage. Backups may retain older data according to the deployment's retention policy. See docs/OPERATIONS.md.

Per-process rate limits and two workers target one app instance. Behind a shared reverse proxy, address limits may aggregate users. Do not trust arbitrary X-Forwarded-For headers to work around this; establish a verified proxy boundary first.

Exports are capped at 200 rows and 1 MiB and use a tested spreadsheet formula-escaping policy. Do not reuse this implementation for unbounded datasets. Add external storage and streaming only when downstream needs justify it.

No external destructive integration exists. The synthetic operation requires an access administrator and a default-off configuration flag. A downstream integration still needs its own permissions, idempotency/reconciliation, and acceptance checks.

Dependencies, copied beta components, migrations, database grants, and CI changes need human review. Required CI is authoritative; instructions alone do not enforce these boundaries. Real Workspace sign-in and hosted proxy behavior require account-backed acceptance before release.

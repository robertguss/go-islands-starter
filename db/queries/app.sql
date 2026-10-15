-- name: SetActor :exec
SELECT set_config('app.actor_id', sqlc.arg(actor_id)::text, true);

-- name: CurrentUser :one
SELECT * FROM app.users WHERE id = app.actor() AND NOT disabled;

-- name: LinkIdentity :one
UPDATE app.users SET subject = sqlc.arg(subject)::text
WHERE email = sqlc.arg(email)::text AND NOT disabled
  AND (subject IS NULL OR subject = sqlc.arg(subject)::text)
RETURNING *;

-- name: CreateSession :exec
INSERT INTO app.sessions (token_hash, user_id) VALUES ($1, $2);

-- name: SessionUser :one
SELECT u.* FROM app.sessions s JOIN app.users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.expires_at > now()
  AND s.touched_at > now() - interval '30 minutes' AND NOT u.disabled;

-- name: TouchSession :exec
UPDATE app.sessions SET touched_at = now() WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM app.sessions WHERE token_hash = $1;

-- name: CreateLoginAttempt :exec
INSERT INTO app.login_attempts (state_hash, nonce, verifier) VALUES ($1,$2,$3);

-- name: ConsumeLoginAttempt :one
DELETE FROM app.login_attempts WHERE state_hash = $1 AND expires_at > now() RETURNING *;

-- name: ListRequests :many
SELECT * FROM app.requests
WHERE (sqlc.arg(search)::text = '' OR title ILIKE '%' || sqlc.arg(search)::text || '%')
ORDER BY created_at DESC, id LIMIT 200;

-- name: CreateRequest :one
INSERT INTO app.requests (title, description) VALUES ($1, $2) RETURNING *;

-- name: ReviewRequest :one
UPDATE app.requests SET status = sqlc.arg(status)::text, reviewed_by = app.actor(), reviewed_at = now()
WHERE id = sqlc.arg(id)::uuid AND status = 'pending' AND owner_id <> app.actor()
RETURNING *;

-- name: RequestsByID :many
SELECT * FROM app.requests WHERE id = ANY(sqlc.arg(ids)::uuid[]) ORDER BY created_at, id;

-- name: CreateExport :one
INSERT INTO app.exports (request_ids) VALUES ($1) RETURNING *;

-- name: GetExport :one
SELECT * FROM app.exports WHERE id = $1 FOR UPDATE;

-- name: ListExports :many
SELECT id, owner_id, state, created_at, expires_at, job_id FROM app.exports ORDER BY created_at DESC LIMIT 20;

-- name: SetExportJob :exec
UPDATE app.exports SET job_id = $2 WHERE id = $1;

-- name: ManageUser :exec
SELECT app.manage_user(sqlc.arg(email)::text, sqlc.arg(display_name)::text, sqlc.arg(requester)::boolean, sqlc.arg(reviewer)::boolean, sqlc.arg(access_admin)::boolean, sqlc.arg(disabled)::boolean, sqlc.arg(correlation)::text);

-- name: ListUsers :many
SELECT id, email, display_name, requester, reviewer, access_admin, disabled FROM app.users WHERE app.permitted('access_admin') ORDER BY email LIMIT 200;

-- name: CompleteExport :exec
UPDATE app.exports SET state = 'complete', content = $2 WHERE id = $1 AND state = 'pending';

-- name: FailExport :exec
UPDATE app.exports SET state = 'failed', content = NULL WHERE id = $1 AND state = 'pending';

-- name: AuditRequest :exec
INSERT INTO app.audit_events(action, request_id, correlation_id) VALUES ($1,$2,$3);

-- name: AuditExport :exec
INSERT INTO app.audit_events(action, export_id, correlation_id) VALUES ($1,$2,$3);

-- name: ListAudit :many
SELECT * FROM app.audit_events ORDER BY created_at DESC, id DESC LIMIT 50;

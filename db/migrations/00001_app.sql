-- +goose Up
CREATE SCHEMA app;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

CREATE TABLE app.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL UNIQUE,
    subject text UNIQUE,
    display_name text NOT NULL,
    requester boolean NOT NULL DEFAULT false,
    reviewer boolean NOT NULL DEFAULT false,
    access_admin boolean NOT NULL DEFAULT false,
    disabled boolean NOT NULL DEFAULT false
);

CREATE TABLE app.sessions (
    token_hash bytea PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES app.users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    touched_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '8 hours'
);

-- +goose StatementBegin
CREATE FUNCTION app.actor() RETURNS uuid LANGUAGE sql STABLE AS $$
    SELECT nullif(current_setting('app.actor_id', true), '')::uuid
$$;
CREATE FUNCTION app.permitted(role_name text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT EXISTS (SELECT 1 FROM app.users WHERE id = app.actor() AND NOT disabled AND
        CASE role_name WHEN 'requester' THEN requester WHEN 'reviewer' THEN reviewer
        WHEN 'access_admin' THEN access_admin ELSE false END)
$$;
-- +goose StatementEnd

CREATE TABLE app.requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL DEFAULT app.actor() REFERENCES app.users(id),
    title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
    description text NOT NULL CHECK (char_length(description) <= 2000),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_by uuid REFERENCES app.users(id),
    reviewed_at timestamptz,
    CHECK ((status = 'pending' AND reviewed_by IS NULL AND reviewed_at IS NULL) OR
           (status <> 'pending' AND reviewed_by IS NOT NULL AND reviewed_at IS NOT NULL AND reviewed_by <> owner_id))
);
CREATE INDEX ON app.requests(owner_id, created_at DESC);
ALTER TABLE app.requests ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.requests FORCE ROW LEVEL SECURITY;
CREATE POLICY requests_read ON app.requests FOR SELECT USING (
    (owner_id = app.actor() AND app.permitted('requester')) OR app.permitted('reviewer'));
CREATE POLICY requests_create ON app.requests FOR INSERT WITH CHECK (
    owner_id = app.actor() AND app.permitted('requester') AND status = 'pending');
CREATE POLICY requests_review ON app.requests FOR UPDATE
    USING (app.permitted('reviewer') AND owner_id <> app.actor() AND status = 'pending')
    WITH CHECK (app.permitted('reviewer') AND owner_id <> app.actor() AND reviewed_by = app.actor() AND status IN ('approved','rejected'));

CREATE TABLE app.exports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL DEFAULT app.actor() REFERENCES app.users(id),
    request_ids uuid[] NOT NULL CHECK (cardinality(request_ids) BETWEEN 1 AND 200),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','complete','failed')),
    content bytea CHECK (octet_length(content) <= 1048576),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '1 hour'
);
ALTER TABLE app.exports ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.exports FORCE ROW LEVEL SECURITY;
-- Owner visibility also lets a revoked user see that their export failed, but never download it.
CREATE POLICY exports_owner ON app.exports USING (
    owner_id = app.actor() AND EXISTS (SELECT 1 FROM app.users WHERE id = app.actor() AND NOT disabled))
    WITH CHECK (owner_id = app.actor() AND EXISTS (SELECT 1 FROM app.users WHERE id = app.actor() AND NOT disabled));

CREATE TABLE app.audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id uuid NOT NULL DEFAULT app.actor() REFERENCES app.users(id),
    action text NOT NULL CHECK (action IN ('request.created','request.approved','request.rejected','export.requested','export.completed','export.failed')),
    request_id uuid REFERENCES app.requests(id),
    export_id uuid REFERENCES app.exports(id),
    correlation_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(request_id, export_id) = 1)
);
ALTER TABLE app.audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE app.audit_events FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_read ON app.audit_events FOR SELECT USING (
    EXISTS (SELECT 1 FROM app.requests r WHERE r.id = request_id) OR
    EXISTS (SELECT 1 FROM app.exports e WHERE e.id = export_id));
CREATE POLICY audit_insert ON app.audit_events FOR INSERT WITH CHECK (
    actor_id = app.actor() AND (EXISTS (SELECT 1 FROM app.requests r WHERE r.id = request_id) OR
    EXISTS (SELECT 1 FROM app.exports e WHERE e.id = export_id)));

GRANT USAGE ON SCHEMA app TO islands_app;
GRANT SELECT ON app.users TO islands_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON app.sessions TO islands_app;
GRANT SELECT, INSERT ON app.requests TO islands_app;
GRANT UPDATE(status, reviewed_by, reviewed_at) ON app.requests TO islands_app;
GRANT SELECT, INSERT ON app.exports TO islands_app;
GRANT UPDATE(state, content) ON app.exports TO islands_app;
GRANT SELECT, INSERT ON app.audit_events TO islands_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA app TO islands_app;

-- Only the OIDC subject may be linked by the application, after admission and token validation.
GRANT UPDATE(subject) ON app.users TO islands_app;

-- +goose Down
DROP SCHEMA app CASCADE;

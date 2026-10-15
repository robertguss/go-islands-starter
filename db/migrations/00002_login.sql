-- +goose Up
CREATE TABLE app.login_attempts (
    state_hash bytea PRIMARY KEY,
    nonce text NOT NULL,
    verifier text NOT NULL,
    expires_at timestamptz NOT NULL DEFAULT now() + interval '10 minutes'
);
GRANT INSERT, DELETE, SELECT ON app.login_attempts TO islands_app;

-- +goose Down
DROP TABLE app.login_attempts;

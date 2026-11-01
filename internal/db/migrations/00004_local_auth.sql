-- Local accounts and revocable sessions (ADR-0004).
-- +goose Up
CREATE TABLE auth_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    initialized boolean NOT NULL DEFAULT false
);
INSERT INTO auth_state (singleton) VALUES (true);

CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    email_canonical text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin','user')),
    disabled boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_activity timestamptz NOT NULL DEFAULT now(),
    absolute_expiry timestamptz NOT NULL,
    revoked_at timestamptz
);
CREATE INDEX auth_sessions_user_idx ON auth_sessions (user_id);

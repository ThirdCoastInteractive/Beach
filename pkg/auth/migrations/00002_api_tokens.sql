-- +goose Up
-- Idempotent DDL (IF NOT EXISTS); see 00001_identity.sql for why.

-- API/bearer tokens for non-interactive principals (service accounts, CLIs).
-- A token has the shape "<prefix>.<secret>": the prefix is a public, indexed
-- lookup handle; the secret is the bearer credential. token_hash is
-- SHA256(secret) and is a secret column — neither the raw secret nor the hash
-- may appear in a RETURNING clause or SELECT *. Tokens resolve to a user, whose
-- roles/permissions are resolved exactly as for an interactive login.
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS api_tokens (
    prefix      text        PRIMARY KEY,          -- public lookup handle
    token_hash  bytea       NOT NULL,             -- SHA256(secret); raw secret never stored
    user_id     bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        text        NOT NULL DEFAULT '',  -- human label for the token
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz,                      -- NULL means no expiry
    revoked_at  timestamptz,                      -- non-NULL once revoked
    last_used_at timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS api_tokens_user_id_idx ON api_tokens (user_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE api_tokens;
-- +goose StatementEnd

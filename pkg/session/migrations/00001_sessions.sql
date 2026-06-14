-- +goose Up
-- +goose StatementBegin
CREATE TABLE sessions (
    token_hash  bytea       PRIMARY KEY,         -- SHA256(token); raw token never stored
    user_id     bigint      NOT NULL,
    roles       text[]      NOT NULL DEFAULT '{}',
    csrf_secret bytea       NOT NULL,
    expires_at  timestamptz NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sessions;
-- +goose StatementEnd

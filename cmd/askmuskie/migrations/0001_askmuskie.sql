-- +goose Up
-- Idempotent DDL (IF NOT EXISTS); pg.Migrate keys this source's goose version
-- table off the first filename, so adding migrations later won't orphan history.
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS askmuskie_players (
    user_id         bigint      PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    tos_accepted_at timestamptz,
    shape           text        NOT NULL DEFAULT '',
    body_color      integer     NOT NULL DEFAULT -1, -- -1 = unset (derive from id)
    skin_color      integer     NOT NULL DEFAULT -1,
    wisdom          integer     NOT NULL DEFAULT 0,
    fed             integer     NOT NULL DEFAULT 0,
    chats           integer     NOT NULL DEFAULT 0,
    updated_at      timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS askmuskie_chat (
    id         bigserial   PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    username   text        NOT NULL,
    body       text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS askmuskie_chat_created_idx ON askmuskie_chat (created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS askmuskie_chat;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS askmuskie_players;
-- +goose StatementEnd

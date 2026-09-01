-- The driftbottle transcript ARCHIVE. The live matchmaking and the rolling
-- feed are in-memory on the hot path; these tables are written OFF the hot path
-- (a background persistLoop drains a buffered channel) and are NEVER read on the
-- hot path. They exist so a transcript survives the process, not to drive the UI.

-- +goose Up
-- +goose StatementBegin
-- One row per pairing: the two anonymous session ids and the private pair topic
-- that keyed their conversation. ended_at is stamped when either side leaves.
CREATE TABLE db_pairings (
    id         bigserial   PRIMARY KEY,
    pair_topic text        NOT NULL,
    a_sid      text        NOT NULL,
    b_sid      text        NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at   timestamptz
);
-- +goose StatementEnd

-- +goose StatementBegin
-- One row per message, keyed by the pair topic so a transcript reads back in
-- order. body is the cleaned/filtered text that was actually fanned out.
CREATE TABLE db_messages (
    id         bigserial   PRIMARY KEY,
    pair_topic text        NOT NULL,
    from_sid   text        NOT NULL,
    body       text        NOT NULL,
    ts         timestamptz NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose StatementBegin
-- The transcript read shape: one pairing's messages, oldest first.
CREATE INDEX db_messages_pair_ts_idx ON db_messages (pair_topic, ts);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE db_messages;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE db_pairings;
-- +goose StatementEnd

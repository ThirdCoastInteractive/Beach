-- +goose Up
-- Assent records: one row every time someone checks a legal checkbox — the
-- 18+/terms agreement at the three signup doors (kind 'tos-18') and the
-- 18 U.S.C. § 2257 certification on every image upload (kind '2257').
-- A checkbox you can't prove was checked is worthless: the row carries who,
-- from where, with what browser, and when.
CREATE TABLE hush_agreements (
    id         bigserial   PRIMARY KEY,
    handle     text        NOT NULL DEFAULT '',  -- '' in database-less open mode
    kind       text        NOT NULL,             -- tos-18 | 2257
    ip         text        NOT NULL DEFAULT '',
    user_agent text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX hush_agreements_handle_idx ON hush_agreements (handle, kind, created_at);

-- +goose Down
DROP TABLE hush_agreements;

-- +goose Up
-- Applications are the gate: one pending row per signup request. On approval an
-- auth `users` row (+ local credential + member role) is created from it; the
-- password the applicant chose is held hashed until then.
CREATE TABLE hush_applications (
    id            bigserial   PRIMARY KEY,
    handle        text        NOT NULL,
    pitch         text        NOT NULL,
    password_hash text        NOT NULL,
    photo_key     text        NOT NULL DEFAULT '',
    status        text        NOT NULL DEFAULT 'pending', -- pending | approved | rejected
    created_at    timestamptz NOT NULL DEFAULT now(),
    decided_at    timestamptz
);

CREATE INDEX hush_applications_status_idx ON hush_applications (status, created_at);

-- +goose Down
DROP TABLE hush_applications;
